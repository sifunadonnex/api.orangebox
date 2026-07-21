package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"fdm-backend/models"
)

var allowedFlightPhases = map[string]bool{
	"GROUND": true, "TAXI-OUT": true, "TAXI-IN": true,
	"TAKEOFF": true, "INITIAL_CLIMB": true, "CLIMB": true,
	"CRUISE": true, "DESCENT": true, "APPROACH": true,
	"FINAL_APPROACH": true, "LANDING": true,
}

var severityRank = map[string]int{
	"LOW": 1, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4,
}

type ruleValidation struct {
	Errors   []string
	Warnings []string
	RuleJSON string
	RuleHash string
}

type legacyRuleProjection struct {
	EventParameter  string
	EventTrigger    string
	FlightPhase     string
	TriggerType     string
	DetectionPeriod string
	Severities      string
}

func validateEventDefinition(req *models.EventDefinitionRequest) ruleValidation {
	result := ruleValidation{Errors: []string{}, Warnings: []string{}}
	req.EventCode = strings.ToUpper(strings.TrimSpace(req.EventCode))
	req.EventType = strings.ToLower(strings.TrimSpace(req.EventType))
	req.EventName = strings.TrimSpace(req.EventName)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.EventDescription = strings.TrimSpace(req.EventDescription)
	req.SOP = strings.TrimSpace(req.SOP)
	req.ChangeSummary = strings.TrimSpace(req.ChangeSummary)

	if req.EventCode == "" || len(req.EventCode) > 64 {
		result.Errors = append(result.Errors, "eventCode is required and must not exceed 64 characters")
	}
	for _, r := range req.EventCode {
		if !(r == '-' || r == '_' || r == '.' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			result.Errors = append(result.Errors, "eventCode may contain only A-Z, 0-9, '.', '-' and '_'")
			break
		}
	}
	if req.EventType != "safety" && req.EventType != "fuel" && req.EventType != "maintenance" {
		result.Errors = append(result.Errors, "eventType must be safety, fuel, or maintenance")
	}
	if req.EventName == "" || req.DisplayName == "" || req.EventDescription == "" || req.SOP == "" {
		result.Errors = append(result.Errors, "eventName, displayName, eventDescription, and sop are required")
	}
	if req.ChangeSummary == "" {
		result.Errors = append(result.Errors, "changeSummary is required for auditability")
	}
	if len(req.Assignments) == 0 {
		result.Errors = append(result.Errors, "at least one applicability assignment is required")
	}

	rule := &req.Rule
	if rule.SchemaVersion != models.EventRuleSchemaVersion {
		result.Errors = append(result.Errors, fmt.Sprintf("unsupported rule schemaVersion %d", rule.SchemaVersion))
	}

	parameterTypes := make(map[string]string)
	parameterUnits := make(map[string]string)
	for index := range rule.Parameters {
		parameter := &rule.Parameters[index]
		parameter.ID = normalizeParameterID(parameter.ID)
		parameter.DataType = strings.ToLower(strings.TrimSpace(parameter.DataType))
		parameter.Unit = strings.TrimSpace(parameter.Unit)
		if parameter.ID == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("parameters[%d].id is required", index))
			continue
		}
		if _, exists := parameterTypes[parameter.ID]; exists {
			result.Errors = append(result.Errors, fmt.Sprintf("parameter %s is declared more than once", parameter.ID))
			continue
		}
		if parameter.DataType != "numeric" && parameter.DataType != "discrete" {
			result.Errors = append(result.Errors, fmt.Sprintf("parameter %s dataType must be numeric or discrete", parameter.ID))
		}
		if parameter.DataType == "numeric" && parameter.Unit == "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("numeric parameter %s has no engineering unit", parameter.ID))
		}
		parameterTypes[parameter.ID] = parameter.DataType
		parameterUnits[parameter.ID] = parameter.Unit
	}
	if len(rule.Parameters) == 0 {
		result.Errors = append(result.Errors, "rule.parameters must declare at least one parameter")
	}

	if len(rule.Applicability.Phases) == 0 {
		result.Errors = append(result.Errors, "applicability.phases must contain at least one flight phase")
	}
	seenPhases := make(map[string]bool)
	for index, phase := range rule.Applicability.Phases {
		phase = strings.ToUpper(strings.TrimSpace(phase))
		rule.Applicability.Phases[index] = phase
		if !allowedFlightPhases[phase] {
			result.Errors = append(result.Errors, fmt.Sprintf("unsupported flight phase %s", phase))
		}
		if seenPhases[phase] {
			result.Errors = append(result.Errors, fmt.Sprintf("flight phase %s is duplicated", phase))
		}
		seenPhases[phase] = true
	}

	triggerType := validateRuleNode(&rule.Trigger, "trigger", parameterTypes, parameterUnits, &result)
	if triggerType != "boolean" {
		result.Errors = append(result.Errors, "trigger must evaluate to a boolean value")
	}
	if rule.Applicability.Condition != nil {
		conditionType := validateRuleNode(rule.Applicability.Condition, "applicability.condition", parameterTypes, parameterUnits, &result)
		if conditionType != "boolean" {
			result.Errors = append(result.Errors, "applicability.condition must evaluate to a boolean value")
		}
	}

	if rule.Temporal.MinimumDurationMs < 0 || rule.Temporal.MergeGapMs < 0 || rule.Temporal.CooldownMs < 0 {
		result.Errors = append(result.Errors, "temporal durations must be non-negative milliseconds")
	}
	if rule.Temporal.MergeGapMs > 0 && rule.Temporal.MergeGapMs < rule.Temporal.MinimumDurationMs/10 {
		result.Warnings = append(result.Warnings, "mergeGapMs is very small relative to minimumDurationMs")
	}

	rule.Measurement.ParameterID = normalizeParameterID(rule.Measurement.ParameterID)
	rule.Measurement.Aggregation = strings.ToUpper(strings.TrimSpace(rule.Measurement.Aggregation))
	if parameterTypes[rule.Measurement.ParameterID] != "numeric" {
		result.Errors = append(result.Errors, "measurement.parameterId must reference a declared numeric parameter")
	}
	if !containsString([]string{"MAX", "MIN", "ABS_MAX", "AVG", "LAST"}, rule.Measurement.Aggregation) {
		result.Errors = append(result.Errors, "measurement.aggregation must be MAX, MIN, ABS_MAX, AVG, or LAST")
	}
	if expectedUnit := parameterUnits[rule.Measurement.ParameterID]; expectedUnit != "" && rule.Measurement.Unit != "" && expectedUnit != rule.Measurement.Unit {
		result.Errors = append(result.Errors, "measurement unit does not match the declared parameter unit")
	}

	validateSeverities(rule, &result)

	if len(result.Errors) == 0 {
		canonical, err := json.Marshal(rule)
		if err != nil {
			result.Errors = append(result.Errors, "rule could not be serialized")
		} else {
			digest := sha256.Sum256(canonical)
			result.RuleJSON = string(canonical)
			result.RuleHash = hex.EncodeToString(digest[:])
		}
	}

	return result
}

