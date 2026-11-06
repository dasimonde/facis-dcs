package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/semantichub"
	signingpg "digital-contracting-service/internal/signingmanagement/db/pg"
)

func TestContractSearchProjectionFollowsWritePaths(t *testing.T) {
	database := pgtest.Open(t)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	require.NoError(t, semantichub.Seed(ctx, database))
	defer validation.SwapShapeSource(semantichub.HubShapeSource{DB: database})()
	repo := &cwepg.PostgresContractRepo{}
	const field = "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"
	document := func(value string) *datatype.JSON {
		raw, err := json.Marshal(map[string]any{"dcs:contractFields": []any{map[string]any{"dcs:shape": map[string]any{"@id": field}, "dcs:value": value}}})
		require.NoError(t, err)
		data := datatype.JSON(raw)
		return &data
	}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	data := cwedb.Contract{DID: "local", CreatedBy: "org", CreatedAt: time.Now(), State: "DRAFT", TemplateDID: "tpl", TemplateVersion: 1, ContractData: document("DEU")}
	require.NoError(t, repo.Create(ctx, tx, data))
	data.DID, data.ContractData = "remote", document("DEU")
	require.NoError(t, repo.RemoteCreate(ctx, tx, data))
	search := func(value string) []cwedb.ContractMetadata {
		rows, err := repo.ReadAllMetaDataByFilter(ctx, tx, cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: field, Value: value}}}, datatype.Pagination{})
		require.NoError(t, err)
		return rows
	}
	require.Len(t, search("DEU"), 2)
	require.NoError(t, repo.Update(ctx, tx, cwedb.ContractUpdateData{DID: "local", ContractData: document("FRA")}))
	data.ContractData = document("FRA")
	require.NoError(t, repo.RemoteUpdate(ctx, tx, data))
	require.Empty(t, search("DEU"))
	require.Len(t, search("FRA"), 2)
	require.NoError(t, (&signingpg.PostgresContractRepo{}).UpdateContractData(ctx, tx, "local", *document("ESP")))
	require.Len(t, search("FRA"), 1)
	require.Len(t, search("ESP"), 1)
	invalid := datatype.JSON(`{"broken"`)
	require.Error(t, repo.Update(ctx, tx, cwedb.ContractUpdateData{DID: "local", ContractData: &invalid}))
	require.Len(t, search("ESP"), 1)
	large := datatype.JSON(`{"dcs:contractFields":[{"dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"},"dcs:value":18446744073709551615}]}`)
	require.NoError(t, repo.Update(ctx, tx, cwedb.ContractUpdateData{DID: "local", ContractData: &large}))
	for _, tc := range []struct {
		value string
		count int
	}{{"18446744073709551615", 1}, {"18446744073709551614", 0}} {
		rows, err := repo.ReadAllMetaDataByFilter(ctx, tx, cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: field, Value: tc.value, Datatype: "http://www.w3.org/2001/XMLSchema#unsignedLong"}}}, datatype.Pagination{})
		require.NoError(t, err)
		require.Len(t, rows, tc.count)
	}
	typed := datatype.JSON(`{"dcs:contractFields":[{"dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"},"dcs:value":{"@type":"http://www.w3.org/2001/XMLSchema#integer","@value":"18446744073709551615"}}]}`)
	require.NoError(t, repo.Update(ctx, tx, cwedb.ContractUpdateData{DID: "local", ContractData: &typed}))
	typedRows, err := repo.ReadAllMetaDataByFilter(ctx, tx, cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: field, Value: "18446744073709551615", Datatype: "http://www.w3.org/2001/XMLSchema#unsignedLong"}}}, datatype.Pagination{})
	require.NoError(t, err)
	require.Len(t, typedRows, 1)
	iriDocument := datatype.JSON(`{"https://example.org/property":{"@id":"https://example.org/value","@type":"https://example.org/Class"}}`)
	require.NoError(t, repo.Update(ctx, tx, cwedb.ContractUpdateData{DID: "local", ContractData: &iriDocument}))
	rows, err := repo.ReadAllMetaDataByFilter(ctx, tx, cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: "https://example.org/property", Value: "https://example.org/value", Datatype: "http://www.w3.org/2001/XMLSchema#anyURI"}}}, datatype.Pagination{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}
