package service

import (
	"context"
	"testing"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/event/outboxtest"
)

func TestRevokeRejectsBlankReasonBeforeCommandMutation(t *testing.T) {
	t.Parallel()

	sqlDB, _ := outboxtest.Open(t)
	service := &signatureManagementsrvc{DB: sqlDB}
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
