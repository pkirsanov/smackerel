//go:build integration

// Spec 083 Card Rewards Companion (Scope 08) — T-08-04.
// Live-PostgreSQL integration test for the store-driven calendar sync
// (CardCalendarBridge.SyncPeriod): a calendar sync run is audited via a
// card_runs row with run_type="calendar_sync" recording events_written
// (SCN-083-H06, Principle 8). The Store and DB are real and ephemeral; only the
// EXTERNAL calendar-server boundary is faked (fakeCalDAVClient, defined in
// calendar_test.go). No internal component is mocked.
//
// The test also proves, against live PG, that:
//   - calendar_event_uid is persisted back to each synced recommendation so a
//     re-sync UPDATES the same event (SCN-083-H02 at the store level — the
//     second run leaves exactly one event per UID, not a duplicate);
//   - a recommendation with no recommended card writes no event and stores no
//     UID;
//   - open re-enrollment actions produce their reminder events (SCN-083-H03).
//
// Run via: ./smackerel.sh test integration --go-run CardCalendarBridgeLivePG
// The runner sets DATABASE_URL to the disposable test Postgres and adds
// ./internal/cardrewards/... to the integration package list. Each test
// namespaces its catalog ids / categories with a per-test prefix so repeated
// runs never collide; assertions about counts are scoped to this run's own rows
// (the period's row set within one ephemeral DB) or pinned to the specific
// audit run id returned by SyncPeriod.

package cardrewards

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// SCN-083-H06 — the calendar sync run is audited. Seeds carded + un-carded
// recommendations and an open re-enrollment, runs the store-driven SyncPeriod,
// and asserts a card_runs row with run_type="calendar_sync" records
// events_written, that each synced recommendation persisted its stable
// calendar_event_uid, and that a re-sync updates the same events (no duplicate).
func TestCardCalendarBridgeLivePG_SyncRunAudited_H06(t *testing.T) {
	s := cardRewardsIntegrationStore(t)
	ctx := context.Background()
	prefix := cardRewardsPrefix(t)
	period := "2026-06"
	now := dateUTC(2026, 7, 15)

	// Two wallet cards the recommendations point at.
	diningCat := seedCatalogWithBase(t, ctx, s, prefix, "dining-card",
		`[{"category":"dining","rate":4,"rate_type":"percent"}]`)
	diningWallet := addWalletCard(t, ctx, s, diningCat, "Dining Card")
	grocCat := seedCatalogWithBase(t, ctx, s, prefix, "groc-card",
		`[{"category":"groceries","rate":5,"rate_type":"percent"}]`)
	grocWallet := addWalletCard(t, ctx, s, grocCat, "Groceries Card")

	catDining := prefix + "-Dining"
	catGroc := prefix + "-Groceries"
	catTravel := prefix + "-Travel" // no card → no event

	mustUpsertRec(t, ctx, s, period, catDining, &diningWallet, 4, "4% on dining", now)
	mustUpsertRec(t, ctx, s, period, catGroc, &grocWallet, 5, "5% on groceries", now)
	mustUpsertRec(t, ctx, s, period, catTravel, nil, 0, "no eligible card", now)

	// An open re-enrollment action (window covers `now`).
	if err := s.CreateSelection(ctx, &Selection{
		ID:             uuid.NewString(),
		UserCardID:     diningWallet,
		Category:       catDining,
		Tier:           ptrInt(1),
		PeriodLabel:    "2026-Q3",
		Enrolled:       false,
		EffectiveStart: ptrTime(dateUTC(2026, 7, 1)),
		EffectiveEnd:   ptrTime(dateUTC(2026, 9, 30)),
	}); err != nil {
		t.Fatalf("seed pending re-enrollment selection: %v", err)
	}

	fake := newFakeCalDAVClient()
	b := NewCardCalendarBridge(fake, s, true, "smackerel")
	b.now = func() time.Time { return now }

	res, err := b.SyncPeriod(ctx, period, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("SyncPeriod: %v", err)
	}

	// --- H06: the run is audited via card_runs run_type="calendar_sync" -------
	if res.Skipped {
		t.Fatal("SyncPeriod Skipped=true, want false (feature enabled)")
	}
	if res.RunID == "" {
		t.Fatal("SyncPeriod returned no RunID — the sync was not audited")
	}
	if res.EventsFailed != 0 {
		t.Fatalf("EventsFailed = %d, want 0 (all periods are valid months)", res.EventsFailed)
	}
	// My own rows: 2 carded recs + 1 re-enrollment = 3 events at minimum.
	if res.EventsWritten < 3 {
		t.Fatalf("EventsWritten = %d, want >= 3 (2 recommendations + 1 re-enrollment)", res.EventsWritten)
	}

	var runType, status string
	var eventsWritten int
	if err := s.Pool.QueryRow(ctx,
		`SELECT run_type, status, events_written FROM card_runs WHERE id = $1`, res.RunID,
	).Scan(&runType, &status, &eventsWritten); err != nil {
		t.Fatalf("read audited run %s: %v", res.RunID, err)
	}
	if runType != RunTypeCalendarSync {
		t.Fatalf("audited run_type = %q, want %q", runType, RunTypeCalendarSync)
	}
	if eventsWritten != res.EventsWritten {
		t.Fatalf("audited events_written = %d, want %d (must match the sync result)", eventsWritten, res.EventsWritten)
	}
	if status != RunStatusSuccess {
		t.Fatalf("audited status = %q, want %q", status, RunStatusSuccess)
	}
	if n, err := s.CountRunsByType(ctx, RunTypeCalendarSync); err != nil || n < 1 {
		t.Fatalf("CountRunsByType(calendar_sync) = %d (err=%v), want >= 1", n, err)
	}

	// --- calendar_event_uid is persisted for each synced recommendation -------
	wantDiningUID := b.RecommendationUID(period, catDining)
	wantGrocUID := b.RecommendationUID(period, catGroc)
	assertRecUID(t, ctx, s, period, catDining, &wantDiningUID)
	assertRecUID(t, ctx, s, period, catGroc, &wantGrocUID)
	assertRecUID(t, ctx, s, period, catTravel, nil) // un-carded rec → no event, no UID

	// The fake calendar server holds the two recommendation events and the
	// re-enrollment reminder under their stable UIDs.
	wantReenrollUID := b.ReEnrollmentUID(diningWallet, "2026-Q3")
	for _, uid := range []string{wantDiningUID, wantGrocUID, wantReenrollUID} {
		if _, ok := fake.events[uid]; !ok {
			t.Fatalf("expected calendar event under UID %q (have %v)", uid, fakeEventUIDs(fake.events))
		}
	}

	// --- re-sync UPDATES the same events (no duplicate), and writes a 2nd run -
	putAfterFirst := fake.putCalls
	res2, err := b.SyncPeriod(ctx, period, RunTriggerScheduled)
	if err != nil {
		t.Fatalf("SyncPeriod (re-sync): %v", err)
	}
	if fake.putCalls <= putAfterFirst {
		t.Fatalf("re-sync made no PutEvent calls (putCalls=%d)", fake.putCalls)
	}
	if _, ok := fake.events[wantDiningUID]; !ok {
		t.Fatalf("re-sync lost the dining event under %q", wantDiningUID)
	}
	if res2.RunID == res.RunID {
		t.Fatal("re-sync reused the first RunID — each sync must append its own audit run")
	}
	if n, err := s.CountRunsByType(ctx, RunTypeCalendarSync); err != nil || n < 2 {
		t.Fatalf("CountRunsByType(calendar_sync) after re-sync = %d (err=%v), want >= 2", n, err)
	}
	// The dining recommendation still resolves to the SAME UID (stable; no
	// duplicate row, no UID churn).
	assertRecUID(t, ctx, s, period, catDining, &wantDiningUID)
}

