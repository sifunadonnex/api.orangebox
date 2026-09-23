package detection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRecordingSegmentsUsesSampleResets(t *testing.T) {
	path := writeSegmentCSV(t, "Sample,Value\n1,10\n2,11\n3,12\n1,20\n2,21\n")
	segments, _, err := DetectRecordingSegments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("got %d segments, want 2: %#v", len(segments), segments)
	}
	if segments[0].StartRow != 2 || segments[0].EndRow != 4 || segments[1].StartRow != 5 || segments[1].BoundarySource != "sample_discontinuity" {
		t.Fatalf("unexpected segments: %#v", segments)
	}
}

func TestDetectRecordingSegmentsPrefersExplicitIdentifiers(t *testing.T) {
	path := writeSegmentCSV(t, "Sample,SessionID,Value\n1,A,10\n2,A,11\n3,B,20\n4,B,21\n")
	segments, _, err := DetectRecordingSegments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 || segments[1].BoundarySource != "explicit_identifier" || segments[1].StartRow != 4 {
		t.Fatalf("unexpected segments: %#v", segments)
	}
}

func TestAnalyzeFileScopesAndRebasesFlightRows(t *testing.T) {
	path := writeSegmentCSV(t, "Sample,PHASE,Speed\n1,CRUISE,10\n2,CRUISE,20\n1,CRUISE,30\n2,CRUISE,40\n")
	result, err := AnalyzeFile(path, nil, Options{SampleIntervalMs: 1000, StartRow: 4, EndRow: 5, RebaseTime: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 2 || result.SampleIntervalMs != 1000 {
		t.Fatalf("unexpected scoped result: %#v", result)
	}
}

func TestValidateRecordingRangesAllowsOnlyCompleteNonOverlappingCoverage(t *testing.T) {
	path := writeSegmentCSV(t, "Sample,Value\n1,10\n2,11\n[ ],[kt]\n1,20\n2,21\n")
	info, err := ValidateRecordingRanges(path, []RowRange{{StartRow: 2, EndRow: 3}, {StartRow: 5, EndRow: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if info.RowCount != 4 || info.FirstRow != 2 || info.LastRow != 6 {
		t.Fatalf("unexpected recording info: %#v", info)
	}
	if _, err = ValidateRecordingRanges(path, []RowRange{{StartRow: 2, EndRow: 4}, {StartRow: 3, EndRow: 6}}); err == nil {
		t.Fatal("expected overlapping ranges to be rejected")
	}
	if _, err = ValidateRecordingRanges(path, []RowRange{{StartRow: 2, EndRow: 3}}); err == nil {
		t.Fatal("expected incomplete coverage to be rejected")
	}
}

func writeSegmentCSV(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
