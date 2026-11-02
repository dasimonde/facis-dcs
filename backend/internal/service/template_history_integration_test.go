package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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

	require.NoError(t, repo.UpdateState(ctx, tx, "provenance", "DRAFT", "SUBMITTED"))
	require.NoError(t, repo.UpdateState(ctx, tx, "provenance", "SUBMITTED", "APPROVED"))

	history, err := repo.ReadHistoryByDID(ctx, tx, "provenance")
	require.NoError(t, err)
	states := make([]string, 0, len(history))
	for _, entry := range history {
		states = append(states, entry.State)
	}
	require.ElementsMatch(t, []string{"DRAFT", "SUBMITTED"}, states)
}
