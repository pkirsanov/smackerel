package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/auth/webcreds"
	"github.com/smackerel/smackerel/internal/auth/webinvite"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// Pipeliner processes capture requests through the ML pipeline.
type Pipeliner interface {
	Process(ctx context.Context, req *pipeline.ProcessRequest) (*pipeline.ProcessResult, error)
}

// Searcher handles semantic search operations.
type Searcher interface {
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, int, string, error)
}

// DigestGenerator produces daily/weekly digests.
type DigestGenerator interface {
	GetLatest(ctx context.Context, date string) (*digest.Digest, error)
}

// WebUI serves the HTMX web interface routes.
type WebUI interface {
	SearchPage(w http.ResponseWriter, r *http.Request)
	SearchResults(w http.ResponseWriter, r *http.Request)
	ArtifactDetail(w http.ResponseWriter, r *http.Request)
	EvidenceBundleBuilderPage(w http.ResponseWriter, r *http.Request)
	DigestPage(w http.ResponseWriter, r *http.Request)
	TopicsPage(w http.ResponseWriter, r *http.Request)
	SettingsPage(w http.ResponseWriter, r *http.Request)
	StatusPage(w http.ResponseWriter, r *http.Request)
	RecommendationsPage(w http.ResponseWriter, r *http.Request)
	RecommendationsResults(w http.ResponseWriter, r *http.Request)
	RecommendationPreferencesPage(w http.ResponseWriter, r *http.Request)
	RecommendationFeedback(w http.ResponseWriter, r *http.Request)
	RecommendationDetail(w http.ResponseWriter, r *http.Request)
	RecommendationWatchesPage(w http.ResponseWriter, r *http.Request)
	RecommendationWatchEditorPage(w http.ResponseWriter, r *http.Request)
	RecommendationWatchDetailPage(w http.ResponseWriter, r *http.Request)
	RecommendationWatchPauseAction(w http.ResponseWriter, r *http.Request)
	RecommendationWatchResumeAction(w http.ResponseWriter, r *http.Request)
	RecommendationWatchSilenceAction(w http.ResponseWriter, r *http.Request)
	RecommendationWatchDeleteAction(w http.ResponseWriter, r *http.Request)
	TripDossierPage(w http.ResponseWriter, r *http.Request)
	NotificationDashboard(w http.ResponseWriter, r *http.Request)
	NotificationSourcesPage(w http.ResponseWriter, r *http.Request)
	NotificationNtfySourcePage(w http.ResponseWriter, r *http.Request)
	NotificationNtfyDeadLettersPage(w http.ResponseWriter, r *http.Request)
	NotificationNtfyDeadLetterDetailPage(w http.ResponseWriter, r *http.Request)
	NotificationEventsPage(w http.ResponseWriter, r *http.Request)
	NotificationIncidentsPage(w http.ResponseWriter, r *http.Request)
	NotificationIncidentDetailPage(w http.ResponseWriter, r *http.Request)
	NotificationApprovalsPage(w http.ResponseWriter, r *http.Request)
	NotificationApprovalDetailPage(w http.ResponseWriter, r *http.Request)
	NotificationSuppressionsPage(w http.ResponseWriter, r *http.Request)
	NotificationSummaryPage(w http.ResponseWriter, r *http.Request)
	NotificationOutputsPage(w http.ResponseWriter, r *http.Request)
	SyncConnectorHandler(w http.ResponseWriter, r *http.Request)
	BookmarkUploadHandler(w http.ResponseWriter, r *http.Request)
	KnowledgeDashboard(w http.ResponseWriter, r *http.Request)
	ConceptsList(w http.ResponseWriter, r *http.Request)
	ConceptDetail(w http.ResponseWriter, r *http.Request)
	EntitiesList(w http.ResponseWriter, r *http.Request)
	EntityDetail(w http.ResponseWriter, r *http.Request)
	LintReport(w http.ResponseWriter, r *http.Request)
	LintFindingDetail(w http.ResponseWriter, r *http.Request)
}

// AgentAdminUI exposes the spec 037 Scope 8 operator-UI handlers. The
// concrete implementation lives in internal/web (AgentAdminHandler).
type AgentAdminUI interface {
	TracesIndex(w http.ResponseWriter, r *http.Request)
	TracesShow(w http.ResponseWriter, r *http.Request)
	ScenariosIndex(w http.ResponseWriter, r *http.Request)
	ScenariosShow(w http.ResponseWriter, r *http.Request)
	ToolsIndex(w http.ResponseWriter, r *http.Request)
	ToolsShow(w http.ResponseWriter, r *http.Request)
}

// OAuthFlow handles OAuth2 authorization flows and status.
type OAuthFlow interface {
	StartHandler(w http.ResponseWriter, r *http.Request)
	CallbackHandler(w http.ResponseWriter, r *http.Request)
	StatusHandler(w http.ResponseWriter, r *http.Request)
}

// TelegramHealthChecker checks Telegram bot connection health.
type TelegramHealthChecker interface {
	Healthy() bool
}

