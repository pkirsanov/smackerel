package intelligence

import (
	"sort"
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

// === Improve: difficultyOrder sorts beginner < intermediate < advanced (IMP-006-R02) ===

func TestDifficultyOrder(t *testing.T) {
	tests := []struct {
		difficulty LearningDifficulty
		expected   int
	}{
		{DifficultyBeginner, 0},
		{DifficultyIntermediate, 1},
		{DifficultyAdvanced, 2},
		{"unknown", 1}, // unknown defaults to intermediate position
		{"", 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.difficulty), func(t *testing.T) {
			got := difficultyOrder(tt.difficulty)
			if got != tt.expected {
				t.Errorf("difficultyOrder(%q) = %d, want %d", tt.difficulty, got, tt.expected)
			}
		})
	}
}

func TestLearningPath_ResourcesSortedByDifficulty(t *testing.T) {
	// Verify that resources within a path are ordered by difficulty
	// (beginner → intermediate → advanced) after GetLearningPaths processing.
	resources := []LearningResource{
		{ArtifactID: "a1", Difficulty: DifficultyAdvanced, Title: "Advanced Topic"},
		{ArtifactID: "a2", Difficulty: DifficultyBeginner, Title: "Getting Started"},
		{ArtifactID: "a3", Difficulty: DifficultyIntermediate, Title: "Middle Ground"},
		{ArtifactID: "a4", Difficulty: DifficultyBeginner, Title: "Basics 101"},
		{ArtifactID: "a5", Difficulty: DifficultyAdvanced, Title: "Expert Level"},
	}

	// Simulate the sort that GetLearningPaths now performs
	sort.SliceStable(resources, func(i, j int) bool {
		return difficultyOrder(resources[i].Difficulty) < difficultyOrder(resources[j].Difficulty)
	})

	// Verify order: all beginners first, then intermediate, then advanced
	if resources[0].Difficulty != DifficultyBeginner || resources[1].Difficulty != DifficultyBeginner {
		t.Errorf("first two resources should be beginner, got %s and %s", resources[0].Difficulty, resources[1].Difficulty)
	}
	if resources[2].Difficulty != DifficultyIntermediate {
		t.Errorf("third resource should be intermediate, got %s", resources[2].Difficulty)
	}
	if resources[3].Difficulty != DifficultyAdvanced || resources[4].Difficulty != DifficultyAdvanced {
		t.Errorf("last two resources should be advanced, got %s and %s", resources[3].Difficulty, resources[4].Difficulty)
	}

	// Verify stable sort preserves title order within same difficulty
	if resources[0].Title != "Getting Started" || resources[1].Title != "Basics 101" {
		t.Errorf("stable sort should preserve original order within same difficulty, got %q and %q",
			resources[0].Title, resources[1].Title)
	}
}

// === BUG-004: Time estimation tests ===

func TestEstimateReadingTime_Article(t *testing.T) {
	// 5000 chars / 1000 chars per minute = 5 minutes
	got := estimateReadingTime("article", 5000, "")
	if got != 5 {
		t.Errorf("estimateReadingTime(article, 5000) = %d, want 5", got)
	}
}

func TestEstimateReadingTime_ShortArticle(t *testing.T) {
	// 500 chars should be at least 1 minute
	got := estimateReadingTime("article", 500, "")
	if got != 1 {
		t.Errorf("estimateReadingTime(article, 500) = %d, want 1", got)
	}
}

func TestEstimateReadingTime_ZeroLength(t *testing.T) {
	// Zero content length should default to 10
	got := estimateReadingTime("article", 0, "")
	if got != 10 {
		t.Errorf("estimateReadingTime(article, 0) = %d, want 10", got)
	}
}

func TestEstimateReadingTime_YouTube(t *testing.T) {
	// Duration 600 seconds = 10 minutes
	got := estimateReadingTime("youtube", 0, "600")
	if got != 10 {
		t.Errorf("estimateReadingTime(youtube, 0, 600) = %d, want 10", got)
	}
}

func TestEstimateReadingTime_YouTubeNoDuration(t *testing.T) {
	// No duration defaults to 15
	got := estimateReadingTime("youtube", 0, "")
	if got != 15 {
		t.Errorf("estimateReadingTime(youtube, no duration) = %d, want 15", got)
	}
}

func TestEstimateReadingTime_PDF(t *testing.T) {
	// PDFs use same char-based estimation as articles
	got := estimateReadingTime("pdf", 10000, "")
	if got != 10 {
		t.Errorf("estimateReadingTime(pdf, 10000) = %d, want 10", got)
	}
}

func TestEstimateReadingTime_UnknownType(t *testing.T) {
	got := estimateReadingTime("note/text", 0, "")
	if got != 10 {
		t.Errorf("estimateReadingTime(note/text) = %d, want 10", got)
	}
}

func TestEstimateReadingTime_YouTubeRoundsUp(t *testing.T) {
	// 91 seconds should round up to 2 minutes
	got := estimateReadingTime("youtube", 0, "91")
	if got != 2 {
		t.Errorf("estimateReadingTime(youtube, 91s) = %d, want 2", got)
	}
}
