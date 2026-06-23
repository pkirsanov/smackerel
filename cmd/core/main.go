package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
	"github.com/smackerel/smackerel/internal/backup"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
	telegramassistant "github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	whatsappadapter "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"

	"github.com/go-chi/chi/v5"
)

// version, commitHash, and buildTime are set by -ldflags at build time.
var (
	version    = "dev"
	commitHash = "unknown"
	buildTime  = "unknown"
)

func main() {
	// CLI subcommand dispatch (spec 037 Scope 6: `smackerel agent ...`).
	// Subcommands run to completion and exit; the runtime loop is the
	// default when no subcommand is supplied.
	if len(os.Args) > 1 && os.Args[1] == "agent" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		os.Exit(runAgentCommand(ctx, os.Args[2:]))
	}
	// Spec 044 Scope 01 — `smackerel auth <subcommand>` operator surface
	// (enroll | rotate | revoke | list-users | bootstrap | keygen).
	if len(os.Args) > 1 && os.Args[1] == "auth" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		os.Exit(runAuthCommand(ctx, os.Args[2:]))
	}
	// Spec 070 — `smackerel-core users <subcommand>` operator surface
	// (add | set-password | list).
	if len(os.Args) > 1 && os.Args[1] == "users" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		os.Exit(runUsersCommand(ctx, os.Args[2:]))
	}
	// Spec 071 SCOPE-03 — `smackerel-core assistant <subcommand>`
	// operator surface (replay-intent).
	if len(os.Args) > 1 && os.Args[1] == "assistant" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		os.Exit(runAssistantCommand(ctx, os.Args[2:]))
	}
	// BUG-056-002 Scope B — `smackerel-core connector <connector> <subcommand>`
	// operator surface (twitter authorize-begin|authorize-finalize|
	// authorize-status: the User-Context OAuth 2.0 PKCE authorize flow).
	if len(os.Args) > 1 && os.Args[1] == "connector" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		os.Exit(runConnectorCommand(ctx, os.Args[2:]))
	}
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spec 062 SCOPE-2 — per-transport fail-loud validation. Each
	// adapter delegates its SST presence check to the transportconfig
	// registry; the first missing REQUIRED env var aborts startup
	// with the registry's exact FailLoudMsg before any other config
	// is touched. Calling each adapter's ValidateTransportConfig
	// directly (rather than transportconfig.ValidateAllFromOSEnv)
	// makes the consumer relationship visible to grep + reviewer.
	if err := httpadapter.ValidateTransportConfig(); err != nil {
		return fmt.Errorf("assistant http transport configuration: %w", err)
	}
	if err := whatsappadapter.ValidateTransportConfig(); err != nil {
		return fmt.Errorf("assistant whatsapp transport configuration: %w", err)
	}
	if err := telegramassistant.ValidateTransportConfig(); err != nil {
		return fmt.Errorf("assistant telegram transport configuration: %w", err)
	}
	if err := telegram.ValidateTransportConfig(); err != nil {
		return fmt.Errorf("legacy telegram transport configuration: %w", err)
	}

	// Load and validate configuration — fails loudly on missing required vars
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if err := configureLogging(cfg); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	slog.Info("starting smackerel-core", "port", cfg.Port, "version", version, "commit", commitHash, "build_time", buildTime)

	// Build core services (DB, NATS, pipeline, knowledge, etc.)
	svc, err := buildCoreServices(ctx, cfg)
	if err != nil {
		return err
	}

	// Register and start all connectors
	if err := registerConnectors(ctx, cfg, svc); err != nil {
		return err
	}

	// Build API dependencies (annotations, lists, etc.)
	deps, listResolver, listStore, err := buildAPIDeps(ctx, cfg, svc)
	if err != nil {
		return fmt.Errorf("buildAPIDeps: %w", err)
	}

	// Spec 037 Scope 10 — wire the agent runtime (bridge + router +
	// executor) into deps so POST /v1/agent/invoke is live and
	// scheduler/pipeline call sites have a runner to invoke.
	agentBridge, agentRT, err := wireAgentBridge(ctx, svc, deps)
	if err != nil {
		// Fail-loud per SST: missing agent config or DB/NATS-derived
		// dependencies must surface, not be silently disabled.
		return fmt.Errorf("agent bridge wiring: %w", err)
	}

	// Spec 061 SCOPE-03 — design §7.2 rule #6 ("scenario YAMLs
	// present"). Runs only when the assistant capability is enabled.
	scenarioDir, err := agentScenarioDir()
	if err != nil {
		return fmt.Errorf("agent scenario dir lookup: %w", err)
	}
	if err := validateAssistantScenariosPresent(cfg, scenarioDir); err != nil {
		return err
	}

	// Spec 061 SCOPE-06 — wire per-skill runtime services into the
	// agent tool registry. Runs AFTER wireAgentBridge has populated
	// the registry via blank-import side effects and BEFORE
	// wireAssistantFacade is invoked. Fail-loud when an SST-enabled
	// skill is missing its production dependency.
	if err := wireAssistantSkillServices(cfg, svc); err != nil {
		return fmt.Errorf("assistant skill services wiring: %w", err)
	}

	// Spec 064 SCOPE-12 — install the live open-knowledge agent
	// behind the substrate `open_knowledge_invoke` tool. No-op when
	// assistant.open_knowledge.enabled=false. Fail-loud per SST when
	// enabled but a required dep is missing.
	if err := wireOpenKnowledge(cfg, svc, scenarioDir); err != nil {
		return fmt.Errorf("open-knowledge subsystem wiring: %w", err)
	}

	// BUG-034-003 follow-up: expense handler MUST be constructed BEFORE
	// api.NewRouter(deps), because NewRouter does a single-pass route
	// registration that skips deps.ExpenseHandler when it is nil. The
	// later wireExpenseTracking(...) call (after the router is built)
	// would set deps.ExpenseHandler but no longer wire it into chi,
	// leaving /api/expenses returning 404 forever. Construct first,
	// register routes during NewRouter, then call any post-router
	// wiring (none today for expenses) below.
	wireExpenseTracking(ctx, cfg, svc, deps)

	// BUG-034-004 follow-up: same construction-order rule applies to
	// meal-plan handler. Split into wireMealPlanningHandler (early —
	// no scheduler/tgBot dependency) and wireMealPlanningSchedulerAndBot
	// (late — needs sched + tgBot constructed after NewRouter).
	wireMealPlanningHandler(cfg, svc, deps, listResolver, listStore)

	// Spec 083 Scope 02 — card-rewards CRUD handler. Same construction-order
	// rule as meal planning: construct + attach to deps BEFORE NewRouter so
	// the routes register in the single-pass route build. Mounted whenever a
	// Postgres pool exists (independent of the card_rewards ingestion flag).
	wireCardRewardsHandler(svc, deps)

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	tgBot := startTelegramBotIfConfigured(ctx, cfg, deps)
	attachDriveSaveBridgeToTelegram(svc, tgBot)
	attachDriveRetrieveBridgeToTelegram(svc, tgBot)
	wireLegacyAliasInterceptor(cfg, svc, tgBot)

	// Spec 076 SCOPE-4b — wire the annotation classifier dual-write
	// shadow comparator into the HTTP handler + Telegram bot.
	wireAnnotationShadowComparator(agentBridge, deps, tgBot)

	// Spec 021 BUG-021-005 — wire the LLM-driven relationship-cooling
	// evaluator into the intelligence engine (replaces the hardcoded
	// magic-number heuristic). Nil bridge ⇒ cooling alerts disabled.
	wireRelationshipCoolingEvaluator(agentBridge, svc.intEngine)

	// Spec 021 BUG-021-006 — wire the LLM-driven alert-timing evaluator
	// into the intelligence engine for the bill / trip-prep / return-window
	// producers (replaces hardcoded N-day windows). Nil bridge ⇒ disabled.
	wireAlertTimingEvaluator(agentBridge, svc.intEngine)

	// Spec 021 BUG-021-007 — wire the LLM-driven resurfacing-worthiness
	// evaluator into the intelligence engine for the dormancy strategy
	// (replaces hardcoded 30-day/0.3-relevance thresholds). Nil bridge ⇒
	// dormancy resurfacing disabled (serendipity unaffected).
	wireResurfaceEvaluator(agentBridge, svc.intEngine)

	// Spec 021 BUG-021-008 — wire the LLM-driven expertise classifier into the
	// intelligence engine for the expertise map (replaces the hardcoded
	// depth-score weights + tier/velocity thresholds). Nil bridge ⇒ the
	// expertise endpoint fails loud (no hardcoded tier fallback).
	wireExpertiseEvaluator(agentBridge, svc.intEngine)

	// Spec 021 BUG-021-010 — wire the LLM-driven hospitality concern evaluator
	// into the digest generator (replaces hardcoded sentiment/rating/issue-count
	// alert thresholds) on the reusable agent.InvokeJudgment foundation. Nil
	// bridge ⇒ guest/property concern alerts disabled (no threshold fallback).
	wireHospitalityEvaluator(agentBridge, svc.digestGen)

	// Spec 021 BUG-021-009 — wire the operational bounds for LLM-driven seasonal
	// pattern detection (replaces the hardcoded 0.7/1.5 volume-ratio threshold;
	// significance is judged by the seasonal.analyze ML path). SST load failure
	// ⇒ seasonal detection disabled.
	wireSeasonalConfig(svc.intEngine)

	// Spec 095 SCOPE-07 / PKT-095-B — late-bind the production evergreen judge
	// into the ingestion front-door scorer (built + injected into the connector
	// publisher in buildCoreServices). Nil scorer (evergreen disabled) or nil
	// bridge ⇒ no-op; ingestion keeps using the deterministic TierSignals
	// fallback. Only wired when judgment_source=scenario.
	wireEvergreenScorer(agentBridge, svc.evergreenScorer, cfg.Retrieval.Evergreen.JudgmentSource)

	// Spec 061 SCOPE-05 design §17.4 — when Telegram is configured to
	// run in webhook mode, register the POST handler on the existing
	// chi router OUTSIDE bearer-auth (Telegram does not send our
	// bearer; the X-Telegram-Bot-Api-Secret-Token header authenticates
	// each delivery). The handler is registered ONLY when mode=webhook
	// AND the bot was successfully constructed; long_poll mode leaves
	// the route unregistered.
	if tgBot != nil && cfg.Assistant.TelegramMode == "webhook" {
		mux, ok := router.(*chi.Mux)
		if !ok {
			return fmt.Errorf("assistant telegram webhook: api.NewRouter must return *chi.Mux for webhook route registration; got %T", router)
		}
		webhookHandler := telegram.NewWebhookHandler(telegram.WebhookHandlerOptions{
			Bot:    tgBot,
			Secret: cfg.Assistant.TelegramWebhookSecret,
		})
		mux.Method(http.MethodPost, cfg.Assistant.TelegramWebhookPath, webhookHandler)
		// Also register a 405-emitting catchall for the same path so
		// non-POST requests return 405 even before the handler runs
		// (chi's default 405 path requires explicit method registration).
		mux.MethodFunc(http.MethodGet, cfg.Assistant.TelegramWebhookPath, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
		})
		slog.Info("telegram webhook route registered",
			"path", cfg.Assistant.TelegramWebhookPath)
	}

	// Spec 072 SCOPE-1/SCOPE-4 — WhatsApp Business Cloud API webhook ingress.
	// Mounted ONLY when assistant.transports.whatsapp.enabled=true AND
	// the full SST credential set has resolved (validateAssistantConfig
	// has already failed loud otherwise). SCOPE-4 routes both the
	// disabled and enabled paths through MountWebhookRoutes so the
	// operator-status gauges (assistant_whatsapp_enabled,
	// assistant_whatsapp_credentials_ready) always reflect boot state
	// and disabling WhatsApp leaves Telegram + HTTP wiring untouched.
	if cfg.Assistant.Enabled {
		mux, ok := router.(*chi.Mux)
		if !ok {
			return fmt.Errorf("assistant whatsapp webhook: api.NewRouter must return *chi.Mux for webhook route registration; got %T", router)
		}
		var whatsappAdapter *whatsappadapter.Adapter
		if cfg.Assistant.WhatsappEnabled {
			if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
				return errors.New("assistant whatsapp webhook: postgres pool is required")
			}
			var err error
			whatsappAdapter, err = whatsappadapter.NewAdapter(whatsappadapter.Options{
				Verify: whatsappadapter.HMACVerifier{
					AppSecret:   cfg.Assistant.WhatsappAppSecret,
					VerifyToken: cfg.Assistant.WhatsappWebhookVerifyToken,
				},
				IdentityRegistry:          transportidentity.NewPgRegistry(svc.pg.Pool),
				IdentityHashKey:           cfg.Assistant.WhatsappIdentityHashKey,
				MaxTextChars:              cfg.Assistant.WhatsappMaxTextChars,
				RateLimitPerUserPerMinute: cfg.Assistant.WhatsappRateLimitPerUserPerMinute,
			})
			if err != nil {
				return fmt.Errorf("assistant whatsapp adapter: %w", err)
			}
		}
		mounted, err := whatsappadapter.MountWebhookRoutes(mux, whatsappadapter.MountOptions{
			Enabled: cfg.Assistant.WhatsappEnabled,
			Adapter: whatsappAdapter,
			Path:    cfg.Assistant.WhatsappWebhookPath,
		})
		if err != nil {
			return fmt.Errorf("assistant whatsapp webhook mount: %w", err)
		}
		if mounted {
			slog.Info("whatsapp webhook route registered",
				"path", cfg.Assistant.WhatsappWebhookPath)
		} else {
			slog.Info("whatsapp webhook route skipped",
				"reason", "assistant.transports.whatsapp.enabled=false")
		}
	}

	// Spec 061 SCOPE-05 — construct the capability-layer facade +
	// Telegram reference adapter and bind it to the bot AFTER the
	// bot has started. The facade wiring synchronously pre-computes
	// router intent-example embeddings against the ML sidecar's
	// POST /embed endpoint (see internal/agent/router.go NewRouter);
	// on a cold-start where ml-sidecar boots in parallel with core,
	// that probe can fail before the sidecar is reachable and crash
	// the core HTTP listener before it ever binds. To break that
	// boot-order coupling, run facade wiring in a background
	// goroutine with bounded retries: HTTP /api/assistant/turn is
	// already mounted via a LateBoundHandler that returns 503
	// "assistant_http_not_ready" until SetAdapter fires, and the
	// telegram bot.SetAssistantAdapter is also a late setter. SST
	// misconfiguration (nil cfg, missing tokens, etc.) still surfaces
	// loud via repeated error logs; transient sidecar unavailability
	// drains naturally as ml-sidecar finishes its own startup.
	go runAssistantFacadeWiringWithRetry(ctx, cfg, svc, agentRT, tgBot, scenarioDir)

	// Start digest scheduler + intelligence jobs
	sched := scheduler.New(svc.digestGen, tgBot, svc.intEngine, svc.topicLifecycle)

	// Spec 021 Scope 4 — Unified Surfacing Controller. Wires the
	// SST-validated daily-budget / dedupe / suppression / urgent-
	// escalation contract into every producer that dispatches to
	// Telegram / web push / ntfy / email-out.
	//
	// Spec 054 Scope 9 — the SAME controller and ack registry are shared with
	// the notification decision engine so user-facing notifications honor the
	// one global interruption budget, cross-channel dedupe, and ack-suppression
	// state (GAP-06 cohesion). sharedAck is lifted to a named var so the
	// notification incident-ack path can record acks the controller observes.
	sharedAck := surfacing.NewInMemoryAck()
	if surfacingCtrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        cfg.Surfacing.DailyNudgeBudget,
		SuppressionWindowHours:  cfg.Surfacing.SuppressionWindowHours,
		DedupeWindowHours:       cfg.Surfacing.DedupeWindowHours,
		UrgentEscalationEnabled: cfg.Surfacing.UrgentEscalationEnabled,
	}, sharedAck, metrics.SurfacingMetrics{}); err != nil {
		slog.Error("surfacing controller wiring failed (SST validation)", "error", err)
	} else {
		sched.SetSurfacingController(surfacingCtrl)
		if svc.notificationService != nil {
			svc.notificationService.SetSurfacingController(surfacingCtrl)
			svc.notificationService.SetSurfacingAck(sharedAck)
		}
		slog.Info("surfacing controller wired",
			"daily_budget", cfg.Surfacing.DailyNudgeBudget,
			"dedupe_window_hours", cfg.Surfacing.DedupeWindowHours,
			"suppression_window_hours", cfg.Surfacing.SuppressionWindowHours,
			"urgent_escalation_enabled", cfg.Surfacing.UrgentEscalationEnabled,
			"notification_producer_wired", svc.notificationService != nil)
	}

	// Subscribe intelligence engine to annotation events (spec 027)
	if svc.intEngine != nil {
		if err := svc.intEngine.SubscribeAnnotations(ctx); err != nil {
			slog.Warn("annotation subscription failed", "error", err)
		}
	}

	// Subscribe intelligence engine to list completion events (spec 028)
	if svc.intEngine != nil {
		if err := svc.intEngine.SubscribeListsCompleted(ctx); err != nil {
			slog.Warn("list completion subscription failed", "error", err)
		}
	}

	// Wire optional feature services (knowledge, meal planning, recommendations).
	// NOTE: wireExpenseTracking + wireMealPlanningHandler have already
	// run BEFORE api.NewRouter above (see BUG-034-003 + BUG-034-004
	// follow-up comments) so their routes are registered. The late
	// scheduler/bot wiring for meal-planning runs here.
	wireKnowledgeLinter(sched, cfg, svc)
	wireMealPlanningSchedulerAndBot(cfg, svc, sched, tgBot)
	wireRecommendationWatchPoller(sched, agentBridge, svc, cfg, tgBot, deps.RecommendationWatchHandlers)
	wireLegacyRetirementScheduler(cfg, svc, sched)
	wireCardRewardsScheduler(cfg, svc, sched)

	if err := sched.Start(ctx, cfg.DigestCron); err != nil {
		slog.Warn("digest scheduler failed to start", "error", err)
	}

	// Spec 048 — background poll of BACKUP_STATUS_FILE so
	// smackerel_backup_last_success_unixtime and related metrics stay
	// fresh between scrapes. Runs in a goroutine; exits when ctx is
	// canceled at shutdown.
	backupWatcher := backup.NewWatcher(
		cfg.BackupStatusFile,
		time.Duration(cfg.BackupWatcherPollSecs)*time.Second,
		metrics.NewBackupMetricsSink(),
	)
	go backupWatcher.Run(ctx)
	slog.Info("backup watcher started",
		"status_file", cfg.BackupStatusFile,
		"poll_interval_seconds", cfg.BackupWatcherPollSecs,
		"retention_daily", cfg.BackupRetentionDaily,
		"retention_weekly", cfg.BackupRetentionWeekly,
	)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Spec 064 SCOPE-17 / Spec 084 / Spec 087 / Spec 088 — WriteTimeout
		// sized for the longest legitimate open-knowledge reasoning turn. The
		// /ask fast-path (facade.go::runOpenKnowledgeDirect) runs the agent
		// loop directly with THIS request context, so WriteTimeout — not the
		// substrate timeout_ms — is the real ceiling. It tracks the worst-case
		// invariant (max_iterations + synthesis_retry_budget) × per_llm_timeout:
		// spec 084 set max_iterations=6 and llm_timeout_ms=600s; spec 087 added
		// synthesis_retry_budget=1 stronger-prompt synthesis retry, so
		// (6 + 1) × 600s = 4200s. The forced-final synthesis turn (+ its retry)
		// runs the reasoning synthesis model bounded by the same llm_timeout_ms,
		// so the envelope stays honest. Spec 088 — a per-request model override
		// (--model= / API model) only SWAPS which model occupies the synthesis
		// seat; it adds NO turns and changes neither max_iterations nor
		// synthesis_retry_budget, so a switched (even slower) synthesis model is
		// bounded by the SAME 4200s envelope — NO value change. A first-class
		// compare-both affordance (deferred, F-COMPARE-LATENCY) WOULD run two
		// full passes and double the bound to 8400s; if it is ever shipped this
		// value MUST be re-derived. Realistic GPU / home-lab turns complete in
		// ~40-90s; this is the pathological-slow-CPU-dev backstop. If an
		// operator raises max_iterations or synthesis_retry_budget, recompute.
		// Spec 089 — the persistent deepseek-r1:32b default + a per-request
		// gather override (--gather-model= / API gather_model) + a per-user
		// sticky preference all only SWAP which model occupies the synthesis or
		// gather seat on the EXISTING turns; none adds a turn or changes
		// max_iterations / synthesis_retry_budget, so the 4200s envelope is
		// unchanged (NFR-2). The 32b default changes TYPICAL latency (~1.9×),
		// not the MAX. F-RETRYBUDGET (raising synthesis_retry_budget 1→2 →
		// (6+2)×600s = 4800s) is the only deferred knob that would re-derive it.
		WriteTimeout: 4200 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Spec 037 Scope 10 — SIGHUP triggers an agent.Bridge reload so
	// scenario YAML changes (BS-001) and hot-reload pinning (BS-019)
	// can be exercised without restarting the runtime.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			if agentBridge == nil {
				continue
			}
			rejected, err := agentBridge.Reload(ctx)
			if err != nil {
				slog.Warn("agent bridge reload failed", "error", err)
				continue
			}
			for _, r := range rejected {
				slog.Warn("agent scenario rejected on reload", "path", r.Path, "message", r.Message)
			}
			slog.Info("agent bridge reloaded", "scenario_count", len(agentBridge.KnownIntents()))
		}
	}()

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errCh:
		return err
	}

	// Explicit sequential shutdown — replaces defer-based ordering to prevent
	// resource races (e.g., NATS drain racing DB pool close).
	// Timeout budget: cfg.ShutdownTimeoutS with 5s margin before Docker SIGKILL.
	shutdownAll(cfg.ShutdownTimeoutS, sched, srv, tgBot, svc.ntfyRuntime, svc.resultSub, svc.synthesisSub, svc.domainSub, svc.supervisor, svc.nc, svc.pg, svc.assistantTracerShutdown)

	slog.Info("smackerel-core stopped")
	return nil
}

