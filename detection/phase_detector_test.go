package detection

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFlightPhasesCompleteFlight(t *testing.T) {
	path := writePhaseFixture(t, nil, completeFlightRows())

	result, err := DetectFlightPhases(path, PhaseOptions{AircraftProfile: "CARAVAN", SampleIntervalMs: 1000})
	if err != nil {
		t.Fatalf("DetectFlightPhases() error = %v", err)
	}
	if result.EngineVersion != PhaseEngineVersion {
		t.Fatalf("engine version = %q, want %q", result.EngineVersion, PhaseEngineVersion)
	}
	if result.Profile.Code != "CARAVAN" {
		t.Fatalf("profile = %q, want CARAVAN", result.Profile.Code)
	}
	if len(result.Flights) != 1 {
		t.Fatalf("flight count = %d, want 1", len(result.Flights))
	}
	flight := result.Flights[0]
	if flight.Status != "COMPLETE" || !flight.DepartureCaptured || !flight.ArrivalCaptured {
		t.Fatalf("flight = %+v, want complete departure and arrival", flight)
	}
	for _, wanted := range []string{"TAXI-OUT", "TAKEOFF", "CRUISE", "APPROACH", "LANDING", "TAXI-IN"} {
		if !hasPhase(result.Runs, wanted) {
			t.Errorf("phase %q missing from runs: %+v", wanted, result.Runs)
		}
	}
	for _, run := range result.Runs {
		if run.Phase == "" || run.EndRow < run.StartRow || run.DurationMs <= 0 {
			t.Errorf("invalid run: %+v", run)
		}
	}
}

func TestDetectFlightPhasesSeparatesSampleReset(t *testing.T) {
	rows := make([][]string, 0)
	for sample := 0; sample < 20; sample++ {
		rows = append(rows, phaseRow(sample, 100, 1200-float64(sample*50), -600, 0, 60))
	}
	for sample := 20; sample < 30; sample++ {
		rows = append(rows, phaseRow(sample, 0, 200, 0, 1, 20))
	}
	for sample := 0; sample < 10; sample++ {
		rows = append(rows, phaseRow(sample, 15, 300, 0, 1, 55))
	}
	for sample := 10; sample < 30; sample++ {
		rows = append(rows, phaseRow(sample, 100, 300+float64((sample-10)*50), 600, 0, 60))
	}

	result, err := DetectFlightPhases(writePhaseFixture(t, nil, rows), PhaseOptions{AircraftProfile: "CARAVAN", SampleIntervalMs: 1000})
	if err != nil {
		t.Fatalf("DetectFlightPhases() error = %v", err)
	}
	if len(result.Flights) != 2 {
		t.Fatalf("flight count = %d, want 2: %+v", len(result.Flights), result.Flights)
	}
	if result.Flights[0].Status != "ARRIVAL_ONLY" {
		t.Errorf("first status = %q, want ARRIVAL_ONLY", result.Flights[0].Status)
	}
	if result.Flights[1].Status != "DEPARTURE_ONLY" {
		t.Errorf("second status = %q, want DEPARTURE_ONLY", result.Flights[1].Status)
	}
}

func TestDetectFlightPhasesFindsGarminHeaderAfterMetadata(t *testing.T) {
	metadata := [][]string{{"Garmin Flight Data Log"}, {"Aircraft", "5Y-SLQ"}}
	result, err := DetectFlightPhases(writePhaseFixture(t, metadata, completeFlightRows()), PhaseOptions{AircraftMake: "Cessna", ModelNumber: "208B", SampleIntervalMs: 1000})
	if err != nil {
		t.Fatalf("DetectFlightPhases() error = %v", err)
	}
	if result.Profile.Code != "CARAVAN" {
		t.Fatalf("inferred profile = %q, want CARAVAN", result.Profile.Code)
	}
	if len(result.Flights) != 1 || result.Flights[0].Status != "COMPLETE" {
		t.Fatalf("flights = %+v, want one complete flight", result.Flights)
	}
}

func TestDetectFlightPhasesUsesPopulatedAltitudeAlias(t *testing.T) {
	rows := completeFlightRows()
	for index, row := range rows {
		rows[index] = []string{row[0], row[1], "", row[2], row[3], row[4], row[5]}
	}
	path := writePhaseFixtureWithHeader(t, nil, []string{"Sample", "IAS", "AltMSL", "AltInd", "VSpd", "WOW", "E1 Torq"}, rows)

	result, err := DetectFlightPhases(path, PhaseOptions{AircraftProfile: "CARAVAN", SampleIntervalMs: 1000})
	if err != nil {
		t.Fatalf("DetectFlightPhases() error = %v", err)
	}
	if len(result.Flights) != 1 || result.Flights[0].Status != "COMPLETE" {
		t.Fatalf("flights = %+v, want one complete flight", result.Flights)
	}
}

