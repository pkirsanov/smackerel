// BUG-064-003 — adversarial regression for the router-construction contract.
//
// The bug: tests/integration/agent/openknowledge_routing_test.go wrapped
// agent.NewRouter in a hard-coded 30-second context while NewRouter issued 79
// sequential POST /embed calls into a possibly-cold sentence-transformer. The
// verdict of a routing test therefore depended on sidecar warmth. Two recorded
// failures aborted at embed ordinals 31/79 and 13/79.
//
// These tests would all FAIL against the pre-fix tree:
//
//   - TestBUG064003_WarmupGateIsLoadBearing proves the gate CAUSES the fix.
//     The same scripted cold embedder, under the same derived budget, fails
//     without the gate and succeeds with it. A fix that merely enlarged the
//     constant, or a regression that deleted the warm-up call, flips it red.
//     It never depends on the host being warm: the "cold" cost is scripted.
//   - TestBUG064003_ZeroWarmResultIsStructurallyRefused proves the ordering is
//     enforced by the type system, not by convention.
//   - TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting proves an
//     embedder that never warms produces a readiness verdict distinguishable
//     from a routing failure (SCN-064-003-02).
//   - TestBUG064003_RoutingValuesFailLoudWithoutFallback proves an absent or
//     unparseable SST value is an error, not a silent 0.65 / 5
//     (SCN-064-003-04). Pre-fix, parseFloatEnv/parseIntEnv returned those
//     constants.
//   - TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST proves the test's
//     per-call ceiling tracks agent.routing.embed_timeout_ms rather than the
//     pre-fix 5s literal (SCN-064-003-05).
//   - TestBUG064003_RoutingTestCarriesNoWallClockLiteral is the source-shape
//     guard: it rejects reintroduction of a duration literal, of a direct
//     agent.NewRouter call, of the env-fallback helper shape, and of any
//     ordering where the build precedes the warm-up (SCN-064-003-03).
//
// This file is package agent_test (the external test package) because
// routerwarmup imports internal/agent; an internal test package would create
// an import cycle.
package agent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/tests/integration/agent/routerwarmup"
)

// scriptedEmbedder reproduces the shape of a cold ML sidecar deterministically:
// the first coldCalls embeds pay coldLatency (sentence-transformer load), every
// later embed pays warmLatency. Latency is scripted, so these assertions hold
// identically on a warm laptop and on a contended CI host — the property the
// bug report says a "raise the number" fix could never establish.
type scriptedEmbedder struct {
	mu          sync.Mutex
	calls       int
	coldCalls   int
	coldLatency time.Duration
	warmLatency time.Duration
}

func (e *scriptedEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	e.mu.Lock()
	e.calls++
	ordinal := e.calls
	e.mu.Unlock()

	delay := e.warmLatency
	if ordinal <= e.coldCalls {
		delay = e.coldLatency
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return []float32{1, 0, 0}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("scriptedEmbedder: embed #%d abandoned: %w", ordinal, ctx.Err())
	}
}

func (e *scriptedEmbedder) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// bug064003Scenarios returns n scenarios each declaring one intent example, so
// agent.NewRouter issues exactly n sequential embed calls.
func bug064003Scenarios(n int) []*agent.Scenario {
	scenarios := make([]*agent.Scenario, 0, n)
	for i := 0; i < n; i++ {
		scenarios = append(scenarios, &agent.Scenario{
			ID:             fmt.Sprintf("bug064003_scenario_%02d", i),
			IntentExamples: []string{fmt.Sprintf("example %d", i)},
		})
	}
	return scenarios
}

func bug064003RoutingConfig() agent.RoutingConfig {
	return agent.RoutingConfig{ConfidenceFloor: 0.65, ConsiderTopN: 5, FallbackScenarioID: "open_knowledge"}
}

