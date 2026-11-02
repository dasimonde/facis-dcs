package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

// From Approved onwards a template is changed through a copy, never in place.
func TestTemplateEditsLockedFromApprovedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	repo := &pg.PostgresContractTemplateRepo{}
	updater := &command.Updater{DB: database, CTRepo: repo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	manager := &command.UpdateManager{DB: database, CTRepo: repo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	future := time.Now().Add(time.Hour)

	for _, state := range []string{"DRAFT", "REVIEWED", "APPROVED", "REGISTERED", "PUBLISHED", "DEPRECATED"} {
		t.Run(state, func(t *testing.T) {
			did := "lock-" + state
			_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','before')`, did, state)
			require.NoError(t, err)
			name := "after"
			updateErr := updater.Handle(ctx, command.UpdateCmd{DID: did, Name: &name, UpdatedAt: future, UpdatedBy: "creator", UserRoles: userrole.UserRoles{userrole.TemplateCreator, userrole.TemplateManager}})
			manageErr := manager.Handle(ctx, command.UpdateManageCmd{DID: did, Name: &name, UpdatedAt: future, UpdatedBy: "manager", UserRoles: userrole.UserRoles{userrole.TemplateManager}})
			var stored string
			require.NoError(t, database.Get(&stored, `SELECT name FROM contract_templates WHERE did=$1`, did))
			if state == "DRAFT" {
				require.NoError(t, updateErr)
				require.NoError(t, manageErr)
				require.Equal(t, "after", stored)
				return
			}
			require.Error(t, updateErr)
			require.Error(t, manageErr)
			require.Equal(t, "before", stored)
		})
	}
}
