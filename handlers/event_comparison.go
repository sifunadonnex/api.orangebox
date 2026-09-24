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

// GetEventComparison compares two record-date cohorts while reusing the same
// role scope, evaluated-flight denominator, and event grouping as overview.
func (h *ReportHandler) GetEventComparison(c *gin.Context) {
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
	topN, err := strconv.Atoi(c.DefaultQuery("topN", "10"))
	if err != nil || topN < 1 || topN > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "topN must be between 1 and 100"})
		return
	}

	baselineFrom := strings.TrimSpace(c.Query("baselineFrom"))
	baselineTo := strings.TrimSpace(c.Query("baselineTo"))
	comparisonFrom := strings.TrimSpace(c.Query("comparisonFrom"))
	comparisonTo := strings.TrimSpace(c.Query("comparisonTo"))
	if !validComparisonPeriod(baselineFrom, baselineTo) || !validComparisonPeriod(comparisonFrom, comparisonTo) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "baselineFrom, baselineTo, comparisonFrom, and comparisonTo must be valid YYYY-MM-DD ranges"})
		return
	}

	baselineFilters := filters
	baselineFilters.From = baselineFrom
	baselineFilters.To = baselineTo
	comparisonFilters := filters
	comparisonFilters.From = comparisonFrom
	comparisonFilters.To = comparisonTo

	baselineSeries, baselineCoverage, err := h.eventReportSeries(scope, baselineFilters, metric)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	comparisonSeries, comparisonCoverage, err := h.eventReportSeries(scope, comparisonFilters, metric)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}

	points := make(map[string]*models.EventReportComparisonPoint)
	for _, item := range baselineSeries {
		points[item.Key] = &models.EventReportComparisonPoint{
			Key:             item.Key,
			Label:           item.Label,
			BaselineValue:   item.Value,
			BaselineFlights: item.EligibleFlights,
		}
	}
	for _, item := range comparisonSeries {
		point := points[item.Key]
		if point == nil {
			point = &models.EventReportComparisonPoint{Key: item.Key, Label: item.Label}
			points[item.Key] = point
		}
		point.ComparisonValue = item.Value
		point.ComparisonFlights = item.EligibleFlights
	}

	series := make([]models.EventReportComparisonPoint, 0, len(points))
	for _, point := range points {
		point.AbsoluteDelta = point.ComparisonValue - point.BaselineValue
		if point.BaselineValue != 0 {
			relative := point.AbsoluteDelta * 100 / point.BaselineValue
			point.RelativeDelta = &relative
		}
		series = append(series, *point)
	}
	sort.SliceStable(series, func(i, j int) bool {
		iMagnitude := maxFloat(series[i].BaselineValue, series[i].ComparisonValue)
		jMagnitude := maxFloat(series[j].BaselineValue, series[j].ComparisonValue)
		if iMagnitude == jMagnitude {
			return series[i].Label < series[j].Label
		}
		return iMagnitude > jMagnitude
	})
	if len(series) > topN {
		series = series[:topN]
	}

	c.JSON(http.StatusOK, models.EventReportComparisonResponse{
		Scope:   h.scopeResponse(scope),
		Filters: filters,
		Metric:  metric,
		TopN:    topN,
		Baseline: models.EventReportComparisonCohort{
			Label: "Baseline", From: baselineFrom, To: baselineTo, Coverage: baselineCoverage,
		},
		Comparison: models.EventReportComparisonCohort{
			Label: "Comparison", From: comparisonFrom, To: comparisonTo, Coverage: comparisonCoverage,
		},
		Series: series,
		AsOf:   time.Now().UTC().Format(time.RFC3339),
	})
}

func validComparisonPeriod(from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	fromDate, fromOK := parseReportDate(from)
	toDate, toOK := parseReportDate(to)
	return fromOK && toOK && !fromDate.After(toDate)
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
