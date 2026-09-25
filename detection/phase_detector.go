package detection

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const PhaseEngineVersion = "go-phase-1.0.0"

type PhaseOptions struct {
	AircraftProfile  string
	AircraftMake     string
	ModelNumber      string
	SampleIntervalMs int64
}

type PhaseFlight struct {
	Index               int      `json:"index"`
	Status              string   `json:"status"`
	StartRow            int      `json:"startRow"`
	EndRow              int      `json:"endRow"`
	AirborneStartRow    int      `json:"airborneStartRow"`
	AirborneEndRow      int      `json:"airborneEndRow"`
	StartSample         string   `json:"startSample,omitempty"`
	EndSample           string   `json:"endSample,omitempty"`
	AirborneStartSample string   `json:"airborneStartSample,omitempty"`
	AirborneEndSample   string   `json:"airborneEndSample,omitempty"`
	DepartureCaptured   bool     `json:"departureCaptured"`
	ArrivalCaptured     bool     `json:"arrivalCaptured"`
	DepartureAltitude   *float64 `json:"departureAltitude,omitempty"`
	ArrivalAltitude     *float64 `json:"arrivalAltitude,omitempty"`
	DurationMs          int64    `json:"durationMs"`
	AirborneDurationMs  int64    `json:"airborneDurationMs"`
}

type PhaseRun struct {
	FlightIndex  int    `json:"flightIndex,omitempty"`
	FlightStatus string `json:"flightStatus,omitempty"`
	Phase        string `json:"phase"`
	StartRow     int    `json:"startRow"`
	EndRow       int    `json:"endRow"`
	StartSample  string `json:"startSample,omitempty"`
	EndSample    string `json:"endSample,omitempty"`
	DurationMs   int64  `json:"durationMs"`
}

type PhaseDetectionResult struct {
	EngineVersion    string        `json:"engineVersion"`
	Profile          PhaseProfile  `json:"profile"`
	TimingSource     string        `json:"timingSource"`
	SampleIntervalMs int64         `json:"sampleIntervalMs"`
	RowCount         int           `json:"rowCount"`
	Flights          []PhaseFlight `json:"flights"`
	Runs             []PhaseRun    `json:"runs"`
	Diagnostics      []Diagnostic  `json:"diagnostics"`
}

type phaseSample struct {
	row           rawRow
	recording     int
	timeMs        int64
	airspeed      float64
	groundSpeed   float64
	movementSpeed float64
	altitude      float64
	verticalSpeed float64
	torque        float64
	flaps         float64
	speedbrake    bool
	airborne      bool
	groundRef     float64
	agl           float64
	flightIndex   int
	phase         string
}

type phaseFlightState struct {
	PhaseFlight
	startIndex    int
	endIndex      int
	airStartIndex int
	airEndIndex   int
	topIndex      int
	taxiOutStart  int
	takeoffStart  int
	landingEnd    int
	taxiInEnd     int
}

type boolRun struct {
	start int
	end   int
	value bool
}

var phaseAliases = map[string][]string{
	"airspeed":      {"AIRSPEEDL", "AIRSPEEDR", "AIRSPEED", "COMPUTEDAIRSPEED", "IAS", "GNDSPD", "GROUNDSPEED"},
	"altitude":      {"ALTITUDEL", "ALTITUDER", "ALTITUDE", "ELEVATION", "ALTMSL", "ALTIND", "ALTGPS", "ALTB", "ALTITUDE2992"},
	"airGround":     {"AIRGROUND", "WOW", "WOWMLG", "WOWNLG", "WEIGHTONWHEELS"},
	"groundSpeed":   {"GNDSPD", "GROUNDSPEED"},
	"flaps":         {"TEFLAPPOSNRIGHT", "TEFLAPPOSNLEFT", "FLAPS", "ALTFLAPS", "FLAPPOS"},
	"speedbrake":    {"SPEEDBRKHDLPOSN", "SPOILERPOSNNO7", "SPOILERPOSNNO2"},
	"verticalSpeed": {"VERTICALSPEED", "VSPD", "VS"},
}

var phaseTorqueAliases = []string{"TQ1", "TQ2", "ENG1", "ENG2", "E1TORQ", "E2TORQ"}

