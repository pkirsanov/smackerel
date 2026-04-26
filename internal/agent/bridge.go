// Spec 037 Scope 10 — agent.Bridge wires the loader, router, and
// executor into a single object that surfaces (api, telegram,
// scheduler, pipeline) call into via a uniform Invoke / KnownIntents
// contract.
//
// Why a Bridge type and not direct *Router + *Executor injection at
// every surface?
//
//  1. Surfaces should not know the order of route → execute. The bridge
//     owns that ordering once and applies the same policy everywhere
//     (BS-014 never-invent + BS-001 zero-Go-change scenario adds).
//  2. Hot reload (BS-019) replaces the router and the cached scenario
//     set atomically; surfaces hold a stable *Bridge pointer across
//     reloads instead of needing to chase router pointer swaps.
//  3. The bridge is a single dependency the wiring layer constructs and
//     hands to api.AgentInvokeHandler.Runner and telegram.NewAgentBridge,
//     so production wiring is one allocation, not N.
//
// Reload semantics (BS-019):
//
//   - Reload reads the scenario directory, builds a fresh router, and
//     atomically swaps the new router + scenario id list under a write
//     lock. In-flight Invoke calls already past the read-lock acquisition
//     hold their own *Scenario pointer (the executor pins it for the
//     loop's lifetime), so a swap mid-flight does NOT mutate the running
//     invocation. The hot-reload guarantee is enforced in the executor;
//     Bridge only owns the swap mechanism.
//   - Reload is safe under concurrent Invoke: the new router is built
//     OFF the lock, then installed under the write lock in O(1) so reads
//     are not blocked during the embed work.
//
// The bridge has no logging or metrics of its own — those belong to the
// individual surfaces and to the tracer.

package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// NoopEmbedder returns a fixed unit vector for every text. The router
// will compute identical similarity scores for every scenario, so the
// effective behavior with NoopEmbedder is "explicit-id route only" —
// any free-text input falls below the configured confidence floor and
// either selects the fallback scenario or returns unknown-intent.
//
// Production wiring should replace this with a NATS-backed embedder
// once specs 034/035/036 land their first scenarios with similarity-
// based intent_examples. Until then NoopEmbedder lets the bridge boot
// without a hard dependency on the ML sidecar embedding subject and
// preserves BS-002 (explicit-id fast path makes no embed call).
type NoopEmbedder struct{}

// Embed implements Embedder. The vector is non-empty so router
// construction (which validates dim consistency) succeeds, but the
// fixed value guarantees identical scores across every scenario.
func (NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{1.0}, nil
}

// Bridge composes the loader, router, and executor. Construct one per
// process via NewBridge and share it across goroutines — Invoke is
// safe under any number of concurrent callers.
type Bridge struct {
	cfg          *Config
	scenarioDir  string
	scenarioGlob string
	loader       Loader
	embedder     Embedder
	executor     *Executor

	mu        sync.RWMutex
	router    Router
	scenIDs   []string // sorted ascending; the cached KnownIntents return value
	scenarios []*Scenario
}

// BridgeOptions carries the immutable wiring inputs for NewBridge.
// Every field is required EXCEPT Loader (DefaultLoader is substituted
// when nil) and Embedder (NoopEmbedder is substituted when nil — see
// the type comment for the operational implications).
type BridgeOptions struct {
	Config   *Config
	Loader   Loader    // optional; defaults to DefaultLoader()
	Embedder Embedder  // optional; defaults to NoopEmbedder{}
	Executor *Executor // required
}

// NewBridge constructs a Bridge and performs the initial scenario load
// + router build. Returns an error if scenario loading is fatal (e.g.,
// duplicate scenario id) or router construction fails. A non-fatal load
// (some files rejected, some registered) is allowed; the rejected files
// are exposed via LoaderRejections() so callers can log them.
//
// The first invocation can happen immediately after NewBridge returns
// nil error.
func NewBridge(ctx context.Context, opts BridgeOptions) (*Bridge, []LoadError, error) {
	if opts.Config == nil {
		return nil, nil, errors.New("agent.NewBridge: Config is required")
	}
	if opts.Executor == nil {
		return nil, nil, errors.New("agent.NewBridge: Executor is required")
	}
	loader := opts.Loader
	if loader == nil {
		loader = DefaultLoader()
	}
	embedder := opts.Embedder
	if embedder == nil {
		embedder = NoopEmbedder{}
	}
	b := &Bridge{
		cfg:          opts.Config,
		scenarioDir:  opts.Config.ScenarioDir,
		scenarioGlob: opts.Config.ScenarioGlob,
		loader:       loader,
		embedder:     embedder,
		executor:     opts.Executor,
	}
	rejected, err := b.Reload(ctx)
	if err != nil {
		return nil, rejected, err
	}
	return b, rejected, nil
}

