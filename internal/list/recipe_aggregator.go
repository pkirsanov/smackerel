package list

import (
	"encoding/json"

	"github.com/smackerel/smackerel/internal/recipe"
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
			normName := recipe.NormalizeIngredientName(ing.Name)
			qty, unit := recipe.ParseQuantity(ing.Quantity, ing.Unit)
			normUnit := recipe.NormalizeUnit(unit)

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
					category: recipe.CategorizeIngredient(normName),
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
		content := recipe.FormatIngredient(s.item.name, s.item.quantity, s.item.unit, s.item.preparation)

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

// ParseQuantity delegates to the shared recipe package.
var ParseQuantity = recipe.ParseQuantity

// NormalizeUnit delegates to the shared recipe package.
var NormalizeUnit = recipe.NormalizeUnit

// NormalizeIngredientName delegates to the shared recipe package.
var NormalizeIngredientName = recipe.NormalizeIngredientName

// CategorizeIngredient delegates to the shared recipe package.
var CategorizeIngredient = recipe.CategorizeIngredient

// FormatIngredient delegates to the shared recipe package.
var FormatIngredient = recipe.FormatIngredient
