package models

import (
	"time"
)

// User represents a user in the system
type User struct {
	ID          string    `json:"id" db:"id"`
	Email       string    `json:"email" db:"email"`
	Role        *string   `json:"role" db:"role"`
	FullName    *string   `json:"fullName" db:"fullName"`
	Designation *string   `json:"designation" db:"designation"`
	Department  *string   `json:"department" db:"department"`
	GateID      *string   `json:"gateId" db:"gateId"`
	Username    *string   `json:"username" db:"username"`
	Password    *string   `json:"password" db:"password"`
	Image       *string   `json:"image" db:"image"`
	Company     *string   `json:"company" db:"company"`
	Phone       *string   `json:"phone" db:"phone"`
	CreatedAt   time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updatedAt"`
}

// Aircraft represents an aircraft in the system
type Aircraft struct {
	ID           string    `json:"id" db:"id"`
	Airline      string    `json:"airline" db:"airline"`
	AircraftMake string    `json:"aircraftMake" db:"aircraftMake"`
	ModelNumber  *string   `json:"modelNumber" db:"modelNumber"`
	SerialNumber string    `json:"serialNumber" db:"serialNumber"`
	UserID       string    `json:"userId" db:"userId"`
	Parameters   *string   `json:"parameters" db:"parameters"`
	CreatedAt    time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updatedAt"`
}

// CSV represents a CSV file in the system
type CSV struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	File        string    `json:"file" db:"file"`
	Status      *string   `json:"status" db:"status"`
	Departure   *string   `json:"departure" db:"departure"`
	Pilot       *string   `json:"pilot" db:"pilot"`
	Destination *string   `json:"destination" db:"destination"`
	FlightHours *string   `json:"flightHours" db:"flightHours"`
	AircraftID  string    `json:"aircraftId" db:"aircraftId"`
	CreatedAt   time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updatedAt"`
}

// Flight represents a flight in the system
type Flight struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	AircraftID string    `json:"aircraftId" db:"aircraftId"`
	CreatedAt  time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt" db:"updatedAt"`
}

// EventLog represents an event log in the system
type EventLog struct {
	ID               string    `json:"id" db:"id"`
	EventName        *string   `json:"eventName" db:"eventName"`
	DisplayName      string    `json:"displayName" db:"displayName"`
	EventCode        string    `json:"eventCode" db:"eventCode"`
	EventDescription string    `json:"eventDescription" db:"eventDescription"`
	EventParameter   string    `json:"eventParameter" db:"eventParameter"`
	EventTrigger     string    `json:"eventTrigger" db:"eventTrigger"`
	EventType        string    `json:"eventType" db:"eventType"`
	FlightPhase      string    `json:"flightPhase" db:"flightPhase"`
	High             *string   `json:"high" db:"high"`
	High1            *string   `json:"high1" db:"high1"`
	High2            *string   `json:"high2" db:"high2"`
	Low              *string   `json:"low" db:"low"`
	Low1             *string   `json:"low1" db:"low1"`
	Low2             *string   `json:"low2" db:"low2"`
	SOP              string    `json:"sop" db:"sop"`
	AircraftID       string    `json:"aircraftId" db:"aircraftId"`
	CreatedAt        time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt" db:"updatedAt"`
}

// Exceedance represents an exceedance in the system
type Exceedance struct {
	ID               string    `json:"id" db:"id"`
	ExceedanceValues string    `json:"exceedanceValues" db:"exceedanceValues"`
	FlightPhase      string    `json:"flightPhase" db:"flightPhase"`
	ParameterName    string    `json:"parameterName" db:"parameterName"`
	Description      string    `json:"description" db:"description"`
	EventStatus      string    `json:"eventStatus" db:"eventStatus"`
	AircraftID       string    `json:"aircraftId" db:"aircraftId"`
	FlightID         string    `json:"flightId" db:"flightId"`
	File             *string   `json:"file" db:"file"`
	EventID          *string   `json:"eventId" db:"eventId"`
	Comment          *string   `json:"comment" db:"comment"`
	ExceedanceLevel  *string   `json:"exceedanceLevel" db:"exceedanceLevel"`
	CreatedAt        time.Time `json:"createdAt" db:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt" db:"updatedAt"`
}

