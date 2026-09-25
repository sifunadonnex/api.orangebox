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
	"unicode"

	"fdm-backend/models"
)

const EngineVersion = "2.1.0"

type Definition struct {
	DefinitionID string
	VersionID    string
	EventCode    string
	DisplayName  string
	Description  string
	RuleHash     string
	Rule         models.EventRule
}

type Options struct {
	SampleIntervalMs  int64
	MaxEvidencePoints int
	StartRow          int
	EndRow            int
	RebaseTime        bool
	PhaseRuns         []PhaseRun
}

type Diagnostic struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	RuleVersionID string `json:"ruleVersionId,omitempty"`
	Count         int    `json:"count"`
}

type EvidencePoint struct {
	Row    int     `json:"row"`
	Sample string  `json:"sample,omitempty"`
	TimeMs int64   `json:"timeMs"`
	Value  float64 `json:"value"`
}

type Occurrence struct {
	DefinitionID string          `json:"definitionId"`
	VersionID    string          `json:"versionId"`
	EventCode    string          `json:"eventCode"`
	EventName    string          `json:"eventName"`
	Description  string          `json:"description"`
	RuleHash     string          `json:"ruleHash"`
	ParameterID  string          `json:"parameterId"`
	Unit         string          `json:"unit,omitempty"`
	Aggregation  string          `json:"aggregation"`
	Value        float64         `json:"value"`
	Severity     string          `json:"severity"`
	Phase        string          `json:"phase"`
	StartTimeMs  int64           `json:"startTimeMs"`
	EndTimeMs    int64           `json:"endTimeMs"`
	DurationMs   int64           `json:"durationMs"`
	StartRow     int             `json:"startRow"`
	EndRow       int             `json:"endRow"`
	StartSample  string          `json:"startSample,omitempty"`
	EndSample    string          `json:"endSample,omitempty"`
	PointCount   int             `json:"pointCount"`
	Points       []EvidencePoint `json:"points"`
}

type Result struct {
	EngineVersion       string       `json:"engineVersion"`
	TimingSource        string       `json:"timingSource"`
	SampleIntervalMs    int64        `json:"sampleIntervalMs"`
	RowCount            int          `json:"rowCount"`
	ApplicableRuleCount int          `json:"applicableRuleCount"`
	EvaluatedRuleCount  int          `json:"evaluatedRuleCount"`
	Occurrences         []Occurrence `json:"occurrences"`
	Diagnostics         []Diagnostic `json:"diagnostics"`
}

type rawRow struct {
	row    int
	values map[string]string
	sample string
	phase  string
}

type frame struct {
	row    int
	timeMs int64
	sample string
	phase  string
	values map[string]string
}

type diagnosticCollector map[string]*Diagnostic

func (d diagnosticCollector) add(code, severity, message, ruleVersionID string) {
	key := code + "\x00" + ruleVersionID
	if existing, ok := d[key]; ok {
		existing.Count++
		return
	}
	d[key] = &Diagnostic{Code: code, Severity: severity, Message: message, RuleVersionID: ruleVersionID, Count: 1}
}

func (d diagnosticCollector) list() []Diagnostic {
	result := make([]Diagnostic, 0, len(d))
	for _, item := range d {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Severity != result[j].Severity {
			return result[i].Severity < result[j].Severity
		}
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		return result[i].RuleVersionID < result[j].RuleVersionID
	})
	return result
}

