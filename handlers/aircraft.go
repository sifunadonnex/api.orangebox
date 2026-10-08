package handlers

import (
	"database/sql"
	"errors"
	"fdm-backend/models"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AircraftHandler struct {
	db *sql.DB
}

func NewAircraftHandler(db *sql.DB) *AircraftHandler {
	return &AircraftHandler{db: db}
}

// GetAircrafts retrieves all aircraft with related data
func (h *AircraftHandler) GetAircrafts(c *gin.Context) {
	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, parameters, createdAt, updatedAt FROM Aircraft`
	args := []interface{}{}
	if !hasGlobalCompanyAccess(c) {
		companyID, ok := tenantCompanyID(c)
		if !ok {
			c.JSON(http.StatusOK, []interface{}{})
			return
		}
		query += " WHERE companyId = ?"
		args = append(args, companyID)
	}
	rows, err := h.db.Query(query, args...)
	if err != nil {
		println("GetAircrafts query error:", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, registration, parameters sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &registration, &aircraft.CompanyID, &parameters, &createdAtStr, &updatedAtStr)
		if err != nil {
			println("GetAircrafts scan error:", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning aircraft", "details": err.Error()})
			return
		}

		// Handle nullable fields
		if modelNumber.Valid {
			aircraft.ModelNumber = &modelNumber.String
		}
		if registration.Valid {
			aircraft.Registration = &registration.String
		}
		if parameters.Valid {
			aircraft.Parameters = &parameters.String
		}
		// Parse timestamps
		if createdAtStr.Valid {
			if t, err := parseTimestamp(createdAtStr.String); err == nil {
				aircraft.CreatedAt = t
			}
		}
		if updatedAtStr.Valid {
			if t, err := parseTimestamp(updatedAtStr.String); err == nil {
				aircraft.UpdatedAt = t
			}
		}

		// Get company details
		company, _ := h.getAircraftCompany(aircraft.CompanyID)
		if err != nil {
			println("Error getting company for aircraft", aircraft.ID, ":", err.Error())
			// Don't fail the request, just log the error
		}

		// Get related CSV files
		csvs, err := h.getAircraftCSVs(aircraft.ID, hasGlobalCompanyAccess(c))
		if err != nil {
			println("Error getting CSVs for aircraft", aircraft.ID, ":", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting CSV files", "details": err.Error()})
			return
		}

		// Get related event logs
		eventLogs, err := h.getAircraftEventLogs(aircraft.ID)
		if err != nil {
			println("Error getting event logs for aircraft", aircraft.ID, ":", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting event logs", "details": err.Error()})
			return
		}

		// Get related exceedances
		exceedances, err := h.getAircraftExceedances(aircraft.ID, hasGlobalCompanyAccess(c))
		if err != nil {
			println("Error getting exceedances for aircraft", aircraft.ID, ":", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting exceedances", "details": err.Error()})
			return
		}

		aircraftWithRelations := struct {
			models.Aircraft
			Company    *models.Company     `json:"company"`
			CSV        []models.CSV        `json:"csv"`
			EventLog   []models.EventLog   `json:"EventLog"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			Aircraft:   aircraft,
			Company:    company,
			CSV:        csvs,
			EventLog:   eventLogs,
			Exceedance: exceedances,
		}

		aircrafts = append(aircrafts, aircraftWithRelations)
	}

	c.JSON(http.StatusOK, aircrafts)
}

