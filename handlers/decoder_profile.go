package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fdm-backend/ingestion"
	"fdm-backend/models"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DecoderProfileHandler struct {
	db *sql.DB
}

const maxDecoderValidationRecordingBytes = 128 * 1024 * 1024

func NewDecoderProfileHandler(db *sql.DB) *DecoderProfileHandler {
	return &DecoderProfileHandler{db: db}
}

func (h *DecoderProfileHandler) List(c *gin.Context) {
	aircraftID := c.Param("id")
	if !h.canManageAircraft(c, aircraftID) {
		return
	}
	rows, err := h.db.Query(`SELECT id, aircraftId, version, name, parameterFormat,
		parameterFileName, decoderConfig, checksum, notes, status, validationStatus,
		validationSummary, validationRecordingName, validationRecordingChecksum,
		validatedBy, validatedAt, createdBy, publishedBy, createdAt, publishedAt
		FROM AircraftDecoderProfile WHERE aircraftId = ? ORDER BY version DESC`, aircraftID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer rows.Close()
	profiles := make([]models.AircraftDecoderProfile, 0)
	for rows.Next() {
		profile, scanErr := scanDecoderProfile(rows, false)
		if scanErr != nil {
			respondDatabaseError(c, scanErr)
			return
		}
		profiles = append(profiles, profile)
	}
	c.JSON(http.StatusOK, profiles)
}

func (h *DecoderProfileHandler) Get(c *gin.Context) {
	profile, companyID, err := h.loadProfile(c.Param("profileId"), true)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Decoder profile not found"})
			return
		}
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot access this decoder profile"})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (h *DecoderProfileHandler) Create(c *gin.Context) {
	aircraftID := c.Param("id")
	if !h.canManageAircraft(c, aircraftID) {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A parameter definition file is required"})
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > ingestion.MaxParameterFileBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Parameter definition must be between 1 byte and 10 MB"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Parameter definition could not be opened"})
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, ingestion.MaxParameterFileBytes+1))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Parameter definition could not be read"})
		return
	}
	summary, err := ingestion.ValidateParameterProfile(fileHeader.Filename, content)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Parameter definition is invalid", "details": err.Error()})
		return
	}
	decoderConfig, err := ingestion.ValidateDecoderConfig(c.PostForm("decoderConfig"))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(fileHeader.Filename), filepath.Ext(fileHeader.Filename))
	}
	if len(name) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Profile name must be 120 characters or fewer"})
		return
	}
	notes := strings.TrimSpace(c.PostForm("notes"))
	if notes == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Change notes are required"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version), 0) + 1 FROM AircraftDecoderProfile WHERE aircraftId = ?`, aircraftID).Scan(&version); err != nil {
		respondDatabaseError(c, err)
		return
	}
	validationJSON, _ := json.Marshal(gin.H{
		"structural":          summary,
		"recordingValidation": "required",
	})
	hash := sha256.Sum256(append(append([]byte{}, content...), []byte(decoderConfig)...))
	id, now := uuid.NewString(), time.Now()
	createdBy, _ := contextString(c, "userId")
	_, err = tx.Exec(`INSERT INTO AircraftDecoderProfile
		(id, aircraftId, version, name, parameterFormat, parameterFileName,
		 parameterText, decoderConfig, checksum, notes, status, validationStatus,
		 validationSummary, createdBy, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'draft', 'failed', ?, ?, ?)`,
		id, aircraftID, version, name, summary.Format, filepath.Base(fileHeader.Filename),
		string(content), decoderConfig, hex.EncodeToString(hash[:]), notes,
		string(validationJSON), optionalStringPointer(createdBy), now.UnixMilli())
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": id, "aircraftId": aircraftID, "version": version, "name": name,
		"parameterFormat": summary.Format, "parameterFileName": filepath.Base(fileHeader.Filename),
		"decoderConfig": decoderConfig, "checksum": hex.EncodeToString(hash[:]), "notes": notes,
		"status": "draft", "validationStatus": "failed", "validationSummary": string(validationJSON),
		"createdBy": optionalStringPointer(createdBy), "createdAt": now,
	})
}