// ConnectorHealthLister reports health for all registered connectors.
type ConnectorHealthLister interface {
	ListConnectorHealth(ctx context.Context) map[string]string
}

// ArtifactQuerier provides typed access to artifact CRUD operations.
type ArtifactQuerier interface {
	RecentArtifacts(ctx context.Context, limit int) ([]db.RecentArtifact, error)
	GetArtifact(ctx context.Context, id string) (*db.ArtifactDetail, error)
	GetArtifactWithDomain(ctx context.Context, id string) (*db.ArtifactWithDomain, error)
	ExportArtifacts(ctx context.Context, cursor time.Time, limit int) (*db.ExportResult, error)
}

// Dependencies holds shared service dependencies for API handlers.
type Dependencies struct {
	DB                 DBHealthChecker
	NATS               NATSHealthChecker
	IntelligenceEngine *intelligence.Engine
	StartTime          time.Time
	MLSidecarURL       string
	MLClient           *http.Client
	mlClientOnce       sync.Once
	Pipeline           Pipeliner
	SearchEngine       Searcher
	DigestGen          DigestGenerator
	WebHandler         WebUI
	// AppearanceHeadMiddleware, when set, is the spec 106 SCOPE-106-01 server
	// head-adapter (web.AppearanceHeadStamp): it resolves the smk_appearance
	// cookie and stamps the resolved data-theme/data-density onto the <html>
	// element of server-rendered HTML pages before first paint. Injected from
	// cmd/core to avoid the api->web import cycle; nil in router unit tests
	// (no stamping, no behavior change).
	AppearanceHeadMiddleware func(http.Handler) http.Handler
	OAuthHandler             OAuthFlow
	TelegramBot              TelegramHealthChecker
	ConnectorRegistry        ConnectorHealthLister
	ArtifactStore            ArtifactQuerier
	ContextHandler           *ContextHandler
	BookmarkPub              BookmarkPublisher
	OllamaURL                string
	AuthToken                string
	// Environment is the deployment environment value (allowed: development |
	// test | production) sourced from runtime.environment in smackerel.yaml
	// via SMACKEREL_ENV. MIT-040-S-004 — bearerAuthMiddleware uses this to
	// reject empty-token requests when Environment == "production"
	// (defense-in-depth; the wiring constructor already fails fast in
	// production with an empty token).
	Environment string
	Version     string
	CommitHash  string
	BuildTime   string

	// Spec 044 Scope 02 — per-user bearer-auth subsystem wiring. When
	// AuthConfig.Enabled is true (production-class deployments), the
	// bearerAuthMiddleware validates per-user PASETO v4.public tokens
	// against AuthVerifyOptions, consults RevocationCache for the
	// revocation-on-next-request contract (NFR-AUTH-006), and attaches
	// an auth.Session to the request context for downstream handlers.
	// All four MUST be non-nil when AuthConfig.Enabled is true; the
	// wiring layer enforces this at startup.
	AuthConfig        config.AuthConfig
	AuthVerifyOptions auth.VerifyOptions
	BearerStore       *auth.BearerStore
	RevocationCache   *revocation.Cache
	AuthAdminHandlers *AuthAdminHandlers

	// Spec 070 — web operator credential layer (username/password login).
	// nil when the deployment has no Postgres pool (config-validate mode,
	// some tests). HandleWebLogin falls back to the existing token-form
	// path when this is nil OR when the form does not include username +
	// password fields.
	WebCredentials webcreds.Repo

	// Spec 091 — web self-registration invite-token gate
	// (WEB_REGISTRATION_INVITE_TOKEN). OPTIONAL: empty ⇒ POST
	// /v1/web/register is disabled (fail-loud at POST, never open signup).
	// Compared in constant time by HandleWebRegister; never logged.
	WebRegistrationInviteToken string

	// Spec 093 — DB-backed single-use registration invites. nil when no
	// Postgres pool (config-validate mode, router unit tests): the /register
	// DB-invite branch is then simply not taken and the static-secret path is
	// unchanged. The admin invites UI shares this SAME repo instance via
	// CardRewardsWebHandler.SetInvites (wired in cmd/core/wiring.go).
	WebInvites webinvite.Repo

	// Spec 037 Scope 8 — admin web routes for the operator UI
	// (optional — nil when the agent runtime is not enabled).
	AgentAdminHandler AgentAdminUI

	// Spec 037 Scope 9 — POST /v1/agent/invoke handler
	// (optional — nil when the agent runtime is not enabled).
	AgentInvokeHandler *AgentInvokeHandler

	// Spec 089 — GET/PUT/DELETE /v1/agent/model claim-bound per-user sticky
	// model preference handler (optional — nil when open-knowledge is not
	// enabled). Reads the shared agenttool.ModelPref() store +
	// agenttool.SwitchableModels() validator the /ask fast-path uses.
	AgentModelHandler *AgentModelHandler

	// Spec 096 SCOPE-06 — operator-gated /v1/admin/model-connections* surface
	// (wire/test/enable/disable model-provider connections) + its operator gate
	// (R1). Both nil when there is no Postgres pool (config-validate mode) or no
	// db-mode connection is declared; the routes are then simply not mounted.
	// When set, the routes are mounted behind bearerAuthMiddleware AND
	// ModelConnectionsOperatorGate.Middleware (operator-only, claim-bound).
	ModelConnectionsAdminHandler *ModelConnectionsAdminHandler
	ModelConnectionsOperatorGate *OperatorGate

	// Spec 038 Scope 1 — drive connector API handlers.
	DriveHandlers *DriveHandlers

	// Spec 038 Scope 5 — Save Rules CRUD + audit + dry-run.
	DriveRulesHandlers *DriveRulesHandlers

	// Spec 038 Scope 5 — POST /v1/drive/save + recent requests listing.
	DriveSaveHandlers *DriveSaveHandlers

	// Spec 038 Scope 6 — POST/GET /v1/drive/confirmations/{id}.
	DriveConfirmationsHandlers *DriveConfirmationsHandlers

	// Spec 040 Scope 1 — photo library API handlers.
	PhotosHandlers *PhotosHandlers

	// Annotation handlers (optional — nil when annotations not configured)
	AnnotationHandlers *AnnotationHandlers

	// Knowledge layer (optional — nil when knowledge is disabled)
	KnowledgeStore                  KnowledgeSearcher
	KnowledgeConceptSearchThreshold float64
	KnowledgeHealthCacheTTL         time.Duration

	// Knowledge health cache — RWMutex avoids serializing concurrent
	// health checks on slow DB calls when the cache TTL expires (C-023-C001).
	knowledgeHealthMu    sync.RWMutex
	knowledgeHealthCache *KnowledgeHealthSection
	knowledgeHealthAt    time.Time

	// Intelligence engine health cache (BUG-021-002 — stabilize R13).
	// Without caching, every /api/health request triggered two synchronous
	// DB round-trips (GetLastSynthesisTime + HasStalePendingAlerts) against
	// the IntelligenceEngine pool, for data that updates at most once per
	// 24h (synthesis) or every 15 min (alert sweep). Reuses the same SST
	// TTL contract as KnowledgeHealthCacheTTL (ML_HEALTH_CACHE_TTL_S).
	IntelligenceHealthCacheTTL time.Duration
	intelligenceHealthMu       sync.RWMutex
	intelligenceHealthCache    *intelligenceHealthSnapshot
	intelligenceHealthAt       time.Time

	// Actionable list handlers (optional — nil when lists not configured)
	ListHandlers *ListHandlers

	// Expense handler (optional — nil when expenses not enabled)
	ExpenseHandler *ExpenseHandler

	// Meal plan handler (optional — nil when meal planning not enabled)
	MealPlanHandler *MealPlanHandler

	// Card-rewards handler (spec 083) — wallet/offers/selections/bonuses CRUD
	// + card-name resolution. Optional — nil when no Postgres pool is
	// available (config-validate mode, some tests).
	CardRewardsHandler *CardRewardsHandler

	// Card-rewards web UI (spec 083 Scope 10) — server-rendered wallet/offers/
	// selections/bonuses/categories pages. Optional — nil when no Postgres pool
	// is available (config-validate mode, router unit tests). Mounted behind
	// webAuthMiddleware; routes are registered via RegisterRoutes.
	CardRewardsWebHandler CardRewardsWebUI

	// Recommendation handlers (optional — nil when recommendations not enabled)
	RecommendationHandlers *RecommendationHandlers

	// Recommendation watch handlers (optional — nil when recommendations not enabled)
	RecommendationWatchHandlers *RecommendationWatchHandlers

	// QF evidence export handlers (optional — nil when QF connector is not enabled)
	QFEvidenceHandlers *QFEvidenceHandlers

	// QF personal-context read API handlers (optional — nil when QF
	// connector is not enabled). Spec 041 Scope 7.
	PersonalContextHandlers *PersonalContextHandlers

	// Notification source status handlers (optional — nil until spec 054 is wired)
	NotificationHandlers *NotificationHandlers

	// Spec 058 — Chrome Extension Bridge ingest handler
	// (POST /v1/connectors/extension/ingest). Mounted behind
	// bearerAuthMiddleware + auth.RequireScope("extension:bookmarks",
	// "extension:history"). Nil when extension wiring is absent.
	ExtensionIngestHandler http.Handler

	// Spec 058 Scope 5 — admin devices view
	// (GET /v1/admin/extension/devices). Mounted behind
	// bearerAuthMiddleware; the handler itself enforces admin scoping.
	ExtensionDevicesHandler http.Handler

	// Spec 058 BUG-058 BLOCKER-3 — extension devices admin HTML page
	// (GET /admin/extension/devices) on the shared internal/web/admin
	// scaffold. Mounted behind webAuthMiddleware (same as the agent admin
	// UI); the handler enforces the same admin scoping as the JSON view.
	// Nil when extension wiring is absent.
	ExtensionDevicesUIHandler http.Handler

	// Spec 069 SCOPE-1a — Assistant HTTP transport handler. Routes
	// POST /v1/assistant/turn through the late-bound HTTPAdapter
	// (wireAssistantFacade installs the backing adapter post-boot).
	AssistantTurnHandler http.Handler

	// Spec 074 — capture-as-fallback / explicit-capture policy
	// recorder and dedup HMAC key. CapturePolicyRecorder persists
	// artifact_capture_policy rows; CaptureFallbackHashKey scopes
	// the per-user normalized-text hash used for dedup lookups.
	CapturePolicyRecorder  capturefallback.CapturePolicyStore
	CaptureFallbackHashKey string

	// CORS allowed origins (SST-compliant — from smackerel.yaml via config generate)
	CORSAllowedOrigins []string

	// Runtime trusted reverse-proxy CIDR allowlist (BUG-020-005,
	// F-SEC-R30-001). Consumed by trustedProxyRealIPMiddleware in
	// internal/api/realip.go to decide whether the connecting TCP peer
	// is allowed to set forwarded-IP headers that the API will trust.
	// Empty slice = secure-by-default: forwarded headers are ignored
	// regardless of which peer sends them; per-IP rate limits key on
	// the raw TCP peer; slog `remote_addr` reflects the raw TCP peer.
	// Source: runtime.trusted_proxies in smackerel.yaml via
	// RUNTIME_TRUSTED_PROXIES (comma-separated CIDRs).
	TrustedProxies []string

	// Spec 080 SCOPE-080-02..04 — Knowledge Graph Public API handlers.
	// Non-nil ONLY in the ENABLED activation state (operator cursor
	// secret present); resolved together with GraphCapability in
	// cmd/core/wiring.go. In the DISABLED (fail-soft) state these stay
	// nil and internal/api/router.go serves the typed 503
	// capability_disabled responder for every graph path instead.
	TopicsHandlers *graphapi.TopicsHandlers
	PeopleHandlers *graphapi.PeopleHandlers
	// Spec 080 SCOPE-080-03 — places + time handlers (ENABLED only).
	PlacesHandlers *graphapi.PlacesHandlers
	TimeHandlers   *graphapi.TimeHandlers
	// Spec 080 SCOPE-080-04 — graph edges handler (ENABLED only).
	EdgesHandlers *graphapi.EdgesHandlers

	// Spec 080 + BUG-080-001 — fail-soft graph activation capability.
	// Carries the resolved enabled/disabled activation derived from the
	// operator cursor-secret presence (graphapi.ResolveActivation in
	// cmd/core/wiring.go). Non-nil whenever the KNOWLEDGE_GRAPH_API_*
	// envelope loaded (enabled OR disabled). internal/api/router.go
	// mounts the atomic graph route manifest gated on this field:
	// DISABLED -> every path answers the typed 503 capability_disabled
	// (present-but-disabled, never a silent Chi 404); ENABLED -> the
	// Guard is a passthrough and the live handlers above serve.
	GraphCapability *graphapi.GraphCapability

	// GraphReadiness is the BUG-080-001 SCOPE-03 product readiness
	// projection. It derives the AUTHENTICATED Graph capability detail
	// on GET /api/health and the STRICT Graph readiness answer on
	// GET /readyz?strict=true from exactly two inputs: the explicit
	// activation policy on GraphCapability and a published, validated
	// graphsynthetic aggregate. nil in router unit tests and in any
	// deployment that has not wired it — both surfaces then fail CLOSED
	// (no capability section, no ready Graph journey) rather than
	// inferring readiness from mounted routes or database liveness.
	GraphReadiness *GraphReadiness
}

