package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-FR-CWE-08: the history lists each saved version once, with its own id,
// in the order of the saves, also when two saves share one contract_version.
func TestContractHistoryIsOrderedBySaveTimeWithIDsPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contract_history (did,origin,created_by,contract_version,state,template_did,template_version,name,modified_by,modified_at) VALUES
		('c','o','Org A',1,'DRAFT','tpl',1,'third','Org A',now() - interval '1 minute'),
		('c','o','Org A',1,'DRAFT','tpl',1,'first','Org A',now() - interval '3 minute'),
		('c','o','Org A',1,'DRAFT','tpl',1,'second','Org A',now() - interval '2 minute')`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	history, err := (&cwepg.PostgresContractRepo{}).ReadHistoryByDID(ctx, tx, "c")
	require.NoError(t, err)
	names := []string{}
	ids := map[string]bool{}
	for _, entry := range history {
		names = append(names, *entry.Name)
		require.NotEmpty(t, entry.ID)
		ids[entry.ID] = true
	}
	require.Equal(t, []string{"first", "second", "third"}, names)
	require.Len(t, ids, 3)
}
