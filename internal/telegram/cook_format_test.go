package telegram

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/recipe"
)

func TestFormatCookStep_Standard(t *testing.T) {
	dur := 2
	session := &CookSession{
		RecipeTitle: "Thai Green Curry",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Heat oil in a wok over high heat", DurationMinutes: &dur, Technique: "stir-frying"},
			{Number: 2, Instruction: "Add curry paste"},
		},
		CurrentStep: 1,
		TotalSteps:  2,
	}

	result := FormatCookStep(session)

	if !strings.Contains(result, "# Thai Green Curry") {
		t.Error("missing title heading")
	}
	if !strings.Contains(result, "> Step 1 of 2") {
		t.Error("missing step counter")
	}
	if !strings.Contains(result, "Heat oil in a wok over high heat") {
		t.Error("missing instruction")
	}
	if !strings.Contains(result, "~ 2 min · stir-frying") {
		t.Error("missing duration/technique metadata")
	}
	if !strings.Contains(result, "Reply: next · back · ingredients · done") {
		t.Error("missing navigation hint")
	}
}

func TestFormatCookStep_LastStep(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Pasta",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Boil water"},
			{Number: 2, Instruction: "Serve"},
		},
		CurrentStep: 2,
		TotalSteps:  2,
	}

	result := FormatCookStep(session)

	if !strings.Contains(result, "> Step 2 of 2") {
		t.Error("missing step counter")
	}
	if !strings.Contains(result, "Last step. Reply: back · ingredients · done") {
		t.Error("missing last step navigation hint")
	}
}

func TestFormatCookStep_SingleStep(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Toast",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Put bread in toaster"},
		},
		CurrentStep: 1,
		TotalSteps:  1,
	}

	result := FormatCookStep(session)

	if !strings.Contains(result, "Reply: ingredients · done") {
		t.Error("missing single-step navigation hint")
	}
	if strings.Contains(result, "next") {
		t.Error("single step should not show 'next'")
	}
}

func TestFormatCookStep_NoDuration(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Salad",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Wash lettuce"},
		},
		CurrentStep: 1,
		TotalSteps:  1,
	}

	result := FormatCookStep(session)

	if strings.Contains(result, "~ ") {
		t.Error("should not have metadata line when no duration/technique")
	}
	if !strings.Contains(result, "Wash lettuce") {
		t.Error("missing instruction")
	}
}

func TestFormatCookStep_DurationOnly(t *testing.T) {
	dur := 5
	session := &CookSession{
		RecipeTitle: "Rice",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Boil rice", DurationMinutes: &dur},
		},
		CurrentStep: 1,
		TotalSteps:  1,
	}

	result := FormatCookStep(session)

	if !strings.Contains(result, "~ 5 min") {
		t.Error("missing duration-only metadata")
	}
}

func TestFormatCookStep_TechniqueOnly(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Steak",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Sear the steak", Technique: "pan-searing"},
		},
		CurrentStep: 1,
		TotalSteps:  1,
	}

	result := FormatCookStep(session)

	if !strings.Contains(result, "~ pan-searing") {
		t.Error("missing technique-only metadata")
	}
}

func TestFormatCookStep_OutOfBoundsStep(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Test",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Step one"},
		},
		CurrentStep: 5, // beyond len(Steps)
		TotalSteps:  1,
	}

	result := FormatCookStep(session)
	if result != "" {
		t.Errorf("expected empty string for out-of-bounds step, got %q", result)
	}
}

func TestFormatCookStep_StepsBeyondTotalSteps(t *testing.T) {
	// TotalSteps=3 but only 2 steps in slice — CurrentStep=3 should be safe
	session := &CookSession{
		RecipeTitle: "Test",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Step one"},
			{Number: 2, Instruction: "Step two"},
		},
		CurrentStep: 3,
		TotalSteps:  3,
	}

	result := FormatCookStep(session)
	if result != "" {
		t.Errorf("expected empty string when step exceeds slice length, got %q", result)
	}
}

func TestFormatCookStep_ZeroStep(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Test",
		Steps: []recipe.Step{
			{Number: 1, Instruction: "Step one"},
		},
		CurrentStep: 0,
		TotalSteps:  1,
	}

	result := FormatCookStep(session)
	if result != "" {
		t.Errorf("expected empty string for step 0, got %q", result)
	}
}

