//go:build integration

// Spec 069 SCOPE-1a — facade-invocation integration test.
//
// Drives the route handler in-process (no live HTTP server, no DB,
// no NATS) and asserts that an authenticated POST /api/assistant/turn
// request invokes Facade.Handle EXACTLY ONCE for a "web"-transport
// AssistantMessage, that the response is schema v1, and that
// transport_message_id round-trips verbatim. Auth/scope/limit
// rejections layer on top in SCOPE-2.

package api_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

type countingFacade struct {
	inner contracts.Assistant
	calls int
	last  contracts.AssistantMessage
}

func (c *countingFacade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	c.calls++
	c.last = msg
	return c.inner.Handle(ctx, msg)
}

func newTestFacade(t *testing.T) contracts.Assistant {
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

func newAdapter(t *testing.T, facade contracts.Assistant) *httpadapter.HTTPAdapter {
	t.Helper()
	a, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   time.Now,
		Config: httpadapter.HTTPTransportConfig{
			Enabled:                true,
			SchemaVersion:          httpadapter.SchemaVersionV1,
			BodySizeMaxBytes:       1 << 20,
			TransportHintAllowlist: []string{"web", "mobile", "bridge"},
			RequiredScope:          "assistant.turn",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	return a
}

// TestAssistantHTTPTurnInvokesFacadeExactlyOnce — SCN-069-A01.
func TestAssistantHTTPTurnInvokesFacadeExactlyOnce(t *testing.T) {
	counter := &countingFacade{inner: newTestFacade(t)}
	adapter := newAdapter(t, counter)

	body := mustJSON(t, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "test-turn-001",
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/reset",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{
		UserID: "u-int-1",
		Source: auth.SessionSourcePerUserToken,
	}))
	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp httpadapter.TurnResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, rr.Body.String())
	}
	if resp.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", resp.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if resp.Transport != "web" {
		t.Errorf("transport = %q, want web", resp.Transport)
	}
	if resp.TransportMessageID != "test-turn-001" {
		t.Errorf("transport_message_id echo = %q, want test-turn-001", resp.TransportMessageID)
	}
	if !resp.FacadeInvoked {
		t.Errorf("facade_invoked = false, want true")
	}
	if counter.calls != 1 {
		t.Errorf("Facade.Handle calls = %d, want exactly 1", counter.calls)
	}
	if counter.last.Transport != "web" {
		t.Errorf("delivered transport = %q, want web", counter.last.Transport)
	}
	if counter.last.UserID != "u-int-1" {
		t.Errorf("delivered user_id = %q, want u-int-1", counter.last.UserID)
	}
}

// TestAssistantHTTPTurnRejectsUnauthenticatedRequest proves the route
// refuses requests without an auth session and never calls the
// facade. SCOPE-2 layers full bearer/scope middleware on top.
func TestAssistantHTTPTurnRejectsUnauthenticatedRequest(t *testing.T) {
	counter := &countingFacade{inner: newTestFacade(t)}
	adapter := newAdapter(t, counter)

	body := mustJSON(t, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "anon-1",
		Kind:               string(contracts.KindText),
		Text:               "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if counter.calls != 0 {
		t.Errorf("Facade.Handle calls = %d, want 0", counter.calls)
	}
}

// TestAssistantHTTPTurnRejectsSchemaDrift proves any non-v1 wire
// payload is rejected pre-facade.
func TestAssistantHTTPTurnRejectsSchemaDrift(t *testing.T) {
	counter := &countingFacade{inner: newTestFacade(t)}
	adapter := newAdapter(t, counter)

	body := mustJSON(t, map[string]any{
		"schema_version":       "v0",
		"transport_message_id": "drift-1",
		"kind":                 "text",
		"text":                 "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{
		UserID: "u-int-2",
		Source: auth.SessionSourcePerUserToken,
	}))
	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if counter.calls != 0 {
		t.Errorf("Facade.Handle calls = %d, want 0", counter.calls)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
