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
	if sched == nil {
		return
	}
	if !cfg.CardRewards.Enabled {
		slog.Info("card-rewards scheduler jobs not registered (card_rewards disabled in SST)")
		return
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		slog.Warn("card-rewards scheduler: postgres pool unavailable; refresh + recommend jobs not registered")
		return
	}

	// The source connector is registered + connected in connectors.go (only
	// when card_rewards is enabled). Reuse that same connected instance.
	conn, ok := svc.registry.Get(cardrewardsConnector.ConnectorID)
	if !ok {
		slog.Warn("card-rewards scheduler: source connector not registered; refresh + recommend jobs not registered")
		return
	}

	store := cardrewards.NewStore(svc.pg.Pool)

	// Extraction orchestrator over the ML sidecar (Constitution C2 — the model
	// call lives in the Python sidecar). Fail-loud: missing ML_SIDECAR_URL /
	// AUTH_TOKEN is a misconfiguration when the feature is enabled.
	timeout := time.Duration(cfg.CardRewards.FetchTimeoutSeconds) * time.Second
	sidecar, err := cardrewards.NewHTTPSidecarExtractor(cfg.MLSidecarURL, cfg.AuthToken, timeout)
	if err != nil {
		slog.Warn("card-rewards scheduler: sidecar extractor construction failed; jobs not registered", "error", err)
		return
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
		slog.Warn("card-rewards scheduler: pipeline construction failed; jobs not registered", "error", err)
		return
	}

	sched.SetCardRewardsJobs(pipeline, cfg.CardRewards.ScrapeCron, cfg.CardRewards.MonthlyRecommendCron)
	slog.Info("card-rewards scheduler jobs wired",
		"scrape_cron", cfg.CardRewards.ScrapeCron,
		"monthly_recommend_cron", cfg.CardRewards.MonthlyRecommendCron,
		"calendar_sync", cfg.CardRewards.CalendarSync)
}
