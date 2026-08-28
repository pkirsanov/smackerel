//go:build stress

// Package stress — BUG-064-003 T6: SLA stress profile for the router warm-up
// contract.
//
// The defect this protects against is literally "router warm-up exceeds fixed
// deadline". Scope 1 therefore declares three latency budgets in the SST:
//
//	config/smackerel.yaml agent.routing.warmup_target_latency_ms
//	  → AGENT_ROUTING_WARMUP_TARGET_LATENCY_MS
//	config/smackerel.yaml agent.routing.warmup_budget_ms
//	  → AGENT_ROUTING_WARMUP_BUDGET_MS
//	config/smackerel.yaml agent.routing.build_per_call_budget_ms
//	  → AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS
//
// Gate G026 requires a scope that declares latency SLAs to own a stress run.
// This file is that run, and it asserts two separate things:
//
//   - SST shape. The budgets are present in the environment this lane receives
//     and satisfy the ordering invariant the SST itself documents
//     (build_per_call_budget_ms >= warmup_target_latency_ms). Absent values
//     fail loud per smackerel-no-defaults policy — there is no hidden fallback.
//
//   - The budget is a real upper bound UNDER LOAD. Against an embedder that is
//     reachable but can never qualify as warm, WaitForWarmEmbedder must return
//     ErrEmbedderNotWarm within its declared WarmupBudget — every time, across
//     sequential repeats and concurrent waiters. An unbounded or overshooting
//     wait is the exact regression this bug fixed.
//
// The load assertions deliberately use a SHORT synthetic budget instead of the
// SST's 60s production value. The property under test — "Elapsed never exceeds
// WarmupBudget" — is scale-free, and a 60s budget would make the concurrent
// profile run for tens of minutes without exercising anything the short budget
// does not. The production values are still asserted, by the first test,
// against the real environment.
//
// Like the other stress profiles here this needs no ML container: the embedder
// is an in-process fake whose latency is chosen relative to the budgets. The
// disposable-stack guard still runs, so the file refuses to execute if it is
// ever wired at the persistent dev stack.
package stress

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/tests/integration/agent/routerwarmup"
)

// slaEmbedder is a reachable embedder whose probe latency is fixed. Latency
// above Timings.WarmTargetLatency means it answers successfully but can never
// qualify as warm, which is the condition that must terminate at the budget
// rather than run unbounded.
type slaEmbedder struct {
	latency time.Duration

	mu    sync.Mutex
	calls int
}

