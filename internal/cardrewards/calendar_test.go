package cardrewards

// Spec 083 Card Rewards Companion (Scope 08) — T-08-01 / T-08-02 / T-08-03.
// Unit tests for the CalDAV calendar bridge (calendar.go). No database — every
// scenario is decided against an in-memory FAKE of the EXTERNAL calendar-server
// boundary (CalDAVClient). That fake is an external-dependency fake (the
// calendar server), explicitly allowed by the test-integrity policy; it is NOT
// an internal-component mock (no Store, no business logic is mocked).
//
// SCN-083-H02 (re-sync updates the same UID, not a duplicate) is ADVERSARIAL:
// the fake keys events by UID exactly like a real CalDAV server, so a second
// PutEvent for the same recommendation UPDATES the one event. The test asserts
// PutEvent was called twice yet exactly one event exists with the UPDATED rate
// — it FAILS if the UID were not stable (e.g. carried a timestamp/uuid), which
// is the duplicate-event regression this scenario guards against.
//
// Reuses ptrStr / dateUTC / ptrTime / ptrInt from the unit-build test helpers
// (reconcile_test.go).

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeCalDAVEvent is one event recorded by the fake calendar server.
type fakeCalDAVEvent struct {
	uid         string
	summary     string
	description string
	start       time.Time
	end         time.Time
	categories  []string
	props       map[string]string
}

// fakeCalDAVClient is an in-memory fake of the EXTERNAL CalDAV calendar server.
// It keys events by UID — putting the same UID twice updates the single event
// (no duplicate), exactly like a real CalDAV server. It records call counts so
// tests can prove "called twice, one event" (the update-not-duplicate property).
type fakeCalDAVClient struct {
	events      map[string]fakeCalDAVEvent
	putCalls    int
	deleteCalls int
}

func newFakeCalDAVClient() *fakeCalDAVClient {
	return &fakeCalDAVClient{events: map[string]fakeCalDAVEvent{}}
}

func (f *fakeCalDAVClient) PutEvent(_ context.Context, uid, summary, description string, start, end time.Time, categories []string, props map[string]string) error {
	f.putCalls++
	f.events[uid] = fakeCalDAVEvent{
		uid:         uid,
		summary:     summary,
		description: description,
		start:       start,
		end:         end,
		categories:  categories,
		props:       props,
	}
	return nil
}

func (f *fakeCalDAVClient) DeleteEvent(_ context.Context, uid string) error {
	f.deleteCalls++
	delete(f.events, uid)
	return nil
}

func newUnitBridge(client CalDAVClient, enabled bool) *CardCalendarBridge {
	b := NewCardCalendarBridge(client, nil, enabled, "smackerel")
	b.now = func() time.Time { return dateUTC(2026, 6, 15) }
	return b
}

func recEvent(id, period, category string, cardID *string, cardName string, rate float64, reason string, starred bool) RecommendationEvent {
	return RecommendationEvent{
		Recommendation: CardRecommendation{
			ID:                    id,
			PeriodLabel:           period,
			Category:              category,
			RecommendedUserCardID: cardID,
			Rate:                  rate,
			Reason:                reason,
			Starred:               starred,
			GeneratedAt:           dateUTC(2026, 6, 1),
		},
		CardName: cardName,
	}
}

// TestCardCalendarBridge_UIDSchemes verifies the stable UID schemes and the
// category slug (design §7): recommendations and re-enrollments derive both
// schemes from the single configured prefix.
func TestCardCalendarBridge_UIDSchemes(t *testing.T) {
	b := newUnitBridge(newFakeCalDAVClient(), true)

	if got, want := b.RecommendationUID("2026-06", "Restaurants"), "smackerel-cardrec-2026-06-restaurants"; got != want {
		t.Fatalf("RecommendationUID = %q, want %q", got, want)
	}
	if got, want := b.RecommendationUID("2026-06", "Gas Stations"), "smackerel-cardrec-2026-06-gas-stations"; got != want {
		t.Fatalf("RecommendationUID(slug) = %q, want %q", got, want)
	}
	if got, want := b.ReEnrollmentUID("uc-42", "2026-Q3"), "smackerel-cardreenroll-uc-42-2026-Q3"; got != want {
		t.Fatalf("ReEnrollmentUID = %q, want %q", got, want)
	}
}

