// Package routerwarmup carries the BUG-064-003 router-construction contract
// used by the spec 064 SCOPE-12 routing integration test.
//
// The defect it removes: the routing test wrapped agent.NewRouter in a
// hard-coded 30-second context. NewRouter issues ONE sequential POST /embed
// per intent_examples entry across every registered scenario — 79 calls at the
// time of writing — so that constant was a wall clock placed over variable
// cost, cold-start-dominated warm-up work. Two recorded failures aborted at
// embed ordinals 31/79 and 13/79, implying the full set needed ~79s and ~197s
// respectively. Enlarging the constant fixes neither run, and the repository's
// own SST already contradicts the assumption behind it: agent.routing.
// embed_timeout_ms budgets 30 000 ms for ONE cold call.
//
// The contract here is two-phase:
//
//	Phase 1 (WaitForWarmEmbedder) — pay the cold sentence-transformer load
//	OUTSIDE any measured region. Probe /embed until a probe returns at or
//	under the SST warm-latency target. Exceeding the SST warm-up budget is
//	reported as ErrEmbedderNotWarm, which names readiness as the cause and is
//	distinguishable from a routing failure.
//
//	Phase 2 (BuildRouter) — measure only router construction, under a budget
//	DERIVED from the work: (number of intent_examples) × the SST per-call
//	build unit. No wall-clock literal governs it, and it grows automatically
//	when a scenario adds an example. Exceeding it is reported as
//	ErrBuildBudgetExceeded — a sidecar-throughput verdict, never a routing one.
//
// BuildRouter REFUSES a zero WarmResult. WarmResult carries an unexported
// marker that only WaitForWarmEmbedder can set, so "warm up before you
// measure" is enforced by the type system rather than by convention. The
// adversarial regression in internal/agent proves the gate is load-bearing:
// the same cold embedder under the same derived budget FAILS without the gate
// and SUCCEEDS with it.
//
// Every timing value is resolved fail-loud from the SST-generated environment.
// There is no fallback: an absent or unparseable key is an error, matching
// internal/agent/config.go's requireFloat/requireInt posture and the
// smackerel-no-defaults policy.
package routerwarmup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// Lookup is the environment accessor. os.LookupEnv satisfies it directly;
// tests supply a map-backed implementation to exercise absent and malformed
// values without mutating the process environment.
type Lookup func(key string) (string, bool)

// SST environment keys. All are emitted by scripts/commands/config.sh from
// config/smackerel.yaml agent.routing.* and are present in every generated
// env file the integration container receives via --env-file.
const (
	EnvConfidenceFloor     = "AGENT_ROUTING_CONFIDENCE_FLOOR"
	EnvConsiderTopN        = "AGENT_ROUTING_CONSIDER_TOP_N"
	EnvFallbackScenarioID  = "AGENT_ROUTING_FALLBACK_SCENARIO_ID"
	EnvEmbedTimeoutMs      = "ASSISTANT_ROUTING_EMBED_TIMEOUT_MS"
	EnvWarmTargetLatencyMs = "AGENT_ROUTING_WARMUP_TARGET_LATENCY_MS"
	EnvWarmupBudgetMs      = "AGENT_ROUTING_WARMUP_BUDGET_MS"
	EnvBuildPerCallMs      = "AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS"
)

var (
	// ErrEmbedderNotWarm reports that the ML sidecar's embedder never reached
	// the SST warm-latency target inside the SST warm-up budget. It is the
	// readiness verdict spec.md EB-2 requires, and it must never be confused
	// with a routing-assertion failure.
	ErrEmbedderNotWarm = errors.New("routerwarmup: ML sidecar embedder did not reach warm latency")

	// ErrEmbedderUnreachable reports that the probe never got a response from
	// the sidecar at all (connection refused, DNS failure). The stack is not
	// up; this is not a statement about routing or about warmth.
	ErrEmbedderUnreachable = errors.New("routerwarmup: ML sidecar /embed unreachable")

	// ErrWarmupGateSkipped reports an attempt to build the router without a
	// completed warm-up gate. Reaching it means the two-phase contract was
	// bypassed.
	ErrWarmupGateSkipped = errors.New("routerwarmup: router construction attempted without a completed warm-up gate")

	// ErrBuildBudgetExceeded reports that router construction outran the
	// derived budget on an embedder that had already proven warm. That is a
	// sidecar-throughput verdict, not a routing verdict.
	ErrBuildBudgetExceeded = errors.New("routerwarmup: router construction exceeded its derived embed budget")
)

