package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Text markers used by the Telegram bot. No emoji allowed.
// Full set of 8 markers per spec SCN-001-004: . ? ! > - ~ # @
const (
	MarkerSuccess   = ". " // saved/confirmed
	MarkerUncertain = "? " // uncertainty/low confidence
	MarkerAction    = "! " // action needed
	MarkerInfo      = "> " // information/result
	MarkerListItem  = "- " // list item
	MarkerContinued = "~ " // continued/related
	MarkerHeading   = "# " // heading/topic
	MarkerMention   = "@ " // mention/entity reference
)

// maxRecipeIngredients is the maximum number of ingredients shown in a recipe card.
const maxRecipeIngredients = 10

// maxProductPros is the maximum number of pros shown in a product card.
const maxProductPros = 5

// maxProductCons is the maximum number of cons shown in a product card.
const maxProductCons = 3

// formatDomainCard dispatches to a domain-specific renderer based on the "domain" field
// in the provided JSON domain_data. Returns an empty string for nil/empty data or
// unrecognized domain types.
func formatDomainCard(domainData json.RawMessage) string {
	if len(domainData) == 0 {
		return ""
	}

	var envelope struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(domainData, &envelope); err != nil {
		return ""
	}

	switch envelope.Domain {
	case "recipe":
		return formatRecipeCard(domainData)
	case "product":
		return formatProductCard(domainData)
	default:
		return ""
	}
}

// recipeData represents the JSON structure for recipe domain data.
type recipeData struct {
	Domain      string             `json:"domain"`
	Title       string             `json:"title"`
	Timing      recipeTimingData   `json:"timing"`
	Servings    int                `json:"servings"`
	Cuisine     string             `json:"cuisine"`
	Difficulty  string             `json:"difficulty"`
	DietaryTags []string           `json:"dietary_tags"`
	Ingredients []recipeIngredient `json:"ingredients"`
}

type recipeTimingData struct {
	Prep  string `json:"prep"`
	Cook  string `json:"cook"`
	Total string `json:"total"`
}

type recipeIngredient struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Unit     string `json:"unit"`
}

// formatRecipeCard renders a recipe domain card for Telegram display.
func formatRecipeCard(data json.RawMessage) string {
	var recipe recipeData
	if err := json.Unmarshal(data, &recipe); err != nil {
		return ""
	}

	var lines []string
	lines = append(lines, "# Recipe Details")

	// Timing and servings
	var timingParts []string
	if recipe.Timing.Prep != "" {
		timingParts = append(timingParts, "Prep: "+recipe.Timing.Prep)
	}
	if recipe.Timing.Cook != "" {
		timingParts = append(timingParts, "Cook: "+recipe.Timing.Cook)
	}
	if recipe.Timing.Total != "" {
		timingParts = append(timingParts, "Total: "+recipe.Timing.Total)
	}
	if len(timingParts) > 0 {
		lines = append(lines, "> "+strings.Join(timingParts, " | "))
	}
	if recipe.Servings > 0 {
		lines = append(lines, fmt.Sprintf("> Servings: %d", recipe.Servings))
	}

	// Cuisine and difficulty
	if recipe.Cuisine != "" {
		lines = append(lines, "> Cuisine: "+recipe.Cuisine)
	}
	if recipe.Difficulty != "" {
		lines = append(lines, "> Difficulty: "+recipe.Difficulty)
	}

	// Dietary tags
	if len(recipe.DietaryTags) > 0 {
		lines = append(lines, "~ Tags: "+strings.Join(recipe.DietaryTags, ", "))
	}

	// Ingredients (up to maxRecipeIngredients)
	if len(recipe.Ingredients) > 0 {
		lines = append(lines, "")
		lines = append(lines, "# Ingredients")
		limit := len(recipe.Ingredients)
		if limit > maxRecipeIngredients {
			limit = maxRecipeIngredients
		}
		for _, ing := range recipe.Ingredients[:limit] {
			qty := ""
			if ing.Quantity != "" {
				qty = ing.Quantity
				if ing.Unit != "" {
					qty += " " + ing.Unit
				}
				qty += " "
			}
			lines = append(lines, fmt.Sprintf("- %s%s", qty, ing.Name))
		}
		if len(recipe.Ingredients) > maxRecipeIngredients {
			lines = append(lines, fmt.Sprintf("~ ... and %d more", len(recipe.Ingredients)-maxRecipeIngredients))
		}
	}

	return strings.Join(lines, "\n")
}

// productData represents the JSON structure for product domain data.
type productData struct {
	Domain string            `json:"domain"`
	Title  string            `json:"title"`
	Brand  string            `json:"brand"`
	Price  productPriceData  `json:"price"`
	Rating productRatingData `json:"rating"`
	Pros   []string          `json:"pros"`
	Cons   []string          `json:"cons"`
}

type productPriceData struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type productRatingData struct {
	Score float64 `json:"score"`
	Max   float64 `json:"max"`
}

// formatProductCard renders a product domain card for Telegram display.
func formatProductCard(data json.RawMessage) string {
	var product productData
	if err := json.Unmarshal(data, &product); err != nil {
		return ""
	}

	var lines []string
	lines = append(lines, "# Product Details")

	if product.Brand != "" {
		lines = append(lines, "> Brand: "+product.Brand)
	}
	if product.Price.Currency != "" || product.Price.Amount > 0 {
		lines = append(lines, fmt.Sprintf("> Price: %.2f %s", product.Price.Amount, product.Price.Currency))
	}
	if product.Rating.Max > 0 {
		lines = append(lines, fmt.Sprintf("> Rating: %.1f/%.0f", product.Rating.Score, product.Rating.Max))
	}

	// Pros (up to maxProductPros)
	if len(product.Pros) > 0 {
		lines = append(lines, "")
		lines = append(lines, "# Pros")
		limit := len(product.Pros)
		if limit > maxProductPros {
			limit = maxProductPros
		}
		for _, p := range product.Pros[:limit] {
			lines = append(lines, "- "+p)
		}
		if len(product.Pros) > maxProductPros {
			lines = append(lines, fmt.Sprintf("~ ... and %d more", len(product.Pros)-maxProductPros))
		}
	}

	// Cons (up to maxProductCons)
	if len(product.Cons) > 0 {
		lines = append(lines, "")
		lines = append(lines, "# Cons")
		limit := len(product.Cons)
		if limit > maxProductCons {
			limit = maxProductCons
		}
		for _, c := range product.Cons[:limit] {
			lines = append(lines, "- "+c)
		}
		if len(product.Cons) > maxProductCons {
			lines = append(lines, fmt.Sprintf("~ ... and %d more", len(product.Cons)-maxProductCons))
		}
	}

	return strings.Join(lines, "\n")
}
