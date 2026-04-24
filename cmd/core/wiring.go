package main

import (
	"context"
	"log/slog"
	"os"
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

// configureLogging sets up the global slog handler based on cfg.LogLevel.
func configureLogging(cfg *config.Config) {
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
}

// buildAPIDeps assembles the api.Dependencies struct including annotation and
// actionable-list handlers (specs 027 and 028). It returns the deps plus the
// list resolver and store so callers can reuse them when wiring meal planning.
func buildAPIDeps(cfg *config.Config, svc *coreServices) (*api.Dependencies, list.ArtifactResolver, *list.Store) {
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

	// Wire actionable list handlers (spec 028)
	listResolver := list.NewPostgresArtifactResolver(svc.pg.Pool)
	listStore := list.NewStore(svc.pg.Pool, svc.nc)
	listAggregators := map[string]list.Aggregator{
		"recipe":  &list.RecipeAggregator{},
		"reading": &list.ReadingAggregator{},
		"product": &list.CompareAggregator{},
	}
	listGenerator := list.NewGenerator(listResolver, listStore, listAggregators)
	deps.ListHandlers = &api.ListHandlers{
		Generator: listGenerator,
		Store:     listStore,
	}
	slog.Info("actionable list handlers configured")

	return deps, listResolver, listStore
}

// startTelegramBotIfConfigured creates and starts the Telegram bot when a
// bot token is configured. Returns nil when Telegram is disabled or fails.
func startTelegramBotIfConfigured(ctx context.Context, cfg *config.Config, deps *api.Dependencies) *telegram.Bot {
	if cfg.TelegramBotToken == "" {
		return nil
	}
	tgBot, err := telegram.NewBot(telegram.Config{
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
		return nil
	}
	tgBot.Start(ctx)
	deps.TelegramBot = tgBot
	slog.Info("telegram bot started")
	return tgBot
}

// wireKnowledgeLinter attaches the knowledge linter to the scheduler when the
// knowledge layer is enabled.
func wireKnowledgeLinter(sched *scheduler.Scheduler, cfg *config.Config, svc *coreServices) {
	if !cfg.KnowledgeEnabled || svc.knowledgeStore == nil {
		return
	}
	linterCfg := knowledge.LinterConfig{
		StaleDays:                cfg.KnowledgeLintStaleDays,
		MaxSynthesisRetries:      cfg.KnowledgeMaxSynthesisRetries,
		PromptContractVersion:    cfg.KnowledgePromptContractIngestSynthesis,
		MaxSynthesisContextItems: 50,
		MaxSynthesisContentChars: 8000,
	}
	knowledgeLinter := knowledge.NewLinter(svc.knowledgeStore, svc.pg.Pool, linterCfg, svc.nc)
	sched.SetKnowledgeLinter(knowledgeLinter, cfg.KnowledgeLintCron)
	slog.Info("knowledge linter configured", "cron", cfg.KnowledgeLintCron,
		"stale_days", cfg.KnowledgeLintStaleDays,
		"max_retries", cfg.KnowledgeMaxSynthesisRetries,
	)
}

// wireExpenseTracking wires the expense tracking handler when enabled (spec 034).
func wireExpenseTracking(ctx context.Context, cfg *config.Config, svc *coreServices, deps *api.Dependencies) {
	if !cfg.ExpensesEnabled {
		return
	}
	expenseClassifier := intelligence.NewExpenseClassifier(svc.pg.Pool, cfg)

	// Seed vendor aliases on startup (idempotent)
	if err := expenseClassifier.SeedVendorAliases(ctx); err != nil {
		slog.Warn("failed to seed vendor aliases", "error", err)
	}

	deps.ExpenseHandler = api.NewExpenseHandler(svc.pg.Pool, expenseClassifier, cfg)
	slog.Info("expense tracking enabled",
		"default_currency", cfg.ExpensesDefaultCurrency,
		"export_max_rows", cfg.ExpensesExportMaxRows,
	)
}

// wireMealPlanning wires meal-planning services (spec 036): the API handler,
// scheduler auto-complete job, and the Telegram meal-plan command handler.
func wireMealPlanning(
	cfg *config.Config,
	svc *coreServices,
	deps *api.Dependencies,
	sched *scheduler.Scheduler,
	listResolver list.ArtifactResolver,
	listStore *list.Store,
	tgBot *telegram.Bot,
) {
	if !cfg.MealPlanEnabled {
		return
	}
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
	shoppingBridge := mealplan.NewShoppingBridge(listResolver, &list.RecipeAggregator{}, listStore)

	deps.MealPlanHandler = api.NewMealPlanHandler(mealPlanService, shoppingBridge, nil)

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
