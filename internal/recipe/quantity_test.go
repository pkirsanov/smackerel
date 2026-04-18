package recipe

import (
	"math"
	"testing"
)

func TestParseQuantity_Integer(t *testing.T) {
	qty, _ := ParseQuantity("3", "cups")
	if qty != 3 {
		t.Fatalf("expected 3, got %f", qty)
	}
}

func TestParseQuantity_Decimal(t *testing.T) {
	qty, _ := ParseQuantity("2.5", "tbsp")
	if qty != 2.5 {
		t.Fatalf("expected 2.5, got %f", qty)
	}
}

func TestParseQuantity_MixedFraction(t *testing.T) {
	qty, _ := ParseQuantity("2 1/2", "cups")
	if qty != 2.5 {
		t.Fatalf("expected 2.5, got %f", qty)
	}
}

func TestParseQuantity_SimpleFraction(t *testing.T) {
	qty, _ := ParseQuantity("1/4", "tsp")
	if qty != 0.25 {
		t.Fatalf("expected 0.25, got %f", qty)
	}
}

func TestParseQuantity_OneThird(t *testing.T) {
	qty, _ := ParseQuantity("1/3", "cup")
	if math.Abs(qty-0.333333) > 0.001 {
		t.Fatalf("expected ~0.333, got %f", qty)
	}
}

func TestParseQuantity_Empty(t *testing.T) {
	qty, _ := ParseQuantity("", "pinch")
	if qty != 0 {
		t.Fatalf("expected 0 for empty quantity, got %f", qty)
	}
}

func TestParseQuantity_UnparseableStrings(t *testing.T) {
	cases := []string{"to taste", "a pinch", "some"}
	for _, input := range cases {
		qty, _ := ParseQuantity(input, "")
		if qty != 0 {
			t.Errorf("ParseQuantity(%q) = %f, want 0", input, qty)
		}
	}
}

func TestParseQuantity_UnicodeFractions(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"½", 0.5},
		{"⅓", 0.333},
		{"⅔", 0.667},
		{"¼", 0.25},
		{"¾", 0.75},
		{"⅛", 0.125},
		{"⅜", 0.375},
		{"⅝", 0.625},
		{"⅞", 0.875},
	}

	for _, tc := range cases {
		qty, _ := ParseQuantity(tc.input, "cup")
		if math.Abs(qty-tc.expected) > 0.01 {
			t.Errorf("ParseQuantity(%q) = %f, want ~%f", tc.input, qty, tc.expected)
		}
	}
}

func TestParseQuantity_MixedWithUnicode(t *testing.T) {
	// "1½" should be handled as "1 1/2" after unicode replacement
	qty, _ := ParseQuantity("1 ½", "cup")
	if qty != 1.5 {
		t.Fatalf("expected 1.5, got %f", qty)
	}
}

func TestNormalizeUnit(t *testing.T) {
	cases := map[string]string{
		"tablespoon":  "tbsp",
		"tablespoons": "tbsp",
		"teaspoon":    "tsp",
		"cups":        "cup",
		"ounces":      "oz",
		"pounds":      "lb",
		"grams":       "g",
		"tbsp":        "tbsp",
		"Tbsp":        "tbsp",
		"":            "",
	}
	for input, expected := range cases {
		got := NormalizeUnit(input)
		if got != expected {
			t.Errorf("NormalizeUnit(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeIngredientName(t *testing.T) {
	cases := map[string]string{
		"Chicken Breasts": "chicken breast",
		"garlic":          "garlic",
		"Tomatoes":        "tomato",
		"hummus":          "hummus",
	}
	for input, expected := range cases {
		got := NormalizeIngredientName(input)
		if got != expected {
			t.Errorf("NormalizeIngredientName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestCategorizeIngredient(t *testing.T) {
	cases := map[string]string{
		"chicken breast": "proteins",
		"garlic":         "produce",
		"butter":         "dairy",
		"olive oil":      "pantry",
		"cumin":          "spices",
		"flour":          "baking",
		"water":          "beverages",
		"mystery item":   "other",
	}
	for input, expected := range cases {
		got := CategorizeIngredient(input)
		if got != expected {
			t.Errorf("CategorizeIngredient(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFormatIngredient(t *testing.T) {
	cases := []struct {
		name, unit, prep string
		qty              float64
		expected         string
	}{
		{"flour", "cup", "", 2, "2 cup flour"},
		{"chicken", "lb", "diced", 1.5, "1.5 lb chicken (diced)"},
		{"salt", "", "", 0, "salt"},
		{"garlic", "cloves", "", 3, "3 cloves garlic"},
	}
	for _, tc := range cases {
		got := FormatIngredient(tc.name, tc.qty, tc.unit, tc.prep)
		if got != tc.expected {
			t.Errorf("FormatIngredient(%q, %f, %q, %q) = %q, want %q",
				tc.name, tc.qty, tc.unit, tc.prep, got, tc.expected)
		}
	}
}
