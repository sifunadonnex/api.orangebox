package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// GetProgrammeReport returns validated safety results. Provisional, nuisance,
// and false detections are exposed only as review workload and never inflate
// the published occurrence rates.
func (h *ReportHandler) GetProgrammeReport(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}
	filters = programmeDefaultRange(filters, time.Now().UTC())

	validatedScope := scope
	validatedScope.CanViewAllStatus = false
	flightWhere, flightArgs := reportFlightWhere(validatedScope, filters)
	validWhere, validArgs := programmeExceedanceWhere(scope, filters, "e.eventStatus = 'Valid'")
	allWhere, allArgs := programmeExceedanceWhere(scope, filters, "1 = 1")

	months, err := h.programmeMonths(filters, flightWhere, flightArgs, validWhere, validArgs)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	summary, err := h.programmeSummary(flightWhere, flightArgs, validWhere, validArgs, allWhere, allArgs)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	events, err := h.programmeEvents(summary.Flights, validWhere, validArgs)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	aircraft, err := h.programmeAircraft(flightWhere, flightArgs, validWhere, validArgs)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	priority, err := h.programmePriority(validWhere, validArgs)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	targets, err := h.programmeTargets(scope.CompanyID)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.ProgrammeReportResponse{
		Scope: h.scopeResponse(scope), Filters: filters, Summary: summary, Months: months,
		Events: events, Aircraft: aircraft, Priority: priority, Targets: targets,
		StatusPolicy: "Validated occurrences only; provisional, nuisance, and false detections are excluded from rates.",
		AsOf:         time.Now().UTC().Format(time.RFC3339),
	})
}

func programmeDefaultRange(filters models.ReportFilters, now time.Time) models.ReportFilters {
	end := now.UTC()
	if parsed, ok := parseReportDate(filters.To); ok && !parsed.IsZero() {
		end = parsed
	}
	if filters.To == "" {
		filters.To = end.Format("2006-01-02")
	}
	if filters.From == "" {
		start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
		filters.From = start.Format("2006-01-02")
	}
	return filters
}

func programmeExceedanceWhere(scope reportAccessScope, filters models.ReportFilters, statusCondition string) (string, []any) {
	allStatusScope := scope
	allStatusScope.CanViewAllStatus = true
	where, args := reportExceedanceWhere(allStatusScope, filters)
	return where + " AND " + statusCondition, args
}

func programmeMonthKeyExpression() string {
	return "strftime('%Y-%m', f.createdAt / 1000, 'unixepoch')"
}

func (h *ReportHandler) programmeMonths(filters models.ReportFilters, flightWhere string, flightArgs []any, validWhere string, validArgs []any) ([]models.ProgrammeMonth, error) {
	months := make(map[string]*models.ProgrammeMonth)
	from, _ := parseReportDate(filters.From)
	to, _ := parseReportDate(filters.To)
	if !from.IsZero() && !to.IsZero() {
		cursor := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
		for count := 0; !cursor.After(to) && count < 60; count++ {
			key := cursor.Format("2006-01")
			months[key] = &models.ProgrammeMonth{Month: key}
			cursor = cursor.AddDate(0, 1, 0)
		}
	}

	flightRows, err := h.db.Query(`SELECT `+programmeMonthKeyExpression()+`, COUNT(DISTINCT f.id)
		FROM FlightLeg f JOIN Aircraft a ON a.id = f.aircraftId `+flightWhere+`
		AND f.status IN ('completed', 'completed_with_warnings') GROUP BY 1`, flightArgs...)
	if err != nil {
		return nil, err
	}
	for flightRows.Next() {
		var key string
		var count int
		if err = flightRows.Scan(&key, &count); err != nil {
			flightRows.Close()
			return nil, err
		}
		if months[key] == nil {
			months[key] = &models.ProgrammeMonth{Month: key}
		}
		months[key].Flights = count
	}
	if err = flightRows.Close(); err != nil {
		return nil, err
	}

	eventRows, err := h.db.Query(`SELECT `+programmeMonthKeyExpression()+`, COUNT(1),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) IN ('HIGH', 'CRITICAL') THEN 1 ELSE 0 END)
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId `+validWhere+` GROUP BY 1`, validArgs...)
	if err != nil {
		return nil, err
	}
	for eventRows.Next() {
		var key string
		var count, highCritical int
		if err = eventRows.Scan(&key, &count, &highCritical); err != nil {
			eventRows.Close()
			return nil, err
		}
		if months[key] == nil {
			months[key] = &models.ProgrammeMonth{Month: key}
		}
		months[key].Occurrences = count
		months[key].HighCritical = highCritical
	}
	if err = eventRows.Close(); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(months))
	for key := range months {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]models.ProgrammeMonth, 0, len(keys))
	for _, key := range keys {
		month := months[key]
		if month.Flights > 0 {
			month.EventRatePer100Flights = float64(month.Occurrences) * 100 / float64(month.Flights)
			month.HighCriticalRatePer1000 = float64(month.HighCritical) * 1000 / float64(month.Flights)
		}
		result = append(result, *month)
	}
	return result, nil
}

