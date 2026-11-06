package pq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
)

// A failed erase request waits for its backoff before the next attempt,
// and a refused one is not sent again. Both keep the peer's answer.
func TestPendingErasuresBackOffAndEndOnRefusal(t *testing.T) {
	repo := &PostgresEraseRepository{DB: pgtest.Open(t)}
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, "c1", "did:web:down.example"))
	require.NoError(t, repo.Upsert(ctx, "c1", "did:web:stranger.example"))

	pending, err := repo.GetPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 2)

	require.NoError(t, repo.RecordAttempt(ctx, "c1", "did:web:down.example", "connection refused"))
	require.NoError(t, repo.Refuse(ctx, "c1", "did:web:stranger.example", "not_found"))
	pending, err = repo.GetPending(ctx)
	require.NoError(t, err)
	require.Empty(t, pending, "a request was retried within its backoff, or after a final refusal")

	_, err = repo.DB.Exec(`UPDATE contract_erasures SET last_tried_at = now() - INTERVAL '1 hour'`)
	require.NoError(t, err)
	pending, err = repo.GetPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "did:web:down.example", pending[0].PeerDID)
	require.Equal(t, "connection refused", *pending[0].LastError)

	all, err := repo.ListByDID(ctx, "c1")
	require.NoError(t, err)
	for _, request := range all {
		if request.PeerDID == "did:web:stranger.example" {
			require.NotNil(t, request.RefusedAt)
			require.Equal(t, "not_found", *request.LastError)
		}
	}
}