// DBHealthChecker is the interface for database health checks.
type DBHealthChecker interface {
	Healthy(ctx context.Context) bool
	ArtifactCount(ctx context.Context) (int64, error)
}

// NATSHealthChecker is the interface for NATS health checks.
type NATSHealthChecker interface {
	Healthy() bool
}

// KnowledgeSearcher abstracts knowledge store operations needed by API handlers.
type KnowledgeSearcher interface {
	SearchConcepts(ctx context.Context, query string, threshold float64) (*knowledge.ConceptMatch, error)
	GetConceptByID(ctx context.Context, id string) (*knowledge.ConceptPage, error)
	GetEntityByID(ctx context.Context, id string) (*knowledge.EntityProfile, error)
	ListConceptsFiltered(ctx context.Context, q, sort string, limit, offset int) ([]*knowledge.ConceptPage, int, error)
	ListEntitiesFiltered(ctx context.Context, q, sort string, limit, offset int) ([]*knowledge.EntityProfile, int, error)
	GetLatestLintReport(ctx context.Context) (*knowledge.LintReport, error)
	GetStats(ctx context.Context) (*knowledge.KnowledgeStats, error)
	GetKnowledgeHealthStats(ctx context.Context) (*knowledge.KnowledgeHealthStats, error)
	CountEntitiesForConcept(ctx context.Context, conceptID string) (int, error)
	HasContradictions(ctx context.Context, conceptID string) (bool, error)
}

