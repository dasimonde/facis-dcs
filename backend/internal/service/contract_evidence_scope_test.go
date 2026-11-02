package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"goa.design/goa/v3/security"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
)

// DCS-IR-CWE-02: the contract creator stores and reads evidence while creating
// the contract.
func TestContractCreatorMayStoreAndReadEvidence(t *testing.T) {
	stop := errors.New("stop")
	var required []string
	auth := func(ctx context.Context, _ string, scheme *security.JWTScheme) (context.Context, error) {
		required = scheme.RequiredScopes
		return ctx, stop
	}
	for name, call := range map[string]func() error{
		"store": func() error {
			_, err := cwe.NewStoreEndpoint(nil, auth)(context.Background(), &cwe.ContractStoreRequest{})
			return err
		},
		"list_evidence": func() error {
			_, err := cwe.NewListEvidenceEndpoint(nil, auth)(context.Background(), &cwe.ListEvidencePayload{})
			return err
		},
		"retrieve_evidence": func() error {
			_, err := cwe.NewRetrieveEvidenceEndpoint(nil, auth)(context.Background(), &cwe.RetrieveEvidencePayload{})
			return err
		},
	} {
		require.ErrorIs(t, call(), stop, name)
		require.Contains(t, required, "Contract Creator", name)
	}
}
