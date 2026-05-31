package agent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"
)

// BUG-061-004 — verifies that with an input-dependent Embedder
// (real cosine similarity), distinct user inputs route to distinct
// scenarios and the alphabetical-tie-break does NOT fire when scores
// genuinely differ.

type hashEmbedder struct{}

func (hashEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	const dim = 16
	out := make([]float32, dim)
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == ' ' {
			if i > start {
				word := text[start:i]
				h := sha256.Sum256([]byte(word))
				for k := 0; k < dim; k++ {
					b := binary.BigEndian.Uint16(h[k*2 : k*2+2])
					out[k] += float32(b) / 65535.0
				}
			}
			start = i + 1
		}
	}
	var norm float64
	for _, v := range out {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		inv := float32(1.0 / math.Sqrt(norm))
		for i := range out {
			out[i] *= inv
		}
	}
	return out, nil
}

func TestRouter_DistinctInputsRouteToDistinctScenarios(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("aaaa_pretender", "unrelated content about astronomy and stargazing"),
		makeScenario("recipe_search", "find best recipe", "show me a dinner recipe", "cooking recipe ideas"),
		makeScenario("weather_query", "what is the weather", "today forecast", "is it raining"),
	}
	cfg := RoutingConfig{ConfidenceFloor: 0.20, ConsiderTopN: 5}
	router := newTestRouter(t, cfg, scenarios, hashEmbedder{})

	cases := []struct {
		input   string
		wantID  string
		notWant string
	}{
		{"find best recipe", "recipe_search", "aaaa_pretender"},
		{"what is the weather", "weather_query", "aaaa_pretender"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			sc, dec, ok := router.Route(context.Background(), IntentEnvelope{RawInput: tc.input})
			if !ok {
				t.Fatalf("Route(%q) ok=false; decision=%+v", tc.input, dec)
			}
			if sc.ID != tc.wantID {
				t.Errorf("Route(%q) chose %q; want %q (decision=%+v)", tc.input, sc.ID, tc.wantID, dec)
			}
			if sc.ID == tc.notWant {
				t.Errorf("Route(%q) routed to alphabetical-tie-break winner %q; embedder is being ignored", tc.input, tc.notWant)
			}
			if dec.Reason != ReasonSimilarityMatch {
				t.Errorf("Route(%q) reason=%q; want %q", tc.input, dec.Reason, ReasonSimilarityMatch)
			}
		})
	}
}

func TestRouter_NoopEmbedderProducesAlphabeticalTieBreak(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("aaaa_pretender", "unrelated"),
		makeScenario("recipe_search", "find best recipe"),
		makeScenario("weather_query", "what is the weather"),
	}
	cfg := RoutingConfig{ConfidenceFloor: 0.5, ConsiderTopN: 5}
	router := newTestRouter(t, cfg, scenarios, NoopEmbedder{})

	for _, input := range []string{"find best recipe", "what is the weather", "anything at all"} {
		sc, dec, ok := router.Route(context.Background(), IntentEnvelope{RawInput: input})
		if !ok {
			t.Fatalf("Route(%q) ok=false under NoopEmbedder", input)
		}
		if sc.ID != "aaaa_pretender" {
			t.Errorf("Route(%q) chose %q under NoopEmbedder; expected alphabetical winner aaaa_pretender (top_score=%v)", input, sc.ID, dec.TopScore)
		}
		if dec.TopScore < 0.999 {
			t.Errorf("Route(%q) top_score=%v under NoopEmbedder; expected 1.0", input, dec.TopScore)
		}
	}
}
