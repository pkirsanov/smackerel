package cardrewards

// Spec 083 Scope 11 — service methods backing the web Dashboard, Recommendations,
// Rotating-Verify, Report, and Admin pages (design §9 / FR-CR-016, FR-CR-019,
// Principle 8). These extend the Scope 02/07 Service with the read + mutation
// seams those pages need:
//
//   - rotating-category listing, source-citation lookup, and manual
//     verify/override (SCN-083-K04/K05),
//   - per-source observation seeding + reconcile (the rotating-verify e2e seam
//     that produces a needs_verification record from disagreeing observations
//     and proves a manual override is never overwritten — FR-CR-009..012),
//   - recommendation upsert + starred-override (SCN-083-K02/K03),
//   - audit run-history listing for the admin page (SCN-083-K07/K08),
//   - pending re-enrollment surfacing for the dashboard (SCN-083-K01).
//
// All validation is fail-loud; no method introduces a config default
// (smackerel-no-defaults). Persistence stays in the Store.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---- rotating-category verification (web rotating-verify page) -------------

// ListRotatingCategories returns every reconciled rotating-category record
// (all lifecycle states) ordered by (card, period) for the rotating-verify
// page (SCN-083-K04).
func (s *Service) ListRotatingCategories(ctx context.Context) ([]RotatingCategory, error) {
	return s.store.ListAllRotatingCategories(ctx)
}

// ListActiveRotatingCategories returns only the records currently in the
// active lifecycle state — the dashboard's "current active rotating
// categories" panel (SCN-083-K01).
func (s *Service) ListActiveRotatingCategories(ctx context.Context) ([]RotatingCategory, error) {
	return s.store.ListActiveRotatingCategories(ctx)
}

// GetRotatingCategoryByID returns one rotating-category record or
// ErrRotatingNotFound.
func (s *Service) GetRotatingCategoryByID(ctx context.Context, id string) (*RotatingCategory, error) {
	rc, err := s.store.GetRotatingCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rc == nil {
		return nil, fmt.Errorf("%w: %s", ErrRotatingNotFound, id)
	}
	return rc, nil
}

// ListObservations returns the per-source observations behind a reconciled
// rotating-category record — the source citations the rotating-verify page
// shows (Principle 4 / SCN-083-K04).
func (s *Service) ListObservations(ctx context.Context, catalogID, period string) ([]RotatingCategoryObservation, error) {
	return s.store.ListObservationsByCardPeriod(ctx, catalogID, period)
}

