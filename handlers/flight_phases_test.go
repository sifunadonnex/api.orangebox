package handlers

import (
	"database/sql"
	"encoding/csv"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fdm-backend/detection"
	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestStreamCSVRangeUsesTelemetryHeaderAndAddsDetectedPhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garmin.csv")
	contents := "#airframe_info,Product=GIFD\n#yyy-mm-dd,hh:mm:ss,kt,ft\nLcl Date,Lcl Time,IAS,AltInd\n2026-07-11,06:36:52,70,5200\n2026-07-11,06:36:53,80,5250\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	runs := []detection.PhaseRun{{Phase: "TAKEOFF", StartRow: 4, EndRow: 5, DurationMs: 2000}}
	if err := streamCSVRange(context, path, "Test flight", 4, 5, runs); err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(recorder.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0][0] != "Lcl Date" || records[0][4] != "Detected Phase" {
		t.Fatalf("unexpected enriched CSV header: %#v", records)
	}
	if records[1][4] != "TAKEOFF" || records[2][4] != "TAKEOFF" {
		t.Fatalf("detected phases were not appended: %#v", records)
	}
}

func TestIntegratedFlightPhasePathWithRealFixture(t *testing.T) {
	fixtureDir := os.Getenv("PHASE_FIXTURE_DIR")
	if fixtureDir == "" {
		t.Skip("PHASE_FIXTURE_DIR is not set")
	}
	matches, err := filepath.Glob(filepath.Join(fixtureDir, "*.csv"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("find real phase fixture: matches=%d err=%v", len(matches), err)
	}
	model := "208B"
	fixture := ""
	var result detection.PhaseDetectionResult
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || info.Size() == 0 {
			continue
		}
		candidate, detectErr := detectUploadPhases(match, models.Aircraft{AircraftMake: "Cessna", ModelNumber: &model}, 1000)
		if detectErr == nil && len(candidate.Flights) > 0 {
			fixture = match
			result = candidate
			break
		}
	}
	if fixture == "" {
		t.Fatal("no real fixture with a detected flight was found")
	}
	if len(result.Runs) == 0 {
		t.Fatal("real fixture produced no phase runs")
	}
	if _, err = phaseSegments(fixture, result); err != nil {
		t.Fatalf("phase-derived boundaries were not upload-safe: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE Csv (
			id TEXT PRIMARY KEY, updatedAt INTEGER NOT NULL,
			phaseEngineVersion TEXT, phaseProfile TEXT, phaseTimingSource TEXT, phaseSummary TEXT
		);
		CREATE TABLE FlightPhaseRun (
			id TEXT PRIMARY KEY, recordingId TEXT NOT NULL, flightIndex INTEGER NOT NULL,
			flightStatus TEXT, phase TEXT NOT NULL, startRow INTEGER NOT NULL, endRow INTEGER NOT NULL,
			startSample TEXT, endSample TEXT, durationMs INTEGER NOT NULL, createdAt INTEGER NOT NULL,
			FOREIGN KEY (recordingId) REFERENCES Csv(id) ON DELETE CASCADE
		);
		INSERT INTO Csv (id, updatedAt) VALUES ('recording-1', 1);
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = persistPhaseDetection(tx, "recording-1", result, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	handler := NewCSVHandler(db)
	stored, err := handler.loadFlightPhaseRuns("recording-1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(result.Runs) {
		t.Fatalf("stored phase runs = %d, want %d", len(stored), len(result.Runs))
	}
	t.Logf("integrated %s: profile=%s flights=%d phaseRuns=%d", filepath.Base(fixture), result.Profile.Code, len(result.Flights), len(stored))
}
