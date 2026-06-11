//go:build integration

// Spec 083 Card Rewards Companion (Scope 06) — T-06-04 (+ live proofs for the
// adversarial scenarios). Live-PostgreSQL integration tests for the reconciler:
// idempotent upsert (SCN-083-F07), re-enrollment pending actions
// (SCN-083-F06), date-driven lifecycle transitions and exclusion of expired
// records (SCN-083-F04/F05), and the DB-level proof that disagreement retains
// both observations (F02) and a manual override is never rewritten (F03).
//
// No mocks — the Store and DB are real and ephemeral. Run via:
//
//	./smackerel.sh test integration --run CardRewardsReconcile
//
// The runner sets DATABASE_URL to the disposable test Postgres and adds
// ./internal/cardrewards/... to the integration package list. Each test
// namespaces its catalog ids with a per-test prefix so repeated runs never
// collide; global list assertions filter to the test's own rows.

package cardrewards

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedReconcileObservations persists len(sets) per-source observations for one
// (card, period) via a real extract run (FK-safe) so the reconciler can read
// them back. Mirrors the production path: the extractor writes observations,
// the reconciler merges them.
func seedReconcileObservations(t *testing.T, ctx context.Context, s *Store, cardID, period string, sets [][]string, confs []float64, start, end time.Time) {
	t.Helper()
	runID := uuid.NewString()
	run := &CardRun{
		ID:               runID,
		RunType:          RunTypeExtract,
		Trigger:          RunTriggerScheduled,
		Status:           RunStatusSuccess,
		SourcesAttempted: len(sets),
		SourcesSucceeded: len(sets),
		StartedAt:        ptrTime(start),
		FinishedAt:       ptrTime(end),
	}
	obs := make([]RotatingCategoryObservation, 0, len(sets))
	for i, set := range sets {
		st, en := start, end
		obs = append(obs, RotatingCategoryObservation{
			ID:              uuid.NewString(),
			CardCatalogID:   cardID,
			PeriodLabel:     period,
			PeriodStart:     &st,
			PeriodEnd:       &en,
			Categories:      set,
			Confidence:      confs[i],
			SourceName:      fmt.Sprintf("Source%d", i+1),
			SourceURL:       "https://example.test/" + cardID,
			SourceEvidence:  ptrStr("seed evidence"),
			ExtractionRunID: runID,
			ObservedAt:      time.Now().UTC(),
		})
	}
	if _, err := s.PersistExtractionRun(ctx, run, obs, nil); err != nil {
		t.Fatalf("seed observations: %v", err)
	}
}

// SCN-083-F07 — reconciling the same observations twice yields exactly ONE
// rotating_categories row per (card, period), with a stable id.
func TestReconcileLivePG_IdempotentUpsert_F07(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)

	// Two agreeing sources (same set, different order).
	seedReconcileObservations(t, ctx, s, cardID, period,
		[][]string{{"Restaurants", "PayPal"}, {"PayPal", "Restaurants"}},
		[]float64{0.90, 0.85}, start, end)

	r := NewReconciler(s, 0.70, nil)
	r.now = func() time.Time { return dateUTC(2026, 8, 15) }
	ref := CardPeriodRef{CardCatalogID: cardID, PeriodLabel: period}

	res1, err := r.Reconcile(ctx, []CardPeriodRef{ref}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Reconcile run 1: %v", err)
	}
	if res1.Reconciled != 1 || res1.Run == nil || res1.Run.RunType != RunTypeReconcile {
		t.Fatalf("run 1: reconciled=%d run=%+v, want 1 reconciled + a reconcile run", res1.Reconciled, res1.Run)
	}
	first, err := s.GetRotatingCategory(ctx, cardID, period)
	if err != nil || first == nil {
		t.Fatalf("GetRotatingCategory after run 1: %v (row=%v)", err, first)
	}
	firstID := first.ID

	// Run again on the same observations — must NOT create a second row.
	res2, err := r.Reconcile(ctx, []CardPeriodRef{ref}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Reconcile run 2: %v", err)
	}
	if res2.Reconciled != 1 {
		t.Fatalf("run 2: reconciled=%d, want 1", res2.Reconciled)
	}

	n, err := s.CountRotatingCategoriesByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("CountRotatingCategoriesByCardPeriod: %v", err)
	}
	if n != 1 {
		t.Fatalf("F07 REGRESSION: expected exactly 1 rotating_categories row per (card,period), got %d", n)
	}

	second, err := s.GetRotatingCategory(ctx, cardID, period)
	if err != nil || second == nil {
		t.Fatalf("GetRotatingCategory after run 2: %v (row=%v)", err, second)
	}
	if second.ID != firstID {
		t.Fatalf("F07: row id changed across runs (%s → %s) — not idempotent", firstID, second.ID)
	}
	if second.SourceCount != 2 || second.NeedsVerification {
		t.Errorf("F07: agreed record should have source_count=2 needs_verification=false, got %d / %v",
			second.SourceCount, second.NeedsVerification)
	}
	if !equalStrings(second.Categories, []string{"PayPal", "Restaurants"}) {
		t.Errorf("F07: categories = %v, want [PayPal Restaurants]", second.Categories)
	}
}

