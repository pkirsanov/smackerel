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

func TestComputeDepthScore(t *testing.T) {
	tests := []struct {
		name     string
		te       TopicExpertise
		expected float64
	}{
		{
			name: "all zeros",
			te:   TopicExpertise{},
			// 0*0.3 + 0*15 + 0*20 + 0*0.1 + 0*10 = 0
			expected: 0,
		},
		{
			name: "expert-level topic",
			te: TopicExpertise{
				CaptureCount:      120,
				SourceDiversity:   5,
				DepthRatio:        0.8,
				Engagement:        200,
				ConnectionDensity: 3.0,
			},
			// 120*0.3 + 5*15 + 0.8*20 + 200*0.1 + 3.0*10 = 36+75+16+20+30 = 177
			expected: 177,
		},
		{
			name: "novice topic",
			te: TopicExpertise{
				CaptureCount:      3,
				SourceDiversity:   1,
				DepthRatio:        0.0,
				Engagement:        2,
				ConnectionDensity: 0,
			},
			// 3*0.3 + 1*15 + 0 + 2*0.1 + 0 = 0.9+15+0+0.2+0 = 16.1
			expected: 16.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDepthScore(tt.te)
			if diff := got - tt.expected; diff > 0.01 || diff < -0.01 {
				t.Errorf("computeDepthScore() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAssignTier(t *testing.T) {
	tests := []struct {
		name         string
		captureCount int
		depthScore   float64
		expected     ExpertiseTier
	}{
		{"novice low count", 3, 5, TierNovice},
		{"novice high score but low count", 3, 100, TierNovice},
		{"foundation", 10, 15, TierFoundation},
		{"intermediate", 30, 45, TierIntermediate},
		{"deep", 70, 75, TierDeep},
		{"expert", 150, 100, TierExpert},
		{"boundary: exactly 5 captures", 5, 5, TierNovice},
		{"boundary: exactly 6 captures", 6, 15, TierFoundation},
		{"boundary: high count but low depth", 200, 5, TierNovice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assignTier(tt.captureCount, tt.depthScore)
			if got != tt.expected {
				t.Errorf("assignTier(%d, %v) = %s, want %s", tt.captureCount, tt.depthScore, got, tt.expected)
			}
		})
	}
}

func TestComputeTrajectory(t *testing.T) {
	tests := []struct {
		name       string
		recent30d  int
		avgMonthly float64
		expected   GrowthTrajectory
	}{
		{"accelerating", 20, 10, TrajectoryAccelerating},
		{"steady", 10, 10, TrajectorySteady},
		{"decelerating", 5, 10, TrajectoryDecelerating},
		{"stopped", 1, 10, TrajectoryStopped},
		{"zero avg with recent", 5, 0, TrajectoryAccelerating},
		{"zero avg no recent", 0, 0, TrajectoryStopped},
		{"boundary: exactly 1.5x", 15, 10, TrajectorySteady},
		{"boundary: exactly 0.7x", 7, 10, TrajectorySteady},
		{"boundary: exactly 0.3x", 3, 10, TrajectoryDecelerating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeTrajectory(tt.recent30d, tt.avgMonthly)
			if got != tt.expected {
				t.Errorf("computeTrajectory(%d, %v) = %s, want %s", tt.recent30d, tt.avgMonthly, got, tt.expected)
			}
		})
	}
}

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
