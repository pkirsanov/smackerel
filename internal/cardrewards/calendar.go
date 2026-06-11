package cardrewards

// Card-rewards CalDAV calendar delivery (spec 083 Scope 08, design §7 /
// FR-CR-015, UC-005, Principle 8). Google Calendar is the operator's PRIMARY
// consumption surface for card-rewards (preserved from CCManager), so this
// bridge turns the persisted monthly recommendations and pending
// re-enrollment actions into stable CalDAV events.
//
// The implementation mirrors internal/mealplan/calendar.go: a CalDAVClient
// (PutEvent/DeleteEvent) is the EXTERNAL calendar-server boundary, and events
// carry STABLE UIDs so a re-sync UPDATES the same event rather than creating a
// duplicate (UC-005 A3). The bridge owns no model/network code of its own — a
// real CalDAVClient wraps the existing internal/connector/caldav client + its
// OAuth credentials (design §7); tests use an in-memory fake of that external
// boundary (NOT an internal-component mock).
//
// UID scheme (design §7):
//   - recommendation : <prefix>-cardrec-<period>-<category-slug>
//     e.g. smackerel-cardrec-2026-06-restaurants
//   - re-enrollment  : <prefix>-cardreenroll-<user_card_id>-<period>
//
// The configurable <prefix> is the namespace token from
// card_rewards.calendar_uid_prefix (SST, fail-loud, no default — Scope 01);
// the kind infix (cardrec / cardreenroll) is fixed by this contract so both
// schemes derive from the single configured prefix.

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CalDAVClient is the external calendar-server boundary the card-rewards bridge
// writes through. It is intentionally the SAME shape as
// internal/mealplan.CalDAVClient (PutEvent/DeleteEvent): design §7 calls for
// reusing that proven interface rather than inventing a new Google Calendar
// client. A real implementation wraps internal/connector/caldav (Google
// Calendar speaks CalDAV) with its OAuth credentials. PutEvent is an upsert
// keyed on uid — putting the same uid twice UPDATES the event, it does not
// duplicate it (that property is what makes re-sync idempotent).
type CalDAVClient interface {
	PutEvent(ctx context.Context, uid, summary, description string, start, end time.Time, categories []string, extraProps map[string]string) error
	DeleteEvent(ctx context.Context, uid string) error
}

const (
	// calendarCategoryTag tags every card-rewards calendar event so the
	// operator can filter/colour them in Google Calendar (design §7).
	calendarCategoryTag = "smackerel-cardrewards"

	// uidKindRecommendation / uidKindReEnrollment are the fixed kind infixes in
	// the stable UID scheme (design §7). They are deliberately constants, not
	// config, so that a re-sync always recomputes the identical UID.
	uidKindRecommendation = "cardrec"
	uidKindReEnrollment   = "cardreenroll"
)

// slugNonAlnum collapses any run of non-[a-z0-9] characters into a single
// hyphen for the category slug ("Gas Stations" → "gas-stations").
var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// CalendarSyncResult summarizes one calendar sync. Skipped is true when the
// feature flag card_rewards.calendar_sync is off (the bridge wrote nothing but
// the recommendation data remains in the store for the Web UI — SCN-083-H04).
type CalendarSyncResult struct {
	Period              string `json:"period"`
	Skipped             bool   `json:"skipped"`
	RecommendationsSeen int    `json:"recommendations_seen"`
	ReEnrollmentsSeen   int    `json:"reenrollments_seen"`
	EventsWritten       int    `json:"events_written"`
	EventsFailed        int    `json:"events_failed"`
	RunID               string `json:"run_id,omitempty"`
}

// RecommendationEvent pairs a recommendation with the display name of its
// recommended card (resolved from the wallet) so the calendar summary can read
// "<Category>: use <Card> (<rate>%)" without the bridge needing store access.
// SyncPeriod resolves the names from the store; unit tests construct these
// directly with a fake CalDAVClient.
type RecommendationEvent struct {
	Recommendation CardRecommendation
	CardName       string
}

