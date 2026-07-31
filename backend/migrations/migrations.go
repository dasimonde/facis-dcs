package migrations

import (
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Run executes all SQL migration files that haven't been applied yet.
// Files are sorted alphabetically (use naming like 20260128_*, 20260206_*, etc).
func Run(db *sqlx.DB) error {
	// Ensure migrations tracking table exists
	if _, err := db.Exec(createMigrationsTable); err != nil {
		return err
	}

	// Get list of already applied migrations
	applied := make(map[string]bool)
	rows, err := db.Query("SELECT filename FROM schema_migrations")
	if err != nil {
		return err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Println(err)
		}
	}(rows)
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return err
		}
		applied[filename] = true
	}

	// Read all SQL files
	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		return err
	}

	// Sort files alphabetically to ensure consistent ordering
	var fileNames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			fileNames = append(fileNames, entry.Name())
		}
	}
	sort.Strings(fileNames)

	// Run pending migrations
	for _, fileName := range fileNames {
		if applied[fileName] {
			log.Printf("Migration already applied: %s", fileName)
			continue
		}

		content, err := fs.ReadFile(sqlFiles, "sql/"+fileName)
		if err != nil {
			return err
		}

		log.Printf("Running migration: %s", fileName)

		// Execute migration in a transaction
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(string(content)); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return rollbackErr
			}
			return err
		}

		// Record migration as applied
		if _, err := tx.Exec("INSERT INTO schema_migrations (filename) VALUES ($1)", fileName); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return rollbackErr
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		log.Printf("Migration applied: %s", fileName)
	}

	log.Println("All migrations completed")
	return nil
}
