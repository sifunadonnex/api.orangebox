package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type EventHandler struct {
	db *sql.DB
}

func NewEventHandler(db *sql.DB) *EventHandler {
	return &EventHandler{db: db}
}

// CreateEvent creates a new event log
func (h *EventHandler) CreateEvent(c *gin.Context) {
	var req models.CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate ID and timestamps
	id := uuid.New().String()
	now := time.Now()

	// Insert event
	query := `INSERT INTO EventLog (id, eventName, displayName, eventCode, eventDescription, eventParameter, eventTrigger, eventType, flightPhase, high, low, low1, high1, low2, high2, sop, aircraftId, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	_, err := h.db.Exec(query, id, req.EventName, req.DisplayName, req.EventCode, req.EventDescription,
		req.EventParameter, req.EventTrigger, req.EventType, req.FlightPhase, req.High, req.Low,
		req.Low1, req.High1, req.Low2, req.High2, req.SOP, req.AircraftID, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating event"})
		return
	}

	// Return created event
	event := models.EventLog{
		ID:               id,
		EventName:        req.EventName,
		DisplayName:      req.DisplayName,
		EventCode:        req.EventCode,
		EventDescription: req.EventDescription,
		EventParameter:   req.EventParameter,
		EventTrigger:     req.EventTrigger,
		EventType:        req.EventType,
		FlightPhase:      req.FlightPhase,
		High:             req.High,
		Low:              req.Low,
		Low1:             req.Low1,
		High1:            req.High1,
		Low2:             req.Low2,
		High2:            req.High2,
		SOP:              req.SOP,
		AircraftID:       req.AircraftID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	c.JSON(http.StatusOK, event)
}

// GetEvents retrieves all events with aircraft information
func (h *EventHandler) GetEvents(c *gin.Context) {
	query := `SELECT e.id, e.eventName, e.displayName, e.eventCode, e.eventDescription, e.eventParameter, e.eventTrigger, e.eventType, e.flightPhase, e.high, e.high1, e.high2, e.low, e.low1, e.low2, e.sop, e.aircraftId, e.createdAt, e.updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.userId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt
			  FROM EventLog e 
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id`
	
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var events []interface{}
	for rows.Next() {
		var event models.EventLog
		var aircraft models.Aircraft
		var aircraftCreatedAt, aircraftUpdatedAt sql.NullTime

		err := rows.Scan(&event.ID, &event.EventName, &event.DisplayName, &event.EventCode,
			&event.EventDescription, &event.EventParameter, &event.EventTrigger, &event.EventType,
			&event.FlightPhase, &event.High, &event.High1, &event.High2, &event.Low,
			&event.Low1, &event.Low2, &event.SOP, &event.AircraftID, &event.CreatedAt, &event.UpdatedAt,
			&aircraft.ID, &aircraft.Airline, &aircraft.AircraftMake, &aircraft.ModelNumber,
			&aircraft.SerialNumber, &aircraft.UserID, &aircraft.Parameters, &aircraftCreatedAt, &aircraftUpdatedAt)
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning event"})
			return
		}

		// Handle nullable timestamps
		if aircraftCreatedAt.Valid {
			aircraft.CreatedAt = aircraftCreatedAt.Time
		}
		if aircraftUpdatedAt.Valid {
			aircraft.UpdatedAt = aircraftUpdatedAt.Time
		}

		eventWithAircraft := struct {
			models.EventLog
			Aircraft models.Aircraft `json:"aircraft"`
		}{
			EventLog: event,
			Aircraft: aircraft,
		}

		events = append(events, eventWithAircraft)
	}

	c.JSON(http.StatusOK, events)
}

// GetEventByID retrieves an event by ID
func (h *EventHandler) GetEventByID(c *gin.Context) {
	id := c.Param("id")
	
	query := `SELECT id, eventName, displayName, eventCode, eventDescription, eventParameter, eventTrigger, eventType, flightPhase, high, high1, high2, low, low1, low2, sop, aircraftId, createdAt, updatedAt FROM EventLog WHERE id = ?`
	
	var event models.EventLog
	row := h.db.QueryRow(query, id)
	err := row.Scan(&event.ID, &event.EventName, &event.DisplayName, &event.EventCode,
		&event.EventDescription, &event.EventParameter, &event.EventTrigger, &event.EventType,
		&event.FlightPhase, &event.High, &event.High1, &event.High2, &event.Low,
		&event.Low1, &event.Low2, &event.SOP, &event.AircraftID, &event.CreatedAt, &event.UpdatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// UpdateEvent updates an existing event
func (h *EventHandler) UpdateEvent(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()

	query := `UPDATE EventLog SET eventName = ?, displayName = ?, eventCode = ?, eventDescription = ?, eventParameter = ?, eventTrigger = ?, eventType = ?, flightPhase = ?, high = ?, low = ?, low1 = ?, high1 = ?, low2 = ?, high2 = ?, sop = ?, aircraftId = ?, updatedAt = ? WHERE id = ?`
	
	result, err := h.db.Exec(query, req.EventName, req.DisplayName, req.EventCode, req.EventDescription,
		req.EventParameter, req.EventTrigger, req.EventType, req.FlightPhase, req.High, req.Low,
		req.Low1, req.High1, req.Low2, req.High2, req.SOP, req.AircraftID, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating event"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	// Return updated event
	event := models.EventLog{
		ID:               id,
		EventName:        req.EventName,
		DisplayName:      req.DisplayName,
		EventCode:        req.EventCode,
		EventDescription: req.EventDescription,
		EventParameter:   req.EventParameter,
		EventTrigger:     req.EventTrigger,
		EventType:        req.EventType,
		FlightPhase:      req.FlightPhase,
		High:             req.High,
		Low:              req.Low,
		Low1:             req.Low1,
		High1:            req.High1,
		Low2:             req.Low2,
		High2:            req.High2,
		SOP:              req.SOP,
		AircraftID:       req.AircraftID,
		UpdatedAt:        now,
	}

	c.JSON(http.StatusOK, event)
}

// DeleteEvent deletes an event
func (h *EventHandler) DeleteEvent(c *gin.Context) {
	id := c.Param("id")
	
	query := `DELETE FROM EventLog WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting event"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Event deleted successfully"})
}
