package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
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
	// Parse form data
	var req models.UploadCSVRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded", "code": 400})
		return
	}

	// Generate unique filename
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	filename := fmt.Sprintf("%d-%s", timestamp, file.Filename)

	// Save file to csvs directory
	csvPath := filepath.Join("csvs", filename)
	if err := c.SaveUploadedFile(file, csvPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "File upload failed", "code": 500})
		return
	}

	// Save to database
	id := uuid.New().String()
	now := time.Now()

	query := `INSERT INTO Csv (id, name, file, aircraftId, departure, destination, flightHours, pilot, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = h.db.Exec(query, id, req.Name, filename, req.AircraftID, req.Departure, req.Destination, req.FlightHours, req.Pilot, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving CSV record", "details": err.Error()})
		return
	}

	// Return created CSV record
	csv := models.CSV{
		ID:          id,
		Name:        req.Name,
		File:        filename,
		AircraftID:  req.AircraftID,
		Departure:   req.Departure,
		Destination: req.Destination,
		FlightHours: req.FlightHours,
		Pilot:       req.Pilot,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	c.JSON(http.StatusOK, csv)
}

// GetCSVs retrieves all CSV files with exceedances
func (h *CSVHandler) GetCSVs(c *gin.Context) {
	query := `SELECT c.id, c.name, c.file, c.status, c.departure, c.pilot, c.destination, c.flightHours, c.aircraftId, c.createdAt, c.updatedAt,
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
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtStr, &updatedAtStr,
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

	query := `SELECT id, name, file, status, departure, pilot, destination, flightHours, aircraftId, createdAt, updatedAt FROM Csv WHERE id = ?`

	var csv models.CSV
	var createdAtStr, updatedAtStr sql.NullString
	row := h.db.QueryRow(query, id)
	err := row.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtStr, &updatedAtStr)

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

// DeleteCSV deletes a CSV record
func (h *CSVHandler) DeleteCSV(c *gin.Context) {
	id := c.Param("id")

	query := `DELETE FROM Csv WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting CSV"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "CSV not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "CSV deleted successfully"})
}

// Helper function to get exceedances for a CSV
func (h *CSVHandler) getCSVExceedances(csvID string) ([]models.Exceedance, error) {
	query := `SELECT id, exceedanceValues, flightPhase, parameterName, description, eventStatus, aircraftId, flightId, file, eventId, comment, exceedanceLevel, createdAt, updatedAt FROM Exceedance WHERE flightId = ?`
	rows, err := h.db.Query(query, csvID)
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

	return exceedances, nil
}
