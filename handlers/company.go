package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CompanyHandler struct {
	db *sql.DB
}

func NewCompanyHandler(db *sql.DB) *CompanyHandler {
	return &CompanyHandler{db: db}
}

// CreateCompany creates a new company
func (h *CompanyHandler) CreateCompany(c *gin.Context) {
	var req models.CreateCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate UUID
	id := uuid.New().String()
	now := time.Now()
	var subscriptionStartedAt, subscriptionEndsAt *time.Time
	if req.SubscriptionID != nil && *req.SubscriptionID != "" {
		var err error
		subscriptionStartedAt, subscriptionEndsAt, err = h.subscriptionWindow(*req.SubscriptionID, now)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Selected subscription plan does not exist"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve subscription period", "details": err.Error()})
			return
		}
	}

	query := `
		INSERT INTO Company (id, name, email, phone, address, country, logo, status, subscriptionId, subscriptionStartedAt, subscriptionEndsAt, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)
	`

	_, err := h.db.Exec(query, id, req.Name, req.Email, req.Phone, req.Address, req.Country, req.Logo, req.SubscriptionID, subscriptionStartedAt, subscriptionEndsAt, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create company", "details": err.Error()})
		return
	}

	// Fetch the created company
	company, err := h.getCompanyByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Company created but failed to fetch", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, company)
}

