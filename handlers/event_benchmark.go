package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	minimumBenchmarkPeerCompanies   = 5
	minimumBenchmarkEligibleFlights = 30
)

// GetEventBenchmark compares a selected company with a privacy-thresholded
// peer distribution. All companies use Valid occurrences, including oversight
// users, so review-state differences cannot distort the comparison.
func (h *ReportHandler) GetEventBenchmark(c *gin.Context) {
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
	eventDefinitionID := strings.TrimSpace(c.Query("eventDefinitionId"))
	if eventDefinitionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "eventDefinitionId is required"})
		return
	}

	focusCompanyID := strings.TrimSpace(c.Query("focusCompanyId"))
	if scope.CompanyID != nil {
		if focusCompanyID != "" && focusCompanyID != *scope.CompanyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "The focus company is outside your report scope"})
			return
		}
		focusCompanyID = *scope.CompanyID
	}
	if focusCompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Select a focus company before running a benchmark"})
		return
	}

	eventLabel, err := h.eventBenchmarkLabel(eventDefinitionID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event definition not found"})
		return
	}
	if err != nil {
		respondDatabaseError(c, err)
		return
	}

	companies, err := h.eventBenchmarkCompanies(filters, eventDefinitionID, metric)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	focus, found := companies[focusCompanyID]
	if !found {
		var companyName string
		if err := h.db.QueryRow("SELECT name FROM Company WHERE id = ?", focusCompanyID).Scan(&companyName); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Focus company not found"})
				return
			}
			respondDatabaseError(c, err)
			return
		}
		focus = models.EventBenchmarkCompany{CompanyID: focusCompanyID, CompanyName: companyName}
	}

	eligiblePeers := make([]models.EventBenchmarkCompany, 0)
	for companyID, company := range companies {
		if companyID != focusCompanyID && company.EligibleFlights >= minimumBenchmarkEligibleFlights {
			eligiblePeers = append(eligiblePeers, company)
		}
	}
	sort.SliceStable(eligiblePeers, func(i, j int) bool {
		if eligiblePeers[i].Value == eligiblePeers[j].Value {
			return eligiblePeers[i].CompanyName < eligiblePeers[j].CompanyName
		}
		return eligiblePeers[i].Value < eligiblePeers[j].Value
	})

	response := models.EventBenchmarkResponse{
		Scope:                  h.scopeResponse(scope),
		Filters:                filters,
		Metric:                 metric,
		EventDefinitionID:      eventDefinitionID,
		EventLabel:             eventLabel,
		StatusPolicy:           models.ExceedanceStatusValid,
		MinimumPeerCompanies:   minimumBenchmarkPeerCompanies,
		MinimumEligibleFlights: minimumBenchmarkEligibleFlights,
		EligiblePeerCompanies:  len(eligiblePeers),
		Focus:                  focus,
		AsOf:                   time.Now().UTC().Format(time.RFC3339),
	}

	if focus.EligibleFlights < minimumBenchmarkEligibleFlights {
		response.Suppressed = true
		response.SuppressionReason = "The focus company does not have enough evaluated flights for this event."
	} else if len(eligiblePeers) < minimumBenchmarkPeerCompanies {
		response.Suppressed = true
		response.SuppressionReason = "The peer cohort is below the privacy threshold."
	}
	if response.Suppressed {
		if !scope.CanViewAllStatus {
			response.EligiblePeerCompanies = 0
		}
		c.JSON(http.StatusOK, response)
		return
	}

	values := make([]float64, len(eligiblePeers))
	for index, company := range eligiblePeers {
		values[index] = company.Value
	}
	response.Percentiles = &models.EventBenchmarkPercentiles{
		P25: benchmarkPercentile(values, 0.25),
		P50: benchmarkPercentile(values, 0.50),
		P75: benchmarkPercentile(values, 0.75),
		P90: benchmarkPercentile(values, 0.90),
	}
	if scope.CanViewAllStatus {
		response.Peers = eligiblePeers
	}
	c.JSON(http.StatusOK, response)
}

