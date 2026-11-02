package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
)

// DCS-FR-UC-12-2, DCS-FR-UC-12-3: a System Contract Reviewer forwards a
// submitted contract to approval, and a System Contract Approver sends a
// reviewed contract back to review, through the same submit command a person
// uses.
func TestSystemReviewerAndApproverSubmitPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES ('c1',$1,'Org A','SUBMITTED','tpl',1)`, peer)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_review_task (did,state,reviewer,created_by) VALUES ('c1','OPEN',$1,'Org A')`, peer)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_approval_task (did,state,approver,created_by) VALUES ('c1','OPEN',$1,'Org A')`, peer)
	require.NoError(t, err)

	submitter := command.Submitter{
		DB: database, CRepo: &cwepg.PostgresContractRepo{}, RTRepo: &cwepg.PostgresReviewTaskRepo{},
		ATRepo: &cwepg.PostgresApprovalTaskRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *document,
	}
	submit := func(role userrole.UserRole, flag actionflag.ActionFlag) error {
		return submitter.Handle(context.Background(), command.SubmitCmd{
			DID: "c1", UpdatedAt: time.Now().Add(time.Hour), SubmittedBy: peer, ActionFlag: &flag,
			HolderDID: "dcs-orce-client", UserRoles: userrole.UserRoles{role}, CauserDID: peer,
		})
	}
	state := func() string {
		var s string
		require.NoError(t, database.Get(&s, `SELECT state FROM contracts WHERE did = 'c1'`))
		return s
	}

	require.NoError(t, submit(userrole.SystemContractReviewer, actionflag.Approval))
	require.Equal(t, "REVIEWED", state())

	require.NoError(t, submit(userrole.SystemContractApprover, actionflag.Approval))
	require.Equal(t, "SUBMITTED", state())
}
