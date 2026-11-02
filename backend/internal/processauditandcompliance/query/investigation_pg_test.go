package qry_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/pgtest"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	pacrepo "digital-contracting-service/internal/processauditandcompliance/db/pg"
	pacevent "digital-contracting-service/internal/processauditandcompliance/event"
	qry "digital-contracting-service/internal/processauditandcompliance/query"
)

func insertContract(t *testing.T, db *sqlx.DB, state string, startDate time.Time) string {
	t.Helper()
	did := "did:web:test:contract:" + uuid.NewString()
	if _, err := db.Exec(`INSERT INTO contracts (did, created_by, state, template_did, template_version, start_date)
        VALUES ($1, 'did:web:test', $2, 'did:web:test:template', 1, $3)`, did, state, startDate); err != nil {
		t.Fatalf("insert contract: %v", err)
	}
	return did
}

// ADR-38 §2: a signed copy refused as a signing conflict is a compliance risk
// on both instances, the one that refused it and the one whose copy it was.
func TestMonitorFlagsSigningConflictsOnBothSides(t *testing.T) {
	db := pgtest.Open(t)
	refused := insertContract(t, db, "SIGNED", time.Now().Add(48*time.Hour))
	ctx := context.Background()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{pacevent.SigningConflictReceived, pacevent.SigningConflictSent} {
		if err := pacevent.RecordSigningConflictTx(ctx, tx, refused, "did:web:peer.example", direction, "refused"); err != nil {
			t.Fatalf("record signing conflict: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	monitor := qry.ComplianceMonitor{DB: db, ATRepo: &cwerepo.PostgresApprovalTaskRepo{}, CRepo: &cwerepo.PostgresContractRepo{}, FRepo: &pacrepo.PostgresRiskFindingRepo{}}
	result, err := monitor.Handle(ctx, qry.MonitorQry{MonitoredBy: qry.SystemMonitorPrincipal})
	if err != nil {
		t.Fatalf("monitor: %v", err)
	}
	var details []string
	for _, risk := range result.Risks {
		if risk.DID == refused && risk.RiskType == qry.RiskTypeSigningConflict {
			details = append(details, risk.Detail)
		}
	}
	want := []string{
		"this instance refused the signed copy of did:web:peer.example: both instances hold a signature over the same revision",
		"did:web:peer.example refused the signed copy of this instance: both instances hold a signature over the same revision",
	}
	if len(details) != len(want) || details[0] != want[0] || details[1] != want[1] {
		t.Errorf("signing conflict risks = %q, want %q", details, want)
	}
}

// DCS-FR-PACM-05: the monitor flags late signatures and signatures that rely
// on a revoked certificate.
func TestMonitorFlagsLateSignaturesAndInvalidCredentials(t *testing.T) {
	db := pgtest.Open(t)
	late := insertContract(t, db, "APPROVED", time.Now().Add(-48*time.Hour))
	notYetDue := insertContract(t, db, "APPROVED", time.Now().Add(48*time.Hour))
	revoked := insertContract(t, db, "SIGNED", time.Now().Add(-48*time.Hour))
	if _, err := db.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, status, field_name, cert_revoked_at)
        VALUES ($1, 'did:key:signer', 'AES', 'SIGNED', 'did:web:org-a', now())`, revoked); err != nil {
		t.Fatalf("insert signature: %v", err)
	}

	monitor := qry.ComplianceMonitor{DB: db, ATRepo: &cwerepo.PostgresApprovalTaskRepo{}, CRepo: &cwerepo.PostgresContractRepo{}, FRepo: &pacrepo.PostgresRiskFindingRepo{}}
	result, err := monitor.Handle(context.Background(), qry.MonitorQry{MonitoredBy: qry.SystemMonitorPrincipal})
	if err != nil {
		t.Fatalf("monitor: %v", err)
	}
	found := map[string]string{}
	for _, risk := range result.Risks {
		if risk.DID == late || risk.DID == notYetDue || risk.DID == revoked {
			found[risk.DID] = risk.RiskType
		}
	}
	if found[late] != qry.RiskTypeLateSignature {
		t.Errorf("late contract: risk %q, want %s", found[late], qry.RiskTypeLateSignature)
	}
	if _, flagged := found[notYetDue]; flagged {
		t.Errorf("contract before its start date flagged as %s", found[notYetDue])
	}
	if found[revoked] != qry.RiskTypeInvalidCredential {
		t.Errorf("revoked certificate: risk %q, want %s", found[revoked], qry.RiskTypeInvalidCredential)
	}
}

// DCS-FR-PACM-05: an incident report opens a case, and the case file exports
// its findings and records the export with the justification.
func TestIncidentReportOpensACaseWithAnExportableCaseFile(t *testing.T) {
	db := pgtest.Open(t)
	did := insertContract(t, db, "SIGNED", time.Now())
	ctx := context.Background()

	caseID, err := (&qry.IncidentReporter{DB: db}).Handle(ctx, qry.IncidentReportQry{
		DID: did, ReportedBy: "did:web:compliance-officer",
		Findings: []qry.IncidentFinding{{RiskType: "LATE_SIGNATURE", Detail: "signed after the start date"}},
	})
	if err != nil || caseID == "" {
		t.Fatalf("incident report: case %q, err %v", caseID, err)
	}

	refused := errors.New("refused")
	denying := qry.CaseFileExporter{DB: db, Authorize: func(context.Context, string) error { return refused }}
	if _, err := denying.Handle(ctx, qry.CaseFileQry{CaseID: caseID, Justification: "x"}); !errors.Is(err, refused) {
		t.Fatalf("export without read access: err %v, want the authorization error", err)
	}
	exporter := qry.CaseFileExporter{DB: db, Authorize: func(context.Context, string) error { return nil }}
	raw, err := exporter.Handle(ctx, qry.CaseFileQry{CaseID: caseID, ExportedBy: "did:web:auditor", Justification: "regulator request"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var file qry.CaseFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("decode case file: %v", err)
	}
	if file.CaseID != caseID || file.DID != did || len(file.Findings) != 1 || file.Findings[0].RiskType != "LATE_SIGNATURE" {
		t.Fatalf("case file = %+v", file)
	}
	var justification string
	if err := db.Get(&justification, `SELECT event_data->>'justification' FROM outbox_events
        WHERE event_type = 'PAC_CASE_FILE_EXPORTED' AND event_data->>'case_id' = $1`, caseID); err != nil || justification != "regulator request" {
		t.Fatalf("export event justification %q, err %v", justification, err)
	}

	if _, err := exporter.Handle(ctx, qry.CaseFileQry{CaseID: uuid.NewString()}); !errors.Is(err, qry.ErrCaseNotFound) {
		t.Fatalf("unknown case: err %v, want ErrCaseNotFound", err)
	}
}

// DCS-FR-PACM-07: the party report reads the contract and its signatures
// with their ceremonies from the database.
func TestPartyReportReadsTheContractFromTheDatabase(t *testing.T) {
	db := pgtest.Open(t)
	did := insertContract(t, db, "SIGNED", time.Now())
	if _, err := db.Exec(`UPDATE contracts SET contract_data = $2 WHERE did = $1`, did,
		`{"dcs:signatureFields":[{"dcs:signatoryName":"did:web:org-a","dcs:requiredCredentialType":"QES"}]}`); err != nil {
		t.Fatalf("update contract: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, status, field_name)
        VALUES ($1, 'did:key:signer', 'AES', 'SIGNED', 'did:web:org-a')`, did); err != nil {
		t.Fatalf("insert signature: %v", err)
	}
	reporter := qry.PartyReporter{DB: db}
	raw, err := reporter.Handle(context.Background(), qry.PartyReportQry{DID: did})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var report qry.PartyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(report.Parties) != 1 || report.Parties[0].Status != qry.StatusNonCompliant ||
		report.Parties[0].Credential.AchievedLevel != "AES" || report.Parties[0].Credential.RequiredLevel != "QES" {
		t.Fatalf("report = %+v", report)
	}
	if _, err := reporter.Handle(context.Background(), qry.PartyReportQry{DID: "did:web:test:missing"}); !errors.Is(err, qry.ErrContractNotFound) {
		t.Fatalf("missing contract: err %v", err)
	}
}
