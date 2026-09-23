package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// tenantCompanyID derives the tenant exclusively from authenticated context.
// Request parameters and bodies are never trusted to choose a tenant.
func tenantCompanyID(c *gin.Context) (string, bool) {
	return contextString(c, "userCompanyId")
}

// hasGlobalCompanyAccess identifies oversight roles whose duties span every
// operator. All other roles remain constrained to their authenticated tenant.
func hasGlobalCompanyAccess(c *gin.Context) bool {
	role, ok := contextString(c, "userRole")
	return ok && (role == models.RoleAdmin || role == models.RoleFDA)
}

func canAccessCompany(c *gin.Context, companyID string) bool {
	if hasGlobalCompanyAccess(c) {
		return true
	}
	userCompanyID, ok := tenantCompanyID(c)
	return ok && userCompanyID == companyID
}

func requireTenantCompany(c *gin.Context) (string, bool) {
	companyID, ok := tenantCompanyID(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Your account is not assigned to a company"})
		return "", false
	}
	return companyID, true
}

func aircraftBelongsToCompany(db *sql.DB, aircraftID, companyID string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM Aircraft WHERE id = ? AND companyId = ?
	)`, aircraftID, companyID).Scan(&exists)
	return exists, err
}

func flightBelongsToCompany(db *sql.DB, flightID, companyID string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM FlightLeg f
		JOIN Aircraft a ON a.id = f.aircraftId
		WHERE f.id = ? AND a.companyId = ?
	)`, flightID, companyID).Scan(&exists)
	return exists, err
}
