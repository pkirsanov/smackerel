// Spec 061 SCOPE-04 — test-support helpers exported for out-of-package
// stress and integration test consumers (tests/stress, tests/e2e, etc.).
//
// This file is part of the production package on purpose — Go's
// internal_test.go pattern only exposes helpers to tests *within the
// same package*, but the spec 061 stress test lives under
// tests/stress/ to keep `./smackerel.sh test stress` discoverable.
// The helpers here are documented as test-only and intentionally
// surfaced as the minimum surface required to construct a Facade from
// outside the package.
//
// Do NOT call these from production callers. Production code must use
// LoadSkillsManifest against the on-disk sibling YAML.

package assistant

import (
	"context"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

// ManifestEntryForTest mirrors the unexported manifestEntry struct
// one-to-one so test consumers in other packages can stage a manifest
// without going through the YAML loader.
//
// FOR TESTS ONLY.
type ManifestEntryForTest struct {
	UserFacingLabel    string
	SlashShortcut      string
	RequiresProvenance bool
	ConfirmRequired    bool
	EnableSSTKey       string
	Enabled            bool
}

// NewManifestForTest builds a SkillsManifest from an inline map. The
// supplied entries are stored verbatim — no validation is performed
// (callers in tests are expected to know what they are constructing).
//
// FOR TESTS ONLY.
func NewManifestForTest(entries map[string]ManifestEntryForTest) (*SkillsManifest, error) {
	out := &SkillsManifest{entries: make(map[string]manifestEntry, len(entries))}
	for id, e := range entries {
		out.entries[id] = manifestEntry{
			UserFacingLabel:    e.UserFacingLabel,
			SlashShortcut:      e.SlashShortcut,
			RequiresProvenance: e.RequiresProvenance,
			ConfirmRequired:    e.ConfirmRequired,
			EnableSSTKey:       e.EnableSSTKey,
			Enabled:            e.Enabled,
		}
	}
	return out, nil
}

// --- Spec 061 SCOPE-10 — exported FOR-TESTS-ONLY harness primitives ---
//
// The offline evaluation harness under tests/eval/assistant/ needs to
// build a Facade end-to-end. These exported types mirror the
// in-package helpers in facade_test_helpers_test.go. They are
// intentionally minimal: just enough surface for the harness to drive
// the capability layer against a corpus.

// InMemoryContextStore is an in-memory implementation of
// assistantctx.Store keyed by (user_id, transport). FOR TESTS ONLY.
type InMemoryContextStore struct {
	mu   sync.Mutex
	rows map[string]assistantctx.Conversation
}

// NewInMemoryContextStore returns a freshly-initialized
// InMemoryContextStore. FOR TESTS ONLY.
func NewInMemoryContextStore() *InMemoryContextStore {
	return &InMemoryContextStore{rows: map[string]assistantctx.Conversation{}}
}

func (m *InMemoryContextStore) key(userID, transport string) string {
	return userID + "|" + transport
}

// Load implements assistantctx.Store.
func (m *InMemoryContextStore) Load(_ context.Context, userID, transport string) (assistantctx.Conversation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conv, ok := m.rows[m.key(userID, transport)]
	if !ok {
		return assistantctx.Conversation{UserID: userID, Transport: transport}, false, nil
	}
	return conv, true, nil
}

// Persist implements assistantctx.Store.
func (m *InMemoryContextStore) Persist(_ context.Context, conv assistantctx.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(conv.UserID, conv.Transport)] = conv
	return nil
}

// DeleteByKey implements assistantctx.Store.
func (m *InMemoryContextStore) DeleteByKey(_ context.Context, userID, transport string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(userID, transport))
	return nil
}

// SweepIdle implements assistantctx.Store. Always returns 0 (test
// fixture does not expire rows).
func (m *InMemoryContextStore) SweepIdle(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// StubExecutor is a ScenarioExecutor whose behaviour can be driven by
// a per-test Run closure. FOR TESTS ONLY.
type StubExecutor struct {
	mu          sync.Mutex
	RunFunc     func(ctx context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult
	Invocations int
}

// NewStubExecutor returns a freshly-initialized StubExecutor whose
// default Run returns an OK result with a literal "ok" final body.
// FOR TESTS ONLY.
func NewStubExecutor() *StubExecutor { return &StubExecutor{} }

// Run implements ScenarioExecutor.
func (s *StubExecutor) Run(ctx context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
	s.mu.Lock()
	s.Invocations++
	fn := s.RunFunc
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, sc, env)
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

// MapRegistry is a ScenarioRegistry backed by a map. FOR TESTS ONLY.
type MapRegistry struct {
	Scenarios map[string]*agent.Scenario
}

// NewMapRegistry returns a MapRegistry seeded with the provided
// scenarios. FOR TESTS ONLY.
func NewMapRegistry(scenarios map[string]*agent.Scenario) *MapRegistry {
	return &MapRegistry{Scenarios: scenarios}
}

// Scenario implements ScenarioRegistry.
func (m *MapRegistry) Scenario(id string) (*agent.Scenario, bool) {
	sc, ok := m.Scenarios[id]
	return sc, ok
}

// StubRouter is an agent.Router whose Route always returns a
// pre-configured (Scenario, RoutingDecision, ok) tuple. FOR TESTS ONLY.
type StubRouter struct {
	Chosen   *agent.Scenario
	Decision agent.RoutingDecision
	OK       bool
}

// NewStubRouter returns a fresh StubRouter. FOR TESTS ONLY.
func NewStubRouter() *StubRouter { return &StubRouter{} }

// Route implements agent.Router.
func (s *StubRouter) Route(_ context.Context, _ agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	return s.Chosen, s.Decision, s.OK
}

// RecordingAudit is an AuditWriter that records every turn into a
// slice for later inspection. FOR TESTS ONLY.
type RecordingAudit struct {
	mu    sync.Mutex
	turns []AuditTurn
}

// NewRecordingAudit returns a fresh RecordingAudit. FOR TESTS ONLY.
func NewRecordingAudit() *RecordingAudit { return &RecordingAudit{} }

// Write implements AuditWriter.
func (r *RecordingAudit) Write(_ context.Context, turn AuditTurn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.turns = append(r.turns, turn)
	return nil
}

// Snapshot returns a copy of every recorded turn.
func (r *RecordingAudit) Snapshot() []AuditTurn {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditTurn, len(r.turns))
	copy(out, r.turns)
	return out
}
