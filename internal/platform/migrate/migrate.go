package migrate

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
)

// RunAll applies any unapplied SQL migrations from fsys in lexicographic filename order.
// It creates a schema_migrations table on first run to track what has been applied.
func RunAll(db *sql.DB, fsys fs.FS) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("migrate: create tracking table: %w", err)
	}

	var files []string
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".sql") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate: walk: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		name := f
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			name = f[idx+1:]
		}

		var exists bool
		if err := db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)`, name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("migrate: check %s: %w", name, err)
		}
		if exists {
			continue
		}

		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}

		log.Printf("migrate: applying %s", name)
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", name, err)
		}

		if _, err := db.Exec(
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			return fmt.Errorf("migrate: record %s: %w", name, err)
		}
	}

	log.Printf("migrate: all migrations applied")
	return nil
}

// WipeSchema drops and recreates the public schema, erasing all tables and data.
// Use only in development/staging when a clean reset is needed before re-migrating.
func WipeSchema(db *sql.DB) error {
	log.Printf("migrate: wiping public schema — all data will be lost")
	if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
		return fmt.Errorf("migrate: drop schema: %w", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
		return fmt.Errorf("migrate: create schema: %w", err)
	}
	if _, err := db.Exec(`GRANT ALL ON SCHEMA public TO public`); err != nil {
		return fmt.Errorf("migrate: grant schema: %w", err)
	}
	log.Printf("migrate: schema wiped — ready for fresh migrations")
	return nil
}