// AnalyzeFile parses a flight once and evaluates every applicable published
// rule against the same timestamped frames. Invalid or missing data is reported
// as an explicit diagnostic and never coerced to zero.
func AnalyzeFile(path string, definitions []Definition, options Options) (Result, error) {
	if options.SampleIntervalMs <= 0 {
		return Result{}, errors.New("sample interval must be greater than zero")
	}
	if options.MaxEvidencePoints <= 0 {
		options.MaxEvidencePoints = 500
	}

	headers, rows, parseDiagnostics, err := readCSV(path)
	if err != nil {
		return Result{}, err
	}
	rows, err = rowsInRange(rows, options.StartRow, options.EndRow)
	if err != nil {
		return Result{}, err
	}
	diagnostics := diagnosticCollector{}
	for _, item := range parseDiagnostics {
		diagnostics.add(item.Code, item.Severity, item.Message, item.RuleVersionID)
	}
	frames, timingSource, timingDiagnostics, err := timestampRows(rows, options.SampleIntervalMs)
	if err != nil {
		return Result{}, err
	}
	if options.RebaseTime {
		rebaseFrameTimes(frames)
	}
	applyPhaseRunsToFrames(frames, options.PhaseRuns)
	for _, item := range timingDiagnostics {
		diagnostics.add(item.Code, item.Severity, item.Message, item.RuleVersionID)
	}
	if len(definitions) == 0 {
		diagnostics.add("NO_APPLICABLE_RULES", "warning", "No published event definition is assigned to this aircraft", "")
	}

	states := make([]ruleState, 0, len(definitions))
	for _, definition := range definitions {
		state := newRuleState(definition, options.MaxEvidencePoints)
		missing := make([]string, 0)
		for _, parameter := range definition.Rule.Parameters {
			if _, ok := headers[canonical(parameter.ID)]; !ok && (parameter.Required || parameter.ID == definition.Rule.Measurement.ParameterID) {
				missing = append(missing, parameter.ID)
			}
		}
		if len(missing) > 0 {
			state.disabled = true
			diagnostics.add("RULE_REQUIRED_COLUMNS_MISSING", "error", "Required CSV columns are missing: "+strings.Join(missing, ", "), definition.VersionID)
		}
		states = append(states, state)
	}

	occurrences := make([]Occurrence, 0)
	evaluatedRules := 0
	for index := range states {
		if !states[index].disabled {
			evaluatedRules++
		}
	}
	for _, current := range frames {
		for index := range states {
			states[index].consume(current, diagnostics, &occurrences)
		}
	}
	for index := range states {
		states[index].finish(diagnostics, &occurrences)
	}

	return Result{
		EngineVersion: EngineVersion, TimingSource: timingSource,
		SampleIntervalMs: options.SampleIntervalMs, RowCount: len(frames),
		ApplicableRuleCount: len(definitions), EvaluatedRuleCount: evaluatedRules,
		Occurrences: occurrences, Diagnostics: diagnostics.list(),
	}, nil
}

func readCSV(path string) (map[string]string, []rawRow, []Diagnostic, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open CSV: %w", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	reader.LazyQuotes = true
	headerLine := 0
	var headerRecord []string
	for line := 1; line <= 50; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, nil, fmt.Errorf("inspect CSV row %d: %w", line, readErr)
		}
		if looksLikeCSVHeader(record) {
			headerLine = line
			headerRecord = append([]string(nil), record...)
			break
		}
	}
	if headerLine == 0 {
		return nil, nil, nil, errors.New("CSV header was not found in the first 50 rows")
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
	if len(headers) == 0 {
		return nil, nil, nil, errors.New("CSV has no usable headers")
	}

	rows := make([]rawRow, 0, 1024)
	rowNumber := headerLine
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		rowNumber++
		if readErr != nil {
			return nil, nil, nil, fmt.Errorf("read CSV row %d: %w", rowNumber, readErr)
		}
		values := make(map[string]string, len(indices))
		nonEmpty := 0
		for key, index := range indices {
			if index >= len(record) {
				continue
			}
			value := strings.TrimSpace(record[index])
			if value != "" {
				values[key] = value
				nonEmpty++
			}
		}
		if nonEmpty == 0 || looksLikeUnitsRow(values) {
			continue
		}
		rows = append(rows, rawRow{row: rowNumber, values: values, sample: firstValue(values, "SAMPLE", "FRAME"), phase: strings.ToUpper(strings.TrimSpace(firstValue(values, "PHASE", "FLIGHTPHASE")))})
	}
	if len(rows) == 0 {
		return nil, nil, nil, errors.New("CSV contains no data rows")
	}
	return headers, rows, diagnostics, nil
}

