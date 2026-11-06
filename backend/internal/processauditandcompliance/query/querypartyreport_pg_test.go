package qry_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/pgtest"
	qry "digital-contracting-service/internal/processauditandcompliance/query"
)

// DCS-FR-PACM-07: a counterparty that signed on its own instance is a signed
// party of the report, and the report generation is recorded.
func TestPartyReportIncludesTheSignaturesOfThePeerInstance(t *testing.T) {
	db := pgtest.Open(t)
	did := insertContract(t, db, "SIGNED", time.Now())
	if _, err := db.Exec(`UPDATE contracts SET contract_data = $2 WHERE did = $1`, did,
		`{"dcs:signatureFields":[{"dcs:signatoryName":"did:web:org-a"},{"dcs:signatoryName":"did:web:org-b"}]}`); err != nil {
		t.Fatalf("update contract: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, status, field_name)
        VALUES ($1, 'did:key:jane', 'AES', 'SIGNED', 'did:web:org-a')`, did); err != nil {
		t.Fatalf("insert signature: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contract_sync_signatures (did, contract_version, from_peer_did, jades_signature, poa_evidence)
        VALUES ($1, 1, 'did:web:org-b', 'h.p.s', '[{"party":"did:web:org-b","signatory":"did:key:bob","poa_organization":"did:web:org-b"}]')`, did); err != nil {
		t.Fatalf("insert peer signature: %v", err)
	}
	raw, err := (&qry.PartyReporter{DB: db}).Handle(context.Background(), qry.PartyReportQry{DID: did, GeneratedBy: "did:web:compliance"})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var report qry.PartyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != qry.StatusCompliant || len(report.Parties) != 2 || report.Parties[1].Status != qry.StatusCompliant ||
		report.Parties[1].Credential == nil || report.Parties[1].Credential.SignerDID != "did:key:bob" || report.Parties[1].Credential.Source != "peer" {
		t.Fatalf("report = %+v", report)
	}
	var generatedBy string
	if err := db.Get(&generatedBy, `SELECT event_data->>'generated_by' FROM outbox_events
        WHERE event_type = 'PAC_PARTY_REPORT_GENERATED' AND did = $1`, did); err != nil || generatedBy != "did:web:compliance" {
		t.Fatalf("report event generated_by %q, err %v", generatedBy, err)
	}
}
