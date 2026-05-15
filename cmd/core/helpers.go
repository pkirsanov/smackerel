package main

import (
	"encoding/json"
	"log/slog"
)

// parseJSONArray parses a JSON array string into []interface{}.
// Returns nil on empty string. Logs a warning with the key name and returns nil on parse error.
func parseJSONArray(s string) []interface{} {
	return parseJSONArrayVal("", s)
}

// parseJSONArrayVal parses a JSON array string with a key name for structured logging.
func parseJSONArrayVal(key, s string) []interface{} {
	if s == "" {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		slog.Warn("failed to parse JSON array from env var — using empty value", "key", key, "error", err, "input_length", len(s))
		return nil
	}
	return result
}
