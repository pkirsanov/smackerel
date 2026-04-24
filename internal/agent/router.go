// Intent router for spec 037 Scope 4.
//
// The router selects a scenario for an incoming IntentEnvelope using one
// of two paths:
//
//  1. Explicit ScenarioID — the envelope names a scenario id directly.
//     The router looks the scenario up by id and short-circuits. NO
//     embedding call is made on this path (BS-002 fast path).
//
//  2. Similarity routing — the router embeds envelope.RawInput once,
//     then ranks every registered scenario by the maximum cosine
//     similarity between the input embedding and the embeddings of the
//     scenario's intent_examples (which were precomputed at router
//     construction). The top scenario is chosen iff its score is at
//     or above the effective confidence floor; otherwise the router
//     falls back to the configured fallback scenario, or returns
//     unknown-intent if no fallback is configured.
//
// REGEX, switch-on-input, keyword maps, and hardcoded vendor lists are
// FORBIDDEN in this file and anywhere else under internal/agent/. The
// guard at tests/integration/agent/forbidden_pattern_test.go enforces
// that mechanically (BS-014 indirectly, via the linter discipline).
//
// The router is fully deterministic for a fixed Embedder and scenario
// set: the cached scenario list is sorted by id, scores are computed in
// that order, and ties at the top break by id ascending.

package agent

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// IntentEnvelope is the typed surface input handed to the router and
// executor. Surfaces (telegram, api, scheduler, pipeline) construct one
// of these per incoming intent.
type IntentEnvelope struct {
	Source            string  // "telegram" | "api" | "scheduler" | "pipeline"
	RawInput          string  // free-text intent (may be empty for system triggers)
	StructuredContext []byte  // surface-specific structured context (opaque to router)
	ScenarioID        string  // optional explicit override; bypasses similarity routing
	ConfidenceFloor   float64 // optional override of cfg default; 0 means "use cfg"

	// Routing is the decision the router produced for this envelope.
	// Populated by the surface AFTER router.Route succeeds and BEFORE
	// executor.Run. The executor passes it through to the tracer so the
	// trace row records why this scenario was chosen (Scope 6).
	Routing RoutingDecision `json:"routing"`
}

// RouteReason names the path the router took. The trace records this so
// the operator can see why a scenario was chosen (or why none was).
type RouteReason string

const (
	// ReasonExplicitScenarioID — envelope named a scenario id and it
	// existed in the registered set. No embedding call was made.
	ReasonExplicitScenarioID RouteReason = "explicit_scenario_id"
	// ReasonSimilarityMatch — the top similarity score met or exceeded
	// the effective confidence floor.
	ReasonSimilarityMatch RouteReason = "similarity_match"
	// ReasonFallbackClarify — the top similarity score was below floor
	// and the configured fallback scenario was selected.
	ReasonFallbackClarify RouteReason = "fallback_clarify"
	// ReasonUnknownIntent — explicit id was unknown, OR top score below
	// floor with no usable fallback. The router returns ok=false; the
	// executor surfaces outcome class "unknown-intent".
	ReasonUnknownIntent RouteReason = "unknown_intent"
)

// CandidateScore is one row in the considered list. The router records
// every scenario it evaluated (truncated to cfg.routing.consider_top_n)
// so the trace can show what was rejected and by how much.
type CandidateScore struct {
	ScenarioID string  `json:"scenario_id"`
	Score      float64 `json:"score"`
}

// RoutingDecision is the structured outcome of a single Route call.
// Every field is recorded on the trace; the executor consults Reason
// and Chosen to decide what to do next.
type RoutingDecision struct {
	Reason     RouteReason      `json:"reason"`
	Chosen     string           `json:"chosen"`     // empty when ok=false
	TopScore   float64          `json:"top_score"`  // 0 when no similarity ran
	Threshold  float64          `json:"threshold"`  // effective confidence floor
	Considered []CandidateScore `json:"considered"` // top-N by descending score
}

