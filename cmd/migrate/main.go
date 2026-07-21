package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"fdm-backend/database"

	_ "modernc.org/sqlite"
)

func main() {
	databasePath := flag.String("database", "prisma/dev.db", "path to the SQLite database")
	backupPath := flag.String("backup", "", "optional path for a pre-migration database copy")
	flag.Parse()

	resolvedDatabase, err := filepath.Abs(*databasePath)
	if err != nil {
		log.Fatal(err)
	}
	if *backupPath != "" {
		resolvedBackup, err := filepath.Abs(*backupPath)
		if err != nil {
			log.Fatal(err)
		}
		if err = copyFile(resolvedDatabase, resolvedBackup); err != nil {
			log.Fatalf("backup failed: %v", err)
		}
		log.Printf("Pre-migration backup created at %s", resolvedBackup)
	}

	db, err := sql.Open("sqlite", resolvedDatabase)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatal(err)
	}
	if err = database.RunMigrations(db); err != nil {
		log.Fatal(err)
	}

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		log.Fatal("migration completed with a foreign-key violation")
	}

	var definitions, exceedances int
	if err = db.QueryRow("SELECT COUNT(1) FROM EventDefinition").Scan(&definitions); err != nil {
		log.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(1) FROM Exceedance").Scan(&exceedances); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Migration complete: definitions=%d exceedances=%d\n", definitions, exceedances)
}

func copyFile(source, destination string) error {
	if source == destination {
		return fmt.Errorf("backup path must differ from database path")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
