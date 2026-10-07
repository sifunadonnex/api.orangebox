package handlers

import "testing"

func float64Pointer(value float64) *float64 {
	return &value
}

func TestNormalizeReviewedFlightDetails(t *testing.T) {
	item := &reviewedFlightInput{
		FlightDate:    "2026-06-05",
		FlightNumber:  "  kq 100 ",
		Departure:     " hkyw ",
		Destination:   " hkmo ",
		Pilot:         " pic-17 ",
		FOCrewCode:    " fo-42 ",
		TakeoffWeight: float64Pointer(18_450),
		VRef:          float64Pointer(126),
	}

	if err := normalizeReviewedFlightDetails(item); err != nil {
		t.Fatalf("normalize flight details: %v", err)
	}
	if item.FlightNumber != "KQ 100" || item.Departure != "HKYW" || item.Destination != "HKMO" {
		t.Fatalf("expected normalized flight identifiers, got %#v", item)
	}
	if item.PICCrewCode != "PIC-17" || item.Pilot != "PIC-17" || item.FOCrewCode != "FO-42" {
		t.Fatalf("expected normalized crew codes, got PIC=%q pilot=%q FO=%q", item.PICCrewCode, item.Pilot, item.FOCrewCode)
	}
	if item.WeightUnit != "kg" {
		t.Fatalf("expected kg default for supplied weights, got %q", item.WeightUnit)
	}
}

func TestNormalizeReviewedFlightDetailsRejectsInvalidICAO(t *testing.T) {
	item := &reviewedFlightInput{Departure: "NBO"}

	if err := normalizeReviewedFlightDetails(item); err == nil {
		t.Fatal("expected invalid ICAO code to be rejected")
	}
}

func TestNormalizeReviewedFlightDetailsRejectsInvalidReferenceSpeed(t *testing.T) {
	item := &reviewedFlightInput{VApp: float64Pointer(1001)}

	if err := normalizeReviewedFlightDetails(item); err == nil {
		t.Fatal("expected out-of-range reference speed to be rejected")
	}
}
