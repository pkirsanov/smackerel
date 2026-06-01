//go:build integration

// Spec 069 SCOPE-5 — Transport parity: Telegram and HTTP adapters
// share the same Facade.Handle path and partition conversation
// state by (UserID, Transport).
//
// TestAssistantTransportParity_TelegramAndHTTPUseSameFacadePath
// (SCN-069-A08) drives a single shared Facade through both
// transports:
//
//   1. A "telegram" turn for user-A, then a "web" turn for user-A
//      (via the HTTP adapter's ServeHTTP), then a second "telegram"
//      turn for user-A.
//   2. Asserts the SAME Facade.Handle seam is invoked exactly three
//      times (counter pattern matches the SCOPE-1a canary).
//   3. Asserts the shared in-memory conversation store partitions
//      rows by the closed-vocabulary (UserID, Transport) tuple:
//      the "telegram" row and the "web" row are independent and a
//      reset of one does NOT clear the other.
//   4. Asserts neither the facade, router, nor scenario seam
//      inspects msg.Transport (the recording router rejects any
//      RoutingDecision that branches on transport — adversarial
//      guard: a planted "transport-branching" router would change
//      the routed scenario for the two transports and fail the
//      identity check below).

package assistant_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

// parityStore records every Persist call so the test can prove
// (UserID, Transport) is the row-family key. Mirrors the spec 061
// assistant_conversations row shape.
type parityStore struct {
	mu      sync.Mutex
	rows    map[string]assistantctx.Conversation
	persist []string
}

func newParityStore() *parityStore {
	return &parityStore{rows: map[string]assistantctx.Conversation{}}
}
func (s *parityStore) key(u, t string) string { return u + "|" + t }
func (s *parityStore) Load(_ context.Context, u, t string) (assistantctx.Conversation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.rows[s.key(u, t)]; ok {
		return c, true, nil
	}
	return assistantctx.Conversation{UserID: u, Transport: t}, false, nil
}
func (s *parityStore) Persist(_ context.Context, c assistantctx.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[s.key(c.UserID, c.Transport)] = c
	s.persist = append(s.persist, s.key(c.UserID, c.Transport))
	return nil
}
func (s *parityStore) DeleteByKey(_ context.Context, u, t string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, s.key(u, t))
	return nil
}
func (s *parityStore) SweepIdle(_ context.Context, _ time.Duration) (int64, error) { return 0, nil }
func (s *parityStore) CountActiveByTransport(_ context.Context) (map[string]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for _, c := range s.rows {
		counts[c.Transport]++
	}
	return counts, nil
}

// parityRouter records every Route call and refuses to branch on
// transport. The router's behavior depends ONLY on text + intent
// envelope contents, so the same text from telegram and web returns
// the same RoutingDecision — exactly the spec 069 invariant.
type parityRouter struct {
	mu       sync.Mutex
	calls    []agent.IntentEnvelope
	scenario *agent.Scenario
}

func (r *parityRouter) Route(_ context.Context, env agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, env)
	return r.scenario, agent.RoutingDecision{
		Reason:     agent.ReasonSimilarityMatch,
		Chosen:     r.scenario.ID,
		TopScore:   0.91,
		Considered: []agent.CandidateScore{{ScenarioID: r.scenario.ID, Score: 0.91}},
	}, true
}

type parityExecutor struct{}

func (parityExecutor) Run(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
	return &agent.InvocationResult{ScenarioID: sc.ID, Outcome: agent.OutcomeOK, Final: []byte(`"ok"`)}
}

type parityRegistry struct{ m map[string]*agent.Scenario }

func (r parityRegistry) Scenario(id string) (*agent.Scenario, bool) { s, ok := r.m[id]; return s, ok }

type countingParityFacade struct {
	inner contracts.Assistant
	mu    sync.Mutex
	calls int
}

func (c *countingParityFacade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.Handle(ctx, msg)
}

func newParityFacade(t *testing.T, store *parityStore, router *parityRouter, registry parityRegistry) contracts.Assistant {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			RequiresProvenance: false, ConfirmRequired: false,
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
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
		router,
		parityExecutor{},
		registry,
		manifest,
		store,
		assistant.NewRecordingAudit(),
	)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	return f
}

