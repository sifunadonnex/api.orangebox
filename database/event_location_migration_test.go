package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEventLocationMigrationValidatesCoordinatesAndCascades(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA foreign_keys = ON; CREATE TABLE Exceedance (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/010_event_locations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO Exceedance VALUES ('event-1')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO ExceedanceLocation VALUES ('event-1', -1.25, 36.8, 5000, 12000, 250, 'replay_nearest_time', 'near', 1, 1)`); err != nil {
		t.Fatalf("valid location was rejected: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO Exceedance VALUES ('event-2')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO ExceedanceLocation VALUES ('event-2', 91, 36.8, NULL, 12000, 0, 'replay_nearest_time', 'exact', 1, 1)`); err == nil {
		t.Fatal("invalid latitude should be rejected")
	}
	if _, err = db.Exec(`DELETE FROM Exceedance WHERE id = 'event-1'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(1) FROM ExceedanceLocation WHERE exceedanceId = 'event-1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected cascade delete, found %d location rows", count)
	}
}
