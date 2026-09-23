package detection

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
)

// RecordingSegment is an inclusive source-record range that can be analysed
// as an independent flight. Rows use the same numbering exposed in evidence.
type RecordingSegment struct {
	Index          int    `json:"index"`
	StartRow       int    `json:"startRow"`
	EndRow         int    `json:"endRow"`
	StartSample    string `json:"startSample,omitempty"`
	EndSample      string `json:"endSample,omitempty"`
	BoundarySource string `json:"boundarySource"`
}

type RowRange struct {
	StartRow int `json:"startRow"`
	EndRow   int `json:"endRow"`
}

type RecordingRows struct {
	FirstRow int `json:"firstRow"`
	LastRow  int `json:"lastRow"`
	RowCount int `json:"rowCount"`
}

func InspectRecordingRows(path string) (RecordingRows, error) {
	_, rows, _, err := readCSV(path)
	if err != nil {
		return RecordingRows{}, err
	}
	return RecordingRows{FirstRow: rows[0].row, LastRow: rows[len(rows)-1].row, RowCount: len(rows)}, nil
}

// DetectRecordingSegments finds only high-confidence recording boundaries.
// Phase/air-ground based flight splitting is intentionally deferred to the
// dedicated flight-phase detector.
func DetectRecordingSegments(path string) ([]RecordingSegment, []Diagnostic, error) {
	headers, rows, parseDiagnostics, err := readCSV(path)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, parseDiagnostics, errors.New("CSV contains no data rows")
	}

	identifierKey := firstHeaderKey(headers, "FLIGHTID", "SESSIONID", "RECORDINGID", "SESSIONNUM")
	typicalStep, hasNumericSamples := typicalSampleStep(rows)
	maximumContinuousStep := math.Max(typicalStep*10, typicalStep+1)

	starts := []int{0}
	sources := []string{"whole_recording"}
	for index := 1; index < len(rows); index++ {
		boundarySource := ""
		if identifierKey != "" {
			previous := strings.TrimSpace(rows[index-1].values[identifierKey])
			current := strings.TrimSpace(rows[index].values[identifierKey])
			if previous != "" && current != "" && !strings.EqualFold(previous, current) {
				boundarySource = "explicit_identifier"
			}
		}
		if boundarySource == "" && hasNumericSamples {
			previous, previousOK := numericSample(rows[index-1].sample)
			current, currentOK := numericSample(rows[index].sample)
			if previousOK && currentOK {
				difference := current - previous
				if difference <= 0 || difference > maximumContinuousStep {
					boundarySource = "sample_discontinuity"
				}
			}
		}
		if boundarySource != "" {
			starts = append(starts, index)
			sources = append(sources, boundarySource)
		}
	}

	segments := make([]RecordingSegment, 0, len(starts))
	for index, start := range starts {
		end := len(rows) - 1
		if index+1 < len(starts) {
			end = starts[index+1] - 1
		}
		source := sources[index]
		if len(starts) > 1 && index == 0 {
			source = "file_start"
		}
		segments = append(segments, RecordingSegment{
			Index: index + 1, StartRow: rows[start].row, EndRow: rows[end].row,
			StartSample: rows[start].sample, EndSample: rows[end].sample,
			BoundarySource: source,
		})
	}
	return segments, parseDiagnostics, nil
}

// ValidateRecordingRanges verifies that reviewed flight ranges are ordered,
// non-overlapping, and cover every usable data record exactly once. Gaps that
// contain only blank/unit records are permitted.
func ValidateRecordingRanges(path string, ranges []RowRange) (RecordingRows, error) {
	_, rows, _, err := readCSV(path)
	if err != nil {
		return RecordingRows{}, err
	}
	info := RecordingRows{FirstRow: rows[0].row, LastRow: rows[len(rows)-1].row, RowCount: len(rows)}
	if len(ranges) == 0 {
		return info, errors.New("at least one flight range is required")
	}

	ordered := append([]RowRange(nil), ranges...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartRow < ordered[j].StartRow })
	usableRows := make(map[int]bool, len(rows))
	for _, row := range rows {
		usableRows[row.row] = true
	}
	for index, item := range ordered {
		if item.StartRow <= 0 || item.EndRow < item.StartRow {
			return info, errors.New("every flight must have a valid inclusive start and end row")
		}
		if !usableRows[item.StartRow] || !usableRows[item.EndRow] {
			return info, errors.New("flight boundaries must point to usable CSV data rows")
		}
		if index > 0 && item.StartRow <= ordered[index-1].EndRow {
			return info, errors.New("flight row ranges must not overlap")
		}
	}

	for _, row := range rows {
		coverage := 0
		for _, item := range ordered {
			if row.row >= item.StartRow && row.row <= item.EndRow {
				coverage++
			}
		}
		if coverage != 1 {
			return info, errors.New("flight row ranges must cover every usable CSV data row exactly once")
		}
	}
	return info, nil
}

func firstHeaderKey(headers map[string]string, aliases ...string) string {
	for _, alias := range aliases {
		if _, ok := headers[alias]; ok {
			return alias
		}
	}
	return ""
}

func typicalSampleStep(rows []rawRow) (float64, bool) {
	differences := make([]float64, 0, len(rows)-1)
	for index := 1; index < len(rows); index++ {
		previous, previousOK := numericSample(rows[index-1].sample)
		current, currentOK := numericSample(rows[index].sample)
		if previousOK && currentOK && current > previous {
			differences = append(differences, current-previous)
		}
	}
	if len(differences) == 0 {
		return 1, false
	}
	sort.Float64s(differences)
	middle := len(differences) / 2
	if len(differences)%2 == 0 {
		return (differences[middle-1] + differences[middle]) / 2, true
	}
	return differences[middle], true
}

func numericSample(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func rowsInRange(rows []rawRow, startRow, endRow int) ([]rawRow, error) {
	if startRow <= 0 && endRow <= 0 {
		return rows, nil
	}
	filtered := make([]rawRow, 0, len(rows))
	for _, row := range rows {
		if startRow > 0 && row.row < startRow {
			continue
		}
		if endRow > 0 && row.row > endRow {
			continue
		}
		filtered = append(filtered, row)
	}
	if len(filtered) == 0 {
		return nil, errors.New("flight row range contains no data rows")
	}
	return filtered, nil
}

func rebaseFrameTimes(frames []frame) {
	if len(frames) == 0 {
		return
	}
	base := frames[0].timeMs
	for index := range frames {
		frames[index].timeMs -= base
	}
}
