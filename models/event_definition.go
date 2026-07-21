package models

import "time"

const (
	EventRuleSchemaVersion = 1

	EventVersionDraft     = "draft"
	EventVersionValidated = "validated"
	EventVersionPublished = "published"
	EventVersionRetired   = "retired"

	EventLifecycleActive  = "active"
	EventLifecycleRetired = "retired"
)

// RuleParameter declares the canonical signals required to evaluate a rule.
// Source column aliases remain an aircraft-decoding concern and are not stored
// in executable expressions.
type RuleParameter struct {
	ID       string `json:"id"`
	DataType string `json:"dataType"` // numeric or discrete
	Unit     string `json:"unit,omitempty"`
	Required bool   `json:"required"`
}

// RuleNode is a deliberately small, non-executable AST. It replaces persisted
// JavaScript strings and can be validated before a definition is published.
// Supported node types are all, any, not, comparison, arithmetic, parameter,
// number, and string.
type RuleNode struct {
	Type        string     `json:"type"`
	Operator    string     `json:"operator,omitempty"`
	ParameterID string     `json:"parameterId,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	NumberValue *float64   `json:"numberValue,omitempty"`
	StringValue *string    `json:"stringValue,omitempty"`
	Left        *RuleNode  `json:"left,omitempty"`
	Right       *RuleNode  `json:"right,omitempty"`
	Children    []RuleNode `json:"children,omitempty"`
}

type RuleApplicability struct {
	Phases    []string  `json:"phases"`
	Condition *RuleNode `json:"condition,omitempty"`
}

type RuleTemporal struct {
	MinimumDurationMs int64    `json:"minimumDurationMs"`
	MergeGapMs        int64    `json:"mergeGapMs"`
	CooldownMs        int64    `json:"cooldownMs"`
	ClearThreshold    *float64 `json:"clearThreshold,omitempty"`
}

type RuleMeasurement struct {
	ParameterID string `json:"parameterId"`
	Aggregation string `json:"aggregation"` // MAX, MIN, ABS_MAX, AVG, LAST
	Unit        string `json:"unit,omitempty"`
}

type RuleSeverity struct {
	Level             string  `json:"level"`
	Operator          string  `json:"operator"`
	Threshold         float64 `json:"threshold"`
	MinimumDurationMs int64   `json:"minimumDurationMs"`
}

type EventRule struct {
	SchemaVersion int               `json:"schemaVersion"`
	Parameters    []RuleParameter   `json:"parameters"`
	Applicability RuleApplicability `json:"applicability"`
	Trigger       RuleNode          `json:"trigger"`
	Temporal      RuleTemporal      `json:"temporal"`
	Measurement   RuleMeasurement   `json:"measurement"`
	Severities    []RuleSeverity    `json:"severities"`
}

type EventAssignmentInput struct {
	ScopeType    string  `json:"scopeType" binding:"required"`
	CompanyID    *string `json:"companyId,omitempty"`
	AircraftID   *string `json:"aircraftId,omitempty"`
	AircraftMake *string `json:"aircraftMake,omitempty"`
	ModelNumber  *string `json:"modelNumber,omitempty"`
}

type EventDefinitionRequest struct {
	EventCode        string                 `json:"eventCode" binding:"required"`
	EventType        string                 `json:"eventType" binding:"required"`
	EventName        string                 `json:"eventName" binding:"required"`
	DisplayName      string                 `json:"displayName" binding:"required"`
	EventDescription string                 `json:"eventDescription" binding:"required"`
	SOP              string                 `json:"sop" binding:"required"`
	ChangeSummary    string                 `json:"changeSummary" binding:"required"`
	Rule             EventRule              `json:"rule" binding:"required"`
	Assignments      []EventAssignmentInput `json:"assignments" binding:"required,min=1"`
}

type EventDefinitionAssignment struct {
	ID           string  `json:"id"`
	ScopeType    string  `json:"scopeType"`
	CompanyID    *string `json:"companyId,omitempty"`
	AircraftID   *string `json:"aircraftId,omitempty"`
	AircraftMake *string `json:"aircraftMake,omitempty"`
	ModelNumber  *string `json:"modelNumber,omitempty"`
}

// EventDefinitionResponse exposes the v2 rule while retaining read-only legacy
// projections required by existing dashboards until the detector is replaced.
type EventDefinitionResponse struct {
	ID               string                      `json:"id"`
	VersionID        string                      `json:"versionId"`
	Version          int                         `json:"version"`
	Status           string                      `json:"status"`
	LifecycleStatus  string                      `json:"lifecycleStatus"`
	CompanyID        *string                     `json:"companyId,omitempty"`
	EventCode        string                      `json:"eventCode"`
	EventType        string                      `json:"eventType"`
	EventName        string                      `json:"eventName"`
	DisplayName      string                      `json:"displayName"`
	EventDescription string                      `json:"eventDescription"`
	SOP              string                      `json:"sop"`
	ChangeSummary    string                      `json:"changeSummary"`
	SchemaVersion    int                         `json:"schemaVersion"`
	RuleHash         string                      `json:"ruleHash"`
	Rule             EventRule                   `json:"rule"`
	Assignments      []EventDefinitionAssignment `json:"assignments"`
	CreatedBy        string                      `json:"createdBy"`
	ApprovedBy       *string                     `json:"approvedBy,omitempty"`
	ValidatedAt      *time.Time                  `json:"validatedAt,omitempty"`
	PublishedAt      *time.Time                  `json:"publishedAt,omitempty"`
	EffectiveFrom    *time.Time                  `json:"effectiveFrom,omitempty"`
	EffectiveTo      *time.Time                  `json:"effectiveTo,omitempty"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
	IsActive         bool                        `json:"isActive"`

	// Legacy projections. These are generated from Rule and are never accepted
	// as an independent source of truth.
	EventParameter  string    `json:"eventParameter"`
	EventTrigger    string    `json:"eventTrigger"`
	FlightPhase     string    `json:"flightPhase"`
	TriggerType     string    `json:"triggerType"`
	DetectionPeriod string    `json:"detectionPeriod"`
	Severities      string    `json:"severities"`
	AircraftID      string    `json:"aircraftId"`
	Aircraft        *Aircraft `json:"aircraft,omitempty"`
}

type EventValidationResponse struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	RuleHash string   `json:"ruleHash,omitempty"`
}
