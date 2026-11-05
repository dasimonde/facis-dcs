package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/semantichub"
)

func TestArchiveSearchProjectionUsesEachSnapshot(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	defer validation.SwapShapeSource(semantichub.HubShapeSource{DB: database})()
	did := "did:web:example:contract:projection"
	_, err := database.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version) VALUES ($1, 'did:web:example', 'did:web:creator', 'SIGNED', 'did:web:template', 1)`, did)
	require.NoError(t, err)
	evidence, err := datatype.NewJSON(map[string]any{"source": "SIGNING_WORKFLOW_COMPLETION", "snapshot_hash_algorithm": "SHA-256"})
	require.NoError(t, err)
	for i, value := range []string{"DEU", "FRA"} {
		snapshot, err := datatype.NewJSON(map[string]any{"contract_data": map[string]any{"dcs:contractFields": []any{map[string]any{"dcs:shape": map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"}, "dcs:value": value}}}})
		require.NoError(t, err)
		tx, err := database.BeginTxx(ctx, nil)
		require.NoError(t, err)
		err = (&pg.PostgresContractRepo{}).StoreArchiveEntry(ctx, tx, db.ContractArchiveEntry{DID: did, ContractVersion: i + 1, StoredBy: "did:web:creator", StoredAt: time.Now(), ContractSnapshot: snapshot, ContentHash: "sha256:" + zeroHash, SnapshotCID: "bafy", Evidence: &evidence})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	}
	for version, expected := range map[int]string{1: "DEU", 2: "FRA"} {
		var value string
		err := database.Get(&value, `SELECT search_projection -> $3 ->> 0 FROM contract_archive_entries WHERE did=$1 AND contract_version=$2`, did, version, "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction")
		require.NoError(t, err)
		require.Equal(t, expected, value)
	}
	service := archiveTestService(t, database)
	for version, value := range map[int]string{1: "DEU", 2: "FRA"} {
		filter := `[{"filter_id":"jurisdiction","value":"` + value + `"}]`
		rows, err := service.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{ContractVersion: &version, SemanticFilters: &filter})
		require.NoError(t, err)
		require.Len(t, rows, 1)
	}
	searchEvents := archiveEvents(t, database, "SEARCH_ARCHIVED_CONTRACTS")
	require.Len(t, searchEvents, 2)
	criteria, ok := searchEvents[0]["criteria"].(map[string]any)
	require.True(t, ok)
	var audited []map[string]any
	require.NoError(t, json.Unmarshal([]byte(criteria["semantic_filters"].(string)), &audited))
	require.Equal(t, "jurisdiction", audited[0]["filter_id"])
	require.Equal(t, "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction", audited[0]["field_iri"])
	require.NotEmpty(t, audited[0]["value"])
	wrong := `[{"filter_id":"jurjurisdiction","value":"DEU"}]`
	_, err = service.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{SemanticFilters: &wrong})
	require.Error(t, err)
	wrong = `[{"filter_id":"jurisdiction","value":"DEU"}]`
	version := 2
	rows, err := service.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{ContractVersion: &version, SemanticFilters: &wrong})
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = database.Exec(`UPDATE contract_archive_entries SET search_projection='{}'::jsonb WHERE did=$1 AND contract_version=1`, did)
	require.ErrorContains(t, err, "immutable contract archive search projection")
}
