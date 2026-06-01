// Spec 071 SCOPE-01 — IntentTrace v1 schema golden contract test
// (SCN-071-A10).
//
// This test pins the wire schema for the v1 IntentTrace RedactedPayload:
// every field name, JSON tag, required-field presence, and closed
// vocabulary value MUST be stable. Any change without bumping
// SchemaVersionV1 (and updating this test) fails the build.
//
// Why pin the canonical JSON literal here: the trace is consumed by
// replay (Scope 3), the spec 067 bypass guard, and the dashboard
// (Scope 4). A silent field rename or vocabulary expansion is exactly
// the kind of drift this guard exists to catch — spec 071 SCN-071-A10.

package intenttrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// goldenV1Payload is the canonical v1 RedactedPayload fixture. The
// numeric/string values are arbitrary but the SHAPE — every key, every
// type — is the contract.
const goldenV1Payload = `{
  "schema_version": "v1",
  "trace_id": "trace-01",
  "turn_id": "turn-01",
  "user_id_hash": "deadbeef",
  "transport": "web",
  "transport_message_id": "msg-01",
  "sampled": true,
  "compiler_invoked": true,
  "action_class": "external_lookup",
  "side_effect_class": "external_read",
  "confidence": 0.91,
  "route_decision": "scenarios/weather",
  "tool_calls": [
    {"name": "weather.lookup", "arguments_redacted": true, "outcome": "ok"}
  ],
  "final_response_status": "checking_weather",
  "model_route": "intent-compiler-v1",
  "seed": "seed-01",
  "slots_redaction_summary": {
    "raw_text": "absent",
    "slot_classes": {"location": "safe"},
    "redacted_count": 0
  }
}`

// goldenV1PayloadHash is the sha256 of the canonical JSON
// re-marshalled by encoding/json. Recompute when the schema legitimately
// bumps to v2 (and only when the SchemaVersion constant moves with it).
const goldenV1PayloadHash = "664428d7cc755e56d38273c8ea08131d3440a33f9ae4416a35224e94ef3e4653"

func TestSchemaVersionV1IsPinned(t *testing.T) {
	if SchemaVersionV1 != "v1" {
		t.Fatalf("SchemaVersionV1 must be %q; got %q. Schema bumps require a NEW constant.", "v1", SchemaVersionV1)
	}
}

func TestGoldenV1PayloadRoundTrip(t *testing.T) {
	var p RedactedPayload
	if err := json.Unmarshal([]byte(goldenV1Payload), &p); err != nil {
		t.Fatalf("golden v1 payload failed to unmarshal: %v", err)
	}
	if p.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("expected SchemaVersion %q, got %q", SchemaVersionV1, p.SchemaVersion)
	}
	if p.Transport != TransportWeb {
		t.Fatalf("expected transport %q, got %q", TransportWeb, p.Transport)
	}
	if p.FinalResponseStatus != StatusCheckingWeather {
		t.Fatalf("expected status %q, got %q", StatusCheckingWeather, p.FinalResponseStatus)
	}
	if len(p.ToolCalls) != 1 || p.ToolCalls[0].Name != "weather.lookup" {
		t.Fatalf("tool_calls did not round-trip: %+v", p.ToolCalls)
	}

	// Re-marshal and assert every documented contract field is
	// present in the JSON encoder output (catches a field rename
	// where the JSON tag drifts away from the contract name).
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("re-marshal failed: %v", err)
	}
	required := []string{
		`"schema_version"`,
		`"trace_id"`,
		`"turn_id"`,
		`"user_id_hash"`,
		`"transport"`,
		`"transport_message_id"`,
		`"sampled"`,
		`"compiler_invoked"`,
		`"action_class"`,
		`"side_effect_class"`,
		`"tool_calls"`,
		`"final_response_status"`,
		`"slots_redaction_summary"`,
	}
	encoded := string(out)
	for _, key := range required {
		if !strings.Contains(encoded, key) {
			t.Errorf("v1 schema drift: required field %s missing from encoded JSON: %s", key, encoded)
		}
	}
}

func TestGoldenV1PayloadHashPinned(t *testing.T) {
	// We canonicalise by Unmarshal→Marshal so whitespace in the
	// fixture does not affect the hash, but field order does. Go's
	// encoding/json emits struct fields in declaration order, so the
	// hash is stable as long as RedactedPayload's field ordering is
	// stable. Reordering struct fields (without bumping the schema
	// version) breaks the hash and trips the regression.
	var p RedactedPayload
	if err := json.Unmarshal([]byte(goldenV1Payload), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sum := sha256.Sum256(out)
	got := hex.EncodeToString(sum[:])
	if got != goldenV1PayloadHash {
		t.Fatalf("v1 payload hash drift: any rename/reorder/added-field without a SchemaVersion bump trips this guard.\n  got:  %s\n  want: %s\n  canonical JSON: %s", got, goldenV1PayloadHash, string(out))
	}
}

func TestClosedVocabulariesPinned(t *testing.T) {
	wantTransports := map[Transport]bool{
		TransportTelegram: true, TransportWhatsApp: true,
		TransportWeb: true, TransportMobile: true,
	}
	if len(AllTransports) != len(wantTransports) {
		t.Fatalf("transport vocabulary drift: AllTransports=%v", AllTransports)
	}
	for _, tp := range AllTransports {
		if !wantTransports[tp] {
			t.Fatalf("unexpected transport in v1 vocabulary: %q", tp)
		}
	}

	wantStatuses := map[FinalResponseStatus]bool{
		StatusOK: true, StatusClarify: true, StatusRefused: true,
		StatusCaptureFallback: true, StatusUnavailable: true, StatusCheckingWeather: true,
	}
	if len(AllFinalResponseStatuses) != len(wantStatuses) {
		t.Fatalf("status vocabulary drift: AllFinalResponseStatuses=%v", AllFinalResponseStatuses)
	}
	for _, s := range AllFinalResponseStatuses {
		if !wantStatuses[s] {
			t.Fatalf("unexpected status in v1 vocabulary: %q", s)
		}
	}
}
