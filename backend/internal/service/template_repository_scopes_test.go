package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"goa.design/goa/v3/security"

	templaterepository "digital-contracting-service/gen/template_repository"
)

// templateScopeRecorder stops every endpoint at its JWT check and keeps the scopes it requires.
type templateScopeRecorder struct {
	templaterepository.Service
	required []string
}

func (r *templateScopeRecorder) JWTAuth(ctx context.Context, _ string, scheme *security.JWTScheme) (context.Context, error) {
	r.required = scheme.RequiredScopes
	return ctx, errors.New("stop")
}

func requiredTemplateScopes(t *testing.T, call func(*templaterepository.Endpoints) error) []string {
	t.Helper()
	recorder := &templateScopeRecorder{}
	if err := call(templaterepository.NewEndpoints(recorder)); err == nil {
		t.Fatal("endpoint passed the JWT check")
	}
	return recorder.required
}

// DCS-FR-TR-06: the Template Manager does not approve or reject; DCS-IR-TR-07: it reads the template audit.
func TestTemplateRepositoryScopes(t *testing.T) {
	ctx := context.Background()
	approve := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Approve(ctx, &templaterepository.ContractTemplateApproveRequest{})
		return err
	})
	reject := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Reject(ctx, &templaterepository.ContractTemplateRejectRequest{})
		return err
	})
	audit := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Audit(ctx, &templaterepository.ContractTemplateAuditRequest{})
		return err
	})
	for name, scopes := range map[string][]string{"approve": approve, "reject": reject} {
		if !slices.Equal(scopes, []string{"Template Approver"}) {
			t.Errorf("%s scopes = %v, want only Template Approver", name, scopes)
		}
	}
	if !slices.Contains(audit, "Template Manager") {
		t.Errorf("audit scopes = %v, want Template Manager", audit)
	}
}

// DCS-FR-TR-09, DCS-FR-TR-20: the template user (Contract Creator) verifies a template and its provenance.
func TestTemplateUserMayVerifyTemplates(t *testing.T) {
	ctx := context.Background()
	verify := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Verify(ctx, &templaterepository.ContractTemplateVerifyRequest{})
		return err
	})
	provenance := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Provenance(ctx, &templaterepository.TemplateProvenanceRetrieveRequest{})
		return err
	})
	for name, scopes := range map[string][]string{"verify": verify, "provenance": provenance} {
		if !slices.Contains(scopes, "Contract Creator") {
			t.Errorf("%s scopes = %v, want Contract Creator", name, scopes)
		}
	}
	// DCS-IR-TR-05: the approver reads the validation results before the decision.
	if !slices.Contains(verify, "Template Approver") {
		t.Errorf("verify scopes = %v, want Template Approver", verify)
	}
}
