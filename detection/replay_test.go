package detection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildReplayResolvesAliasesAndOptionalCapabilities(t *testing.T) {
	path := writeReplayCSV(t, "Lcl Date,Lcl Time,GPS Lat,GPS Lon,AltMSL,GndSpd,TRK,PHASE\n"+
		"6/4/2025,06:38:16,-1.3,36.8,5500,0,90,GROUND\n"+
		"6/4/2025,06:38:17,-1.2,36.7,5600,120,91,CLIMB\n"+
		"6/4/2025,06:38:18,-1.1,36.6,5700,125,92,CLIMB\n")
	result, err := BuildReplay(path, ReplayOptions{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.TimingSource != "date_time" || result.DurationMs != 2000 {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if result.Mappings["latitude"] != "GPS Lat" || result.Mappings["longitude"] != "GPS Lon" {
		t.Fatalf("aliases were not preserved: %#v", result.Mappings)
	}
	if !result.Capabilities.Altitude || !result.Capabilities.GroundSpeed || !result.Capabilities.Heading || !result.Capabilities.Phase {
		t.Fatalf("expected optional capabilities: %#v", result.Capabilities)
	}
	if result.Capabilities.Airspeed || len(result.Points) != 3 {
		t.Fatalf("unexpected optional data: %#v", result)
	}
}

func TestBuildReplayGracefullyRejectsMissingPosition(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),Altitude\n0,100\n1,200\n")
	result, err := BuildReplay(path, ReplayOptions{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported || result.UnsupportedReason == "" || len(result.Diagnostics) == 0 {
		t.Fatalf("expected an explicit unsupported result: %#v", result)
	}
}

func TestBuildReplaySupportsElapsedTimeAndCanonicalCoordinates(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),Latitude,Longitude,Airspeed\n10,-1.3,36.8,100\n11,-1.2,36.7,110\n")
	result, err := BuildReplay(path, ReplayOptions{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.StartTimeMs != 10000 || result.EndTimeMs != 11000 || !result.Capabilities.Airspeed {
		t.Fatalf("unexpected elapsed replay result: %#v", result)
	}
}

func TestBuildReplaySupportsExpandedAircraftHeaderAliases(t *testing.T) {
	path := writeReplayCSV(t, "Elapsed Seconds,Latitude (deg),Longitude (deg),Baro Altitude,GS Kts,Flight Stage\n"+
		"0,-1.3,36.8,5000,120,CLIMB\n"+
		"1,-1.2,36.7,5100,125,CRUISE\n")
	result, err := BuildReplay(path, ReplayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.TimingSource != "elapsedseconds" || !result.Capabilities.Altitude || !result.Capabilities.GroundSpeed || !result.Capabilities.Phase {
		t.Fatalf("expanded aliases were not resolved: %#v", result)
	}
	if result.Points[1].Phase != "CRUISE" {
		t.Fatalf("expected mapped phase value, got %#v", result.Points[1])
	}
}

func writeReplayCSV(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flight.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
