package handlers

import (
	"fdm-backend/models"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UpdateProgrammeTargets is intentionally restricted twice: the route uses
// AdminOrFDA middleware and this handler verifies the role again before making
// a company-scoped upsert. A missing target and alert clears only that exact
// company's indicator.
func (h *ReportHandler) UpdateProgrammeTargets(c *gin.Context) {
	role, ok := contextString(c, "userRole")
	if !ok || (role != models.RoleAdmin && role != models.RoleFDA) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin or FDA access is required"})
		return
	}

	var request models.UpdateSafetyIndicatorTargetsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId and targets are required"})
		return
	}
	request.CompanyID = strings.TrimSpace(request.CompanyID)
	if request.CompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId is required"})
		return
	}
	var companyExists bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM Company WHERE id = ?)`, request.CompanyID).Scan(&companyExists); err != nil {
		respondDatabaseError(c, err)
		return
	}
	if !companyExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	seen := make(map[string]bool)
	for index := range request.Targets {
		item := &request.Targets[index]
		item.Key = strings.TrimSpace(item.Key)
		if item.Key == "" || len(item.Key) > 160 || seen[item.Key] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Indicator keys must be unique and no longer than 160 characters"})
			return
		}
		if (item.Target != nil && *item.Target < 0) || (item.Alert != nil && *item.Alert < 0) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Targets and alert levels cannot be negative"})
			return
		}
		if item.Target != nil && item.Alert != nil && *item.Alert < *item.Target {
			c.JSON(http.StatusBadRequest, gin.H{"error": "An alert level cannot be lower than its target"})
			return
		}
		seen[item.Key] = true
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondDatabaseError(c, err)
		return
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for _, item := range request.Targets {
		if item.Target == nil && item.Alert == nil {
			if _, err = tx.Exec(`DELETE FROM SafetyIndicatorTarget WHERE companyId = ? AND indicatorKey = ?`, request.CompanyID, item.Key); err != nil {
				respondDatabaseError(c, err)
				return
			}
			continue
		}
		if _, err = tx.Exec(`INSERT INTO SafetyIndicatorTarget
			(id, companyId, indicatorKey, target, alert, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(companyId, indicatorKey) DO UPDATE SET target = excluded.target, alert = excluded.alert, updatedAt = excluded.updatedAt`,
			uuid.NewString(), request.CompanyID, item.Key, item.Target, item.Alert, now, now); err != nil {
			respondDatabaseError(c, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		respondDatabaseError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "targets": request.Targets})
}
