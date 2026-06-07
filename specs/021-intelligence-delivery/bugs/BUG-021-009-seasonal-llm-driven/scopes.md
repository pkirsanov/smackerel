# Scopes: BUG-021-009

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Continues BUG-021-008 (same directive; delivered through the existing
`seasonal.analyze` ML/NATS path rather than `agent.Bridge`).

## Scope 1 — LLM-driven seasonal significance via the ML path

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `DetectSeasonalPatterns` reworked to gather raw signals (this-month vs last-year volume; topic candidates above an operational floor) and delegate the significance JUDGMENT to the `seasonal.analyze` ML path; the `ratio < 0.7` / `ratio > 1.5` decision and the `topic_seasonal` "≥5" claim are removed
      → Evidence: report.md `### Audit Evidence` (grep of `ratio < 0.7` / `ratio > 1.5` in monthly.go = 0)
- [x] `handle_seasonal_analyze` reworked to judge significance from the raw signals and return `{observations:[...]}`, fixing the prior request/response contract mismatch on both ends
      → Evidence: report.md `### ML Handler Evidence` (TestSeasonalAnalyze PASS; new contract)
- [x] No hardcoded fallback: nil config / nil NATS / no-LLM ⇒ seasonal detection skipped (the monthly report omits the section)
      → Evidence: report.md `### ML Handler Evidence` (no-LLM → empty observations) + `### Code Diff Evidence` (skip paths)
- [x] Operational bounds as fail-loud SST: `intelligence.seasonal.{min_data_days,topic_min_captures,topic_candidate_limit,max_observations}` in smackerel.yaml + config.sh + `internal/config/seasonal.go`; `./smackerel.sh config generate` resolves them
      → Evidence: report.md `### Validation Evidence` (config generate OK; keys in dev.env; loader tests PASS)
- [x] Wiring `wireSeasonalConfig` builds `intelligence.SeasonalConfig` from SST and calls `SetSeasonalConfig`; SST load failure ⇒ seasonal detection disabled
      → Evidence: report.md `### Code Diff Evidence` (wiring_cooling.go + main.go)
- [x] `go build ./...`, `go vet`, intelligence + config + cmd/core packages green; full Python ML suite green
      → Evidence: report.md `### Validation Evidence`
- [x] `./smackerel.sh lint` (Go lint + Python ruff + web-validate) passes
      → Evidence: report.md `### Lint Evidence`
- [x] `SCN-021-SEASONAL-01..02` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage — the SST loader tests + the reworked ML handler tests persist the operational bounds and the no-LLM/no-signal skip contract; they fail if the path regresses
      → Evidence: report.md `### ML Handler Evidence` + `### Validation Evidence`
- [x] Broader regression — the existing `TestDetectSeasonalPatterns_NilPool` and the full intelligence + config + Python suites run green with the rework
      → Evidence: report.md `### Validation Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-021-SEASONAL-01 | TestLoadSeasonalConfig_* (populate, fail-loud, range) | internal/config/seasonal_test.go | unit (SST) | SCN-021-SEASONAL-01 |
| T-021-SEASONAL-02 | TestSeasonalAnalyze::test_no_llm_returns_empty_observations / test_no_signals_returns_empty / test_response_shape_has_observations_key | ml/tests/test_intelligence_handlers.py | unit (ML handler) | SCN-021-SEASONAL-02 |

### Non-Goals

- A new `agent.Bridge` seasonal scenario (the existing ML path is reused).
- Live-LLM behavioral validation (live-stack tier).
