package models

import "time"

// Session represents an active user session
type Session struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Token      string    `json:"token"`
	DeviceInfo *string   `json:"deviceInfo,omitempty"`
	IPAddress  *string   `json:"ipAddress,omitempty"`
	IsActive   bool      `json:"isActive"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// SessionInfo represents session details returned to the client
type SessionInfo struct {
	SessionID  string    `json:"sessionId"`
	DeviceInfo string    `json:"deviceInfo,omitempty"`
	IPAddress  string    `json:"ipAddress,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	IsCurrent  bool      `json:"isCurrent"`
}
