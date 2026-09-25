package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fdm-backend/detection"
	"fdm-backend/models"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CSVHandler struct {
	db *sql.DB
}

func NewCSVHandler(db *sql.DB) *CSVHandler {
	return &CSVHandler{db: db}
}

// UploadCSV handles CSV file upload
func (h *CSVHandler) UploadCSV(c *gin.Context) {
	var req models.UploadCSVRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded", "code": 400})
		return
	}
	if !strings.EqualFold(filepath.Ext(file.Filename), ".csv") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only CSV files are accepted"})
		return
	}
	if file.Size <= 0 || file.Size > 100*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSV file size must be between 1 byte and 100 MB"})
		return
	}

	aircraft, err := h.getDetectionAircraft(req.AircraftID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, aircraft.CompanyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot upload flight data for this aircraft"})
		return
	}

	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	filename := fmt.Sprintf("%d-%s", timestamp, filepath.Base(file.Filename))

	csvPath := filepath.Join("csvs", filename)
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Upload directory is unavailable", "code": 500})
		return
	}
	if err := c.SaveUploadedFile(file, csvPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "File upload failed", "code": 500})
		return
	}
	contentHash, err := hashFile(csvPath)
	if err != nil {
		_ = os.Remove(csvPath)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "CSV file could not be verified", "details": err.Error()})
		return
	}
	var existingRecordingID string
	err = h.db.QueryRow(`SELECT id FROM Csv WHERE aircraftId = ? AND contentHash = ? LIMIT 1`, req.AircraftID, contentHash).Scan(&existingRecordingID)
	if err == nil {
		_ = os.Remove(csvPath)
		c.JSON(http.StatusConflict, gin.H{
			"error": "This recording has already been uploaded for the selected aircraft",
			"code":  "DUPLICATE_RECORDING", "recordingId": existingRecordingID,
		})
		return
	}
	if err != sql.ErrNoRows {
		_ = os.Remove(csvPath)
		respondDatabaseError(c, err)
		return
	}
	segments, segmentationDiagnostics, err := detection.DetectRecordingSegments(csvPath)
	if err != nil {
		_ = os.Remove(csvPath)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "CSV recording could not be segmented", "details": err.Error()})
		return
	}
	phaseResult, phaseErr := detectUploadPhases(csvPath, aircraft, req.SampleIntervalMs)
	phaseResponse := phaseDetectionResponse{Status: "failed"}
	if phaseErr != nil {
		phaseResponse.Error = phaseErr.Error()
		segmentationDiagnostics = append(segmentationDiagnostics, detection.Diagnostic{
			Code: "PHASE_DETECTION_FAILED", Severity: "warning",
			Message: "Flight phases could not be detected; recording boundaries and any source phase column will be used: " + phaseErr.Error(), Count: 1,
		})
	} else {
		phaseResponse = phaseDetectionResponse{Status: "completed", Result: &phaseResult}
		segmentationDiagnostics = append(segmentationDiagnostics, phaseResult.Diagnostics...)
		if detectedSegments, segmentErr := phaseSegments(csvPath, phaseResult); segmentErr == nil {
			segments = detectedSegments
		} else {
			segmentationDiagnostics = append(segmentationDiagnostics, detection.Diagnostic{
				Code: "PHASE_BOUNDARIES_FALLBACK", Severity: "warning",
				Message: "Detected phases were retained, but conservative recording boundaries were used: " + segmentErr.Error(), Count: 1,
			})
		}
	}

	id := uuid.New().String()
	now := time.Now()
	uploadedBy, _ := contextString(c, "userId")
	role, _ := contextString(c, "userRole")
	uploadSource := "client_portal"
	if role == models.RoleAdmin || role == models.RoleFDA {
		uploadSource = "oversight_portal"
	}
	tx, err := h.db.Begin()
	if err != nil {
		_ = os.Remove(csvPath)
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	query := `INSERT INTO Csv (id, name, file, status, aircraftId, departure, destination, flightHours, pilot,
		sampleIntervalMs, originalFilename, contentHash, uploadedBy, uploadSource, createdAt, updatedAt)
		VALUES (?, ?, ?, 'processing', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.Exec(query, id, req.Name, filename, req.AircraftID, req.Departure, req.Destination,
		req.FlightHours, req.Pilot, req.SampleIntervalMs, filepath.Base(file.Filename), contentHash,
		optionalStringPointer(uploadedBy), uploadSource, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		_ = os.Remove(csvPath)
		if strings.Contains(err.Error(), "Csv.aircraftId, Csv.contentHash") {
			c.JSON(http.StatusConflict, gin.H{"error": "This recording has already been uploaded for the selected aircraft", "code": "DUPLICATE_RECORDING"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving CSV record", "details": err.Error()})
		return
	}

	recording := models.CSV{
		ID:               id,
		Name:             req.Name,
		File:             filename,
		AircraftID:       req.AircraftID,
		Departure:        req.Departure,
		Destination:      req.Destination,
		FlightHours:      req.FlightHours,
		Pilot:            req.Pilot,
		SampleIntervalMs: &req.SampleIntervalMs,
		OriginalFilename: stringPointer(filepath.Base(file.Filename)),
		ContentHash:      stringPointer(contentHash),
		UploadedBy:       optionalStringPointer(uploadedBy),
		UploadSource:     stringPointer(uploadSource),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if phaseErr == nil {
		recording.PhaseEngineVersion = stringPointer(phaseResult.EngineVersion)
		recording.PhaseProfile = stringPointer(phaseResult.Profile.Code)
		recording.PhaseTimingSource = stringPointer(phaseResult.TimingSource)
		if phaseJSON, marshalErr := json.Marshal(phaseResult); marshalErr == nil {
			recording.PhaseSummary = stringPointer(string(phaseJSON))
		}
	}
	flights := make([]models.FlightLeg, 0, len(segments))
	for index, segment := range segments {
		flightID := uuid.New().String()
		flightName := req.Name
		departure, destination, flightHours, pilot := req.Departure, req.Destination, req.FlightHours, req.Pilot
		if index == 0 {
			// Preserve existing single-flight URLs and migrated identifiers.
			flightID = id
		}
		if len(segments) > 1 {
			flightName = fmt.Sprintf("%s · Flight %d", req.Name, index+1)
			departure, destination, flightHours, pilot = nil, nil, nil, nil
		}
		startSample, endSample := optionalStringPointer(segment.StartSample), optionalStringPointer(segment.EndSample)
		endRow := segment.EndRow
		flight := models.FlightLeg{
			ID: flightID, RecordingID: id, Name: flightName, AircraftID: req.AircraftID,
			LegIndex: index + 1, Status: stringPointer("processing"), Departure: departure,
			RecordingFlightCount: len(segments),
			Destination:          destination, FlightHours: flightHours, Pilot: pilot,
			StartRow: segment.StartRow, EndRow: &endRow, StartSample: startSample,
			EndSample: endSample, BoundarySource: segment.BoundarySource,
			File: filename, SampleIntervalMs: &req.SampleIntervalMs, CreatedAt: now, UpdatedAt: now,
		}
		_, err = tx.Exec(`INSERT INTO FlightLeg
			(id, recordingId, name, aircraftId, legIndex, status, departure, pilot,
			 destination, flightHours, startRow, endRow, startSample, endSample,
			 boundarySource, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, 'processing', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			flight.ID, flight.RecordingID, flight.Name, flight.AircraftID, flight.LegIndex,
			flight.Departure, flight.Pilot, flight.Destination, flight.FlightHours,
			flight.StartRow, flight.EndRow, flight.StartSample, flight.EndSample,
			flight.BoundarySource, now.UnixMilli(), now.UnixMilli())
		if err != nil {
			_ = os.Remove(csvPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving detected flight legs", "details": err.Error()})
			return
		}
		flights = append(flights, flight)
	}
	if phaseErr == nil {
		if err = persistPhaseDetection(tx, id, phaseResult, now); err != nil {
			_ = os.Remove(csvPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving detected flight phases", "details": err.Error()})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		_ = os.Remove(csvPath)
		respondDatabaseError(c, err)
		return
	}

	triggeredBy, _ := contextString(c, "userId")
	analyses := make([]flightAnalysisResponse, 0, len(flights))
	for index := range flights {
		analysis := h.analyzeUploadedFlight(csvPath, filename, &flights[index], aircraft, triggeredBy)
		flights[index].Status = &analysis.Status
		analyses = append(analyses, analysis)
	}
	aggregate := aggregateFlightAnalyses(analyses, segmentationDiagnostics, len(flights))
	recording.Status = &aggregate.Status
	summaryJSON, _ := json.Marshal(aggregate)
	recording.AnalysisSummary = stringPointer(string(summaryJSON))
	_, _ = h.db.Exec(`UPDATE Csv SET status = ?, analysisSummary = ?, updatedAt = ? WHERE id = ?`,
		aggregate.Status, string(summaryJSON), time.Now().UnixMilli(), recording.ID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true, "data": flights[0], "recording": recording,
		"flights": flights, "flightCount": len(flights), "analyses": analyses, "analysis": aggregate,
		"phaseDetection": phaseResponse,
	})
}

// GetCSVs retrieves logical flights with their source-recording metadata.
func (h *CSVHandler) GetCSVs(c *gin.Context) {
	query := `SELECT f.id, f.recordingId, f.name, c.file, f.status, f.departure, f.pilot,
			  f.destination, f.flightHours, f.aircraftId, c.sampleIntervalMs,
			  f.analysisSummary, f.legIndex, f.startRow, f.endRow, f.startSample,
			  f.endSample, f.boundarySource,
			  (SELECT COUNT(1) FROM FlightLeg sibling WHERE sibling.recordingId = f.recordingId),
			  f.createdAt, f.updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.registration, a.companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, co.name as company_name, co.email as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, co.status as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM FlightLeg f
			  JOIN Csv c ON c.id = f.recordingId
			  LEFT JOIN Aircraft a ON f.aircraftId = a.id
			  LEFT JOIN Company co ON a.companyId = co.id`
	args := []interface{}{}
	if !hasGlobalCompanyAccess(c) {
		companyID, ok := tenantCompanyID(c)
		if !ok {
			c.JSON(http.StatusOK, []interface{}{})
			return
		}
		query += " WHERE a.companyId = ?"
		args = append(args, companyID)
	}
	query += " ORDER BY f.createdAt DESC, f.recordingId, f.legIndex"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var flights []interface{}
	for rows.Next() {
		var flight models.FlightLeg
		var aircraft models.Aircraft
		var company models.Company
		var createdAtStr, updatedAtStr sql.NullString
		var aircraftID sql.NullString
		var aircraftCreatedAtStr, aircraftUpdatedAtStr sql.NullString
		var companyID sql.NullString
		var companyCreatedAtStr, companyUpdatedAtStr sql.NullString

		err := rows.Scan(&flight.ID, &flight.RecordingID, &flight.Name, &flight.File,
			&flight.Status, &flight.Departure, &flight.Pilot, &flight.Destination,
			&flight.FlightHours, &flight.AircraftID, &flight.SampleIntervalMs,
			&flight.AnalysisSummary, &flight.LegIndex, &flight.StartRow, &flight.EndRow,
			&flight.StartSample, &flight.EndSample, &flight.BoundarySource,
			&flight.RecordingFlightCount, &createdAtStr, &updatedAtStr,
			&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
			&aircraft.SerialNumber, &aircraft.Registration, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAtStr, &aircraftUpdatedAtStr,
			&companyID, &company.Name, &company.Email, &company.Phone, &company.Address, &company.Country, &company.Logo, &company.Status, &company.SubscriptionID, &companyCreatedAtStr, &companyUpdatedAtStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning CSV"})
			return
		}

		// Parse CSV timestamps
		if createdAtStr.Valid {
			parsedTime, err := parseTimestamp(createdAtStr.String)
			if err == nil {
				flight.CreatedAt = parsedTime
			}
		}
		if updatedAtStr.Valid {
			parsedTime, err := parseTimestamp(updatedAtStr.String)
			if err == nil {
				flight.UpdatedAt = parsedTime
			}
		}

		// Handle aircraft
		var aircraftPtr *models.Aircraft
		if aircraftID.Valid {
			aircraft.ID = aircraftID.String
			if aircraftCreatedAtStr.Valid {
				parsedTime, err := parseTimestamp(aircraftCreatedAtStr.String)
				if err == nil {
					aircraft.CreatedAt = parsedTime
				}
			}
			if aircraftUpdatedAtStr.Valid {
				parsedTime, err := parseTimestamp(aircraftUpdatedAtStr.String)
				if err == nil {
					aircraft.UpdatedAt = parsedTime
				}
			}

			// Handle company
			var companyPtr *models.Company
			if companyID.Valid {
				company.ID = companyID.String
				if companyCreatedAtStr.Valid {
					parsedTime, err := parseTimestamp(companyCreatedAtStr.String)
					if err == nil {
						company.CreatedAt = parsedTime
					}
				}
				if companyUpdatedAtStr.Valid {
					parsedTime, err := parseTimestamp(companyUpdatedAtStr.String)
					if err == nil {
						company.UpdatedAt = parsedTime
					}
				}
				companyPtr = &company
			}

			// Add company to aircraft if available
			if companyPtr != nil {
				aircraft.Company = companyPtr
			}

			aircraftPtr = &aircraft
		}

		// Get related exceedances
		exceedances, _ := h.getCSVExceedances(flight.ID)

		flightWithExceedances := struct {
			models.FlightLeg
			Aircraft   *models.Aircraft    `json:"aircraft"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			FlightLeg:  flight,
			Aircraft:   aircraftPtr,
			Exceedance: exceedances,
		}

		flights = append(flights, flightWithExceedances)
	}

	c.JSON(http.StatusOK, flights)
}

// DownloadCSV serves the source rows belonging to the requested flight as an
// analysis view. The immutable stored file is never rewritten; a virtual
// Detected Phase column is appended from the persisted row-range timeline.
func (h *CSVHandler) DownloadCSV(c *gin.Context) {
	flight, aircraft, err := h.getStoredFlight(c.Param("id"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, aircraft.CompanyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot access this flight data"})
		return
	}
	filePath, err := storedCSVPath(flight.File)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stored recording not found"})
		return
	}
	phaseRuns, err := h.loadFlightPhaseRuns(flight.RecordingID, flight.StartRow, valueOrZeroInt(flight.EndRow))
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = streamCSVRange(c, filePath, flight.Name, flight.StartRow, valueOrZeroInt(flight.EndRow), phaseRuns); err != nil {
		log.Printf("Error streaming flight CSV: %v", err)
	}
}

// GetCSVByID retrieves a logical flight and its source-recording metadata.
func (h *CSVHandler) GetCSVByID(c *gin.Context) {
	id := c.Param("id")
	query := `SELECT f.id, f.recordingId, f.name, c.file, f.status, f.departure,
		f.pilot, f.destination, f.flightHours, f.aircraftId, c.sampleIntervalMs,
		f.analysisSummary, f.legIndex, f.startRow, f.endRow, f.startSample,
		f.endSample, f.boundarySource,
		(SELECT COUNT(1) FROM FlightLeg sibling WHERE sibling.recordingId = f.recordingId),
		f.createdAt, f.updatedAt
		FROM FlightLeg f JOIN Csv c ON c.id = f.recordingId
		JOIN Aircraft a ON a.id = f.aircraftId
		WHERE f.id = ?`
	args := []interface{}{id}
	if !hasGlobalCompanyAccess(c) {
		companyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		query += " AND a.companyId = ?"
		args = append(args, companyID)
	}

	var flight models.FlightLeg
	var createdAtStr, updatedAtStr sql.NullString
	row := h.db.QueryRow(query, args...)
	err := row.Scan(&flight.ID, &flight.RecordingID, &flight.Name, &flight.File,
		&flight.Status, &flight.Departure, &flight.Pilot, &flight.Destination,
		&flight.FlightHours, &flight.AircraftID, &flight.SampleIntervalMs,
		&flight.AnalysisSummary, &flight.LegIndex, &flight.StartRow, &flight.EndRow,
		&flight.StartSample, &flight.EndSample, &flight.BoundarySource,
		&flight.RecordingFlightCount, &createdAtStr, &updatedAtStr)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		} else {
			log.Printf("Error scanning CSV record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		}
		return
	}

	// Parse timestamps using the helper function
	if createdAtStr.Valid {
		parsedTime, err := parseTimestamp(createdAtStr.String)
		if err == nil {
			flight.CreatedAt = parsedTime
		}
	}
	if updatedAtStr.Valid {
		parsedTime, err := parseTimestamp(updatedAtStr.String)
		if err == nil {
			flight.UpdatedAt = parsedTime
		}
	}

	c.JSON(http.StatusOK, flight)
}

// Helper function to get exceedances for a CSV with related EventLog and Aircraft data
func (h *CSVHandler) getCSVExceedances(csvID string) ([]models.Exceedance, error) {
	query := `SELECT e.id, e.exceedanceValues, e.flightPhase, e.parameterName, e.description, e.eventStatus,
			  e.aircraftId, COALESCE(e.flightLegId, e.flightId), e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  a.serialNumber as aircraftRegistration,
			  ev.id as eventLogId, ev.eventName, ev.displayName, ev.eventCode, ev.eventDescription,
			  ev.eventParameter, ev.eventTrigger, ev.eventType, ev.flightPhase as eventFlightPhase
			  FROM Exceedance e
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id
			  LEFT JOIN EventLog ev ON e.eventId = ev.id
			  WHERE COALESCE(e.flightLegId, e.flightId) = ? AND e.isCurrent = 1`
	rows, err := h.db.Query(query, csvID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exceedances []models.Exceedance
	for rows.Next() {
		var exceedance models.Exceedance
		var createdAtStr, updatedAtStr sql.NullString
		var aircraftRegistration sql.NullString
		// EventLog fields
		var eventLogId, eventName, displayName, eventCode, eventDescription sql.NullString
		var eventParameter, eventTrigger, eventType, eventFlightPhase sql.NullString

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &createdAtStr, &updatedAtStr,
			&aircraftRegistration,
			&eventLogId, &eventName, &displayName, &eventCode, &eventDescription,
			&eventParameter, &eventTrigger, &eventType, &eventFlightPhase)
		if err != nil {
			continue
		}

		// Parse timestamps
		if createdAtStr.Valid {
			parsedTime, err := parseTimestamp(createdAtStr.String)
			if err == nil {
				exceedance.CreatedAt = parsedTime
			}
		}
		if updatedAtStr.Valid {
			parsedTime, err := parseTimestamp(updatedAtStr.String)
			if err == nil {
				exceedance.UpdatedAt = parsedTime
			}
		}

		// Set aircraft registration
		if aircraftRegistration.Valid {
			exceedance.AircraftRegistration = &aircraftRegistration.String
		}

		// Build EventLog if available
		if eventLogId.Valid {
			eventLog := &models.EventLog{
				ID: eventLogId.String,
			}
			if eventName.Valid {
				eventLog.EventName = &eventName.String
			}
			if displayName.Valid {
				eventLog.DisplayName = displayName.String
			}
			if eventCode.Valid {
				eventLog.EventCode = eventCode.String
			}
			if eventDescription.Valid {
				eventLog.EventDescription = eventDescription.String
			}
			if eventParameter.Valid {
				eventLog.EventParameter = eventParameter.String
			}
			if eventTrigger.Valid {
				eventLog.EventTrigger = eventTrigger.String
			}
			if eventType.Valid {
				eventLog.EventType = eventType.String
			}
			if eventFlightPhase.Valid {
				eventLog.FlightPhase = eventFlightPhase.String
			}
			exceedance.EventLog = eventLog
		}

		exceedances = append(exceedances, exceedance)
	}

	return exceedances, nil
}

// DeleteCSV deletes a CSV file and its associated data
func (h *CSVHandler) DeleteCSV(c *gin.Context) {
	id := c.Param("id")
	var recordingID string
	lookupQuery := `SELECT f.recordingId FROM FlightLeg f
		JOIN Aircraft a ON a.id = f.aircraftId
		WHERE f.id = ?`
	lookupArgs := []interface{}{id}
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
	err := h.db.QueryRow(lookupQuery, lookupArgs...).Scan(&recordingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	var legCount int
	err = h.db.QueryRow("SELECT COUNT(1) FROM FlightLeg WHERE recordingId = ?", recordingID).Scan(&legCount)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if legCount > 1 {
		c.JSON(http.StatusConflict, gin.H{
			"error":       "This source recording contains multiple flights. Recording-level deletion requires a separate explicit confirmation.",
			"recordingId": recordingID, "flightCount": legCount,
		})
		return
	}

	query := `DELETE FROM Csv WHERE id = ?`
	deleteArgs := []interface{}{recordingID}
	if !hasGlobalCompanyAccess(c) {
		query += " AND aircraftId IN (SELECT id FROM Aircraft WHERE companyId = ?)"
		deleteArgs = append(deleteArgs, companyID)
	}
	result, err := h.db.Exec(query, deleteArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete flight", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Flight deleted successfully"})
}

func optionalStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func aggregateFlightAnalyses(analyses []flightAnalysisResponse, segmentationDiagnostics []detection.Diagnostic, flightCount int) flightAnalysisResponse {
	aggregate := flightAnalysisResponse{
		Status: "completed", EngineVersion: detection.EngineVersion, TriggerType: "upload",
		Occurrences: []detection.Occurrence{}, Diagnostics: append([]detection.Diagnostic{}, segmentationDiagnostics...),
	}
	failed := 0
	for _, analysis := range analyses {
		if aggregate.SampleIntervalMs == 0 {
			aggregate.SampleIntervalMs = analysis.SampleIntervalMs
		}
		if aggregate.RuleSetHash == "" {
			aggregate.RuleSetHash = analysis.RuleSetHash
		}
		aggregate.RowCount += analysis.RowCount
		aggregate.ApplicableRuleCount += analysis.ApplicableRuleCount
		aggregate.EvaluatedRuleCount += analysis.EvaluatedRuleCount
		aggregate.OccurrenceCount += analysis.OccurrenceCount
		aggregate.Occurrences = append(aggregate.Occurrences, analysis.Occurrences...)
		aggregate.Diagnostics = append(aggregate.Diagnostics, analysis.Diagnostics...)
		if analysis.Status == "failed" {
			failed++
		} else if analysis.Status == "completed_with_warnings" {
			aggregate.Status = "completed_with_warnings"
		}
	}
	if flightCount > 1 {
		aggregate.Status = "completed_with_warnings"
		aggregate.Diagnostics = append(aggregate.Diagnostics, detection.Diagnostic{
			Code: "MULTIPLE_FLIGHTS_DETECTED", Severity: "warning",
			Message: fmt.Sprintf("%d independent recorder sessions were detected and analyzed as separate flights; review their metadata", flightCount), Count: 1,
		})
	}
	if failed == len(analyses) && failed > 0 {
		aggregate.Status = "failed"
		aggregate.Error = "Analysis failed for every detected flight"
	} else if failed > 0 {
		aggregate.Status = "completed_with_warnings"
		aggregate.Diagnostics = append(aggregate.Diagnostics, detection.Diagnostic{
			Code: "FLIGHT_ANALYSIS_PARTIAL_FAILURE", Severity: "error",
			Message: fmt.Sprintf("Analysis failed for %d of %d detected flights", failed, len(analyses)), Count: failed,
		})
	}
	return aggregate
}

func streamCSVRange(c *gin.Context, path, flightName string, startRow, endRow int, phaseRuns []detection.PhaseRun) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	headerLine := 0
	var header []string
	for line := 1; line <= 50; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		if isFlightCSVHeader(record) {
			headerLine = line
			header = append([]string(nil), record...)
			break
		}
	}
	if headerLine == 0 {
		return fmt.Errorf("CSV header was not found in the first 50 rows")
	}
	header = append(header, "Detected Phase")
	safeName := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(strings.TrimSpace(flightName))
	if safeName == "" {
		safeName = "flight"
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.csv\"", safeName))
	writer := csv.NewWriter(c.Writer)
	if err = writer.Write(header); err != nil {
		return err
	}
	recordNumber := headerLine
	runIndex := 0
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		recordNumber++
		if startRow > 0 && recordNumber < startRow {
			continue
		}
		if endRow > 0 && recordNumber > endRow {
			break
		}
		for runIndex < len(phaseRuns) && recordNumber > phaseRuns[runIndex].EndRow {
			runIndex++
		}
		phase := ""
		if runIndex < len(phaseRuns) && recordNumber >= phaseRuns[runIndex].StartRow && recordNumber <= phaseRuns[runIndex].EndRow {
			phase = phaseRuns[runIndex].Phase
		}
		record = append(record, phase)
		if err = writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func isFlightCSVHeader(record []string) bool {
	known := map[string]bool{
		"SAMPLE": true, "FRAME": true, "TIME": true, "UTCTIME": true, "LCLTIME": true,
		"TIMEELAPSED": true, "ELAPSEDSECONDS": true, "LATITUDE": true, "LONGITUDE": true,
		"IAS": true, "AIRSPEED": true, "ALTMSL": true, "ALTIND": true, "ALTITUDE": true,
	}
	nonEmpty := 0
	hasKnown := false
	for _, value := range record {
		key := strings.Map(func(character rune) rune {
			if character >= 'a' && character <= 'z' {
				return character - ('a' - 'A')
			}
			if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
				return character
			}
			return -1
		}, strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
		if key == "" {
			continue
		}
		nonEmpty++
		hasKnown = hasKnown || known[key]
	}
	return nonEmpty >= 2 && hasKnown
}
