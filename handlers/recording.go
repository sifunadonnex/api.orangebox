package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
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
	ID               string   `json:"id" binding:"required"`
	Name             string   `json:"name" binding:"required"`
	FlightDate       string   `json:"flightDate"`
	FlightNumber     string   `json:"flightNumber"`
	Departure        string   `json:"departure"`
	Destination      string   `json:"destination"`
	Pilot            string   `json:"pilot"`
	PICCrewCode      string   `json:"picCrewCode"`
	FOCrewCode       string   `json:"foCrewCode"`
	TechLogReference string   `json:"techLogReference"`
	LoadSheetNumber  string   `json:"loadSheetNumber"`
	TakeoffWeight    *float64 `json:"takeoffWeight"`
	LandingWeight    *float64 `json:"landingWeight"`
	WeightUnit       string   `json:"weightUnit"`
	V1               *float64 `json:"v1"`
	VR               *float64 `json:"vr"`
	V2               *float64 `json:"v2"`
	VRef             *float64 `json:"vref"`
	VApp             *float64 `json:"vapp"`
	Notes            string   `json:"notes"`
	FlightHours      string   `json:"flightHours"`
	StartRow         int      `json:"startRow" binding:"required,min=1"`
	EndRow           int      `json:"endRow" binding:"required,min=1"`
}

