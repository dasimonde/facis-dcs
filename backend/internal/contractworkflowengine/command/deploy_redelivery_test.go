package command

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// DCS-NFR-SF-01: the event bus delivers an event at least once. The automatic
// deployment uses the event id as its correlation id, so a redelivered
// signature event finds its deployment and dispatches nothing.
func TestRedeliveredDeployEventDispatchesNothing(t *testing.T) {
	raw, err := sql.Open("callback-no-query", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })

	deployments := &kpiDeploymentRepoFake{deployment: &db.ContractDeployment{DID: kpiContractDID, CorrelationID: "event-1"}}
	deployer := &Deployer{
		DB:             sqlx.NewDb(raw, "postgres"),
		CRepo:          &kpiContractRepoFake{contract: &db.Contract{DID: kpiContractDID, State: string(contractstate.Signed)}},
		DeploymentRepo: deployments,
	}

	_, err = deployer.Handle(context.Background(), DeployCmd{DID: kpiContractDID, RequestedBy: "system:auto-deploy", EventID: "event-1"})

	require.ErrorIs(t, err, ErrAlreadyDispatched)
}
