package handlers

import (
	"encoding/csv"
	"fdm-backend/detection"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamCSVRangeRetainsOptionalUnits(t *testing.T) {
	cases := []struct {
		name, source string
		start, end   int
		units        bool
		firstValue   string
	}{
		{"before header", "#airframe_info,Product=GIFD\n#yyy-mm-dd,hh:mm:ss,kt,ft msl\nLcl Date,Lcl Time,IAS,AltMSL\n2026-10-31,05:51:47,80,6000\n2026-10-31,05:51:48,90,6100\n", 5, 5, true, "90"},
		{"after header plain", "Time,IAS,AltMSL\ns,kt,ft\n0,80,6000\n1,90,6100\n", 4, 4, true, "90"},
		{"bracketed partial", "Time,IAS,AltMSL\n,[knots],\n0,80,6000\n", 3, 3, true, "80"},
		{"no units", "Time,IAS,AltMSL\n0,80,6000\n1,90,6100\n", 2, 3, false, "80"},
		{"categorical data", "Time,Mode,State\n0,CRUISE,NORMAL\n", 2, 2, false, "CRUISE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "source.csv")
			if err := os.WriteFile(file, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			runs := []detection.PhaseRun{{StartRow: tc.start, EndRow: tc.end, Phase: "CLIMB"}}
			if err := streamCSVRange(context, file, "Test", tc.start, tc.end, runs); err != nil {
				t.Fatal(err)
			}
			records, err := csv.NewReader(strings.NewReader(recorder.Body.String())).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			dataIndex := 1
			if tc.units {
				dataIndex = 2
				if !strings.Contains(strings.Join(records[1], ","), "[") {
					t.Fatalf("units lost: %#v", records)
				}
			}
			if tc.name == "before header" && (records[1][2] != "[kt]" || records[1][3] != "[ft msl]" || records[1][4] != "") {
				t.Fatalf("units/derived phase columns misaligned: %#v", records[1])
			}
			if tc.name == "bracketed partial" && (records[1][0] != "" || records[1][1] != "[knots]" || records[1][2] != "") {
				t.Fatalf("missing units must remain empty: %#v", records[1])
			}
			if records[dataIndex][len(records[0])-1] != "CLIMB" {
				t.Fatalf("source row/phase alignment changed: %#v", records)
			}
			if !strings.Contains(strings.Join(records[dataIndex], ","), tc.firstValue) {
				t.Fatalf("wrong source row: %#v", records)
			}
			source, _ := os.ReadFile(file)
			if string(source) != tc.source {
				t.Fatal("source recording was changed")
			}
		})
	}
}

func TestCSVUnitsRecognitionConservative(t *testing.T) {
	for _, row := range [][]string{{"0", "70", "6000"}, {"CRUISE", "NORMAL", "ON"}, {"#airframe_info", "Product=GIFD", "version=1"}} {
		if isCSVUnitRecord(row, 3) {
			t.Fatalf("data/metadata treated as units: %#v", row)
		}
	}
	if !isCSVUnitRecord([]string{"s", "ft lbs", "deg C"}, 3) {
		t.Fatal("plain units unrecognised")
	}
	if !isCSVUnitRecord([]string{"[s]", "[custom widgets]", ""}, 3) {
		t.Fatal("explicit custom units lost")
	}
}

func TestInlineCSVUnitHeaderRecognised(t *testing.T) {
	if !isFlightCSVHeader([]string{"Time (s)", "Engine Temp [deg C]"}) {
		t.Fatal("inline units hid telemetry header")
	}
	if isCSVUnitRecord([]string{"CRUISE", "F"}, 2) {
		t.Fatal("categorical data was mistaken for units")
	}
}

func TestOptionalCSVUnitsAndExplicitDimensionless(t *testing.T) {
	row := []string{"[s]", "[1]", "[N/A]", "-"}
	if !isCSVUnitRecord(row, 4) {
		t.Fatal("explicit dimensionless unit not recognised")
	}
	units := normalizedCSVUnitRecord(row, 4)
	if units[1] != "[1]" || units[2] != "" || units[3] != "" {
		t.Fatalf("placeholder units must stay absent: %#v", units)
	}
}
