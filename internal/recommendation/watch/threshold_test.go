// Spec 039 BUG-039-004 — tests for the price-drop threshold resolution.
// The fallback is now an OPERATIONAL SST value (no hardcoded 0.10 literal); this
// proves the trigger -> filter -> SST-default precedence.
package watch

import (
	"testing"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestResolvePriceDropThreshold_Precedence(t *testing.T) {
	const sstDefault = 0.10

	cases := []struct {
		name    string
		trigger map[string]any
		filter  map[string]any
		want    float64
	}{
		{
			name:    "trigger wins over filter and default",
			trigger: map[string]any{"threshold_pct": 0.25},
			filter:  map[string]any{"threshold_pct": 0.15},
			want:    0.25,
		},
		{
			name:    "filter wins when trigger absent",
			trigger: map[string]any{},
			filter:  map[string]any{"threshold_pct": 0.15},
			want:    0.15,
		},
		{
			name:    "filter wins when trigger zero",
			trigger: map[string]any{"threshold_pct": 0.0},
			filter:  map[string]any{"threshold_pct": 0.15},
			want:    0.15,
		},
		{
			name:    "SST default when neither trigger nor filter set",
			trigger: map[string]any{},
			filter:  map[string]any{},
			want:    sstDefault,
		},
		{
			name:    "SST default when both zero",
			trigger: map[string]any{"threshold_pct": 0.0},
			filter:  map[string]any{"threshold_pct": 0.0},
			want:    sstDefault,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trigger := TriggerContext{Kind: "price_check", Context: tc.trigger}
			watch := recstore.WatchRecord{WatchInput: recstore.WatchInput{Filters: tc.filter}}
			got := resolvePriceDropThreshold(trigger, watch, sstDefault)
			if got != tc.want {
				t.Errorf("resolvePriceDropThreshold = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolvePriceDropThreshold_UsesConfiguredDefault(t *testing.T) {
	// Proves the fallback is the CONFIGURED value, not a hardcoded 0.10: a
	// different default flows straight through when no override is present.
	trigger := TriggerContext{Kind: "price_check", Context: map[string]any{}}
	watch := recstore.WatchRecord{WatchInput: recstore.WatchInput{Filters: map[string]any{}}}
	if got := resolvePriceDropThreshold(trigger, watch, 0.30); got != 0.30 {
		t.Errorf("resolvePriceDropThreshold default passthrough = %v, want 0.30", got)
	}
}