// HealthResponse is the JSON response for GET /api/health.
type HealthResponse struct {
	Status     string                   `json:"status"`
	Version    string                   `json:"version,omitempty"`
	CommitHash string                   `json:"commit_hash,omitempty"`
	BuildTime  string                   `json:"build_time,omitempty"`
	Services   map[string]ServiceStatus `json:"services"`
	Knowledge  *KnowledgeHealthSection  `json:"knowledge,omitempty"`
	// Graph is the BUG-080-001 SCOPE-03 Knowledge Graph capability
	// detail. It is populated ONLY for authenticated callers and is
	// omitted entirely otherwise, so the unauthenticated response shape
	// is unchanged.
	Graph *GraphHealthSection `json:"graph,omitempty"`
}

// KnowledgeHealthSection represents knowledge layer stats in the health response.
type KnowledgeHealthSection struct {
	ConceptCount     int        `json:"concept_count"`
	EntityCount      int        `json:"entity_count"`
	SynthesisPending int        `json:"synthesis_pending"`
	LastSynthesisAt  *time.Time `json:"last_synthesis_at,omitempty"`
}

// ServiceStatus represents the health of a single service.
type ServiceStatus struct {
	Status        string `json:"status"`
	UptimeSeconds *int64 `json:"uptime_seconds,omitempty"`
	ArtifactCount *int64 `json:"artifact_count,omitempty"`
	ModelLoaded   *bool  `json:"model_loaded,omitempty"`
}

