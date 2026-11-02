package migrations_test

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/migrations"
)

// The erasure migration upgrades a database that holds a key shredded before
// it: such a row still has its wrapped key, and the migration removes it.
func TestErasureMigrationUpgradesAShreddedKeyThatKeptItsWrappedKey(t *testing.T) {
	const erasure = "20261101_archive_1_erasure.sql"
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
	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Mark the erasure migration and every later one as applied, so the
	// first run stops at the schema before it.
	files, err := os.ReadDir("sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (filename VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= erasure {
			if _, err := db.Exec(`INSERT INTO schema_migrations (filename) VALUES ($1)`, file.Name()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := migrations.Run(db); err != nil {
		t.Fatalf("migrate to the schema before %s: %v", erasure, err)
	}
	if _, err := db.Exec(`INSERT INTO content_encryption_keys (scope_kind, scope_id, recipient_did, wrapped_cek, shredded_at, shredded_by, shred_reason)
		VALUES ('contract', 'did:web:x:contract:1', 'did:web:x', '{"k":"wrapped"}', now(), 'archive manager', 'gdpr')`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE filename >= $1`, erasure); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatalf("upgrade with a shredded key: %v", err)
	}
	var keys int
	if err := db.Get(&keys, `SELECT count(*) FROM content_encryption_keys WHERE wrapped_cek IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if keys != 0 {
		t.Fatalf("the shredded row keeps its wrapped key")
	}
}
