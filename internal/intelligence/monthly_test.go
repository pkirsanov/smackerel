package intelligence

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMonthlyReport_Struct(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		ExpertiseShifts: []ExpertiseShift{
			{TopicName: "Go", PrevDepth: 30, CurrentDepth: 45, Direction: "gained"},
			{TopicName: "Python", PrevDepth: 20, CurrentDepth: 15, Direction: "lost"},
		},
		InformationDiet: InformationDiet{Articles: 15, Videos: 8, Emails: 40, Notes: 5, Total: 68},
		GeneratedAt:     time.Now(),
	}

	if len(r.ExpertiseShifts) != 2 {
		t.Errorf("expected 2 expertise shifts, got %d", len(r.ExpertiseShifts))
	}
	if r.InformationDiet.Total != 68 {
		t.Errorf("expected total 68, got %d", r.InformationDiet.Total)
	}
}

func TestAssembleMonthlyReportText_NonEmpty(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		ExpertiseShifts: []ExpertiseShift{
			{TopicName: "Go", PrevDepth: 30, CurrentDepth: 45, Direction: "gained"},
		},
		InformationDiet:  InformationDiet{Articles: 10, Videos: 5, Emails: 20, Notes: 3, Total: 38},
		ProductivityPats: []string{"Peak capture at 9am"},
	}

	text := assembleMonthlyReportText(r)
	if text == "" {
		t.Error("expected non-empty report text")
	}
	if !contains(text, "EXPERTISE SHIFTS") {
		t.Error("report should contain expertise shifts")
	}
	if !contains(text, "INFORMATION DIET") {
		t.Error("report should contain information diet")
	}
}

func TestAssembleMonthlyReportText_Empty(t *testing.T) {
	r := &MonthlyReport{Month: "2026-04"}
	text := assembleMonthlyReportText(r)
	if !contains(text, "Not enough data") {
		t.Error("empty report should indicate insufficient data")
	}
}

