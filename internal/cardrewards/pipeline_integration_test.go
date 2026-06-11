//go:build integration

// Spec 083 Card Rewards Companion (Scope 09) — T-09-02 / T-09-03.
//
// Live-PostgreSQL integration tests for the refresh/recommend Pipeline — the
// shared code path the scheduler's jobs + manual triggers drive (design §8 /
// FR-CR-018, FR-CR-019, NFR-CR-005). The Store and DB are real and ephemeral;
// only the two EXTERNAL boundaries are faked:
//
//   - the SOURCE WEBSITE (pipelineStubConnector returns pre-fetched
//     source-page artifacts — the real connector's fetch is covered by Scope
//     04; here we fake the external fetch result, mirroring the blessed
//     CalDAVClient external-boundary fake);
//   - the ML SIDECAR / Ollama (a real HTTPSidecarExtractor pointed at a local
//     httptest server returning 503 — a genuine HTTP round-trip that fails
//     because no model is available, the deferred live-Ollama state). The
//     extract orchestrator records a PARTIAL extract run and flags targets; it
//     NEVER fabricates an observation.
//
// Scenarios:
//   - SCN-083-I03: the daily refresh runs connector sync → extract → reconcile
//     → lifecycle and writes card_runs (scrape + extract + 2×reconcile), with
//     reconcile genuinely merging seeded observations into the authoritative
//     rotating_categories record on live PG.
//   - SCN-083-I04: the monthly recommend runs optimize → recommend → calendar
//     sync, persisting a card_recommendations row + an optimize run + a
//     calendar_sync run + a CalDAV event.
//   - SCN-083-I05: the manual "scrape now" trigger reuses the SAME refresh code
//     path with trigger="manual" (the card_runs rows carry trigger="manual").
//   - SCN-083-I06 (ADVERSARIAL): re-running the refresh upserts the same single
//     rotating_categories row (no duplicate) and re-running the recommend
//     upserts the same card_recommendations row + updates the same CalDAV UID
//     (no duplicate event). The assertions FAIL if idempotency regresses.
//
// Run via: ./smackerel.sh test integration --go-run CardRewardsPipelineLivePG
// The runner sets DATABASE_URL to the disposable test Postgres and adds
// ./internal/cardrewards/... to the integration package list. Each test
// namespaces its catalog ids / categories with a per-test prefix; run-row
// assertions use before/after deltas (the runner executes integration tests
// serially, -p 1).

package cardrewards

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/connector"
)

// pipelineStubConnector is an external-boundary fake of the source website: it
// returns pre-fetched source-page artifacts so the daily refresh's connector
// stage runs without external network. The real connector's fetch behavior is
// covered by Scope 04 (connector_test.go).
type pipelineStubConnector struct {
	artifacts []connector.RawArtifact
	err       error
}

func (c *pipelineStubConnector) Sync(_ context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	if c.err != nil {
		return nil, cursor, c.err
	}
	return c.artifacts, "cursor-" + time.Now().UTC().Format(time.RFC3339Nano), nil
}

// unavailableSidecarExtractor builds a REAL HTTPSidecarExtractor pointed at a
// local httptest server that returns 503 — a genuine HTTP round-trip to the ML
// sidecar that fails because no model is available (the deferred live-Ollama
// state). The Scope 05 orchestrator wrapping it records a partial extract run;
// nothing is fabricated. The server is torn down on test cleanup.
func unavailableSidecarExtractor(t *testing.T, store *Store, threshold float64) *Extractor {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no model available on disposable test stack"}`))
	}))
	t.Cleanup(srv.Close)
	sidecar, err := NewHTTPSidecarExtractor(srv.URL, "test-token", 5*time.Second)
	if err != nil {
		t.Fatalf("NewHTTPSidecarExtractor: %v", err)
	}
	return NewExtractor(store, sidecar, threshold, nil)
}

// oneSourceArtifact returns a single source-attributed artifact (one fetched
// source page) carrying provenance metadata, as the real connector would emit.
func oneSourceArtifact(capturedAt time.Time) connector.RawArtifact {
	return connector.RawArtifact{
		SourceID:    "card-rewards",
		SourceRef:   "https://src.test/rotating#2026",
		ContentType: "card-rewards/source-page",
		Title:       "Card rewards source: TestSource",
		RawContent:  "Discover it rotating categories Q3 2026: Restaurants, PayPal.",
		URL:         "https://src.test/rotating",
		Metadata: map[string]any{
			"source_name": "TestSource",
			"source_url":  "https://src.test/rotating",
			"issuer_hint": "Discover",
		},
		CapturedAt: capturedAt,
	}
}

