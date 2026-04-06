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
