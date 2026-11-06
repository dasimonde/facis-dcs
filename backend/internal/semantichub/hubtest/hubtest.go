// Package hubtest gives tests a migrated Postgres schema with a seeded
// Semantic Hub installed as the validation shape source.
package hubtest

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
)

// Open returns pgtest.Open's database with the hub seeded and installed as the
// process-wide shape source. Contract and template writes need both. The prior
// source is restored before the database closes, so no later test reads a
// closed database.
func Open(t *testing.T) *sqlx.DB {
	t.Helper()
	db := pgtest.Open(t)
	if err := semantichub.Seed(context.Background(), db); err != nil {
		t.Fatalf("seed semantic hub: %v", err)
	}
	t.Cleanup(validation.SwapShapeSource(semantichub.HubShapeSource{DB: db}))
	return db
}