// SCN-083-H01 — a monthly recommendation creates a CalDAV event with the stable
// UID smackerel-cardrec-<period>-<category-slug>.
func TestCardCalendarBridge_RecommendationEventStableUID_H01(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, true)
	ctx := context.Background()

	ev := recEvent("rec-1", "2026-06", "Restaurants", ptrStr("uc-1"), "Amex Gold", 4, "4% on dining", false)

	res, written, err := b.SyncRecommendations(ctx, []RecommendationEvent{ev})
	if err != nil {
		t.Fatalf("SyncRecommendations: %v", err)
	}

	wantUID := "smackerel-cardrec-2026-06-restaurants"
	if res.Skipped {
		t.Fatal("result Skipped=true, want false (feature enabled)")
	}
	if res.EventsWritten != 1 || res.EventsFailed != 0 {
		t.Fatalf("result events written=%d failed=%d, want 1/0", res.EventsWritten, res.EventsFailed)
	}
	if got := written["rec-1"]; got != wantUID {
		t.Fatalf("written UID for rec-1 = %q, want %q", got, wantUID)
	}
	if fake.putCalls != 1 || len(fake.events) != 1 {
		t.Fatalf("fake calendar putCalls=%d events=%d, want 1/1", fake.putCalls, len(fake.events))
	}
	got, ok := fake.events[wantUID]
	if !ok {
		t.Fatalf("no event written under stable UID %q (have %v)", wantUID, fake.events)
	}
	if want := "Restaurants: use Amex Gold (4%)"; got.summary != want {
		t.Fatalf("event summary = %q, want %q", got.summary, want)
	}
	if len(got.categories) != 1 || got.categories[0] != "smackerel-cardrewards" {
		t.Fatalf("event categories = %v, want [smackerel-cardrewards]", got.categories)
	}
	if got.props["X-SMACKEREL-CARDREC-ID"] != "rec-1" || got.props["X-SMACKEREL-PERIOD"] != "2026-06" {
		t.Fatalf("event props = %v, want CARDREC-ID=rec-1 PERIOD=2026-06", got.props)
	}
	// Event is placed on the first of the period month at 09:00 UTC.
	if want := dateUTC(2026, 6, 1).Add(9 * time.Hour); !got.start.Equal(want) {
		t.Fatalf("event start = %s, want %s", got.start, want)
	}
}

