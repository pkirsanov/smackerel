package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockDB implements DBHealthChecker for testing.
type mockDB struct {
	healthy       bool
	artifactCount int64
	countErr      error
}

func (m *mockDB) Healthy(_ context.Context) bool { return m.healthy }
func (m *mockDB) ArtifactCount(_ context.Context) (int64, error) {
	return m.artifactCount, m.countErr
}

// mockNATS implements NATSHealthChecker for testing.
type mockNATS struct {
	healthy bool
}

func (m *mockNATS) Healthy() bool { return m.healthy }

func TestHealthHandler_AllHealthy(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true, artifactCount: 42},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now().Add(-10 * time.Second),
		MLSidecarURL: "", // no ML sidecar in unit test
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// With no ML sidecar URL, ml_sidecar will be down, so status is degraded
	if resp.Services["api"].Status != "up" {
		t.Errorf("expected api status up, got %s", resp.Services["api"].Status)
	}
	if resp.Services["postgres"].Status != "up" {
		t.Errorf("expected postgres status up, got %s", resp.Services["postgres"].Status)
	}
	if resp.Services["nats"].Status != "up" {
		t.Errorf("expected nats status up, got %s", resp.Services["nats"].Status)
	}
	if resp.Services["postgres"].ArtifactCount == nil || *resp.Services["postgres"].ArtifactCount != 42 {
		t.Errorf("expected artifact count 42")
	}
	if resp.Services["api"].UptimeSeconds == nil || *resp.Services["api"].UptimeSeconds < 10 {
		t.Errorf("expected uptime >= 10 seconds")
	}
}

func TestHealthHandler_DBDown(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: false},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected degraded status when DB is down, got %s", resp.Status)
	}
	if resp.Services["postgres"].Status != "down" {
		t.Errorf("expected postgres down, got %s", resp.Services["postgres"].Status)
	}
}

func TestHealthHandler_NATSDown(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true, artifactCount: 0},
		NATS:      &mockNATS{healthy: false},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected degraded status when NATS is down, got %s", resp.Status)
	}
	if resp.Services["nats"].Status != "down" {
		t.Errorf("expected nats down, got %s", resp.Services["nats"].Status)
	}
}

// SCN-002-066: Health endpoint accessible without auth even when AuthToken set
func TestSCN002066_HealthNoAuth(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true, artifactCount: 1},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "secret-token",
	}

	router := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	// No Authorization header
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (health exempt from auth), got %d", rec.Code)
	}
}

// SCN-002-067: Auth middleware no-op when AuthToken empty
func TestSCN002067_AuthMiddlewareNoOp(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true, artifactCount: 1},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "", // dev mode
	}

	router := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	// No Authorization header — should still work in dev mode
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should pass auth (no-op) and hit 503/404/200 depending on deps, NOT 401
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("expected auth to be no-op in dev mode, got 401")
	}
}

func TestHealthHandler_NilDeps(t *testing.T) {
	deps := &Dependencies{
		DB:        nil,
		NATS:      nil,
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with nil deps, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("expected degraded with nil deps, got %s", resp.Status)
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestHealthHandler_ResponseStructure(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	requiredServices := []string{"api", "postgres", "nats", "ml_sidecar", "telegram_bot", "ollama"}
	for _, svc := range requiredServices {
		if _, ok := resp.Services[svc]; !ok {
			t.Errorf("missing service in health response: %s", svc)
		}
	}
}

func TestHealthHandler_BothDBAndNATSDown(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: false},
		NATS:      &mockNATS{healthy: false},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health endpoint should always return 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("expected degraded when all critical services down, got %s", resp.Status)
	}
	if resp.Services["postgres"].Status != "down" {
		t.Errorf("expected postgres down, got %s", resp.Services["postgres"].Status)
	}
	if resp.Services["nats"].Status != "down" {
		t.Errorf("expected nats down, got %s", resp.Services["nats"].Status)
	}
}

func TestHealthHandler_DBArtifactCountError(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true, artifactCount: 0, countErr: fmt.Errorf("query timeout")},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	// DB is healthy but artifact count errored — count should be omitted
	if resp.Services["postgres"].Status != "up" {
		t.Errorf("expected postgres up despite count error, got %s", resp.Services["postgres"].Status)
	}
	if resp.Services["postgres"].ArtifactCount != nil {
		t.Errorf("expected nil artifact count when query errors, got %v", *resp.Services["postgres"].ArtifactCount)
	}
}

func TestHealthHandler_VersionAndCommitHash(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		Version:    "1.2.3",
		CommitHash: "abc123",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", resp.Version)
	}
	if resp.CommitHash != "abc123" {
		t.Errorf("expected commit abc123, got %s", resp.CommitHash)
	}
}

func TestCheckMLSidecar_EmptyURL(t *testing.T) {
	status := checkMLSidecar(context.Background(), "", &http.Client{})
	if status.Status != "down" {
		t.Errorf("expected down for empty ML sidecar URL, got %s", status.Status)
	}
}

func TestCheckMLSidecar_HealthyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected /health path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	status := checkMLSidecar(context.Background(), ts.URL, ts.Client())
	if status.Status != "up" {
		t.Errorf("expected up for healthy ML sidecar, got %s", status.Status)
	}
	if status.ModelLoaded == nil || !*status.ModelLoaded {
		t.Error("expected ModelLoaded to be true")
	}
}

func TestCheckMLSidecar_UnhealthyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	status := checkMLSidecar(context.Background(), ts.URL, ts.Client())
	if status.Status != "down" {
		t.Errorf("expected down for 500 ML sidecar, got %s", status.Status)
	}
}

func TestCheckMLSidecar_ConnectionRefused(t *testing.T) {
	// Use a URL that will fail to connect
	status := checkMLSidecar(context.Background(), "http://127.0.0.1:1", &http.Client{})
	if status.Status != "down" {
		t.Errorf("expected down when ML sidecar unreachable, got %s", status.Status)
	}
}

func TestHealthHandler_MLSidecarHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		MLSidecarURL: ts.URL,
		MLClient:     ts.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("expected healthy when all services up, got %s", resp.Status)
	}
	if resp.Services["ml_sidecar"].Status != "up" {
		t.Errorf("expected ml_sidecar up, got %s", resp.Services["ml_sidecar"].Status)
	}
}

func TestHealthHandler_BearerAuthRequired(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "secret-token-for-test-1234",
	}

	router := NewRouter(deps)

	// Capture endpoint SHOULD require auth
	req := httptest.NewRequest(http.MethodPost, "/api/capture", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth token on /api/capture, got %d", rec.Code)
	}

	// With valid token
	req = httptest.NewRequest(http.MethodPost, "/api/capture", nil)
	req.Header.Set("Authorization", "Bearer secret-token-for-test-1234")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should pass auth (may get 400/500 from handler, but NOT 401)
	if rec.Code == http.StatusUnauthorized {
		t.Error("expected auth to pass with valid Bearer token")
	}
}

func TestHealthHandler_InvalidBearerToken(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "correct-secret-token-1234",
	}

	router := NewRouter(deps)
	req := httptest.NewRequest(http.MethodPost, "/api/capture", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong Bearer token, got %d", rec.Code)
	}
}
