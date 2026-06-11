// wiring_cardrewards_scheduler.go — spec 083 Scope 09.
//
// Constructs the card-rewards refresh/recommend pipeline from the SST config +
// the already-connected source connector and hands it to the scheduler's
// SetCardRewardsJobs setter. Called from main() after the scheduler is
// constructed and before Start() (same placement as
// wireLegacyRetirementScheduler).
//
// Fail-loud per Gate G028 / smackerel-no-defaults: the two crons and every
// extraction tunable are already validated by config.LoadCardRewardsConfig().
// This helper additionally WARN-and-skips (registers no jobs) when the feature
// is disabled, when the SQL pool / connector / extractor cannot be built, or
// when a cron is empty — so dev/test installs without card_rewards enabled
// still boot with no card-rewards jobs (the connector itself is auto-started
// only when enabled; see connectors.go).
package main

import (
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/cardrewards"
	"github.com/smackerel/smackerel/internal/config"
	cardrewardsConnector "github.com/smackerel/smackerel/internal/connector/cardrewards"
	"github.com/smackerel/smackerel/internal/scheduler"
)

func wireCardRewardsScheduler(cfg *config.Config, svc *coreServices, sched *scheduler.Scheduler) {
	if sched == nil || svc == nil {
		return
	}

	// Spec 083 Scope 11 — expose the scheduler's manual triggers to the admin
	// web page (TriggerCardRewardsRefreshNow / TriggerCardRewardsRecommendNow).
	// Done regardless of card_rewards.enabled so an operator can trigger a
	// refresh/recommend on demand even when the cron jobs are not auto-
	// registered; the admin page degrades to read-only run history if no
	// pipeline gets set below.
	if svc.cardRewardsWebHandler != nil {
		svc.cardRewardsWebHandler.SetTriggers(sched)
	}

	if svc.pg == nil || svc.pg.Pool == nil {
		slog.Warn("card-rewards scheduler: postgres pool unavailable; pipeline + jobs not wired")
		return
	}

	// The source connector is registered unconditionally (connectors.go) and
	// auto-started only when card_rewards is enabled. Reuse the registered
	// instance for the pipeline; when the feature is disabled it is simply not
	// connected, so a manual "scrape now" records a failed scrape run (no
	// fabrication) and the model-independent stages still run.
	conn, ok := svc.registry.Get(cardrewardsConnector.ConnectorID)
	if !ok {
		slog.Warn("card-rewards scheduler: source connector not registered; pipeline + jobs not wired")
		return
	}

	store := cardrewards.NewStore(svc.pg.Pool)

	// Extraction orchestrator over the ML sidecar (Constitution C2 — the model
	// call lives in the Python sidecar). The HTTP sidecar is fail-loud: it needs
	// a non-empty ML_SIDECAR_URL + AUTH_TOKEN. When card_rewards is ENABLED a
	// failure is a real misconfiguration → WARN and skip wiring (fail-loud). When
	// DISABLED (dev/test, where AUTH_TOKEN is an empty placeholder), wire a
	// degraded pipeline with NO sidecar so the admin manual triggers still record
	// scrape/optimize runs: the connector is not connected when disabled, so the
	// refresh's extract stage receives zero inputs and never dereferences the nil
	// sidecar (Extractor.Run only calls the sidecar per-input). Live extraction is
	// simply unavailable until the feature is enabled with a real AUTH_TOKEN.
	timeout := time.Duration(cfg.CardRewards.FetchTimeoutSeconds) * time.Second
	var sidecar cardrewards.SidecarExtractor
	if httpSidecar, sErr := cardrewards.NewHTTPSidecarExtractor(cfg.MLSidecarURL, cfg.AuthToken, timeout); sErr != nil {
		if cfg.CardRewards.Enabled {
			slog.Warn("card-rewards scheduler: sidecar extractor construction failed; pipeline + jobs not wired", "error", sErr)
			return
		}
		slog.Warn("card-rewards scheduler: sidecar extractor unavailable (feature disabled / AUTH_TOKEN unset); wiring a degraded pipeline for manual triggers only — live extraction disabled until card_rewards is enabled with a real AUTH_TOKEN", "error", sErr)
	} else {
		sidecar = httpSidecar
	}
	extractor := cardrewards.NewExtractor(store, sidecar, cfg.CardRewards.Extraction.ConfidenceThreshold, nil)
	reconciler := cardrewards.NewReconciler(store, cfg.CardRewards.Extraction.ConfidenceThreshold, nil)
	recommender := cardrewards.NewRecommender(store)

	// Calendar delivery (Scope 08) requires a concrete CalDAV client. The
	// production CalDAV-client construction for cards is not wired here (it
	// follows the meal-plan precedent, which also injects its CalDAV client
	// separately); the calendar bridge + its sync behavior are delivered and
	// covered by Scope 08 + Scope 09 integration tests against the
	// external-boundary CalDAV fake. The pipeline accepts a nil bridge and the
	// recommend run records zero calendar events without error. Surface the
	// config-vs-wiring gap loudly when calendar_sync is requested.
	if cfg.CardRewards.CalendarSync {
		slog.Warn("card-rewards scheduler: calendar_sync is enabled in SST but the production CalDAV client is not wired yet; recommendations are generated and persisted, calendar events are not delivered")
	}

	pipeline, err := cardrewards.NewPipeline(conn, extractor, reconciler, recommender, nil, store, nil)
	if err != nil {
		slog.Warn("card-rewards scheduler: pipeline construction failed; pipeline + jobs not wired", "error", err)
		return
	}

	// Cron auto-registration stays gated on card_rewards.enabled: pass the real
	// crons only when enabled, empty strings otherwise. SetCardRewardsJobs still
	// sets the pipeline (so the admin manual triggers work in dev/test), but
	// scheduleCardRewardsJobs registers a cron job only for a non-empty cron —
	// so a disabled install boots with the pipeline available for manual
	// triggers yet performs NO auto-scrape.
	scrapeCron, recommendCron := "", ""
	if cfg.CardRewards.Enabled {
		scrapeCron = cfg.CardRewards.ScrapeCron
		recommendCron = cfg.CardRewards.MonthlyRecommendCron
	}
	sched.SetCardRewardsJobs(pipeline, scrapeCron, recommendCron)
	slog.Info("card-rewards scheduler wired",
		"enabled", cfg.CardRewards.Enabled,
		"scrape_cron", scrapeCron,
		"monthly_recommend_cron", recommendCron,
		"manual_triggers", svc.cardRewardsWebHandler != nil,
		"calendar_sync", cfg.CardRewards.CalendarSync)
}
