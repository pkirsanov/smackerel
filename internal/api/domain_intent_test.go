package api

import (
	"testing"
)

func TestParseDomainIntent_RecipeWithIngredient(t *testing.T) {
	intent := parseDomainIntent("recipes with chicken")
	if intent == nil {
		t.Fatal("expected recipe intent")
	}
	if intent.Domain != "recipe" {
		t.Errorf("expected domain recipe, got %s", intent.Domain)
	}
	if len(intent.Attributes) != 1 || intent.Attributes[0] != "chicken" {
		t.Errorf("expected [chicken], got %v", intent.Attributes)
	}
}

func TestParseDomainIntent_RecipeMultipleIngredients(t *testing.T) {
	intent := parseDomainIntent("recipes with chicken and garlic")
	if intent == nil {
		t.Fatal("expected recipe intent")
	}
	if len(intent.Attributes) < 1 {
		t.Errorf("expected at least 1 ingredient, got %v", intent.Attributes)
	}
}

func TestParseDomainIntent_RecipeNoIngredient(t *testing.T) {
	intent := parseDomainIntent("Italian recipes")
	if intent == nil {
		t.Fatal("expected recipe intent")
	}
	if intent.Domain != "recipe" {
		t.Errorf("expected domain recipe, got %s", intent.Domain)
	}
}

func TestParseDomainIntent_ProductUnderPrice(t *testing.T) {
	intent := parseDomainIntent("cameras under $500")
	if intent == nil {
		t.Fatal("expected product intent")
	}
	if intent.Domain != "product" {
		t.Errorf("expected domain product, got %s", intent.Domain)
	}
	if intent.PriceMax != 500 {
		t.Errorf("expected PriceMax 500, got %f", intent.PriceMax)
	}
}

func TestParseDomainIntent_ProductNoPrice(t *testing.T) {
	intent := parseDomainIntent("best headphones")
	if intent == nil {
		t.Fatal("expected product intent")
	}
	if intent.Domain != "product" {
		t.Errorf("expected domain product, got %s", intent.Domain)
	}
	if intent.PriceMax != 0 {
		t.Errorf("expected PriceMax 0, got %f", intent.PriceMax)
	}
}

func TestParseDomainIntent_NoIntent(t *testing.T) {
	intent := parseDomainIntent("that article about leadership")
	if intent != nil {
		t.Errorf("expected nil for non-domain query, got domain=%s", intent.Domain)
	}
}

func TestParseDomainIntent_EmptyQuery(t *testing.T) {
	intent := parseDomainIntent("")
	if intent != nil {
		t.Error("expected nil for empty query")
	}
}

func TestParseDomainIntent_IngredientListFormat(t *testing.T) {
	intent := parseDomainIntent("recipe ingredients: chicken, garlic, lemon")
	if intent == nil {
		t.Fatal("expected recipe intent")
	}
	if len(intent.Attributes) != 3 {
		t.Errorf("expected 3 ingredients, got %d: %v", len(intent.Attributes), intent.Attributes)
	}
}

func TestParseDomainIntent_CaseInsensitive(t *testing.T) {
	intent := parseDomainIntent("RECIPES WITH MUSHROOMS")
	if intent == nil {
		t.Fatal("expected recipe intent for uppercase")
	}
	if intent.Domain != "recipe" {
		t.Errorf("expected domain recipe, got %s", intent.Domain)
	}
}

func TestParseDomainIntent_DishKeyword(t *testing.T) {
	intent := parseDomainIntent("easy dishes for dinner")
	if intent == nil {
		t.Fatal("expected recipe intent for 'dishes'")
	}
	if intent.Domain != "recipe" {
		t.Errorf("expected domain recipe, got %s", intent.Domain)
	}
}
