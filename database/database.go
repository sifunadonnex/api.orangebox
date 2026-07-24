package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB initializes the database connection
func InitDB() (*sql.DB, error) {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Construct the database path
	dbPath := filepath.Join(wd, "prisma", "dev.db")

	// Check if database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file does not exist at path: %s", dbPath)
	}

	// Apply connection-local SQLite settings to every pooled connection.
	// A single connection can deadlock when a handler needs a nested read
	// while iterating an open result set.
	dsn := dbPath + "?_pragma=foreign_keys%3dON&_pragma=busy_timeout%3d5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// WAL allows readers to continue while a writer is active. The mode is
	// persistent for the database; busy_timeout is applied per connection by
	// the DSN above.
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL journal mode: %w", err)
	}

	log.Println("Database connection established successfully")
	return db, nil
}

// CreateTables creates the necessary tables if they don't exist
// Note: Since we're using the existing Prisma database, we don't need to create tables
func CreateTables(db *sql.DB) error {
	// Tables are already created by Prisma migrations
	// This function is kept for potential future schema changes
	return nil
}
