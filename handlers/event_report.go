package handlers

import (
	"fdm-backend/models"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	eventMetricCount    = "count"
	eventMetricRate     = "ratePer100Flights"
	eventMetricAffected = "affectedFlightPercentage"
)

type eventReportAccumulator struct {
	point models.EventReportPoint
}

// GetEventAggregate provides the first events-report vertical slice. It keeps
// the denominator on evaluated flight/event pairs instead of all uploaded
// flights, and applies report authorization before any aggregation.
func (h *ReportHandler) GetEventAggregate(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}

	metric := strings.TrimSpace(c.DefaultQuery("metric", eventMetricRate))
	if metric != eventMetricCount && metric != eventMetricRate && metric != eventMetricAffected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metric must be count, ratePer100Flights, or affectedFlightPercentage"})
		return
	}
	order := strings.ToLower(strings.TrimSpace(c.DefaultQuery("order", "top")))
	if order != "top" && order != "bottom" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order must be top or bottom"})
		return
	}
	topN, err := strconv.Atoi(c.DefaultQuery("topN", "10"))
	if err != nil || topN < 1 || topN > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "topN must be between 1 and 100"})
		return
	}

	series, coverage, err := h.eventReportSeries(scope, filters, metric)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	severities, err := h.eventReportBreakdown(scope, filters, "severity")
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	phases, err := h.eventReportBreakdown(scope, filters, "phase")
	if err != nil {
		respondDatabaseError(c, err)
		return
	}

	sort.SliceStable(series, func(i, j int) bool {
		if series[i].Value == series[j].Value {
			if series[i].Occurrences == series[j].Occurrences {
				return series[i].Label < series[j].Label
			}
			return series[i].Occurrences > series[j].Occurrences
		}
		if order == "bottom" {
			return series[i].Value < series[j].Value
		}
		return series[i].Value > series[j].Value
	})
	if len(series) > topN {
		series = series[:topN]
	}

	c.JSON(http.StatusOK, models.EventReportAggregateResponse{
		Scope:      h.scopeResponse(scope),
		Filters:    filters,
		Metric:     metric,
		Order:      order,
		TopN:       topN,
		Coverage:   coverage,
		Series:     series,
		Severities: severities,
		Phases:     phases,
		AsOf:       time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ReportHandler) eventReportSeries(scope reportAccessScope, filters models.ReportFilters, metric string) ([]models.EventReportPoint, models.EventReportCoverage, error) {
	flightWhere, flightArgs := reportFlightWhere(scope, filters)
	eligibilityQuery := `SELECT rd.definitionId,
		COALESCE((SELECT COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode)
			FROM EventDefinitionVersion v WHERE v.definitionId = d.id ORDER BY v.version DESC LIMIT 1), d.eventCode),
		COUNT(DISTINCT f.id)
		FROM DetectionRunDefinition rd
		JOIN DetectionRun dr ON dr.id = rd.detectionRunId
		JOIN FlightLeg f ON f.id = COALESCE(dr.flightLegId, dr.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN EventDefinition d ON d.id = rd.definitionId ` + flightWhere + `
		AND rd.isCurrent = 1 AND rd.status = 'evaluated'
		AND dr.status IN ('completed', 'completed_with_warnings')
		GROUP BY rd.definitionId, d.eventCode`

	rows, err := h.db.Query(eligibilityQuery, flightArgs...)
	if err != nil {
		return nil, models.EventReportCoverage{}, err
	}
	points := make(map[string]*eventReportAccumulator)
	for rows.Next() {
		var key, label string
		var eligible int
		if err = rows.Scan(&key, &label, &eligible); err != nil {
			rows.Close()
			return nil, models.EventReportCoverage{}, err
		}
		points[key] = &eventReportAccumulator{point: models.EventReportPoint{Key: key, Label: label, EligibleFlights: eligible}}
	}
	if err = rows.Close(); err != nil {
		return nil, models.EventReportCoverage{}, err
	}

	exceedanceWhere, exceedanceArgs := reportExceedanceWhere(scope, filters)
	occurrenceQuery := `SELECT v.definitionId, COUNT(1), COUNT(DISTINCT f.id)
		FROM Exceedance e
		JOIN EventDefinitionVersion v ON v.id = e.eventId
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + exceedanceWhere + `
		GROUP BY v.definitionId`
	occurrenceRows, err := h.db.Query(occurrenceQuery, exceedanceArgs...)
	if err != nil {
		return nil, models.EventReportCoverage{}, err
	}
	for occurrenceRows.Next() {
		var key string
		var occurrences, affected int
		if err = occurrenceRows.Scan(&key, &occurrences, &affected); err != nil {
			occurrenceRows.Close()
			return nil, models.EventReportCoverage{}, err
		}
		if accumulator := points[key]; accumulator != nil {
			accumulator.point.Occurrences = occurrences
			accumulator.point.AffectedFlights = affected
		}
	}
	if err = occurrenceRows.Close(); err != nil {
		return nil, models.EventReportCoverage{}, err
	}

	series := make([]models.EventReportPoint, 0, len(points))
	for _, accumulator := range points {
		point := accumulator.point
		switch metric {
		case eventMetricCount:
			point.Value = float64(point.Occurrences)
		case eventMetricAffected:
			if point.EligibleFlights > 0 {
				point.Value = float64(point.AffectedFlights) * 100 / float64(point.EligibleFlights)
			}
		default:
			if point.EligibleFlights > 0 {
				point.Value = float64(point.Occurrences) * 100 / float64(point.EligibleFlights)
			}
		}
		series = append(series, point)
	}

	coverage, err := h.eventReportCoverage(scope, filters)
	return series, coverage, err
}

func (h *ReportHandler) eventReportCoverage(scope reportAccessScope, filters models.ReportFilters) (models.EventReportCoverage, error) {
	flightWhere, flightArgs := reportFlightWhere(scope, filters)
	query := `SELECT
		COUNT(DISTINCT CASE WHEN rd.isCurrent = 1 AND rd.status = 'evaluated' AND dr.status IN ('completed', 'completed_with_warnings') THEN f.id END),
		COUNT(CASE WHEN rd.isCurrent = 1 AND rd.status = 'skipped' THEN 1 END),
		COUNT(DISTINCT CASE WHEN dr.status = 'failed' THEN f.id END)
		FROM FlightLeg f
		JOIN Aircraft a ON a.id = f.aircraftId
		LEFT JOIN DetectionRun dr ON COALESCE(dr.flightLegId, dr.flightId) = f.id
		LEFT JOIN DetectionRunDefinition rd ON rd.detectionRunId = dr.id ` + flightWhere
	var coverage models.EventReportCoverage
	if err := h.db.QueryRow(query, flightArgs...).Scan(&coverage.EligibleFlights, &coverage.SkippedEvaluations, &coverage.FailedFlights); err != nil {
		return coverage, err
	}

	exceedanceWhere, exceedanceArgs := reportExceedanceWhere(scope, filters)
	occurrenceQuery := `SELECT COUNT(1), COUNT(DISTINCT f.id)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + exceedanceWhere
	if err := h.db.QueryRow(occurrenceQuery, exceedanceArgs...).Scan(&coverage.Occurrences, &coverage.AffectedFlights); err != nil {
		return coverage, err
	}
	return coverage, nil
}

func (h *ReportHandler) eventReportBreakdown(scope reportAccessScope, filters models.ReportFilters, dimension string) ([]models.EventReportBreakdown, error) {
	keyExpression := `CASE WHEN TRIM(COALESCE(e.exceedanceLevel, '')) = '' THEN 'UNKNOWN' ELSE UPPER(TRIM(e.exceedanceLevel)) END`
	if dimension == "phase" {
		keyExpression = `CASE
			WHEN TRIM(COALESCE(e.flightPhase, '')) = '' THEN 'UNKNOWN'
			WHEN INSTR(e.flightPhase, ',') > 0 THEN 'MULTI / UNKNOWN'
			ELSE UPPER(TRIM(e.flightPhase)) END`
	}
	where, args := reportExceedanceWhere(scope, filters)
	query := `SELECT ` + keyExpression + ` AS bucket, COUNT(1)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + where + `
		GROUP BY bucket ORDER BY COUNT(1) DESC, bucket`
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.EventReportBreakdown, 0)
	for rows.Next() {
		var item models.EventReportBreakdown
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		item.Label = strings.ReplaceAll(item.Key, "_", " ")
		result = append(result, item)
	}
	return result, rows.Err()
}