// VerifyRotatingCategory applies a manual verify/override to a reconciled
// rotating-category record (SCN-083-K05): it stores the operator-confirmed
// category set, marks the record manual_override=true with full confidence,
// and clears needs_verification. Because manual_override is now set, the Scope
// 06 reconciler refuses to overwrite it on any future extraction (FR-CR-011) —
// the adversarial property this scope proves end-to-end.
func (s *Service) VerifyRotatingCategory(ctx context.Context, id string, categories []string) (*RotatingCategory, error) {
	cleaned := make([]string, 0, len(categories))
	for _, c := range categories {
		if c = strings.TrimSpace(c); c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if len(cleaned) == 0 {
		return nil, validationErr("at least one category is required to verify a rotating record")
	}
	rc, err := s.store.GetRotatingCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rc == nil {
		return nil, fmt.Errorf("%w: %s", ErrRotatingNotFound, id)
	}
	rc.Categories = cleaned
	rc.ManualOverride = true
	rc.NeedsVerification = false
	rc.Confidence = 1
	if rc.SourceCount < 1 {
		rc.SourceCount = 1
	}
	if err := s.store.UpsertRotatingCategory(ctx, rc); err != nil {
		return nil, err
	}
	return s.store.GetRotatingCategoryByID(ctx, id)
}

// CreateObservation persists a single per-source rotating-category observation
// under a fresh extract audit run (Principle 8). It is the seam the rotating
// e2e-ui test uses to seed the source observations the reconciler then merges
// (and the source citations the verify page shows). Validation mirrors the
// strict-schema extractor: a known card_catalog_id, a period_label, at least
// one category, a confidence in (0,1], and a source name + url are required.
func (s *Service) CreateObservation(ctx context.Context, obs RotatingCategoryObservation) (*RotatingCategoryObservation, error) {
	obs.CardCatalogID = strings.TrimSpace(obs.CardCatalogID)
	obs.PeriodLabel = strings.TrimSpace(obs.PeriodLabel)
	obs.SourceName = strings.TrimSpace(obs.SourceName)
	obs.SourceURL = strings.TrimSpace(obs.SourceURL)
	cleaned := make([]string, 0, len(obs.Categories))
	for _, c := range obs.Categories {
		if c = strings.TrimSpace(c); c != "" {
			cleaned = append(cleaned, c)
		}
	}
	obs.Categories = cleaned
	switch {
	case obs.CardCatalogID == "":
		return nil, validationErr("observation card_catalog_id is required")
	case obs.PeriodLabel == "":
		return nil, validationErr("observation period_label is required")
	case len(obs.Categories) == 0:
		return nil, validationErr("observation requires at least one category")
	case obs.Confidence <= 0 || obs.Confidence > 1:
		return nil, validationErr("observation confidence must be in (0,1], got %v", obs.Confidence)
	case obs.SourceName == "":
		return nil, validationErr("observation source_name is required")
	case obs.SourceURL == "":
		return nil, validationErr("observation source_url is required")
	}
	cat, err := s.store.GetCatalogCard(ctx, obs.CardCatalogID)
	if err != nil {
		return nil, err
	}
	if cat == nil {
		return nil, fmt.Errorf("%w: %s", ErrCatalogNotFound, obs.CardCatalogID)
	}
	now := time.Now().UTC()
	obs.ID = uuid.NewString()
	obs.ObservedAt = now
	run := &CardRun{
		ID:                  uuid.NewString(),
		RunType:             RunTypeExtract,
		Trigger:             RunTriggerManual,
		Status:              RunStatusSuccess,
		SourcesAttempted:    1,
		SourcesSucceeded:    1,
		CategoriesExtracted: len(obs.Categories),
		StartedAt:           &now,
		FinishedAt:          &now,
	}
	obs.ExtractionRunID = run.ID
	if _, err := s.store.PersistExtractionRun(ctx, run, []RotatingCategoryObservation{obs}, nil); err != nil {
		return nil, err
	}
	return &obs, nil
}

// Reconcile merges every stored per-source observation into its authoritative
// rotating_categories record using the Scope 06 reconciler: it forces
// needs_verification on disagreement/low-confidence and refuses to overwrite
// any manual_override record (FR-CR-009..012). threshold is
// card_rewards.extraction.confidence_threshold; it is a REQUIRED, validated
// input — this method introduces no config default (smackerel-no-defaults).
// trigger MUST be scheduled|manual (defaults to manual only when empty, which
// is an explicit caller convenience, not a config fallback). This is the
// operator/seed seam the rotating-verify e2e-ui test uses to (a) produce a
// needs_verification record from disagreeing observations and (b) prove a
// manual override is not overwritten by a later reconcile (SCN-083-K04/K05).
func (s *Service) Reconcile(ctx context.Context, threshold float64, trigger string) (*ReconcileResult, error) {
	if threshold <= 0 || threshold > 1 {
		return nil, validationErr("reconcile threshold must be in (0,1], got %v", threshold)
	}
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		trigger = RunTriggerManual
	}
	if !ValidRunTrigger(trigger) {
		return nil, validationErr("reconcile trigger must be scheduled|manual, got %q", trigger)
	}
	refs, err := s.store.ListObservationRefs(ctx)
	if err != nil {
		return nil, err
	}
	reconciler := NewReconciler(s.store, threshold, nil)
	return reconciler.Reconcile(ctx, refs, trigger)
}

