package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"fdm-backend/detection"
	"fdm-backend/models"

	"github.com/gin-gonic/gin"
)

type recordingReviewResponse struct {
	Recording models.CSV              `json:"recording"`
	Flights   []models.FlightLeg      `json:"flights"`
	Rows      detection.RecordingRows `json:"rows"`
}

type reviewedFlightInput struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Departure   string `json:"departure"`
	Destination string `json:"destination"`
	Pilot       string `json:"pilot"`
	FlightHours string `json:"flightHours"`
	StartRow    int    `json:"startRow" binding:"required,min=1"`
	EndRow      int    `json:"endRow" binding:"required,min=1"`
}

type updateRecordingFlightsRequest struct {
	Flights []reviewedFlightInput `json:"flights" binding:"required,min=1,dive"`
}

type deleteRecordingRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

func (h *CSVHandler) GetRecordingReview(c *gin.Context) {
	review, companyID, err := h.loadRecordingReview(c.Param("id"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recording not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessRecordingCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot access this recording"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": review})
}

func (h *CSVHandler) UpdateRecordingFlights(c *gin.Context) {
	var request updateRecordingFlightsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	review, companyID, err := h.loadRecordingReview(c.Param("id"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recording not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessRecordingCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot update this recording"})
		return
	}
	if len(request.Flights) != len(review.Flights) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Every flight in the recording must be submitted together"})
		return
	}

	existing := make(map[string]models.FlightLeg, len(review.Flights))
	for _, flight := range review.Flights {
		existing[flight.ID] = flight
	}
	seen := make(map[string]bool, len(request.Flights))
	ranges := make([]detection.RowRange, 0, len(request.Flights))
	for index := range request.Flights {
		item := &request.Flights[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Every flight requires a name"})
			return
		}
		if _, ok := existing[item.ID]; !ok || seen[item.ID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Flight IDs must match this recording exactly once"})
			return
		}
		seen[item.ID] = true
		if strings.TrimSpace(item.FlightHours) != "" {
			hours, parseErr := strconv.ParseFloat(strings.TrimSpace(item.FlightHours), 64)
			if parseErr != nil || hours < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Flight hours must be a non-negative number"})
				return
			}
		}
		ranges = append(ranges, detection.RowRange{StartRow: item.StartRow, EndRow: item.EndRow})
	}
	path, err := storedCSVPath(review.Recording.File)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Stored recording is unavailable"})
		return
	}
	if _, err = detection.ValidateRecordingRanges(path, ranges); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sort.Slice(request.Flights, func(i, j int) bool { return request.Flights[i].StartRow < request.Flights[j].StartRow })
	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	changedBoundaries := make([]string, 0)
	for index, item := range request.Flights {
		prior := existing[item.ID]
		priorEnd := valueOrZeroInt(prior.EndRow)
		boundaryChanged := prior.StartRow != item.StartRow || priorEnd != item.EndRow
		status := valueOrDefaultString(prior.Status, "pending_reanalysis")
		analysisSummary := prior.AnalysisSummary
		boundarySource := prior.BoundarySource
		if boundaryChanged {
			status = "pending_reanalysis"
			analysisSummary = nil
			boundarySource = "manual"
			changedBoundaries = append(changedBoundaries, item.ID)
		}
		_, err = tx.Exec(`UPDATE FlightLeg SET name = ?, departure = ?, destination = ?,
			pilot = ?, flightHours = ?, startRow = ?, endRow = ?, legIndex = ?,
			boundarySource = ?, status = ?, analysisSummary = ?, updatedAt = ?
			WHERE id = ? AND recordingId = ?`, item.Name, nullableReviewedText(item.Departure),
			nullableReviewedText(item.Destination), nullableReviewedText(item.Pilot),
			nullableReviewedText(item.FlightHours), item.StartRow, item.EndRow, index+1,
			boundarySource, status, analysisSummary, now, item.ID, review.Recording.ID)
		if err != nil {
			respondDatabaseError(c, err)
			return
		}
		if boundaryChanged {
			if _, err = tx.Exec(`UPDATE Exceedance SET isCurrent = 0, supersededAt = ?, updatedAt = ?
				WHERE flightLegId = ? AND isCurrent = 1`, now, now, item.ID); err != nil {
				respondDatabaseError(c, err)
				return
			}
			if _, err = tx.Exec(`UPDATE DetectionRunDefinition SET isCurrent = 0, supersededAt = ?
				WHERE isCurrent = 1 AND detectionRunId IN
				(SELECT id FROM DetectionRun WHERE flightLegId = ?)`, now, item.ID); err != nil {
				respondDatabaseError(c, err)
				return
			}
		}
	}
	if len(changedBoundaries) > 0 {
		if _, err = tx.Exec(`UPDATE Csv SET status = 'pending_reanalysis', analysisSummary = NULL, updatedAt = ? WHERE id = ?`, now, review.Recording.ID); err != nil {
			respondDatabaseError(c, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	updated, _, err := h.loadRecordingReview(review.Recording.ID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "data": updated, "boundaryChangedFlightIds": changedBoundaries,
		"requiresReanalysis": len(changedBoundaries) > 0,
	})
}

func (h *CSVHandler) DeleteRecording(c *gin.Context) {
	var request deleteRecordingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Type the recording name to confirm deletion"})
		return
	}
	var name, filename string
	lookupQuery := `SELECT c.name, c.file FROM Csv c
		JOIN Aircraft a ON a.id = c.aircraftId
		WHERE c.id = ?`
	lookupArgs := []interface{}{c.Param("id")}
	var companyID string
	if !hasGlobalCompanyAccess(c) {
		var ok bool
		companyID, ok = requireTenantCompany(c)
		if !ok {
			return
		}
		lookupQuery += " AND a.companyId = ?"
		lookupArgs = append(lookupArgs, companyID)
	}
	err := h.db.QueryRow(lookupQuery, lookupArgs...).Scan(&name, &filename)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recording not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if strings.TrimSpace(request.Confirmation) != name {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Confirmation does not match the recording name"})
		return
	}
	deleteQuery := `DELETE FROM Csv WHERE id = ?`
	deleteArgs := []interface{}{c.Param("id")}
	if !hasGlobalCompanyAccess(c) {
		deleteQuery += " AND aircraftId IN (SELECT id FROM Aircraft WHERE companyId = ?)"
		deleteArgs = append(deleteArgs, companyID)
	}
	result, err := h.db.Exec(deleteQuery, deleteArgs...)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recording not found"})
		return
	}
	fileDeleted := false
	warning := ""
	if path, pathErr := storedCSVPath(filename); pathErr == nil {
		if removeErr := os.Remove(path); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
			fileDeleted = true
		} else {
			warning = "Database records were deleted, but the stored source file could not be removed"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "message": "Recording and all contained flights were deleted",
		"fileDeleted": fileDeleted, "warning": warning,
	})
}

