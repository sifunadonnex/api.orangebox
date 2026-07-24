package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type EventHandler struct {
	db *sql.DB
}

func NewEventHandler(db *sql.DB) *EventHandler {
	return &EventHandler{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

const eventSelectColumns = `
	d.id, d.companyId, d.eventCode, d.eventType, d.lifecycleStatus,
	v.id, v.version, v.status, v.schemaVersion, v.eventName, v.displayName,
	v.eventDescription, v.sop, v.ruleJson, v.ruleHash, v.changeSummary,
	v.eventParameter, v.eventTrigger, v.flightPhase, v.triggerType,
	v.detectionPeriod, v.severities, v.primaryAircraftId, v.createdBy,
	v.approvedBy, v.validatedAt, v.publishedAt, v.effectiveFrom,
	v.effectiveTo, v.createdAt, v.updatedAt`

// CreateEvent creates a stable definition identity and its first immutable
// draft version. Drafts never participate in detection.
func (h *EventHandler) CreateEvent(c *gin.Context) {
	var req models.EventDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event definition", "details": err.Error()})
		return
	}

	validation := validateEventDefinition(&req)
	assignments, companyID, primaryAircraftID, assignmentErrors := h.resolveAssignments(c, req.Assignments, req.Rule.Parameters)
	validation.Errors = append(validation.Errors, assignmentErrors...)
	if len(validation.Errors) > 0 {
		c.JSON(http.StatusUnprocessableEntity, models.EventValidationResponse{
			Valid: false, Errors: validation.Errors, Warnings: validation.Warnings,
		})
		return
	}

	projection, err := buildLegacyProjection(req.Rule)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Rule projection failed", "details": err.Error()})
		return
	}

	userID, ok := contextString(c, "userId")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authenticated user was not found"})
		return
	}
	if h.eventCodeExists("", companyID, req.EventCode) {
		c.JSON(http.StatusConflict, gin.H{"error": "An event definition with this code already exists in the selected company"})
		return
	}

	now := time.Now().UnixMilli()
	definitionID := uuid.New().String()
	versionID := uuid.New().String()
	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO EventDefinition
		(id, companyId, eventCode, eventType, lifecycleStatus, createdBy, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, 'active', ?, ?, ?)`,
		definitionID, nullableString(companyID), req.EventCode, req.EventType, userID, now, now)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO EventDefinitionVersion
			(id, definitionId, version, status, schemaVersion, eventName, displayName,
			eventDescription, sop, ruleJson, ruleHash, changeSummary, eventParameter,
			eventTrigger, flightPhase, triggerType, detectionPeriod, severities,
			primaryAircraftId, createdBy, createdAt, updatedAt)
			VALUES (?, ?, 1, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, definitionID, req.Rule.SchemaVersion, req.EventName, req.DisplayName,
			req.EventDescription, req.SOP, validation.RuleJSON, validation.RuleHash,
			req.ChangeSummary, projection.EventParameter, projection.EventTrigger,
			projection.FlightPhase, projection.TriggerType, projection.DetectionPeriod,
			projection.Severities, nullableString(primaryAircraftID), userID, now, now)
	}
	if err == nil {
		err = insertAssignments(tx, versionID, assignments, now)
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}

	event, err := h.getEventByDefinitionID(c, definitionID, false)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusCreated, event)
}

// ValidateEventPayload performs the same authoritative checks as creation but
// does not persist anything. It powers form previews and future dry-run tools.
func (h *EventHandler) ValidateEventPayload(c *gin.Context) {
	var req models.EventDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event definition", "details": err.Error()})
		return
	}
	validation := validateEventDefinition(&req)
	_, _, _, assignmentErrors := h.resolveAssignments(c, req.Assignments, req.Rule.Parameters)
	validation.Errors = append(validation.Errors, assignmentErrors...)
	c.JSON(http.StatusOK, models.EventValidationResponse{
		Valid: len(validation.Errors) == 0, Errors: validation.Errors,
		Warnings: validation.Warnings, RuleHash: validation.RuleHash,
	})
}

// GetEvents returns published versions by default. Management screens can ask
// for includeDrafts=true; tenant scoping is always applied in SQL.
func (h *EventHandler) GetEvents(c *gin.Context) {
	includeDrafts := strings.EqualFold(c.Query("includeDrafts"), "true") && canManageEventDefinitions(c)
	versionPredicate := "v2.status = 'published'"
	if includeDrafts {
		versionPredicate = "v2.status <> 'retired'"
	}

	query := `SELECT ` + eventSelectColumns + `
		FROM EventDefinition d
		JOIN EventDefinitionVersion v ON v.id = (
			SELECT v2.id FROM EventDefinitionVersion v2
			WHERE v2.definitionId = d.id AND ` + versionPredicate + `
			ORDER BY v2.version DESC LIMIT 1
		)
		WHERE d.lifecycleStatus = 'active'`
	args := []any{}
	if !isSystemEventRole(c) {
		companyID, ok := contextString(c, "userCompanyId")
		if !ok {
			c.JSON(http.StatusOK, []models.EventDefinitionResponse{})
			return
		}
		query += " AND d.companyId = ?"
		args = append(args, companyID)
	}
	query += " ORDER BY d.eventCode, v.version DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer rows.Close()

	events := make([]models.EventDefinitionResponse, 0)
	for rows.Next() {
		event, err := h.scanEventBase(rows)
		if err != nil {
			respondDatabaseError(c, err)
			return
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = rows.Close(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	for index := range events {
		events[index].Assignments, err = h.getAssignments(events[index].VersionID)
		if err != nil {
			respondDatabaseError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, events)
}

func (h *EventHandler) GetEventByID(c *gin.Context) {
	event, err := h.getEventByDefinitionID(c, c.Param("id"), true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if errors.Is(err, errEventAccessDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot access this event definition"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, event)
}

// UpdateEvent edits an existing draft. Updating a validated or published
// version creates a new draft, preserving the exact version used historically.
func (h *EventHandler) UpdateEvent(c *gin.Context) {
	definitionID := c.Param("id")
	var req models.EventDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event definition", "details": err.Error()})
		return
	}
	validation := validateEventDefinition(&req)
	assignments, companyID, primaryAircraftID, assignmentErrors := h.resolveAssignments(c, req.Assignments, req.Rule.Parameters)
	validation.Errors = append(validation.Errors, assignmentErrors...)
	if len(validation.Errors) > 0 {
		c.JSON(http.StatusUnprocessableEntity, models.EventValidationResponse{Valid: false, Errors: validation.Errors, Warnings: validation.Warnings})
		return
	}
	projection, err := buildLegacyProjection(req.Rule)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Rule projection failed", "details": err.Error()})
		return
	}

	var existingCompany sql.NullString
	var latestVersionID, latestStatus string
	var latestVersion int
	err = h.db.QueryRow(`SELECT d.companyId, v.id, v.version, v.status
		FROM EventDefinition d JOIN EventDefinitionVersion v ON v.id = (
			SELECT id FROM EventDefinitionVersion WHERE definitionId = d.id ORDER BY version DESC LIMIT 1
		) WHERE d.id = ? AND d.lifecycleStatus = 'active'`, definitionID).
		Scan(&existingCompany, &latestVersionID, &latestVersion, &latestStatus)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !canAccessEventCompany(c, nullStringPointer(existingCompany)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You cannot update this event definition"})
		return
	}
	if !sameNullableString(existingCompany, companyID) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "A definition cannot be moved to another company; create a new definition instead"})
		return
	}
	if h.eventCodeExists(definitionID, companyID, req.EventCode) {
		c.JSON(http.StatusConflict, gin.H{"error": "An event definition with this code already exists in the selected company"})
		return
	}

	userID, _ := contextString(c, "userId")
	now := time.Now().UnixMilli()
	versionID := latestVersionID
	versionNumber := latestVersion
	updateDraft := latestStatus == models.EventVersionDraft
	if !updateDraft {
		versionID = uuid.New().String()
		versionNumber++
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE EventDefinition SET eventCode = ?, eventType = ?, updatedAt = ? WHERE id = ?`, req.EventCode, req.EventType, now, definitionID)
	if err == nil && updateDraft {
		_, err = tx.Exec(`UPDATE EventDefinitionVersion SET status = 'draft', schemaVersion = ?,
			eventName = ?, displayName = ?, eventDescription = ?, sop = ?, ruleJson = ?,
			ruleHash = ?, changeSummary = ?, eventParameter = ?, eventTrigger = ?,
			flightPhase = ?, triggerType = ?, detectionPeriod = ?, severities = ?,
			primaryAircraftId = ?, approvedBy = NULL, validatedAt = NULL,
			publishedAt = NULL, effectiveFrom = NULL, effectiveTo = NULL, updatedAt = ?
			WHERE id = ?`, req.Rule.SchemaVersion, req.EventName, req.DisplayName,
			req.EventDescription, req.SOP, validation.RuleJSON, validation.RuleHash,
			req.ChangeSummary, projection.EventParameter, projection.EventTrigger,
			projection.FlightPhase, projection.TriggerType, projection.DetectionPeriod,
			projection.Severities, nullableString(primaryAircraftID), now, versionID)
		if err == nil {
			_, err = tx.Exec("DELETE FROM EventDefinitionAssignment WHERE definitionVersionId = ?", versionID)
		}
	} else if err == nil {
		_, err = tx.Exec(`INSERT INTO EventDefinitionVersion
			(id, definitionId, version, status, schemaVersion, eventName, displayName,
			eventDescription, sop, ruleJson, ruleHash, changeSummary, eventParameter,
			eventTrigger, flightPhase, triggerType, detectionPeriod, severities,
			primaryAircraftId, createdBy, createdAt, updatedAt)
			VALUES (?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, definitionID, versionNumber, req.Rule.SchemaVersion, req.EventName,
			req.DisplayName, req.EventDescription, req.SOP, validation.RuleJSON,
			validation.RuleHash, req.ChangeSummary, projection.EventParameter,
			projection.EventTrigger, projection.FlightPhase, projection.TriggerType,
			projection.DetectionPeriod, projection.Severities, nullableString(primaryAircraftID),
			userID, now, now)
	}
	if err == nil {
		err = insertAssignments(tx, versionID, assignments, now)
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}

	event, err := h.getEventByDefinitionID(c, definitionID, false)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, event)
}

// ValidateEventVersion revalidates the persisted canonical rule and advances a
// draft to validated. Validation never implies approval or publication.
func (h *EventHandler) ValidateEventVersion(c *gin.Context) {
	event, err := h.getEventByDefinitionID(c, c.Param("id"), true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if event.Status == models.EventVersionPublished {
		c.JSON(http.StatusOK, models.EventValidationResponse{Valid: true, Errors: []string{}, Warnings: []string{}, RuleHash: event.RuleHash})
		return
	}
	if event.Status != models.EventVersionDraft && event.Status != models.EventVersionValidated {
		c.JSON(http.StatusConflict, gin.H{"error": "Only a draft can be validated"})
		return
	}

	req := requestFromResponse(event)
	validation := validateEventDefinition(&req)
	_, _, _, assignmentErrors := h.resolveAssignments(c, req.Assignments, req.Rule.Parameters)
	validation.Errors = append(validation.Errors, assignmentErrors...)
	if len(validation.Errors) > 0 {
		c.JSON(http.StatusUnprocessableEntity, models.EventValidationResponse{Valid: false, Errors: validation.Errors, Warnings: validation.Warnings})
		return
	}
	now := time.Now().UnixMilli()
	_, err = h.db.Exec(`UPDATE EventDefinitionVersion SET status = 'validated', validatedAt = ?, updatedAt = ? WHERE id = ?`, now, now, event.VersionID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.EventValidationResponse{Valid: true, Errors: []string{}, Warnings: validation.Warnings, RuleHash: validation.RuleHash})
}

// PublishEventVersion atomically retires the previous published version and
// activates the latest validated version.
func (h *EventHandler) PublishEventVersion(c *gin.Context) {
	event, err := h.getEventByDefinitionID(c, c.Param("id"), true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if event.Status != models.EventVersionValidated {
		c.JSON(http.StatusConflict, gin.H{"error": "The latest version must be validated before publication"})
		return
	}
	userID, _ := contextString(c, "userId")
	now := time.Now().UnixMilli()
	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE EventDefinitionVersion SET status = 'retired', effectiveTo = ?, updatedAt = ?
		WHERE definitionId = ? AND status = 'published'`, now, now, event.ID)
	if err == nil {
		_, err = tx.Exec(`UPDATE EventDefinitionVersion SET status = 'published', approvedBy = ?,
			publishedAt = ?, effectiveFrom = ?, effectiveTo = NULL, updatedAt = ? WHERE id = ?`,
			userID, now, now, now, event.VersionID)
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	published, err := h.getEventByDefinitionID(c, event.ID, false)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, published)
}

