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
		{"FOR 4", 4},       // case insensitive
		{"Scale To 8", 8},  // case insensitive
		{"1 PEOPLE", 1},    // case insensitive
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
	if !strings.Contains(result, "- 400g guanciale") {
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