// ---- audit run history (web admin page) ------------------------------------

// ListRuns returns the most recent audit runs for the admin run-history page
// (SCN-083-K07/K08).
func (s *Service) ListRuns(ctx context.Context, limit int) ([]CardRun, error) {
	return s.store.ListRuns(ctx, limit)
}

// ---- dashboard pending actions ---------------------------------------------

// ListPendingReEnrollments surfaces selectable-card re-enrollment actions whose
// window has opened but the user has not yet enrolled — the dashboard's pending
// re-enrollment alerts (SCN-083-K01 / UC-003 A2).
func (s *Service) ListPendingReEnrollments(ctx context.Context) ([]PendingReEnrollment, error) {
	return s.store.ListPendingReEnrollments(ctx, time.Now().UTC())
}

// ---- recommendation mutation (web recommendations page) --------------------

// UpsertRecommendation creates or updates a per-period, per-category
// recommendation from the web recommendations page (SCN-083-K02 add/edit). It
// is idempotent on (period_label, category): a fresh row gets a generated id +
// generated_at; an existing row preserves its id (and its generated_at unless a
// new one is supplied). An empty period resolves to the current monthly period.
func (s *Service) UpsertRecommendation(ctx context.Context, rec CardRecommendation) (*CardRecommendation, error) {
	rec.PeriodLabel = strings.TrimSpace(rec.PeriodLabel)
	rec.Category = strings.TrimSpace(rec.Category)
	rec.Reason = strings.TrimSpace(rec.Reason)
	if rec.PeriodLabel == "" {
		rec.PeriodLabel = s.recommender.CurrentPeriod()
	}
	if rec.Category == "" {
		return nil, validationErr("recommendation category is required")
	}
	if rec.RecommendedUserCardID != nil {
		if err := s.ensureUserCardExists(ctx, rec.RecommendedUserCardID); err != nil {
			return nil, err
		}
	}
	existing, err := s.store.GetRecommendation(ctx, rec.PeriodLabel, rec.Category)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		rec.ID = existing.ID
		if rec.GeneratedAt.IsZero() {
			rec.GeneratedAt = existing.GeneratedAt
		}
	} else {
		rec.ID = uuid.NewString()
	}
	if rec.GeneratedAt.IsZero() {
		rec.GeneratedAt = time.Now().UTC()
	}
	if err := s.store.UpsertRecommendation(ctx, &rec); err != nil {
		return nil, err
	}
	return s.store.GetRecommendation(ctx, rec.PeriodLabel, rec.Category)
}

// StarRecommendation sets (or clears) the starred manual override on an
// existing recommendation (SCN-083-K02 star). Starring marks
// starred_override=true so the Scope 07 regenerate preserves the operator's
// pick over the optimizer's (SCN-083-K03); un-starring clears both flags so the
// next regenerate re-optimizes the category. An empty period resolves to the
// current monthly period.
func (s *Service) StarRecommendation(ctx context.Context, period, category string, starred bool) (*CardRecommendation, error) {
	period = strings.TrimSpace(period)
	category = strings.TrimSpace(category)
	if period == "" {
		period = s.recommender.CurrentPeriod()
	}
	if category == "" {
		return nil, validationErr("recommendation category is required")
	}
	rec, err := s.store.GetRecommendation(ctx, period, category)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrRecommendationNotFound, period, category)
	}
	rec.Starred = starred
	rec.StarredOverride = starred
	if err := s.store.UpsertRecommendation(ctx, rec); err != nil {
		return nil, err
	}
	return s.store.GetRecommendation(ctx, period, category)
}

// CurrentPeriod exposes the recommender's current monthly period label (e.g.
// "2026-06") to the web layer so the recommendations/report/dashboard pages can
// default to it without re-deriving the clock.
func (s *Service) CurrentPeriod() string {
	return s.recommender.CurrentPeriod()
}
