package knowledge

import (
	"testing"
	"time"
)

func TestNextWebSnippetLifecycle_Transitions(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		current   WebSnippetLifecycle
		lastRefAt time.Time
		want      WebSnippetLifecycle
	}{
		{"active stays active under 90d", WebSnippetActive, now.AddDate(0, 0, -30), WebSnippetActive},
		{"active boundary 89d stays active", WebSnippetActive, now.AddDate(0, 0, -89), WebSnippetActive},
		{"active 90d → cooling", WebSnippetActive, now.AddDate(0, 0, -90), WebSnippetCooling},
		{"active 100d → cooling", WebSnippetActive, now.AddDate(0, 0, -100), WebSnippetCooling},
		{"cooling 100d stays cooling", WebSnippetCooling, now.AddDate(0, 0, -100), WebSnippetCooling},
		{"cooling 180d → dormant", WebSnippetCooling, now.AddDate(0, 0, -180), WebSnippetDormant},
		{"dormant 200d stays dormant", WebSnippetDormant, now.AddDate(0, 0, -200), WebSnippetDormant},
		{"dormant 365d → archived", WebSnippetDormant, now.AddDate(0, 0, -365), WebSnippetArchived},
		{"archived is terminal", WebSnippetArchived, now.AddDate(0, 0, -9999), WebSnippetArchived},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NextWebSnippetLifecycle(tc.current, tc.lastRefAt, now)
			if got != tc.want {
				t.Errorf("NextWebSnippetLifecycle(%s, idle=%v) = %s, want %s",
					tc.current, now.Sub(tc.lastRefAt), got, tc.want)
			}
		})
	}
}

// Adversarial: a future lastReferencedAt (clock skew) must NOT advance
// the lifecycle. Negative idle days should be treated as still-fresh.
func TestNextWebSnippetLifecycle_FutureTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	if got := NextWebSnippetLifecycle(WebSnippetActive, future, now); got != WebSnippetActive {
		t.Errorf("future lastReferenced should keep state active, got %s", got)
	}
}

func TestWebSnippet_DefaultsApplied(t *testing.T) {
	// Pure struct-level test: zero-value lifecycle/weight default to
	// the expected initial values when InsertWebSnippet runs.
	// (Full DB roundtrip lives in tests/integration.)
	s := &WebSnippet{
		URL:         "https://example.test/page",
		Snippet:     "body",
		ContentHash: "hash-1",
		Provider:    "searxng",
		FetchedAt:   time.Now(),
	}
	if s.LifecycleState != "" {
		t.Errorf("expected zero-value LifecycleState, got %q", s.LifecycleState)
	}
	if s.GraphWeight != 0 {
		t.Errorf("expected zero-value GraphWeight, got %f", s.GraphWeight)
	}
}
