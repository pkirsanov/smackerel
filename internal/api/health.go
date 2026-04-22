package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
	DigestPage(w http.ResponseWriter, r *http.Request)
	TopicsPage(w http.ResponseWriter, r *http.Request)
	SettingsPage(w http.ResponseWriter, r *http.Request)
	StatusPage(w http.ResponseWriter, r *http.Request)
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
	OAuthHandler       OAuthFlow
	TelegramBot        TelegramHealthChecker
	ConnectorRegistry  ConnectorHealthLister
	ArtifactStore      ArtifactQuerier
	ContextHandler     *ContextHandler
	BookmarkPub        BookmarkPublisher
	OllamaURL          string
	AuthToken          string
	Version            string
	CommitHash         string
	BuildTime          string

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

	// Actionable list handlers (optional — nil when lists not configured)
	ListHandlers *ListHandlers

	// Expense handler (optional — nil when expenses not enabled)
	ExpenseHandler *ExpenseHandler

	// Meal plan handler (optional — nil when meal planning not enabled)
	MealPlanHandler *MealPlanHandler

	// CORS allowed origins (SST-compliant — from smackerel.yaml via config generate)
	CORSAllowedOrigins []string
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

// HealthHandler handles GET /api/health.
func (d *Dependencies) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	services := make(map[string]ServiceStatus)

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
	// Each probe has a 2s context timeout; sequential execution would
	// bottleneck at 4s+ when both services are unreachable, exceeding
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

	// Intelligence engine status — runs while external probes are in flight
	if d.IntelligenceEngine != nil {
		if d.IntelligenceEngine.Pool == nil {
			services["intelligence"] = ServiceStatus{Status: "down"}
		} else {
			lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
			if err != nil {
				slog.Warn("intelligence freshness check failed", "error", err)
				services["intelligence"] = ServiceStatus{Status: "up"}
			} else if lastSynthesis.IsZero() || lastSynthesis.Year() < 2000 {
				// No synthesis has ever run (fresh install) — not stale, just not started
				services["intelligence"] = ServiceStatus{Status: "up"}
			} else if time.Since(lastSynthesis) > 48*time.Hour {
				services["intelligence"] = ServiceStatus{Status: "stale"}
			} else {
				services["intelligence"] = ServiceStatus{Status: "up"}
			}

			// Alert delivery pipeline freshness: pending alerts older than 30 minutes
			// indicate the delivery sweep is not running (2 missed sweep cycles).
			staleAlerts, err := d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)
			if err != nil {
				slog.Warn("alert delivery freshness check failed", "error", err)
			} else if staleAlerts {
				services["alert_delivery"] = ServiceStatus{Status: "stale"}
			} else {
				services["alert_delivery"] = ServiceStatus{Status: "up"}
			}
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
		// Connector-specific statuses that indicate degraded health
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
	if d.isAuthenticated(r) {
		resp.Services = services
		resp.Version = d.Version
		resp.CommitHash = d.CommitHash
		resp.BuildTime = d.BuildTime

		// Knowledge layer health (optional — nil when knowledge is disabled)
		if d.KnowledgeStore != nil {
			resp.Knowledge = d.getCachedKnowledgeHealth(ctx)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ReadyzHandler handles GET /readyz — a lightweight readiness probe.
// Only checks core DB connectivity. Returns 200 when the service can serve
// requests, 503 when it cannot. Intended for Docker HEALTHCHECK and
// orchestrator readiness probes (separate from the full /api/health liveness check).
func (d *Dependencies) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if d.DB == nil || !d.DB.Healthy(ctx) {
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
	stats, err := d.KnowledgeStore.GetKnowledgeHealthStats(ctx)
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
			d.MLClient = &http.Client{Timeout: 2 * time.Second}
		}
	})
	return d.MLClient
}

// checkMLSidecar probes the ML sidecar health endpoint.
func checkMLSidecar(ctx context.Context, baseURL string, client *http.Client) ServiceStatus {
	if baseURL == "" {
		return ServiceStatus{Status: "not_configured"}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	if resp.StatusCode == http.StatusOK {
		loaded := true
		return ServiceStatus{Status: "up", ModelLoaded: &loaded}
	}
	return ServiceStatus{Status: "down"}
}

// checkOllama probes the Ollama health endpoint.
func checkOllama(ctx context.Context, ollamaURL string, client *http.Client) ServiceStatus {
	if ollamaURL == "" {
		return ServiceStatus{Status: "not_configured"}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, ollamaURL+"/api/tags", nil)
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

	if resp.StatusCode == http.StatusOK {
		return ServiceStatus{Status: "up"}
	}
	return ServiceStatus{Status: "down"}
}
