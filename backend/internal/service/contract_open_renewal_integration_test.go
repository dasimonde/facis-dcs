package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-FR-CWE-22: a renewal counts as open until it is withdrawn, revoked,
// terminated or expired; a second renewal is then refused as a conflict.
func TestAnOpenRenewalIsFoundPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES
		('orig','o','Org A','ACTIVE','tpl',1,'{}'),
		('old','o','Org A','WITHDRAWN','tpl',1,'{"dcs:renewsContract":{"@id":"https://dcs.example/contract/orig"}}'),
		('new','o','Org A','DRAFT','tpl',1,'{"dcs:renewsContract":{"@id":"https://dcs.example/contract/orig"}}')`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	repo := &cwepg.PostgresContractRepo{}

	open, err := repo.ReadOpenRenewalDID(ctx, tx, "orig")
	require.NoError(t, err)
	require.Equal(t, "new", open)
	_, err = tx.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = 'new'`)
	require.NoError(t, err)
	open, err = repo.ReadOpenRenewalDID(ctx, tx, "orig")
	require.NoError(t, err)
	require.Empty(t, open)

	require.Equal(t, "conflict", errorName(mapContractCommandError(fmt.Errorf("%w: new", command.ErrRenewalExists))))
}
