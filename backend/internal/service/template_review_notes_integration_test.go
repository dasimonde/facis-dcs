package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

// DCS-IR-TR-05: a rejection and the approver's resubmission state a reason.
func TestTemplateApproverDecisionsRequireAReasonPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	ctRepo, rtRepo, atRepo := &pg.PostgresContractTemplateRepo{}, &pg.PostgresReviewTaskRepo{}, &pg.PostgresApprovalTaskRepo{}
	_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ('tpl','creator','REVIEWED','CONTRACT_TEMPLATE','SINGLE_PARTY','n')`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ('tpl','OPEN','org','org')`)
	require.NoError(t, err)
	future := time.Now().Add(time.Hour)
	approver := userrole.UserRoles{userrole.TemplateApprover}

	err = (&command.Rejecter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}).Handle(ctx, command.RejectCmd{DID: "tpl", UpdatedAt: future, RejectedBy: "org", Reason: " ", UserRoles: approver})
	require.ErrorIs(t, err, command.ErrInvalidRequest)

	submitter := &command.Submitter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}
	err = submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: "org", Comments: []string{""}, UserRoles: approver})
	require.ErrorIs(t, err, command.ErrInvalidRequest)
	require.Equal(t, "bad_request", errorName(mapTemplateCommandError(err)))
	require.NoError(t, submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: "org", Comments: []string{"Clarify the liability clause"}, UserRoles: approver}))
}

// DCS-IR-TR-04, DCS-IR-TR-05: the creator and the approver read the review
// comments and the rejection reason with the template. DCS-FR-TR-19: a
// template user without a template role reads registered templates only.
func TestTemplateRetrieveByIDReviewNotesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo, rtRepo, atRepo := &pg.PostgresContractTemplateRepo{}, &pg.PostgresReviewTaskRepo{}, &pg.PostgresApprovalTaskRepo{}
	const org = "did:web:org.example"
	future := time.Now().Add(time.Hour)

	data := datatype.JSON(compliantTemplateData)
	require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
		DID: "tpl", CreatedBy: org, TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))
	submitter := &command.Submitter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}
	submit := func(holder string, roles userrole.UserRoles, flag *actionflag.ActionFlag, comments ...string) {
		t.Helper()
		require.NoError(t, submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: org, HolderDID: holder, UserRoles: roles, ActionFlag: flag, Comments: comments}))
	}
	verify := func() {
		t.Helper()
		_, err := (&query.Verifier{DB: database, CTRepo: ctRepo, RTRepo: rtRepo}).Handle(ctx, query.VerifyQry{DID: "tpl", VerifiedBy: org, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
		require.NoError(t, err)
	}
	creator, reviewer := userrole.UserRoles{userrole.TemplateCreator}, userrole.UserRoles{userrole.TemplateReviewer}
	toDraft, toApproval := actionflag.Draft, actionflag.Approval

	submit("did:key:creator", creator, nil)
	verify()
	submit("did:key:reviewer", reviewer, &toDraft, "Bind the service clause")
	submit("did:key:creator", creator, nil)
	verify()
	submit("did:key:reviewer", reviewer, &toApproval, "Ready")
	require.NoError(t, (&command.Rejecter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}).Handle(ctx, command.RejectCmd{
		DID: "tpl", UpdatedAt: future, RejectedBy: org, Reason: "The SLA is missing", HolderDID: "did:key:approver", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	}))

	svc := &templateRepositorysrvc{DB: database, CTRepo: ctRepo}
	retrieve := func(role userrole.UserRole) (*templaterepository.ContractTemplateRetrieveByIDResponse, error) {
		return svc.RetrieveByID(middleware.InjectAuthContext(ctx, []string{role.String()}, "did:key:reader", org),
			&templaterepository.ContractTemplateRetrieveByIDRequest{Did: "tpl"})
	}
	for _, role := range []userrole.UserRole{userrole.TemplateCreator, userrole.TemplateApprover} {
		res, err := retrieve(role)
		require.NoError(t, err)
		require.Len(t, res.ReviewNotes, 3, role)
		require.Equal(t, "RETURNED_TO_DRAFT", res.ReviewNotes[0].Kind)
		require.Equal(t, []string{"Bind the service clause"}, res.ReviewNotes[0].Text)
		require.Equal(t, "did:key:reviewer", *res.ReviewNotes[0].By)
		require.Equal(t, "FORWARDED_TO_APPROVAL", res.ReviewNotes[1].Kind)
		require.Equal(t, "REJECTED", res.ReviewNotes[2].Kind)
		require.Equal(t, []string{"The SLA is missing"}, res.ReviewNotes[2].Text)
	}

	_, err := retrieve(userrole.ContractCreator)
	require.Equal(t, "not_found", errorName(err), "a contract creator read a rejected template")

	_, err = database.Exec(`UPDATE contract_templates SET state = 'DEPRECATED' WHERE did = 'tpl'`)
	require.NoError(t, err)
	res, err := retrieve(userrole.ContractCreator)
	require.NoError(t, err)
	require.Empty(t, res.ReviewNotes)
}
