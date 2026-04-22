# Report: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Traced NATS publish calls in Go intelligence code; found publish-without-subscribe pattern

## Evidence
- `internal/intelligence/monthly.go:250`: publishes to `smk.monthly.generate` then immediately calls `assembleMonthlyReportText` — never awaits response
- `internal/intelligence/monthly.go:389`: publishes to `smk.content.analyze` then immediately generates template angles — never awaits response
- `internal/intelligence/learning.go`: no NATS publish at all — uses `classifyDifficultyHeuristic` only
- `internal/intelligence/lookups.go`: no NATS publish — stores pre-computed content
- `internal/intelligence/monthly.go::DetectSeasonalPatterns`: no NATS publish — SQL-only
