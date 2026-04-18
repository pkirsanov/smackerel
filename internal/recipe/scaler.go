package recipe

// ScaleIngredients returns a new slice of ScaledIngredient with quantities
// adjusted by the ratio targetServings/originalServings. Ingredients with
// unparseable quantities (empty, "to taste", "a pinch") are returned with
// Scaled=false and their original text preserved.
//
// Returns nil if originalServings or targetServings is <= 0.
func ScaleIngredients(
	ingredients []Ingredient,
	originalServings int,
	targetServings int,
) []ScaledIngredient {
	if originalServings <= 0 || targetServings <= 0 {
		return nil
	}

	ratio := float64(targetServings) / float64(originalServings)
	result := make([]ScaledIngredient, 0, len(ingredients))

	for _, ing := range ingredients {
		qty, _ := ParseQuantity(ing.Quantity, ing.Unit)

		si := ScaledIngredient{
			Name:        ing.Name,
			Quantity:    ing.Quantity,
			Unit:        ing.Unit,
			Preparation: ing.Preparation,
		}

		if qty == 0 {
			// Unparseable quantity — return original text unscaled
			si.Scaled = false
			si.DisplayQuantity = ing.Quantity
		} else {
			scaledQty := qty * ratio
			si.Scaled = true
			si.ScaledValue = scaledQty
			si.DisplayQuantity = FormatQuantity(scaledQty)
		}

		result = append(result, si)
	}

	return result
}
