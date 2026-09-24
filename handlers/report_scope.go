package handlers

import (
	"fdm-backend/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type reportAccessScope struct {
	Role             string
	CompanyID        *string
	CanSelectCompany bool
	CanViewAllStatus bool
}

// resolveReportAccess is deliberately report-specific. Generic tenant helpers
// remain fail-closed for operational writes and other company-owned resources.
func resolveReportAccess(c *gin.Context) (reportAccessScope, bool) {
	role, ok := contextString(c, "userRole")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found in context"})
		return reportAccessScope{}, false
	}

	requestedCompanyID := strings.TrimSpace(c.Query("companyId"))
	if role == models.RoleAdmin || role == models.RoleFDA {
		scope := reportAccessScope{Role: role, CanSelectCompany: true, CanViewAllStatus: true}
		if requestedCompanyID != "" {
			scope.CompanyID = stringPointer(requestedCompanyID)
		}
		return scope, true
	}

	companyID, ok := requireTenantCompany(c)
	if !ok {
		return reportAccessScope{}, false
	}
	if requestedCompanyID != "" && requestedCompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only access reports for your company"})
		return reportAccessScope{}, false
	}

	return reportAccessScope{
		Role:             role,
		CompanyID:        stringPointer(companyID),
		CanSelectCompany: false,
		CanViewAllStatus: false,
	}, true
}