// Timings is the SST-sourced timing contract. Every field comes from
// config/smackerel.yaml; none has a literal in this package.
type Timings struct {
	// EmbedCallTimeout is the per-call /embed ceiling, from
	// agent.routing.embed_timeout_ms. Passing this to sidecar.New keeps the
	// test's per-call ceiling from being stricter than production's
	// (spec.md EB-4 / AC-4).
	EmbedCallTimeout time.Duration

	// WarmTargetLatency is the probe latency at or under which the embedder
	// counts as warm, from agent.routing.warmup_target_latency_ms.
	WarmTargetLatency time.Duration

	// WarmupBudget bounds the readiness wait itself, from
	// agent.routing.warmup_budget_ms.
	WarmupBudget time.Duration

	// BuildPerCallBudget is the per-embed unit the router-construction budget
	// is derived from, from agent.routing.build_per_call_budget_ms.
	BuildPerCallBudget time.Duration
}

// Validate enforces the relationships the SST comment documents. A per-call
// build unit below the proven-warm target would make the derived budget
// unsatisfiable even on a healthy sidecar; one above the SST per-call ceiling
// would let the test tolerate latency production itself rejects.
func (t Timings) Validate() error {
	var problems []string
	if t.EmbedCallTimeout <= 0 {
		problems = append(problems, fmt.Sprintf("%s must be > 0", EnvEmbedTimeoutMs))
	}
	if t.WarmTargetLatency <= 0 {
		problems = append(problems, fmt.Sprintf("%s must be > 0", EnvWarmTargetLatencyMs))
	}
	if t.WarmupBudget <= 0 {
		problems = append(problems, fmt.Sprintf("%s must be > 0", EnvWarmupBudgetMs))
	}
	if t.BuildPerCallBudget <= 0 {
		problems = append(problems, fmt.Sprintf("%s must be > 0", EnvBuildPerCallMs))
	}
	if len(problems) == 0 {
		if t.BuildPerCallBudget < t.WarmTargetLatency {
			problems = append(problems, fmt.Sprintf(
				"%s (%s) must be >= %s (%s) — the derived build budget cannot be tighter than the latency the warm-up gate proves",
				EnvBuildPerCallMs, t.BuildPerCallBudget, EnvWarmTargetLatencyMs, t.WarmTargetLatency))
		}
		if t.BuildPerCallBudget > t.EmbedCallTimeout {
			problems = append(problems, fmt.Sprintf(
				"%s (%s) must be <= %s (%s) — the build unit cannot exceed the SST-legal per-call embed ceiling",
				EnvBuildPerCallMs, t.BuildPerCallBudget, EnvEmbedTimeoutMs, t.EmbedCallTimeout))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("routerwarmup: invalid SST timing contract: %s", strings.Join(problems, "; "))
	}
	return nil
}

// LoadTimings resolves the timing contract from the SST environment. Absent or
// unparseable values are errors; nothing is substituted.
func LoadTimings(lookup Lookup) (Timings, error) {
	var missing, bad []string
	ms := func(key string) time.Duration {
		raw, ok := lookup(key)
		if !ok || strings.TrimSpace(raw) == "" {
			missing = append(missing, key)
			return 0
		}
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s (must be integer milliseconds, got %q)", key, raw))
			return 0
		}
		if n <= 0 {
			bad = append(bad, fmt.Sprintf("%s (must be > 0, got %d)", key, n))
			return 0
		}
		return time.Duration(n) * time.Millisecond
	}

	t := Timings{
		EmbedCallTimeout:   ms(EnvEmbedTimeoutMs),
		WarmTargetLatency:  ms(EnvWarmTargetLatencyMs),
		WarmupBudget:       ms(EnvWarmupBudgetMs),
		BuildPerCallBudget: ms(EnvBuildPerCallMs),
	}
	if err := joinResolutionProblems("timing contract", missing, bad); err != nil {
		return Timings{}, err
	}
	if err := t.Validate(); err != nil {
		return Timings{}, err
	}
	return t, nil
}