func (h *DecoderProfileHandler) ValidateRecording(c *gin.Context) {
	profile, companyID, err := h.loadProfile(c.Param("profileId"), true)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Decoder profile not found"})
			return
		}
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot validate this decoder profile"})
		return
	}
	if profile.ParameterFormat != "fred" {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Executable recording validation currently supports ARINC 647A FRED profiles",
		})
		return
	}
	if profile.ParameterText == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Decoder profile has no parameter definition"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDecoderValidationRecordingBytes+4*1024*1024)
	fileHeader, err := c.FormFile("recording")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A controlled recorder sample is required"})
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > maxDecoderValidationRecordingBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation recording must be between 1 byte and 128 MB"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Validation recording could not be opened"})
		return
	}
	defer file.Close()
	recording, err := io.ReadAll(io.LimitReader(file, maxDecoderValidationRecordingBytes+1))
	if err != nil || len(recording) > maxDecoderValidationRecordingBytes {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Validation recording could not be read within the 128 MB limit"})
		return
	}

	prepared, preparationErr := ingestion.PrepareRecorderPayload(fileHeader.Filename, recording)
	var report ingestion.RecordingValidationReport
	validationErr := preparationErr
	if validationErr == nil {
		report, validationErr = ingestion.ValidateFREDRecording([]byte(*profile.ParameterText), prepared.Payload)
	}
	status := "passed"
	var validationJSON []byte
	if validationErr != nil {
		status = "failed"
		validationJSON, _ = json.Marshal(gin.H{"error": validationErr.Error(), "report": report})
	} else {
		validationJSON, _ = json.Marshal(report)
	}
	hash := sha256.Sum256(recording)
	now := time.Now()
	validatedBy, _ := contextString(c, "userId")
	_, err = h.db.Exec(`UPDATE AircraftDecoderProfile SET validationStatus = ?, validationSummary = ?,
		validationRecordingName = ?, validationRecordingChecksum = ?, validatedBy = ?, validatedAt = ?
		WHERE id = ?`, status, string(validationJSON), filepath.Base(fileHeader.Filename),
		hex.EncodeToString(hash[:]), optionalStringPointer(validatedBy), now.UnixMilli(), profile.ID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if validationErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Recording validation failed", "details": validationErr.Error(), "validation": report,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "status": status, "validation": report,
		"recordingName": filepath.Base(fileHeader.Filename), "validatedAt": now,
		"recorderAdapter": prepared.Adapter, "containerFormat": prepared.ContainerFormat,
	})
}

func (h *DecoderProfileHandler) Publish(c *gin.Context) {
	profile, companyID, err := h.loadProfile(c.Param("profileId"), false)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Decoder profile not found"})
			return
		}
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot publish this decoder profile"})
		return
	}
	if profile.ValidationStatus != "passed" {
		c.JSON(http.StatusConflict, gin.H{"error": "Only a validated decoder profile can be published"})
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	now := time.Now()
	publishedBy, _ := contextString(c, "userId")
	if _, err = tx.Exec(`UPDATE AircraftDecoderProfile SET status = 'retired'
		WHERE aircraftId = ? AND status = 'published' AND id <> ?`, profile.AircraftID, profile.ID); err != nil {
		respondDatabaseError(c, err)
		return
	}
	result, err := tx.Exec(`UPDATE AircraftDecoderProfile SET status = 'published', publishedBy = ?, publishedAt = ?
		WHERE id = ? AND status IN ('draft', 'retired')`, optionalStringPointer(publishedBy), now.UnixMilli(), profile.ID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 && profile.Status != "published" {
		c.JSON(http.StatusConflict, gin.H{"error": "Decoder profile cannot be published from its current state"})
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "id": profile.ID, "status": "published", "publishedAt": now})
}

func (h *DecoderProfileHandler) Delete(c *gin.Context) {
	profile, companyID, err := h.loadProfile(c.Param("profileId"), false)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Decoder profile not found"})
			return
		}
		respondDatabaseError(c, err)
		return
	}
	if !canAccessCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot delete this decoder profile"})
		return
	}
	if profile.Status == "published" {
		c.JSON(http.StatusConflict, gin.H{"error": "Published decoder profiles cannot be deleted; publish a replacement instead"})
		return
	}
	if _, err = h.db.Exec(`DELETE FROM AircraftDecoderProfile WHERE id = ? AND status <> 'published'`, profile.ID); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DecoderProfileHandler) canManageAircraft(c *gin.Context, aircraftID string) bool {
	var companyID string
	err := h.db.QueryRow(`SELECT companyId FROM Aircraft WHERE id = ?`, aircraftID).Scan(&companyID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Aircraft not found"})
		return false
	}
	if err != nil {
		respondDatabaseError(c, err)
		return false
	}
	if !canAccessCompany(c, companyID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot manage decoder profiles for this aircraft"})
		return false
	}
	return true
}