// DetectFlightPhases performs native, deterministic phase classification. It
// returns compact source-row runs so callers can persist the result without
// rewriting the uploaded recording.
func DetectFlightPhases(path string, options PhaseOptions) (PhaseDetectionResult, error) {
	profileCode := strings.ToUpper(strings.TrimSpace(options.AircraftProfile))
	if profileCode == "" {
		profileCode = InferPhaseProfile(options.AircraftMake, options.ModelNumber)
	}
	profile, knownProfile := PhaseProfileByCode(profileCode)

	headers, rows, diagnostics, err := readPhaseCSV(path)
	if err != nil {
		return PhaseDetectionResult{}, err
	}
	collector := diagnosticCollector{}
	for _, diagnostic := range diagnostics {
		collector.add(diagnostic.Code, diagnostic.Severity, diagnostic.Message, "")
	}
	if !knownProfile {
		collector.add("PHASE_PROFILE_UNKNOWN", "warning", "Unknown aircraft phase profile; generic turboprop thresholds were used", "")
	}

	keys := make(map[string]string, len(phaseAliases))
	for name, aliases := range phaseAliases {
		if name == "airGround" {
			keys[name] = firstPhaseHeader(headers, aliases...)
			continue
		}
		keys[name] = bestPhaseNumericHeader(headers, rows, aliases...)
	}
	if keys["airspeed"] == "" || keys["altitude"] == "" {
		return PhaseDetectionResult{}, errors.New("phase detection requires a usable airspeed or groundspeed column and an altitude column")
	}

	rows = usablePhaseRows(rows, keys["airspeed"], keys["altitude"])
	if len(rows) == 0 {
		return PhaseDetectionResult{}, errors.New("phase detection found no rows containing both speed and altitude")
	}

	intervalMs, timingSource := phaseSampleInterval(rows, options.SampleIntervalMs)
	samples, err := buildPhaseSamples(rows, keys, profile, intervalMs, collector)
	if err != nil {
		return PhaseDetectionResult{}, err
	}
	recordingRanges := assignPhaseRecordings(samples, intervalMs)
	flights := detectPhaseFlights(samples, recordingRanges, profile, intervalMs)
	classifyPhaseSamples(samples, flights, recordingRanges, profile, intervalMs)
	runs := buildPhaseRuns(samples, flights, intervalMs)

	publicFlights := make([]PhaseFlight, len(flights))
	for index := range flights {
		publicFlights[index] = flights[index].PhaseFlight
	}
	if len(publicFlights) == 0 {
		collector.add("PHASE_NO_AIRBORNE_FLIGHT", "warning", "No sustained airborne segment was detected; ground and taxi phases remain available", "")
	}

	return PhaseDetectionResult{
		EngineVersion: PhaseEngineVersion, Profile: profile, TimingSource: timingSource,
		SampleIntervalMs: intervalMs, RowCount: len(samples), Flights: publicFlights,
		Runs: runs, Diagnostics: collector.list(),
	}, nil
}

func readPhaseCSV(path string) (map[string]string, []rawRow, []Diagnostic, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open phase CSV: %w", err)
	}
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	headerLine := 0
	var headerRecord []string
	for line := 1; line <= 50; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return nil, nil, nil, fmt.Errorf("inspect phase CSV row %d: %w", line, readErr)
		}
		keys := make(map[string]bool, len(record))
		for _, value := range record {
			keys[canonical(value)] = true
		}
		if phaseHeaderContains(keys, phaseAliases["airspeed"]) && phaseHeaderContains(keys, phaseAliases["altitude"]) {
			headerLine, headerRecord = line, record
			break
		}
	}
	_ = file.Close()
	if headerLine == 0 {
		return nil, nil, nil, errors.New("could not find a telemetry header containing speed and altitude within the first 50 CSV rows")
	}

	headers := make(map[string]string, len(headerRecord))
	indices := make(map[string]int, len(headerRecord))
	diagnostics := make([]Diagnostic, 0)
	for index, raw := range headerRecord {
		if index == 0 {
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		key := canonical(raw)
		if key == "" {
			continue
		}
		if prior, exists := headers[key]; exists {
			diagnostics = append(diagnostics, Diagnostic{Code: "DUPLICATE_HEADER", Severity: "warning", Message: fmt.Sprintf("CSV columns %q and %q normalize to the same name; the first is used", prior, raw), Count: 1})
			continue
		}
		headers[key] = strings.TrimSpace(raw)
		indices[key] = index
	}

	file, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reopen phase CSV: %w", err)
	}
	defer file.Close()
	reader = csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows := make([]rawRow, 0, 4096)
	for line := 1; ; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, nil, fmt.Errorf("read phase CSV row %d: %w", line, readErr)
		}
		if line <= headerLine {
			continue
		}
		values := make(map[string]string, len(indices))
		for key, index := range indices {
			if index >= len(record) {
				continue
			}
			value := strings.TrimSpace(record[index])
			if value != "" {
				values[key] = value
			}
		}
		if len(values) == 0 || looksLikeUnitsRow(values) {
			continue
		}
		rows = append(rows, rawRow{row: line, values: values, sample: firstValue(values, "SAMPLE", "FRAME")})
	}
	if len(rows) == 0 {
		return nil, nil, nil, errors.New("CSV contains no telemetry rows after its detected header")
	}
	return headers, rows, diagnostics, nil
}