func looksLikeCSVHeader(record []string) bool {
	keys := make(map[string]bool, len(record))
	nonEmpty := 0
	for index, raw := range record {
		if index == 0 {
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		key := canonical(raw)
		if key != "" {
			keys[key] = true
			nonEmpty++
		}
	}
	if nonEmpty < 2 {
		return false
	}
	for _, key := range []string{
		"SAMPLE", "FRAME", "TIME", "UTCTIME", "LCLTIME", "TIMEELAPSED", "ELAPSEDTIME", "ELAPSEDSECONDS", "TIMESEC",
		"LATITUDE", "LATITUDEDEG", "GPSLATITUDE", "LONGITUDE", "LONGITUDEDEG", "GPSLONGITUDE",
		"IAS", "AIRSPEED", "ALTMSL", "ALTITUDE", "ALTITUDEFT", "ALTITUDEM", "PHASE", "FLIGHTPHASE", "FLIGHTSTAGE",
	} {
		if keys[key] {
			return true
		}
	}
	return false
}

func applyPhaseRunsToFrames(frames []frame, runs []PhaseRun) {
	if len(frames) == 0 || len(runs) == 0 {
		return
	}
	ordered := append([]PhaseRun(nil), runs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartRow < ordered[j].StartRow })
	runIndex := 0
	for index := range frames {
		for runIndex < len(ordered) && frames[index].row > ordered[runIndex].EndRow {
			runIndex++
		}
		if runIndex < len(ordered) && frames[index].row >= ordered[runIndex].StartRow && frames[index].row <= ordered[runIndex].EndRow {
			frames[index].phase = strings.ToUpper(strings.TrimSpace(ordered[runIndex].Phase))
		}
	}
}

func looksLikeUnitsRow(values map[string]string) bool {
	sample := firstValue(values, "SAMPLE", "FRAME")
	if sample != "" {
		if _, err := strconv.ParseFloat(sample, 64); err == nil {
			return false
		}
	}
	bracketed := 0
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			bracketed++
		}
	}
	return bracketed >= 2
}

func timestampRows(rows []rawRow, intervalMs int64) ([]frame, string, []Diagnostic, error) {
	candidates := make([]candidate, 0)
	for _, alias := range []string{"TIMEELAPSED", "TIMESEC", "ELAPSEDTIME", "ELAPSEDSECONDS", "TIMESTAMPSEC", "FLIGHTTIME", "SECONDS"} {
		item := candidate{name: strings.ToLower(alias), times: make([]int64, len(rows)), valid: make([]bool, len(rows))}
		for i, row := range rows {
			if value := row.values[alias]; value != "" {
				item.times[i], item.valid[i] = parseSeconds(value)
			}
		}
		candidates = append(candidates, item)
	}
	minutes := candidate{name: "time_min", times: make([]int64, len(rows)), valid: make([]bool, len(rows))}
	for i, row := range rows {
		if value := row.values["TIMEMIN"]; value != "" {
			if numeric, err := strconv.ParseFloat(value, 64); err == nil && isFinite(numeric) {
				minutes.times[i], minutes.valid[i] = int64(math.Round(numeric*60000)), true
			}
		}
	}
	candidates = append(candidates, minutes)
	dateTime := buildDateTime(rows)
	candidates = append(candidates, dateTime)
	gmt := buildGMTTime(rows)
	candidates = append(candidates, gmt)
	sample := buildSampleTime(rows, intervalMs)
	candidates = append(candidates, sample)

	minimumValid := int(math.Ceil(float64(len(rows)) * 0.90))
	if minimumValid < 1 {
		minimumValid = 1
	}
	selected := candidate{}
	for _, item := range candidates {
		if countValid(item.valid) >= minimumValid && monotonic(item.times, item.valid) {
			selected = item
			break
		}
	}
	if selected.name == "" {
		return nil, "", nil, errors.New("CSV has no reliable monotonic time source; provide numeric Sample values and a correct frame interval")
	}
	diagnostics := make([]Diagnostic, 0)
	frames := make([]frame, 0, len(rows))
	for i, row := range rows {
		if !selected.valid[i] {
			diagnostics = append(diagnostics, Diagnostic{Code: "ROW_TIME_MISSING", Severity: "warning", Message: "A row was skipped because its selected time value is missing or invalid", Count: 1})
			continue
		}
		frames = append(frames, frame{row: row.row, timeMs: selected.times[i], sample: row.sample, phase: row.phase, values: row.values})
	}
	if len(frames) == 0 {
		return nil, selected.name, diagnostics, errors.New("CSV has no timestamped data rows")
	}
	return frames, selected.name, diagnostics, nil
}

