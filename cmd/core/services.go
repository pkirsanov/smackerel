package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/domain"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/internal/graph"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/mealplan"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
	"github.com/smackerel/smackerel/internal/pipeline"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/topics"
	"github.com/smackerel/smackerel/internal/web"

	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	assistanttracing "github.com/smackerel/smackerel/internal/assistant/tracing"
)

// coreServices holds all runtime dependencies built during startup.
type coreServices struct {
	pg                     *db.Postgres
	nc                     *smacknats.Client
	guestRepo              *db.GuestRepository
	propertyRepo           *db.PropertyRepository
	hospitalityLinker      *graph.HospitalityLinker
	registry               *connector.Registry
	supervisor             *connector.Supervisor
	resultSub              *pipeline.ResultSubscriber
	synthesisSub           *pipeline.SynthesisResultSubscriber
	domainSub              *pipeline.DomainResultSubscriber
	knowledgeStore         *knowledge.KnowledgeStore
	proc                   *pipeline.Processor
	searchEngine           *api.SearchEngine
	digestGen              *digest.Generator
	intEngine              *intelligence.Engine
	topicLifecycle         *topics.Lifecycle
	tokenStore             *auth.TokenStore
	oauthHandler           *auth.OAuthHandler
	webHandler             *web.Handler
	contextHandler         *api.ContextHandler
	recommendationStore    *recstore.Store
	recommendationRegistry *recprovider.Registry
	notificationStore      *notification.Store
	ntfyStore              *ntfysource.Store
	ntfyRuntime            *ntfysource.Runtime
	driveSaveService       *save.Service
	driveRetrieveService   *retrieve.Service // spec 038 Scope 7 — drive retrieval
	mealPlanSaveBack       *mealplan.DriveSaveBack

	// BUG-034-004 follow-up — meal-plan handler must be constructed
	// BEFORE api.NewRouter so /api/meal-plans routes register. The
	// scheduler + telegram wiring depends on `sched` and `tgBot` which
	// are constructed AFTER NewRouter, so wireMealPlanning is split:
	// wireMealPlanningHandler runs early (builds these stashed
	// services) and wireMealPlanningSchedulerAndBot runs late
	// (consumes them).
	mealPlanServiceForLateWiring *mealplan.Service
	mealPlanShoppingBridge       *mealplan.ShoppingBridge
	// Spec 044 Scope 02 — auth revocation NATS broadcaster. May be nil
	// when auth is disabled or NATS is unavailable; in that case the
	// revocation cache still hydrates from the database via the
	// periodic refresh and revoke calls still update the canonical
	// auth_revocations table.
	authRevocationBroadcaster *revocation.Broadcaster

	// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — OTel SDK
	// substrate. tracer is non-nil after buildCoreServices runs (the
	// no-op TracerProvider path is the production-default). assistantTracerShutdown
	// is wired into the shutdownAll graceful-shutdown sequence so any
	// buffered spans are flushed on exit.
	assistantTracer         *assistanttracing.Tracer
	assistantTracerShutdown assistanttracing.ShutdownFunc

	// Spec 069 SCOPE-1a — late-bound HTTP transport adapter handler.
	// Built in wiring.go before api.NewRouter so the route mount can
	// see it; the adapter inside is installed by wireAssistantFacade
	// after the capability-layer Facade is constructed. Until then,
	// the handler returns 503 with "assistant_http_not_ready".
	assistantHTTPHandler *httpadapter.LateBoundHandler

	// Spec 083 Scope 11 — card-rewards web handler, stashed at early
	// (pre-router) construction so wireCardRewardsScheduler can late-wire
	// the admin manual-trigger seam (the scheduler is built AFTER the
	// router, so the admin "scrape now" / "sync calendar now" triggers
	// cannot be injected at construction time). May be nil when no
	// Postgres pool is available.
	cardRewardsWebHandler *web.CardRewardsWebHandler
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

	// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) — initialize
	// the OTel SDK substrate via the testable helper in wiring.go.
	// The production default ships with otel_enabled=false, so the
	// no-op TracerProvider path is taken and the SDK pipeline is
	// proven without exporting any spans or touching the network;
	// dev/test stacks flip otel_enabled=true and point otel_endpoint
	// at the jaegertracing/all-in-one sidecar under the dev-otel or
	// test compose profile. When enabled, initAssistantTracing runs
	// a fail-loud TCP probe BEFORE constructing the SDK so an
	// unreachable endpoint aborts startup per design §7.2-OTel
	// validation rule rather than silently buffering spans.
	tracer, tracerShutdown, err := initAssistantTracing(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("assistant tracing init: %w", err)
	}
	svc.assistantTracer = tracer
	svc.assistantTracerShutdown = tracerShutdown
	slog.Info("assistant tracing initialized",
		"otel_enabled", cfg.Assistant.Observability.OtelEnabled,
		"otel_endpoint", cfg.Assistant.Observability.OtelEndpoint,
		"otel_service_name", cfg.Assistant.Observability.OtelServiceName,
	)

	// Connect to NATS
	svc.nc, err = smacknats.Connect(ctx, cfg.NATSURL, cfg.AuthToken)
	if err != nil {
		return nil, fmt.Errorf("NATS connection: %w", err)
	}
	// nc.Close() is called in shutdownAll() — no defer here

	// Ensure JetStream streams
	if err := svc.nc.EnsureStreams(ctx, cfg.NATSStreamMaxBytes); err != nil {
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
	svc.notificationStore = notification.NewStore(svc.pg.Pool)
	svc.ntfyStore = ntfysource.NewStore(svc.pg.Pool)

	// Start result subscriber for ML processing results
	svc.resultSub = pipeline.NewResultSubscriber(svc.pg.Pool, svc.nc, svc.registry)
	svc.resultSub.Processor.HospitalityLinker = svc.hospitalityLinker

	// Wire knowledge synthesis into pipeline if enabled
	if cfg.KnowledgeEnabled {
		svc.knowledgeStore = knowledge.NewKnowledgeStore(svc.pg.Pool)
		svc.knowledgeStore.MaxTokens = cfg.KnowledgeConceptMaxTokens
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

	// ML sidecar readiness gate runs in the background so the core HTTP
	// listener can bind and /api/health can answer the Docker healthcheck
	// within the start_period budget even on a cold fresh build (where the
	// ml sidecar may still be warming up while core boots — core's
	// depends_on does NOT gate on ml: service_healthy). Search requests
	// that arrive before the gate completes fall back to text mode, which
	// is the same documented behavior as a readiness timeout.
	if cfg.MLReadinessTimeoutS > 0 {
		readinessTimeout := time.Duration(cfg.MLReadinessTimeoutS) * time.Second
		go svc.searchEngine.WaitForMLReady(ctx, readinessTimeout)
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
	svc.recommendationStore = recstore.New(svc.pg.Pool)
	svc.recommendationRegistry = recprovider.RuntimeRegistry()
	svc.webHandler = web.NewHandler(svc.pg.Pool, svc.nc, time.Now())
	svc.webHandler.KnowledgeStore = svc.knowledgeStore
	svc.webHandler.Supervisor = svc.supervisor
	svc.webHandler.RecommendationsEnabled = cfg.Recommendations.Enabled
	svc.webHandler.RecommendationProviders = recprovider.DefaultRegistry
	svc.webHandler.RecommendationStore = svc.recommendationStore
	svc.webHandler.RecommendationRegistry = svc.recommendationRegistry
	svc.webHandler.RecommendationConfig = cfg.Recommendations
	svc.webHandler.NotificationStore = svc.notificationStore
	svc.webHandler.NtfyStore = svc.ntfyStore

	// Create context enrichment handler for GuestHost connector
	svc.contextHandler = api.NewContextHandler(svc.guestRepo, svc.propertyRepo, svc.pg.Pool)

	return svc, nil
}
