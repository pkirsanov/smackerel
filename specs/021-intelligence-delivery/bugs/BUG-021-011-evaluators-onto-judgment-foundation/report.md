# Report: BUG-021-011 — migrate the four prior evaluators onto agent.InvokeJudgment

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-010

## Summary

The four pre-existing LLM-judgment evaluators (cooling, alert timing, resurface,
expertise) each carried a private runner interface, a redundant "unavailable"
sentinel, and a hand-rolled copy of the marshal/invoke/validate/decode transport
that BUG-021-010 centralized into `agent.InvokeJudgment[T]`. This refactor
retrofits all four onto the foundation, leaving one judgment transport and zero
duplication. Behaviour is preserved — every existing evaluator unit test passes.

## Root Cause

The foundation post-dated the four evaluators, so each still owned its own copy
of the transport.

## Fix

Each `BridgeXEvaluator` method body becomes a single `agent.InvokeJudgment[T]`
call; the private runner interfaces and the per-evaluator sentinels are removed;
the unused `fmt` import is dropped from each file.

## Test Evidence

### All four migrated evaluators (existing tests, unchanged in intent)

```
$ go test -v -count=1 -run 'Cooling|AlertTiming|Resurface|Expertise' ./internal/intelligence/
--- PASS: TestBridgeAlertTimingEvaluator_ParsesDecision (0.00s)
--- PASS: TestBridgeAlertTimingEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeAlertTimingEvaluator_NilReceiverAndRunner (0.00s)
--- PASS: TestBridgeCoolingEvaluator_ParsesCoolingDecision (0.00s)
--- PASS: TestBridgeCoolingEvaluator_NotCooling (0.00s)
--- PASS: TestBridgeCoolingEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeCoolingEvaluator_NilReceiver (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_ParsesBatch (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_EmptyTopics (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_NilReceiverAndRunner (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_ParsesDecision (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_NotWorth (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_NilReceiverAndRunner (0.00s)
ok      github.com/smackerel/smackerel/internal/intelligence    0.048s
```

The nil-receiver/nil-runner tests now assert `agent.ErrJudgmentUnavailable`; the
parse / routing / signal-forwarding / non-leak / error-path tests are unchanged
and pass against the `InvokeJudgment`-backed bodies.

### The shared primitive the four now route through

```
$ go test -v -count=1 -run 'InvokeJudgment' ./internal/agent/
--- PASS: TestInvokeJudgment_ParsesRoutesAndForwardsSignals (0.00s)
--- PASS: TestInvokeJudgment_NilRunner (0.00s)
--- PASS: TestInvokeJudgment_ErrorPaths (0.00s)
ok      github.com/smackerel/smackerel/internal/agent   0.015s
```

## Code Diff Evidence

```
$ go build ./...
# BUILD=0  (unused fmt imports would fail here)
$ go vet ./internal/intelligence/ ./cmd/core/
# VET=0
$ git diff --stat (the 8 files)
 internal/intelligence/alert_timing.go        | 49 +++-----------------
 internal/intelligence/alert_timing_test.go   |  8 ++---
 internal/intelligence/cooling.go             | 50 ++++-----------------
 internal/intelligence/cooling_test.go        |  6 +--
 internal/intelligence/expertise_eval.go      | 47 ++++---------------
 internal/intelligence/expertise_eval_test.go |  8 ++---
 internal/intelligence/resurface_eval.go      | 49 +++-----------------
 internal/intelligence/resurface_eval_test.go |  8 ++---
```

Net ~135 fewer lines (the four ~30-line transport bodies + four interfaces +
four sentinels collapse to four one-line `InvokeJudgment` calls). No schema
migration.

### Validation Evidence

```
$ go test -count=1 ./internal/intelligence/ ./internal/agent/ ./internal/digest/ ./cmd/core/ ./cmd/scenario-lint/
ok      github.com/smackerel/smackerel/internal/intelligence    0.056s
ok      github.com/smackerel/smackerel/internal/agent   0.098s
ok      github.com/smackerel/smackerel/internal/digest  0.369s
ok      github.com/smackerel/smackerel/cmd/core 0.903s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.205s
```

The production wiring (`cmd/core`) and the scenario linter compile and pass
against the retyped `agent.JudgmentRunner` field — `*agent.Bridge` satisfies it.

### Audit Evidence

```
$ grep -rnE 'coolingBridgeRunner|alertTimingBridgeRunner|resurfaceBridgeRunner|expertiseBridgeRunner|ErrCoolingEvaluatorUnavailable|ErrAlertTimingEvaluatorUnavailable|ErrResurfaceEvaluatorUnavailable|ErrExpertiseEvaluatorUnavailable' internal/ cmd/ || echo "none — fully removed"
none — fully removed
$ git status --short | grep -E 'internal/db/migrations/' || echo "(empty — no migration)"
(empty — no migration)
$ go test -count=1 ./internal/intelligence/
ok      github.com/smackerel/smackerel/internal/intelligence    0.045s
```

The diff is confined to the four evaluator files and their four tests. No
scenario YAML, SST config, wiring, producer, or schema change — a pure transport
refactor. No `.github/bubbles` framework files.

## Completion Statement

All four pre-existing LLM-judgment evaluators now route their bridge transport
through the single `agent.InvokeJudgment[T]` primitive: the four private runner
interfaces, the four redundant sentinels, and the four hand-rolled
marshal/invoke/validate/decode bodies are gone (~135 fewer lines), replaced by
one `InvokeJudgment` call each. Behaviour is preserved — every existing evaluator
unit test (parse, routing, signal forwarding, internal-field non-leak,
nil-receiver/nil-runner now on `agent.ErrJudgmentUnavailable`, all error paths)
passes, and build/vet/tests across the affected packages are green. There are no
dangling references to the removed identifiers. The codebase now has exactly one
judgment transport. Scope 1 DoD is complete (10/10). BUG-021-011 is Done and
continues BUG-021-010.
