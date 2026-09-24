package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ReportHandler struct {
	db *sql.DB
}

func NewReportHandler(db *sql.DB) *ReportHandler {
	return &ReportHandler{db: db}
}

func (h *ReportHandler) GetOptions(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}

	companies, err := h.reportCompanies(scope)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	fleets, err := h.reportFleets(scope)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	aircraft, err := h.reportAircraft(scope, filters)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	phases, err := h.reportPhases(scope, filters)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.ReportOptionsResponse{
		Scope:     h.scopeResponse(scope),
		Filters:   filters,
		Companies: companies,
		Fleets:    fleets,
		Aircraft:  aircraft,
		Phases:    phases,
	})
}

func (h *ReportHandler) GetOverview(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}

	where, args := reportFlightWhere(scope, filters)
	flightQuery := `SELECT COUNT(1),
		COALESCE(SUM(CASE WHEN f.status IN ('completed', 'completed_with_warnings') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CAST(NULLIF(f.flightHours, '') AS REAL)), 0)
		FROM FlightLeg f JOIN Aircraft a ON a.id = f.aircraftId ` + where

	var flight models.FlightReportOverview
	if err := h.db.QueryRow(flightQuery, args...).Scan(&flight.TotalFlights, &flight.AnalyzedFlights, &flight.TotalFlightHours); err != nil {
		respondDatabaseError(c, err)
		return
	}

	exceedanceWhere, exceedanceArgs := reportExceedanceWhere(scope, filters)
	exceedanceQuery := `SELECT COUNT(1),
		COALESCE(SUM(CASE WHEN UPPER(COALESCE(e.exceedanceLevel, '')) IN ('HIGH', 'CRITICAL') THEN 1 ELSE 0 END), 0)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + exceedanceWhere

	var severity models.SeverityReportOverview
	if err := h.db.QueryRow(exceedanceQuery, exceedanceArgs...).Scan(&severity.TotalExceedances, &severity.HighCritical); err != nil {
		respondDatabaseError(c, err)
		return
	}
	flight.TotalExceedances = severity.TotalExceedances

	c.JSON(http.StatusOK, models.ReportOverviewResponse{
		Scope:    h.scopeResponse(scope),
		Filters:  filters,
		Flight:   flight,
		Severity: severity,
	})
}

func parseReportFilters(c *gin.Context, scope reportAccessScope) (models.ReportFilters, bool) {
	filters := models.ReportFilters{
		CompanyID:    scope.CompanyID,
		AircraftMake: strings.TrimSpace(c.Query("aircraftMake")),
		ModelNumber:  strings.TrimSpace(c.Query("modelNumber")),
		AircraftIDs:  normalizedQueryValues(c.QueryArray("aircraftId")),
		From:         strings.TrimSpace(c.Query("from")),
		To:           strings.TrimSpace(c.Query("to")),
		Phases:       normalizedUpperQueryValues(c.QueryArray("phase")),
	}

	if filters.ModelNumber != "" && filters.AircraftMake == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "aircraftMake is required when modelNumber is supplied"})
		return models.ReportFilters{}, false
	}
	from, fromOK := parseReportDate(filters.From)
	to, toOK := parseReportDate(filters.To)
	if !fromOK || !toOK {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must use YYYY-MM-DD"})
		return models.ReportFilters{}, false
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must not be later than to"})
		return models.ReportFilters{}, false
	}

	return filters, true
}

func parseReportDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return parsed, err == nil
}

func normalizedQueryValues(values []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				normalized = append(normalized, item)
			}
		}
	}
	sort.Strings(normalized)
	return normalized
}

func normalizedUpperQueryValues(values []string) []string {
	expanded := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			expanded = append(expanded, strings.ToUpper(item))
		}
	}
	return normalizedQueryValues(expanded)
}

func reportFlightWhere(scope reportAccessScope, filters models.ReportFilters) (string, []any) {
	conditions := []string{"1 = 1"}
	args := make([]any, 0)
	if scope.CompanyID != nil {
		conditions = append(conditions, "a.companyId = ?")
		args = append(args, *scope.CompanyID)
	}
	if filters.AircraftMake != "" {
		conditions = append(conditions, "LOWER(TRIM(a.aircraftMake)) = LOWER(?)")
		args = append(args, filters.AircraftMake)
	}
	if filters.ModelNumber != "" {
		conditions = append(conditions, "LOWER(TRIM(COALESCE(a.modelNumber, ''))) = LOWER(?)")
		args = append(args, filters.ModelNumber)
	}
	if len(filters.AircraftIDs) > 0 {
		conditions = append(conditions, "a.id IN ("+placeholders(len(filters.AircraftIDs))+")")
		for _, id := range filters.AircraftIDs {
			args = append(args, id)
		}
	}
	if filters.From != "" {
		conditions = append(conditions, "f.createdAt >= CAST(strftime('%s', ?) AS INTEGER) * 1000")
		args = append(args, filters.From)
	}
	if filters.To != "" {
		conditions = append(conditions, "f.createdAt < CAST(strftime('%s', date(?, '+1 day')) AS INTEGER) * 1000")
		args = append(args, filters.To)
	}
	if len(filters.Phases) > 0 {
		phaseConditions, phaseArgs := reportPhaseConditions("phase_e", filters.Phases)
		statusCondition := ""
		if !scope.CanViewAllStatus {
			statusCondition = " AND phase_e.eventStatus = 'Valid'"
		}
		conditions = append(conditions, "EXISTS (SELECT 1 FROM Exceedance phase_e WHERE COALESCE(phase_e.flightLegId, phase_e.flightId) = f.id AND phase_e.isCurrent = 1"+statusCondition+" AND ("+strings.Join(phaseConditions, " OR ")+"))")
		args = append(args, phaseArgs...)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func reportExceedanceWhere(scope reportAccessScope, filters models.ReportFilters) (string, []any) {
	where, args := reportFlightWhere(scope, filters)
	conditions := []string{strings.TrimPrefix(where, "WHERE ")}
	conditions = append(conditions, "e.isCurrent = 1")
	if !scope.CanViewAllStatus {
		conditions = append(conditions, "e.eventStatus = 'Valid'")
	}
	if len(filters.Phases) > 0 {
		phaseConditions, phaseArgs := reportPhaseConditions("e", filters.Phases)
		conditions = append(conditions, "("+strings.Join(phaseConditions, " OR ")+")")
		args = append(args, phaseArgs...)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func reportPhaseConditions(alias string, phases []string) ([]string, []any) {
	conditions := make([]string, 0, len(phases))
	args := make([]any, 0, len(phases))
	for _, phase := range phases {
		conditions = append(conditions, "INSTR(',' || REPLACE(UPPER("+alias+".flightPhase), ' ', '') || ',', ',' || REPLACE(UPPER(?), ' ', '') || ',') > 0")
		args = append(args, phase)
	}
	return conditions, args
}

func placeholders(count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ",")
}

func (h *ReportHandler) validateSelectedCompany(c *gin.Context, scope reportAccessScope) bool {
	if scope.CompanyID == nil {
		return true
	}
	var exists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM Company WHERE id = ?)", *scope.CompanyID).Scan(&exists); err != nil {
		respondDatabaseError(c, err)
		return false
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return false
	}
	return true
}

func (h *ReportHandler) scopeResponse(scope reportAccessScope) models.ReportScopeResponse {
	response := models.ReportScopeResponse{Mode: "global", CompanyID: scope.CompanyID, CanSelectCompany: scope.CanSelectCompany}
	if scope.CompanyID != nil {
		response.Mode = "company"
		var name string
		if h.db.QueryRow("SELECT name FROM Company WHERE id = ?", *scope.CompanyID).Scan(&name) == nil {
			response.CompanyName = stringPointer(name)
		}
	}
	return response
}

func (h *ReportHandler) reportCompanies(scope reportAccessScope) ([]models.ReportCompanyOption, error) {
	query := "SELECT id, name, status FROM Company"
	args := []any{}
	if !scope.CanSelectCompany && scope.CompanyID != nil {
		query += " WHERE id = ?"
		args = append(args, *scope.CompanyID)
	}
	query += " ORDER BY name"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.ReportCompanyOption, 0)
	for rows.Next() {
		var option models.ReportCompanyOption
		if err := rows.Scan(&option.ID, &option.Name, &option.Status); err != nil {
			return nil, err
		}
		result = append(result, option)
	}
	return result, rows.Err()
}

func (h *ReportHandler) reportFleets(scope reportAccessScope) ([]models.ReportFleetOption, error) {
	query := `SELECT DISTINCT TRIM(aircraftMake), TRIM(COALESCE(modelNumber, '')) FROM Aircraft`
	args := []any{}
	if scope.CompanyID != nil {
		query += " WHERE companyId = ?"
		args = append(args, *scope.CompanyID)
	}
	query += " ORDER BY 1, 2"
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.ReportFleetOption, 0)
	for rows.Next() {
		var option models.ReportFleetOption
		if err := rows.Scan(&option.AircraftMake, &option.ModelNumber); err != nil {
			return nil, err
		}
		option.Label = strings.TrimSpace(option.AircraftMake + " " + option.ModelNumber)
		result = append(result, option)
	}
	return result, rows.Err()
}

func (h *ReportHandler) reportAircraft(scope reportAccessScope, filters models.ReportFilters) ([]models.ReportAircraftOption, error) {
	conditions := []string{"1 = 1"}
	args := []any{}
	if scope.CompanyID != nil {
		conditions = append(conditions, "companyId = ?")
		args = append(args, *scope.CompanyID)
	}
	if filters.AircraftMake != "" {
		conditions = append(conditions, "LOWER(TRIM(aircraftMake)) = LOWER(?)")
		args = append(args, filters.AircraftMake)
	}
	if filters.ModelNumber != "" {
		conditions = append(conditions, "LOWER(TRIM(COALESCE(modelNumber, ''))) = LOWER(?)")
		args = append(args, filters.ModelNumber)
	}
	query := `SELECT id, COALESCE(registration, ''), serialNumber FROM Aircraft WHERE ` + strings.Join(conditions, " AND ") + ` ORDER BY COALESCE(registration, serialNumber)`
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.ReportAircraftOption, 0)
	for rows.Next() {
		var option models.ReportAircraftOption
		if err := rows.Scan(&option.ID, &option.Registration, &option.SerialNumber); err != nil {
			return nil, err
		}
		option.Label = option.Registration
		if option.Label == "" {
			option.Label = option.SerialNumber
		}
		result = append(result, option)
	}
	return result, rows.Err()
}

func (h *ReportHandler) reportPhases(scope reportAccessScope, filters models.ReportFilters) ([]string, error) {
	conditions := []string{"e.isCurrent = 1"}
	args := []any{}
	if scope.CompanyID != nil {
		conditions = append(conditions, "a.companyId = ?")
		args = append(args, *scope.CompanyID)
	}
	if !scope.CanViewAllStatus {
		conditions = append(conditions, "e.eventStatus = 'Valid'")
	}
	if filters.AircraftMake != "" {
		conditions = append(conditions, "LOWER(TRIM(a.aircraftMake)) = LOWER(?)")
		args = append(args, filters.AircraftMake)
	}
	if filters.ModelNumber != "" {
		conditions = append(conditions, "LOWER(TRIM(COALESCE(a.modelNumber, ''))) = LOWER(?)")
		args = append(args, filters.ModelNumber)
	}
	if len(filters.AircraftIDs) > 0 {
		conditions = append(conditions, "a.id IN ("+placeholders(len(filters.AircraftIDs))+")")
		for _, id := range filters.AircraftIDs {
			args = append(args, id)
		}
	}
	query := `SELECT DISTINCT e.flightPhase FROM Exceedance e JOIN Aircraft a ON a.id = e.aircraftId WHERE ` + strings.Join(conditions, " AND ")
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[string]bool)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		for _, phase := range strings.Split(value, ",") {
			phase = strings.ToUpper(strings.TrimSpace(phase))
			if phase != "" {
				seen[phase] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(seen))
	for phase := range seen {
		result = append(result, phase)
	}
	sort.Strings(result)
	return result, nil
}
