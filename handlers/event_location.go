package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"fdm-backend/models"

	"github.com/gin-gonic/gin"
)

// GetEventLocation returns aggregated map cells. It never returns raw
// occurrence coordinates and always reports unmatched occurrences so missing
// position data cannot make the selected cohort look cleaner than it is.
func (h *ReportHandler) GetEventLocation(c *gin.Context) {
	scope, ok := resolveReportAccess(c)
	if !ok {
		return
	}
	filters, ok := parseReportFilters(c, scope)
	if !ok || !h.validateSelectedCompany(c, scope) {
		return
	}
	precision, err := strconv.Atoi(c.DefaultQuery("precision", "2"))
	if err != nil || precision < 0 || precision > 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "precision must be between 0 and 3"})
		return
	}

	coverage, cells, bounds, err := h.eventLocationCells(scope, filters, precision)
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.EventLocationResponse{
		Scope: h.scopeResponse(scope), Filters: filters, Precision: precision,
		Coverage: coverage, Bounds: bounds, Cells: cells,
		AsOf: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ReportHandler) eventLocationCells(scope reportAccessScope, filters models.ReportFilters, precision int) (models.EventLocationCoverage, []models.EventLocationCell, *models.EventLocationBounds, error) {
	where, args := reportExceedanceWhere(scope, filters)
	coverageQuery := `SELECT COUNT(1), COUNT(l.exceedanceId), COUNT(1) - COUNT(l.exceedanceId), COUNT(DISTINCT f.id)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		LEFT JOIN ExceedanceLocation l ON l.exceedanceId = e.id ` + where
	var coverage models.EventLocationCoverage
	if err := h.db.QueryRow(coverageQuery, args...).Scan(
		&coverage.TotalOccurrences, &coverage.LocatedOccurrences,
		&coverage.UnlocatedOccurrences, &coverage.AffectedFlights,
	); err != nil {
		return coverage, nil, nil, err
	}
	reportCoverage, err := h.eventReportCoverage(scope, filters)
	if err != nil {
		return coverage, nil, nil, err
	}
	coverage.EligibleFlights = reportCoverage.EligibleFlights

	cellQuery := `SELECT ROUND(l.latitude, ?), ROUND(l.longitude, ?), COUNT(1), COUNT(DISTINCT f.id),
		SUM(CASE WHEN l.quality = 'exact' THEN 1 ELSE 0 END),
		SUM(CASE WHEN l.quality = 'near' THEN 1 ELSE 0 END)
		FROM Exceedance e
		JOIN FlightLeg f ON f.id = COALESCE(e.flightLegId, e.flightId)
		JOIN Aircraft a ON a.id = f.aircraftId
		JOIN ExceedanceLocation l ON l.exceedanceId = e.id ` + where + `
		GROUP BY ROUND(l.latitude, ?), ROUND(l.longitude, ?)`
	// The GROUP BY placeholders occur after the report filters in SQL order.
	groupArgs := []any{precision, precision}
	groupArgs = append(groupArgs, args...)
	groupArgs = append(groupArgs, precision, precision)
	rows, err := h.db.Query(cellQuery, groupArgs...)
	if err != nil {
		return coverage, nil, nil, err
	}
	defer rows.Close()
	cells := make([]models.EventLocationCell, 0)
	var bounds *models.EventLocationBounds
	for rows.Next() {
		var cell models.EventLocationCell
		if err = rows.Scan(&cell.Latitude, &cell.Longitude, &cell.Occurrences,
			&cell.AffectedFlights, &cell.ExactMatches, &cell.NearMatches); err != nil {
			return coverage, nil, nil, err
		}
		cell.Key = fmt.Sprintf("%.*f,%.*f", precision, cell.Latitude, precision, cell.Longitude)
		if coverage.EligibleFlights > 0 {
			cell.RatePer100Flights = float64(cell.Occurrences) * 100 / float64(coverage.EligibleFlights)
		}
		if bounds == nil {
			bounds = &models.EventLocationBounds{West: cell.Longitude, South: cell.Latitude, East: cell.Longitude, North: cell.Latitude}
		} else {
			if cell.Longitude < bounds.West {
				bounds.West = cell.Longitude
			}
			if cell.Longitude > bounds.East {
				bounds.East = cell.Longitude
			}
			if cell.Latitude < bounds.South {
				bounds.South = cell.Latitude
			}
			if cell.Latitude > bounds.North {
				bounds.North = cell.Latitude
			}
		}
		cells = append(cells, cell)
	}
	if err = rows.Err(); err != nil {
		return coverage, nil, nil, err
	}
	sort.SliceStable(cells, func(i, j int) bool {
		if cells[i].Occurrences == cells[j].Occurrences {
			return cells[i].Key < cells[j].Key
		}
		return cells[i].Occurrences > cells[j].Occurrences
	})
	return coverage, cells, bounds, nil
}
