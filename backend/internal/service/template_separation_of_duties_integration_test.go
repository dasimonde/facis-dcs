package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

const sodTemplateData = `{
  "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/"},
  "@type": "dcs:ContractTemplate",
  "dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "SoD", "dcs:templateType": "dcs:ContractTemplate"},
  "dcs:documentStructure": {
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {"@list": [{"@id": "urn:uuid:block-clause-1", "@type": "dcs:Clause", "dcs:content": {"@list": ["Text"]}}]},
    "dcs:layout": [{"@id": "urn:uuid:block-root", "dcs:isRoot": true, "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]}}]
  }
}`

// DCS-FR-TR-06: the Template Manager neither reviews nor approves a template.
func TestTemplateManagerDoesNotReviewOrApprovePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo, rtRepo, atRepo := &pg.PostgresContractTemplateRepo{}, &pg.PostgresReviewTaskRepo{}, &pg.PostgresApprovalTaskRepo{}
	submitter := &command.Submitter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}
	verifier := &query.Verifier{DB: database, CTRepo: ctRepo, RTRepo: rtRepo}
	future := time.Now().Add(time.Hour)
	const org = "did:web:org.example"
	manager := userrole.UserRoles{userrole.TemplateManager}

	seed := func(did, state string) {
		t.Helper()
		_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','n',$3)`, did, state, compliantTemplateData)
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO contract_templates_review_task (did,state,reviewer,created_by) VALUES ($1,'OPEN',$2,$2)`, did, org)
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ($1,'OPEN',$2,$2)`, did, org)
		require.NoError(t, err)
	}
	reviewTaskState := func(did string) string {
		var state string
		require.NoError(t, database.Get(&state, `SELECT state FROM contract_templates_review_task WHERE did=$1`, did))
		return state
	}

	seed("sod-submitted", "SUBMITTED")
	_, err := verifier.Handle(ctx, query.VerifyQry{DID: "sod-submitted", VerifiedBy: org, UserRoles: manager})
	require.NoError(t, err)
	require.Equal(t, "OPEN", reviewTaskState("sod-submitted"), "a manager's verification completed the review task")

	_, err = verifier.Handle(ctx, query.VerifyQry{DID: "sod-submitted", VerifiedBy: org, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
	require.NoError(t, err)
	require.Equal(t, "VERIFIED", reviewTaskState("sod-submitted"))

	approval := actionflag.Approval
	err = submitter.Handle(ctx, command.SubmitCmd{DID: "sod-submitted", UpdatedAt: future, SubmittedBy: org, ActionFlag: &approval, UserRoles: manager})
	require.Error(t, err, "a manager forwarded a submitted template to approval")

	seed("sod-reviewed", "REVIEWED")
	err = submitter.Handle(ctx, command.SubmitCmd{DID: "sod-reviewed", UpdatedAt: future, SubmittedBy: org, UserRoles: manager})
	require.Error(t, err, "a manager returned a reviewed template to review")

	var states []string
	require.NoError(t, database.Select(&states, `SELECT state FROM contract_templates WHERE did IN ('sod-submitted','sod-reviewed') ORDER BY did`))
	require.Equal(t, []string{"REVIEWED", "SUBMITTED"}, states)
}
