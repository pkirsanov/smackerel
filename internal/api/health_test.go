package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
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

// SCN-023-01: Concurrent health checks are race-free via sync.Once on mlClient.
func TestMLClient_ConcurrentAccess(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	const goroutines = 50
	clients := make(chan *http.Client, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			clients <- deps.mlClient()
		}()
	}

	var first *http.Client
	for i := 0; i < goroutines; i++ {
		c := <-clients
		if c == nil {
			t.Fatal("mlClient() returned nil")
		}
		if first == nil {
			first = c
		} else if c != first {
			t.Fatal("mlClient() returned different pointers under concurrent access")
		}
	}
}

// SCN-023-01: mlClient respects pre-set MLClient value.
func TestMLClient_PreSet(t *testing.T) {
	preset := &http.Client{Timeout: 99 * time.Second}
	deps := &Dependencies{
		MLClient: preset,
	}

	got := deps.mlClient()
	if got != preset {
		t.Fatal("mlClient() should return pre-set MLClient")
	}
}

// SCN-023-01: Concurrent HealthHandler calls are race-free.
// Exercises the full handler path (DB, NATS, ML, Ollama, Telegram) under
// parallel access to catch races deeper than the mlClient() pointer test.
func TestHealthHandler_ConcurrentAccess(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ollamaServer.Close()

	mlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mlServer.Close()

	deps := &Dependencies{
		DB:           &mockDB{healthy: true, artifactCount: 5},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now().Add(-30 * time.Second),
		MLSidecarURL: mlServer.URL,
		OllamaURL:    ollamaServer.URL,
		TelegramBot:  &mockTelegramHealth{healthy: true},
	}

	const goroutines = 50
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			rec := httptest.NewRecorder()
			deps.HealthHandler(rec, req)

			if rec.Code != http.StatusOK {
				errs <- fmt.Errorf("expected 200, got %d", rec.Code)
				return
			}

			var resp HealthResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				errs <- fmt.Errorf("decode: %v", err)
				return
			}
			if resp.Status != "healthy" {
				errs <- fmt.Errorf("expected healthy, got %s", resp.Status)
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// mockTelegramHealth implements TelegramHealthChecker for testing.
type mockTelegramHealth struct {
	healthy bool
}

func (m *mockTelegramHealth) Healthy() bool { return m.healthy }

// SCN-023-06: Ollama health reflects actual reachability.
func TestCheckOllama_Healthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	status := checkOllama(context.Background(), ts.URL, ts.Client())
	if status.Status != "up" {
		t.Errorf("expected up, got %s", status.Status)
	}
}

func TestCheckOllama_Down(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	status := checkOllama(context.Background(), ts.URL, ts.Client())
	if status.Status != "down" {
		t.Errorf("expected down, got %s", status.Status)
	}
}

func TestCheckOllama_NotConfigured(t *testing.T) {
	status := checkOllama(context.Background(), "", &http.Client{})
	if status.Status != "not_configured" {
		t.Errorf("expected not_configured, got %s", status.Status)
	}
}

func TestCheckOllama_Unreachable(t *testing.T) {
	status := checkOllama(context.Background(), "http://127.0.0.1:1", &http.Client{})
	if status.Status != "down" {
		t.Errorf("expected down when unreachable, got %s", status.Status)
	}
}

// SCN-023-07: Telegram bot health reflects actual connection state.
func TestHealthHandler_TelegramConnected(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		TelegramBot: &mockTelegramHealth{healthy: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Services["telegram_bot"].Status != "connected" {
		t.Errorf("expected connected, got %s", resp.Services["telegram_bot"].Status)
	}
}

func TestHealthHandler_TelegramDisconnected(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		// TelegramBot is nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Services["telegram_bot"].Status != "disconnected" {
		t.Errorf("expected disconnected, got %s", resp.Services["telegram_bot"].Status)
	}
}

