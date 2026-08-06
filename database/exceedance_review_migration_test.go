package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestExceedanceReviewMigrationPreservesOccurrencesAndNormalizesStatuses(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err = db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Exceedance (id TEXT PRIMARY KEY, eventStatus TEXT NOT NULL);
		INSERT INTO User (id) VALUES ('reviewer-1');
		INSERT INTO Exceedance (id, eventStatus) VALUES
			('occurrence-1', 'Pending'),
			('occurrence-2', 'Under Review'),
			('occurrence-3', 'Invalid');
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := os.ReadFile("migrations/008_exceedance_reviews.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}

	var occurrenceCount, pendingCount, falseCount, reviewTableCount int
	if err = db.QueryRow("SELECT COUNT(*) FROM Exceedance").Scan(&occurrenceCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM Exceedance WHERE eventStatus = 'Pending'").Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM Exceedance WHERE eventStatus = 'False'").Scan(&falseCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'ExceedanceReview'").Scan(&reviewTableCount); err != nil {
		t.Fatal(err)
	}

	if occurrenceCount != 3 || pendingCount != 2 || falseCount != 1 || reviewTableCount != 1 {
		t.Fatalf("unexpected migrated state: occurrences=%d pending=%d false=%d reviews=%d",
			occurrenceCount, pendingCount, falseCount, reviewTableCount)
	}
}
