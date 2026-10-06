package ingestion

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxParameterFileBytes = 10 * 1024 * 1024

type ParameterProfileSummary struct {
	Format         string `json:"format"`
	ParameterCount int    `json:"parameterCount"`
	Message        string `json:"message"`
}

var sectionPattern = regexp.MustCompile(`(?im)^\s*\[\s*PARAMETER(?:\s*:\s*|\s+)[^\]]+\s*\]`)

func ValidateParameterProfile(filename string, content []byte) (ParameterProfileSummary, error) {
	if len(content) < 20 {
		return ParameterProfileSummary{}, errors.New("parameter definition is empty or too short")
	}
	if len(content) > MaxParameterFileBytes {
		return ParameterProfileSummary{}, fmt.Errorf("parameter definition exceeds %d MB", MaxParameterFileBytes/(1024*1024))
	}
	if !utf8.Valid(content) {
		return ParameterProfileSummary{}, errors.New("parameter definition must be valid UTF-8 text")
	}

	extension := strings.ToLower(filepath.Ext(filename))
	summary := ParameterProfileSummary{}
	switch extension {
	case ".tbx":
		summary.Format = "tbx"
	case ".prm", ".txt":
		summary.Format = "prm"
	case ".json":
		summary.Format = "json"
		var payload any
		if err := json.Unmarshal(content, &payload); err != nil {
			return ParameterProfileSummary{}, fmt.Errorf("invalid JSON parameter definition: %w", err)
		}
		summary.ParameterCount = jsonParameterCount(payload)
	case ".xml", ".fred":
		summary.Format = "fred"
		decoder := xml.NewDecoder(strings.NewReader(string(content)))
		for {
			token, err := decoder.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return ParameterProfileSummary{}, fmt.Errorf("invalid FRED XML: %w", err)
			}
			if start, ok := token.(xml.StartElement); ok && strings.Contains(strings.ToLower(start.Name.Local), "parameter") {
				summary.ParameterCount++
			}
		}
		profile, err := CompileFREDProfile(content)
		if err != nil {
			return ParameterProfileSummary{}, fmt.Errorf("FRED profile cannot be compiled: %w", err)
		}
		summary.ParameterCount = len(profile.Parameters)
	default:
		return ParameterProfileSummary{}, errors.New("supported parameter definitions are .tbx, .prm, .json, .xml, and .fred")
	}

	if summary.Format == "tbx" || summary.Format == "prm" {
		summary.ParameterCount = len(sectionPattern.FindAll(content, -1))
	}
	if summary.ParameterCount == 0 {
		return ParameterProfileSummary{}, errors.New("parameter definition contains no recognizable parameters")
	}
	summary.Message = fmt.Sprintf("Validated %d parameter definitions", summary.ParameterCount)
	return summary, nil
}

func ValidateDecoderConfig(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}", nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return "", fmt.Errorf("decoder configuration must be a JSON object: %w", err)
	}
	normalized, _ := json.Marshal(config)
	return string(normalized), nil
}

func jsonParameterCount(payload any) int {
	switch value := payload.(type) {
	case []any:
		return len(value)
	case map[string]any:
		if parameters, ok := value["params"].([]any); ok {
			return len(parameters)
		}
		if parameters, ok := value["parameters"].([]any); ok {
			return len(parameters)
		}
	}
	return 0
}