// GetAircraftByID retrieves a single aircraft by its ID
func (h *AircraftHandler) GetAircraftByID(c *gin.Context) {
	aircraftID := c.Param("id")
	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, parameters, createdAt, updatedAt FROM Aircraft WHERE id = ?`
	args := []interface{}{aircraftID}
	if !hasGlobalCompanyAccess(c) {
		companyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		query += " AND companyId = ?"
		args = append(args, companyID)
	}

	var aircraft models.Aircraft
	var modelNumber, registration, parameters sql.NullString
	var createdAtStr, updatedAtStr sql.NullString

	err := h.db.QueryRow(query, args...).Scan(
		&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
		&aircraft.SerialNumber, &registration, &aircraft.CompanyID, &parameters, &createdAtStr, &updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}

	// Handle nullable fields
	if modelNumber.Valid {
		aircraft.ModelNumber = &modelNumber.String
	}
	if registration.Valid {
		aircraft.Registration = &registration.String
	}
	if parameters.Valid {
		aircraft.Parameters = &parameters.String
	}
	// Parse timestamps
	if createdAtStr.Valid {
		if t, err := parseTimestamp(createdAtStr.String); err == nil {
			aircraft.CreatedAt = t
		}
	}
	if updatedAtStr.Valid {
		if t, err := parseTimestamp(updatedAtStr.String); err == nil {
			aircraft.UpdatedAt = t
		}
	}

	// Get company details
	company, _ := h.getAircraftCompany(aircraft.CompanyID)

	// Get related CSV files
	csvs, _ := h.getAircraftCSVs(aircraft.ID, hasGlobalCompanyAccess(c))

	// Get related event logs
	eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

	// Get related exceedances
	exceedances, _ := h.getAircraftExceedances(aircraft.ID, hasGlobalCompanyAccess(c))

	aircraftWithRelations := struct {
		models.Aircraft
		Company    *models.Company     `json:"company"`
		CSV        []models.CSV        `json:"csv"`
		EventLog   []models.EventLog   `json:"EventLog"`
		Exceedance []models.Exceedance `json:"Exceedance"`
	}{
		Aircraft:   aircraft,
		Company:    company,
		CSV:        csvs,
		EventLog:   eventLogs,
		Exceedance: exceedances,
	}

	c.JSON(http.StatusOK, aircraftWithRelations)
}

// GetAircraftsByUserID retrieves aircraft by company ID (kept for backward compatibility)
func (h *AircraftHandler) GetAircraftsByUserID(c *gin.Context) {
	companyID := c.Param("id")
	if !hasGlobalCompanyAccess(c) {
		requestCompanyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		if companyID != requestCompanyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only access aircraft in your company"})
			return
		}
	}

	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, parameters, createdAt, updatedAt FROM Aircraft WHERE companyId = ?`
	rows, err := h.db.Query(query, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, registration, parameters sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &registration, &aircraft.CompanyID, &parameters, &createdAtStr, &updatedAtStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning aircraft"})
			return
		}

		// Handle nullable fields
		if modelNumber.Valid {
			aircraft.ModelNumber = &modelNumber.String
		}
		if registration.Valid {
			aircraft.Registration = &registration.String
		}
		if parameters.Valid {
			aircraft.Parameters = &parameters.String
		}
		// Parse timestamps
		if createdAtStr.Valid {
			if t, err := parseTimestamp(createdAtStr.String); err == nil {
				aircraft.CreatedAt = t
			}
		}
		if updatedAtStr.Valid {
			if t, err := parseTimestamp(updatedAtStr.String); err == nil {
				aircraft.UpdatedAt = t
			}
		}

		// Get company details
		company, _ := h.getAircraftCompany(aircraft.CompanyID)

		// Get related CSV files
		csvs, _ := h.getAircraftCSVs(aircraft.ID, hasGlobalCompanyAccess(c))

		// Get related event logs
		eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

		// Get related exceedances
		exceedances, _ := h.getAircraftExceedances(aircraft.ID, hasGlobalCompanyAccess(c))

		aircraftWithRelations := struct {
			models.Aircraft
			Company    *models.Company     `json:"company"`
			CSV        []models.CSV        `json:"csv"`
			EventLog   []models.EventLog   `json:"EventLog"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			Aircraft:   aircraft,
			Company:    company,
			CSV:        csvs,
			EventLog:   eventLogs,
			Exceedance: exceedances,
		}

		aircrafts = append(aircrafts, aircraftWithRelations)
	}

	c.JSON(http.StatusOK, aircrafts)
}

// CreateAircraft creates a new aircraft
func (h *AircraftHandler) CreateAircraft(c *gin.Context) {
	var req models.CreateAircraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if hasGlobalCompanyAccess(c) {
		if req.CompanyID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "companyId is required"})
			return
		}
	} else {
		companyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		if req.CompanyID != "" && req.CompanyID != companyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You cannot create aircraft for another company"})
			return
		}
		req.CompanyID = companyID
	}

	// Generate ID and timestamps
	id := uuid.New().String()
	now := time.Now()

	// Insert aircraft
	query := `INSERT INTO Aircraft (id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, parameters, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := h.db.Exec(query, id, req.Airline, req.AircraftMake, req.ModelNumber, req.SerialNumber, req.Registration, req.CompanyID, req.Parameters, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating aircraft"})
		return
	}

	// Return created aircraft
	aircraft := models.Aircraft{
		ID:           id,
		Airline:      req.Airline,
		AircraftMake: req.AircraftMake,
		ModelNumber:  req.ModelNumber,
		SerialNumber: req.SerialNumber,
		Registration: req.Registration,
		CompanyID:    req.CompanyID,
		Parameters:   req.Parameters,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	c.JSON(http.StatusOK, aircraft)
}

// UpdateAircraft updates an existing aircraft
func (h *AircraftHandler) UpdateAircraft(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateAircraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	companyID := req.CompanyID
	if hasGlobalCompanyAccess(c) {
		if companyID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "companyId is required"})
			return
		}
	} else {
		var ok bool
		companyID, ok = requireTenantCompany(c)
		if !ok {
			return
		}
		if req.CompanyID != "" && req.CompanyID != companyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You cannot move aircraft to another company"})
			return
		}
	}

	now := time.Now()

	query := `UPDATE Aircraft SET airline = ?, aircraftMake = ?, modelNumber = ?, serialNumber = ?, registration = ?, companyId = ?, parameters = ?, updatedAt = ? WHERE id = ?`
	args := []interface{}{req.Airline, req.AircraftMake, req.ModelNumber, req.SerialNumber, req.Registration, companyID, req.Parameters, now.UnixMilli(), id}
	if !hasGlobalCompanyAccess(c) {
		query += " AND companyId = ?"
		args = append(args, companyID)
	}
	result, err := h.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating aircraft"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
		return
	}

	// Return updated aircraft
	aircraft := models.Aircraft{
		ID:           id,
		Airline:      req.Airline,
		AircraftMake: req.AircraftMake,
		ModelNumber:  req.ModelNumber,
		SerialNumber: req.SerialNumber,
		Registration: req.Registration,
		CompanyID:    companyID,
		Parameters:   req.Parameters,
		UpdatedAt:    now,
	}

	c.JSON(http.StatusOK, aircraft)
}