func phaseHeaderContains(headers map[string]bool, aliases []string) bool {
	for _, alias := range aliases {
		if headers[alias] {
			return true
		}
	}
	return false
}

func firstPhaseHeader(headers map[string]string, aliases ...string) string {
	for _, alias := range aliases {
		if _, ok := headers[alias]; ok {
			return alias
		}
	}
	return ""
}

func bestPhaseNumericHeader(headers map[string]string, rows []rawRow, aliases ...string) string {
	bestKey, bestCount := "", -1
	for _, alias := range aliases {
		if _, ok := headers[alias]; !ok {
			continue
		}
		count := 0
		for _, row := range rows {
			if _, ok := numericValue(row.values, alias); ok {
				count++
			}
		}
		if count > bestCount {
			bestKey, bestCount = alias, count
		}
	}
	return bestKey
}

func usablePhaseRows(rows []rawRow, speedKey, altitudeKey string) []rawRow {
	result := make([]rawRow, 0, len(rows))
	for _, row := range rows {
		_, speedOK := numericValue(row.values, speedKey)
		_, altitudeOK := numericValue(row.values, altitudeKey)
		if speedOK && altitudeOK {
			result = append(result, row)
		}
	}
	return result
}

func phaseSampleInterval(rows []rawRow, fallback int64) (int64, string) {
	differences := make([]int64, 0, len(rows)-1)
	var prior time.Time
	for _, row := range rows {
		dateValue := firstValue(row.values, "LCLDATE", "LOCALDATE", "UTCDATE", "DATE")
		timeValue := firstValue(row.values, "LCLTIME", "LOCALTIME", "UTCTIME", "TIME")
		parsed, ok := parseDateTime(dateValue, timeValue)
		if !ok {
			continue
		}
		if !prior.IsZero() {
			difference := parsed.Sub(prior).Milliseconds()
			if difference > 0 && difference <= 60000 {
				differences = append(differences, difference)
			}
		}
		prior = parsed
	}
	if len(differences) > 0 {
		sort.Slice(differences, func(i, j int) bool { return differences[i] < differences[j] })
		return differences[len(differences)/2], "date_time"
	}
	if fallback > 0 {
		return fallback, "configured_interval"
	}
	return 500, "default_500ms"
}

