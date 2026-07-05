package cardrewards

// Card-rewards multi-source reconciliation & category lifecycle (spec 083
// Scope 06, design §5 / FR-CR-009, FR-CR-011, FR-CR-012, Principle 3 —
// "Knowledge Breathes").
//
// This file merges the per-source rotating-category observations produced by
// the Scope 05 extractor into the single authoritative rotating_categories
// record per (card_catalog_id, period_label), advances each record's
// lifecycle_state by date, and surfaces pending selectable-card re-enrollment
// actions for the dashboard. It is the trustworthy companion to the extractor:
// where the extractor refuses to store unvalidated data, the reconciler refuses
// to (a) overwrite a manual override and (b) silently serve stale/low-confidence
// or disagreeing data — it flags needs_verification instead (the CCManager
// silent-fallback failure mode this feature replaces, FR-CR-010).
//
// The merge and lifecycle decisions are PURE functions (mergeObservations,
// deriveLifecycle) of their inputs so every scenario SCN-083-F01..F05 is
// unit-testable with no database and no mocks. Reconcile / AdvanceLifecycle
// wire those decisions to a real Store for the live-PostgreSQL paths
// (SCN-083-F06/F07).

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Reconciler merges observations into the authoritative rotating-category
// record, advances lifecycle by date, and reports pending re-enrollments. It
// owns no model/network access — only the Store and a clock. threshold is
// card_rewards.extraction.confidence_threshold (SST, fail-loud at config load;
// passed in by the caller — this package introduces no config default).
type Reconciler struct {
	store     *Store
	threshold float64
	now       func() time.Time
	logger    *slog.Logger
}

// NewReconciler constructs a Reconciler. A nil logger is replaced with
// slog.Default so call sites need not guard. The clock defaults to
// time.Now().UTC(); tests in this package may override the unexported now field
// for deterministic date-driven lifecycle assertions.
func NewReconciler(store *Store, threshold float64, logger *slog.Logger) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		store:     store,
		threshold: threshold,
		now:       func() time.Time { return time.Now().UTC() },
		logger:    logger,
	}
}

// MergeOutcome is the pure verdict of reconciling one (card, period)'s
// observations against any existing record. Record is the record to upsert
// (or, when OverrideProtected, the existing record left untouched); it is nil
// only when there is neither an existing record nor any observation. The
// orchestrator stamps a fresh ID when Record.ID is empty.
type MergeOutcome struct {
	Record            *RotatingCategory
	OverrideProtected bool
	Disagreement      bool
}

