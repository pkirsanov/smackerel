# Scopes: BUG-021-008

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Continues BUG-021-007 (same directive, same pattern; batch evaluator variant).

## Scope 1 — LLM-driven expertise tier & growth classification

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] New scenario `config/prompt_contracts/expertise-classify-v1.yaml` (`expertise_classify`): batched per-situation tier + growth judgment with input/output array schema + a no-op tool; loads cleanly
      → Evidence: report.md `## Test Evidence` (scenario-lint registered 13; agent loader test green)
- [x] `internal/intelligence/expertise_eval.go`: `ExpertiseEvaluator` + `BridgeExpertiseEvaluator` (mockable) routing to the scenario; `ExpertiseConfig` bundle; `noop_expertise_classify` registered via `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] `GenerateExpertiseMap` reworked to gather signals (capped by operational `max_topics`) and make ONE batched LLM call, mapping classifications back by `ref`; `computeDepthScore`/`assignTier`/`computeTrajectory`/`DepthScore` removed; fails loud when the evaluator is nil (no tier fallback)
      → Evidence: report.md `### Code Diff Evidence` + `### Audit Evidence` (grep of the removed functions = 0)
- [x] Evaluator unit-tested with a scripted bridge runner: parses the batch, routes to `expertise_classify`, forwards public signals, does NOT leak TopicID, correlates by `ref`, short-circuits empty input, errors on every failure path
      → Evidence: report.md `## Test Evidence` (TestBridgeExpertiseEvaluator_* PASS)
- [x] Operational bounds as fail-loud SST: `intelligence.expertise.{max_topics,maturity_days,blind_spot_min_mentions,blind_spot_max_captures,blind_spot_limit}` in smackerel.yaml + config.sh + `internal/config/expertise.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `wireExpertiseEvaluator` builds the evaluator from the bridge + SST and calls `SetExpertiseConfig`; nil bridge ⇒ endpoint fails loud
      → Evidence: report.md `### Code Diff Evidence` (wiring_cooling.go + main.go)
- [x] `go build ./...`, `go vet`, full intelligence + config + cmd/core + scenario-lint + api packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-EXPERTISE-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage — the evaluator + SST tests persist the LLM-driven batch contract and the no-leak/ref/error invariants; they fail if the path regresses
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression — the lock tests for the removed magic-number functions are deleted; the remaining intelligence + config + api packages run green
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-EXPERTISE-01 | TestBridgeExpertiseEvaluator_ParsesBatch (+ ErrorPaths, NilReceiverAndRunner) | internal/intelligence/expertise_eval_test.go | unit (mock bridge) | SCN-021-EXPERTISE-01 |
| T-021-EXPERTISE-02 | TestBridgeExpertiseEvaluator_EmptyTopics (+ ref correlation in ParsesBatch) | internal/intelligence/expertise_eval_test.go | unit (batch contract) | SCN-021-EXPERTISE-02 |
| T-021-EXPERTISE-03 | TestLoadExpertiseConfig_* (populate, fail-loud, range) | internal/config/expertise_test.go | unit (SST) | SCN-021-EXPERTISE-03 |

### Non-Goals

- Live-LLM behavioral validation (live-stack tier).
- Re-classifying blind-spot detection as an LLM judgment (only its bounds move
  to SST).
