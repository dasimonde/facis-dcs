package service

import (
	"context"
	"testing"

	signaturemanagement "digital-contracting-service/gen/signature_management"
)

func TestRevokeRejectsBlankReasonBeforeCommandMutation(t *testing.T) {
	t.Parallel()

	service := &signatureManagementsrvc{}
	for _, reason := range []string{"", " \t\n "} {
		_, err := service.Revoke(context.Background(), &signaturemanagement.SMContractRevokeRequest{
			Did:       "did:web:example.test:contracts:1",
			SignerDid: "did:web:example.test:signers:1",
			Reason:    reason,
		})
		if err == nil {
			t.Fatalf("Revoke() with reason %q returned nil error", reason)
		}
	}
}
