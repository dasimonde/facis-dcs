package command

import (
	"context"
	"errors"
	"testing"
)

func TestRevokerRejectsBlankReasonBeforeStartingTransaction(t *testing.T) {
	t.Parallel()

	revoker := Revoker{}
	for _, reason := range []string{"", " \t\n "} {
		err := revoker.Handle(context.Background(), RevokeCmd{
			DID:       "did:web:example.test:contracts:1",
			SignerDID: "did:web:example.test:signers:1",
			Reason:    reason,
		})
		if !errors.Is(err, ErrRevocationReasonRequired) {
			t.Fatalf("Handle() error = %v, want ErrRevocationReasonRequired", err)
		}
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
