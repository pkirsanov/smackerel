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
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
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
	nc, err := smacknats.Connect(ctx, cfg.NATSURL)
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

	// Start digest scheduler
	sched := scheduler.New(digestGen, tgBot)
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

	slog.Info("smackerel-core stopped")
	return nil
}