// TestBUG064003_WarmupGateIsLoadBearing is the causal proof. Both halves use an
// identically-scripted cold embedder and the SAME derived budget; the only
// difference is whether the warm-up gate ran first.
func TestBUG064003_WarmupGateIsLoadBearing(t *testing.T) {
	scenarios := bug064003Scenarios(5)
	timings := routerwarmup.Timings{
		EmbedCallTimeout:   2 * time.Second,
		WarmTargetLatency:  20 * time.Millisecond,
		WarmupBudget:       5 * time.Second,
		BuildPerCallBudget: 40 * time.Millisecond,
	}
	if err := timings.Validate(); err != nil {
		t.Fatalf("scripted timings must satisfy the SST contract: %v", err)
	}

	budget := routerwarmup.BuildBudget(scenarios, timings)
	if want := 5 * 40 * time.Millisecond; budget != want {
		t.Fatalf("derived budget = %s; want %s (5 intent_examples × %s)", budget, want, timings.BuildPerCallBudget)
	}

	newColdEmbedder := func() *scriptedEmbedder {
		return &scriptedEmbedder{
			coldCalls:   1,
			coldLatency: 600 * time.Millisecond, // 3× the whole derived budget
			warmLatency: 2 * time.Millisecond,
		}
	}

	t.Run("without_the_gate_the_derived_budget_cannot_absorb_cold_start", func(t *testing.T) {
		embedder := newColdEmbedder()
		buildCtx, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()

		router, err := agent.NewRouter(buildCtx, bug064003RoutingConfig(), scenarios, embedder)
		if err == nil {
			t.Fatalf("agent.NewRouter succeeded (router=%v) on a cold embedder whose first call costs %s under a %s budget; the cold-start cost this bug is about was not reproduced, so the paired assertion below would prove nothing",
				router, 600*time.Millisecond, budget)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected the budget to expire on the cold call, got a different failure: %v", err)
		}
		if got := embedder.callCount(); got != 1 {
			t.Fatalf("expected the build to die on embed #1, saw %d calls", got)
		}
	})

	t.Run("with_the_gate_the_same_budget_and_embedder_succeed", func(t *testing.T) {
		embedder := newColdEmbedder()

		warm, err := routerwarmup.WaitForWarmEmbedder(context.Background(), embedder, timings, "probe")
		if err != nil {
			t.Fatalf("warm-up gate failed on an embedder that becomes warm after one call: %v", err)
		}
		if !warm.Warmed() {
			t.Fatalf("WaitForWarmEmbedder returned err=nil but Warmed()=false: %s", warm.Summary())
		}
		if warm.Probes < 2 {
			t.Fatalf("expected the gate to spend at least one probe on the cold call and one reaching the target, got %s", warm.Summary())
		}
		if warm.QualifyingLatency > timings.WarmTargetLatency {
			t.Fatalf("qualifying probe latency %s exceeds the target %s: %s", warm.QualifyingLatency, timings.WarmTargetLatency, warm.Summary())
		}

		beforeBuild := embedder.callCount()
		router, err := routerwarmup.BuildRouter(context.Background(), warm, bug064003RoutingConfig(), scenarios, embedder, timings)
		if err != nil {
			t.Fatalf("BuildRouter failed under the SAME %s budget that the ungated build could not meet: %v", budget, err)
		}
		if router == nil {
			t.Fatal("BuildRouter returned a nil router with a nil error")
		}
		if got, want := embedder.callCount()-beforeBuild, len(scenarios); got != want {
			t.Fatalf("router construction issued %d embeds; want one per intent_example (%d)", got, want)
		}
	})
}

// TestBUG064003_ZeroWarmResultIsStructurallyRefused proves the ordering is not
// merely conventional: a caller cannot fabricate a warm result and skip the
// gate, because the marker WaitForWarmEmbedder sets is unexported.
func TestBUG064003_ZeroWarmResultIsStructurallyRefused(t *testing.T) {
	scenarios := bug064003Scenarios(2)
	timings := routerwarmup.Timings{
		EmbedCallTimeout:   time.Second,
		WarmTargetLatency:  10 * time.Millisecond,
		WarmupBudget:       time.Second,
		BuildPerCallBudget: 500 * time.Millisecond,
	}
	embedder := &scriptedEmbedder{warmLatency: time.Millisecond}

	router, err := routerwarmup.BuildRouter(context.Background(), routerwarmup.WarmResult{}, bug064003RoutingConfig(), scenarios, embedder, timings)
	if err == nil {
		t.Fatalf("BuildRouter accepted a fabricated zero WarmResult and built %v; the warm-up gate is bypassable", router)
	}
	if !errors.Is(err, routerwarmup.ErrWarmupGateSkipped) {
		t.Fatalf("expected ErrWarmupGateSkipped, got: %v", err)
	}
	if got := embedder.callCount(); got != 0 {
		t.Fatalf("BuildRouter issued %d embeds before refusing; it must refuse before any measured work", got)
	}
}

// TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting covers SCN-064-003-02:
// an embedder that never becomes warm must name readiness, and must not read as
// a routing failure.
func TestBUG064003_UnreadyEmbedderReportsReadinessNotRouting(t *testing.T) {
	timings := routerwarmup.Timings{
		EmbedCallTimeout:   time.Second,
		WarmTargetLatency:  10 * time.Millisecond,
		WarmupBudget:       400 * time.Millisecond,
		BuildPerCallBudget: 500 * time.Millisecond,
	}

	cases := []struct {
		name     string
		embedder *scriptedEmbedder
	}{
		{
			name:     "responds_but_never_reaches_warm_latency",
			embedder: &scriptedEmbedder{coldCalls: 1 << 30, coldLatency: 120 * time.Millisecond, warmLatency: time.Millisecond},
		},
		{
			name:     "every_probe_times_out",
			embedder: &scriptedEmbedder{coldCalls: 1 << 30, coldLatency: 10 * time.Second, warmLatency: time.Millisecond},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warm, err := routerwarmup.WaitForWarmEmbedder(context.Background(), tc.embedder, timings, "probe")
			if err == nil {
				t.Fatalf("warm-up gate reported success against an embedder that never warms: %s", warm.Summary())
			}
			if !errors.Is(err, routerwarmup.ErrEmbedderNotWarm) {
				t.Fatalf("expected ErrEmbedderNotWarm, got: %v", err)
			}
			if warm.Warmed() {
				t.Fatalf("failed gate still reported Warmed()=true: %s", warm.Summary())
			}
			msg := err.Error()
			if !strings.Contains(msg, "did not reach warm latency") {
				t.Fatalf("readiness failure does not name the cause: %s", msg)
			}
			if strings.Contains(msg, "NewRouter") {
				t.Fatalf("readiness failure is dressed as a router failure — the exact mis-attribution BUG-064-003 records: %s", msg)
			}
			if warm.Probes < 1 {
				t.Fatalf("gate reported %d probes; it must actually probe before declaring the embedder unready", warm.Probes)
			}
		})
	}
}

// TestBUG064003_RoutingValuesFailLoudWithoutFallback covers SCN-064-003-04. The
// pre-fix helpers returned 0.65 and 5 for BOTH an absent and an unparseable
// value, so a propagation failure kept the test asserting against a stale
// constant. Each case below asserts an error AND that the poisoned constant was
// not produced.
func TestBUG064003_RoutingValuesFailLoudWithoutFallback(t *testing.T) {
	complete := map[string]string{
		routerwarmup.EnvConfidenceFloor:    "0.65",
		routerwarmup.EnvConsiderTopN:       "5",
		routerwarmup.EnvFallbackScenarioID: "open_knowledge",
	}
	lookupFrom := func(env map[string]string) routerwarmup.Lookup {
		return func(key string) (string, bool) {
			v, ok := env[key]
			return v, ok
		}
	}

	t.Run("complete_environment_resolves", func(t *testing.T) {
		cfg, err := routerwarmup.LoadRoutingConfig(lookupFrom(complete))
		if err != nil {
			t.Fatalf("complete SST environment must resolve: %v", err)
		}
		if cfg.ConfidenceFloor != 0.65 || cfg.ConsiderTopN != 5 || cfg.FallbackScenarioID != "open_knowledge" {
			t.Fatalf("resolved config does not match the environment: %+v", cfg)
		}
	})

	mutations := []struct {
		name    string
		mutate  func(map[string]string)
		wantKey string
	}{
		{"confidence_floor_absent", func(e map[string]string) { delete(e, routerwarmup.EnvConfidenceFloor) }, routerwarmup.EnvConfidenceFloor},
		{"confidence_floor_empty", func(e map[string]string) { e[routerwarmup.EnvConfidenceFloor] = "" }, routerwarmup.EnvConfidenceFloor},
		{"confidence_floor_unparseable", func(e map[string]string) { e[routerwarmup.EnvConfidenceFloor] = "not-a-float" }, routerwarmup.EnvConfidenceFloor},
		{"consider_top_n_absent", func(e map[string]string) { delete(e, routerwarmup.EnvConsiderTopN) }, routerwarmup.EnvConsiderTopN},
		{"consider_top_n_empty", func(e map[string]string) { e[routerwarmup.EnvConsiderTopN] = "" }, routerwarmup.EnvConsiderTopN},
		{"consider_top_n_unparseable", func(e map[string]string) { e[routerwarmup.EnvConsiderTopN] = "many" }, routerwarmup.EnvConsiderTopN},
		{"fallback_scenario_id_absent", func(e map[string]string) { delete(e, routerwarmup.EnvFallbackScenarioID) }, routerwarmup.EnvFallbackScenarioID},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			env := make(map[string]string, len(complete))
			for k, v := range complete {
				env[k] = v
			}
			m.mutate(env)

			cfg, err := routerwarmup.LoadRoutingConfig(lookupFrom(env))
			if err == nil {
				t.Fatalf("%s did not fail loud; resolved %+v instead", m.name, cfg)
			}
			if !strings.Contains(err.Error(), m.wantKey) {
				t.Fatalf("error does not name the offending key %s: %v", m.wantKey, err)
			}
			if cfg.ConfidenceFloor == 0.65 && cfg.ConsiderTopN == 5 {
				t.Fatalf("%s produced the pre-fix fallback constants (0.65 / 5) alongside its error: %+v", m.name, cfg)
			}
		})
	}
}

// TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST covers SCN-064-003-05
// and AC-4: the per-call ceiling the routing test applies is the SST value, and
// the derived build unit may never exceed it.
func TestBUG064003_PerCallEmbedTimeoutIsNotStricterThanSST(t *testing.T) {
	env := map[string]string{
		routerwarmup.EnvEmbedTimeoutMs:      "30000",
		routerwarmup.EnvWarmTargetLatencyMs: "1000",
		routerwarmup.EnvWarmupBudgetMs:      "60000",
		routerwarmup.EnvBuildPerCallMs:      "2000",
	}
	lookupFrom := func(e map[string]string) routerwarmup.Lookup {
		return func(key string) (string, bool) {
			v, ok := e[key]
			return v, ok
		}
	}

	timings, err := routerwarmup.LoadTimings(lookupFrom(env))
	if err != nil {
		t.Fatalf("complete SST timing environment must resolve: %v", err)
	}
	if timings.EmbedCallTimeout != 30*time.Second {
		t.Fatalf("per-call embed ceiling = %s; want the SST value 30000ms", timings.EmbedCallTimeout)
	}
	if timings.EmbedCallTimeout <= 5*time.Second {
		t.Fatalf("per-call ceiling %s is no better than the pre-fix 5s literal", timings.EmbedCallTimeout)
	}

	t.Run("build_unit_above_the_sst_per_call_ceiling_is_rejected", func(t *testing.T) {
		poisoned := make(map[string]string, len(env))
		for k, v := range env {
			poisoned[k] = v
		}
		poisoned[routerwarmup.EnvBuildPerCallMs] = "45000"
		if _, err := routerwarmup.LoadTimings(lookupFrom(poisoned)); err == nil {
			t.Fatal("a build unit above the SST per-call embed ceiling was accepted")
		} else if !strings.Contains(err.Error(), routerwarmup.EnvEmbedTimeoutMs) {
			t.Fatalf("rejection does not cite the SST ceiling it violates: %v", err)
		}
	})

	t.Run("build_unit_below_the_proven_warm_latency_is_rejected", func(t *testing.T) {
		poisoned := make(map[string]string, len(env))
		for k, v := range env {
			poisoned[k] = v
		}
		poisoned[routerwarmup.EnvBuildPerCallMs] = "500"
		if _, err := routerwarmup.LoadTimings(lookupFrom(poisoned)); err == nil {
			t.Fatal("a build unit below the warm-latency target was accepted; the derived budget would be unsatisfiable on a healthy sidecar")
		} else if !strings.Contains(err.Error(), routerwarmup.EnvWarmTargetLatencyMs) {
			t.Fatalf("rejection does not cite the warm target it violates: %v", err)
		}
	})

	for key := range env {
		t.Run("absent_"+key, func(t *testing.T) {
			partial := make(map[string]string, len(env))
			for k, v := range env {
				if k != key {
					partial[k] = v
				}
			}
			if _, err := routerwarmup.LoadTimings(lookupFrom(partial)); err == nil {
				t.Fatalf("%s absent did not fail loud", key)
			} else if !strings.Contains(err.Error(), key) {
				t.Fatalf("error does not name the missing key %s: %v", key, err)
			}
		})
	}
}

var (
	bug064003DurationLiteral = regexp.MustCompile(`\b\d+\s*\*\s*time\.(?:Nanosecond|Microsecond|Millisecond|Second|Minute|Hour)\b`)
	bug064003FallbackHelper  = regexp.MustCompile(`func\s+parse(?:Float|Int)Env\s*\(`)
)