// LoadRoutingConfig resolves the routing knobs from the SST environment,
// mirroring internal/agent/config.go's fail-loud requireFloat/requireInt. It
// replaces the local parseFloatEnv/parseIntEnv fallback helpers the routing
// test used to carry: those returned 0.65 and 5 on an absent OR unparseable
// value, so a propagation failure would have kept the test asserting against a
// stale constant instead of failing loud.
//
// FallbackScenarioID is read but deliberately permitted to be empty: SST
// documents "" as a legitimate value meaning "no fallback". The key must still
// be present.
func LoadRoutingConfig(lookup Lookup) (agent.RoutingConfig, error) {
	var missing, bad []string

	floor := 0.0
	if raw, ok := lookup(EnvConfidenceFloor); !ok || strings.TrimSpace(raw) == "" {
		missing = append(missing, EnvConfidenceFloor)
	} else if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err != nil {
		bad = append(bad, fmt.Sprintf("%s (must be a float, got %q)", EnvConfidenceFloor, raw))
	} else if f < 0 || f > 1 {
		bad = append(bad, fmt.Sprintf("%s (must be in range [0, 1], got %g)", EnvConfidenceFloor, f))
	} else {
		floor = f
	}

	topN := 0
	if raw, ok := lookup(EnvConsiderTopN); !ok || strings.TrimSpace(raw) == "" {
		missing = append(missing, EnvConsiderTopN)
	} else if n, err := strconv.Atoi(strings.TrimSpace(raw)); err != nil {
		bad = append(bad, fmt.Sprintf("%s (must be an integer, got %q)", EnvConsiderTopN, raw))
	} else if n < 1 {
		bad = append(bad, fmt.Sprintf("%s (must be >= 1, got %d)", EnvConsiderTopN, n))
	} else {
		topN = n
	}

	fallbackID, ok := lookup(EnvFallbackScenarioID)
	if !ok {
		missing = append(missing, EnvFallbackScenarioID)
	}

	if err := joinResolutionProblems("routing config", missing, bad); err != nil {
		return agent.RoutingConfig{}, err
	}
	return agent.RoutingConfig{
		ConfidenceFloor:    floor,
		ConsiderTopN:       topN,
		FallbackScenarioID: fallbackID,
	}, nil
}

func joinResolutionProblems(what string, missing, bad []string) error {
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing required SST values: "+strings.Join(missing, ", "))
	}
	if len(bad) > 0 {
		parts = append(parts, "invalid SST values: "+strings.Join(bad, "; "))
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("routerwarmup: %s unresolved (smackerel-no-defaults: no value is substituted): %s",
		what, strings.Join(parts, "; "))
}

// WarmResult records what the warm-up gate observed. Its zero value is not a
// warm result: the `warmed` marker is unexported and set only by
// WaitForWarmEmbedder, so no caller outside this package can fabricate one and
// skip the gate.
type WarmResult struct {
	warmed bool

	// Probes is the number of probe embeds issued.
	Probes int
	// Latencies is the measured latency of every probe, in issue order.
	Latencies []time.Duration
	// QualifyingLatency is the latency of the probe that met the target.
	QualifyingLatency time.Duration
	// Elapsed is the total time the gate spent reaching warmth.
	Elapsed time.Duration
}

// Warmed reports whether this result came from a completed warm-up gate.
func (w WarmResult) Warmed() bool { return w.warmed }

// Summary renders the gate's observations for evidence and failure messages.
func (w WarmResult) Summary() string {
	seen := make([]string, 0, len(w.Latencies))
	for i, d := range w.Latencies {
		seen = append(seen, fmt.Sprintf("#%d=%s", i+1, d.Round(time.Millisecond)))
	}
	return fmt.Sprintf("probes=%d latencies=[%s] qualifying=%s elapsed=%s",
		w.Probes, strings.Join(seen, " "), w.QualifyingLatency.Round(time.Millisecond), w.Elapsed.Round(time.Millisecond))
}

// WaitForWarmEmbedder blocks until a probe embed completes at or under
// t.WarmTargetLatency, or until t.WarmupBudget expires.
//
// Each probe is bounded by t.EmbedCallTimeout — the SST-legal per-call ceiling,
// sized for exactly this cold sentence-transformer load — and by whatever
// remains of the warm-up budget. A probe that times out is a cold-start
// symptom and is retried while budget remains. A probe that fails without ever
// getting a response (connection refused, DNS failure) means the sidecar is not
// up: that returns ErrEmbedderUnreachable immediately rather than burning the
// budget. Any other probe error (auth rejection, malformed body) is permanent
// and is returned as-is.
func WaitForWarmEmbedder(ctx context.Context, embedder agent.Embedder, t Timings, probeText string) (WarmResult, error) {
	if embedder == nil {
		return WarmResult{}, errors.New("routerwarmup: WaitForWarmEmbedder requires a non-nil Embedder")
	}
	if err := t.Validate(); err != nil {
		return WarmResult{}, err
	}

	started := time.Now()
	deadline := started.Add(t.WarmupBudget)
	result := WarmResult{}
	var lastErr error

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		probeTimeout := t.EmbedCallTimeout
		if remaining < probeTimeout {
			probeTimeout = remaining
		}

		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		callStart := time.Now()
		_, err := embedder.Embed(probeCtx, probeText)
		latency := time.Since(callStart)
		cancel()

		result.Probes++
		result.Latencies = append(result.Latencies, latency)

		if err == nil {
			if latency <= t.WarmTargetLatency {
				result.warmed = true
				result.QualifyingLatency = latency
				result.Elapsed = time.Since(started)
				return result, nil
			}
			lastErr = nil
			continue
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			result.Elapsed = time.Since(started)
			return result, fmt.Errorf("routerwarmup: warm-up gate cancelled after %s: %w", result.Summary(), ctxErr)
		}
		if isTimeout(err) {
			lastErr = err
			continue
		}
		if isUnreachable(err) {
			result.Elapsed = time.Since(started)
			return result, fmt.Errorf("%w after %s: %v", ErrEmbedderUnreachable, result.Summary(), err)
		}
		result.Elapsed = time.Since(started)
		return result, fmt.Errorf("routerwarmup: probe embed failed after %s: %w", result.Summary(), err)
	}

	result.Elapsed = time.Since(started)
	msg := fmt.Sprintf("%s within %s (%s target from %s, budget from %s): %s",
		ErrEmbedderNotWarm.Error(), t.WarmupBudget, t.WarmTargetLatency, EnvWarmTargetLatencyMs, EnvWarmupBudgetMs, result.Summary())
	if lastErr != nil {
		return result, fmt.Errorf("%s; last probe error: %w", msg, errors.Join(ErrEmbedderNotWarm, lastErr))
	}
	return result, fmt.Errorf("%s: %w", msg, ErrEmbedderNotWarm)
}