const healthAuxiliaryProbeTimeout = 1500 * time.Millisecond

// HealthHandler handles GET /api/health.
func (d *Dependencies) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authenticated := d.isAuthenticated(r)
	services := make(map[string]ServiceStatus)

	var knowledgeHealthCh chan *KnowledgeHealthSection
	if authenticated && d.KnowledgeStore != nil {
		knowledgeHealthCh = make(chan *KnowledgeHealthSection, 1)
		go func() {
			knowledgeHealthCh <- d.getCachedKnowledgeHealth(ctx)
		}()
	}

	// API status
	uptime := int64(time.Since(d.StartTime).Seconds())
	services["api"] = ServiceStatus{
		Status:        "up",
		UptimeSeconds: &uptime,
	}

	// PostgreSQL status
	dbStatus := ServiceStatus{Status: "down"}
	if d.DB != nil && d.DB.Healthy(ctx) {
		dbStatus.Status = "up"
		if count, err := d.DB.ArtifactCount(ctx); err == nil {
			dbStatus.ArtifactCount = &count
		}
	}
	services["postgres"] = dbStatus

	// NATS status
	natsStatus := ServiceStatus{Status: "down"}
	if d.NATS != nil && d.NATS.Healthy() {
		natsStatus.Status = "up"
	}
	services["nats"] = natsStatus

	// Start external health probes in parallel (IMP-023-R19-001).
	// Each probe has a bounded context timeout; sequential execution would
	// bottleneck at 3s+ when both services are unreachable, exceeding
	// Docker HEALTHCHECK's typical 3s timeout and causing false restarts.
	var (
		mlStatus     ServiceStatus
		ollamaStatus ServiceStatus
		probeWg      sync.WaitGroup
	)
	client := d.mlClient() // safe: sync.Once guarantees single init
	probeWg.Add(2)
	go func() {
		defer probeWg.Done()
		mlStatus = checkMLSidecar(ctx, d.MLSidecarURL, client)
	}()
	go func() {
		defer probeWg.Done()
		ollamaStatus = checkOllama(ctx, d.OllamaURL, client)
	}()

	// Intelligence engine status — runs while external probes are in flight.
	// Both DB queries (GetLastSynthesisTime + HasStalePendingAlerts) are
	// TTL-cached via getCachedIntelligenceHealth to avoid hammering Postgres
	// on every /api/health request (BUG-021-002 — stabilize R13). The
	// pre-existing response shape is preserved exactly: nil engine ⇒ no
	// "intelligence" key; nil pool ⇒ "intelligence"="down" with no
	// "alert_delivery" key; alert-probe error ⇒ "alert_delivery" omitted.
	if d.IntelligenceEngine != nil {
		snap := d.getCachedIntelligenceHealth(ctx)
		services["intelligence"] = ServiceStatus{Status: snap.intelligenceStatus}
		if snap.alertDeliveryStatus != "" {
			services["alert_delivery"] = ServiceStatus{Status: snap.alertDeliveryStatus}
		}
	}

	// Telegram bot health — local check, no network I/O
	if d.TelegramBot != nil && d.TelegramBot.Healthy() {
		services["telegram_bot"] = ServiceStatus{Status: "connected"}
	} else {
		services["telegram_bot"] = ServiceStatus{Status: "disconnected"}
	}

	// Wait for external probes and record results
	probeWg.Wait()
	services["ml_sidecar"] = mlStatus
	services["ollama"] = ollamaStatus

	// Connector health
	if d.ConnectorRegistry != nil {
		connectors := d.ConnectorRegistry.ListConnectorHealth(ctx)
		for id, status := range connectors {
			services["connector:"+id] = ServiceStatus{Status: status}
		}
	}

	// Aggregate status
	overall := "healthy"
	for name, svc := range services {
		if name == "telegram_bot" || name == "ollama" {
			continue // optional services don't affect overall status
		}
		// An UNCONFIGURED optional connector reports "disconnected" — that is a
		// connector's INITIAL state from connector.New()/HealthDisconnected before
		// Connect() is ever called (and the state Close() returns it to). It is NOT
		// a fault: a connector that IS configured and then breaks reports
		// "error"/"failing"/"degraded" instead (the fault paths never set
		// "disconnected"; see e.g. youtube/discord/markets/guesthost), so real
		// connector faults still degrade the aggregate below. On this single-user
		// self-hosted most source connectors are registered but never provisioned (no
		// OAuth/API credentials), so they legitimately sit at "disconnected". A
		// deliberately-unprovisioned optional source must NOT drag the whole product
		// to "degraded" (redteam degraded-driver finding: the live prod aggregate
		// was "degraded" SOLELY because 14 unconfigured connectors reported
		// "disconnected" while every core service — postgres, nats, intelligence,
		// alert_delivery, ml_sidecar — was healthy). This refines DEV-003-002, which
		// over-broadly lumped connector "disconnected" in with the fault statuses.
		// (Alertmanager / alert-routing standup is a separate deferral, R-082-C, and
		// is NOT the driver here — alert_delivery reports "up".)
		if strings.HasPrefix(name, "connector:") && svc.Status == "disconnected" {
			continue
		}
		// Statuses that indicate degraded health.
		switch svc.Status {
		case "down", "stale", "error", "failing", "disconnected", "degraded":
			overall = "degraded"
		}
	}

	resp := HealthResponse{
		Status: overall,
	}

	// Only expose service topology, version, and commit to authenticated callers
	// to prevent infrastructure reconnaissance (CWE-200). Unauthenticated callers
	// (including Docker healthcheck) only see the overall status.
	if authenticated {
		resp.Services = services
		resp.Version = d.Version
		resp.CommitHash = d.CommitHash
		resp.BuildTime = d.BuildTime

		// Knowledge layer health (optional — nil when knowledge is disabled)
		if knowledgeHealthCh != nil {
			resp.Knowledge = <-knowledgeHealthCh
		}

		// BUG-080-001 SCOPE-03 — Knowledge Graph capability detail. It
		// lives INSIDE this authenticated branch for the same reason the
		// service topology above does: per-family read outcomes are
		// capability detail, and an unauthenticated caller must keep
		// seeing the aggregate-only response. The detail is derived from
		// the explicit activation policy plus the published synthetic
		// aggregate, so a disabled deployment reports policy_disabled and
		// an unproven one reports unavailable — neither can read as ready.
		// It is informational: it deliberately does NOT feed the aggregate
		// `status` above, because a deliberately-disabled Graph capability
		// is a valid deployment state and not a process fault.
		if d.GraphReadiness != nil {
			graph := d.GraphReadiness.Snapshot()
			resp.Graph = &graph
		}
	}

	// Liveness vs readiness (redteam F1 / BUG-050-002): /api/health is ALWAYS
	// 200 by default because it is the container liveness probe — a "degraded"
	// process is still ALIVE, and returning 503 here would make the Docker
	// HEALTHCHECK mark the container unhealthy and flap/restart it. Callers on
	// the operator / monitoring / knb verify.sh path OPT IN to a status-aware
	// signal via ?strict=true, which returns 503 when the aggregate status is
	// not "healthy". The default (no param — what the Docker healthcheck sends)
	// is byte-for-byte unchanged.
	statusCode := http.StatusOK
	if overall != "healthy" && healthStrictRequested(r) {
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(w, statusCode, resp)
}

