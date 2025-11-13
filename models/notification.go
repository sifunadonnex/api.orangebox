package models

import "time"

type Notification struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	ExceedanceID string    `json:"exceedanceId"`
	Message      string    `json:"message"`
	Level        string    `json:"level"`
	IsRead       bool      `json:"isRead"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	
	// Relations
	Exceedance   *Exceedance `json:"exceedance,omitempty"`
	User         *User       `json:"user,omitempty"`
}

type CreateNotificationRequest struct {
	FlightID    string `json:"flightId"`
	AircraftID  string `json:"aircraftId"`
	Exceedances []struct {
		Description string `json:"description"`
		Level      string `json:"level"`
		Phase      string `json:"phase"`
		Parameter  string `json:"parameter"`
	} `json:"exceedances"`
}