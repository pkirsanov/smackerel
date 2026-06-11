package cardrewards

// Card-rewards refresh/recommend pipeline (spec 083 Scope 09, design §8 /
// FR-CR-018, FR-CR-019, NFR-CR-005). This file wires the stages built in
// Scopes 04–08 into the two composite runs the scheduler fires:
//
//   - RunDailyRefresh  — connector sync → extract → reconcile → advance
//     lifecycle (the `card_rewards_refresh` job, SCN-083-I01/I03).
//   - RunMonthlyRecommend — optimize → recommend → calendar sync (the
//     `card_rewards_recommend` job, SCN-083-I02/I04).
//
// Both methods are the SINGLE code path used by BOTH the scheduled cron jobs
// AND the admin "scrape now" / "sync calendar now" manual triggers — the only
// difference is the trigger label passed in ("scheduled" vs "manual",
// SCN-083-I05 / NFR-CR-005). There is no second, manual-only path to drift.
//
// Every stage writes its own card_runs audit row via the Scope 04–08 code it
// calls (Principle 8); the refresh additionally writes the connector-sync
// `scrape` run here. Re-running either method is idempotent (SCN-083-I06):
// reconcile upserts the authoritative rotating_categories row per (card,
// period) and recommend upserts card_recommendations + updates the SAME CalDAV
// UIDs, so a second run never duplicates rows or events.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/connector"
)

// SourceConnector is the fetch-only card-rewards source connector seam the
// daily refresh drives (Scope 04). It is declared as an interface so the
// pipeline depends on behavior, not the concrete connector, and so the daily
// refresh stays decoupled from the network boundary. The production
// *connector/cardrewards.Connector satisfies it; tests supply an
// external-boundary source fake (the source website is the external
// dependency, mirroring the blessed CalDAVClient fake).
type SourceConnector interface {
	Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
}

// CategoryExtractor is the strict-schema LLM extraction orchestrator seam
// (Scope 05). *Extractor satisfies it. The pipeline depends on the interface
// so the refresh stays decoupled from the ML-sidecar transport; a sidecar that
// is unavailable (no model) makes Extractor.Run record a partial extract run
// and flag targets for verification — it never fabricates an observation and
// never aborts the rest of the refresh (lifecycle is date-driven; reconcile
// still merges any observations already present).
type CategoryExtractor interface {
	Run(ctx context.Context, inputs []ExtractInput, trigger string) (*ExtractResult, error)
}

// Pipeline composes the card-rewards stages into the daily-refresh and
// monthly-recommend runs. connector, extractor, reconciler, recommender and
// store are REQUIRED; calendar may be nil (CalDAV delivery disabled), in which
// case RunMonthlyRecommend records zero calendar events without error.
type Pipeline struct {
	connector   SourceConnector
	extractor   CategoryExtractor
	reconciler  *Reconciler
	recommender *Recommender
	calendar    *CardCalendarBridge // nil when calendar sync is disabled
	store       *Store
	logger      *slog.Logger
	now         func() time.Time
}

