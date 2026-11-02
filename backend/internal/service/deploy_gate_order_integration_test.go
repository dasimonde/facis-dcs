package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
)

// DCS-NFR-BR-03: a contract with an unsigned declared signature field is
// refused as incomplete before the deployment workflow gate is dispatched.
// The service has no gate configured here, so a call that reached the gate
// would fail as BLOCKED instead.
func TestDeployRefusesIncompleteSigningBeforeTheGatePostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES ('half-signed',$1,'Org A','SIGNED','template',1,'half-signed',
		'{"dcs:signatureFields":[{"dcs:signatoryName":"SignerOne"},{"dcs:signatoryName":"SignerTwo"}]}')`, peer)
	require.NoError(t, err)

	s := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *document, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Contract Manager"}, "holder-a", "Org A")
	_, err = s.Deploy(ctx, &cwe.ContractDeployRequest{Did: "half-signed", UpdatedAt: "2026-10-30T12:00:00Z"})

	require.ErrorIs(t, err, command.ErrSigningIncomplete)
	var blocked *workflowgate.BlockedError
	require.NotErrorAs(t, err, &blocked, "the gate ran for a contract that may not deploy")
	var runs int
	require.NoError(t, database.Get(&runs, `SELECT count(*) FROM pac_workflow_gate_runs`))
	require.Zero(t, runs)
}
