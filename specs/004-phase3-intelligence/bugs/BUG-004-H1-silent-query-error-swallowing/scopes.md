# Scopes: BUG-004-H1 Silent query-error swallowing in WeeklySynthesis subqueries

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Scope 1: Make all six WeeklySynthesis subquery failures observable

**Status:** Done
**Priority:** P2 (hardening — operator visibility, no functional defect)
**Depends On:** none (self-contained)

### Gherkin Scenarios

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

### Implementation Plan
- Patch `internal/intelligence/synthesis.go`:
  - Replace `if err == nil { ... }` with `if err != nil { slog.Warn(...) } else { ... }` at the topic movement query, open loops query, capture day-of-week query, and capture hour-of-day query call sites.
  - Replace `if err == nil { ws.Insights = insights }` and `if err == nil { ws.SerendipityPicks = candidates }` with explicit `if err != nil { slog.Warn(...) } else { ... }` shapes.
  - Use a stable structured key naming the section (`weekly synthesis topic movement query failed`, etc.) for grep-friendliness.
- Add a unit test `TestGenerateWeeklySynthesis_NilPool` asserting the nil-pool fail-fast contract is preserved and named.

### Implementation Files
- `internal/intelligence/synthesis.go`
- `internal/intelligence/synthesis_test.go`

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Nil pool fail-fast contract preserved | Unit | internal/intelligence/synthesis_test.go | BUG-004-H1-3 |
| 2 | Existing weekly synthesis text assembly stays green | Unit (regression) | internal/intelligence/synthesis_test.go | (regression) |
| 3 | Existing word-cap behavior stays green | Unit (regression) | internal/intelligence/synthesis_test.go | (regression) |
| 4 | Existing run-synthesis empty-pool path stays green | Unit (regression) | internal/intelligence/synthesis_test.go | (regression) |

### Definition of Done
- [x] All six query/helper failure paths in `GenerateWeeklySynthesis` and `detectCapturePatterns` log via `slog.Warn` with a stable section key
  > Evidence: Patched `internal/intelligence/synthesis.go` lines 152, 157, 192, 215, 244, 271. Each `if err != nil { slog.Warn(...) } else { ... }` shape names the section: `weekly synthesis inner RunSynthesis failed`, `weekly synthesis topic movement query failed`, `weekly synthesis open loops query failed`, `weekly synthesis inner Resurface failed`, `capture pattern day-of-week query failed`, `capture pattern hour-of-day query failed`. Verified via `grep -n "slog.Warn" internal/intelligence/synthesis.go` — 11 warn sites total (5 pre-existing iteration warnings + 6 new query/helper warnings).
- [x] New `TestGenerateWeeklySynthesis_NilPool` test asserts nil-pool fail-fast contract
  > Evidence: Added test in `internal/intelligence/synthesis_test.go` that constructs an `Engine{Pool: nil}` and asserts `GenerateWeeklySynthesis` returns the documented error string `"weekly synthesis requires a database connection"` without panicking. Test passes: `--- PASS: TestGenerateWeeklySynthesis_NilPool (0.00s)`.
- [x] All previously green tests in `internal/intelligence` continue to pass
  > Evidence: `go test ./internal/intelligence/... -count=1 -timeout 120s` — `ok  github.com/smackerel/smackerel/internal/intelligence    0.281s`. No new failures, no flakes.
- [x] `./smackerel.sh check` (Go vet + build) reports zero new findings against the changed file
  > Evidence: `go vet ./internal/intelligence/...` — exits 0 with no output. `go build ./internal/intelligence/...` — exits 0 with no output.
- [x] BUG-004-H1-1 (topic movement subquery failure logged): code path patched, log line shape verified by inspection
  > Evidence: `synthesis.go` line 158-160 — `if err != nil { slog.Warn("weekly synthesis topic movement query failed", "error", err) } else { defer topicRows.Close() ... }`. Same shape for open loops at line 193-195.
- [x] BUG-004-H1-2 (capture pattern subquery failure logged): code path patched, log line shape verified by inspection
  > Evidence: `synthesis.go` line 245-247 (day-of-week) and line 272-274 (hour-of-day) — both use `if err != nil { slog.Warn(...) } else { ... }` with stable section keys.
- [x] BUG-004-H1-3 (inner synthesis failure observable to operators): code path patched, log line shape verified by inspection
  > Evidence: `synthesis.go` line 152-156 — `if err != nil { slog.Warn("weekly synthesis inner RunSynthesis failed", "error", err) } else { ws.Insights = insights }`. Same shape for `Resurface` at line 215-219.
- [x] No functional regression: word-cap, quiet-week, and assemble-text behavior unchanged
  > Evidence: `TestAssembleWeeklySynthesisText_FullWeek`, `TestAssembleWeeklySynthesisText_QuietWeek`, `TestAssembleWeeklySynthesisText_WordCountCap` all PASS in the post-patch run.

---

## Spec Linkage
- Parent spec: [specs/004-phase3-intelligence/spec.md](../../spec.md) — R-302 Weekly Synthesis Digest
- Parent design: [specs/004-phase3-intelligence/design.md](../../design.md) — Synthesis pipeline §15
- Triggered by: stochastic-quality-sweep round 2 of 20 (parent invocation), trigger=harden, mapped child mode=harden-to-doc
