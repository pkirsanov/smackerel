package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/mealplan"
)

func TestResolveDayName(t *testing.T) {
	tests := []struct {
		input    string
		wantDay  time.Weekday
		wantZero bool
	}{
		{"monday", time.Monday, false},
		{"Mon", time.Monday, false},
		{"tuesday", time.Tuesday, false},
		{"Tue", time.Tuesday, false},
		{"wednesday", time.Wednesday, false},
		{"Wed", time.Wednesday, false},
		{"thursday", time.Thursday, false},
		{"Thu", time.Thursday, false},
		{"friday", time.Friday, false},
		{"Fri", time.Friday, false},
		{"saturday", time.Saturday, false},
		{"Sat", time.Saturday, false},
		{"sunday", time.Sunday, false},
		{"Sun", time.Sunday, false},
		{"today", time.Now().Weekday(), false},
		{"tomorrow", time.Now().AddDate(0, 0, 1).Weekday(), false},
		{"tonight", time.Now().Weekday(), false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveDayName(tt.input)
			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("resolveDayName(%q) should be zero, got %v", tt.input, got)
				}
				return
			}
			if got.IsZero() {
				t.Errorf("resolveDayName(%q) should not be zero", tt.input)
				return
			}
			if got.Weekday() != tt.wantDay {
				t.Errorf("resolveDayName(%q) weekday = %v, want %v", tt.input, got.Weekday(), tt.wantDay)
			}
		})
	}
}

func TestThisWeekRange(t *testing.T) {
	start, end := thisWeekRange()

	if start.Weekday() != time.Monday {
		t.Errorf("start weekday = %v, want Monday", start.Weekday())
	}
	if end.Weekday() != time.Sunday {
		t.Errorf("end weekday = %v, want Sunday", end.Weekday())
	}
	if end.Sub(start) != 6*24*time.Hour {
		t.Errorf("duration = %v, want 6 days", end.Sub(start))
	}
}

func TestNextWeekRange(t *testing.T) {
	start, end := nextWeekRange()

	if start.Weekday() != time.Monday {
		t.Errorf("start weekday = %v, want Monday", start.Weekday())
	}
	if end.Weekday() != time.Sunday {
		t.Errorf("end weekday = %v, want Sunday", end.Weekday())
	}

	thisStart, _ := thisWeekRange()
	if start.Sub(thisStart) != 7*24*time.Hour {
		t.Errorf("next week start should be 7 days after this week start")
	}
}

func TestMealPlanPatternMatching(t *testing.T) {
	tests := []struct {
		input string
		re    string
		want  bool
	}{
		{"meal plan this week", "mealPlanThisWeek", true},
		{"Meal Plan This Week", "mealPlanThisWeek", true},
		{"meal plan next week", "mealPlanNextWeek", true},
		{"activate plan", "activatePlan", true},
		{"meal plan", "showPlan", true},
		{"show plan", "showPlan", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var matched bool
			switch tt.re {
			case "mealPlanThisWeek":
				matched = mealPlanThisWeekRe.MatchString(tt.input)
			case "mealPlanNextWeek":
				matched = mealPlanNextWeekRe.MatchString(tt.input)
			case "activatePlan":
				matched = activatePlanRe.MatchString(tt.input)
			case "showPlan":
				matched = showPlanRe.MatchString(tt.input)
			}
			if matched != tt.want {
				t.Errorf("pattern %s match(%q) = %v, want %v", tt.re, tt.input, matched, tt.want)
			}
		})
	}
}

func TestSlotAssignPattern(t *testing.T) {
	tests := []struct {
		input        string
		wantDay      string
		wantMeal     string
		wantRecipe   string
		wantServings string
	}{
		{
			"Monday dinner Pasta Carbonara for 4",
			"Monday", "dinner", "Pasta Carbonara", "4",
		},
		{
			"Tue lunch: Caesar Salad",
			"Tue", "lunch", "Caesar Salad", "",
		},
		{
			"Wed breakfast Overnight Oats for 2 servings",
			"Wed", "breakfast", "Overnight Oats", "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := slotAssignRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.wantDay {
				t.Errorf("day = %q, want %q", m[1], tt.wantDay)
			}
			if m[2] != tt.wantMeal {
				t.Errorf("meal = %q, want %q", m[2], tt.wantMeal)
			}
			if m[3] != tt.wantRecipe {
				t.Errorf("recipe = %q, want %q", m[3], tt.wantRecipe)
			}
			servings := ""
			if len(m) >= 5 {
				servings = m[4]
			}
			if servings != tt.wantServings {
				t.Errorf("servings = %q, want %q", servings, tt.wantServings)
			}
		})
	}
}

