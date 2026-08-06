package handlers

import (
	"testing"
	"time"
)

func TestParseTimestampSupportsDatabaseRepresentations(t *testing.T) {
	tests := []struct {
		name  string
		value string
		year  int
	}{
		{name: "unix milliseconds", value: "1785931854580", year: 2026},
		{name: "RFC3339", value: "2026-08-05T14:10:54.58Z", year: 2026},
		{name: "SQLite datetime", value: "2026-08-05 14:10:54", year: 2026},
		{name: "legacy Go value", value: "2025-11-12 10:15:50.3591628 +0300 EAT m=+0.116219601", year: 2025},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseTimestamp(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.IsZero() || parsed.Year() != test.year {
				t.Fatalf("unexpected timestamp: %s", parsed)
			}
		})
	}
}

func TestParseTimestampRejectsInvalidValue(t *testing.T) {
	if parsed, err := parseTimestamp("not-a-date"); err == nil || !parsed.Equal(time.Time{}) {
		t.Fatalf("expected invalid timestamp to return an error, got %s, %v", parsed, err)
	}
}

func TestNullableTimestampScansIntegerMilliseconds(t *testing.T) {
	var timestamp nullableTimestamp
	if err := timestamp.Scan(int64(1785931854580)); err != nil {
		t.Fatal(err)
	}
	if !timestamp.Valid || timestamp.Time.Year() != 2026 {
		t.Fatalf("unexpected scanned timestamp: %#v", timestamp)
	}
}
