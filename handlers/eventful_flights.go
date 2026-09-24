package handlers

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
)

const (
	eventfulOrderMostEvents      = "mostEvents"
	eventfulOrderRecent          = "recent"
	eventfulOrderHighestSeverity = "highestSeverity"
)

func (h *ReportHandler) GetEventfulFlights(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}
	order := strings.TrimSpace(c.DefaultQuery("order", eventfulOrderMostEvents))
	if order != eventfulOrderMostEvents && order != eventfulOrderRecent && order != eventfulOrderHighestSeverity {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order must be mostEvents, recent, or highestSeverity"})
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page must be at least 1"})
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pageSize must be between 1 and 100"})
		return
	}

	summary, flights, err := h.eventfulFlights(scope, filters, order, page, pageSize)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	totalPages := 0
	if summary.TotalEventfulFlights > 0 {
		totalPages = (summary.TotalEventfulFlights + pageSize - 1) / pageSize
	}
	c.JSON(http.StatusOK, models.EventfulFlightsResponse{
		Scope: h.scopeResponse(scope), Filters: filters, Order: order,
		Page: page, PageSize: pageSize, TotalPages: totalPages,
		Summary: summary, Flights: flights, AsOf: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ReportHandler) eventfulFlights(scope reportAccessScope, filters models.ReportFilters, order string, page, pageSize int) (models.EventfulFlightsSummary, []models.EventfulFlight, error) {
	where, args := reportExceedanceWhere(scope, filters)
	grouped := `SELECT f.id AS flightId, COUNT(1) AS occurrences,
		MAX(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) IN ('CRITICAL', 'HIGH') THEN 1 ELSE 0 END) AS highCritical
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + where + ` GROUP BY f.id`
	var summary models.EventfulFlightsSummary
	if err := h.db.QueryRow(`SELECT COUNT(1), COALESCE(SUM(occurrences), 0), COALESCE(SUM(highCritical), 0) FROM (`+grouped+`)`, args...).Scan(
		&summary.TotalEventfulFlights, &summary.TotalOccurrences, &summary.HighCriticalFlights,
	); err != nil {
		return summary, nil, err
	}

	orderBy := "occurrences DESC, highestSeverity DESC, f.createdAt DESC, f.id"
	if order == eventfulOrderRecent {
		orderBy = "f.createdAt DESC, occurrences DESC, f.id"
	} else if order == eventfulOrderHighestSeverity {
		orderBy = "highestSeverity DESC, occurrences DESC, f.createdAt DESC, f.id"
	}
	query := `SELECT f.id, COALESCE(f.name, ''), a.id, COALESCE(a.registration, ''),
		a.companyId, COALESCE(c.name, ''), COALESCE(f.departure, ''), COALESCE(f.destination, ''),
		COALESCE(f.flightHours, ''), COALESCE(f.status, ''), f.createdAt,
		COUNT(1) AS occurrences, COUNT(DISTINCT COALESCE(v.definitionId, e.eventId, e.parameterName)) AS eventTypes,
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'CRITICAL' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'HIGH' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'MEDIUM' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) = 'LOW' THEN 1 ELSE 0 END),
		SUM(CASE WHEN UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) NOT IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW') THEN 1 ELSE 0 END),
		MAX(CASE UPPER(TRIM(COALESCE(e.exceedanceLevel, ''))) WHEN 'CRITICAL' THEN 4 WHEN 'HIGH' THEN 3 WHEN 'MEDIUM' THEN 2 WHEN 'LOW' THEN 1 ELSE 0 END) AS highestSeverity,
		GROUP_CONCAT(DISTINCT COALESCE(d.eventCode, e.parameterName)),
		GROUP_CONCAT(DISTINCT CASE WHEN TRIM(COALESCE(e.flightPhase, '')) = '' THEN 'UNKNOWN' ELSE UPPER(TRIM(e.flightPhase)) END)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN Company c ON c.id = a.companyId
		LEFT JOIN EventDefinitionVersion v ON v.id = e.eventId
		LEFT JOIN EventDefinition d ON d.id = v.definitionId ` + where + `
		GROUP BY f.id, f.name, a.id, a.registration, a.companyId, c.name, f.departure,
			f.destination, f.flightHours, f.status, f.createdAt
		ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		return summary, nil, err
	}
	defer rows.Close()
	flights := make([]models.EventfulFlight, 0)
	for rows.Next() {
		var flight models.EventfulFlight
		var eventCodes, phases sql.NullString
		var highestSeverity int
		if err = rows.Scan(
			&flight.FlightID, &flight.FlightName, &flight.AircraftID, &flight.Registration,
			&flight.CompanyID, &flight.CompanyName, &flight.Departure, &flight.Destination,
			&flight.FlightHours, &flight.FlightStatus, &flight.OccurredAt,
			&flight.Occurrences, &flight.EventTypes,
			&flight.Severity.Critical, &flight.Severity.High, &flight.Severity.Medium,
			&flight.Severity.Low, &flight.Severity.Other, &highestSeverity,
			&eventCodes, &phases,
		); err != nil {
			return summary, nil, err
		}
		flight.EventCodes = splitReportList(eventCodes.String)
		flight.Phases = splitReportList(phases.String)
		flights = append(flights, flight)
	}
	return summary, flights, rows.Err()
}

func splitReportList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	values := normalizedQueryValues(strings.Split(value, ","))
	sort.Strings(values)
	return values
}