// bug064003RepoRoot anchors the repository from this file's compile-time path.
// It works bare (`go test ./...`) and inside the smackerel.sh test container,
// where the repo is mounted at /workspace.
func bug064003RepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) unavailable — cannot anchor the source-shape guard against the repo")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestBUG064003_RoutingTestCarriesNoWallClockLiteral covers SCN-064-003-03. It
// reads the SCOPE-12 routing test as source text and rejects every shape the
// bug was made of. Against the pre-fix tree all four assertions fail:
// `5*time.Second` and `30*time.Second` were present, agent.NewRouter was called
// directly, `func parseFloatEnv(` existed, and no warm-up call preceded the
// build.
func TestBUG064003_RoutingTestCarriesNoWallClockLiteral(t *testing.T) {
	routingTest := filepath.Join(bug064003RepoRoot(t), "tests", "integration", "agent", "openknowledge_routing_test.go")
	raw, err := os.ReadFile(routingTest)
	if err != nil {
		t.Fatalf("the SCOPE-12 routing test this guard protects is unreadable at %s: %v", routingTest, err)
	}
	src := string(raw)

	if hits := bug064003DurationLiteral.FindAllString(src, -1); len(hits) > 0 {
		t.Errorf("%s reintroduces wall-clock duration literals %v; every timing value must come from SST via routerwarmup (spec.md AC-3)", routingTest, hits)
	}
	if strings.Contains(src, "agent.NewRouter(") {
		t.Errorf("%s calls agent.NewRouter directly; it must go through routerwarmup.BuildRouter so the derived budget and the warm-up precondition are enforced", routingTest)
	}
	if hits := bug064003FallbackHelper.FindAllString(src, -1); len(hits) > 0 {
		t.Errorf("%s reintroduces the env-fallback helper shape %v; SST values must resolve fail-loud (spec.md EB-5)", routingTest, hits)
	}
	if !strings.Contains(src, "routerwarmup.LoadRoutingConfig(") {
		t.Errorf("%s no longer resolves its routing config through routerwarmup.LoadRoutingConfig", routingTest)
	}
	if !strings.Contains(src, "timings.EmbedCallTimeout") {
		t.Errorf("%s no longer passes the SST per-call embed ceiling to the sidecar embedder (spec.md AC-4)", routingTest)
	}

	warmIdx := strings.Index(src, "routerwarmup.WaitForWarmEmbedder(")
	buildIdx := strings.Index(src, "routerwarmup.BuildRouter(")
	if warmIdx < 0 {
		t.Fatalf("%s does not call routerwarmup.WaitForWarmEmbedder; the readiness gate is gone", routingTest)
	}
	if buildIdx < 0 {
		t.Fatalf("%s does not call routerwarmup.BuildRouter", routingTest)
	}
	if warmIdx > buildIdx {
		t.Fatalf("%s builds the router at offset %d before warming the embedder at offset %d; cold-start cost is back inside the measured region", routingTest, buildIdx, warmIdx)
	}
}

