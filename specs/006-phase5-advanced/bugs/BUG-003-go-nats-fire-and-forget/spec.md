# Bug: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes Without Response Consumption

> **Parent Spec:** [specs/006-phase5-advanced](../../spec.md)
> **Severity:** Moderate
> **Found By:** bubbles.gaps
> **Date:** April 22, 2026

## Problem

The Go runtime publishes to Phase 5 NATS subjects (`smk.monthly.generate`, `smk.content.analyze`) but never waits for or consumes the ML sidecar's response. It immediately falls through to local fallback generation.

### Specific locations:
1. `monthly.go:250` — publishes to `smk.monthly.generate`, then immediately calls `assembleMonthlyReportText()` regardless
2. `monthly.go:389` — publishes to `smk.content.analyze`, then immediately generates template writing angles regardless

### Missing publishes:
3. `learning.go` — never publishes to `smk.learning.classify` at all; uses local heuristic only
4. `lookups.go` — never publishes to `smk.quickref.generate`; stores pre-computed content directly
5. `monthly.go::DetectSeasonalPatterns` — never publishes to `smk.seasonal.analyze`

## Impact

Even after BUG-001 is fixed (ML sidecar handlers added), the Go runtime won't use LLM-enhanced results because it doesn't consume the responses. Features that should benefit from LLM intelligence produce formulaic local output instead.

## Expected Behavior

For features requiring LLM delegation, the Go runtime should:
1. Publish the request to NATS
2. Subscribe to/await the response on the paired response subject
3. Use the LLM-enhanced result when available
4. Fall back to local generation only if NATS publish fails or times out
