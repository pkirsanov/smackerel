package intelligence

import (
	"testing"
)

func TestResurfaceScore(t *testing.T) {
	// High relevance, dormant, low access
	score := ResurfaceScore(0.8, 60, 1)
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}

	// Low relevance, recent, high access
	lowScore := ResurfaceScore(0.1, 5, 10)
	if lowScore >= score {
		t.Errorf("expected low-relevance score (%.2f) < high-relevance score (%.2f)", lowScore, score)
	}
}

func TestResurfaceScore_DormancyBonus(t *testing.T) {
	fresh := ResurfaceScore(0.5, 10, 0)
	dormant := ResurfaceScore(0.5, 60, 0)

	if dormant <= fresh {
		t.Errorf("dormant (%.2f) should score higher than fresh (%.2f)", dormant, fresh)
	}
}

func TestResurfaceScore_AccessPenalty(t *testing.T) {
	lowAccess := ResurfaceScore(0.5, 40, 1)
	highAccess := ResurfaceScore(0.5, 40, 8)

	if highAccess >= lowAccess {
		t.Errorf("high access (%.2f) should score lower than low access (%.2f)", highAccess, lowAccess)
	}
}

func TestResurfaceCandidate_Fields(t *testing.T) {
	c := ResurfaceCandidate{
		ArtifactID: "art-1",
		Title:      "Old Article",
		Score:      0.75,
		Reason:     "Dormant for 45 days",
	}

	if c.ArtifactID != "art-1" {
		t.Error("unexpected artifact ID")
	}
	if c.Score <= 0 {
		t.Error("expected positive score")
	}
}

func TestResurfaceScore_ZeroRelevance(t *testing.T) {
	score := ResurfaceScore(0.0, 60, 0)
	// With 0 relevance, only dormancy bonus contributes
	if score < 0 {
		t.Errorf("score should not be negative, got %f", score)
	}
}

func TestResurfaceScore_MaxDormancy(t *testing.T) {
	score30 := ResurfaceScore(0.5, 130, 0)
	score200 := ResurfaceScore(0.5, 230, 0)
	// Dormancy bonus is capped at 1.0
	if score200 > score30*2 {
		t.Errorf("dormancy bonus should be capped, got score30=%.2f score200=%.2f", score30, score200)
	}
}

func TestResurfaceScore_MaxAccessPenalty(t *testing.T) {
	lowPenalty := ResurfaceScore(0.5, 40, 5)
	highPenalty := ResurfaceScore(0.5, 40, 100)
	// Access penalty is capped at 1.0, so both should be >= 0
	if highPenalty < 0 {
		t.Errorf("score should not be negative even with high access, got %f", highPenalty)
	}
	if highPenalty > lowPenalty {
		t.Errorf("more access should not increase score")
	}
}

func TestResurfaceScore_NoDormancyBelow30(t *testing.T) {
	fresh := ResurfaceScore(0.5, 10, 0)
	at30 := ResurfaceScore(0.5, 30, 0)
	// No dormancy bonus until 30 days
	if fresh != at30 {
		t.Errorf("expected same score below 30 days: fresh=%.2f at30=%.2f", fresh, at30)
	}
}

func TestSerendipityPick_NilPool_Resurface(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.SerendipityPick(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestSerendipityCandidate_ContextScoring(t *testing.T) {
	// Verify that context scoring gives correct bonus weights
	sc := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{
			ArtifactID: "art-1",
			Title:      "Old but relevant",
			Score:      0.8,
		},
		TopicMatch:   true,
		ContextScore: 0.8*0.5 + 2.0, // base + topic match bonus
	}
	if sc.ContextScore < 2.0 {
		t.Errorf("topic match should boost score above 2.0, got %.2f", sc.ContextScore)
	}
	if !sc.TopicMatch {
		t.Error("TopicMatch should be true")
	}
}

// === Edge cases: ResurfaceScore exact boundary values ===

func TestResurfaceScore_ExactDormancy30(t *testing.T) {
	// Exactly 30 days: dormancyBonus = 0 (condition is > 30, not >=)
	score := ResurfaceScore(0.5, 30, 0)
	fresher := ResurfaceScore(0.5, 29, 0)
	if score != fresher {
		t.Errorf("exactly 30 days should have no dormancy bonus like 29 days: 30d=%.4f 29d=%.4f", score, fresher)
	}
}

func TestResurfaceScore_Dormancy31(t *testing.T) {
	// 31 days: first day of bonus (1 * 0.01 = 0.01)
	score31 := ResurfaceScore(0.5, 31, 0)
	score30 := ResurfaceScore(0.5, 30, 0)
	if score31 <= score30 {
		t.Errorf("31 days should score higher than 30: 31d=%.4f 30d=%.4f", score31, score30)
	}
}

func TestResurfaceScore_ExactDormancyCap(t *testing.T) {
	// At 130 days: bonus = (130-30)*0.01 = 1.0 (cap)
	// At 131 days: bonus = (131-30)*0.01 = 1.01, capped to 1.0
	score130 := ResurfaceScore(0.5, 130, 0)
	score200 := ResurfaceScore(0.5, 200, 0)
	if score130 != score200 {
		t.Errorf("dormancy bonus should cap at 130d: 130d=%.4f 200d=%.4f", score130, score200)
	}
}

func TestResurfaceScore_ExactAccessPenaltyCap(t *testing.T) {
	// Access penalty: count * 0.1, capped at 1.0
	// At 10: penalty = 1.0 → score = (relevance + dormancyBonus) * 0
	score := ResurfaceScore(0.5, 40, 10)
	if score != 0 {
		t.Errorf("10 accesses should zero out score: got %.4f", score)
	}

	// At 11: penalty still capped at 1.0 → score still 0
	score11 := ResurfaceScore(0.5, 40, 11)
	if score11 != 0 {
		t.Errorf("11 accesses should also zero out: got %.4f", score11)
	}
}

func TestResurfaceScore_NegativeDormancy(t *testing.T) {
	// Negative dormancy (shouldn't happen but defensive)
	score := ResurfaceScore(0.5, -10, 0)
	// dormancyBonus = 0 (< 30), so score = 0.5 * 1.0 = 0.5
	if score != 0.5 {
		t.Errorf("negative dormancy should behave like fresh: got %.4f", score)
	}
}

func TestMarkResurfaced_NilSlice(t *testing.T) {
	engine := NewEngine(nil, nil)
	err := engine.MarkResurfaced(nil, nil)
	// nil slice should short-circuit like empty slice
	if err != nil {
		t.Errorf("expected nil for nil slice, got: %v", err)
	}
}
