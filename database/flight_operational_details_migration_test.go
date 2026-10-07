package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFlightOperationalDetailsMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err = db.Exec(`CREATE TABLE FlightLeg (id TEXT PRIMARY KEY, pilot TEXT);
		INSERT INTO FlightLeg (id, pilot) VALUES ('flight-1', ' pic-17 ');`); err != nil {
		t.Fatalf("create legacy flight table: %v", err)
	}

	migrationPath := filepath.Join("migrations", "019_flight_operational_details.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var picCode string
	if err = db.QueryRow(`SELECT picCrewCode FROM FlightLeg WHERE id = 'flight-1'`).Scan(&picCode); err != nil {
		t.Fatalf("read migrated PIC code: %v", err)
	}
	if picCode != "PIC-17" {
		t.Fatalf("expected legacy pilot to be backfilled as PIC-17, got %q", picCode)
	}

	if _, err = db.Exec(`UPDATE FlightLeg SET flightDate = '2026-06-05', flightNumber = 'KQ100',
		takeoffWeight = 18450, landingWeight = 17600, weightUnit = 'kg', vref = 126,
		techLogReference = 'TL-22', loadSheetNumber = 'LS-9' WHERE id = 'flight-1'`); err != nil {
		t.Fatalf("write operational details: %v", err)
	}
}