func buildPhaseSamples(rows []rawRow, keys map[string]string, profile PhaseProfile, intervalMs int64, diagnostics diagnosticCollector) ([]phaseSample, error) {
	airspeed, airspeedOK := phaseNumericSeries(rows, keys["airspeed"], 0, 500)
	altitude, altitudeOK := phaseNumericSeries(rows, keys["altitude"], -1000, 60000)
	if !airspeedOK || !altitudeOK {
		return nil, errors.New("speed or altitude data is not numeric enough for phase detection")
	}
	airspeed = cleanPhaseSpikes(airspeed, 75)
	altitude = cleanPhaseSpikes(altitude, 500)

	groundSpeed := append([]float64(nil), airspeed...)
	if key := keys["groundSpeed"]; key != "" {
		if values, ok := phaseNumericSeries(rows, key, 0, 600); ok {
			groundSpeed = values
		}
	}

	verticalSpeed := make([]float64, len(rows))
	if key := keys["verticalSpeed"]; key != "" {
		for index, row := range rows {
			if value, ok := numericValue(row.values, key); ok && isFinite(value) {
				verticalSpeed[index] = value
			}
		}
	} else {
		minutes := float64(intervalMs) / 60000
		for index := 1; index < len(rows); index++ {
			verticalSpeed[index] = (altitude[index] - altitude[index-1]) / minutes
		}
		diagnostics.add("PHASE_VERTICAL_SPEED_DERIVED", "warning", "Vertical speed was derived from altitude because no recorded vertical-speed column was found", "")
	}
	verticalSpeed = smoothPhaseSeries(verticalSpeed, 10)

	flaps := make([]float64, len(rows))
	if key := keys["flaps"]; key != "" {
		last := 0.0
		for index, row := range rows {
			if value, ok := numericValue(row.values, key); ok {
				last = value
			}
			flaps[index] = last
		}
	}
	torque := phaseTorqueSeries(rows)
	samples := make([]phaseSample, len(rows))
	for index, row := range rows {
		speedbrake := false
		if key := keys["speedbrake"]; key != "" {
			value, ok := numericValue(row.values, key)
			speedbrake = ok && value > 0
		}
		samples[index] = phaseSample{
			row: row, timeMs: int64(index) * intervalMs, airspeed: airspeed[index],
			groundSpeed: groundSpeed[index], movementSpeed: math.Max(airspeed[index], groundSpeed[index]),
			altitude: altitude[index], verticalSpeed: verticalSpeed[index], torque: torque[index],
			flaps: flaps[index], speedbrake: speedbrake,
		}
	}

	provisional := phasePercentile(altitude, 0.01)
	lowSpeedAltitudes := make([]float64, 0)
	for index := range samples {
		if samples[index].movementSpeed < profile.TaxiSpeed {
			lowSpeedAltitudes = append(lowSpeedAltitudes, samples[index].altitude)
		}
	}
	if len(lowSpeedAltitudes) > 0 {
		provisional = phaseMedian(lowSpeedAltitudes)
	}
	airGroundKey := keys["airGround"]
	if airGroundKey == "" {
		diagnostics.add("PHASE_AIR_GROUND_INFERRED", "warning", "AIR/GROUND or weight-on-wheels data is unavailable; airborne state was inferred", "")
	}
	for index := range samples {
		samples[index].groundRef = provisional
		samples[index].agl = math.Max(0, samples[index].altitude-provisional)
		if airGroundKey != "" {
			value := strings.ToUpper(strings.TrimSpace(samples[index].row.values[airGroundKey]))
			if strings.Contains(airGroundKey, "WOW") || airGroundKey == "WEIGHTONWHEELS" {
				samples[index].airborne = value == "OFF" || value == "AIR" || value == "FALSE" || value == "0"
			} else {
				samples[index].airborne = value == "AIR" || value == "IN AIR" || value == "TRUE" || value == "1"
			}
		} else {
			samples[index].airborne = samples[index].movementSpeed >= profile.InferredAirborneSpeed ||
				(math.Abs(samples[index].verticalSpeed) >= profile.InferredAirborneRate && samples[index].movementSpeed >= profile.TaxiSpeed) ||
				(samples[index].agl >= profile.InferredAirborneAltitude && samples[index].movementSpeed >= profile.TaxiSpeed)
		}
	}
	return samples, nil
}

func phaseNumericSeries(rows []rawRow, key string, minimum, maximum float64) ([]float64, bool) {
	values := make([]float64, len(rows))
	valid := make([]bool, len(rows))
	for index, row := range rows {
		value, ok := numericValue(row.values, key)
		if ok && value >= minimum && value <= maximum {
			values[index], valid[index] = value, true
		}
	}
	count := 0
	for _, ok := range valid {
		if ok {
			count++
		}
	}
	if count == 0 {
		return values, false
	}
	fillPhaseSeries(values, valid)
	return values, true
}

func fillPhaseSeries(values []float64, valid []bool) {
	first := -1
	for index, ok := range valid {
		if ok {
			first = index
			break
		}
	}
	if first < 0 {
		return
	}
	for index := 0; index < first; index++ {
		values[index] = values[first]
	}
	prior := first
	for index := first + 1; index < len(values); index++ {
		if !valid[index] {
			continue
		}
		gap := index - prior
		for offset := 1; offset < gap; offset++ {
			ratio := float64(offset) / float64(gap)
			values[prior+offset] = values[prior] + (values[index]-values[prior])*ratio
		}
		prior = index
	}
	for index := prior + 1; index < len(values); index++ {
		values[index] = values[prior]
	}
}

func cleanPhaseSpikes(values []float64, threshold float64) []float64 {
	result := append([]float64(nil), values...)
	for index := 1; index+1 < len(result); index++ {
		if math.Abs(result[index]-result[index-1]) > threshold && math.Abs(result[index]-result[index+1]) > threshold && math.Abs(result[index-1]-result[index+1]) < threshold/2 {
			result[index] = (result[index-1] + result[index+1]) / 2
		}
	}
	copyValues := append([]float64(nil), result...)
	for index, value := range copyValues {
		start, end := maxInt(0, index-4), minInt(len(copyValues)-1, index+4)
		median := phaseMedian(append([]float64(nil), copyValues[start:end+1]...))
		if math.Abs(value-median) > threshold {
			result[index] = median
		}
	}
	return result
}

