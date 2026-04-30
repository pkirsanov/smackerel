//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRecommendationsWeb_RendersAPIBoundResultsAndProvenance(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedRamenSignalArtifact(t)

	resp, err := apiGet(cfg, "/recommendations")
	if err != nil {
		t.Fatalf("recommendations shell request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read recommendations shell: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	shell := string(body)
	for _, want := range []string{"Recommendations", "hx-post=\"/recommendations/results\"", "precision_policy"} {
		if !strings.Contains(shell, want) {
			t.Fatalf("recommendations shell missing %q: %s", want, shell)
		}
	}

	form := url.Values{}
	form.Set("query", "quiet ramen near mission")
	form.Set("location_ref", "gps:37.7749,-122.4194")
	form.Set("precision_policy", "neighborhood")
	resultsResp, err := postWebForm(cfg, "/recommendations/results", form)
	if err != nil {
		t.Fatalf("recommendations results request failed: %v", err)
	}
	resultsBody, err := readBody(resultsResp)
	if err != nil {
		t.Fatalf("read recommendations results: %v", err)
	}
	if resultsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsResp.StatusCode, string(resultsBody))
	}
	html := string(resultsBody)
	for _, want := range []string{"recommendation-card", "Menkichi", "Fixture Google Places", "href=\"https://fixture.example/ramen/menkichi\"", "Why?", "ART-123"} {
		if !strings.Contains(html, want) {
			t.Fatalf("recommendations results missing %q: %s", want, html)
		}
	}
}

func postWebForm(cfg e2eConfig, path string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}