func countRunsBy(t *testing.T, ctx context.Context, s *Store, runType, trigger string) int {
	t.Helper()
	var n int
	var err error
	if trigger == "" {
		err = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM card_runs WHERE run_type=$1`, runType).Scan(&n)
	} else {
		err = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM card_runs WHERE run_type=$1 AND trigger=$2`, runType, trigger).Scan(&n)
	}
	if err != nil {
		t.Fatalf("count runs run_type=%q trigger=%q: %v", runType, trigger, err)
	}
	return n
}

func latestRunStatus(t *testing.T, ctx context.Context, s *Store, runType string) string {
	t.Helper()
	var status string
	if err := s.Pool.QueryRow(ctx,
		`SELECT status FROM card_runs WHERE run_type=$1 ORDER BY created_at DESC, id DESC LIMIT 1`, runType,
	).Scan(&status); err != nil {
		t.Fatalf("latest run status run_type=%q: %v", runType, err)
	}
	return status
}

func countRotating(t *testing.T, ctx context.Context, s *Store, cardID, period string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rotating_categories WHERE card_catalog_id=$1 AND period_label=$2`, cardID, period,
	).Scan(&n); err != nil {
		t.Fatalf("count rotating_categories %s/%s: %v", cardID, period, err)
	}
	return n
}

func countObservations(t *testing.T, ctx context.Context, s *Store, cardID, period string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rotating_category_observations WHERE card_catalog_id=$1 AND period_label=$2`, cardID, period,
	).Scan(&n); err != nil {
		t.Fatalf("count observations %s/%s: %v", cardID, period, err)
	}
	return n
}

