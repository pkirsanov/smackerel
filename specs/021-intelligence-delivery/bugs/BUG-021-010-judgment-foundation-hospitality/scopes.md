# Scopes: BUG-021-010

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Continues BUG-021-009; introduces the reusable judgment foundation and the first
hospitality + first `digest`-package LLM judgment on it.

## Scope 1 — Reusable judgment foundation + LLM-driven hospitality alerts

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `agent.InvokeJudgment[T]` foundation + `JudgmentRunner` + `ErrJudgmentUnavailable`, unit-tested (routing, source, signal forwarding, `json:"-"` non-leak, nil-runner sentinel, all error paths)
      → Evidence: report.md `## Test Evidence` (TestInvokeJudgment_* PASS)
- [x] New scenario `config/prompt_contracts/hospitality-concern-evaluate-v1.yaml` (`hospitality_concern_evaluate`): batched guest+property concern judgment with input/output schema + a no-op tool; loads cleanly
      → Evidence: report.md `## Test Evidence` (scenario-lint registered 14; agent loader green)
- [x] `internal/digest/hospitality_eval.go`: `HospitalityEvaluator` + `BridgeHospitalityEvaluator` (on `InvokeJudgment`); `noop_hospitality_concern` registered via `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] `hospitality.go` reworked: threshold queries replaced by `gatherGuestSignals`/`gatherPropertySignals` + `assembleConcernAlerts` (LLM judges, maps by `ref`); `AssembleHospitalityContext` takes the evaluator + bounds; nil eval ⇒ no concern alerts (no threshold fallback)
      → Evidence: report.md `### Audit Evidence` (grep of `sentiment_score < 0.3` / `avg_rating < 3.5` / `issue_count >= 5` = 0)
- [x] Evaluator unit-tested with a scripted bridge runner: batch parse, routing to `hospitality_concern_evaluate`, guest/property signal forwarding, internal-Email non-leak, empty-input short-circuit, all error paths
      → Evidence: report.md `## Test Evidence` (TestBridgeHospitalityEvaluator_* PASS)
- [x] Operational bounds as fail-loud SST: `digest.hospitality.{guest_candidate_limit,property_candidate_limit}` in smackerel.yaml + config.sh + `internal/config/hospitality.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `wireHospitalityEvaluator` builds the evaluator from the bridge + SST and calls `SetHospitalityEvaluator`; `cmd/scenario-lint` blank-imports `internal/digest` for BS-010
      → Evidence: report.md `### Code Diff Evidence` (wiring_hospitality.go + main.go + scenario-lint)
- [x] `go build ./...`, `go vet`, agent + digest + config + cmd/core + scenario-lint packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-021-HOSP-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Broader regression — the existing 16 hospitality unit tests (IsEmpty, fallback formatting, struct) stay green with the rework
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-HOSP-01 | TestInvokeJudgment_* (parse/route/forward/non-leak, nil-runner, error paths) | internal/agent/judgment_test.go | unit (foundation) | SCN-021-HOSP-01 |
| T-021-HOSP-02 | TestBridgeHospitalityEvaluator_* (batch parse, routing, non-leak, empty, error paths) | internal/digest/hospitality_eval_test.go | unit (mock bridge) | SCN-021-HOSP-02 |
| T-021-HOSP-03 | TestLoadHospitalityConfig_* (populate, fail-loud, range) | internal/config/hospitality_test.go | unit (SST) | SCN-021-HOSP-03 |

### Non-Goals

- Migrating the existing four `agent.Bridge` evaluators onto `InvokeJudgment`
  (behaviour-preserving follow-up).
- Live-LLM behavioral validation (live-stack tier).