func (h *ReportHandler) eventBenchmarkLabel(eventDefinitionID string) (string, error) {
	var label string
	err := h.db.QueryRow(`SELECT COALESCE(
		(SELECT COALESCE(NULLIF(v.displayName, ''), NULLIF(v.eventName, ''), d.eventCode)
		 FROM EventDefinitionVersion v WHERE v.definitionId = d.id ORDER BY v.version DESC LIMIT 1),
		d.eventCode)
		FROM EventDefinition d WHERE d.id = ?`, eventDefinitionID).Scan(&label)
	return label, err
}

func (h *ReportHandler) eventBenchmarkCompanies(filters models.ReportFilters, eventDefinitionID, metric string) (map[string]models.EventBenchmarkCompany, error) {
	peerScope := reportAccessScope{CanViewAllStatus: false}
	peerFilters := filters
	peerFilters.CompanyID = nil
	peerFilters.AircraftIDs = nil
	flightWhere, flightArgs := reportFlightWhere(peerScope, peerFilters)

	eligibilityQuery := `SELECT co.id, co.name, COUNT(DISTINCT f.id)
		FROM DetectionRunDefinition rd
		JOIN DetectionRun dr ON dr.id = rd.detectionRunId
		JOIN FlightLeg f ON f.id = COALESCE(dr.flightLegId, dr.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN Company co ON co.id = a.companyId ` + flightWhere + `
		AND rd.definitionId = ? AND rd.isCurrent = 1 AND rd.status = 'evaluated'
		AND dr.status IN ('completed', 'completed_with_warnings')
		GROUP BY co.id, co.name`
	rows, err := h.db.Query(eligibilityQuery, append(flightArgs, eventDefinitionID)...)
	if err != nil {
		return nil, err
	}
	companies := make(map[string]models.EventBenchmarkCompany)
	for rows.Next() {
		var company models.EventBenchmarkCompany
		if err := rows.Scan(&company.CompanyID, &company.CompanyName, &company.EligibleFlights); err != nil {
			rows.Close()
			return nil, err
		}
		companies[company.CompanyID] = company
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	exceedanceWhere, exceedanceArgs := reportExceedanceWhere(peerScope, peerFilters)
	occurrenceQuery := `SELECT co.id, COUNT(1), COUNT(DISTINCT f.id)
		FROM Exceedance e
		JOIN EventDefinitionVersion v ON v.id = e.eventId
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN Company co ON co.id = a.companyId ` + exceedanceWhere + `
		AND v.definitionId = ? GROUP BY co.id`
	occurrenceRows, err := h.db.Query(occurrenceQuery, append(exceedanceArgs, eventDefinitionID)...)
	if err != nil {
		return nil, err
	}
	for occurrenceRows.Next() {
		var companyID string
		var occurrences, affected int
		if err := occurrenceRows.Scan(&companyID, &occurrences, &affected); err != nil {
			occurrenceRows.Close()
			return nil, err
		}
		company := companies[companyID]
		company.Occurrences = occurrences
		company.AffectedFlights = affected
		companies[companyID] = company
	}
	if err := occurrenceRows.Close(); err != nil {
		return nil, err
	}

	for companyID, company := range companies {
		switch metric {
		case eventMetricCount:
			company.Value = float64(company.Occurrences)
		case eventMetricAffected:
			if company.EligibleFlights > 0 {
				company.Value = float64(company.AffectedFlights) * 100 / float64(company.EligibleFlights)
			}
		default:
			if company.EligibleFlights > 0 {
				company.Value = float64(company.Occurrences) * 100 / float64(company.EligibleFlights)
			}
		}
		companies[companyID] = company
	}
	return companies, nil
}

func benchmarkPercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	position := float64(len(sortedValues)-1) * percentile
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedValues[lower]
	}
	weight := position - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}
