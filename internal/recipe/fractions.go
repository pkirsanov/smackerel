package recipe

import (
	"fmt"
	"math"
)

// kitchenFraction maps a fractional decimal value to its display string.
type kitchenFraction struct {
	value   float64
	display string
}

// fractionTable lists kitchen fractions in order for matching.
// Tolerance of ±0.02 is applied during lookup.
var fractionTable = []kitchenFraction{
	{0.125, "1/8"},
	{0.167, "1/6"},
	{0.25, "1/4"},
	{0.333, "1/3"},
	{0.375, "3/8"},
	{0.5, "1/2"},
	{0.625, "5/8"},
	{0.667, "2/3"},
	{0.75, "3/4"},
	{0.875, "7/8"},
}

const fractionTolerance = 0.02

// FormatQuantity converts a float64 quantity to a human-readable kitchen
// fraction string. Integer results stay as integers ("3" not "3.0").
// Fractional results use the nearest practical kitchen fraction.
func FormatQuantity(qty float64) string {
	if qty <= 0 {
		return "0"
	}

	// Integer check
	if math.Abs(qty-math.Round(qty)) < 0.001 {
		return fmt.Sprintf("%d", int(math.Round(qty)))
	}

	whole := math.Floor(qty)
	frac := qty - whole

	// Try to match fractional part against the table
	display := matchFraction(frac)
	if display == "" {
		// Round to nearest 1/8 and retry
		rounded := math.Round(frac*8) / 8
		display = matchFraction(rounded)
	}

	if display != "" {
		if whole > 0 {
			return fmt.Sprintf("%d %s", int(whole), display)
		}
		return display
	}

	// Fallback: one decimal place
	if whole > 0 {
		return fmt.Sprintf("%.1f", qty)
	}
	return fmt.Sprintf("%.1f", qty)
}

func matchFraction(frac float64) string {
	for _, kf := range fractionTable {
		if math.Abs(frac-kf.value) < fractionTolerance {
			return kf.display
		}
	}
	return ""
}