// healthStrictRequested reports whether the caller opted into a status-aware
// /api/health via ?strict=true|1|yes. Only the operator / monitoring path sets
// it; the Docker liveness HEALTHCHECK never does, so container liveness is
// unaffected (redteam F1 / BUG-050-002).
func healthStrictRequested(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("strict"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// ReadyzHandler handles GET /readyz — a lightweight readiness probe.
// Only checks core DB connectivity. Returns 200 when the service can serve
// requests, 503 when it cannot. Intended for Docker HEALTHCHECK and
// orchestrator readiness probes (separate from the full /api/health liveness check).
//
// BUG-080-001 SCOPE-03 — callers on the operator / monitoring / knb
// verify.sh path OPT IN to STRICT readiness via ?strict=true, which
// additionally requires the Knowledge Graph journey to be PROVEN ready:
// the explicit activation policy must be ENABLED and a fresh published
// graphsynthetic aggregate must report available. Static Wiki assets, a
// mounted graph route manifest, and general database liveness cannot
// satisfy it — the check reads d.graphJourneyReady, which has no branch
// capable of deriving ready from any of them, and which fails CLOSED
// when the readiness projection is unwired. A deliberately DISABLED
// deployment reports policy_disabled and therefore does not satisfy
// strict readiness either; that is the truthful answer, and it is why
// strict is an explicit opt-in. The default (no param — what the Docker
// HEALTHCHECK sends) is byte-for-byte unchanged.
func (d *Dependencies) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")
	if d.DB == nil || !d.DB.Healthy(ctx) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"ready":false}`))
		return
	}
	// Strict readiness stays AGGREGATE-ONLY on the wire: /readyz is an
	// unauthenticated orchestrator probe, so it reports a single boolean
	// and never the Graph capability detail. That detail is available to
	// AUTHENTICATED callers of GET /api/health only.
	if healthStrictRequested(r) && !d.graphJourneyReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"ready":false}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ready":true}`))
}

