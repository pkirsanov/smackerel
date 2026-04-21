package list

import (
	"encoding/json"
	"testing"
)

func TestRecipeAggregator_MergeDuplicates(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"2","unit":"cloves"}]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"3","unit":"cloves"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(seeds))
	}

	if seeds[0].Quantity == nil || *seeds[0].Quantity != 5 {
		t.Fatalf("expected quantity 5, got %v", seeds[0].Quantity)
	}

	if len(seeds[0].SourceArtifactIDs) != 2 {
		t.Fatalf("expected 2 source artifacts, got %d", len(seeds[0].SourceArtifactIDs))
	}
}

func TestRecipeAggregator_DifferentIngredients(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"chicken","quantity":"2","unit":"lbs"},
			{"name":"garlic","quantity":"3","unit":"cloves"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 2 {
		t.Fatalf("expected 2 items, got %d", len(seeds))
	}
}

func TestRecipeAggregator_CategoriesAssigned(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"chicken breast","quantity":"2","unit":"lbs"},
			{"name":"garlic","quantity":"3","unit":"cloves"},
			{"name":"olive oil","quantity":"2","unit":"tbsp"},
			{"name":"salt","quantity":"1","unit":"tsp"},
			{"name":"flour","quantity":"1","unit":"cup"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	categories := make(map[string]bool)
	for _, s := range seeds {
		categories[s.Category] = true
	}

	for _, expected := range []string{"proteins", "produce", "pantry", "spices", "baking"} {
		if !categories[expected] {
			t.Errorf("expected category %s, not found", expected)
		}
	}
}

func TestRecipeAggregator_SkipsEmptyIngredients(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"","quantity":"2","unit":"cups"},
			{"name":"flour","quantity":"1","unit":"cup"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 item (empty name skipped), got %d", len(seeds))
	}
}

func TestRecipeAggregator_HandlesInvalidJSON(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`not json`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"flour","quantity":"1","unit":"cup"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	// Should still get items from the valid source
	if len(seeds) != 1 {
		t.Fatalf("expected 1 item from valid source, got %d", len(seeds))
	}
}

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

func TestParseQuantity_Empty(t *testing.T) {
	qty, _ := ParseQuantity("", "pinch")
	if qty != 0 {
		t.Fatalf("expected 0 for empty quantity, got %f", qty)
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
		"hummus":          "hummus", // don't strip 's' from words ending in 'ss'
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
		{"garlic", "cloves", "", 5, "5 cloves garlic"},
		{"flour", "cup", "", 2, "2 cup flour"},
		{"salt", "tsp", "", 0.5, "0.5 tsp salt"},
		{"oil", "", "", 0, "oil"},
		{"chicken", "lbs", "diced", 2, "2 lbs chicken (diced)"},
	}
	for _, tc := range cases {
		got := FormatIngredient(tc.name, tc.qty, tc.unit, tc.prep)
		if got != tc.expected {
			t.Errorf("FormatIngredient(%q, %f, %q, %q) = %q, want %q",
				tc.name, tc.qty, tc.unit, tc.prep, got, tc.expected)
		}
	}
}

func TestRecipeAggregator_SameUnitsMerged(t *testing.T) {
	// Gherkin: "Normalize units before merging"
	// Two recipes with the same ingredient in the same canonical unit are merged.
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"milk","quantity":"1","unit":"cup"}]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"milk","quantity":"2","unit":"cups"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	// "cups" normalizes to "cup", so both entries share the merge key (milk, cup).
	if len(seeds) != 1 {
		t.Fatalf("expected 1 merged item for milk, got %d", len(seeds))
	}
	if seeds[0].Quantity == nil || *seeds[0].Quantity != 3 {
		t.Fatalf("expected merged quantity 3, got %v", seeds[0].Quantity)
	}
	if len(seeds[0].SourceArtifactIDs) != 2 {
		t.Fatalf("expected 2 source artifacts, got %d", len(seeds[0].SourceArtifactIDs))
	}
}

func TestRecipeAggregator_DifferentUnitsMergedByAlias(t *testing.T) {
	// When one recipe says "tablespoon" and another says "tbsp", they normalize to the same
	// canonical unit and MUST merge.
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"olive oil","quantity":"2","unit":"tablespoon"}]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"olive oil","quantity":"1","unit":"tbsp"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 merged item for olive oil (alias merge), got %d", len(seeds))
	}
	if seeds[0].Quantity == nil || *seeds[0].Quantity != 3 {
		t.Fatalf("expected merged quantity 3, got %v", seeds[0].Quantity)
	}
}

func TestRecipeAggregator_IncompatibleUnitsKeptSeparate(t *testing.T) {
	// Gherkin: "Keep incompatible units separate"
	// "2 cloves garlic" and "1 tbsp minced garlic" have different normalized units
	// (cloves vs tbsp) and must appear as separate items.
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"2","unit":"cloves"}]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"1","unit":"tbsp"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 2 {
		t.Fatalf("expected 2 separate items for garlic (incompatible units), got %d", len(seeds))
	}

	// Verify each item exists with the correct unit
	units := map[string]bool{}
	for _, s := range seeds {
		units[s.Unit] = true
	}
	if !units["cloves"] {
		t.Error("expected a garlic item with unit 'cloves'")
	}
	if !units["tbsp"] {
		t.Error("expected a garlic item with unit 'tbsp'")
	}
}

func TestRecipeAggregator_ThreeRecipeMerge(t *testing.T) {
	// End-to-end test: 3 recipes with overlapping ingredients.
	// Verifies merge, separate items, and multi-source traceability.
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"garlic","quantity":"2","unit":"cloves"},
			{"name":"olive oil","quantity":"2","unit":"tbsp"},
			{"name":"chicken","quantity":"1","unit":"lb"}
		]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"garlic","quantity":"3","unit":"cloves"},
			{"name":"olive oil","quantity":"1","unit":"tbsp"},
			{"name":"rice","quantity":"2","unit":"cup"}
		]}`)},
		{ArtifactID: "a3", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"garlic","quantity":"1","unit":"cloves"},
			{"name":"salt","quantity":"1","unit":"tsp"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	// Expected: garlic 6 cloves (merged from 3), olive oil 3 tbsp (merged from 2),
	// chicken 1 lb, rice 2 cup, salt 1 tsp = 5 distinct items
	if len(seeds) != 5 {
		t.Fatalf("expected 5 items, got %d", len(seeds))
	}

	// Find garlic and verify it merged from all 3 recipes
	for _, s := range seeds {
		if s.NormalizedName == "garlic" && s.Unit == "cloves" {
			if s.Quantity == nil || *s.Quantity != 6 {
				t.Errorf("expected garlic quantity 6, got %v", s.Quantity)
			}
			if len(s.SourceArtifactIDs) != 3 {
				t.Errorf("expected garlic from 3 sources, got %d", len(s.SourceArtifactIDs))
			}
			return
		}
	}
	t.Error("garlic item not found in merged results")
}
