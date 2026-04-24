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
	"github.com/smackerel/smackerel/internal/scheduler"
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

	configureLogging(cfg)
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
	deps, listResolver, listStore := buildAPIDeps(cfg, svc)

	router := api.NewRouter(deps)

	// Start Telegram bot if configured
	tgBot := startTelegramBotIfConfigured(ctx, cfg, deps)

	// Start digest scheduler + intelligence jobs
	sched := scheduler.New(svc.digestGen, tgBot, svc.intEngine, svc.topicLifecycle)

	// Subscribe intelligence engine to annotation events (spec 027)
	if svc.intEngine != nil {
		if err := svc.intEngine.SubscribeAnnotations(ctx); err != nil {
			slog.Warn("annotation subscription failed", "error", err)
		}
	}

	// Wire optional feature services (knowledge, expenses, meal planning)
	wireKnowledgeLinter(sched, cfg, svc)
	wireExpenseTracking(ctx, cfg, svc, deps)
	wireMealPlanning(cfg, svc, deps, sched, listResolver, listStore, tgBot)

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
