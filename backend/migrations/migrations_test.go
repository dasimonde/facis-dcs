package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// "A field can never be signed twice" (DCS-FR-SM-07/-17) is a rule two
// concurrent submits can only be held to by the database: each reads the other's
// row before it exists, so whichever check the application runs, the constraint
// is what makes a second SIGNED row impossible. 20260714c stated the rule and
// created a plain index.
//
// It must be PARTIAL: a revoked signature keeps its row and the field may be
// signed again, and signatures predating field_name carry none — a unique index
// over the bare column pair would reject both.
func TestOneSignedSignaturePerFieldIsEnforcedByAUniqueIndex(t *testing.T) {
	statement := regexp.MustCompile(
		`(?is)CREATE\s+UNIQUE\s+INDEX[^;]*?ON\s+contract_signatures\s*\(\s*contract_did\s*,\s*field_name\s*\)([^;]*);`)

	found := ""
	for name, sql := range migrationSQL(t) {
		if m := statement.FindStringSubmatch(sql); m != nil {
			found = name + ": " + m[1]
			break
		}
	}
	if found == "" {
		t.Fatal("no unique index on contract_signatures (contract_did, field_name): a second SIGNED row for one field is only prevented by application code")
	}
	if !strings.Contains(found, "WHERE") || !strings.Contains(found, "'SIGNED'") {
		t.Errorf("the unique index is not restricted to SIGNED rows, so a revoked-then-re-signed field would be rejected: %s", found)
	}
	if !strings.Contains(found, "field_name IS NOT NULL") {
		t.Errorf("the unique index is not restricted to rows that name a field, so the single-signer rows predating field_name would collide: %s", found)
	}
}

// A status list entry is a contract's revocation bit, so two contracts holding
// one entry means terminating either revokes both. The allocation table is what
// stops that, and it only stops it if the slot index is UNIQUE and PARTIAL:
// unique so two allocations cannot meet, partial because assignments inherited
// from the retired hash scheme may already collide and the migration preserves
// them rather than failing on the deployments that have the problem.
func TestAllocatedStatusListEntriesAreUniquePerSlot(t *testing.T) {
	statement := regexp.MustCompile(
		`(?is)CREATE\s+UNIQUE\s+INDEX[^;]*?ON\s+status_list_entries\s*\(\s*list_id\s*,\s*entry_index\s*\)([^;]*);`)

	found := ""
	for name, sql := range migrationSQL(t) {
		if m := statement.FindStringSubmatch(sql); m != nil {
			found = name + ": " + m[1]
			break
		}
	}
	if found == "" {
		t.Fatal("no unique index on status_list_entries (list_id, entry_index): two contracts can be allocated one revocation bit")
	}
	if !strings.Contains(found, "WHERE") || !strings.Contains(found, "'allocated'") {
		t.Errorf("the unique index is not restricted to allocated rows, so a deployment whose inherited hash indices already collide cannot migrate: %s", found)
	}
}

// Contracts that predate the allocation table have credentials in the wild
// advertising a hash-derived entry. The backfill is what keeps those credentials
// resolvable — without it every existing contract would be handed a fresh entry
// and its issued credentials would point at a bit nothing updates.
func TestExistingSubjectsKeepTheirInheritedStatusListEntry(t *testing.T) {
	backfill := regexp.MustCompile(`(?is)INSERT\s+INTO\s+status_list_entries[^;]*?FROM\s+(contracts|contract_templates)[^;]*;`)

	sources := map[string]bool{}
	for _, sql := range migrationSQL(t) {
		for _, m := range backfill.FindAllStringSubmatch(sql, -1) {
			if !strings.Contains(m[0], "sha256") {
				t.Errorf("the backfill does not reproduce the retired index expression, so it moves existing contracts off the entry their credentials name: %s", m[0])
			}
			sources[m[1]] = true
		}
	}
	for _, table := range []string{"contracts", "contract_templates"} {
		if !sources[table] {
			t.Errorf("no status_list_entries backfill from %s: its existing subjects would be reallocated", table)
		}
	}
}

// migrationSQL returns the contents of every embedded migration file.
func migrationSQL(t *testing.T) map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	contents := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(sqlFiles, "sql/"+entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		contents[entry.Name()] = string(body)
	}
	return contents
}
