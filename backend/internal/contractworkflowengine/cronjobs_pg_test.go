package contractworkflowengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/pdfgeneration/statuspublication"
)

// DCS-OR-C2PA-005: an expiry reaches the status list this deployment serves,
// as a termination and a revocation do. The cron task queues the publication
// in the transaction that persists EXPIRED, and the queue sets the contract's
// revocation bit.
func TestExpiryQueuesStatusPublication(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	const did = "did:web:dcs.example:contract:expiring"
	_, err := db.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,exp_date)
		VALUES ($1,'','Org A','SIGNED','tpl',1,NOW() - INTERVAL '1 day')`, did)
	require.NoError(t, err)

	repo := &cwepg.PostgresContractRepo{}
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	expired, err := repo.ReadExpiredContracts(ctx, tx)
	require.NoError(t, err)
	var found bool
	for _, c := range expired {
		if c.DID == did {
			require.NoError(t, expiryTask{repo}.handle(ctx, tx, c))
			found = true
		}
	}
	require.True(t, found, "the expired contract was not read")
	require.NoError(t, tx.Commit())

	var status string
	require.NoError(t, db.Get(&status, `SELECT status FROM status_publication_queue WHERE contract_did = $1`, did))
	require.Equal(t, "EXPIRED", status)

	revocations := &provenance.PostgresStatusListRevocations{DB: db}
	allocator := provenance.NewPostgresStatusListAllocator(db, provenance.DefaultListID)
	worker := &statuspublication.Worker{DB: db, Publisher: provenance.NewDCSStatusListPublisher(
		func(listID int) string { return provenance.StatusListURI("https://dcs.example", listID) }, allocator, revocations)}
	processed, err := worker.ProcessOnce(ctx)
	require.NoError(t, err)
	require.True(t, processed)

	entry, err := allocator.Allocate(ctx, did)
	require.NoError(t, err)
	revoked, err := revocations.RevokedIndices(ctx, entry.ListID)
	require.NoError(t, err)
	require.Contains(t, revoked, entry.Index, "the expired contract's revocation bit is not set")
}