func smoothPhaseSeries(values []float64, window int) []float64 {
	result := make([]float64, len(values))
	radius := window / 2
	for index := range values {
		start, end := maxInt(0, index-radius), minInt(len(values)-1, index+radius)
		total := 0.0
		for cursor := start; cursor <= end; cursor++ {
			total += values[cursor]
		}
		result[index] = total / float64(end-start+1)
	}
	return result
}

func phaseTorqueSeries(rows []rawRow) []float64 {
	result := make([]float64, len(rows))
	for index, row := range rows {
		total, count := 0.0, 0
		for _, key := range phaseTorqueAliases {
			if value, ok := numericValue(row.values, key); ok {
				total, count = total+value, count+1
				if count == 2 {
					break
				}
			}
		}
		if count == 0 {
			result[index] = 50
		} else {
			result[index] = total / float64(count)
		}
	}
	return result
}

func assignPhaseRecordings(samples []phaseSample, intervalMs int64) [][2]int {
	if len(samples) == 0 {
		return nil
	}
	starts := []int{0}
	positiveSteps := make([]float64, 0)
	for index := 1; index < len(samples); index++ {
		previous, previousOK := numericSample(samples[index-1].row.sample)
		current, currentOK := numericSample(samples[index].row.sample)
		if previousOK && currentOK && current > previous {
			positiveSteps = append(positiveSteps, current-previous)
		}
	}
	typicalStep := 1.0
	if len(positiveSteps) > 0 {
		typicalStep = phaseMedian(positiveSteps)
	}
	maximumStep := math.Max(typicalStep*10, typicalStep+1)
	for index := 1; index < len(samples); index++ {
		boundary := false
		previous, previousOK := numericSample(samples[index-1].row.sample)
		current, currentOK := numericSample(samples[index].row.sample)
		if previousOK && currentOK {
			difference := current - previous
			boundary = difference <= 0 || difference > maximumStep
		}
		if !boundary {
			priorTime, priorOK := phaseRowTime(samples[index-1].row)
			currentTime, currentTimeOK := phaseRowTime(samples[index].row)
			if priorOK && currentTimeOK {
				difference := currentTime.Sub(priorTime).Milliseconds()
				maximumTimeStep := maxInt64(intervalMs*10, intervalMs+1000)
				boundary = difference < 0 || difference > maximumTimeStep
			}
		}
		if boundary {
			starts = append(starts, index)
		}
	}
	ranges := make([][2]int, len(starts))
	for index, start := range starts {
		end := len(samples) - 1
		if index+1 < len(starts) {
			end = starts[index+1] - 1
		}
		ranges[index] = [2]int{start, end}
		for cursor := start; cursor <= end; cursor++ {
			samples[cursor].recording = index + 1
			samples[cursor].timeMs = int64(cursor-start) * intervalMs
		}
	}
	return ranges
}

func phaseRowTime(row rawRow) (time.Time, bool) {
	return parseDateTime(firstValue(row.values, "LCLDATE", "LOCALDATE", "UTCDATE", "DATE"), firstValue(row.values, "LCLTIME", "LOCALTIME", "UTCTIME", "TIME"))
}

