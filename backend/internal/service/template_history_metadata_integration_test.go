package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
)

// DCS-FR-TR-05, DCS-FR-TR-21: a change of the classification or the
// structure is kept in the history and in the update event.
func TestTemplateClassificationChangeIsTrackedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	ctRepo := &pg.PostgresContractTemplateRepo{}
	data := datatype.JSON(sodTemplateData)
	jurisdiction := "DEU"
	require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
		DID: "tpl", CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &data, Jurisdiction: &jurisdiction, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))

	newJurisdiction, level := "FRA", "FRAME_AGREEMENT"
	require.NoError(t, (&command.UpdateManager{DB: database, CTRepo: ctRepo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}).Handle(ctx, command.UpdateManageCmd{
		DID: "tpl", UpdatedAt: time.Now().Add(time.Hour), UpdatedBy: "org", Jurisdiction: &newJurisdiction, TemplateLevel: &level,
		UserRoles: userrole.UserRoles{userrole.TemplateManager},
	}))

	history, err := (&contracttemplate.GetHistoryByIDHandler{Ctx: ctx, DB: database, CTRepo: ctRepo}).Handle(ctx, contracttemplate.GetHistoryByIDQry{DID: "tpl"})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "DEU", *history[0].Jurisdiction)
	require.Equal(t, "AGREEMENT", history[0].TemplateLevel)

	var changes struct {
		OldJurisdiction string `db:"old_jurisdiction"`
		NewJurisdiction string `db:"new_jurisdiction"`
		OldLevel        string `db:"old_level"`
		NewLevel        string `db:"new_level"`
	}
	require.NoError(t, database.Get(&changes, `SELECT
		event_data->'old_classification'->>'jurisdiction' AS old_jurisdiction,
		event_data->'new_classification'->>'jurisdiction' AS new_jurisdiction,
		event_data->'old_classification'->>'template_level' AS old_level,
		event_data->'new_classification'->>'template_level' AS new_level
		FROM outbox_events WHERE event_type = 'UPDATE_CONTRACT_TEMPLATE' AND did = 'tpl'`))
	require.Equal(t, "DEU", changes.OldJurisdiction)
	require.Equal(t, "FRA", changes.NewJurisdiction)
	require.Equal(t, "AGREEMENT", changes.OldLevel)
	require.Equal(t, "FRAME_AGREEMENT", changes.NewLevel)
}
