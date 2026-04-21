package recipe

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Unicode fraction normalization map.
var unicodeFractions = map[string]string{
	"½": "1/2",
	"⅓": "1/3",
	"⅔": "2/3",
	"¼": "1/4",
	"¾": "3/4",
	"⅛": "1/8",
	"⅜": "3/8",
	"⅝": "5/8",
	"⅞": "7/8",
	"⅙": "1/6",
	"⅚": "5/6",
}

// Compiled regexes for quantity parsing.
var (
	fractionRe     = regexp.MustCompile(`^(\d+)\s+(\d+)/(\d+)$`)
	simpleRe       = regexp.MustCompile(`^(\d+(?:\.\d+)?)$`)
	fractionOnlyRe = regexp.MustCompile(`^(\d+)/(\d+)$`)
)

// ParseQuantity parses a quantity string and returns a float and unit.
// Unicode fraction characters are normalized before parsing.
// Returns 0 for unparseable quantities (empty, "to taste", "a pinch", etc.).
func ParseQuantity(qtyStr, unitStr string) (float64, string) {
	qtyStr = strings.TrimSpace(qtyStr)
	unitStr = strings.TrimSpace(unitStr)

	if qtyStr == "" {
		return 0, unitStr
	}

	// Normalize Unicode fractions to ASCII equivalents.
	// Prepend a space so "1½" becomes "1 1/2" (not "11/2").
	for unicode, ascii := range unicodeFractions {
		qtyStr = strings.ReplaceAll(qtyStr, unicode, " "+ascii)
	}
	qtyStr = strings.TrimSpace(qtyStr)

	// Mixed fraction: "2 1/2"
	if m := fractionRe.FindStringSubmatch(qtyStr); len(m) == 4 {
		whole, _ := strconv.ParseFloat(m[1], 64)
		num, _ := strconv.ParseFloat(m[2], 64)
		den, _ := strconv.ParseFloat(m[3], 64)
		if den > 0 {
			result := whole + num/den
			if math.IsInf(result, 0) || math.IsNaN(result) {
				return 0, unitStr
			}
			return result, unitStr
		}
	}

	// Simple fraction: "1/2"
	if m := fractionOnlyRe.FindStringSubmatch(qtyStr); len(m) == 3 {
		num, _ := strconv.ParseFloat(m[1], 64)
		den, _ := strconv.ParseFloat(m[2], 64)
		if den > 0 {
			result := num / den
			if math.IsInf(result, 0) || math.IsNaN(result) {
				return 0, unitStr
			}
			return result, unitStr
		}
	}

	// Simple number: "2" or "2.5"
	if m := simpleRe.FindStringSubmatch(qtyStr); len(m) == 2 {
		v, _ := strconv.ParseFloat(m[1], 64)
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return 0, unitStr
		}
		return v, unitStr
	}

	return 0, unitStr
}

// unitAliases maps unit aliases to their canonical forms.
var unitAliases = map[string]string{
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

// NormalizeUnit converts unit aliases to canonical form.
func NormalizeUnit(unit string) string {
	unit = strings.ToLower(strings.TrimSpace(unit))
	if canonical, ok := unitAliases[unit]; ok {
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

// ingredientCategories maps category names to their keyword lists.
var ingredientCategories = []struct {
	category string
	keywords []string
}{
	{"proteins", []string{"chicken", "beef", "pork", "lamb", "fish", "salmon", "tuna", "shrimp", "tofu", "turkey", "bacon", "sausage", "egg"}},
	{"dairy", []string{"milk", "cream", "butter", "cheese", "yogurt", "sour cream"}},
	{"produce", []string{"onion", "garlic", "tomato", "pepper", "lettuce", "carrot", "celery", "potato", "mushroom", "lemon", "lime", "avocado", "spinach", "broccoli", "ginger", "cilantro", "parsley", "basil", "thyme", "rosemary", "scallion", "zucchini", "cucumber", "corn", "bean", "pea"}},
	{"spices", []string{"salt", "pepper", "cumin", "paprika", "oregano", "cinnamon", "nutmeg", "turmeric", "chili", "cayenne", "bay leaf", "clove", "coriander"}},
	{"baking", []string{"flour", "sugar", "baking soda", "baking powder", "yeast", "cocoa", "vanilla", "cornstarch"}},
	{"pantry", []string{"oil", "olive oil", "vinegar", "soy sauce", "honey", "maple syrup", "rice", "pasta", "noodle", "bread", "broth", "stock", "ketchup", "mustard", "mayonnaise", "hot sauce"}},
	{"beverages", []string{"water", "wine", "beer", "juice", "coffee", "tea"}},
}

// CategorizeIngredient maps an ingredient name to a grocery category.
func CategorizeIngredient(name string) string {
	name = strings.ToLower(name)

	for _, cat := range ingredientCategories {
		for _, kw := range cat.keywords {
			if strings.Contains(name, kw) {
				return cat.category
			}
		}
	}

	return "other"
}

// FormatIngredient formats a merged ingredient for display.
func FormatIngredient(name string, qty float64, unit, preparation string) string {
	var parts []string

	if qty > 0 {
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
