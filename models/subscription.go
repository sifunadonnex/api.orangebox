package models

import "time"

// Subscription represents a subscription plan
type Subscription struct {
	ID                 string     `json:"id"`
	PlanName           string     `json:"planName" binding:"required"`
	PlanType           string     `json:"planType" binding:"required"` // monthly, yearly
	MaxUsers           int        `json:"maxUsers"`
	MaxAircraft        int        `json:"maxAircraft"`
	MaxFlightsPerMonth int        `json:"maxFlightsPerMonth"`
	MaxStorageGB       int        `json:"maxStorageGB"`
	Price              float64    `json:"price" binding:"required"`
	Currency           string     `json:"currency"`
	StartDate          time.Time  `json:"startDate" binding:"required"`
	EndDate            time.Time  `json:"endDate" binding:"required"`
	IsActive           bool       `json:"isActive"`
	AutoRenew          bool       `json:"autoRenew"`
	LastPaymentDate    *time.Time `json:"lastPaymentDate"`
	NextPaymentDate    *time.Time `json:"nextPaymentDate"`
	AlertSentAt        *time.Time `json:"alertSentAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// CreateSubscriptionRequest represents the request to create a subscription
type CreateSubscriptionRequest struct {
	PlanName           string    `json:"planName" binding:"required"`
	PlanType           string    `json:"planType" binding:"required"`
	MaxUsers           int       `json:"maxUsers"`
	MaxAircraft        int       `json:"maxAircraft"`
	MaxFlightsPerMonth int       `json:"maxFlightsPerMonth"`
	MaxStorageGB       int       `json:"maxStorageGB"`
	Price              float64   `json:"price" binding:"required"`
	Currency           string    `json:"currency"`
	StartDate          time.Time `json:"startDate" binding:"required"`
	EndDate            time.Time `json:"endDate" binding:"required"`
	AutoRenew          bool      `json:"autoRenew"`
}

// UpdateSubscriptionRequest represents the request to update a subscription
type UpdateSubscriptionRequest struct {
	PlanName           *string    `json:"planName"`
	PlanType           *string    `json:"planType"`
	MaxUsers           *int       `json:"maxUsers"`
	MaxAircraft        *int       `json:"maxAircraft"`
	MaxFlightsPerMonth *int       `json:"maxFlightsPerMonth"`
	MaxStorageGB       *int       `json:"maxStorageGB"`
	Price              *float64   `json:"price"`
	Currency           *string    `json:"currency"`
	StartDate          *time.Time `json:"startDate"`
	EndDate            *time.Time `json:"endDate"`
	IsActive           *bool      `json:"isActive"`
	AutoRenew          *bool      `json:"autoRenew"`
	LastPaymentDate    *time.Time `json:"lastPaymentDate"`
	NextPaymentDate    *time.Time `json:"nextPaymentDate"`
}

// SubscriptionStatus represents subscription status information
type SubscriptionStatus struct {
	IsActive       bool      `json:"isActive"`
	DaysRemaining  int       `json:"daysRemaining"`
	Status         string    `json:"status"` // active, expiring_soon, expired
	EndDate        time.Time `json:"endDate"`
	PlanName       string    `json:"planName"`
	UsersUsed      int       `json:"usersUsed"`
	UsersLimit     int       `json:"usersLimit"`
	AircraftUsed   int       `json:"aircraftUsed"`
	AircraftLimit  int       `json:"aircraftLimit"`
	FlightsUsed    int       `json:"flightsUsed"`
	FlightsLimit   int       `json:"flightsLimit"`
	StorageUsedGB  float64   `json:"storageUsedGB"`
	StorageLimitGB int       `json:"storageLimitGB"`
}
