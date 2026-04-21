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

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/list"
	"github.com/smackerel/smackerel/internal/mealplan"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
)

// version, commitHash, and buildTime are set by -ldflags at build time.
var (
	version    = "dev"
	commitHash = "unknown"
	buildTime  = "unknown"
)

func main() {
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

	// Configure structured logging
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	if cfg.AuthToken == "" {
		slog.Warn("SMACKEREL_AUTH_TOKEN is empty — system running without authentication")
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

	// Set up API
	deps := &api.Dependencies{
		DB:                              svc.pg,
		NATS:                            svc.nc,
		IntelligenceEngine:              svc.intEngine,
		StartTime:                       time.Now(),
		MLSidecarURL:                    cfg.MLSidecarURL,
		Pipeline:                        svc.proc,
		SearchEngine:                    svc.searchEngine,
		DigestGen:                       svc.digestGen,
		WebHandler:                      svc.webHandler,
		OAuthHandler:                    svc.oauthHandler,
		ContextHandler:                  svc.contextHandler,
		ArtifactStore:                   svc.pg,
		OllamaURL:                       cfg.OllamaURL,
		AuthToken:                       cfg.AuthToken,
		ConnectorRegistry:               svc.registry,
		Version:                         version,
		CommitHash:                      commitHash,
		BuildTime:                       buildTime,
		KnowledgeStore:                  svc.knowledgeStore,
		KnowledgeConceptSearchThreshold: cfg.KnowledgeConceptSearchThreshold,
		KnowledgeHealthCacheTTL:         time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
		CORSAllowedOrigins:              cfg.CORSAllowedOrigins,
	}

	// Wire annotation handlers (spec 027)
	annotationStore := annotation.NewStore(svc.pg.Pool, svc.nc)
	deps.AnnotationHandlers = &api.AnnotationHandlers{Store: annotationStore}

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	var tgBot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		var err error
		tgBot, err = telegram.NewBot(telegram.Config{
			BotToken:                     cfg.TelegramBotToken,
			ChatIDs:                      cfg.TelegramChatIDs,
			CoreAPIURL:                   cfg.CoreAPIURL,
			AuthToken:                    cfg.AuthToken,
			AssemblyWindowSeconds:        cfg.TelegramAssemblyWindowSeconds,
			AssemblyMaxMessages:          cfg.TelegramAssemblyMaxMessages,
			MediaGroupWindowSeconds:      cfg.TelegramMediaGroupWindowSeconds,
			DisambiguationTimeoutSeconds: cfg.TelegramDisambiguationTimeoutSeconds,
			CookSessionTimeoutMinutes:    cfg.TelegramCookSessionTimeoutMinutes,
			CookSessionMaxPerChat:        cfg.TelegramCookSessionMaxPerChat,
		})
		if err != nil {
			slog.Warn("telegram bot initialization failed", "error", err)
		} else {
			tgBot.Start(ctx)
			deps.TelegramBot = tgBot
			slog.Info("telegram bot started")
		}
	}

	// Start digest scheduler + intelligence jobs
	sched := scheduler.New(svc.digestGen, tgBot, svc.intEngine, svc.topicLifecycle)

	// Subscribe intelligence engine to annotation events (spec 027)
	if svc.intEngine != nil {
		if err := svc.intEngine.SubscribeAnnotations(ctx); err != nil {
			slog.Warn("annotation subscription failed", "error", err)
		}
	}

	// Wire knowledge linter into scheduler if knowledge layer is enabled
	if cfg.KnowledgeEnabled && svc.knowledgeStore != nil {
		linterCfg := knowledge.LinterConfig{
			StaleDays:           cfg.KnowledgeLintStaleDays,
			MaxSynthesisRetries: cfg.KnowledgeMaxSynthesisRetries,
		}
		knowledgeLinter := knowledge.NewLinter(svc.knowledgeStore, svc.pg.Pool, linterCfg, svc.nc)
		sched.SetKnowledgeLinter(knowledgeLinter, cfg.KnowledgeLintCron)
		slog.Info("knowledge linter configured", "cron", cfg.KnowledgeLintCron,
			"stale_days", cfg.KnowledgeLintStaleDays,
			"max_retries", cfg.KnowledgeMaxSynthesisRetries,
		)
	}

	// Wire expense tracking services (spec 034)
	if cfg.ExpensesEnabled {
		expenseClassifier := intelligence.NewExpenseClassifier(svc.pg.Pool, cfg)

		// Seed vendor aliases on startup (idempotent)
		if err := expenseClassifier.SeedVendorAliases(ctx); err != nil {
			slog.Warn("failed to seed vendor aliases", "error", err)
		}

		expenseHandler := api.NewExpenseHandler(svc.pg.Pool, expenseClassifier, cfg)
		deps.ExpenseHandler = expenseHandler
		slog.Info("expense tracking enabled",
			"default_currency", cfg.ExpensesDefaultCurrency,
			"export_max_rows", cfg.ExpensesExportMaxRows,
		)
	}

	// Wire meal planning services (spec 036)
	if cfg.MealPlanEnabled {
		mealPlanStore := mealplan.NewStore(svc.pg.Pool)
		mealPlanService := mealplan.NewService(
			mealPlanStore,
			cfg.MealPlanMealTypes,
			cfg.MealPlanDefaultServings,
			cfg.MealPlanCalendarSync,
			cfg.MealPlanAutoComplete,
			cfg.MealPlanAutoCompleteCron,
		)

		// Build shopping bridge using existing list infrastructure (spec 028)
		resolver := list.NewPostgresArtifactResolver(svc.pg.Pool)
		listStore := list.NewStore(svc.pg.Pool)
		aggregator := &list.RecipeAggregator{}
		shoppingBridge := mealplan.NewShoppingBridge(resolver, aggregator, listStore)

		mealPlanHandler := api.NewMealPlanHandler(mealPlanService, shoppingBridge, nil)
		deps.MealPlanHandler = mealPlanHandler

		// Wire auto-complete scheduler
		if cfg.MealPlanAutoComplete && cfg.MealPlanAutoCompleteCron != "" {
			sched.SetMealPlanAutoComplete(mealPlanService, cfg.MealPlanAutoCompleteCron)
			slog.Info("meal plan auto-complete configured", "cron", cfg.MealPlanAutoCompleteCron)
		}

		// Wire Telegram meal plan handler
		if tgBot != nil {
			tgMealPlan := telegram.NewMealPlanCommandHandler(mealPlanService, shoppingBridge)
			tgMealPlan.CookDelegate = func(chatID int64, recipeName string, servings int) {
				tgBot.TriggerCookMode(chatID, recipeName, servings)
			}
			tgMealPlan.RecipeResolver = func(ctx context.Context, name string) (string, string, error) {
				rd, artifactID, err := tgBot.ResolveRecipeByName(ctx, name)
				if err != nil {
					return "", "", err
				}
				return artifactID, rd.Title, nil
			}
			tgBot.SetMealPlanHandler(tgMealPlan)
			slog.Info("telegram meal plan handler configured")
		}

		slog.Info("meal planning enabled",
			"meal_types", cfg.MealPlanMealTypes,
			"default_servings", cfg.MealPlanDefaultServings,
			"calendar_sync", cfg.MealPlanCalendarSync,
		)
	}

	if err := sched.Start(ctx, cfg.DigestCron); err != nil {
		slog.Warn("digest scheduler failed to start", "error", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errCh:
		return err
	}

	// Explicit sequential shutdown — replaces defer-based ordering to prevent
	// resource races (e.g., NATS drain racing DB pool close).
	// Timeout budget: cfg.ShutdownTimeoutS with 5s margin before Docker SIGKILL.
	shutdownAll(cfg.ShutdownTimeoutS, sched, srv, tgBot, svc.resultSub, svc.synthesisSub, svc.domainSub, svc.supervisor, svc.nc, svc.pg)

	slog.Info("smackerel-core stopped")
	return nil
}
