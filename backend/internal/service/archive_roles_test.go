package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"goa.design/goa/v3/security"

	archive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
)

// requiredScopes returns the roles the generated endpoint admits: the
// security scheme it hands to the JWT check before the service runs.
func requiredScopes(t *testing.T, newEndpoint func(security.AuthJWTFunc) func(context.Context, any) (any, error), payload any) []string {
	t.Helper()
	var scopes []string
	stop := errors.New("stop after the scope check")
	endpoint := newEndpoint(func(ctx context.Context, _ string, scheme *security.JWTScheme) (context.Context, error) {
		scopes = scheme.RequiredScopes
		return ctx, stop
	})
	_, err := endpoint(context.Background(), payload)
	require.ErrorIs(t, err, stop)
	return scopes
}

// DCS-IR-CSA-03: the Archive Manager terminates archived contracts.
func TestArchiveManagerMayTerminateAContract(t *testing.T) {
	scopes := requiredScopes(t, func(auth security.AuthJWTFunc) func(context.Context, any) (any, error) {
		return contractworkflowengine.NewTerminateEndpoint(nil, auth)
	}, &contractworkflowengine.ContractTerminateRequest{})
	require.Contains(t, scopes, "Archive Manager")
}

// DCS-FR-CSA-24: the Auditor and the Compliance Officer inspect the archive
// entries, their compliance verdict, evidence and sections from the archive
// viewer. The archive writes stay with the Archive Manager.
func TestAuditorAndComplianceOfficerInspectTheArchive(t *testing.T) {
	reads := map[string]func(security.AuthJWTFunc) func(context.Context, any) (any, error){
		"retrieve": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewRetrieveEndpoint(nil, a)
		},
		"detail": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewDetailEndpoint(nil, a)
		},
		"search": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewSearchEndpoint(nil, a)
		},
		"compliance": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewComplianceEndpoint(nil, a)
		},
		"evidence": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewEvidenceEndpoint(nil, a)
		},
		"sections": func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
			return archive.NewSectionsEndpoint(nil, a)
		},
	}
	payloads := map[string]any{
		"retrieve": &archive.ArchiveRetrieveRequest{}, "detail": &archive.DetailPayload{}, "search": &archive.ArchiveSearchRequest{},
		"compliance": &archive.CompliancePayload{}, "evidence": &archive.EvidencePayload{}, "sections": &archive.SectionsPayload{},
	}
	for name, endpoint := range reads {
		scopes := requiredScopes(t, endpoint, payloads[name])
		require.Contains(t, scopes, "Auditor", name)
		require.Contains(t, scopes, "Compliance Officer", name)
	}
	scopes := requiredScopes(t, func(a security.AuthJWTFunc) func(context.Context, any) (any, error) {
		return archive.NewDeleteEndpoint(nil, a)
	}, &archive.DeletePayload{})
	require.Equal(t, []string{"Archive Manager"}, scopes)
}

// DCS-FR-CSA-24: the Auditor reads the contract content from the audit viewer.
func TestAuditorMayReadAContract(t *testing.T) {
	scopes := requiredScopes(t, func(auth security.AuthJWTFunc) func(context.Context, any) (any, error) {
		return contractworkflowengine.NewRetrieveByIDEndpoint(nil, auth)
	}, &contractworkflowengine.ContractRetrieveByIDRequest{})
	require.Contains(t, scopes, "Auditor")
}
