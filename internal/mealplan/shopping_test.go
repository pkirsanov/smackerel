package mealplan

import (
	"encoding/json"
	"testing"
)

func TestScaleRecipeDomainData_ScalesCorrectly(t *testing.T) {
	servings := 4
	domainData, _ := json.Marshal(map[string]any{
		"domain":   "recipe",
		"title":    "Pasta",
		"servings": servings,
		"ingredients": []map[string]string{
			{"name": "spaghetti", "quantity": "400", "unit": "g"},
			{"name": "eggs", "quantity": "4", "unit": ""},
		},
		"steps": []any{},
	})

	result, err := scaleRecipeDomainData(domainData, 8)
	if err != nil {
		t.Fatalf("scale failed: %v", err)
	}

	var rd struct {
		Ingredients []struct {
			Quantity string `json:"quantity"`
		} `json:"ingredients"`
		Servings int `json:"servings"`
	}
	if err := json.Unmarshal(result, &rd); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if rd.Servings != 8 {
		t.Errorf("servings = %d, want 8", rd.Servings)
	}

	// Spaghetti: 400g × 2 = 800g
	if rd.Ingredients[0].Quantity != "800" {
		t.Errorf("spaghetti quantity = %q, want 800", rd.Ingredients[0].Quantity)
	}

	// Eggs: 4 × 2 = 8
	if rd.Ingredients[1].Quantity != "8" {
		t.Errorf("eggs quantity = %q, want 8", rd.Ingredients[1].Quantity)
	}
}

func TestScaleRecipeDomainData_InvalidJSON(t *testing.T) {
	_, err := scaleRecipeDomainData([]byte("not json"), 4)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestScaleRecipeDomainData_ZeroServings(t *testing.T) {
	domainData, _ := json.Marshal(map[string]any{
		"domain":      "recipe",
		"title":       "Test",
		"servings":    0,
		"ingredients": []any{},
		"steps":       []any{},
	})

	// zero/null servings → treat as 1, should still scale
	result, err := scaleRecipeDomainData(domainData, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
