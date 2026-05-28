//go:build stress

// Spec 061 SCOPE-04 — assistant Facade overhead stress test (G026 budget).
//
// Drives 1500 mixed-band Handle calls against an in-process Facade
// with stubbed router + stubbed executor (sub-microsecond returns) so
// the latency measurement reflects ONLY the capability layer:
// reference resolution, borderline post-processor, band dispatch,
// disambiguation prompt construction, provenance gate, context
// persistence (in-memory store), and audit writer (noop).
//
// Asserts:
//   * G1: every Handle call returns nil error.
//   * G2: per-call facade overhead p95 < 5 ms (generous; design
//         predicts sub-millisecond per turn).
//   * G3: reports p50/p95/p99 in the test log so a regression
//         toward serialization is visible to the operator.
//
// Skips cleanly when STRESS_ASSISTANT_FACADE_DURATION_MS env is set
// to 0 so a contributor can dial it down for CI smoke runs.

package stress

import (
	"context"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantpkg "github.com/smackerel/smackerel/internal/assistant"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

const (
	assistantStressTurnCount   = 1500
	assistantStressWorkerCount = 32
	assistantStressP95BudgetMs = 5
)

// memStore is an in-memory Store stand-in so the stress test does not
// depend on PostgreSQL. The persistence layer is exercised elsewhere
// by the integration test in internal/assistant/context/pg_store_test.go.
type memStore struct {
	mu   sync.Mutex
	rows map[string]assistantctx.Conversation
}

func newMemStore() *memStore               { return &memStore{rows: map[string]assistantctx.Conversation{}} }
func (m *memStore) key(u, t string) string { return u + "|" + t }
func (m *memStore) Load(_ context.Context, u, t string) (assistantctx.Conversation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.rows[m.key(u, t)]; ok {
		return c, true, nil
	}
	return assistantctx.Conversation{UserID: u, Transport: t}, false, nil
}
func (m *memStore) Persist(_ context.Context, c assistantctx.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(c.UserID, c.Transport)] = c
	return nil
}
func (m *memStore) DeleteByKey(_ context.Context, u, t string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(u, t))
	return nil
}
func (m *memStore) SweepIdle(_ context.Context, _ time.Duration) (int64, error) { return 0, nil }
func (m *memStore) CountActiveByTransport(_ context.Context) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counts := map[string]int{}
	for _, c := range m.rows {
		counts[c.Transport]++
	}
	return counts, nil
}

// stubRouter rotates through three RoutingDecisions to exercise all
// three bands roughly evenly.
type stubRouter struct {
	chosen *agent.Scenario
	calls  uint64
	mu     sync.Mutex
}

func (s *stubRouter) Route(_ context.Context, _ agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	s.mu.Lock()
	n := s.calls
	s.calls++
	s.mu.Unlock()

	switch n % 3 {
	case 0: // high band
		return s.chosen, agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.91,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.91}},
		}, true
	case 1: // borderline band
		return nil, agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.62,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.62},
				{ScenarioID: "notification_schedule", Score: 0.55},
			},
		}, true
	default: // low band
		return nil, agent.RoutingDecision{Reason: agent.ReasonUnknownIntent}, false
	}
}

// stubExecutor returns a constant InvocationResult in O(1).
type stubExecutor struct{}

func (stubExecutor) Run(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
	return &agent.InvocationResult{
		ScenarioID: sc.ID, Outcome: agent.OutcomeOK, Final: []byte(`"ok"`),
	}
}

type mapRegistry struct{ m map[string]*agent.Scenario }

func (r mapRegistry) Scenario(id string) (*agent.Scenario, bool) {
	sc, ok := r.m[id]
	return sc, ok
}

func TestAssistantFacadeStressOverhead(t *testing.T) {
	if v := os.Getenv("STRESS_ASSISTANT_FACADE_TURNS"); v == "0" {
		t.Skip("stress: STRESS_ASSISTANT_FACADE_TURNS=0 — facade stress disabled")
	}
	turns := assistantStressTurnCount
	if v := os.Getenv("STRESS_ASSISTANT_FACADE_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			turns = n
		}
	}

	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{m: map[string]*agent.Scenario{"weather_query": scenario}}
	manifest, err := assistantpkg.NewManifestForTest(map[string]assistantpkg.ManifestEntryForTest{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			RequiresProvenance: false, ConfirmRequired: false,
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
		"notification_schedule": {
			UserFacingLabel: "remind me", SlashShortcut: "/remind",
			RequiresProvenance: false, ConfirmRequired: false,
			EnableSSTKey: "assistant.skill.notification_schedule.enabled", Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}

	store := newMemStore()
	audit := assistantpkg.NewNoopAuditWriter()
	router := &stubRouter{chosen: scenario}
	executor := stubExecutor{}

	cfg := assistantpkg.FacadeConfig{
		BorderlineFloor:      0.75,
		AgentConfidenceFloor: 0.50,
		SourcesMax:           5,
		BodyMaxChars:         1000,
		WindowTurns:          5,
		DisambigMaxChoices:   3,
		DisambigTimeout:      30 * time.Second,
		Now:                  time.Now,
	}
	facade, err := assistantpkg.NewFacade(cfg, router, executor, registry, manifest, store, audit)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}

	latencies := make([]time.Duration, turns)
	work := make(chan int, turns)
	for i := 0; i < turns; i++ {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	for w := 0; w < assistantStressWorkerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			for i := range work {
				start := time.Now()
				_, err := facade.Handle(ctx, contracts.AssistantMessage{
					UserID:    "stress-u-" + strconv.Itoa(workerID),
					Transport: "telegram",
					Text:      "weather in barcelona today",
					Kind:      contracts.KindText,
				})
				latencies[i] = time.Since(start)
				if err != nil {
					t.Errorf("turn %d: %v", i, err)
				}
			}
		}(w)
	}
	wg.Wait()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]

	t.Logf("Assistant Facade overhead — turns=%d workers=%d p50=%v p95=%v p99=%v max=%v",
		turns, assistantStressWorkerCount, p50, p95, p99, maxL)

	budget := time.Duration(assistantStressP95BudgetMs) * time.Millisecond
	if p95 > budget {
		t.Errorf("G026 budget breach: p95=%v exceeds budget=%v (design predicts sub-millisecond)", p95, budget)
	}
}