func buildDateTime(rows []rawRow) candidate {
	item := candidate{name: "date_time", times: make([]int64, len(rows)), valid: make([]bool, len(rows))}
	var base time.Time
	for i, row := range rows {
		dateValue := firstValue(row.values, "LCLDATE", "LOCALDATE", "UTCDATE", "DATE")
		timeValue := firstValue(row.values, "LCLTIME", "LOCALTIME", "UTCTIME", "TIME")
		if dateValue == "" || timeValue == "" {
			continue
		}
		parsed, ok := parseDateTime(dateValue, timeValue)
		if !ok {
			continue
		}
		if base.IsZero() {
			base = parsed
		}
		item.times[i] = parsed.Sub(base).Milliseconds()
		item.valid[i] = true
	}
	return item
}

func parseDateTime(dateValue, timeValue string) (time.Time, bool) {
	combined := strings.TrimSpace(dateValue) + " " + strings.TrimSpace(timeValue)
	for _, layout := range []string{
		"1/2/2006 15:04:05.999999999",
		"1/2/2006 15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2/1/2006 15:04:05.999999999",
		"2/1/2006 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, combined, time.UTC); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseSeconds(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return 0, false
		}
		seconds := 0.0
		multiplier := 1.0
		for i := len(parts) - 1; i >= 0; i-- {
			number, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
			if err != nil {
				return 0, false
			}
			seconds += number * multiplier
			multiplier *= 60
		}
		return int64(math.Round(seconds * 1000)), true
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFinite(number) {
		return 0, false
	}
	return int64(math.Round(number * 1000)), true
}

func buildGMTTime(rows []rawRow) candidate {
	item := candidate{name: "gmt", times: make([]int64, len(rows)), valid: make([]bool, len(rows))}
	var previous int64 = -1
	var dayOffset int64
	for i, row := range rows {
		hour, hourErr := strconv.Atoi(row.values["GMTHOURS"])
		minute, minuteErr := strconv.Atoi(row.values["GMTMINUTES"])
		secondValue, secondErr := strconv.ParseFloat(row.values["GMTSECONDS"], 64)
		if hourErr != nil || minuteErr != nil || secondErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 || secondValue < 0 || secondValue >= 60 {
			continue
		}
		current := int64(math.Round((float64(hour*3600+minute*60)+secondValue)*1000)) + dayOffset
		if previous >= 0 && current < previous && previous-current > 12*60*60*1000 {
			dayOffset += 24 * 60 * 60 * 1000
			current += 24 * 60 * 60 * 1000
		}
		item.times[i], item.valid[i], previous = current, true, current
	}
	return item
}

func buildSampleTime(rows []rawRow, intervalMs int64) candidate {
	item := candidate{name: "sample_interval", times: make([]int64, len(rows)), valid: make([]bool, len(rows))}
	base := 0.0
	baseSet := false
	for i, row := range rows {
		value, err := strconv.ParseFloat(row.sample, 64)
		if err != nil || !isFinite(value) {
			continue
		}
		if !baseSet {
			base, baseSet = value, true
		}
		item.times[i] = int64(math.Round((value - base) * float64(intervalMs)))
		item.valid[i] = true
	}
	return item
}

type candidate struct {
	name  string
	times []int64
	valid []bool
}

