package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEventDefinitionV2MigrationResetsOnlyDerivedSafetyData(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	legacySchema := `
		PRAGMA foreign_keys = ON;
		CREATE TABLE Company (id TEXT PRIMARY KEY);
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Aircraft (id TEXT PRIMARY KEY, companyId TEXT, aircraftMake TEXT, modelNumber TEXT);
		CREATE TABLE Csv (id TEXT PRIMARY KEY);
		CREATE TABLE EventLog (id TEXT PRIMARY KEY);
		CREATE TABLE Exceedance (
			id TEXT PRIMARY KEY, exceedanceValues TEXT NOT NULL, flightPhase TEXT NOT NULL,
			parameterName TEXT NOT NULL, description TEXT NOT NULL, eventStatus TEXT NOT NULL,
			aircraftId TEXT NOT NULL, flightId TEXT NOT NULL, file TEXT, eventId TEXT,
			comment TEXT, exceedanceLevel TEXT, createdAt INTEGER NOT NULL, updatedAt INTEGER NOT NULL,
			FOREIGN KEY (eventId) REFERENCES EventLog(id)
		);
		CREATE TABLE Notification (
			id TEXT PRIMARY KEY, userId TEXT NOT NULL, exceedanceId TEXT NOT NULL,
			message TEXT NOT NULL, level TEXT NOT NULL, isRead INTEGER NOT NULL,
			createdAt INTEGER NOT NULL, updatedAt INTEGER NOT NULL
		);
		INSERT INTO Company(id) VALUES ('company-1');
		INSERT INTO User(id) VALUES ('user-1');
		INSERT INTO Aircraft(id, companyId, aircraftMake, modelNumber) VALUES ('aircraft-1', 'company-1', 'Test', 'One');
		INSERT INTO Csv(id) VALUES ('flight-1');
		INSERT INTO EventLog(id) VALUES ('event-1');
		INSERT INTO Exceedance VALUES ('ex-1', '[]', 'LANDING', 'P', 'D', 'Valid', 'aircraft-1', 'flight-1', NULL, 'event-1', NULL, 'LOW', 1, 1);
		INSERT INTO Notification VALUES ('n-1', 'user-1', 'ex-1', 'message', 'Level 1', 0, 1, 1);
	`
	if _, err = db.Exec(legacySchema); err != nil {
		t.Fatalf("failed to create legacy fixture: %v", err)
	}

	migration, err := os.ReadFile("migrations/005_event_definitions_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	detectionMigration, err := os.ReadFile("migrations/006_detection_engine_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(detectionMigration)); err != nil {
		t.Fatalf("detection migration failed: %v", err)
	}

	for _, table := range []string{"EventDefinition", "EventDefinitionVersion", "EventDefinitionAssignment", "DetectionRun", "Exceedance", "Notification"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}
	var viewCount int
	if err := db.QueryRow("SELECT COUNT(1) FROM sqlite_master WHERE type = 'view' AND name = 'EventLog'").Scan(&viewCount); err != nil || viewCount != 1 {
		t.Fatal("expected EventLog compatibility view")
	}
	for _, table := range []string{"Exceedance", "Notification"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(1) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("expected %s data to be cleared", table)
		}
	}
	var companyCount int
	if err := db.QueryRow("SELECT COUNT(1) FROM Company").Scan(&companyCount); err != nil || companyCount != 1 {
		t.Fatal("company data was not preserved")
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left a foreign-key violation")
	}
}
