# Design: BUG-019-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [019 spec](../../spec.md) | [019 scopes](../../scopes.md) | [019 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 019 was authored before two traceability-guard tightenings landed:

1. The Test Plan row path-extraction regex `([A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+` (in `extract_path_candidates()` at `traceability-guard.sh:276-279`) does not match paths that contain a `*` glob. The original Test Plan rows used wildcards like `Existing internal/connector/discord/*_test.go` to point at "all matching test files". The regex stops at the `*`, so no candidate path is produced and the guard reports "mapped row has no concrete test file path".
2. Gate G068's `scenario_matches_dod` matcher requires either trace-ID equality (`SCN-019-NNN` token in both Gherkin and DoD bullet) or "≥ N significant words shared". The pre-existing DoD bullets accurately described delivered behavior (auto-start blocks, descriptive errors, helper functions, SST compliance) but did not embed the `SCN-019-002`/`SCN-019-003` trace IDs and the fuzzy matcher fell below threshold.

A third symptom — `report is missing evidence reference for concrete test file: internal/api/health_test.go` — was caused by `report.md` never spelling that path verbatim. The path appeared only in `scopes.md` Test Plan rows; `report.md` referenced `health_test.go` only as a bare filename.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only preferred. No production code changes." — is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts:

1. **Concrete test file paths** in `scopes.md` Test Plan rows. The wildcard rows are replaced with explicit comma-separated paths:
   - SCN-019-002 row → `internal/connector/discord/discord_test.go, internal/connector/discord/gateway_test.go`
   - SCN-019-003 row → `internal/connector/discord/discord_test.go, internal/connector/twitter/twitter_test.go, internal/connector/markets/markets_test.go, internal/connector/alerts/alerts_test.go`
   - SCN-019-004 is reassigned cleanly to the existing `tests/integration/test_connector_wiring.sh` row (the natural home for a config-entries scenario) by removing it from the twitter unit-test row that incorrectly carried it
2. **Trace-ID-bearing DoD bullets** added at the bottom of Scope 1 DoD in `scopes.md`. Each new bullet contains the literal `Scenario SCN-019-002` / `Scenario SCN-019-003` token plus raw `go test` output and source-file pointers. No existing DoD bullet is deleted or weakened — Gate G068's stated failure mode ("DoD may have been rewritten to match delivery instead of the spec") is therefore not triggered: the new bullets *add* the trace ID and evidence the gate requires while preserving the original DoD claims.
3. **Report cross-reference** appended to `specs/019-connector-wiring/report.md` with a full "BUG-019-001 — DoD Scenario Fidelity Gap" section. The section spells out `internal/api/health_test.go` verbatim (multiple times), classifies the gap, and embeds raw `go test` output for all 14 underlying behavior tests. This satisfies `report_mentions_path` for SCN-019-005.
4. **Bug folder packet** under `specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/` with the canonical six artifacts so the bug itself is artifact-lint-clean.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (auto-start blocks, descriptive errors, helper functions, SST compliance — all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (7 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence". The 14 underlying behavior tests for the previously-flagged scenarios still pass with no regressions and are recorded inline.
