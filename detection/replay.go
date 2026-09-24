package detection

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type ReplayOptions struct {
	SampleIntervalMs int64
	MaxPoints        int
	StartRow         int
	EndRow           int
	RebaseTime       bool
}

type ReplayCapabilities struct {
	Position      bool `json:"position"`
	Timing        bool `json:"timing"`
	Altitude      bool `json:"altitude"`
	GroundSpeed   bool `json:"groundSpeed"`
	Airspeed      bool `json:"airspeed"`
	Heading       bool `json:"heading"`
	VerticalSpeed bool `json:"verticalSpeed"`
	Pitch         bool `json:"pitch"`
	Roll          bool `json:"roll"`
	Phase         bool `json:"phase"`
	Airborne      bool `json:"airborne"`
}

type ReplayBounds struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

type ReplayMeasurement struct {
	Source     string `json:"source,omitempty"`
	SourceUnit string `json:"sourceUnit,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Reference  string `json:"reference,omitempty"`
	Inferred   bool   `json:"inferred,omitempty"`
}

type ReplayPoint struct {
	TimeMs         int64    `json:"timeMs"`
	GMTTimeMs      *int64   `json:"gmtTimeMs,omitempty"`
	Latitude       float64  `json:"latitude"`
	Longitude      float64  `json:"longitude"`
	Altitude       *float64 `json:"altitude,omitempty"`
	AltitudeMeters *float64 `json:"altitudeMeters,omitempty"`
	GroundSpeed    *float64 `json:"groundSpeed,omitempty"`
	Airspeed       *float64 `json:"airspeed,omitempty"`
	Heading        *float64 `json:"heading,omitempty"`
	VerticalSpeed  *float64 `json:"verticalSpeed,omitempty"`
	Pitch          *float64 `json:"pitch,omitempty"`
	Roll           *float64 `json:"roll,omitempty"`
	Phase          string   `json:"phase,omitempty"`
	Airborne       *bool    `json:"airborne,omitempty"`
	Segment        int      `json:"segment"`
	SourceRow      int      `json:"sourceRow"`
}

type ReplayResult struct {
	Supported          bool                         `json:"supported"`
	UnsupportedReason  string                       `json:"unsupportedReason,omitempty"`
	TimingSource       string                       `json:"timingSource,omitempty"`
	StartTimeMs        int64                        `json:"startTimeMs"`
	EndTimeMs          int64                        `json:"endTimeMs"`
	DurationMs         int64                        `json:"durationMs"`
	SourceRowCount     int                          `json:"sourceRowCount"`
	ValidPositionCount int                          `json:"validPositionCount"`
	PointCount         int                          `json:"pointCount"`
	Downsampled        bool                         `json:"downsampled"`
	Capabilities       ReplayCapabilities           `json:"capabilities"`
	Mappings           map[string]string            `json:"mappings"`
	Measurements       map[string]ReplayMeasurement `json:"measurements"`
	HeadingSource      string                       `json:"headingSource,omitempty"`
	Bounds             *ReplayBounds                `json:"bounds,omitempty"`
	Points             []ReplayPoint                `json:"points"`
	Diagnostics        []Diagnostic                 `json:"diagnostics"`
}

// NearestReplayPoint finds the position sample closest to a flight-relative
// timestamp. Callers provide an explicit tolerance so gaps in recorded
// position data are never presented as precise event locations.
func NearestReplayPoint(points []ReplayPoint, targetTimeMs, toleranceMs int64) (ReplayPoint, int64, bool) {
	if len(points) == 0 || toleranceMs < 0 {
		return ReplayPoint{}, 0, false
	}

	index := sort.Search(len(points), func(index int) bool {
		return points[index].TimeMs >= targetTimeMs
	})
	candidates := make([]int, 0, 2)
	if index < len(points) {
		candidates = append(candidates, index)
	}
	if index > 0 {
		candidates = append(candidates, index-1)
	}

	bestDelta := int64(math.MaxInt64)
	bestIndex := -1
	for _, candidate := range candidates {
		delta := points[candidate].TimeMs - targetTimeMs
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			bestDelta = delta
			bestIndex = candidate
		}
	}
	if bestIndex < 0 || bestDelta > toleranceMs {
		return ReplayPoint{}, bestDelta, false
	}
	return points[bestIndex], bestDelta, true
}

var replayAliases = map[string][]string{
	"latitude":      {"LATITUDE", "LAT", "GPSLATITUDE", "GPSLAT", "LATDEG", "LATITUDEDEG", "GPSLATITUDEDEG", "POSITIONLATITUDE", "POSLAT"},
	"longitude":     {"LONGITUDE", "LON", "LONG", "LNG", "GPSLONGITUDE", "GPSLON", "LONDEG", "LONGITUDEDEG", "GPSLONGITUDEDEG", "POSITIONLONGITUDE", "POSLON"},
	"altitude":      {"ALTMSL", "ALTITUDEMSL", "ALTITUDEAVG", "ALTB", "ALTGPS", "GPSALT", "GPSALTITUDE", "BAROALTITUDE", "PRESSUREALTITUDE", "ALTITUDEAGL", "ALTITUDEFT", "ALTITUDEM", "ALTITUDEMETERS", "ALTITUDEMETRES", "ALTITUDE"},
	"groundSpeed":   {"GNDSPD", "GROUNDSPEED", "GROUNDSPEEDKTS", "GS", "GSKTS", "GPSGROUNDSPEED", "GROUNDSPEEDKMH", "GROUNDSPEEDKPH", "GROUNDSPEEDMPS"},
	"airspeed":      {"IAS", "AIRSPEEDAVG", "AIRSPEED", "INDICATEDAIRSPEED", "CALIBRATEDAIRSPEED", "CAS", "TRUEAIRSPEED", "TAS", "AIRSPEEDKMH", "AIRSPEEDKPH", "AIRSPEEDMPS"},
	"heading":       {"TRK", "TRACK", "GPSTRACK", "COURSE", "HDG", "HEADING", "MAGNETICHEADING", "TRUEHEADING"},
	"verticalSpeed": {"VERTICALSPEEDSMOOTH", "VSPD", "VERTICALSPEED", "RATEOFCLIMB", "VERTICALRATE", "VSI", "ROC", "VERTICALSPEEDMPS", "VERTICALSPEEDFPM"},
	"pitch":         {"PITCH", "PITCHANGLE"},
	"roll":          {"ROLL", "ROLLANGLE", "BANKANGLE"},
	"phase":         {"PHASE", "FLIGHTPHASE", "FLIGHTSTAGE", "DETECTEDPHASE"},
	"airborne":      {"ISAIRBORNE", "AIRBORNE", "INFLIGHT"},
}

func BuildReplay(path string, options ReplayOptions) (ReplayResult, error) {
	result := ReplayResult{
		Mappings:     map[string]string{},
		Measurements: map[string]ReplayMeasurement{},
		Points:       []ReplayPoint{},
		Diagnostics:  []Diagnostic{},
	}
	headers, rows, parseDiagnostics, err := readCSV(path)
	if err != nil {
		return result, err
	}
	rows, err = rowsInRange(rows, options.StartRow, options.EndRow)
	if err != nil {
		return result, err
	}
	result.SourceRowCount = len(rows)
	diagnostics := diagnosticCollector{}
	for _, item := range parseDiagnostics {
		diagnostics.add(item.Code, item.Severity, item.Message, "")
	}
	for name, aliases := range replayAliases {
		if original, ok := resolveReplayHeader(headers, aliases); ok {
			result.Mappings[name] = original
		}
	}
	keys := make(map[string]string, len(replayAliases))
	for name, aliases := range replayAliases {
		keys[name] = replayKey(headers, aliases)
	}
	result.Measurements = replayMeasurements(keys, result.Mappings)
	latitudeKey := keys["latitude"]
	longitudeKey := keys["longitude"]
	if latitudeKey == "" || longitudeKey == "" {
		result.UnsupportedReason = "Latitude and longitude columns were not found"
		diagnostics.add("REPLAY_POSITION_COLUMNS_MISSING", "error", result.UnsupportedReason, "")
		result.Diagnostics = diagnostics.list()
		return result, nil
	}
	frames, timingSource, timingDiagnostics, timingErr := timestampRows(rows, options.SampleIntervalMs)
	for _, item := range timingDiagnostics {
		diagnostics.add(item.Code, item.Severity, item.Message, "")
	}
	if timingErr != nil {
		result.UnsupportedReason = timingErr.Error()
		diagnostics.add("REPLAY_TIMING_UNAVAILABLE", "error", result.UnsupportedReason, "")
		result.Diagnostics = diagnostics.list()
		return result, nil
	}
	if options.RebaseTime {
		rebaseFrameTimes(frames)
	}
	result.TimingSource = timingSource
	result.Capabilities.Timing = true

	if altitude, ok := result.Measurements["altitude"]; ok && altitude.SourceUnit == "" {
		diagnostics.add("REPLAY_ALTITUDE_UNIT_UNKNOWN", "warning", "Altitude data is available, but its unit could not be established; values are shown in recorded units", altitude.Source)
	} else if ok && altitude.Inferred {
		diagnostics.add("REPLAY_ALTITUDE_UNIT_INFERRED", "warning", "Altitude units were inferred from the aviation parameter name; verify the source definition when precise vertical placement is required", altitude.Source)
	}
	points := make([]ReplayPoint, 0, len(frames))
	segment := 0
	var prior *ReplayPoint
	for _, current := range frames {
		latitude, latOK := numericValue(current.values, latitudeKey)
		longitude, lonOK := numericValue(current.values, longitudeKey)
		if !latOK || !lonOK || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 || (latitude == 0 && longitude == 0) {
			diagnostics.add("REPLAY_POSITION_INVALID", "warning", "A row was skipped because its coordinates are missing or outside valid decimal-degree ranges", "")
			prior = nil
			continue
		}
		altitude, altitudeMeters := normalizedAltitude(current.values, keys["altitude"], result.Measurements["altitude"])
		point := ReplayPoint{
			TimeMs: current.timeMs, GMTTimeMs: recordedGMTMillis(current.values), Latitude: latitude, Longitude: longitude,
			Altitude: altitude, AltitudeMeters: altitudeMeters,
			GroundSpeed:   normalizedSpeed(current.values, keys["groundSpeed"], result.Measurements["groundSpeed"]),
			Airspeed:      normalizedSpeed(current.values, keys["airspeed"], result.Measurements["airspeed"]),
			Heading:       normalizedAngle(current.values, keys["heading"], result.Measurements["heading"], true),
			VerticalSpeed: normalizedVerticalSpeed(current.values, keys["verticalSpeed"], result.Measurements["verticalSpeed"]),
			Pitch:         normalizedAngle(current.values, keys["pitch"], result.Measurements["pitch"], false),
			Roll:          normalizedAngle(current.values, keys["roll"], result.Measurements["roll"], false),
			Phase:         strings.ToUpper(strings.TrimSpace(firstValue(current.values, keys["phase"]))), Airborne: optionalBool(current.values, keys["airborne"]),
			Segment: segment, SourceRow: current.row,
		}
		if prior != nil {
			deltaMs := point.TimeMs - prior.TimeMs
			distance := haversineMeters(prior.Latitude, prior.Longitude, point.Latitude, point.Longitude)
			implausible := deltaMs <= 0 || deltaMs > 120000
			if deltaMs > 0 {
				knots := distance / (float64(deltaMs) / 1000) * 1.9438444924
				implausible = implausible || knots > 700
			}
			if implausible {
				segment++
				point.Segment = segment
				diagnostics.add("REPLAY_TRACK_SEGMENT_BREAK", "warning", "The route was split at a timing gap or implausible coordinate jump", "")
			}
		} else if len(points) > 0 {
			segment++
			point.Segment = segment
		}
		points = append(points, point)
		prior = &points[len(points)-1]
	}
	result.ValidPositionCount = len(points)
	if len(points) < 2 {
		result.UnsupportedReason = "Fewer than two valid coordinate samples are available"
		diagnostics.add("REPLAY_POSITION_INSUFFICIENT", "error", result.UnsupportedReason, "")
		result.Diagnostics = diagnostics.list()
		return result, nil
	}
	result.Supported = true
	result.HeadingSource = deriveReplayHeadings(points)
	if result.HeadingSource == "derived" {
		result.Measurements["heading"] = ReplayMeasurement{Source: "derived GPS track", SourceUnit: "deg", Unit: "deg", Inferred: true}
	} else if result.HeadingSource == "mixed" {
		measurement := result.Measurements["heading"]
		measurement.Inferred = true
		result.Measurements["heading"] = measurement
	}
	result.Capabilities.Position = true
	result.StartTimeMs = points[0].TimeMs
	result.EndTimeMs = points[len(points)-1].TimeMs
	result.DurationMs = result.EndTimeMs - result.StartTimeMs
	result.Bounds = replayBounds(points)
	result.Capabilities = replayCapabilities(points, result.Mappings)
	result.Capabilities.Position = true
	result.Capabilities.Timing = true
	maxPoints := options.MaxPoints
	if maxPoints <= 0 {
		maxPoints = 10000
	}
	result.Downsampled = len(points) > maxPoints
	result.Points = downsampleReplay(points, maxPoints)
	result.PointCount = len(result.Points)
	result.Diagnostics = diagnostics.list()
	return result, nil
}

func resolveReplayHeader(headers map[string]string, aliases []string) (string, bool) {
	for _, alias := range aliases {
		if original, ok := headers[alias]; ok {
			return original, true
		}
	}
	return "", false
}

func replayKey(headers map[string]string, aliases []string) string {
	for _, alias := range aliases {
		if _, ok := headers[alias]; ok {
			return alias
		}
	}
	return ""
}

func numericValue(values map[string]string, key string) (float64, bool) {
	if key == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(values[key]), 64)
	return value, err == nil && isFinite(value)
}

func optionalNumeric(values map[string]string, key string) *float64 {
	value, ok := numericValue(values, key)
	if !ok {
		return nil
	}
	return &value
}

func replayMeasurements(keys, mappings map[string]string) map[string]ReplayMeasurement {
	measurements := make(map[string]ReplayMeasurement)
	for name, key := range keys {
		if key == "" {
			continue
		}
		source := mappings[name]
		measurement := ReplayMeasurement{Source: source}
		switch name {
		case "altitude":
			measurement.SourceUnit, measurement.Inferred = altitudeSourceUnit(key, source)
			if measurement.SourceUnit != "" {
				measurement.Unit = "ft"
			}
			measurement.Reference = altitudeReference(key)
		case "groundSpeed", "airspeed":
			measurement.SourceUnit, measurement.Inferred = speedSourceUnit(key, source)
			if measurement.SourceUnit != "" {
				measurement.Unit = "kt"
			}
		case "verticalSpeed":
			measurement.SourceUnit, measurement.Inferred = verticalSpeedSourceUnit(key, source)
			if measurement.SourceUnit != "" {
				measurement.Unit = "ft/min"
			}
		case "latitude", "longitude", "heading", "pitch", "roll":
			measurement.SourceUnit, measurement.Inferred = angleSourceUnit(source)
			measurement.Unit = "deg"
		}
		measurements[name] = measurement
	}
	return measurements
}

func altitudeSourceUnit(key, source string) (string, bool) {
	lower := strings.ToLower(source)
	canonicalSource := canonical(source)
	if strings.Contains(lower, "(m)") || strings.Contains(lower, "[m]") || strings.Contains(lower, " metres") || strings.Contains(lower, " meters") || strings.HasSuffix(canonicalSource, "METERS") || strings.HasSuffix(canonicalSource, "METRES") || key == "ALTITUDEM" {
		return "m", false
	}
	if strings.Contains(lower, "(ft)") || strings.Contains(lower, "[ft]") || strings.Contains(lower, " feet") || strings.HasSuffix(canonicalSource, "FT") {
		return "ft", false
	}
	knownFeet := map[string]bool{
		"ALTMSL": true, "ALTITUDEMSL": true, "ALTITUDEAVG": true, "ALTB": true,
		"ALTGPS": true, "GPSALT": true, "GPSALTITUDE": true, "BAROALTITUDE": true,
		"PRESSUREALTITUDE": true, "ALTITUDEAGL": true,
	}
	if knownFeet[key] {
		return "ft", true
	}
	return "", false
}

func altitudeReference(key string) string {
	switch {
	case strings.Contains(key, "AGL"):
		return "AGL"
	case strings.Contains(key, "PRESSURE"):
		return "pressure"
	case strings.Contains(key, "MSL"), strings.Contains(key, "GPS"), key == "ALTB", key == "BAROALTITUDE", key == "ALTITUDEAVG":
		return "MSL"
	default:
		return "unknown"
	}
}

func speedSourceUnit(key, source string) (string, bool) {
	value := canonical(source)
	switch {
	case strings.Contains(value, "KMH"), strings.Contains(value, "KPH"):
		return "km/h", false
	case strings.Contains(value, "MPS"), strings.Contains(value, "MSEC"):
		return "m/s", false
	case strings.Contains(value, "MPH"):
		return "mph", false
	case strings.Contains(value, "KNOT"), strings.Contains(value, "KTS"):
		return "kt", false
	}
	knownKnots := map[string]bool{
		"GNDSPD": true, "GROUNDSPEED": true, "GS": true, "GPSGROUNDSPEED": true,
		"IAS": true, "AIRSPEEDAVG": true, "AIRSPEED": true, "INDICATEDAIRSPEED": true,
		"CALIBRATEDAIRSPEED": true, "CAS": true, "TRUEAIRSPEED": true, "TAS": true,
	}
	if knownKnots[key] {
		return "kt", true
	}
	return "", false
}

func verticalSpeedSourceUnit(key, source string) (string, bool) {
	value := canonical(source)
	switch {
	case strings.Contains(value, "MPS"), strings.Contains(value, "MSEC"):
		return "m/s", false
	case strings.Contains(value, "FPS"), strings.Contains(value, "FTSEC"):
		return "ft/s", false
	case strings.Contains(value, "FPM"), strings.Contains(value, "FTMIN"):
		return "ft/min", false
	}
	knownFeetPerMinute := map[string]bool{
		"VERTICALSPEEDSMOOTH": true, "VSPD": true, "VERTICALSPEED": true,
		"RATEOFCLIMB": true, "VERTICALRATE": true, "VSI": true, "ROC": true,
	}
	if knownFeetPerMinute[key] {
		return "ft/min", true
	}
	return "", false
}

func angleSourceUnit(source string) (string, bool) {
	value := canonical(source)
	if strings.Contains(value, "RAD") {
		return "rad", false
	}
	if strings.Contains(value, "DEG") {
		return "deg", false
	}
	return "deg", true
}

func normalizedAltitude(values map[string]string, key string, measurement ReplayMeasurement) (*float64, *float64) {
	raw := optionalNumeric(values, key)
	if raw == nil {
		return nil, nil
	}
	value := *raw
	switch measurement.SourceUnit {
	case "ft":
		meters := value * 0.3048
		return floatPointer(value), floatPointer(meters)
	case "m":
		feet := value / 0.3048
		return floatPointer(feet), floatPointer(value)
	default:
		return raw, nil
	}
}

func recordedGMTMillis(values map[string]string) *int64 {
	hour, hourErr := strconv.Atoi(strings.TrimSpace(values["GMTHOURS"]))
	minute, minuteErr := strconv.Atoi(strings.TrimSpace(values["GMTMINUTES"]))
	second, secondErr := strconv.ParseFloat(strings.TrimSpace(values["GMTSECONDS"]), 64)
	if hourErr != nil || minuteErr != nil || secondErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second >= 60 || !isFinite(second) {
		return nil
	}

	milliseconds := int64(math.Round((float64(hour*3600+minute*60) + second) * 1000))
	return &milliseconds
}

func normalizedSpeed(values map[string]string, key string, measurement ReplayMeasurement) *float64 {
	raw := optionalNumeric(values, key)
	if raw == nil {
		return nil
	}
	value := *raw
	switch measurement.SourceUnit {
	case "km/h":
		value *= 0.539956803
	case "m/s":
		value *= 1.9438444924
	case "mph":
		value *= 0.868976242
	}
	return floatPointer(value)
}

func normalizedVerticalSpeed(values map[string]string, key string, measurement ReplayMeasurement) *float64 {
	raw := optionalNumeric(values, key)
	if raw == nil {
		return nil
	}
	value := *raw
	switch measurement.SourceUnit {
	case "m/s":
		value *= 196.8503937
	case "ft/s":
		value *= 60
	}
	return floatPointer(value)
}

func normalizedAngle(values map[string]string, key string, measurement ReplayMeasurement, heading bool) *float64 {
	raw := optionalNumeric(values, key)
	if raw == nil {
		return nil
	}
	value := *raw
	if measurement.SourceUnit == "rad" {
		value *= 180 / math.Pi
	}
	if heading {
		value = math.Mod(value, 360)
		if value < 0 {
			value += 360
		}
	} else {
		value = math.Mod(value+180, 360)
		if value < 0 {
			value += 360
		}
		value -= 180
	}
	return floatPointer(value)
}

func floatPointer(value float64) *float64 {
	return &value
}

func deriveReplayHeadings(points []ReplayPoint) string {
	recorded := 0
	derived := 0
	for i := range points {
		if points[i].Heading != nil {
			recorded++
			continue
		}
		if heading, ok := neighboringTrack(points, i); ok {
			points[i].Heading = floatPointer(heading)
			derived++
		}
	}
	switch {
	case recorded > 0 && derived > 0:
		return "mixed"
	case recorded > 0:
		return "recorded"
	case derived > 0:
		return "derived"
	default:
		return "unavailable"
	}
}

func neighboringTrack(points []ReplayPoint, index int) (float64, bool) {
	current := points[index]
	for next := index + 1; next < len(points) && points[next].Segment == current.Segment; next++ {
		if haversineMeters(current.Latitude, current.Longitude, points[next].Latitude, points[next].Longitude) > 0.5 {
			return initialBearingDegrees(current.Latitude, current.Longitude, points[next].Latitude, points[next].Longitude), true
		}
	}
	for prior := index - 1; prior >= 0 && points[prior].Segment == current.Segment; prior-- {
		if haversineMeters(points[prior].Latitude, points[prior].Longitude, current.Latitude, current.Longitude) > 0.5 {
			return initialBearingDegrees(points[prior].Latitude, points[prior].Longitude, current.Latitude, current.Longitude), true
		}
	}
	return 0, false
}

func initialBearingDegrees(lat1, lon1, lat2, lon2 float64) float64 {
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	deltaLambda := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(deltaLambda) * math.Cos(phi2)
	x := math.Cos(phi1)*math.Sin(phi2) - math.Sin(phi1)*math.Cos(phi2)*math.Cos(deltaLambda)
	bearing := math.Atan2(y, x) * 180 / math.Pi
	if bearing < 0 {
		bearing += 360
	}
	return bearing
}

func optionalBool(values map[string]string, key string) *bool {
	if key == "" {
		return nil
	}
	value := strings.ToLower(strings.TrimSpace(values[key]))
	var parsed bool
	switch value {
	case "true", "1", "yes", "on", "airborne":
		parsed = true
	case "false", "0", "no", "off", "ground":
		parsed = false
	default:
		return nil
	}
	return &parsed
}

func replayCapabilities(points []ReplayPoint, mappings map[string]string) ReplayCapabilities {
	capabilities := ReplayCapabilities{Phase: mappings["phase"] != ""}
	for _, point := range points {
		capabilities.Altitude = capabilities.Altitude || point.Altitude != nil
		capabilities.GroundSpeed = capabilities.GroundSpeed || point.GroundSpeed != nil
		capabilities.Airspeed = capabilities.Airspeed || point.Airspeed != nil
		capabilities.Heading = capabilities.Heading || point.Heading != nil
		capabilities.VerticalSpeed = capabilities.VerticalSpeed || point.VerticalSpeed != nil
		capabilities.Pitch = capabilities.Pitch || point.Pitch != nil
		capabilities.Roll = capabilities.Roll || point.Roll != nil
		capabilities.Airborne = capabilities.Airborne || point.Airborne != nil
	}
	return capabilities
}

func replayBounds(points []ReplayPoint) *ReplayBounds {
	bounds := &ReplayBounds{West: points[0].Longitude, East: points[0].Longitude, South: points[0].Latitude, North: points[0].Latitude}
	for _, point := range points[1:] {
		bounds.West = math.Min(bounds.West, point.Longitude)
		bounds.East = math.Max(bounds.East, point.Longitude)
		bounds.South = math.Min(bounds.South, point.Latitude)
		bounds.North = math.Max(bounds.North, point.Latitude)
	}
	return bounds
}

func downsampleReplay(points []ReplayPoint, maxPoints int) []ReplayPoint {
	if len(points) <= maxPoints || maxPoints < 2 {
		return points
	}
	keep := map[int]bool{0: true, len(points) - 1: true}
	stride := int(math.Ceil(float64(len(points)) / float64(maxPoints)))
	for index := 0; index < len(points); index += stride {
		keep[index] = true
	}
	for index := 1; index < len(points); index++ {
		if points[index].Phase != points[index-1].Phase || points[index].Segment != points[index-1].Segment {
			keep[index-1], keep[index] = true, true
		}
	}
	indices := make([]int, 0, len(keep))
	for index := range keep {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	result := make([]ReplayPoint, 0, len(indices))
	for _, index := range indices {
		result = append(result, points[index])
	}
	return result
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	deltaPhi, deltaLambda := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	if a < 0 || math.IsNaN(a) {
		return 0
	}
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}
