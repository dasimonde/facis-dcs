package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/semantichub/hubtest"
)

func TestContractEventAudiencePostgres(t *testing.T) {
	database := hubtest.Open(t)
	ctx := context.Background()
	peer := "did:web:local"
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	data := datatype.JSON(`{"dcs:parties":[{"dcs:legalName":"Org B"}]}`)
	require.NoError(t, (&cwepg.PostgresContractRepo{}).Create(ctx, tx, cwedb.Contract{DID: "audience", Origin: peer, CreatedBy: "Org A", CreatedAt: time.Now(), State: "DRAFT", ContractData: &data, TemplateDID: "template", TemplateVersion: 1}))
	require.NoError(t, tx.Commit())
	t.Cleanup(func() { _, _ = database.Exec(`DELETE FROM contracts WHERE did = 'audience'`) })

	admit := ContractEventAudience(database, peer)
	require.True(t, admit(ctx, "contract.created", "audience", "Org A"), "creator")
	require.True(t, admit(ctx, "contract.created", "audience", "Org B"), "named party")
	require.False(t, admit(ctx, "contract.created", "audience", "Org C"), "foreign organization")
	require.False(t, admit(ctx, "contract.expired", "unknown", "Org A"), "unknown contract")
	require.True(t, admit(ctx, "template.updated", "any-template", "Org C"), "template events are instance-wide")
}
