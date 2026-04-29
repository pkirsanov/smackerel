//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReactiveRamenRegression_BS001(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedRamenSignalArtifact(t)

	started := time.Now()
	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "quiet ramen near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     3,
	})
	if err != nil {
		t.Fatalf("recommendation request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read recommendation body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("reactive recommendation took %s, want <= 5s", elapsed)
	}

	var parsed struct {
		RequestID       string `json:"request_id"`
		Status          string `json:"status"`
		TraceID         string `json:"trace_id"`
		Recommendations []struct {
			ID              string   `json:"id"`
			Title           string   `json:"title"`
			Rank            int      `json:"rank"`
			GraphSignalRefs []string `json:"graph_signal_refs"`
			Rationale       []string `json:"rationale"`
			ProviderBadges  []struct {
				ProviderID string `json:"provider_id"`
				Label      string `json:"label"`
			} `json:"provider_badges"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse recommendation response: %v; body=%s", err, string(body))
	}
	if parsed.RequestID == "" || parsed.TraceID == "" {
		t.Fatalf("response missing request/trace ids: %+v", parsed)
	}
	if parsed.Status != "delivered" {
		t.Fatalf("status = %q, want delivered; body=%s", parsed.Status, string(body))
	}
	if len(parsed.Recommendations) != 3 {
		t.Fatalf("recommendation count = %d, want 3; body=%s", len(parsed.Recommendations), string(body))
	}
	for i, rec := range parsed.Recommendations {
		if rec.ID == "" || rec.Title == "" {
			t.Fatalf("recommendation[%d] missing identity/title: %+v", i, rec)
		}
		if rec.Rank != i+1 {
			t.Fatalf("recommendation[%d] rank = %d, want %d", i, rec.Rank, i+1)
		}
		if len(rec.ProviderBadges) == 0 {
			t.Fatalf("recommendation[%d] has no provider badge: %+v", i, rec)
		}
	}
	if !contains(parsed.Recommendations[0].GraphSignalRefs, "ART-123") {
		t.Fatalf("top recommendation missing ART-123 graph signal: %+v", parsed.Recommendations[0])
	}
	if !rationaleMentions(parsed.Recommendations[0].Rationale, "ART-123") {
		t.Fatalf("top recommendation rationale does not cite ART-123: %+v", parsed.Recommendations[0].Rationale)
	}
}

func seedRamenSignalArtifact(t *testing.T) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `
INSERT INTO artifacts (
    id, artifact_type, title, summary, content_raw, content_hash,
    source_id, source_ref, source_quality, processing_status, key_ideas, entities, action_items, topics, source_qualifiers
) VALUES (
    'ART-123', 'note', 'Ramen preference signal', 'Prefers quiet ramen counters',
    'The actor likes quiet ramen places with rich broth and short waits.', 'scope-039-art-123',
    'e2e', 'scope-039-art-123', 'trusted', 'processed', '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '{}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    content_raw = EXCLUDED.content_raw,
    updated_at = NOW()
`)
	if err != nil {
		t.Fatalf("seed ART-123 artifact: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func rationaleMentions(values []string, token string) bool {
	for _, value := range values {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}