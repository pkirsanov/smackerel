// Spec 061 SCOPE-04 — borderline post-processor golden table.

package assistant

import (
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestBorderlineGoldenTable(t *testing.T) {
	t.Parallel()

	const (
		agentFloor      = 0.50
		borderlineFloor = 0.75
	)

	cases := []struct {
		name            string
		decision        agent.RoutingDecision
		ok              bool
		borderlineFloor float64
		agentFloor      float64
		want            Band
	}{
		{
			name:            "high band — TopScore well above borderline floor",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.91},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandHigh,
		},
		{
			name:            "high band — TopScore exactly at borderline floor (inclusive)",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.75},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandHigh,
		},
		{
			name:            "borderline band — TopScore between floors",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.65},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandBorderline,
		},
		{
			name:            "borderline band — TopScore exactly at agent floor (inclusive of borderline)",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.50},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandBorderline,
		},
		{
			name:            "borderline band — TopScore just below borderline floor",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.7499},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandBorderline,
		},
		{
			name:            "low band — TopScore strictly below agent floor",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.49},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandLow,
		},
		{
			name:            "low band — TopScore zero",
			decision:        agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, TopScore: 0.0},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandLow,
		},
		{
			name:            "low band — !ok overrides high TopScore",
			decision:        agent.RoutingDecision{Reason: agent.ReasonUnknownIntent, TopScore: 0.95},
			ok:              false,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandLow,
		},
		{
			name:            "low band — ReasonUnknownIntent with ok=true (defensive) still demotes",
			decision:        agent.RoutingDecision{Reason: agent.ReasonUnknownIntent, TopScore: 0.95},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandLow,
		},
		{
			name:            "high band — fallback_clarify with high TopScore is honoured",
			decision:        agent.RoutingDecision{Reason: agent.ReasonFallbackClarify, TopScore: 0.85},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandHigh,
		},
		{
			// Spec 061 Round 52 — adversarial test for Defect 2 (BS-002
			// borderline-low for explicit-id). Uses PRODUCTION floors so
			// TopScore=0 would otherwise fall below agentFloor and return
			// BandLow. The ReasonExplicitScenarioID special case in
			// borderline.go MUST short-circuit that path.
			name:            "high band — explicit_scenario_id with TopScore zero at production floors (router fast path bypasses scoring)",
			decision:        agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, TopScore: 0.0},
			ok:              true,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandHigh,
		},
		{
			// Spec 061 Round 52 — defensive ordering test: !ok MUST still
			// override ReasonExplicitScenarioID. A future refactor that
			// places the explicit-id branch BEFORE the !ok guard would
			// fail this case.
			name:            "low band — explicit_scenario_id with !ok still demotes (!ok precedence)",
			decision:        agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, TopScore: 0.0},
			ok:              false,
			borderlineFloor: borderlineFloor,
			agentFloor:      agentFloor,
			want:            BandLow,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Borderline(tc.decision, tc.ok, tc.borderlineFloor, tc.agentFloor)
			if got != tc.want {
				t.Errorf("Borderline(reason=%s,top=%.4f,ok=%v,bf=%.4f,af=%.4f) = %q; want %q",
					tc.decision.Reason, tc.decision.TopScore, tc.ok, tc.borderlineFloor, tc.agentFloor, got, tc.want)
			}
		})
	}
}

// TestBorderlineBandClosedVocabulary asserts that every Band literal
// declared in borderline.go appears in AllBands exactly once.
func TestBorderlineBandClosedVocabulary(t *testing.T) {
	t.Parallel()
	want := map[Band]int{BandHigh: 1, BandBorderline: 1, BandLow: 1}
	got := map[Band]int{}
	for _, b := range AllBands {
		got[b]++
	}
	if len(got) != len(want) {
		t.Fatalf("AllBands cardinality mismatch: got %v, want %v", got, want)
	}
	for b, n := range want {
		if got[b] != n {
			t.Errorf("AllBands[%s] = %d; want %d", b, got[b], n)
		}
	}
}