// SCN-083-F06 — a selectable card whose re-enrollment window opens today (and
// is not yet enrolled) is surfaced as a pending re-enrollment action; future
// windows and already-enrolled selections are NOT.
func TestReconcileLivePG_PendingReEnrollment_F06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "citi-custom-cash", CardTypeUserSelected)

	uc := &UserCard{ID: uuid.NewString(), CardCatalogID: cardID, Active: true}
	if err := s.CreateUserCard(ctx, uc); err != nil {
		t.Fatalf("CreateUserCard: %v", err)
	}

	today := dateUTC(2026, 8, 15)
	windowEnd := today.AddDate(0, 3, 0)

	// (1) window opens today, NOT enrolled → pending.
	openSel := &Selection{
		ID: uuid.NewString(), UserCardID: uc.ID, Category: "Restaurants", PeriodLabel: "Q3_2026",
		Enrolled: false, EffectiveStart: ptrTime(today), EffectiveEnd: ptrTime(windowEnd),
	}
	// (2) window opens in the future → NOT pending yet.
	futureSel := &Selection{
		ID: uuid.NewString(), UserCardID: uc.ID, Category: "Travel", PeriodLabel: "Q4_2026",
		Enrolled: false, EffectiveStart: ptrTime(today.AddDate(0, 1, 0)), EffectiveEnd: ptrTime(today.AddDate(0, 4, 0)),
	}
	// (3) window open but already enrolled → NOT pending.
	enrolledSel := &Selection{
		ID: uuid.NewString(), UserCardID: uc.ID, Category: "Gas", PeriodLabel: "Q3_2026",
		Enrolled: true, EnrolledAt: ptrTime(today), EffectiveStart: ptrTime(today), EffectiveEnd: ptrTime(windowEnd),
	}
	for _, sel := range []*Selection{openSel, futureSel, enrolledSel} {
		if err := s.CreateSelection(ctx, sel); err != nil {
			t.Fatalf("CreateSelection %s: %v", sel.Category, err)
		}
	}

	r := NewReconciler(s, 0.70, nil)
	r.now = func() time.Time { return today }
	res, err := r.AdvanceLifecycle(ctx, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("AdvanceLifecycle: %v", err)
	}
	if res.Run == nil || res.Run.RunType != RunTypeReconcile {
		t.Fatalf("F06: expected a reconcile run emitted, got %+v", res.Run)
	}

	mine := filterPendingByUserCard(res.PendingReEnrollments, uc.ID)
	if len(mine) != 1 {
		t.Fatalf("F06: AdvanceLifecycle pending for this card = %d, want 1 (only the window opening today, not enrolled): %+v", len(mine), mine)
	}
	if mine[0].Category != "Restaurants" {
		t.Errorf("F06: pending category = %q, want Restaurants", mine[0].Category)
	}
	if mine[0].CatalogName != "Test citi-custom-cash" {
		t.Errorf("F06: pending catalog name = %q, want resolved card name", mine[0].CatalogName)
	}

	// Direct store query gives the same result (the dashboard's read path).
	pend, err := s.ListPendingReEnrollments(ctx, today)
	if err != nil {
		t.Fatalf("ListPendingReEnrollments: %v", err)
	}
	if got := len(filterPendingByUserCard(pend, uc.ID)); got != 1 {
		t.Fatalf("F06: store ListPendingReEnrollments for this card = %d, want 1", got)
	}
}

