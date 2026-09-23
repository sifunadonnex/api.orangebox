package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"log"
	"net/http"
	"sort"
	"strings"
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

// accessFilter keeps tenant and visibility rules in the database query. The
// frontend may narrow results further, but it is never the security boundary.
func accessFilter(c *gin.Context) (string, []interface{}, bool) {
	roleValue, roleExists := c.Get("userRole")
	role, roleOK := roleValue.(string)
	if !roleExists || !roleOK {
		return "", nil, false
	}

	if role == models.RoleAdmin || role == models.RoleFDA {
		return "", nil, true
	}
	companyID, companyOK := tenantCompanyID(c)
	if !companyOK {
		return "", nil, false
	}
	return " AND a.companyId = ? AND e.eventStatus = ?", []interface{}{companyID, models.ExceedanceStatusValid}, true
}

func isValidExceedanceStatus(status string) bool {
	switch status {
	case models.ExceedanceStatusPending, models.ExceedanceStatusValid, models.ExceedanceStatusNuisance, models.ExceedanceStatusFalse:
		return true
	default:
		return false
	}
}

// GetExceedances retrieves all exceedances with related data
func (h *ExceedanceHandler) GetExceedances(c *gin.Context) {
	query := `SELECT e.id, COALESCE(e.exceedanceValues, '') as exceedanceValues, COALESCE(e.flightPhase, '') as flightPhase, COALESCE(e.parameterName, '') as parameterName, COALESCE(e.description, '') as description, COALESCE(e.eventStatus, '') as eventStatus, COALESCE(e.aircraftId, '') as aircraftId, COALESCE(e.flightLegId, e.flightId, '') as flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.detectionRunId, e.startTimeMs, e.endTimeMs, e.durationMs, e.peakValue, e.ruleHash, e.isCurrent, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, COALESCE(el.displayName, '') as displayName, COALESCE(el.eventCode, '') as eventCode, COALESCE(el.eventDescription, '') as eventDescription, COALESCE(el.eventParameter, '') as eventParameter, COALESCE(el.eventTrigger, '') as eventTrigger, COALESCE(el.eventType, '') as eventType, COALESCE(el.flightPhase, '') as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, COALESCE(el.sop, '') as sop, COALESCE(el.aircraftId, '') as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  f.id as csv_id, COALESCE(f.name, '') as name, COALESCE(c.file, '') as csv_file, f.status, f.departure, f.pilot, f.destination, f.flightHours, COALESCE(f.aircraftId, '') as csv_aircraftId, f.createdAt as csv_createdAt, f.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, COALESCE(a.airline, '') as airline, COALESCE(a.aircraftMake, '') as aircraftMake, a.modelNumber, COALESCE(a.serialNumber, '') as serialNumber, a.registration, COALESCE(a.companyId, '') as companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, COALESCE(co.name, '') as company_name, COALESCE(co.email, '') as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, COALESCE(co.status, '') as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN FlightLeg f ON COALESCE(e.flightLegId, e.flightId) = f.id
			  LEFT JOIN Csv c ON f.recordingId = c.id
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id
			  LEFT JOIN Company co ON a.companyId = co.id
			  WHERE e.isCurrent = 1`

	filter, args, allowed := accessFilter(c)
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "No company access is assigned to this account"})
		return
	}
	query += filter + " ORDER BY e.createdAt DESC"

	rows, err := h.db.Query(query, args...)
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
		var eventLogCreatedAt, eventLogUpdatedAt nullableTimestamp
		var csvID sql.NullString
		var csvCreatedAt, csvUpdatedAt nullableTimestamp
		var aircraftID sql.NullString
		var aircraftCreatedAt, aircraftUpdatedAt nullableTimestamp
		var exceedanceCreatedAt, exceedanceUpdatedAt nullableTimestamp
		var companyID sql.NullString
		var companyCreatedAt, companyUpdatedAt nullableTimestamp

		err := rows.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
			&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
			&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
			&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.DetectionRunID,
			&exceedance.StartTimeMs, &exceedance.EndTimeMs, &exceedance.DurationMs,
			&exceedance.PeakValue, &exceedance.RuleHash, &exceedance.IsCurrent, &exceedanceCreatedAt, &exceedanceUpdatedAt,
			&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
			&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
			&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
			&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAt, &eventLogUpdatedAt,
			&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAt, &csvUpdatedAt,
			&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
			&aircraft.SerialNumber, &aircraft.Registration, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAt, &aircraftUpdatedAt,
			&companyID, &company.Name, &company.Email, &company.Phone, &company.Address, &company.Country, &company.Logo, &company.Status, &company.SubscriptionID, &companyCreatedAt, &companyUpdatedAt)

		if err != nil {
			log.Println("Error scanning exceedance:", err)
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

	query := `SELECT e.id, COALESCE(e.exceedanceValues, '') as exceedanceValues, COALESCE(e.flightPhase, '') as flightPhase, COALESCE(e.parameterName, '') as parameterName, COALESCE(e.description, '') as description, COALESCE(e.eventStatus, '') as eventStatus, COALESCE(e.aircraftId, '') as aircraftId, COALESCE(e.flightLegId, e.flightId, '') as flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.detectionRunId, e.startTimeMs, e.endTimeMs, e.durationMs, e.peakValue, e.ruleHash, e.isCurrent, e.createdAt, e.updatedAt,
			  el.id as eventlog_id, el.eventName, COALESCE(el.displayName, '') as displayName, COALESCE(el.eventCode, '') as eventCode, COALESCE(el.eventDescription, '') as eventDescription, COALESCE(el.eventParameter, '') as eventParameter, COALESCE(el.eventTrigger, '') as eventTrigger, COALESCE(el.eventType, '') as eventType, COALESCE(el.flightPhase, '') as eventlog_flightPhase, el.high, el.high1, el.high2, el.low, el.low1, el.low2, el.triggerType, el.detectionPeriod, el.severities, COALESCE(el.sop, '') as sop, COALESCE(el.aircraftId, '') as eventlog_aircraftId, el.createdAt as eventlog_createdAt, el.updatedAt as eventlog_updatedAt,
			  f.id as csv_id, COALESCE(f.name, '') as name, COALESCE(c.file, '') as csv_file, f.status, f.departure, f.pilot, f.destination, f.flightHours, COALESCE(f.aircraftId, '') as csv_aircraftId, f.createdAt as csv_createdAt, f.updatedAt as csv_updatedAt,
			  a.id as aircraft_id, COALESCE(a.airline, '') as airline, COALESCE(a.aircraftMake, '') as aircraftMake, a.modelNumber, COALESCE(a.serialNumber, '') as serialNumber, a.registration, COALESCE(a.companyId, '') as companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, COALESCE(co.name, '') as company_name, COALESCE(co.email, '') as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, COALESCE(co.status, '') as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM Exceedance e 
			  LEFT JOIN EventLog el ON e.eventId = el.id 
			  LEFT JOIN FlightLeg f ON COALESCE(e.flightLegId, e.flightId) = f.id
			  LEFT JOIN Csv c ON f.recordingId = c.id
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id 
			  LEFT JOIN Company co ON a.companyId = co.id
			  WHERE e.id = ? AND e.isCurrent = 1`

	filter, accessArgs, allowed := accessFilter(c)
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "No company access is assigned to this account"})
		return
	}
	query += filter
	queryArgs := append([]interface{}{id}, accessArgs...)

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

	row := h.db.QueryRow(query, queryArgs...)
	err := row.Scan(&exceedance.ID, &exceedance.ExceedanceValues, &exceedance.FlightPhase,
		&exceedance.ParameterName, &exceedance.Description, &exceedance.EventStatus,
		&exceedance.AircraftID, &exceedance.FlightID, &exceedance.File, &exceedance.EventID,
		&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.DetectionRunID,
		&exceedance.StartTimeMs, &exceedance.EndTimeMs, &exceedance.DurationMs,
		&exceedance.PeakValue, &exceedance.RuleHash, &exceedance.IsCurrent, &exceedanceCreatedAtStr, &exceedanceUpdatedAtStr,
		&eventLogID, &eventLog.EventName, &eventLog.DisplayName, &eventLog.EventCode,
		&eventLog.EventDescription, &eventLog.EventParameter, &eventLog.EventTrigger, &eventLog.EventType,
		&eventLog.FlightPhase, &eventLog.High, &eventLog.High1, &eventLog.High2, &eventLog.Low,
		&eventLog.Low1, &eventLog.Low2, &eventLog.TriggerType, &eventLog.DetectionPeriod, &eventLog.Severities, &eventLog.SOP, &eventLog.AircraftID, &eventLogCreatedAtStr, &eventLogUpdatedAtStr,
		&csvID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csvCreatedAtStr, &csvUpdatedAtStr,
		&aircraftID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
		&aircraft.SerialNumber, &aircraft.Registration, &aircraft.CompanyID, &aircraft.Parameters, &aircraftCreatedAtStr, &aircraftUpdatedAtStr,
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

	reviews, err := h.getExceedanceReviews(id)
	if err != nil {
		log.Printf("Error loading exceedance reviews: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading exceedance review history"})
		return
	}

	response := struct {
		models.Exceedance
		EventLog *models.EventLog          `json:"eventlog"`
		CSV      models.CSV                `json:"csv"`
		Aircraft models.Aircraft           `json:"aircraft"`
		Reviews  []models.ExceedanceReview `json:"reviews"`
	}{
		Exceedance: exceedance,
		EventLog:   eventLogPtr,
		CSV:        csv,
		Aircraft:   aircraft,
		Reviews:    reviews,
	}

	c.JSON(http.StatusOK, response)
}