func (h *CSVHandler) loadRecordingReview(recordingID string) (recordingReviewResponse, string, error) {
	var response recordingReviewResponse
	var companyID string
	var createdAt, updatedAt nullableTimestamp
	err := h.db.QueryRow(`SELECT c.id, c.name, c.file, c.status, c.departure, c.pilot,
		c.destination, c.flightHours, c.aircraftId, c.sampleIntervalMs,
		c.analysisSummary, c.createdAt, c.updatedAt, a.companyId
		FROM Csv c JOIN Aircraft a ON a.id = c.aircraftId WHERE c.id = ?`, recordingID).Scan(
		&response.Recording.ID, &response.Recording.Name, &response.Recording.File,
		&response.Recording.Status, &response.Recording.Departure, &response.Recording.Pilot,
		&response.Recording.Destination, &response.Recording.FlightHours,
		&response.Recording.AircraftID, &response.Recording.SampleIntervalMs,
		&response.Recording.AnalysisSummary, &createdAt, &updatedAt, &companyID,
	)
	if err != nil {
		return response, "", err
	}
	if createdAt.Valid {
		response.Recording.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		response.Recording.UpdatedAt = updatedAt.Time
	}
	path, err := storedCSVPath(response.Recording.File)
	if err != nil {
		return response, "", err
	}
	response.Rows, err = detection.InspectRecordingRows(path)
	if err != nil {
		return response, "", err
	}

	rows, err := h.db.Query(`SELECT id, recordingId, name, aircraftId, legIndex,
		status, departure, pilot, destination, flightHours, startRow, endRow,
		startSample, endSample, boundarySource, analysisSummary, createdAt, updatedAt
		FROM FlightLeg WHERE recordingId = ? ORDER BY legIndex, startRow`, recordingID)
	if err != nil {
		return response, "", err
	}
	defer rows.Close()
	response.Flights = make([]models.FlightLeg, 0)
	for rows.Next() {
		var flight models.FlightLeg
		var flightCreatedAt, flightUpdatedAt nullableTimestamp
		if err = rows.Scan(&flight.ID, &flight.RecordingID, &flight.Name, &flight.AircraftID,
			&flight.LegIndex, &flight.Status, &flight.Departure, &flight.Pilot,
			&flight.Destination, &flight.FlightHours, &flight.StartRow, &flight.EndRow,
			&flight.StartSample, &flight.EndSample, &flight.BoundarySource,
			&flight.AnalysisSummary, &flightCreatedAt, &flightUpdatedAt); err != nil {
			return response, "", err
		}
		flight.File = response.Recording.File
		flight.SampleIntervalMs = response.Recording.SampleIntervalMs
		if flight.EndRow == nil {
			endRow := response.Rows.LastRow
			flight.EndRow = &endRow
		}
		if flightCreatedAt.Valid {
			flight.CreatedAt = flightCreatedAt.Time
		}
		if flightUpdatedAt.Valid {
			flight.UpdatedAt = flightUpdatedAt.Time
		}
		response.Flights = append(response.Flights, flight)
	}
	if err = rows.Err(); err != nil {
		return response, "", err
	}
	for index := range response.Flights {
		response.Flights[index].RecordingFlightCount = len(response.Flights)
	}
	return response, companyID, nil
}

func canAccessRecordingCompany(c *gin.Context, companyID string) bool {
	return canAccessCompany(c, companyID)
}

func nullableReviewedText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func valueOrDefaultString(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}
