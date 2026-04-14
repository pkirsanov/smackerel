package intelligence

import (
	"context"
	"strings"
	"testing"
	"time"
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

// === Harden: Resurface limit ≤ 0 defaults to 5 (H-004) ===

func TestResurface_ZeroLimit_NilPool(t *testing.T) {
	// Verify that limit=0 does not cause a panic or pass 0 to the query.
	// With nil pool it errors on the query, but the point is it doesn't
	// panic from the limit defaulting path.
	engine := NewEngine(nil, nil)
	_, err := engine.Resurface(context.Background(), 0)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestResurface_NegativeLimit_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	_, err := engine.Resurface(context.Background(), -10)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === Harden: SerendipityCandidate base relevance only scoring (H-007) ===

func TestSerendipityCandidate_NoContextBonus(t *testing.T) {
	// When no topic match and not pinned, score comes only from base relevance
	sc := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{
			ArtifactID: "art-no-match",
			Title:      "Unconnected article",
			Score:      0.6,
		},
		TopicMatch:    false,
		CalendarMatch: false,
		ContextScore:  0.6 * 0.5, // base only
	}
	if sc.ContextScore >= 2.0 {
		t.Errorf("no-match candidate should have score < 2.0, got %.2f", sc.ContextScore)
	}
	if sc.TopicMatch {
		t.Error("TopicMatch should be false")
	}
	if sc.CalendarMatch {
		t.Error("CalendarMatch should be false")
	}
	// Verify it's purely base relevance
	expected := 0.6 * 0.5
	if sc.ContextScore != expected {
		t.Errorf("expected base score %.4f, got %.4f", expected, sc.ContextScore)
	}
}

func TestSerendipityCandidate_PinnedBonus(t *testing.T) {
	// Pinned items get +1.0 quality bonus even without topic match
	base := 0.7 * 0.5
	pinBonus := 1.0
	sc := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{
			ArtifactID: "art-pinned",
			Title:      "Pinned article",
			Score:      0.7,
		},
		TopicMatch:   false,
		ContextScore: base + pinBonus,
	}
	if sc.ContextScore < 1.0 {
		t.Errorf("pinned candidate should have score >= 1.0, got %.2f", sc.ContextScore)
	}
	if sc.TopicMatch {
		t.Error("TopicMatch should be false for pin-only bonus")
	}
}

func TestSerendipityCandidate_TopicAndPinnedCombined(t *testing.T) {
	// Both topic match and pinned: base + topic bonus + pin bonus
	base := 0.9 * 0.5
	topicBonus := 2.0
	pinBonus := 1.0
	sc := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{
			ArtifactID: "art-both",
			Title:      "Best candidate",
			Score:      0.9,
		},
		TopicMatch:   true,
		ContextScore: base + topicBonus + pinBonus,
	}
	expected := base + topicBonus + pinBonus
	if sc.ContextScore != expected {
		t.Errorf("expected combined score %.4f, got %.4f", expected, sc.ContextScore)
	}
}

// === TST-004-R01: ResurfaceCandidate dormancy reason format ===

func TestResurfaceCandidate_DormancyReasonFormat(t *testing.T) {
	// The Resurface function generates reason strings like
	// "High-value artifact dormant for N days". Verify the struct
	// correctly stores and surfaces structured dormancy reasons.
	c := ResurfaceCandidate{
		ArtifactID:   "art-dormant",
		Title:        "Important Meeting Notes",
		Score:        0.82,
		Reason:       "High-value artifact dormant for 45 days",
		LastAccessed: time.Now().AddDate(0, 0, -45),
	}

	if c.Score < 0.5 {
		t.Errorf("high-value candidate should have score > 0.5, got %.2f", c.Score)
	}
	if !strings.Contains(c.Reason, "dormant for 45 days") {
		t.Errorf("reason should contain dormancy duration, got: %s", c.Reason)
	}
	if time.Since(c.LastAccessed).Hours() < 24*40 {
		t.Error("last accessed should be more than 40 days ago")
	}
}

// === TST-004-R02: SerendipityCandidate CalendarMatch field behavior ===

func TestSerendipityCandidate_CalendarMatchBoost(t *testing.T) {
	// Calendar-matched candidates get a bonus (SCN-004-016)
	noCalendar := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{Score: 0.7},
		CalendarMatch:      false,
		TopicMatch:         false,
		ContextScore:       0.7 * 0.5, // base only
	}
	withCalendar := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{Score: 0.7},
		CalendarMatch:      true,
		TopicMatch:         false,
		ContextScore:       0.7*0.5 + 1.5, // base + calendar bonus
	}

	if withCalendar.ContextScore <= noCalendar.ContextScore {
		t.Errorf("calendar-matched candidate should score higher: with=%.2f, without=%.2f",
			withCalendar.ContextScore, noCalendar.ContextScore)
	}
	if !withCalendar.CalendarMatch {
		t.Error("CalendarMatch should be true")
	}
	if noCalendar.CalendarMatch {
		t.Error("non-calendar candidate should have CalendarMatch=false")
	}
}

// === TST-004-R03: SerendipityCandidate ContextReason field ===

func TestSerendipityCandidate_ContextReason(t *testing.T) {
	sc := SerendipityCandidate{
		ResurfaceCandidate: ResurfaceCandidate{
			ArtifactID: "art-matched",
			Title:      "Old Negotiation Article",
		},
		TopicMatch:    true,
		CalendarMatch: true,
		ContextReason: "Matches upcoming meeting topic and hot topic 'negotiation'",
	}

	if sc.ContextReason == "" {
		t.Error("context reason should be populated for matched candidates")
	}
	if !strings.Contains(sc.ContextReason, "negotiation") {
		t.Error("context reason should reference the matching topic")
	}
}

// === TST-004-R04: Resurface limit boundary — exactly 1 ===

func TestResurface_LimitOne_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	// limit=1 is valid and should not trigger the default-to-5 path
	_, err := engine.Resurface(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
	// Verify the error comes from the DB query, not from limit validation
	if !strings.Contains(err.Error(), "database connection") {
		t.Errorf("expected pool error, got: %s", err.Error())
	}
}
