package main

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"strconv"
)

// parseJSONArray parses a JSON array string into []interface{}.
// Returns nil on empty string. Logs a warning with the key name and returns nil on parse error.
func parseJSONArray(s string) []interface{} {
	return parseJSONArrayVal("", s)
}

// parseJSONArrayEnv reads an environment variable and parses it as a JSON array.
// Returns nil on empty string. Logs a warning with the key name and returns nil on parse error.
func parseJSONArrayEnv(key string) []interface{} {
	s := os.Getenv(key)
	return parseJSONArrayVal(key, s)
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

// parseJSONObject parses a JSON object string into map[string]interface{}.
// Returns nil on empty string. Logs a warning with the key name and returns nil on parse error.
func parseJSONObject(s string) map[string]interface{} {
	return parseJSONObjectVal("", s)
}

// parseJSONObjectEnv reads an environment variable and parses it as a JSON object.
// Returns nil on empty string. Logs a warning with the key name and returns nil on parse error.
func parseJSONObjectEnv(key string) map[string]interface{} {
	s := os.Getenv(key)
	return parseJSONObjectVal(key, s)
}

// parseJSONObjectVal parses a JSON object string with a key name for structured logging.
func parseJSONObjectVal(key, s string) map[string]interface{} {
	if s == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		slog.Warn("failed to parse JSON object from env var — using empty value", "key", key, "error", err, "input_length", len(s))
		return nil
	}
	return result
}

// parseFloatEnv reads an environment variable and parses it as float64.
// Returns 0 on empty string. Logs a warning and returns 0 on parse error.
func parseFloatEnv(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		slog.Warn("failed to parse float from env var — using 0", "key", key, "error", err)
		return 0
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		slog.Warn("non-finite float value in env var — using 0", "key", key, "value", s)
		return 0
	}
	return f
}
