//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendationFeedback_NotInterestedScopedToWatch(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome := runFeedbackSeedRecommendation(t, store, "feedback-watch-user")
	if len(outcome.Recommendations) == 0 {
		t.Fatal("seed run delivered no recommendations")
	}
	rec := outcome.Recommendations[0]
	watchID := "watch-feedback-scope"
	insertFeedbackWatch(t, pool, watchID, "feedback-watch-user")

	result, err := store.RecordFeedback(ctx, recstore.FeedbackInput{
		RecommendationID: rec.ID,
		ActorUserID:      "feedback-watch-user",
		FeedbackType:     "not_interested",
		SourceWatchID:    watchID,
		Payload:          map[string]any{"surface": "integration"},
	})
	if err != nil {
		t.Fatalf("RecordFeedback returned error: %v", err)
	}
	if result.SuppressionEffect.Reason != "suppressed:user-not-interested" {
		t.Fatalf("suppression reason = %q, want suppressed:user-not-interested", result.SuppressionEffect.Reason)
	}

	canonicalKey := canonicalKeyForRecommendation(t, pool, rec.ID)
	sameWatch, err := store.ActiveSuppressionDecisions(ctx, recstore.SuppressionLookupInput{
		ActorUserID:   "feedback-watch-user",
		Category:      string(recommendation.CategoryPlace),
		CanonicalKeys: []string{canonicalKey},
		SourceWatchID: watchID,
	})
	if err != nil {
		t.Fatalf("same-watch suppression lookup failed: %v", err)
	}
	if len(sameWatch) != 1 || sameWatch[0].Reason != "suppressed:user-not-interested" {
		t.Fatalf("same-watch decisions = %+v, want user-not-interested suppression", sameWatch)
	}

	unrelated, err := store.ActiveSuppressionDecisions(ctx, recstore.SuppressionLookupInput{
		ActorUserID:   "feedback-watch-user",
		Category:      string(recommendation.CategoryPlace),
		CanonicalKeys: []string{canonicalKey},
		SourceWatchID: "watch-unrelated",
	})
	if err != nil {
		t.Fatalf("unrelated suppression lookup failed: %v", err)
	}
	if len(unrelated) != 0 {
		t.Fatalf("watch-scoped not_interested leaked into unrelated context: %+v", unrelated)
	}
}

func TestRecommendationFeedback_DislikeSuppressesAcrossSurfaces(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome := runFeedbackSeedRecommendation(t, store, "feedback-dislike-user")
	if len(outcome.Recommendations) == 0 {
		t.Fatal("seed run delivered no recommendations")
	}
	disliked := outcome.Recommendations[0]

	result, err := store.RecordFeedback(ctx, recstore.FeedbackInput{
		RecommendationID: disliked.ID,
		ActorUserID:      "feedback-dislike-user",
		FeedbackType:     "tried_disliked",
		Payload:          map[string]any{"surface": "integration"},
	})
	if err != nil {
		t.Fatalf("RecordFeedback returned error: %v", err)
	}
	if result.SuppressionEffect.Reason != "suppressed:user-disliked" {
		t.Fatalf("suppression reason = %q, want suppressed:user-disliked", result.SuppressionEffect.Reason)
	}

	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC) },
	})
	later, err := engine.Run(ctx, reactive.Request{
		ActorUserID:     "feedback-dislike-user",
		Source:          "api",
		Query:           "quiet ramen near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("later Run returned error: %v", err)
	}
	for _, rec := range later.Recommendations {
		if rec.CandidateID == disliked.CandidateID || rec.Title == disliked.Title {
			t.Fatalf("disliked candidate was delivered again: %+v", later.Recommendations)
		}
	}
	assertSuppressedRecommendationReason(t, pool, later.ID, disliked.CandidateID, "suppressed:user-disliked")
}

func runFeedbackSeedRecommendation(t *testing.T, store *recstore.Store, actor string) recstore.RenderedRequest {
	t.Helper()
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC) },
	})
	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     actor,
		Source:          "api",
		Query:           "quiet ramen near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("seed Run returned error: %v", err)
	}
	return outcome
}

func insertFeedbackWatch(t *testing.T, pool *pgxpool.Pool, watchID, actor string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
INSERT INTO recommendation_watches (
    id, actor_user_id, name, kind, enabled, consent, scope, filters, allowed_sources,
    schedule, max_alerts_per_window, alert_window_seconds, cooldown_seconds,
    quiet_hours, location_precision, delivery_channel, queue_policy, created_at, updated_at
) VALUES (
    $1, $2, 'Feedback watch', 'location_radius', true, '{"current":{},"revisions":[]}'::jsonb,
    '{"category":"place"}'::jsonb, '{}'::jsonb, ARRAY['fixture_google_places'],
    '{"kind":"manual"}'::jsonb, 1, 86400, 0, '{}'::jsonb, 'neighborhood', 'api', 'drop', NOW(), NOW()
)
ON CONFLICT (id) DO UPDATE SET actor_user_id = EXCLUDED.actor_user_id`, watchID, actor)
	if err != nil {
		t.Fatalf("insert feedback watch: %v", err)
	}
}

func canonicalKeyForRecommendation(t *testing.T, pool *pgxpool.Pool, recommendationID string) string {
	t.Helper()
	row := pool.QueryRow(context.Background(), `
SELECT c.canonical_key
FROM recommendations r
JOIN recommendation_candidates c ON c.id = r.candidate_id
WHERE r.id = $1`, recommendationID)
	var canonicalKey string
	if err := row.Scan(&canonicalKey); err != nil {
		t.Fatalf("load canonical key: %v", err)
	}
	return canonicalKey
}

func assertSuppressedRecommendationReason(t *testing.T, pool *pgxpool.Pool, requestID, candidateID, reason string) {
	t.Helper()
	row := pool.QueryRow(context.Background(), `
SELECT COUNT(*)
FROM recommendations
WHERE request_id = $1 AND candidate_id = $2 AND status = 'suppressed' AND status_reason = $3`, requestID, candidateID, reason)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count suppressed recommendations: %v", err)
	}
	if count != 1 {
		t.Fatalf("suppressed recommendation rows = %d, want 1 for %s", count, reason)
	}
}
