package migrations_test

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/migrations"
)

const drillDID = "did:web:dcs.example:contract:restore-drill"

// DCS-NFR-SF-03: a dump made the way the backup CronJob makes it
// (deployment/helm/templates/backup-cronjob.yaml: pg_dump --format=custom)
// restores into an empty database with the restore command of
// docs/backup-integration-guide.md §3. The restored database starts like a
// backend start does (migrations.Run is a no-op on it), every table holds the
// same rows, and the contract and its wrapped content key read back.
func TestBackupDumpRestoresIntoAnEmptyDatabase(t *testing.T) {
	base := pgtest.URL(t)
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s is required for the restore drill: %v", tool, err)
		}
	}
	src := pgtest.Open(t)
	var schema string
	if err := src.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO contracts (did, created_by, state, template_did, template_version, contract_data)
		 VALUES ($1, 'did:web:creator', 'SIGNED', 'did:web:tpl', 1, '{"clause":"terms"}')`,
		`INSERT INTO content_encryption_keys (scope_kind, scope_id, recipient_did, wrapped_cek)
		 VALUES ('contract', $1, 'did:web:dcs.example', '{"alg":"ECDH-ES+A256KW","encrypted_key":"x"}')`,
	} {
		if _, err := src.Exec(statement, drillDID); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	started := time.Now()
	dump := filepath.Join(t.TempDir(), "dcs.dump")
	run(t, "pg_dump", "--format=custom", "--schema="+schema, "--dbname="+base, "--file="+dump)

	admin, err := sqlx.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	database := "r_" + strings.ToLower(rand.Text()[:12])
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, database)); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(fmt.Sprintf(`DROP DATABASE "%s" WITH (FORCE)`, database)) })
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	run(t, "pg_restore", "--no-owner", "--exit-on-error", "--dbname="+u.String(), dump)
	t.Logf("dump and restore took %s", time.Since(started))

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	restored, err := sqlx.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if err := migrations.Run(restored); err != nil {
		t.Fatalf("the restored database does not start: %v", err)
	}

	var tables []string
	if err := src.Select(&tables, `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY 1`); err != nil {
		t.Fatal(err)
	}
	if len(tables) < 20 {
		t.Fatalf("only %d tables in the source schema", len(tables))
	}
	for _, table := range tables {
		digest := fmt.Sprintf(`SELECT count(*) || ':' || coalesce(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM %q t`, table)
		var want, got string
		if err := src.Get(&want, digest); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if err := restored.Get(&got, digest); err != nil {
			t.Fatalf("restored %s: %v", table, err)
		}
		if got != want {
			t.Errorf("table %s: restored %s, source %s", table, got, want)
		}
	}

	var data, wrapped string
	if err := restored.Get(&data, `SELECT contract_data::text FROM contracts WHERE did = $1`, drillDID); err != nil {
		t.Fatalf("read the contract back: %v", err)
	}
	if err := restored.Get(&wrapped, `SELECT wrapped_cek::text FROM content_encryption_keys WHERE scope_id = $1`, drillDID); err != nil {
		t.Fatalf("read the wrapped content key back: %v", err)
	}
	if !strings.Contains(data, "terms") || !strings.Contains(wrapped, "ECDH-ES+A256KW") {
		t.Fatalf("restored contract %s, key %s", data, wrapped)
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "PGCONNECT_TIMEOUT=10")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
}