func (h *DecoderProfileHandler) loadProfile(id string, includeText bool) (models.AircraftDecoderProfile, string, error) {
	columns := `p.id, p.aircraftId, p.version, p.name, p.parameterFormat,
		p.parameterFileName, p.decoderConfig, p.checksum, p.notes, p.status,
		p.validationStatus, p.validationSummary, p.validationRecordingName,
		p.validationRecordingChecksum, p.validatedBy, p.validatedAt,
		p.createdBy, p.publishedBy, p.createdAt, p.publishedAt`
	if includeText {
		columns += `, p.parameterText`
	}
	row := h.db.QueryRow(`SELECT `+columns+`, a.companyId FROM AircraftDecoderProfile p
		JOIN Aircraft a ON a.id = p.aircraftId WHERE p.id = ?`, id)
	profile, companyID, err := scanDecoderProfileWithCompany(row, includeText)
	return profile, companyID, err
}

func scanDecoderProfile(scanner rowScanner, includeText bool) (models.AircraftDecoderProfile, error) {
	var profile models.AircraftDecoderProfile
	var createdAt nullableTimestamp
	var publishedAt nullableTimestamp
	var validatedAt nullableTimestamp
	destinations := []any{
		&profile.ID, &profile.AircraftID, &profile.Version, &profile.Name,
		&profile.ParameterFormat, &profile.ParameterFileName, &profile.DecoderConfig,
		&profile.Checksum, &profile.Notes, &profile.Status, &profile.ValidationStatus,
		&profile.ValidationSummary, &profile.ValidationRecordingName,
		&profile.ValidationRecordingChecksum, &profile.ValidatedBy, &validatedAt,
		&profile.CreatedBy, &profile.PublishedBy,
		&createdAt, &publishedAt,
	}
	if includeText {
		destinations = append(destinations, &profile.ParameterText)
	}
	if err := scanner.Scan(destinations...); err != nil {
		return profile, err
	}
	applyDecoderProfileTimes(&profile, validatedAt, createdAt, publishedAt)
	return profile, nil
}

func scanDecoderProfileWithCompany(scanner rowScanner, includeText bool) (models.AircraftDecoderProfile, string, error) {
	var profile models.AircraftDecoderProfile
	var createdAt nullableTimestamp
	var publishedAt nullableTimestamp
	var validatedAt nullableTimestamp
	var companyID string
	destinations := []any{
		&profile.ID, &profile.AircraftID, &profile.Version, &profile.Name,
		&profile.ParameterFormat, &profile.ParameterFileName, &profile.DecoderConfig,
		&profile.Checksum, &profile.Notes, &profile.Status, &profile.ValidationStatus,
		&profile.ValidationSummary, &profile.ValidationRecordingName,
		&profile.ValidationRecordingChecksum, &profile.ValidatedBy, &validatedAt,
		&profile.CreatedBy, &profile.PublishedBy,
		&createdAt, &publishedAt,
	}
	if includeText {
		destinations = append(destinations, &profile.ParameterText)
	}
	destinations = append(destinations, &companyID)
	if err := scanner.Scan(destinations...); err != nil {
		return profile, "", err
	}
	applyDecoderProfileTimes(&profile, validatedAt, createdAt, publishedAt)
	return profile, companyID, nil
}

func applyDecoderProfileTimes(profile *models.AircraftDecoderProfile, validatedAt, createdAt, publishedAt nullableTimestamp) {
	if validatedAt.Valid {
		value := validatedAt.Time
		profile.ValidatedAt = &value
	}
	if createdAt.Valid {
		profile.CreatedAt = createdAt.Time
	}
	if publishedAt.Valid {
		value := publishedAt.Time
		profile.PublishedAt = &value
	}
}
