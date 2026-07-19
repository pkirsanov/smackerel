package httpadapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func defaultConfig() HTTPTransportConfig {
	return HTTPTransportConfig{
		Enabled:                   true,
		SchemaVersion:             SchemaVersionV1,
		BodySizeMaxBytes:          1 << 20,
		RateLimitPerUserPerMinute: 60,
		CORSAllowedOrigins:        []string{"https://example.test"},
		ConversationTTL:           24 * time.Hour,
		TransportHintAllowlist:    []string{"web", "mobile", "bridge"},
		RequiredScope:             "assistant:turn",
	}
}

type stubFacade struct {
	calls    int
	lastMsg  contracts.AssistantMessage
	response contracts.AssistantResponse
}

func (s *stubFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	s.calls++
	s.lastMsg = msg
	return s.response, nil
}

// TestHTTPAdapterTranslatesTextTurnToAssistantMessage proves SCN-069-A01
// translation: a wire text turn becomes an AssistantMessage with
// Transport=web and the request id echoed verbatim.
func TestHTTPAdapterTranslatesTextTurnToAssistantMessage(t *testing.T) {
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	adapter, err := NewHTTPAdapter(Options{
		Facade:  &stubFacade{},
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return now },
		Config:  defaultConfig(),
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}

	req := &TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: "test-turn-001",
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "weather in palm springs ca tomorrow",
	}
	msg, err := adapter.Translate(context.Background(), &translatePayload{UserID: "u-1", Request: req})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if msg.Transport != TransportName {
		t.Errorf("Transport = %q, want %q", msg.Transport, TransportName)
	}
	if msg.UserID != "u-1" {
		t.Errorf("UserID = %q, want u-1", msg.UserID)
	}
	if msg.TransportMessageID != "test-turn-001" {
		t.Errorf("TransportMessageID = %q, want test-turn-001", msg.TransportMessageID)
	}
	if msg.Kind != contracts.KindText {
		t.Errorf("Kind = %q, want text", msg.Kind)
	}
	if got := msg.TransportMetadata["transport_hint"]; got != "web" {
		t.Errorf("TransportMetadata.transport_hint = %q, want web", got)
	}
	if !msg.ReceivedAt.Equal(now) {
		t.Errorf("ReceivedAt = %v, want %v", msg.ReceivedAt, now)
	}
}

func TestNewHTTPAdapterRejectsMissingConversationTTL(t *testing.T) {
	cfg := defaultConfig()
	cfg.ConversationTTL = 0
	_, err := NewHTTPAdapter(Options{
		Facade:  &stubFacade{},
		Capture: func(context.Context, string, string, string) {},
		Clock:   time.Now,
		Config:  cfg,
	})
	if err == nil || !strings.Contains(err.Error(), "ConversationTTL") {
		t.Fatalf("NewHTTPAdapter error=%v, want fail-loud ConversationTTL error", err)
	}
}

// TestHTTPAdapter_ValidateRejectsUnknownHint proves
// transport_hint is enforced by the wire-schema allowlist before
// facade invocation.
func TestHTTPAdapter_ValidateRejectsUnknownHint(t *testing.T) {
	cfg := defaultConfig()
	req := &TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: "id-1",
		Kind:               string(contracts.KindText),
		Text:               "hello",
		TransportHint:      "carrier-pigeon",
	}
	if err := req.Validate(cfg); err == nil {
		t.Fatal("Validate accepted unknown transport_hint; want rejection")
	}
}

// TestHTTPAdapter_ValidateRejectsBadSchemaVersion proves
// schema_version drift fails closed.
func TestHTTPAdapter_ValidateRejectsBadSchemaVersion(t *testing.T) {
	req := &TurnRequest{
		SchemaVersion:      "v0",
		TransportMessageID: "id-1",
		Kind:               string(contracts.KindText),
		Text:               "hello",
	}
	if err := req.Validate(defaultConfig()); err == nil {
		t.Fatal("Validate accepted schema_version=v0; want rejection")
	}
}

// TestHTTPAdapter_ValidateConfirmKindRequiresRefAndChoice
// proves callback kinds enforce their required fields.
func TestHTTPAdapter_ValidateConfirmKindRequiresRefAndChoice(t *testing.T) {
	cfg := defaultConfig()
	cases := []*TurnRequest{
		{SchemaVersion: SchemaVersionV1, TransportMessageID: "id-1", Kind: string(contracts.KindConfirm)},
		{SchemaVersion: SchemaVersionV1, TransportMessageID: "id-1", Kind: string(contracts.KindConfirm), ConfirmRef: "cr-1"},
		{SchemaVersion: SchemaVersionV1, TransportMessageID: "id-1", Kind: string(contracts.KindDisambiguation)},
	}
	for i, req := range cases {
		if err := req.Validate(cfg); err == nil {
			t.Errorf("case %d: Validate accepted incomplete callback turn; want rejection", i)
		}
	}
}
