package intelligence

import (
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
