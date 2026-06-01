package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/backup"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"

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
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	tgBot := startTelegramBotIfConfigured(ctx, cfg, deps)
	attachDriveSaveBridgeToTelegram(svc, tgBot)
	attachDriveRetrieveBridgeToTelegram(svc, tgBot)

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

	// Spec 061 SCOPE-05 — construct the capability-layer facade +
	// Telegram reference adapter and bind it to the bot AFTER the
	// bot has started. Fail-loud per SST: when the assistant block
	// is enabled but wiring fails, surface the error so operators
	// see the misconfiguration; when the assistant is disabled this
	// is a no-op.
	if err := wireAssistantFacade(ctx, cfg, svc, agentRT, tgBot, scenarioDir); err != nil {
		return fmt.Errorf("assistant facade wiring: %w", err)
	}

	// Start digest scheduler + intelligence jobs
	sched := scheduler.New(svc.digestGen, tgBot, svc.intEngine, svc.topicLifecycle)

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
		// Spec 064 SCOPE-17 — WriteTimeout sized for the longest legitimate
		// substrate scenario: the open_knowledge agent loop may run up to
		// max_iterations × per_llm_timeout (e.g. 3 × 600s = 30 min) before
		// flushing the final response on CPU-only dev with gemma3:4b.
		// GPU / smaller models complete in seconds.
		WriteTimeout: 1800 * time.Second,
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
