//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRecommendationWatchConsent_NoAutoWatchFromPassiveBehavior proves
// SCN-039-038 (BS-021): the create endpoint MUST NOT accept a watch unless the
// caller supplies a CONSENT_REQUIRED-cleared confirmation envelope. A request
// missing confirmation flags returns 422 with code CONSENT_REQUIRED — passive
// behavior cannot turn into a watch on its own.
func TestRecommendationWatchConsent_NoAutoWatchFromPassiveBehavior(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiPostJSON(cfg, "/api/recommendations/watches", map[string]any{
		"name":                  "Auto-passive watch (must reject)",
		"kind":                  "location_radius",
		"enabled":               true,
		"scope":                 map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		"filters":               map[string]any{"category": "place", "query": "coffee"},
		"allowed_sources":       []string{"fixture_google_places"},
		"schedule":              map[string]any{"kind": "manual"},
		"max_alerts_per_window": 1,
		"alert_window_seconds":  3600,
		"cooldown_seconds":      0,
		"quiet_hours":           map[string]any{},
		"location_precision":    "neighborhood",
		"delivery_channel":      "telegram",
		"queue_policy":          "drop",
		"freshness_seconds":     86400,
		"consent": map[string]any{
			"scope":            map[string]any{"category": "place"},
			"sources":          []string{"fixture_google_places"},
			"delivery_channel": "telegram",
			"max_alerts":       1,
			"window_seconds":   3600,
			"precision":        "neighborhood",
			"hard_constraints": []string{},
		},
		// Confirmation block is intentionally empty — emulates a passive
		// "auto-create from behavior" attempt.
		"consent_confirmation": map[string]any{},
	})
	if err != nil {
		t.Fatalf("create watch failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 CONSENT_REQUIRED; body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Code         string   `json:"code"`
		Reason       string   `json:"reason"`
		MissingFlags []string `json:"missing_flags"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse error body: %v; body=%s", err, string(body))
	}
	if parsed.Code != "CONSENT_REQUIRED" {
		t.Fatalf("code = %q, want CONSENT_REQUIRED; body=%s", parsed.Code, string(body))
	}
	if parsed.Reason != "create" {
		t.Fatalf("reason = %q, want create; body=%s", parsed.Reason, string(body))
	}
	if len(parsed.MissingFlags) == 0 {
		t.Fatalf("missing_flags must enumerate unconfirmed knobs; body=%s", string(body))
	}
}

// TestRecommendationWatchConsent_ScopeCannotBroadenSilently proves
// SCN-039-039 (BS-022): a watch update that broadens scope (or any other
// consent-tracked knob) MUST be rejected with 422 CONSENT_REQUIRED unless the
// caller supplies a refreshed confirmation envelope citing the broadened
// fields. This is the planning-routed behavior; below
// `TestConsentRegression_BS022_NoSilentBroadening` is the explicit regression
// adversarial test required by the spec.
func TestRecommendationWatchConsent_ScopeCannotBroadenSilently(t *testing.T) {
	runScopeBroadeningRejectionScenario(t, "scope-broaden-rejection")
}

// TestConsentRegression_BS022_NoSilentBroadening is the mandatory adversarial
// regression test required by the spec. It would FAIL if a future regression
// allowed a watch update to broaden scope/sources/precision/rate without a new
// consent confirmation. The test:
//   1. creates a narrow watch with full confirmation
//   2. sends an update that broadens scope (adds an extra category) WITHOUT
//      confirmation — MUST receive 422 CONSENT_REQUIRED with broadened_fields
//   3. proves the persisted watch is unchanged
//   4. sends the SAME update WITH consent_confirmation flags set — MUST 200
func TestConsentRegression_BS022_NoSilentBroadening(t *testing.T) {
	runScopeBroadeningRejectionScenario(t, "consent-regression-bs022")
}

func runScopeBroadeningRejectionScenario(t *testing.T, prefix string) {
	t.Helper()
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	suffix := time.Now().UnixNano()
	name := prefix + "-" + jsonInt(suffix)

	// Step 1 — create a narrow watch with full consent confirmation.
	createResp, err := apiPostJSON(cfg, "/api/recommendations/watches", map[string]any{
		"name":                  name,
		"kind":                  "location_radius",
		"enabled":               true,
		"scope":                 map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		"filters":               map[string]any{"category": "place", "query": "coffee"},
		"allowed_sources":       []string{"fixture_google_places"},
		"schedule":              map[string]any{"kind": "manual"},
		"max_alerts_per_window": 1,
		"alert_window_seconds":  3600,
		"cooldown_seconds":      0,
		"quiet_hours":           map[string]any{},
		"location_precision":    "neighborhood",
		"delivery_channel":      "telegram",
		"queue_policy":          "drop",
		"freshness_seconds":     86400,
		"consent": map[string]any{
			"scope":            map[string]any{"category": "place"},
			"sources":          []string{"fixture_google_places"},
			"delivery_channel": "telegram",
			"max_alerts":       1,
			"window_seconds":   3600,
			"precision":        "neighborhood",
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
		t.Fatalf("create watch failed: %v", err)
	}
	createBody, err := readBody(createResp)
	if err != nil {
		t.Fatalf("read create body: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", createResp.StatusCode, string(createBody))
	}
	var created struct {
		ID                 string         `json:"id"`
		Scope              map[string]any `json:"scope"`
		MaxAlertsPerWindow int            `json:"max_alerts_per_window"`
		LocationPrecision  string         `json:"location_precision"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		t.Fatalf("parse create body: %v; body=%s", err, string(createBody))
	}
	if created.ID == "" {
		t.Fatalf("created watch missing id: %s", string(createBody))
	}
	t.Cleanup(func() {
		_, _ = httpDelete(cfg, "/api/recommendations/watches/"+created.ID)
	})

	// Step 2 — broaden scope (add radius category) WITHOUT confirmation.
	broadenResp, err := apiPutJSON(cfg, "/api/recommendations/watches/"+created.ID, map[string]any{
		"name":                  name,
		"kind":                  "location_radius",
		"enabled":               true,
		"scope":                 map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		"filters":               map[string]any{"category": "place", "query": "coffee"},
		"allowed_sources":       []string{"fixture_google_places", "fixture_yelp"}, // BROADENED sources
		"schedule":              map[string]any{"kind": "manual"},
		"max_alerts_per_window": 5, // BROADENED rate
		"alert_window_seconds":  3600,
		"cooldown_seconds":      0,
		"quiet_hours":           map[string]any{},
		"location_precision":    "city", // BROADENED precision
		"delivery_channel":      "telegram",
		"queue_policy":          "drop",
		"freshness_seconds":     86400,
		"consent": map[string]any{
			"scope":            map[string]any{"category": "place"},
			"sources":          []string{"fixture_google_places", "fixture_yelp"},
			"delivery_channel": "telegram",
			"max_alerts":       5,
			"window_seconds":   3600,
			"precision":        "city",
			"hard_constraints": []string{},
		},
		"consent_confirmation": map[string]any{}, // Adversarial: silent attempt.
	})
	if err != nil {
		t.Fatalf("broaden update failed: %v", err)
	}
	broadenBody, err := readBody(broadenResp)
	if err != nil {
		t.Fatalf("read broaden body: %v", err)
	}
	if broadenResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("broaden silently returned %d, want 422 CONSENT_REQUIRED; body=%s", broadenResp.StatusCode, string(broadenBody))
	}
	var broadenErr struct {
		Code             string   `json:"code"`
		Reason           string   `json:"reason"`
		BroadenedFields  []string `json:"broadened_fields"`
	}
	if err := json.Unmarshal(broadenBody, &broadenErr); err != nil {
		t.Fatalf("parse broaden body: %v; body=%s", err, string(broadenBody))
	}
	if broadenErr.Code != "CONSENT_REQUIRED" {
		t.Fatalf("broaden code = %q, want CONSENT_REQUIRED; body=%s", broadenErr.Code, string(broadenBody))
	}
	if broadenErr.Reason != "broaden" {
		t.Fatalf("broaden reason = %q, want broaden; body=%s", broadenErr.Reason, string(broadenBody))
	}
	if len(broadenErr.BroadenedFields) == 0 {
		t.Fatalf("broadened_fields must enumerate the broadened knobs; body=%s", string(broadenBody))
	}

	// Step 3 — confirm persisted watch was NOT mutated.
	getResp, err := apiGet(cfg, "/api/recommendations/watches/"+created.ID)
	if err != nil {
		t.Fatalf("get watch failed: %v", err)
	}
	getBody, err := readBody(getResp)
	if err != nil {
		t.Fatalf("read get body: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200; body=%s", getResp.StatusCode, string(getBody))
	}
	var current struct {
		MaxAlertsPerWindow int    `json:"max_alerts_per_window"`
		LocationPrecision  string `json:"location_precision"`
	}
	if err := json.Unmarshal(getBody, &current); err != nil {
		t.Fatalf("parse get body: %v; body=%s", err, string(getBody))
	}
	if current.MaxAlertsPerWindow != 1 {
		t.Fatalf("watch max_alerts_per_window = %d after rejected broaden, want 1 (unchanged)", current.MaxAlertsPerWindow)
	}
	if !strings.EqualFold(current.LocationPrecision, "neighborhood") {
		t.Fatalf("watch precision = %q after rejected broaden, want neighborhood (unchanged)", current.LocationPrecision)
	}

	// Step 4 — same update WITH confirmation flags set MUST succeed.
	confirmedResp, err := apiPutJSON(cfg, "/api/recommendations/watches/"+created.ID, map[string]any{
		"name":                  name,
		"kind":                  "location_radius",
		"enabled":               true,
		"scope":                 map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		"filters":               map[string]any{"category": "place", "query": "coffee"},
		"allowed_sources":       []string{"fixture_google_places", "fixture_yelp"},
		"schedule":              map[string]any{"kind": "manual"},
		"max_alerts_per_window": 5,
		"alert_window_seconds":  3600,
		"cooldown_seconds":      0,
		"quiet_hours":           map[string]any{},
		"location_precision":    "city",
		"delivery_channel":      "telegram",
		"queue_policy":          "drop",
		"freshness_seconds":     86400,
		"consent": map[string]any{
			"scope":            map[string]any{"category": "place"},
			"sources":          []string{"fixture_google_places", "fixture_yelp"},
			"delivery_channel": "telegram",
			"max_alerts":       5,
			"window_seconds":   3600,
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
		t.Fatalf("confirmed broaden failed: %v", err)
	}
	confirmedBody, err := readBody(confirmedResp)
	if err != nil {
		t.Fatalf("read confirmed body: %v", err)
	}
	if confirmedResp.StatusCode != http.StatusOK {
		t.Fatalf("confirmed broaden status = %d, want 200; body=%s", confirmedResp.StatusCode, string(confirmedBody))
	}
}
