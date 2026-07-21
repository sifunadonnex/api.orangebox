package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"fmt"
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
	if file.Size <= 0 || file.Size > 50*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSV file size must be between 1 byte and 50 MB"})
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
	if !isSystemEventRole(c) {
		companyID, ok := contextString(c, "userCompanyId")
		if !ok || companyID != aircraft.CompanyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You cannot upload flight data for this aircraft"})
			return
		}
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

	id := uuid.New().String()
	now := time.Now()
	query := `INSERT INTO Csv (id, name, file, status, aircraftId, departure, destination, flightHours, pilot, sampleIntervalMs, createdAt, updatedAt)
		VALUES (?, ?, ?, 'processing', ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = h.db.Exec(query, id, req.Name, filename, req.AircraftID, req.Departure, req.Destination, req.FlightHours, req.Pilot, req.SampleIntervalMs, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		_ = os.Remove(csvPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving CSV record", "details": err.Error()})
		return
	}

	csv := models.CSV{
		ID:               id,
		Name:             req.Name,
		File:             filename,
		AircraftID:       req.AircraftID,
		Departure:        req.Departure,
		Destination:      req.Destination,
		FlightHours:      req.FlightHours,
		Pilot:            req.Pilot,
		SampleIntervalMs: &req.SampleIntervalMs,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	analysis := h.analyzeUploadedFlight(csvPath, filename, &csv, aircraft)
	csv.Status = &analysis.Status
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": csv, "analysis": analysis})
}

// GetCSVs retrieves all CSV files with exceedances
func (h *CSVHandler) GetCSVs(c *gin.Context) {
	query := `SELECT c.id, c.name, c.file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId, c.sampleIntervalMs, c.analysisSummary, c.createdAt, c.updatedAt,
			  a.id as aircraft_id, a.airline, a.aircraftMake, a.modelNumber, a.serialNumber, a.registration, a.companyId, a.parameters, a.createdAt as aircraft_createdAt, a.updatedAt as aircraft_updatedAt,
			  co.id as company_id, co.name as company_name, co.email as company_email, co.phone as company_phone, co.address as company_address, co.country as company_country, co.logo as company_logo, co.status as company_status, co.subscriptionId as company_subscriptionId, co.createdAt as company_createdAt, co.updatedAt as company_updatedAt
			  FROM Csv c
			  LEFT JOIN Aircraft a ON c.aircraftId = a.id
			  LEFT JOIN Company co ON a.companyId = co.id`
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var csvs []interface{}
	for rows.Next() {
		var csv models.CSV
		var aircraft models.Aircraft
		var company models.Company
		var createdAtStr, updatedAtStr sql.NullString
		var aircraftID sql.NullString
		var aircraftCreatedAtStr, aircraftUpdatedAtStr sql.NullString
		var companyID sql.NullString
		var companyCreatedAtStr, companyUpdatedAtStr sql.NullString

		err := rows.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csv.SampleIntervalMs, &csv.AnalysisSummary, &createdAtStr, &updatedAtStr,
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
				csv.CreatedAt = parsedTime
			}
		}
		if updatedAtStr.Valid {
			parsedTime, err := parseTimestamp(updatedAtStr.String)
			if err == nil {
				csv.UpdatedAt = parsedTime
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
		exceedances, _ := h.getCSVExceedances(csv.ID)

		csvWithExceedances := struct {
			models.CSV
			Aircraft   *models.Aircraft    `json:"aircraft"`
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			CSV:        csv,
			Aircraft:   aircraftPtr,
			Exceedance: exceedances,
		}

		csvs = append(csvs, csvWithExceedances)
	}

	c.JSON(http.StatusOK, csvs)
}

// DownloadCSV serves a CSV file for download
func (h *CSVHandler) DownloadCSV(c *gin.Context) {
	filename := c.Param("id")
	filePath := filepath.Join("csvs", filename)

	// Check if file exists
	if _, err := filepath.Abs(filePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	c.File(filePath)
}

// GetCSVByID retrieves a CSV record by ID
func (h *CSVHandler) GetCSVByID(c *gin.Context) {
	id := c.Param("id")

	query := `SELECT id, name, file, status, departure, pilot, destination, flightHours, aircraftId, sampleIntervalMs, analysisSummary, createdAt, updatedAt FROM Csv WHERE id = ?`

	var csv models.CSV
	var createdAtStr, updatedAtStr sql.NullString
	row := h.db.QueryRow(query, id)
	err := row.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &csv.SampleIntervalMs, &csv.AnalysisSummary, &createdAtStr, &updatedAtStr)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "CSV not found"})
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
			csv.CreatedAt = parsedTime
		}
	}
	if updatedAtStr.Valid {
		parsedTime, err := parseTimestamp(updatedAtStr.String)
		if err == nil {
			csv.UpdatedAt = parsedTime
		}
	}

	c.JSON(http.StatusOK, csv)
}

// Helper function to get exceedances for a CSV with related EventLog and Aircraft data
func (h *CSVHandler) getCSVExceedances(csvID string) ([]models.Exceedance, error) {
	query := `SELECT e.id, e.exceedanceValues, e.flightPhase, e.parameterName, e.description, e.eventStatus,
			  e.aircraftId, e.flightId, e.file, e.eventId, e.comment, e.exceedanceLevel, e.createdAt, e.updatedAt,
			  a.serialNumber as aircraftRegistration,
			  ev.id as eventLogId, ev.eventName, ev.displayName, ev.eventCode, ev.eventDescription,
			  ev.eventParameter, ev.eventTrigger, ev.eventType, ev.flightPhase as eventFlightPhase
			  FROM Exceedance e
			  LEFT JOIN Aircraft a ON e.aircraftId = a.id
			  LEFT JOIN EventLog ev ON e.eventId = ev.id
			  WHERE e.flightId = ?`
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

	// Check if CSV exists
	var exists bool
	var filename string
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM Csv WHERE id = ?), (SELECT file FROM Csv WHERE id = ?)", id, id).Scan(&exists, &filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Flight not found"})
		return
	}

	// Delete associated exceedances first (foreign key constraint)
	_, err = h.db.Exec("DELETE FROM Exceedance WHERE flightId = ?", id)
	if err != nil {
		log.Printf("Warning: Failed to delete associated exceedances: %v", err)
	}

	// Delete CSV record from database
	query := "DELETE FROM Csv WHERE id = ?"
	result, err := h.db.Exec(query, id)
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
