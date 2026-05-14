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
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
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

	// Spec 044 — Per-user bearer auth foundation. When AUTH_ENABLED=true
	// the runtime MUST refuse to start in production with empty signing
	// material, an empty at-rest hashing key, or a hashing key that
	// equals the signing key (OQ-8). config.Load already validates
	// these cases at the loader boundary; this check is defense-in-
	// depth so a bug in the loader cannot silently let a misconfigured
	// production runtime serve traffic. Delegated to
	// auth.ValidateRuntimeAuthStartup so the contract is unit-testable
	// from outside cmd/core.
	if err := auth.ValidateRuntimeAuthStartup(cfg.Environment, auth.RuntimeAuthConfig{
		Enabled:                 cfg.Auth.Enabled,
		SigningActivePrivateKey: cfg.Auth.SigningActivePrivateKey,
		SigningActiveKeyID:      cfg.Auth.SigningActiveKeyID,
		AtRestHashingKey:        cfg.Auth.AtRestHashingKey,
	}); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

// resolveBroadcasterInstanceID returns the auth-revocation broadcaster's
// per-replica instance identifier derived from the HOSTNAME env var.
//
// Returns an error when HOSTNAME is unset or empty. This is the Gate G028
// fail-loud read closing HL-RESCAN-008: the prior form silently fell back
// to the literal string "smackerel-core", which collided every replica's
// broadcaster identity to the same name and defeated per-replica
// deduplication on the NATS subject. The helper is package-private and
// unit-tested in wiring_revocation_test.go.
func resolveBroadcasterInstanceID() (string, error) {
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		return "", fmt.Errorf("HOSTNAME env var is empty — refusing to construct revocation broadcaster (HL-RESCAN-008 / Gate G028 / spec 044: silent fallback to a literal instance name would defeat per-replica deduplication)")
	}
	return hostname, nil
}

