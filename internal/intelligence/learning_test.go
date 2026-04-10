package intelligence

import (
	"testing"
)

func TestLearningDifficulty_Constants(t *testing.T) {
	diffs := []LearningDifficulty{DifficultyBeginner, DifficultyIntermediate, DifficultyAdvanced}
	if len(diffs) != 3 {
		t.Errorf("expected 3 difficulty levels, got %d", len(diffs))
	}
}

func TestClassifyDifficultyHeuristic(t *testing.T) {
	tests := []struct {
		title    string
		expected LearningDifficulty
	}{
		{"Introduction to Go", DifficultyBeginner},
		{"Getting Started with React", DifficultyBeginner},
		{"TypeScript 101", DifficultyBeginner},
		{"Fundamentals of Machine Learning", DifficultyBeginner},
		{"Advanced Kubernetes Optimization", DifficultyAdvanced},
		{"Deep Dive into Linux Internals", DifficultyAdvanced},
		{"Performance Tuning Guide", DifficultyAdvanced},
		{"Building APIs with Express", DifficultyIntermediate},
		{"Understanding Databases", DifficultyIntermediate},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := classifyDifficultyHeuristic(tt.title, "article", 0)
			if got != tt.expected {
				t.Errorf("classifyDifficultyHeuristic(%q) = %s, want %s", tt.title, got, tt.expected)
			}
		})
	}
}

func TestDetectGaps(t *testing.T) {
	tests := []struct {
		name      string
		resources []LearningResource
		gapCount  int
	}{
		{
			name: "no gaps",
			resources: []LearningResource{
				{Difficulty: DifficultyBeginner},
				{Difficulty: DifficultyIntermediate},
				{Difficulty: DifficultyAdvanced},
			},
			gapCount: 0,
		},
		{
			name: "missing intermediate",
			resources: []LearningResource{
				{Difficulty: DifficultyBeginner},
				{Difficulty: DifficultyAdvanced},
			},
			gapCount: 1,
		},
		{
			name: "missing beginner",
			resources: []LearningResource{
				{Difficulty: DifficultyIntermediate},
				{Difficulty: DifficultyAdvanced},
			},
			gapCount: 1,
		},
		{
			name: "all same difficulty",
			resources: []LearningResource{
				{Difficulty: DifficultyIntermediate},
				{Difficulty: DifficultyIntermediate},
			},
			gapCount: 1, // missing beginner
		},
		{
			name:      "empty",
			resources: []LearningResource{},
			gapCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gaps := detectGaps(tt.resources)
			if len(gaps) != tt.gapCount {
				t.Errorf("detectGaps() returned %d gaps, want %d: %v", len(gaps), tt.gapCount, gaps)
			}
		})
	}
}

func TestGetLearningPaths_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GetLearningPaths(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestMarkLearningResourceCompleted_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	err := engine.MarkLearningResourceCompleted(nil, "t1", "a1")
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestResurfaceScore_Phase5(t *testing.T) {
	// Verify the existing ResurfaceScore function still works as Phase 5 serendipity scoring foundation
	score := ResurfaceScore(0.8, 200, 1)
	if score <= 0 {
		t.Errorf("expected positive score, got %v", score)
	}

	// Higher dormancy = higher score
	lowDormancy := ResurfaceScore(0.8, 31, 1)
	highDormancy := ResurfaceScore(0.8, 200, 1)
	if highDormancy <= lowDormancy {
		t.Errorf("expected higher dormancy to increase score: low=%v high=%v", lowDormancy, highDormancy)
	}
}

func TestSerendipityPick_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.SerendipityPick(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}
