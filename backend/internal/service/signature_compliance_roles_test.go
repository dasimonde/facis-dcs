package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	goa "goa.design/goa/v3/pkg"
	"goa.design/goa/v3/security"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/event/outboxtest"
)

// DCS-IR-SM-05, DCS-IR-SM-06, DCS-IR-SM-07: the compliance users of the
// Signature Compliance Viewer (Auditor, Compliance Officer) validate, revoke
// and run the compliance check.
func TestComplianceRolesMayValidateRevokeAndRunCompliance(t *testing.T) {
	for name, endpoint := range map[string]struct {
		build   func(signaturemanagement.Service, security.AuthJWTFunc) goa.Endpoint
		payload any
	}{
		"validate":   {signaturemanagement.NewValidateEndpoint, &signaturemanagement.SMContractValidateRequest{}},
		"revoke":     {signaturemanagement.NewRevokeEndpoint, &signaturemanagement.SMContractRevokeRequest{}},
		"compliance": {signaturemanagement.NewComplianceEndpoint, &signaturemanagement.SMContractComplianceRequest{}},
	} {
		var required []string
		stop := errors.New("stop after the scope check")
		_, err := endpoint.build(nil, func(ctx context.Context, _ string, scheme *security.JWTScheme) (context.Context, error) {
			required = scheme.RequiredScopes
			return ctx, stop
		})(context.Background(), endpoint.payload)
		if !errors.Is(err, stop) {
			t.Fatalf("%s: the endpoint did not run the scope check: %v", name, err)
		}
		for _, role := range []string{"Auditor", "Compliance Officer"} {
			if !slices.Contains(required, role) {
				t.Errorf("%s does not admit %q: requires one of %v", name, role, required)
			}
		}
	}
}

// A revocation is scoped like every other signature action on a contract:
// a contract the caller may not read cannot have its signature revoked.
func TestRevokeRefusesAContractTheCallerMayNotRead(t *testing.T) {
	sqlDB, rec := outboxtest.Open(t)
	service := &signatureManagementsrvc{DB: sqlDB, DIDDocument: *jadesTestDIDDocument(t, "local")}
	_, err := service.Revoke(context.Background(), &signaturemanagement.SMContractRevokeRequest{
		Did: "did:web:example.test:contracts:1", SignerDid: "did:web:example.test:signers:1", Reason: "Signer credential compromised",
	})
	var named interface{ ErrorName() string }
	if !errors.As(err, &named) || (named.ErrorName() != "not_found" && named.ErrorName() != "forbidden") {
		t.Fatalf("Revoke() error = %v, want not_found or forbidden", err)
	}
	if events := rec.Committed(); len(events) != 0 {
		t.Fatalf("a refused read must not reach the revocation: %+v", events)
	}
}
