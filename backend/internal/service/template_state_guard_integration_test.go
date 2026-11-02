package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	tpldb "digital-contracting-service/internal/templaterepository/db"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

// DCS-FR-TR-14: a state transition applies only from the state the command
// checked, so a deleted template does not become registered.
func TestTemplateUpdateStateRequiresExpectedStatePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ('tpl','creator','DELETED','CONTRACT_TEMPLATE','SINGLE_PARTY','n')`)
	require.NoError(t, err)

	tx := database.MustBegin()
	err = (&pg.PostgresContractTemplateRepo{}).UpdateState(ctx, tx, "tpl", "APPROVED", "REGISTERED")
	require.NoError(t, tx.Rollback())
	require.ErrorIs(t, err, tpldb.ErrInvalidTemplateState)

	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contract_templates WHERE did = 'tpl'`))
	require.Equal(t, "DELETED", state)
}

// DCS-FR-TR-14: of two concurrent submissions of one draft, one succeeds,
// and the template has one review task and one approval task.
func TestTemplateConcurrentSubmitCreatesOneTaskPairPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	data := datatype.JSON(sodTemplateData)
	require.NoError(t, (&command.Creator{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}}).Handle(ctx, command.CreateCmd{
		DID: "tpl", CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))

	// Hold the row so that both submissions wait at the state change.
	holder := database.MustBegin()
	t.Cleanup(func() { _ = holder.Rollback() })
	_, err := holder.Exec(`SELECT 1 FROM contract_templates WHERE did = 'tpl' FOR NO KEY UPDATE`)
	require.NoError(t, err)

	submitter := &command.Submitter{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- submitter.Handle(ctx, command.SubmitCmd{
				DID: "tpl", UpdatedAt: time.Now().Add(time.Hour), SubmittedBy: "org",
				UserRoles: userrole.UserRoles{userrole.TemplateCreator},
			})
		}()
	}
	require.Eventually(t, func() bool {
		var waiting int
		err := database.Get(&waiting, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%UPDATE contract_templates ct%'`)
		return err == nil && waiting == 2
	}, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, holder.Commit())

	succeeded := 0
	for range 2 {
		if <-results == nil {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded)
	var reviewTasks, approvalTasks int
	require.NoError(t, database.Get(&reviewTasks, `SELECT count(*) FROM contract_templates_review_task WHERE did = 'tpl'`))
	require.NoError(t, database.Get(&approvalTasks, `SELECT count(*) FROM contract_templates_approval_task WHERE did = 'tpl'`))
	require.Equal(t, 1, reviewTasks)
	require.Equal(t, 1, approvalTasks)
}

// DCS-IR-TR-03: a Template Manager edit of a template in review reopens the
// review task, so the reviewer verifies the changed content before the
// forward to approval.
func TestTemplateManagerEditReopensReviewPostgres(t *testing.T) {
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
	require.NoError(t, submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: org, UserRoles: userrole.UserRoles{userrole.TemplateCreator}}))
	_, err := (&query.Verifier{DB: database, CTRepo: ctRepo, RTRepo: rtRepo}).Handle(ctx, query.VerifyQry{DID: "tpl", VerifiedBy: org, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
	require.NoError(t, err)

	name := "changed by the manager"
	require.NoError(t, (&command.UpdateManager{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}).Handle(ctx, command.UpdateManageCmd{
		DID: "tpl", UpdatedAt: future, UpdatedBy: org, Name: &name, UserRoles: userrole.UserRoles{userrole.TemplateManager},
	}))

	var task string
	require.NoError(t, database.Get(&task, `SELECT state FROM contract_templates_review_task WHERE did = 'tpl'`))
	require.Equal(t, "OPEN", task)
	forward := actionflag.Approval
	err = submitter.Handle(ctx, command.SubmitCmd{DID: "tpl", UpdatedAt: future, SubmittedBy: org, ActionFlag: &forward, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
	require.ErrorContains(t, err, "needs to be verified")
}

// A write that does not change the content (here a state change) does not
// make an editor's update stale: the optimistic lock of update and
// update_manage compares the content timestamp, as the other commands do. A
// content edit in between still conflicts.
func TestTemplateUpdateLockComparesContentPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	ctRepo := &pg.PostgresContractTemplateRepo{}
	loaded := map[string]time.Time{}
	for _, did := range []string{"by-creator", "by-manager"} {
		data := datatype.JSON(sodTemplateData)
		require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
			DID: did, CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
			TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
		}))
		var at time.Time
		require.NoError(t, database.Get(&at, `SELECT updated_at FROM contract_templates WHERE did = $1`, did))
		loaded[did] = at
	}

	time.Sleep(1100 * time.Millisecond)
	_, err := database.Exec(`UPDATE contract_templates SET state = 'REJECTED'`)
	require.NoError(t, err)

	name := "renamed"
	update := command.UpdateCmd{DID: "by-creator", UpdatedAt: loaded["by-creator"], UpdatedBy: "org", Name: &name, UserRoles: userrole.UserRoles{userrole.TemplateCreator}}
	updater := &command.Updater{DB: database, CTRepo: ctRepo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	require.NoError(t, updater.Handle(ctx, update))
	require.NoError(t, (&command.UpdateManager{DB: database, CTRepo: ctRepo}).Handle(ctx, command.UpdateManageCmd{
		DID: "by-manager", UpdatedAt: loaded["by-manager"], UpdatedBy: "org", Name: &name, UserRoles: userrole.UserRoles{userrole.TemplateManager},
	}))

	// The name edit above moved the content timestamp past the loaded one.
	require.ErrorIs(t, updater.Handle(ctx, update), base.ErrUpdatedElsewhere)
}
