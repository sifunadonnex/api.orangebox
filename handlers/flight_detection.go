package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"fdm-backend/detection"
	"fdm-backend/models"

	"github.com/google/uuid"
)

type flightAnalysisResponse struct {
	RunID               string                 `json:"runId"`
	Status              string                 `json:"status"`
	EngineVersion       string                 `json:"engineVersion"`
	TimingSource        string                 `json:"timingSource,omitempty"`
	SampleIntervalMs    int64                  `json:"sampleIntervalMs"`
	RowCount            int                    `json:"rowCount"`
	ApplicableRuleCount int                    `json:"applicableRuleCount"`
	EvaluatedRuleCount  int                    `json:"evaluatedRuleCount"`
	OccurrenceCount     int                    `json:"occurrenceCount"`
	Occurrences         []detection.Occurrence `json:"occurrences"`
	Diagnostics         []detection.Diagnostic `json:"diagnostics"`
	RuleSetHash         string                 `json:"ruleSetHash"`
	TriggerType         string                 `json:"triggerType"`
	Reused              bool                   `json:"reused"`
	Error               string                 `json:"error,omitempty"`
}

type flightAnalysisOptions struct {
	TriggerType        string
	TriggeredBy        string
	ReuseCompleted     bool
	UpdateFlightOnFail bool
}

func (h *CSVHandler) getDetectionAircraft(id string) (models.Aircraft, error) {
	var aircraft models.Aircraft
	err := h.db.QueryRow(`SELECT id, airline, aircraftMake, modelNumber, serialNumber,
		registration, companyId, parameters FROM Aircraft WHERE id = ?`, id).Scan(
		&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.Registration, &aircraft.CompanyID, &aircraft.Parameters,
	)
	return aircraft, err
}

func (h *CSVHandler) analyzeUploadedFlight(path, filename string, flight *models.CSV, aircraft models.Aircraft, triggeredBy string) flightAnalysisResponse {
	definitions, err := h.loadApplicableDefinitions(aircraft, "")
	if err != nil {
		response := newFlightAnalysisResponse(flight, "upload")
		return h.failFlightAnalysis(flight.ID, "", response, fmt.Errorf("load applicable definitions: %w", err), true)
	}
	return h.analyzeFlight(path, filename, flight, aircraft, definitions, flightAnalysisOptions{
		TriggerType: "upload", TriggeredBy: triggeredBy, UpdateFlightOnFail: true,
	})
}

func newFlightAnalysisResponse(flight *models.CSV, triggerType string) flightAnalysisResponse {
	return flightAnalysisResponse{
		Status: "processing", EngineVersion: detection.EngineVersion, TriggerType: triggerType,
		SampleIntervalMs: valueOrZeroInt64(flight.SampleIntervalMs),
		Occurrences:      []detection.Occurrence{}, Diagnostics: []detection.Diagnostic{},
	}
}

func (h *CSVHandler) analyzeFlight(path, filename string, flight *models.CSV, aircraft models.Aircraft, definitions []detection.Definition, options flightAnalysisOptions) flightAnalysisResponse {
	response := newFlightAnalysisResponse(flight, options.TriggerType)
	response.ApplicableRuleCount = len(definitions)
	response.RuleSetHash = hashRuleSet(definitions)
	inputHash, err := hashFile(path)
	if err != nil {
		return h.failFlightAnalysis(flight.ID, "", response, fmt.Errorf("hash stored CSV: %w", err), options.UpdateFlightOnFail)
	}
	if options.ReuseCompleted {
		if prior, found, loadErr := h.loadReusableDetectionRun(flight.ID, inputHash, response.SampleIntervalMs, response.RuleSetHash); loadErr != nil {
			return h.failFlightAnalysis(flight.ID, "", response, fmt.Errorf("inspect prior detection runs: %w", loadErr), options.UpdateFlightOnFail)
		} else if found {
			prior.Reused = true
			return prior
		}
	}

	runID := uuid.New().String()
	response.RunID = runID
	now := time.Now().UnixMilli()
	_, err = h.db.Exec(`INSERT INTO DetectionRun
		(id, flightId, aircraftId, engineVersion, status, inputHash, ruleSetHash,
		 triggerType, triggeredBy, sampleIntervalMs, applicableRuleCount, diagnosticsJson, startedAt)
		VALUES (?, ?, ?, ?, 'processing', ?, ?, ?, ?, ?, ?, '[]', ?)`,
		runID, flight.ID, aircraft.ID, detection.EngineVersion, inputHash, response.RuleSetHash,
		options.TriggerType, nullableText(options.TriggeredBy), response.SampleIntervalMs, len(definitions), now)
	if err != nil {
		return h.failFlightAnalysis(flight.ID, "", response, fmt.Errorf("start detection run: %w", err), options.UpdateFlightOnFail)
	}

	result, err := detection.AnalyzeFile(path, definitions, detection.Options{
		SampleIntervalMs: response.SampleIntervalMs, MaxEvidencePoints: 500,
	})
	if err != nil {
		return h.failFlightAnalysis(flight.ID, runID, response, err, options.UpdateFlightOnFail)
	}
	response.TimingSource = result.TimingSource
	response.RowCount = result.RowCount
	response.EvaluatedRuleCount = result.EvaluatedRuleCount
	response.OccurrenceCount = len(result.Occurrences)
	response.Occurrences = result.Occurrences
	response.Diagnostics = result.Diagnostics
	response.Status = "completed"
	if len(result.Diagnostics) > 0 {
		response.Status = "completed_with_warnings"
	}

	if err = h.persistDetectionResult(runID, filename, flight, aircraft, definitions, response); err != nil {
		return h.failFlightAnalysis(flight.ID, runID, response, fmt.Errorf("persist detection result: %w", err), options.UpdateFlightOnFail)
	}
	return response
}

