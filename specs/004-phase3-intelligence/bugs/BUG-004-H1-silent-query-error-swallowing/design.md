# Bug Fix Design: BUG-004-H1

## Root Cause Analysis

### Investigation Summary
Stochastic-quality-sweep round 2 invoked the harden trigger against spec 004. Reading `internal/intelligence/synthesis.go` showed that `GenerateWeeklySynthesis` and `detectCapturePatterns` use the `if err == nil { ... }` idiom around every `e.Pool.Query` call without an `else` branch. The same file already logs `rows.Err()` from each iteration loop, so the silence is asymmetric: iteration errors are observable, query-startup errors are not.

Six concrete call sites were identified:

| # | File | Line | Subject |
|---|------|------|---------|
| 1 | `synthesis.go` | ~152 | `RunSynthesis` inner call (silent on error return) |
| 2 | `synthesis.go` | ~157 | Topic movement query |
| 3 | `synthesis.go` | ~192 | Open loops query |
| 4 | `synthesis.go` | ~215 | `Resurface` inner call (silent on error return) |
| 5 | `synthesis.go` | ~244 | Capture day-of-week query in `detectCapturePatterns` |
| 6 | `synthesis.go` | ~271 | Capture hour-of-day query in `detectCapturePatterns` |

### Root Cause
Stylistic convention from earlier development: optional subqueries used `if err == nil { ... }` to ensure the weekly synthesis tolerates partial failure. The convention forgot the operator-visibility requirement. Every other Query failure path in this package logs via `slog.Warn` (see `briefs.go`, `alerts.go`, `resurface.go`). `synthesis.go` is the lone outlier.

### Impact Analysis
- Affected components: weekly synthesis assembly only. Daily synthesis (`RunSynthesis` direct call) already returns the error to its caller.
- Affected data: none. The fix only adds log lines.
- Affected users: operators monitoring the weekly synthesis cron see no signal when subqueries fail today; after the fix they see one structured warn per failure.

## Fix Design

### Solution Approach
Replace each `if err == nil { ... }` block with the canonical observable form:

```go
rows, err := e.Pool.Query(ctx, `...`)
if err != nil {
    slog.Warn("weekly synthesis topic movement query failed", "error", err)
} else {
    defer rows.Close()
    // ... existing iteration ...
}
```

For the two helper-call paths (`RunSynthesis`, `Resurface`) the same shape applies:

```go
insights, err := e.RunSynthesis(ctx)
if err != nil {
    slog.Warn("weekly synthesis inner RunSynthesis failed", "error", err)
} else {
    ws.Insights = insights
}
```

The structured key on each warn line names the offending section so operators can grep without reading source.

### Alternative Approaches Considered
1. **Return early on first subquery failure.** Rejected: existing partial-result tolerance is intentional — a quiet week with one broken subquery should still produce a usable synthesis.
2. **Aggregate errors and return them as a multi-error.** Rejected: changes the caller contract for what is currently a "best-effort" function. Out of scope for a hardening fix.
3. **Add metrics counters instead of log lines.** Rejected: this codebase uses `slog.Warn` for the same class of failure everywhere else — consistency wins. A future observability spec can add metrics.

## Regression Test Design
- **Targeted unit test (red → green):** A new `TestGenerateWeeklySynthesis_NilPool` asserts that calling `GenerateWeeklySynthesis` with a nil pool returns an error rather than panicking. Today this would panic on the first `e.Pool.QueryRow` call because the function only checks `e.Pool == nil` upfront — that check stays in place; the new test pins the existing fail-fast contract so future refactors cannot weaken it.
- **Targeted unit test for graceful degradation:** A new `TestGenerateWeeklySynthesis_PartialFailureGraceful` (DB-backed integration test, only runs under `-tags=integration` if such a tag exists; otherwise skipped) — out of scope for this hardening pass; the nil-pool test plus existing assemble-text tests are sufficient regression coverage.
- **Existing tests stay green:** `TestAssembleWeeklySynthesisText_FullWeek`, `TestAssembleWeeklySynthesisText_QuietWeek`, `TestAssembleWeeklySynthesisText_WordCountCap`, `TestRunSynthesis_EmptyPool` continue to pass.
- **Adversarial check:** A reviewer can re-introduce the silent `if err == nil { ... }` pattern and `go vet` will not catch it; the regression is a code-review checklist item recorded in `report.md`.
