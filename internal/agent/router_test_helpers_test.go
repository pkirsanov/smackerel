package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
)

// recordingEmbedder is a deterministic Embedder for router tests.
// It looks up the input text in a fixture map and returns the
// canned vector. Unknown inputs return a zero vector of the same
// dimension as the first canned vector — surfaces of the test pin
// that down via assertions on calls() and the returned RoutingDecision.
//
// All call counters are atomic so tests can run with `-race` and still
// assert deterministic call counts on the explicit-id (no-embed) path.
type recordingEmbedder struct {
	vectors map[string][]float32
	calls   atomic.Int64
	dim     int
}

func newRecordingEmbedder(vectors map[string][]float32) *recordingEmbedder {
	dim := 0
	for _, v := range vectors {
		dim = len(v)
		break
	}
	return &recordingEmbedder{vectors: vectors, dim: dim}
}

func (e *recordingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls.Add(1)
	if v, ok := e.vectors[text]; ok {
		out := make([]float32, len(v))
		copy(out, v)
		return out, nil
	}
	// Default to a zero vector of the same dimension so the router can
	// still compute a (zero) cosine without dimension mismatch.
	return make([]float32, e.dim), nil
}

func (e *recordingEmbedder) Calls() int64 { return e.calls.Load() }

// makeScenario builds a *Scenario with just the fields the router cares
// about: ID and IntentExamples. The router does not touch any other
// field. This bypasses the YAML loader so router tests stay focused on
// routing logic rather than re-exercising loader behavior.
func makeScenario(id string, examples ...string) *Scenario {
	return &Scenario{
		ID:             id,
		IntentExamples: examples,
		// The remaining fields are unused by the router but populated
		// to non-zero values so any future router code that
		// inadvertently dereferences them does not panic.
		Version:         id + "-v1",
		Description:     id + " test scenario",
		SystemPrompt:    "test",
		AllowedTools:    []AllowedTool{{Name: id + "_tool", SideEffectClass: SideEffectRead}},
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		Limits:          ScenarioLimits{MaxLoopIterations: 1, TimeoutMs: 1, SchemaRetryBudget: 0, PerToolTimeoutMs: 1},
		SideEffectClass: SideEffectRead,
	}
}

// defaultRoutingCfg is the router config used by every test that does
// not need to override a specific field. The values are the ones the
// production config/smackerel.yaml ships with at the time these tests
// were written, so behavior under test mirrors the live runtime.
func defaultRoutingCfg() RoutingConfig {
	return RoutingConfig{
		ConfidenceFloor: 0.65,
		ConsiderTopN:    5,
	}
}

// newTestRouter wraps NewRouter with t.Fatal on construction error.
func newTestRouter(t *testing.T, cfg RoutingConfig, scenarios []*Scenario, e Embedder) Router {
	t.Helper()
	r, err := NewRouter(context.Background(), cfg, scenarios, e)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}
