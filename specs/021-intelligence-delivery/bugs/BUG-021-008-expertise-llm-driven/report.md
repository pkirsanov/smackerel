# Report: BUG-021-008 — LLM-driven expertise tier & growth classification

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-007

## Summary

The expertise map decided "how expert is the user, and is the topic growing?"
with a hardcoded weighted score (`computeDepthScore`) and numeric boundaries
(`assignTier`, `computeTrajectory`). Per docs/smackerel.md §3.6 (domain
reasoning is LLM-driven) and the owner directive (no const limits), this moves
the tier + growth judgment to the LLM via the new `expertise_classify` scenario,
reusing the BUG-021-007 pattern in a BATCH variant (one call classifies all
topics). The Go core now only gathers per-topic signals within an operational
topic cap.

## Root Cause

`GenerateExpertiseMap` computed `depthScore = computeDepthScore(te)` (a fixed
weighted sum), then `assignTier(...)` (numeric tier boundaries) and
`computeTrajectory(...)` (fixed velocity cutoffs). Lock tests cemented those
magic numbers.

## Fix

New `expertise_classify` batched scenario + `BridgeExpertiseEvaluator`;
`GenerateExpertiseMap` gathers signals (capped by operational `max_topics`) and
makes one LLM call, mapping classifications back by `ref`; the magic-number
functions and the `DepthScore` field are removed; operational bounds moved to
fail-loud SST.

## Test Evidence

### Evaluator (LLM-driven batch core, scripted bridge — no live LLM)

```
$ go test -v -count=1 -run 'Expertise' ./internal/intelligence/
--- PASS: TestBridgeExpertiseEvaluator_ParsesBatch (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_EmptyTopics (0.00s)
=== RUN   TestBridgeExpertiseEvaluator_ErrorPaths/nil_result
=== RUN   TestBridgeExpertiseEvaluator_ErrorPaths/non_ok_outcome
=== RUN   TestBridgeExpertiseEvaluator_ErrorPaths/empty_final
=== RUN   TestBridgeExpertiseEvaluator_ErrorPaths/bad_json
--- PASS: TestBridgeExpertiseEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeExpertiseEvaluator_NilReceiverAndRunner (0.00s)
--- PASS: TestExpertiseTier_Constants (0.00s)
--- PASS: TestExpertiseMap_Struct (0.00s)
--- PASS: TestGenerateExpertiseMap_NilPool (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.034s
```

`ParsesBatch` asserts the envelope routes to `expertise_classify`, carries
`data_days` + the public per-topic signals (incl. `ref`), and does NOT leak the
internal TopicID; classifications correlate back by `ref`. `EmptyTopics` proves
the runner is not invoked for empty input.