func detectPhaseFlights(samples []phaseSample, recordings [][2]int, profile PhaseProfile, intervalMs int64) []phaseFlightState {
	flights := make([]phaseFlightState, 0)
	minimumStateSamples := maxInt(1, int(math.Ceil(5000/float64(intervalMs))))
	for _, recording := range recordings {
		states := make([]bool, recording[1]-recording[0]+1)
		for index := range states {
			states[index] = samples[recording[0]+index].airborne
		}
		states = debouncePhaseStates(states, minimumStateSamples)
		for index, state := range states {
			samples[recording[0]+index].airborne = state
		}
		runs := phaseBoolRuns(states)
		airborneRuns := make([]boolRun, 0)
		for _, run := range runs {
			if run.value {
				run.start += recording[0]
				run.end += recording[0]
				airborneRuns = append(airborneRuns, run)
			}
		}
		for runIndex, airborne := range airborneRuns {
			regionStart := recording[0]
			if runIndex > 0 {
				regionStart = (airborneRuns[runIndex-1].end+airborne.start)/2 + 1
			}
			regionEnd := recording[1]
			if runIndex+1 < len(airborneRuns) {
				regionEnd = (airborne.end + airborneRuns[runIndex+1].start) / 2
			}
			departureCaptured := airborne.start > recording[0]
			arrivalCaptured := airborne.end < recording[1]
			status := "AIRBORNE_FRAGMENT"
			switch {
			case departureCaptured && arrivalCaptured:
				status = "COMPLETE"
			case departureCaptured:
				status = "DEPARTURE_ONLY"
			case arrivalCaptured:
				status = "ARRIVAL_ONLY"
			}
			departureAltitude := estimatePhaseGround(samples, regionStart, airborne.start-1, profile.TaxiSpeed)
			arrivalAltitude := estimatePhaseGround(samples, airborne.end+1, regionEnd, profile.TaxiSpeed)
			fallback := phaseRangePercentile(samples, airborne.start, airborne.end, 0.01)
			if departureAltitude != nil {
				fallback = *departureAltitude
			} else if arrivalAltitude != nil {
				fallback = *arrivalAltitude
			}
			topIndex := airborne.start
			for index := airborne.start + 1; index <= airborne.end; index++ {
				if samples[index].altitude > samples[topIndex].altitude {
					topIndex = index
				}
			}
			for index := regionStart; index <= regionEnd; index++ {
				reference := fallback
				if index <= topIndex && departureAltitude != nil {
					reference = *departureAltitude
				}
				if index > topIndex && arrivalAltitude != nil && samples[index].altitude < *arrivalAltitude+profile.ApproachAltitude {
					reference = *arrivalAltitude
				}
				samples[index].groundRef = reference
				samples[index].agl = math.Max(0, samples[index].altitude-reference)
			}

			taxiOutStart := airborne.start
			takeoffStart := airborne.start
			for index := regionStart; index < airborne.start; index++ {
				activeCount := 0
				for cursor := index; cursor < minInt(airborne.start, index+10); cursor++ {
					if samples[cursor].movementSpeed >= 5 || samples[cursor].torque >= 20 {
						activeCount++
					}
				}
				if activeCount >= 5 {
					taxiOutStart = index
					break
				}
			}
			for index := regionStart; index < airborne.start; index++ {
				if samples[index].movementSpeed >= profile.TaxiSpeed && samples[index].torque >= 50 {
					takeoffStart = index
					break
				}
			}
			landingEnd := airborne.end
			taxiInEnd := airborne.end
			for index := airborne.end + 1; index <= regionEnd; index++ {
				if samples[index].movementSpeed < profile.TaxiSpeed {
					landingEnd = maxInt(airborne.end, index-1)
					break
				}
			}
			for index := airborne.end + 1; index <= regionEnd; index++ {
				if samples[index].movementSpeed >= 5 || samples[index].torque >= 20 {
					taxiInEnd = index
				}
			}
			flightIndex := len(flights) + 1
			for index := regionStart; index <= regionEnd; index++ {
				samples[index].flightIndex = flightIndex
			}
			flight := phaseFlightState{
				PhaseFlight: PhaseFlight{
					Index: flightIndex, Status: status,
					StartRow: samples[regionStart].row.row, EndRow: samples[regionEnd].row.row,
					AirborneStartRow: samples[airborne.start].row.row, AirborneEndRow: samples[airborne.end].row.row,
					StartSample: phaseRecordedSample(samples[regionStart]), EndSample: phaseRecordedSample(samples[regionEnd]),
					AirborneStartSample: phaseRecordedSample(samples[airborne.start]), AirborneEndSample: phaseRecordedSample(samples[airborne.end]),
					DepartureCaptured: departureCaptured, ArrivalCaptured: arrivalCaptured,
					DepartureAltitude: departureAltitude, ArrivalAltitude: arrivalAltitude,
					DurationMs:         int64(regionEnd-regionStart+1) * intervalMs,
					AirborneDurationMs: int64(airborne.end-airborne.start+1) * intervalMs,
				},
				startIndex: regionStart, endIndex: regionEnd, airStartIndex: airborne.start,
				airEndIndex: airborne.end, topIndex: topIndex, taxiOutStart: taxiOutStart,
				takeoffStart: takeoffStart, landingEnd: landingEnd, taxiInEnd: taxiInEnd,
			}
			flights = append(flights, flight)
		}
	}
	return flights
}

