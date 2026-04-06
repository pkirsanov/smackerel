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
