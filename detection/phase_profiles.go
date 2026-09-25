package detection

import "strings"

// PhaseProfile contains the aircraft-specific thresholds used by the native
// phase detector. Values use knots, feet, and feet per minute.
type PhaseProfile struct {
	Code                     string  `json:"code"`
	Name                     string  `json:"name"`
	TaxiSpeed                float64 `json:"taxiSpeed"`
	TakeoffSpeed             float64 `json:"takeoffSpeed"`
	RotationSpeed            float64 `json:"rotationSpeed"`
	ClimbRate                float64 `json:"climbRate"`
	DescentRate              float64 `json:"descentRate"`
	CruiseVerticalSpeed      float64 `json:"cruiseVerticalSpeed"`
	ApproachAltitude         float64 `json:"approachAltitude"`
	InitialClimbAltitude     float64 `json:"initialClimbAltitude"`
	LandingAltitude          float64 `json:"landingAltitude"`
	InferredAirborneSpeed    float64 `json:"inferredAirborneSpeed"`
	InferredAirborneRate     float64 `json:"inferredAirborneRate"`
	InferredAirborneAltitude float64 `json:"inferredAirborneAltitude"`
}

var phaseProfiles = map[string]PhaseProfile{
	"DASH8_300":         phaseProfile("DASH8_300", "De Havilland Canada Dash 8-300", 30, 70, 90, 500, -300, 200, 3000, 1500, 500),
	"Q400":              phaseProfile("Q400", "Bombardier Q400", 30, 80, 100, 500, -300, 200, 3000, 1500, 500),
	"ATR72":             phaseProfile("ATR72", "ATR 72", 25, 75, 95, 500, -300, 200, 3000, 1500, 500),
	"CARAVAN":           phaseProfile("CARAVAN", "Cessna 208 Caravan", 20, 55, 70, 300, -300, 150, 2500, 1200, 300),
	"B737":              phaseProfile("B737", "Boeing 737", 30, 100, 140, 800, -500, 200, 3000, 1500, 500),
	"A320":              phaseProfile("A320", "Airbus A320", 30, 95, 135, 800, -500, 200, 3000, 1500, 500),
	"B767":              phaseProfile("B767", "Boeing 767", 30, 110, 150, 900, -700, 250, 3000, 1500, 500),
	"B777":              phaseProfile("B777", "Boeing 777", 35, 120, 160, 1000, -800, 250, 3000, 1500, 500),
	"GENERIC_TURBOPROP": phaseProfile("GENERIC_TURBOPROP", "Generic turboprop", 30, 80, 100, 500, -300, 200, 3000, 1500, 500),
	"GENERIC_JET":       phaseProfile("GENERIC_JET", "Generic jet", 30, 100, 140, 800, -500, 200, 3000, 1500, 500),
}

func phaseProfile(code, name string, taxi, takeoff, rotation, climb, descent, cruise, approach, initialClimb, landing float64) PhaseProfile {
	return PhaseProfile{
		Code: code, Name: name, TaxiSpeed: taxi, TakeoffSpeed: takeoff,
		RotationSpeed: rotation, ClimbRate: climb, DescentRate: descent,
		CruiseVerticalSpeed: cruise, ApproachAltitude: approach,
		InitialClimbAltitude: initialClimb, LandingAltitude: landing,
		InferredAirborneSpeed: 45, InferredAirborneRate: 300,
		InferredAirborneAltitude: 150,
	}
}

// PhaseProfileByCode returns a copy of a supported profile. Unknown values use
// the generic turboprop profile and report false.
func PhaseProfileByCode(code string) (PhaseProfile, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	profile, ok := phaseProfiles[normalized]
	if ok {
		return profile, true
	}
	return phaseProfiles["GENERIC_TURBOPROP"], false
}

// InferPhaseProfile maps the aircraft metadata already stored by the API to a
// detector profile. The result can be overridden explicitly in PhaseOptions.
func InferPhaseProfile(aircraftMake, modelNumber string) string {
	value := strings.ToUpper(strings.TrimSpace(aircraftMake + " " + modelNumber))
	switch {
	case strings.Contains(value, "CARAVAN"), strings.Contains(value, "CESSNA 208"), strings.Contains(value, "C208"):
		return "CARAVAN"
	case strings.Contains(value, "DASH 8-300"), strings.Contains(value, "DASH8-300"), strings.Contains(value, "DH8C"):
		return "DASH8_300"
	case strings.Contains(value, "Q400"), strings.Contains(value, "DASH 8-400"), strings.Contains(value, "DH8D"):
		return "Q400"
	case strings.Contains(value, "ATR 72"), strings.Contains(value, "ATR72"):
		return "ATR72"
	case strings.Contains(value, "737"):
		return "B737"
	case strings.Contains(value, "A320"):
		return "A320"
	case strings.Contains(value, "767"):
		return "B767"
	case strings.Contains(value, "777"):
		return "B777"
	case strings.Contains(value, "AIRBUS"), strings.Contains(value, "BOEING"), strings.Contains(value, "JET"):
		return "GENERIC_JET"
	default:
		return "GENERIC_TURBOPROP"
	}
}
