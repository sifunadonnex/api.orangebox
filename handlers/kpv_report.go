package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
)

const maximumKPVOutliers = 100

type kpvSample struct {
	OccurrenceID string
	FlightID     string
	AircraftID   string
	Registration string
	AircraftMake string
	ModelNumber  string
	CompanyID    string
	CompanyName  string
	Phase        string
	Value        float64
	OccurredAt   int64
}

func kpvUnitExpression(alias string) string {
	return `CASE WHEN json_valid(` + alias + `.exceedanceValues)
		THEN COALESCE(json_extract(` + alias + `.exceedanceValues, '$.unit'), '') ELSE '' END`
}

func (h *ReportHandler) GetKPVOptions(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}
	where, args := reportExceedanceWhere(scope, filters)
	unitExpression := kpvUnitExpression("e")
	query := `SELECT v.definitionId,
		COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode),
		e.parameterName, ` + unitExpression + ` AS unit,
		COUNT(1), COUNT(DISTINCT f.id)
		FROM Exceedance e
		JOIN EventDefinitionVersion v ON v.id = e.eventId
		JOIN EventDefinition d ON d.id = v.definitionId
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId ` + where + `
		AND e.peakValue IS NOT NULL AND TRIM(COALESCE(e.parameterName, '')) <> ''
		GROUP BY v.definitionId, v.displayName, v.eventName, d.eventCode, e.parameterName, unit
		ORDER BY 2, e.parameterName, unit`
	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer rows.Close()
	options := make([]models.KPVOption, 0)
	for rows.Next() {
		var option models.KPVOption
		if err = rows.Scan(&option.EventDefinitionID, &option.EventLabel, &option.ParameterName,
			&option.Unit, &option.SampleCount, &option.AffectedFlights); err != nil {
			respondDatabaseError(c, err)
			return
		}
		options = append(options, option)
	}
	if err = rows.Err(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.KPVOptionsResponse{Scope: h.scopeResponse(scope), Filters: filters, Options: options})
}

