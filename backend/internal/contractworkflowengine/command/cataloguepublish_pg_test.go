package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

type catalogueFake struct {
	published []string
	revoked   []string
}

func (f *catalogueFake) PublishAsset(context.Context, map[string]any) (string, error) {
	hash := fmt.Sprintf("asset-%d", len(f.published)+1)
	f.published = append(f.published, hash)
	return hash, nil
}

func (f *catalogueFake) RevokeAsset(_ context.Context, assetHash string) error {
	f.revoked = append(f.revoked, assetHash)
	return nil
}

func catalogueTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('c','did:web:local','Org A','APPROVED','tpl',1,'c','{}')`)
	require.NoError(t, err)
	return database
}

func catalogueEvents(t *testing.T, database *sqlx.DB, eventType string) int {
	t.Helper()
	var n int
	require.NoError(t, database.Get(&n, `SELECT count(*) FROM outbox_events WHERE event_type = $1`, eventType))
	return n
}

// DCS-IR-CWE-10: registering a contract again replaces its catalogue entry,
// so the catalogue never lists one contract twice.
func TestARepublishedContractHasOneCatalogueEntryPostgres(t *testing.T) {
	database := catalogueTestDB(t)
	fake := &catalogueFake{}
	publisher := &CataloguePublisher{DB: database, CRepo: &cwepg.PostgresContractRepo{}}
	cmd := CataloguePublishCmd{DID: "c", PublishedBy: "Org A", InstanceDID: "did:web:local"}

	first, err := publisher.handle(context.Background(), fake, cmd)
	require.NoError(t, err)
	second, err := publisher.handle(context.Background(), fake, cmd)
	require.NoError(t, err)

	require.Equal(t, []string{first}, fake.revoked)
	var entries []string
	require.NoError(t, database.Select(&entries, `SELECT asset_hash FROM contract_catalogue_entries WHERE did = 'c'`))
	require.Equal(t, []string{second}, entries)
	require.Equal(t, 2, catalogueEvents(t, database, "PUBLISH_CONTRACT_TO_CATALOGUE"))
	require.Equal(t, 1, catalogueEvents(t, database, "WITHDRAW_CONTRACT_FROM_CATALOGUE"))
}

// DCS-FR-CWE-23: a contract that is terminated (or expires, or is revoked)
// leaves the catalogue; an approved one stays.
func TestATerminatedContractIsWithdrawnFromTheCataloguePostgres(t *testing.T) {
	database := catalogueTestDB(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('active','did:web:local','Org A','ACTIVE','tpl',1,'a','{}')`)
	require.NoError(t, err)
	fake := &catalogueFake{}
	publisher := &CataloguePublisher{DB: database, CRepo: &cwepg.PostgresContractRepo{}}
	asset, err := publisher.handle(context.Background(), fake, CataloguePublishCmd{DID: "c", PublishedBy: "Org A", InstanceDID: "did:web:local"})
	require.NoError(t, err)
	_, err = publisher.handle(context.Background(), fake, CataloguePublishCmd{DID: "active", PublishedBy: "Org A", InstanceDID: "did:web:local"})
	require.NoError(t, err)

	sweepCatalogue(context.Background(), database, fake)
	require.Empty(t, fake.revoked, "published contracts stay in the catalogue")

	_, err = database.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = 'c'`)
	require.NoError(t, err)
	sweepCatalogue(context.Background(), database, fake)

	require.Equal(t, []string{asset}, fake.revoked)
	var left []string
	require.NoError(t, database.Select(&left, `SELECT did FROM contract_catalogue_entries`))
	require.Equal(t, []string{"active"}, left)
	require.Equal(t, 1, catalogueEvents(t, database, "WITHDRAW_CONTRACT_FROM_CATALOGUE"))
}

// DCS-IR-CWE-10: a catalogue entry whose audit record cannot be written is
// revoked again, so the catalogue holds no entry the audit trail lacks.
func TestAnUnrecordedCatalogueEntryIsRevokedPostgres(t *testing.T) {
	database := catalogueTestDB(t)
	_, err := database.Exec(`
		CREATE FUNCTION refuse_entry() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$;
		CREATE TRIGGER refuse_entry BEFORE INSERT ON contract_catalogue_entries FOR EACH ROW EXECUTE FUNCTION refuse_entry()`)
	require.NoError(t, err)
	fake := &catalogueFake{}
	publisher := &CataloguePublisher{DB: database, CRepo: &cwepg.PostgresContractRepo{}}

	_, err = publisher.handle(context.Background(), fake, CataloguePublishCmd{DID: "c", PublishedBy: "Org A", InstanceDID: "did:web:local"})
	require.Error(t, err)
	require.Equal(t, fake.published, fake.revoked)
	require.Zero(t, catalogueEvents(t, database, "PUBLISH_CONTRACT_TO_CATALOGUE"))
}
