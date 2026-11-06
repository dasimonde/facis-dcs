package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"goa.design/goa/v3/security"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	pacqry "digital-contracting-service/internal/processauditandcompliance/query"
)

// DCS-FR-PACM-07: the clause report carries the credential metadata of the
// signatures over the contract.
func TestClauseReportCarriesCredentialMetadata(t *testing.T) {
	signedAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	subject, org := "CN=Jane Smith", "did:web:org-a"
	got := mapClauseReportCredentials([]pacqry.CredentialMetadata{{
		Party: org, SignerDID: "did:key:jane", SignatureStatus: "SIGNED", SignedAt: &signedAt,
		AchievedLevel: "AES", RequiredLevel: "QES", CertSubject: &subject, PoAOrganization: &org,
	}})
	if len(got) != 1 || got[0].Party != org || got[0].AchievedLevel != "AES" || got[0].RequiredLevel != "QES" ||
		got[0].SignedAt == nil || *got[0].SignedAt != "2026-10-01T09:00:00Z" ||
		got[0].SignerCertSubject == nil || *got[0].SignerCertSubject != subject || got[0].PoaOrganization == nil {
		t.Fatalf("credentials = %+v", got)
	}
}

// DCS-FR-PACM-05: a Compliance Officer investigates non-compliance, so the
// report and case file exports admit the role.
func TestComplianceOfficerMayExportReportsAndCaseFiles(t *testing.T) {
	stop := errors.New("stop after authorization")
	var required []string
	auth := func(ctx context.Context, _ string, scheme *security.JWTScheme) (context.Context, error) {
		required = scheme.RequiredScopes
		return ctx, stop
	}
	endpoints := map[string]func() error{
		"audit_report": func() error {
			_, err := pac.NewAuditReportEndpoint(nil, auth)(context.Background(), &pac.AuditReportPayload{})
			return err
		},
		"case_file": func() error {
			_, err := pac.NewCaseFileEndpoint(nil, auth)(context.Background(), &pac.CaseFilePayload{})
			return err
		},
	}
	for name, call := range endpoints {
		required = nil
		if err := call(); !errors.Is(err, stop) {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Contains(required, "Compliance Officer") {
			t.Errorf("%s admits %v, not the Compliance Officer", name, required)
		}
	}
}