// runAssistantFacadeWiringWithRetry invokes wireAssistantFacade in a
// loop with bounded exponential backoff until it succeeds or ctx is
// canceled. The wiring step pre-computes scenario embeddings via the
// ML sidecar's /embed endpoint; on a cold-start where ml-sidecar is
// still booting it can transiently fail, and we must not bring down
// the core HTTP listener for that. SST misconfiguration surfaces as
// repeated structured error logs instead of process exit; operators
// notice via /api/assistant/turn returning 503 "assistant_http_not_ready"
// indefinitely combined with the error log stream.
func runAssistantFacadeWiringWithRetry(
	ctx context.Context,
	cfg *config.Config,
	svc *coreServices,
	agentRT *agentRuntime,
	tgBot *telegram.Bot,
	scenarioDir string,
) {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second
	attempt := 0
	for {
		attempt++
		err := wireAssistantFacade(ctx, cfg, svc, agentRT, tgBot, scenarioDir)
		if err == nil {
			if attempt > 1 {
				slog.Info("assistant facade wired after deferred retries", "attempts", attempt)
			}
			return
		}
		slog.Error("assistant facade wiring failed; will retry in background",
			"attempt", attempt, "backoff", backoff.String(), "error", err)
		select {
		case <-ctx.Done():
			slog.Warn("assistant facade wiring abandoned (shutdown)",
				"attempts", attempt, "last_error", err)
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
