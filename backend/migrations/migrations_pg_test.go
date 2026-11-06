package migrations_test

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/migrations"
)

// DCS-NFR-PER-02: the replicas of a fresh deployment start together and all
// run the migrations on the same empty database. Each must start; none may
// fail on a migration another replica is applying.
func TestReplicasStartingTogetherAllMigrate(t *testing.T) {
	base := pgtest.URL(t)
	schema := "t_" + strings.ToLower(rand.Text()[:12])
	admin, err := sqlx.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema)) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	const replicas = 3
	errs := make(chan error, replicas)
	for i := 0; i < replicas; i++ {
		db, err := sqlx.Open("postgres", u.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		go func() { errs <- migrations.Run(db) }()
	}
	for i := 0; i < replicas; i++ {
		if err := <-errs; err != nil {
			t.Errorf("replica migration failed: %v", err)
		}
	}
}
