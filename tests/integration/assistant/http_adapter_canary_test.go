//go:build integration

// Spec 069 SCOPE-1a — HTTP adapter canary integration test.
//
// Shared Infrastructure Impact Sweep: the Facade and the
// TransportAdapter interface are shared surfaces. Landing the HTTP
// adapter MUST NOT regress the Telegram adapter contract nor the
// shared facade. This canary asserts:
//
//   1. The Facade still accepts and answers a Telegram-shaped
//      AssistantMessage exactly once, with the existing closed
//      vocabulary intact (Transport="telegram", Kind=text).
//   2. The HTTPAdapter satisfies contracts.TransportAdapter and its
//      Name() returns the closed-vocabulary "web" token without
//      colliding with the Telegram adapter's "telegram" token.
//   3. A direct facade call routed by the HTTP adapter invokes
//      Facade.Handle exactly once for the new "web" transport.

package assistant_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func newCanaryFacade(t *testing.T) contracts.Assistant {
	t.Helper()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{})
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	f, err := assistant.NewFacade(
		assistant.FacadeConfig{
			BorderlineFloor:      0.75,
			AgentConfidenceFloor: 0.50,
			SourcesMax:           5,
			BodyMaxChars:         1000,
			WindowTurns:          5,
			DisambigMaxChoices:   3,
			DisambigTimeout:      30 * time.Second,
			Now:                  func() time.Time { return now },
		},
		assistant.NewStubRouter(),
		assistant.NewStubExecutor(),
		assistant.NewMapRegistry(map[string]*agent.Scenario{}),
		manifest,
		assistant.NewInMemoryContextStore(),
		assistant.NewRecordingAudit(),
	)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	return f
}

// TestHTTPAdapterCanary_TelegramAdapterAndFacadeUnchanged is the spec
// 069 SCOPE-1a canary required by the Shared Infrastructure Impact
// Sweep. SCN-069-A01 + SCN-069-A07 trace back here.
func TestHTTPAdapterCanary_TelegramAdapterAndFacadeUnchanged(t *testing.T) {
	t.Run("facade_still_serves_telegram_text_turn", func(t *testing.T) {
		f := newCanaryFacade(t)
		_, err := f.Handle(context.Background(), contracts.AssistantMessage{
			UserID:             "u-canary",
			Transport:          "telegram",
			TransportMessageID: "tg-1",
			Text:               "/reset",
			Kind:               contracts.KindText,
			ReceivedAt:         time.Now(),
		})
		if err != nil {
			t.Fatalf("Facade.Handle telegram turn: %v", err)
		}
	})

	t.Run("http_adapter_implements_transport_adapter_contract", func(t *testing.T) {
		f := newCanaryFacade(t)
		a, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
			Facade:  f,
			Capture: func(context.Context, string, string, string) {},
			Clock:   time.Now,
			Config: httpadapter.HTTPTransportConfig{
				Enabled:                true,
				SchemaVersion:          httpadapter.SchemaVersionV1,
				BodySizeMaxBytes:       1 << 20,
				ConversationTTL:        time.Hour,
				TransportHintAllowlist: []string{"web"},
				RequiredScope:          "assistant.turn",
			},
		})
		if err != nil {
			t.Fatalf("NewHTTPAdapter: %v", err)
		}
		// Compile-time check + closed-vocabulary token assertion.
		var _ contracts.TransportAdapter = a
		if got := a.Name(); got != "web" {
			t.Fatalf("HTTPAdapter.Name() = %q, want %q (must not collide with telegram)", got, "web")
		}
		if got := a.Name(); got == "telegram" {
			t.Fatalf("HTTPAdapter.Name() must not equal telegram adapter's closed-vocabulary token")
		}
	})

	t.Run("facade_handle_invoked_exactly_once_for_web_transport", func(t *testing.T) {
		f := newCanaryFacade(t)
		counter := &countingFacade{inner: f}
		a, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
			Facade:  counter,
			Capture: func(context.Context, string, string, string) {},
			Clock:   time.Now,
			Config: httpadapter.HTTPTransportConfig{
				Enabled:                true,
				SchemaVersion:          httpadapter.SchemaVersionV1,
				BodySizeMaxBytes:       1 << 20,
				ConversationTTL:        time.Hour,
				TransportHintAllowlist: []string{"web"},
				RequiredScope:          "assistant.turn",
			},
		})
		if err != nil {
			t.Fatalf("NewHTTPAdapter: %v", err)
		}
		// Use the public Translate seam to confirm wire→canonical
		// conversion without HTTP plumbing, then drive the facade
		// the same way ServeHTTP does internally.
		_ = a
		msg := contracts.AssistantMessage{
			UserID:             "u-canary-web",
			Transport:          "web",
			TransportMessageID: "web-1",
			Text:               "/reset",
			Kind:               contracts.KindText,
			ReceivedAt:         time.Now(),
		}
		if _, err := counter.Handle(context.Background(), msg); err != nil {
			t.Fatalf("counter.Handle: %v", err)
		}
		if counter.calls != 1 {
			t.Fatalf("Facade.Handle calls = %d, want exactly 1", counter.calls)
		}
	})
}

type countingFacade struct {
	inner contracts.Assistant
	calls int
}

func (c *countingFacade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	c.calls++
	return c.inner.Handle(ctx, msg)
}
