package semantichub

import (
	"context"
	"testing"

	"digital-contracting-service/internal/base/pgtest"
)

// DCS-NFR-PER-02: the replicas of a fresh deployment seed the hub at the same
// time. Each must start, and each genesis schema must exist once.
func TestReplicasStartingTogetherAllSeed(t *testing.T) {
	db := pgtest.Open(t)
	const replicas = 3
	errs := make(chan error, replicas)
	for i := 0; i < replicas; i++ {
		go func() { errs <- Seed(context.Background(), db) }()
	}
	for i := 0; i < replicas; i++ {
		if err := <-errs; err != nil {
			t.Errorf("replica seed failed: %v", err)
		}
	}
	var versions int
	if err := db.Get(&versions, `SELECT count(*) FROM semantic_schemas WHERE name = $1 AND kind = 'shapes'`, ShapesName); err != nil || versions != 1 {
		t.Fatalf("genesis shapes versions = %d, %v; want 1", versions, err)
	}
}
