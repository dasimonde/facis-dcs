// Package pgtest gives tests a migrated Postgres schema of their own.
package pgtest

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/migrations"
)

// URL returns QA_DATABASE_URL. Without it the test is skipped, except in CI
// (CI set), where the test fails: CI must run every Postgres-backed test.
func URL(t testing.TB) string {
	t.Helper()
	base := os.Getenv("QA_DATABASE_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("QA_DATABASE_URL not set in CI; a Postgres-backed test must not be skipped")
		}
		t.Skip("QA_DATABASE_URL not set; skipping Postgres-backed test")
	}
	return base
}

// Open returns a database handle on a fresh schema with every migration
// applied. See URL for when the test is skipped. The schema is dropped when
// the test ends.
func Open(t *testing.T) *sqlx.DB {
	t.Helper()
	base := URL(t)
	schema := "t_" + strings.ToLower(rand.Text()[:12])

	admin, err := sqlx.Open("postgres", base)
	if err != nil {
		t.Fatalf("open %s: %v", base, err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse QA_DATABASE_URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		t.Fatalf("open schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		admin, err := sqlx.Open("postgres", base)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close() }()
		_, _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema))
	})
	if err := migrations.Run(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}
