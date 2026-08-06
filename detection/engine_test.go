package detection

import (
	"os"
	"path/filepath"
	"testing"

	"fdm-backend/models"
)

func TestAnalyzeFileUsesElapsedTimeAndEmitsHighestSeverity(t *testing.T) {
	path := writeTestCSV(t, "Sample,Time(sec),Phase,AIRSPEED\n"+
		"1,0,TAKEOFF,90\n"+
		"2,0.5,TAKEOFF,110\n"+
		"3,1.0,TAKEOFF,120\n"+
		"4,1.5,TAKEOFF,130\n"+
		"5,2.0,TAKEOFF,90\n")
	rule := numericRule("AIRSPEED", ">", 100)
	rule.Temporal.MinimumDurationMs = 1000
	rule.Severities = []models.RuleSeverity{
		{Level: "LOW", Operator: ">", Threshold: 100},
		{Level: "HIGH", Operator: ">", Threshold: 125},
	}

	result, err := AnalyzeFile(path, []Definition{testDefinition(rule)}, Options{SampleIntervalMs: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if result.TimingSource != "timesec" {
		t.Fatalf("expected explicit elapsed time, got %q", result.TimingSource)
	}
	if len(result.Occurrences) != 1 {
		t.Fatalf("expected one occurrence, got %d", len(result.Occurrences))
	}
	occurrence := result.Occurrences[0]
	if occurrence.DurationMs != 1000 || occurrence.Value != 130 || occurrence.Severity != "HIGH" {
		t.Fatalf("unexpected occurrence: %#v", occurrence)
	}
}

func TestAnalyzeFileMergesOnlyConfiguredTimeGap(t *testing.T) {
	path := writeTestCSV(t, "Sample,Phase,AIRSPEED\n"+
		"0,APPROACH,110\n"+
		"1,APPROACH,90\n"+
		"2,APPROACH,120\n"+
		"3,APPROACH,90\n"+
		"4,APPROACH,90\n")
	rule := numericRule("AIRSPEED", ">", 100)
	rule.Temporal.MinimumDurationMs = 2000
	rule.Temporal.MergeGapMs = 1000

	result, err := AnalyzeFile(path, []Definition{testDefinition(rule)}, Options{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Occurrences) != 1 {
		t.Fatalf("expected merged occurrence, got %#v", result.Occurrences)
	}
	if result.Occurrences[0].PointCount != 2 || result.Occurrences[0].DurationMs != 2000 {
		t.Fatalf("unexpected merged evidence: %#v", result.Occurrences[0])
	}
}

func TestAnalyzeFileSkipsRuleWithMissingRequiredColumn(t *testing.T) {
	path := writeTestCSV(t, "Sample,Phase,ALTITUDE\n0,CLIMB,1000\n1,CLIMB,1200\n")
	result, err := AnalyzeFile(path, []Definition{testDefinition(numericRule("AIRSPEED", ">", 100))}, Options{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.EvaluatedRuleCount != 0 || len(result.Occurrences) != 0 {
		t.Fatalf("missing parameter rule must not be evaluated: %#v", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "RULE_REQUIRED_COLUMNS_MISSING" {
		t.Fatalf("expected explicit missing-column diagnostic: %#v", result.Diagnostics)
	}
}

func TestAnalyzeFileHandlesUnitsRowAndDiscreteCondition(t *testing.T) {
	path := writeTestCSV(t, "Sample,Phase,SPEED,BRAKE\n,[ ],[ kt ],[ ]\n0,LANDING,80,OFF\n1,LANDING,70,ON\n2,LANDING,65,ON\n")
	rule := numericRule("SPEED", "<", 75)
	rule.Parameters = append(rule.Parameters, models.RuleParameter{ID: "BRAKE", DataType: "discrete", Required: true})
	on := "ON"
	rule.Applicability.Condition = &models.RuleNode{Type: "comparison", Operator: "==", Left: &models.RuleNode{Type: "parameter", ParameterID: "BRAKE"}, Right: &models.RuleNode{Type: "string", StringValue: &on}}
	rule.Severities = []models.RuleSeverity{{Level: "MEDIUM", Operator: "<", Threshold: 75}}

	result, err := AnalyzeFile(path, []Definition{testDefinition(rule)}, Options{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 3 || len(result.Occurrences) != 1 || result.Occurrences[0].Value != 65 {
		t.Fatalf("unexpected discrete/units result: %#v", result)
	}
}

func TestAnalyzeFileRejectsFrozenElapsedClockAndFallsBackToSample(t *testing.T) {
	path := writeTestCSV(t, "Sample,Time(sec),Phase,SPEED\n0,0,CRUISE,10\n1,0,CRUISE,11\n2,0,CRUISE,12\n")
	result, err := AnalyzeFile(path, nil, Options{SampleIntervalMs: 250})
	if err != nil {
		t.Fatal(err)
	}
	if result.TimingSource != "sample_interval" {
		t.Fatalf("a frozen elapsed clock is not reliable, got %q", result.TimingSource)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "NO_APPLICABLE_RULES" {
		t.Fatalf("expected explicit no-rules diagnostic: %#v", result.Diagnostics)
	}
}

func TestAnalyzeFileUsesCombinedLocalDateAndTime(t *testing.T) {
	path := writeTestCSV(t, "Lcl Date,Lcl Time,Latitude,Longitude,Phase,AIRSPEED\n"+
		"6/4/2025,06:38:16,-1.3,36.8,GROUND,0\n"+
		"6/4/2025,06:38:17,-1.3,36.8,TAKEOFF,110\n"+
		"6/4/2025,06:38:18,-1.2,36.7,CLIMB,120\n")
	result, err := AnalyzeFile(path, nil, Options{SampleIntervalMs: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.TimingSource != "date_time" || result.RowCount != 3 {
		t.Fatalf("unexpected date/time result: %#v", result)
	}
}

func numericRule(parameter, operator string, threshold float64) models.EventRule {
	return models.EventRule{
		SchemaVersion: 1,
		Parameters:    []models.RuleParameter{{ID: parameter, DataType: "numeric", Unit: "kt", Required: true}},
		Applicability: models.RuleApplicability{Phases: []string{"TAKEOFF", "APPROACH", "CLIMB", "LANDING"}},
		Trigger: models.RuleNode{Type: "comparison", Operator: operator,
			Left:  &models.RuleNode{Type: "parameter", ParameterID: parameter},
			Right: &models.RuleNode{Type: "number", NumberValue: &threshold}},
		Measurement: models.RuleMeasurement{ParameterID: parameter, Aggregation: aggregationFor(operator), Unit: "kt"},
		Severities:  []models.RuleSeverity{{Level: "LOW", Operator: operator, Threshold: threshold}},
	}
}

func aggregationFor(operator string) string {
	if operator == "<" || operator == "<=" {
		return "MIN"
	}
	return "MAX"
}

func testDefinition(rule models.EventRule) Definition {
	return Definition{DefinitionID: "definition-1", VersionID: "version-1", EventCode: "TEST-1", DisplayName: "Test event", Description: "Test definition", RuleHash: "rule-hash", Rule: rule}
}

func writeTestCSV(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flight.csv")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
