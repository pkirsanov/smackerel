// Spec 069 SCOPE-5 — Transport hint is closed-vocabulary and
// telemetry-only (SCN-069-A09).
//
// Unit test. Proves three contract invariants of the wire-level
// transport_hint field:
//
//   1. Every value in AllowedTransportHints is accepted by
//      TurnRequest.Validate when the configured allowlist exposes
//      it; the hint is recorded on the canonical AssistantMessage
//      ONLY in TransportMetadata["transport_hint"] (telemetry side
//      channel — never on AssistantMessage.Kind, Text, or any other
//      scenario-selection field).
//   2. An empty hint is accepted (the wire field is optional) and
//      does not populate TransportMetadata.
//   3. An unknown hint is REJECTED before facade invocation with a
//      stable error message naming the offending token, and no
//      AssistantMessage is produced.
//
// These three invariants together prove the hint is closed
// vocabulary AND telemetry-only: it cannot influence routing,
// tools, response shape, or side-effect behavior because it never
// lands anywhere a scenario, facade, or executor inspects.

package httpadapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func newHintTestAdapter(t *testing.T) *HTTPAdapter {
	t.Helper()
	a, err := NewHTTPAdapter(Options{
		Facade:  stubFacadeOK{},
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
		Config: HTTPTransportConfig{
			Enabled:                true,
			SchemaVersion:          SchemaVersionV1,
			BodySizeMaxBytes:       1 << 20,
			ConversationTTL:        time.Hour,
			TransportHintAllowlist: append([]string{}, AllowedTransportHints...),
			RequiredScope:          "assistant:turn",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	return a
}

type stubFacadeOK struct{}

func (stubFacadeOK) Handle(context.Context, contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{Status: contracts.StatusThinking}, nil
}

// TestTransportHintIsClosedVocabularyAndTelemetryOnly — SCN-069-A09.
func TestTransportHintIsClosedVocabularyAndTelemetryOnly(t *testing.T) {
	adapter := newHintTestAdapter(t)

	t.Run("every_allowed_hint_is_accepted_and_lands_only_in_TransportMetadata", func(t *testing.T) {
		for _, hint := range AllowedTransportHints {
			req := &TurnRequest{
				SchemaVersion:      SchemaVersionV1,
				TransportMessageID: "hint-" + hint,
				Kind:               string(contracts.KindText),
				Text:               "weather in barcelona",
				TransportHint:      hint,
			}
			msg, err := adapter.Translate(context.Background(), &translatePayload{
				UserID:  "u-hint",
				Request: req,
			})
			if err != nil {
				t.Fatalf("hint=%q: Translate err=%v (allowed hint must be accepted)", hint, err)
			}
			if msg.Transport != TransportName {
				t.Errorf("hint=%q: msg.Transport=%q, want %q (hint must NOT mutate Transport)", hint, msg.Transport, TransportName)
			}
			if msg.Kind != contracts.KindText {
				t.Errorf("hint=%q: msg.Kind=%q, want %q (hint must NOT mutate Kind)", hint, msg.Kind, contracts.KindText)
			}
			if msg.Text != req.Text {
				t.Errorf("hint=%q: msg.Text=%q, want %q (hint must NOT mutate Text)", hint, msg.Text, req.Text)
			}
			gotHint, ok := msg.TransportMetadata["transport_hint"]
			if !ok || gotHint != hint {
				t.Errorf("hint=%q: TransportMetadata[transport_hint]=%q ok=%v, want %q true (hint must land in telemetry-only metadata)", hint, gotHint, ok, hint)
			}
		}
	})

	t.Run("empty_hint_is_accepted_and_TransportMetadata_remains_empty", func(t *testing.T) {
		msg, err := adapter.Translate(context.Background(), &translatePayload{
			UserID: "u-empty",
			Request: &TurnRequest{
				SchemaVersion:      SchemaVersionV1,
				TransportMessageID: "hint-empty",
				Kind:               string(contracts.KindText),
				Text:               "hello",
				TransportHint:      "",
			},
		})
		if err != nil {
			t.Fatalf("Translate err=%v (empty hint must be accepted)", err)
		}
		if _, ok := msg.TransportMetadata["transport_hint"]; ok {
			t.Errorf("empty hint produced TransportMetadata[transport_hint]; want absent")
		}
	})

	t.Run("unknown_hint_is_rejected_before_facade_with_named_error", func(t *testing.T) {
		req := &TurnRequest{
			SchemaVersion:      SchemaVersionV1,
			TransportMessageID: "hint-bogus",
			Kind:               string(contracts.KindText),
			Text:               "hello",
			TransportHint:      "carrier-pigeon",
		}
		_, err := adapter.Translate(context.Background(), &translatePayload{
			UserID:  "u-bogus",
			Request: req,
		})
		if err == nil {
			t.Fatal("Translate returned nil error for unknown hint; want rejection before facade")
		}
		if !strings.Contains(err.Error(), "transport_hint") || !strings.Contains(err.Error(), "carrier-pigeon") {
			t.Errorf("error %q does not name the offending hint token; want stable message naming 'transport_hint' and 'carrier-pigeon'", err.Error())
		}
		if !strings.Contains(err.Error(), "allowlist") {
			t.Errorf("error %q does not mention the closed-vocabulary allowlist", err.Error())
		}
	})

	t.Run("adversarial_hint_does_not_leak_into_Text_or_Kind_via_any_path", func(t *testing.T) {
		// Adversarial: if a future refactor accidentally concatenated the
		// hint into Text or coerced Kind based on hint, this would catch it.
		req := &TurnRequest{
			SchemaVersion:      SchemaVersionV1,
			TransportMessageID: "hint-adv",
			Kind:               string(contracts.KindText),
			Text:               "ORIGINAL_TEXT_TOKEN",
			TransportHint:      "mobile",
		}
		msg, err := adapter.Translate(context.Background(), &translatePayload{
			UserID:  "u-adv",
			Request: req,
		})
		if err != nil {
			t.Fatalf("Translate err=%v", err)
		}
		if strings.Contains(msg.Text, "mobile") {
			t.Errorf("msg.Text=%q leaked hint token; want %q verbatim", msg.Text, "ORIGINAL_TEXT_TOKEN")
		}
		if string(msg.Kind) == "mobile" {
			t.Errorf("msg.Kind=%q coerced from hint; want %q", msg.Kind, contracts.KindText)
		}
	})
}
