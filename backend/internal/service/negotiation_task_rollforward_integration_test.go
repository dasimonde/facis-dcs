package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// A signed PDF from the peer bumps the version without reopening accepted
// negotiation tasks; a new negotiation round reopens them.
func TestNegotiationTaskRollForwardPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES ('signed','','Org A','SIGNED','tpl',1)`)
	require.NoError(t, err)
	repo := &cwepg.PostgresNegotiationTaskRepo{}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = repo.Create(ctx, tx, cwedb.NegotiationTaskData{DID: "signed", ContractVersion: 1, State: "ACCEPTED", Negotiator: "did:web:peer", CreatedBy: "creator"})
	require.NoError(t, err)
	state := func(version int) string {
		var s string
		require.NoError(t, tx.GetContext(ctx, &s, `SELECT state FROM contract_negotiation_task WHERE did='signed' AND contract_version=$1`, version))
		return s
	}

	require.NoError(t, repo.RollForwardKeepingState(ctx, tx, "signed", 1, 2))
	require.Equal(t, "ACCEPTED", state(2))

	require.NoError(t, repo.RollForward(ctx, tx, "signed", 2, 3))
	require.Equal(t, "OPEN", state(3))
}
