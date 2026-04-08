package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Dependencies holds shared service dependencies for API handlers.
type Dependencies struct {
	DB           DBHealthChecker
	NATS         NATSHealthChecker
	StartTime    time.Time
	MLSidecarURL string
	MLClient     *http.Client
	Pipeline     interface{}
	SearchEngine interface{}
	DigestGen    interface{}
	WebHandler   interface{}
	OAuthHandler interface{}
	AuthToken    string
	Version      string
	CommitHash   string
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

// HealthResponse is the JSON response for GET /api/health.
type HealthResponse struct {
	Status     string                   `json:"status"`
	Version    string                   `json:"version,omitempty"`
	CommitHash string                   `json:"commit_hash,omitempty"`
	Services   map[string]ServiceStatus `json:"services"`
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

	// ML sidecar status
	mlStatus := checkMLSidecar(ctx, d.MLSidecarURL, d.mlClient())
	services["ml_sidecar"] = mlStatus

	// Telegram bot (placeholder — not yet wired)
	services["telegram_bot"] = ServiceStatus{Status: "disconnected"}

	// Ollama (placeholder — optional)
	services["ollama"] = ServiceStatus{Status: "unavailable"}

	// Aggregate status
	overall := "healthy"
	for name, svc := range services {
		if name == "telegram_bot" || name == "ollama" {
			continue // optional services don't affect overall status
		}
		if svc.Status == "down" {
			overall = "degraded"
		}
	}

	resp := HealthResponse{
		Status:     overall,
		Version:    d.Version,
		CommitHash: d.CommitHash,
		Services:   services,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("failed to encode health response", "error", err)
	}
}

// mlClient returns the shared HTTP client for ML sidecar health checks,
// initialising it on first use.
func (d *Dependencies) mlClient() *http.Client {
	if d.MLClient == nil {
		d.MLClient = &http.Client{Timeout: 2 * time.Second}
	}
	return d.MLClient
}

// checkMLSidecar probes the ML sidecar health endpoint.
func checkMLSidecar(ctx context.Context, baseURL string, client *http.Client) ServiceStatus {
	if baseURL == "" {
		return ServiceStatus{Status: "down"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return ServiceStatus{Status: "down"}
	}

	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{Status: "down"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		loaded := true
		return ServiceStatus{Status: "up", ModelLoaded: &loaded}
	}
	return ServiceStatus{Status: "down"}
}