func validateRuleNode(node *models.RuleNode, path string, parameterTypes, parameterUnits map[string]string, result *ruleValidation) string {
	if node == nil {
		result.Errors = append(result.Errors, path+" is required")
		return "invalid"
	}
	node.Type = strings.ToLower(strings.TrimSpace(node.Type))
	node.Operator = strings.ToUpper(strings.TrimSpace(node.Operator))

	switch node.Type {
	case "all", "any":
		if len(node.Children) < 1 {
			result.Errors = append(result.Errors, path+" must contain at least one child")
		}
		for index := range node.Children {
			if validateRuleNode(&node.Children[index], fmt.Sprintf("%s.children[%d]", path, index), parameterTypes, parameterUnits, result) != "boolean" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s.children[%d] must be boolean", path, index))
			}
		}
		return "boolean"
	case "not":
		if len(node.Children) != 1 {
			result.Errors = append(result.Errors, path+" must contain exactly one child")
			return "boolean"
		}
		if validateRuleNode(&node.Children[0], path+".children[0]", parameterTypes, parameterUnits, result) != "boolean" {
			result.Errors = append(result.Errors, path+" child must be boolean")
		}
		return "boolean"
	case "comparison":
		if !containsString([]string{">", ">=", "<", "<=", "==", "!="}, node.Operator) {
			result.Errors = append(result.Errors, path+" has an unsupported comparison operator")
		}
		leftType := validateRuleNode(node.Left, path+".left", parameterTypes, parameterUnits, result)
		rightType := validateRuleNode(node.Right, path+".right", parameterTypes, parameterUnits, result)
		if leftType != rightType && leftType != "invalid" && rightType != "invalid" {
			result.Errors = append(result.Errors, path+" compares incompatible value types")
		}
		if containsString([]string{">", ">=", "<", "<="}, node.Operator) && (leftType != "number" || rightType != "number") {
			result.Errors = append(result.Errors, path+" ordered comparisons require numeric operands")
		}
		return "boolean"
	case "arithmetic":
		if !containsString([]string{"+", "-", "*", "/"}, node.Operator) {
			result.Errors = append(result.Errors, path+" has an unsupported arithmetic operator")
		}
		if validateRuleNode(node.Left, path+".left", parameterTypes, parameterUnits, result) != "number" || validateRuleNode(node.Right, path+".right", parameterTypes, parameterUnits, result) != "number" {
			result.Errors = append(result.Errors, path+" arithmetic operands must be numeric")
		}
		if node.Operator == "/" && node.Right != nil && node.Right.Type == "number" && node.Right.NumberValue != nil && *node.Right.NumberValue == 0 {
			result.Errors = append(result.Errors, path+" divides by zero")
		}
		return "number"
	case "parameter":
		node.ParameterID = normalizeParameterID(node.ParameterID)
		dataType, exists := parameterTypes[node.ParameterID]
		if !exists {
			result.Errors = append(result.Errors, fmt.Sprintf("%s references undeclared parameter %s", path, node.ParameterID))
			return "invalid"
		}
		if node.Unit != "" && parameterUnits[node.ParameterID] != "" && node.Unit != parameterUnits[node.ParameterID] {
			result.Errors = append(result.Errors, path+" unit does not match the parameter declaration")
		}
		if dataType == "numeric" {
			return "number"
		}
		return "string"
	case "number":
		if node.NumberValue == nil || math.IsNaN(valueOrZero(node.NumberValue)) || math.IsInf(valueOrZero(node.NumberValue), 0) {
			result.Errors = append(result.Errors, path+" must contain a finite numberValue")
		}
		return "number"
	case "string":
		if node.StringValue == nil || strings.TrimSpace(*node.StringValue) == "" {
			result.Errors = append(result.Errors, path+" must contain a non-empty stringValue")
		}
		return "string"
	default:
		result.Errors = append(result.Errors, path+" has an unsupported node type")
		return "invalid"
	}
}

