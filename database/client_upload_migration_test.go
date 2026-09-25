package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestClientUploadMigrationAddsProvenanceAndDuplicateGuard(t *testing.T) {
	db, err := sql.Open("sqlite", "file:client-upload-migration?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Csv (id TEXT PRIMARY KEY, aircraftId TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/012_client_recording_uploads.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO User (id) VALUES ('user-a');
		INSERT INTO Csv (id, aircraftId, contentHash, uploadedBy, uploadSource)
		VALUES ('recording-a', 'aircraft-a', 'hash-a', 'user-a', 'client_portal');`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO Csv (id, aircraftId, contentHash)
		VALUES ('recording-b', 'aircraft-a', 'hash-a')`); err == nil {
		t.Fatal("expected duplicate content hash for one aircraft to be rejected")
	}
	if _, err = db.Exec(`INSERT INTO Csv (id, aircraftId, contentHash)
		VALUES ('recording-c', 'aircraft-b', 'hash-a')`); err != nil {
		t.Fatalf("the same source data may belong to a different aircraft: %v", err)
	}
}