// mergeObservations applies the design §5 reconciliation contract to one
// (card, period)'s observations. PURE — a function of its inputs only.
//
//   - F03 (FR-CR-011): if an existing record carries manual_override, it is
//     returned UNCHANGED — categories, confidence, and needs_verification are
//     never touched by an extraction; the observations remain in the store for
//     audit only.
//   - F01: when every source agrees on the category set, the agreed set is
//     adopted, source_count = number of sources, confidence = the highest
//     source confidence, and needs_verification is false (unless the aggregate
//     confidence is below threshold, UC-002 A2).
//   - F02 (FR-CR-009/010): when sources disagree, needs_verification is forced
//     true, the conservative (lowest) confidence is used (UC-002 A3), and
//     source_count reflects only the agreeing plurality — both observations
//     remain persisted for audit.
//
// The chosen category set is the plurality group; ties are broken by highest
// in-group confidence, then lexicographically, so the result is deterministic.
func mergeObservations(existing *RotatingCategory, obs []RotatingCategoryObservation, threshold float64, now time.Time) MergeOutcome {
	// (F03) Manual override is authoritative — never overwritten by extraction.
	if existing != nil && existing.ManualOverride {
		return MergeOutcome{Record: existing, OverrideProtected: true}
	}

	if len(obs) == 0 {
		// No new observations: keep whatever exists (may be nil).
		return MergeOutcome{Record: existing}
	}

	type group struct {
		members []RotatingCategoryObservation
		maxConf float64
	}
	groups := map[string]*group{}
	for _, o := range obs {
		sig := categorySignature(o.Categories)
		g, ok := groups[sig]
		if !ok {
			g = &group{}
			groups[sig] = g
		}
		g.members = append(g.members, o)
		if o.Confidence > g.maxConf {
			g.maxConf = o.Confidence
		}
	}

	// Deterministic plurality selection: most members, then highest in-group
	// confidence, then lexicographically smallest signature.
	sigs := make([]string, 0, len(groups))
	for sig := range groups {
		sigs = append(sigs, sig)
	}
	sort.Strings(sigs)
	var chosen *group
	for _, sig := range sigs {
		g := groups[sig]
		switch {
		case chosen == nil:
			chosen = g
		case len(g.members) > len(chosen.members):
			chosen = g
		case len(g.members) == len(chosen.members) && g.maxConf > chosen.maxConf:
			chosen = g
		}
	}

	disagreement := len(groups) > 1

	// Representative = the highest-confidence member of the chosen group; it
	// carries the period dates / spend limit / activation flag for the record.
	rep := chosen.members[0]
	for _, m := range chosen.members[1:] {
		if m.Confidence > rep.Confidence {
			rep = m
		}
	}

	var (
		confidence  float64
		needsVerify bool
		sourceCount int
	)
	if !disagreement {
		confidence = chosen.maxConf
		// Confidence-based flagging requires a threshold in the valid (0,1] range
		// — the same contract Service.Reconcile enforces. When the threshold is
		// out of range (e.g. the card_rewards feature is DISABLED and the SST
		// placeholder is a degenerate 0.0, as in the disposable e2e-ui stack),
		// the reconciler cannot classify confidence and MUST NOT downgrade a
		// needs_verification flag a valid-threshold pass already set — it
		// preserves the existing flag rather than silently clearing it. Without
		// this guard a parallel pipeline RunDailyRefresh reconciling every row at
		// 0.0 clears each single-source low-confidence flag (`confidence < 0.0`
		// is always false), the SCN-083-K01 cross-reconcile pollution. A
		// brand-new record with no prior flag stays unflagged. Disagreement-based
		// flags are threshold-independent (handled in the else branch below).
		switch {
		case threshold > 0 && threshold <= 1:
			needsVerify = confidence < threshold // (UC-002 A2) low confidence still flags
		case existing != nil:
			needsVerify = existing.NeedsVerification // threshold unusable — preserve prior verdict
		}
		sourceCount = len(obs)
	} else {
		confidence = obs[0].Confidence
		for _, o := range obs[1:] {
			if o.Confidence < confidence { // (UC-002 A3) conservative lower confidence
				confidence = o.Confidence
			}
		}
		needsVerify = true
		sourceCount = len(chosen.members)
	}

	// Lifecycle is derived from the period window (date-driven path of the
	// shared deriveLifecycle); an undated observation is treated as active now.
	state, ok := deriveLifecycle("", rep.PeriodStart, rep.PeriodEnd, now)
	if !ok {
		state = LifecycleActive
	}
	rec := &RotatingCategory{
		CardCatalogID:      rep.CardCatalogID,
		PeriodLabel:        rep.PeriodLabel,
		PeriodStart:        rep.PeriodStart,
		PeriodEnd:          rep.PeriodEnd,
		Categories:         dedupeSorted(rep.Categories),
		LimitCents:         rep.LimitCents,
		ActivationRequired: rep.ActivationRequired != nil && *rep.ActivationRequired,
		LifecycleState:     state,
		Confidence:         confidence,
		NeedsVerification:  needsVerify,
		ManualOverride:     false,
		SourceCount:        sourceCount,
	}
	if existing != nil {
		rec.ID = existing.ID // preserve identity so the upsert stays one row (F07)
	}
	return MergeOutcome{Record: rec, Disagreement: disagreement}
}

// categorySignature normalizes a category set to an order- and case-insensitive
// signature so two sources that list the same categories in different order or
// casing count as agreement.
func categorySignature(cats []string) string {
	norm := make([]string, 0, len(cats))
	for _, c := range cats {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" {
			norm = append(norm, c)
		}
	}
	sort.Strings(norm)
	return strings.Join(norm, "\x1f")
}

