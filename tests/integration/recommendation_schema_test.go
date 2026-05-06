//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendationSchema_RejectsUnknownCandidateBeforeDelivery(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	_, err := store.CreateReactiveRequest(context.Background(), recstore.ReactiveOutcomeInput{
		ActorUserID:                "integration-schema",
		Source:                     "api",
		ScenarioID:                 "recommendation_reactive",
		ScenarioVersion:            "recommendation-reactive-v1",
		ScenarioHash:               "recommendation-reactive-v1",
		RawInput:                   "quiet ramen near mission",
		ParsedRequest:              map[string]any{"query": "quiet ramen near mission"},
		LocationPrecisionRequested: "neighborhood",
		LocationPrecisionSent:      "neighborhood",
		Status:                     "delivered",
		ToolCalls: []recstore.ToolCallRecord{{
			Name:            "recommendation_rank_candidates",
			SideEffectClass: "read",
			Arguments:       map[string]any{"candidate_count": 0},
			Result:          map[string]any{"ranked_count": 1},
			StartedAt:       time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		}},
		Recommendations: []recstore.RecommendationInput{{
			CandidateLocalID: "cand-injected",
			RankPosition:     1,
			Status:           "delivered",
			StatusReason:     "eligible",
			ScoreBreakdown:   map[string]float64{"total": 1},
			Rationale:        []string{"Injected candidate"},
		}},
		StartedAt:   time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 4, 27, 12, 0, 1, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("CreateReactiveRequest delivered an unbacked recommendation; want rejection")
	}
	if !strings.Contains(err.Error(), "unknown candidate") {
		t.Fatalf("error = %v, want unknown candidate rejection", err)
	}
}