func (e *slaEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	select {
	case <-time.After(e.latency):
		return []float32{0.1, 0.2, 0.3}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *slaEmbedder) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// stressTimings is the short synthetic profile used by the load assertions.
// Relationship between the values is what matters:
//
//	WarmTargetLatency (10ms) < neverWarmLatency (50ms) < EmbedCallTimeout (200ms)
//
// so the fake is reachable (every probe succeeds well inside the per-call
// timeout) yet never qualifies as warm (every probe is slower than the target).
// BuildPerCallBudget is kept >= WarmTargetLatency to satisfy the same ordering
// invariant the SST documents.
func stressTimings() routerwarmup.Timings {
	return routerwarmup.Timings{
		EmbedCallTimeout:   200 * time.Millisecond,
		WarmTargetLatency:  10 * time.Millisecond,
		WarmupBudget:       300 * time.Millisecond,
		BuildPerCallBudget: 50 * time.Millisecond,
	}
}

const (
	neverWarmLatency = 50 * time.Millisecond
	warmLatency      = 1 * time.Millisecond
)

// budgetTolerance allows for one in-flight probe plus scheduler noise. The loop
// caps each probe's timeout by the remaining budget, so a correct
// implementation lands at or just past the budget; a broken one overshoots by
// a whole probe or never returns at all. The tolerance is wide enough not to
// flake under a loaded CI box and far too tight to admit an unbounded wait.
const budgetTolerance = 250 * time.Millisecond

// TestBUG064003_SLABudgetsArePresentAndOrdered asserts the SST contract shape
// against the REAL environment this lane receives. It is the half of T6 that
// covers the production values; the load tests below cover the behaviour.
func TestBUG064003_SLABudgetsArePresentAndOrdered(t *testing.T) {
	requireDisposableStack(t)

	timings, err := routerwarmup.LoadTimings(os.LookupEnv)
	if err != nil {
		t.Fatalf("BUG-064-003 T6: the three routing SLA budgets must be present in this lane's env "+
			"(smackerel-no-defaults: no hidden fallback is permitted): %v", err)
	}

	if timings.WarmTargetLatency <= 0 {
		t.Fatalf("%s must be > 0; got %s", routerwarmup.EnvWarmTargetLatencyMs, timings.WarmTargetLatency)
	}
	if timings.WarmupBudget <= 0 {
		t.Fatalf("%s must be > 0; got %s", routerwarmup.EnvWarmupBudgetMs, timings.WarmupBudget)
	}
	if timings.BuildPerCallBudget <= 0 {
		t.Fatalf("%s must be > 0; got %s", routerwarmup.EnvBuildPerCallMs, timings.BuildPerCallBudget)
	}

	// Documented in config/smackerel.yaml: the per-call build unit must not be
	// stricter than the latency a probe has to beat to count as warm, or router
	// construction would reject an embedder the warm-up gate just accepted.
	if timings.BuildPerCallBudget < timings.WarmTargetLatency {
		t.Fatalf("SST ordering invariant violated: %s (%s) must be >= %s (%s)",
			routerwarmup.EnvBuildPerCallMs, timings.BuildPerCallBudget,
			routerwarmup.EnvWarmTargetLatencyMs, timings.WarmTargetLatency)
	}

	// The readiness wait has to be able to admit at least one qualifying probe.
	if timings.WarmupBudget < timings.WarmTargetLatency {
		t.Fatalf("SST ordering invariant violated: %s (%s) must be >= %s (%s)",
			routerwarmup.EnvWarmupBudgetMs, timings.WarmupBudget,
			routerwarmup.EnvWarmTargetLatencyMs, timings.WarmTargetLatency)
	}

	t.Logf("BUG-064-003 T6 SST budgets: target=%s budget=%s per-call=%s embed-timeout=%s",
		timings.WarmTargetLatency, timings.WarmupBudget,
		timings.BuildPerCallBudget, timings.EmbedCallTimeout)
}

// TestBUG064003_WarmupBudgetBoundsRepeatedNeverWarmWaits runs the never-warm
// path many times in sequence. Each run must terminate with the readiness
// verdict inside its budget. Before the fix the wait was governed by a fixed
// deadline that the warm-up could exceed; the regression this guards against is
// any run whose Elapsed climbs past WarmupBudget.
func TestBUG064003_WarmupBudgetBoundsRepeatedNeverWarmWaits(t *testing.T) {
	requireDisposableStack(t)

	timings := stressTimings()
	const iterations = 25

	var worst time.Duration
	for i := 0; i < iterations; i++ {
		embedder := &slaEmbedder{latency: neverWarmLatency}

		start := time.Now()
		result, err := routerwarmup.WaitForWarmEmbedder(context.Background(), embedder, timings, "probe")
		wall := time.Since(start)

		if !errors.Is(err, routerwarmup.ErrEmbedderNotWarm) {
			t.Fatalf("iteration %d: an embedder that answers every probe but never beats the warm "+
				"target must end as ErrEmbedderNotWarm; got err=%v warmed=%v", i, err, result.Warmed())
		}
		if result.Warmed() {
			t.Fatalf("iteration %d: result reports warmed despite every probe exceeding the target", i)
		}
		if embedder.callCount() == 0 {
			t.Fatalf("iteration %d: budget expired without probing the embedder at all", i)
		}
		if wall > timings.WarmupBudget+budgetTolerance {
			t.Fatalf("iteration %d: warm-up overran its declared budget — this is BUG-064-003 reappearing. "+
				"budget=%s observed=%s probes=%d", i, timings.WarmupBudget, wall, result.Probes)
		}
		if wall > worst {
			worst = wall
		}
	}

	t.Logf("BUG-064-003 T6 sequential never-warm: %d iterations, budget=%s, worst observed=%s",
		iterations, timings.WarmupBudget, worst)
}

// TestBUG064003_WarmupBudgetHoldsUnderConcurrentWaiters is the load half. Many
// waiters contend at once; the budget must remain an upper bound for every one
// of them, not merely on average.
func TestBUG064003_WarmupBudgetHoldsUnderConcurrentWaiters(t *testing.T) {
	requireDisposableStack(t)

	timings := stressTimings()
	const waiters = 32

	type outcome struct {
		wall   time.Duration
		err    error
		warmed bool
		probes int
	}

	results := make([]outcome, waiters)
	var wg sync.WaitGroup
	wg.Add(waiters)

	for i := 0; i < waiters; i++ {
		go func(idx int) {
			defer wg.Done()
			embedder := &slaEmbedder{latency: neverWarmLatency}
			start := time.Now()
			res, err := routerwarmup.WaitForWarmEmbedder(context.Background(), embedder, timings, "probe")
			results[idx] = outcome{wall: time.Since(start), err: err, warmed: res.Warmed(), probes: res.Probes}
		}(i)
	}
	wg.Wait()

	var worst time.Duration
	for i, r := range results {
		if !errors.Is(r.err, routerwarmup.ErrEmbedderNotWarm) {
			t.Fatalf("waiter %d: expected ErrEmbedderNotWarm under contention; got err=%v warmed=%v",
				i, r.err, r.warmed)
		}
		if r.wall > timings.WarmupBudget+budgetTolerance {
			t.Fatalf("waiter %d: warm-up overran its declared budget under contention — "+
				"budget=%s observed=%s probes=%d", i, timings.WarmupBudget, r.wall, r.probes)
		}
		if r.wall > worst {
			worst = r.wall
		}
	}

	t.Logf("BUG-064-003 T6 concurrent never-warm: %d waiters, budget=%s, worst observed=%s",
		waiters, timings.WarmupBudget, worst)
}

// TestBUG064003_WarmEmbedderQualifiesWellInsideBudgetUnderLoad is the positive
// counterpart. A fast embedder must be recognised as warm promptly rather than
// burning the whole budget, and it must report the latency that qualified it.
// Without this, a "fix" that simply waited the full budget every time would
// still satisfy the upper-bound tests above.
func TestBUG064003_WarmEmbedderQualifiesWellInsideBudgetUnderLoad(t *testing.T) {
	requireDisposableStack(t)

	timings := stressTimings()
	const waiters = 32

	walls := make([]time.Duration, waiters)
	errs := make([]error, waiters)
	qualifying := make([]time.Duration, waiters)

	var wg sync.WaitGroup
	wg.Add(waiters)
	for i := 0; i < waiters; i++ {
		go func(idx int) {
			defer wg.Done()
			embedder := &slaEmbedder{latency: warmLatency}
			start := time.Now()
			res, err := routerwarmup.WaitForWarmEmbedder(context.Background(), embedder, timings, "probe")
			walls[idx] = time.Since(start)
			errs[idx] = err
			qualifying[idx] = res.QualifyingLatency
		}(i)
	}
	wg.Wait()

	var worst time.Duration
	for i := range walls {
		if errs[i] != nil {
			t.Fatalf("waiter %d: a probe at %s must qualify against a %s target; got %v",
				i, warmLatency, timings.WarmTargetLatency, errs[i])
		}
		if qualifying[i] <= 0 || qualifying[i] > timings.WarmTargetLatency {
			t.Fatalf("waiter %d: qualifying latency %s must be >0 and within the %s target",
				i, qualifying[i], timings.WarmTargetLatency)
		}
		// A warm embedder must not consume the budget. Half of it is generous
		// for a 1ms probe and still fails a fix that always waits the full term.
		if walls[i] > timings.WarmupBudget/2 {
			t.Fatalf("waiter %d: a warm embedder consumed %s of a %s budget — warm-up is not "+
				"returning on the first qualifying probe", i, walls[i], timings.WarmupBudget)
		}
		if walls[i] > worst {
			worst = walls[i]
		}
	}

	t.Logf("BUG-064-003 T6 concurrent warm path: %d waiters, budget=%s, worst observed=%s",
		waiters, timings.WarmupBudget, worst)
}
