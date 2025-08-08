package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ExceedanceHandler struct {
	db *sql.DB
}

func NewExceedanceHandler(db *sql.DB) *ExceedanceHandler {
	return &ExceedanceHandler{db: db}
}

// GetExceedances retrieves all exceedances with related data
func (h *ExceedanceHandler) GetExceedances(c *gin.Context) {
	query := `SELECT e.id, e.exceedanceValues, e.flightPhase, e.parameterName, e.description, e.eventStatus, e.aircraftId, e.flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, el.displayName, el.eventCode, el.eventDescription, el.eventParameter, el.eventTrigger, el.eventType, el.flightPhase as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.sop, el.aircraftId as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, c.name, c.file as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.userId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN Csv c ON e.flightId = c.id 
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id`
	
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var exceedances []interface{}
	for rows.Next() {
		var exceedance models.Exceedance
		var eventLog models.EventLog
		var csv models.CSV
		var aircraft models.Aircraft
		
		// Nullable fields for joins
		var eventLogID, eventLogCreatedAt, eventLogUpdatedAt sql.NullString
		var csvID, csvCreatedAt, csvUpdatedAt sql.NullString
		var aircraftID, aircraftCreatedAt, aircraftUpdatedAt sql.NullString

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.CreatedAt, &exceedance.UpdatedAt,
			&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
			&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
			&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
			&eventLog.Low1, &eventLog.Low2, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAt, &eventLogUpdatedAt,
			&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAt, &csvUpdatedAt,
			&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
			&aircraft.SerialNumber, &aircraft.UserID, &aircraft.Parameters, &aircraftCreatedAt, &aircraftUpdatedAt)
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning exceedance"})
			return
		}

		// Handle nullable eventlog
		var eventLogPtr *models.EventLog
		if eventLogID.Valid {
			eventLog.ID = eventLogID.String
			if eventLogCreatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, eventLogCreatedAt.String); err == nil {
					eventLog.CreatedAt = t
				}
			}
			if eventLogUpdatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, eventLogUpdatedAt.String); err == nil {
					eventLog.UpdatedAt = t
				}
			}
			eventLogPtr = &eventLog
		}

		// Handle csv
		if csvID.Valid {
			csv.ID = csvID.String
			if csvCreatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, csvCreatedAt.String); err == nil {
					csv.CreatedAt = t
				}
			}
			if csvUpdatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, csvUpdatedAt.String); err == nil {
					csv.UpdatedAt = t
				}
			}
		}

		// Handle aircraft
		if aircraftID.Valid {
			aircraft.ID = aircraftID.String
			if aircraftCreatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, aircraftCreatedAt.String); err == nil {
					aircraft.CreatedAt = t
				}
			}
			if aircraftUpdatedAt.Valid {
				if t, err := time.Parse(time.RFC3339, aircraftUpdatedAt.String); err == nil {
					aircraft.UpdatedAt = t
				}
			}
		}

		exceedanceWithRelations := struct {
			models.Exceedance
			EventLog *models.EventLog `json:"eventlog"`
			CSV      models.CSV       `json:"csv"`
			Aircraft models.Aircraft  `json:"aircraft"`
		}{
			Exceedance: exceedance,
			EventLog:   eventLogPtr,
			CSV:        csv,
			Aircraft:   aircraft,
		}

		exceedances = append(exceedances, exceedanceWithRelations)
	}

	c.JSON(http.StatusOK, exceedances)
}

