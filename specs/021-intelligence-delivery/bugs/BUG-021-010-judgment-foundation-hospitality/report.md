# Report: BUG-021-010 — reusable judgment foundation + LLM-driven hospitality alerts

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-009

## Summary

The owner asked for the "best solution long term, but also short term value."
Long-term: `agent.InvokeJudgment[T]` captures, once, the
`marshal → invoke → validate → decode` contract that the four prior judgment
evaluators each re-implemented. Short-term: the GuestHost digest's guest/property
concern alerts — previously hardcoded SQL thresholds (`sentiment_score < 0.3`,
`avg_rating < 3.5`, `issue_count >= 5`, `total_stays > 1`) — are now LLM-judged
on that foundation, the first LLM judgment in the `digest` package.

## Root Cause

The concern decision lived in SQL `WHERE`/`CASE` thresholds, and each prior
evaluator carried its own copy of the bridge plumbing.

## Fix

New `agent.InvokeJudgment[T]` foundation; new `hospitality_concern_evaluate`
scenario + `BridgeHospitalityEvaluator` on it; the digest gathers candidate
signals within operational caps and the LLM judges which warrant a host alert;
operational caps moved to fail-loud SST.

## Test Evidence

### Reusable foundation (scripted runner — no live LLM)

```
$ go test -v -count=1 -run 'InvokeJudgment' ./internal/agent/
--- PASS: TestInvokeJudgment_ParsesRoutesAndForwardsSignals (0.00s)
--- PASS: TestInvokeJudgment_NilRunner (0.00s)
--- PASS: TestInvokeJudgment_ErrorPaths (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/agent   0.015s
```

`ParsesRoutesAndForwardsSignals` asserts the envelope routes to the named
scenario with the named source, forwards the public signals, and does NOT leak a
`json:"-"` field; `NilRunner` asserts the `ErrJudgmentUnavailable` sentinel.

### Hospitality evaluator (scripted bridge — no live LLM)

```
$ go test -v -count=1 -run 'Hospitality' ./internal/digest/
--- PASS: TestBridgeHospitalityEvaluator_ParsesBatch (0.00s)
--- PASS: TestBridgeHospitalityEvaluator_EmptyInput (0.00s)
--- PASS: TestBridgeHospitalityEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeHospitalityEvaluator_NilReceiverAndRunner (0.00s)
--- PASS: TestHospitalityDigestContext_IsEmpty_AllZero (0.00s)
--- PASS: TestHospitalityDigestContext_IsEmpty_WithGuestAlerts (0.00s)
--- PASS: TestFormatHospitalityFallback_GuestAndPropertyAlerts (0.00s)
--- PASS: TestFormatHospitalityFallback_Full (0.00s)
--- PASS: TestDigestContext_WithHospitality (0.00s)
ok      github.com/smackerel/smackerel/internal/digest  0.023s
```

`ParsesBatch` asserts routing to `hospitality_concern_evaluate`, that guest +
property signals (incl. `ref`) are forwarded, and that the internal guest Email
is NOT leaked; `EmptyInput` asserts the runner is not invoked for empty input.
The pre-existing 16 hospitality tests stay green.

### SST loader (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'Hospitality' ./internal/config/
--- PASS: TestLoadHospitalityConfig_Populates (0.00s)
--- PASS: TestLoadHospitalityConfig_FailLoudOnMissing (0.00s)
--- PASS: TestLoadHospitalityConfig_RejectsOutOfRange (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.037s
```

### Scenario loads cleanly + registry grows

```
$ go run ./cmd/scenario-lint config/prompt_contracts
scenarios registered: 14, rejected: 2
$ go test -count=1 ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.355s
ok      github.com/smackerel/smackerel/internal/agent   0.112s
```

The 2 rejects are pre-existing env-placeholder scenarios; `hospitality_concern_evaluate`
registers (count rose 13 → 14). `noop_hospitality_concern` is registered via the
digest package `init()`, and `cmd/scenario-lint` blank-imports `internal/digest`
so BS-010 validation finds it.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/digest/ ./internal/config/ ./internal/agent/ ./cmd/core/ ./cmd/scenario-lint/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/main.go               |   6 ++
 cmd/scenario-lint/main.go      |   5 ++
 config/smackerel.yaml          |  12 +++
 internal/digest/generator.go   |  18 ++-
 internal/digest/hospitality.go | 172 +++++++++++++++++++++--------------
 scripts/commands/config.sh     |   4 +
?? internal/agent/judgment.go
?? internal/agent/judgment_test.go
?? config/prompt_contracts/hospitality-concern-evaluate-v1.yaml
?? internal/config/hospitality.go
?? internal/config/hospitality_test.go
?? internal/digest/hospitality_eval.go
?? internal/digest/hospitality_eval_test.go
?? cmd/core/wiring_hospitality.go
```

No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'DIGEST_HOSPITALITY' config/generated/dev.env
config/generated/dev.env:192:DIGEST_HOSPITALITY_GUEST_CANDIDATE_LIMIT=50
config/generated/dev.env:193:DIGEST_HOSPITALITY_PROPERTY_CANDIDATE_LIMIT=50
$ go test -count=1 ./internal/agent/ ./internal/digest/ ./internal/config/ ./cmd/core/ ./cmd/scenario-lint/
ok      github.com/smackerel/smackerel/internal/agent   0.112s
ok      github.com/smackerel/smackerel/internal/digest  0.711s
ok      github.com/smackerel/smackerel/internal/config  29.145s
ok      github.com/smackerel/smackerel/cmd/core 1.142s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.355s
```

The SST pipeline resolves the new operational keys end-to-end, and every
affected package returns `ok` (including the existing hospitality unit tests).

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ grep -cE "sentiment_score < 0\.3|avg_rating < 3\.5|issue_count >= 5" internal/digest/hospitality.go
0
```

No hardcoded hospitality alert threshold remains in `hospitality.go`. The diff
is confined to the new foundation, the new scenario/evaluator/config, the digest
rework, and the wiring. No migration, no `.github/bubbles` framework files.

## Completion Statement

`agent.InvokeJudgment[T]` now provides the single reusable LLM-judgment primitive
(long-term foundation), and the GuestHost digest's guest/property concern alerts
are LLM-judged on it (short-term value) — the first LLM judgment in the `digest`
package. No hardcoded sentiment/rating/issue-count threshold remains; a nil
evaluator yields no concern alerts with no fallback, leaving the rest of the
digest unaffected. The foundation, evaluator, and SST loader are unit-tested; the
scenario loads cleanly (registry 14); the SST pipeline resolves the operational
caps; build, vet, and the affected packages (including the 16 pre-existing
hospitality tests) return `ok`. Scope 1 DoD is complete (10/10). BUG-021-010 is
Done and continues BUG-021-009.