// GetCompanies retrieves all companies with subscription details and counts
func (h *CompanyHandler) GetCompanies(c *gin.Context) {
	query := `
		SELECT 
			c.id, c.name, c.email, c.phone, c.address, c.country, c.logo, c.status, c.subscriptionId, c.subscriptionStartedAt, c.subscriptionEndsAt, c.createdAt, c.updatedAt,
			s.id, s.planName, s.planType, s.trialDays, s.maxUsers, s.maxAircraft, s.maxFlightsPerMonth, s.maxStorageGB,
			s.price, s.currency, s.startDate, s.endDate, s.isActive, s.autoRenew,
			s.lastPaymentDate, s.nextPaymentDate, s.alertSentAt, s.createdAt, s.updatedAt
		FROM Company c
		LEFT JOIN Subscription s ON c.subscriptionId = s.id
		ORDER BY c.createdAt DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch companies", "details": err.Error()})
		return
	}
	defer rows.Close()

	var companiesWithDetails []gin.H
	for rows.Next() {
		var company models.Company
		var subscription models.Subscription
		var subID, subPlanName, subPlanType, subCurrency sql.NullString
		var subTrialDays, subMaxUsers, subMaxAircraft, subMaxFlightsPerMonth, subMaxStorageGB sql.NullInt64
		var subPrice sql.NullFloat64
		var subStartDate, subEndDate, subLastPaymentDate, subNextPaymentDate, subAlertSentAt, subCreatedAt, subUpdatedAt sql.NullTime
		var subIsActive, subAutoRenew sql.NullBool

		err := rows.Scan(
			&company.ID, &company.Name, &company.Email, &company.Phone, &company.Address,
			&company.Country, &company.Logo, &company.Status, &company.SubscriptionID,
			&company.SubscriptionStartedAt, &company.SubscriptionEndsAt,
			&company.CreatedAt, &company.UpdatedAt,
			&subID, &subPlanName, &subPlanType, &subTrialDays, &subMaxUsers, &subMaxAircraft,
			&subMaxFlightsPerMonth, &subMaxStorageGB, &subPrice, &subCurrency,
			&subStartDate, &subEndDate, &subIsActive, &subAutoRenew,
			&subLastPaymentDate, &subNextPaymentDate, &subAlertSentAt,
			&subCreatedAt, &subUpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan company", "details": err.Error()})
			return
		}

		// Fetch user count
		var userCount int
		h.db.QueryRow("SELECT COUNT(*) FROM User WHERE companyId = ?", company.ID).Scan(&userCount)

		// Fetch aircraft count
		var aircraftCount int
		h.db.QueryRow("SELECT COUNT(*) FROM Aircraft WHERE companyId = ?", company.ID).Scan(&aircraftCount)

		// Build response
		companyData := gin.H{
			"id":                    company.ID,
			"name":                  company.Name,
			"email":                 company.Email,
			"phone":                 company.Phone,
			"address":               company.Address,
			"country":               company.Country,
			"logo":                  company.Logo,
			"status":                company.Status,
			"subscriptionId":        company.SubscriptionID,
			"subscriptionStartedAt": company.SubscriptionStartedAt,
			"subscriptionEndsAt":    company.SubscriptionEndsAt,
			"createdAt":             company.CreatedAt,
			"updatedAt":             company.UpdatedAt,
			"userCount":             userCount,
			"aircraftCount":         aircraftCount,
		}

		// Add subscription if exists
		if subID.Valid {
			subscription.ID = subID.String
			subscription.PlanName = subPlanName.String
			subscription.PlanType = subPlanType.String
			subscription.TrialDays = int(subTrialDays.Int64)
			subscription.MaxUsers = int(subMaxUsers.Int64)
			subscription.MaxAircraft = int(subMaxAircraft.Int64)
			subscription.MaxFlightsPerMonth = int(subMaxFlightsPerMonth.Int64)
			subscription.MaxStorageGB = int(subMaxStorageGB.Int64)
			subscription.Price = subPrice.Float64
			subscription.Currency = subCurrency.String
			subscription.StartDate = subStartDate.Time
			subscription.EndDate = subEndDate.Time
			subscription.IsActive = subIsActive.Bool
			subscription.AutoRenew = subAutoRenew.Bool

			if subLastPaymentDate.Valid {
				subscription.LastPaymentDate = &subLastPaymentDate.Time
			}
			if subNextPaymentDate.Valid {
				subscription.NextPaymentDate = &subNextPaymentDate.Time
			}
			if subAlertSentAt.Valid {
				subscription.AlertSentAt = &subAlertSentAt.Time
			}
			subscription.CreatedAt = subCreatedAt.Time
			subscription.UpdatedAt = subUpdatedAt.Time
			if company.SubscriptionStartedAt != nil {
				subscription.StartDate = *company.SubscriptionStartedAt
			}
			if company.SubscriptionEndsAt != nil {
				subscription.EndDate = *company.SubscriptionEndsAt
			}

			companyData["subscription"] = subscription
		} else {
			companyData["subscription"] = nil
		}

		companiesWithDetails = append(companiesWithDetails, companyData)
	}

	c.JSON(http.StatusOK, companiesWithDetails)
}

// GetCompanyByID retrieves a company by ID with subscription details
func (h *CompanyHandler) GetCompanyByID(c *gin.Context) {
	id := c.Param("id")
	role, _ := contextString(c, "userRole")
	if role != models.RoleAdmin {
		companyID, ok := requireTenantCompany(c)
		if !ok {
			return
		}
		if id != companyID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only access your own company"})
			return
		}
	}

	company, err := h.getCompanyByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch company", "details": err.Error()})
		return
	}

	// Fetch subscription details if exists
	if company.SubscriptionID != nil {
		subscription, err := h.getSubscriptionByID(*company.SubscriptionID)
		if err == nil {
			if company.SubscriptionStartedAt != nil {
				subscription.StartDate = *company.SubscriptionStartedAt
			}
			if company.SubscriptionEndsAt != nil {
				subscription.EndDate = *company.SubscriptionEndsAt
			}
			company.Subscription = subscription
		}
	}

	// Fetch user count
	userCountQuery := `SELECT COUNT(*) FROM User WHERE companyId = ?`
	var userCount int
	h.db.QueryRow(userCountQuery, id).Scan(&userCount)

	// Fetch aircraft count
	aircraftCountQuery := `SELECT COUNT(*) FROM Aircraft WHERE companyId = ?`
	var aircraftCount int
	h.db.QueryRow(aircraftCountQuery, id).Scan(&aircraftCount)

	response := gin.H{
		"company":       company,
		"userCount":     userCount,
		"aircraftCount": aircraftCount,
	}

	c.JSON(http.StatusOK, response)
}

// UpdateCompany updates a company
func (h *CompanyHandler) UpdateCompany(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateCompanyRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build dynamic update query
	query := "UPDATE Company SET updatedAt = ?"
	args := []interface{}{time.Now()}

	if req.Name != nil {
		query += ", name = ?"
		args = append(args, *req.Name)
	}
	if req.Email != nil {
		query += ", email = ?"
		args = append(args, *req.Email)
	}
	if req.Phone != nil {
		query += ", phone = ?"
		args = append(args, *req.Phone)
	}
	if req.Address != nil {
		query += ", address = ?"
		args = append(args, *req.Address)
	}
	if req.Country != nil {
		query += ", country = ?"
		args = append(args, *req.Country)
	}
	if req.Logo != nil {
		query += ", logo = ?"
		args = append(args, *req.Logo)
	}
	if req.Status != nil {
		query += ", status = ?"
		args = append(args, *req.Status)
	}
	if req.SubscriptionID != nil {
		var currentSubscriptionID sql.NullString
		if err := h.db.QueryRow("SELECT subscriptionId FROM Company WHERE id = ?", id).Scan(&currentSubscriptionID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to inspect current subscription", "details": err.Error()})
			return
		}

		query += ", subscriptionId = ?"
		args = append(args, *req.SubscriptionID)
		if !currentSubscriptionID.Valid || currentSubscriptionID.String != *req.SubscriptionID {
			startedAt, endsAt, err := h.subscriptionWindow(*req.SubscriptionID, time.Now())
			if err != nil {
				if err == sql.ErrNoRows {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Selected subscription plan does not exist"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve subscription period", "details": err.Error()})
				return
			}
			query += ", subscriptionStartedAt = ?, subscriptionEndsAt = ?"
			args = append(args, startedAt, endsAt)
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	result, err := h.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update company", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	// Fetch updated company
	company, err := h.getCompanyByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Company updated but failed to fetch", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, company)
}

// DeleteCompany deletes a company
func (h *CompanyHandler) DeleteCompany(c *gin.Context) {
	id := c.Param("id")

	// Check if company has users
	var userCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM User WHERE companyId = ?", id).Scan(&userCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check users", "details": err.Error()})
		return
	}

	if userCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete company with existing users. Please delete or reassign users first."})
		return
	}

	query := "DELETE FROM Company WHERE id = ?"
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete company", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Company deleted successfully"})
}

// SuspendCompany suspends a company account
func (h *CompanyHandler) SuspendCompany(c *gin.Context) {
	id := c.Param("id")

	query := "UPDATE Company SET status = 'suspended', updatedAt = ? WHERE id = ?"
	result, err := h.db.Exec(query, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to suspend company", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Company suspended successfully"})
}

// ActivateCompany activates a company account
func (h *CompanyHandler) ActivateCompany(c *gin.Context) {
	id := c.Param("id")

	query := "UPDATE Company SET status = 'active', updatedAt = ? WHERE id = ?"
	result, err := h.db.Exec(query, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to activate company", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Company activated successfully"})
}

// Helper functions

func (h *CompanyHandler) getCompanyByID(id string) (*models.Company, error) {
	query := `SELECT id, name, email, phone, address, country, logo, status, subscriptionId, subscriptionStartedAt, subscriptionEndsAt, createdAt, updatedAt FROM Company WHERE id = ?`

	var company models.Company
	err := h.db.QueryRow(query, id).Scan(
		&company.ID, &company.Name, &company.Email, &company.Phone, &company.Address,
		&company.Country, &company.Logo, &company.Status, &company.SubscriptionID,
		&company.SubscriptionStartedAt, &company.SubscriptionEndsAt,
		&company.CreatedAt, &company.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &company, nil
}

func (h *CompanyHandler) subscriptionWindow(subscriptionID string, start time.Time) (*time.Time, *time.Time, error) {
	var planType string
	var trialDays int
	if err := h.db.QueryRow("SELECT planType, trialDays FROM Subscription WHERE id = ? AND isActive = 1", subscriptionID).Scan(&planType, &trialDays); err != nil {
		return nil, nil, err
	}

	var end time.Time
	switch planType {
	case "trial":
		if trialDays < 1 {
			trialDays = 30
		}
		end = start.AddDate(0, 0, trialDays)
	case "yearly":
		end = start.AddDate(1, 0, 0)
	case "lifetime":
		end = start.AddDate(100, 0, 0)
	default:
		end = start.AddDate(0, 1, 0)
	}

	return &start, &end, nil
}

func (h *CompanyHandler) getSubscriptionByID(id string) (*models.Subscription, error) {
	query := `
		SELECT id, planName, planType, trialDays, maxUsers, maxAircraft, maxFlightsPerMonth, maxStorageGB,
		       price, currency, startDate, endDate, isActive, autoRenew, lastPaymentDate, 
		       nextPaymentDate, alertSentAt, createdAt, updatedAt 
		FROM Subscription WHERE id = ?
	`

	var sub models.Subscription
	err := h.db.QueryRow(query, id).Scan(
		&sub.ID, &sub.PlanName, &sub.PlanType, &sub.TrialDays, &sub.MaxUsers, &sub.MaxAircraft,
		&sub.MaxFlightsPerMonth, &sub.MaxStorageGB, &sub.Price, &sub.Currency,
		&sub.StartDate, &sub.EndDate, &sub.IsActive, &sub.AutoRenew,
		&sub.LastPaymentDate, &sub.NextPaymentDate, &sub.AlertSentAt,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &sub, nil
}