// NewPipeline constructs the refresh/recommend orchestrator. It is fail-loud
// (smackerel-no-defaults): every required dependency must be non-nil. A nil
// logger is replaced with slog.Default so call sites need not guard. calendar
// is the only optional dependency (nil = calendar sync disabled).
func NewPipeline(
	conn SourceConnector,
	extractor CategoryExtractor,
	reconciler *Reconciler,
	recommender *Recommender,
	calendar *CardCalendarBridge,
	store *Store,
	logger *slog.Logger,
) (*Pipeline, error) {
	if conn == nil {
		return nil, errors.New("cardrewards.NewPipeline: connector is required")
	}
	if extractor == nil {
		return nil, errors.New("cardrewards.NewPipeline: extractor is required")
	}
	if reconciler == nil {
		return nil, errors.New("cardrewards.NewPipeline: reconciler is required")
	}
	if recommender == nil {
		return nil, errors.New("cardrewards.NewPipeline: recommender is required")
	}
	if store == nil {
		return nil, errors.New("cardrewards.NewPipeline: store is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		connector:   conn,
		extractor:   extractor,
		reconciler:  reconciler,
		recommender: recommender,
		calendar:    calendar,
		store:       store,
		logger:      logger,
		now:         func() time.Time { return time.Now().UTC() },
	}, nil
}

// RunDailyRefresh executes connector sync → extract → reconcile → advance
// lifecycle, auditing each stage via card_runs (design §8, SCN-083-I03). It is
// the SHARED code path for both the scheduled `card_rewards_refresh` job and
// the manual "scrape now" trigger; the only difference is the trigger label
// (SCN-083-I05 / NFR-CR-005). trigger MUST be "scheduled" or "manual".
//
// Observable outcome is the persisted card_runs / rotating_categories state
// (callers and tests assert against the datastore); the method returns only an
// error so it satisfies the scheduler's pipeline seam without coupling it to
// card-rewards result types.
//
// Resilience contract: a connector that errors (not connected) or a sidecar
// that is unavailable does NOT abort the refresh — the failure is recorded in
// its stage's audit row and the later, model-independent stages (reconcile of
// existing observations, date-driven lifecycle) still run. Only a genuine
// datastore error fails the run (fail-loud).
func (p *Pipeline) RunDailyRefresh(ctx context.Context, trigger string) error {
	if !ValidRunTrigger(trigger) {
		return fmt.Errorf("cardrewards: invalid run trigger %q", trigger)
	}

	// Stage 1 — connector sync (scrape). The connector emits one
	// source-attributed artifact per source that responded; it performs no
	// parsing. We audit the sync with a `scrape` card_runs row recording the
	// emitted-artifact count (per-source failure granularity lives in connector
	// health, Scope 04). A sync error is recorded as a failed run and does not
	// abort the model-independent later stages.
	started := p.now()
	artifacts, _, syncErr := p.connector.Sync(ctx, "")
	finished := p.now()
	scrapeStatus := RunStatusSuccess
	var scrapeErrDetail *string
	if syncErr != nil {
		scrapeStatus = RunStatusFailed
		msg := syncErr.Error()
		scrapeErrDetail = &msg
		p.logger.Warn("card-rewards refresh: connector sync failed — recording scrape run, continuing with model-independent stages",
			"trigger", trigger, "error", syncErr)
	}
	scrapeRun := &CardRun{
		ID:               uuid.NewString(),
		RunType:          RunTypeScrape,
		Trigger:          trigger,
		Status:           scrapeStatus,
		SourcesAttempted: len(artifacts),
		SourcesSucceeded: len(artifacts),
		ErrorDetail:      scrapeErrDetail,
		StartedAt:        &started,
		FinishedAt:       &finished,
	}
	if err := p.store.CreateRun(ctx, scrapeRun); err != nil {
		return fmt.Errorf("cardrewards refresh: write scrape audit run: %w", err)
	}

	// Stage 2 — extract. Fan each fetched source page out across the known
	// rotating catalog cards for the current quarter and hand the batch to the
	// Scope 05 orchestrator. With the sidecar unavailable, Run records a
	// partial extract run and flags targets — it never fabricates an
	// observation. A returned error is a genuine datastore/trigger failure.
	inputs := p.buildExtractInputs(ctx, artifacts)
	extractRes, err := p.extractor.Run(ctx, inputs, trigger)
	if err != nil {
		return fmt.Errorf("cardrewards refresh: extract stage: %w", err)
	}

	// Stage 3 — reconcile. Merge every (card, period) that has observations
	// into its authoritative rotating_categories record. Driving off the
	// stored observation refs (not just this run's) is what makes a re-run
	// idempotent: the same observations upsert the same single row (I06).
	refs, err := p.store.ListObservationRefs(ctx)
	if err != nil {
		return fmt.Errorf("cardrewards refresh: list observation refs: %w", err)
	}
	reconcileRes, err := p.reconciler.Reconcile(ctx, refs, trigger)
	if err != nil {
		return fmt.Errorf("cardrewards refresh: reconcile stage: %w", err)
	}

	// Stage 4 — advance lifecycle by date (upcoming → active → expired) and
	// surface pending re-enrollments. Model-independent (Principle 3).
	lifecycleRes, err := p.reconciler.AdvanceLifecycle(ctx, trigger)
	if err != nil {
		return fmt.Errorf("cardrewards refresh: lifecycle stage: %w", err)
	}

	p.logger.Info("card-rewards daily refresh complete",
		"trigger", trigger,
		"artifacts", len(artifacts),
		"extracted", extractRes.Stored,
		"reconciled", reconcileRes.Reconciled,
		"lifecycle_transitioned", lifecycleRes.Transitioned)
	return nil
}

// RunMonthlyRecommend executes optimize → recommend → calendar sync for the
// current monthly period, auditing the optimize run and (when calendar
// delivery is enabled) the calendar_sync run via card_runs (design §8,
// SCN-083-I04). It is the SHARED code path for both the scheduled
// `card_rewards_recommend` job and the manual "sync calendar now" trigger
// (SCN-083-I05 / NFR-CR-005). trigger MUST be "scheduled" or "manual".
//
// Idempotent (SCN-083-I06): GenerateRecommendations upserts card_recommendations
// keyed on (period, category) and SyncPeriod updates the SAME stable CalDAV
// UIDs, so a re-run never duplicates a recommendation row or a calendar event.
func (p *Pipeline) RunMonthlyRecommend(ctx context.Context, trigger string) error {
	if !ValidRunTrigger(trigger) {
		return fmt.Errorf("cardrewards: invalid run trigger %q", trigger)
	}
	period := p.recommender.CurrentPeriod()

	report, err := p.recommender.GenerateRecommendations(ctx, period, trigger)
	if err != nil {
		return fmt.Errorf("cardrewards recommend: generate recommendations: %w", err)
	}

	calEvents := 0
	if p.calendar != nil {
		syncRes, err := p.calendar.SyncPeriod(ctx, period, trigger)
		if err != nil {
			return fmt.Errorf("cardrewards recommend: calendar sync: %w", err)
		}
		calEvents = syncRes.EventsWritten
	}

	p.logger.Info("card-rewards monthly recommend complete",
		"trigger", trigger,
		"period", period,
		"generated", report.Generated,
		"preserved_override", report.PreservedOverride,
		"calendar_events", calEvents)
	return nil
}

// buildExtractInputs fans each fetched source page out across the known
// rotating catalog cards for the current quarter, building one ExtractInput per
// (artifact × rotating card). Each input carries the artifact's verbatim page
// text plus its source provenance (Principle 4); the Scope 05 orchestrator
// targets the card_id + period and discards any response that names a different
// card/period. A store error here is logged and yields no inputs (the refresh
// then has nothing new to extract but still reconciles + advances lifecycle) —
// the extraction stage is best-effort relative to the model-independent stages.
func (p *Pipeline) buildExtractInputs(ctx context.Context, artifacts []connector.RawArtifact) []ExtractInput {
	if len(artifacts) == 0 {
		return nil
	}
	cards, err := p.store.ListCatalogCards(ctx)
	if err != nil {
		p.logger.Warn("card-rewards refresh: list catalog cards for extraction failed — skipping extract fan-out",
			"error", err)
		return nil
	}
	period := currentQuarterLabel(p.now())
	var inputs []ExtractInput
	for _, art := range artifacts {
		sourceName, sourceURL, issuerHint := artifactProvenance(art)
		for i := range cards {
			if cards[i].CardType != CardTypeRotating {
				continue
			}
			inputs = append(inputs, ExtractInput{
				CardID:      cards[i].ID,
				IssuerHint:  issuerHint,
				PeriodLabel: period,
				SourceName:  sourceName,
				SourceURL:   sourceURL,
				PageText:    art.RawContent,
			})
		}
	}
	return inputs
}

// currentQuarterLabel returns the rotating-period label for t's calendar
// quarter (e.g. 2026-07-15 → "Q3_2026"), matching the period_label shape the
// importer and reconciler use for quarterly rotating categories.
func currentQuarterLabel(t time.Time) string {
	q := (int(t.Month())-1)/3 + 1
	return fmt.Sprintf("Q%d_%d", q, t.Year())
}

// artifactProvenance extracts the source name, URL, and issuer hint a
// card-rewards connector stamps on each RawArtifact (Principle 4). It falls
// back to the artifact's own URL/Title when a metadata key is absent so an
// extraction input always carries a traceable source, never a fabricated one.
func artifactProvenance(art connector.RawArtifact) (name, url, issuer string) {
	url = art.URL
	if art.Metadata != nil {
		if v, ok := art.Metadata["source_name"].(string); ok {
			name = v
		}
		if v, ok := art.Metadata["source_url"].(string); ok && v != "" {
			url = v
		}
		if v, ok := art.Metadata["issuer_hint"].(string); ok {
			issuer = v
		}
	}
	if name == "" {
		name = art.Title
	}
	return name, url, issuer
}
