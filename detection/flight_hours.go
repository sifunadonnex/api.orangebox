package detection

import "strconv"

// FlightHoursEstimate is a suggestion, never a replacement for reviewed operational time.
type FlightHoursEstimate struct {
	Hours        string `json:"hours"`
	Basis        string `json:"basis"`
	TimingSource string `json:"timingSource"`
}

// EstimateFlightHours uses recorded clocks independently per leg. It deliberately
// excludes sample-interval fallback, which is configured rather than recorded time.
func EstimateFlightHours(path string, ranges []RowRange, phases *PhaseDetectionResult) []*FlightHoursEstimate {
	estimates := make([]*FlightHoursEstimate, len(ranges))
	_, rows, _, err := readCSV(path)
	if err != nil {
		return estimates
	}
	for index, bounds := range ranges {
		selected, err := rowsInRange(rows, bounds.StartRow, bounds.EndRow)
		if err != nil || len(selected) < 2 {
			continue
		}
		frames, source, _, err := timestampRows(selected, 1000)
		if err != nil || source == "sample_interval" || len(frames) < 2 {
			continue
		}
		// Missing edge timestamps must not silently shorten the estimate.
		if frames[0].row != bounds.StartRow || frames[len(frames)-1].row != bounds.EndRow {
			continue
		}
		start, end := frames[0].timeMs, frames[len(frames)-1].timeMs
		basis := "recording_span"
		if phases != nil {
			for _, flight := range phases.Flights {
				if flight.StartRow != bounds.StartRow || flight.EndRow != bounds.EndRow || !flight.DepartureCaptured || !flight.ArrivalCaptured {
					continue
				}
				var airStart, airEnd int64
				startFound, endFound := false, false
				for _, frame := range frames {
					if frame.row == flight.AirborneStartRow {
						airStart, startFound = frame.timeMs, true
					}
					if frame.row == flight.AirborneEndRow {
						airEnd, endFound = frame.timeMs, true
					}
				}
				if startFound && endFound && airEnd > airStart {
					start, end, basis = airStart, airEnd, "airborne"
				}
			}
		}
		if end <= start {
			continue
		}
		hours := strconv.FormatFloat(float64(end-start)/3600000, 'f', 3, 64)
		if hours == "0.000" {
			hours = strconv.FormatFloat(float64(end-start)/3600000, 'f', 6, 64)
		}
		estimates[index] = &FlightHoursEstimate{Hours: hours, Basis: basis, TimingSource: source}
	}
	return estimates
}
