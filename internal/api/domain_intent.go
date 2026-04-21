package api

import (
	"fmt"
	"regexp"
	"strings"
)

// DomainIntent represents detected domain-specific search intent.
type DomainIntent struct {
	Domain     string // "recipe", "product", etc.
	Attributes []string
	PriceMax   float64
	Cleaned    string // query with domain markers removed
}

var (
	recipeIntentRe     = regexp.MustCompile(`(?i)\b(recipes?|dishes?|meals?|cooking)\b`)
	ingredientIntentRe = regexp.MustCompile(`(?i)\bwith\s+([\w\s,]+?)(?:\s+(?:for|recipe|dish)|$)`)
	productIntentRe    = regexp.MustCompile(`(?i)\b(products?|cameras?|headphones?|laptops?|phones?|gadgets?)\b`)
	priceIntentRe      = regexp.MustCompile(`(?i)\bunder\s+\$?(\d+(?:\.\d{2})?)\b`)
	ingredientListRe   = regexp.MustCompile(`(?i)\bingredients?\s*:\s*([\w\s,]+)`)
)

// parseDomainIntent detects domain-specific search intent from a query.
// Returns nil if no domain intent is detected.
func parseDomainIntent(query string) *DomainIntent {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}

	// Check for recipe intent
	if recipeIntentRe.MatchString(q) {
		intent := &DomainIntent{
			Domain:  "recipe",
			Cleaned: recipeIntentRe.ReplaceAllString(q, ""),
		}

		// Extract ingredients from "with chicken and garlic" patterns
		if m := ingredientIntentRe.FindStringSubmatch(q); len(m) >= 2 {
			raw := m[1]
			// Split on both "," and " and " to handle multi-ingredient queries
			parts := strings.Split(raw, ",")
			var expanded []string
			for _, p := range parts {
				for _, sub := range strings.Split(p, " and ") {
					sub = strings.TrimSpace(sub)
					if sub != "" {
						expanded = append(expanded, strings.ToLower(sub))
					}
				}
			}
			intent.Attributes = append(intent.Attributes, expanded...)
		}

		// Extract ingredients from "ingredients: chicken, garlic" patterns
		if m := ingredientListRe.FindStringSubmatch(q); len(m) >= 2 {
			for _, ing := range strings.Split(m[1], ",") {
				ing = strings.TrimSpace(ing)
				if ing != "" {
					intent.Attributes = append(intent.Attributes, strings.ToLower(ing))
				}
			}
		}

		intent.Cleaned = strings.TrimSpace(intent.Cleaned)
		return intent
	}

	// Check for product intent
	if productIntentRe.MatchString(q) {
		intent := &DomainIntent{
			Domain:  "product",
			Cleaned: productIntentRe.ReplaceAllString(q, ""),
		}

		// Extract price ceiling from "under $500" patterns
		if m := priceIntentRe.FindStringSubmatch(q); len(m) >= 2 {
			var price float64
			fmt.Sscanf(m[1], "%f", &price)
			if price > 0 {
				intent.PriceMax = price
			}
		}

		intent.Cleaned = strings.TrimSpace(intent.Cleaned)
		return intent
	}

	return nil
}
