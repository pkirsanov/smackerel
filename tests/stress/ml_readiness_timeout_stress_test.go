//go:build stress

// Package stress — BUG-031-006:Scope-3 SLA stress profile for spec 031 Scope 6
// (ML Sidecar Readiness Gate).
//
// SCN-BUG-031-006-005, SCN-BUG-031-006-006, SCN-BUG-031-006-007: assert the
// configurable ML readiness timeout fires at the value documented by the SST
// contract, and that adversarial regressions in internal/api/ml_readiness.go
// are detected before merge.
//
// SST contract:
//
//	config/smackerel.yaml services.ml.readiness_timeout_s
//	→ ML_READINESS_TIMEOUT_S (canonical SST env var)
//	→ internal/config.Config.MLReadinessTimeoutS
//	→ cmd/core/services.go calls
//	   SearchEngine.WaitForMLReady(ctx, time.Duration(MLReadinessTimeoutS)*time.Second).
//
// BUG-031-006:Scope-3 DoD names two logical aliases as acceptable input names:
// SMACKEREL_ML_READINESS_TIMEOUT (alias for the timeout) and ML_BASE_URL (alias
// for the ML sidecar URL). This test reads the alias first, falls back to the
// canonical SST name, and fails loud per smackerel-no-defaults policy when both
// are unset. Either name satisfies the contract.
//
// The test exercises the production SearchEngine.WaitForMLReady code path via
// an in-process httptest mock that simulates the ML sidecar; it does not
// require a running ML container. The disposable-test-stack guard
// (requireDisposableStack) still asserts that NO env value points at the
// persistent dev/prod stack — so the test will refuse to run if accidentally
// wired to the dev environment.
package stress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
)

// sstReadinessTimeout reads the ML readiness timeout from the SST env contract.
// Order of precedence:
//  1. SMACKEREL_ML_READINESS_TIMEOUT (BUG-031-006:Scope-3 DoD alias; Go duration
//     OR integer seconds)
//  2. ML_READINESS_TIMEOUT_S (canonical SST env; integer seconds)
//
// Adversarial case 4 (missing-env fail-loud): when both are empty the test MUST
// fail loud — no hidden default, per smackerel-no-defaults policy.
func sstReadinessTimeout(t *testing.T) time.Duration {
	t.Helper()
	raw := os.Getenv("SMACKEREL_ML_READINESS_TIMEOUT")
	source := "SMACKEREL_ML_READINESS_TIMEOUT"
	if raw == "" {
		raw = os.Getenv("ML_READINESS_TIMEOUT_S")
		source = "ML_READINESS_TIMEOUT_S"
	}
	if raw == "" {
		t.Fatalf("stress: neither SMACKEREL_ML_READINESS_TIMEOUT nor ML_READINESS_TIMEOUT_S is set — SST contract requires the ML readiness timeout (smackerel-no-defaults policy; fail-loud)")
	}
	// Accept either Go duration ("60s", "2m") or integer seconds ("60").
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		t.Fatalf("stress: %s=%q must be a positive integer seconds or a Go duration", source, raw)
	}
	return time.Duration(secs) * time.Second
}