var icaoCodePattern = regexp.MustCompile(`^[A-Z]{4}$`)

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
	if !hasGlobalCompanyAccess(c) {
		review.Recording.AnalysisSummary = customerAnalysisSummary(review.Recording.AnalysisSummary)
		for index := range review.Flights {
			review.Flights[index].AnalysisSummary = customerAnalysisSummary(review.Flights[index].AnalysisSummary)
		}
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
		if err := normalizeReviewedFlightDetails(item); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Flight " + strconv.Itoa(index+1) + ": " + err.Error()})
			return
		}
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
		_, err = tx.Exec(`UPDATE FlightLeg SET name = ?, flightDate = ?, flightNumber = ?,
			departure = ?, destination = ?, pilot = ?, picCrewCode = ?, foCrewCode = ?,
			techLogReference = ?, loadSheetNumber = ?, takeoffWeight = ?, landingWeight = ?, weightUnit = ?,
			v1 = ?, vr = ?, v2 = ?, vref = ?, vapp = ?, notes = ?,
			flightHours = ?, startRow = ?, endRow = ?, legIndex = ?,
			boundarySource = ?, status = ?, analysisSummary = ?, updatedAt = ?
			WHERE id = ? AND recordingId = ?`, item.Name, nullableReviewedText(item.FlightDate),
			nullableReviewedText(item.FlightNumber), nullableReviewedText(item.Departure),
			nullableReviewedText(item.Destination), nullableReviewedText(item.PICCrewCode),
			nullableReviewedText(item.PICCrewCode), nullableReviewedText(item.FOCrewCode),
			nullableReviewedText(item.TechLogReference), nullableReviewedText(item.LoadSheetNumber),
			item.TakeoffWeight, item.LandingWeight, nullableReviewedText(item.WeightUnit),
			item.V1, item.VR, item.V2, item.VRef, item.VApp, nullableReviewedText(item.Notes),
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
	if !hasGlobalCompanyAccess(c) {
		updated.Recording.AnalysisSummary = customerAnalysisSummary(updated.Recording.AnalysisSummary)
		for index := range updated.Flights {
			updated.Flights[index].AnalysisSummary = customerAnalysisSummary(updated.Flights[index].AnalysisSummary)
		}
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
	var rawFilename sql.NullString
	lookupQuery := `SELECT c.name, c.file, c.rawSourceFile FROM Csv c
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
	err := h.db.QueryRow(lookupQuery, lookupArgs...).Scan(&name, &filename, &rawFilename)
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
	if rawFilename.Valid {
		if path, pathErr := storedRawRecordingPath(rawFilename.String); pathErr == nil {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				warning = "Database records were deleted, but the retained raw recording could not be removed"
			}
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
		c.analysisSummary, c.originalFilename, c.uploadedBy, c.uploadSource,
		c.sourceFormat, c.sourceEntry, c.normalizedBytes, c.rawSourceFormat,
		c.rawSourceFile, c.recorderContainerFormat, c.recorderAdapter,
		c.decoderProfileId, c.decoderProfileVersion, c.decoderProfileChecksum,
		u.fullName, u.email, c.createdAt, c.updatedAt, a.companyId
		FROM Csv c JOIN Aircraft a ON a.id = c.aircraftId
		LEFT JOIN User u ON u.id = c.uploadedBy WHERE c.id = ?`, recordingID).Scan(
		&response.Recording.ID, &response.Recording.Name, &response.Recording.File,
		&response.Recording.Status, &response.Recording.Departure, &response.Recording.Pilot,
		&response.Recording.Destination, &response.Recording.FlightHours,
		&response.Recording.AircraftID, &response.Recording.SampleIntervalMs,
		&response.Recording.AnalysisSummary, &response.Recording.OriginalFilename,
		&response.Recording.UploadedBy, &response.Recording.UploadSource,
		&response.Recording.SourceFormat, &response.Recording.SourceEntry,
		&response.Recording.NormalizedBytes, &response.Recording.RawSourceFormat,
		&response.Recording.RawSourceFile, &response.Recording.RecorderContainerFormat,
		&response.Recording.RecorderAdapter, &response.Recording.DecoderProfileID,
		&response.Recording.DecoderProfileVersion, &response.Recording.DecoderProfileChecksum,
		&response.Recording.UploaderName, &response.Recording.UploaderEmail,
		&createdAt, &updatedAt, &companyID,
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
		status, departure, pilot, destination, flightHours,
		flightDate, flightNumber, picCrewCode, foCrewCode, techLogReference, loadSheetNumber,
		takeoffWeight, landingWeight, weightUnit, v1, vr, v2, vref, vapp, notes, startRow, endRow,
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
			&flight.Destination, &flight.FlightHours, &flight.FlightDate, &flight.FlightNumber,
			&flight.PICCrewCode, &flight.FOCrewCode, &flight.TechLogReference, &flight.LoadSheetNumber,
			&flight.TakeoffWeight, &flight.LandingWeight, &flight.WeightUnit,
			&flight.V1, &flight.VR, &flight.V2, &flight.VRef, &flight.VApp, &flight.Notes,
			&flight.StartRow, &flight.EndRow,
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

func normalizeReviewedFlightDetails(item *reviewedFlightInput) error {
	item.FlightDate = strings.TrimSpace(item.FlightDate)
	if item.FlightDate != "" {
		if _, err := time.Parse("2006-01-02", item.FlightDate); err != nil {
			return errors.New("date must use YYYY-MM-DD")
		}
	}
	item.FlightNumber = strings.ToUpper(strings.TrimSpace(item.FlightNumber))
	item.Departure = strings.ToUpper(strings.TrimSpace(item.Departure))
	item.Destination = strings.ToUpper(strings.TrimSpace(item.Destination))
	for label, value := range map[string]string{"departure ICAO": item.Departure, "destination ICAO": item.Destination} {
		if value != "" && !icaoCodePattern.MatchString(value) {
			return fmt.Errorf("%s must contain exactly four letters", label)
		}
	}
	item.PICCrewCode = strings.ToUpper(strings.TrimSpace(item.PICCrewCode))
	if item.PICCrewCode == "" {
		item.PICCrewCode = strings.ToUpper(strings.TrimSpace(item.Pilot))
	}
	item.Pilot = item.PICCrewCode
	item.FOCrewCode = strings.ToUpper(strings.TrimSpace(item.FOCrewCode))
	item.TechLogReference = strings.TrimSpace(item.TechLogReference)
	item.LoadSheetNumber = strings.TrimSpace(item.LoadSheetNumber)
	item.Notes = strings.TrimSpace(item.Notes)
	item.WeightUnit = strings.ToLower(strings.TrimSpace(item.WeightUnit))
	if item.WeightUnit == "" && (item.TakeoffWeight != nil || item.LandingWeight != nil) {
		item.WeightUnit = "kg"
	}
	if item.WeightUnit != "" && item.WeightUnit != "kg" && item.WeightUnit != "lb" {
		return errors.New("weight unit must be kg or lb")
	}
	for label, value := range map[string]*float64{
		"take-off weight": item.TakeoffWeight, "landing weight": item.LandingWeight,
	} {
		if value != nil && (*value < 0 || *value > 10_000_000) {
			return fmt.Errorf("%s is outside the supported range", label)
		}
	}
	for label, value := range map[string]*float64{
		"V1": item.V1, "VR": item.VR, "V2": item.V2, "Vref": item.VRef, "Vapp": item.VApp,
	} {
		if value != nil && (*value < 0 || *value > 1000) {
			return fmt.Errorf("%s must be between 0 and 1,000 kt", label)
		}
	}
	for label, value := range map[string]string{
		"flight number": item.FlightNumber, "PIC crew code": item.PICCrewCode,
		"FO crew code": item.FOCrewCode, "DFR / tech log": item.TechLogReference,
		"load sheet number": item.LoadSheetNumber,
	} {
		if len(value) > 64 {
			return fmt.Errorf("%s must not exceed 64 characters", label)
		}
	}
	if len(item.Notes) > 2000 {
		return errors.New("notes must not exceed 2,000 characters")
	}
	return nil
}

func valueOrDefaultString(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}
