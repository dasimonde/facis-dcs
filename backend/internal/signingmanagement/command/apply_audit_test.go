package command

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/event/outboxtest"
	"digital-contracting-service/internal/signingmanagement/db"
)

type unreadableContractRepo struct{ db.ContractRepo }

var errUnreadable = errors.New("contract unreadable")

func (unreadableContractRepo) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	return nil, errUnreadable
}

// DCS-FR-SM-19: a refused prepare or submit rolls back its own transaction, so
// the refusal is audited in a transaction of its own, with the signer, the
// credential and the outcome. It has its own type, so it does not trigger the
// deployment an applied signature triggers.
func TestApplierAuditsRefusedSigningAttempts(t *testing.T) {
	cmd := ApplyCmd{
		DID: revokeContractDID, SignerDID: revokeSignerDID, FieldName: "signature-1", CeremonyID: "ceremony-1",
		CredentialType: "AES", AppliedBy: "participant-1",
	}
	for step, attempt := range map[string]func(*Applier) error{
		"PREPARE": func(h *Applier) error { _, err := h.Prepare(context.Background(), cmd); return err },
		"SUBMIT": func(h *Applier) error {
			return h.SubmitSignature(context.Background(), SubmitSignatureCmd{ApplyCmd: cmd, SignedPDF: []byte("%PDF")})
		},
	} {
		t.Run(step, func(t *testing.T) {
			sqlDB, rec := outboxtest.Open(t)
			err := attempt(&Applier{DB: sqlDB, CRepo: unreadableContractRepo{}})
			if err == nil {
				t.Fatal("the attempt must be refused")
			}

			evt := onlyEvent(t, rec)
			if evt.Type != "APPLY_SIGNATURE_FAILED" {
				t.Fatalf("event type = %q, want APPLY_SIGNATURE_FAILED", evt.Type)
			}
			for field, want := range map[string]any{
				"did": revokeContractDID, "signer_did": revokeSignerDID, "credential_type": "AES", "ceremony_id": "ceremony-1",
				"field_name": "signature-1", "step": step, "outcome": "FAILED", "error": err.Error(), "applied_by": "participant-1",
			} {
				if evt.Data[field] != want {
					t.Errorf("%s = %#v, want %#v", field, evt.Data[field], want)
				}
			}
			if evt.Data["occurred_at"] == nil {
				t.Error("occurred_at is missing")
			}
		})
	}
}