func validateSeverities(rule *models.EventRule, result *ruleValidation) {
	if len(rule.Severities) == 0 {
		result.Errors = append(result.Errors, "at least one severity band is required")
		return
	}
	seen := make(map[string]bool)
	direction := ""
	previousRank := 0
	previousThreshold := 0.0
	for index := range rule.Severities {
		severity := &rule.Severities[index]
		severity.Level = strings.ToUpper(strings.TrimSpace(severity.Level))
		severity.Operator = strings.TrimSpace(severity.Operator)
		rank, validLevel := severityRank[severity.Level]
		if !validLevel {
			result.Errors = append(result.Errors, fmt.Sprintf("severities[%d].level is unsupported", index))
		}
		if seen[severity.Level] {
			result.Errors = append(result.Errors, fmt.Sprintf("severity %s is duplicated", severity.Level))
		}
		seen[severity.Level] = true
		if rank <= previousRank {
			result.Errors = append(result.Errors, "severities must be ordered LOW, MEDIUM, HIGH, CRITICAL")
		}
		if math.IsNaN(severity.Threshold) || math.IsInf(severity.Threshold, 0) {
			result.Errors = append(result.Errors, fmt.Sprintf("severities[%d].threshold must be finite", index))
		}
		if severity.MinimumDurationMs < 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("severities[%d].minimumDurationMs must be non-negative", index))
		}
		currentDirection := comparisonDirection(severity.Operator)
		if currentDirection == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("severities[%d].operator is unsupported", index))
		} else if direction == "" {
			direction = currentDirection
		} else if direction != currentDirection {
			result.Errors = append(result.Errors, "all severity operators must use the same comparison direction")
		}
		if index > 0 {
			if direction == "high" && severity.Threshold <= previousThreshold {
				result.Errors = append(result.Errors, "higher severities must have increasing thresholds")
			}
			if direction == "low" && severity.Threshold >= previousThreshold {
				result.Errors = append(result.Errors, "higher severities must have decreasing thresholds")
			}
		}
		previousRank = rank
		previousThreshold = severity.Threshold
	}
	if !seen["CRITICAL"] {
		result.Warnings = append(result.Warnings, "no CRITICAL severity band is defined")
	}
}

