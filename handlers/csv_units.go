package handlers

import (
	"regexp"
	"strconv"
	"strings"
)

var csvUnitSuffix = regexp.MustCompile(`\s*(?:\[[^\]]+\]|\([^)]+\))\s*$`)

var csvUnitToken = regexp.MustCompile(`(?i)^(?:ft(?:\s+(?:msl|baro|wgs|agl))?|feet|m(?:eters?)?|cm|mm|km|nm|kt|kts|knots?|mph|kph|km/h|m/s|m/s2|m/s\^2|fpm|ft/min|deg(?:rees?)?(?:\s*[cf])?|[°º][cf]|[cf]|rad|g|%|percent|rpm|hz|mhz|khz|v|volts?|a|amps?|psi|pa|kpa|hpa|inhg|inch|inches|bar|mb|mbar|kg|lbs?|lbf|n|ft\s*[-*·]?\s*lbs?|ft\s*[-*·]?\s*lbf|gals?|liters?|litres?|l|gph|gpm|l/h|kg/h|kg/hr|lbs?/h|lbs?/hr|s|sec(?:onds?)?|ms|min(?:utes?)?|hours?|bool|boolean|enum|ident|text|string|hh[: -]?mm(?::?ss(?:\.sss)?)?|yyy?y?[-/]mm[-/]dd|dd[-/]mm[-/]yyy?y?)$`)

func cleanCSVUnit(value string, first bool) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	if first {
		value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	}
	if len(value) >= 2 && ((value[0] == '[' && value[len(value)-1] == ']') || (value[0] == '(' && value[len(value)-1] == ')')) {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	switch strings.ToLower(value) {
	case "-", "--", "---", "n/a", "na", "none", "null", "unknown", "not available":
		return ""
	}
	return value
}

// Recognise unit rows by their content, not aircraft-specific column names.
// Unknown tokens are kept only when the surrounding row provides strong unit cues.
func isCSVUnitRecord(record []string, width int) bool {
	if len(record) == 0 || width == 0 {
		return false
	}
	if len(record) > width {
		for _, value := range record[width:] {
			if strings.TrimSpace(value) != "" {
				return false
			}
		}
	}
	nonempty, recognised, bracketed := 0, 0, 0
	for index, raw := range record {
		if index >= width {
			break
		}
		value := cleanCSVUnit(raw, index == 0)
		if value == "" {
			continue
		}
		nonempty++
		trimmed := strings.TrimSpace(raw)
		wrapped := (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) || (strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")"))
		if _, err := strconv.ParseFloat(value, 64); err == nil && !wrapped {
			return false
		}
		if wrapped {
			bracketed++
		}
		if csvUnitToken.MatchString(value) {
			recognised++
		}
	}
	return nonempty > 0 && ((recognised == nonempty || (recognised >= 2 && recognised*2 >= nonempty)) || (bracketed >= 2 && bracketed == nonempty))
}

func normalizedCSVUnitRecord(record []string, width int) []string {
	result := make([]string, width)
	for index := range result {
		if index >= len(record) {
			continue
		}
		unit := cleanCSVUnit(record[index], index == 0)
		if unit != "" {
			result[index] = "[" + unit + "]"
		}
	}
	return result
}