// Embedder produces a vector for a free-text input. The router calls
// Embed exactly once per Route invocation that takes the similarity
// path — never on the explicit-id path. Implementations MUST return
// vectors of consistent dimension across calls; the router does not
// rebuild caches mid-flight.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Router selects a scenario for an envelope. Implementations MUST be
// safe for concurrent use after construction; the executor calls Route
// from many goroutines.
type Router interface {
	Route(ctx context.Context, env IntentEnvelope) (chosen *Scenario, decision RoutingDecision, ok bool)
}

// scenarioEmbedding is the cached precomputed embedding set for one
// scenario's intent_examples. The router holds these immutably for the
// lifetime of the router; hot reload constructs a new router.
type scenarioEmbedding struct {
	scenario   *Scenario
	exampleVec [][]float32 // one vector per non-empty intent_example
}

// router is the default Router implementation.
type router struct {
	cfg       RoutingConfig
	embedder  Embedder
	byID      map[string]*Scenario
	withExmpl []scenarioEmbedding // scenarios that have at least one example, sorted by id
}

// NewRouter constructs a router. It precomputes embeddings for every
// scenario's intent_examples by calling embedder.Embed once per
// example. A scenario with zero intent_examples is registered for
// explicit-id lookup and fallback selection but is NEVER a candidate
// for similarity routing (so it cannot accidentally "win" on a low
// score over scenarios that did declare examples).
//
// Returns an error if any embedding call fails — the operator must
// fix the embedder (or the scenario examples) before the router can
// safely make routing decisions.
func NewRouter(ctx context.Context, cfg RoutingConfig, scenarios []*Scenario, embedder Embedder) (Router, error) {
	if embedder == nil {
		return nil, fmt.Errorf("agent: NewRouter requires a non-nil Embedder")
	}
	if cfg.ConsiderTopN < 1 {
		return nil, fmt.Errorf("agent: NewRouter requires routing.consider_top_n >= 1, got %d", cfg.ConsiderTopN)
	}

	r := &router{
		cfg:      cfg,
		embedder: embedder,
		byID:     make(map[string]*Scenario, len(scenarios)),
	}

	// Sort by id for deterministic iteration order. Stable so ties at
	// the top of the ranking break by id ascending.
	sorted := append([]*Scenario(nil), scenarios...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for _, sc := range sorted {
		if sc == nil {
			continue
		}
		if _, dup := r.byID[sc.ID]; dup {
			return nil, fmt.Errorf("agent: NewRouter: duplicate scenario id %q", sc.ID)
		}
		r.byID[sc.ID] = sc

		if len(sc.IntentExamples) == 0 {
			continue
		}
		vecs := make([][]float32, 0, len(sc.IntentExamples))
		for i, ex := range sc.IntentExamples {
			v, err := embedder.Embed(ctx, ex)
			if err != nil {
				return nil, fmt.Errorf("agent: NewRouter: embed scenario %q intent_examples[%d]: %w", sc.ID, i, err)
			}
			if len(v) == 0 {
				return nil, fmt.Errorf("agent: NewRouter: embed scenario %q intent_examples[%d] returned empty vector", sc.ID, i)
			}
			vecs = append(vecs, v)
		}
		r.withExmpl = append(r.withExmpl, scenarioEmbedding{scenario: sc, exampleVec: vecs})
	}

	return r, nil
}

// Route implements the design §4.1 decision table.
func (r *router) Route(ctx context.Context, env IntentEnvelope) (*Scenario, RoutingDecision, bool) {
	threshold := r.cfg.ConfidenceFloor
	if env.ConfidenceFloor > 0 {
		threshold = env.ConfidenceFloor
	}

	// Path 1: explicit scenario id — short-circuit, no embedding call.
	if env.ScenarioID != "" {
		if sc, ok := r.byID[env.ScenarioID]; ok {
			return sc, RoutingDecision{
				Reason:    ReasonExplicitScenarioID,
				Chosen:    sc.ID,
				Threshold: threshold,
			}, true
		}
		return nil, RoutingDecision{
			Reason:    ReasonUnknownIntent,
			Threshold: threshold,
		}, false
	}

	// Path 2: similarity routing. If there are no scenarios with
	// intent_examples, similarity routing cannot succeed; consult the
	// fallback before declaring unknown-intent.
	if len(r.withExmpl) == 0 {
		return r.fallbackOrUnknown(threshold, nil, 0)
	}

	vec, err := r.embedder.Embed(ctx, env.RawInput)
	if err != nil {
		// Embedder failure is treated as unknown-intent at the router
		// boundary; the executor surfaces the structured outcome and
		// the trace records the absent score set. This deliberately
		// does NOT silently fall back — the operator should see the
		// embedder failure in tracing, not a wrong "fallback hit".
		return nil, RoutingDecision{
			Reason:    ReasonUnknownIntent,
			Threshold: threshold,
		}, false
	}

	// Score every scenario as max(cosine(vec, example)) over its
	// cached example vectors. Skip scenarios whose dimension does not
	// match the input vector (defensive: should not happen if the
	// embedder is consistent, but if it does we surface no false
	// positives rather than crash).
	scored := make([]CandidateScore, 0, len(r.withExmpl))
	for _, s := range r.withExmpl {
		best := math.Inf(-1)
		matched := false
		for _, ev := range s.exampleVec {
			if len(ev) != len(vec) {
				continue
			}
			cs := cosine(vec, ev)
			if cs > best {
				best = cs
			}
			matched = true
		}
		if !matched {
			continue
		}
		scored = append(scored, CandidateScore{ScenarioID: s.scenario.ID, Score: best})
	}

	// Sort by descending score; tie-break by ascending id (stable on
	// already-sorted-by-id slice keeps determinism).
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].ScenarioID < scored[j].ScenarioID
	})

	considered := truncate(scored, r.cfg.ConsiderTopN)

	if len(scored) == 0 {
		return r.fallbackOrUnknown(threshold, considered, 0)
	}

	top := scored[0]
	if top.Score >= threshold {
		sc := r.byID[top.ScenarioID]
		return sc, RoutingDecision{
			Reason:     ReasonSimilarityMatch,
			Chosen:     sc.ID,
			TopScore:   top.Score,
			Threshold:  threshold,
			Considered: considered,
		}, true
	}

	return r.fallbackOrUnknown(threshold, considered, top.Score)
}