// DeleteEvent is a soft retirement. Historical versions remain available to
// explain exceedances that were produced under them.
func (h *EventHandler) DeleteEvent(c *gin.Context) {
	id := c.Param("id")
	now := time.Now().UnixMilli()
	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE EventDefinition SET lifecycleStatus = 'retired', updatedAt = ? WHERE id = ? AND lifecycleStatus = 'active'`, now, id)
	if err == nil {
		_, err = tx.Exec(`UPDATE EventDefinitionVersion SET status = 'retired', effectiveTo = COALESCE(effectiveTo, ?), updatedAt = ? WHERE definitionId = ?`, now, now, id)
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Event definition retired successfully"})
}

var errEventAccessDenied = errors.New("event definition access denied")

func (h *EventHandler) getEventByDefinitionID(c *gin.Context, definitionID string, enforceAccess bool) (models.EventDefinitionResponse, error) {
	query := `SELECT ` + eventSelectColumns + `
		FROM EventDefinition d JOIN EventDefinitionVersion v ON v.id = (
			SELECT id FROM EventDefinitionVersion WHERE definitionId = d.id ORDER BY version DESC LIMIT 1
		) WHERE d.id = ?`
	event, err := h.scanEvent(h.db.QueryRow(query, definitionID))
	if err != nil {
		return models.EventDefinitionResponse{}, err
	}
	if enforceAccess && !canAccessEventCompany(c, event.CompanyID) {
		return models.EventDefinitionResponse{}, errEventAccessDenied
	}
	return event, nil
}

func (h *EventHandler) scanEvent(scanner rowScanner) (models.EventDefinitionResponse, error) {
	event, err := h.scanEventBase(scanner)
	if err != nil {
		return event, err
	}
	event.Assignments, err = h.getAssignments(event.VersionID)
	return event, err
}

func (h *EventHandler) scanEventBase(scanner rowScanner) (models.EventDefinitionResponse, error) {
	var event models.EventDefinitionResponse
	var companyID, primaryAircraftID, approvedBy sql.NullString
	var validatedAt, publishedAt, effectiveFrom, effectiveTo sql.NullInt64
	var createdAt, updatedAt int64
	var ruleJSON string
	err := scanner.Scan(
		&event.ID, &companyID, &event.EventCode, &event.EventType, &event.LifecycleStatus,
		&event.VersionID, &event.Version, &event.Status, &event.SchemaVersion,
		&event.EventName, &event.DisplayName, &event.EventDescription, &event.SOP,
		&ruleJSON, &event.RuleHash, &event.ChangeSummary, &event.EventParameter,
		&event.EventTrigger, &event.FlightPhase, &event.TriggerType,
		&event.DetectionPeriod, &event.Severities, &primaryAircraftID,
		&event.CreatedBy, &approvedBy, &validatedAt, &publishedAt, &effectiveFrom,
		&effectiveTo, &createdAt, &updatedAt,
	)
	if err != nil {
		return event, err
	}
	if err = json.Unmarshal([]byte(ruleJSON), &event.Rule); err != nil {
		return event, fmt.Errorf("definition %s contains invalid canonical rule JSON: %w", event.ID, err)
	}
	event.CompanyID = nullStringPointer(companyID)
	event.ApprovedBy = nullStringPointer(approvedBy)
	event.AircraftID = primaryAircraftID.String
	event.ValidatedAt = nullableTime(validatedAt)
	event.PublishedAt = nullableTime(publishedAt)
	event.EffectiveFrom = nullableTime(effectiveFrom)
	event.EffectiveTo = nullableTime(effectiveTo)
	event.CreatedAt = time.UnixMilli(createdAt)
	event.UpdatedAt = time.UnixMilli(updatedAt)
	event.IsActive = event.LifecycleStatus == models.EventLifecycleActive && event.Status == models.EventVersionPublished
	return event, nil
}

func (h *EventHandler) getAssignments(versionID string) ([]models.EventDefinitionAssignment, error) {
	rows, err := h.db.Query(`SELECT id, scopeType, companyId, aircraftId, aircraftMake, modelNumber
		FROM EventDefinitionAssignment WHERE definitionVersionId = ? ORDER BY scopeType, id`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assignments := make([]models.EventDefinitionAssignment, 0)
	for rows.Next() {
		var assignment models.EventDefinitionAssignment
		var companyID, aircraftID, aircraftMake, modelNumber sql.NullString
		if err := rows.Scan(&assignment.ID, &assignment.ScopeType, &companyID, &aircraftID, &aircraftMake, &modelNumber); err != nil {
			return nil, err
		}
		assignment.CompanyID = nullStringPointer(companyID)
		assignment.AircraftID = nullStringPointer(aircraftID)
		assignment.AircraftMake = nullStringPointer(aircraftMake)
		assignment.ModelNumber = nullStringPointer(modelNumber)
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}

func (h *EventHandler) resolveAssignments(c *gin.Context, inputs []models.EventAssignmentInput, parameters []models.RuleParameter) ([]models.EventAssignmentInput, *string, *string, []string) {
	errorsFound := []string{}
	resolved := make([]models.EventAssignmentInput, 0, len(inputs))
	seen := make(map[string]bool)
	var definitionCompanyID *string
	var primaryAircraftID *string

	for index, input := range inputs {
		input.ScopeType = strings.ToLower(strings.TrimSpace(input.ScopeType))
		var companyID string
		switch input.ScopeType {
		case "aircraft":
			if input.AircraftID == nil || strings.TrimSpace(*input.AircraftID) == "" {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d].aircraftId is required", index))
				continue
			}
			aircraftID := strings.TrimSpace(*input.AircraftID)
			var makeName string
			var modelNumber sql.NullString
			if err := h.db.QueryRow("SELECT companyId, aircraftMake, modelNumber FROM Aircraft WHERE id = ?", aircraftID).Scan(&companyID, &makeName, &modelNumber); err != nil {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] references an unknown aircraft", index))
				continue
			}
			input.AircraftID = stringPointer(aircraftID)
			input.CompanyID = stringPointer(companyID)
			input.AircraftMake = nil
			input.ModelNumber = nil
			if primaryAircraftID == nil {
				primaryAircraftID = stringPointer(aircraftID)
			}
		case "model":
			if input.CompanyID == nil || input.AircraftMake == nil || input.ModelNumber == nil {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] model scope requires companyId, aircraftMake, and modelNumber", index))
				continue
			}
			companyID = strings.TrimSpace(*input.CompanyID)
			makeName := strings.TrimSpace(*input.AircraftMake)
			modelNumber := strings.TrimSpace(*input.ModelNumber)
			var count int
			if err := h.db.QueryRow("SELECT COUNT(1) FROM Aircraft WHERE companyId = ? AND aircraftMake = ? AND modelNumber = ?", companyID, makeName, modelNumber).Scan(&count); err != nil || count == 0 {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] does not match an aircraft model in the company", index))
				continue
			}
			input.CompanyID = stringPointer(companyID)
			input.AircraftMake = stringPointer(makeName)
			input.ModelNumber = stringPointer(modelNumber)
			input.AircraftID = nil
		case "company":
			if input.CompanyID == nil || strings.TrimSpace(*input.CompanyID) == "" {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d].companyId is required", index))
				continue
			}
			companyID = strings.TrimSpace(*input.CompanyID)
			var count int
			if err := h.db.QueryRow("SELECT COUNT(1) FROM Company WHERE id = ?", companyID).Scan(&count); err != nil || count == 0 {
				errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] references an unknown company", index))
				continue
			}
			input.CompanyID = stringPointer(companyID)
			input.AircraftID, input.AircraftMake, input.ModelNumber = nil, nil, nil
		default:
			errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d].scopeType must be company, model, or aircraft", index))
			continue
		}

		if definitionCompanyID == nil {
			definitionCompanyID = stringPointer(companyID)
		} else if *definitionCompanyID != companyID {
			errorsFound = append(errorsFound, "all assignments for a definition must belong to the same company")
		}
		if !canAccessEventCompany(c, stringPointer(companyID)) {
			errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] is outside your company", index))
		}
		keyBytes, _ := json.Marshal(input)
		key := string(keyBytes)
		if seen[key] {
			errorsFound = append(errorsFound, fmt.Sprintf("assignments[%d] duplicates another assignment", index))
			continue
		}
		seen[key] = true
		resolved = append(resolved, input)
	}

	if len(errorsFound) == 0 {
		errorsFound = append(errorsFound, h.validateParameterCoverage(resolved, parameters)...)
	}
	return resolved, definitionCompanyID, primaryAircraftID, errorsFound
}

func (h *EventHandler) validateParameterCoverage(assignments []models.EventAssignmentInput, parameters []models.RuleParameter) []string {
	required := make(map[string]bool)
	for _, parameter := range parameters {
		if parameter.Required {
			required[normalizeParameterForMatch(parameter.ID)] = true
		}
	}
	if len(required) == 0 {
		return nil
	}
	errorsFound := []string{}
	checkedAircraft := make(map[string]bool)
	for _, assignment := range assignments {
		query := "SELECT id, parameters FROM Aircraft WHERE companyId = ?"
		args := []any{*assignment.CompanyID}
		if assignment.ScopeType == "aircraft" {
			query += " AND id = ?"
			args = append(args, *assignment.AircraftID)
		} else if assignment.ScopeType == "model" {
			query += " AND aircraftMake = ? AND modelNumber = ?"
			args = append(args, *assignment.AircraftMake, *assignment.ModelNumber)
		}
		rows, err := h.db.Query(query, args...)
		if err != nil {
			return []string{"aircraft parameter coverage could not be checked"}
		}
		for rows.Next() {
			var aircraftID string
			var raw sql.NullString
			if err := rows.Scan(&aircraftID, &raw); err != nil {
				errorsFound = append(errorsFound, "aircraft parameter coverage could not be read")
				continue
			}
			if checkedAircraft[aircraftID] {
				continue
			}
			checkedAircraft[aircraftID] = true
			available := parseAircraftParameters(raw.String)
			for parameter := range required {
				if !available[parameter] {
					errorsFound = append(errorsFound, fmt.Sprintf("aircraft %s does not declare required parameter %s", aircraftID, parameter))
				}
			}
		}
		rows.Close()
	}
	return errorsFound
}

func parseAircraftParameters(raw string) map[string]bool {
	available := make(map[string]bool)
	var values []any
	if json.Unmarshal([]byte(raw), &values) != nil {
		return available
	}
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			available[normalizeParameterForMatch(typed)] = true
		case map[string]any:
			for _, key := range []string{"id", "name", "parameter", "label"} {
				if text, ok := typed[key].(string); ok {
					available[normalizeParameterForMatch(text)] = true
				}
			}
		}
	}
	return available
}

func insertAssignments(tx *sql.Tx, versionID string, assignments []models.EventAssignmentInput, now int64) error {
	for _, assignment := range assignments {
		_, err := tx.Exec(`INSERT INTO EventDefinitionAssignment
			(id, definitionVersionId, scopeType, companyId, aircraftId, aircraftMake, modelNumber, createdAt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New().String(), versionID,
			assignment.ScopeType, nullableString(assignment.CompanyID), nullableString(assignment.AircraftID),
			nullableString(assignment.AircraftMake), nullableString(assignment.ModelNumber), now)
		if err != nil {
			return err
		}
	}
	return nil
}