func TestDetectFlightPhasesRealCaravanFixture(t *testing.T) {
	directory := os.Getenv("PHASE_FIXTURE_DIR")
	if directory == "" {
		t.Skip("set PHASE_FIXTURE_DIR to run against real Garmin logs")
	}
	matches, err := filepath.Glob(filepath.Join(directory, "*.csv"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("find real fixture: matches=%d err=%v", len(matches), err)
	}

	files, rows, flights := 0, 0, 0
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil {
			t.Errorf("stat %q: %v", match, statErr)
			continue
		}
		if info.Size() == 0 {
			continue
		}
		result, detectErr := DetectFlightPhases(match, PhaseOptions{AircraftProfile: "CARAVAN"})
		if detectErr != nil {
			t.Errorf("DetectFlightPhases(%q) error = %v", match, detectErr)
			continue
		}
		if result.RowCount == 0 || len(result.Runs) == 0 {
			t.Errorf("%q produced no classified rows", match)
			continue
		}
		if result.Profile.Code != "CARAVAN" {
			t.Errorf("%q profile = %q, want CARAVAN", match, result.Profile.Code)
		}
		files++
		rows += result.RowCount
		flights += len(result.Flights)
	}
	if files == 0 {
		t.Fatal("no non-empty real Garmin fixtures were processed")
	}
	t.Logf("processed %d Garmin files: rows=%d flights=%d", files, rows, flights)
}

func completeFlightRows() [][]string {
	rows := make([][]string, 0, 240)
	for sample := 0; sample < 20; sample++ {
		rows = append(rows, phaseRow(sample, 0, 100, 0, 1, 10))
	}
	for sample := 20; sample < 40; sample++ {
		rows = append(rows, phaseRow(sample, 15, 100, 0, 1, 45))
	}
	for sample := 40; sample < 50; sample++ {
		rows = append(rows, phaseRow(sample, 65+float64(sample-40)*3, 100, 0, 1, 80))
	}
	for sample := 50; sample < 100; sample++ {
		rows = append(rows, phaseRow(sample, 105, 100+float64(sample-49)*50, 1000, 0, 80))
	}
	for sample := 100; sample < 140; sample++ {
		rows = append(rows, phaseRow(sample, 120, 2650, 0, 0, 65))
	}
	for sample := 140; sample < 190; sample++ {
		rows = append(rows, phaseRow(sample, 100, 2650-float64(sample-139)*50, -1000, 0, 55))
	}
	for sample := 190; sample < 200; sample++ {
		rows = append(rows, phaseRow(sample, 75, 150-float64(sample-190)*5, -300, 0, 50))
	}
	for sample := 200; sample < 210; sample++ {
		rows = append(rows, phaseRow(sample, 50-float64(sample-200)*3, 100, 0, 1, 40))
	}
	for sample := 210; sample < 230; sample++ {
		rows = append(rows, phaseRow(sample, 12, 100, 0, 1, 35))
	}
	for sample := 230; sample < 240; sample++ {
		rows = append(rows, phaseRow(sample, 0, 100, 0, 1, 10))
	}
	return rows
}

func phaseRow(sample int, speed, altitude, verticalSpeed float64, wow int, torque float64) []string {
	return []string{
		fmt.Sprintf("%d", sample), fmt.Sprintf("%.1f", speed), fmt.Sprintf("%.1f", altitude),
		fmt.Sprintf("%.1f", verticalSpeed), fmt.Sprintf("%d", wow), fmt.Sprintf("%.1f", torque),
	}
}

func writePhaseFixture(t *testing.T, metadata, rows [][]string) string {
	return writePhaseFixtureWithHeader(t, metadata, []string{"Sample", "IAS", "AltMSL", "VSpd", "WOW", "E1 Torq"}, rows)
}

func writePhaseFixtureWithHeader(t *testing.T, metadata [][]string, header []string, rows [][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flight.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	for _, row := range metadata {
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasPhase(runs []PhaseRun, phase string) bool {
	for _, run := range runs {
		if run.Phase == phase {
			return true
		}
	}
	return false
}