// SCN-083-H02 — re-syncing after a rate change UPDATES the same event (same
// UID), it does NOT create a duplicate. ADVERSARIAL: a non-stable UID would
// leave two events; this asserts exactly one event with the UPDATED rate.
func TestCardCalendarBridge_ReSyncUpdatesSameUID_H02(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, true)
	ctx := context.Background()

	first := recEvent("rec-1", "2026-06", "Restaurants", ptrStr("uc-1"), "Amex Gold", 3, "3% on dining", false)
	if _, _, err := b.SyncRecommendations(ctx, []RecommendationEvent{first}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Rate changes from 3% to 5%; re-sync the same (period, category).
	second := recEvent("rec-1", "2026-06", "Restaurants", ptrStr("uc-1"), "Amex Gold", 5, "5% on dining (boosted)", false)
	if _, _, err := b.SyncRecommendations(ctx, []RecommendationEvent{second}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if fake.putCalls != 2 {
		t.Fatalf("putCalls = %d, want 2 (synced twice)", fake.putCalls)
	}
	if len(fake.events) != 1 {
		t.Fatalf("H02 REGRESSION: %d events after re-sync, want exactly 1 (update, not duplicate)", len(fake.events))
	}
	wantUID := "smackerel-cardrec-2026-06-restaurants"
	got, ok := fake.events[wantUID]
	if !ok {
		t.Fatalf("event not under stable UID %q (have %v)", wantUID, fake.events)
	}
	if want := "Restaurants: use Amex Gold (5%)"; got.summary != want {
		t.Fatalf("updated summary = %q, want %q (must reflect the new rate)", got.summary, want)
	}
	if !strings.Contains(got.description, "5% on dining (boosted)") {
		t.Fatalf("updated description = %q, want it to carry the new reason", got.description)
	}
}

// SCN-083-H03 — a pending re-enrollment action creates a re-enrollment reminder
// event with the stable UID smackerel-cardreenroll-<user_card_id>-<period>.
func TestCardCalendarBridge_ReEnrollmentEvent_H03(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, true)
	ctx := context.Background()

	pending := PendingReEnrollment{
		UserCardID:     "uc-9",
		CatalogName:    "US Bank Cash+",
		Category:       "Restaurants",
		Tier:           ptrInt(1),
		PeriodLabel:    "2026-Q3",
		EffectiveStart: ptrTime(dateUTC(2026, 7, 1)),
		EffectiveEnd:   ptrTime(dateUTC(2026, 9, 30)),
	}

	n, err := b.SyncReEnrollments(ctx, []PendingReEnrollment{pending})
	if err != nil {
		t.Fatalf("SyncReEnrollments: %v", err)
	}
	if n != 1 {
		t.Fatalf("re-enrollments written = %d, want 1", n)
	}
	wantUID := "smackerel-cardreenroll-uc-9-2026-Q3"
	got, ok := fake.events[wantUID]
	if !ok {
		t.Fatalf("no re-enrollment event under stable UID %q (have %v)", wantUID, fake.events)
	}
	if want := "Re-enroll: US Bank Cash+ — Restaurants"; got.summary != want {
		t.Fatalf("re-enrollment summary = %q, want %q", got.summary, want)
	}
	if got.props["X-SMACKEREL-USER-CARD-ID"] != "uc-9" {
		t.Fatalf("re-enrollment props = %v, want USER-CARD-ID=uc-9", got.props)
	}
	if want := dateUTC(2026, 7, 1).Add(9 * time.Hour); !got.start.Equal(want) {
		t.Fatalf("re-enrollment start = %s, want %s (window open date 09:00)", got.start, want)
	}
}

// SCN-083-H04 — when card_rewards.calendar_sync is disabled the bridge writes
// NO CalDAV events (recommendations OR re-enrollments) but the recommendation
// data is left untouched, so it remains visible in the Web UI.
func TestCardCalendarBridge_DisabledSyncSkipsWritesKeepsData_H04(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, false) // calendar_sync disabled
	ctx := context.Background()

	// This slice models the recommendation data the Web UI reads.
	uiData := []RecommendationEvent{
		recEvent("rec-1", "2026-06", "Restaurants", ptrStr("uc-1"), "Amex Gold", 4, "4% on dining", true),
		recEvent("rec-2", "2026-06", "Groceries", ptrStr("uc-2"), "Citi Custom Cash", 5, "5% on groceries", false),
	}

	res, written, err := b.SyncRecommendations(ctx, uiData)
	if err != nil {
		t.Fatalf("SyncRecommendations (disabled): %v", err)
	}
	if !res.Skipped {
		t.Fatal("result Skipped=false, want true (calendar_sync disabled)")
	}
	if res.EventsWritten != 0 {
		t.Fatalf("EventsWritten = %d, want 0 when disabled", res.EventsWritten)
	}
	if written != nil {
		t.Fatalf("written UID map = %v, want nil when disabled", written)
	}
	if fake.putCalls != 0 || len(fake.events) != 0 {
		t.Fatalf("NO calendar write expected when disabled: putCalls=%d events=%d", fake.putCalls, len(fake.events))
	}

	// Re-enrollment sync is likewise a no-op when disabled.
	n, err := b.SyncReEnrollments(ctx, []PendingReEnrollment{{
		UserCardID: "uc-1", CatalogName: "Amex Gold", Category: "Restaurants",
		PeriodLabel: "2026-Q3", EffectiveStart: ptrTime(dateUTC(2026, 7, 1)),
	}})
	if err != nil {
		t.Fatalf("SyncReEnrollments (disabled): %v", err)
	}
	if n != 0 || fake.putCalls != 0 {
		t.Fatalf("re-enrollment writes when disabled: n=%d putCalls=%d, want 0/0", n, fake.putCalls)
	}

	// The recommendation data is intact (still visible to the Web UI): the
	// bridge neither deleted nor mutated the rows it was asked to (not) sync.
	if len(uiData) != 2 {
		t.Fatalf("UI recommendation data len = %d, want 2 (unchanged)", len(uiData))
	}
	if uiData[0].Recommendation.ID != "rec-1" || uiData[0].Recommendation.Rate != 4 ||
		uiData[1].Recommendation.ID != "rec-2" || uiData[1].Recommendation.Rate != 5 {
		t.Fatalf("UI recommendation data mutated by disabled sync: %+v", uiData)
	}
}

