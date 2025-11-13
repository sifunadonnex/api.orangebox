package database

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
)

// RunMigrations executes all SQL migrations in the migrations directory
func RunMigrations(db *sql.DB) error {
	// Get migration files
	migrationFiles, err := filepath.Glob("database/migrations/*.sql")
	if err != nil {
		return fmt.Errorf("failed to find migration files: %w", err)
	}

	// Execute each migration file
	for _, file := range migrationFiles {
		log.Printf("Running migration: %s", file)
		
		// Read migration file
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		// Execute migration
		_, err = db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", file, err)
		}

		log.Printf("Successfully executed migration: %s", file)
	}

	return nil
}