func countRecommendations(t *testing.T, ctx context.Context, s *Store, period, category string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM card_recommendations WHERE period_label=$1 AND category=$2`, period, category,
	).Scan(&n); err != nil {
		t.Fatalf("count recommendations %s/%s: %v", period, category, err)
	}
	return n
}

// SCN-083-I03 — the daily refresh runs the full pipeline on live PG and audits
// each stage. Extract fails loud (sidecar 503) without fabricating; reconcile
// genuinely merges the seeded observations into the authoritative record.
func TestCardRewardsPipelineLivePG_DailyRefreshFullPipeline_I03(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	now := dateUTC(2026, 8, 15) // Q3_2026
	period := "Q3_2026"

	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	// Seed two agreeing per-source observations (what a prior successful
	// extraction produced). PersistExtractionRun writes one extract run too.
	seedReconcileObservations(t, ctx, s, cardID, period,
		[][]string{{"Restaurants", "PayPal"}, {"PayPal", "Restaurants"}},
		[]float64{0.90, 0.85}, dateUTC(2026, 7, 1), dateUTC(2026, 9, 30))

	pipeline := newRefreshPipeline(t, s, now)

	beforeScrape := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerScheduled)
	beforeExtract := countRunsBy(t, ctx, s, RunTypeExtract, "")
	beforeReconcile := countRunsBy(t, ctx, s, RunTypeReconcile, RunTriggerScheduled)
	beforeObs := countObservations(t, ctx, s, cardID, period)

	if err := pipeline.RunDailyRefresh(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunDailyRefresh: %v", err)
	}

	// Stage 1 (connector sync) audited via exactly one scheduled scrape run.
	if got := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerScheduled) - beforeScrape; got != 1 {
		t.Fatalf("scheduled scrape runs delta = %d, want 1", got)
	}
	if st := latestRunStatus(t, ctx, s, RunTypeScrape); st != RunStatusSuccess {
		t.Fatalf("latest scrape run status = %q, want %q (connector returned artifacts)", st, RunStatusSuccess)
	}
	// Stage 2 (extract) attempted + audited; fails loud (sidecar 503 → partial),
	// and fabricated NOTHING — the observation count is unchanged.
	if got := countRunsBy(t, ctx, s, RunTypeExtract, "") - beforeExtract; got != 1 {
		t.Fatalf("extract runs delta = %d, want 1 (extract stage must run)", got)
	}
	if st := latestRunStatus(t, ctx, s, RunTypeExtract); st != RunStatusPartial {
		t.Fatalf("latest extract run status = %q, want %q (sidecar unavailable → all discarded)", st, RunStatusPartial)
	}
	if got := countObservations(t, ctx, s, cardID, period); got != beforeObs {
		t.Fatalf("observations for %s/%s = %d, want %d (extract must NOT fabricate observations)", cardID, period, got, beforeObs)
	}
	// Stages 3+4 (reconcile + advance lifecycle) each audited via a reconcile run.
	if got := countRunsBy(t, ctx, s, RunTypeReconcile, RunTriggerScheduled) - beforeReconcile; got != 2 {
		t.Fatalf("scheduled reconcile runs delta = %d, want 2 (reconcile + advance-lifecycle)", got)
	}
	// Reconcile genuinely produced the authoritative rotating_categories record.
	if got := countRotating(t, ctx, s, cardID, period); got != 1 {
		t.Fatalf("rotating_categories for %s/%s = %d, want 1 (reconcile must merge the seeded observations)", cardID, period, got)
	}
}

// SCN-083-I04 — the monthly recommend runs optimize → recommend → calendar sync
// on live PG: a card_recommendations row, an optimize run, a calendar_sync run,
// and a CalDAV event are all produced for the recommended card.
func TestCardRewardsPipelineLivePG_MonthlyRecommendFullPipeline_I04(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	now := dateUTC(2026, 8, 15)
	period := "2026-08"
	category := prefix + "-dining"

	// A wallet card with a base benefit for the tracked category, plus the
	// category alias that makes it a tracked category.
	catID := seedCatalogWithBase(t, ctx, s, prefix, "dining-card",
		`[{"category":"`+category+`","rate":4,"rate_type":"percent"}]`)
	walletID := addWalletCard(t, ctx, s, catID, "Dining Card")
	if err := s.UpsertCategoryAlias(ctx, &CategoryAlias{
		ID:                uuid.NewString(),
		CanonicalCategory: category,
		Equivalents:       []string{category},
		BuiltIn:           true,
	}); err != nil {
		t.Fatalf("seed category alias: %v", err)
	}

	fake := newFakeCalDAVClient()
	pipeline := newRecommendPipeline(t, s, now, fake)

	beforeOptimize := countRunsBy(t, ctx, s, RunTypeOptimize, RunTriggerScheduled)
	beforeCalendar := countRunsBy(t, ctx, s, RunTypeCalendarSync, RunTriggerScheduled)

	if err := pipeline.RunMonthlyRecommend(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunMonthlyRecommend: %v", err)
	}

	// optimize → recommend wrote exactly my (period, category) recommendation,
	// pointing at my wallet card, and audited one scheduled optimize run.
	if got := countRecommendations(t, ctx, s, period, category); got != 1 {
		t.Fatalf("card_recommendations for %s/%s = %d, want 1", period, category, got)
	}
	rec, err := s.GetRecommendation(ctx, period, category)
	if err != nil || rec == nil {
		t.Fatalf("GetRecommendation %s/%s: %v (rec=%v)", period, category, err, rec)
	}
	if rec.RecommendedUserCardID == nil || *rec.RecommendedUserCardID != walletID {
		t.Fatalf("recommendation card = %v, want %q", rec.RecommendedUserCardID, walletID)
	}
	if rec.Rate != 4 {
		t.Fatalf("recommendation rate = %v, want 4", rec.Rate)
	}
	if got := countRunsBy(t, ctx, s, RunTypeOptimize, RunTriggerScheduled) - beforeOptimize; got != 1 {
		t.Fatalf("scheduled optimize runs delta = %d, want 1", got)
	}
	// calendar sync audited one scheduled calendar_sync run and wrote the event.
	if got := countRunsBy(t, ctx, s, RunTypeCalendarSync, RunTriggerScheduled) - beforeCalendar; got != 1 {
		t.Fatalf("scheduled calendar_sync runs delta = %d, want 1", got)
	}
	wantUID := pipeline.calendar.RecommendationUID(period, category)
	if _, ok := fake.events[wantUID]; !ok {
		t.Fatalf("expected a CalDAV event under UID %q after recommend", wantUID)
	}
	// And the recommendation persisted that stable UID for the next sync.
	rec2, _ := s.GetRecommendation(ctx, period, category)
	if rec2.CalendarEventUID == nil || *rec2.CalendarEventUID != wantUID {
		t.Fatalf("recommendation calendar_event_uid = %v, want %q", rec2.CalendarEventUID, wantUID)
	}
}

// SCN-083-I05 — the manual "scrape now" trigger reuses the SAME refresh code
// path with trigger="manual": the refresh's card_runs rows carry
// trigger="manual" and the pipeline genuinely ran the stages on live PG.
func TestCardRewardsPipelineLivePG_ManualTriggerReusesRefresh_I05(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	now := dateUTC(2026, 8, 15)
	period := "Q3_2026"

	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	seedReconcileObservations(t, ctx, s, cardID, period,
		[][]string{{"Grocery Stores"}, {"Grocery Stores"}},
		[]float64{0.92, 0.88}, dateUTC(2026, 7, 1), dateUTC(2026, 9, 30))

	pipeline := newRefreshPipeline(t, s, now)

	beforeManualScrape := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerManual)
	beforeScheduledScrape := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerScheduled)

	// The manual trigger is the SAME RunDailyRefresh method the scheduler calls,
	// invoked with trigger="manual" (NFR-CR-005). Driving it directly here is
	// exactly what scheduler.TriggerCardRewardsRefreshNow does.
	if err := pipeline.RunDailyRefresh(ctx, RunTriggerManual); err != nil {
		t.Fatalf("RunDailyRefresh(manual): %v", err)
	}

	if got := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerManual) - beforeManualScrape; got != 1 {
		t.Fatalf("manual scrape runs delta = %d, want 1 (manual trigger must write a manual-trigger run)", got)
	}
	if got := countRunsBy(t, ctx, s, RunTypeScrape, RunTriggerScheduled) - beforeScheduledScrape; got != 0 {
		t.Fatalf("scheduled scrape runs delta = %d, want 0 (manual trigger must NOT mislabel as scheduled)", got)
	}
	// Same code path → the manual refresh genuinely reconciled on live PG.
	if got := countRotating(t, ctx, s, cardID, period); got != 1 {
		t.Fatalf("rotating_categories for %s/%s = %d, want 1 (manual refresh ran reconcile)", cardID, period, got)
	}
}

// SCN-083-I06 (ADVERSARIAL) — re-running both jobs is idempotent: the refresh
// upserts the SAME single rotating_categories row (no duplicate) and the
// recommend upserts the SAME card_recommendations row + updates the SAME CalDAV
// UID (no duplicate event). These assertions FAIL if idempotency regresses
// (e.g., an INSERT replacing the upsert).
func TestCardRewardsPipelineLivePG_ReRunIdempotent_I06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	now := dateUTC(2026, 8, 15)
	quarter := "Q3_2026"
	month := "2026-08"
	category := prefix + "-groceries"

	// --- refresh idempotency: rotating_categories upserted, never duplicated --
	rotCard := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	seedReconcileObservations(t, ctx, s, rotCard, quarter,
		[][]string{{"Grocery Stores", "Drug Stores"}, {"Drug Stores", "Grocery Stores"}},
		[]float64{0.90, 0.86}, dateUTC(2026, 7, 1), dateUTC(2026, 9, 30))

	refresh := newRefreshPipeline(t, s, now)
	if err := refresh.RunDailyRefresh(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunDailyRefresh #1: %v", err)
	}
	if got := countRotating(t, ctx, s, rotCard, quarter); got != 1 {
		t.Fatalf("after refresh #1: rotating_categories = %d, want 1", got)
	}
	if err := refresh.RunDailyRefresh(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunDailyRefresh #2: %v", err)
	}
	// ADVERSARIAL: a non-idempotent reconcile would now show 2.
	if got := countRotating(t, ctx, s, rotCard, quarter); got != 1 {
		t.Fatalf("after refresh #2: rotating_categories = %d, want 1 (re-run must upsert, not duplicate)", got)
	}

	// --- recommend idempotency: recommendation upserted; calendar UID reused ---
	catID := seedCatalogWithBase(t, ctx, s, prefix, "groc-card",
		`[{"category":"`+category+`","rate":5,"rate_type":"percent"}]`)
	walletID := addWalletCard(t, ctx, s, catID, "Groceries Card")
	if err := s.UpsertCategoryAlias(ctx, &CategoryAlias{
		ID:                uuid.NewString(),
		CanonicalCategory: category,
		Equivalents:       []string{category},
		BuiltIn:           true,
	}); err != nil {
		t.Fatalf("seed category alias: %v", err)
	}

	fake := newFakeCalDAVClient()
	recommend := newRecommendPipeline(t, s, now, fake)
	if err := recommend.RunMonthlyRecommend(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunMonthlyRecommend #1: %v", err)
	}
	wantUID := recommend.calendar.RecommendationUID(month, category)
	putsAfterFirst := fake.putCalls
	if got := countRecommendations(t, ctx, s, month, category); got != 1 {
		t.Fatalf("after recommend #1: card_recommendations = %d, want 1", got)
	}
	if _, ok := fake.events[wantUID]; !ok {
		t.Fatalf("after recommend #1: expected CalDAV event under UID %q", wantUID)
	}

	if err := recommend.RunMonthlyRecommend(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("RunMonthlyRecommend #2: %v", err)
	}
	// ADVERSARIAL: a non-idempotent recommend would create a 2nd row or a 2nd event.
	if got := countRecommendations(t, ctx, s, month, category); got != 1 {
		t.Fatalf("after recommend #2: card_recommendations = %d, want 1 (re-run must upsert, not duplicate)", got)
	}
	if fake.putCalls <= putsAfterFirst {
		t.Fatalf("re-sync made no PutEvent calls (putCalls=%d) — the event must be UPDATED", fake.putCalls)
	}
	if _, ok := fake.events[wantUID]; !ok {
		t.Fatalf("after recommend #2: lost the event under UID %q", wantUID)
	}
	// The recommendation still resolves to the SAME stable UID (no churn) and
	// still points at the same wallet card.
	rec, _ := s.GetRecommendation(ctx, month, category)
	if rec == nil || rec.CalendarEventUID == nil || *rec.CalendarEventUID != wantUID {
		t.Fatalf("recommendation UID after re-run = %v, want %q", rec, wantUID)
	}
	if rec.RecommendedUserCardID == nil || *rec.RecommendedUserCardID != walletID {
		t.Fatalf("recommendation card after re-run = %v, want %q", rec.RecommendedUserCardID, walletID)
	}
}

// newRefreshPipeline builds a refresh pipeline over the live store with a
// fixed clock, a one-artifact source fake, and a genuinely-unavailable ML
// sidecar (503). calendar is nil (the refresh path does not touch it).
func newRefreshPipeline(t *testing.T, s *Store, now time.Time) *Pipeline {
	t.Helper()
	conn := &pipelineStubConnector{artifacts: []connector.RawArtifact{oneSourceArtifact(now)}}
	extractor := unavailableSidecarExtractor(t, s, 0.70)
	reconciler := NewReconciler(s, 0.70, nil)
	reconciler.now = func() time.Time { return now }
	recommender := NewRecommender(s)
	recommender.now = func() time.Time { return now }
	p, err := NewPipeline(conn, extractor, reconciler, recommender, nil, s, nil)
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	p.now = func() time.Time { return now }
	return p
}

// newRecommendPipeline builds a recommend pipeline over the live store with a
// fixed clock and a real calendar bridge over the EXTERNAL-boundary CalDAV fake.
func newRecommendPipeline(t *testing.T, s *Store, now time.Time, fake *fakeCalDAVClient) *Pipeline {
	t.Helper()
	conn := &pipelineStubConnector{artifacts: []connector.RawArtifact{oneSourceArtifact(now)}}
	extractor := unavailableSidecarExtractor(t, s, 0.70)
	reconciler := NewReconciler(s, 0.70, nil)
	reconciler.now = func() time.Time { return now }
	recommender := NewRecommender(s)
	recommender.now = func() time.Time { return now }
	calendar := NewCardCalendarBridge(fake, s, true, "smackerel")
	calendar.now = func() time.Time { return now }
	p, err := NewPipeline(conn, extractor, reconciler, recommender, calendar, s, nil)
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	p.now = func() time.Time { return now }
	return p
}
