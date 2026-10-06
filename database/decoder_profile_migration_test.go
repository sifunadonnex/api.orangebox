package database

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDecoderProfileMigrationVersionsAndLocksPublishedProfile(t *testing.T) {
	db, err := sql.Open("sqlite", "file:decoder-profile-migration?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA foreign_keys = ON;
		CREATE TABLE User (id TEXT PRIMARY KEY);
		CREATE TABLE Aircraft (id TEXT PRIMARY KEY);`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("migrations/015_aircraft_decoder_profiles.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	recordingValidationMigration, err := os.ReadFile("migrations/016_decoder_profile_recording_validation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(recordingValidationMigration)); err != nil {
		t.Fatalf("recording validation migration failed: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO Aircraft (id) VALUES ('aircraft-a');
		INSERT INTO AircraftDecoderProfile
		(id, aircraftId, version, name, parameterFormat, parameterFileName, parameterText,
		 decoderConfig, checksum, notes, status, validationStatus, validationSummary, createdAt)
		VALUES ('profile-1', 'aircraft-a', 1, 'Profile 1', 'tbx', 'one.tbx', '[PARAMETER A]',
		 '{}', 'hash-1', 'initial', 'published', 'passed', '{}', 1);`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO AircraftDecoderProfile
		(id, aircraftId, version, name, parameterFormat, parameterFileName, parameterText,
		 decoderConfig, checksum, notes, status, validationStatus, validationSummary, createdAt)
		VALUES ('profile-2', 'aircraft-a', 2, 'Profile 2', 'tbx', 'two.tbx', '[PARAMETER A]',
		 '{}', 'hash-2', 'replacement', 'published', 'passed', '{}', 2)`); err == nil {
		t.Fatal("expected one published profile per aircraft")
	}
	if _, err = db.Exec(`UPDATE AircraftDecoderProfile SET validationRecordingName = 'sample.ddf',
		validationRecordingChecksum = 'recording-hash', validatedAt = 3 WHERE id = 'profile-1'`); err != nil {
		t.Fatalf("recording validation evidence could not be persisted: %v", err)
	}
}
