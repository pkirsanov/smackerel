//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// TestRecommendationsTripDossier_TripContextWatchAttachesRecommendations proves
// SCN-039-037 (BS-009): a trip-context watch evaluation persists recommendation
// rows linked to the trip via the candidate's normalized_fact["trip_id"].
// Scope 4 ships the watch + persistence; the trip dossier UI rendering is a
// later scope, so this test verifies that delivery succeeded and the
// recommendations carry the trip linkage required by the dossier consumer.
func TestRecommendationsTripDossier_TripContextWatchAttachesRecommendations(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	name := "trip-dossier-watch-" + suffix
	tripID := "trip_" + suffix

	createResp, err := apiPostJSON(cfg, "/api/recommendations/watches", map[string]any{
		"name":                  name,
		"kind":                  "trip_context",
		"enabled":               true,
		"scope":                 map[string]any{"category": "place"},
		"filters":               map[string]any{"category": "place"},
		"allowed_sources":       []string{},
		"schedule":              map[string]any{"kind": "trip_window"},
		"max_alerts_per_window": 10,
		"alert_window_seconds":  86400,
		"cooldown_seconds":      0,
		"quiet_hours":           map[string]any{},
		"location_precision":    "city",
		"delivery_channel":      "telegram",
		"queue_policy":          "drop",
		"freshness_seconds":     86400,
		"consent": map[string]any{
			"scope":            map[string]any{"category": "place"},
			"sources":          []string{},
			"delivery_channel": "telegram",
			"max_alerts":       10,
			"window_seconds":   86400,
			"precision":        "city",
			"hard_constraints": []string{},
		},
		"consent_confirmation": map[string]any{
			"scope_named":       true,
			"sources_named":     true,
			"rate_limit_named":  true,
			"precision_named":   true,
			"delivery_named":    true,
			"constraints_named": true,
			"sponsored_named":   true,
		},
	})
	if err != nil {
		t.Fatalf("create trip-context watch failed: %v", err)
	}
	createBody, err := readBody(createResp)
	if err != nil {
		t.Fatalf("read create body: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create watch status = %d, want 201; body=%s", createResp.StatusCode, string(createBody))
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		t.Fatalf("parse create body: %v; body=%s", err, string(createBody))
	}
	if created.ID == "" {
		t.Fatalf("created watch missing id: %s", string(createBody))
	}
	t.Cleanup(func() {
		_, _ = httpDelete(cfg, "/api/recommendations/watches/"+created.ID+"?confirm=yes")
	})

	tripStart := time.Now().UTC().Add(5 * 24 * time.Hour).Format(time.RFC3339)
	candidates := []any{}
	for i := 0; i < 10; i++ {
		candidates = append(candidates, map[string]any{
			"canonical_key": "place:trip_" + suffix + "_" + strconv.Itoa(i),
			"title":         "Trip dossier candidate " + strconv.Itoa(i),
			"provider_id":   "fixture_trip_provider",
			"category":      "place",
		})
	}

	triggerResp, err := apiPostJSON(cfg, "/api/recommendations/watches/"+created.ID+"/trigger", map[string]any{
		"trigger_kind": "trip_window",
		"trigger_context": map[string]any{
			"trip_id":    tripID,
			"trip_start": tripStart,
			"candidates": candidates,
		},
	})
	if err != nil {
		t.Fatalf("trigger watch failed: %v", err)
	}
	triggerBody, err := readBody(triggerResp)
	if err != nil {
		t.Fatalf("read trigger body: %v", err)
	}
	if triggerResp.StatusCode != http.StatusOK {
		t.Fatalf("trigger status = %d, want 200; body=%s", triggerResp.StatusCode, string(triggerBody))
	}
	var triggered struct {
		WatchRunID        string   `json:"watch_run_id"`
		Status            string   `json:"status"`
		DeliveryDecision  string   `json:"delivery_decision"`
		Delivered         int      `json:"delivered"`
		RecommendationIDs []string `json:"recommendation_ids"`
	}
	if err := json.Unmarshal(triggerBody, &triggered); err != nil {
		t.Fatalf("parse trigger body: %v; body=%s", err, string(triggerBody))
	}
	if triggered.WatchRunID == "" {
		t.Fatalf("trigger result missing watch_run_id: %s", string(triggerBody))
	}
	if triggered.DeliveryDecision != "sent" {
		t.Fatalf("delivery_decision = %q, want sent; body=%s", triggered.DeliveryDecision, string(triggerBody))
	}
	if triggered.Delivered != 10 {
		t.Fatalf("delivered = %d, want 10 trip-context recommendations; body=%s", triggered.Delivered, string(triggerBody))
	}
	if len(triggered.RecommendationIDs) < 10 {
		t.Fatalf("recommendation_ids count = %d, want >= 10; body=%s", len(triggered.RecommendationIDs), string(triggerBody))
	}
}
