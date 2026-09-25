package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFlightPhaseMigrationPersistsRecordingTimeline(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE Csv (id TEXT PRIMARY KEY, updatedAt INTEGER NOT NULL);
		INSERT INTO Csv VALUES ('recording-1', 1);
	`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/011_flight_phase_detection.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO FlightPhaseRun
		(id, recordingId, flightIndex, flightStatus, phase, startRow, endRow, durationMs, createdAt)
		VALUES ('phase-1', 'recording-1', 1, 'COMPLETE', 'TAKEOFF', 10, 20, 11000, 2)`); err != nil {
		t.Fatal(err)
	}
	var phase string
	if err = db.QueryRow(`SELECT phase FROM FlightPhaseRun WHERE recordingId = 'recording-1'`).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "TAKEOFF" {
		t.Fatalf("phase = %q, want TAKEOFF", phase)
	}
}
