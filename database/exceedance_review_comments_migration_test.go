package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestExceedanceReviewCommentMigrationPreservesHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Exceedance (id TEXT PRIMARY KEY, eventStatus TEXT NOT NULL);
		INSERT INTO User VALUES ('reviewer');
		INSERT INTO Exceedance VALUES ('occurrence', 'Valid');`)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile("migrations/008_exceedance_reviews.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO ExceedanceReview
		(id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES ('existing', 'occurrence', 'status_change', 'Pending', 'Valid', 'Validated', 'reviewer', 1)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/022_exceedance_review_comments.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("comment migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO ExceedanceReview
		(id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES ('comment', 'occurrence', 'comment', 'Valid', 'Valid', 'Reviewed by operator', 'reviewer', 2)`); err != nil {
		t.Fatalf("comment audit row failed: %v", err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM ExceedanceReview").Scan(&count); err != nil || count != 2 {
		t.Fatalf("review history was not preserved: count=%d err=%v", count, err)
	}
	if _, err = db.Exec(`INSERT INTO ExceedanceReview
		(id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES ('invalid', 'occurrence', 'unknown', 'Valid', 'Valid', 'Invalid', 'reviewer', 3)`); err == nil {
		t.Fatal("unknown review action should be rejected")
	}
}
