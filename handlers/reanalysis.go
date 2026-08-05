package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
)

type backfillFlightResult struct {
	FlightID        string `json:"flightId"`
	RunID           string `json:"runId,omitempty"`
	Status          string `json:"status"`
	OccurrenceCount int    `json:"occurrenceCount"`
	Reused          bool   `json:"reused"`
	Error           string `json:"error,omitempty"`
}

type eventBackfillResponse struct {
	EventDefinitionID  string                 `json:"eventDefinitionId"`
	MatchedFlightCount int                    `json:"matchedFlightCount"`
	AnalyzedCount      int                    `json:"analyzedCount"`
	ReusedCount        int                    `json:"reusedCount"`
	FailedCount        int                    `json:"failedCount"`
	OccurrenceCount    int                    `json:"occurrenceCount"`
	Results            []backfillFlightResult `json:"results"`
}

// ReanalyzeCSV evaluates a stored flight without requiring its source file to
// be uploaded again. By default an identical completed run is reused; callers
// may explicitly request ?force=true to produce a fresh audited run.
func (h *CSVHandler) ReanalyzeCSV(c *gin.Context) {
	flight, aircraft, err := h.getStoredFlight(c.Param("id"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !isSystemEventRole(c) {
		companyID, ok := contextString(c, "userCompanyId")
		if !ok || companyID != aircraft.CompanyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You cannot analyze this flight"})
			return
		}
	}
	definitions, err := h.loadApplicableDefinitions(aircraft, "")
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	path, err := storedCSVPath(flight.File)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	triggeredBy, _ := contextString(c, "userId")
	analysis := h.analyzeFlight(path, flight.File, &flight, aircraft, definitions, flightAnalysisOptions{
		TriggerType: "manual", TriggeredBy: triggeredBy,
		ReuseCompleted: !strings.EqualFold(c.Query("force"), "true"),
	})
	status := http.StatusOK
	if analysis.Status == "failed" {
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, gin.H{"success": analysis.Status != "failed", "data": flight, "analysis": analysis})
}

// BackfillEvent analyzes every stored flight covered by the currently
// published version and its assignments. Runs are committed per flight, so a
// bad or missing source file cannot roll back successful flights.
func (h *CSVHandler) BackfillEvent(c *gin.Context) {
	definitionID := c.Param("id")
	companyID, err := h.getBackfillEventCompany(definitionID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active published event definition not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessEventCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot backfill this event definition"})
		return
	}
	flights, aircrafts, err := h.listBackfillCandidates(companyID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	requestedBy, _ := contextString(c, "userId")
	response := eventBackfillResponse{
		EventDefinitionID: definitionID,
		Results:           make([]backfillFlightResult, 0),
	}
	for index := range flights {
		flight := flights[index]
		aircraft := aircrafts[index]
		definitions, loadErr := h.loadApplicableDefinitions(aircraft, definitionID)
		if loadErr != nil {
			response.FailedCount++
			response.Results = append(response.Results, backfillFlightResult{FlightID: flight.ID, Status: "failed", Error: loadErr.Error()})
			continue
		}
		if len(definitions) == 0 {
			continue
		}
		response.MatchedFlightCount++
		path, pathErr := storedCSVPath(flight.File)
		if pathErr != nil {
			response.FailedCount++
			response.Results = append(response.Results, backfillFlightResult{FlightID: flight.ID, Status: "failed", Error: pathErr.Error()})
			continue
		}
		analysis := h.analyzeFlight(path, flight.File, &flight, aircraft, definitions, flightAnalysisOptions{
			TriggerType: "event_backfill", TriggeredBy: requestedBy, ReuseCompleted: true,
		})
		item := backfillFlightResult{
			FlightID: flight.ID, RunID: analysis.RunID, Status: analysis.Status,
			OccurrenceCount: analysis.OccurrenceCount, Reused: analysis.Reused, Error: analysis.Error,
		}
		response.Results = append(response.Results, item)
		if analysis.Status == "failed" {
			response.FailedCount++
			continue
		}
		if analysis.Reused {
			response.ReusedCount++
		} else {
			response.AnalyzedCount++
		}
		response.OccurrenceCount += analysis.OccurrenceCount
	}
	c.JSON(http.StatusOK, gin.H{"success": response.FailedCount == 0, "data": response})
}

func (h *CSVHandler) getStoredFlight(id string) (models.CSV, models.Aircraft, error) {
	var flight models.CSV
	var aircraft models.Aircraft
	err := h.db.QueryRow(`SELECT c.id, c.name, c.file, c.aircraftId, c.sampleIntervalMs,
		a.id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber,
		a.registration, a.companyId, a.parameters
		FROM Csv c JOIN Aircraft a ON a.id = c.aircraftId WHERE c.id = ?`, id).Scan(
		&flight.ID, &flight.Name, &flight.File, &flight.AircraftID, &flight.SampleIntervalMs,
		&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.Registration, &aircraft.CompanyID, &aircraft.Parameters,
	)
	return flight, aircraft, err
}

func (h *CSVHandler) getBackfillEventCompany(definitionID string) (*string, error) {
	var companyID sql.NullString
	now := time.Now().UnixMilli()
	err := h.db.QueryRow(`SELECT d.companyId FROM EventDefinition d
		WHERE d.id = ? AND d.lifecycleStatus = 'active' AND EXISTS (
			SELECT 1 FROM EventDefinitionVersion v WHERE v.definitionId = d.id
			AND v.status = 'published'
			AND (v.effectiveFrom IS NULL OR v.effectiveFrom <= ?)
			AND (v.effectiveTo IS NULL OR v.effectiveTo > ?)
		)`, definitionID, now, now).Scan(&companyID)
	if !companyID.Valid {
		return nil, err
	}
	return stringPointer(companyID.String), err
}

func (h *CSVHandler) listBackfillCandidates(companyID *string) ([]models.CSV, []models.Aircraft, error) {
	query := `SELECT c.id, c.name, c.file, c.aircraftId, c.sampleIntervalMs,
		a.id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber,
		a.registration, a.companyId, a.parameters
		FROM Csv c JOIN Aircraft a ON a.id = c.aircraftId`
	args := []any{}
	if companyID != nil {
		query += " WHERE a.companyId = ?"
		args = append(args, *companyID)
	}
	query += " ORDER BY c.createdAt, c.id"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	flights := make([]models.CSV, 0)
	aircrafts := make([]models.Aircraft, 0)
	for rows.Next() {
		var flight models.CSV
		var aircraft models.Aircraft
		if err = rows.Scan(&flight.ID, &flight.Name, &flight.File, &flight.AircraftID,
			&flight.SampleIntervalMs, &aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake,
			&aircraft.ModelNumber, &aircraft.SerialNumber, &aircraft.Registration,
			&aircraft.CompanyID, &aircraft.Parameters); err != nil {
			return nil, nil, err
		}
		flights = append(flights, flight)
		aircrafts = append(aircrafts, aircraft)
	}
	return flights, aircrafts, rows.Err()
}

func storedCSVPath(filename string) (string, error) {
	if filename == "" || filename != filepath.Base(filename) {
		return "", &os.PathError{Op: "resolve stored CSV", Path: filename, Err: os.ErrInvalid}
	}
	path := filepath.Join("csvs", filename)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", &os.PathError{Op: "open stored CSV", Path: path, Err: os.ErrInvalid}
	}
	return path, nil
}
