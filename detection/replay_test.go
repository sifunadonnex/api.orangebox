package detection

import (
	"math"
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
	if result.Points[0].AltitudeMeters == nil || math.Abs(*result.Points[0].AltitudeMeters-1676.4) > 0.01 {
		t.Fatalf("expected altitude to be normalized for 3D rendering: %#v", result.Points[0])
	}
	if result.Measurements["altitude"].Unit != "ft" || result.Measurements["altitude"].Reference != "MSL" {
		t.Fatalf("unexpected altitude metadata: %#v", result.Measurements["altitude"])
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

func TestBuildReplayPreservesRecordedGMTAlongsideElapsedClock(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),GMT Hours,GMT Minutes,GMT Seconds,Latitude,Longitude\n"+
		"10,6,38,16.25,-1.3,36.8\n"+
		"11,6,38,17.25,-1.2,36.7\n")
	result, err := BuildReplay(path, ReplayOptions{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.TimingSource != "timesec" || len(result.Points) != 2 {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if result.Points[0].GMTTimeMs == nil || *result.Points[0].GMTTimeMs != 23896250 {
		t.Fatalf("expected recorded GMT time to be preserved: %#v", result.Points[0])
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

func TestBuildReplayConvertsExplicitMetricMeasurements(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),Latitude,Longitude,Altitude (m),Ground Speed (kmh),Vertical Speed (mps)\n"+
		"0,-1.3,36.8,1000,185.2,5\n"+
		"1,-1.29,36.81,1010,203.72,4\n")
	result, err := BuildReplay(path, ReplayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	point := result.Points[0]
	if point.Altitude == nil || math.Abs(*point.Altitude-3280.839895) > 0.01 || point.AltitudeMeters == nil || *point.AltitudeMeters != 1000 {
		t.Fatalf("metric altitude was not normalized: %#v", point)
	}
	if point.GroundSpeed == nil || math.Abs(*point.GroundSpeed-100) > 0.05 {
		t.Fatalf("metric speed was not normalized: %#v", point)
	}
	if point.VerticalSpeed == nil || math.Abs(*point.VerticalSpeed-984.2519685) > 0.05 {
		t.Fatalf("metric vertical speed was not normalized: %#v", point)
	}
}

func TestBuildReplayPreservesUnknownAltitudeInRecordedUnits(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),Latitude,Longitude,Altitude\n0,-1.3,36.8,1000\n1,-1.29,36.81,1100\n")
	result, err := BuildReplay(path, ReplayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Capabilities.Altitude || result.Points[0].Altitude == nil || result.Points[0].AltitudeMeters != nil {
		t.Fatalf("unknown altitude should remain visible but not be used as metres: %#v", result)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "REPLAY_ALTITUDE_UNIT_UNKNOWN"
	}
	if !found {
		t.Fatalf("expected an unknown-unit diagnostic: %#v", result.Diagnostics)
	}
}

func TestBuildReplayDerivesTrackWhenHeadingIsMissing(t *testing.T) {
	path := writeReplayCSV(t, "Time(sec),Latitude,Longitude\n0,-1.3,36.800\n1,-1.3,36.801\n2,-1.3,36.802\n")
	result, err := BuildReplay(path, ReplayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.HeadingSource != "derived" || !result.Capabilities.Heading {
		t.Fatalf("expected a derived heading capability: %#v", result)
	}
	for _, point := range result.Points {
		if point.Heading == nil || math.Abs(*point.Heading-90) > 0.1 {
			t.Fatalf("unexpected derived heading: %#v", point)
		}
	}
}

func TestNearestReplayPointUsesClosestSampleWithinTolerance(t *testing.T) {
	points := []ReplayPoint{
		{TimeMs: 1000, Latitude: -1, Longitude: 36},
		{TimeMs: 3000, Latitude: -2, Longitude: 37},
		{TimeMs: 5000, Latitude: -3, Longitude: 38},
	}

	point, delta, ok := NearestReplayPoint(points, 3800, 1000)
	if !ok || point.TimeMs != 3000 || delta != 800 {
		t.Fatalf("expected the 3000ms sample at delta 800ms, got point=%+v delta=%d ok=%v", point, delta, ok)
	}
	if _, delta, ok = NearestReplayPoint(points, 7000, 1000); ok || delta != 2000 {
		t.Fatalf("expected a 2000ms sample gap to be rejected, got delta=%d ok=%v", delta, ok)
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
