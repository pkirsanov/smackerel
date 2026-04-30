//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRecommendationPreferences_CorrectionAffectsLaterRanking(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedSpicyPreferenceArtifact(t)

	before := postSpicyRecommendation(t, cfg)
	if len(before.Recommendations) == 0 {
		t.Fatal("initial spicy recommendation delivered no candidates")
	}
	if before.Recommendations[0].ScoreBreakdown["graph_boost"] <= 0 {
		t.Fatalf("initial recommendation did not use graph boost: %+v", before.Recommendations[0].ScoreBreakdown)
	}

	correctionID := createPreferenceCorrection(t, cfg, before.Recommendations[0].ID, "loves_spicy")
	after := postSpicyRecommendation(t, cfg)
	if len(after.Recommendations) == 0 {
		t.Fatal("post-correction spicy recommendation delivered no candidates")
	}
	if after.Recommendations[0].ScoreBreakdown["graph_boost"] != 0 {
		t.Fatalf("corrected preference still applied graph boost: %+v", after.Recommendations[0].ScoreBreakdown)
	}
	if !rationaleMentions(after.Recommendations[0].Rationale, correctionID) {
		t.Fatalf("post-correction rationale does not cite correction %s: %+v", correctionID, after.Recommendations[0].Rationale)
	}
	assertRankTraceCitesCorrection(t, after.TraceID, correctionID)
}

type spicyRecommendationResponse struct {
	RequestID       string `json:"request_id"`
	TraceID         string `json:"trace_id"`
	Status          string `json:"status"`
	Recommendations []struct {
		ID             string             `json:"id"`
		Title          string             `json:"title"`
		ScoreBreakdown map[string]float64 `json:"score_breakdown"`
		Rationale      []string           `json:"rationale"`
	} `json:"recommendations"`
}

func postSpicyRecommendation(t *testing.T, cfg e2eConfig) spicyRecommendationResponse {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "spicy ramen near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     3,
	})
	if err != nil {
		t.Fatalf("spicy recommendation request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read spicy recommendation body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var parsed spicyRecommendationResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse spicy recommendation response: %v; body=%s", err, string(body))
	}
	if parsed.Status != "delivered" {
		t.Fatalf("spicy recommendation status = %q, want delivered; body=%s", parsed.Status, string(body))
	}
	return parsed
}

func createPreferenceCorrection(t *testing.T, cfg e2eConfig, recommendationID, preferenceKey string) string {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/recommendations/"+recommendationID+"/feedback", map[string]any{
		"feedback_type":   "wrong_preference",
		"preference_key":  preferenceKey,
		"correction_kind": "remove",
		"payload":         map[string]any{"reason": "user says this preference is wrong"},
	})
	if err != nil {
		t.Fatalf("preference correction request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read preference correction body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		PreferenceEffect struct {
			CorrectionID string `json:"correction_id"`
		} `json:"preference_effect"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse preference correction response: %v; body=%s", err, string(body))
	}
	if parsed.PreferenceEffect.CorrectionID == "" {
		t.Fatalf("preference correction response missing correction id: %s", string(body))
	}
	t.Cleanup(func() {
		revokePreferenceCorrectionRecord(t, parsed.PreferenceEffect.CorrectionID)
	})
	return parsed.PreferenceEffect.CorrectionID
}

func revokePreferenceCorrectionRecord(t *testing.T, correctionID string) {
	t.Helper()
	pool := e2ePool(t)
	if _, err := pool.Exec(context.Background(), `
UPDATE recommendation_preference_corrections
SET revoked_at = NOW()
WHERE id = $1
  AND revoked_at IS NULL`, correctionID); err != nil {
		t.Fatalf("revoke preference correction %s: %v", correctionID, err)
	}
}

func cleanupRecommendationFeedbackEffects(t *testing.T, recommendationID string) {
	t.Helper()
	pool := e2ePool(t)
	if _, err := pool.Exec(context.Background(), `
DELETE FROM recommendation_suppression_state suppression
USING recommendations recommendation
WHERE recommendation.id = $1
  AND suppression.actor_user_id = recommendation.actor_user_id
  AND suppression.candidate_id = recommendation.candidate_id`, recommendationID); err != nil {
		t.Fatalf("cleanup recommendation suppression for %s: %v", recommendationID, err)
	}
	if _, err := pool.Exec(context.Background(), `
DELETE FROM recommendation_feedback
WHERE recommendation_id = $1`, recommendationID); err != nil {
		t.Fatalf("cleanup recommendation feedback for %s: %v", recommendationID, err)
	}
}

func seedSpicyPreferenceArtifact(t *testing.T) {
	t.Helper()
	pool := e2ePool(t)
	_, err := pool.Exec(context.Background(), `
INSERT INTO artifacts (
    id, artifact_type, title, summary, content_raw, content_hash,
    source_id, source_ref, source_quality, processing_status, key_ideas, entities, action_items, topics, source_qualifiers
) VALUES (
    'ART-SPICY-039', 'note', 'The user loves spicy ramen', 'The user loves spicy ramen and spicy food',
    'The user loves spicy ramen and spicy food.', 'scope-039-art-spicy',
    'e2e', 'scope-039-art-spicy', 'trusted', 'processed', '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '{}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    content_raw = EXCLUDED.content_raw,
    updated_at = NOW()
`)
	if err != nil {
		t.Fatalf("seed spicy preference artifact: %v", err)
	}
}

func assertRankTraceCitesCorrection(t *testing.T, traceID, correctionID string) {
	t.Helper()
	pool := e2ePool(t)
	var count int
	if err := pool.QueryRow(context.Background(), `
SELECT COUNT(*)
FROM agent_tool_calls
WHERE trace_id = $1
  AND tool_name = 'recommendation_rank_candidates'
  AND result::text LIKE '%' || $2 || '%'`, traceID, correctionID).Scan(&count); err != nil {
		t.Fatalf("count rank trace correction refs: %v", err)
	}
	if count == 0 {
		t.Fatalf("rank trace %s did not cite correction %s", traceID, correctionID)
	}
}

func feedbackHTMLMentions(html string, tokens ...string) bool {
	for _, token := range tokens {
		if !strings.Contains(html, token) {
			return false
		}
	}
	return true
}