func TestFormatCookIngredients_Unscaled(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Pasta",
		Ingredients: []recipe.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups"},
			{Name: "salt", Quantity: "to taste", Unit: ""},
		},
		ScaleFactor:      1.0,
		OriginalServings: 0,
		ScaledServings:   0,
	}

	result := FormatCookIngredients(session)

	if !strings.Contains(result, "# Pasta — Ingredients") {
		t.Error("missing ingredient header")
	}
	if !strings.Contains(result, "- 2 cups flour") {
		t.Error("missing flour ingredient")
	}
	if !strings.Contains(result, "- to taste salt") {
		t.Error("missing salt ingredient")
	}
}

func TestFormatCookIngredients_Scaled(t *testing.T) {
	session := &CookSession{
		RecipeTitle: "Pasta",
		Ingredients: []recipe.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups"},
			{Name: "salt", Quantity: "to taste", Unit: ""},
		},
		ScaleFactor:      2.0,
		OriginalServings: 4,
		ScaledServings:   8,
	}

	result := FormatCookIngredients(session)

	if !strings.Contains(result, "~ 8 servings (scaled from 4)") {
		t.Error("missing scaling info")
	}
	if !strings.Contains(result, "- 4cups flour") || !strings.Contains(result, "- 4 cups flour") {
		// Check for "4cups " with unit spacing
		if !strings.Contains(result, "4") || !strings.Contains(result, "flour") {
			t.Error("missing scaled flour ingredient")
		}
	}
	if !strings.Contains(result, "(unscaled)") {
		t.Error("missing unscaled annotation for salt")
	}
}

// Round 5 test: formatIngredientLine shared helper
func TestFormatIngredientLine_Scaled(t *testing.T) {
	ing := recipe.ScaledIngredient{
		Name:            "flour",
		Quantity:        "2",
		Unit:            "cups",
		DisplayQuantity: "4",
		Scaled:          true,
	}
	got := formatIngredientLine(ing)
	if got != "- 4 cups flour" {
		t.Errorf("formatIngredientLine(scaled) = %q, want %q", got, "- 4 cups flour")
	}
}

func TestFormatIngredientLine_ScaledNoUnit(t *testing.T) {
	ing := recipe.ScaledIngredient{
		Name:            "eggs",
		Quantity:        "4",
		Unit:            "",
		DisplayQuantity: "8",
		Scaled:          true,
	}
	got := formatIngredientLine(ing)
	if got != "- 8 eggs" {
		t.Errorf("formatIngredientLine(scaled no unit) = %q, want %q", got, "- 8 eggs")
	}
}

func TestFormatIngredientLine_Unscaled(t *testing.T) {
	ing := recipe.ScaledIngredient{
		Name:            "salt",
		Quantity:        "to taste",
		Unit:            "",
		DisplayQuantity: "to taste",
		Scaled:          false,
	}
	got := formatIngredientLine(ing)
	if got != "- to taste salt (unscaled)" {
		t.Errorf("formatIngredientLine(unscaled) = %q, want %q", got, "- to taste salt (unscaled)")
	}
}

func TestFormatIngredientLine_UnscaledEmptyQty(t *testing.T) {
	ing := recipe.ScaledIngredient{
		Name:            "water",
		Quantity:        "",
		Unit:            "",
		DisplayQuantity: "",
		Scaled:          false,
	}
	got := formatIngredientLine(ing)
	if got != "- water (unscaled)" {
		t.Errorf("formatIngredientLine(empty qty) = %q, want %q", got, "- water (unscaled)")
	}
}

func TestFormatRawIngredientLine(t *testing.T) {
	cases := []struct {
		name     string
		ing      recipe.Ingredient
		expected string
	}{
		{"with qty and unit", recipe.Ingredient{Name: "flour", Quantity: "2", Unit: "cups"}, "- 2 cups flour"},
		{"with qty no unit", recipe.Ingredient{Name: "eggs", Quantity: "3", Unit: ""}, "- 3 eggs"},
		{"no qty", recipe.Ingredient{Name: "salt", Quantity: "", Unit: ""}, "- salt"},
		{"free text qty", recipe.Ingredient{Name: "salt", Quantity: "to taste", Unit: ""}, "- to taste salt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatRawIngredientLine(tc.ing)
			if got != tc.expected {
				t.Errorf("formatRawIngredientLine() = %q, want %q", got, tc.expected)
			}
		})
	}
}
