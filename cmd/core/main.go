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
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	alertsConnector "github.com/smackerel/smackerel/internal/connector/alerts"
	bookmarksConnector "github.com/smackerel/smackerel/internal/connector/bookmarks"
	browserConnector "github.com/smackerel/smackerel/internal/connector/browser"
	caldavConnector "github.com/smackerel/smackerel/internal/connector/caldav"
	discordConnector "github.com/smackerel/smackerel/internal/connector/discord"
	guesthostConnector "github.com/smackerel/smackerel/internal/connector/guesthost"
	hospitableConnector "github.com/smackerel/smackerel/internal/connector/hospitable"
	imapConnector "github.com/smackerel/smackerel/internal/connector/imap"
	keepConnector "github.com/smackerel/smackerel/internal/connector/keep"
	mapsConnector "github.com/smackerel/smackerel/internal/connector/maps"
	marketsConnector "github.com/smackerel/smackerel/internal/connector/markets"
	rssConnector "github.com/smackerel/smackerel/internal/connector/rss"
	twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
	weatherConnector "github.com/smackerel/smackerel/internal/connector/weather"
	youtubeConnector "github.com/smackerel/smackerel/internal/connector/youtube"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/graph"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/topics"
	"github.com/smackerel/smackerel/internal/web"
)