func (h *ReportHandler) programmeSummary(flightWhere string, flightArgs []any, validWhere string, validArgs []any, allWhere string, allArgs []any) (models.ProgrammeSummary, error) {
	var summary models.ProgrammeSummary
	if err := h.db.QueryRow(`SELECT COUNT(DISTINCT f.id) FROM FlightLeg f JOIN Aircraft a ON a.id = f.aircraftId `+flightWhere+`
		AND f.status IN ('completed', 'completed_with_warnings')`, flightArgs...).Scan(&summary.Flights); err != nil {
		return summary, err
	}
	if err := h.db.QueryRow(`SELECT COUNT(1), COALESCE(SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) IN ('HIGH', 'CRITICAL') THEN 1 ELSE 0 END), 0)
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId `+validWhere, validArgs...).Scan(&summary.Occurrences, &summary.HighCritical); err != nil {
		return summary, err
	}
	if err := h.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN e.eventStatus = 'Pending' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN e.eventStatus IN ('Under Review', 'Nuisance') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN e.eventStatus = 'False' THEN 1 ELSE 0 END), 0)
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId `+allWhere, allArgs...).Scan(&summary.PendingReview, &summary.UnderReview, &summary.FalseOccurrences); err != nil {
		return summary, err
	}
	if summary.Flights > 0 {
		summary.EventRatePer100Flights = float64(summary.Occurrences) * 100 / float64(summary.Flights)
		summary.HighCriticalRatePer1000 = float64(summary.HighCritical) * 1000 / float64(summary.Flights)
	}
	return summary, nil
}

func (h *ReportHandler) programmeEvents(flights int, validWhere string, validArgs []any) ([]models.ProgrammeEvent, error) {
	query := `SELECT COALESCE(v.definitionId, e.eventId, e.parameterName, 'unlabelled'),
		COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode, e.parameterName, 'Unlabelled event'),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'LOW' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'MEDIUM' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'HIGH' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'CRITICAL' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) NOT IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL') THEN 1 ELSE 0 END), COUNT(1)
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		LEFT JOIN EventDefinitionVersion v ON v.id = e.eventId
		LEFT JOIN EventDefinition d ON d.id = v.definitionId ` + validWhere + `
		GROUP BY 1, 2 ORDER BY COUNT(1) DESC, 2`
	rows, err := h.db.Query(query, validArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.ProgrammeEvent, 0)
	for rows.Next() {
		var item models.ProgrammeEvent
		if err = rows.Scan(&item.Key, &item.Label, &item.Low, &item.Medium, &item.High, &item.Critical, &item.Other, &item.Total); err != nil {
			return nil, err
		}
		if flights > 0 {
			item.RatePer100Flights = float64(item.Total) * 100 / float64(flights)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (h *ReportHandler) programmeAircraft(flightWhere string, flightArgs []any, validWhere string, validArgs []any) ([]models.ProgrammeAircraft, error) {
	items := make(map[string]*models.ProgrammeAircraft)
	rows, err := h.db.Query(`SELECT a.id, COALESCE(NULLIF(a.registration, ''), a.serialNumber), COUNT(DISTINCT f.id)
		FROM FlightLeg f JOIN Aircraft a ON a.id = f.aircraftId `+flightWhere+`
		AND f.status IN ('completed', 'completed_with_warnings') GROUP BY a.id, 2`, flightArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		item := &models.ProgrammeAircraft{}
		if err = rows.Scan(&item.AircraftID, &item.Registration, &item.Flights); err != nil {
			rows.Close()
			return nil, err
		}
		items[item.AircraftID] = item
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	eventRows, err := h.db.Query(`SELECT a.id, COUNT(1),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) IN ('HIGH', 'CRITICAL') THEN 1 ELSE 0 END)
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId `+validWhere+` GROUP BY a.id`, validArgs...)
	if err != nil {
		return nil, err
	}
	for eventRows.Next() {
		var id string
		var occurrences, highCritical int
		if err = eventRows.Scan(&id, &occurrences, &highCritical); err != nil {
			eventRows.Close()
			return nil, err
		}
		if item := items[id]; item != nil {
			item.Occurrences = occurrences
			item.HighCritical = highCritical
		}
	}
	if err = eventRows.Close(); err != nil {
		return nil, err
	}

	result := make([]models.ProgrammeAircraft, 0, len(items))
	for _, item := range items {
		if item.Flights > 0 {
			item.RatePer100Flights = float64(item.Occurrences) * 100 / float64(item.Flights)
		}
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Registration < result[j].Registration })
	return result, nil
}

func (h *ReportHandler) programmePriority(validWhere string, validArgs []any) ([]models.ProgrammePriorityOccurrence, error) {
	query := `SELECT e.id, f.id, COALESCE(f.name, ''), a.id,
		COALESCE(NULLIF(a.registration, ''), a.serialNumber),
		COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode, e.parameterName, 'Unlabelled event'),
		COALESCE(e.exceedanceLevel, ''), COALESCE(e.flightPhase, ''), COALESCE(e.parameterName, ''), e.peakValue, f.createdAt
		FROM Exceedance e JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		LEFT JOIN EventDefinitionVersion v ON v.id = e.eventId
		LEFT JOIN EventDefinition d ON d.id = v.definitionId ` + validWhere + `
		AND UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) IN ('HIGH', 'CRITICAL')
		ORDER BY CASE UPPER(TRIM(e.exceedanceLevel)) WHEN 'CRITICAL' THEN 2 ELSE 1 END DESC, f.createdAt DESC LIMIT 50`
	rows, err := h.db.Query(query, validArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.ProgrammePriorityOccurrence, 0)
	for rows.Next() {
		var item models.ProgrammePriorityOccurrence
		var peak sql.NullFloat64
		if err = rows.Scan(&item.OccurrenceID, &item.FlightID, &item.FlightName, &item.AircraftID,
			&item.Registration, &item.EventLabel, &item.Level, &item.Phase, &item.Parameter, &peak, &item.OccurredAt); err != nil {
			return nil, err
		}
		if peak.Valid {
			item.PeakValue = &peak.Float64
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (h *ReportHandler) programmeTargets(companyID *string) ([]models.SafetyIndicatorTarget, error) {
	if companyID == nil {
		return []models.SafetyIndicatorTarget{}, nil
	}
	rows, err := h.db.Query(`SELECT indicatorKey, target, alert FROM SafetyIndicatorTarget WHERE companyId = ? ORDER BY indicatorKey`, *companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.SafetyIndicatorTarget, 0)
	for rows.Next() {
		var item models.SafetyIndicatorTarget
		var target, alert sql.NullFloat64
		if err = rows.Scan(&item.Key, &target, &alert); err != nil {
			return nil, err
		}
		if target.Valid {
			item.Target = &target.Float64
		}
		if alert.Valid {
			item.Alert = &alert.Float64
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