// getCachedKnowledgeHealth returns cached knowledge health stats, refreshing when stale.
// Uses RWMutex so concurrent readers don't block each other, and releases the lock
// before making the DB call to avoid serialising health checks on slow queries (C-023-C001).
func (d *Dependencies) getCachedKnowledgeHealth(ctx context.Context) *KnowledgeHealthSection {
	// Fast path: serve from cache under read lock (concurrent readers OK).
	d.knowledgeHealthMu.RLock()
	if d.KnowledgeHealthCacheTTL > 0 && d.knowledgeHealthCache != nil && time.Since(d.knowledgeHealthAt) < d.KnowledgeHealthCacheTTL {
		cached := d.knowledgeHealthCache
		d.knowledgeHealthMu.RUnlock()
		return cached
	}
	stale := d.knowledgeHealthCache
	d.knowledgeHealthMu.RUnlock()

	// Slow path: fetch fresh data WITHOUT holding any lock.
	refreshCtx, cancel := context.WithTimeout(ctx, healthAuxiliaryProbeTimeout)
	defer cancel()
	stats, err := d.KnowledgeStore.GetKnowledgeHealthStats(refreshCtx)
	if err != nil {
		slog.Warn("knowledge health stats query failed", "error", err)
		return stale // return stale cache if available
	}

	section := &KnowledgeHealthSection{
		ConceptCount:     stats.ConceptCount,
		EntityCount:      stats.EntityCount,
		SynthesisPending: stats.SynthesisPending,
		LastSynthesisAt:  stats.LastSynthesisAt,
	}

	// Update cache under write lock.
	d.knowledgeHealthMu.Lock()
	d.knowledgeHealthCache = section
	d.knowledgeHealthAt = time.Now()
	d.knowledgeHealthMu.Unlock()

	return section
}

// intelligenceHealthSnapshot caches the result of HealthHandler's two
// intelligence-engine DB probes (GetLastSynthesisTime, HasStalePendingAlerts).
// intelligenceStatus is always populated when the snapshot is valid
// ("up" | "down" | "stale"). alertDeliveryStatus is empty when the alert
// probe errored, preserving the pre-cache behaviour of omitting the
// "alert_delivery" service key from the response in that case.
type intelligenceHealthSnapshot struct {
	intelligenceStatus  string
	alertDeliveryStatus string
}

// intelligenceSynthesisFreshnessBudget is the age past which a durably-persisted
// synthesis output is reported as stale by /api/health. Preserved from the
// pre-fix 48h threshold; consumed when building the SynthesisPersistenceOutcome
// that intelligence.DeriveSynthesisHealth maps to a truthful verdict.
const intelligenceSynthesisFreshnessBudget = 48 * time.Hour

// getCachedIntelligenceHealth returns a cached intelligence/alert-delivery
// snapshot, refreshing when the TTL is exceeded. Mirrors getCachedKnowledgeHealth's
// RWMutex+TTL pattern (BUG-021-002 — stabilize R13). Without caching, every
// /api/health request triggered two synchronous DB round-trips against the
// IntelligenceEngine pool for data that updates at most once per 24h
// (synthesis) or every 15 min (alert sweep), allowing health probes to
// amplify backpressure under DB contention.
//
// The slow path computes the snapshot with no lock held — the underlying DB
// calls are the slow part, and holding the cache mutex across them would
// re-introduce the C-023-C001 serialisation bug.
func (d *Dependencies) getCachedIntelligenceHealth(ctx context.Context) *intelligenceHealthSnapshot {
	// Fast path: serve from cache under read lock when TTL allows.
	d.intelligenceHealthMu.RLock()
	if d.IntelligenceHealthCacheTTL > 0 && d.intelligenceHealthCache != nil &&
		time.Since(d.intelligenceHealthAt) < d.IntelligenceHealthCacheTTL {
		cached := d.intelligenceHealthCache
		d.intelligenceHealthMu.RUnlock()
		return cached
	}
	d.intelligenceHealthMu.RUnlock()

	// Slow path: compute fresh snapshot with no locks held.
	snapshot := &intelligenceHealthSnapshot{}
	if d.IntelligenceEngine.Pool == nil {
		snapshot.intelligenceStatus = "down"
	} else {
		// Route the queryable synthesis run-state through the single HEALTH-TRUTH
		// authority (intelligence.DeriveSynthesisHealth). This replaces the pre-fix
		// mapping where a freshness-probe error or a never-run synthesis was
		// reported "up", so a system that never produced — or could not be
		// evaluated — no longer reports green (BUG-004-004 SCOPE-04). The richer
		// per-insight/per-citation atomic post-commit read-back gate is the
		// deferred durable-persistence slice; the coarse read-back available here
		// is this successful GetLastSynthesisTime query, which reads a durable
		// synthesis_insights row back.
		lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
		var outcome intelligence.SynthesisPersistenceOutcome
		switch {
		case err != nil:
			slog.Warn("intelligence freshness check failed", "error", err)
			outcome = intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseProbeError}
		case lastSynthesis.IsZero() || lastSynthesis.Year() < 2000:
			// No synthesis has ever persisted an insight (fresh install / never-run).
			outcome = intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseNoRun}
		default:
			outcome = intelligence.SynthesisPersistenceOutcome{
				Phase:    intelligence.PhaseCommitted,
				ReadBack: intelligence.ReadBackOK,
				Output:   intelligence.OutputKindFull,
				Stale:    time.Since(lastSynthesis) > intelligenceSynthesisFreshnessBudget,
			}
		}
		snapshot.intelligenceStatus = intelligence.DeriveSynthesisHealth(outcome).IntelligenceStatus

		// Alert delivery pipeline freshness: pending alerts older than 30 minutes
		// indicate the delivery sweep is not running (2 missed sweep cycles).
		staleAlerts, err := d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)
		if err != nil {
			slog.Warn("alert delivery freshness check failed", "error", err)
			// leave alertDeliveryStatus empty — call site omits the service key.
		} else if staleAlerts {
			snapshot.alertDeliveryStatus = "stale"
		} else {
			snapshot.alertDeliveryStatus = "up"
		}
	}

	// Update cache under write lock.
	d.intelligenceHealthMu.Lock()
	d.intelligenceHealthCache = snapshot
	d.intelligenceHealthAt = time.Now()
	d.intelligenceHealthMu.Unlock()

	return snapshot
}