func TestBatchSlotPattern(t *testing.T) {
	tests := []struct {
		input      string
		wantStart  string
		wantEnd    string
		wantMeal   string
		wantRecipe string
	}{
		{
			"Mon-Thu breakfast: Overnight Oats for 2",
			"Mon", "Thu", "breakfast", "Overnight Oats",
		},
		{
			"Tue-Fri dinner: Pasta",
			"Tue", "Fri", "dinner", "Pasta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := batchSlotRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.wantStart {
				t.Errorf("start = %q, want %q", m[1], tt.wantStart)
			}
			if m[2] != tt.wantEnd {
				t.Errorf("end = %q, want %q", m[2], tt.wantEnd)
			}
			if m[3] != tt.wantMeal {
				t.Errorf("meal = %q, want %q", m[3], tt.wantMeal)
			}
			if m[4] != tt.wantRecipe {
				t.Errorf("recipe = %q, want %q", m[4], tt.wantRecipe)
			}
		})
	}
}

func TestWhatsForPattern(t *testing.T) {
	tests := []struct {
		input    string
		wantMeal string
		wantDay  string
	}{
		{"what's for dinner?", "dinner", ""},
		{"what's for dinner tomorrow?", "dinner", "tomorrow"},
		{"what's for lunch Tuesday?", "lunch", "Tuesday"},
		{"what is for dinner?", "dinner", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := whatsForMealRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.wantMeal {
				t.Errorf("meal = %q, want %q", m[1], tt.wantMeal)
			}
			day := ""
			if len(m) >= 3 {
				day = m[2]
			}
			if day != tt.wantDay {
				t.Errorf("day = %q, want %q", day, tt.wantDay)
			}
		})
	}
}

func TestCookFromPlanPattern(t *testing.T) {
	tests := []struct {
		input    string
		wantMeal string
	}{
		{"cook tonight's dinner", "dinner"},
		{"cook tonights dinner", "dinner"},
		{"cook tonight's breakfast", "breakfast"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := cookTonightRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			if m[1] != tt.wantMeal {
				t.Errorf("meal = %q, want %q", m[1], tt.wantMeal)
			}
		})
	}
}

func TestFormatPlanView(t *testing.T) {
	plan := &mealplan.PlanWithSlots{
		Plan: mealplan.Plan{
			Title:     "Week of Apr 20",
			StartDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
			Status:    mealplan.StatusActive,
		},
		Slots: []mealplan.Slot{
			{
				SlotDate:    time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
				MealType:    "dinner",
				RecipeTitle: "Pasta Carbonara",
				Servings:    4,
			},
			{
				SlotDate:    time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
				MealType:    "dinner",
				RecipeTitle: "Thai Green Curry",
				Servings:    2,
			},
		},
	}

	output := formatPlanView(plan)
	if output == "" {
		t.Error("expected non-empty output")
	}

	// Check key elements
	if !mealPlanContains(output, "Week of Apr 20") {
		t.Error("expected plan title in output")
	}
	if !mealPlanContains(output, "active") {
		t.Error("expected status in output")
	}
	if !mealPlanContains(output, "Pasta Carbonara") {
		t.Error("expected recipe name in output")
	}
	if !mealPlanContains(output, "2 meals planned") {
		t.Error("expected meal count in output")
	}
}

func mealPlanContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Round 44: Draft context cleanup sweeps expired entries
func TestMealPlanCommandHandler_SweepDrafts(t *testing.T) {
	h := NewMealPlanCommandHandler(nil, nil)

	h.setDraft(100, "plan-100")
	h.setDraft(200, "plan-200")

	// Manually expire entry 100
	h.mu.Lock()
	h.drafts[100].ExpiresAt = time.Now().Add(-1 * time.Minute)
	h.mu.Unlock()

	h.sweepDrafts()

	if h.getDraft(100) != "" {
		t.Error("expected expired draft 100 to be swept")
	}
	if h.getDraft(200) != "plan-200" {
		t.Error("expected active draft 200 to be preserved")
	}
}

// Round 44: Stop is idempotent
func TestMealPlanCommandHandler_StopIdempotent(t *testing.T) {
	h := NewMealPlanCommandHandler(nil, nil)
	h.StartCleanup()
	h.Stop()
	h.Stop() // second call should not panic
}

// Round 82: weeklyMealRe must parse "lunches this week" → meal "lunch"
func TestWeeklyMealPattern_PluralStripping(t *testing.T) {
	tests := []struct {
		input    string
		wantMeal string
	}{
		{"dinners this week", "dinner"},
		{"lunches this week", "lunch"},
		{"breakfasts this week", "breakfast"},
		{"snacks this week", "snack"},
		{"Dinners this week?", "dinner"},
		{"Lunches This Week?", "lunch"},
		// singular form should also work
		{"dinner this week", "dinner"},
		{"lunch this week", "lunch"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := weeklyMealRe.FindStringSubmatch(tt.input)
			if m == nil {
				t.Fatalf("no match for %q", tt.input)
			}
			// Replicate the handler's plural-stripping logic
			meal := strings.ToLower(m[1])
			if trimmed := strings.TrimSuffix(meal, "es"); trimmed != meal {
				meal = trimmed
			} else {
				meal = strings.TrimSuffix(meal, "s")
			}
			if meal != tt.wantMeal {
				t.Errorf("meal = %q, want %q (raw capture: %q)", meal, tt.wantMeal, m[1])
			}
		})
	}
}
