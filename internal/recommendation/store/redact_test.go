package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRecommendationRedaction_NoSecretsOrRawLocationInLogsOrTraces is the
// SCN-039-053 unit-level guard: serialized recommendation logs and
// traces MUST NOT leak provider keys, raw provider payloads, exact GPS
// coordinates, or sensitive graph prompt text.
//
// The test covers:
//
//   - a benign serialized log line that should NOT trip the guard;
//   - five adversarial payloads (one per forbidden category) that MUST
//     trip the guard, each labeled with the marker it leaked.
//
// Each adversarial case explicitly demonstrates that the bug being
// guarded against would slip past a regex that only looked at field
// names without checking that the value is non-empty.
func TestRecommendationRedaction_NoSecretsOrRawLocationInLogsOrTraces(t *testing.T) {
	// Realistic provider key fixture — included as a forbidden substring
	// so the guard rejects this exact key even if it leaked under a
	// non-standard JSON key (e.g. "X-Goog-Api-Key" in an embedded log).
	forbiddenKey := "AIzaSyA-fixture-google-places-key-39_06"
	forbiddenSubstrings := []string{forbiddenKey, "secret-graph-prompt-token"}

	t.Run("safe-payload-passes", func(t *testing.T) {
		safe := mustJSON(t, map[string]any{
			"event":              "recommendation_persist_outcome",
			"watch_kind":         "topic_keyword",
			"actor_user_id":      "local",
			"location_precision": "neighborhood",
			"location_cell_id":   "h3:8a2a1072b59ffff",
			"recommendation_id":  "rec_demo",
			"providers": []map[string]any{
				{"id": "fixture_google_places", "status": "healthy"},
				{"id": "fixture_yelp", "status": "degraded"},
			},
			"api_key":              "", // empty placeholder is allowed
			"raw_provider_payload": "",
		})
		if err := AssertRedactSafe(safe, forbiddenSubstrings); err != nil {
			t.Fatalf("safe payload tripped guard: %v", err)
		}
	})

	t.Run("provider-api-key-blocked", func(t *testing.T) {
		// A leaked provider API key in an embedded log line must be
		// detected even when wrapped in a tracer envelope.
		leak := mustJSON(t, map[string]any{
			"event":  "provider_call_audit",
			"detail": "POST https://maps.googleapis.com/place/details?key=" + forbiddenKey,
		})
		err := AssertRedactSafe(leak, forbiddenSubstrings)
		if err == nil {
			t.Fatal("expected guard to flag leaked provider API key")
		}
		if !IsRedactionViolation(err) {
			t.Fatalf("expected RedactionViolation, got %T: %v", err, err)
		}
	})

	t.Run("secret-field-non-empty-blocked", func(t *testing.T) {
		// Even when the field name is plausible (e.g. "client_secret"
		// inside a configuration snapshot), a non-empty value MUST be
		// rejected. The empty-string variant is allowed (placeholder
		// pattern from config/smackerel.yaml).
		leak := mustJSON(t, map[string]any{
			"event":         "config_snapshot",
			"client_secret": "deadbeefcafebabe1234",
		})
		if err := AssertRedactSafe(leak, nil); err == nil {
			t.Fatal("expected guard to flag non-empty client_secret value")
		}
	})

	t.Run("raw-gps-coordinate-blocked", func(t *testing.T) {
		// A raw GPS local-ref MUST never appear in non-redacted logs;
		// the reactive engine reduces precision before persistence.
		leak := mustJSON(t, map[string]any{
			"event":           "trace_turn_persist",
			"location_ref":    "gps:37.7749,-122.4194",
			"precision_label": "exact",
		})
		err := AssertRedactSafe(leak, nil)
		if err == nil {
			t.Fatal("expected guard to flag raw GPS coordinate")
		}
		if !strings.Contains(err.Error(), "raw-gps-coordinate") {
			t.Fatalf("expected raw-gps-coordinate marker, got %v", err)
		}
	})

	t.Run("raw-provider-payload-blocked", func(t *testing.T) {
		// Raw provider payloads must be reduced to a hash before
		// persistence; a non-empty raw_payload field is a leak.
		leak := mustJSON(t, map[string]any{
			"event":       "provider_response",
			"raw_payload": "{\"name\":\"Tartine Bakery\",\"address\":\"600 Guerrero St\"}",
		})
		if err := AssertRedactSafe(leak, nil); err == nil {
			t.Fatal("expected guard to flag non-empty raw_payload")
		}
	})

	t.Run("sensitive-graph-text-blocked", func(t *testing.T) {
		// The agent tracer's redact policy can permit sensitive_graph_text
		// in the in-flight envelope; downstream persistence MUST still be
		// guarded.
		leak := mustJSON(t, map[string]any{
			"event":                "graph_snapshot",
			"sensitive_graph_text": "user noted dietary restriction: severe peanut allergy",
		})
		if err := AssertRedactSafe(leak, nil); err == nil {
			t.Fatal("expected guard to flag non-empty sensitive_graph_text")
		}
	})

	t.Run("empty-input-passes", func(t *testing.T) {
		if err := AssertRedactSafe("", forbiddenSubstrings); err != nil {
			t.Fatalf("empty serialized input must pass: %v", err)
		}
	})
}

// mustJSON marshals v and fails the test on error. Test-only helper.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}
