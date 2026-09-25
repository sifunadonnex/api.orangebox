package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"fdm-backend/detection"
)

func main() {
	input := flag.String("input", "", "path to a telemetry CSV")
	profile := flag.String("profile", "", "aircraft profile (for example CARAVAN or Q400)")
	aircraftMake := flag.String("make", "", "aircraft manufacturer used for profile inference")
	model := flag.String("model", "", "aircraft model used for profile inference")
	interval := flag.Int64("sample-interval-ms", 0, "fallback sample interval when timestamps are unavailable")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		flag.Usage()
		os.Exit(2)
	}

	result, err := detection.DetectFlightPhases(*input, detection.PhaseOptions{
		AircraftProfile:  *profile,
		AircraftMake:     *aircraftMake,
		ModelNumber:      *model,
		SampleIntervalMs: *interval,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "phase detection failed: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "write result: %v\n", err)
		os.Exit(1)
	}
}
