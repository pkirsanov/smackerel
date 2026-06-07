# Scopes: BUG-021-007

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Continues BUG-021-006 (same directive, same pattern).

## Scope 1 — LLM-driven resurfacing worthiness for the dormancy strategy

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] New scenario `config/prompt_contracts/resurface-evaluate-v1.yaml` (`resurface_evaluate`): per-situation resurfacing-worthiness judgment with input/output schema + a no-op tool; loads cleanly
      → Evidence: report.md `## Test Evidence` (scenario-lint registered 12; agent loader test green)
- [x] `internal/intelligence/resurface_eval.go`: `ResurfaceEvaluator` + `BridgeResurfaceEvaluator` (mockable) routing to the scenario; `ResurfaceConfig` bundle; `noop_resurface_evaluate` registered via `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] `Engine.Resurface` Strategy 1 reworked to retrieve dormant candidates via `gatherResurfaceCandidates` within the operational `min_dormancy_days` floor and let the LLM judge each; gate only on the operational confidence floor; skip when the evaluator is nil (no window fallback); serendipity unchanged
      → Evidence: report.md `### Code Diff Evidence` (resurface.go diff; no `INTERVAL '30 days'` / `relevance_score > 0.3` remains in Strategy 1)
- [x] Evaluator unit-tested with a scripted bridge runner: parses decision, routes to `resurface_evaluate`, forwards public signals, does NOT leak ArtifactID, errors on every failure path
      → Evidence: report.md `## Test Evidence` (TestBridgeResurfaceEvaluator_* PASS)
- [x] Operational bounds as fail-loud SST: `intelligence.resurface.{min_dormancy_days,max_candidates,confidence_floor}` in smackerel.yaml + config.sh + `internal/config/resurface.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `wireResurfaceEvaluator` builds the evaluator from the bridge + SST and calls `SetResurfaceConfig`; nil bridge ⇒ dormancy disabled
      → Evidence: report.md `### Code Diff Evidence` (wiring_cooling.go + main.go)
- [x] `go build ./...`, `go vet`, full intelligence + config + scheduler + cmd/core + scenario-lint packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-RESURFACE-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage — the evaluator + helper + SST tests persist the LLM-driven decision contract and the no-leak/error invariants; they fail if the path regresses
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression — the full intelligence + config packages run green with the rework; existing resurface tests (serendipity, MarkResurfaced, struct fields) unaffected
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-RESURFACE-01 | TestBridgeResurfaceEvaluator_ParsesDecision (+ NotWorth, ErrorPaths, NilReceiverAndRunner) | internal/intelligence/resurface_eval_test.go | unit (mock bridge) | SCN-021-RESURFACE-01 |
| T-021-RESURFACE-02 | TestResurfaceShouldSurface | internal/intelligence/resurface_eval_test.go | unit (pure helper) | SCN-021-RESURFACE-02 |
| T-021-RESURFACE-03 | TestLoadResurfaceConfig_* (populate, fail-loud, range) | internal/config/resurface_test.go | unit (SST) | SCN-021-RESURFACE-03 |

### Non-Goals

- The serendipity strategy (intentionally non-deterministic rediscovery).
- Live-LLM behavioral validation (live-stack tier).
