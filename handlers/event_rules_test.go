package handlers

import (
	"strings"
	"testing"

	"fdm-backend/models"
)

func number(value float64) *float64 { return &value }

func validDefinitionRequest() models.EventDefinitionRequest {
	return models.EventDefinitionRequest{
		EventCode:        "HL-001",
		EventType:        "safety",
		EventName:        "Hard landing",
		DisplayName:      "Hard Landing",
		EventDescription: "Vertical acceleration exceeded the landing limit.",
		SOP:              "Review landing technique and recorded-data quality.",
		ChangeSummary:    "Initial controlled definition.",
		Assignments: []models.EventAssignmentInput{{
			ScopeType: "aircraft",
		}},
		Rule: models.EventRule{
			SchemaVersion: models.EventRuleSchemaVersion,
			Parameters: []models.RuleParameter{{
				ID: "VERTICAL ACCELERATION", DataType: "numeric", Unit: "g", Required: true,
			}},
			Applicability: models.RuleApplicability{Phases: []string{"LANDING"}},
			Trigger: models.RuleNode{
				Type: "comparison", Operator: ">",
				Left:  &models.RuleNode{Type: "parameter", ParameterID: "VERTICAL ACCELERATION", Unit: "g"},
				Right: &models.RuleNode{Type: "number", NumberValue: number(2.0), Unit: "g"},
			},
			Temporal: models.RuleTemporal{MinimumDurationMs: 100, MergeGapMs: 250, CooldownMs: 1000},
			Measurement: models.RuleMeasurement{
				ParameterID: "VERTICAL ACCELERATION", Aggregation: "MAX", Unit: "g",
			},
			Severities: []models.RuleSeverity{
				{Level: "LOW", Operator: ">", Threshold: 2.0, MinimumDurationMs: 100},
				{Level: "MEDIUM", Operator: ">", Threshold: 2.3, MinimumDurationMs: 100},
				{Level: "HIGH", Operator: ">", Threshold: 2.6, MinimumDurationMs: 100},
				{Level: "CRITICAL", Operator: ">", Threshold: 3.0, MinimumDurationMs: 100},
			},
		},
	}
}

func TestValidateEventDefinitionAcceptsCanonicalRule(t *testing.T) {
	req := validDefinitionRequest()
	result := validateEventDefinition(&req)
	if len(result.Errors) != 0 {
		t.Fatalf("expected valid definition, got errors: %v", result.Errors)
	}
	if len(result.RuleHash) != 64 || result.RuleJSON == "" {
		t.Fatalf("expected canonical JSON and SHA-256 hash, got %q", result.RuleHash)
	}

	projection, err := buildLegacyProjection(req.Rule)
	if err != nil {
		t.Fatalf("projection failed: %v", err)
	}
	if !strings.Contains(projection.EventTrigger, "[VERTICAL ACCELERATION]") {
		t.Fatalf("projection did not preserve parameter identity: %s", projection.EventTrigger)
	}
}

func TestValidateEventDefinitionRejectsUnorderedSeverity(t *testing.T) {
	req := validDefinitionRequest()
	req.Rule.Severities[1].Threshold = 1.5
	result := validateEventDefinition(&req)
	if len(result.Errors) == 0 {
		t.Fatal("expected a decreasing high-side severity threshold to be rejected")
	}
}

func TestValidateEventDefinitionSupportsLowSideSeverity(t *testing.T) {
	req := validDefinitionRequest()
	req.Rule.Trigger.Operator = "<"
	req.Rule.Trigger.Right.NumberValue = number(-1.0)
	req.Rule.Measurement.Aggregation = "MIN"
	req.Rule.Severities = []models.RuleSeverity{
		{Level: "LOW", Operator: "<", Threshold: -1.0, MinimumDurationMs: 100},
		{Level: "MEDIUM", Operator: "<", Threshold: -1.5, MinimumDurationMs: 100},
		{Level: "HIGH", Operator: "<", Threshold: -2.0, MinimumDurationMs: 100},
		{Level: "CRITICAL", Operator: "<", Threshold: -2.5, MinimumDurationMs: 100},
	}
	result := validateEventDefinition(&req)
	if len(result.Errors) != 0 {
		t.Fatalf("expected valid low-side severity definition, got: %v", result.Errors)
	}
}

func TestValidateEventDefinitionRejectsUndeclaredParameter(t *testing.T) {
	req := validDefinitionRequest()
	req.Rule.Trigger.Left.ParameterID = "NOT DECLARED"
	result := validateEventDefinition(&req)
	if len(result.Errors) == 0 {
		t.Fatal("expected an undeclared parameter reference to be rejected")
	}
}
