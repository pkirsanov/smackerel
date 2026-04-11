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

// === Edge cases: detectGaps additional scenarios ===

func TestDetectGaps_OnlyAdvanced(t *testing.T) {
	resources := []LearningResource{
		{Difficulty: DifficultyAdvanced},
		{Difficulty: DifficultyAdvanced},
	}
	gaps := detectGaps(resources)
	// Missing beginner (has advanced but no beginner)
	if len(gaps) != 1 {
		t.Errorf("expected 1 gap (missing beginner), got %d: %v", len(gaps), gaps)
	}
}

func TestDetectGaps_OnlyBeginner(t *testing.T) {
	resources := []LearningResource{
		{Difficulty: DifficultyBeginner},
		{Difficulty: DifficultyBeginner},
	}
	gaps := detectGaps(resources)
	// Only beginner: no gap triggered (the gap rules require specific combos)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps for only-beginner, got %d: %v", len(gaps), gaps)
	}
}

func TestDetectGaps_BeginnerAndIntermediate(t *testing.T) {
	resources := []LearningResource{
		{Difficulty: DifficultyBeginner},
		{Difficulty: DifficultyIntermediate},
	}
	gaps := detectGaps(resources)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps for beginner+intermediate, got %d: %v", len(gaps), gaps)
	}
}

func TestDetectGaps_IntermediateOnly(t *testing.T) {
	// Has intermediate but no beginner → gap
	resources := []LearningResource{
		{Difficulty: DifficultyIntermediate},
	}
	gaps := detectGaps(resources)
	if len(gaps) != 1 {
		t.Errorf("expected 1 gap (missing beginner), got %d: %v", len(gaps), gaps)
	}
}

// === Edge cases: classifyDifficultyHeuristic ===

func TestClassifyDifficultyHeuristic_ContentTypeInfluence(t *testing.T) {
	// contentType is concatenated with title for matching
	got := classifyDifficultyHeuristic("Some Title", "introduction", 0)
	if got != DifficultyBeginner {
		t.Errorf("contentType 'introduction' should classify as beginner, got %s", got)
	}

	got = classifyDifficultyHeuristic("Some Title", "advanced", 0)
	if got != DifficultyAdvanced {
		t.Errorf("contentType 'advanced' should classify as advanced, got %s", got)
	}
}

func TestClassifyDifficultyHeuristic_AdvancedPrecedesBeginner(t *testing.T) {
	// Title contains both advanced and beginner terms — advanced checked first
	got := classifyDifficultyHeuristic("Advanced Introduction to Go", "article", 0)
	if got != DifficultyAdvanced {
		t.Errorf("advanced keyword should take precedence over beginner, got %s", got)
	}
}

func TestClassifyDifficultyHeuristic_EmptyInputs(t *testing.T) {
	got := classifyDifficultyHeuristic("", "", 0)
	if got != DifficultyIntermediate {
		t.Errorf("empty inputs should default to intermediate, got %s", got)
	}
}

func TestSerendipityPick_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.SerendipityPick(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === Chaos: Resurface limit edge cases ===

func TestResurface_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.Resurface(nil, 5)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestResurface_ZeroLimit(t *testing.T) {
	// limit=0 should be normalized to 5, then fail on nil pool
	engine := &Engine{Pool: nil}
	_, err := engine.Resurface(nil, 0)
	if err == nil {
		t.Error("expected error for nil pool even with limit=0")
	}
}

func TestResurface_NegativeLimit(t *testing.T) {
	// negative limit should be normalized to 5, then fail on nil pool
	engine := &Engine{Pool: nil}
	_, err := engine.Resurface(nil, -10)
	if err == nil {
		t.Error("expected error for nil pool even with negative limit")
	}
}