// buildAPIDeps assembles the api.Dependencies struct including annotation and
// actionable-list handlers (specs 027 and 028). It returns the deps plus the
// list resolver and store so callers can reuse them when wiring meal planning.
//
// Spec 044 Scope 02 added an error return — the per-user bearer-auth
// subsystem (BearerStore + RevocationCache + Broadcaster) has fail-fast
// validation paths (e.g. nil pool, malformed PASETO key material) that
// MUST surface to the caller rather than be silently swallowed.
func buildAPIDeps(cfg *config.Config, svc *coreServices) (*api.Dependencies, list.ArtifactResolver, *list.Store, error) {
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
		DriveHandlers:                   api.NewDriveHandlersWithPool(drive.DefaultRegistry, svc.pg.Pool).WithEnvironment(cfg.Environment),
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
	deps.AnnotationHandlers = &api.AnnotationHandlers{
		Store:       annotationStore,
		Environment: cfg.Environment,
	}

	// Spec 044 Scope 02 — wire the per-user bearer-auth subsystem.
	// Production deployments (`auth.enabled=true`) need the verifier
	// options pre-populated with the active+prior public keys so the
	// hot-path middleware does not pay parse cost per request, the
	// revocation cache hydrated from the auth_revocations table for
	// the revocation-on-next-request contract (NFR-AUTH-006), and the
	// NATS broadcaster subscribed so multi-replica deployments see
	// revocation events propagate within the SST-derived window.
	deps.AuthConfig = cfg.Auth
	bearerStore, err := auth.NewBearerStore(svc.pg.Pool)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("auth bearer store: %w", err)
	}
	deps.BearerStore = bearerStore

	revocationCache := revocation.NewCache()
	if cfg.Auth.Enabled {
		bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 10*time.Second)
		count, err := revocationCache.BootstrapFromDB(bootstrapCtx, bearerStore)
		bootstrapCancel()
		if err != nil {
			slog.Error("auth revocation cache bootstrap failed", "error", err)
		} else {
			slog.Info("auth revocation cache bootstrapped",
				"size", count,
				"refresh_interval_seconds", cfg.Auth.RevocationCacheRefreshIntervalSeconds)
		}
	}
	deps.RevocationCache = revocationCache

	if cfg.Auth.Enabled && svc.nc != nil && svc.nc.Conn != nil && cfg.Auth.RevocationNATSSubject != "" {
		// HL-RESCAN-008 / Gate G028 / spec 044 (no-defaults SST policy):
		// the prior form was `instanceID := os.Getenv("HOSTNAME")` followed by
		// `if instanceID == "" { instanceID = "smackerel-core" }`. The literal
		// fallback collided every replica's broadcaster identity to the same
		// string, defeating per-replica deduplication on the NATS subject.
		// Per Gate G028 the read must be `os.Getenv` + empty check → loud
		// refusal, never a hidden default. Helper is unit-tested in
		// wiring_revocation_test.go (truthy returns hostname; empty returns
		// error with HL-RESCAN-008 attribution).
		instanceID, hostnameErr := resolveBroadcasterInstanceID()
		switch {
		case hostnameErr != nil:
			slog.Error("auth revocation broadcaster construction refused",
				"error", hostnameErr,
				"subject", cfg.Auth.RevocationNATSSubject)
		default:
			broadcaster, err := revocation.NewBroadcaster(svc.nc.Conn, cfg.Auth.RevocationNATSSubject, revocationCache, instanceID)
			switch {
			case err != nil:
				slog.Error("auth revocation broadcaster construction failed", "error", err)
			default:
				if subErr := broadcaster.Subscribe(); subErr != nil {
					slog.Error("auth revocation broadcaster subscribe failed", "error", subErr)
				} else {
					slog.Info("auth revocation broadcaster subscribed",
						"subject", cfg.Auth.RevocationNATSSubject,
						"instance_id", instanceID)
					svc.authRevocationBroadcaster = broadcaster
				}
			}
		}
	}

	// Pre-derive the active public key from the configured private
	// key so the hot-path verifier does not re-parse per request. The
	// key derivation is a single elliptic-curve point multiplication;
	// doing it once at startup keeps middleware allocation-free.
	if cfg.Auth.Enabled && cfg.Auth.SigningActivePrivateKey != "" {
		activePub, err := auth.PublicHexFromSecretHex(cfg.Auth.SigningActivePrivateKey)
		if err != nil {
			slog.Error("auth active public key derivation failed", "error", err)
		} else {
			deps.AuthVerifyOptions = auth.VerifyOptions{
				ActivePublicKey:    activePub,
				ActiveKeyID:        cfg.Auth.SigningActiveKeyID,
				PriorPublicKey:     cfg.Auth.SigningPriorPublicKey,
				PriorKeyID:         cfg.Auth.SigningPriorKeyID,
				Issuer:             "smackerel",
				ClockSkewTolerance: time.Duration(cfg.Auth.ClockSkewToleranceSeconds) * time.Second,
				Now:                time.Now,
			}
		}
	}

	authAdmin, err := api.NewAuthAdminHandlers(bearerStore, cfg, svc.authRevocationBroadcaster)
	if err != nil {
		slog.Error("auth admin handlers wiring failed", "error", err)
	} else {
		deps.AuthAdminHandlers = authAdmin
	}

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

	return deps, listResolver, listStore, nil
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
		// Spec 044 Scope 03 — production claim-binding for the
		// Telegram entry point. In production an unmapped chat_id
		// is dropped at handleMessage; in dev/test the empty mapping
		// is acceptable.
		Environment: cfg.Environment,
		UserMapping: cfg.TelegramUserMapping,
	})
	if err != nil {
		slog.Warn("telegram bot initialization failed", "error", err)
		return nil
	}

	// Spec 044 Scope 04 — F02 closure. In production with auth.enabled
	// AND signing-key material present, wire a PerUserTokenMinter so
	// every internal-API call originating from a Telegram chat carries
	// a per-user PASETO bearer (instead of the legacy shared bearer).
	// In dev/test (or when signing material is absent), the minter
	// stays nil and bearerForChat falls back to b.authToken — the
	// existing single-user development workflow keeps functioning.
	if cfg.Environment == "production" && cfg.Auth.Enabled &&
		cfg.Auth.SigningActivePrivateKey != "" && cfg.Auth.SigningActiveKeyID != "" {
		minter, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
			Bot:        tgBot,
			SigningKey: cfg.Auth.SigningActivePrivateKey,
			KeyID:      cfg.Auth.SigningActiveKeyID,
			Issuer:     "smackerel",
			TTL:        5 * time.Minute,
		})
		if err != nil {
			// A nil minter means production telegram traffic falls
			// back to the legacy shared bearer; the deprecation flag
			// (auth.production_shared_token_fallback_enabled) governs
			// whether the middleware accepts that. We log and
			// continue so the bot itself remains operational.
			slog.Warn("telegram per-user token minter setup failed; per-user PASETO disabled",
				"error", err)
		} else {
			tgBot.SetPerUserTokenMinter(minter)
			slog.Info("telegram per-user token minter wired (spec 044 Scope 04 F02 closure)")
		}
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
		tgMealPlan.RecipeResolver = func(ctx context.Context, chatID int64, name string) (string, string, error) {
			rd, artifactID, err := tgBot.ResolveRecipeByName(ctx, chatID, name)
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
