package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/confirm"
	"github.com/smackerel/smackerel/internal/drive/google"
	drivepolicy "github.com/smackerel/smackerel/internal/drive/policy"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	drivetools "github.com/smackerel/smackerel/internal/drive/tools"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/list"
	"github.com/smackerel/smackerel/internal/mealplan"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/web"
)

// configureLogging sets up the global slog handler based on cfg.LogLevel.
// MIT-040-S-004 — also enforces the SMACKEREL_ENV-conditional auth-token
// contract: in the production environment an empty SMACKEREL_AUTH_TOKEN is
// fatal (returns a non-nil error so main.go exits with a non-zero code);
// in development/test it is logged at WARN level and the runtime continues
// in dev-mode bypass.
func configureLogging(cfg *config.Config) error {
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
		if cfg.Environment == "production" {
			return fmt.Errorf("config: SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production")
		}
		slog.Warn("SMACKEREL_AUTH_TOKEN is empty — auth bypassed (dev-mode)", "environment", cfg.Environment)
	}
	return nil
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
		Environment:                     cfg.Environment,
		ConnectorRegistry:               svc.registry,
		Version:                         version,
		CommitHash:                      commitHash,
		BuildTime:                       buildTime,
		KnowledgeStore:                  svc.knowledgeStore,
		KnowledgeConceptSearchThreshold: cfg.KnowledgeConceptSearchThreshold,
		KnowledgeHealthCacheTTL:         time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
		CORSAllowedOrigins:              cfg.CORSAllowedOrigins,
		AgentAdminHandler:               web.NewAgentAdminHandler(svc.pg.Pool),
		DriveHandlers:                   api.NewDriveHandlersWithPool(drive.DefaultRegistry, svc.pg.Pool),
		PhotosHandlers:                  api.NewPhotosHandlers(photolib.NewStore(svc.pg.Pool), cfg.Photos, cfg.Environment),
		RecommendationHandlers:          api.NewRecommendationHandlers(svc.recommendationStore, svc.recommendationRegistry, cfg.Recommendations),
		RecommendationWatchHandlers:     api.NewRecommendationWatchHandlers(svc.recommendationStore),
	}

	if provider, ok := drive.DefaultRegistry.Get("google"); ok {
		if googleProvider, ok := provider.(*google.Provider); ok {
			caps := googleProvider.Capabilities()
			caps.MaxFileSizeBytes = cfg.Drive.Limits.MaxFileSizeBytes
			googleProvider.Configure(caps)
			googleProvider.ConfigureRuntime(svc.pg.Pool, http.DefaultClient, cfg.Drive.Providers.Google)
		} else {
			slog.Warn("registered google drive provider has unexpected type", "type", "not *google.Provider")
		}
	} else {
		slog.Warn("google drive provider is not registered")
	}

	// Spec 038 Scope 5 — wire Save Rules + Save Service against the
	// drive.DefaultRegistry. The Save Service runs in-process; HTTP
	// handlers and Telegram/meal-plan bridges all share this instance so
	// idempotency keys, drive_save_requests rows, and folder mappings
	// remain coherent across surfaces.
	saveResolver := api.NewProviderResolverAdapter(drive.DefaultRegistry)
	saveService := save.NewService(svc.pg.Pool, saveResolver, cfg.Drive.Save.ProviderURLPrefix)
	deps.DriveRulesHandlers = api.NewDriveRulesHandlers(svc.pg.Pool, saveResolver)
	deps.DriveSaveHandlers = api.NewDriveSaveHandlers(svc.pg.Pool, saveService)
	svc.driveSaveService = saveService
	slog.Info("drive save service wired",
		"provider_url_prefix", cfg.Drive.Save.ProviderURLPrefix)

	// Spec 038 Scope 7 — wire Retrieval Service against the same pool
	// and provider registry. The provider lookup is injected as a pure
	// function so the retrieve package does not depend on internal/drive
	// (which would create an import cycle once internal/drive/tools
	// registers agent tools that import retrieve).
	retrieveSearcher := retrieve.NewPostgresSearcher(svc.pg.Pool)
	retrieveFetcher := retrieve.NewProviderBytesFetcher(svc.pg.Pool, func(ctx context.Context, providerID, connectionID, providerFileID string) (io.ReadCloser, string, error) {
		provider, ok := drive.DefaultRegistry.Get(providerID)
		if !ok {
			return nil, "", fmt.Errorf("retrieve wiring: provider %q not registered", providerID)
		}
		body, err := provider.GetFile(ctx, connectionID, providerFileID)
		if err != nil {
			return nil, "", err
		}
		return body.Reader, body.MimeType, nil
	})
	retrievePolicy := drivepolicy.NewEngine()
	retrieveService := retrieve.NewService(
		retrieveSearcher,
		retrieveFetcher,
		retrievePolicy,
		cfg.Drive.Telegram.MaxInlineSizeBytes,
		retrieve.DefaultReasonTable(),
	)
	svc.driveRetrieveService = retrieveService
	slog.Info("drive retrieve service wired",
		"max_inline_size_bytes", cfg.Drive.Telegram.MaxInlineSizeBytes,
		"max_link_files_per_reply", cfg.Drive.Telegram.MaxLinkFilesPerReply,
	)

	// Spec 037 + 038 Scope 7 — wire the four drive agent tools so the
	// scenario-agent runtime can call them through the registry. The
	// tools are read/external; agent traces inherit the same policy
	// refusals (BS-025) that the HTTP and Telegram surfaces enforce.
	drivetools.SetToolServices(&drivetools.ToolServices{
		Retriever:   retrieveService,
		SaveService: saveService,
		RulesRepo:   rules.NewRepository(svc.pg.Pool),
		RulesEngine: rules.NewEngine(time.Now),
		Policy:      retrievePolicy,
	})
	slog.Info("drive agent tools wired",
		"tools", drivetools.ToolNames,
	)

	// Spec 038 Scope 6 — wire low-confidence confirmations store and
	// HTTP handler. The store backs both Screen 11 web modal and the
	// Telegram numbered-reply path; both flow through
	// /v1/drive/confirmations/{id}.
	confirmTTL := time.Duration(cfg.Drive.Classification.ConfirmationTTLSeconds) * time.Second
	confirmStore := confirm.NewStore(svc.pg.Pool, confirmTTL)
	deps.DriveConfirmationsHandlers = api.NewDriveConfirmationsHandlers(confirmStore)
	slog.Info("drive confirmations handler wired",
		"confirm_threshold", cfg.Drive.Classification.ConfirmThreshold,
		"confirmation_ttl_seconds", cfg.Drive.Classification.ConfirmationTTLSeconds)

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
		// MIT-040-S-006 — SST byte caps for the photo upload path.
		PhotoDownloadMaxBytes:  cfg.Photos.IOLimits.TelegramResponseMaxBytes,
		UploadResponseMaxBytes: cfg.Photos.IOLimits.ProviderMetadataMaxBytes,
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

