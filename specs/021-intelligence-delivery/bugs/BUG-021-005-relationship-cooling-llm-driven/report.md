# Report: BUG-021-005 — LLM-driven relationship-cooling judgment

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Supersedes:** BUG-021-004

## Summary

The relationship-cooling producer decided "is this relationship cooling?" with
hardcoded SQL magic numbers, and BUG-021-004 cemented them as Go constants + a
lock test. Per the product architecture (docs/smackerel.md §3.6 — domain
reasoning is LLM-driven), this change moves the judgment to the LLM via the new
`relationship_cooling_evaluate` scenario, following the established
`annotation_classify` precedent. The Go core now only retrieves candidate
signals; the LLM judges each per situation. The only remaining numbers are
operational SST bounds (throughput cap, confidence floor, dedup window).

## Root Cause

`ProduceRelationshipCoolingAlerts` ran an inline query whose `HAVING` clause
encoded the cooling judgment as constants (`> 30` days, `>= 4` interactions in a
fixed 90-180 day window, `LIMIT 10`). BUG-021-004 extracted these into Go
constants + `TestRelationshipCoolingHeuristic_MatchesDocumentedContract`, which
cemented the magic numbers — the opposite of the documented architecture.

## Fix

New `relationship_cooling_evaluate` scenario + `BridgeCoolingEvaluator`; the
producer retrieves candidate signals and lets the LLM judge each; operational
bounds moved to fail-loud SST; BUG-021-004's constants + builder + lock test
removed.

## Test Evidence

### Evaluator + helpers (LLM-driven core, scripted bridge — no live LLM)

```
$ go test -v -count=1 -run 'Cooling' ./internal/intelligence/
=== RUN   TestBridgeCoolingEvaluator_ParsesCoolingDecision
--- PASS: TestBridgeCoolingEvaluator_ParsesCoolingDecision (0.00s)
=== RUN   TestBridgeCoolingEvaluator_NotCooling
--- PASS: TestBridgeCoolingEvaluator_NotCooling (0.00s)
=== RUN   TestBridgeCoolingEvaluator_ErrorPaths
--- PASS: TestBridgeCoolingEvaluator_ErrorPaths (0.00s)
=== RUN   TestBridgeCoolingEvaluator_NilReceiver
--- PASS: TestBridgeCoolingEvaluator_NilReceiver (0.00s)
=== RUN   TestCoolingTypicalGapDays
--- PASS: TestCoolingTypicalGapDays (0.00s)
=== RUN   TestCoolingShouldSurface
--- PASS: TestCoolingShouldSurface (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/intelligence    0.026s
```

The evaluator test asserts the envelope routes to `relationship_cooling_evaluate`,
forwards the candidate signals, and does NOT leak the internal PersonID into the
LLM prompt.

### SST loader (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'RelationshipCooling' ./internal/config/
=== RUN   TestLoadRelationshipCoolingConfig_Populates
--- PASS: TestLoadRelationshipCoolingConfig_Populates (0.00s)
=== RUN   TestLoadRelationshipCoolingConfig_FailLoudOnMissing
--- PASS: TestLoadRelationshipCoolingConfig_FailLoudOnMissing (0.00s)
=== RUN   TestLoadRelationshipCoolingConfig_RejectsOutOfRange
--- PASS: TestLoadRelationshipCoolingConfig_RejectsOutOfRange (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.016s
```

### Scenario loads cleanly + registry stable

```
$ go test -count=1 ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.204s
ok      github.com/smackerel/smackerel/internal/agent   0.131s
```

`TestSpec061_LivePromptContractsLoadCleanly` (0 rejected, >= 8 registered) stays
green with the new scenario; the `noop_relationship_cooling` tool registers via
the intelligence package `init()`.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/intelligence/ ./internal/config/ ./cmd/core/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/main.go                              |   5 +
 config/smackerel.yaml                         |  16 +++
 internal/intelligence/alert_producers.go      | 157 +++++++++++------------
 internal/intelligence/alert_producers_test.go |  47 --------   <- lock test removed
 internal/intelligence/engine.go               |  15 +++
 scripts/commands/config.sh                    |   9 ++
 ?? cmd/core/wiring_cooling.go
 ?? config/prompt_contracts/relationship-cooling-evaluate-v1.yaml
 ?? internal/config/relationship_cooling.go
 ?? internal/config/relationship_cooling_test.go
 ?? internal/intelligence/cooling.go
 ?? internal/intelligence/cooling_test.go
```

No hardcoded "cooling" threshold remains in Go (the constants + builder + lock
test are gone). No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'INTELLIGENCE_RELATIONSHIP_COOLING' config/generated/dev.env
config/generated/dev.env:174:INTELLIGENCE_RELATIONSHIP_COOLING_MAX_CANDIDATES=25
config/generated/dev.env:175:INTELLIGENCE_RELATIONSHIP_COOLING_CONFIDENCE_FLOOR=0.7
config/generated/dev.env:176:INTELLIGENCE_RELATIONSHIP_COOLING_DEDUP_WINDOW_DAYS=30
$ go test -count=1 ./internal/intelligence/ ./internal/config/ ./internal/scheduler/ ./cmd/core/
ok      github.com/smackerel/smackerel/internal/intelligence    0.068s
ok      github.com/smackerel/smackerel/internal/config  32.651s
ok      github.com/smackerel/smackerel/internal/scheduler       5.044s
ok      github.com/smackerel/smackerel/cmd/core 1.130s
```

The SST pipeline resolves the new operational keys end-to-end, and every
affected package is green.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ git status --short config/generated/
# (gitignored — generated env not committed)
$ grep -rc 'coolingMinPriorInteractions\|relationshipCoolingAlertQuery' internal/intelligence/
internal/intelligence/alert_producers.go:0
```

The diff is confined to the cooling producer, the new LLM scenario/evaluator/
config, and the wiring. No migration, no `.github/bubbles` framework files. The
BUG-021-004 magic-number constants and lock test are fully removed.

## Completion Statement

The "is this relationship cooling?" decision is now LLM-driven via the
`relationship_cooling_evaluate` scenario; the Go core only retrieves candidate
signals and applies an operational confidence-floor gate. No hardcoded cooling
threshold remains, and BUG-021-004's constants + lock test are removed. The
evaluator, helpers, and SST loader are unit-tested; the scenario loads cleanly;
the SST pipeline resolves the operational bounds; build/vet/tests across the
affected packages are green. Scope 1 DoD is complete (11/11). BUG-021-005 is
Done and supersedes BUG-021-004.