// fallbackOrUnknown returns the configured fallback if and only if the
// scenario id is set AND resolves to a registered scenario. Otherwise
// it returns ok=false with reason unknown_intent. This is the ONLY path
// by which "fallback_clarify" can be returned, ensuring the trace
// distinguishes "we routed to clarify" from "we could not route at all".
func (r *router) fallbackOrUnknown(threshold float64, considered []CandidateScore, topScore float64) (*Scenario, RoutingDecision, bool) {
	if r.cfg.FallbackScenarioID != "" {
		if sc, ok := r.byID[r.cfg.FallbackScenarioID]; ok {
			return sc, RoutingDecision{
				Reason:     ReasonFallbackClarify,
				Chosen:     sc.ID,
				TopScore:   topScore,
				Threshold:  threshold,
				Considered: considered,
			}, true
		}
	}
	return nil, RoutingDecision{
		Reason:     ReasonUnknownIntent,
		TopScore:   topScore,
		Threshold:  threshold,
		Considered: considered,
	}, false
}

// cosine returns the cosine similarity of two equal-length vectors.
// Zero-magnitude inputs return 0 (not NaN) so they cannot accidentally
// out-rank legitimate matches.
func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func truncate(s []CandidateScore, n int) []CandidateScore {
	if n <= 0 || len(s) <= n {
		return append([]CandidateScore(nil), s...)
	}
	return append([]CandidateScore(nil), s[:n]...)
}