// dedupeSorted returns the trimmed, de-duplicated category set in stable sorted
// order (case-insensitive de-duplication, first original casing wins).
func dedupeSorted(cats []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ReconcileResult summarizes one reconciliation pass for callers/tests.
type ReconcileResult struct {
	Run                *CardRun
	Reconciled         int // records upserted (override-protected refs excluded)
	OverridesProtected int
	Disagreements      int
	NeedsVerification  int
}

// Reconcile merges the observations for each (card, period) ref into its
// authoritative rotating_categories record and emits one card_runs reconcile
// audit row (Principle 8). It is idempotent: rerunning on the same observations
// upserts the same single row per (card, period) (SCN-083-F07). Refs whose
// record carries a manual override are left untouched (SCN-083-F03).
func (r *Reconciler) Reconcile(ctx context.Context, refs []CardPeriodRef, trigger string) (*ReconcileResult, error) {
	if !ValidRunTrigger(trigger) {
		return nil, fmt.Errorf("cardrewards: invalid run trigger %q", trigger)
	}
	started := time.Now().UTC()
	now := r.now()
	res := &ReconcileResult{}
	totalCategories := 0

	for _, ref := range refs {
		obs, err := r.store.ListObservationsByCardPeriod(ctx, ref.CardCatalogID, ref.PeriodLabel)
		if err != nil {
			return nil, fmt.Errorf("cardrewards: list observations for %s/%s: %w", ref.CardCatalogID, ref.PeriodLabel, err)
		}
		existing, err := r.store.GetRotatingCategory(ctx, ref.CardCatalogID, ref.PeriodLabel)
		if err != nil {
			return nil, fmt.Errorf("cardrewards: get rotating category for %s/%s: %w", ref.CardCatalogID, ref.PeriodLabel, err)
		}

		out := mergeObservations(existing, obs, r.threshold, now)
		if out.Record == nil {
			continue // nothing to reconcile (no observations, no existing record)
		}
		if out.OverrideProtected {
			res.OverridesProtected++
			r.logger.Info("card-rewards reconcile: manual override protected — record unchanged, observations retained for audit",
				"card_id", ref.CardCatalogID, "period", ref.PeriodLabel)
			continue
		}

		if out.Record.ID == "" {
			out.Record.ID = uuid.NewString()
		}
		if err := r.store.UpsertRotatingCategory(ctx, out.Record); err != nil {
			return nil, fmt.Errorf("cardrewards: upsert rotating category for %s/%s: %w", ref.CardCatalogID, ref.PeriodLabel, err)
		}
		res.Reconciled++
		totalCategories += len(out.Record.Categories)
		if out.Record.NeedsVerification {
			res.NeedsVerification++
		}
		if out.Disagreement {
			res.Disagreements++
			r.logger.Warn("card-rewards reconcile: sources disagree — flagged needs_verification, both observations retained",
				"card_id", ref.CardCatalogID, "period", ref.PeriodLabel)
		}
	}

	finished := time.Now().UTC()
	run := &CardRun{
		ID:                  uuid.NewString(),
		RunType:             RunTypeReconcile,
		Trigger:             trigger,
		Status:              RunStatusSuccess,
		SourcesAttempted:    len(refs),
		SourcesSucceeded:    res.Reconciled,
		CategoriesExtracted: totalCategories,
		StartedAt:           &started,
		FinishedAt:          &finished,
	}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("cardrewards: create reconcile run: %w", err)
	}
	res.Run = run
	return res, nil
}

// LifecycleResult summarizes one daily lifecycle pass for callers/tests.
type LifecycleResult struct {
	Run                  *CardRun
	Scanned              int
	Transitioned         int
	PendingReEnrollments []PendingReEnrollment
}

// AdvanceLifecycle advances every rotating-category record's lifecycle_state by
// date (upcoming → active → expired, FR-CR-012 / Principle 3 "Knowledge
// Breathes"), surfaces selectable-card re-enrollment windows that have opened
// (SCN-083-F06), and emits one card_runs reconcile audit row. Expired records
// are excluded from current recommendations by lifecycle_state (SCN-083-F05);
// callers select current categories via Store.ListActiveRotatingCategories.
func (r *Reconciler) AdvanceLifecycle(ctx context.Context, trigger string) (*LifecycleResult, error) {
	if !ValidRunTrigger(trigger) {
		return nil, fmt.Errorf("cardrewards: invalid run trigger %q", trigger)
	}
	started := time.Now().UTC()
	now := r.now()

	all, err := r.store.ListAllRotatingCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("cardrewards: list rotating categories: %w", err)
	}
	res := &LifecycleResult{Scanned: len(all)}
	for _, rc := range all {
		desired, ok := deriveLifecycle("", rc.PeriodStart, rc.PeriodEnd, now)
		if !ok || desired == rc.LifecycleState {
			continue
		}
		ok, err := r.store.UpdateRotatingLifecycle(ctx, rc.ID, desired)
		if err != nil {
			return nil, fmt.Errorf("cardrewards: update lifecycle for %s/%s: %w", rc.CardCatalogID, rc.PeriodLabel, err)
		}
		if ok {
			res.Transitioned++
			r.logger.Info("card-rewards lifecycle transition",
				"card_id", rc.CardCatalogID, "period", rc.PeriodLabel, "from", rc.LifecycleState, "to", desired)
		}
	}

	pending, err := r.store.ListPendingReEnrollments(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("cardrewards: list pending re-enrollments: %w", err)
	}
	res.PendingReEnrollments = pending
	if len(pending) > 0 {
		r.logger.Info("card-rewards lifecycle: pending re-enrollment actions surfaced for dashboard", "count", len(pending))
	}

	finished := time.Now().UTC()
	run := &CardRun{
		ID:               uuid.NewString(),
		RunType:          RunTypeReconcile,
		Trigger:          trigger,
		Status:           RunStatusSuccess,
		SourcesAttempted: len(all),
		SourcesSucceeded: len(all),
		EventsWritten:    res.Transitioned,
		StartedAt:        &started,
		FinishedAt:       &finished,
	}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("cardrewards: create lifecycle run: %w", err)
	}
	res.Run = run
	return res, nil
}
