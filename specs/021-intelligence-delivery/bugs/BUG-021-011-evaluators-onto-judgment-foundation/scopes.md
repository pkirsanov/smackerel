# Scopes: BUG-021-011

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Completes the `agent.InvokeJudgment` foundation from BUG-021-010 by retrofitting
the four pre-existing evaluators onto it. Behaviour-preserving.

## Scope 1 — Migrate the four evaluators onto agent.InvokeJudgment

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `cooling.go`, `alert_timing.go`, `resurface_eval.go`, `expertise_eval.go`: each `BridgeXEvaluator` method body is a single `agent.InvokeJudgment[T]` call (plus the nil-receiver guard; expertise unwraps `resp.Classifications`); no marshal/invoke/validate/decode plumbing remains in the four methods
      → Evidence: report.md `### Code Diff Evidence` (179 deletions / 44 insertions; BUILD=0; VET=0)
- [x] Each `BridgeXEvaluator.Runner` is `agent.JudgmentRunner`; the four private `xBridgeRunner` interfaces are removed
      → Evidence: report.md `### Audit Evidence` (grep of the four interface names = 0)
- [x] The four `ErrXEvaluatorUnavailable` sentinels are removed; the evaluators yield `agent.ErrJudgmentUnavailable` on the nil-receiver / unavailable path
      → Evidence: report.md `### Audit Evidence` (grep of the four sentinel names = 0)
- [x] The now-unused `fmt` import is dropped from each evaluator file; `json` / `errors` remain for `init()`
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0 — unused imports would fail the build)
- [x] Every existing evaluator unit test passes — parse, routing, signal forwarding, internal-field non-leak, nil-receiver/nil-runner (now asserting `agent.ErrJudgmentUnavailable`), all error paths
      → Evidence: report.md `## Test Evidence` (TestBridge{Cooling,AlertTiming,Resurface,Expertise}Evaluator_* PASS)
- [x] The four test nil-receiver/nil-runner assertions switch to `agent.ErrJudgmentUnavailable`; the stale `coolingBridgeRunner` comment is corrected
      → Evidence: report.md `## Test Evidence` + `### Code Diff Evidence`
- [x] `go build ./...`, `go vet`, and the affected packages (intelligence, agent, digest, cmd/core, scenario-lint) are green
      → Evidence: report.md `### Validation Evidence`
- [x] No dangling references to the removed identifiers remain anywhere under `internal/` or `cmd/`
      → Evidence: report.md `### Audit Evidence` (grep = none)
- [x] Zero behaviour change — no scenario YAML, SST, wiring, producer, or schema edits
      → Evidence: report.md `### Audit Evidence` (diff confined to the 8 files; no migration)
- [x] Broader regression — the full intelligence + agent + digest + cmd/core packages run green with the refactor
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-MIGRATE-01 | TestBridgeCoolingEvaluator_* / TestBridgeAlertTimingEvaluator_* / TestBridgeResurfaceEvaluator_* / TestBridgeExpertiseEvaluator_* (parse, routing, non-leak, nil paths, error paths) | internal/intelligence/{cooling,alert_timing,resurface_eval,expertise_eval}_test.go | unit (mock bridge) | SCN-021-MIGRATE-01 |
| T-021-MIGRATE-02 | TestInvokeJudgment_* (the shared primitive the four now route through) | internal/agent/judgment_test.go | unit (foundation) | SCN-021-MIGRATE-02 |

### Non-Goals

- New behaviour, scenarios, or config (transport refactor only).
- Live-LLM behavioral validation (live-stack tier).