// version and commitHash are set by -ldflags at build time.
var (
	version    = "dev"
	commitHash = "unknown"
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

	slog.Info("starting smackerel-core", "port", cfg.Port, "version", version, "commit", commitHash)

	// Connect to PostgreSQL
	pg, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	// pg.Close() is called in shutdownAll() — no defer here

	// Run schema migrations
	if err := db.Migrate(ctx, pg.Pool); err != nil {
		return fmt.Errorf("database migration: %w", err)
	}
	slog.Info("database migrations complete")

	// Connect to NATS
	nc, err := smacknats.Connect(ctx, cfg.NATSURL, cfg.AuthToken)
	if err != nil {
		return fmt.Errorf("NATS connection: %w", err)
	}
	// nc.Close() is called in shutdownAll() — no defer here

	// Ensure JetStream streams
	if err := nc.EnsureStreams(ctx); err != nil {
		return fmt.Errorf("NATS stream setup: %w", err)
	}
	slog.Info("NATS JetStream streams configured")

	// Create hospitality graph repositories and linker
	guestRepo := db.NewGuestRepository(pg.Pool)
	propertyRepo := db.NewPropertyRepository(pg.Pool)
	hospitalityLinker := graph.NewHospitalityLinker(guestRepo, propertyRepo, pg.Pool, graph.NewLinker(pg.Pool))

	// Seed hospitality topics (idempotent — safe to call on every startup)
	if err := graph.SeedHospitalityTopics(ctx, pg.Pool); err != nil {
		slog.Warn("failed to seed hospitality topics", "error", err)
	}

	// Create connector registry (used by digest generator for hospitality detection)
	registry := connector.NewRegistry()

	// Start result subscriber for ML processing results
	resultSub := pipeline.NewResultSubscriber(pg.Pool, nc, registry)
	resultSub.Processor.HospitalityLinker = hospitalityLinker

	// Wire knowledge synthesis into pipeline if enabled
	var knowledgeStore *knowledge.KnowledgeStore
	var synthesisSub *pipeline.SynthesisResultSubscriber
	if cfg.KnowledgeEnabled {
		knowledgeStore = knowledge.NewKnowledgeStore(pg.Pool)
		resultSub.KnowledgeEnabled = true
		resultSub.KnowledgeStore = knowledgeStore
		resultSub.PromptContractVersion = cfg.KnowledgePromptContractIngestSynthesis
		slog.Info("knowledge synthesis pipeline enabled",
			"contract", cfg.KnowledgePromptContractIngestSynthesis,
		)

		synthesisSub = pipeline.NewSynthesisResultSubscriber(pg.Pool, nc, knowledgeStore)
		synthesisSub.CrossSourceConfidenceThreshold = cfg.KnowledgeCrossSourceConfidenceThreshold
		synthesisSub.CrossSourcePromptContractVersion = cfg.KnowledgePromptContractCrossSource
	}

	if err := resultSub.Start(ctx); err != nil {
		return fmt.Errorf("start result subscriber: %w", err)
	}

	// Start synthesis result subscriber after NATS streams are ready
	if synthesisSub != nil {
		if err := synthesisSub.Start(ctx); err != nil {
			slog.Warn("synthesis result subscriber failed to start", "error", err)
		}
	}

	// Create pipeline processor
	proc := pipeline.NewProcessor(pg.Pool, nc)
	proc.HospitalityLinker = hospitalityLinker

	// Create search engine
	searchEngine := &api.SearchEngine{
		Pool:           pg.Pool,
		NATS:           nc,
		MLSidecarURL:   cfg.MLSidecarURL,
		HealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
	}

	// Create digest generator
	digestGen := digest.NewGenerator(pg.Pool, nc, registry)
	digestGen.KnowledgeEnabled = cfg.KnowledgeEnabled

	// Create intelligence engine for synthesis, alerts, and resurfacing
	intEngine := intelligence.NewEngine(pg.Pool, nc)

	// Create topic lifecycle manager for momentum tracking
	topicLifecycle := topics.NewLifecycle(pg.Pool)

	// Create and start connector supervisor
	stateStore := connector.NewStateStore(pg.Pool)
	supervisor := connector.NewSupervisor(registry, stateStore)

	// Wire artifact publisher so connector-produced RawArtifacts flow into the NATS pipeline
	artifactPublisher := pipeline.NewRawArtifactPublisher(pg.Pool, nc)
	supervisor.SetPublisher(artifactPublisher)

	// Register connectors and start those with valid OAuth tokens
	imapConn := imapConnector.New("gmail")
	caldavConn := caldavConnector.New("google-calendar")
	ytConn := youtubeConnector.New("youtube")
	rssConn := rssConnector.New("rss", nil) // feed URLs configured via source_config
	keepConn := keepConnector.New("google-keep")
	bmConn := bookmarksConnector.NewConnectorWithPool("bookmarks", pg.Pool)
	browserHistConn := browserConnector.New("browser-history")
	mapsConn := mapsConnector.New("google-maps-timeline")
	hospitableConn := hospitableConnector.New("hospitable")
	guesthostConn := guesthostConnector.New()
	discordConn := discordConnector.New("discord")
	twitterConn := twitterConnector.New("twitter")
	weatherConn := weatherConnector.New("weather")
	alertsConn := alertsConnector.New("gov-alerts")
	marketsConn := marketsConnector.New("financial-markets")
	for _, c := range []connector.Connector{
		imapConn, caldavConn, ytConn, rssConn, keepConn,
		bmConn, browserHistConn, mapsConn, hospitableConn, guesthostConn,
		discordConn, twitterConn, weatherConn, alertsConn, marketsConn,
	} {
		if err := registry.Register(c); err != nil {
			return fmt.Errorf("register connector %q: %w", c.ID(), err)
		}
	}
	slog.Info("connector registry initialized", "count", registry.Count())

	// Auto-start bookmarks connector (no OAuth needed — file-based import)
	if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {
		bmConfig := connector.ConnectorConfig{
			AuthType:       "none",
			Enabled:        true,
			ProcessingTier: "full",
			SyncSchedule:   cfg.BookmarksSyncSchedule,
			SourceConfig: map[string]interface{}{
				"import_dir":        cfg.BookmarksImportDir,
				"archive_processed": false,
			},
		}
		if err := bmConn.Connect(ctx, bmConfig); err == nil {
			supervisor.SetConfig("bookmarks", bmConfig)
			supervisor.StartConnector(ctx, "bookmarks")
			slog.Info("bookmarks connector started", "import_dir", cfg.BookmarksImportDir)
		} else {
			slog.Warn("bookmarks connector failed to start", "error", err)
		}
	}

	// Auto-start browser history connector (no OAuth needed — file-based)
	if cfg.BrowserHistoryPath != "" {
		browserCfg := connector.ConnectorConfig{
			AuthType: "none",
			Enabled:  true,
			SourceConfig: map[string]interface{}{
				"history_path": cfg.BrowserHistoryPath,
			},
		}
		if err := browserHistConn.Connect(ctx, browserCfg); err == nil {
			supervisor.SetConfig("browser-history", browserCfg)
			supervisor.StartConnector(ctx, "browser-history")
			slog.Info("browser history connector started", "history_path", cfg.BrowserHistoryPath)
		} else {
			slog.Warn("browser history connector failed to start", "error", err)
		}
	}

	// Auto-start Google Maps Timeline connector (no OAuth needed — file-based Takeout import)
	if cfg.MapsImportDir != "" {
		mapsCfg := connector.ConnectorConfig{
			AuthType:     "none",
			Enabled:      true,
			SyncSchedule: os.Getenv("MAPS_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"import_dir":               cfg.MapsImportDir,
				"watch_interval":           os.Getenv("MAPS_WATCH_INTERVAL"),
				"archive_processed":        os.Getenv("MAPS_ARCHIVE_PROCESSED") == "true",
				"min_distance_m":           parseFloatEnv("MAPS_MIN_DISTANCE_M"),
				"min_duration_min":         parseFloatEnv("MAPS_MIN_DURATION_MIN"),
				"location_radius_m":        parseFloatEnv("MAPS_LOCATION_RADIUS_M"),
				"home_detection":           os.Getenv("MAPS_HOME_DETECTION"),
				"commute_min_occurrences":  parseFloatEnv("MAPS_COMMUTE_MIN_OCCURRENCES"),
				"commute_window_days":      parseFloatEnv("MAPS_COMMUTE_WINDOW_DAYS"),
				"commute_weekdays_only":    os.Getenv("MAPS_COMMUTE_WEEKDAYS_ONLY") == "true",
				"trip_min_distance_km":     parseFloatEnv("MAPS_TRIP_MIN_DISTANCE_KM"),
				"trip_min_overnight_hours": parseFloatEnv("MAPS_TRIP_MIN_OVERNIGHT_HOURS"),
				"link_time_extend_min":     parseFloatEnv("MAPS_LINK_TIME_EXTEND_MIN"),
				"link_proximity_radius_m":  parseFloatEnv("MAPS_LINK_PROXIMITY_RADIUS_M"),
			},
		}
		if err := mapsConn.Connect(ctx, mapsCfg); err == nil {
			supervisor.SetConfig("google-maps-timeline", mapsCfg)
			supervisor.StartConnector(ctx, "google-maps-timeline")
			slog.Info("google maps timeline connector started", "import_dir", cfg.MapsImportDir)
		} else {
			slog.Warn("google maps timeline connector failed to start", "error", err)
		}
	}

	// Auto-start Discord connector (token-based)
	if os.Getenv("DISCORD_ENABLED") == "true" {
		discordCfg := connector.ConnectorConfig{
			AuthType:     "token",
			Credentials:  map[string]string{"bot_token": os.Getenv("DISCORD_BOT_TOKEN")},
			Enabled:      true,
			SyncSchedule: os.Getenv("DISCORD_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"enable_gateway":     os.Getenv("DISCORD_ENABLE_GATEWAY") == "true",
				"backfill_limit":     parseFloatEnv("DISCORD_BACKFILL_LIMIT"),
				"include_threads":    os.Getenv("DISCORD_INCLUDE_THREADS") == "true",
				"include_pins":       os.Getenv("DISCORD_INCLUDE_PINS") == "true",
				"capture_commands":   parseJSONArrayEnv("DISCORD_CAPTURE_COMMANDS"),
				"monitored_channels": parseJSONArrayEnv("DISCORD_MONITORED_CHANNELS"),
			},
		}
		if err := discordConn.Connect(ctx, discordCfg); err == nil {
			supervisor.SetConfig("discord", discordCfg)
			supervisor.StartConnector(ctx, "discord")
			slog.Info("discord connector started")
		} else {
			slog.Warn("discord connector failed to start", "error", err)
		}
	}

	// Auto-start Twitter/X connector (token or file-based)
	if os.Getenv("TWITTER_ENABLED") == "true" {
		twitterCfg := connector.ConnectorConfig{
			AuthType:     "token",
			Credentials:  map[string]string{"bearer_token": os.Getenv("TWITTER_BEARER_TOKEN")},
			Enabled:      true,
			SyncSchedule: os.Getenv("TWITTER_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"sync_mode":   os.Getenv("TWITTER_SYNC_MODE"),
				"archive_dir": os.Getenv("TWITTER_ARCHIVE_DIR"),
			},
		}
		if err := twitterConn.Connect(ctx, twitterCfg); err == nil {
			supervisor.SetConfig("twitter", twitterCfg)
			supervisor.StartConnector(ctx, "twitter")
			slog.Info("twitter connector started")
		} else {
			slog.Warn("twitter connector failed to start", "error", err)
		}
	}

	// Auto-start Weather connector (no auth — Open-Meteo is free)
	if os.Getenv("WEATHER_ENABLED") == "true" {
		weatherCfg := connector.ConnectorConfig{
			AuthType:     "none",
			Enabled:      true,
			SyncSchedule: os.Getenv("WEATHER_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"locations":     parseJSONArrayEnv("WEATHER_LOCATIONS"),
				"enable_alerts": os.Getenv("WEATHER_ENABLE_ALERTS") == "true",
				"forecast_days": parseFloatEnv("WEATHER_FORECAST_DAYS"),
				"precision":     parseFloatEnv("WEATHER_PRECISION"),
			},
		}
		if err := weatherConn.Connect(ctx, weatherCfg); err == nil {
			supervisor.SetConfig("weather", weatherCfg)
			supervisor.StartConnector(ctx, "weather")
			slog.Info("weather connector started")
		} else {
			slog.Warn("weather connector failed to start", "error", err)
		}
	}

	// Auto-start Gov Alerts connector (no auth — USGS/NWS are free)
	if os.Getenv("GOV_ALERTS_ENABLED") == "true" {
		// Wire proactive alert notifier to publish extreme/severe alerts to NATS
		alertsConn.Notifier = &alertsConnector.NATSAlertNotifier{
			PublishFn: nc.Publish,
			Subject:   smacknats.SubjectAlertsNotify,
		}

		alertsCfg := connector.ConnectorConfig{
			AuthType:     "api_key",
			Credentials:  map[string]string{"airnow_api_key": os.Getenv("GOV_ALERTS_AIRNOW_API_KEY")},
			Enabled:      true,
			SyncSchedule: os.Getenv("GOV_ALERTS_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"locations":                parseJSONArrayEnv("GOV_ALERTS_LOCATIONS"),
				"min_earthquake_magnitude": parseFloatEnv("GOV_ALERTS_MIN_EARTHQUAKE_MAG"),
				"travel_locations":         parseJSONArrayEnv("GOV_ALERTS_TRAVEL_LOCATIONS"),
				"source_earthquake":        os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true",
				"source_weather":           os.Getenv("GOV_ALERTS_SOURCE_WEATHER") == "true",
				"source_tsunami":           os.Getenv("GOV_ALERTS_SOURCE_TSUNAMI") == "true",
				"source_volcano":           os.Getenv("GOV_ALERTS_SOURCE_VOLCANO") == "true",
				"source_wildfire":          os.Getenv("GOV_ALERTS_SOURCE_WILDFIRE") == "true",
				"source_airnow":            os.Getenv("GOV_ALERTS_SOURCE_AIRNOW") == "true",
				"source_gdacs":             os.Getenv("GOV_ALERTS_SOURCE_GDACS") == "true",
			},
		}
		if err := alertsConn.Connect(ctx, alertsCfg); err == nil {
			supervisor.SetConfig("gov-alerts", alertsCfg)
			supervisor.StartConnector(ctx, "gov-alerts")
			slog.Info("gov-alerts connector started")
		} else {
			slog.Warn("gov-alerts connector failed to start", "error", err)
		}
	}

	// Auto-start Financial Markets connector (API key auth)
	if os.Getenv("FINANCIAL_MARKETS_ENABLED") == "true" {
		marketsCfg := connector.ConnectorConfig{
			AuthType: "api_key",
			Credentials: map[string]string{
				"finnhub_api_key": os.Getenv("FINANCIAL_MARKETS_FINNHUB_API_KEY"),
				"fred_api_key":    os.Getenv("FINANCIAL_MARKETS_FRED_API_KEY"),
			},
			Enabled:      true,
			SyncSchedule: os.Getenv("FINANCIAL_MARKETS_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"watchlist":         parseJSONObjectEnv("FINANCIAL_MARKETS_WATCHLIST"),
				"alert_threshold":   parseFloatEnv("FINANCIAL_MARKETS_ALERT_THRESHOLD"),
				"coingecko_enabled": os.Getenv("FINANCIAL_MARKETS_COINGECKO_ENABLED") == "true",
				"fred_enabled":      os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
				"fred_series":       parseJSONArrayEnv("FINANCIAL_MARKETS_FRED_SERIES"),
			},
		}
		if err := marketsConn.Connect(ctx, marketsCfg); err == nil {
			supervisor.SetConfig("financial-markets", marketsCfg)
			supervisor.StartConnector(ctx, "financial-markets")
			slog.Info("financial-markets connector started")
		} else {
			slog.Warn("financial-markets connector failed to start", "error", err)
		}
	}

	// Set up OAuth handler for connector authorization
	// Auth token is used as the encryption key for OAuth tokens at rest (AES-256-GCM)
	tokenStore := auth.NewTokenStore(pg.Pool, cfg.AuthToken)
	oauthHandler := auth.NewOAuthHandler(tokenStore)
	slog.Info("OAuth handler initialized")

	// Start connectors that have valid OAuth tokens
	if tokenStore.HasToken(ctx, "google") {
		token, err := tokenStore.Get(ctx, "google")
		if err == nil && token != nil {
			creds := map[string]string{"access_token": token.AccessToken}
			imapConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: os.Getenv("IMAP_SYNC_SCHEDULE"),
			}
			if err := imapConn.Connect(ctx, imapConfig); err == nil {
				supervisor.SetConfig("gmail", imapConfig)
				supervisor.StartConnector(ctx, "gmail")
				slog.Info("gmail connector started with OAuth token")
			}
			caldavConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: os.Getenv("CALDAV_SYNC_SCHEDULE"),
			}
			if err := caldavConn.Connect(ctx, caldavConfig); err == nil {
				supervisor.SetConfig("google-calendar", caldavConfig)
				supervisor.StartConnector(ctx, "google-calendar")
				slog.Info("google-calendar connector started with OAuth token")
			}
			ytConfig := connector.ConnectorConfig{
				AuthType:     "oauth2",
				Credentials:  creds,
				Enabled:      true,
				SyncSchedule: os.Getenv("YOUTUBE_SYNC_SCHEDULE"),
			}
			if err := ytConn.Connect(ctx, ytConfig); err == nil {
				supervisor.SetConfig("youtube", ytConfig)
				supervisor.StartConnector(ctx, "youtube")
				slog.Info("youtube connector started with OAuth token")
			}
		}
	} else {
		slog.Info("no Google OAuth token found — connectors will start when user authorizes via /auth/google/start")
	}

	// Create web UI handler
	webHandler := web.NewHandler(pg.Pool, nc, time.Now())
	webHandler.KnowledgeStore = knowledgeStore

	// Create context enrichment handler for GuestHost connector
	contextHandler := api.NewContextHandler(guestRepo, propertyRepo, pg.Pool)

	// Set up API
	deps := &api.Dependencies{
		DB:                              pg,
		NATS:                            nc,
		IntelligenceEngine:              intEngine,
		StartTime:                       time.Now(),
		MLSidecarURL:                    cfg.MLSidecarURL,
		Pipeline:                        proc,
		SearchEngine:                    searchEngine,
		DigestGen:                       digestGen,
		WebHandler:                      webHandler,
		OAuthHandler:                    oauthHandler,
		ContextHandler:                  contextHandler,
		ArtifactStore:                   pg,
		OllamaURL:                       cfg.OllamaURL,
		AuthToken:                       cfg.AuthToken,
		ConnectorRegistry:               registry,
		Version:                         version,
		CommitHash:                      commitHash,
		KnowledgeStore:                  knowledgeStore,
		KnowledgeConceptSearchThreshold: cfg.KnowledgeConceptSearchThreshold,
		KnowledgeHealthCacheTTL:         time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
	}

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	var tgBot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		var err error
		tgBot, err = telegram.NewBot(telegram.Config{
			BotToken:                cfg.TelegramBotToken,
			ChatIDs:                 cfg.TelegramChatIDs,
			CoreAPIURL:              cfg.CoreAPIURL,
			AuthToken:               cfg.AuthToken,
			AssemblyWindowSeconds:   cfg.TelegramAssemblyWindowSeconds,
			AssemblyMaxMessages:     cfg.TelegramAssemblyMaxMessages,
			MediaGroupWindowSeconds: cfg.TelegramMediaGroupWindowSeconds,
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
	sched := scheduler.New(digestGen, tgBot, intEngine, topicLifecycle)

	// Wire knowledge linter into scheduler if knowledge layer is enabled
	if cfg.KnowledgeEnabled && knowledgeStore != nil {
		linterCfg := knowledge.LinterConfig{
			StaleDays:           cfg.KnowledgeLintStaleDays,
			MaxSynthesisRetries: cfg.KnowledgeMaxSynthesisRetries,
		}
		knowledgeLinter := knowledge.NewLinter(knowledgeStore, pg.Pool, linterCfg, nc)
		sched.SetKnowledgeLinter(knowledgeLinter, cfg.KnowledgeLintCron)
		slog.Info("knowledge linter configured", "cron", cfg.KnowledgeLintCron,
			"stale_days", cfg.KnowledgeLintStaleDays,
			"max_retries", cfg.KnowledgeMaxSynthesisRetries,
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
	shutdownAll(cfg.ShutdownTimeoutS, sched, srv, tgBot, resultSub, synthesisSub, supervisor, nc, pg)

	slog.Info("smackerel-core stopped")
	return nil
}
