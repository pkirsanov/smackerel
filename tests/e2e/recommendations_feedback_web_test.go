//go:build e2e

package e2e

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRecommendationsFeedbackWeb_UpdatesCardAndPreferences(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedSpicyPreferenceArtifact(t)

	parsed := postSpicyRecommendation(t, cfg)
	if len(parsed.Recommendations) == 0 {
		t.Fatal("web feedback seed delivered no recommendations")
	}
	recommendationID := parsed.Recommendations[0].ID
	t.Cleanup(func() {
		cleanupRecommendationFeedbackEffects(t, recommendationID)
	})

	feedbackForm := url.Values{}
	feedbackForm.Set("feedback_type", "not_interested")
	feedbackResp, err := postWebForm(cfg, "/recommendations/"+recommendationID+"/feedback", feedbackForm)
	if err != nil {
		t.Fatalf("web feedback post failed: %v", err)
	}
	feedbackBody, err := readBody(feedbackResp)
	if err != nil {
		t.Fatalf("read feedback html: %v", err)
	}
	if feedbackResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", feedbackResp.StatusCode, string(feedbackBody))
	}
	feedbackHTML := string(feedbackBody)
	if !feedbackHTMLMentions(feedbackHTML, "data-feedback-state=\"suppressed\"", "suppressed:user-not-interested") {
		t.Fatalf("feedback html did not show suppressed state: %s", feedbackHTML)
	}

	correctionID := createPreferenceCorrection(t, cfg, recommendationID, "loves_spicy")
	preferencesResp, err := apiGet(cfg, "/recommendations/preferences")
	if err != nil {
		t.Fatalf("preferences page request failed: %v", err)
	}
	preferencesBody, err := readBody(preferencesResp)
	if err != nil {
		t.Fatalf("read preferences page: %v", err)
	}
	if preferencesResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", preferencesResp.StatusCode, string(preferencesBody))
	}
	preferencesHTML := string(preferencesBody)
	preferencesText := html.UnescapeString(preferencesHTML)
	for _, want := range []string{"Recommendations > Preferences", "loves_spicy", correctionID, "Revoke"} {
		if !strings.Contains(preferencesText, want) {
			t.Fatalf("preferences page missing %q: %s", want, preferencesHTML)
		}
	}
}
