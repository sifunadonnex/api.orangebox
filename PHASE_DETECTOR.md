# Native flight-phase detector

The `detection` package contains a Go-native flight-phase detector derived from
the behavior of the 5Y-SLQ Python analysis. It has no Python, pandas, NumPy, or
plotting dependency and is connected to the upload transaction.

## Run it

```powershell
go run ./cmd/phase-detect `
  -input "C:\path\to\flight.csv" `
  -profile CARAVAN
```

The command writes JSON to standard output. It reports:

- the detector and aircraft profile versions;
- detected flight legs and completeness (`COMPLETE`, `DEPARTURE_ONLY`,
  `ARRIVAL_ONLY`, or `AIRBORNE_FRAGMENT`);
- compact source-row phase runs rather than duplicating every telemetry row;
- timing source, inferred sample interval, and diagnostics.

Supported profiles are `CARAVAN`, `DASH8_300`, `Q400`, `ATR72`, `B737`,
`A320`, `B767`, `B777`, `GENERIC_TURBOPROP`, and `GENERIC_JET`. When no
profile is supplied, `-make` and `-model` are used to infer one.

## Use it from the API

```go
result, err := detection.DetectFlightPhases(path, detection.PhaseOptions{
    AircraftMake: "Cessna",
    ModelNumber:  "208B",
})
```

The reader handles Garmin metadata before the CSV header, malformed quotes in
metadata, alternate telemetry column names, sample resets, multiple legs in a
single recording, and recorded or inferred airborne state.

During `POST /api/csv`, successful results are stored as compact
`FlightPhaseRun` row ranges. They are used for reliable multi-flight boundaries,
supplied to exceedance rules when the source has no phase column, and overlaid
on replay points. Detection errors remain visible warnings while the
conservative recording segmenter provides a safe fallback.

## Tests

The normal unit suite uses generated fixtures:

```powershell
go test ./detection
```

To validate every non-empty CSV in the real Caravan fixture folder:

```powershell
$env:PHASE_FIXTURE_DIR = "C:\Users\sifun\Desktop\Caravan\5Y-SLQ\data_log"
go test ./detection -run TestDetectFlightPhasesRealCaravanFixture -v -count=1
```

The real-data test is skipped when `PHASE_FIXTURE_DIR` is not set, so the
repository test suite remains portable.