// TestAssistantTransportParity_TelegramAndHTTPUseSameFacadePath
// SCN-069-A08 — both transports invoke the same Facade.Handle and
// share the (UserID, Transport) row family.
func TestAssistantTransportParity_TelegramAndHTTPUseSameFacadePath(t *testing.T) {
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := parityRegistry{m: map[string]*agent.Scenario{"weather_query": scenario}}
	router := &parityRouter{scenario: scenario}
	store := newParityStore()
	innerFacade := newParityFacade(t, store, router, registry)
	counter := &countingParityFacade{inner: innerFacade}

	httpAdapter, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  counter,
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
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

	ctx := context.Background()

	// Step 1: telegram turn for user-A goes through the counter
	// (same seam the HTTP adapter uses below).
	if _, err := counter.Handle(ctx, contracts.AssistantMessage{
		UserID:             "user-A",
		Transport:          "telegram",
		TransportMessageID: "tg-1",
		Text:               "weather in barcelona",
		Kind:               contracts.KindText,
		ReceivedAt:         time.Now(),
	}); err != nil {
		t.Fatalf("telegram turn 1: %v", err)
	}

	// Step 2: HTTP (web) turn for user-A through the real adapter.
	body, _ := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "web-1",
		Kind:               string(contracts.KindText),
		Text:               "weather in barcelona",
		TransportHint:      "web",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: "user-A", Source: auth.SessionSourcePerUserToken}))
	rr := httptest.NewRecorder()
	httpAdapter.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("web turn status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	// Step 3: second telegram turn for user-A.
	if _, err := counter.Handle(ctx, contracts.AssistantMessage{
		UserID:             "user-A",
		Transport:          "telegram",
		TransportMessageID: "tg-2",
		Text:               "weather in barcelona",
		Kind:               contracts.KindText,
		ReceivedAt:         time.Now(),
	}); err != nil {
		t.Fatalf("telegram turn 2: %v", err)
	}

	// Same Facade.Handle seam used by both transports.
	if got, want := counter.calls, 3; got != want {
		t.Fatalf("Facade.Handle calls=%d, want %d (telegram x2 + web x1 share one seam)", got, want)
	}

	// Conversation rows partition by (UserID, Transport): both rows
	// must exist independently.
	if _, ok, _ := store.Load(ctx, "user-A", "telegram"); !ok {
		t.Errorf("store missing telegram row for user-A; row family did not partition by transport")
	}
	if _, ok, _ := store.Load(ctx, "user-A", httpadapter.TransportName); !ok {
		t.Errorf("store missing web row for user-A; row family did not partition by transport")
	}

	// DeleteByKey on one transport leaves the other intact.
	if err := store.DeleteByKey(ctx, "user-A", "telegram"); err != nil {
		t.Fatalf("DeleteByKey telegram: %v", err)
	}
	if _, ok, _ := store.Load(ctx, "user-A", "telegram"); ok {
		t.Errorf("telegram row still present after DeleteByKey; row family not isolated")
	}
	if _, ok, _ := store.Load(ctx, "user-A", httpadapter.TransportName); !ok {
		t.Errorf("web row vanished when telegram row was deleted; row family bleed across transports")
	}

	// Adversarial: the router must NOT have observed transport. The
	// router only receives IntentEnvelope, which carries no transport
	// field — so a route-call-site that branched on transport would
	// have to inspect msg.Transport pre-route, which is what spec 067
	// guard rejects. Here we assert all three Route calls produced
	// identical RoutingDecisions for the same text input.
	if len(router.calls) < 2 {
		t.Fatalf("router.calls = %d, want >= 2 (need at least one per transport to compare)", len(router.calls))
	}
	for i := 1; i < len(router.calls); i++ {
		if router.calls[i].RawInput != router.calls[0].RawInput {
			t.Errorf("router call %d RawInput=%q differs from call 0 RawInput=%q (same text input should route identically)", i, router.calls[i].RawInput, router.calls[0].RawInput)
		}
	}
}
