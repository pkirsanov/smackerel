package telegram

import (
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/recipe"
)

// FormatCookStep formats a cook mode step display per UX-2.2.
func FormatCookStep(session *CookSession) string {
	if session.CurrentStep < 1 || session.CurrentStep > session.TotalSteps {
		return ""
	}

	step := session.Steps[session.CurrentStep-1]
	isLastStep := session.CurrentStep == session.TotalSteps
	isSingleStep := session.TotalSteps == 1

	var lines []string

	// Header
	lines = append(lines, fmt.Sprintf("# %s", session.RecipeTitle))
	lines = append(lines, fmt.Sprintf("> Step %d of %d", session.CurrentStep, session.TotalSteps))
	lines = append(lines, "")

	// Instruction
	lines = append(lines, step.Instruction)

	// Duration and technique metadata
	meta := formatStepMeta(step)
	if meta != "" {
		lines = append(lines, "")
		lines = append(lines, meta)
	}

	// Navigation hint
	lines = append(lines, "")
	if isSingleStep {
		lines = append(lines, "Reply: ingredients · done")
	} else if isLastStep {
		lines = append(lines, "Last step. Reply: back · ingredients · done")
	} else {
		lines = append(lines, "Reply: next · back · ingredients · done")
	}

	return strings.Join(lines, "\n")
}

// formatStepMeta builds the "~ duration · technique" line.
// Returns empty string if neither duration nor technique is present.
func formatStepMeta(step recipe.Step) string {
	var parts []string

	if step.DurationMinutes != nil && *step.DurationMinutes > 0 {
		parts = append(parts, fmt.Sprintf("%d min", *step.DurationMinutes))
	}

	if step.Technique != "" {
		parts = append(parts, step.Technique)
	}

	if len(parts) == 0 {
		return ""
	}

	return "~ " + strings.Join(parts, " · ")
}

// FormatCookIngredients formats the ingredient list during cook mode per UX-2.4.
func FormatCookIngredients(session *CookSession) string {
	var lines []string

	// Header
	lines = append(lines, fmt.Sprintf("# %s — Ingredients", session.RecipeTitle))

	// Scaling info
	if session.ScaledServings > 0 && session.OriginalServings > 0 {
		lines = append(lines, fmt.Sprintf("~ %d servings (scaled from %d)", session.ScaledServings, session.OriginalServings))
	}
	lines = append(lines, "")

	// Ingredient list
	if session.ScaleFactor != 1.0 && session.ScaleFactor > 0 {
		// Scaled ingredients
		scaled := recipe.ScaleIngredients(session.Ingredients, session.OriginalServings, session.ScaledServings)
		for _, ing := range scaled {
			if !ing.Scaled {
				qtyPart := ing.Quantity
				if qtyPart == "" {
					lines = append(lines, fmt.Sprintf("- %s (unscaled)", ing.Name))
				} else {
					unitPart := ""
					if ing.Unit != "" {
						unitPart = " " + ing.Unit
					}
					lines = append(lines, fmt.Sprintf("- %s%s %s (unscaled)", qtyPart, unitPart, ing.Name))
				}
			} else {
				unitPart := ""
				if ing.Unit != "" {
					unitPart = ing.Unit + " "
				}
				lines = append(lines, fmt.Sprintf("- %s%s%s", ing.DisplayQuantity, unitPart, ing.Name))
			}
		}
	} else {
		// Unscaled — display as-is
		for _, ing := range session.Ingredients {
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
	}

	lines = append(lines, "")
	lines = append(lines, "Reply: next · back · done")

	return strings.Join(lines, "\n")
}