func filterPendingByUserCard(in []PendingReEnrollment, userCardID string) []PendingReEnrollment {
	var out []PendingReEnrollment
	for _, p := range in {
		if p.UserCardID == userCardID {
			out = append(out, p)
		}
	}
	return out
}

// SCN-083-F04 + F05 — the daily lifecycle pass advances upcoming→active and
// active→expired by date, and expired records are excluded from the current
// (active) set used for recommendations.
func TestReconcileLivePG_LifecycleTransitions_F04_F05(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	today := dateUTC(2026, 8, 15)

	// (F04) currently 'upcoming' but its window is now current → should become active.
	toActive := &RotatingCategory{
		ID: uuid.NewString(), CardCatalogID: cardID, PeriodLabel: "P_active",
		PeriodStart: ptrTime(dateUTC(2026, 7, 1)), PeriodEnd: ptrTime(dateUTC(2026, 9, 30)),
		Categories: []string{"Restaurants"}, LifecycleState: LifecycleUpcoming, Confidence: 0.9,
	}
	// (F05) currently 'active' but its window has ended → should become expired.
	toExpired := &RotatingCategory{
		ID: uuid.NewString(), CardCatalogID: cardID, PeriodLabel: "P_expired",
		PeriodStart: ptrTime(dateUTC(2026, 4, 1)), PeriodEnd: ptrTime(dateUTC(2026, 6, 30)),
		Categories: []string{"Gas Stations"}, LifecycleState: LifecycleActive, Confidence: 0.9,
	}
	for _, rc := range []*RotatingCategory{toActive, toExpired} {
		if err := s.UpsertRotatingCategory(ctx, rc); err != nil {
			t.Fatalf("seed rotating category %s: %v", rc.PeriodLabel, err)
		}
	}

	r := NewReconciler(s, 0.70, nil)
	r.now = func() time.Time { return today }
	if _, err := r.AdvanceLifecycle(ctx, RunTriggerScheduled); err != nil {
		t.Fatalf("AdvanceLifecycle: %v", err)
	}

	gotActive, err := s.GetRotatingCategory(ctx, cardID, "P_active")
	if err != nil || gotActive == nil {
		t.Fatalf("get P_active: %v", err)
	}
	if gotActive.LifecycleState != LifecycleActive {
		t.Fatalf("F04: P_active lifecycle = %q, want active", gotActive.LifecycleState)
	}
	gotExpired, err := s.GetRotatingCategory(ctx, cardID, "P_expired")
	if err != nil || gotExpired == nil {
		t.Fatalf("get P_expired: %v", err)
	}
	if gotExpired.LifecycleState != LifecycleExpired {
		t.Fatalf("F05: P_expired lifecycle = %q, want expired", gotExpired.LifecycleState)
	}

	// F05 exclusion: the active set includes the now-active record and excludes
	// the expired one.
	active, err := s.ListActiveRotatingCategories(ctx)
	if err != nil {
		t.Fatalf("ListActiveRotatingCategories: %v", err)
	}
	var sawActive, sawExpired bool
	for _, rc := range active {
		if rc.CardCatalogID != cardID {
			continue
		}
		switch rc.PeriodLabel {
		case "P_active":
			sawActive = true
		case "P_expired":
			sawExpired = true
		}
	}
	if !sawActive {
		t.Error("F05: the now-active record must be in the current (active) set")
	}
	if sawExpired {
		t.Error("F05 REGRESSION: an expired record must be excluded from the current (active) set")
	}
}

