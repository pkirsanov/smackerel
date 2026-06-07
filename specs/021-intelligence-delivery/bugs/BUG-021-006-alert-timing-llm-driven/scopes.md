# Scopes: BUG-021-006

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Continues BUG-021-005 (same directive, same pattern).

## Scope 1 — LLM-driven alert timing for bill/trip/return producers

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] New scenario `config/prompt_contracts/alert-timing-evaluate-v1.yaml` (`alert_timing_evaluate`): per-situation, per-kind alert-timing judgment with input/output schema + a no-op tool; loads cleanly
      → Evidence: report.md `## Test Evidence` (scenario-lint registered 11; agent loader test green)
- [x] `internal/intelligence/alert_timing.go`: `AlertTimingEvaluator` + `BridgeAlertTimingEvaluator` (mockable) routing to the scenario; shared `evaluateAndCreateTimedAlert` helper; `noop_alert_timing` registered via `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] All three producers (bill, trip-prep, return-window) reworked to retrieve candidates within the operational `lookahead_days` horizon and let the LLM judge each; gate only on the operational confidence floor; skip when the evaluator is nil (no window fallback)
      → Evidence: report.md `### Code Diff Evidence` (alert_producers.go diff; no `> 3` / `INTERVAL '5 days'` remains)
- [x] Evaluator unit-tested with a scripted bridge runner: parses decision, routes to `alert_timing_evaluate`, forwards public signals, does NOT leak ArtifactID/AlertType/Priority, errors on every failure path
      → Evidence: report.md `## Test Evidence` (TestBridgeAlertTimingEvaluator_* PASS)
- [x] Operational bounds as fail-loud SST: `intelligence.alert_timing.{lookahead_days,max_candidates,confidence_floor}` in smackerel.yaml + config.sh + `internal/config/alert_timing.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `wireAlertTimingEvaluator` builds the evaluator from the bridge + SST and calls `SetAlertTimingConfig`; nil bridge ⇒ producers disabled
      → Evidence: report.md `### Code Diff Evidence` (wiring_cooling.go + main.go)
- [x] `go build ./...`, `go vet`, full intelligence + config + scheduler + cmd/core + scenario-lint packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-TIMING-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage — the evaluator + helper + SST tests persist the LLM-driven decision contract and the no-leak/error invariants; they fail if the path regresses
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression — the full intelligence + config packages run green with the rework
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-TIMING-01 | TestBridgeAlertTimingEvaluator_ParsesDecision (+ ErrorPaths, NilReceiverAndRunner) | internal/intelligence/alert_timing_test.go | unit (mock bridge) | SCN-021-TIMING-01 |
| T-021-TIMING-02 | TestAlertTimingShouldSurface | internal/intelligence/alert_timing_test.go | unit (pure helper) | SCN-021-TIMING-02 |
| T-021-TIMING-03 | TestLoadAlertTimingConfig_* (populate, fail-loud, range) | internal/config/alert_timing_test.go | unit (SST) | SCN-021-TIMING-03 |

### Non-Goals

- `commitment_overdue` / `meeting_brief` producers (event-driven).
- Live-LLM behavioral validation (live-stack tier).
