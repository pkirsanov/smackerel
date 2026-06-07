package intelligence

import (
	"testing"
	"time"
)

func TestExpertiseTier_Constants(t *testing.T) {
	tiers := []ExpertiseTier{TierNovice, TierFoundation, TierIntermediate, TierDeep, TierExpert}
	if len(tiers) != 5 {
		t.Errorf("expected 5 expertise tiers, got %d", len(tiers))
	}
	seen := make(map[ExpertiseTier]bool)
	for _, tier := range tiers {
		if tier == "" {
			t.Error("tier must not be empty")
		}
		if seen[tier] {
			t.Errorf("duplicate tier: %s", tier)
		}
		seen[tier] = true
	}
}

func TestGrowthTrajectory_Constants(t *testing.T) {
	trajectories := []GrowthTrajectory{TrajectoryAccelerating, TrajectorySteady, TrajectoryDecelerating, TrajectoryStopped}
	if len(trajectories) != 4 {
		t.Errorf("expected 4 trajectories, got %d", len(trajectories))
	}
}

// NOTE (BUG-021-008): the former TestComputeDepthScore / TestAssignTier /
// TestComputeTrajectory lock tests were removed. They pinned the hardcoded
// depth-score weights and the numeric tier/velocity boundaries that this bug
// deletes — tier and growth are now LLM-judged per situation
// (docs/smackerel.md §3.6). The LLM-driven decision contract is covered by
// expertise_eval_test.go (scripted bridge: parse, routing, signal forwarding,
// internal-id non-leak, ref correlation, error paths).

func TestExpertiseMap_Struct(t *testing.T) {
	em := &ExpertiseMap{
		Topics: []TopicExpertise{
			{
				TopicID:      "t1",
				TopicName:    "Go Programming",
				CaptureCount: 55,
				Tier:         TierDeep,
				Growth:       TrajectoryAccelerating,
			},
		},
		BlindSpots: []BlindSpot{
			{TopicName: "analytics", MentionCount: 20, CaptureCount: 3, Gap: 17},
		},
		TotalTopics: 1,
		DataDays:    120,
		Mature:      true,
		GeneratedAt: time.Now(),
	}

	if len(em.Topics) != 1 {
		t.Errorf("expected 1 topic, got %d", len(em.Topics))
	}
	if em.Topics[0].Tier != TierDeep {
		t.Errorf("expected TierDeep, got %s", em.Topics[0].Tier)
	}
	if !em.Mature {
		t.Error("expected mature=true for 120 data days")
	}
	if em.BlindSpots[0].Gap != 17 {
		t.Errorf("expected gap=17, got %d", em.BlindSpots[0].Gap)
	}
}

func TestBlindSpot_GapCalculation(t *testing.T) {
	bs := BlindSpot{MentionCount: 25, CaptureCount: 3}
	bs.Gap = bs.MentionCount - bs.CaptureCount
	if bs.Gap != 22 {
		t.Errorf("expected gap=22, got %d", bs.Gap)
	}
}

func TestGenerateExpertiseMap_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GenerateExpertiseMap(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === Edge case: ExpertiseMap immature data days ===

func TestExpertiseMap_ImmatureData(t *testing.T) {
	em := &ExpertiseMap{DataDays: 89, Mature: false}
	if em.Mature {
		t.Error("89 data days should not be mature (needs 90+)")
	}
	em.DataDays = 90
	em.Mature = em.DataDays >= 90
	if !em.Mature {
		t.Error("exactly 90 data days should be mature")
	}
}
