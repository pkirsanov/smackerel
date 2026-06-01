// Spec 068 SCOPE-1 — CompiledIntent schema validation.
//
// ValidateCompiledIntent enforces the closed-vocabulary + required
// fields contract defined in spec.md §3 ("CompiledIntent minimum
// schema"). ParseAndValidate is the single entry point the compiler
// uses to turn raw JSON bytes from the LLM sidecar into a typed
// CompiledIntent or a typed error. Malformed JSON, unknown enum
// values, missing required fields, or out-of-range confidence all
// produce a SchemaError so the facade can take the canonical
// compiler-failure path (spec.md SCN-068-A06).

package intent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SchemaError is the typed compiler-failure carrier. The Cause field
// is the closed-vocabulary cause label stamped on the
// intent_compiler_error_total metric.
type SchemaError struct {
	Cause   string // "schema_invalid" | "json_invalid"
	Detail  string
	RawBody []byte
}

func (e *SchemaError) Error() string {
	if e == nil {
		return "<nil schema error>"
	}
	return fmt.Sprintf("intent compiler %s: %s", e.Cause, e.Detail)
}

// IsSchemaError unwraps the typed schema error.
func IsSchemaError(err error) (*SchemaError, bool) {
	var se *SchemaError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}

// ParseAndValidate parses raw compiler JSON bytes and runs the closed-
// vocabulary + required-field checks. Returns a SchemaError on any
// failure; the caller increments
// intent_compiler_error_total{cause=...} and emits the canonical
// refusal-with-capture response (no router invocation). Spec 068
// SCN-068-A06.
func ParseAndValidate(raw []byte) (CompiledIntent, error) {
	var ci CompiledIntent
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ci); err != nil {
		return CompiledIntent{}, &SchemaError{
			Cause:   "json_invalid",
			Detail:  fmt.Sprintf("decode: %v", err),
			RawBody: raw,
		}
	}
	if err := ValidateCompiledIntent(ci); err != nil {
		// Promote validation errors to the schema_invalid cause used
		// by the intent_compiler_error_total metric.
		var se *SchemaError
		if errors.As(err, &se) {
			se.RawBody = raw
			return CompiledIntent{}, se
		}
		return CompiledIntent{}, &SchemaError{
			Cause:   "schema_invalid",
			Detail:  err.Error(),
			RawBody: raw,
		}
	}
	return ci, nil
}

// ValidateCompiledIntent enforces the closed-vocabulary + required-
// field contract defined in spec.md §3 and design §"Data Model".
func ValidateCompiledIntent(ci CompiledIntent) error {
	if ci.Version == "" {
		return &SchemaError{Cause: "schema_invalid", Detail: "version is required"}
	}
	if ci.Language == "" {
		return &SchemaError{Cause: "schema_invalid", Detail: "language is required"}
	}
	if ci.UserGoal == "" {
		return &SchemaError{Cause: "schema_invalid", Detail: "user_goal is required"}
	}
	if !isValidActionClass(ci.ActionClass) {
		return &SchemaError{Cause: "schema_invalid", Detail: fmt.Sprintf("action_class %q is not in closed vocabulary", ci.ActionClass)}
	}
	if !isValidSideEffectClass(ci.SideEffectClass) {
		return &SchemaError{Cause: "schema_invalid", Detail: fmt.Sprintf("side_effect_class %q is not in closed vocabulary", ci.SideEffectClass)}
	}
	if ci.Confidence < 0 || ci.Confidence > 1 {
		return &SchemaError{Cause: "schema_invalid", Detail: fmt.Sprintf("confidence %.4f out of range [0,1]", ci.Confidence)}
	}
	// clarify requires a clarification prompt or at least one missing slot.
	if ci.ActionClass == ActionClarify && (ci.ClarificationPrompt == nil || *ci.ClarificationPrompt == "") && len(ci.MissingSlots) == 0 {
		return &SchemaError{Cause: "schema_invalid", Detail: "action_class=clarify requires a clarification_prompt or missing_slots"}
	}
	// source_policy.allowed_source_kinds must not contain empties.
	for i, k := range ci.SourcePolicy.AllowedSourceKinds {
		if k == "" {
			return &SchemaError{Cause: "schema_invalid", Detail: fmt.Sprintf("source_policy.allowed_source_kinds[%d] is empty", i)}
		}
	}
	return nil
}

func isValidActionClass(a ActionClass) bool {
	for _, v := range AllActionClasses {
		if v == a {
			return true
		}
	}
	return false
}

func isValidSideEffectClass(s SideEffectClass) bool {
	for _, v := range AllSideEffectClasses {
		if v == s {
			return true
		}
	}
	return false
}
