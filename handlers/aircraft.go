package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
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

// Helper function to parse timestamp strings from SQLite
func parseTimestamp(timeStr string) (time.Time, error) {
	// Try Go's time.Time string format first (what's in the DB)
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", timeStr); err == nil {
		return t, nil
	}
	// Fallback to RFC3339
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}
	// Fallback to simple datetime format
	if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
		return t, nil
	}
	return time.Time{}, nil
}

// GetAircrafts retrieves all aircraft with related data
func (h *AircraftHandler) GetAircrafts(c *gin.Context) {
	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, companyId, parameters, createdAt, updatedAt FROM Aircraft`
	rows, err := h.db.Query(query)
	if err != nil {
		println("GetAircrafts query error:", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, parameters sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &aircraft.CompanyID, &parameters, &createdAtStr, &updatedAtStr)
		if err != nil {
			println("GetAircrafts scan error:", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning aircraft", "details": err.Error()})
			return
		}

		// Handle nullable fields
		if modelNumber.Valid {
			aircraft.ModelNumber = &modelNumber.String
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
		csvs, err := h.getAircraftCSVs(aircraft.ID)
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
		exceedances, err := h.getAircraftExceedances(aircraft.ID)
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

	var aircraft models.Aircraft
	var modelNumber, registration, parameters sql.NullString
	var createdAtStr, updatedAtStr sql.NullString

	err := h.db.QueryRow(query, aircraftID).Scan(
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
	csvs, _ := h.getAircraftCSVs(aircraft.ID)

	// Get related event logs
	eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

	// Get related exceedances
	exceedances, _ := h.getAircraftExceedances(aircraft.ID)

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

	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, companyId, parameters, createdAt, updatedAt FROM Aircraft WHERE companyId = ?`
	rows, err := h.db.Query(query, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, parameters sql.NullString
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &aircraft.CompanyID, &parameters, &createdAtStr, &updatedAtStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning aircraft"})
			return
		}

		// Handle nullable fields
		if modelNumber.Valid {
			aircraft.ModelNumber = &modelNumber.String
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
		csvs, _ := h.getAircraftCSVs(aircraft.ID)

		// Get related event logs
		eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

		// Get related exceedances
		exceedances, _ := h.getAircraftExceedances(aircraft.ID)

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

	// Generate ID and timestamps
	id := uuid.New().String()
	now := time.Now()

	// Insert aircraft
	query := `INSERT INTO Aircraft (id, airline, aircraftMake, serialNumber, companyId, parameters, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := h.db.Exec(query, id, req.Airline, req.AircraftMake, req.SerialNumber, req.CompanyID, req.Parameters, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating aircraft"})
		return
	}

	// Return created aircraft
	aircraft := models.Aircraft{
		ID:           id,
		Airline:      req.Airline,
		AircraftMake: req.AircraftMake,
		SerialNumber: req.SerialNumber,
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

	now := time.Now()

	query := `UPDATE Aircraft SET airline = ?, aircraftMake = ?, serialNumber = ?, companyId = ?, parameters = ?, updatedAt = ? WHERE id = ?`
	result, err := h.db.Exec(query, req.Airline, req.AircraftMake, req.SerialNumber, req.CompanyID, req.Parameters, now.UnixMilli(), id)
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
		SerialNumber: req.SerialNumber,
		CompanyID:    req.CompanyID,
		Parameters:   req.Parameters,
		UpdatedAt:    now,
	}

	c.JSON(http.StatusOK, aircraft)
}

// DeleteAircraft deletes an aircraft
func (h *AircraftHandler) DeleteAircraft(c *gin.Context) {
	id := c.Param("id")

	query := `DELETE FROM Aircraft WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting aircraft"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Aircraft deleted successfully"})
}

// Helper functions

func (h *AircraftHandler) getAircraftCSVs(aircraftID string) ([]models.CSV, error) {
	query := `SELECT id, name, file, status, departure, pilot, destination, flightHours, aircraftId, createdAt, updatedAt FROM Csv WHERE aircraftId = ?`
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
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtStr, &updatedAtStr)
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

func (h *AircraftHandler) getAircraftExceedances(aircraftID string) ([]models.Exceedance, error) {
	query := `SELECT id, exceedanceValues, flightPhase, parameterName, description, eventStatus, aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt FROM Exceedance WHERE aircraftId = ?`
	rows, err := h.db.Query(query, aircraftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exceedances []models.Exceedance
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