// requireDisposableStack (adversarial case 3: wrong-stack URL fails fast)
// asserts that NO env var points at the persistent dev/prod stack. The
// disposable test stack uses Compose project name `smackerel-test`, named
// volumes prefixed `smackerel-test-`, and host port range 47001-47004. Any
// match against a dev/prod marker MUST fail loud.
func requireDisposableStack(t *testing.T) {
	t.Helper()
	devMarkers := []string{
		"smackerel-dev",
		"smackerel-prod",
		// Host-port prefixes that appear ONLY in dev/prod CORE_EXTERNAL_URL
		// (dev http://127.0.0.1:40001-40004, home-lab/prod :41001-41004). The
		// internal container ports (:8080/:8081/:5432/:4222) are shared by EVERY
		// stack — including the disposable test stack — so they cannot separate
		// dev/prod and previously false-positived the test stack's own internal
		// core URL (http://smackerel-core:8080). The test stack's core host port
		// is :45001, which these dev/prod prefixes never match.
		":4000", // dev core host-port prefix (CORE_EXTERNAL_URL 40001-40004)
		":4100", // home-lab/prod core host-port prefix (CORE_EXTERNAL_URL 41001-41004)
	}
	for _, key := range []string{
		"CORE_EXTERNAL_URL",
		"DATABASE_URL",
		"NATS_URL",
		"ML_SIDECAR_URL",
		"ML_BASE_URL",
	} {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		for _, marker := range devMarkers {
			if strings.Contains(value, marker) {
				t.Fatalf("stress: %s=%q contains persistent dev/prod stack marker %q — refuse to run; this stress test requires the disposable test stack (Compose project smackerel-test, ports 47001-47004, named volumes smackerel-test-*)", key, value, marker)
			}
		}
	}
}

// adversarialBoundary returns the boundary value passed to WaitForMLReady. The
// SST contract defines a 60-second default. BUG-031-006 design.md Risks &
// Mitigations explicitly allows SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE as a
// compress hook for CI feedback ("Use SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE=2s
// in the stress stack to compress the loop while still proving the boundary
// code path"). When the override is unset the SST-configured value is used.
func adversarialBoundary(t *testing.T) time.Duration {
	t.Helper()
	if override := os.Getenv("SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE"); override != "" {
		if d, err := time.ParseDuration(override); err == nil && d > 0 {
			t.Logf("stress: using SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE=%s (design-allowed compress hook)", d)
			return d
		}
		t.Fatalf("stress: SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE=%q must be a positive Go duration", override)
	}
	return sstReadinessTimeout(t)
}

// TestMLReadinessTimeoutBoundary (SCN-BUG-031-006-005) drives the production
// internal/api.SearchEngine.WaitForMLReady against a controllable mock ML
// server and asserts:
//
//  1. SST env contract honoured (fail-loud when both alias + canonical are
//     unset) — adversarial case 4.
//  2. The timeout fires at the configured boundary within tolerance —
//     adversarial case 1 (silent-bypass detection).
//  3. The production probe loop is actually exercised (probeCount > 0).
//  4. The test never points at the persistent dev stack — adversarial case 3.
//
// If a hypothetical edit removes the 60-second timeout from
// internal/api/ml_readiness.go, WaitForMLReady would either loop forever (test
// times out → adversarial-too-slow fail) or return ready=true (boundary fail).
func TestMLReadinessTimeoutBoundary(t *testing.T) {
	requireDisposableStack(t)
	_ = sstReadinessTimeout(t) // prove SST env contract is honoured before any mock setup

	boundary := adversarialBoundary(t)

	var probeCount int32
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&probeCount, 1)
		// 503 = ML sidecar is never ready. WaitForMLReady MUST honour the
		// configured timeout boundary and return false.
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockML.Close()

	engine := &api.SearchEngine{
		MLSidecarURL:   mockML.URL,
		HealthCacheTTL: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), boundary+5*time.Second)
	defer cancel()

	start := time.Now()
	ready := engine.WaitForMLReady(ctx, boundary)
	elapsed := time.Since(start)
	probes := atomic.LoadInt32(&probeCount)

	if ready {
		t.Fatalf("adversarial-silent-bypass: WaitForMLReady returned ready=true while mock /health returned 503 (probes=%d elapsed=%s) — timeout boundary was bypassed", probes, elapsed)
	}

	// Tolerance: WaitForMLReady ticks at 500ms; mock handler latency is
	// negligible. Allow [boundary - 500ms, boundary + 2s].
	minBound := boundary - 500*time.Millisecond
	maxBound := boundary + 2*time.Second
	if elapsed < minBound {
		t.Fatalf("adversarial-too-fast: WaitForMLReady returned after %s, before configured boundary %s (probes=%d) — timeout was not honoured", elapsed, boundary, probes)
	}
	if elapsed > maxBound {
		t.Fatalf("adversarial-too-slow: WaitForMLReady took %s vs configured boundary %s (probes=%d) — timeout drifted", elapsed, boundary, probes)
	}
	if probes == 0 {
		t.Fatalf("adversarial-no-probes: zero /health probes during %s — production code path not exercised", elapsed)
	}

	t.Logf("SLA boundary: configured=%s observed=%s probes=%d (within tolerance %s..%s)", boundary, elapsed, probes, minBound, maxBound)
}

