package detection

import (
	"math/rand"
	"testing"
)

func referenceReplayHeadings(points []ReplayPoint) string {
	recorded, derived := 0, 0
	for index := range points {
		if points[index].Heading != nil {
			recorded++
			continue
		}
		if heading, found := neighboringTrack(points, index); found {
			points[index].Heading = floatPointer(heading)
			derived++
		}
	}
	if recorded > 0 && derived > 0 {
		return "mixed"
	}
	if recorded > 0 {
		return "recorded"
	}
	if derived > 0 {
		return "derived"
	}
	return "unavailable"
}

func TestReplayHeadingRunsMatchOriginalSearch(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for trial := 0; trial < 60; trial++ {
		points := make([]ReplayPoint, 300)
		latitude, longitude, segment := -1.3, 36.8, 0
		for index := range points {
			if random.Intn(40) == 0 {
				segment++
			}
			switch random.Intn(6) {
			case 0:
				latitude += 0.0001
				longitude += 0.0001
			case 1:
				latitude += 0.0000001 // Below the heading movement threshold.
			}
			points[index] = ReplayPoint{Latitude: latitude, Longitude: longitude, Segment: segment}
			if random.Intn(8) == 0 {
				points[index].Heading = floatPointer(float64(random.Intn(360)))
			}
		}
		want, got := append([]ReplayPoint(nil), points...), append([]ReplayPoint(nil), points...)
		wantSource, gotSource := referenceReplayHeadings(want), deriveReplayHeadings(got)
		if wantSource != gotSource {
			t.Fatalf("heading source changed: %s vs %s", wantSource, gotSource)
		}
		for index := range want {
			left, right := want[index].Heading, got[index].Heading
			if (left == nil) != (right == nil) || (left != nil && *left != *right) {
				t.Fatalf("trial %d sample %d changed heading: %v vs %v", trial, index, left, right)
			}
		}
	}
}

func TestReplayHeadingLongStationaryRunPreservesMissingAndRecordedValues(t *testing.T) {
	points := make([]ReplayPoint, 10000)
	for index := range points {
		points[index] = ReplayPoint{Latitude: -1.3, Longitude: 36.8}
	}
	if source := deriveReplayHeadings(points); source != "unavailable" {
		t.Fatalf("invented stationary heading: %s", source)
	}
	points[500].Heading = floatPointer(123)
	if source := deriveReplayHeadings(points); source != "recorded" {
		t.Fatalf("recorded stationary heading lost: %s", source)
	}
	if *points[500].Heading != 123 || points[499].Heading != nil || points[501].Heading != nil {
		t.Fatal("recorded heading spread to missing samples")
	}
}
