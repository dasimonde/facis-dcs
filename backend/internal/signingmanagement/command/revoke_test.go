package command

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/event/outboxtest"
	"digital-contracting-service/internal/signingmanagement/db"
)

const (
	revokeContractDID = "did:web:example.test:contracts:1"
	revokeSignerDID   = "did:web:example.test:signers:1"
)

type revokeRepoFake struct {
	db.ContractRepo
	revokeErr error
}

func (r revokeRepoFake) ReadProcessDataByDID(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	return &db.ContractProcessData{State: "SIGNED", ContractVersion: 2}, nil
}
func (r revokeRepoFake) RevokeSignature(context.Context, *sqlx.Tx, string, string) error {
	return r.revokeErr
}
func (r revokeRepoFake) UpdateState(context.Context, *sqlx.Tx, string, string, string) error {
	return nil
}

func revokeCmd(reason string) RevokeCmd {
	return RevokeCmd{DID: revokeContractDID, SignerDID: revokeSignerDID, Reason: reason, RevokedBy: "participant-1"}
}

func onlyEvent(t *testing.T, rec *outboxtest.Recorder) outboxtest.Event {
	t.Helper()
	events := rec.Committed()
	if len(events) != 1 {
		t.Fatalf("committed outbox events = %+v, want exactly one", events)
	}
	return events[0]
}

func TestRevokerAuditsSignerAndOutcome(t *testing.T) {
	sqlDB, rec := outboxtest.Open(t)
	revoker := Revoker{DB: sqlDB, CRepo: revokeRepoFake{}}

	if err := revoker.Handle(context.Background(), revokeCmd("Signer credential compromised")); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	evt := onlyEvent(t, rec)
	if evt.Type != "REVOKE_SIGNATURE" {
		t.Fatalf("event type = %q, want REVOKE_SIGNATURE", evt.Type)
	}
	for field, want := range map[string]any{
		"signer_did": revokeSignerDID, "outcome": "COMPLETED", "reason": "Signer credential compromised", "revoked_by": "participant-1",
	} {
		if evt.Data[field] != want {
			t.Errorf("%s = %#v, want %#v", field, evt.Data[field], want)
		}
	}
	if evt.Data["occurred_at"] == nil {
		t.Error("occurred_at is missing")
	}
}

// A failed attempt rolls back the revocation and its REVOKE_SIGNATURE entry, so
// the failure must commit an entry of its own. It has its own type because
// dcstodcs ships the contract to peers on REVOKE_SIGNATURE.
func TestRevokerAuditsFailedAttempts(t *testing.T) {
	for name, tc := range map[string]struct {
		reason string
		repo   revokeRepoFake
		want   error
	}{
		"blank reason":   {reason: " \t\n ", want: ErrRevocationReasonRequired},
		"unknown signer": {reason: "Signer credential compromised", repo: revokeRepoFake{revokeErr: db.ErrSignatureNotFound}, want: db.ErrSignatureNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			sqlDB, rec := outboxtest.Open(t)
			revoker := Revoker{DB: sqlDB, CRepo: tc.repo}

			err := revoker.Handle(context.Background(), revokeCmd(tc.reason))
			if !errors.Is(err, tc.want) {
				t.Fatalf("Handle() error = %v, want %v", err, tc.want)
			}

			evt := onlyEvent(t, rec)
			if evt.Type != "REVOKE_SIGNATURE_FAILED" {
				t.Fatalf("event type = %q, want REVOKE_SIGNATURE_FAILED", evt.Type)
			}
			if evt.Data["outcome"] != "FAILED" || evt.Data["signer_did"] != revokeSignerDID || evt.Data["error"] != err.Error() {
				t.Fatalf("event data = %#v, want outcome FAILED, signer and error", evt.Data)
			}
		})
	}
}

func TestNormalizeRevocationReasonTrimsOuterWhitespace(t *testing.T) {
	t.Parallel()

	reason, err := NormalizeRevocationReason(" \tSigner credential compromised\n")
	if err != nil {
		t.Fatalf("NormalizeRevocationReason() error = %v", err)
	}
	if reason != "Signer credential compromised" {
		t.Fatalf("NormalizeRevocationReason() = %q, want %q", reason, "Signer credential compromised")
	}
}
