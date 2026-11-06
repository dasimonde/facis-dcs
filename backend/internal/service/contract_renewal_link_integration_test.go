package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-FR-CWE-29: the contract list names the contract a renewal renews, so the
// structure view can link both ways.
func TestContractListNamesTheRenewedContractPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES
		('old','','Org A','EXPIRED','tpl',1,'{}'),
		('new','','Org A','DRAFT','tpl',1,'{"dcs:renewsContract":{"@id":"https://dcs.example/contract/old","dcs:version":1}}')`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	rows, err := (&cwepg.PostgresContractRepo{}).ReadAllMetaData(ctx, tx, datatype.Pagination{}, cwedb.ListOptions{})
	require.NoError(t, err)
	renews := map[string]string{}
	for _, row := range rows {
		if row.RenewsContractDID != nil {
			renews[row.DID] = *row.RenewsContractDID
		}
	}
	require.Equal(t, "old", renews["new"])
	require.NotContains(t, renews, "old")
}
