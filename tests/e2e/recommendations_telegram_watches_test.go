//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/telegram"
)

// TestRecommendationsTelegramWatches_AlertCardRenderingMatchesDesign proves the
// spec 039 Scope 4 Telegram alert renders with the marker prefixes (no emoji),
// includes the title|provider header, the why line, and the four compact
// action buttons. The rendering helper is invoked directly from the live
// telegram package, mirroring the pattern used by the other recommendation
// telegram e2e tests (no real Telegram round-trip required).
func TestRecommendationsTelegramWatches_AlertCardRenderingMatchesDesign(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	_ = cfg

	alert := telegram.WatchAlert{
		WatchID:     "watch_001",
		WatchName:   "Quiet ramen near mission",
		ActorUserID: "local",
		Title:       "Menkichi",
		Subtitle:    "Quiet ramen counter near 16th & Mission",
		Provider:    "Fixture Google Places",
		Why:         "Matches your watch (saved 4d ago)",
		Labels:      []string{"open now", "vegetarian options"},
	}

	rendered := telegram.RenderWatchAlertForTests(alert)
	if rendered == "" {
		t.Fatalf("rendered watch alert is empty")
	}
	wants := []string{
		telegram.MarkerInfo + "Menkichi | Fixture Google Places",
		telegram.MarkerListItem + "Quiet ramen counter near 16th & Mission",
		telegram.MarkerListItem + "open now",
		telegram.MarkerListItem + "vegetarian options",
		telegram.MarkerInfo + "Why? Matches your watch (saved 4d ago)",
		telegram.MarkerListItem + "[Open] [Why?] [Not interested] [Snooze 30d]",
	}
	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered alert missing %q\nfull alert:\n%s", want, rendered)
		}
	}
	if containsEmoji(rendered) {
		t.Fatalf("rendered alert contains emoji which is forbidden by the design no-emoji marker rule:\n%s", rendered)
	}
}

func containsEmoji(s string) bool {
	for _, r := range s {
		if r > 0x1F000 {
			return true
		}
	}
	return false
}
