//go:build e2e

// Scope 5 DoD: GET /api/recommendations/providers MUST return a sanitized
// view by default and an operator detail view (with reason/observed_at/
// quota/attribution) when `?view=operator` is passed. Neither view may
// EVER include API keys, secrets, or other credentials. SCN-039-039 / BS-024.
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRecommendationsProviders_SanitizedAndOperatorViews_BS024(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Sanitized view: default (no view param).
	resp, err := apiGet(cfg, "/api/recommendations/providers")
	if err != nil {
		t.Fatalf("providers GET (sanitized) failed: %v", err)
	}
	defaultBody, err := readBody(resp)
	if err != nil {
		t.Fatalf("read sanitized body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sanitized status = %d, want 200; body=%s", resp.StatusCode, string(defaultBody))
	}
	defaultText := string(defaultBody)
	// Hard guard: no credential-shaped tokens in the sanitized payload.
	credentialTokens := []string{"api_key", "apikey", "access_token", "secret", "password", "bearer "}
	for _, token := range credentialTokens {
		if strings.Contains(strings.ToLower(defaultText), token) {
			t.Fatalf("sanitized providers payload leaked credential token %q; body=%s", token, defaultText)
		}
	}
	var sanitized struct {
		Providers []struct {
			ProviderID  string   `json:"provider_id"`
			DisplayName string   `json:"display_name"`
			Categories  []string `json:"categories"`
			Status      string   `json:"status"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(defaultBody, &sanitized); err != nil {
		t.Fatalf("parse sanitized body: %v; body=%s", err, defaultText)
	}
	if len(sanitized.Providers) == 0 {
		t.Fatalf("sanitized providers list is empty; expected at least one registered provider; body=%s", defaultText)
	}
	for _, p := range sanitized.Providers {
		if p.ProviderID == "" || p.DisplayName == "" || p.Status == "" {
			t.Fatalf("sanitized provider missing required field: %+v", p)
		}
	}

	// Operator view: ?view=operator.
	respOp, err := apiGet(cfg, "/api/recommendations/providers?view=operator")
	if err != nil {
		t.Fatalf("providers GET (operator) failed: %v", err)
	}
	opBody, err := readBody(respOp)
	if err != nil {
		t.Fatalf("read operator body: %v", err)
	}
	if respOp.StatusCode != http.StatusOK {
		t.Fatalf("operator status = %d, want 200; body=%s", respOp.StatusCode, string(opBody))
	}
	opText := string(opBody)
	// Hard guard: even the operator view MUST NOT leak credentials.
	for _, token := range credentialTokens {
		if strings.Contains(strings.ToLower(opText), token) {
			t.Fatalf("operator providers payload leaked credential token %q; body=%s", token, opText)
		}
	}
	var operator struct {
		Providers []struct {
			ProviderID           string   `json:"provider_id"`
			DisplayName          string   `json:"display_name"`
			Categories           []string `json:"categories"`
			Status               string   `json:"status"`
			Reason               string   `json:"reason"`
			ObservedAt           string   `json:"observed_at"`
			AttributionLabel     string   `json:"attribution_label"`
			QuotaWindowSeconds   int      `json:"quota_window_seconds"`
			MaxRequestsWindow    int      `json:"max_requests_window"`
			ConfiguredCategories []string `json:"configured_categories"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(opBody, &operator); err != nil {
		t.Fatalf("parse operator body: %v; body=%s", err, opText)
	}
	if len(operator.Providers) != len(sanitized.Providers) {
		t.Fatalf("operator providers count = %d, sanitized = %d; views must list the same providers", len(operator.Providers), len(sanitized.Providers))
	}
	// At least one provider MUST surface an observed_at timestamp; otherwise
	// the operator view is degenerate. (We don't require every provider has
	// fired health yet, but at least one should be observable in a healthy stack.)
	var sawObserved bool
	for _, p := range operator.Providers {
		if p.ObservedAt != "" {
			sawObserved = true
			break
		}
	}
	if !sawObserved {
		t.Fatalf("operator view has no provider with observed_at; expected at least one; body=%s", opText)
	}
}