// ExampleCount returns the number of sequential embed calls agent.NewRouter
// will issue for these scenarios — one per intent_examples entry. It is the
// work term the router-construction budget scales with, so adding an example
// to a scenario widens the budget automatically instead of silently eroding it.
func ExampleCount(scenarios []*agent.Scenario) int {
	total := 0
	for _, sc := range scenarios {
		if sc == nil {
			continue
		}
		total += len(sc.IntentExamples)
	}
	return total
}

// BuildBudget derives the router-construction budget: one SST per-call unit per
// embed agent.NewRouter will issue. It is a function of the work and of SST —
// never a wall-clock literal.
func BuildBudget(scenarios []*agent.Scenario, t Timings) time.Duration {
	return time.Duration(ExampleCount(scenarios)) * t.BuildPerCallBudget
}

// BuildRouter constructs the router under the derived budget.
//
// It REFUSES a WarmResult that did not come from WaitForWarmEmbedder: cold
// sentence-transformer load must be paid before the measured region begins, or
// the budget is measuring warm-up again and the whole contract collapses.
//
// A budget expiry is reported as ErrBuildBudgetExceeded and names the embed
// throughput actually required, so the failure reads as a sidecar-performance
// verdict rather than as the bare "NewRouter: embed scenario … context deadline
// exceeded" that spec.md AC-1 forbids.
func BuildRouter(ctx context.Context, warm WarmResult, cfg agent.RoutingConfig, scenarios []*agent.Scenario, embedder agent.Embedder, t Timings) (agent.Router, error) {
	if !warm.warmed {
		return nil, fmt.Errorf("%w (call WaitForWarmEmbedder first)", ErrWarmupGateSkipped)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}

	calls := ExampleCount(scenarios)
	budget := BuildBudget(scenarios, t)
	buildCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	router, err := agent.NewRouter(buildCtx, cfg, scenarios, embedder)
	if err != nil {
		// Attribute the failure to the budget only when the budget context is
		// the thing that fired. An embedder error unrelated to the deadline
		// still surfaces verbatim.
		if buildCtx.Err() != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("%w: %d embed calls did not complete within %s (%d × %s from %s) — the embedder proved warm at %s beforehand (%s), so this is ML sidecar throughput, not routing: %v",
				ErrBuildBudgetExceeded, calls, budget, calls, t.BuildPerCallBudget, EnvBuildPerCallMs,
				warm.QualifyingLatency.Round(time.Millisecond), warm.Summary(), err)
		}
		return nil, err
	}
	return router, nil
}

// BuildReport renders the derived budget for evidence capture.
func BuildReport(scenarios []*agent.Scenario, t Timings) string {
	calls := ExampleCount(scenarios)
	return fmt.Sprintf("embed_calls=%d per_call_budget=%s (%s) derived_build_budget=%s",
		calls, t.BuildPerCallBudget, EnvBuildPerCallMs, BuildBudget(scenarios, t))
}

// isTimeout reports whether err is the deadline-shaped failure a cold
// sentence-transformer load produces: either a context deadline or a net
// timeout raised by http.Client's per-call Timeout.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isUnreachable reports whether the probe never reached the sidecar at all.
// A net.OpError (connection refused) or a DNS error means the stack is not up;
// waiting out the warm-up budget would report the wrong cause slowly.
func isUnreachable(err error) bool {
	if isTimeout(err) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