// DeleteAircraft deletes an aircraft
func (h *AircraftHandler) DeleteAircraft(c *gin.Context) {
	id := c.Param("id")
	lookupQuery := `SELECT id FROM Aircraft WHERE id = ?`
	args := []interface{}{id}
	if !hasGlobalCompanyAccess(c) {
		companyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		lookupQuery += " AND companyId = ?"
		args = append(args, companyID)
	}
	if err := h.db.QueryRowContext(c.Request.Context(), lookupQuery, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
			return
		}
		respondDatabaseError(c, err)
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	counts, files, err := collectAircraftDeletionData(tx, id)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = deleteAircraftDependencies(tx, id); err != nil {
		respondDatabaseError(c, err)
		return
	}
	result, err := tx.ExecContext(c.Request.Context(), `DELETE FROM Aircraft WHERE id = ?`, id)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if rowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "Aircraft deletion did not complete"})
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	committed = true

	filesDeleted, fileWarning := removeAircraftStoredFiles(files)
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "Aircraft and all related flight data were deleted",
		"deleted":      counts,
		"filesDeleted": filesDeleted,
		"warning":      fileWarning,
	})
}

type aircraftDeletionCounts struct {
	Recordings          int64 `json:"recordings"`
	Flights             int64 `json:"flights"`
	Exceedances         int64 `json:"exceedances"`
	DetectionRuns       int64 `json:"detectionRuns"`
	EventConfigurations int64 `json:"eventConfigurations"`
}

type aircraftStoredFile struct {
	CSV string
	Raw sql.NullString
}

func collectAircraftDeletionData(tx *sql.Tx, aircraftID string) (aircraftDeletionCounts, []aircraftStoredFile, error) {
	var counts aircraftDeletionCounts
	var err error
	if counts.Recordings, err = countAircraftRows(tx, "Csv", "aircraftId", aircraftID); err != nil {
		return counts, nil, err
	}
	flightLegs, err := countAircraftRows(tx, "FlightLeg", "aircraftId", aircraftID)
	if err != nil {
		return counts, nil, err
	}
	legacyFlights, err := countAircraftRows(tx, "Flight", "aircraftId", aircraftID)
	if err != nil {
		return counts, nil, err
	}
	counts.Flights = flightLegs + legacyFlights
	if counts.Exceedances, err = countAircraftRows(tx, "Exceedance", "aircraftId", aircraftID); err != nil {
		return counts, nil, err
	}
	if counts.DetectionRuns, err = countAircraftRows(tx, "DetectionRun", "aircraftId", aircraftID); err != nil {
		return counts, nil, err
	}
	eventLogs, err := countAircraftRows(tx, "EventLog", "aircraftId", aircraftID)
	if err != nil {
		return counts, nil, err
	}
	assignments, err := countAircraftRows(tx, "EventDefinitionAssignment", "aircraftId", aircraftID)
	if err != nil {
		return counts, nil, err
	}
	counts.EventConfigurations = eventLogs + assignments

	csvExists, err := tableExistsTx(tx, "Csv")
	if err != nil || !csvExists {
		return counts, nil, err
	}
	rawColumnExists, err := columnExistsTx(tx, "Csv", "rawSourceFile")
	if err != nil {
		return counts, nil, err
	}
	rawColumn := "NULL"
	if rawColumnExists {
		rawColumn = "rawSourceFile"
	}
	rows, err := tx.Query(fmt.Sprintf(`SELECT file, %s FROM Csv WHERE aircraftId = ?`, rawColumn), aircraftID)
	if err != nil {
		return counts, nil, err
	}
	defer rows.Close()
	files := make([]aircraftStoredFile, 0, counts.Recordings)
	for rows.Next() {
		var file aircraftStoredFile
		if err = rows.Scan(&file.CSV, &file.Raw); err != nil {
			return counts, nil, err
		}
		files = append(files, file)
	}
	return counts, files, rows.Err()
}

