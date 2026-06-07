# Scopes: BUG-039-004

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Completes the owner's "NO const limits" sweep with the one remaining hardcoded
operational constant outside the intelligence/digest engines.

## Scope 1 — Price-drop threshold fallback → fail-loud SST

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] No hardcoded price-drop threshold literal remains in `internal/recommendation/watch/`
      → Evidence: report.md `### Audit Evidence` (grep of `= 0.10` in evaluator.go = 0)
- [x] New SST key `recommendations.watches.default_price_drop_threshold_pct` (`RECOMMENDATIONS_WATCHES_DEFAULT_PRICE_DROP_THRESHOLD_PCT`), fail-loud via `parseUnitFloat` + `> 0` guard (range (0,1])
      → Evidence: report.md `### Validation Evidence` (config generate OK; key in dev.env; config tests PASS)
- [x] Threaded through `watch.Options.DefaultPriceDropThresholdPct` → `Evaluator.defaultPriceDropThresholdPct`, wired from `cfg.Recommendations.Watches.DefaultPriceDropThresholdPct`
      → Evidence: report.md `### Code Diff Evidence` (evaluator.go + wiring_recommendation_watches.go; BUILD=0; VET=0)
- [x] `resolvePriceDropThreshold` helper extracted (trigger → filter → SST default), unit-tested without a DB
      → Evidence: report.md `## Test Evidence` (TestResolvePriceDropThreshold_* PASS)
- [x] Configured-default passthrough proven (a different default flows through, not a baked-in 0.10)
      → Evidence: report.md `## Test Evidence` (TestResolvePriceDropThreshold_UsesConfiguredDefault PASS)
- [x] `setRequiredEnv` in validate_test.go sets the new key so existing config-load tests stay green
      → Evidence: report.md `### Validation Evidence` (internal/config green)
- [x] `go build ./...`, `go vet`, and the recommendation/watch + config + cmd/core packages green
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-039-004-01..02` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] No regression — the price-drop integration test (always supplies an explicit threshold) and all watch tests stay green
      → Evidence: report.md `### Validation Evidence` + `bug.md` (no test coupled to the old literal)
- [x] No schema migration; `config/generated/` (gitignored) regenerated per run; `config generate --env test` carries the key into the test stack
      → Evidence: report.md `### Audit Evidence`

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-039-004-01 | TestResolvePriceDropThreshold_Precedence | internal/recommendation/watch/threshold_test.go | unit (precedence) | SCN-039-004-01 |
| T-039-004-02 | TestResolvePriceDropThreshold_UsesConfiguredDefault | internal/recommendation/watch/threshold_test.go | unit (SST passthrough) | SCN-039-004-02 |

### Non-Goals

- LLM-judging price-drop significance (the threshold is user-config; only the
  fallback moves to SST).
- Live-LLM / live-stack behavioral validation (integration tier).
