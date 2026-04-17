package intelligence

import (
	"math"
	"strings"
	"testing"
)

// === synthesisConfidence formula verification ===

func TestSynthesisConfidence_ExactFormula(t *testing.T) {
	// Formula: min(1.0, 0.6*log2(artifactCount)/5 + 0.4*log2(sourceCount)/3)
	tests := []struct {
		name      string
		artifacts int
		sources   int
		wantMin   float64
		wantMax   float64
	}{
		{"(4,2): volume=0.6*2/5=0.24, div=0.4*1/3=0.133", 4, 2, 0.37, 0.38},
		{"(8,4): volume=0.6*3/5=0.36, div=0.4*2/3=0.267", 8, 4, 0.62, 0.63},
		{"(16,8): volume=0.6*4/5=0.48, div=0.4*3/3=0.40", 16, 8, 0.87, 0.89},
		{"(32,8): volume=0.6*5/5=0.6, div=0.4*3/3=0.40", 32, 8, 1.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := synthesisConfidence(tt.artifacts, tt.sources)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("synthesisConfidence(%d,%d) = %.6f, want [%.2f, %.2f]",
					tt.artifacts, tt.sources, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSynthesisConfidence_MonotonicInVolume(t *testing.T) {
	// Holding source count constant, more artifacts should always produce
	// equal or higher confidence.
	prev := synthesisConfidence(2, 3)
	for art := 3; art <= 50; art++ {
		cur := synthesisConfidence(art, 3)
		if cur < prev {
			t.Errorf("confidence decreased at artifact count %d: %.6f < %.6f", art, cur, prev)
		}
		prev = cur
	}
}

func TestSynthesisConfidence_MonotonicInDiversity(t *testing.T) {
	// Holding artifact count constant, more sources should always produce
	// equal or higher confidence.
	prev := synthesisConfidence(10, 1)
	for src := 2; src <= 20; src++ {
		cur := synthesisConfidence(10, src)
		if cur < prev {
			t.Errorf("confidence decreased at source count %d: %.6f < %.6f", src, cur, prev)
		}
		prev = cur
	}
}

func TestSynthesisConfidence_NeverExceedsOne(t *testing.T) {
	// Exhaustive sweep: confidence must never exceed 1.0
	for art := 1; art <= 100; art++ {
		for src := 1; src <= 50; src++ {
			conf := synthesisConfidence(art, src)
			if conf > 1.0 {
				t.Fatalf("confidence(%d,%d) = %f > 1.0", art, src, conf)
			}
		}
	}
}

func TestSynthesisConfidence_ReturnsFloat64(t *testing.T) {
	// Verify the function returns a proper float, not NaN or Inf
	conf := synthesisConfidence(5, 3)
	if math.IsNaN(conf) || math.IsInf(conf, 0) {
		t.Errorf("confidence should be finite, got %f", conf)
	}
}

// === assembleWeeklySynthesisText confidence percentage formatting ===

func TestAssembleWeeklySynthesisText_ConfidencePercentageFormat(t *testing.T) {
	ws := &WeeklySynthesis{
		Insights: []SynthesisInsight{
			{ThroughLine: "Distributed systems convergence", Confidence: 0.857},
		},
	}
	text := assembleWeeklySynthesisText(ws)

	// Confidence is formatted as %.0f%% — should show "86%" (rounded)
	if !strings.Contains(text, "86%") {
		t.Errorf("expected confidence displayed as 86%%, got: %s", text)
	}
}

func TestAssembleWeeklySynthesisText_MultipleInsightsBulletFormat(t *testing.T) {
	ws := &WeeklySynthesis{
		Insights: []SynthesisInsight{
			{ThroughLine: "Caching patterns", Confidence: 0.80},
			{ThroughLine: "Security practices", Confidence: 0.65},
			{ThroughLine: "API design", Confidence: 0.50},
		},
	}
	text := assembleWeeklySynthesisText(ws)

	// Each insight should be on its own bullet line
	if !strings.Contains(text, "• Caching patterns") {
		t.Error("missing bullet for first insight")
	}
	if !strings.Contains(text, "• Security practices") {
		t.Error("missing bullet for second insight")
	}
	if !strings.Contains(text, "• API design") {
		t.Error("missing bullet for third insight")
	}
}

func TestAssembleWeeklySynthesisText_StatsFormatting(t *testing.T) {
	ws := &WeeklySynthesis{
		Stats: WeeklyStats{ArtifactsProcessed: 0, NewConnections: 0, TopicsActive: 0},
	}
	text := assembleWeeklySynthesisText(ws)

	// Zero artifacts processed → no THIS WEEK section
	if strings.Contains(text, "THIS WEEK") {
		t.Error("should not include THIS WEEK section when ArtifactsProcessed is 0")
	}

	ws.Stats.ArtifactsProcessed = 1
	text = assembleWeeklySynthesisText(ws)
	if !strings.Contains(text, "THIS WEEK") {
		t.Error("should include THIS WEEK section when ArtifactsProcessed > 0")
	}
	if !strings.Contains(text, "1 artifacts") {
		t.Error("should show artifact count in stats")
	}
}

func TestAssembleWeeklySynthesisText_SectionOrdering(t *testing.T) {
	ws := &WeeklySynthesis{
		Stats:    WeeklyStats{ArtifactsProcessed: 10, NewConnections: 5, TopicsActive: 3},
		Insights: []SynthesisInsight{{ThroughLine: "test", Confidence: 0.5}},
		TopicMovement: []TopicMovement{
			{TopicName: "Go", Direction: "stable", Captures: 5},
		},
		OpenLoops:        []string{"Review budget"},
		SerendipityPicks: []ResurfaceCandidate{{Title: "Old article", Reason: "relevant"}},
		Patterns:         []string{"Peak capture on Tuesdays"},
	}
	text := assembleWeeklySynthesisText(ws)

	// Verify section ordering: THIS WEEK → CONNECTION → TOPIC → OPEN LOOPS → ARCHIVE → PATTERNS
	idx := func(section string) int { return strings.Index(text, section) }

	thisWeek := idx("THIS WEEK")
	connection := idx("CONNECTION DISCOVERED")
	momentum := idx("TOPIC MOMENTUM")
	loops := idx("OPEN LOOPS")
	archive := idx("FROM THE ARCHIVE")
	patterns := idx("PATTERNS NOTICED")

	if thisWeek >= connection {
		t.Error("THIS WEEK should come before CONNECTION DISCOVERED")
	}
	if connection >= momentum {
		t.Error("CONNECTION DISCOVERED should come before TOPIC MOMENTUM")
	}
	if momentum >= loops {
		t.Error("TOPIC MOMENTUM should come before OPEN LOOPS")
	}
	if loops >= archive {
		t.Error("OPEN LOOPS should come before FROM THE ARCHIVE")
	}
	if archive >= patterns {
		t.Error("FROM THE ARCHIVE should come before PATTERNS NOTICED")
	}
}

// === TopicMovement direction logic ===

func TestTopicMovement_DirectionClassification(t *testing.T) {
	// Replicate the direction logic from GenerateWeeklySynthesis:
	// captures > lastWeek+1 → rising, captures < lastWeek-1 → falling, else stable
	tests := []struct {
		thisWeek int
		lastWeek int
		want     string
	}{
		{10, 5, "rising"},  // 10 > 5+1
		{3, 8, "falling"},  // 3 < 8-1
		{5, 5, "stable"},   // equal
		{6, 5, "stable"},   // exactly +1 is stable
		{4, 5, "stable"},   // exactly -1 is stable
		{7, 5, "rising"},   // +2 is rising
		{3, 5, "falling"},  // -2 is falling
		{0, 0, "stable"},   // both zero
		{100, 0, "rising"}, // large jump from zero
		{0, 100, "falling"},
	}
	for _, tt := range tests {
		var dir string
		if tt.thisWeek > tt.lastWeek+1 {
			dir = "rising"
		} else if tt.thisWeek < tt.lastWeek-1 {
			dir = "falling"
		} else {
			dir = "stable"
		}
		if dir != tt.want {
			t.Errorf("thisWeek=%d, lastWeek=%d: got %q, want %q",
				tt.thisWeek, tt.lastWeek, dir, tt.want)
		}
	}
}

// === WeeklySynthesis struct defaults ===

func TestWeeklySynthesis_EmptyDefaults(t *testing.T) {
	ws := &WeeklySynthesis{}
	if ws.WeekOf != "" {
		t.Error("empty WeeklySynthesis should have empty WeekOf")
	}
	if ws.Stats.ArtifactsProcessed != 0 {
		t.Error("empty stats should have zero artifacts")
	}
	if len(ws.Insights) != 0 {
		t.Error("empty WeeklySynthesis should have no insights")
	}
	if len(ws.TopicMovement) != 0 {
		t.Error("empty WeeklySynthesis should have no topic movements")
	}
	if len(ws.OpenLoops) != 0 {
		t.Error("empty WeeklySynthesis should have no open loops")
	}
	if len(ws.SerendipityPicks) != 0 {
		t.Error("empty WeeklySynthesis should have no serendipity picks")
	}
	if len(ws.Patterns) != 0 {
		t.Error("empty WeeklySynthesis should have no patterns")
	}
	if ws.WordCount != 0 {
		t.Error("empty WeeklySynthesis should have zero word count")
	}
	if ws.SynthesisText != "" {
		t.Error("empty WeeklySynthesis should have empty synthesis text")
	}
}

func TestAssembleWeeklySynthesisText_TopicMovementArrows(t *testing.T) {
	ws := &WeeklySynthesis{
		TopicMovement: []TopicMovement{
			{TopicName: "Go", Direction: "rising", Captures: 15},
			{TopicName: "Rust", Direction: "falling", Captures: 2},
			{TopicName: "Python", Direction: "stable", Captures: 7},
		},
	}
	text := assembleWeeklySynthesisText(ws)

	if !strings.Contains(text, "↑ Go (15 this week)") {
		t.Errorf("rising topic should show ↑ arrow with captures, got: %s", text)
	}
	if !strings.Contains(text, "↓ Rust (2 this week)") {
		t.Errorf("falling topic should show ↓ arrow with captures, got: %s", text)
	}
	if !strings.Contains(text, "→ Python (7 this week)") {
		t.Errorf("stable topic should show → arrow with captures, got: %s", text)
	}
}
