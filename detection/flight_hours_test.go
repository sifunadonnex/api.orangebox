package detection

import "testing"

func TestEstimateFlightHoursRecordedClocks(t *testing.T) {
	tests := []struct {
		name, csv, want string
		bounds          RowRange
	}{
		{"short recording", "TimeSec,IAS\n0,10\n1,20\n", "0.000278", RowRange{2, 3}},
		{"elapsed", "TimeSec,IAS\n0,20\n1800,100\n3600,20\n", "1.000", RowRange{2, 4}},
		{"midnight dates", "Lcl Date,Lcl Time,IAS\n2026-11-06,23:45:00,30\n2026-11-07,00:15:00,30\n", "0.500", RowRange{2, 3}},
		{"gmt rollover", "GMT Hours,GMT Minutes,GMT Seconds,IAS\n23,45,0,30\n0,15,0,30\n", "0.500", RowRange{2, 3}},
		{"minutes", "Time Min,IAS\n2,10\n32,20\n", "0.500", RowRange{2, 3}},
		{"sample only", "Sample,IAS\n0,10\n3600,20\n", "", RowRange{2, 3}},
		{"frozen", "TimeSec,IAS\n0,10\n0,20\n", "", RowRange{2, 3}},
		{"backward", "TimeSec,IAS\n100,10\n90,20\n", "", RowRange{2, 3}},
		{"single frame", "TimeSec,IAS\n100,10\n", "", RowRange{2, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimates := EstimateFlightHours(writeSegmentCSV(t, tt.csv), []RowRange{tt.bounds}, nil)
			if tt.want == "" {
				if estimates[0] != nil {
					t.Fatalf("unexpected estimate: %#v", estimates[0])
				}
				return
			}
			if estimates[0] == nil || estimates[0].Hours != tt.want || estimates[0].Basis != "recording_span" {
				t.Fatalf("unexpected estimate: %#v", estimates[0])
			}
		})
	}
}

func TestEstimateFlightHoursPerLegAndAirborne(t *testing.T) {
	path := writeSegmentCSV(t, "TimeSec,IAS\n0,0\n600,80\n1800,80\n3600,0\n0,0\n1800,0\n")
	ranges := []RowRange{{2, 5}, {6, 7}}
	phases := &PhaseDetectionResult{Flights: []PhaseFlight{{StartRow: 2, EndRow: 5, AirborneStartRow: 3, AirborneEndRow: 4, DepartureCaptured: true, ArrivalCaptured: true}}}
	estimates := EstimateFlightHours(path, ranges, phases)
	if estimates[0] == nil || estimates[0].Hours != "0.333" || estimates[0].Basis != "airborne" || estimates[1] == nil || estimates[1].Hours != "0.500" {
		t.Fatalf("wrong estimates: %#v", estimates)
	}
	phases.Flights[0].ArrivalCaptured = false
	estimates = EstimateFlightHours(path, ranges, phases)
	if estimates[0].Basis != "recording_span" || estimates[0].Hours != "1.000" {
		t.Fatalf("incomplete flight must use clearly labelled span: %#v", estimates[0])
	}
	// Reviewed boundaries must never reuse an old airborne estimate.
	estimates = EstimateFlightHours(path, []RowRange{{3, 5}}, phases)
	if estimates[0].Basis != "recording_span" || estimates[0].Hours != "0.833" {
		t.Fatalf("stale phase boundaries reused: %#v", estimates[0])
	}
}

func TestEstimateFlightHoursMissingEdgeDoesNotShortenDuration(t *testing.T) {
	csv := "TimeSec,IAS\n,0\n"
	for i := 0; i < 20; i++ {
		csv += "1,20\n"
	}
	csv += "3600,0\n"
	estimates := EstimateFlightHours(writeSegmentCSV(t, csv), []RowRange{{2, 23}}, nil)
	if estimates[0] != nil {
		t.Fatalf("missing first timestamp must not shorten time: %#v", estimates[0])
	}
}
