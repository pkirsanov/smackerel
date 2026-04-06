package api

import (
	"context"
	"encoding/json"
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