// GetExceedancesByFlightID retrieves exceedances by flight ID
func (h *ExceedanceHandler) GetExceedancesByFlightID(c *gin.Context) {
	flightID := c.Param("id")

	query := `SELECT e.id, e.exceedanceValues, e.flightPhase, e.parameterName, e.description, e.eventStatus, e.aircraftId, COALESCE(e.flightLegId, e.flightId), e.file, e.eventId, e.comment, e.exceedanceLevel,
		e.detectionRunId, e.startTimeMs, e.endTimeMs, e.durationMs, e.peakValue, e.ruleHash, e.isCurrent, e.createdAt, e.updatedAt
		FROM Exceedance e JOIN Aircraft a ON a.id = e.aircraftId WHERE COALESCE(e.flightLegId, e.flightId) = ? AND e.isCurrent = 1`
	filter, accessArgs, allowed := accessFilter(c)
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "No company access is assigned to this account"})
		return
	}
	query += filter + " ORDER BY e.startTimeMs, e.createdAt"
	queryArgs := append([]interface{}{flightID}, accessArgs...)
	rows, err := h.db.Query(query, queryArgs...)
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
			&exceedance.Comment, &exceedance.ExceedanceLevel, &exceedance.DetectionRunID,
			&exceedance.StartTimeMs, &exceedance.EndTimeMs, &exceedance.DurationMs,
			&exceedance.PeakValue, &exceedance.RuleHash, &exceedance.IsCurrent,
			&createdAtStr, &updatedAtStr)
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
	for _, exceedance := range exceedances {
		var flightCompanyID string
		err := h.db.QueryRow(`SELECT a.companyId
			FROM FlightLeg f JOIN Aircraft a ON a.id = f.aircraftId
			WHERE f.id = ? AND f.aircraftId = ?`, exceedance.FlightID, exceedance.AircraftID).Scan(&flightCompanyID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "An exceedance references invalid flight data"})
			return
		}
		if err != nil {
			respondDatabaseError(c, err)
			return
		}
		if !canAccessCompany(c, flightCompanyID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "An exceedance references flight data outside your company"})
			return
		}
		if exceedance.EventID != nil && *exceedance.EventID != "" {
			var eventCompanyID string
			err = h.db.QueryRow(`SELECT d.companyId FROM EventDefinitionVersion v
				JOIN EventDefinition d ON d.id = v.definitionId
				WHERE v.id = ?`, *exceedance.EventID).Scan(&eventCompanyID)
			if err == sql.ErrNoRows {
				c.JSON(http.StatusBadRequest, gin.H{"error": "An exceedance references an invalid event"})
				return
			}
			if err != nil {
				respondDatabaseError(c, err)
				return
			}
			if eventCompanyID != flightCompanyID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "The event and flight must belong to the same company"})
				return
			}
		}
	}

	var createdExceedances []models.Exceedance
	now := time.Now()

	for _, exceedance := range exceedances {
		// Generate ID
		id := uuid.New().String()
		var recordingID string
		if err := h.db.QueryRow("SELECT recordingId FROM FlightLeg WHERE id = ?", exceedance.FlightID).Scan(&recordingID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid flightId is required for every exceedance"})
			return
		}

		query := `INSERT INTO Exceedance (id, exceedanceValues, flightPhase, parameterName, description, eventStatus, aircraftId, flightId, flightLegId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

		_, err := h.db.Exec(query, id, exceedance.ExceedanceValues, exceedance.FlightPhase,
			exceedance.ParameterName, exceedance.Description, exceedance.EventStatus,
			exceedance.AircraftID, recordingID, exceedance.FlightID, exceedance.File, exceedance.EventID,
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

	comment := ""
	if req.Comment != nil {
		comment = strings.TrimSpace(*req.Comment)
	}
	if comment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A reviewer comment is required"})
		return
	}
	if !isValidExceedanceStatus(req.EventStatus) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exceedance status", "allowed": []string{
			models.ExceedanceStatusPending,
			models.ExceedanceStatusValid,
			models.ExceedanceStatusNuisance,
			models.ExceedanceStatusFalse,
		}})
		return
	}

	userIDValue, ok := c.Get("userId")
	userID, userIDOK := userIDValue.(string)
	if !ok || !userIDOK || strings.TrimSpace(userID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Reviewer identity is unavailable"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error starting exceedance review"})
		return
	}
	defer tx.Rollback()

	accessQuery := `SELECT e.eventStatus FROM Exceedance e`
	accessArgs := []interface{}{id}
	var companyID string
	if !hasGlobalCompanyAccess(c) {
		var companyOK bool
		companyID, companyOK = requireTenantCompany(c)
		if !companyOK {
			return
		}
		accessQuery += " JOIN Aircraft a ON a.id = e.aircraftId"
		accessArgs = append(accessArgs, companyID)
	}
	accessQuery += " WHERE e.id = ? AND e.isCurrent = 1"
	if !hasGlobalCompanyAccess(c) {
		accessQuery += " AND a.companyId = ?"
	}
	var previousStatus string
	if err = tx.QueryRow(accessQuery, accessArgs...).Scan(&previousStatus); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading exceedance"})
		return
	}
	if previousStatus == req.EventStatus {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Select a status different from the current status"})
		return
	}

	now := time.Now()
	updateQuery := `UPDATE Exceedance SET comment = ?, eventStatus = ?, updatedAt = ? WHERE id = ? AND isCurrent = 1`
	updateArgs := []interface{}{comment, req.EventStatus, now.UnixMilli(), id}
	if !hasGlobalCompanyAccess(c) {
		updateQuery += " AND aircraftId IN (SELECT id FROM Aircraft WHERE companyId = ?)"
		updateArgs = append(updateArgs, companyID)
	}
	if _, err = tx.Exec(updateQuery, updateArgs...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating exceedance"})
		return
	}
	if _, err = tx.Exec(`INSERT INTO ExceedanceReview
		(id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES (?, ?, 'status_change', ?, ?, ?, ?, ?)`,
		uuid.NewString(), id, previousStatus, req.EventStatus, comment, userID, now.UnixMilli()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error recording exceedance review"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error committing exceedance review"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Exceedance review recorded successfully",
		"id":          id,
		"eventStatus": req.EventStatus,
	})
}

// DeleteExceedance archives an exceedance while retaining its evidence and audit history.
func (h *ExceedanceHandler) DeleteExceedance(c *gin.Context) {
	id := c.Param("id")
	var req models.ArchiveExceedanceRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "An archive reason is required"})
		return
	}

	userIDValue, ok := c.Get("userId")
	userID, userIDOK := userIDValue.(string)
	if !ok || !userIDOK || strings.TrimSpace(userID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Reviewer identity is unavailable"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error starting archive operation"})
		return
	}
	defer tx.Rollback()

	accessQuery := `SELECT e.eventStatus FROM Exceedance e`
	accessArgs := []interface{}{id}
	var companyID string
	if !hasGlobalCompanyAccess(c) {
		var companyOK bool
		companyID, companyOK = requireTenantCompany(c)
		if !companyOK {
			return
		}
		accessQuery += " JOIN Aircraft a ON a.id = e.aircraftId"
		accessArgs = append(accessArgs, companyID)
	}
	accessQuery += " WHERE e.id = ? AND e.isCurrent = 1"
	if !hasGlobalCompanyAccess(c) {
		accessQuery += " AND a.companyId = ?"
	}
	var previousStatus string
	if err = tx.QueryRow(accessQuery, accessArgs...).Scan(&previousStatus); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exceedance not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error loading exceedance"})
		return
	}

	now := time.Now()
	updateQuery := `UPDATE Exceedance SET isCurrent = 0, supersededAt = ?, updatedAt = ? WHERE id = ? AND isCurrent = 1`
	updateArgs := []interface{}{now.UnixMilli(), now.UnixMilli(), id}
	if !hasGlobalCompanyAccess(c) {
		updateQuery += " AND aircraftId IN (SELECT id FROM Aircraft WHERE companyId = ?)"
		updateArgs = append(updateArgs, companyID)
	}
	if _, err = tx.Exec(updateQuery, updateArgs...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error archiving exceedance"})
		return
	}
	if _, err = tx.Exec(`INSERT INTO ExceedanceReview
		(id, exceedanceId, action, previousStatus, newStatus, comment, reviewedBy, createdAt)
		VALUES (?, ?, 'archive', ?, NULL, ?, ?, ?)`,
		uuid.NewString(), id, previousStatus, strings.TrimSpace(req.Reason), userID, now.UnixMilli()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error recording archive history"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error committing archive operation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Exceedance archived successfully"})
}

func (h *ExceedanceHandler) getExceedanceReviews(exceedanceID string) ([]models.ExceedanceReview, error) {
	rows, err := h.db.Query(`SELECT r.id, r.exceedanceId, r.action, r.previousStatus, r.newStatus,
		r.comment, r.reviewedBy, COALESCE(u.fullName, u.email, 'Unknown reviewer'), r.createdAt
		FROM ExceedanceReview r LEFT JOIN User u ON u.id = r.reviewedBy
		WHERE r.exceedanceId = ? ORDER BY r.createdAt DESC`, exceedanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reviews := make([]models.ExceedanceReview, 0)
	for rows.Next() {
		var review models.ExceedanceReview
		var createdAt nullableTimestamp
		var reviewerName string
		if err = rows.Scan(&review.ID, &review.ExceedanceID, &review.Action, &review.PreviousStatus,
			&review.NewStatus, &review.Comment, &review.ReviewedBy, &reviewerName, &createdAt); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			review.CreatedAt = createdAt.Time
		}
		review.ReviewerName = &reviewerName
		reviews = append(reviews, review)
	}

	return reviews, rows.Err()
}

// GetGlobalBenchmarks retains the legacy response shape. Admin and FDA roles
// receive cross-company oversight data; tenant roles receive only their company.
func (h *ExceedanceHandler) GetGlobalBenchmarks(c *gin.Context) {
	globalAccess := hasGlobalCompanyAccess(c)
	companyID := ""
	if !globalAccess {
		var ok bool
		companyID, ok = requireTenantCompany(c)
		if !ok {
			return
		}
	}
	// Optional filter by aircraft model
	aircraftModel := c.Query("model")

	// Get total flights and exceedances grouped by aircraft model
	var modelQuery string
	var rows *sql.Rows
	var err error

	if aircraftModel != "" && globalAccess {
		modelQuery = `
			SELECT
				a.aircraftMake,
				a.modelNumber,
				COUNT(DISTINCT flight.id) as totalFlights,
				COUNT(e.id) as totalExceedances,
				SUM(CASE WHEN e.exceedanceLevel = 'None' THEN 1 ELSE 0 END) as severityNone,
				SUM(CASE WHEN e.exceedanceLevel = 'Low' THEN 1 ELSE 0 END) as severityLow,
				SUM(CASE WHEN e.exceedanceLevel = 'Medium' THEN 1 ELSE 0 END) as severityMedium,
				SUM(CASE WHEN e.exceedanceLevel = 'High' THEN 1 ELSE 0 END) as severityHigh,
				SUM(CASE WHEN e.exceedanceLevel = 'Critical' THEN 1 ELSE 0 END) as severityCritical
			FROM Aircraft a
			LEFT JOIN FlightLeg flight ON flight.aircraftId = a.id
			LEFT JOIN Exceedance e ON e.flightLegId = flight.id AND e.isCurrent = 1
			WHERE a.aircraftMake = ? OR a.modelNumber = ?
			GROUP BY a.aircraftMake, a.modelNumber`
		rows, err = h.db.Query(modelQuery, aircraftModel, aircraftModel)
	} else if aircraftModel != "" {
		modelQuery = `
			SELECT 
				a.aircraftMake,
				a.modelNumber,
				COUNT(DISTINCT flight.id) as totalFlights,
				COUNT(e.id) as totalExceedances,
				SUM(CASE WHEN e.exceedanceLevel = 'None' THEN 1 ELSE 0 END) as severityNone,
				SUM(CASE WHEN e.exceedanceLevel = 'Low' THEN 1 ELSE 0 END) as severityLow,
				SUM(CASE WHEN e.exceedanceLevel = 'Medium' THEN 1 ELSE 0 END) as severityMedium,
				SUM(CASE WHEN e.exceedanceLevel = 'High' THEN 1 ELSE 0 END) as severityHigh,
				SUM(CASE WHEN e.exceedanceLevel = 'Critical' THEN 1 ELSE 0 END) as severityCritical
			FROM Aircraft a
			LEFT JOIN FlightLeg flight ON flight.aircraftId = a.id
			LEFT JOIN Exceedance e ON e.flightLegId = flight.id AND e.isCurrent = 1
			WHERE a.companyId = ? AND (a.aircraftMake = ? OR a.modelNumber = ?)
			GROUP BY a.aircraftMake, a.modelNumber`
		rows, err = h.db.Query(modelQuery, companyID, aircraftModel, aircraftModel)
	} else if globalAccess {
		modelQuery = `
			SELECT
				a.aircraftMake,
				a.modelNumber,
				COUNT(DISTINCT flight.id) as totalFlights,
				COUNT(e.id) as totalExceedances,
				SUM(CASE WHEN e.exceedanceLevel = 'None' THEN 1 ELSE 0 END) as severityNone,
				SUM(CASE WHEN e.exceedanceLevel = 'Low' THEN 1 ELSE 0 END) as severityLow,
				SUM(CASE WHEN e.exceedanceLevel = 'Medium' THEN 1 ELSE 0 END) as severityMedium,
				SUM(CASE WHEN e.exceedanceLevel = 'High' THEN 1 ELSE 0 END) as severityHigh,
				SUM(CASE WHEN e.exceedanceLevel = 'Critical' THEN 1 ELSE 0 END) as severityCritical
			FROM Aircraft a
			LEFT JOIN FlightLeg flight ON flight.aircraftId = a.id
			LEFT JOIN Exceedance e ON e.flightLegId = flight.id AND e.isCurrent = 1
			GROUP BY a.aircraftMake, a.modelNumber`
		rows, err = h.db.Query(modelQuery)
	} else {
		modelQuery = `
			SELECT 
				a.aircraftMake,
				a.modelNumber,
				COUNT(DISTINCT flight.id) as totalFlights,
				COUNT(e.id) as totalExceedances,
				SUM(CASE WHEN e.exceedanceLevel = 'None' THEN 1 ELSE 0 END) as severityNone,
				SUM(CASE WHEN e.exceedanceLevel = 'Low' THEN 1 ELSE 0 END) as severityLow,
				SUM(CASE WHEN e.exceedanceLevel = 'Medium' THEN 1 ELSE 0 END) as severityMedium,
				SUM(CASE WHEN e.exceedanceLevel = 'High' THEN 1 ELSE 0 END) as severityHigh,
				SUM(CASE WHEN e.exceedanceLevel = 'Critical' THEN 1 ELSE 0 END) as severityCritical
			FROM Aircraft a
			LEFT JOIN FlightLeg flight ON flight.aircraftId = a.id
			LEFT JOIN Exceedance e ON e.flightLegId = flight.id AND e.isCurrent = 1
			WHERE a.companyId = ?
			GROUP BY a.aircraftMake, a.modelNumber`
		rows, err = h.db.Query(modelQuery, companyID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	defer rows.Close()

	type ModelStats struct {
		AircraftMake     string  `json:"aircraftMake"`
		ModelNumber      string  `json:"modelNumber"`
		TotalFlights     int     `json:"totalFlights"`
		TotalExceedances int     `json:"totalExceedances"`
		EventsPer100     float64 `json:"eventsPer100"`
		SeverityNone     int     `json:"severityNone"`
		SeverityLow      int     `json:"severityLow"`
		SeverityMedium   int     `json:"severityMedium"`
		SeverityHigh     int     `json:"severityHigh"`
		SeverityCritical int     `json:"severityCritical"`
	}

	var modelStats []ModelStats
	var globalTotalFlights, globalTotalExceedances int
	var globalSeverityNone, globalSeverityLow, globalSeverityMedium, globalSeverityHigh, globalSeverityCritical int

	for rows.Next() {
		var stats ModelStats
		var modelNumber sql.NullString
		err := rows.Scan(&stats.AircraftMake, &modelNumber, &stats.TotalFlights, &stats.TotalExceedances,
			&stats.SeverityNone, &stats.SeverityLow, &stats.SeverityMedium, &stats.SeverityHigh, &stats.SeverityCritical)
		if err != nil {
			continue
		}

		if modelNumber.Valid {
			stats.ModelNumber = modelNumber.String
		}

		if stats.TotalFlights > 0 {
			stats.EventsPer100 = float64(stats.TotalExceedances) / float64(stats.TotalFlights) * 100
		}

		modelStats = append(modelStats, stats)

		globalTotalFlights += stats.TotalFlights
		globalTotalExceedances += stats.TotalExceedances
		globalSeverityNone += stats.SeverityNone
		globalSeverityLow += stats.SeverityLow
		globalSeverityMedium += stats.SeverityMedium
		globalSeverityHigh += stats.SeverityHigh
		globalSeverityCritical += stats.SeverityCritical
	}

	// Calculate global averages
	var globalEventsPer100 float64
	if globalTotalFlights > 0 {
		globalEventsPer100 = float64(globalTotalExceedances) / float64(globalTotalFlights) * 100
	}

	// Calculate percentiles from model stats
	var eventRates []float64
	for _, stats := range modelStats {
		if stats.TotalFlights > 0 {
			eventRates = append(eventRates, stats.EventsPer100)
		}
	}

	// Sort event rates to calculate percentiles
	sort.Float64s(eventRates)

	var percentile25, percentile50, percentile75, percentile90 float64
	n := len(eventRates)
	if n > 0 {
		percentile25 = getPercentile(eventRates, 25)
		percentile50 = getPercentile(eventRates, 50)
		percentile75 = getPercentile(eventRates, 75)
		percentile90 = getPercentile(eventRates, 90)
	}

	// Calculate severity rates per 100 flights
	var severityPer100 = map[string]float64{}
	if globalTotalFlights > 0 {
		severityPer100["None"] = float64(globalSeverityNone) / float64(globalTotalFlights) * 100
		severityPer100["Low"] = float64(globalSeverityLow) / float64(globalTotalFlights) * 100
		severityPer100["Medium"] = float64(globalSeverityMedium) / float64(globalTotalFlights) * 100
		severityPer100["High"] = float64(globalSeverityHigh) / float64(globalTotalFlights) * 100
		severityPer100["Critical"] = float64(globalSeverityCritical) / float64(globalTotalFlights) * 100
	}

	// Get event type breakdown
	eventTypeQuery := `
		SELECT 
			COALESCE(el.displayName, e.description, 'Unknown') as eventName,
			COUNT(*) as count
		FROM Exceedance e
		LEFT JOIN EventLog el ON e.eventId = el.id
		JOIN Aircraft a ON a.id = e.aircraftId
		WHERE e.isCurrent = 1`
	eventArgs := []interface{}{}
	if !globalAccess {
		eventTypeQuery += " AND a.companyId = ?"
		eventArgs = append(eventArgs, companyID)
	}
	eventTypeQuery += `
		GROUP BY eventName
		ORDER BY count DESC
		LIMIT 20`

	eventRows, err := h.db.Query(eventTypeQuery, eventArgs...)
	if err == nil {
		defer eventRows.Close()
	}

	type EventTypeStats struct {
		EventName string  `json:"eventName"`
		Count     int     `json:"count"`
		Per100    float64 `json:"per100"`
	}

	var eventTypeStats []EventTypeStats
	if eventRows != nil {
		for eventRows.Next() {
			var stats EventTypeStats
			err := eventRows.Scan(&stats.EventName, &stats.Count)
			if err != nil {
				continue
			}
			if globalTotalFlights > 0 {
				stats.Per100 = float64(stats.Count) / float64(globalTotalFlights) * 100
			}
			eventTypeStats = append(eventTypeStats, stats)
		}
	}

	response := gin.H{
		"globalStats": gin.H{
			"totalFlights":        globalTotalFlights,
			"totalExceedances":    globalTotalExceedances,
			"averageEventsPer100": globalEventsPer100,
			"percentile25":        percentile25,
			"percentile50":        percentile50,
			"percentile75":        percentile75,
			"percentile90":        percentile90,
			"bySeverity":          severityPer100,
		},
		"byModel":     modelStats,
		"byEventType": eventTypeStats,
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to calculate percentile
func getPercentile(sortedData []float64, percentile float64) float64 {
	n := len(sortedData)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sortedData[0]
	}
	index := (percentile / 100) * float64(n-1)
	lower := int(index)
	upper := lower + 1
	if upper >= n {
		return sortedData[n-1]
	}
	weight := index - float64(lower)
	return sortedData[lower]*(1-weight) + sortedData[upper]*weight
}