func TestRecommendationSchema_ExistingCandidateReuseDoesNotWaitOnUpdateLock(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	canonicalKey := "integration-candidate-reuse-" + testID(t)

	seeded, err := store.CreateReactiveRequest(context.Background(), reactiveCandidateReuseInput(canonicalKey, "seed"))
	if err != nil {
		t.Fatalf("seed existing candidate: %v", err)
	}
	if len(seeded.Recommendations) != 1 {
		t.Fatalf("seeded recommendations = %d, want 1", len(seeded.Recommendations))
	}

	lockCtx, lockCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer lockCancel()
	tx, err := pool.Begin(lockCtx)
	if err != nil {
		t.Fatalf("begin candidate lock tx: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var lockedID string
	if err := tx.QueryRow(lockCtx, `
SELECT id
FROM recommendation_candidates
WHERE category = 'place' AND canonical_key = $1
FOR NO KEY UPDATE`, canonicalKey).Scan(&lockedID); err != nil {
		t.Fatalf("lock existing candidate row: %v", err)
	}

	createCtx, createCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer createCancel()
	rendered, err := store.CreateReactiveRequest(createCtx, reactiveCandidateReuseInput(canonicalKey, "contended"))
	if err != nil {
		t.Fatalf("reuse existing candidate while update lock is held: %v", err)
	}
	if len(rendered.Recommendations) != 1 {
		t.Fatalf("rendered recommendations = %d, want 1", len(rendered.Recommendations))
	}
	if rendered.Recommendations[0].CandidateID != lockedID {
		t.Fatalf("candidate id = %q, want locked existing id %q", rendered.Recommendations[0].CandidateID, lockedID)
	}
}

func TestRecommendationSchema_ProviderFactSnapshotDoesNotWaitOnUniqueFactLock(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	testSlug := testID(t)
	providerCandidateID := "provider-fact-lock-" + testSlug
	canonicalKey := "integration-provider-fact-lock-" + testSlug
	retrievedAt := time.Date(2026, 5, 5, 12, 0, 0, 123456000, time.UTC)

	lockCtx, lockCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer lockCancel()
	tx, err := pool.Begin(lockCtx)
	if err != nil {
		t.Fatalf("begin provider fact lock tx: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(lockCtx, `
INSERT INTO recommendation_provider_facts (
    id, request_id, watch_run_id, provider_id, provider_candidate_id,
    category, normalized_fact, source_retrieved_at, source_updated_at,
    source_payload_hash, raw_payload_expires_at, attribution,
    sponsored_state, restricted_flags, created_at
) VALUES (
    $1, NULL, NULL, 'fixture_google_places', $2,
    'place', $3::jsonb, $4, NULL,
    'locked-provider-fact', $5, $6::jsonb,
    'none', '{}'::jsonb, $4
)`, "rec_fact_locked_"+testSlug, providerCandidateID, `{"title":"Locked Provider Fact"}`, retrievedAt, retrievedAt.Add(24*time.Hour), `{"label":"Locked Fixture"}`)
	if err != nil {
		t.Fatalf("insert locked provider fact: %v", err)
	}

	createCtx, createCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer createCancel()
	rendered, err := store.CreateReactiveRequest(createCtx, reactiveProviderFactSnapshotInput(canonicalKey, providerCandidateID, retrievedAt))
	if err != nil {
		t.Fatalf("persist request-scoped provider fact while conflicting fact lock is held: %v", err)
	}
	if len(rendered.Recommendations) != 1 {
		t.Fatalf("rendered recommendations = %d, want 1", len(rendered.Recommendations))
	}
	if len(rendered.Recommendations[0].ProviderBadges) == 0 {
		t.Fatalf("request-scoped provider badge was not rendered: %+v", rendered.Recommendations[0])
	}
}

func TestRecommendationSchema_ConcurrentReadbackDoesNotDeadlockPool(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	maxConns := int(pool.Stat().MaxConns())
	if maxConns < 2 {
		t.Fatalf("test pool max connections = %d, want at least 2", maxConns)
	}

	workerCount := maxConns * 2
	requestIDs := make([]string, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		input := reactiveCandidateReuseInput(fmt.Sprintf("integration-readback-%s-%02d", testID(t), i), fmt.Sprintf("readback-%02d", i))
		rendered, err := store.CreateReactiveRequest(context.Background(), input)
		if err != nil {
			t.Fatalf("seed readback request %d: %v", i, err)
		}
		requestIDs = append(requestIDs, rendered.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, workerCount)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for _, requestID := range requestIDs {
		requestID := requestID
		go func() {
			defer wg.Done()
			<-start
			rendered, err := store.GetRequest(ctx, requestID)
			if err != nil {
				errs <- err
				return
			}
			if len(rendered.Recommendations) != 1 {
				errs <- fmt.Errorf("request %s recommendations = %d, want 1", requestID, len(rendered.Recommendations))
				return
			}
			if len(rendered.Recommendations[0].ProviderBadges) == 0 {
				errs <- fmt.Errorf("request %s provider badges were not rendered", requestID)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent recommendation readback should not exhaust the pool: %v", err)
		}
	}
}

func reactiveCandidateReuseInput(canonicalKey, suffix string) recstore.ReactiveOutcomeInput {
	now := time.Now().UTC()
	localFactID := "fact-" + suffix
	localCandidateID := "cand-" + suffix
	return recstore.ReactiveOutcomeInput{
		ActorUserID:                "integration-candidate-reuse-" + suffix,
		Source:                     "api",
		ScenarioID:                 "recommendation_reactive",
		ScenarioVersion:            "recommendation-reactive-v1",
		ScenarioHash:               "recommendation-reactive-v1",
		RawInput:                   "candidate reuse coffee",
		ParsedRequest:              map[string]any{"query": "candidate reuse coffee", "category": "place"},
		LocationPrecisionRequested: "neighborhood",
		LocationPrecisionSent:      "neighborhood",
		Status:                     "delivered",
		ToolCalls: []recstore.ToolCallRecord{{
			Name:            "recommendation_rank_candidates",
			SideEffectClass: "read",
			Arguments:       map[string]any{"candidate_count": 1},
			Result:          map[string]any{"ranked_count": 1},
			StartedAt:       now,
		}},
		ProviderFacts: []recstore.ProviderFactInput{{
			LocalID:             localFactID,
			ProviderID:          "fixture_google_places",
			ProviderCandidateID: "provider-" + suffix,
			Category:            "place",
			Title:               "Concurrent Coffee",
			NormalizedFact: map[string]any{
				"title":         "Concurrent Coffee",
				"canonical_key": canonicalKey,
				"canonical_url": "https://fixture.example/coffee/concurrent",
			},
			RetrievedAt:     now,
			Attribution:     map[string]any{"label": "Fixture Google Places", "url": "https://fixture.example/coffee/concurrent"},
			SponsoredState:  "none",
			RestrictedFlags: map[string]any{},
		}},
		Candidates: []recstore.CandidateInput{{
			LocalID:              localCandidateID,
			Category:             "place",
			CanonicalKey:         canonicalKey,
			Title:                "Concurrent Coffee",
			CanonicalURL:         "https://fixture.example/coffee/concurrent",
			CanonicalFact:        map[string]any{"title": "Concurrent Coffee", "canonical_key": canonicalKey},
			DedupeReason:         map[string]any{"strategy": "canonical_key"},
			ProviderFactLocalIDs: []string{localFactID},
			MergeReason:          "same-canonical-key",
		}},
		Recommendations: []recstore.RecommendationInput{{
			CandidateLocalID: localCandidateID,
			RankPosition:     1,
			Status:           "delivered",
			StatusReason:     "eligible",
			ScoreBreakdown:   map[string]float64{"total": 1},
			Rationale:        []string{"Provider-backed candidate reused without rewriting canonical candidate row."},
			PolicyDecisions:  []map[string]any{},
			QualityDecisions: []map[string]any{},
			DeliveryChannel:  "api",
		}},
		StartedAt:   now,
		CompletedAt: now,
	}
}

func reactiveProviderFactSnapshotInput(canonicalKey, providerCandidateID string, retrievedAt time.Time) recstore.ReactiveOutcomeInput {
	localFactID := "fact-provider-lock"
	localCandidateID := "cand-provider-lock"
	return recstore.ReactiveOutcomeInput{
		ActorUserID:                "integration-provider-fact-lock",
		Source:                     "api",
		ScenarioID:                 "recommendation_reactive",
		ScenarioVersion:            "recommendation-reactive-v1",
		ScenarioHash:               "recommendation-reactive-v1",
		RawInput:                   "provider fact lock coffee",
		ParsedRequest:              map[string]any{"query": "provider fact lock coffee", "category": "place"},
		LocationPrecisionRequested: "neighborhood",
		LocationPrecisionSent:      "neighborhood",
		Status:                     "delivered",
		ToolCalls: []recstore.ToolCallRecord{{
			Name:            "recommendation_rank_candidates",
			SideEffectClass: "read",
			Arguments:       map[string]any{"candidate_count": 1},
			Result:          map[string]any{"ranked_count": 1},
			StartedAt:       retrievedAt,
		}},
		ProviderFacts: []recstore.ProviderFactInput{{
			LocalID:             localFactID,
			ProviderID:          "fixture_google_places",
			ProviderCandidateID: providerCandidateID,
			Category:            "place",
			Title:               "Provider Fact Lock Coffee",
			NormalizedFact: map[string]any{
				"title":         "Provider Fact Lock Coffee",
				"canonical_key": canonicalKey,
				"canonical_url": "https://fixture.example/coffee/provider-fact-lock",
			},
			RetrievedAt:     retrievedAt,
			Attribution:     map[string]any{"label": "Fixture Google Places", "url": "https://fixture.example/coffee/provider-fact-lock"},
			SponsoredState:  "none",
			RestrictedFlags: map[string]any{},
		}},
		Candidates: []recstore.CandidateInput{{
			LocalID:              localCandidateID,
			Category:             "place",
			CanonicalKey:         canonicalKey,
			Title:                "Provider Fact Lock Coffee",
			CanonicalURL:         "https://fixture.example/coffee/provider-fact-lock",
			CanonicalFact:        map[string]any{"title": "Provider Fact Lock Coffee", "canonical_key": canonicalKey},
			DedupeReason:         map[string]any{"strategy": "canonical_key"},
			ProviderFactLocalIDs: []string{localFactID},
			MergeReason:          "same-canonical-key",
		}},
		Recommendations: []recstore.RecommendationInput{{
			CandidateLocalID: localCandidateID,
			RankPosition:     1,
			Status:           "delivered",
			StatusReason:     "eligible",
			ScoreBreakdown:   map[string]float64{"total": 1},
			Rationale:        []string{"Provider-backed candidate persisted with request-scoped provider fact evidence."},
			PolicyDecisions:  []map[string]any{},
			QualityDecisions: []map[string]any{},
			DeliveryChannel:  "api",
		}},
		StartedAt:   retrievedAt,
		CompletedAt: retrievedAt,
	}
}
