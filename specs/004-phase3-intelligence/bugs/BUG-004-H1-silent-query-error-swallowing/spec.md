# Feature: BUG-004-H1 Silent query-error swallowing in WeeklySynthesis subqueries

## Problem Statement
`internal/intelligence/synthesis.go` builds a weekly synthesis from six independent subqueries (topic movement, open loops, capture day-of-week pattern, capture hour-of-day pattern, plus the inner `RunSynthesis` and `Resurface` calls). Every subquery uses the pattern:

```go
rows, err := e.Pool.Query(ctx, `...`)
if err == nil {
    defer rows.Close()
    // ... process rows ...
}
```

When `e.Pool.Query` itself returns an error (transient pool exhaustion, schema drift, query timeout, planner failure), the entire block is silently skipped — there is no `else` branch, no `slog.Warn`, no metric increment. The weekly synthesis returns a partial result with empty `TopicMovement`, `OpenLoops`, or `Patterns` fields and no operational signal anywhere. The same `if err == nil { ... }` swallowing pattern wraps the inner `RunSynthesis` and `Resurface` calls so even synthesis-engine failures during weekly assembly are invisible to operators.

Surrounding code in the same file already logs `rows.Err()` from the iteration loop and logs `Scan` failures, which proves the engineering intent is observable failure — the `Query` error path is the only silent one. The pattern is duplicated 6 times across `GenerateWeeklySynthesis` and `detectCapturePatterns`.

## Outcome Contract
**Intent:** Every Postgres query failure in `GenerateWeeklySynthesis` and `detectCapturePatterns` produces an observable log entry so a partial weekly synthesis cannot hide a broken subquery.

**Success Signal:** Forcing one of the subqueries to fail (for example by injecting a syntactic error or by exercising the path with a closed pool) produces a `slog.Warn` line with the failing query identifier and the wrapped error, and the function still returns the surviving sections without panicking.

**Hard Constraints:**
- The fix MUST log every `Query` failure inline using the same `slog.Warn` shape already used for `rows.Err()` failures in this file.
- The fix MUST NOT change the function's success behavior or error-return contract for callers — partial-result tolerance is preserved.
- The fix MUST NOT introduce new panics, timeouts, or extra round-trips.
- The fix MUST cover all six call sites consistently.

**Failure Condition:** Any subquery failure in `GenerateWeeklySynthesis` or `detectCapturePatterns` continues to be silently absorbed without operator-visible signal.

## Goals
- Replace `if err == nil { ... }` with `if err != nil { slog.Warn(...) } else { ... }` (or equivalent early-exit) at every `Query`/`Resurface`/`RunSynthesis` call site inside the weekly synthesis path.
- Add focused regression tests proving the synthesis path tolerates a `nil` pool gracefully and that `RunSynthesis` returns an error for the nil-pool case so callers can observe the failure (already covered for `Resurface`).
- Preserve all existing test behavior. Word-count cap, quiet-week handling, and successful-path assertions stay green.

## Non-Goals
- Restructuring the weekly synthesis pipeline (NATS migration, separate jobs, etc.).
- Changing the SQL of any subquery.
- Tightening or loosening the 250-word cap.
- Touching the brief path (`briefs.go`) or alert path (`alerts.go`).

## Requirements
- Six `Query` failure paths (4 in `GenerateWeeklySynthesis`, 2 in `detectCapturePatterns`) MUST log via `slog.Warn` with the calling section identifier as a structured key.
- The two helper-call paths (`RunSynthesis`, `Resurface` invoked from `GenerateWeeklySynthesis`) MUST log on error rather than silently discarding it.
- The function MUST still return a `*WeeklySynthesis` value rather than nil when subqueries fail, so the assembled synthesis text degrades gracefully.

## User Scenarios (Gherkin)

```gherkin
Scenario: BUG-004-H1-1 Topic movement subquery failure is logged
  Given the topics-and-edges join cannot execute (transient DB failure)
  When GenerateWeeklySynthesis runs
  Then a slog.Warn line is emitted naming the topic movement section and the wrapped error
  And the returned WeeklySynthesis has empty TopicMovement
  And the function still returns the other surviving sections

Scenario: BUG-004-H1-2 Capture pattern subquery failure is logged
  Given the artifacts day-of-week query cannot execute
  When detectCapturePatterns runs
  Then a slog.Warn line is emitted naming the day-of-week pattern and the wrapped error
  And the returned patterns slice does not include the day-of-week observation

Scenario: BUG-004-H1-3 Inner synthesis failure is observable to operators
  Given the inner RunSynthesis call returns an error during weekly assembly
  When GenerateWeeklySynthesis runs
  Then a slog.Warn line is emitted naming the inner synthesis call and the wrapped error
  And the WeeklySynthesis Insights field stays empty
  And the function still returns the other surviving sections
```

## Acceptance Criteria
- All six call sites in `internal/intelligence/synthesis.go` have an explicit error branch that calls `slog.Warn` with a stable section identifier key.
- A new unit test exercises the nil-pool path for `GenerateWeeklySynthesis` and asserts the function returns a non-nil `*WeeklySynthesis` value (graceful degradation contract).
- All previously green tests in `internal/intelligence` continue to pass.
- `./smackerel.sh check` (Go vet + build) reports zero new findings against the changed file.