func deleteAircraftDependencies(tx *sql.Tx, aircraftID string) error {
	// Delete restrictive children first. Cascades then remove notifications,
	// reviews, locations, run definitions, and phase rows owned by those records.
	for _, target := range []struct {
		table  string
		column string
	}{
		{"Exceedance", "aircraftId"},
		{"DetectionRun", "aircraftId"},
		{"FlightLeg", "aircraftId"},
		{"Csv", "aircraftId"},
		{"Flight", "aircraftId"},
		{"EventLog", "aircraftId"},
		{"EventDefinitionAssignment", "aircraftId"},
		{"AircraftDecoderProfile", "aircraftId"},
	} {
		if _, err := deleteAircraftRows(tx, target.table, target.column, aircraftID); err != nil {
			return fmt.Errorf("delete %s records: %w", target.table, err)
		}
	}

	versionTableExists, err := tableExistsTx(tx, "EventDefinitionVersion")
	if err != nil {
		return err
	}
	if versionTableExists {
		if _, err = tx.Exec(`UPDATE EventDefinitionVersion SET primaryAircraftId = NULL WHERE primaryAircraftId = ?`, aircraftID); err != nil {
			return fmt.Errorf("clear event definition aircraft reference: %w", err)
		}
	}
	return nil
}

func countAircraftRows(tx *sql.Tx, table, column, aircraftID string) (int64, error) {
	exists, err := tableExistsTx(tx, table)
	if err != nil || !exists {
		return 0, err
	}
	var count int64
	if err = tx.QueryRow(fmt.Sprintf(`SELECT COUNT(1) FROM %s WHERE %s = ?`, table, column), aircraftID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func deleteAircraftRows(tx *sql.Tx, table, column, aircraftID string) (int64, error) {
	exists, err := tableExistsTx(tx, table)
	if err != nil || !exists {
		return 0, err
	}
	result, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, table, column), aircraftID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func tableExistsTx(tx *sql.Tx, table string) (bool, error) {
	var exists int
	err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, table).Scan(&exists)
	return exists == 1, err
}

func columnExistsTx(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue interface{}
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func removeAircraftStoredFiles(files []aircraftStoredFile) (int, string) {
	removed := 0
	failed := 0
	for _, file := range files {
		if path, err := storedCSVPath(file.CSV); err == nil {
			if err = os.Remove(path); err == nil {
				removed++
			} else if !errors.Is(err, os.ErrNotExist) {
				failed++
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			failed++
		}
		if file.Raw.Valid {
			if path, err := storedRawRecordingPath(file.Raw.String); err == nil {
				if err = os.Remove(path); err == nil {
					removed++
				} else if !errors.Is(err, os.ErrNotExist) {
					failed++
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				failed++
			}
		}
	}
	if failed > 0 {
		return removed, fmt.Sprintf("Database records were deleted, but %d stored file(s) could not be removed", failed)
	}
	return removed, ""
}

// Helper functions

func (h *AircraftHandler) getAircraftCSVs(aircraftID string, includeUnvalidated bool) ([]models.CSV, error) {
	query := `SELECT f.id, f.name, c.file, f.status, f.departure, f.pilot,
		f.destination, f.flightHours, f.aircraftId, c.sampleIntervalMs,
		f.analysisSummary, f.createdAt, f.updatedAt
		FROM FlightLeg f JOIN Csv c ON c.id = f.recordingId
		WHERE f.aircraftId = ? ORDER BY f.createdAt DESC, f.recordingId, f.legIndex`
	rows, err := h.db.Query(query, aircraftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var csvs []models.CSV
	for rows.Next() {
		var csv models.CSV
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csv.SampleIntervalMs, &csv.AnalysisSummary, &createdAtStr, &updatedAtStr)
		if err != nil {
			continue
		}

		if createdAtStr.Valid {
			if t, err := parseTimestamp(createdAtStr.String); err == nil {
				csv.CreatedAt = t
			}
		}
		if updatedAtStr.Valid {
			if t, err := parseTimestamp(updatedAtStr.String); err == nil {
				csv.UpdatedAt = t
			}
		}

		if !includeUnvalidated {
			csv.AnalysisSummary = customerAnalysisSummary(csv.AnalysisSummary)
		}
		csvs = append(csvs, csv)
	}

	return csvs, nil
}

func (h *AircraftHandler) getAircraftEventLogs(aircraftID string) ([]models.EventLog, error) {
	query := `SELECT id, eventName, displayName, eventCode, eventDescription, eventParameter, eventTrigger, eventType, flightPhase, high, high1, high2, low, low1, low2, triggerType, detectionPeriod, severities, sop, aircraftId, createdAt, updatedAt FROM EventLog WHERE aircraftId = ?`
	rows, err := h.db.Query(query, aircraftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eventLogs []models.EventLog
	for rows.Next() {
		var eventLog models.EventLog
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&eventLog.ID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
			&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
			&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
			&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &createdAtStr, &updatedAtStr)
		if err != nil {
			continue
		}

		if createdAtStr.Valid {
			if t, err := parseTimestamp(createdAtStr.String); err == nil {
				eventLog.CreatedAt = t
			}
		}
		if updatedAtStr.Valid {
			if t, err := parseTimestamp(updatedAtStr.String); err == nil {
				eventLog.UpdatedAt = t
			}
		}

		eventLogs = append(eventLogs, eventLog)
	}

	return eventLogs, nil
}

func (h *AircraftHandler) getAircraftExceedances(aircraftID string, includeUnvalidated bool) ([]models.Exceedance, error) {
	query := `SELECT id, COALESCE(exceedanceValues, ''), COALESCE(flightPhase, ''), COALESCE(parameterName, ''), COALESCE(description, ''), COALESCE(eventStatus, ''), aircraftId, COALESCE(flightLegId, flightId), file, eventId, comment, exceedanceLevel, createdAt, updatedAt FROM Exceedance WHERE aircraftId = ? AND isCurrent = 1`
	if !includeUnvalidated {
		query += " AND eventStatus = 'Valid'"
	}
	rows, err := h.db.Query(query, aircraftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exceedances := make([]models.Exceedance, 0)
	for rows.Next() {
		var exceedance models.Exceedance
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &createdAtStr, &updatedAtStr)
		if err != nil {
			continue
		}

		if createdAtStr.Valid {
			if t, err := parseTimestamp(createdAtStr.String); err == nil {
				exceedance.CreatedAt = t
			}
		}
		if updatedAtStr.Valid {
			if t, err := parseTimestamp(updatedAtStr.String); err == nil {
				exceedance.UpdatedAt = t
			}
		}

		exceedances = append(exceedances, exceedance)
	}

	return exceedances, nil
}

func (h *AircraftHandler) getAircraftCompany(companyID string) (*models.Company, error) {
	var company models.Company
	query := `SELECT id, name, email, phone, address, country, logo, status, subscriptionId, createdAt, updatedAt FROM Company WHERE id = ?`

	var createdAtStr, updatedAtStr sql.NullString
	err := h.db.QueryRow(query, companyID).Scan(
		&company.ID, &company.Name, &company.Email, &company.Phone,
		&company.Address, &company.Country, &company.Logo, &company.Status,
		&company.SubscriptionID, &createdAtStr, &updatedAtStr)

	if err != nil {
		return nil, err
	}

	if createdAtStr.Valid {
		if t, err := parseTimestamp(createdAtStr.String); err == nil {
			company.CreatedAt = t
		}
	}
	if updatedAtStr.Valid {
		if t, err := parseTimestamp(updatedAtStr.String); err == nil {
			company.UpdatedAt = t
		}
	}

	// Get primary user (gatekeeper or first active user) for notification purposes
	var user models.User
	userQuery := `SELECT id, email, fullName, role FROM User WHERE companyId = ? AND isActive = 1 ORDER BY 
		CASE role 
			WHEN 'gatekeeper' THEN 1 
			WHEN 'user' THEN 2 
			ELSE 3 
		END 
		LIMIT 1`

	var fullName sql.NullString
	userErr := h.db.QueryRow(userQuery, companyID).Scan(&user.ID, &user.Email, &fullName, &user.Role)
	if userErr == nil {
		if fullName.Valid {
			user.FullName = &fullName.String
		}
		// Add single user to company (for notification purposes)
		company.Users = []models.User{user}
	}

	return &company, nil
}
