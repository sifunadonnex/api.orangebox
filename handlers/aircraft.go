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

// GetAircrafts retrieves all aircraft with related data
func (h *AircraftHandler) GetAircrafts(c *gin.Context) {
	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, userId, parameters, createdAt, updatedAt FROM Aircraft`
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, parameters sql.NullString
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &aircraft.UserID, &parameters, &createdAtUnix, &updatedAtUnix)
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
		if createdAtUnix.Valid {
			aircraft.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			aircraft.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}

		// Get related CSV files
		csvs, _ := h.getAircraftCSVs(aircraft.ID)

		// Get related event logs
		eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

		// Get related exceedances
		exceedances, _ := h.getAircraftExceedances(aircraft.ID)

		aircraftWithRelations := struct {
			models.Aircraft
			CSV        []models.CSV        `json:"csv"`
			EventLog   []models.EventLog   `json:"EventLog"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			Aircraft:   aircraft,
			CSV:        csvs,
			EventLog:   eventLogs,
			Exceedance: exceedances,
		}

		aircrafts = append(aircrafts, aircraftWithRelations)
	}

	c.JSON(http.StatusOK, aircrafts)
}

// GetAircraftsByUserID retrieves aircraft by user ID
func (h *AircraftHandler) GetAircraftsByUserID(c *gin.Context) {
	userID := c.Param("id")

	query := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, userId, parameters, createdAt, updatedAt FROM Aircraft WHERE userId = ?`
	rows, err := h.db.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var aircrafts []interface{}
	for rows.Next() {
		var aircraft models.Aircraft
		var modelNumber, parameters sql.NullString
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &modelNumber,
			&aircraft.SerialNumber, &aircraft.UserID, &parameters, &createdAtUnix, &updatedAtUnix)
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
		if createdAtUnix.Valid {
			aircraft.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			aircraft.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning aircraft"})
			return
		}

		// Get related CSV files
		csvs, _ := h.getAircraftCSVs(aircraft.ID)

		// Get related event logs
		eventLogs, _ := h.getAircraftEventLogs(aircraft.ID)

		// Get related exceedances
		exceedances, _ := h.getAircraftExceedances(aircraft.ID)

		aircraftWithRelations := struct {
			models.Aircraft
			CSV        []models.CSV        `json:"csv"`
			EventLog   []models.EventLog   `json:"EventLog"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			Aircraft:   aircraft,
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
	query := `INSERT INTO Aircraft (id, airline, aircraftMake, serialNumber, userId, parameters, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := h.db.Exec(query, id, req.Airline, req.AircraftMake, req.SerialNumber, req.User, req.Parameters, now.UnixMilli(), now.UnixMilli())
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
		UserID:       req.User,
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

	query := `UPDATE Aircraft SET airline = ?, aircraftMake = ?, serialNumber = ?, userId = ?, parameters = ?, updatedAt = ? WHERE id = ?`
	result, err := h.db.Exec(query, req.Airline, req.AircraftMake, req.SerialNumber, req.User, req.Parameters, now.UnixMilli(), id)
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
		UserID:       req.User,
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
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			continue
		}

		if createdAtUnix.Valid {
			csv.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			csv.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
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
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&eventLog.ID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
			&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
			&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
			&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			continue
		}

		if createdAtUnix.Valid {
			eventLog.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			eventLog.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
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
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			continue
		}

		if createdAtUnix.Valid {
			exceedance.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			exceedance.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}

		exceedances = append(exceedances, exceedance)
	}

	return exceedances, nil
}
