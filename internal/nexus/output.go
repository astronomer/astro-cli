package nexus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astronomer/nexus/shared"
)

var fieldFormatter = shared.NewFieldFormatter()

// formatAPIError extracts a human-readable message from an API error response.
// If the response is JSON with a "message" field, returns "Error: <message>".
// Otherwise returns the raw error string.
func formatAPIError(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}

	var errResp struct {
		Message    string `json:"message"`
		StatusCode int    `json:"statusCode"`
	}
	if err := json.Unmarshal([]byte(trimmed), &errResp); err == nil && errResp.Message != "" {
		if errResp.StatusCode > 0 {
			return fmt.Sprintf("Error (%d): %s", errResp.StatusCode, errResp.Message)
		}
		return "Error: " + errResp.Message
	}

	return raw
}

// formatResult pretty-prints JSON when running in a terminal, returns raw JSON otherwise.
func formatResult(result string) string {
	if !isTerminal() {
		return result
	}
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result
	}
	var raw any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return result
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return result
	}
	return string(pretty)
}
