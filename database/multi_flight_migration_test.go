package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMultiFlightMigrationBackfillsLegacyFlightOwnership(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE Aircraft (id TEXT PRIMARY KEY);
		CREATE TABLE Csv (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, aircraftId TEXT NOT NULL,
			status TEXT, departure TEXT, pilot TEXT, destination TEXT,
			flightHours TEXT, analysisSummary TEXT, createdAt INTEGER NOT NULL,
			updatedAt INTEGER NOT NULL,
			FOREIGN KEY (aircraftId) REFERENCES Aircraft(id)
		);
		CREATE TABLE DetectionRun (
			id TEXT PRIMARY KEY, flightId TEXT NOT NULL, engineVersion TEXT,
			inputHash TEXT, sampleIntervalMs INTEGER, ruleSetHash TEXT, status TEXT,
			FOREIGN KEY (flightId) REFERENCES Csv(id)
		);
		CREATE TABLE Exceedance (
			id TEXT PRIMARY KEY, flightId TEXT NOT NULL, isCurrent INTEGER NOT NULL,
			FOREIGN KEY (flightId) REFERENCES Csv(id)
		);
		INSERT INTO Aircraft VALUES ('aircraft-1');
		INSERT INTO Csv VALUES ('recording-1', 'Legacy flight', 'aircraft-1',
			'completed', 'A', 'Pilot', 'B', '1.0', '{}', 1, 2);
		INSERT INTO DetectionRun VALUES ('run-1', 'recording-1', '2.0.0', 'hash', 1000, 'rules', 'completed');
		INSERT INTO Exceedance VALUES ('occurrence-1', 'recording-1', 1);
	`)
	if err != nil {
		t.Fatal(err)
	}

	migration, err := os.ReadFile("migrations/009_multi_flight_recordings.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}

	var legID, runLegID, occurrenceLegID string
	if err = db.QueryRow("SELECT id FROM FlightLeg WHERE recordingId = 'recording-1'").Scan(&legID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT flightLegId FROM DetectionRun WHERE id = 'run-1'").Scan(&runLegID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT flightLegId FROM Exceedance WHERE id = 'occurrence-1'").Scan(&occurrenceLegID); err != nil {
		t.Fatal(err)
	}
	if legID != "recording-1" || runLegID != legID || occurrenceLegID != legID {
		t.Fatalf("unexpected backfill: leg=%q run=%q occurrence=%q", legID, runLegID, occurrenceLegID)
	}
}
