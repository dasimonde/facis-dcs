package command

import (
	"context"
	"fmt"
	"testing"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-NFR-PER-02: the replicas of a fresh deployment register the configured
// contract targets at the same time. Each must start, and each target must
// exist once.
func TestReplicasStartingTogetherAllSeedContractTargets(t *testing.T) {
	database := pgtest.Open(t)
	var entries []SeedTarget
	for i := 0; i < 30; i++ {
		entries = append(entries, SeedTarget{Name: fmt.Sprintf("target-%d", i), URL: "https://target.example/deploy"})
	}
	const replicas = 8
	errs := make(chan error, replicas)
	for i := 0; i < replicas; i++ {
		go func() {
			_, err := SeedContractTargets(context.Background(), database, &pg.PostgresContractTargetRepo{}, entries)
			errs <- err
		}()
	}
	for i := 0; i < replicas; i++ {
		if err := <-errs; err != nil {
			t.Errorf("replica target seed failed: %v", err)
		}
	}
	var n int
	if err := database.Get(&n, `SELECT count(*) FROM contract_targets`); err != nil || n != len(entries) {
		t.Fatalf("targets = %d, %v; want %d, each once", n, err, len(entries))
	}
}
