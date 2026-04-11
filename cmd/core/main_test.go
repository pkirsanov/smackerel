package main

import (
	"testing"
)

// --- parseJSONArray tests ---

func TestParseJSONArray_ValidArray(t *testing.T) {
	result := parseJSONArray(`["a", "b", "c"]`)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected values: %v", result)
	}
}

func TestParseJSONArray_EmptyString(t *testing.T) {
	result := parseJSONArray("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseJSONArray_EmptyArray(t *testing.T) {
	result := parseJSONArray("[]")
	if result == nil {
		t.Fatal("expected non-nil for empty JSON array")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestParseJSONArray_InvalidJSON(t *testing.T) {
	result := parseJSONArray("{not valid json")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestParseJSONArray_MixedTypes(t *testing.T) {
	result := parseJSONArray(`["text", 42, true, null]`)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
}

func TestParseJSONArray_NestedArrays(t *testing.T) {
	result := parseJSONArray(`[["a", "b"], ["c"]]`)
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
}

func TestParseJSONArray_NotAnArray(t *testing.T) {
	// A JSON object is not a valid JSON array
	result := parseJSONArray(`{"key": "value"}`)
	if result != nil {
		t.Errorf("expected nil for JSON object input, got %v", result)
	}
}

// --- parseJSONObject tests ---

func TestParseJSONObject_ValidObject(t *testing.T) {
	result := parseJSONObject(`{"key": "value", "count": 42}`)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
	if result["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", result["count"])
	}
}

func TestParseJSONObject_EmptyString(t *testing.T) {
	result := parseJSONObject("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseJSONObject_EmptyObject(t *testing.T) {
	result := parseJSONObject("{}")
	if result == nil {
		t.Fatal("expected non-nil for empty JSON object")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 keys, got %d", len(result))
	}
}

func TestParseJSONObject_InvalidJSON(t *testing.T) {
	result := parseJSONObject("[not valid")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestParseJSONObject_NotAnObject(t *testing.T) {
	// A JSON array is not a valid JSON object
	result := parseJSONObject(`["a", "b"]`)
	if result != nil {
		t.Errorf("expected nil for JSON array input, got %v", result)
	}
}

func TestParseJSONObject_NestedObject(t *testing.T) {
	result := parseJSONObject(`{"outer": {"inner": "value"}}`)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	inner, ok := result["outer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", result["outer"])
	}
	if inner["inner"] != "value" {
		t.Errorf("expected inner=value, got %v", inner["inner"])
	}
}

// --- parseFloatEnv tests ---

func TestParseFloatEnv_ValidFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "3.14")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 3.14 {
		t.Errorf("expected 3.14, got %f", result)
	}
}

func TestParseFloatEnv_Integer(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "42")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 42.0 {
		t.Errorf("expected 42.0, got %f", result)
	}
}

func TestParseFloatEnv_EmptyString(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for empty string, got %f", result)
	}
}

func TestParseFloatEnv_UnsetVar(t *testing.T) {
	// Ensure the var does not exist
	t.Setenv("TEST_FLOAT_UNSET", "")
	result := parseFloatEnv("TEST_FLOAT_UNSET")
	if result != 0 {
		t.Errorf("expected 0 for unset var, got %f", result)
	}
}

func TestParseFloatEnv_InvalidFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "not-a-number")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for invalid float, got %f", result)
	}
}

func TestParseFloatEnv_NegativeFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "-1.5")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != -1.5 {
		t.Errorf("expected -1.5, got %f", result)
	}
}

func TestParseFloatEnv_Zero(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "0")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0, got %f", result)
	}
}

func TestParseFloatEnv_ScientificNotation(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "1.5e2")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 150.0 {
		t.Errorf("expected 150.0, got %f", result)
	}
}
