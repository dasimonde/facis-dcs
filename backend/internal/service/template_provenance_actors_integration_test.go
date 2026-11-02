package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

// DCS-FR-TR-08, DCS-FR-TR-09: the provenance credential names the reviewer
// and the approver by the holder DIDs of the persons who decided, not by
// their organization.
func TestTemplateProvenanceNamesReviewerAndApproverPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo, rtRepo, atRepo := &pg.PostgresContractTemplateRepo{}, &pg.PostgresReviewTaskRepo{}, &pg.PostgresApprovalTaskRepo{}
	const org = "did:web:org.example"
	const creator, reviewer, approver, registrar = "did:key:creator", "did:key:reviewer", "did:key:approver", "did:key:registrar"

	data := datatype.JSON(compliantTemplateData)
	require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
		DID: "tpl", CreatedBy: org, HolderDID: creator, TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))
	submitter := &command.Submitter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}
	future := time.Now().Add(time.Hour)
	require.NoError(t, submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: org, HolderDID: creator, UserRoles: userrole.UserRoles{userrole.TemplateCreator}}))
	result, err := (&query.Verifier{DB: database, CTRepo: ctRepo, RTRepo: rtRepo}).Handle(ctx, query.VerifyQry{DID: "tpl", VerifiedBy: org, HolderDID: reviewer, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
	require.NoError(t, err)
	require.Empty(t, result.Findings)
	forward := actionflag.Approval
	require.NoError(t, submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: org, HolderDID: reviewer, ActionFlag: &forward, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}}))
	require.NoError(t, (&command.Approver{DB: database, CTRepo: ctRepo, ATRepo: atRepo}).Handle(ctx, command.ApproveCmd{DID: "tpl", UpdatedAt: future, ApprovedBy: org, HolderDID: approver, UserRoles: userrole.UserRoles{userrole.TemplateApprover}}))
	_, err = (&command.Registrar{DB: database, CTRepo: ctRepo, VCSigner: echoVCSigner{}, IssuerDID: "did:web:issuer.example"}).
		Handle(ctx, command.RegisterCmd{DID: "tpl", RegisteredBy: org, HolderDID: registrar, UserRoles: userrole.UserRoles{userrole.TemplateManager}})
	require.NoError(t, err)

	var raw string
	require.NoError(t, database.Get(&raw, `SELECT credential::text FROM template_provenance_credentials WHERE did = 'tpl'`))
	var credential struct {
		CredentialSubject struct {
			ReviewedBy         []string `json:"reviewed_by"`
			ApprovedBy         []string `json:"approved_by"`
			RegistrarHolderDID string   `json:"registrar_holder_did"`
		} `json:"credentialSubject"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &credential))
	require.Equal(t, []string{reviewer}, credential.CredentialSubject.ReviewedBy)
	require.Equal(t, []string{approver}, credential.CredentialSubject.ApprovedBy)
	require.Equal(t, registrar, credential.CredentialSubject.RegistrarHolderDID)
}
