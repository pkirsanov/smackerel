# Report: BUG-021-007 — LLM-driven resurfacing worthiness for the dormancy strategy

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-006

## Summary

The digest resurfacing pipeline decided "is this dormant artifact worth
resurfacing?" with a hardcoded dormancy + relevance window
(`INTERVAL '30 days'`, `relevance_score > 0.3`). Per docs/smackerel.md §3.6
(domain reasoning is LLM-driven) and the owner directive (no const limits), this
moves the worthiness judgment to the LLM via the new `resurface_evaluate`
scenario, reusing the BUG-021-006 alert-timing pattern. The Go core now only
retrieves dormant candidates within an operational dormancy-retrieval floor; the
LLM judges each per situation. Serendipity (random rediscovery) is unchanged.

## Root Cause

`Engine.Resurface` Strategy 1 used the hardcoded window as both the candidate
filter and the worthiness decision: the SQL `WHERE last_accessed < NOW() -
INTERVAL '30 days' AND relevance_score > 0.3` fully decided which dormant
artifacts were surfaced.

## Fix

New `resurface_evaluate` scenario + `BridgeResurfaceEvaluator`; Strategy 1
retrieves dormant candidates via `gatherResurfaceCandidates` within the
operational `min_dormancy_days` floor and lets the LLM judge each; operational
bounds moved to fail-loud SST.

## Test Evidence

### Evaluator + helper (LLM-driven core, scripted bridge — no live LLM)

```
$ go test -v -count=1 -run 'Resurface' ./internal/intelligence/
--- PASS: TestBridgeResurfaceEvaluator_ParsesDecision (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_NotWorth (0.00s)
=== RUN   TestBridgeResurfaceEvaluator_ErrorPaths/nil_result
=== RUN   TestBridgeResurfaceEvaluator_ErrorPaths/non_ok_outcome
=== RUN   TestBridgeResurfaceEvaluator_ErrorPaths/empty_final
=== RUN   TestBridgeResurfaceEvaluator_ErrorPaths/bad_json
--- PASS: TestBridgeResurfaceEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeResurfaceEvaluator_NilReceiverAndRunner (0.00s)
=== RUN   TestResurfaceShouldSurface/worth_above_floor
=== RUN   TestResurfaceShouldSurface/worth_at_floor
=== RUN   TestResurfaceShouldSurface/worth_below_floor
=== RUN   TestResurfaceShouldSurface/not_worth_high_conf
--- PASS: TestResurfaceShouldSurface (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.021s
```

The evaluator test asserts the envelope routes to `resurface_evaluate`,
forwards the public signals (`title`, `access_count`, …), and does NOT leak the
internal ArtifactID into the LLM prompt.

### SST loader (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'Resurface' ./internal/config/
--- PASS: TestLoadResurfaceConfig_Populates (0.00s)
=== RUN   TestLoadResurfaceConfig_FailLoudOnMissing/INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS
=== RUN   TestLoadResurfaceConfig_FailLoudOnMissing/INTELLIGENCE_RESURFACE_MAX_CANDIDATES
=== RUN   TestLoadResurfaceConfig_FailLoudOnMissing/INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR
--- PASS: TestLoadResurfaceConfig_FailLoudOnMissing (0.00s)
=== RUN   TestLoadResurfaceConfig_RejectsOutOfRange/dormancy_zero
=== RUN   TestLoadResurfaceConfig_RejectsOutOfRange/dormancy_negative
=== RUN   TestLoadResurfaceConfig_RejectsOutOfRange/max_candidates_zero
=== RUN   TestLoadResurfaceConfig_RejectsOutOfRange/confidence_above_one
=== RUN   TestLoadResurfaceConfig_RejectsOutOfRange/confidence_negative
--- PASS: TestLoadResurfaceConfig_RejectsOutOfRange (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.036s
```

### Scenario loads cleanly + registry grows

```
$ go run ./cmd/scenario-lint config/prompt_contracts
scenarios registered: 12, rejected: 2
$ go test -count=1 ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.327s
ok      github.com/smackerel/smackerel/internal/agent   0.127s
```

The 2 rejects are pre-existing env-placeholder scenarios (retrieval-qa,
recipe-search), unaffected by this change; `resurface_evaluate` registers (count
rose 11 → 12 after this scenario). `noop_resurface_evaluate` registers via the
intelligence package `init()`.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/intelligence/ ./internal/config/ ./cmd/core/
# VET=0
$ git status --short (modified + new)
 M cmd/core/main.go
 M cmd/core/wiring_cooling.go
 M config/smackerel.yaml
 M internal/intelligence/engine.go
 M internal/intelligence/resurface.go
 M scripts/commands/config.sh
?? config/prompt_contracts/resurface-evaluate-v1.yaml
?? internal/config/resurface.go
?? internal/config/resurface_test.go
?? internal/intelligence/resurface_eval.go
?? internal/intelligence/resurface_eval_test.go
```

No hardcoded dormancy worthiness window remains in Strategy 1 (`INTERVAL '30
days'` + `relevance_score > 0.3` gone). No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml
$ grep -n 'INTELLIGENCE_RESURFACE' config/generated/dev.env
config/generated/dev.env:180:INTELLIGENCE_RESURFACE_MIN_DORMANCY_DAYS=7
config/generated/dev.env:181:INTELLIGENCE_RESURFACE_MAX_CANDIDATES=25
config/generated/dev.env:182:INTELLIGENCE_RESURFACE_CONFIDENCE_FLOOR=0.7
$ go test -count=1 ./internal/intelligence/ ./internal/config/ ./internal/scheduler/ ./cmd/core/ ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/internal/intelligence    0.072s
ok      github.com/smackerel/smackerel/internal/config  31.628s
ok      github.com/smackerel/smackerel/internal/scheduler       5.075s
ok      github.com/smackerel/smackerel/cmd/core 1.345s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.327s
ok      github.com/smackerel/smackerel/internal/agent   0.127s
```

The SST pipeline resolves the new operational keys end-to-end, and every
affected package returns `ok`. The existing resurface tests (serendipity,
`MarkResurfaced`, struct fields) continue to return `ok` alongside the new
dormancy path.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ grep -cE "INTERVAL '30 days'|relevance_score > 0.3" internal/intelligence/resurface.go
1
$ grep -n "INTERVAL '30 days'" internal/intelligence/resurface.go
305:            AND created_at > NOW() - INTERVAL '30 days'
```

The single remaining `INTERVAL '30 days'` (line 305) is the serendipity
calendar-context query for upcoming events — operational retrieval, not the
dormancy worthiness threshold, and intentionally left. The `relevance_score >
0.3` dormancy threshold is fully removed. The diff is confined to the dormancy
strategy, the new LLM scenario/evaluator/config, and the wiring. No migration,
no `.github/bubbles` framework files.

## Completion Statement

The dormancy strategy now decides "worth resurfacing?" via the
`resurface_evaluate` LLM scenario; the Go core only retrieves dormant candidates
within an operational dormancy-retrieval floor and applies a confidence-floor
gate. No hardcoded dormancy worthiness window remains. Serendipity is unchanged.
The evaluator, helper, and SST loader are unit-tested; the scenario loads
cleanly; the SST pipeline resolves the operational bounds; build, vet, and the
affected packages return `ok`. Scope 1 DoD is complete (10/10). BUG-021-007 is
Done and continues BUG-021-006.
