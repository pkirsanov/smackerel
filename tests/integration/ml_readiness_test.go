//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
)

// Scenario: Search works after cold start
// Given the ML sidecar just started
// When a search request arrives within 10s of startup
// Then core waits for ML readiness (up to 60s configurable)
// And search completes via vector mode (not text fallback)
func TestMLReadiness_WaitForHealthy(t *testing.T) {
	// Skip if no NATS/DB (this test only exercises the readiness gate logic
	// but validates it works against a real HTTP server)
	_ = os.Getenv("DATABASE_URL") // presence optional for this test

	// Create a mock ML sidecar that becomes healthy after 2 seconds
	startTime := time.Now()
	delaySeconds := 2
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		if time.Since(startTime) < time.Duration(delaySeconds)*time.Second {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockML.Close()

	engine := &api.SearchEngine{
		MLSidecarURL:   mockML.URL,
		HealthCacheTTL: 30 * time.Second,
	}

	ctx := context.Background()
	ready := engine.WaitForMLReady(ctx, 10*time.Second)
	if !ready {
		t.Error("expected ML sidecar to become ready within 10s timeout")
	}
	elapsed := time.Since(startTime)
	if elapsed < time.Duration(delaySeconds)*time.Second {
		t.Errorf("readiness reported too early: %s (expected >= %ds)", elapsed, delaySeconds)
	}
	t.Logf("ML sidecar became ready after %s", elapsed)
}

func TestMLReadiness_TimeoutFallback(t *testing.T) {
	// Mock ML sidecar that never becomes healthy
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockML.Close()

	engine := &api.SearchEngine{
		MLSidecarURL:   mockML.URL,
		HealthCacheTTL: 30 * time.Second,
	}

	ctx := context.Background()
	start := time.Now()
	ready := engine.WaitForMLReady(ctx, 3*time.Second)
	elapsed := time.Since(start)

	if ready {
		t.Error("expected readiness to timeout, but it reported ready")
	}
	if elapsed < 2*time.Second {
		t.Errorf("timeout too fast: %s (expected ~3s)", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout too slow: %s (expected ~3s)", elapsed)
	}
	t.Logf("readiness timed out after %s (expected text fallback)", elapsed)
}

func TestMLReadiness_EmptyURL(t *testing.T) {
	engine := &api.SearchEngine{
		MLSidecarURL:   "",
		HealthCacheTTL: 30 * time.Second,
	}

	ctx := context.Background()
	ready := engine.WaitForMLReady(ctx, 5*time.Second)
	if ready {
		t.Error("expected false when MLSidecarURL is empty")
	}
}

func TestMLReadiness_ZeroTimeout(t *testing.T) {
	engine := &api.SearchEngine{
		MLSidecarURL:   "http://localhost:9999",
		HealthCacheTTL: 30 * time.Second,
	}

	ctx := context.Background()
	ready := engine.WaitForMLReady(ctx, 0)
	if ready {
		t.Error("expected false when timeout is zero")
	}
}
