// Spec 039 Scope 6 redaction guard.
//
// SCN-039-053 / R-027 / R-028 / R-034: serialized recommendation logs
// and traces MUST NOT leak provider API keys, raw provider payloads,
// raw GPS coordinates, or sensitive graph prompt text. The renderer-safe
// envelopes already exclude these surfaces, but operator log/trace
// inspection still needs an explicit guard so a regression in upstream
// store code can be caught by a single unit test.
//
// The redaction policy is intentionally narrow:
//
//   - provider api keys are never persisted in the store layer; the
//     guard rejects any output payload that contains a known key
//     fingerprint or a raw "api_key"/"access_token"/"secret"/"password"
//     property carrying a non-empty value.
//   - raw provider payloads are stored only on the boundary table
//     (recommendation_provider_facts.source_payload_hash) — never in
//     log or trace strings. The guard rejects any payload that contains
//     the marker substring `"raw_payload"` or `"raw_provider_payload"`
//     populated with a non-empty value.
//   - raw GPS coordinates (gps:<lat>,<lon>) MUST be reduced before
//     provider transmission. The guard rejects any payload that
//     contains a `gps:` ref AND a finer precision label than the
//     configured policy allows.
//   - sensitive graph prompt text (any value flagged x-sensitive=true
//     by the scenario contract) MUST never appear in non-redacted
//     persistence. The guard rejects payloads carrying the
//     `sensitive_graph_text` marker with a non-empty value.
//
// Usage: callers serialize the log/trace value to JSON, then call
// AssertRedactSafe(serialized, []string{...known forbidden substrings...}).
// The guard returns nil when the payload is safe and a typed
// RedactionViolation otherwise. This allows the same helper to be used
// from test code without coupling production code to test fixtures.
package store

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// RedactionViolation is the typed error returned by the guard when a
// serialized payload leaks a forbidden substring or pattern.
type RedactionViolation struct {
	Marker string
	Where  string
}

// Error implements the error interface.
func (v *RedactionViolation) Error() string {
	return fmt.Sprintf("recommendation redaction violation: %s leaked at %s", v.Marker, v.Where)
}

// gpsCoordPattern matches a raw GPS coordinate in the local-ref form
// `gps:<lat>,<lon>` (e.g. `gps:37.7749,-122.4194`). The pattern requires
// a sign-optional decimal number with at least one digit before and one
// digit after the decimal point so we never trigger on bare integers.
var gpsCoordPattern = regexp.MustCompile(`gps:-?\d+\.\d+,-?\d+\.\d+`)

// secretFieldPattern matches the JSON-encoded form of a non-empty secret
// field. The serialized envelope always uses double-quoted keys and the
// field-followed-by-colon form, so a regex over that representation is
// sufficient and avoids false positives from words like "secret_garden".
var secretFieldPattern = regexp.MustCompile(`"(api_key|access_token|password|client_secret|bearer_token)"\s*:\s*"[^"]+"`)

// rawPayloadFieldPattern matches the JSON-encoded form of a non-empty
// raw provider payload field. Empty strings are allowed because they
// indicate an explicit absence of payload.
var rawPayloadFieldPattern = regexp.MustCompile(`"(raw_payload|raw_provider_payload|raw_response_body)"\s*:\s*"[^"]+"`)

// sensitiveGraphPattern matches the marker the agent tracer emits for
// sensitive graph context that has been allowed into a structured
// envelope but MUST be redacted before persistence. A non-empty value is
// the violation.
var sensitiveGraphPattern = regexp.MustCompile(`"sensitive_graph_text"\s*:\s*"[^"]+"`)

// AssertRedactSafe scans a serialized payload (typically the JSON form
// of a structured log line or agent trace turn-log) for forbidden
// substrings and pattern matches. It returns nil when the payload is
// safe, and a typed *RedactionViolation otherwise.
//
// The forbiddenSubstrings parameter lets a caller add per-fixture
// markers (e.g. a known provider API key value) so the regression
// remains live even when the patterns above evolve.
func AssertRedactSafe(serialized string, forbiddenSubstrings []string) error {
	if serialized == "" {
		return nil
	}
	for _, marker := range forbiddenSubstrings {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		if strings.Contains(serialized, marker) {
			return &RedactionViolation{Marker: marker, Where: "forbidden-substring"}
		}
	}
	if loc := gpsCoordPattern.FindString(serialized); loc != "" {
		return &RedactionViolation{Marker: loc, Where: "raw-gps-coordinate"}
	}
	if loc := secretFieldPattern.FindString(serialized); loc != "" {
		return &RedactionViolation{Marker: loc, Where: "secret-field"}
	}
	if loc := rawPayloadFieldPattern.FindString(serialized); loc != "" {
		return &RedactionViolation{Marker: loc, Where: "raw-provider-payload"}
	}
	if loc := sensitiveGraphPattern.FindString(serialized); loc != "" {
		return &RedactionViolation{Marker: loc, Where: "sensitive-graph-text"}
	}
	return nil
}

// IsRedactionViolation reports whether err is a RedactionViolation.
// This is a thin convenience for test assertions.
func IsRedactionViolation(err error) bool {
	if err == nil {
		return false
	}
	var v *RedactionViolation
	return errors.As(err, &v)
}
