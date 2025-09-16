package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"fmt"
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
	
	_, err = h.db.Exec(query, id, req.Name, filename, req.AircraftID, req.Departure, req.Destination, req.FlightHours, req.Pilot, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error saving CSV record"})
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
	query := `SELECT id, name, file, status, departure, pilot, destination, flightHours, aircraftId, createdAt, updatedAt FROM Csv`
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var csvs []interface{}
	for rows.Next() {
		var csv models.CSV
		var createdAtUnix, updatedAtUnix sql.NullInt64
		
		err := rows.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
			&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning CSV"})
			return
		}

		if createdAtUnix.Valid {
			csv.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			csv.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}

		// Get related exceedances
		exceedances, _ := h.getCSVExceedances(csv.ID)

		csvWithExceedances := struct {
			models.CSV
			Exceedance []models.Exceedance `json:"Exceedance"`
		}{
			CSV:        csv,
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
	var createdAtUnix, updatedAtUnix sql.NullInt64
	row := h.db.QueryRow(query, id)
	err := row.Scan(&csv.ID, &csv.Name, &csv.File, &csv.Status, &csv.Departure, &csv.Pilot,
		&csv.Destination, &csv.FlightHours, &csv.AircraftID, &createdAtUnix, &updatedAtUnix)
	
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "CSV not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
		return
	}

	if createdAtUnix.Valid {
		csv.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
	}
	if updatedAtUnix.Valid {
		csv.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
	}
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "CSV not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
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
