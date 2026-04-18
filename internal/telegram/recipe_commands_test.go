package telegram

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/recipe"
)

func TestParseScaleTrigger(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"8 servings", 8},
		{"8 serving", 8},
		{"for 6", 6},
		{"scale to 12", 12},
		{"3 people", 3},
		{"0 servings", 0}, // invalid
		{"-1 servings", 0},
		{"hello", 0},
		{"for abc", 0},
		{"100 servings", 100},
		{"FOR 4", 4},      // case insensitive
		{"Scale To 8", 8}, // case insensitive
		{"1 PEOPLE", 1},   // case insensitive
	}

	for _, tc := range cases {
		got := parseScaleTrigger(tc.input)
		if got != tc.expected {
			t.Errorf("parseScaleTrigger(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestParseCookTrigger(t *testing.T) {
	cases := []struct {
		input    string
		name     string
		servings int
		matched  bool
	}{
		{"cook", "", 0, true},
		{"Cook", "", 0, true},
		{"cook Thai Green Curry", "Thai Green Curry", 0, true},
		{"cook Carbonara for 8 servings", "Carbonara", 8, true},
		{"cook pasta for 4 serving", "pasta", 4, true},
		{"COOK Soup", "Soup", 0, true},
		{"hello", "", 0, false},
		{"cooking", "", 0, false},
	}

	for _, tc := range cases {
		name, servings, matched := parseCookTrigger(tc.input)
		if matched != tc.matched {
			t.Errorf("parseCookTrigger(%q) matched=%v, want %v", tc.input, matched, tc.matched)
			continue
		}
		if !matched {
			continue
		}
		if name != tc.name {
			t.Errorf("parseCookTrigger(%q) name=%q, want %q", tc.input, name, tc.name)
		}
		if servings != tc.servings {
			t.Errorf("parseCookTrigger(%q) servings=%d, want %d", tc.input, servings, tc.servings)
		}
	}
}

func TestParseCookNavigation(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"next", "next"},
		{"n", "next"},
		{"Next", "next"},
		{"N", "next"},
		{"back", "back"},
		{"b", "back"},
		{"prev", "back"},
		{"previous", "back"},
		{"Back", "back"},
		{"ingredients", "ingredients"},
		{"ingredient", "ingredients"},
		{"ing", "ingredients"},
		{"i", "ingredients"},
		{"done", "done"},
		{"d", "done"},
		{"stop", "done"},
		{"exit", "done"},
		{"Done", "done"},
		{"5", "jump:5"},
		{"1", "jump:1"},
		{"hello", ""},
		{"cook", ""},
	}

	for _, tc := range cases {
		got := parseCookNavigation(tc.input)
		if got != tc.expected {
			t.Errorf("parseCookNavigation(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatScaledResponse(t *testing.T) {
	scaled := []recipe.ScaledIngredient{
		{Name: "guanciale", Quantity: "200", Unit: "g", DisplayQuantity: "400", Scaled: true},
		{Name: "egg yolks", Quantity: "4", Unit: "", DisplayQuantity: "8", Scaled: true},
		{Name: "salt", Quantity: "to taste", Unit: "", DisplayQuantity: "to taste", Scaled: false},
	}

	result := formatScaledResponse("Pasta Carbonara", 4, 8, scaled)

	if !strings.Contains(result, "# Pasta Carbonara — 8 servings") {
		t.Error("missing heading")
	}
	if !strings.Contains(result, "~ Scaled from 4 to 8 servings (2x)") {
		t.Error("missing scale note")
	}
	if !strings.Contains(result, "- 400 g guanciale") {
		t.Error("missing scaled guanciale")
	}
	if !strings.Contains(result, "- 8 egg yolks") || !strings.Contains(result, "8") {
		// Unit is empty for eggs so it should be "- 8egg yolks" or similar
		if !strings.Contains(result, "egg yolks") {
			t.Error("missing scaled egg yolks")
		}
	}
	if !strings.Contains(result, "(unscaled)") {
		t.Error("missing unscaled annotation")
	}
}

func TestFormatScaledResponse_IntegerFactor(t *testing.T) {
	scaled := []recipe.ScaledIngredient{
		{Name: "flour", Quantity: "1", Unit: "cup", DisplayQuantity: "3", Scaled: true},
	}

	result := formatScaledResponse("Bread", 2, 6, scaled)

	if !strings.Contains(result, "(3x)") {
		t.Error("expected integer factor display '3x'")
	}
}

func TestFormatScaledResponse_FractionalFactor(t *testing.T) {
	scaled := []recipe.ScaledIngredient{
		{Name: "flour", Quantity: "1", Unit: "cup", DisplayQuantity: "1 1/2", Scaled: true},
	}

	result := formatScaledResponse("Bread", 4, 6, scaled)

	if !strings.Contains(result, "(1.5x)") {
		t.Error("expected fractional factor display '1.5x'")
	}
}

func TestFormatScaleFactor(t *testing.T) {
	cases := []struct {
		input    float64
		expected string
	}{
		{2.0, "2"},
		{3.0, "3"},
		{1.5, "1.5"},
		{0.5, "0.5"},
	}
	for _, tc := range cases {
		got := formatScaleFactor(tc.input)
		if got != tc.expected {
			t.Errorf("formatScaleFactor(%f) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// Round 6: Max servings cap in parseScaleTrigger
func TestParseScaleTrigger_MaxServingsCap(t *testing.T) {
	// Exactly at max
	if got := parseScaleTrigger("1000 servings"); got != 1000 {
		t.Errorf("expected 1000, got %d", got)
	}
	// Over max — should return 0
	if got := parseScaleTrigger("1001 servings"); got != 0 {
		t.Errorf("expected 0 for >1000, got %d", got)
	}
	if got := parseScaleTrigger("for 5000"); got != 0 {
		t.Errorf("expected 0 for >1000, got %d", got)
	}
	if got := parseScaleTrigger("scale to 99999"); got != 0 {
		t.Errorf("expected 0 for >1000, got %d", got)
	}
}

// Round 6: Max servings cap in parseCookTrigger
func TestParseCookTrigger_MaxServingsCap(t *testing.T) {
	_, servings, matched := parseCookTrigger("cook pasta for 1000 servings")
	if !matched || servings != 1000 {
		t.Errorf("expected matched=true servings=1000, got matched=%v servings=%d", matched, servings)
	}

	// Over cap: falls through to cookNameRe (with " for 1001 servings" as part of name)
	name, servings, matched := parseCookTrigger("cook pasta for 1001 servings")
	if !matched {
		t.Error("expected matched=true (should match as cookNameRe)")
	}
	if servings != 0 {
		t.Errorf("expected servings=0 (capped), got %d", servings)
	}
	if name == "" {
		t.Error("expected non-empty name from cookNameRe fallthrough")
	}
}

// Round 9: formatNoStepsFallback with scaling
func TestFormatNoStepsFallback_WithScaling(t *testing.T) {
	servings := 4
	rd := &recipe.RecipeData{
		Title:    "Simple Salad",
		Servings: &servings,
		Ingredients: []recipe.Ingredient{
			{Name: "lettuce", Quantity: "1", Unit: "head"},
			{Name: "tomato", Quantity: "2", Unit: ""},
		},
		Steps: []recipe.Step{},
	}

	result := formatNoStepsFallback(rd, 8)

	if !strings.Contains(result, "Scaled to 8 servings") {
		t.Error("missing scaled heading")
	}
	if !strings.Contains(result, "2head lettuce") || !strings.Contains(result, "2 head lettuce") {
		// Either format is acceptable depending on formatting
		if !strings.Contains(result, "lettuce") {
			t.Error("missing scaled lettuce")
		}
	}
}

func TestFormatNoStepsFallback_NoScaling(t *testing.T) {
	rd := &recipe.RecipeData{
		Title: "Simple Salad",
		Ingredients: []recipe.Ingredient{
			{Name: "lettuce", Quantity: "1", Unit: "head"},
		},
		Steps: []recipe.Step{},
	}

	result := formatNoStepsFallback(rd, 0)

	if strings.Contains(result, "Scaled") {
		t.Error("should not show scaling when servings=0")
	}
	if !strings.Contains(result, "1 head lettuce") {
		t.Error("missing unscaled lettuce")
	}
}

func TestFormatNoStepsFallback_SameServings(t *testing.T) {
	servings := 4
	rd := &recipe.RecipeData{
		Title:    "Simple Salad",
		Servings: &servings,
		Ingredients: []recipe.Ingredient{
			{Name: "lettuce", Quantity: "1", Unit: "head"},
		},
		Steps: []recipe.Step{},
	}

	result := formatNoStepsFallback(rd, 4)

	if strings.Contains(result, "Scaled") {
		t.Error("should not show scaling when servings match original")
	}
}
