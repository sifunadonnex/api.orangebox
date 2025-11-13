package models

import "time"

// Company represents an organization/client company
type Company struct {
	ID             string        `json:"id"`
	Name           string        `json:"name" binding:"required"`
	Email          string        `json:"email" binding:"required,email"`
	Phone          *string       `json:"phone"`
	Address        *string       `json:"address"`
	Country        *string       `json:"country"`
	Logo           *string       `json:"logo"`
	Status         string        `json:"status"` // active, suspended, expired
	SubscriptionID *string       `json:"subscriptionId"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	Subscription   *Subscription `json:"subscription,omitempty"`
	Users          []User        `json:"users,omitempty"`
	Aircraft       []Aircraft    `json:"aircraft,omitempty"`
}

// CreateCompanyRequest represents the request to create a company
type CreateCompanyRequest struct {
	Name           string  `json:"name" binding:"required"`
	Email          string  `json:"email" binding:"required,email"`
	Phone          *string `json:"phone"`
	Address        *string `json:"address"`
	Country        *string `json:"country"`
	Logo           *string `json:"logo"`
	SubscriptionID *string `json:"subscriptionId"`
}

// UpdateCompanyRequest represents the request to update a company
type UpdateCompanyRequest struct {
	Name           *string `json:"name"`
	Email          *string `json:"email"`
	Phone          *string `json:"phone"`
	Address        *string `json:"address"`
	Country        *string `json:"country"`
	Logo           *string `json:"logo"`
	Status         *string `json:"status"`
	SubscriptionID *string `json:"subscriptionId"`
}
