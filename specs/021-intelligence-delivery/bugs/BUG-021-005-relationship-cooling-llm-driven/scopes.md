# Scopes: BUG-021-005

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Supersedes `BUG-021-004` (removes its constants + lock test).

## Scope 1 — LLM-driven relationship-cooling judgment

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] New scenario `config/prompt_contracts/relationship-cooling-evaluate-v1.yaml` (`relationship_cooling_evaluate`): per-situation cooling judgment with input/output schema + a no-op tool; loads cleanly (scenario-lint not rejected)
      → Evidence: report.md `## Test Evidence` (scenario-lint registered, 0 new rejects; agent loader test green)
- [x] `internal/intelligence/cooling.go`: `CoolingEvaluator` + `BridgeCoolingEvaluator` (mockable runner) routing to the scenario via the agent bridge; `noop_relationship_cooling` registered via `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] Producer reworked to retrieve candidate SIGNALS (no hardcoded cooling threshold) and let the LLM judge each; gates only on the operational confidence floor; skips when the evaluator is nil (no magic-number fallback)
      → Evidence: report.md `### Code Diff Evidence` (alert_producers.go diff)
- [x] `BUG-021-004` constants + `relationshipCoolingAlertQuery()` builder + `TestRelationshipCoolingHeuristic_MatchesDocumentedContract` lock test REMOVED
      → Evidence: report.md `### Code Diff Evidence` (alert_producers_test.go -47 lines; no `coolingMinPriorInteractions` remains)
- [x] Evaluator unit-tested with a scripted bridge runner: parses decision, routes to `relationship_cooling_evaluate`, forwards signals, does NOT leak PersonID, errors on every failure path
      → Evidence: report.md `## Test Evidence` (TestBridgeCoolingEvaluator_* PASS)
- [x] Operational bounds as fail-loud SST: `intelligence.relationship_cooling.{max_candidates,confidence_floor,dedup_window_days}` in smackerel.yaml + config.sh + `internal/config/relationship_cooling.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `cmd/core/wiring_cooling.go` builds the evaluator from the bridge + SST and calls `SetCoolingConfig`; nil bridge ⇒ cooling disabled
      → Evidence: report.md `### Code Diff Evidence` (wiring_cooling.go)
- [x] `go build ./...`, `go vet`, full intelligence + config + scheduler + cmd/core + scenario-lint + agent packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-COOLING-LLM-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage — the evaluator + helper tests persist the LLM-driven decision contract and the no-leak/error invariants; they fail if the path regresses
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression — the full intelligence + config packages run green with the rework (and the removed lock test no longer cements magic numbers)
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-COOLING-LLM-01 | TestBridgeCoolingEvaluator_ParsesCoolingDecision (+ NotCooling, ErrorPaths, NilReceiver) | internal/intelligence/cooling_test.go | unit (mock bridge) | SCN-021-COOLING-LLM-01 |
| T-021-COOLING-LLM-02 | TestCoolingTypicalGapDays, TestCoolingShouldSurface | internal/intelligence/cooling_test.go | unit (pure helpers) | SCN-021-COOLING-LLM-02 |
| T-021-COOLING-LLM-03 | TestLoadRelationshipCoolingConfig_* (populate, fail-loud, range) | internal/config/relationship_cooling_test.go | unit (SST) | SCN-021-COOLING-LLM-03 |

### Non-Goals

- Live-LLM behavioral validation (live-stack tier).
- Other digest/recommendation hardcoded business thresholds (separate follow-up).
