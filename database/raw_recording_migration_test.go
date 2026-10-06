package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRawRecordingMigrationStoresDecoderProvenance(t *testing.T) {
	db, err := sql.Open("sqlite", "file:raw-recording-migration?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE AircraftDecoderProfile (id TEXT PRIMARY KEY);
		CREATE TABLE Csv (id TEXT PRIMARY KEY);`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/017_raw_recording_decoder_provenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO AircraftDecoderProfile (id) VALUES ('profile-1');
		INSERT INTO Csv (id, rawSourceFormat, rawSourceFile, decoderProfileId,
			decoderProfileVersion, decoderProfileChecksum)
		VALUES ('recording-1', 'ddf', 'source.ddf', 'profile-1', 3, 'profile-hash');`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO Csv (id, rawSourceFormat) VALUES ('invalid', 'fdr')`); err == nil {
		t.Fatal("expected an unsupported raw source format to be rejected")
	}
}
