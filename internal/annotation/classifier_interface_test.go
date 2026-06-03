// Spec 076 SCOPE-4b — TP-076-04b-02.
//
// Unit-level proof that:
//
//  1. The `Classifier` interface is implemented by every production
//     variant: InlineClassifier, *WarmCacheClassifier, *BridgeClassifier.
//  2. The warm-cache decorator returns the canonical InteractionType
//     for every cached token (warm-cache hit path) and falls through
//     to the inner Classifier otherwise (warm-cache miss path).
//  3. `annotation.classify.v1` is the production wiring's
//     scenario id: BridgeClassifier issues an `IntentEnvelope` whose
//     `ScenarioID` equals "annotation_classify" so the router takes
//     the BS-002 explicit-id fast path.
//
// No live LLM is exercised — the bridge runner is a scripted fake.
// TP-076-04b-01 / TP-076-04b-03 / TP-076-04b-04 cover the integration
// + e2e paths against the live stack.
package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// fakeRunner is the scripted BridgeRunner stand-in. Each invocation
// records the envelope it received and returns the scripted result.
type fakeRunner struct {
	gotEnv agent.IntentEnvelope
	result *agent.InvocationResult
}

func (f *fakeRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	f.gotEnv = env
	return f.result, &agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: env.ScenarioID}
}

func TestClassifierInterface_ImplementedByClassifyV1(t *testing.T) {
	// (1) Interface conformance — compile-time first, runtime second.
	var (
		_ Classifier = InlineClassifier{}
		_ Classifier = (*WarmCacheClassifier)(nil)
		_ Classifier = (*BridgeClassifier)(nil)
	)

	// (2) BridgeClassifier issues an envelope with the production
	// scenario id "annotation_classify" so the router takes the
	// BS-002 explicit-id fast path.
	finalBytes, err := json.Marshal(map[string]any{
		"interaction_type": "made_it",
		"confidence":       0.9,
		"rationale":        "user said 'made it'",
	})
	if err != nil {
		t.Fatalf("marshal final: %v", err)
	}
	runner := &fakeRunner{
		result: &agent.InvocationResult{
			ScenarioID: "annotation_classify",
			Outcome:    agent.OutcomeOK,
			Final:      finalBytes,
		},
	}
	bc := &BridgeClassifier{Runner: runner, ConfidenceFloor: 0.6}

	it, conf, err := bc.Classify(context.Background(), "made it", ChannelAPI)
	if err != nil {
		t.Fatalf("BridgeClassifier.Classify returned err: %v", err)
	}
	if it != InteractionMadeIt {
		t.Fatalf("BridgeClassifier.Classify InteractionType = %q, want %q", it, InteractionMadeIt)
	}
	if conf != 0.9 {
		t.Fatalf("BridgeClassifier.Classify confidence = %v, want 0.9", conf)
	}
	if runner.gotEnv.ScenarioID != "annotation_classify" {
		t.Fatalf("BridgeClassifier envelope ScenarioID = %q, want %q (explicit-id fast path)", runner.gotEnv.ScenarioID, "annotation_classify")
	}
	if runner.gotEnv.RawInput != "made it" {
		t.Fatalf("BridgeClassifier envelope RawInput = %q, want %q", runner.gotEnv.RawInput, "made it")
	}
	if string(runner.gotEnv.Source) != "api" {
		t.Fatalf("BridgeClassifier envelope Source = %q, want %q", runner.gotEnv.Source, "api")
	}

	// (3) Below-floor confidence returns ErrBelowConfidenceFloor with
	// EMPTY InteractionType so callers cannot accidentally consume a
	// guessed value.
	belowFinal, _ := json.Marshal(map[string]any{"interaction_type": "made_it", "confidence": 0.2})
	runner.result = &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: belowFinal}
	it, conf, err = bc.Classify(context.Background(), "made it", ChannelAPI)
	if !errors.Is(err, ErrBelowConfidenceFloor) {
		t.Fatalf("below-floor expected ErrBelowConfidenceFloor, got %v", err)
	}
	if it != "" {
		t.Fatalf("below-floor InteractionType = %q, want empty", it)
	}
	if conf != 0.2 {
		t.Fatalf("below-floor confidence = %v, want 0.2", conf)
	}

	// (4) Warm-cache hit short-circuits the inner classifier.
	innerCalled := 0
	innerSpy := classifierFunc(func(_ context.Context, _ string, _ SourceChannel) (InteractionType, float64, error) {
		innerCalled++
		return "", 0.0, errors.New("inner must not be called on warm-cache hit")
	})
	wc := NewWarmCacheClassifier(innerSpy, true)
	for token, want := range warmCacheTable {
		it, conf, err := wc.Classify(context.Background(), token, ChannelAPI)
		if err != nil {
			t.Fatalf("warm-cache hit %q returned err: %v", token, err)
		}
		if it != want {
			t.Fatalf("warm-cache hit %q InteractionType = %q, want %q", token, it, want)
		}
		if conf != 1.0 {
			t.Fatalf("warm-cache hit %q confidence = %v, want 1.0", token, conf)
		}
	}
	if innerCalled != 0 {
		t.Fatalf("warm-cache hit must not call inner; got %d calls", innerCalled)
	}

	// (5) Warm-cache miss delegates to inner verbatim (including
	// inner errors — no silent fallback).
	innerCalled = 0
	miss := classifierFunc(func(_ context.Context, _ string, _ SourceChannel) (InteractionType, float64, error) {
		innerCalled++
		return InteractionTriedIt, 0.85, nil
	})
	wc = NewWarmCacheClassifier(miss, true)
	it, conf, err = wc.Classify(context.Background(), "I tried it last night", ChannelAPI)
	if err != nil || it != InteractionTriedIt || conf != 0.85 {
		t.Fatalf("warm-cache miss delegation failed: it=%q conf=%v err=%v", it, conf, err)
	}
	if innerCalled != 1 {
		t.Fatalf("warm-cache miss must invoke inner exactly once; got %d", innerCalled)
	}

	// (6) Warm cache disabled: cache is bypassed even for cached tokens.
	innerCalled = 0
	wc = NewWarmCacheClassifier(miss, false)
	if _, _, err := wc.Classify(context.Background(), "made it", ChannelAPI); err != nil {
		t.Fatalf("disabled warm-cache delegation err: %v", err)
	}
	if innerCalled != 1 {
		t.Fatalf("disabled warm-cache must always delegate; got %d", innerCalled)
	}
}

// classifierFunc is a test-only adapter so the test can pass a
// closure where a Classifier is required.
type classifierFunc func(context.Context, string, SourceChannel) (InteractionType, float64, error)

func (f classifierFunc) Classify(ctx context.Context, text string, ch SourceChannel) (InteractionType, float64, error) {
	return f(ctx, text, ch)
}
