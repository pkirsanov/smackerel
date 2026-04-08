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
	caldavConnector "github.com/smackerel/smackerel/internal/connector/caldav"
	imapConnector "github.com/smackerel/smackerel/internal/connector/imap"
	keepConnector "github.com/smackerel/smackerel/internal/connector/keep"
	rssConnector "github.com/smackerel/smackerel/internal/connector/rss"
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

	slog.Info("starting smackerel-core", "port", cfg.Port)

	// Connect to PostgreSQL
	pg, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	defer pg.Close()

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
	defer nc.Close()

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
	searchEngine := &api.SearchEngine{Pool: pg.Pool, NATS: nc}

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
	registry.Register(imapConn)
	registry.Register(caldavConn)
	registry.Register(ytConn)
	registry.Register(rssConn)
	registry.Register(keepConn)

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
				supervisor.StartConnector(ctx, "gmail")
				slog.Info("gmail connector started with OAuth token")
			}
			if err := caldavConn.Connect(ctx, connConfig); err == nil {
				supervisor.StartConnector(ctx, "google-calendar")
				slog.Info("google-calendar connector started with OAuth token")
			}
			connConfig.AuthType = "oauth2"
			if err := ytConn.Connect(ctx, connConfig); err == nil {
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
		DB:           pg,
		NATS:         nc,
		StartTime:    time.Now(),
		MLSidecarURL: cfg.MLSidecarURL,
		Pipeline:     proc,
		SearchEngine: searchEngine,
		DigestGen:    digestGen,
		WebHandler:   webHandler,
		OAuthHandler: oauthHandler,
		AuthToken:    cfg.AuthToken,
	}

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	var tgBot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		var err error
		tgBot, err = telegram.NewBot(telegram.Config{
			BotToken:   cfg.TelegramBotToken,
			ChatIDs:    cfg.TelegramChatIDs,
			CoreAPIURL: "http://localhost:" + cfg.Port,
			AuthToken:  cfg.AuthToken,
		})
		if err != nil {
			slog.Warn("telegram bot initialization failed", "error", err)
		} else {
			tgBot.Start(ctx)
			slog.Info("telegram bot started")
		}
	}

	// Start digest scheduler + intelligence jobs
	sched := scheduler.New(digestGen, tgBot, intEngine, topicLifecycle)
	if err := sched.Start(ctx, cfg.DigestCron); err != nil {
		slog.Warn("digest scheduler failed to start", "error", err)
	}
	defer sched.Stop()

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	// Drain components gracefully
	if tgBot != nil {
		tgBot.Stop()
	}
	resultSub.Stop()
	supervisor.StopAll()

	slog.Info("smackerel-core stopped")
	return nil
}