func buildLegacyProjection(rule models.EventRule) (legacyRuleProjection, error) {
	severityBytes, err := json.Marshal(rule.Severities)
	if err != nil {
		return legacyRuleProjection{}, err
	}
	parameterIDs := make([]string, 0, len(rule.Parameters))
	for _, parameter := range rule.Parameters {
		parameterIDs = append(parameterIDs, parameter.ID)
	}
	trigger, err := projectRuleNode(rule.Trigger)
	if err != nil {
		return legacyRuleProjection{}, err
	}
	detectionPeriod := "All Flight"
	if rule.Applicability.Condition != nil {
		detectionPeriod, err = projectRuleNode(*rule.Applicability.Condition)
		if err != nil {
			return legacyRuleProjection{}, err
		}
	}
	return legacyRuleProjection{
		EventParameter:  strings.Join(parameterIDs, ", "),
		EventTrigger:    trigger,
		FlightPhase:     strings.Join(rule.Applicability.Phases, ", "),
		TriggerType:     "structured-v1",
		DetectionPeriod: detectionPeriod,
		Severities:      string(severityBytes),
	}, nil
}

func projectRuleNode(node models.RuleNode) (string, error) {
	switch node.Type {
	case "all", "any":
		joiner := " && "
		if node.Type == "any" {
			joiner = " || "
		}
		parts := make([]string, 0, len(node.Children))
		for _, child := range node.Children {
			projected, err := projectRuleNode(child)
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+projected+")")
		}
		return strings.Join(parts, joiner), nil
	case "not":
		child, err := projectRuleNode(node.Children[0])
		return "!(" + child + ")", err
	case "comparison", "arithmetic":
		left, err := projectRuleNode(*node.Left)
		if err != nil {
			return "", err
		}
		right, err := projectRuleNode(*node.Right)
		if err != nil {
			return "", err
		}
		return "(" + left + " " + node.Operator + " " + right + ")", nil
	case "parameter":
		return "[" + node.ParameterID + "]", nil
	case "number":
		return strconv.FormatFloat(*node.NumberValue, 'f', -1, 64), nil
	case "string":
		encoded, _ := json.Marshal(*node.StringValue)
		return string(encoded), nil
	default:
		return "", fmt.Errorf("cannot project node type %s", node.Type)
	}
}

func normalizeParameterID(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func comparisonDirection(operator string) string {
	switch operator {
	case ">", ">=":
		return "high"
	case "<", "<=":
		return "low"
	default:
		return ""
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