// Request/Response DTOs

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// CreateUserRequest represents the create user request payload
type CreateUserRequest struct {
	FullName string `json:"fullName" binding:"required"`
	Role     string `json:"role" binding:"required"`
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Company  string `json:"company" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UpdateUserRequest represents the update user request payload
type UpdateUserRequest struct {
	UpdateLevel  string  `json:"updateLevel" binding:"required"`
	FullName     *string `json:"fullName,omitempty"`
	Username     *string `json:"username,omitempty"`
	Email        *string `json:"email,omitempty"`
	Company      *string `json:"company,omitempty"`
	Password     *string `json:"password,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	Department   *string `json:"department,omitempty"`
	Designation  *string `json:"designation,omitempty"`
	Role         *string `json:"role,omitempty"`
}

// CreateAircraftRequest represents the create aircraft request payload
type CreateAircraftRequest struct {
	Airline      string  `json:"airline" binding:"required"`
	AircraftMake string  `json:"aircraftMake" binding:"required"`
	SerialNumber string  `json:"serialNumber" binding:"required"`
	User         string  `json:"user" binding:"required"`
	Parameters   *string `json:"parameters,omitempty"`
}

// UpdateAircraftRequest represents the update aircraft request payload
type UpdateAircraftRequest struct {
	Airline      string  `json:"airline" binding:"required"`
	AircraftMake string  `json:"aircraftMake" binding:"required"`
	SerialNumber string  `json:"serialNumber" binding:"required"`
	User         string  `json:"user" binding:"required"`
	Parameters   *string `json:"parameters,omitempty"`
}

// UploadCSVRequest represents the CSV upload request payload
type UploadCSVRequest struct {
	Name        string  `form:"name" binding:"required"`
	AircraftID  string  `form:"aircraftId" binding:"required"`
	Departure   *string `form:"departure,omitempty"`
	Destination *string `form:"destination,omitempty"`
	FlightHours *string `form:"flightHours,omitempty"`
	Pilot       *string `form:"pilot,omitempty"`
}

// CreateEventRequest represents the create event request payload
type CreateEventRequest struct {
	EventName        *string `json:"eventName,omitempty"`
	DisplayName      string  `json:"displayName" binding:"required"`
	EventCode        string  `json:"eventCode" binding:"required"`
	EventDescription string  `json:"eventDescription" binding:"required"`
	EventParameter   string  `json:"eventParameter" binding:"required"`
	EventTrigger     string  `json:"eventTrigger" binding:"required"`
	EventType        string  `json:"eventType" binding:"required"`
	FlightPhase      string  `json:"flightPhase" binding:"required"`
	High             *string `json:"high,omitempty"`
	Low              *string `json:"low,omitempty"`
	Low1             *string `json:"low1,omitempty"`
	High1            *string `json:"high1,omitempty"`
	Low2             *string `json:"low2,omitempty"`
	High2            *string `json:"high2,omitempty"`
	SOP              string  `json:"sop" binding:"required"`
	AircraftID       string  `json:"aircraftId" binding:"required"`
}

// UpdateEventRequest represents the update event request payload
type UpdateEventRequest struct {
	EventName        *string `json:"eventName,omitempty"`
	DisplayName      string  `json:"displayName" binding:"required"`
	EventCode        string  `json:"eventCode" binding:"required"`
	EventDescription string  `json:"eventDescription" binding:"required"`
	EventParameter   string  `json:"eventParameter" binding:"required"`
	EventTrigger     string  `json:"eventTrigger" binding:"required"`
	EventType        string  `json:"eventType" binding:"required"`
	FlightPhase      string  `json:"flightPhase" binding:"required"`
	High             *string `json:"high,omitempty"`
	Low              *string `json:"low,omitempty"`
	Low1             *string `json:"low1,omitempty"`
	High1            *string `json:"high1,omitempty"`
	Low2             *string `json:"low2,omitempty"`
	High2            *string `json:"high2,omitempty"`
	SOP              string  `json:"sop" binding:"required"`
	AircraftID       string  `json:"aircraftId" binding:"required"`
}

// UpdateExceedanceRequest represents the update exceedance request payload
type UpdateExceedanceRequest struct {
	Comment     *string `json:"comment,omitempty"`
	EventStatus string  `json:"eventStatus" binding:"required"`
}