func debouncePhaseStates(values []bool, minimum int) []bool {
	result := append([]bool(nil), values...)
	for iteration := 0; iteration < len(result); iteration++ {
		runs := phaseBoolRuns(result)
		changed := false
		for index := 1; index+1 < len(runs); index++ {
			run := runs[index]
			if run.end-run.start+1 < minimum && runs[index-1].value == runs[index+1].value && runs[index-1].value != run.value {
				for cursor := run.start; cursor <= run.end; cursor++ {
					result[cursor] = runs[index-1].value
				}
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return result
}

func phaseBoolRuns(values []bool) []boolRun {
	if len(values) == 0 {
		return nil
	}
	result := make([]boolRun, 0)
	start := 0
	for index := 1; index <= len(values); index++ {
		if index == len(values) || values[index] != values[start] {
			result = append(result, boolRun{start: start, end: index - 1, value: values[start]})
			start = index
		}
	}
	return result
}

func estimatePhaseGround(samples []phaseSample, start, end int, taxiSpeed float64) *float64 {
	if start > end || start < 0 || end >= len(samples) {
		return nil
	}
	stationary := make([]float64, 0)
	ground := make([]float64, 0)
	for index := start; index <= end; index++ {
		if samples[index].airborne {
			continue
		}
		ground = append(ground, samples[index].altitude)
		if samples[index].movementSpeed < taxiSpeed {
			stationary = append(stationary, samples[index].altitude)
		}
	}
	if len(stationary) > 0 {
		value := phaseMedian(stationary)
		return &value
	}
	if len(ground) > 0 {
		value := phaseMedian(ground)
		return &value
	}
	return nil
}

func classifyPhaseSamples(samples []phaseSample, flights []phaseFlightState, recordings [][2]int, profile PhaseProfile, intervalMs int64) {
	flightByIndex := make(map[int]*phaseFlightState, len(flights))
	for index := range flights {
		flightByIndex[flights[index].Index] = &flights[index]
	}
	for index := range samples {
		flight := flightByIndex[samples[index].flightIndex]
		samples[index].phase = classifyPhaseSample(samples, index, flight, profile)
	}
	minimumDuration := maxInt(1, int(math.Ceil(5000/float64(intervalMs))))
	for _, flight := range flights {
		smoothPhaseLabels(samples, flight.startIndex, flight.endIndex, minimumDuration)
		finalStart := -1
		for index := flight.airStartIndex; index <= flight.airEndIndex; index++ {
			if samples[index].phase == "FINAL_APPROACH" {
				finalStart = index
				break
			}
		}
		if finalStart >= 0 {
			for index := finalStart; index <= flight.airEndIndex; index++ {
				if samples[index].phase == "APPROACH" {
					samples[index].phase = "FINAL_APPROACH"
				}
			}
		}
	}
	for _, recording := range recordings {
		start := recording[0]
		for start <= recording[1] {
			if samples[start].flightIndex != 0 {
				start++
				continue
			}
			end := start
			for end+1 <= recording[1] && samples[end+1].flightIndex == 0 {
				end++
			}
			smoothPhaseLabels(samples, start, end, minimumDuration)
			start = end + 1
		}
	}
}

func classifyPhaseSample(samples []phaseSample, index int, flight *phaseFlightState, profile PhaseProfile) string {
	sample := samples[index]
	if !sample.airborne {
		if flight != nil {
			switch {
			case index < flight.airStartIndex:
				if index >= flight.takeoffStart {
					return "TAKEOFF"
				}
				if index >= flight.taxiOutStart {
					return "TAXI-OUT"
				}
				return "GROUND"
			case index <= flight.landingEnd:
				return "LANDING"
			case index <= flight.taxiInEnd:
				return "TAXI-IN"
			default:
				return "GROUND"
			}
		}
		if sample.movementSpeed < 5 {
			return "GROUND"
		}
		return "TAXI-OUT"
	}

	afterTop := flight != nil && index >= flight.topIndex
	nearDeparture := flight != nil && flight.DepartureCaptured && index >= flight.airStartIndex && index <= flight.airStartIndex+120
	if nearDeparture && sample.agl < profile.LandingAltitude && sample.airspeed >= profile.TaxiSpeed {
		return "TAKEOFF"
	}
	if sample.verticalSpeed > profile.ClimbRate {
		if afterTop && flight != nil && flight.ArrivalCaptured {
			if sample.agl < profile.ApproachAltitude {
				return "APPROACH"
			}
			return "CRUISE"
		}
		if flight != nil && flight.DepartureCaptured && sample.agl < profile.InitialClimbAltitude {
			return "INITIAL_CLIMB"
		}
		return "CLIMB"
	}
	if sample.verticalSpeed < profile.DescentRate {
		if !afterTop {
			return "CRUISE"
		}
		if sample.speedbrake && sample.agl > profile.ApproachAltitude {
			return "DESCENT"
		}
		if sample.agl < profile.ApproachAltitude {
			if sample.flaps > 5 && sample.agl < 1000 {
				return "FINAL_APPROACH"
			}
			return "APPROACH"
		}
		return "DESCENT"
	}
	if sample.agl > profile.ApproachAltitude {
		return "CRUISE"
	}
	if sample.flaps > 5 || afterTop || (flight != nil && sample.agl < profile.ApproachAltitude && (sample.verticalSpeed < 0 || sample.airspeed <= profile.RotationSpeed+20)) {
		return "APPROACH"
	}
	return "CRUISE"
}

func smoothPhaseLabels(samples []phaseSample, start, end, minimum int) {
	for iteration := 0; iteration < 20; iteration++ {
		runs := phaseLabelRuns(samples, start, end)
		changed := false
		for index, run := range runs {
			duration := run[1] - run[0] + 1
			required := minimum
			if samples[run[0]].phase == "TAKEOFF" || samples[run[0]].phase == "LANDING" {
				required = maxInt(1, minimum*2/5)
			}
			replacement := ""
			if index > 0 && index+1 < len(runs) && samples[runs[index-1][0]].phase == samples[runs[index+1][0]].phase && duration <= minimum*9 {
				replacement = samples[runs[index-1][0]].phase
			} else if duration < required {
				switch {
				case index > 0 && index+1 < len(runs):
					previousDuration := runs[index-1][1] - runs[index-1][0] + 1
					nextDuration := runs[index+1][1] - runs[index+1][0] + 1
					if previousDuration >= nextDuration {
						replacement = samples[runs[index-1][0]].phase
					} else {
						replacement = samples[runs[index+1][0]].phase
					}
				case index > 0:
					replacement = samples[runs[index-1][0]].phase
				case index+1 < len(runs):
					replacement = samples[runs[index+1][0]].phase
				}
			}
			if replacement != "" {
				for cursor := run[0]; cursor <= run[1]; cursor++ {
					samples[cursor].phase = replacement
				}
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
}

func phaseLabelRuns(samples []phaseSample, start, end int) [][2]int {
	if start > end {
		return nil
	}
	result := make([][2]int, 0)
	runStart := start
	for index := start + 1; index <= end+1; index++ {
		if index > end || samples[index].phase != samples[runStart].phase {
			result = append(result, [2]int{runStart, index - 1})
			runStart = index
		}
	}
	return result
}

func buildPhaseRuns(samples []phaseSample, flights []phaseFlightState, intervalMs int64) []PhaseRun {
	if len(samples) == 0 {
		return []PhaseRun{}
	}
	statusByFlight := make(map[int]string, len(flights))
	for _, flight := range flights {
		statusByFlight[flight.Index] = flight.Status
	}
	result := make([]PhaseRun, 0)
	start := 0
	for index := 1; index <= len(samples); index++ {
		if index == len(samples) || samples[index].phase != samples[start].phase || samples[index].flightIndex != samples[start].flightIndex || samples[index].recording != samples[start].recording {
			result = append(result, PhaseRun{
				FlightIndex: samples[start].flightIndex, FlightStatus: statusByFlight[samples[start].flightIndex],
				Phase: samples[start].phase, StartRow: samples[start].row.row, EndRow: samples[index-1].row.row,
				StartSample: phaseRecordedSample(samples[start]), EndSample: phaseRecordedSample(samples[index-1]),
				DurationMs: int64(index-start) * intervalMs,
			})
			start = index
		}
	}
	return result
}

func phaseRecordedSample(sample phaseSample) string {
	if strings.TrimSpace(sample.row.sample) != "" {
		return sample.row.sample
	}
	return strconv.Itoa(sample.row.row)
}

func phaseMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 0 {
		return (values[middle-1] + values[middle]) / 2
	}
	return values[middle]
}

func phasePercentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	position := quantile * float64(len(ordered)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return ordered[lower]
	}
	return ordered[lower] + (ordered[upper]-ordered[lower])*(position-float64(lower))
}

func phaseRangePercentile(samples []phaseSample, start, end int, quantile float64) float64 {
	values := make([]float64, 0, end-start+1)
	for index := start; index <= end; index++ {
		values = append(values, samples[index].altitude)
	}
	return phasePercentile(values, quantile)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
