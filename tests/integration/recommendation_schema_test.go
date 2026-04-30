//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendationSchema_RejectsUnknownCandidateBeforeDelivery(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	_, err := store.CreateReactiveRequest(context.Background(), recstore.ReactiveOutcomeInput{
		ActorUserID:                "integration-schema",
		Source:                     "api",
		ScenarioID:                 "recommendation_reactive",
		ScenarioVersion:            "recommendation-reactive-v1",
		ScenarioHash:               "recommendation-reactive-v1",
		RawInput:                   "quiet ramen near mission",
		ParsedRequest:              map[string]any{"query": "quiet ramen near mission"},
		LocationPrecisionRequested: "neighborhood",
		LocationPrecisionSent:      "neighborhood",
		Status:                     "delivered",
		ToolCalls: []recstore.ToolCallRecord{{
			Name:            "recommendation_rank_candidates",
			SideEffectClass: "read",
			Arguments:       map[string]any{"candidate_count": 0},
			Result:          map[string]any{"ranked_count": 1},
			StartedAt:       time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		}},
		Recommendations: []recstore.RecommendationInput{{
			CandidateLocalID: "cand-injected",
			RankPosition:     1,
			Status:           "delivered",
			StatusReason:     "eligible",
			ScoreBreakdown:   map[string]float64{"total": 1},
			Rationale:        []string{"Injected candidate"},
		}},
		StartedAt:   time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 4, 27, 12, 0, 1, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("CreateReactiveRequest delivered an unbacked recommendation; want rejection")
	}
	if !strings.Contains(err.Error(), "unknown candidate") {
		t.Fatalf("error = %v, want unknown candidate rejection", err)
	}
}