// GetExceedanceByID retrieves an exceedance by ID with related data
func (h *ExceedanceHandler) GetExceedanceByID(c *gin.Context) {
	id := c.Param("id")
	
	query := `SELECT e.id, e.exceedanceValues, e.flightPhase, e.parameterName, e.description, e.eventStatus, e.aircraftId, e.flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, el.displayName, el.eventCode, el.eventDescription, el.eventParameter, el.eventTrigger, el.eventType, el.flightPhase as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.sop, el.aircraftId as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, c.name, c.file as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.userId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN Csv c ON e.flightId = c.id 
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id 
			  WHERE e.id = ?`
	
	var exceedance models.Exceedance
	var eventLog models.EventLog
	var csv models.CSV
	var aircraft models.Aircraft
	
	// Nullable fields for joins
	var eventLogID, eventLogCreatedAt, eventLogUpdatedAt sql.NullString
	var csvID, csvCreatedAt, csvUpdatedAt sql.NullString
	var aircraftID, aircraftCreatedAt, aircraftUpdatedAt sql.NullString

	row := h.db.QueryRow(query, id)
	err := row.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
		&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
		&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
		&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.CreatedAt, &exceedance.UpdatedAt,
		&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
		&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
		&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
		&eventLog.Low1, &eventLog.Low2, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAt, &eventLogUpdatedAt,
		&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAt, &csvUpdatedAt,
		&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.UserID, &aircraft.Parameters, &aircraftCreatedAt, &aircraftUpdatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Handle nullable eventlog
	var eventLogPtr *models.EventLog
	if eventLogID.Valid {
		eventLog.ID = eventLogID.String
		if eventLogCreatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, eventLogCreatedAt.String); err == nil {
				eventLog.CreatedAt = t
			}
		}
		if eventLogUpdatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, eventLogUpdatedAt.String); err == nil {
				eventLog.UpdatedAt = t
			}
		}
		eventLogPtr = &eventLog
	}

	// Handle csv
	if csvID.Valid {
		csv.ID = csvID.String
		if csvCreatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, csvCreatedAt.String); err == nil {
				csv.CreatedAt = t
			}
		}
		if csvUpdatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, csvUpdatedAt.String); err == nil {
				csv.UpdatedAt = t
			}
		}
	}

	// Handle aircraft
	if aircraftID.Valid {
		aircraft.ID = aircraftID.String
		if aircraftCreatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, aircraftCreatedAt.String); err == nil {
				aircraft.CreatedAt = t
			}
		}
		if aircraftUpdatedAt.Valid {
			if t, err := time.Parse(time.RFC3339, aircraftUpdatedAt.String); err == nil {
				aircraft.UpdatedAt = t
			}
		}
	}

	response := struct {
		models.Exceedance
		EventLog *models.EventLog `json:"eventlog"`
		CSV      models.CSV       `json:"csv"`
		Aircraft models.Aircraft  `json:"aircraft"`
	}{
		Exceedance: exceedance,
		EventLog:   eventLogPtr,
		CSV:        csv,
		Aircraft:   aircraft,
	}

	c.JSON(http.StatusOK, response)
}

// GetExceedancesByFlightID retrieves exceedances by flight ID
func (h *ExceedanceHandler) GetExceedancesByFlightID(c *gin.Context) {
	flightID := c.Param("id")
	
	query := `SELECT id, exceedanceValues, flightPhase, parameterName, description, eventStatus, aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt FROM Exceedance WHERE flightId = ?`
	rows, err := h.db.Query(query, flightID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var exceedances []models.Exceedance
	for rows.Next() {
		var exceedance models.Exceedance
		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.CreatedAt, &exceedance.UpdatedAt)
		if err != nil {
			continue
		}
		exceedances = append(exceedances, exceedance)
	}

	c.JSON(http.StatusOK, exceedances)
}

// CreateExceedances creates multiple exceedances
func (h *ExceedanceHandler) CreateExceedances(c *gin.Context) {
	var exceedances []models.Exceedance
	if err := c.ShouldBindJSON(&exceedances); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var createdExceedances []models.Exceedance
	now := time.Now()

	for _, exceedance := range exceedances {
		// Generate ID
		id := uuid.New().String()
		
		query := `INSERT INTO Exceedance (id, exceedanceValues, flightPhase, parameterName, description, eventStatus, aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt) 
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		
		_, err := h.db.Exec(query, id, exceedance.ExceedanceValues, exceedance.FlightPhase,
			exceedance.ParameterName, exceedance.Description, exceedance.EventStatus,
			exceedance.AircraftID, exceedance.FlightID, exceedance.File, exceedance.EventID,
			exceedance.Comment, exceedance.ExceedanceLevel, now, now)
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating exceedance"})
			return
		}

		// Set the generated values
		exceedance.ID = id
		exceedance.CreatedAt = now
		exceedance.UpdatedAt = now
		
		createdExceedances = append(createdExceedances, exceedance)
	}

	c.JSON(http.StatusOK, createdExceedances)
}

// UpdateExceedance updates an existing exceedance
func (h *ExceedanceHandler) UpdateExceedance(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateExceedanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()

	query := `UPDATE Exceedance SET comment = ?, eventStatus = ?, updatedAt = ? WHERE id = ?`
	result, err := h.db.Exec(query, req.Comment, req.EventStatus, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating exceedance"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	}

	// Return updated exceedance
	h.GetExceedanceByID(c)
}

// DeleteExceedance deletes an exceedance
func (h *ExceedanceHandler) DeleteExceedance(c *gin.Context) {
	id := c.Param("id")
	
	query := `DELETE FROM Exceedance WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting exceedance"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Exceedance deleted successfully"})
}