// SCN-023-06: Health endpoint shows live Ollama status.
func TestHealthHandler_OllamaUp(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ollamaServer.Close()

	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		OllamaURL: ollamaServer.URL,
		MLClient:  ollamaServer.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Services["ollama"].Status != "up" {
		t.Errorf("expected ollama up, got %s", resp.Services["ollama"].Status)
	}
}

func TestHealthHandler_OllamaNotConfigured(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		// OllamaURL is empty
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Services["ollama"].Status != "not_configured" {
		t.Errorf("expected not_configured, got %s", resp.Services["ollama"].Status)
	}
}

// SCN-023-08: Health check requests excluded from request log.
func TestStructuredLogger_HealthExcluded(t *testing.T) {
	// Capture slog output
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := structuredLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// /api/health should not produce log output
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no log output for /api/health, got: %s", buf.String())
	}
}

func TestStructuredLogger_PingExcluded(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := structuredLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() > 0 {
		t.Errorf("expected no log output for /ping, got: %s", buf.String())
	}
}

func TestStructuredLogger_OtherPathsLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := structuredLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/capture", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() == 0 {
		t.Error("expected log output for /api/capture, got none")
	}
}

// SCN-021-012: Health reports down (and degraded) when intelligence pool is nil.
// Note: the stale path (Pool non-nil, synthesis >48h) requires a real DB connection
// and is covered by integration tests, since GetLastSynthesisTime queries the DB.
func TestHealthHandler_IntelligenceDown(t *testing.T) {
	// IntelligenceEngine with nil Pool → reported as "down"
	engine := &intelligence.Engine{Pool: nil}
	deps := &Dependencies{
		DB:                 &mockDB{healthy: true},
		NATS:               &mockNATS{healthy: true},
		StartTime:          time.Now(),
		IntelligenceEngine: engine,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Services["intelligence"].Status != "down" {
		t.Errorf("expected intelligence down when pool is nil, got %s", resp.Services["intelligence"].Status)
	}
	if resp.Status != "degraded" {
		t.Errorf("expected degraded when intelligence is down, got %s", resp.Status)
	}
}

// SCN-021-013: Health reports up when IntelligenceEngine is nil (not configured)
func TestHealthHandler_IntelligenceNilEngine(t *testing.T) {
	deps := &Dependencies{
		DB:                 &mockDB{healthy: true},
		NATS:               &mockNATS{healthy: true},
		StartTime:          time.Now(),
		IntelligenceEngine: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Intelligence service should not appear when engine is nil
	if _, ok := resp.Services["intelligence"]; ok {
		t.Error("expected no intelligence service when engine is nil")
	}
}

// === Security: Health endpoint fingerprinting prevention ===

func TestHealthHandler_UnauthenticatedHidesVersionAndCommit(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		Version:    "1.2.3",
		CommitHash: "abc123",
		AuthToken:  "secret-token",
	}

	// Request WITHOUT auth header — should not expose version/commit
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Version != "" {
		t.Errorf("unauthenticated health must not expose version, got %q", resp.Version)
	}
	if resp.CommitHash != "" {
		t.Errorf("unauthenticated health must not expose commit hash, got %q", resp.CommitHash)
	}
}

func TestHealthHandler_AuthenticatedShowsVersionAndCommit(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		Version:    "1.2.3",
		CommitHash: "abc123",
		AuthToken:  "secret-token",
	}

	// Request WITH valid Bearer token — should expose version/commit
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Version != "1.2.3" {
		t.Errorf("authenticated health should show version, got %q", resp.Version)
	}
	if resp.CommitHash != "abc123" {
		t.Errorf("authenticated health should show commit hash, got %q", resp.CommitHash)
	}
}

func TestHealthHandler_DevModeShowsVersionAndCommit(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		Version:    "dev",
		CommitHash: "dev123",
		AuthToken:  "", // dev mode — no auth required
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	deps.HealthHandler(rec, req)

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Version != "dev" {
		t.Errorf("dev mode health should show version, got %q", resp.Version)
	}
	if resp.CommitHash != "dev123" {
		t.Errorf("dev mode health should show commit hash, got %q", resp.CommitHash)
	}
}