func (h *CSVHandler) loadApplicableDefinitions(aircraft models.Aircraft, definitionID string) ([]detection.Definition, error) {
	modelNumber := ""
	if aircraft.ModelNumber != nil {
		modelNumber = *aircraft.ModelNumber
	}
	now := time.Now().UnixMilli()
	query := `SELECT DISTINCT d.id, v.id, d.eventCode, v.displayName,
		v.eventDescription, v.ruleHash, v.ruleJson
		FROM EventDefinition d
		JOIN EventDefinitionVersion v ON v.definitionId = d.id
		JOIN EventDefinitionAssignment a ON a.definitionVersionId = v.id
		WHERE d.lifecycleStatus = 'active' AND v.status = 'published'
		  AND (v.effectiveFrom IS NULL OR v.effectiveFrom <= ?)
		  AND (v.effectiveTo IS NULL OR v.effectiveTo > ?)
		  AND (d.companyId IS NULL OR d.companyId = ?)
		  AND (
		    (a.scopeType = 'company' AND a.companyId = ?)
		    OR (a.scopeType = 'aircraft' AND a.companyId = ? AND a.aircraftId = ?)
		    OR (a.scopeType = 'model' AND a.companyId = ? AND UPPER(TRIM(a.aircraftMake)) = UPPER(TRIM(?)) AND UPPER(TRIM(a.modelNumber)) = UPPER(TRIM(?)))
		  )
		  `
	args := []any{now, now, aircraft.CompanyID, aircraft.CompanyID, aircraft.CompanyID,
		aircraft.ID, aircraft.CompanyID, aircraft.AircraftMake, modelNumber}
	if strings.TrimSpace(definitionID) != "" {
		query += " AND d.id = ?"
		args = append(args, definitionID)
	}
	query += " ORDER BY d.eventCode, v.version"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	definitions := make([]detection.Definition, 0)
	for rows.Next() {
		var definition detection.Definition
		var ruleJSON string
		if err = rows.Scan(&definition.DefinitionID, &definition.VersionID, &definition.EventCode,
			&definition.DisplayName, &definition.Description, &definition.RuleHash, &ruleJSON); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(ruleJSON), &definition.Rule); err != nil {
			return nil, fmt.Errorf("definition version %s has invalid rule JSON: %w", definition.VersionID, err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, rows.Err()
}

func (h *CSVHandler) loadReusableDetectionRun(flightID, inputHash string, sampleIntervalMs int64, ruleSetHash string) (flightAnalysisResponse, bool, error) {
	var response flightAnalysisResponse
	var diagnosticsJSON string
	err := h.db.QueryRow(`SELECT id, status, engineVersion, COALESCE(timingSource, ''),
		COALESCE(sampleIntervalMs, 0), rowCount, applicableRuleCount, evaluatedRuleCount,
		occurrenceCount, diagnosticsJson, ruleSetHash, triggerType
		FROM DetectionRun
		WHERE flightId = ? AND engineVersion = ? AND inputHash = ?
		  AND COALESCE(sampleIntervalMs, 0) = ? AND ruleSetHash = ?
		  AND status IN ('completed', 'completed_with_warnings')
		ORDER BY completedAt DESC LIMIT 1`, flightID, detection.EngineVersion, inputHash,
		sampleIntervalMs, ruleSetHash).Scan(&response.RunID, &response.Status, &response.EngineVersion,
		&response.TimingSource, &response.SampleIntervalMs, &response.RowCount,
		&response.ApplicableRuleCount, &response.EvaluatedRuleCount, &response.OccurrenceCount,
		&diagnosticsJSON, &response.RuleSetHash, &response.TriggerType)
	if err == sql.ErrNoRows {
		return flightAnalysisResponse{}, false, nil
	}
	if err != nil {
		return flightAnalysisResponse{}, false, err
	}
	response.Occurrences = []detection.Occurrence{}
	response.Diagnostics = []detection.Diagnostic{}
	if err = json.Unmarshal([]byte(diagnosticsJSON), &response.Diagnostics); err != nil {
		return flightAnalysisResponse{}, false, fmt.Errorf("decode prior run diagnostics: %w", err)
	}
	rows, err := h.db.Query(`SELECT exceedanceValues FROM Exceedance
		WHERE detectionRunId = ? ORDER BY startTimeMs, id`, response.RunID)
	if err != nil {
		return flightAnalysisResponse{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var evidenceJSON string
		if err = rows.Scan(&evidenceJSON); err != nil {
			return flightAnalysisResponse{}, false, err
		}
		var evidence struct {
			detection.Occurrence
		}
		if err = json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
			return flightAnalysisResponse{}, false, fmt.Errorf("decode prior run occurrence: %w", err)
		}
		response.Occurrences = append(response.Occurrences, evidence.Occurrence)
	}
	if err = rows.Err(); err != nil {
		return flightAnalysisResponse{}, false, err
	}
	return response, true, nil
}

func (h *CSVHandler) persistDetectionResult(runID, filename string, flight *models.CSV, aircraft models.Aircraft, definitions []detection.Definition, response flightAnalysisResponse) error {
	diagnosticsJSON, err := json.Marshal(response.Diagnostics)
	if err != nil {
		return err
	}
	summaryJSON, err := json.Marshal(struct {
		RunID               string                 `json:"runId"`
		Status              string                 `json:"status"`
		EngineVersion       string                 `json:"engineVersion"`
		TimingSource        string                 `json:"timingSource"`
		RowCount            int                    `json:"rowCount"`
		ApplicableRuleCount int                    `json:"applicableRuleCount"`
		EvaluatedRuleCount  int                    `json:"evaluatedRuleCount"`
		OccurrenceCount     int                    `json:"occurrenceCount"`
		Diagnostics         []detection.Diagnostic `json:"diagnostics"`
		RuleSetHash         string                 `json:"ruleSetHash"`
		TriggerType         string                 `json:"triggerType"`
	}{runID, response.Status, response.EngineVersion, response.TimingSource, response.RowCount,
		response.ApplicableRuleCount, response.EvaluatedRuleCount, response.OccurrenceCount, response.Diagnostics,
		response.RuleSetHash, response.TriggerType})
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	occurrenceCounts := make(map[string]int, len(definitions))
	for _, occurrence := range response.Occurrences {
		occurrenceCounts[occurrence.VersionID]++
	}
	for _, definition := range definitions {
		definitionDiagnostics := diagnosticsForVersion(response.Diagnostics, definition.VersionID)
		diagnosticsJSON, marshalErr := json.Marshal(definitionDiagnostics)
		if marshalErr != nil {
			return marshalErr
		}
		status := "evaluated"
		isCurrent := 1
		if definitionWasSkipped(definitionDiagnostics) {
			status = "skipped"
			isCurrent = 0
		} else {
			if _, err = tx.Exec(`UPDATE DetectionRunDefinition SET isCurrent = 0, supersededAt = ?
				WHERE definitionId = ? AND isCurrent = 1 AND detectionRunId IN
				(SELECT id FROM DetectionRun WHERE flightId = ?)`, now, definition.DefinitionID, flight.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(`UPDATE Exceedance SET isCurrent = 0, supersededAt = ?, updatedAt = ?
				WHERE flightId = ? AND isCurrent = 1 AND eventId IN
				(SELECT id FROM EventDefinitionVersion WHERE definitionId = ?)`, now, now, flight.ID, definition.DefinitionID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT INTO DetectionRunDefinition
			(detectionRunId, definitionId, definitionVersionId, ruleHash, status,
			 occurrenceCount, diagnosticsJson, isCurrent, createdAt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, runID, definition.DefinitionID,
			definition.VersionID, definition.RuleHash, status, occurrenceCounts[definition.VersionID],
			string(diagnosticsJSON), isCurrent, now); err != nil {
			return err
		}
	}
	for _, occurrence := range response.Occurrences {
		evidenceJSON, marshalErr := json.Marshal(struct {
			SchemaVersion int    `json:"schemaVersion"`
			EngineVersion string `json:"engineVersion"`
			detection.Occurrence
		}{1, detection.EngineVersion, occurrence})
		if marshalErr != nil {
			return marshalErr
		}
		exceedanceID := uuid.New().String()
		level := titleSeverity(occurrence.Severity)
		_, err = tx.Exec(`INSERT INTO Exceedance
			(id, exceedanceValues, flightPhase, parameterName, description, eventStatus,
			 aircraftId, flightId, file, eventId, exceedanceLevel, detectionRunId,
			 startTimeMs, endTimeMs, durationMs, peakValue, ruleHash, isCurrent, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, 'Pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			exceedanceID, string(evidenceJSON), occurrence.Phase, occurrence.ParameterID,
			occurrence.Description, aircraft.ID, flight.ID, filename, occurrence.VersionID,
			level, runID, occurrence.StartTimeMs, occurrence.EndTimeMs, occurrence.DurationMs,
			occurrence.Value, occurrence.RuleHash, now, now)
		if err != nil {
			return err
		}
		if err = createDetectionNotifications(tx, aircraft.CompanyID, exceedanceID, occurrence, now); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE DetectionRun SET status = ?, timingSource = ?, rowCount = ?,
		evaluatedRuleCount = ?, occurrenceCount = ?, diagnosticsJson = ?, completedAt = ? WHERE id = ?`,
		response.Status, response.TimingSource, response.RowCount, response.EvaluatedRuleCount,
		response.OccurrenceCount, string(diagnosticsJSON), now, runID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE Csv SET status = ?, analysisSummary = ?, updatedAt = ? WHERE id = ?`,
		response.Status, string(summaryJSON), now, flight.ID)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	flight.AnalysisSummary = stringPointer(string(summaryJSON))
	return nil
}

func createDetectionNotifications(tx *sql.Tx, companyID, exceedanceID string, occurrence detection.Occurrence, now int64) error {
	rows, err := tx.Query(`SELECT id FROM User WHERE companyId = ? AND isActive = 1
		AND role IN ('admin', 'fda', 'gatekeeper')`, companyID)
	if err != nil {
		return err
	}
	userIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		userIDs = append(userIDs, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	message := fmt.Sprintf("%s (%s) detected during %s: %s %.3f %s for %.3f seconds",
		occurrence.EventName, occurrence.EventCode, occurrence.Phase, occurrence.ParameterID,
		occurrence.Value, occurrence.Unit, float64(occurrence.DurationMs)/1000)
	for _, userID := range userIDs {
		_, err = tx.Exec(`INSERT INTO Notification
			(id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, uuid.New().String(), userID, exceedanceID,
			message, notificationLevel(occurrence.Severity), now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *CSVHandler) failFlightAnalysis(flightID, runID string, response flightAnalysisResponse, analysisErr error, updateFlight bool) flightAnalysisResponse {
	response.Status = "failed"
	response.Error = analysisErr.Error()
	response.Occurrences = []detection.Occurrence{}
	completedAt := time.Now().UnixMilli()
	summaryJSON, _ := json.Marshal(response)
	if runID != "" {
		_, _ = h.db.Exec(`UPDATE DetectionRun SET status = 'failed', errorMessage = ?,
			diagnosticsJson = '[]', completedAt = ? WHERE id = ?`, response.Error, completedAt, runID)
	}
	if updateFlight {
		_, _ = h.db.Exec(`UPDATE Csv SET status = 'failed', analysisSummary = ?, updatedAt = ? WHERE id = ?`,
			string(summaryJSON), completedAt, flightID)
	}
	return response
}

func hashRuleSet(definitions []detection.Definition) string {
	items := append([]detection.Definition(nil), definitions...)
	sort.Slice(items, func(i, j int) bool { return items[i].VersionID < items[j].VersionID })
	hash := sha256.New()
	for _, definition := range items {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\n", definition.VersionID, definition.RuleHash)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func diagnosticsForVersion(diagnostics []detection.Diagnostic, versionID string) []detection.Diagnostic {
	result := make([]detection.Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		if diagnostic.RuleVersionID == versionID {
			result = append(result, diagnostic)
		}
	}
	return result
}

func definitionWasSkipped(diagnostics []detection.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "RULE_REQUIRED_COLUMNS_MISSING" {
			return true
		}
	}
	return false
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func titleSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func notificationLevel(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL", "HIGH":
		return "Level 3"
	case "MEDIUM":
		return "Level 2"
	default:
		return "Level 1"
	}
}

func valueOrZeroInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
