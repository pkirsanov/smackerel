package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Verify marker constants are distinct two-char strings: symbol + space.
// SCN-001-004: Full set of 8 markers (. ? ! > - ~ # @).
func TestMarkerConstants_Unique(t *testing.T) {
	markers := []string{
		MarkerSuccess,
		MarkerUncertain,
		MarkerAction,
		MarkerInfo,
		MarkerListItem,
		MarkerContinued,
		MarkerHeading,
		MarkerMention,
	}
	if len(markers) != 8 {
		t.Errorf("expected 8 markers per spec, got %d", len(markers))
	}
	seen := make(map[string]bool)
	for _, m := range markers {
		if len(m) != 2 || m[1] != ' ' {
			t.Errorf("marker %q should be a single char + space", m)
		}
		if seen[m] {
			t.Errorf("duplicate marker: %q", m)
		}
		seen[m] = true
	}
}

// SCN-001-004 / SCN-002-025: Bot uses text markers, no emoji.
// Markers themselves must be plain ASCII.
func TestMarkerConstants_NoEmoji(t *testing.T) {
	markers := []string{
		MarkerSuccess,
		MarkerUncertain,
		MarkerAction,
		MarkerInfo,
		MarkerListItem,
		MarkerContinued,
		MarkerHeading,
		MarkerMention,
	}
	for _, m := range markers {
		for _, r := range m {
			if r > 127 {
				t.Errorf("marker %q contains non-ASCII rune %U", m, r)
			}
		}
	}
}

// SCN-002-042: Unsupported attachment response uses ? marker.
func TestSCN002042_UnsupportedAttachmentMarker(t *testing.T) {
	response := MarkerUncertain + "Not sure what to do with this. Can you add context?"
	if response[:2] != "? " {
		t.Errorf("unsupported attachment should start with '? ', got %q", response[:2])
	}
}

// --- Domain card tests (SCN-026-09) ---

// T9-01: formatRecipeCard renders timing, servings, cuisine, dietary_tags.
func TestFormatRecipeCard_BasicFields(t *testing.T) {
	data := json.RawMessage(`{
		"domain": "recipe",
		"timing": {"prep": "15 min", "cook": "30 min", "total": "45 min"},
		"servings": 4,
		"cuisine": "Italian",
		"difficulty": "Medium",
		"dietary_tags": ["vegetarian", "gluten-free"],
		"ingredients": [
			{"name": "Tomato", "quantity": "2", "unit": "cups"}
		]
	}`)

	result := formatRecipeCard(data)

	checks := []string{
		"# Recipe Details",
		"Prep: 15 min",
		"Cook: 30 min",
		"Total: 45 min",
		"Servings: 4",
		"Cuisine: Italian",
		"Difficulty: Medium",
		"Tags: vegetarian, gluten-free",
	}
	for _, c := range checks {
		if !strings.Contains(result, c) {
			t.Errorf("recipe card missing %q\ngot:\n%s", c, result)
		}
	}
}