// Reload reads the scenario directory fresh and atomically installs a
// new router + scenario id list. Safe under concurrent Invoke calls;
// see the package doc for the BS-019 hot-reload semantics.
//
// Returns the per-file rejection list so the operator can log which
// scenarios failed to load on this reload. A fatal error (duplicate
// id, embedder failure, etc.) leaves the existing router in place and
// is surfaced as the second return value.
func (b *Bridge) Reload(ctx context.Context) ([]LoadError, error) {
	registered, rejected, fatal := b.loader.Load(b.scenarioDir, b.scenarioGlob)
	if fatal != nil {
		return rejected, fmt.Errorf("agent.Bridge.Reload: loader fatal: %w", fatal)
	}
	router, err := NewRouter(ctx, b.cfg.Routing, registered, b.embedder)
	if err != nil {
		return rejected, fmt.Errorf("agent.Bridge.Reload: build router: %w", err)
	}

	ids := make([]string, 0, len(registered))
	for _, sc := range registered {
		ids = append(ids, sc.ID)
	}
	sort.Strings(ids)

	b.mu.Lock()
	b.router = router
	b.scenIDs = ids
	b.scenarios = registered
	b.mu.Unlock()
	return rejected, nil
}

// KnownIntents returns the sorted list of currently-registered scenario
// ids. Surfaces use this to populate the unknown-intent reply with the
// candidate set the router could have chosen. The returned slice is a
// defensive copy; callers may mutate it freely.
func (b *Bridge) KnownIntents() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.scenIDs))
	copy(out, b.scenIDs)
	return out
}

// Scenarios returns the currently-loaded scenarios. Test-only; the
// production surfaces should use KnownIntents and let the router pick
// the scenario.
func (b *Bridge) Scenarios() []*Scenario {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]*Scenario, len(b.scenarios))
	copy(out, b.scenarios)
	return out
}

// Invoke is the single entry point every surface (api, telegram,
// scheduler, pipeline) calls. It:
//
//  1. Routes env via the currently-installed router.
//  2. On unknown-intent, returns a structured InvocationResult with
//     OutcomeUnknownIntent (no executor call) so surfaces never invent
//     an answer (BS-014).
//  3. Otherwise pins the chosen *Scenario, copies the routing decision
//     into the envelope (so the tracer records why the scenario was
//     chosen), and runs the executor.
//
// The pinned scenario pointer is captured BEFORE Invoke releases the
// read lock, so a concurrent Reload that swaps the router does not
// affect this invocation (BS-019).
func (b *Bridge) Invoke(ctx context.Context, env IntentEnvelope) (*InvocationResult, *RoutingDecision) {
	b.mu.RLock()
	router := b.router
	b.mu.RUnlock()

	if router == nil {
		// Pre-Reload state shouldn't happen in production (NewBridge
		// reloads in its constructor) but defend against it: return a
		// structured outcome rather than panicking.
		decision := RoutingDecision{Reason: ReasonUnknownIntent}
		return &InvocationResult{
			Outcome:       OutcomeUnknownIntent,
			OutcomeDetail: map[string]any{"error": "agent_bridge_not_loaded"},
		}, &decision
	}

	chosen, decision, ok := router.Route(ctx, env)
	if !ok {
		return &InvocationResult{
			ScenarioID: env.ScenarioID, // empty when the explicit id was unknown
			Outcome:    OutcomeUnknownIntent,
			OutcomeDetail: map[string]any{
				"reason":     string(decision.Reason),
				"top_score":  decision.TopScore,
				"threshold":  decision.Threshold,
				"considered": decision.Considered,
				"requested":  env.ScenarioID,
				"known":      b.KnownIntents(),
			},
		}, &decision
	}

	env.Routing = decision
	res := b.executor.Run(ctx, chosen, env)
	return res, &decision
}