// TestMLReadinessTimeoutSilentBypass (SCN-BUG-031-006-006) isolates the silent-
// bypass adversarial case with a deliberately compressed boundary so CI
// feedback stays fast even when SMACKEREL_ML_READINESS_TIMEOUT_OVERRIDE is
// unset. The production code path is identical to TestMLReadinessTimeoutBoundary;
// only the boundary value differs.
func TestMLReadinessTimeoutSilentBypass(t *testing.T) {
	requireDisposableStack(t)
	_ = sstReadinessTimeout(t) // SST env contract required even for the compressed variant

	const compressedBoundary = 2 * time.Second

	var probeCount int32
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&probeCount, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockML.Close()

	engine := &api.SearchEngine{
		MLSidecarURL:   mockML.URL,
		HealthCacheTTL: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), compressedBoundary+3*time.Second)
	defer cancel()

	start := time.Now()
	ready := engine.WaitForMLReady(ctx, compressedBoundary)
	elapsed := time.Since(start)
	probes := atomic.LoadInt32(&probeCount)

	if ready {
		t.Fatalf("adversarial-silent-bypass: WaitForMLReady returned ready=true with 503 mock (probes=%d elapsed=%s)", probes, elapsed)
	}
	if elapsed < compressedBoundary-500*time.Millisecond {
		t.Fatalf("adversarial-silent-bypass: returned in %s (< %s) — timeout not enforced", elapsed, compressedBoundary)
	}
	if elapsed > compressedBoundary+1500*time.Millisecond {
		t.Fatalf("adversarial-silent-bypass: returned in %s (> %s) — boundary drift", elapsed, compressedBoundary+1500*time.Millisecond)
	}
	if probes == 0 {
		t.Fatalf("adversarial-no-probes: zero probes — production loop bypassed")
	}

	t.Logf("silent-bypass guard: boundary=%s observed=%s probes=%d", compressedBoundary, elapsed, probes)
}

// TestMLReadinessAlways200Regression (SCN-BUG-031-006-007) is adversarial
// case 2: if /ml/readyz (or the underlying /health probe) returns 200
// unconditionally — i.e. the production code path is short-circuited or stubbed
// out — WaitForMLReady would return ready=true without actually probing.
// Asserting probes > 0 in addition to ready=true catches the bypass.
func TestMLReadinessAlways200Regression(t *testing.T) {
	requireDisposableStack(t)
	_ = sstReadinessTimeout(t)

	var probeCount int32
	mockML := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&probeCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockML.Close()

	engine := &api.SearchEngine{
		MLSidecarURL:   mockML.URL,
		HealthCacheTTL: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	ready := engine.WaitForMLReady(ctx, 5*time.Second)
	elapsed := time.Since(start)
	probes := atomic.LoadInt32(&probeCount)

	if !ready {
		t.Fatalf("always-200 regression: WaitForMLReady returned ready=false while mock /health returned 200 (probes=%d elapsed=%s)", probes, elapsed)
	}
	if probes == 0 {
		t.Fatalf("adversarial-no-probes: WaitForMLReady returned ready=true without probing — code path bypassed (elapsed=%s)", elapsed)
	}
	// First probe fires at 500ms (ticker cadence). Detection MUST be under 2s.
	if elapsed > 2*time.Second {
		t.Fatalf("always-200 regression: WaitForMLReady took %s to detect healthy 200 (probes=%d) — ticker cadence drift", elapsed, probes)
	}

	t.Logf("always-200 guard: ready=true within %s probes=%d", elapsed, probes)
}
