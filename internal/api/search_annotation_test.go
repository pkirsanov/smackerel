package api

import (
	"testing"
)

func TestParseAnnotationIntent_TopRated(t *testing.T) {
	tests := []struct {
		query       string
		wantRating  int
		wantCleaned string
	}{
		{"my top rated recipes", 4, "recipes"},
		{"best rated chicken", 4, "chicken"},
		{"highest rated pasta", 4, "pasta"},
		{"best dinners", 4, "dinners"},
	}
	for _, tt := range tests {
		intent := parseAnnotationIntent(tt.query)
		if intent == nil {
			t.Errorf("parseAnnotationIntent(%q) = nil, want intent", tt.query)
			continue
		}
		if intent.MinRating != tt.wantRating {
			t.Errorf("parseAnnotationIntent(%q).MinRating = %d, want %d", tt.query, intent.MinRating, tt.wantRating)
		}
		if intent.Cleaned != tt.wantCleaned {
			t.Errorf("parseAnnotationIntent(%q).Cleaned = %q, want %q", tt.query, intent.Cleaned, tt.wantCleaned)
		}
	}
}

func TestParseAnnotationIntent_Interaction(t *testing.T) {
	tests := []struct {
		query string
	}{
		{"things I've made"},
		{"things I made"},
		{"things I've cooked"},
		{"things I've tried"},
	}
	for _, tt := range tests {
		intent := parseAnnotationIntent(tt.query)
		if intent == nil {
			t.Errorf("parseAnnotationIntent(%q) = nil, want intent", tt.query)
			continue
		}
		if !intent.HasInteraction {
			t.Errorf("parseAnnotationIntent(%q).HasInteraction = false, want true", tt.query)
		}
	}
}

func TestParseAnnotationIntent_TagInQuery(t *testing.T) {
	intent := parseAnnotationIntent("#weeknight dinners")
	if intent == nil {
		t.Fatal("expected non-nil intent")
	}
	if intent.Tag != "weeknight" {
		t.Errorf("tag = %q, want weeknight", intent.Tag)
	}
	if intent.Cleaned != "dinners" {
		t.Errorf("cleaned = %q, want dinners", intent.Cleaned)
	}
}

func TestParseAnnotationIntent_PlainQuery(t *testing.T) {
	tests := []string{
		"chicken pasta recipe",
		"how to cook rice",
		"vacation ideas",
	}
	for _, q := range tests {
		intent := parseAnnotationIntent(q)
		if intent != nil {
			t.Errorf("parseAnnotationIntent(%q) = %+v, want nil", q, intent)
		}
	}
}

func TestApplyAnnotationBoost_RatingOnly(t *testing.T) {
	rating := 5
	boost := applyAnnotationBoost(0.5, &rating, nil)
	// Rating 5 → (5-1)/4 * 0.05 = 0.05
	expected := 0.55
	if boost < expected-0.001 || boost > expected+0.001 {
		t.Errorf("boost = %f, want ~%f", boost, expected)
	}
}

func TestApplyAnnotationBoost_UsageOnly(t *testing.T) {
	uses := 10
	boost := applyAnnotationBoost(0.5, nil, &uses)
	// Uses 10 → min(10,10)/10 * 0.03 = 0.03
	expected := 0.53
	if boost < expected-0.001 || boost > expected+0.001 {
		t.Errorf("boost = %f, want ~%f", boost, expected)
	}
}

func TestApplyAnnotationBoost_MaxCap(t *testing.T) {
	rating := 5
	uses := 100
	boost := applyAnnotationBoost(0.5, &rating, &uses)
	// Rating 5: 0.05, Uses 100: 0.03 → 0.08 cap
	expected := 0.58
	if boost < expected-0.001 || boost > expected+0.001 {
		t.Errorf("boost = %f, want ~%f (capped at 0.08)", boost, expected)
	}
}

func TestApplyAnnotationBoost_NoAnnotations(t *testing.T) {
	boost := applyAnnotationBoost(0.5, nil, nil)
	if boost != 0.5 {
		t.Errorf("boost = %f, want 0.5 (no change)", boost)
	}
}

func TestApplyAnnotationBoost_LowRating(t *testing.T) {
	rating := 1
	boost := applyAnnotationBoost(0.5, &rating, nil)
	// Rating 1 → (1-1)/4 * 0.05 = 0.0
	if boost != 0.5 {
		t.Errorf("boost = %f, want 0.5 (rating 1 = no boost)", boost)
	}
}

func TestApplyAnnotationBoost_SmallBoostDoesNotOverwhelmSemantics(t *testing.T) {
	// High annotation score but low similarity should not beat high similarity with no annotations
	rating := 5
	uses := 10
	lowSim := applyAnnotationBoost(0.3, &rating, &uses) // 0.3 + 0.08 = 0.38
	highSim := applyAnnotationBoost(0.8, nil, nil)      // 0.8

	if lowSim >= highSim {
		t.Errorf("annotation boost should not overwhelm semantic difference: annotated=%.2f, plain=%.2f",
			lowSim, highSim)
	}
}
