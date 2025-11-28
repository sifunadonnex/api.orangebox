package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"log"
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
	query := `SELECT e.id, COALESCE(e.exceedanceValues, '') as exceedanceValues, COALESCE(e.flightPhase, '') as flightPhase, COALESCE(e.parameterName, '') as parameterName, COALESCE(e.description, '') as description, COALESCE(e.eventStatus, '') as eventStatus, COALESCE(e.aircraftId, '') as aircraftId, COALESCE(e.flightId, '') as flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, COALESCE(el.displayName, '') as displayName, COALESCE(el.eventCode, '') as eventCode, COALESCE(el.eventDescription, '') as eventDescription, COALESCE(el.eventParameter, '') as eventParameter, COALESCE(el.eventTrigger, '') as eventTrigger, COALESCE(el.eventType, '') as eventType, COALESCE(el.flightPhase, '') as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, COALESCE(el.sop, '') as sop, COALESCE(el.aircraftId, '') as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, COALESCE(c.name, '') as name, COALESCE(c.file, '') as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, COALESCE(c.aircraftId, '') as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, COALESCE(a.airline, '') as airline, COALESCE(a.aircraftMake, '') as aircraftMake, a.modelNumber, COALESCE(a.serialNumber, '') as serialNumber, COALESCE(a.companyId, '') as companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, COALESCE(co.name, '') as company_name, COALESCE(co.email, '') as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, COALESCE(co.status, '') as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
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
		var eventLogCreatedAt, eventLogUpdatedAt sql.NullInt64
		var csvID sql.NullString
		var csvCreatedAt, csvUpdatedAt sql.NullTime
		var aircraftID sql.NullString
		var aircraftCreatedAt, aircraftUpdatedAt sql.NullInt64
		var exceedanceCreatedAt, exceedanceUpdatedAt sql.NullInt64
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
			log.Println("Error scanning exceedance:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning exceedance"})
			return
		}

		// Handle exceedance timestamps
		if exceedanceCreatedAt.Valid {
			exceedance.CreatedAt = time.UnixMilli(exceedanceCreatedAt.Int64)
		}
		if exceedanceUpdatedAt.Valid {
			exceedance.UpdatedAt = time.UnixMilli(exceedanceUpdatedAt.Int64)
		}

		// Handle nullable eventlog
		var eventLogPtr *models.EventLog
		if eventLogID.Valid {
			eventLog.ID = eventLogID.String
			if eventLogCreatedAt.Valid {
				eventLog.CreatedAt = time.UnixMilli(eventLogCreatedAt.Int64)
			}
			if eventLogUpdatedAt.Valid {
				eventLog.UpdatedAt = time.UnixMilli(eventLogUpdatedAt.Int64)
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
				aircraft.CreatedAt = time.UnixMilli(aircraftCreatedAt.Int64)
			}
			if aircraftUpdatedAt.Valid {
				aircraft.UpdatedAt = time.UnixMilli(aircraftUpdatedAt.Int64)
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

	query := `SELECT e.id, COALESCE(e.exceedanceValues, '') as exceedanceValues, COALESCE(e.flightPhase, '') as flightPhase, COALESCE(e.parameterName, '') as parameterName, COALESCE(e.description, '') as description, COALESCE(e.eventStatus, '') as eventStatus, COALESCE(e.aircraftId, '') as aircraftId, COALESCE(e.flightId, '') as flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, COALESCE(el.displayName, '') as displayName, COALESCE(el.eventCode, '') as eventCode, COALESCE(el.eventDescription, '') as eventDescription, COALESCE(el.eventParameter, '') as eventParameter, COALESCE(el.eventTrigger, '') as eventTrigger, COALESCE(el.eventType, '') as eventType, COALESCE(el.flightPhase, '') as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, COALESCE(el.sop, '') as sop, COALESCE(el.aircraftId, '') as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  c.id as csv_id, COALESCE(c.name, '') as name, COALESCE(c.file, '') as csv_file, c.status, c.departure, c.pilot, c.destination, c.flightHours, COALESCE(c.aircraftId, '') as csv_aircraftId, c.createdAt as csv_createdAt, c.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, COALESCE(a.airline, '') as airline, COALESCE(a.aircraftMake, '') as aircraftMake, a.modelNumber, COALESCE(a.serialNumber, '') as serialNumber, COALESCE(a.companyId, '') as companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, COALESCE(co.name, '') as company_name, COALESCE(co.email, '') as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, COALESCE(co.status, '') as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
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
	var eventLogCreatedAtStr, eventLogUpdatedAtStr sql.NullString
	var csvID sql.NullString
	var csvCreatedAtStr, csvUpdatedAtStr sql.NullString
	var aircraftID sql.NullString
	var aircraftCreatedAtStr, aircraftUpdatedAtStr sql.NullString
	var exceedanceCreatedAtStr, exceedanceUpdatedAtStr sql.NullString
	var companyID sql.NullString
	var companyCreatedAtStr, companyUpdatedAtStr sql.NullString

	row := h.db.QueryRow(query, id)
	err := row.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
		&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
		&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
		&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedanceCreatedAtStr, &exceedanceUpdatedAtStr,
		&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
		&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
		&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
		&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAtStr, &eventLogUpdatedAtStr,
		&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAtStr, &csvUpdatedAtStr,
		&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAtStr, &aircraftUpdatedAtStr,
		&companyID, &company.Name, &company.Email, &company.Phone, &company.Address, &company.Country, &company.Logo, &company.Status, &company.SubscriptionID, &companyCreatedAtStr, &companyUpdatedAtStr)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	}
	if err != nil {
		log.Println("Error scanning exceedance:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Handle exceedance timestamps
	if exceedanceCreatedAtStr.Valid {
		parsedTime, err := parseTimestamp(exceedanceCreatedAtStr.String)
		if err == nil {
			exceedance.CreatedAt = parsedTime
		}
	}
	if exceedanceUpdatedAtStr.Valid {
		parsedTime, err := parseTimestamp(exceedanceUpdatedAtStr.String)
		if err == nil {
			exceedance.UpdatedAt = parsedTime
		}
	}

	// Handle nullable eventlog
	var eventLogPtr *models.EventLog
	if eventLogID.Valid {
		eventLog.ID = eventLogID.String
		if eventLogCreatedAtStr.Valid {
			parsedTime, err := parseTimestamp(eventLogCreatedAtStr.String)
			if err == nil {
				eventLog.CreatedAt = parsedTime
			}
		}
		if eventLogUpdatedAtStr.Valid {
			parsedTime, err := parseTimestamp(eventLogUpdatedAtStr.String)
			if err == nil {
				eventLog.UpdatedAt = parsedTime
			}
		}
		eventLogPtr = &eventLog
	}

	// Handle csv
	if csvID.Valid {
		csv.ID = csvID.String
		if csvCreatedAtStr.Valid {
			parsedTime, err := parseTimestamp(csvCreatedAtStr.String)
			if err == nil {
				csv.CreatedAt = parsedTime
			}
		}
		if csvUpdatedAtStr.Valid {
			parsedTime, err := parseTimestamp(csvUpdatedAtStr.String)
			if err == nil {
				csv.UpdatedAt = parsedTime
			}
		}
	}

	// Handle aircraft
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
		var createdAtStr, updatedAtStr sql.NullString

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &createdAtStr, &updatedAtStr)
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
