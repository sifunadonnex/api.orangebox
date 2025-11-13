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

	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
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
