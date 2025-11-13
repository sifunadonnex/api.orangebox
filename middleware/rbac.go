package middleware

import (
	"fdm-backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RoleRequired middleware checks if the user has the required role
func RoleRequired(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user role from context (set by AuthenticateToken middleware)
		userRole, exists := c.Get("userRole")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found in context"})
			c.Abort()
			return
		}

		role, ok := userRole.(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user role format"})
			c.Abort()
			return
		}

		// Check if user's role is in the allowed roles
		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error":    "Access denied",
			"message":  "You do not have permission to access this resource",
			"required": allowedRoles,
			"userRole": role,
		})
		c.Abort()
	}
}

// AdminOnly middleware - only admin can access
func AdminOnly() gin.HandlerFunc {
	return RoleRequired(models.RoleAdmin)
}

// AdminOrFDA middleware - admin or FDA can access
func AdminOrFDA() gin.HandlerFunc {
	return RoleRequired(models.RoleAdmin, models.RoleFDA)
}

// GatekeeperOrAbove middleware - gatekeeper, FDA, or admin can access
func GatekeeperOrAbove() gin.HandlerFunc {
	return RoleRequired(models.RoleAdmin, models.RoleFDA, models.RoleGatekeeper)
}

// AnyAuthenticatedUser middleware - any logged-in user can access
func AnyAuthenticatedUser() gin.HandlerFunc {
	return RoleRequired(models.RoleAdmin, models.RoleFDA, models.RoleGatekeeper, models.RoleUser)
}

// Permission helper functions for specific actions

// CanManageUsers checks if user can manage other users
func CanManageUsers(role string) bool {
	return role == models.RoleAdmin
}

// CanValidateEvents checks if user can validate/approve events
func CanValidateEvents(role string) bool {
	return role == models.RoleAdmin || role == models.RoleFDA
}

// CanAddEvents checks if user can add new events
func CanAddEvents(role string) bool {
	return role == models.RoleAdmin || role == models.RoleFDA || role == models.RoleGatekeeper
}

// CanViewReports checks if user can view reports
func CanViewReports(role string) bool {
	return true // All roles can view reports
}

// CanManageAircraft checks if user can manage aircraft
func CanManageAircraft(role string) bool {
	return role == models.RoleAdmin
}

// CanManageCompanies checks if user can manage companies
func CanManageCompanies(role string) bool {
	return role == models.RoleAdmin
}

// CanManageSubscriptions checks if user can manage subscriptions
func CanManageSubscriptions(role string) bool {
	return role == models.RoleAdmin
}

// CompanyAccessControl middleware - ensures user can only access their company's data
func CompanyAccessControl() gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, _ := c.Get("userRole")
		userCompanyID, _ := c.Get("userCompanyId")

		role := userRole.(string)

		// Admin and FDA can access all companies' data
		if role == models.RoleAdmin || role == models.RoleFDA {
			c.Next()
			return
		}

		// For gatekeeper and user, they can only access their own company's data
		requestedCompanyID := c.Param("companyId")
		if requestedCompanyID == "" {
			requestedCompanyID = c.Query("companyId")
		}

		if requestedCompanyID != "" && requestedCompanyID != userCompanyID.(string) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Access denied",
				"message": "You can only access your own company's data",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
