package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/semantichub"
)

// DCS-NFR-PER-02: the validation anchors are per process. When another
// replica activates a hub version, this replica must re-anchor too.
func TestHubActivationOnAnotherReplicaReanchorsThisReplica(t *testing.T) {
	db := pgtest.Open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshed := make(chan struct{}, 1)
	require.NoError(t, FollowHubActivations(ctx, pgtest.URL(t), func(context.Context) error {
		select {
		case refreshed <- struct{}{}:
		default:
		}
		return nil
	}))

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = semantichub.Repo{}.Register(ctx, tx, "follow-test", "shapes", "text/turtle", registerTestShapes, "did:web:other-replica", 0, true)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("an activation committed on another replica did not re-anchor this replica")
	}
}
