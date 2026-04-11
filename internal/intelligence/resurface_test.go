package intelligence

import (
	"testing"
)

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

func TestMarkResurfaced_NilSlice(t *testing.T) {
	engine := NewEngine(nil, nil)
	err := engine.MarkResurfaced(nil, nil)
	// nil slice should short-circuit like empty slice
	if err != nil {
		t.Errorf("expected nil for nil slice, got: %v", err)
	}
}

// === Chaos: MarkResurfaced filters empty-string IDs ===

func TestMarkResurfaced_EmptyStringIDs(t *testing.T) {
	engine := NewEngine(nil, nil)
	// All-empty IDs should short-circuit without hitting the pool
	err := engine.MarkResurfaced(nil, []string{"", "", ""})
	if err != nil {
		t.Errorf("expected nil for all-empty IDs, got: %v", err)
	}
}

func TestMarkResurfaced_MixedEmptyAndValid(t *testing.T) {
	// With nil pool but valid IDs after filtering, should get a pool error
	engine := NewEngine(nil, nil)
	err := engine.MarkResurfaced(nil, []string{"", "valid-id", ""})
	if err == nil {
		t.Error("expected pool error when valid IDs remain after filtering")
	}
}

func TestMarkResurfaced_EmptySlice(t *testing.T) {
	engine := NewEngine(nil, nil)
	err := engine.MarkResurfaced(nil, []string{})
	if err != nil {
		t.Errorf("expected nil for empty slice, got: %v", err)
	}
}
