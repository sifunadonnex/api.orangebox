package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"fdm-backend/detection"
	"fdm-backend/ingestion"
	"fdm-backend/models"
)

func TestBeaconRawRecordingFeedsCanonicalFlightPipeline(t *testing.T) {
	directory := os.Getenv("BEACON_FLIGHTDATA_DIR")
	if directory == "" {
		t.Skip("BEACON_FLIGHTDATA_DIR is not set")
	}
	profile, err := os.ReadFile(filepath.Join(directory, "CRJ100_200_440-SL-31-014_128WPS_REV00_D_FRED.xml"))
	if err != nil {
		t.Fatal(err)
	}
	recording, err := os.ReadFile(filepath.Join(directory, "5Y-DRM080926.ddf"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath := filepath.Join(t.TempDir(), "beacon-canonical.csv")
	conversion, err := ingestion.DecodeFREDToCanonicalCSV(profile, recording, canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	segments, _, err := detection.DetectRecordingSegments(canonicalPath)
	if err != nil || len(segments) == 0 {
		t.Fatalf("canonical segmentation failed: segments=%d err=%v", len(segments), err)
	}
	model := "CRJ-200"
	phases, err := detectUploadPhases(canonicalPath, models.Aircraft{
		AircraftMake: "Bombardier", ModelNumber: &model,
	}, conversion.SampleIntervalMs)
	if err != nil {
		t.Fatal(err)
	}
	if phases.RowCount != conversion.Rows || len(phases.Runs) == 0 {
		t.Fatalf("canonical phase detection did not consume the conversion: rows=%d/%d runs=%d", phases.RowCount, conversion.Rows, len(phases.Runs))
	}
	t.Logf("canonical pipeline produced %d conservative segments, %d phase runs, and %d airborne flights", len(segments), len(phases.Runs), len(phases.Flights))
}