### SST loader (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'Expertise' ./internal/config/
--- PASS: TestLoadExpertiseConfig_Populates (0.00s)
=== RUN   TestLoadExpertiseConfig_FailLoudOnMissing/INTELLIGENCE_EXPERTISE_MAX_TOPICS
=== RUN   TestLoadExpertiseConfig_FailLoudOnMissing/INTELLIGENCE_EXPERTISE_MATURITY_DAYS
=== RUN   TestLoadExpertiseConfig_FailLoudOnMissing/INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS
=== RUN   TestLoadExpertiseConfig_FailLoudOnMissing/INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES
=== RUN   TestLoadExpertiseConfig_FailLoudOnMissing/INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT
--- PASS: TestLoadExpertiseConfig_FailLoudOnMissing (0.00s)
=== RUN   TestLoadExpertiseConfig_RejectsOutOfRange/max_topics_zero
=== RUN   TestLoadExpertiseConfig_RejectsOutOfRange/maturity_zero
=== RUN   TestLoadExpertiseConfig_RejectsOutOfRange/blind_spot_limit_negative
--- PASS: TestLoadExpertiseConfig_RejectsOutOfRange (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.024s
```

### Scenario loads cleanly + registry grows

```
$ go run ./cmd/scenario-lint config/prompt_contracts
scenarios registered: 13, rejected: 2
$ go test -count=1 ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.214s
ok      github.com/smackerel/smackerel/internal/agent   0.081s
```

The 2 rejects are pre-existing env-placeholder scenarios (retrieval-qa,
recipe-search), unaffected by this change; `expertise_classify` registers (count
rose 12 → 13 after this scenario). `noop_expertise_classify` registers via the
intelligence package `init()`.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/intelligence/ ./internal/config/ ./cmd/core/ ./internal/api/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/main.go                        |   6 +
 cmd/core/wiring_cooling.go              |  38 +++++++
 config/smackerel.yaml                   |  16 +++
 internal/intelligence/engine.go         |  15 +++
 internal/intelligence/expertise.go      | 119 ++++++++++----------
 internal/intelligence/expertise_test.go | 191 ++------------------------------
 scripts/commands/config.sh              |  10 ++
?? config/prompt_contracts/expertise-classify-v1.yaml
?? internal/config/expertise.go
?? internal/config/expertise_test.go
?? internal/intelligence/expertise_eval.go
?? internal/intelligence/expertise_eval_test.go
```

The 246 deletions reflect the removed magic-number functions and their lock
tests. No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'INTELLIGENCE_EXPERTISE' config/generated/dev.env
config/generated/dev.env:183:INTELLIGENCE_EXPERTISE_MAX_TOPICS=100
config/generated/dev.env:184:INTELLIGENCE_EXPERTISE_MATURITY_DAYS=90
config/generated/dev.env:185:INTELLIGENCE_EXPERTISE_BLIND_SPOT_MIN_MENTIONS=5
config/generated/dev.env:186:INTELLIGENCE_EXPERTISE_BLIND_SPOT_MAX_CAPTURES=5
config/generated/dev.env:187:INTELLIGENCE_EXPERTISE_BLIND_SPOT_LIMIT=10
$ go test -count=1 ./internal/intelligence/ ./internal/config/ ./cmd/core/ ./cmd/scenario-lint/ ./internal/api/ ./internal/agent/
ok      github.com/smackerel/smackerel/internal/intelligence    0.055s
ok      github.com/smackerel/smackerel/internal/config  29.753s
ok      github.com/smackerel/smackerel/cmd/core 1.108s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.214s
ok      github.com/smackerel/smackerel/internal/api     9.691s
ok      github.com/smackerel/smackerel/internal/agent   0.081s
```

The SST pipeline resolves the new operational keys end-to-end, and every
affected package returns `ok` (including `internal/api`, which exercises the
expertise HTTP handler).

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ grep -cE 'computeDepthScore|assignTier|computeTrajectory|LIMIT 100|>= 90' internal/intelligence/expertise.go
0
```

No hardcoded depth-score formula or tier/velocity threshold remains in
`expertise.go`; the topic LIMIT and maturity floor are SST-parameterized. The
diff is confined to the expertise map, the new LLM scenario/evaluator/config,
the wiring, and the removal of the superseded lock tests. No migration, no
`.github/bubbles` framework files. The spec-006 references to the removed lock
tests are recorded as superseded in `bug.md` (spec 006 is not edited).

## Completion Statement

`GenerateExpertiseMap` now decides tier + growth via the `expertise_classify`
LLM scenario in a single batched call; the Go core only gathers per-topic
signals within an operational topic cap and fails loud when the evaluator is not
wired. No hardcoded depth-score formula or tier/velocity threshold remains, and
the lock tests that cemented those magic numbers are removed. The evaluator and
SST loader are unit-tested; the scenario loads cleanly; the SST pipeline
resolves the operational bounds; build, vet, and the affected packages return
`ok`. Scope 1 DoD is complete (10/10). BUG-021-008 is Done and continues
BUG-021-007.