// CardCalendarBridge manages the CalDAV event lifecycle for card-rewards
// recommendations and re-enrollment reminders. It mirrors
// mealplan.CalendarBridge. store may be nil for the pure input-taking methods
// (SyncRecommendations / SyncReEnrollments / DeleteRecommendationEvent) that
// the unit tests exercise; SyncPeriod requires a non-nil store.
type CardCalendarBridge struct {
	client    CalDAVClient
	store     *Store
	enabled   bool
	uidPrefix string
	now       func() time.Time
}

// NewCardCalendarBridge constructs a bridge. enabled comes from
// card_rewards.calendar_sync and uidPrefix from card_rewards.calendar_uid_prefix
// (both SST, Scope 01). store may be nil when only the input-taking methods are
// used.
func NewCardCalendarBridge(client CalDAVClient, store *Store, enabled bool, uidPrefix string) *CardCalendarBridge {
	return &CardCalendarBridge{
		client:    client,
		store:     store,
		enabled:   enabled,
		uidPrefix: uidPrefix,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// RecommendationUID returns the stable CalDAV UID for a per-period, per-category
// recommendation (design §7): <prefix>-cardrec-<period>-<category-slug>.
func (b *CardCalendarBridge) RecommendationUID(period, category string) string {
	return fmt.Sprintf("%s-%s-%s-%s", b.uidPrefix, uidKindRecommendation, period, slugCategory(category))
}

// ReEnrollmentUID returns the stable CalDAV UID for a re-enrollment reminder
// (design §7): <prefix>-cardreenroll-<user_card_id>-<period>.
func (b *CardCalendarBridge) ReEnrollmentUID(userCardID, period string) string {
	return fmt.Sprintf("%s-%s-%s-%s", b.uidPrefix, uidKindReEnrollment, userCardID, period)
}

// slugCategory lowercases a category and collapses non-alphanumeric runs into
// single hyphens for a stable, URL/UID-safe slug.
func slugCategory(category string) string {
	s := strings.ToLower(strings.TrimSpace(category))
	s = slugNonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// monthStart parses a YYYY-MM monthly period label and returns 09:00 UTC on the
// first of that month — the recommendation event's start. A non-month label is
// a fail-loud error (no silent fallback), surfaced per-event by the caller.
func monthStart(period string) (time.Time, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, fmt.Errorf("card calendar: recommendation period %q is not a YYYY-MM month label: %w", period, err)
	}
	return t.Add(9 * time.Hour), nil
}

// SyncRecommendations writes (creates/updates) a CalDAV event for each
// recommendation that names a recommended card. The returned map is keyed by
// recommendation id → the stable UID that was written (so SyncPeriod can persist
// calendar_event_uid). When the feature is disabled the bridge writes nothing,
// returns a Skipped result and a nil map, and does NOT touch the recommendation
// data — it stays visible in the Web UI (SCN-083-H04). A re-sync recomputes the
// identical UID, so the calendar event is UPDATED, not duplicated (SCN-083-H02).
func (b *CardCalendarBridge) SyncRecommendations(ctx context.Context, events []RecommendationEvent) (CalendarSyncResult, map[string]string, error) {
	res := CalendarSyncResult{RecommendationsSeen: len(events)}
	if !b.enabled {
		res.Skipped = true
		return res, nil, nil
	}

	written := make(map[string]string, len(events))
	for _, ev := range events {
		rec := ev.Recommendation
		if rec.RecommendedUserCardID == nil {
			// No card to recommend for this category this period — no event.
			continue
		}

		start, err := monthStart(rec.PeriodLabel)
		if err != nil {
			slog.Warn("card calendar: skipping recommendation with non-month period",
				"category", rec.Category, "period", rec.PeriodLabel, "error", err)
			res.EventsFailed++
			continue
		}
		end := start.Add(time.Hour)

		cardName := ev.CardName
		if cardName == "" {
			// Degrade to the stable card id (traceable), never a fabricated name.
			cardName = "card " + *rec.RecommendedUserCardID
		}

		uid := b.RecommendationUID(rec.PeriodLabel, rec.Category)
		summary := fmt.Sprintf("%s: use %s (%g%%)", rec.Category, cardName, rec.Rate)
		description := recommendationDescription(rec)
		props := map[string]string{
			"X-SMACKEREL-CARDREC-ID": rec.ID,
			"X-SMACKEREL-PERIOD":     rec.PeriodLabel,
		}

		if err := b.client.PutEvent(ctx, uid, summary, description, start, end, []string{calendarCategoryTag}, props); err != nil {
			slog.Warn("card calendar: put recommendation event failed", "uid", uid, "error", err)
			res.EventsFailed++
			continue
		}
		written[rec.ID] = uid
		res.EventsWritten++
	}
	return res, written, nil
}

// SyncReEnrollments writes (creates/updates) a CalDAV reminder event for each
// pending re-enrollment action whose enrollment window has a known start. It
// returns the number of events written. When the feature is disabled it writes
// nothing and returns 0 (SCN-083-H04).
func (b *CardCalendarBridge) SyncReEnrollments(ctx context.Context, pending []PendingReEnrollment) (int, error) {
	if !b.enabled {
		return 0, nil
	}

	written := 0
	for _, p := range pending {
		if p.EffectiveStart == nil {
			// No window start → nowhere to place the reminder.
			continue
		}
		start := truncateToDateUTC(*p.EffectiveStart).Add(9 * time.Hour)
		end := start.Add(time.Hour)

		uid := b.ReEnrollmentUID(p.UserCardID, p.PeriodLabel)
		summary := fmt.Sprintf("Re-enroll: %s — %s", p.CatalogName, p.Category)
		description := reEnrollmentDescription(p)
		props := map[string]string{
			"X-SMACKEREL-USER-CARD-ID": p.UserCardID,
			"X-SMACKEREL-PERIOD":       p.PeriodLabel,
		}

		if err := b.client.PutEvent(ctx, uid, summary, description, start, end, []string{calendarCategoryTag}, props); err != nil {
			slog.Warn("card calendar: put re-enrollment event failed", "uid", uid, "error", err)
			continue
		}
		written++
	}
	return written, nil
}

// DeleteRecommendationEvent removes the CalDAV event for a recommendation that
// is being deleted (SCN-083-H05). A recommendation with no stored UID is a
// no-op (nothing was ever synced). Delete is attempted regardless of the
// enabled flag so cleanup still works after the feature is turned off.
func (b *CardCalendarBridge) DeleteRecommendationEvent(ctx context.Context, rec CardRecommendation) error {
	if rec.CalendarEventUID == nil || *rec.CalendarEventUID == "" {
		return nil
	}
	if err := b.client.DeleteEvent(ctx, *rec.CalendarEventUID); err != nil {
		return fmt.Errorf("card calendar cleanup: delete event %q: %w", *rec.CalendarEventUID, err)
	}
	return nil
}

// SyncPeriod is the store-driven calendar sync used by the scheduler and manual
// triggers (Scope 09). It reads the period's recommendations and the open
// re-enrollment actions, writes/updates their CalDAV events, persists each
// recommendation's calendar_event_uid so a later re-sync updates the same
// event, and appends a card_runs audit row with run_type="calendar_sync"
// recording events_written (Principle 8, SCN-083-H06). When the feature is
// disabled it returns a Skipped result without writing events OR an audit run;
// the recommendation rows remain in the store for the Web UI (SCN-083-H04).
func (b *CardCalendarBridge) SyncPeriod(ctx context.Context, period, trigger string) (CalendarSyncResult, error) {
	if b.store == nil {
		return CalendarSyncResult{}, fmt.Errorf("card calendar: SyncPeriod requires a non-nil store")
	}
	if period == "" {
		period = b.now().Format("2006-01")
	}
	if trigger == "" {
		trigger = RunTriggerManual
	}

	res := CalendarSyncResult{Period: period}
	if !b.enabled {
		res.Skipped = true
		return res, nil
	}
	started := b.now()

	recs, err := b.store.ListRecommendationsByPeriod(ctx, period)
	if err != nil {
		return res, fmt.Errorf("list recommendations %s: %w", period, err)
	}
	res.RecommendationsSeen = len(recs)

	events := make([]RecommendationEvent, 0, len(recs))
	for i := range recs {
		ev := RecommendationEvent{Recommendation: recs[i]}
		if recs[i].RecommendedUserCardID != nil {
			uc, err := b.store.GetUserCard(ctx, *recs[i].RecommendedUserCardID)
			if err != nil {
				return res, fmt.Errorf("resolve recommended card %s: %w", *recs[i].RecommendedUserCardID, err)
			}
			if uc != nil {
				ev.CardName = recommendedCardName(uc)
			}
		}
		events = append(events, ev)
	}

	syncRes, writtenUIDs, err := b.SyncRecommendations(ctx, events)
	if err != nil {
		return res, err
	}
	res.EventsWritten += syncRes.EventsWritten
	res.EventsFailed += syncRes.EventsFailed

	// Persist calendar_event_uid for each successfully written recommendation so
	// the next sync updates the same event instead of duplicating it.
	for i := range recs {
		uid, ok := writtenUIDs[recs[i].ID]
		if !ok {
			continue
		}
		if recs[i].CalendarEventUID != nil && *recs[i].CalendarEventUID == uid {
			continue // already stored — skip a needless write
		}
		rec := recs[i]
		rec.CalendarEventUID = &uid
		if err := b.store.UpsertRecommendation(ctx, &rec); err != nil {
			return res, fmt.Errorf("persist calendar uid for recommendation %s: %w", rec.ID, err)
		}
	}

	pending, err := b.store.ListPendingReEnrollments(ctx, b.now())
	if err != nil {
		return res, fmt.Errorf("list pending re-enrollments: %w", err)
	}
	res.ReEnrollmentsSeen = len(pending)
	reWritten, err := b.SyncReEnrollments(ctx, pending)
	if err != nil {
		return res, err
	}
	res.EventsWritten += reWritten

	finished := b.now()
	status := RunStatusSuccess
	if res.EventsFailed > 0 {
		status = RunStatusPartial
	}
	res.RunID = uuid.NewString()
	run := &CardRun{
		ID:            res.RunID,
		RunType:       RunTypeCalendarSync,
		Trigger:       trigger,
		Status:        status,
		EventsWritten: res.EventsWritten,
		StartedAt:     &started,
		FinishedAt:    &finished,
	}
	if err := b.store.CreateRun(ctx, run); err != nil {
		return res, fmt.Errorf("write calendar_sync audit run: %w", err)
	}
	return res, nil
}

// recommendedCardName prefers the wallet nickname, falls back to the catalog
// name, and finally to the stable card id so the calendar summary always names
// a traceable card (never a fabricated default).
func recommendedCardName(uc *UserCard) string {
	if uc.Nickname != nil && strings.TrimSpace(*uc.Nickname) != "" {
		return *uc.Nickname
	}
	if strings.TrimSpace(uc.CatalogName) != "" {
		return uc.CatalogName
	}
	return "card " + uc.ID
}

// recommendationDescription builds the event body from the explainable reason
// plus the period/category context (Principle 8).
func recommendationDescription(rec CardRecommendation) string {
	var sb strings.Builder
	if strings.TrimSpace(rec.Reason) != "" {
		sb.WriteString(rec.Reason)
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "Period: %s\nCategory: %s", rec.PeriodLabel, rec.Category)
	if rec.Starred {
		sb.WriteString("\n★ starred category")
	}
	return strings.TrimSpace(sb.String())
}

// reEnrollmentDescription builds the re-enrollment reminder body.
func reEnrollmentDescription(p PendingReEnrollment) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Re-enroll %s in %q for %s.", p.CatalogName, p.Category, p.PeriodLabel)
	if p.Tier != nil {
		fmt.Fprintf(&sb, "\nTier: %d", *p.Tier)
	}
	if p.EffectiveEnd != nil {
		fmt.Fprintf(&sb, "\nWindow closes: %s", p.EffectiveEnd.Format("2006-01-02"))
	}
	return strings.TrimSpace(sb.String())
}

// truncateToDateUTC zeroes the clock portion of t in UTC (date at 00:00 UTC).
func truncateToDateUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