// isAuthenticated checks whether the request carries a valid Bearer token.
// Returns false when no AuthToken is configured (dev mode allows all).
func (d *Dependencies) isAuthenticated(r *http.Request) bool {
	if d.AuthToken == "" {
		return true // dev mode — no auth required
	}
	return matchBearerToken(r, d.AuthToken)
}

// mlClient returns the shared HTTP client for ML sidecar health checks,
// initialising it on first use. Safe for concurrent access via sync.Once.
func (d *Dependencies) mlClient() *http.Client {
	d.mlClientOnce.Do(func() {
		if d.MLClient == nil {
			d.MLClient = &http.Client{Timeout: healthAuxiliaryProbeTimeout}
		}
	})
	return d.MLClient
}

// probeHTTPGet issues a bounded GET against url and reports whether the
// response status was 200 OK. The caller controls URL composition, success
// translation, and any optional response fields; this helper owns the
// shared timeout / request-build / transport / body-drain plumbing so the
// two service-health probes below do not duplicate it (BUG-023-002).
func probeHTTPGet(ctx context.Context, url string, client *http.Client) bool {
	probeCtx, cancel := context.WithTimeout(ctx, healthAuxiliaryProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	return resp.StatusCode == http.StatusOK
}

// mlHealthBody mirrors the ML sidecar's GET /health JSON contract
// ({"status": "...", "model_loaded": bool}). Only the fields the aggregate
// health surface reflects are decoded.
type mlHealthBody struct {
	Status      string `json:"status"`
	ModelLoaded bool   `json:"model_loaded"`
}

// checkMLSidecar probes the ML sidecar health endpoint and REFLECTS the
// sidecar's self-reported status + model_loaded instead of fabricating them
// (redteam F7 / BUG-050-001). The previous implementation hardcoded
// Status:"up" + ModelLoaded:true on any 200, so the aggregate health surface
// actively misreported an ML sidecar whose model had not loaded
// ({"status":"up","model_loaded":false}).
func checkMLSidecar(ctx context.Context, baseURL string, client *http.Client) ServiceStatus {
	if baseURL == "" {
		return ServiceStatus{Status: "not_configured"}
	}

	probeCtx, cancel := context.WithTimeout(ctx, healthAuxiliaryProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return ServiceStatus{Status: "down"}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{Status: "down"}
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return ServiceStatus{Status: "down"}
	}

	// 200 OK — the sidecar is reachable. Decode its self-report. A 200 with an
	// empty/unparseable body means reachable-but-unknown: report "up" WITHOUT a
	// fabricated model_loaded claim rather than inventing one.
	var body mlHealthBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ServiceStatus{Status: "up"}
	}
	status := body.Status
	if status == "" {
		status = "up"
	}
	loaded := body.ModelLoaded
	return ServiceStatus{Status: status, ModelLoaded: &loaded}
}

// checkOllama probes the Ollama health endpoint.
func checkOllama(ctx context.Context, ollamaURL string, client *http.Client) ServiceStatus {
	if ollamaURL == "" {
		return ServiceStatus{Status: "not_configured"}
	}
	if !probeHTTPGet(ctx, ollamaURL+"/api/tags", client) {
		return ServiceStatus{Status: "down"}
	}
	return ServiceStatus{Status: "up"}
}
