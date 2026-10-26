package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

func TestLogicalFlightIndexSupportsLegacyAndLegLookups(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE Exceedance (id TEXT PRIMARY KEY, flightId TEXT, flightLegId TEXT, isCurrent INTEGER);
		INSERT INTO Exceedance VALUES ('legacy','flight-a',NULL,1), ('leg','recording-a','flight-a',1), ('old','recording-a','flight-a',0);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/023_exceedance_logical_flight_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if _, err = db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	var count, foreignKeys int
	if err = db.QueryRow(`SELECT COUNT(*) FROM Exceedance WHERE COALESCE(flightLegId, flightId) IN (?, ?) AND isCurrent = 1`, "flight-a", "missing").Scan(&count); err != nil || count != 2 {
		t.Fatalf("logical flight lookup: %d %v", count, err)
	}
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatal("migration changed foreign-key enforcement")
	}
	var id, parent, unused int
	var plan string
	err = db.QueryRow(`EXPLAIN QUERY PLAN SELECT id FROM Exceedance WHERE COALESCE(flightLegId, flightId) IN (?, ?) AND isCurrent = 1`, "flight-a", "missing").Scan(&id, &parent, &unused, &plan)
	if err != nil || !strings.Contains(plan, "Exceedance_logical_flight_current_idx") {
		t.Fatalf("lookup does not use index: %s %v", plan, err)
	}
}
