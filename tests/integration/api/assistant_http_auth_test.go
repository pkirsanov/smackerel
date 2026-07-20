//go:build integration

// Spec 069 SCOPE-2 — TP-069-04 auth and scope rejections.
//
// TestAssistantHTTPAuth_MissingBearerReturns401BeforeFacade — SCN-069-A02.
// TestAssistantHTTPAuth_MissingTurnScopeReturns403BeforeFacade — SCN-069-A02.
//
// These tests drive an in-process chi router whose middleware
// stack mirrors the production wiring at the assistant.turn route:
//
//   bearer-gate (synthetic) -> PreFacadeChain (scope -> rate ->
//   body cap) -> HTTPAdapter -> countingFacade.
//
// The bearer-gate is a minimal stand-in for internal/api.bearerAuthMiddleware
// that emits 401 when the Authorization header is missing/invalid
// and otherwise injects a per-user PASETO Session into the context
// — exactly the shape RequireScope expects. The full production
// bearer middleware is exercised separately under internal/api;
// re-coupling it here would multiply the test surface without
// covering the scope-2 layer SCOPE-2 owns.

package api_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

const (
	scope2TestToken    = "test-bearer-token-scope2"
	scope2RequireScope = "assistant.turn"
)

// scope2Facade is a minimal Assistant that records invocations so
// tests can assert facade was NOT called on rejection paths.
type scope2Facade struct {
	calls int
	inner contracts.Assistant
}

func (f *scope2Facade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls++
	return f.inner.Handle(ctx, msg)
}

func newScope2InnerFacade(t *testing.T) contracts.Assistant {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
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

// syntheticBearerGate emulates the relevant 401 + session-injection
// behavior of internal/api.bearerAuthMiddleware. If sessionScopes is
// non-nil, the injected session is per-user PASETO with those
// scopes; passing nil scopes models the missing-scope case.
func syntheticBearerGate(sessionScopes []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if authz == "" || !strings.HasPrefix(authz, "Bearer ") {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"UNAUTHORIZED"}`))
				return
			}
			token := strings.TrimPrefix(authz, "Bearer ")
			if token != scope2TestToken {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"UNAUTHORIZED"}`))
				return
			}
			ctx := auth.WithSession(r.Context(), auth.Session{
				UserID: "scope2-user-1",
				Source: auth.SessionSourcePerUserToken,
				Scopes: sessionScopes,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func mountScope2Route(t *testing.T, facade contracts.Assistant, cfg httpadapter.HTTPTransportConfig, gate func(http.Handler) http.Handler) http.Handler {
	t.Helper()
	adapter, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   time.Now,
		Config:  cfg,
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(gate)
		r.Use(httpadapter.PreFacadeChain(cfg))
		r.Method(http.MethodPost, "/api/assistant/turn", adapter)
	})
	return r
}

func validTurnBody(t *testing.T, id string) []byte {
	t.Helper()
	b, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: id,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "ping",
	})
	if err != nil {
		t.Fatalf("marshal turn: %v", err)
	}
	return b
}

func defaultScope2Config() httpadapter.HTTPTransportConfig {
	return httpadapter.HTTPTransportConfig{
		Enabled:                   true,
		SchemaVersion:             httpadapter.SchemaVersionV1,
		BodySizeMaxBytes:          65536,
		RateLimitPerUserPerMinute: 60,
		ConversationTTL:           time.Hour,
		TransportHintAllowlist:    []string{"web", "mobile", "bridge"},
		RequiredScope:             scope2RequireScope,
	}
}

// TestAssistantHTTPAuth_MissingBearerReturns401BeforeFacade — SCN-069-A02.
func TestAssistantHTTPAuth_MissingBearerReturns401BeforeFacade(t *testing.T) {
	facade := &scope2Facade{inner: newScope2InnerFacade(t)}
	router := mountScope2Route(t, facade, defaultScope2Config(), syntheticBearerGate([]string{scope2RequireScope}))

	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, "missing-bearer-1")))
	// no Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
	if facade.calls != 0 {
		t.Errorf("facade invoked %d times on missing-bearer; want 0", facade.calls)
	}
}

// TestAssistantHTTPAuth_MissingTurnScopeReturns403BeforeFacade — SCN-069-A02.
func TestAssistantHTTPAuth_MissingTurnScopeReturns403BeforeFacade(t *testing.T) {
	facade := &scope2Facade{inner: newScope2InnerFacade(t)}
	// Session present but does NOT carry assistant.turn scope.
	gate := syntheticBearerGate([]string{"unrelated.read"})
	router := mountScope2Route(t, facade, defaultScope2Config(), gate)

	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, "missing-scope-1")))
	req.Header.Set("Authorization", "Bearer "+scope2TestToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if facade.calls != 0 {
		t.Errorf("facade invoked %d times on missing-scope; want 0", facade.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 403 body: %v; raw=%s", err, rr.Body.String())
	}
	if got, _ := body["error"].(string); got != "scope_required" {
		t.Errorf(`403 body.error = %q, want "scope_required"; raw=%s`, got, rr.Body.String())
	}
}

// TestAssistantHTTPAuth_AcceptsBearerWithRequiredScope is the
// adversarial counterpart: prove the chain DOES forward a valid
// authenticated + scoped request to the facade. Without this, the
// rejection tests above could pass against a chain that always 401s
// or always 403s.
func TestAssistantHTTPAuth_AcceptsBearerWithRequiredScope(t *testing.T) {
	facade := &scope2Facade{inner: newScope2InnerFacade(t)}
	router := mountScope2Route(t, facade, defaultScope2Config(), syntheticBearerGate([]string{scope2RequireScope}))

	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, "happy-1")))
	req.Header.Set("Authorization", "Bearer "+scope2TestToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if facade.calls != 1 {
		t.Errorf("facade calls = %d, want 1", facade.calls)
	}
}
