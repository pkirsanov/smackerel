// Spec 061 SCOPE-04 — shared facade test helpers.
//
// Provides in-memory implementations of every Facade dependency so the
// four facade tests in this directory can exercise the capability
// machine without PG, without a real router, and without spec 037
// embedding plumbing. The fakeTransportAdapter here proves the BS-005
// invariant: NO transport-keyed code path in the facade.

package assistant

import (
	"context"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// --- in-memory context store ---

type memContextStore struct {
	mu   sync.Mutex
	rows map[string]assistantctx.Conversation
}

func newMemContextStore() *memContextStore {
	return &memContextStore{rows: map[string]assistantctx.Conversation{}}
}

func (m *memContextStore) key(userID, transport string) string {
	return userID + "|" + transport
}

func (m *memContextStore) Load(_ context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conv, ok := m.rows[m.key(userID, transport)]
	if !ok {
		return assistantctx.Conversation{UserID: userID, Transport: transport}, false, nil
	}
	return conv, true, nil
}

func (m *memContextStore) Persist(_ context.Context, conv assistantctx.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(conv.UserID, conv.Transport)] = conv
	return nil
}

func (m *memContextStore) DeleteByKey(_ context.Context, userID, transport string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(userID, transport))
	return nil
}

func (m *memContextStore) SweepIdle(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// --- stub scenario executor + registry ---

type stubExecutor struct {
	// run is called for every Run; defaults to OK with a literal final.
	run func(ctx context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult
	// invocations records every call for assertions.
	invocations int
}

func (s *stubExecutor) Run(ctx context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
	s.invocations++
	if s.run != nil {
		return s.run(ctx, sc, env)
	}
	return &agent.InvocationResult{
		TraceID:    "test-trace",
		ScenarioID: sc.ID,
		Outcome:    agent.OutcomeOK,
		Final:      []byte(`"ok"`),
		StartedAt:  time.Unix(0, 0),
		EndedAt:    time.Unix(0, 0),
	}
}

type mapRegistry struct {
	scenarios map[string]*agent.Scenario
}

func (m mapRegistry) Scenario(id string) (*agent.Scenario, bool) {
	sc, ok := m.scenarios[id]
	return sc, ok
}

// --- stub router ---

type stubRouter struct {
	chosen   *agent.Scenario
	decision agent.RoutingDecision
	ok       bool
}

func (s *stubRouter) Route(_ context.Context, _ agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	return s.chosen, s.decision, s.ok
}

// --- recording audit writer ---

type recordingAudit struct {
	mu    sync.Mutex
	turns []AuditTurn
}

func (r *recordingAudit) Write(_ context.Context, turn AuditTurn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.turns = append(r.turns, turn)
	return nil
}

func (r *recordingAudit) snapshot() []AuditTurn {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditTurn, len(r.turns))
	copy(out, r.turns)
	return out
}

// --- skills manifest helper ---

// newTestManifest builds a SkillsManifest from a small inline map so
// facade tests can opt scenarios in/out of the enable + provenance gates
// without disk I/O. The manifest format matches LoadSkillsManifest's
// internal map shape.
func newTestManifest(entries map[string]manifestEntry) *SkillsManifest {
	m := &SkillsManifest{entries: map[string]manifestEntry{}}
	for id, e := range entries {
		m.entries[id] = e
	}
	return m
}

// --- BS-005 invariant: fake transport adapter ---
//
// fakeTransportAdapter panics on EVERY method except Name(). The
// invariant test wires it into a Facade and runs Handle; any reach
// into the adapter from the facade panics the test.

type fakeTransportAdapter struct {
	name string
}

func (a *fakeTransportAdapter) Name() string { return a.name }

func (a *fakeTransportAdapter) Translate(context.Context, contracts.TransportPayload) (contracts.AssistantMessage, error) {
	panic("BS-005 violation: facade called Translate")
}
func (a *fakeTransportAdapter) Render(context.Context, contracts.TransportIdentity, contracts.AssistantResponse) error {
	panic("BS-005 violation: facade called Render")
}
func (a *fakeTransportAdapter) Identity(context.Context, contracts.TransportPayload) (contracts.TransportIdentity, error) {
	panic("BS-005 violation: facade called Identity")
}
func (a *fakeTransportAdapter) Start(context.Context, contracts.Assistant) error {
	panic("BS-005 violation: facade called Start")
}
func (a *fakeTransportAdapter) Stop(context.Context) error {
	panic("BS-005 violation: facade called Stop")
}

// --- default facade builder ---

// fixedClock returns a deterministic Now closure.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// defaultFacadeConfig returns a minimal valid FacadeConfig.
func defaultFacadeConfig(now time.Time) FacadeConfig {
	return FacadeConfig{
		BorderlineFloor:      0.75,
		AgentConfidenceFloor: 0.50,
		SourcesMax:           5,
		BodyMaxChars:         1000,
		WindowTurns:          5,
		DisambigMaxChoices:   3,
		DisambigTimeout:      30 * time.Second,
		Now:                  fixedClock(now),
	}
}

// mustFacade builds a Facade from the supplied dependencies and panics
// on any constructor error.
func mustFacade(
	cfg FacadeConfig,
	router agent.Router,
	executor ScenarioExecutor,
	registry ScenarioRegistry,
	manifest *SkillsManifest,
	store assistantctx.Store,
	audit AuditWriter,
) *Facade {
	f, err := NewFacade(cfg, router, executor, registry, manifest, store, audit)
	if err != nil {
		panic("mustFacade: " + err.Error())
	}
	return f
}