// attachDriveSaveBridgeToTelegram wires the spec 038 Scope 5 Drive write-back
// bridge to a running Telegram bot. Safe to call when either side is missing.
func attachDriveSaveBridgeToTelegram(svc *coreServices, tgBot *telegram.Bot) {
	if svc == nil || tgBot == nil || svc.driveSaveService == nil {
		return
	}
	bridge := telegram.NewDriveSaveBridge(
		svc.pg.Pool,
		rules.NewRepository(svc.pg.Pool),
		rules.NewEngine(time.Now),
		svc.driveSaveService,
	)
	tgBot.SetDriveSaveBridge(bridge)
	slog.Info("telegram drive save bridge wired")
}

// attachDriveRetrieveBridgeToTelegram wires the spec 038 Scope 7 Drive
// retrieval bridge to a running Telegram bot. Safe to call when either
// side is missing. The bridge enables "send me X" style prompts to flow
// through retrieve.Service under the same policy contract the HTTP API
// uses.
func attachDriveRetrieveBridgeToTelegram(svc *coreServices, tgBot *telegram.Bot) {
	if svc == nil || tgBot == nil || svc.driveRetrieveService == nil {
		return
	}
	bridge := telegram.NewDriveRetrieveBridge(svc.driveRetrieveService)
	tgBot.SetDriveRetrieveBridge(bridge)
	slog.Info("telegram drive retrieve bridge wired")
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

	// Spec 038 Scope 5 — wire meal-plan Drive write-back if the Save
	// Service is configured. The bridge is purely additive: callers who
	// don't trigger SavePlan don't pay any cost.
	if svc.driveSaveService != nil {
		mealPlanSaveBack := mealplan.NewDriveSaveBack(
			svc.pg.Pool,
			rules.NewRepository(svc.pg.Pool),
			rules.NewEngine(time.Now),
			svc.driveSaveService,
			mealPlanStore,
		)
		svc.mealPlanSaveBack = mealPlanSaveBack
		slog.Info("meal plan drive write-back wired")
	}

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