// mustUpsertRec seeds one card_recommendations row.
func mustUpsertRec(t *testing.T, ctx context.Context, s *Store, period, category string, cardID *string, rate float64, reason string, now time.Time) {
	t.Helper()
	if err := s.UpsertRecommendation(ctx, &CardRecommendation{
		ID:                    uuid.NewString(),
		PeriodLabel:           period,
		Category:              category,
		RecommendedUserCardID: cardID,
		Rate:                  rate,
		Reason:                reason,
		GeneratedAt:           now,
	}); err != nil {
		t.Fatalf("seed recommendation %s/%s: %v", period, category, err)
	}
}

// assertRecUID reads a recommendation back and asserts its calendar_event_uid
// equals want (nil want asserts the UID is unset).
func assertRecUID(t *testing.T, ctx context.Context, s *Store, period, category string, want *string) {
	t.Helper()
	rec, err := s.GetRecommendation(ctx, period, category)
	if err != nil || rec == nil {
		t.Fatalf("GetRecommendation(%s/%s): %v (rec=%v)", period, category, err, rec)
	}
	switch {
	case want == nil:
		if rec.CalendarEventUID != nil {
			t.Fatalf("recommendation %s/%s calendar_event_uid = %q, want unset", period, category, *rec.CalendarEventUID)
		}
	case rec.CalendarEventUID == nil:
		t.Fatalf("recommendation %s/%s calendar_event_uid unset, want %q", period, category, *want)
	case *rec.CalendarEventUID != *want:
		t.Fatalf("recommendation %s/%s calendar_event_uid = %q, want %q", period, category, *rec.CalendarEventUID, *want)
	}
}

// fakeEventUIDs returns the UID keys of a fake event map for diagnostics.
func fakeEventUIDs(m map[string]fakeCalDAVEvent) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
