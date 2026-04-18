package api

import (
	"context"
	"log/slog"
	"time"
)

// WaitForMLReady blocks until the ML sidecar is healthy or the timeout elapses.
// Returns true if the sidecar became healthy, false if timeout was reached.
// On timeout, the search engine will use text fallback permanently until the
// next health check detects recovery.
func (s *SearchEngine) WaitForMLReady(ctx context.Context, timeout time.Duration) bool {
	if s.MLSidecarURL == "" {
		slog.Warn("ML sidecar URL not configured — skipping readiness wait")
		return false
	}

	if timeout <= 0 {
		slog.Warn("ML readiness timeout is zero — skipping readiness wait")
		return false
	}

	slog.Info("waiting for ML sidecar readiness", "timeout", timeout, "url", s.MLSidecarURL)

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			slog.Warn("ML sidecar readiness timeout — using text fallback",
				"timeout", timeout, "url", s.MLSidecarURL)
			s.mlHealthy.Store(false)
			s.mlHealthAt.Store(time.Now().UnixNano())
			return false
		case <-ctx.Done():
			slog.Warn("ML readiness wait cancelled", "error", ctx.Err())
			return false
		case <-ticker.C:
			if s.probeMLHealth(ctx) {
				slog.Info("ML sidecar is ready", "url", s.MLSidecarURL)
				s.mlHealthy.Store(true)
				s.mlHealthAt.Store(time.Now().UnixNano())
				return true
			}
		}
	}
}
