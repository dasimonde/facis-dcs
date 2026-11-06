package query

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/event/outboxtest"
	"digital-contracting-service/internal/signingmanagement/db"
)

type validateRepoFake struct {
	db.ContractRepo
	findingsErr error
}

func (r validateRepoFake) ReadProcessDataByDID(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	return &db.ContractProcessData{ContractVersion: 3}, nil
}
func (r validateRepoFake) CollectValidationFindings(context.Context, *sqlx.Tx, string) ([]string, error) {
	return []string{"Signature of did:web:a is valid"}, r.findingsErr
}
func (r validateRepoFake) LoadSignatures(context.Context, *sqlx.Tx, string) ([]db.SignatureRecord, error) {
	return []db.SignatureRecord{{SignerDID: "did:web:a", Status: "SIGNED"}, {SignerDID: "did:web:b", Status: "REVOKED"}}, nil
}

func validateQry() ValidateQry {
	return ValidateQry{DID: "did:web:example.test:contracts:1", ValidatedBy: "participant-1"}
}

func committedValidateEvent(t *testing.T, rec *outboxtest.Recorder) outboxtest.Event {
	t.Helper()
	events := rec.Committed()
	if len(events) != 1 {
		t.Fatalf("committed outbox events = %+v, want exactly one", events)
	}
	return events[0]
}

func TestValidatorAuditsSignersAndOutcome(t *testing.T) {
	t.Setenv("DSS_URL", "")
	sqlDB, rec := outboxtest.Open(t)
	validator := Validator{DB: sqlDB, CRepo: validateRepoFake{}}

	if _, err := validator.Handle(context.Background(), validateQry()); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	evt := committedValidateEvent(t, rec)
	if evt.Type != "VALIDATE_SIGNATURE" {
		t.Fatalf("event type = %q, want VALIDATE_SIGNATURE", evt.Type)
	}
	if evt.Data["outcome"] != "COMPLETED" || evt.Data["validated_by"] != "participant-1" || evt.Data["occurred_at"] == nil {
		t.Fatalf("event data = %#v, want outcome COMPLETED, actor and timestamp", evt.Data)
	}
	signers, _ := evt.Data["signer_dids"].([]any)
	if len(signers) != 2 || signers[0] != "did:web:a" || signers[1] != "did:web:b" {
		t.Errorf("signer_dids = %#v, want both signers", evt.Data["signer_dids"])
	}
	findings, _ := evt.Data["findings"].([]any)
	if len(findings) != 1 || findings[0] != "Signature of did:web:a is valid" {
		t.Errorf("findings = %#v, want the validation result", evt.Data["findings"])
	}
}

// A failed validation rolls back its VALIDATE_SIGNATURE entry, so the failure
// must commit an entry of its own.
func TestValidatorAuditsFailedAttempts(t *testing.T) {
	t.Setenv("DSS_URL", "")
	sqlDB, rec := outboxtest.Open(t)
	cause := errors.New("database unavailable")
	validator := Validator{DB: sqlDB, CRepo: validateRepoFake{findingsErr: cause}}

	_, err := validator.Handle(context.Background(), validateQry())
	if !errors.Is(err, cause) {
		t.Fatalf("Handle() error = %v, want %v", err, cause)
	}

	evt := committedValidateEvent(t, rec)
	if evt.Type != "VALIDATE_SIGNATURE_FAILED" {
		t.Fatalf("event type = %q, want VALIDATE_SIGNATURE_FAILED", evt.Type)
	}
	if evt.Data["outcome"] != "FAILED" || evt.Data["error"] != err.Error() || evt.Data["validated_by"] != "participant-1" {
		t.Fatalf("event data = %#v, want outcome FAILED, error and actor", evt.Data)
	}
}

type storedPDFRepoFake struct{ db.ContractRepo }

func (storedPDFRepoFake) FetchContractPDFBytes(context.Context, *sqlx.Tx, string) ([]byte, error) {
	return []byte("%PDF signed"), nil
}

// DCS-FR-SM-02 (ADR-40): the compliance viewer checks a contract that records
// a QES against the QES criteria. A QES whose certificate expired without
// long-term validation data (INDETERMINATE/OUT_OF_BOUNDS_NO_POE) passes the
// AES criteria, so only this check shows it.
func TestValidateWithDSSReportsALostQES(t *testing.T) {
	signature := func(indication, sub string) string {
		return `{"simpleReport":{"signatureOrTimestampOrEvidenceRecord":[{"Signature":{"Indication":"` + indication +
			`","SubIndication":"` + sub + `","SignedBy":"CN=Jane Doe","SignatureQualification":"QESig"}}]}}`
	}
	for name, tc := range map[string]struct {
		report    string
		qualified int
		lost      bool
	}{
		"QES without long-term validation data": {signature("INDETERMINATE", "OUT_OF_BOUNDS_NO_POE"), 1, true},
		"QES still valid":                       {signature("TOTAL-PASSED", ""), 1, false},
		"AES contract":                          {signature("INDETERMINATE", "OUT_OF_BOUNDS_NO_POE"), 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.report))
			}))
			defer srv.Close()
			t.Setenv("DSS_URL", srv.URL)

			_, findings, err := (&Validator{CRepo: storedPDFRepoFake{}}).validateWithDSS(context.Background(), nil, "did:web:example.test:contracts:1", tc.qualified)
			if err != nil {
				t.Fatalf("validateWithDSS: %v", err)
			}
			lost := false
			for _, finding := range findings {
				if strings.Contains(finding, "validate as QES") {
					lost = true
				}
			}
			if lost != tc.lost {
				t.Fatalf("findings = %q, want a lost QES finding: %v", findings, tc.lost)
			}
		})
	}
}
