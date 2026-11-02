package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/templaterepository/db"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

func TestTemplateHistoryRecordsStateChangesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	repo := &pg.PostgresContractTemplateRepo{}
	_, err = repo.Create(ctx, tx, db.ContractTemplate{DID: "provenance", CreatedBy: "tester", State: "DRAFT", TemplateType: "CONTRACT_TEMPLATE", PartyType: "SINGLE_PARTY"})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateState(ctx, tx, "provenance", "DRAFT", "SUBMITTED", "submitter"))
	require.NoError(t, repo.UpdateState(ctx, tx, "provenance", "SUBMITTED", "APPROVED", "approver"))

	history, err := repo.ReadHistoryByDID(ctx, tx, "provenance")
	require.NoError(t, err)
	states := make([]string, 0, len(history))
	for _, entry := range history {
		states = append(states, entry.State)
	}
	require.ElementsMatch(t, []string{"DRAFT", "SUBMITTED"}, states)
}

// TestTemplateRecordsTheIdentityBehindEachChangePostgres covers F04 and
// DCS-FR-TR-05: the template and each history entry name who changed it last.
func TestTemplateRecordsTheIdentityBehindEachChangePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	repo := &pg.PostgresContractTemplateRepo{}
	_, err = repo.Create(ctx, tx, db.ContractTemplate{DID: "actors", CreatedBy: "creator", State: "DRAFT", TemplateType: "CONTRACT_TEMPLATE", PartyType: "SINGLE_PARTY"})
	require.NoError(t, err)
	created, err := repo.ReadDataByID(ctx, tx, "actors")
	require.NoError(t, err)
	require.Equal(t, "creator", created.UpdatedBy)

	name := "renamed"
	require.NoError(t, repo.Update(ctx, tx, db.ContractTemplateUpdateData{DID: "actors", Name: &name, UpdatedBy: "editor"}))
	require.NoError(t, repo.UpdateState(ctx, tx, "actors", "DRAFT", "SUBMITTED", "submitter"))

	current, err := repo.ReadDataByID(ctx, tx, "actors")
	require.NoError(t, err)
	require.Equal(t, "creator", current.CreatedBy)
	require.Equal(t, "submitter", current.UpdatedBy)

	listed, err := repo.ReadAllMetaDataByFilter(ctx, tx, db.SearchValues{DID: "actors"}, datatype.Pagination{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "submitter", listed[0].UpdatedBy)

	history, err := repo.ReadHistoryByDID(ctx, tx, "actors")
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "DRAFT", history[0].State)
	require.Equal(t, "editor", history[0].UpdatedBy)
}
