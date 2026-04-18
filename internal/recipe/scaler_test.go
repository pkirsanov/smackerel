package recipe

import (
	"testing"
)

func TestScaleIngredients_SimpleDouble(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "guanciale", Quantity: "200", Unit: "g"},
	}
	result := ScaleIngredients(ingredients, 4, 8)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].DisplayQuantity != "400" {
		t.Errorf("expected display quantity '400', got %q", result[0].DisplayQuantity)
	}
	if !result[0].Scaled {
		t.Error("expected Scaled=true")
	}
	if result[0].Unit != "g" {
		t.Errorf("expected unit 'g', got %q", result[0].Unit)
	}
}

func TestScaleIngredients_FractionalScaling(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "olive oil", Quantity: "1/3", Unit: "cup"},
	}
	result := ScaleIngredients(ingredients, 4, 2)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].Scaled {
		t.Error("expected Scaled=true")
	}
	// 1/3 * 0.5 = 1/6 ≈ 0.167
	if result[0].DisplayQuantity != "1/6" {
		t.Errorf("expected display quantity '1/6', got %q", result[0].DisplayQuantity)
	}
}

func TestScaleIngredients_ScaleDownToOne(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "flour", Quantity: "3", Unit: "cups"},
	}
	result := ScaleIngredients(ingredients, 6, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// 3 / 6 = 0.5
	if result[0].DisplayQuantity != "1/2" {
		t.Errorf("expected display quantity '1/2', got %q", result[0].DisplayQuantity)
	}
}

func TestScaleIngredients_UnparseablePreserved(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "salt", Quantity: "to taste", Unit: ""},
	}
	result := ScaleIngredients(ingredients, 4, 8)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Scaled {
		t.Error("expected Scaled=false for unparseable quantity")
	}
	if result[0].DisplayQuantity != "to taste" {
		t.Errorf("expected display quantity 'to taste', got %q", result[0].DisplayQuantity)
	}
}

func TestScaleIngredients_ZeroServingsReturnsNil(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "flour", Quantity: "1", Unit: "cup"},
	}
	if result := ScaleIngredients(ingredients, 0, 4); result != nil {
		t.Error("expected nil for originalServings=0")
	}
	if result := ScaleIngredients(ingredients, 4, 0); result != nil {
		t.Error("expected nil for targetServings=0")
	}
	if result := ScaleIngredients(ingredients, -1, 4); result != nil {
		t.Error("expected nil for negative originalServings")
	}
}

func TestScaleIngredients_IntegerStaysInteger(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "eggs", Quantity: "2", Unit: ""},
	}
	result := ScaleIngredients(ingredients, 4, 6)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// 2 * (6/4) = 3.0 → should display as "3"
	if result[0].DisplayQuantity != "3" {
		t.Errorf("expected display quantity '3', got %q", result[0].DisplayQuantity)
	}
}

func TestScaleIngredients_LargeScaleFactor(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "vanilla extract", Quantity: "1", Unit: "tsp"},
	}
	result := ScaleIngredients(ingredients, 2, 100)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// 1 * (100/2) = 50
	if result[0].DisplayQuantity != "50" {
		t.Errorf("expected display quantity '50', got %q", result[0].DisplayQuantity)
	}
	if result[0].Unit != "tsp" {
		t.Errorf("expected unit 'tsp', got %q", result[0].Unit)
	}
}

func TestScaleIngredients_MixedUnitsScaleIndependently(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "broth", Quantity: "2", Unit: "cups"},
		{Name: "soy sauce", Quantity: "4", Unit: "tbsp"},
	}
	result := ScaleIngredients(ingredients, 4, 8)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].DisplayQuantity != "4" {
		t.Errorf("broth: expected '4', got %q", result[0].DisplayQuantity)
	}
	if result[1].DisplayQuantity != "8" {
		t.Errorf("soy sauce: expected '8', got %q", result[1].DisplayQuantity)
	}
}

func TestScaleIngredients_EmptyQuantityUnscaled(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "water", Quantity: "", Unit: ""},
	}
	result := ScaleIngredients(ingredients, 4, 8)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Scaled {
		t.Error("expected Scaled=false for empty quantity")
	}
}

func TestScaleIngredients_RangeNotationUnscaled(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "garlic", Quantity: "2-3", Unit: "cloves"},
	}
	result := ScaleIngredients(ingredients, 4, 8)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Scaled {
		t.Error("expected Scaled=false for range notation")
	}
	if result[0].DisplayQuantity != "2-3" {
		t.Errorf("expected '2-3', got %q", result[0].DisplayQuantity)
	}
}

func TestScaleIngredients_EmptyList(t *testing.T) {
	result := ScaleIngredients([]Ingredient{}, 4, 8)
	if result == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestScaleIngredients_NegativeTargetServings(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "flour", Quantity: "1", Unit: "cup"},
	}
	if result := ScaleIngredients(ingredients, 4, -2); result != nil {
		t.Error("expected nil for negative targetServings")
	}
}

func TestScaleIngredients_SameServings(t *testing.T) {
	ingredients := []Ingredient{
		{Name: "flour", Quantity: "2", Unit: "cups"},
	}
	result := ScaleIngredients(ingredients, 4, 4)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].DisplayQuantity != "2" {
		t.Errorf("expected '2' (unchanged), got %q", result[0].DisplayQuantity)
	}
}
