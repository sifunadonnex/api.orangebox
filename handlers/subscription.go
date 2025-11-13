package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type SubscriptionHandler struct {
	db *sql.DB
}

func NewSubscriptionHandler(db *sql.DB) *SubscriptionHandler {
	return &SubscriptionHandler{db: db}
}

// CreateSubscription creates a new subscription
func (h *SubscriptionHandler) CreateSubscription(c *gin.Context) {
	var req models.CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate UUID
	id := uuid.New().String()
	now := time.Now()

	// Set defaults if not provided
	if req.MaxUsers == 0 {
		req.MaxUsers = 5
	}
	if req.MaxAircraft == 0 {
		req.MaxAircraft = 2
	}
	if req.MaxFlightsPerMonth == 0 {
		req.MaxFlightsPerMonth = 100
	}
	if req.MaxStorageGB == 0 {
		req.MaxStorageGB = 10
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	query := `
		INSERT INTO Subscription (
			id, planName, planType, maxUsers, maxAircraft, maxFlightsPerMonth, maxStorageGB,
			price, currency, startDate, endDate, isActive, autoRenew, createdAt, updatedAt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`

	_, err := h.db.Exec(query, id, req.PlanName, req.PlanType, req.MaxUsers, req.MaxAircraft,
		req.MaxFlightsPerMonth, req.MaxStorageGB, req.Price, req.Currency, req.StartDate,
		req.EndDate, req.AutoRenew, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subscription", "details": err.Error()})
		return
	}

	// Fetch the created subscription
	subscription, err := h.getSubscriptionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Subscription created but failed to fetch", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, subscription)
}

// GetSubscriptions retrieves all subscriptions
func (h *SubscriptionHandler) GetSubscriptions(c *gin.Context) {
	query := `
		SELECT id, planName, planType, maxUsers, maxAircraft, maxFlightsPerMonth, maxStorageGB,
		       price, currency, startDate, endDate, isActive, autoRenew, lastPaymentDate,
		       nextPaymentDate, alertSentAt, createdAt, updatedAt
		FROM Subscription ORDER BY createdAt DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscriptions", "details": err.Error()})
		return
	}
	defer rows.Close()

	subscriptions := []models.Subscription{}
	for rows.Next() {
		var sub models.Subscription
		err := rows.Scan(
			&sub.ID, &sub.PlanName, &sub.PlanType, &sub.MaxUsers, &sub.MaxAircraft,
			&sub.MaxFlightsPerMonth, &sub.MaxStorageGB, &sub.Price, &sub.Currency,
			&sub.StartDate, &sub.EndDate, &sub.IsActive, &sub.AutoRenew,
			&sub.LastPaymentDate, &sub.NextPaymentDate, &sub.AlertSentAt,
			&sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan subscription", "details": err.Error()})
			return
		}
		subscriptions = append(subscriptions, sub)
	}

	c.JSON(http.StatusOK, subscriptions)
}

// GetSubscriptionByID retrieves a subscription by ID
func (h *SubscriptionHandler) GetSubscriptionByID(c *gin.Context) {
	id := c.Param("id")

	subscription, err := h.getSubscriptionByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscription", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, subscription)
}

// GetSubscriptionStatus retrieves subscription status with usage
func (h *SubscriptionHandler) GetSubscriptionStatus(c *gin.Context) {
	companyID := c.Param("companyId")

	// Get company
	var subscriptionID *string
	err := h.db.QueryRow("SELECT subscriptionId FROM Company WHERE id = ?", companyID).Scan(&subscriptionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	if subscriptionID == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No subscription found for this company"})
		return
	}

	// Get subscription
	subscription, err := h.getSubscriptionByID(*subscriptionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscription", "details": err.Error()})
		return
	}

	// Calculate days remaining
	daysRemaining := int(time.Until(subscription.EndDate).Hours() / 24)

	// Determine status
	status := "active"
	if daysRemaining <= 0 {
		status = "expired"
	} else if daysRemaining <= 30 {
		status = "expiring_soon"
	}

	// Get usage statistics
	var usersUsed, aircraftUsed, flightsUsed int
	var storageUsedGB float64

	h.db.QueryRow("SELECT COUNT(*) FROM User WHERE companyId = ?", companyID).Scan(&usersUsed)
	h.db.QueryRow("SELECT COUNT(*) FROM Aircraft WHERE companyId = ?", companyID).Scan(&aircraftUsed)
	h.db.QueryRow("SELECT COUNT(*) FROM Csv WHERE aircraftId IN (SELECT id FROM Aircraft WHERE companyId = ?)", companyID).Scan(&flightsUsed)

	// Calculate storage (simplified - you may want to sum actual file sizes)
	storageUsedGB = float64(flightsUsed) * 0.5 // Assume 0.5GB per flight average

	subscriptionStatus := models.SubscriptionStatus{
		IsActive:       subscription.IsActive && status != "expired",
		DaysRemaining:  daysRemaining,
		Status:         status,
		EndDate:        subscription.EndDate,
		PlanName:       subscription.PlanName,
		UsersUsed:      usersUsed,
		UsersLimit:     subscription.MaxUsers,
		AircraftUsed:   aircraftUsed,
		AircraftLimit:  subscription.MaxAircraft,
		FlightsUsed:    flightsUsed,
		FlightsLimit:   subscription.MaxFlightsPerMonth,
		StorageUsedGB:  storageUsedGB,
		StorageLimitGB: subscription.MaxStorageGB,
	}

	c.JSON(http.StatusOK, subscriptionStatus)
}

// UpdateSubscription updates a subscription
func (h *SubscriptionHandler) UpdateSubscription(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateSubscriptionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build dynamic update query
	query := "UPDATE Subscription SET updatedAt = ?"
	args := []interface{}{time.Now()}

	if req.PlanName != nil {
		query += ", planName = ?"
		args = append(args, *req.PlanName)
	}
	if req.PlanType != nil {
		query += ", planType = ?"
		args = append(args, *req.PlanType)
	}
	if req.MaxUsers != nil {
		query += ", maxUsers = ?"
		args = append(args, *req.MaxUsers)
	}
	if req.MaxAircraft != nil {
		query += ", maxAircraft = ?"
		args = append(args, *req.MaxAircraft)
	}
	if req.MaxFlightsPerMonth != nil {
		query += ", maxFlightsPerMonth = ?"
		args = append(args, *req.MaxFlightsPerMonth)
	}
	if req.MaxStorageGB != nil {
		query += ", maxStorageGB = ?"
		args = append(args, *req.MaxStorageGB)
	}
	if req.Price != nil {
		query += ", price = ?"
		args = append(args, *req.Price)
	}
	if req.Currency != nil {
		query += ", currency = ?"
		args = append(args, *req.Currency)
	}
	if req.StartDate != nil {
		query += ", startDate = ?"
		args = append(args, *req.StartDate)
	}
	if req.EndDate != nil {
		query += ", endDate = ?"
		args = append(args, *req.EndDate)
	}
	if req.IsActive != nil {
		query += ", isActive = ?"
		args = append(args, *req.IsActive)
	}
	if req.AutoRenew != nil {
		query += ", autoRenew = ?"
		args = append(args, *req.AutoRenew)
	}
	if req.LastPaymentDate != nil {
		query += ", lastPaymentDate = ?"
		args = append(args, *req.LastPaymentDate)
	}
	if req.NextPaymentDate != nil {
		query += ", nextPaymentDate = ?"
		args = append(args, *req.NextPaymentDate)
	}

	query += " WHERE id = ?"
	args = append(args, id)

	result, err := h.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subscription", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
		return
	}

	// Fetch updated subscription
	subscription, err := h.getSubscriptionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Subscription updated but failed to fetch", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, subscription)
}

// DeleteSubscription deletes a subscription
func (h *SubscriptionHandler) DeleteSubscription(c *gin.Context) {
	id := c.Param("id")

	// Check if subscription is used by any company
	var companyCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM Company WHERE subscriptionId = ?", id).Scan(&companyCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check companies", "details": err.Error()})
		return
	}

	if companyCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete subscription that is assigned to companies"})
		return
	}

	query := "DELETE FROM Subscription WHERE id = ?"
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subscription", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription deleted successfully"})
}

// CheckExpiredSubscriptions checks and updates expired subscriptions
func (h *SubscriptionHandler) CheckExpiredSubscriptions(c *gin.Context) {
	now := time.Now()

	// Get all companies with active subscriptions
	query := `
		SELECT c.id, c.name, c.email, s.endDate, s.id as subscriptionId
		FROM Company c
		JOIN Subscription s ON c.subscriptionId = s.id
		WHERE c.status = 'active' AND s.isActive = 1
	`

	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check subscriptions", "details": err.Error()})
		return
	}
	defer rows.Close()

	expiredCompanies := []string{}
	expiringCompanies := []string{}

	for rows.Next() {
		var companyID, companyName, companyEmail, subscriptionID string
		var endDate time.Time

		err := rows.Scan(&companyID, &companyName, &companyEmail, &endDate, &subscriptionID)
		if err != nil {
			continue
		}

		daysRemaining := int(time.Until(endDate).Hours() / 24)

		if daysRemaining <= 0 {
			// Suspend expired companies
			h.db.Exec("UPDATE Company SET status = 'expired', updatedAt = ? WHERE id = ?", now, companyID)
			expiredCompanies = append(expiredCompanies, companyName)
		} else if daysRemaining <= 7 {
			// Alert for expiring soon (7 days or less)
			expiringCompanies = append(expiringCompanies, companyName)

			// Update alert timestamp if not already sent in last 24 hours
			var alertSentAt *time.Time
			h.db.QueryRow("SELECT alertSentAt FROM Subscription WHERE id = ?", subscriptionID).Scan(&alertSentAt)

			if alertSentAt == nil || time.Since(*alertSentAt).Hours() > 24 {
				h.db.Exec("UPDATE Subscription SET alertSentAt = ? WHERE id = ?", now, subscriptionID)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Subscription check completed",
		"expiredCompanies":  expiredCompanies,
		"expiringCompanies": expiringCompanies,
	})
}

// Helper function
func (h *SubscriptionHandler) getSubscriptionByID(id string) (*models.Subscription, error) {
	query := `
		SELECT id, planName, planType, maxUsers, maxAircraft, maxFlightsPerMonth, maxStorageGB,
		       price, currency, startDate, endDate, isActive, autoRenew, lastPaymentDate,
		       nextPaymentDate, alertSentAt, createdAt, updatedAt
		FROM Subscription WHERE id = ?
	`

	var sub models.Subscription
	err := h.db.QueryRow(query, id).Scan(
		&sub.ID, &sub.PlanName, &sub.PlanType, &sub.MaxUsers, &sub.MaxAircraft,
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