// SCN-083-H05 — deleting a recommendation cleans up its CalDAV event. A
// recommendation carrying a calendar_event_uid has that event removed; one with
// no UID is a safe no-op.
func TestCardCalendarBridge_DeleteCleansUpEvent_H05(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, true)
	ctx := context.Background()

	// Sync first so there is an event to clean up.
	ev := recEvent("rec-1", "2026-06", "Restaurants", ptrStr("uc-1"), "Amex Gold", 4, "4% on dining", false)
	_, written, err := b.SyncRecommendations(ctx, []RecommendationEvent{ev})
	if err != nil {
		t.Fatalf("seed sync: %v", err)
	}
	uid := written["rec-1"]
	if _, ok := fake.events[uid]; !ok || len(fake.events) != 1 {
		t.Fatalf("precondition: expected 1 event under %q before delete (have %v)", uid, fake.events)
	}

	// Delete the recommendation → its event is removed.
	deleted := ev.Recommendation
	deleted.CalendarEventUID = &uid
	if err := b.DeleteRecommendationEvent(ctx, deleted); err != nil {
		t.Fatalf("DeleteRecommendationEvent: %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", fake.deleteCalls)
	}
	if _, ok := fake.events[uid]; ok || len(fake.events) != 0 {
		t.Fatalf("H05: event %q not cleaned up (have %v)", uid, fake.events)
	}

	// A recommendation with no stored UID is a safe no-op (nothing was synced).
	noUID := recEvent("rec-2", "2026-06", "Groceries", ptrStr("uc-2"), "Citi", 5, "5%", false).Recommendation
	if err := b.DeleteRecommendationEvent(ctx, noUID); err != nil {
		t.Fatalf("DeleteRecommendationEvent(no uid): %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d after no-op delete, want still 1", fake.deleteCalls)
	}
}

// TestCardCalendarBridge_NonMonthPeriodCountsFailed proves the fail-loud
// per-event handling: a recommendation whose period is not a YYYY-MM month is
// skipped and counted as failed (no event, no silent fallback to "now").
func TestCardCalendarBridge_NonMonthPeriodCountsFailed(t *testing.T) {
	fake := newFakeCalDAVClient()
	b := newUnitBridge(fake, true)
	ctx := context.Background()

	bad := recEvent("rec-x", "Q3_2026", "Restaurants", ptrStr("uc-1"), "Amex Gold", 4, "4%", false)
	res, written, err := b.SyncRecommendations(ctx, []RecommendationEvent{bad})
	if err != nil {
		t.Fatalf("SyncRecommendations: %v", err)
	}
	if res.EventsWritten != 0 || res.EventsFailed != 1 {
		t.Fatalf("non-month period: written=%d failed=%d, want 0/1", res.EventsWritten, res.EventsFailed)
	}
	if len(written) != 0 || len(fake.events) != 0 {
		t.Fatalf("non-month period must write no event (written=%v events=%v)", written, fake.events)
	}
}