func requestFromResponse(event models.EventDefinitionResponse) models.EventDefinitionRequest {
	assignments := make([]models.EventAssignmentInput, 0, len(event.Assignments))
	for _, assignment := range event.Assignments {
		assignments = append(assignments, models.EventAssignmentInput{
			ScopeType: assignment.ScopeType, CompanyID: assignment.CompanyID,
			AircraftID: assignment.AircraftID, AircraftMake: assignment.AircraftMake,
			ModelNumber: assignment.ModelNumber,
		})
	}
	return models.EventDefinitionRequest{
		EventCode: event.EventCode, EventType: event.EventType, EventName: event.EventName,
		DisplayName: event.DisplayName, EventDescription: event.EventDescription,
		SOP: event.SOP, ChangeSummary: event.ChangeSummary, Rule: event.Rule,
		Assignments: assignments,
	}
}

func (h *EventHandler) eventCodeExists(excludeID string, companyID *string, code string) bool {
	query := "SELECT COUNT(1) FROM EventDefinition WHERE eventCode = ? COLLATE NOCASE AND lifecycleStatus = 'active'"
	args := []any{code}
	if companyID == nil {
		query += " AND companyId IS NULL"
	} else {
		query += " AND companyId = ?"
		args = append(args, *companyID)
	}
	if excludeID != "" {
		query += " AND id <> ?"
		args = append(args, excludeID)
	}
	var count int
	return h.db.QueryRow(query, args...).Scan(&count) == nil && count > 0
}

func canManageEventDefinitions(c *gin.Context) bool {
	role, _ := contextString(c, "userRole")
	return role == models.RoleAdmin || role == models.RoleFDA || role == models.RoleGatekeeper
}

func isSystemEventRole(c *gin.Context) bool {
	role, _ := contextString(c, "userRole")
	return role == models.RoleAdmin || role == models.RoleFDA
}

func canAccessEventCompany(c *gin.Context, companyID *string) bool {
	if isSystemEventRole(c) {
		return true
	}
	userCompanyID, ok := contextString(c, "userCompanyId")
	return ok && companyID != nil && userCompanyID == *companyID
}

func contextString(c *gin.Context, key string) (string, bool) {
	value, exists := c.Get(key)
	if !exists {
		return "", false
	}
	text, ok := value.(string)
	return text, ok && text != ""
}

func nullableString(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return stringPointer(value.String)
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}

func nullableTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := time.UnixMilli(value.Int64)
	return &parsed
}

func sameNullableString(value sql.NullString, other *string) bool {
	if !value.Valid {
		return other == nil
	}
	return other != nil && value.String == *other
}

func normalizeParameterForMatch(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func respondDatabaseError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Database operation failed", "details": err.Error()})
}
