package command

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/event/outboxtest"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type activateContractRepo struct {
	kpiContractRepoFake
	state string
}

func (r *activateContractRepo) ReadProcessDataByDID(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	return &db.ContractProcessData{DID: kpiContractDID, State: "SIGNED", ContractVersion: 3}, nil
}

func (r *activateContractRepo) UpdateState(_ context.Context, _ *sqlx.Tx, _, _ string, state string) error {
	r.state = state
	return nil
}

type activateDeploymentRepo struct{ kpiDeploymentRepoFake }

func (r *activateDeploymentRepo) AcknowledgeDeployment(context.Context, *sqlx.Tx, string, string, string, time.Time) error {
	return nil
}

// DCS-FR-CWE-24, DCS-FR-PACM-01: the acknowledgement that moves a contract to
// ACTIVE writes an audit event, so the transition is in the audit trail and
// reaches the live dashboards.
func TestDeploymentAcknowledgementRecordsTheActivation(t *testing.T) {
	database, recorder := outboxtest.Open(t)
	contracts := &activateContractRepo{}
	handler := &DeploymentCallbackHandler{
		DB:    database,
		CRepo: contracts,
		DeploymentRepo: &activateDeploymentRepo{kpiDeploymentRepoFake{deployment: &db.ContractDeployment{
			DID: kpiContractDID, CorrelationID: "corr-1", ContentHash: "sha256:deadbeef", TargetID: ptr("t-1"),
		}}},
		TargetRepo: &stubTargetRepo{target: &db.ContractTarget{ID: "t-1", OAuthClientID: ptr("dcs-target-t-1")}},
	}

	require.NoError(t, handler.Handle(context.Background(), DeploymentCallbackCmd{
		CallerClientID: "dcs-target-t-1", DID: kpiContractDID, CorrelationID: "corr-1", Status: "ack",
	}))

	require.Equal(t, "ACTIVE", contracts.state)
	committed := recorder.Committed()
	require.Len(t, committed, 1)
	require.Equal(t, "ACTIVATE_CONTRACT", committed[0].Type)
	require.Equal(t, kpiContractDID, committed[0].Data["did"])
	require.Equal(t, "SIGNED", committed[0].Data["previous_state"])
	require.Equal(t, "ACTIVE", committed[0].Data["new_state"])
	require.Equal(t, "dcs-target-t-1", committed[0].Data["activated_by"])
	require.EqualValues(t, 3, committed[0].Data["contract_version"])
}
