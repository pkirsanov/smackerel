# Report: BUG-004-H1 Silent query-error swallowing in WeeklySynthesis subqueries

**Status:** Fixed and Verified
**Origin:** Stochastic-quality-sweep round 2 of 20 (parent invocation), trigger=harden, mapped child mode=harden-to-doc, selection seed 20520512
**Execution model:** parent-expanded-child-mode (nested subagent runtime lacked `runSubagent`; phase owners executed directly)
**Date:** 2026-05-12

---

## 1. Harden Trigger Findings

A read-through of `internal/intelligence/synthesis.go` against the rest of the `intelligence` package surfaced one consistent quality regression: the `if err == nil { ... }` swallowing pattern was applied to every optional subquery in `GenerateWeeklySynthesis` and `detectCapturePatterns` — six call sites total — without an `else` log branch. Every other `Pool.Query` failure path in the same package logs via `slog.Warn`. The weekly synthesis path was the lone outlier, producing partial results with no operator-visible signal that subqueries failed.

Affected call sites (line numbers from pre-patch file):

| # | Function | Line | Subject |
|---|----------|------|---------|
| 1 | `GenerateWeeklySynthesis` | 152 | Inner `RunSynthesis` call |
| 2 | `GenerateWeeklySynthesis` | 170 | Topic movement query |
| 3 | `GenerateWeeklySynthesis` | 196 | Open loops query |
| 4 | `GenerateWeeklySynthesis` | 215 | Inner `Resurface` call |
| 5 | `detectCapturePatterns` | 252 | Day-of-week pattern query |
| 6 | `detectCapturePatterns` | 279 | Hour-of-day pattern query |

This is a hardening defect of the "operator visibility" class — no functional misbehavior, but a class of silent failure that a sweep is supposed to catch.

## 2. Fix Applied

Replaced every `if err == nil { ... }` block with the canonical observable shape:

```go
rows, err := e.Pool.Query(ctx, `...`)
if err != nil {
    slog.Warn("weekly synthesis topic movement query failed", "error", err)
} else {
    defer rows.Close()
    // ... existing iteration ...
}
```

The same shape applies to the two helper-call sites:

```go
insights, err := e.RunSynthesis(ctx)
if err != nil {
    slog.Warn("weekly synthesis inner RunSynthesis failed", "error", err)
} else {
    ws.Insights = insights
}
```

Stable section keys:
- `weekly synthesis inner RunSynthesis failed`
- `weekly synthesis topic movement query failed`
- `weekly synthesis open loops query failed`
- `weekly synthesis inner Resurface failed`
- `capture pattern day-of-week query failed`
- `capture pattern hour-of-day query failed`

The function's external contract is unchanged: callers still receive a `*WeeklySynthesis` value with whatever sections survived, and the upfront nil-pool guard still returns the documented error.

## 3. Test Evidence

### 3.1 Pre-patch baseline (green)

```
$ go build ./internal/intelligence/...
(exit 0)
$ go test ./internal/intelligence/... -run 'TestRunSynthesis|TestSynthesisInsight|TestAssembleWeeklySynthesisText|TestAssembleBriefText|TestCapturePattern' -count=1 -timeout 60s
ok  github.com/smackerel/smackerel/internal/intelligence    0.051s
```

### 3.2 Post-patch verification (green)

```
$ grep -nc "slog.Warn" internal/intelligence/synthesis.go
12
$ grep -n "if err == nil" internal/intelligence/synthesis.go
(no remaining swallowing patterns)
$ go vet ./internal/intelligence/...
(exit 0)
$ go build ./internal/intelligence/...
(exit 0)
```

12 `slog.Warn` sites = 6 pre-existing iteration/scan warnings + 6 new query/helper warnings.

### 3.3 Targeted suite (green)

```
$ go test ./internal/intelligence/... -count=1 -timeout 180s -run 'TestSynthesisFile_NoSilentQueryErrorSwallowing|TestGenerateWeeklySynthesis_NilPool|TestDetectCapturePatterns_NilPool|TestAssembleWeeklySynthesisText|TestRunSynthesis|TestSynthesisInsight|TestSynthesisConfidence|TestWeeklySynthesis|TestTopicMovement' -v
...
--- PASS: TestSynthesisFile_NoSilentQueryErrorSwallowing (0.00s)
--- PASS: TestGenerateWeeklySynthesis_NilPool (0.00s)
--- PASS: TestDetectCapturePatterns_NilPool (0.00s)
--- PASS: TestAssembleWeeklySynthesisText_FullWeek (0.00s)
--- PASS: TestAssembleWeeklySynthesisText_QuietWeek (0.00s)
--- PASS: TestAssembleWeeklySynthesisText_WordCountCap (0.00s)
--- PASS: TestRunSynthesis_EmptyPool (0.00s)
--- PASS: TestRunSynthesis_CancelledContext (0.00s)
... (all suite tests PASS)
PASS
ok  github.com/smackerel/smackerel/internal/intelligence    0.073s
```

### 3.4 Adversarial validation (proves test is not tautological)

Reintroduced the bug at line 152 by reverting the inner `RunSynthesis` site to `if err == nil { ws.Insights = insights }`:

```
$ go test ./internal/intelligence/... -count=1 -run 'TestSynthesisFile_NoSilentQueryErrorSwallowing' -v
=== RUN   TestSynthesisFile_NoSilentQueryErrorSwallowing
    synthesis_test.go:304: BUG-004-H1 regression: synthesis.go reintroduced silent query-error swallowing at line(s) 152 — replace `if err == nil { ... }` with `if err != nil { slog.Warn(...) } else { ... }` so subquery failures are observable
--- FAIL: TestSynthesisFile_NoSilentQueryErrorSwallowing (0.00s)
FAIL  github.com/smackerel/smackerel/internal/intelligence    0.021s
```

Then restored the fix:

```
$ go test ./internal/intelligence/... -count=1 -timeout 240s
ok  github.com/smackerel/smackerel/internal/intelligence    0.076s
$ go vet ./internal/intelligence/...
(exit 0)
$ go build ./...
(exit 0)
```

The test caught the regression at the exact line that was reverted with the exact remediation hint. This is the adversarial proof required by the repo's bug-fix policy.

## 4. Files Changed

| File | Change |
|------|--------|
| `internal/intelligence/synthesis.go` | Replaced 6 `if err == nil { ... }` blocks with explicit `if err != nil { slog.Warn(...) } else { ... }` shapes. Each warn line uses a stable section key for grep-friendliness. |
| `internal/intelligence/synthesis_test.go` | Added `TestSynthesisFile_NoSilentQueryErrorSwallowing` structural regression test that reads `synthesis.go` and fails if the swallowing pattern returns. Imports `os` and `regexp`. |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/spec.md` | New |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/design.md` | New |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/scopes.md` | New |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/state.json` | New |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/report.md` | New (this file) |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/uservalidation.md` | New |
| `specs/004-phase3-intelligence/bugs/BUG-004-H1-silent-query-error-swallowing/scenario-manifest.json` | New |

No source files outside `internal/intelligence/synthesis.go` and `internal/intelligence/synthesis_test.go` were modified. No certification fields on the parent spec 004 were touched — this is a self-contained hardening bug under the parent.

## 5. Closure

- **Status:** Fixed and Verified
- **Verdict:** Closed
- **Parent ledger update:** none required; parent spec 004 remains `done`. The hardening fix lives in the BUG-004-H1 packet.
- **Stochastic sweep continuation:** Round 2 of 20 returns `completed_owned` with one finding fully remediated.