// T9-02: formatRecipeCard renders up to 10 ingredients, truncates remainder.
func TestFormatRecipeCard_IngredientTruncation(t *testing.T) {
	ingredients := make([]map[string]string, 13)
	for i := range ingredients {
		ingredients[i] = map[string]string{
			"name":     fmt.Sprintf("Ingredient %d", i+1),
			"quantity": "1",
			"unit":     "cup",
		}
	}
	raw, err := json.Marshal(map[string]interface{}{
		"domain":      "recipe",
		"timing":      map[string]string{"total": "1 hr"},
		"servings":    2,
		"cuisine":     "Test",
		"difficulty":  "Easy",
		"ingredients": ingredients,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := formatRecipeCard(json.RawMessage(raw))

	// Should show exactly 10 ingredients
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("Ingredient %d", i)
		if !strings.Contains(result, name) {
			t.Errorf("recipe card missing ingredient %q", name)
		}
	}
	// Ingredient 11, 12, 13 should NOT appear
	if strings.Contains(result, "Ingredient 11") {
		t.Error("recipe card should not show Ingredient 11")
	}
	// Should have truncation message
	if !strings.Contains(result, "... and 3 more") {
		t.Errorf("recipe card missing truncation message\ngot:\n%s", result)
	}
}

// T9-03: formatProductCard renders brand, price, rating, pros, cons.
func TestFormatProductCard_BasicFields(t *testing.T) {
	data := json.RawMessage(`{
		"domain": "product",
		"brand": "TestBrand",
		"price": {"amount": 29.99, "currency": "USD"},
		"rating": {"score": 4.5, "max": 5},
		"pros": ["Durable", "Lightweight"],
		"cons": ["Expensive"]
	}`)

	result := formatProductCard(data)

	checks := []string{
		"# Product Details",
		"Brand: TestBrand",
		"Price: 29.99 USD",
		"Rating: 4.5/5",
		"# Pros",
		"- Durable",
		"- Lightweight",
		"# Cons",
		"- Expensive",
	}
	for _, c := range checks {
		if !strings.Contains(result, c) {
			t.Errorf("product card missing %q\ngot:\n%s", c, result)
		}
	}
}

// T9-04: formatProductCard limits pros to 5 and cons to 3.
func TestFormatProductCard_ProConsTruncation(t *testing.T) {
	data := json.RawMessage(`{
		"domain": "product",
		"brand": "TruncBrand",
		"price": {"amount": 10.00, "currency": "EUR"},
		"rating": {"score": 3.0, "max": 5},
		"pros": ["Pro1", "Pro2", "Pro3", "Pro4", "Pro5", "Pro6", "Pro7"],
		"cons": ["Con1", "Con2", "Con3", "Con4", "Con5"]
	}`)

	result := formatProductCard(data)

	// Should show exactly 5 pros
	for i := 1; i <= 5; i++ {
		if !strings.Contains(result, fmt.Sprintf("Pro%d", i)) {
			t.Errorf("product card missing Pro%d", i)
		}
	}
	if strings.Contains(result, "Pro6") {
		t.Error("product card should not show Pro6")
	}
	if !strings.Contains(result, "... and 2 more") {
		t.Errorf("product card missing pros truncation message\ngot:\n%s", result)
	}

	// Should show exactly 3 cons
	for i := 1; i <= 3; i++ {
		if !strings.Contains(result, fmt.Sprintf("Con%d", i)) {
			t.Errorf("product card missing Con%d", i)
		}
	}
	if strings.Contains(result, "Con4") {
		t.Error("product card should not show Con4")
	}
	if !strings.Contains(result, "... and 2 more") {
		t.Errorf("product card missing cons truncation message\ngot:\n%s", result)
	}
}

// T9-05: formatDomainCard returns empty string for nil/empty domain_data.
func TestFormatDomainCard_NilEmpty(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
	}{
		{"nil", nil},
		{"empty", json.RawMessage{}},
		{"empty_bytes", json.RawMessage([]byte{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDomainCard(tt.data)
			if result != "" {
				t.Errorf("expected empty string for %s domain_data, got %q", tt.name, result)
			}
		})
	}
}

// T9-06: formatDomainCard returns empty string for unknown domain type.
func TestFormatDomainCard_UnknownDomain(t *testing.T) {
	data := json.RawMessage(`{"domain": "travel", "destination": "Paris"}`)
	result := formatDomainCard(data)
	if result != "" {
		t.Errorf("expected empty string for unknown domain, got %q", result)
	}
}

// T9-07: formatDomainCard dispatches to recipe renderer for domain="recipe".
func TestFormatDomainCard_DispatchRecipe(t *testing.T) {
	data := json.RawMessage(`{
		"domain": "recipe",
		"timing": {"total": "30 min"},
		"servings": 2,
		"cuisine": "Mexican",
		"difficulty": "Easy",
		"dietary_tags": [],
		"ingredients": [{"name": "Beans", "quantity": "1", "unit": "can"}]
	}`)

	result := formatDomainCard(data)
	if !strings.Contains(result, "# Recipe Details") {
		t.Errorf("formatDomainCard did not dispatch to recipe renderer\ngot:\n%s", result)
	}
	if !strings.Contains(result, "Cuisine: Mexican") {
		t.Errorf("recipe dispatch missing cuisine\ngot:\n%s", result)
	}
}

// T9-08: formatDomainCard dispatches to product renderer for domain="product".
func TestFormatDomainCard_DispatchProduct(t *testing.T) {
	data := json.RawMessage(`{
		"domain": "product",
		"brand": "Acme",
		"price": {"amount": 9.99, "currency": "GBP"},
		"rating": {"score": 4.0, "max": 5},
		"pros": ["Cheap"],
		"cons": ["Fragile"]
	}`)

	result := formatDomainCard(data)
	if !strings.Contains(result, "# Product Details") {
		t.Errorf("formatDomainCard did not dispatch to product renderer\ngot:\n%s", result)
	}
	if !strings.Contains(result, "Brand: Acme") {
		t.Errorf("product dispatch missing brand\ngot:\n%s", result)
	}
}
