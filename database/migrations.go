package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunMigrations executes all SQL migrations in the migrations directory
func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _schema_migrations (
		name TEXT PRIMARY KEY,
		appliedAt INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to initialize migration ledger: %w", err)
	}

	// Get migration files
	migrationFiles, err := filepath.Glob("database/migrations/*.sql")
	if err != nil {
		return fmt.Errorf("failed to find migration files: %w", err)
	}

	sort.Strings(migrationFiles)

	// Execute each migration exactly once. Older migrations in this project are
	// idempotent and will be recorded the first time this ledger-aware runner is
	// used against an existing database.
	for _, file := range migrationFiles {
		name := filepath.Base(file)
		var applied int
		err := db.QueryRow("SELECT COUNT(1) FROM _schema_migrations WHERE name = ?", name).Scan(&applied)
		if err != nil {
			return fmt.Errorf("failed to inspect migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		log.Printf("Running migration: %s", name)

		// Read migration file
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		if _, err = db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", name, err)
		}

		if _, err = db.Exec(
			"INSERT INTO _schema_migrations (name, appliedAt) VALUES (?, ?)",
			name,
			time.Now().UnixMilli(),
		); err != nil {
			return fmt.Errorf("migration %s ran but could not be recorded: %w", name, err)
		}

		log.Printf("Successfully executed migration: %s", name)
	}

	return nil
}
