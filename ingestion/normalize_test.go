package ingestion

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleCSV = "Time,Altitude\n0,1000\n1,1010\n"

func TestNormalizeCSVDirect(t *testing.T) {
	assertNormalized(t, []byte(sampleCSV), "flight.csv", FormatCSV, "")
}

func TestNormalizeCSVGZIP(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(sampleCSV)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	assertNormalized(t, compressed.Bytes(), "flight.csv.gz", FormatGZIP, "flight.csv")
}

func TestNormalizeCSVZIP(t *testing.T) {
	archive := makeZIP(t, map[string]string{"recordings/flight.csv": sampleCSV, "readme.txt": "notes"})
	assertNormalized(t, archive, "download.zip", FormatZIP, "recordings/flight.csv")
}

func TestNormalizeCSVZIPRejectsMultipleRecordings(t *testing.T) {
	archive := makeZIP(t, map[string]string{"one.csv": sampleCSV, "two.csv": sampleCSV})
	destination := filepath.Join(t.TempDir(), "normalized.csv")
	_, err := NormalizeCSV(bytes.NewReader(archive), int64(len(archive)), "download.zip", destination)
	if err == nil || !strings.Contains(err.Error(), "multiple CSV") {
		t.Fatalf("expected multiple CSV error, got %v", err)
	}
}

func TestNormalizeCSVRejectsNonCSV(t *testing.T) {
	data := []byte("not a recording")
	destination := filepath.Join(t.TempDir(), "normalized.csv")
	_, err := NormalizeCSV(bytes.NewReader(data), int64(len(data)), "flight.dat", destination)
	if err == nil || !strings.Contains(err.Error(), ErrUnsupportedFormat.Error()) {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func assertNormalized(t *testing.T, source []byte, name string, format SourceFormat, entry string) {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "normalized.csv")
	result, err := NormalizeCSV(bytes.NewReader(source), int64(len(source)), name, destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceFormat != format || result.SourceEntry != entry {
		t.Fatalf("unexpected result: %+v", result)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != sampleCSV {
		t.Fatalf("unexpected normalized content: %q", content)
	}
}

func makeZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