func countValid(values []bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func monotonic(times []int64, valid []bool) bool {
	var prior int64
	hasPrior := false
	progressed := false
	validCount := 0
	for index, ok := range valid {
		if !ok {
			continue
		}
		validCount++
		if hasPrior && times[index] < prior {
			return false
		}
		if hasPrior && times[index] > prior {
			progressed = true
		}
		prior, hasPrior = times[index], true
	}
	return hasPrior && (validCount == 1 || progressed)
}

type scalar struct {
	kind    string
	number  float64
	text    string
	boolean bool
}

func evaluate(node models.RuleNode, current frame, parameterTypes map[string]string) (scalar, error) {
	switch node.Type {
	case "parameter":
		value, ok := current.values[canonical(node.ParameterID)]
		if !ok || strings.TrimSpace(value) == "" {
			return scalar{}, fmt.Errorf("parameter %s is missing", node.ParameterID)
		}
		if parameterTypes[node.ParameterID] == "numeric" {
			numeric, err := strconv.ParseFloat(value, 64)
			if err != nil || !isFinite(numeric) {
				return scalar{}, fmt.Errorf("parameter %s is not numeric", node.ParameterID)
			}
			return scalar{kind: "number", number: numeric}, nil
		}
		return scalar{kind: "string", text: strings.TrimSpace(value)}, nil
	case "number":
		if node.NumberValue == nil {
			return scalar{}, errors.New("number literal is empty")
		}
		return scalar{kind: "number", number: *node.NumberValue}, nil
	case "string":
		if node.StringValue == nil {
			return scalar{}, errors.New("string literal is empty")
		}
		return scalar{kind: "string", text: *node.StringValue}, nil
	case "arithmetic":
		left, err := evaluate(*node.Left, current, parameterTypes)
		if err != nil {
			return scalar{}, err
		}
		right, err := evaluate(*node.Right, current, parameterTypes)
		if err != nil {
			return scalar{}, err
		}
		if left.kind != "number" || right.kind != "number" {
			return scalar{}, errors.New("arithmetic operand is not numeric")
		}
		result := 0.0
		switch node.Operator {
		case "+":
			result = left.number + right.number
		case "-":
			result = left.number - right.number
		case "*":
			result = left.number * right.number
		case "/":
			if right.number == 0 {
				return scalar{}, errors.New("division by zero")
			}
			result = left.number / right.number
		default:
			return scalar{}, errors.New("unsupported arithmetic operator")
		}
		if !isFinite(result) {
			return scalar{}, errors.New("arithmetic result is not finite")
		}
		return scalar{kind: "number", number: result}, nil
	case "comparison":
		left, err := evaluate(*node.Left, current, parameterTypes)
		if err != nil {
			return scalar{}, err
		}
		right, err := evaluate(*node.Right, current, parameterTypes)
		if err != nil {
			return scalar{}, err
		}
		if left.kind != right.kind {
			return scalar{}, errors.New("comparison types differ")
		}
		result := false
		if left.kind == "number" {
			switch node.Operator {
			case ">":
				result = left.number > right.number
			case ">=":
				result = left.number >= right.number
			case "<":
				result = left.number < right.number
			case "<=":
				result = left.number <= right.number
			case "==":
				result = almostEqual(left.number, right.number)
			case "!=":
				result = !almostEqual(left.number, right.number)
			default:
				return scalar{}, errors.New("unsupported comparison operator")
			}
		} else {
			leftText, rightText := strings.TrimSpace(left.text), strings.TrimSpace(right.text)
			switch node.Operator {
			case "==":
				result = strings.EqualFold(leftText, rightText)
			case "!=":
				result = !strings.EqualFold(leftText, rightText)
			default:
				return scalar{}, errors.New("ordered string comparison is unsupported")
			}
		}
		return scalar{kind: "boolean", boolean: result}, nil
	case "all", "any":
		for _, child := range node.Children {
			value, err := evaluate(child, current, parameterTypes)
			if err != nil {
				return scalar{}, err
			}
			if value.kind != "boolean" {
				return scalar{}, errors.New("logical child is not boolean")
			}
			if node.Type == "all" && !value.boolean {
				return scalar{kind: "boolean", boolean: false}, nil
			}
			if node.Type == "any" && value.boolean {
				return scalar{kind: "boolean", boolean: true}, nil
			}
		}
		return scalar{kind: "boolean", boolean: node.Type == "all"}, nil
	case "not":
		if len(node.Children) != 1 {
			return scalar{}, errors.New("not requires one child")
		}
		value, err := evaluate(node.Children[0], current, parameterTypes)
		if err != nil {
			return scalar{}, err
		}
		if value.kind != "boolean" {
			return scalar{}, errors.New("not child is not boolean")
		}
		return scalar{kind: "boolean", boolean: !value.boolean}, nil
	default:
		return scalar{}, fmt.Errorf("unsupported node type %s", node.Type)
	}
}

type ruleState struct {
	definition        Definition
	parameterTypes    map[string]string
	phases            map[string]bool
	disabled          bool
	active            bool
	phase             string
	startTime         int64
	lastTrueTime      int64
	startRow          int
	lastTrueRow       int
	startSample       string
	lastTrueSample    string
	cooldownUntil     int64
	pointCount        int
	points            []EvidencePoint
	aggCount          int
	aggSum            float64
	aggMin            float64
	aggMax            float64
	aggAbs            float64
	aggLast           float64
	maxEvidencePoints int
}

func newRuleState(definition Definition, maxPoints int) ruleState {
	types := make(map[string]string, len(definition.Rule.Parameters))
	for _, parameter := range definition.Rule.Parameters {
		types[parameter.ID] = parameter.DataType
	}
	phases := make(map[string]bool, len(definition.Rule.Applicability.Phases))
	for _, phase := range definition.Rule.Applicability.Phases {
		phases[strings.ToUpper(strings.TrimSpace(phase))] = true
	}
	return ruleState{definition: definition, parameterTypes: types, phases: phases, maxEvidencePoints: maxPoints}
}

func (s *ruleState) consume(current frame, diagnostics diagnosticCollector, occurrences *[]Occurrence) {
	if s.disabled {
		return
	}
	applicable := s.phases[current.phase]
	if current.phase == "" {
		diagnostics.add("PHASE_MISSING", "warning", "A row has no flight phase and was not evaluated", s.definition.VersionID)
	}
	if applicable && s.definition.Rule.Applicability.Condition != nil {
		value, err := evaluate(*s.definition.Rule.Applicability.Condition, current, s.parameterTypes)
		if err != nil {
			diagnostics.add("APPLICABILITY_NOT_EVALUABLE", "warning", err.Error(), s.definition.VersionID)
			applicable = false
		} else {
			applicable = value.boolean
		}
	}
	if s.active && (!applicable || current.phase != s.phase) {
		s.close(diagnostics, occurrences)
	}
	if !applicable {
		return
	}

	measurement, measurementErr := s.measurement(current)
	triggerValue, triggerErr := evaluate(s.definition.Rule.Trigger, current, s.parameterTypes)
	if triggerErr != nil {
		diagnostics.add("TRIGGER_NOT_EVALUABLE", "warning", triggerErr.Error(), s.definition.VersionID)
	}
	triggered := triggerErr == nil && triggerValue.kind == "boolean" && triggerValue.boolean
	if s.active && !triggered && measurementErr == nil && s.hysteresisHolds(measurement) {
		triggered = true
	}
	if triggered && measurementErr != nil {
		diagnostics.add("MEASUREMENT_NOT_EVALUABLE", "warning", measurementErr.Error(), s.definition.VersionID)
		triggered = false
	}

	if triggered {
		if !s.active {
			if current.timeMs < s.cooldownUntil {
				return
			}
			s.begin(current)
		}
		s.addPoint(current, measurement)
		return
	}
	if s.active && current.timeMs-s.lastTrueTime > s.definition.Rule.Temporal.MergeGapMs {
		s.close(diagnostics, occurrences)
	}
}

func (s *ruleState) measurement(current frame) (float64, error) {
	value, ok := current.values[canonical(s.definition.Rule.Measurement.ParameterID)]
	if !ok || value == "" {
		return 0, fmt.Errorf("measurement parameter %s is missing", s.definition.Rule.Measurement.ParameterID)
	}
	numeric, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFinite(numeric) {
		return 0, fmt.Errorf("measurement parameter %s is not numeric", s.definition.Rule.Measurement.ParameterID)
	}
	return numeric, nil
}

func (s *ruleState) hysteresisHolds(value float64) bool {
	threshold := s.definition.Rule.Temporal.ClearThreshold
	if threshold == nil || len(s.definition.Rule.Severities) == 0 {
		return false
	}
	operator := s.definition.Rule.Severities[0].Operator
	if operator == ">" || operator == ">=" {
		return value > *threshold
	}
	if operator == "<" || operator == "<=" {
		return value < *threshold
	}
	return false
}

func (s *ruleState) begin(current frame) {
	s.active, s.phase = true, current.phase
	s.startTime, s.lastTrueTime = current.timeMs, current.timeMs
	s.startRow, s.lastTrueRow = current.row, current.row
	s.startSample, s.lastTrueSample = current.sample, current.sample
	s.pointCount, s.aggCount, s.aggSum = 0, 0, 0
	s.points = s.points[:0]
}

func (s *ruleState) addPoint(current frame, value float64) {
	s.lastTrueTime, s.lastTrueRow, s.lastTrueSample = current.timeMs, current.row, current.sample
	s.pointCount++
	if len(s.points) < s.maxEvidencePoints {
		s.points = append(s.points, EvidencePoint{Row: current.row, Sample: current.sample, TimeMs: current.timeMs, Value: value})
	}
	if s.aggCount == 0 {
		s.aggMin, s.aggMax, s.aggAbs = value, value, value
	}
	if value < s.aggMin {
		s.aggMin = value
	}
	if value > s.aggMax {
		s.aggMax = value
	}
	if math.Abs(value) > math.Abs(s.aggAbs) {
		s.aggAbs = value
	}
	s.aggCount++
	s.aggSum += value
	s.aggLast = value
}

func (s *ruleState) finish(diagnostics diagnosticCollector, occurrences *[]Occurrence) {
	if s.active {
		s.close(diagnostics, occurrences)
	}
}

func (s *ruleState) close(diagnostics diagnosticCollector, occurrences *[]Occurrence) {
	if !s.active {
		return
	}
	duration := s.lastTrueTime - s.startTime
	value := s.aggregate()
	severity := selectSeverity(s.definition.Rule.Severities, value, duration)
	if duration >= s.definition.Rule.Temporal.MinimumDurationMs && severity != "" {
		points := append([]EvidencePoint(nil), s.points...)
		*occurrences = append(*occurrences, Occurrence{
			DefinitionID: s.definition.DefinitionID, VersionID: s.definition.VersionID,
			EventCode: s.definition.EventCode, EventName: s.definition.DisplayName,
			Description: s.definition.Description, RuleHash: s.definition.RuleHash,
			ParameterID: s.definition.Rule.Measurement.ParameterID, Unit: s.definition.Rule.Measurement.Unit,
			Aggregation: s.definition.Rule.Measurement.Aggregation, Value: value, Severity: severity,
			Phase: s.phase, StartTimeMs: s.startTime, EndTimeMs: s.lastTrueTime,
			DurationMs: duration, StartRow: s.startRow, EndRow: s.lastTrueRow,
			StartSample: s.startSample, EndSample: s.lastTrueSample,
			PointCount: s.pointCount, Points: points,
		})
	} else if duration >= s.definition.Rule.Temporal.MinimumDurationMs && severity == "" {
		diagnostics.add("NO_SEVERITY_MATCH", "warning", "A trigger window met its minimum duration but matched no severity band", s.definition.VersionID)
	}
	s.cooldownUntil = s.lastTrueTime + s.definition.Rule.Temporal.CooldownMs
	s.active = false
}

func (s *ruleState) aggregate() float64 {
	switch s.definition.Rule.Measurement.Aggregation {
	case "MIN":
		return s.aggMin
	case "ABS_MAX":
		return s.aggAbs
	case "AVG":
		if s.aggCount == 0 {
			return 0
		}
		return s.aggSum / float64(s.aggCount)
	case "LAST":
		return s.aggLast
	default:
		return s.aggMax
	}
}

func selectSeverity(severities []models.RuleSeverity, value float64, duration int64) string {
	selected, rank := "", 0
	ranks := map[string]int{"LOW": 1, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4}
	for _, severity := range severities {
		if duration < severity.MinimumDurationMs || !compare(value, severity.Operator, severity.Threshold) {
			continue
		}
		if ranks[severity.Level] > rank {
			selected, rank = severity.Level, ranks[severity.Level]
		}
	}
	return selected
}

func compare(left float64, operator string, right float64) bool {
	switch operator {
	case ">":
		return left > right
	case ">=":
		return left >= right
	case "<":
		return left < right
	case "<=":
		return left <= right
	case "==":
		return almostEqual(left, right)
	case "!=":
		return !almostEqual(left, right)
	}
	return false
}

func canonical(value string) string {
	var builder strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(unicode.ToUpper(character))
		}
	}
	return builder.String()
}

func firstValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := values[key]; value != "" {
			return value
		}
	}
	return ""
}

func almostEqual(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-9*scale
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
