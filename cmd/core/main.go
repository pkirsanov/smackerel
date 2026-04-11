package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	"github.com/smackerel/smackerel/internal/intelligence"
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

	// Start result subscriber for ML processing results
	resultSub := pipeline.NewResultSubscriber(pg.Pool, nc)
	if err := resultSub.Start(ctx); err != nil {
		return fmt.Errorf("start result subscriber: %w", err)
	}

	// Create pipeline processor
	proc := pipeline.NewProcessor(pg.Pool, nc)

	// Create search engine
	searchEngine := &api.SearchEngine{
		Pool:           pg.Pool,
		NATS:           nc,
		MLSidecarURL:   cfg.MLSidecarURL,
		HealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
	}

	// Create digest generator
	digestGen := digest.NewGenerator(pg.Pool, nc)

	// Create intelligence engine for synthesis, alerts, and resurfacing
	intEngine := intelligence.NewEngine(pg.Pool, nc)

	// Create topic lifecycle manager for momentum tracking
	topicLifecycle := topics.NewLifecycle(pg.Pool)

	// Create and start connector supervisor
	registry := connector.NewRegistry()
	stateStore := connector.NewStateStore(pg.Pool)
	supervisor := connector.NewSupervisor(registry, stateStore)

	// Register connectors and start those with valid OAuth tokens
	imapConn := imapConnector.New("gmail")
	caldavConn := caldavConnector.New("google-calendar")
	ytConn := youtubeConnector.New("youtube")
	rssConn := rssConnector.New("rss", nil) // feed URLs configured via source_config
	keepConn := keepConnector.New("google-keep")
	bmConn := bookmarksConnector.NewConnector("bookmarks")
	browserHistConn := browserConnector.New("browser-history")
	mapsConn := mapsConnector.New("google-maps-timeline")
	hospitableConn := hospitableConnector.New("hospitable")
	discordConn := discordConnector.New("discord")
	twitterConn := twitterConnector.New("twitter")
	weatherConn := weatherConnector.New("weather")
	alertsConn := alertsConnector.New("gov-alerts")
	marketsConn := marketsConnector.New("financial-markets")
	registry.Register(imapConn)
	registry.Register(caldavConn)
	registry.Register(ytConn)
	registry.Register(rssConn)
	registry.Register(keepConn)
	registry.Register(bmConn)
	registry.Register(browserHistConn)
	registry.Register(mapsConn)
	registry.Register(hospitableConn)
	registry.Register(discordConn)
	registry.Register(twitterConn)
	registry.Register(weatherConn)
	registry.Register(alertsConn)
	registry.Register(marketsConn)

	// Auto-start bookmarks connector (no OAuth needed — file-based import)
	if cfg.BookmarksImportDir != "" {
		bmConfig := connector.ConnectorConfig{
			AuthType:       "none",
			Enabled:        true,
			ProcessingTier: "full",
			SourceConfig: map[string]interface{}{
				"import_dir":        cfg.BookmarksImportDir,
				"archive_processed": true,
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
			AuthType: "none",
			Enabled:  true,
			SourceConfig: map[string]interface{}{
				"import_dir":        cfg.MapsImportDir,
				"archive_processed": false,
				"min_distance_m":    float64(100),
				"min_duration_min":  float64(2),
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
				"capture_commands":   parseJSONArray(os.Getenv("DISCORD_CAPTURE_COMMANDS")),
				"monitored_channels": parseJSONArray(os.Getenv("DISCORD_MONITORED_CHANNELS")),
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
				"locations": parseJSONArray(os.Getenv("WEATHER_LOCATIONS")),
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
		alertsCfg := connector.ConnectorConfig{
			AuthType:     "none",
			Enabled:      true,
			SyncSchedule: os.Getenv("GOV_ALERTS_SYNC_SCHEDULE"),
			SourceConfig: map[string]interface{}{
				"locations":                parseJSONArray(os.Getenv("GOV_ALERTS_LOCATIONS")),
				"min_earthquake_magnitude": parseFloatEnv("GOV_ALERTS_MIN_EARTHQUAKE_MAG"),
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
				"watchlist":         parseJSONObject(os.Getenv("FINANCIAL_MARKETS_WATCHLIST")),
				"alert_threshold":   parseFloatEnv("FINANCIAL_MARKETS_ALERT_THRESHOLD"),
				"coingecko_enabled": os.Getenv("FINANCIAL_MARKETS_COINGECKO_ENABLED") != "false",
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
			connConfig := connector.ConnectorConfig{
				AuthType:    "oauth2",
				Credentials: creds,
				Enabled:     true,
			}
			if err := imapConn.Connect(ctx, connConfig); err == nil {
				supervisor.SetConfig("gmail", connConfig)
				supervisor.StartConnector(ctx, "gmail")
				slog.Info("gmail connector started with OAuth token")
			}
			if err := caldavConn.Connect(ctx, connConfig); err == nil {
				supervisor.SetConfig("google-calendar", connConfig)
				supervisor.StartConnector(ctx, "google-calendar")
				slog.Info("google-calendar connector started with OAuth token")
			}
			connConfig.AuthType = "oauth2"
			if err := ytConn.Connect(ctx, connConfig); err == nil {
				supervisor.SetConfig("youtube", connConfig)
				supervisor.StartConnector(ctx, "youtube")
				slog.Info("youtube connector started with OAuth token")
			}
		}
	} else {
		slog.Info("no Google OAuth token found — connectors will start when user authorizes via /auth/google/start")
	}

	// Create web UI handler
	webHandler := web.NewHandler(pg.Pool, nc, time.Now())

	// Set up API
	deps := &api.Dependencies{
		DB:                 pg,
		NATS:               nc,
		IntelligenceEngine: intEngine,
		StartTime:          time.Now(),
		MLSidecarURL:       cfg.MLSidecarURL,
		Pipeline:           proc,
		SearchEngine:       searchEngine,
		DigestGen:          digestGen,
		WebHandler:         webHandler,
		OAuthHandler:       oauthHandler,
		OllamaURL:          cfg.OllamaURL,
		AuthToken:          cfg.AuthToken,
		ConnectorRegistry:  registry,
		Version:            version,
		CommitHash:         commitHash,
	}

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	var tgBot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		var err error
		tgBot, err = telegram.NewBot(telegram.Config{
			BotToken:   cfg.TelegramBotToken,
			ChatIDs:    cfg.TelegramChatIDs,
			CoreAPIURL: cfg.CoreAPIURL,
			AuthToken:  cfg.AuthToken,
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
	if err := sched.Start(ctx, cfg.DigestCron); err != nil {
		slog.Warn("digest scheduler failed to start", "error", err)
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
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

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errCh:
		return err
	}

	// Explicit sequential shutdown — replaces defer-based ordering to prevent
	// resource races (e.g., NATS drain racing DB pool close).
	// Timeout budget: cfg.ShutdownTimeoutS with 5s margin before Docker SIGKILL.
	shutdownAll(cfg.ShutdownTimeoutS, sched, srv, tgBot, resultSub, supervisor, nc, pg)

	slog.Info("smackerel-core stopped")
	return nil
}

// shutdownAll performs explicit sequential shutdown in reverse-dependency order.
// Sequence: scheduler → HTTP → Telegram → result subscribers → connectors → NATS → DB.
// Each step gets a timeout budget; if a step hangs, a warning is logged and shutdown proceeds.
func shutdownAll(
	timeoutS int,
	sched *scheduler.Scheduler,
	srv *http.Server,
	tgBot *telegram.Bot,
	resultSub *pipeline.ResultSubscriber,
	supervisor *connector.Supervisor,
	nc *smacknats.Client,
	pg *db.Postgres,
) {
	totalTimeout := time.Duration(timeoutS) * time.Second
	slog.Info("starting graceful shutdown", "timeout_s", timeoutS)

	// Step 1: Stop scheduler (no new cron jobs fire) — 2s budget
	runWithTimeout("scheduler", 2*time.Second, func() {
		if sched != nil {
			sched.Stop()
		}
	})

	// Step 2: Drain HTTP server — allocate most of the budget here
	httpTimeout := totalTimeout - 10*time.Second
	if httpTimeout < 5*time.Second {
		httpTimeout = 5 * time.Second
	}
	runWithTimeout("HTTP server", httpTimeout, func() {
		if srv != nil {
			httpCtx, httpCancel := context.WithTimeout(context.Background(), httpTimeout)
			defer httpCancel()
			if err := srv.Shutdown(httpCtx); err != nil {
				slog.Warn("shutdown: HTTP server drain error", "error", err)
			}
		}
	})

	// Step 3: Stop Telegram bot (cancel long-poll) — 2s budget
	runWithTimeout("Telegram bot", 2*time.Second, func() {
		if tgBot != nil {
			tgBot.Stop()
		}
	})

	// Step 4: Stop result subscribers (NATS consumer drain) — 2s budget
	runWithTimeout("result subscribers", 2*time.Second, func() {
		if resultSub != nil {
			resultSub.Stop()
		}
	})

	// Step 5: Stop connector supervisor (all connectors) — 2s budget
	runWithTimeout("connectors", 2*time.Second, func() {
		if supervisor != nil {
			supervisor.StopAll()
		}
	})

	// Step 6: Drain NATS connection (after all NATS consumers are stopped) — 2s budget
	runWithTimeout("NATS", 2*time.Second, func() {
		if nc != nil {
			nc.Close()
		}
	})

	// Step 7: Close DB pool (last — all DB consumers are already stopped) — 1s budget
	runWithTimeout("database pool", 1*time.Second, func() {
		if pg != nil {
			pg.Close()
		}
	})
}

// runWithTimeout runs fn with a timeout. If fn doesn't complete within budget,
// a warning is logged and control returns immediately so shutdown can proceed.
func runWithTimeout(step string, budget time.Duration, fn func()) {
	slog.Info("shutdown: stopping "+step, "budget", budget)
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		// completed within budget
	case <-time.After(budget):
		slog.Warn("shutdown: step exceeded timeout, proceeding", "step", step, "budget", budget)
	}
}

// parseJSONArray parses a JSON array string into []interface{}.
// Returns nil on empty string or parse error.
func parseJSONArray(s string) []interface{} {
	if s == "" {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseJSONObject parses a JSON object string into map[string]interface{}.
// Returns nil on empty string or parse error.
func parseJSONObject(s string) map[string]interface{} {
	if s == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseFloatEnv reads an environment variable and parses it as float64.
// Returns 0 on empty string or parse error.
func parseFloatEnv(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