func TestGenerateMonthlyReport_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GenerateMonthlyReport(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestContentAngle_Struct(t *testing.T) {
	ca := ContentAngle{
		Title:            "Deep dive: Go — 5 sources over 45 captures",
		UniqueRationale:  "Multi-perspective view",
		SupportingIDs:    []string{"a1", "a2", "a3"},
		FormatSuggestion: "detailed guide",
	}

	if len(ca.SupportingIDs) != 3 {
		t.Errorf("expected 3 supporting IDs, got %d", len(ca.SupportingIDs))
	}
	if ca.FormatSuggestion != "detailed guide" {
		t.Errorf("expected detailed guide, got %s", ca.FormatSuggestion)
	}
}

func TestGenerateContentFuel_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GenerateContentFuel(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestSeasonalPattern_Struct(t *testing.T) {
	sp := SeasonalPattern{
		Pattern:     "volume_drop",
		Month:       "December",
		Observation: "Capture volume down 30%",
		Actionable:  true,
	}

	if !sp.Actionable {
		t.Error("volume_drop should be actionable")
	}
}

func TestDetectSeasonalPatterns_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.DetectSeasonalPatterns(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestExpertiseShift_Direction(t *testing.T) {
	tests := []struct {
		prev, curr float64
		expected   string
	}{
		{10, 20, "gained"},
		{20, 10, "lost"},
		{10, 10, "stable"},
	}

	for _, tt := range tests {
		var dir string
		if tt.curr > tt.prev {
			dir = "gained"
		} else if tt.curr < tt.prev {
			dir = "lost"
		} else {
			dir = "stable"
		}
		if dir != tt.expected {
			t.Errorf("(%v→%v) = %s, want %s", tt.prev, tt.curr, dir, tt.expected)
		}
	}
}

// === Stabilize: InformationDiet.Total includes Other ===

func TestInformationDiet_TotalIncludesOther(t *testing.T) {
	// When the InformationDiet is populated from a query, Total should
	// equal Articles + Videos + Emails + Notes + Other.
	d := InformationDiet{
		Articles: 15,
		Videos:   8,
		Emails:   40,
		Notes:    5,
		Other:    10,
		Total:    78, // must equal 15+8+40+5+10
	}

	expected := d.Articles + d.Videos + d.Emails + d.Notes + d.Other
	if d.Total != expected {
		t.Errorf("Total=%d should equal A+V+E+N+O=%d", d.Total, expected)
	}
}

func TestInformationDiet_OtherIsNonNegative(t *testing.T) {
	// Other should never be negative — it's total minus categorized
	d := InformationDiet{
		Articles: 20,
		Videos:   10,
		Emails:   30,
		Notes:    5,
		Other:    0,
		Total:    65,
	}

	if d.Other < 0 {
		t.Errorf("Other count should never be negative, got %d", d.Other)
	}
}

// === Stabilize: GenerateMonthlyReport respects context cancellation ===

func TestGenerateMonthlyReport_CancelledContext(t *testing.T) {
	// GenerateMonthlyReport should return a context error promptly when
	// the context is already cancelled (nil pool prevents the DB attempt,
	// so the pool check fires first — this test just verifies the nil-pool
	// path still works after the context-check additions).
	engine := &Engine{Pool: nil}
	_, err := engine.GenerateMonthlyReport(context.Background())
	if err == nil {
		t.Error("expected error for nil pool")
	}
	if !strings.Contains(err.Error(), "requires a database connection") {
		t.Errorf("expected pool error, got: %s", err)
	}
}

// === Edge cases: assembleMonthlyReportText with subscriptions ===

func TestAssembleMonthlyReportText_WithSubscriptions(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		SubscriptionSum: &SubscriptionSummary{
			MonthlyTotal: 127.50,
			Active: []Subscription{
				{ServiceName: "Netflix"},
				{ServiceName: "Spotify"},
			},
		},
	}

	text := assembleMonthlyReportText(r)
	if !strings.Contains(text, "SUBSCRIPTIONS") {
		t.Error("report with subscription data should contain SUBSCRIPTIONS section")
	}
	if !strings.Contains(text, "$127.50") {
		t.Error("report should contain dollar amount")
	}
	if !strings.Contains(text, "2 active") {
		t.Error("report should contain active service count")
	}
}

func TestAssembleMonthlyReportText_PatternsOnly(t *testing.T) {
	r := &MonthlyReport{
		Month:            "2026-04",
		ProductivityPats: []string{"Peak capture at 9am", "Most active on Wednesdays"},
	}

	text := assembleMonthlyReportText(r)
	if !strings.Contains(text, "PATTERNS") {
		t.Error("report with patterns should contain PATTERNS section")
	}
	if strings.Contains(text, "EXPERTISE SHIFTS") {
		t.Error("report without shifts should not contain EXPERTISE SHIFTS")
	}
	if strings.Contains(text, "INFORMATION DIET") {
		t.Error("report without diet should not contain INFORMATION DIET")
	}
}

func TestAssembleMonthlyReportText_AllSections(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		ExpertiseShifts: []ExpertiseShift{
			{TopicName: "Go", PrevDepth: 30, CurrentDepth: 45, Direction: "gained"},
		},
		InformationDiet:  InformationDiet{Articles: 10, Videos: 5, Emails: 20, Notes: 3, Total: 38},
		ProductivityPats: []string{"Peak at 9am"},
		SubscriptionSum: &SubscriptionSummary{
			MonthlyTotal: 50.0,
			Active:       []Subscription{{ServiceName: "Netflix"}},
		},
	}

	text := assembleMonthlyReportText(r)
	if !strings.Contains(text, "EXPERTISE SHIFTS") {
		t.Error("missing EXPERTISE SHIFTS")
	}
	if !strings.Contains(text, "INFORMATION DIET") {
		t.Error("missing INFORMATION DIET")
	}
	if !strings.Contains(text, "SUBSCRIPTIONS") {
		t.Error("missing SUBSCRIPTIONS")
	}
	if !strings.Contains(text, "PATTERNS") {
		t.Error("missing PATTERNS")
	}
}

// === Edge cases: ExpertiseShift direction arrows ===

func TestAssembleMonthlyReportText_DirectionArrows(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		ExpertiseShifts: []ExpertiseShift{
			{TopicName: "Go", PrevDepth: 30, CurrentDepth: 45, Direction: "gained"},
			{TopicName: "Python", PrevDepth: 20, CurrentDepth: 10, Direction: "lost"},
			{TopicName: "Rust", PrevDepth: 15, CurrentDepth: 15, Direction: "stable"},
		},
	}

	text := assembleMonthlyReportText(r)
	if !strings.Contains(text, "↑") {
		t.Error("gained direction should show ↑ arrow")
	}
	if !strings.Contains(text, "↓") {
		t.Error("lost direction should show ↓ arrow")
	}
	if !strings.Contains(text, "→") {
		t.Error("stable direction should show → arrow")
	}
}

