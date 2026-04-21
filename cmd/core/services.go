package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/domain"
	"github.com/smackerel/smackerel/internal/graph"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/topics"
	"github.com/smackerel/smackerel/internal/web"
)

// coreServices holds all runtime dependencies built during startup.
type coreServices struct {
	pg                *db.Postgres
	nc                *smacknats.Client
	guestRepo         *db.GuestRepository
	propertyRepo      *db.PropertyRepository
	hospitalityLinker *graph.HospitalityLinker
	registry          *connector.Registry
	supervisor        *connector.Supervisor
	resultSub         *pipeline.ResultSubscriber
	synthesisSub      *pipeline.SynthesisResultSubscriber
	domainSub         *pipeline.DomainResultSubscriber
	knowledgeStore    *knowledge.KnowledgeStore
	proc              *pipeline.Processor
	searchEngine      *api.SearchEngine
	digestGen         *digest.Generator
	intEngine         *intelligence.Engine
	topicLifecycle    *topics.Lifecycle
	tokenStore        *auth.TokenStore
	oauthHandler      *auth.OAuthHandler
	webHandler        *web.Handler
	contextHandler    *api.ContextHandler
}

// buildCoreServices constructs all infrastructure and service dependencies.
func buildCoreServices(ctx context.Context, cfg *config.Config) (*coreServices, error) {
	svc := &coreServices{}

	// Connect to PostgreSQL
	var err error
	svc.pg, err = db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return nil, fmt.Errorf("database connection: %w", err)
	}
	// pg.Close() is called in shutdownAll() — no defer here

	// Run schema migrations
	if err := db.Migrate(ctx, svc.pg.Pool); err != nil {
		return nil, fmt.Errorf("database migration: %w", err)
	}
	slog.Info("database migrations complete")

	// Connect to NATS
	svc.nc, err = smacknats.Connect(ctx, cfg.NATSURL, cfg.AuthToken)
	if err != nil {
		return nil, fmt.Errorf("NATS connection: %w", err)
	}
	// nc.Close() is called in shutdownAll() — no defer here

	// Ensure JetStream streams
	if err := svc.nc.EnsureStreams(ctx); err != nil {
		return nil, fmt.Errorf("NATS stream setup: %w", err)
	}
	slog.Info("NATS JetStream streams configured")

	// Create hospitality graph repositories and linker
	svc.guestRepo = db.NewGuestRepository(svc.pg.Pool)
	svc.propertyRepo = db.NewPropertyRepository(svc.pg.Pool)
	svc.hospitalityLinker = graph.NewHospitalityLinker(svc.guestRepo, svc.propertyRepo, svc.pg.Pool, graph.NewLinker(svc.pg.Pool))

	// Seed hospitality topics (idempotent — safe to call on every startup)
	if err := graph.SeedHospitalityTopics(ctx, svc.pg.Pool); err != nil {
		slog.Warn("failed to seed hospitality topics", "error", err)
	}

	// Create connector registry (used by digest generator for hospitality detection)
	svc.registry = connector.NewRegistry()

	// Start result subscriber for ML processing results
	svc.resultSub = pipeline.NewResultSubscriber(svc.pg.Pool, svc.nc, svc.registry)
	svc.resultSub.Processor.HospitalityLinker = svc.hospitalityLinker

	// Wire knowledge synthesis into pipeline if enabled
	if cfg.KnowledgeEnabled {
		svc.knowledgeStore = knowledge.NewKnowledgeStore(svc.pg.Pool)
		svc.resultSub.KnowledgeEnabled = true
		svc.resultSub.KnowledgeStore = svc.knowledgeStore
		svc.resultSub.PromptContractVersion = cfg.KnowledgePromptContractIngestSynthesis
		slog.Info("knowledge synthesis pipeline enabled",
			"contract", cfg.KnowledgePromptContractIngestSynthesis,
		)

		svc.synthesisSub = pipeline.NewSynthesisResultSubscriber(svc.pg.Pool, svc.nc, svc.knowledgeStore)
		svc.synthesisSub.CrossSourceConfidenceThreshold = cfg.KnowledgeCrossSourceConfidenceThreshold
		svc.synthesisSub.CrossSourcePromptContractVersion = cfg.KnowledgePromptContractCrossSource
	}

	if err := svc.resultSub.Start(ctx); err != nil {
		return nil, fmt.Errorf("start result subscriber: %w", err)
	}

	// Start synthesis result subscriber after NATS streams are ready
	if svc.synthesisSub != nil {
		if err := svc.synthesisSub.Start(ctx); err != nil {
			slog.Warn("synthesis result subscriber failed to start", "error", err)
		}
	}

	// Load domain extraction registry and start domain result subscriber.
	// Domain extraction is independent of knowledge — it fires whenever
	// matching prompt contracts exist in the contracts directory.
	if cfg.PromptContractsDir != "" {
		domainReg, err := domain.LoadRegistry(cfg.PromptContractsDir)
		if err != nil {
			slog.Warn("domain registry load failed", "error", err)
		} else if domainReg.Count() > 0 {
			svc.resultSub.DomainRegistry = domainReg
			slog.Info("domain extraction enabled", "contracts", domainReg.Count())

			svc.domainSub = pipeline.NewDomainResultSubscriber(svc.pg.Pool, svc.nc)
			if err := svc.domainSub.Start(ctx); err != nil {
				slog.Warn("domain result subscriber failed to start", "error", err)
			}
		} else {
			slog.Info("domain extraction disabled — no domain contracts found")
		}
	}

	// Create pipeline processor
	svc.proc = pipeline.NewProcessor(svc.pg.Pool, svc.nc)
	svc.proc.HospitalityLinker = svc.hospitalityLinker

	// Create search engine
	svc.searchEngine = &api.SearchEngine{
		Pool:           svc.pg.Pool,
		NATS:           svc.nc,
		MLSidecarURL:   cfg.MLSidecarURL,
		HealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
	}

	// ML sidecar readiness gate: wait for sidecar health at startup
	// so first search requests don't timeout. Falls back to text mode on timeout.
	if cfg.MLReadinessTimeoutS > 0 {
		readinessTimeout := time.Duration(cfg.MLReadinessTimeoutS) * time.Second
		svc.searchEngine.WaitForMLReady(ctx, readinessTimeout)
	}

	// Create digest generator
	svc.digestGen = digest.NewGenerator(svc.pg.Pool, svc.nc, svc.registry)
	svc.digestGen.KnowledgeEnabled = cfg.KnowledgeEnabled

	// Create intelligence engine for synthesis, alerts, and resurfacing
	svc.intEngine = intelligence.NewEngine(svc.pg.Pool, svc.nc)

	// Create topic lifecycle manager for momentum tracking
	svc.topicLifecycle = topics.NewLifecycle(svc.pg.Pool)

	// Create and start connector supervisor
	stateStore := connector.NewStateStore(svc.pg.Pool)
	svc.supervisor = connector.NewSupervisor(svc.registry, stateStore)

	// Wire artifact publisher so connector-produced RawArtifacts flow into the NATS pipeline
	artifactPublisher := pipeline.NewRawArtifactPublisher(svc.pg.Pool, svc.nc)
	svc.supervisor.SetPublisher(artifactPublisher)

	// Set up OAuth handler for connector authorization
	// Auth token is used as the encryption key for OAuth tokens at rest (AES-256-GCM)
	svc.tokenStore = auth.NewTokenStore(svc.pg.Pool, cfg.AuthToken)
	svc.oauthHandler = auth.NewOAuthHandler(svc.tokenStore)
	slog.Info("OAuth handler initialized")

	// Create web UI handler
	svc.webHandler = web.NewHandler(svc.pg.Pool, svc.nc, time.Now())
	svc.webHandler.KnowledgeStore = svc.knowledgeStore

	// Create context enrichment handler for GuestHost connector
	svc.contextHandler = api.NewContextHandler(svc.guestRepo, svc.propertyRepo, svc.pg.Pool)

	return svc, nil
}
