package httpadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestHTTPAssistantTurnGoldenContractV1 pins schema v1 by comparing
// the encoded TurnRequest/TurnResponse shapes against committed
// golden fixtures. Any added, removed, renamed, or retyped wire
// field breaks this test, forcing SchemaVersionV1 + the goldens to
// move in lockstep (SCN-069-A07).
func TestHTTPAssistantTurnGoldenContractV1(t *testing.T) {
	t.Run("request_v1", func(t *testing.T) {
		got, gotKeys := decodeGolden(t, "request_v1.json")
		want := map[string]any{
			"schema_version":        SchemaVersionV1,
			"transport_message_id":  "test-turn-001",
			"kind":                  "text",
			"transport_hint":        "web",
			"text":                  "weather in palm springs ca tomorrow",
			"confirm_ref":           "",
			"confirm_choice":        "",
			"disambiguation_ref":    "",
			"disambiguation_choice": float64(0),
			"client_context": map[string]any{
				"conversation_id": "client-thread-001",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("request_v1 golden drift\n got: %v\nwant: %v", got, want)
		}
		assertKeysEqual(t, gotKeys, []string{
			"schema_version", "transport_message_id", "kind", "transport_hint",
			"text", "confirm_ref", "confirm_choice", "disambiguation_ref",
			"disambiguation_choice", "client_context",
		})

		// Round-trip: the golden fixture MUST decode into a valid
		// TurnRequest under the live wire validator.
		var req TurnRequest
		raw := mustReadGolden(t, "request_v1.json")
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("Unmarshal request golden: %v", err)
		}
		if err := req.Validate(defaultConfig()); err != nil {
			t.Fatalf("Validate request golden: %v", err)
		}
	})

	t.Run("response_v1", func(t *testing.T) {
		_, gotKeys := decodeGolden(t, "response_v1.json")
		assertKeysEqual(t, gotKeys, []string{
			"schema_version", "transport", "transport_message_id", "status", "body",
			"sources", "sources_overflow_count", "confirm_card",
			"disambiguation_prompt", "error_cause", "capture_route", "trace",
			"facade_invoked", "emitted_at",
		})

		// Round-trip: produce a TurnResponse via RenderJSON whose
		// encoded JSON matches the golden field-for-field.
		emittedAt := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
		resp := contracts.AssistantResponse{
			Status: contracts.StatusCheckingWeather,
			Body:   "Tomorrow in Palm Springs: 32C, sunny.",
			Sources: []contracts.Source{{
				ID:    "open-meteo-forecast",
				Title: "open-meteo",
				Kind:  contracts.SourceExternalProvider,
				Ref: contracts.ExternalProviderRef{
					ProviderName: "open-meteo",
					RetrievedAt:  emittedAt,
				},
			}},
			EmittedAt:  emittedAt,
			Invocation: nil,
		}
		out := RenderJSON(resp, "test-turn-001", "req-001", true)
		// Force the trace ids the golden pins (real wiring sets
		// AssistantTurnID/AgentTraceID from the audit substrate;
		// for this contract test we surface fixed values to keep
		// the golden deterministic).
		out.Trace.AssistantTurnID = "trace-001"
		out.Trace.AgentTraceID = "trace-001"

		gotJSON, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("Marshal response: %v", err)
		}
		var gotMap map[string]any
		if err := json.Unmarshal(gotJSON, &gotMap); err != nil {
			t.Fatalf("re-Unmarshal response: %v", err)
		}
		wantMap, _ := decodeGolden(t, "response_v1.json")
		if !reflect.DeepEqual(gotMap, wantMap) {
			t.Fatalf("response_v1 golden drift\n got: %#v\nwant: %#v", gotMap, wantMap)
		}
		if _, present := gotMap["notice"]; present {
			t.Fatalf("notice-absent response must omit \"notice\" key from the wire body (omitempty); got %#v", gotMap)
		}
	})

	// TP-075-25 (SCN-075-A14) — notice-present round trip pins the
	// optional `notice` sub-object on the wire contract while
	// schema_version stays "v1" (additive v1-compatible field).
	t.Run("response_v1_notice", func(t *testing.T) {
		_, gotKeys := decodeGolden(t, "response_v1_notice.json")
		assertKeysEqual(t, gotKeys, []string{
			"schema_version", "transport", "transport_message_id", "status", "body",
			"sources", "sources_overflow_count", "confirm_card",
			"disambiguation_prompt", "error_cause", "capture_route", "trace",
			"facade_invoked", "emitted_at", "notice",
		})

		emittedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		resp := contracts.AssistantResponse{
			Status:    contracts.StatusCheckingWeather,
			Body:      "Tomorrow in Palm Springs: 32C, sunny.",
			Sources:   nil,
			EmittedAt: emittedAt,
			LegacyRetirementNotice: &contracts.NoticePayload{
				Command:            "/weather",
				ReplacementExample: "weather in palm springs tomorrow",
				CopyKey:            "spec066.weather",
				WindowID:           "2026Q2",
			},
		}
		out := RenderJSON(resp, "test-turn-002", "req-002", true)
		out.Trace.AssistantTurnID = "trace-002"
		out.Trace.AgentTraceID = "trace-002"

		gotJSON, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("Marshal notice response: %v", err)
		}
		var gotMap map[string]any
		if err := json.Unmarshal(gotJSON, &gotMap); err != nil {
			t.Fatalf("re-Unmarshal notice response: %v", err)
		}
		wantMap, _ := decodeGolden(t, "response_v1_notice.json")
		if !reflect.DeepEqual(gotMap, wantMap) {
			t.Fatalf("response_v1_notice golden drift\n got: %#v\nwant: %#v", gotMap, wantMap)
		}
		// Schema_version must remain "v1" — additive optional field.
		if got := gotMap["schema_version"]; got != SchemaVersionV1 {
			t.Fatalf("notice round-trip must keep schema_version=%q, got %q", SchemaVersionV1, got)
		}
	})
}

func mustReadGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return raw
}

func decodeGolden(t *testing.T, name string) (map[string]any, []string) {
	t.Helper()
	raw := mustReadGolden(t, name)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode golden %s: %v", name, err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return m, keys
}

func assertKeysEqual(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wire field set drift\n got: %v\nwant: %v", got, want)
	}
}