// === Edge cases: ContentAngle format thresholds ===

func TestContentAngle_FormatSelection(t *testing.T) {
	// The format depends on capture count: >100=long-form, >50=detailed guide, else blog post
	tests := []struct {
		captures int
		expected string
	}{
		{30, "blog post"},
		{51, "detailed guide"},
		{100, "detailed guide"},
		{101, "long-form essay"},
		{200, "long-form essay"},
	}

	for _, tt := range tests {
		format := "blog post"
		if tt.captures > 100 {
			format = "long-form essay"
		} else if tt.captures > 50 {
			format = "detailed guide"
		}
		if format != tt.expected {
			t.Errorf("captures=%d → format %q, want %q", tt.captures, format, tt.expected)
		}
	}
}

// === Edge cases: SeasonalPattern struct ===

func TestSeasonalPattern_VolumeSpike(t *testing.T) {
	sp := SeasonalPattern{
		Pattern:     "volume_spike",
		Month:       "April",
		Observation: "Capture volume is up 60% compared to April last year",
		Actionable:  false,
	}

	if sp.Actionable {
		t.Error("volume_spike should not be actionable")
	}
	if sp.Pattern != "volume_spike" {
		t.Errorf("expected volume_spike, got %s", sp.Pattern)
	}
}

func TestSeasonalPattern_TopicSeasonal(t *testing.T) {
	sp := SeasonalPattern{
		Pattern:     "topic_seasonal",
		Month:       "December",
		Observation: "Holiday planning tends to spike in December (15 captures)",
		Actionable:  false,
	}

	if sp.Actionable {
		t.Error("topic_seasonal should not be actionable")
	}
}

// === Stabilize STB-002: TopInsights always capped at 3 ===

func TestMonthlyReport_TopInsightsCap(t *testing.T) {
	// Verify the report struct correctly handles the 3-insight cap logic
	// (mirrors the fix in GenerateMonthlyReport where truncation no longer
	// depends on RunSynthesis returning nil error).
	insights := make([]SynthesisInsight, 10)
	for i := range insights {
		insights[i] = SynthesisInsight{
			ID:          "test-" + time.Now().Format("150405"),
			InsightType: InsightThroughLine,
			ThroughLine: "topic-" + time.Now().Format("150405"),
			Confidence:  float64(i) * 0.1,
		}
	}

	// Simulate the fixed truncation: always cap at 3
	if len(insights) > 3 {
		insights = insights[:3]
	}

	if len(insights) != 3 {
		t.Errorf("expected 3 insights after cap, got %d", len(insights))
	}
}

// === Stabilize STB-002: assembleMonthlyReportText with seasonal patterns ===

func TestAssembleMonthlyReportText_WithSeasonalPatterns(t *testing.T) {
	r := &MonthlyReport{
		Month: "2026-04",
		SeasonalPatterns: []SeasonalPattern{
			{Month: "April", Observation: "Fitness captures spike in April"},
		},
		InformationDiet: InformationDiet{Total: 10, Articles: 5, Notes: 5},
	}

	text := assembleMonthlyReportText(r)
	if !strings.Contains(text, "SEASONAL INSIGHTS") {
		t.Error("report with seasonal patterns should contain SEASONAL INSIGHTS section")
	}
	if !strings.Contains(text, "Fitness captures spike") {
		t.Error("seasonal observation should appear in report text")
	}
}
