# Report: BUG-021-006 — LLM-driven alert timing for bill/trip/return producers

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Continues:** BUG-021-005

## Summary

The bill / trip-prep / return-window producers decided "alert now?" with
hardcoded N-day windows (`> 3`, `INTERVAL '5 days'`). Per docs/smackerel.md §3.6
(domain reasoning is LLM-driven) and the owner directive (no const limits), this
moves the timing judgment to the LLM via the new `alert_timing_evaluate`
scenario, reusing the BUG-021-005 cooling pattern. The Go core now only
retrieves candidates within an operational lookahead horizon; the LLM judges
each per situation and per kind.

## Root Cause

Each producer used a hardcoded window as both the candidate filter and the
alert decision. Bill dropped anything `> 3` days out; trip/return only fetched
events within `INTERVAL '5 days'`.

## Fix

New `alert_timing_evaluate` scenario + `BridgeAlertTimingEvaluator`; the three
producers retrieve candidates within the operational `lookahead_days` horizon
and let the LLM judge each; operational bounds moved to fail-loud SST.

## Test Evidence

### Evaluator + helper (LLM-driven core, scripted bridge — no live LLM)

```
$ go test -v -count=1 -run 'AlertTiming' ./internal/intelligence/
--- PASS: TestBridgeAlertTimingEvaluator_ParsesDecision (0.00s)
--- PASS: TestBridgeAlertTimingEvaluator_ErrorPaths (0.00s)
--- PASS: TestBridgeAlertTimingEvaluator_NilReceiverAndRunner (0.00s)
--- PASS: TestAlertTimingShouldSurface (0.00s)
ok      github.com/smackerel/smackerel/internal/intelligence    0.027s
```

The evaluator test asserts the envelope routes to `alert_timing_evaluate`,
forwards the public signals, and does NOT leak the internal ArtifactID /
AlertType / Priority into the LLM prompt.

### SST loader (fail-loud + populate + range)

```
$ go test -v -count=1 -run 'AlertTiming' ./internal/config/
--- PASS: TestLoadAlertTimingConfig_Populates (0.00s)
--- PASS: TestLoadAlertTimingConfig_FailLoudOnMissing (0.00s)
--- PASS: TestLoadAlertTimingConfig_RejectsOutOfRange (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.018s
```

### Scenario loads cleanly + registry stable

```
$ go run ./cmd/scenario-lint config/prompt_contracts
scenarios registered: 11, rejected: 2
$ go test -count=1 ./cmd/scenario-lint/ ./internal/agent/
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.254s
```

The 2 rejects are pre-existing env-placeholder scenarios (retrieval-qa,
recipe-search), unaffected by this change; `alert_timing_evaluate` registers.
`noop_alert_timing` registers via the intelligence package `init()`.

## Code Diff Evidence

```
$ go build ./...
# BUILD=0
$ go vet ./internal/intelligence/ ./internal/config/ ./cmd/core/
# VET=0
$ git diff --stat (modified) ; git status --short (new)
 cmd/core/main.go                         |   5 +
 cmd/core/wiring_cooling.go               |  34 +++++
 config/smackerel.yaml                    |  13 ++
 internal/intelligence/alert_producers.go | 239 ++++++++++++++-----------
 internal/intelligence/engine.go          |  14 ++
 scripts/commands/config.sh               |   6 +
 ?? config/prompt_contracts/alert-timing-evaluate-v1.yaml
 ?? internal/config/alert_timing.go
 ?? internal/config/alert_timing_test.go
 ?? internal/intelligence/alert_timing.go
 ?? internal/intelligence/alert_timing_test.go
```

No hardcoded alert-timing window remains in Go (`> 3`, `INTERVAL '5 days'`
gone). No schema migration.

### Validation Evidence

```
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp OK
Generated ~/smackerel/config/generated/dev.env
$ grep -n 'INTELLIGENCE_ALERT_TIMING' config/generated/dev.env
config/generated/dev.env:177:INTELLIGENCE_ALERT_TIMING_LOOKAHEAD_DAYS=30
config/generated/dev.env:178:INTELLIGENCE_ALERT_TIMING_MAX_CANDIDATES=25
config/generated/dev.env:179:INTELLIGENCE_ALERT_TIMING_CONFIDENCE_FLOOR=0.7
$ go test -count=1 ./internal/intelligence/ ./internal/config/ ./internal/scheduler/ ./cmd/core/ ./cmd/scenario-lint/
ok      github.com/smackerel/smackerel/internal/intelligence    0.112s
ok      github.com/smackerel/smackerel/internal/config  33.900s
ok      github.com/smackerel/smackerel/internal/scheduler       5.090s
ok      github.com/smackerel/smackerel/cmd/core 1.283s
ok      github.com/smackerel/smackerel/cmd/scenario-lint        0.254s
```

The SST pipeline resolves the new operational keys end-to-end, and every
affected package is green.

### Audit Evidence

```
$ git status --short | grep -E 'internal/db/migrations/'
# (empty — no migration)
$ grep -rcE "daysUntilBilling > 3|INTERVAL '5 days'" internal/intelligence/alert_producers.go
internal/intelligence/alert_producers.go:0
```

The diff is confined to the three producers, the new LLM scenario/evaluator/
config, and the wiring. No migration, no `.github/bubbles` framework files. The
hardcoded alert-timing windows are fully removed.

## Completion Statement

The bill / trip-prep / return-window producers now decide "alert now?" via the
`alert_timing_evaluate` LLM scenario; the Go core only retrieves candidates
within an operational lookahead horizon and applies a confidence-floor gate. No
hardcoded alert-timing window remains. The evaluator, helper, and SST loader are
unit-tested; the scenario loads cleanly; the SST pipeline resolves the
operational bounds; build/vet/tests across the affected packages are green.
Scope 1 DoD is complete (10/10). BUG-021-006 is Done and continues BUG-021-005.
