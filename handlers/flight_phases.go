package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fdm-backend/detection"
	"fdm-backend/models"

	"github.com/google/uuid"
)

type phaseDetectionResponse struct {
	Status string                          `json:"status"`
	Result *detection.PhaseDetectionResult `json:"result,omitempty"`
	Error  string                          `json:"error,omitempty"`
}

func detectUploadPhases(path string, aircraft models.Aircraft, sampleIntervalMs int64) (detection.PhaseDetectionResult, error) {
	modelNumber := ""
	if aircraft.ModelNumber != nil {
		modelNumber = *aircraft.ModelNumber
	}
	return detection.DetectFlightPhases(path, detection.PhaseOptions{
		AircraftMake: aircraft.AircraftMake, ModelNumber: modelNumber, SampleIntervalMs: sampleIntervalMs,
	})
}

func phaseSegments(path string, result detection.PhaseDetectionResult) ([]detection.RecordingSegment, error) {
	if len(result.Flights) == 0 {
		return nil, fmt.Errorf("phase detector found no airborne flight")
	}
	segments := make([]detection.RecordingSegment, 0, len(result.Flights))
	ranges := make([]detection.RowRange, 0, len(result.Flights))
	for _, flight := range result.Flights {
		segments = append(segments, detection.RecordingSegment{
			Index: flight.Index, StartRow: flight.StartRow, EndRow: flight.EndRow,
			StartSample: flight.StartSample, EndSample: flight.EndSample, BoundarySource: "phase_detector",
		})
		ranges = append(ranges, detection.RowRange{StartRow: flight.StartRow, EndRow: flight.EndRow})
	}
	if _, err := detection.ValidateRecordingRanges(path, ranges); err != nil {
		return nil, fmt.Errorf("phase-derived flight ranges do not cover the recording: %w", err)
	}
	return segments, nil
}

func persistPhaseDetection(tx *sql.Tx, recordingID string, result detection.PhaseDetectionResult, now time.Time) error {
	summary, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE Csv SET phaseEngineVersion = ?, phaseProfile = ?,
		phaseTimingSource = ?, phaseSummary = ?, updatedAt = ? WHERE id = ?`,
		result.EngineVersion, result.Profile.Code, result.TimingSource, string(summary), now.UnixMilli(), recordingID); err != nil {
		return err
	}
	for _, run := range result.Runs {
		if strings.TrimSpace(run.Phase) == "" {
			continue
		}
		if _, err = tx.Exec(`INSERT INTO FlightPhaseRun
			(id, recordingId, flightIndex, flightStatus, phase, startRow, endRow,
			 startSample, endSample, durationMs, createdAt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New().String(), recordingID,
			run.FlightIndex, nullableText(run.FlightStatus), strings.ToUpper(strings.TrimSpace(run.Phase)),
			run.StartRow, run.EndRow, nullableText(run.StartSample), nullableText(run.EndSample),
			run.DurationMs, now.UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func (h *CSVHandler) loadFlightPhaseRuns(recordingID string, startRow, endRow int) ([]detection.PhaseRun, error) {
	query := `SELECT flightIndex, COALESCE(flightStatus, ''), phase, startRow, endRow,
		COALESCE(startSample, ''), COALESCE(endSample, ''), durationMs
		FROM FlightPhaseRun WHERE recordingId = ?`
	args := []any{recordingID}
	if startRow > 0 {
		query += " AND endRow >= ?"
		args = append(args, startRow)
	}
	if endRow > 0 {
		query += " AND startRow <= ?"
		args = append(args, endRow)
	}
	query += " ORDER BY startRow, endRow"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]detection.PhaseRun, 0)
	for rows.Next() {
		var run detection.PhaseRun
		if err = rows.Scan(&run.FlightIndex, &run.FlightStatus, &run.Phase, &run.StartRow,
			&run.EndRow, &run.StartSample, &run.EndSample, &run.DurationMs); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