func (h *ReportHandler) GetKPVDistribution(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}
	eventDefinitionID := strings.TrimSpace(c.Query("eventDefinitionId"))
	parameterName := strings.TrimSpace(c.Query("parameterName"))
	unit := strings.TrimSpace(c.Query("unit"))
	if eventDefinitionID == "" || parameterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "eventDefinitionId and parameterName are required"})
		return
	}
	splitBy := strings.TrimSpace(c.DefaultQuery("splitBy", "none"))
	if splitBy != "none" && splitBy != "aircraft" && splitBy != "fleet" && splitBy != "company" && splitBy != "phase" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "splitBy must be none, aircraft, fleet, company, or phase"})
		return
	}
	binCount, err := strconv.Atoi(c.DefaultQuery("binCount", "20"))
	if err != nil || binCount < 5 || binCount > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "binCount must be between 5 and 100"})
		return
	}

	eventLabel, samples, err := h.kpvSamples(scope, filters, eventDefinitionID, parameterName, unit)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "The selected KPV series is unavailable in this report scope"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	statistics := kpvStatistics(samples)
	histogram := kpvHistogram(samples, binCount, statistics.Minimum, statistics.Maximum)
	groups := kpvGroups(samples, splitBy)
	outliers := kpvOutliers(samples, statistics.LowerFence, statistics.UpperFence, scope.CanSelectCompany)
	truncated := len(outliers) > maximumKPVOutliers
	if truncated {
		outliers = outliers[:maximumKPVOutliers]
	}
	c.JSON(http.StatusOK, models.KPVDistributionResponse{
		Scope: h.scopeResponse(scope), Filters: filters,
		EventDefinitionID: eventDefinitionID, EventLabel: eventLabel,
		ParameterName: parameterName, Unit: unit, SplitBy: splitBy, BinCount: binCount,
		Statistics: statistics, Histogram: histogram, Groups: groups,
		Outliers: outliers, OutliersTruncated: truncated,
		AsOf: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ReportHandler) kpvSamples(scope reportAccessScope, filters models.ReportFilters, eventDefinitionID, parameterName, unit string) (string, []kpvSample, error) {
	where, args := reportExceedanceWhere(scope, filters)
	unitExpression := kpvUnitExpression("e")
	query := `SELECT e.id, f.id, a.id, COALESCE(a.registration, ''), a.aircraftMake,
		COALESCE(a.modelNumber, ''), a.companyId, COALESCE(c.name, ''),
		CASE WHEN TRIM(COALESCE(e.flightPhase, '')) = '' THEN 'UNKNOWN' ELSE UPPER(TRIM(e.flightPhase)) END,
		e.peakValue, f.createdAt,
		COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode)
		FROM Exceedance e
		JOIN EventDefinitionVersion v ON v.id = e.eventId
		JOIN EventDefinition d ON d.id = v.definitionId
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN Company c ON c.id = a.companyId ` + where + `
		AND v.definitionId = ? AND e.parameterName = ? AND ` + unitExpression + ` = ?
		AND e.peakValue IS NOT NULL ORDER BY e.peakValue, e.id`
	queryArgs := append(append([]any{}, args...), eventDefinitionID, parameterName, unit)
	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	samples := make([]kpvSample, 0)
	eventLabel := ""
	for rows.Next() {
		var sample kpvSample
		if err = rows.Scan(&sample.OccurrenceID, &sample.FlightID, &sample.AircraftID,
			&sample.Registration, &sample.AircraftMake, &sample.ModelNumber,
			&sample.CompanyID, &sample.CompanyName, &sample.Phase, &sample.Value,
			&sample.OccurredAt, &eventLabel); err != nil {
			return "", nil, err
		}
		samples = append(samples, sample)
	}
	if err = rows.Err(); err != nil {
		return "", nil, err
	}
	if len(samples) == 0 {
		return "", nil, sql.ErrNoRows
	}
	return eventLabel, samples, nil
}

func kpvStatistics(samples []kpvSample) models.KPVStatistics {
	values := make([]float64, len(samples))
	mean := 0.0
	for index, sample := range samples {
		values[index] = sample.Value
		mean += sample.Value
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	p25 := reportQuantile(values, 0.25)
	p75 := reportQuantile(values, 0.75)
	statistics := models.KPVStatistics{
		Count: len(values), Minimum: values[0], Maximum: values[len(values)-1], Mean: mean,
		Median: reportQuantile(values, 0.5), StandardDev: math.Sqrt(variance / float64(len(values))),
		P25: p25, P75: p75, IQR: p75 - p25,
	}
	statistics.LowerFence = p25 - 1.5*statistics.IQR
	statistics.UpperFence = p75 + 1.5*statistics.IQR
	for _, value := range values {
		if value < statistics.LowerFence || value > statistics.UpperFence {
			statistics.OutlierCount++
		}
	}
	return statistics
}

func reportQuantile(sortedValues []float64, quantile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	position := quantile * float64(len(sortedValues)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedValues[lower]
	}
	ratio := position - float64(lower)
	return sortedValues[lower] + (sortedValues[upper]-sortedValues[lower])*ratio
}

func kpvHistogram(samples []kpvSample, requestedBins int, minimum, maximum float64) []models.KPVHistogramBin {
	if len(samples) == 0 {
		return []models.KPVHistogramBin{}
	}
	if minimum == maximum {
		return []models.KPVHistogramBin{{LowerBound: minimum, UpperBound: maximum, Count: len(samples), Percentage: 100}}
	}
	binCount := requestedBins
	if binCount > len(samples) {
		binCount = len(samples)
	}
	width := (maximum - minimum) / float64(binCount)
	bins := make([]models.KPVHistogramBin, binCount)
	for index := range bins {
		bins[index].LowerBound = minimum + float64(index)*width
		bins[index].UpperBound = minimum + float64(index+1)*width
	}
	for _, sample := range samples {
		index := int((sample.Value - minimum) / width)
		if index >= binCount {
			index = binCount - 1
		}
		bins[index].Count++
	}
	for index := range bins {
		bins[index].Percentage = float64(bins[index].Count) * 100 / float64(len(samples))
	}
	return bins
}

func kpvGroups(samples []kpvSample, splitBy string) []models.KPVGroup {
	if splitBy == "none" {
		return []models.KPVGroup{}
	}
	type groupValues struct {
		label  string
		values []float64
	}
	grouped := make(map[string]*groupValues)
	for _, sample := range samples {
		key, label := kpvGroupIdentity(sample, splitBy)
		group := grouped[key]
		if group == nil {
			group = &groupValues{label: label, values: make([]float64, 0)}
			grouped[key] = group
		}
		group.values = append(group.values, sample.Value)
	}
	groups := make([]models.KPVGroup, 0, len(grouped))
	for key, groupedValues := range grouped {
		sort.Float64s(groupedValues.values)
		mean := 0.0
		for _, value := range groupedValues.values {
			mean += value
		}
		mean /= float64(len(groupedValues.values))
		groups = append(groups, models.KPVGroup{
			Key: key, Label: groupedValues.label, SampleCount: len(groupedValues.values),
			Minimum: groupedValues.values[0], Maximum: groupedValues.values[len(groupedValues.values)-1],
			Mean: mean, Median: reportQuantile(groupedValues.values, 0.5),
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].SampleCount == groups[j].SampleCount {
			return groups[i].Label < groups[j].Label
		}
		return groups[i].SampleCount > groups[j].SampleCount
	})
	return groups
}

func kpvGroupIdentity(sample kpvSample, splitBy string) (string, string) {
	switch splitBy {
	case "aircraft":
		label := sample.Registration
		if label == "" {
			label = sample.AircraftID
		}
		return sample.AircraftID, label
	case "fleet":
		label := strings.TrimSpace(sample.AircraftMake + " " + sample.ModelNumber)
		return strings.ToLower(label), label
	case "company":
		return sample.CompanyID, sample.CompanyName
	case "phase":
		return sample.Phase, strings.ReplaceAll(sample.Phase, "_", " ")
	default:
		return "all", "All samples"
	}
}

func kpvOutliers(samples []kpvSample, lowerFence, upperFence float64, includeCompany bool) []models.KPVOutlier {
	outliers := make([]models.KPVOutlier, 0)
	for _, sample := range samples {
		if sample.Value >= lowerFence && sample.Value <= upperFence {
			continue
		}
		item := models.KPVOutlier{
			OccurrenceID: sample.OccurrenceID, FlightID: sample.FlightID,
			Registration: sample.Registration, Value: sample.Value, OccurredAt: sample.OccurredAt,
		}
		if includeCompany {
			item.CompanyName = sample.CompanyName
		}
		outliers = append(outliers, item)
	}
	sort.SliceStable(outliers, func(i, j int) bool {
		leftDistance := math.Max(lowerFence-outliers[i].Value, outliers[i].Value-upperFence)
		rightDistance := math.Max(lowerFence-outliers[j].Value, outliers[j].Value-upperFence)
		if leftDistance == rightDistance {
			return outliers[i].OccurredAt > outliers[j].OccurredAt
		}
		return leftDistance > rightDistance
	})
	return outliers
}
