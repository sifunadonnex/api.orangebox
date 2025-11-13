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
			  el.id as eventlog_id, el.eventName, el.displayName, el.eventCode, el.eventDescription, el.eventParameter, el.eventTrigger, el.eventType, el.flightPhase as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, el.sop, el.aircraftId as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, c.name, c.file as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, co.name as company_name, co.email as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, co.status as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN Csv c ON e.flightId = c.id 
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id
			  LEFT JOIN Company co ON a.companyId = co.id`

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
		var company models.Company

		// Nullable fields for joins
		var eventLogID sql.NullString
		var eventLogCreatedAt, eventLogUpdatedAt sql.NullTime
		var csvID sql.NullString
		var csvCreatedAt, csvUpdatedAt sql.NullTime
		var aircraftID sql.NullString
		var aircraftCreatedAt, aircraftUpdatedAt sql.NullTime
		var exceedanceCreatedAt, exceedanceUpdatedAt sql.NullTime
		var companyID sql.NullString
		var companyCreatedAt, companyUpdatedAt sql.NullTime

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedanceCreatedAt, &exceedanceUpdatedAt,
			&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
			&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
			&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
			&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAt, &eventLogUpdatedAt,
			&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAt, &csvUpdatedAt,
			&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
			&aircraft.SerialNumber, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAt, &aircraftUpdatedAt,
			&companyID, &company.Name, &company.Email, &company.Phone, &company.Address, &company.Country, &company.Logo, &company.Status, &company.SubscriptionID, &companyCreatedAt, &companyUpdatedAt)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning exceedance"})
			return
		}

		// Handle exceedance timestamps
		if exceedanceCreatedAt.Valid {
			exceedance.CreatedAt = exceedanceCreatedAt.Time
		}
		if exceedanceUpdatedAt.Valid {
			exceedance.UpdatedAt = exceedanceUpdatedAt.Time
		}

		// Handle nullable eventlog
		var eventLogPtr *models.EventLog
		if eventLogID.Valid {
			eventLog.ID = eventLogID.String
			if eventLogCreatedAt.Valid {
				eventLog.CreatedAt = eventLogCreatedAt.Time
			}
			if eventLogUpdatedAt.Valid {
				eventLog.UpdatedAt = eventLogUpdatedAt.Time
			}
			eventLogPtr = &eventLog
		}

		// Handle csv
		if csvID.Valid {
			csv.ID = csvID.String
			if csvCreatedAt.Valid {
				csv.CreatedAt = csvCreatedAt.Time
			}
			if csvUpdatedAt.Valid {
				csv.UpdatedAt = csvUpdatedAt.Time
			}
		}

		// Handle aircraft
		if aircraftID.Valid {
			aircraft.ID = aircraftID.String
			if aircraftCreatedAt.Valid {
				aircraft.CreatedAt = aircraftCreatedAt.Time
			}
			if aircraftUpdatedAt.Valid {
				aircraft.UpdatedAt = aircraftUpdatedAt.Time
			}
		}

		// Handle company
		var companyPtr *models.Company
		if companyID.Valid {
			company.ID = companyID.String
			if companyCreatedAt.Valid {
				company.CreatedAt = companyCreatedAt.Time
			}
			if companyUpdatedAt.Valid {
				company.UpdatedAt = companyUpdatedAt.Time
			}
			companyPtr = &company
		}

		// Add company to aircraft if available
		if companyPtr != nil {
			aircraft.Company = companyPtr
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
			  el.id as eventlog_id, el.eventName, el.displayName, el.eventCode, el.eventDescription, el.eventParameter, el.eventTrigger, el.eventType, el.flightPhase as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, el.sop, el.aircraftId as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, c.name, c.file as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, co.name as company_name, co.email as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, co.status as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN Csv c ON e.flightId = c.id 
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id 
			  LEFT JOIN Company co ON a.companyId = co.id
			  WHERE e.id = ?`

	var exceedance models.Exceedance
	var eventLog models.EventLog
	var csv models.CSV
	var aircraft models.Aircraft
	var company models.Company

	// Nullable fields for joins
	var eventLogID sql.NullString
	var eventLogCreatedAtUnix, eventLogUpdatedAtUnix sql.NullInt64
	var csvID sql.NullString
	var csvCreatedAtUnix, csvUpdatedAtUnix sql.NullInt64
	var aircraftID sql.NullString
	var aircraftCreatedAtUnix, aircraftUpdatedAtUnix sql.NullInt64
	var exceedanceCreatedAtUnix, exceedanceUpdatedAtUnix sql.NullInt64
	var companyID sql.NullString
	var companyCreatedAtUnix, companyUpdatedAtUnix sql.NullInt64

	row := h.db.QueryRow(query, id)
	err := row.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
		&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
		&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
		&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedanceCreatedAtUnix, &exceedanceUpdatedAtUnix,
		&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
		&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
		&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
		&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAtUnix, &eventLogUpdatedAtUnix,
		&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAtUnix, &csvUpdatedAtUnix,
		&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAtUnix, &aircraftUpdatedAtUnix,
		&companyID, &company.Name, &company.Email, &company.Phone, &company.Address, &company.Country, &company.Logo, &company.Status, &company.SubscriptionID, &companyCreatedAtUnix, &companyUpdatedAtUnix)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Handle exceedance timestamps
	if exceedanceCreatedAtUnix.Valid {
		exceedance.CreatedAt = time.Unix(exceedanceCreatedAtUnix.Int64/1000, 0)
	}
	if exceedanceUpdatedAtUnix.Valid {
		exceedance.UpdatedAt = time.Unix(exceedanceUpdatedAtUnix.Int64/1000, 0)
	}

	// Handle nullable eventlog
	var eventLogPtr *models.EventLog
	if eventLogID.Valid {
		eventLog.ID = eventLogID.String
		if eventLogCreatedAtUnix.Valid {
			eventLog.CreatedAt = time.Unix(eventLogCreatedAtUnix.Int64/1000, 0)
		}
		if eventLogUpdatedAtUnix.Valid {
			eventLog.UpdatedAt = time.Unix(eventLogUpdatedAtUnix.Int64/1000, 0)
		}
		eventLogPtr = &eventLog
	}

	// Handle csv
	if csvID.Valid {
		csv.ID = csvID.String
		if csvCreatedAtUnix.Valid {
			csv.CreatedAt = time.Unix(csvCreatedAtUnix.Int64/1000, 0)
		}
		if csvUpdatedAtUnix.Valid {
			csv.UpdatedAt = time.Unix(csvUpdatedAtUnix.Int64/1000, 0)
		}
	}

	// Handle aircraft
	if aircraftID.Valid {
		aircraft.ID = aircraftID.String
		if aircraftCreatedAtUnix.Valid {
			aircraft.CreatedAt = time.Unix(aircraftCreatedAtUnix.Int64/1000, 0)
		}
		if aircraftUpdatedAtUnix.Valid {
			aircraft.UpdatedAt = time.Unix(aircraftUpdatedAtUnix.Int64/1000, 0)
		}
	}

	// Handle company
	var companyPtr *models.Company
	if companyID.Valid {
		company.ID = companyID.String
		if companyCreatedAtUnix.Valid {
			company.CreatedAt = time.Unix(companyCreatedAtUnix.Int64/1000, 0)
		}
		if companyUpdatedAtUnix.Valid {
			company.UpdatedAt = time.Unix(companyUpdatedAtUnix.Int64/1000, 0)
		}
		companyPtr = &company
	}

	// Add company to aircraft if available
	if companyPtr != nil {
		aircraft.Company = companyPtr
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
		var createdAtUnix, updatedAtUnix sql.NullInt64

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			continue
		}

		// Convert Unix timestamps to time.Time
		if createdAtUnix.Valid {
			exceedance.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			exceedance.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
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
			exceedance.Comment, exceedance.ExceedanceLevel, now.UnixMilli(), now.UnixMilli())

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
	result, err := h.db.Exec(query, req.Comment, req.EventStatus, now.UnixMilli(), id)
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