// SCN-083-F02 (live) — disagreeing sources persist a needs_verification=true
// record AND retain BOTH observations for audit.
func TestReconcileLivePG_DisagreementRetainsBothObservations_F02(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)

	seedReconcileObservations(t, ctx, s, cardID, period,
		[][]string{{"Restaurants"}, {"Groceries"}}, []float64{0.92, 0.88}, start, end)

	r := NewReconciler(s, 0.70, nil)
	r.now = func() time.Time { return dateUTC(2026, 8, 15) }
	res, err := r.Reconcile(ctx, []CardPeriodRef{{CardCatalogID: cardID, PeriodLabel: period}}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Disagreements != 1 || res.NeedsVerification != 1 {
		t.Fatalf("F02: disagreements=%d needs_verification=%d, want 1/1", res.Disagreements, res.NeedsVerification)
	}

	row, err := s.GetRotatingCategory(ctx, cardID, period)
	if err != nil || row == nil {
		t.Fatalf("GetRotatingCategory: %v", err)
	}
	if !row.NeedsVerification {
		t.Fatal("F02 REGRESSION: persisted record must be needs_verification=true on disagreement")
	}

	obs, err := s.ListObservationsByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("ListObservationsByCardPeriod: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("F02: both observations must be retained for audit, got %d", len(obs))
	}
}

// SCN-083-F03 (live) — a manual-override record is NOT rewritten by Reconcile
// even when a high-confidence, disagreeing observation arrives; the observation
// is retained for audit only.
func TestReconcileLivePG_ManualOverrideNotRewritten_F03(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	cardID := seedCatalogCard(t, ctx, s, prefix, "discover-it", CardTypeRotating)
	period := "Q3_2026"
	start, end := dateUTC(2026, 7, 1), dateUTC(2026, 9, 30)

	override := &RotatingCategory{
		ID: uuid.NewString(), CardCatalogID: cardID, PeriodLabel: period,
		PeriodStart: &start, PeriodEnd: &end, Categories: []string{"Gym Memberships"},
		LifecycleState: LifecycleActive, Confidence: 1.0, NeedsVerification: false,
		ManualOverride: true, SourceCount: 1,
	}
	if err := s.UpsertRotatingCategory(ctx, override); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// A new, high-confidence, DISAGREEING observation arrives.
	seedReconcileObservations(t, ctx, s, cardID, period,
		[][]string{{"Restaurants", "PayPal"}}, []float64{0.99}, start, end)

	r := NewReconciler(s, 0.70, nil)
	r.now = func() time.Time { return dateUTC(2026, 8, 15) }
	res, err := r.Reconcile(ctx, []CardPeriodRef{{CardCatalogID: cardID, PeriodLabel: period}}, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.OverridesProtected != 1 || res.Reconciled != 0 {
		t.Fatalf("F03: overrides_protected=%d reconciled=%d, want 1/0", res.OverridesProtected, res.Reconciled)
	}

	row, err := s.GetRotatingCategory(ctx, cardID, period)
	if err != nil || row == nil {
		t.Fatalf("GetRotatingCategory: %v", err)
	}
	if !equalStrings(row.Categories, []string{"Gym Memberships"}) {
		t.Fatalf("F03 REGRESSION: override categories were rewritten to %v", row.Categories)
	}
	if !row.ManualOverride || row.Confidence != 1.0 {
		t.Errorf("F03: override flags changed: manual_override=%v confidence=%v", row.ManualOverride, row.Confidence)
	}

	obs, err := s.ListObservationsByCardPeriod(ctx, cardID, period)
	if err != nil {
		t.Fatalf("ListObservationsByCardPeriod: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("F03: the disagreeing observation must be retained for audit, got %d", len(obs))
	}
}
