package list

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// RecipeAggregator merges recipe ingredients across multiple artifacts.
type RecipeAggregator struct{}

type recipeData struct {
	Ingredients []recipeIngredient `json:"ingredients"`
}

type recipeIngredient struct {
	Name        string `json:"name"`
	Quantity    string `json:"quantity"`
	Unit        string `json:"unit"`
	Preparation string `json:"preparation"`
	Group       string `json:"group"`
}

func (a *RecipeAggregator) Domain() string            { return "recipe" }
func (a *RecipeAggregator) DefaultListType() ListType { return TypeShopping }

func (a *RecipeAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error) {
	type mergeKey struct {
		name string
		unit string
	}

	type mergedItem struct {
		name        string
		quantity    float64
		hasQty      bool
		unit        string
		category    string
		sources     map[string]bool
		preparation string
	}

	merged := make(map[mergeKey]*mergedItem)
	var order []mergeKey

	for _, src := range sources {
		var rd recipeData
		if err := json.Unmarshal(src.DomainData, &rd); err != nil {
			continue
		}

		for _, ing := range rd.Ingredients {
			if ing.Name == "" {
				continue
			}
			normName := NormalizeIngredientName(ing.Name)
			qty, unit := ParseQuantity(ing.Quantity, ing.Unit)
			normUnit := NormalizeUnit(unit)

			key := mergeKey{name: normName, unit: normUnit}

			if existing, ok := merged[key]; ok {
				if qty > 0 && existing.hasQty {
					existing.quantity += qty
				} else if qty > 0 {
					existing.quantity = qty
					existing.hasQty = true
				}
				existing.sources[src.ArtifactID] = true
			} else {
				m := &mergedItem{
					name:     normName,
					quantity: qty,
					hasQty:   qty > 0,
					unit:     normUnit,
					category: CategorizeIngredient(normName),
					sources:  map[string]bool{src.ArtifactID: true},
				}
				if ing.Preparation != "" {
					m.preparation = ing.Preparation
				}
				merged[key] = m
				order = append(order, key)
			}
		}
	}

	// Sort by category then name
	categoryOrder := map[string]int{
		"produce": 0, "proteins": 1, "dairy": 2, "pantry": 3,
		"spices": 4, "baking": 5, "frozen": 6, "beverages": 7, "other": 8,
	}

	type sortItem struct {
		key  mergeKey
		item *mergedItem
	}
	var sorted []sortItem
	for _, k := range order {
		sorted = append(sorted, sortItem{key: k, item: merged[k]})
	}

	// Stable sort by category then name
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			ci := categoryOrder[sorted[i].item.category]
			cj := categoryOrder[sorted[j].item.category]
			if ci > cj || (ci == cj && sorted[i].item.name > sorted[j].item.name) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var seeds []ListItemSeed
	for i, s := range sorted {
		content := FormatIngredient(s.item.name, s.item.quantity, s.item.unit, s.item.preparation)

		var sources []string
		for src := range s.item.sources {
			sources = append(sources, src)
		}

		var qtyPtr *float64
		if s.item.hasQty {
			q := s.item.quantity
			qtyPtr = &q
		}

		seeds = append(seeds, ListItemSeed{
			Content:           content,
			Category:          s.item.category,
			Quantity:          qtyPtr,
			Unit:              s.item.unit,
			NormalizedName:    s.item.name,
			SourceArtifactIDs: sources,
			SortOrder:         i,
		})
	}

	return seeds, nil
}

