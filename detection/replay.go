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

type ReplayPoint struct {
	TimeMs        int64    `json:"timeMs"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	Altitude      *float64 `json:"altitude,omitempty"`
	GroundSpeed   *float64 `json:"groundSpeed,omitempty"`
	Airspeed      *float64 `json:"airspeed,omitempty"`
	Heading       *float64 `json:"heading,omitempty"`
	VerticalSpeed *float64 `json:"verticalSpeed,omitempty"`
	Pitch         *float64 `json:"pitch,omitempty"`
	Roll          *float64 `json:"roll,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	Airborne      *bool    `json:"airborne,omitempty"`
	Segment       int      `json:"segment"`
	SourceRow     int      `json:"sourceRow"`
}

type ReplayResult struct {
	Supported          bool               `json:"supported"`
	UnsupportedReason  string             `json:"unsupportedReason,omitempty"`
	TimingSource       string             `json:"timingSource,omitempty"`
	StartTimeMs        int64              `json:"startTimeMs"`
	EndTimeMs          int64              `json:"endTimeMs"`
	DurationMs         int64              `json:"durationMs"`
	SourceRowCount     int                `json:"sourceRowCount"`
	ValidPositionCount int                `json:"validPositionCount"`
	PointCount         int                `json:"pointCount"`
	Downsampled        bool               `json:"downsampled"`
	Capabilities       ReplayCapabilities `json:"capabilities"`
	Mappings           map[string]string  `json:"mappings"`
	Bounds             *ReplayBounds      `json:"bounds,omitempty"`
	Points             []ReplayPoint      `json:"points"`
	Diagnostics        []Diagnostic       `json:"diagnostics"`
}

var replayAliases = map[string][]string{
	"latitude":      {"LATITUDE", "LAT", "GPSLATITUDE", "GPSLAT", "LATDEG", "LATITUDEDEG", "GPSLATITUDEDEG", "POSITIONLATITUDE", "POSLAT"},
	"longitude":     {"LONGITUDE", "LON", "LONG", "LNG", "GPSLONGITUDE", "GPSLON", "LONDEG", "LONGITUDEDEG", "GPSLONGITUDEDEG", "POSITIONLONGITUDE", "POSLON"},
	"altitude":      {"ALTMSL", "ALTITUDEMSL", "ALTITUDEAVG", "ALTITUDE", "ALTB", "ALTGPS", "GPSALT", "GPSALTITUDE", "BAROALTITUDE", "PRESSUREALTITUDE"},
	"groundSpeed":   {"GNDSPD", "GROUNDSPEED", "GROUNDSPEEDKTS", "GS", "GSKTS", "GPSGROUNDSPEED"},
	"airspeed":      {"IAS", "AIRSPEEDAVG", "AIRSPEED", "INDICATEDAIRSPEED", "CALIBRATEDAIRSPEED", "CAS", "TRUEAIRSPEED", "TAS"},
	"heading":       {"TRK", "TRACK", "GPSTRACK", "COURSE", "HDG", "HEADING", "MAGNETICHEADING", "TRUEHEADING"},
	"verticalSpeed": {"VERTICALSPEEDSMOOTH", "VSPD", "VERTICALSPEED", "RATEOFCLIMB", "VERTICALRATE", "VSI", "ROC"},
	"pitch":         {"PITCH", "PITCHANGLE"},
	"roll":          {"ROLL", "ROLLANGLE", "BANKANGLE"},
	"phase":         {"PHASE", "FLIGHTPHASE", "FLIGHTSTAGE", "DETECTEDPHASE"},
	"airborne":      {"ISAIRBORNE", "AIRBORNE", "INFLIGHT"},
}

func BuildReplay(path string, options ReplayOptions) (ReplayResult, error) {
	result := ReplayResult{
		Mappings:    map[string]string{},
		Points:      []ReplayPoint{},
		Diagnostics: []Diagnostic{},
	}
	headers, rows, parseDiagnostics, err := readCSV(path)
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
	latitudeKey := replayKey(headers, replayAliases["latitude"])
	longitudeKey := replayKey(headers, replayAliases["longitude"])
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
	result.TimingSource = timingSource
	result.Capabilities.Timing = true

	keys := make(map[string]string, len(replayAliases))
	for name, aliases := range replayAliases {
		keys[name] = replayKey(headers, aliases)
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
		point := ReplayPoint{
			TimeMs: current.timeMs, Latitude: latitude, Longitude: longitude,
			Altitude:      optionalNumeric(current.values, keys["altitude"]),
			GroundSpeed:   optionalNumeric(current.values, keys["groundSpeed"]),
			Airspeed:      optionalNumeric(current.values, keys["airspeed"]),
			Heading:       optionalNumeric(current.values, keys["heading"]),
			VerticalSpeed: optionalNumeric(current.values, keys["verticalSpeed"]),
			Pitch:         optionalNumeric(current.values, keys["pitch"]),
			Roll:          optionalNumeric(current.values, keys["roll"]),
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
