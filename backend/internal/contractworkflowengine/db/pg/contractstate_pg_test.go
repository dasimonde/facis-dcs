package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
)

// A state write names the state its caller read. When another transaction
// changed the state since, for example a revocation that committed while a
// signature or a deployment callback was in progress, the write fails and
// the other transaction's state stays.
func TestUpdateContractStateRefusesAStateChangedByAnotherTransaction(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	const did = "did:example:guarded-state"
	_, err := database.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version)
		VALUES ($1, '', 'Org A', 'SIGNED', 'tpl', 1)`, did)
	require.NoError(t, err)

	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var read string
	require.NoError(t, tx.GetContext(ctx, &read, `SELECT state FROM contracts WHERE did = $1`, did))

	_, err = database.Exec(`UPDATE contracts SET state = 'REVOKED' WHERE did = $1`, did)
	require.NoError(t, err)

	err = (&PostgresContractRepo{}).UpdateState(ctx, tx, did, read, contractstate.Active.String())
	require.ErrorIs(t, err, contractstate.ErrInvalidTransition)
	require.NoError(t, tx.Rollback())

	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did = $1`, did))
	require.Equal(t, "REVOKED", state)

	tx, err = database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, (&PostgresContractRepo{}).UpdateState(ctx, tx, did, "REVOKED", contractstate.Approved.String()))
	require.NoError(t, tx.Commit())
}