// ParseQuantity parses a quantity string and returns a float and unit.
var fractionRe = regexp.MustCompile(`^(\d+)\s+(\d+)/(\d+)$`)
var simpleRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)$`)
var fractionOnlyRe = regexp.MustCompile(`^(\d+)/(\d+)$`)

func ParseQuantity(qtyStr, unitStr string) (float64, string) {
	qtyStr = strings.TrimSpace(qtyStr)
	unitStr = strings.TrimSpace(unitStr)

	if qtyStr == "" {
		return 0, unitStr
	}

	// Mixed fraction: "2 1/2"
	if m := fractionRe.FindStringSubmatch(qtyStr); len(m) == 4 {
		whole, _ := strconv.ParseFloat(m[1], 64)
		num, _ := strconv.ParseFloat(m[2], 64)
		den, _ := strconv.ParseFloat(m[3], 64)
		if den > 0 {
			return whole + num/den, unitStr
		}
	}

	// Simple fraction: "1/2"
	if m := fractionOnlyRe.FindStringSubmatch(qtyStr); len(m) == 3 {
		num, _ := strconv.ParseFloat(m[1], 64)
		den, _ := strconv.ParseFloat(m[2], 64)
		if den > 0 {
			return num / den, unitStr
		}
	}

	// Simple number: "2" or "2.5"
	if m := simpleRe.FindStringSubmatch(qtyStr); len(m) == 2 {
		v, _ := strconv.ParseFloat(m[1], 64)
		return v, unitStr
	}

	return 0, unitStr
}

// NormalizeUnit converts unit aliases to canonical form.
func NormalizeUnit(unit string) string {
	unit = strings.ToLower(strings.TrimSpace(unit))
	aliases := map[string]string{
		"tablespoon": "tbsp", "tablespoons": "tbsp", "tbs": "tbsp",
		"teaspoon": "tsp", "teaspoons": "tsp",
		"cups":  "cup",
		"ounce": "oz", "ounces": "oz",
		"pound": "lb", "pounds": "lb", "lbs": "lb",
		"gram": "g", "grams": "g",
		"kilogram": "kg", "kilograms": "kg",
		"milliliter": "ml", "milliliters": "ml",
		"liter": "l", "liters": "l",
		"clove": "cloves",
		"piece": "pieces", "pc": "pieces",
		"slice": "slices",
		"can":   "cans",
		"bunch": "bunches",
	}
	if canonical, ok := aliases[unit]; ok {
		return canonical
	}
	return unit
}

// NormalizeIngredientName normalizes an ingredient name for dedup.
func NormalizeIngredientName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Handle "es" plurals (tomatoes → tomato) before "s" plurals
	if len(name) > 4 && strings.HasSuffix(name, "oes") {
		name = name[:len(name)-2] // tomatoes → tomato
	} else if len(name) > 3 && strings.HasSuffix(name, "s") && !strings.HasSuffix(name, "ss") && !strings.HasSuffix(name, "us") {
		name = name[:len(name)-1]
	}
	return name
}

// CategorizeIngredient maps an ingredient name to a grocery category.
func CategorizeIngredient(name string) string {
	name = strings.ToLower(name)

	proteins := []string{"chicken", "beef", "pork", "lamb", "fish", "salmon", "tuna", "shrimp", "tofu", "turkey", "bacon", "sausage", "egg"}
	dairy := []string{"milk", "cream", "butter", "cheese", "yogurt", "sour cream"}
	produce := []string{"onion", "garlic", "tomato", "pepper", "lettuce", "carrot", "celery", "potato", "mushroom", "lemon", "lime", "avocado", "spinach", "broccoli", "ginger", "cilantro", "parsley", "basil", "thyme", "rosemary", "scallion", "zucchini", "cucumber", "corn", "bean", "pea"}
	spices := []string{"salt", "pepper", "cumin", "paprika", "oregano", "cinnamon", "nutmeg", "turmeric", "chili", "cayenne", "bay leaf", "clove", "coriander"}
	baking := []string{"flour", "sugar", "baking soda", "baking powder", "yeast", "cocoa", "vanilla", "cornstarch"}
	pantry := []string{"oil", "olive oil", "vinegar", "soy sauce", "honey", "maple syrup", "rice", "pasta", "noodle", "bread", "broth", "stock", "ketchup", "mustard", "mayonnaise", "hot sauce"}
	beverages := []string{"water", "wine", "beer", "juice", "coffee", "tea"}

	for _, p := range proteins {
		if strings.Contains(name, p) {
			return "proteins"
		}
	}
	for _, d := range dairy {
		if strings.Contains(name, d) {
			return "dairy"
		}
	}
	for _, p := range produce {
		if strings.Contains(name, p) {
			return "produce"
		}
	}
	for _, s := range spices {
		if strings.Contains(name, s) {
			return "spices"
		}
	}
	for _, b := range baking {
		if strings.Contains(name, b) {
			return "baking"
		}
	}
	for _, p := range pantry {
		if strings.Contains(name, p) {
			return "pantry"
		}
	}
	for _, b := range beverages {
		if strings.Contains(name, b) {
			return "beverages"
		}
	}

	return "other"
}

// FormatIngredient formats a merged ingredient for display.
func FormatIngredient(name string, qty float64, unit, preparation string) string {
	var parts []string

	if qty > 0 {
		// Format quantity nicely (no trailing zeros)
		if qty == math.Floor(qty) {
			parts = append(parts, fmt.Sprintf("%d", int(qty)))
		} else {
			parts = append(parts, fmt.Sprintf("%.1f", qty))
		}
	}

	if unit != "" {
		parts = append(parts, unit)
	}

	parts = append(parts, name)

	if preparation != "" {
		parts = append(parts, fmt.Sprintf("(%s)", preparation))
	}

	return strings.Join(parts, " ")
}