// TestBUG064003_SSTPublishesTheWarmupContract proves the timing values the
// routing test consumes are real SST keys with a generator path, not env vars
// somebody hoped would exist. Without this, the fix would resolve nothing at
// runtime and the test would fail loud on every host.
func TestBUG064003_SSTPublishesTheWarmupContract(t *testing.T) {
	root := bug064003RepoRoot(t)

	sstRaw, err := os.ReadFile(filepath.Join(root, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read config/smackerel.yaml: %v", err)
	}
	sst := string(sstRaw)
	for _, key := range []string{
		"warmup_target_latency_ms:",
		"warmup_budget_ms:",
		"build_per_call_budget_ms:",
		"embed_timeout_ms:",
	} {
		if !strings.Contains(sst, key) {
			t.Errorf("config/smackerel.yaml does not declare agent.routing.%s", strings.TrimSuffix(key, ":"))
		}
	}

	genRaw, err := os.ReadFile(filepath.Join(root, "scripts", "commands", "config.sh"))
	if err != nil {
		t.Fatalf("read scripts/commands/config.sh: %v", err)
	}
	gen := string(genRaw)
	for _, envKey := range []string{
		routerwarmup.EnvEmbedTimeoutMs,
		routerwarmup.EnvWarmTargetLatencyMs,
		routerwarmup.EnvWarmupBudgetMs,
		routerwarmup.EnvBuildPerCallMs,
		routerwarmup.EnvConfidenceFloor,
		routerwarmup.EnvConsiderTopN,
		routerwarmup.EnvFallbackScenarioID,
	} {
		// Once to resolve it from SST, once to write it into the env file.
		if got := strings.Count(gen, envKey); got < 2 {
			t.Errorf("scripts/commands/config.sh mentions %s %d time(s); it must both resolve the SST value and emit it into the generated env", envKey, got)
		}
	}
}

// TestBUG064003_DerivedBudgetScalesWithTheWork proves the property that makes
// the budget honest as the scenario set grows: adding one intent_example widens
// the budget by exactly one per-call unit instead of silently eroding a fixed
// constant. Pre-fix, 79 examples and 800 examples shared one 30s literal.
func TestBUG064003_DerivedBudgetScalesWithTheWork(t *testing.T) {
	timings := routerwarmup.Timings{
		EmbedCallTimeout:   30 * time.Second,
		WarmTargetLatency:  time.Second,
		WarmupBudget:       60 * time.Second,
		BuildPerCallBudget: 2 * time.Second,
	}
	if err := timings.Validate(); err != nil {
		t.Fatalf("SST-shaped timings must validate: %v", err)
	}

	for _, n := range []int{1, 13, 31, 79, 160} {
		scenarios := bug064003Scenarios(n)
		if got := routerwarmup.ExampleCount(scenarios); got != n {
			t.Fatalf("ExampleCount = %d; want %d", got, n)
		}
		want := time.Duration(n) * timings.BuildPerCallBudget
		if got := routerwarmup.BuildBudget(scenarios, timings); got != want {
			t.Fatalf("BuildBudget for %d examples = %s; want %s", n, got, want)
		}
	}

	base := routerwarmup.BuildBudget(bug064003Scenarios(79), timings)
	grown := routerwarmup.BuildBudget(bug064003Scenarios(80), timings)
	if grown-base != timings.BuildPerCallBudget {
		t.Fatalf("adding one intent_example changed the budget by %s; want exactly one per-call unit (%s)", grown-base, timings.BuildPerCallBudget)
	}

	// The whole point of the 30s literal being wrong: at 79 examples the
	// derived budget must exceed it, and must still fit the lane's own
	// `go test -timeout 300s`.
	if base <= 30*time.Second {
		t.Fatalf("derived budget for 79 examples is %s — no wider than the literal this bug removed", base)
	}
	if base >= 300*time.Second {
		t.Fatalf("derived budget for 79 examples is %s — it collides with the integration lane's go test -timeout 300s", base)
	}
}

// TestBUG064003_SSTDefaultsFitTheLaneBudget checks the committed SST numbers
// (not a synthetic set) against the constraint design.md § "Option C" used to
// reject simply enlarging the literal.
func TestBUG064003_SSTDefaultsFitTheLaneBudget(t *testing.T) {
	root := bug064003RepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read config/smackerel.yaml: %v", err)
	}

	read := func(key string) int {
		t.Helper()
		re := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(key) + `:\s*(\d+)`)
		m := re.FindStringSubmatch(string(raw))
		if m == nil {
			t.Fatalf("config/smackerel.yaml does not declare agent.routing.%s", key)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("agent.routing.%s is not an integer: %v", key, err)
		}
		return n
	}

	timings := routerwarmup.Timings{
		EmbedCallTimeout:   time.Duration(read("embed_timeout_ms")) * time.Millisecond,
		WarmTargetLatency:  time.Duration(read("warmup_target_latency_ms")) * time.Millisecond,
		WarmupBudget:       time.Duration(read("warmup_budget_ms")) * time.Millisecond,
		BuildPerCallBudget: time.Duration(read("build_per_call_budget_ms")) * time.Millisecond,
	}
	if err := timings.Validate(); err != nil {
		t.Fatalf("the committed SST values violate their own contract: %v", err)
	}

	// 79 is the intent_example count recorded in design.md § 1.1.
	const observedExamples = 79
	worstCase := timings.WarmupBudget + time.Duration(observedExamples)*timings.BuildPerCallBudget
	const laneTimeout = 300 * time.Second
	if worstCase >= laneTimeout {
		t.Fatalf("warm-up budget (%s) + derived build budget for %d examples (%s) = %s, which does not fit the integration lane's go test -timeout %s",
			timings.WarmupBudget, observedExamples, time.Duration(observedExamples)*timings.BuildPerCallBudget, worstCase, laneTimeout)
	}
}
