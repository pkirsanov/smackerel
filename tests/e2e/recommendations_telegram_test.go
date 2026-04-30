//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/telegram"
)

func TestRecommendationsTelegram_ReactiveCardUsesCompactActions(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedRamenSignalArtifact(t)

	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "quiet ramen near mission",
		"source":           "telegram",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     3,
	})
	if err != nil {
		t.Fatalf("telegram recommendation request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read telegram recommendation body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var request store.RenderedRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("parse telegram recommendation response: %v; body=%s", err, string(body))
	}
	card := telegram.FormatRecommendationCard(request)
	for _, want := range []string{"Recommendations", "Menkichi", "Fixture Google Places", "[Open]", "[Why?]", "[Liked]", "[Not interested]"} {
		if !strings.Contains(card, want) {
			t.Fatalf("telegram card missing %q: %s", want, card)
		}
	}
}
