package handlers

import "encoding/json"

// Processing diagnostics are useful to customers; unvalidated detection evidence
// and its counts are not. Apply this projection to every customer response,
// never to persisted evidence or the Admin/FDA review workflow.
func customerAnalysis(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if json.Unmarshal(encoded, &decoded) != nil {
		return nil
	}
	return stripUnvalidatedEvidence(decoded)
}

func stripUnvalidatedEvidence(value any) any {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "occurrenceCount", "occurrences", "exceedanceCount", "exceedances", "pendingReview", "underReview":
				delete(node, key)
			default:
				node[key] = stripUnvalidatedEvidence(child)
			}
		}
	case []any:
		for index := range node {
			node[index] = stripUnvalidatedEvidence(node[index])
		}
	}
	return value
}

func customerAnalysisSummary(summary *string) *string {
	if summary == nil {
		return nil
	}
	var decoded any
	if json.Unmarshal([]byte(*summary), &decoded) != nil {
		return nil
	}
	encoded, err := json.Marshal(stripUnvalidatedEvidence(decoded))
	if err != nil {
		return nil
	}
	value := string(encoded)
	return &value
}
