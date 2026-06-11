package cardrewards

// Card-rewards monthly recommendation generation (spec 083 Scope 07, design §6
// / FR-CR-014, Principle 8). For the configured tracked categories (the
// category_aliases canonical set) and the current card data, GenerateRecommendations
// writes exactly one card_recommendations row per (period, category)
// (SCN-083-G06), running the PURE optimizer (optimize.go) per category. Rows
// carrying starred_override are PRESERVED over the optimizer's pick
// (SCN-083-G07): a user's manual override always wins, exactly like the
// reconciler refuses to overwrite a manual rotating-category override.
//
// The optimizer is pure; this file owns the live-Store wiring (T-07-03) and the
// per-period upsert. Every run also appends a card_runs audit row (RunTypeOptimize,
// Principle 8) so "why does this period look like this" is always answerable.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Recommender generates per-period card recommendations from live card data.
// It owns no model/network access — only a Store and a clock. The clock
// defaults to time.Now().UTC(); tests may override the unexported now field for
// deterministic period/active-window assertions.
type Recommender struct {
	store *Store
	now   func() time.Time
}

// NewRecommender constructs a Recommender over a Store.
func NewRecommender(store *Store) *Recommender {
	return &Recommender{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// RecommendationReport summarizes one generation run.
type RecommendationReport struct {
	Period            string    `json:"period"`
	GeneratedAt       time.Time `json:"generated_at"`
	TrackedCategories int       `json:"tracked_categories"`
	Generated         int       `json:"generated"`
	PreservedOverride int       `json:"preserved_override"`
	RunID             string    `json:"run_id"`
}

// OptimizationReport is the read-only optimizer breakdown across the tracked
// categories for a period — the source data behind a recommendation, surfaced
// without persisting (SCN-083-G08 report endpoint).
type OptimizationReport struct {
	Period      string               `json:"period"`
	GeneratedAt time.Time            `json:"generated_at"`
	Categories  []OptimizationResult `json:"categories"`
}

// CurrentPeriod returns the recommender's notion of the current monthly period
// label (e.g. "2026-06"), derived from the clock. Used by the REST layer when a
// caller omits an explicit period.
func (r *Recommender) CurrentPeriod() string {
	return r.now().Format("2006-01")
}

// loadInputs builds the optimizer inputs from live card data: every active
// wallet card joined to its catalog, offers, selections, and the card's
// reconciled rotating-category records (all lifecycle states — the optimizer
// filters to active), plus the category_aliases set used for normalization and
// as the tracked-category list.
func (r *Recommender) loadInputs(ctx context.Context) ([]CardInputs, []CategoryAlias, error) {
	cards, err := r.store.ListUserCards(ctx, true)
	if err != nil {
		return nil, nil, fmt.Errorf("list wallet cards: %w", err)
	}
	aliases, err := r.store.ListCategoryAliases(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list category aliases: %w", err)
	}

	// Cache catalog + rotating lookups by catalog id so multiple wallet cards
	// referencing the same catalog card do not re-query.
	catalogCache := map[string]*CatalogCard{}
	rotatingCache := map[string][]RotatingCategory{}

	inputs := make([]CardInputs, 0, len(cards))
	for i := range cards {
		uc := cards[i]
		cat, ok := catalogCache[uc.CardCatalogID]
		if !ok {
			cat, err = r.store.GetCatalogCard(ctx, uc.CardCatalogID)
			if err != nil {
				return nil, nil, fmt.Errorf("get catalog %s: %w", uc.CardCatalogID, err)
			}
			catalogCache[uc.CardCatalogID] = cat
		}
		rot, ok := rotatingCache[uc.CardCatalogID]
		if !ok {
			rot, err = r.store.ListRotatingCategoriesByCard(ctx, uc.CardCatalogID)
			if err != nil {
				return nil, nil, fmt.Errorf("list rotating %s: %w", uc.CardCatalogID, err)
			}
			rotatingCache[uc.CardCatalogID] = rot
		}
		offers, err := r.store.ListOffersByUserCard(ctx, uc.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list offers %s: %w", uc.ID, err)
		}
		sels, err := r.store.ListSelectionsByUserCard(ctx, uc.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list selections %s: %w", uc.ID, err)
		}
		inputs = append(inputs, CardInputs{
			UserCard:   uc,
			Catalog:    cat,
			Offers:     offers,
			Selections: sels,
			Rotating:   rot,
		})
	}
	return inputs, aliases, nil
}

// trackedCategories returns the canonical tracked-category list (the
// category_aliases canonical names) in stable priority-then-name order (the
// store already returns aliases so ordered).
func trackedCategories(aliases []CategoryAlias) []CategoryAlias {
	return aliases
}

// GenerateRecommendations writes one card_recommendations row per tracked
// category for the period, honoring starred overrides, and appends an audit
// run. When period is empty the current monthly period is used.
func (r *Recommender) GenerateRecommendations(ctx context.Context, period, trigger string) (RecommendationReport, error) {
	if period == "" {
		period = r.CurrentPeriod()
	}
	if trigger == "" {
		trigger = RunTriggerManual
	}
	started := r.now()

	inputs, aliases, err := r.loadInputs(ctx)
	if err != nil {
		return RecommendationReport{}, err
	}
	tracked := trackedCategories(aliases)

	report := RecommendationReport{
		Period:            period,
		GeneratedAt:       started,
		TrackedCategories: len(tracked),
		RunID:             uuid.NewString(),
	}

	for _, alias := range tracked {
		category := alias.CanonicalCategory

		// G07: a manual starred override is authoritative — never overwritten
		// by the optimizer's pick. The observation/optimizer output is computed
		// for the report but the persisted recommendation row is left intact.
		existing, err := r.store.GetRecommendation(ctx, period, category)
		if err != nil {
			return RecommendationReport{}, fmt.Errorf("get recommendation %s/%s: %w", period, category, err)
		}
		if existing != nil && existing.StarredOverride {
			report.PreservedOverride++
			continue
		}

		result := Optimize(category, inputs, aliases, r.now())
		rec := &CardRecommendation{
			ID:                    uuid.NewString(),
			PeriodLabel:           period,
			Category:              category,
			RecommendedUserCardID: result.RecommendedUserCardID,
			Rate:                  result.Rate,
			Reason:                result.Reason,
			Starred:               alias.Starred,
			StarredOverride:       false,
			GeneratedAt:           started,
		}
		if existing != nil {
			rec.ID = existing.ID
			rec.CalendarEventUID = existing.CalendarEventUID
			rec.Starred = existing.Starred || alias.Starred
		}
		if err := r.store.UpsertRecommendation(ctx, rec); err != nil {
			return RecommendationReport{}, fmt.Errorf("upsert recommendation %s/%s: %w", period, category, err)
		}
		report.Generated++
	}

	finished := r.now()
	status := RunStatusSuccess
	run := &CardRun{
		ID:                  report.RunID,
		RunType:             RunTypeOptimize,
		Trigger:             trigger,
		Status:              status,
		CategoriesExtracted: report.Generated,
		StartedAt:           &started,
		FinishedAt:          &finished,
	}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return RecommendationReport{}, fmt.Errorf("write optimize audit run: %w", err)
	}
	return report, nil
}

// BuildOptimizationReport computes the optimizer breakdown across the tracked
// categories for a period WITHOUT persisting (the report endpoint). When period
// is empty the current monthly period is used.
func (r *Recommender) BuildOptimizationReport(ctx context.Context, period string) (OptimizationReport, error) {
	if period == "" {
		period = r.CurrentPeriod()
	}
	inputs, aliases, err := r.loadInputs(ctx)
	if err != nil {
		return OptimizationReport{}, err
	}
	tracked := trackedCategories(aliases)
	out := OptimizationReport{
		Period:      period,
		GeneratedAt: r.now(),
		Categories:  make([]OptimizationResult, 0, len(tracked)),
	}
	for _, alias := range tracked {
		out.Categories = append(out.Categories, Optimize(alias.CanonicalCategory, inputs, aliases, r.now()))
	}
	return out, nil
}
