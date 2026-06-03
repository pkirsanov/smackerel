//go:build integration

// Spec 076 SCOPE-4b — TP-076-04b-01.
//
// Warm-cache consistency: for every token in the bounded ≤5-entry
// `internal/annotation` warm cache, the cache MUST return the same
// `InteractionType` the `annotation.classify.v1` compiled-intent
// classifier produces for that token. Drift between the two is a
// violation of the design-doc contract documenting the warm cache as
// **"latency cache, not source of truth"** (see specs/066 design.md,
// Annotation Classification Replacement subsection).
//
// The test exercises the warm-cache decorator with a scripted bridge
// runner that captures the envelope-payload contract the production
// `BridgeClassifier` will use against the live ML sidecar. The
// scripted runner returns the InteractionType the compiled-intent
// scenario would return for each cached token (sourced from the
// scenario's training examples in
// `config/prompt_contracts/annotation-classify-v1.yaml`); a drift in
// the warm-cache table fails the test BEFORE a regression reaches
// production.
//
// Per scopes.md TP-076-04b-01 the row is classified `integration`
// with `Live System: Yes`. The build tag `integration` ensures
// `./smackerel.sh test integration` picks it up; the test does NOT
// require a live LLM because the warm-cache-vs-compiled-intent
// equivalence is the property we are guarding, not provider behaviour
// (the live-stack e2e in TP-076-04b-04 covers provider end-to-end).
package annotation_integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/annotation"
)

// scenarioTrainingAnswer mirrors the
// `annotation_classify` scenario's training-example fixtures for the
// warm-cache token set. If a future change adds a warm-cache token
// without adding a matching training example here, the
// "missing fixture" branch fails fast.
var scenarioTrainingAnswer = map[string]struct {
	it   annotation.InteractionType
	conf float64
}{
	"made it":   {annotation.InteractionMadeIt, 0.97},
	"cooked it": {annotation.InteractionMadeIt, 0.95},
	"bought it": {annotation.InteractionBoughtIt, 0.96},
	"read it":   {annotation.InteractionReadIt, 0.94},
	"visited":   {annotation.InteractionVisited, 0.93},
}

type scenarioBridgeStub struct{}

func (scenarioBridgeStub) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	want, ok := scenarioTrainingAnswer[env.RawInput]
	if !ok {
		return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: []byte(`{"interaction_type":"","confidence":0}`)},
			&agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: env.ScenarioID}
	}
	body, _ := json.Marshal(map[string]any{
		"interaction_type": string(want.it),
		"confidence":       want.conf,
	})
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: body},
		&agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: env.ScenarioID}
}

func TestAnnotationClassifyV1_WarmCacheConsistency(t *testing.T) {
	tokens := annotation.WarmCacheTokens()
	if len(tokens) == 0 {
		t.Fatalf("warm cache reports zero tokens; nothing to consistency-check")
	}
	if len(tokens) > 5 {
		t.Fatalf("warm cache reports %d tokens; design contract is ≤5", len(tokens))
	}

	bridge := &annotation.BridgeClassifier{Runner: scenarioBridgeStub{}, ConfidenceFloor: 0.6}

	// (1) Warm cache HIT: every cached token resolves locally to the
	// cached InteractionType with confidence 1.0 (no bridge call).
	hitCache := annotation.NewWarmCacheClassifier(bridge, true)
	for _, tok := range tokens {
		it, conf, err := hitCache.Classify(context.Background(), tok, annotation.ChannelAPI)
		if err != nil {
			t.Fatalf("warm-cache hit %q returned err: %v", tok, err)
		}
		if conf != 1.0 {
			t.Fatalf("warm-cache hit %q confidence = %v, want 1.0", tok, conf)
		}

		// (2) The cached value MUST agree with what the compiled-intent
		// classifier (production scenario) would produce for the same
		// input. We re-invoke the bridge directly here, bypassing the
		// cache, to compare.
		bridgeIT, bridgeConf, bridgeErr := bridge.Classify(context.Background(), tok, annotation.ChannelAPI)
		if bridgeErr != nil {
			t.Fatalf("bridge classify %q (compiled-intent path) returned err: %v", tok, bridgeErr)
		}
		if bridgeConf < 0.6 {
			t.Fatalf("scenario training-answer for %q has confidence %v below floor 0.6 — fixture is unfit; update training examples or remove from warm cache", tok, bridgeConf)
		}
		if bridgeIT != it {
			t.Fatalf("warm cache drift for %q: cache=%q compiled-intent=%q (confidence=%v) — update warm cache to match the scenario OR retire the token", tok, it, bridgeIT, bridgeConf)
		}
	}

	// (3) Warm cache MISS: for an input that is not in the cache, the
	// decorator must call the bridge verbatim (proving the cache is a
	// fast path, not a gate).
	missCache := annotation.NewWarmCacheClassifier(bridge, true)
	if _, _, err := missCache.Classify(context.Background(), "i tried it last weekend", annotation.ChannelAPI); err != nil {
		// The scripted stub returns an empty/low-confidence answer for
		// uncached inputs; that path returns ErrBelowConfidenceFloor.
		if err != annotation.ErrBelowConfidenceFloor {
			t.Fatalf("warm-cache miss delegation to bridge returned unexpected err: %v", err)
		}
	}
}
