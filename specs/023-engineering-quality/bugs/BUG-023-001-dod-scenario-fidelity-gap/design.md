# Design: BUG-023-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [023 spec](../../spec.md) | [023 scopes](../../scopes.md) | [023 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 023 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the delivered behavior (race-free `mlClient`, typed `Dependencies`, SST connector paths, live Ollama probe, live Telegram health) but did not embed the `SCN-023-NN` trace ID. The traceability-guard's `scenario_matches_dod` function tries trace-ID equality first and falls back to a fuzzy "≥3 significant words shared" check; for SCN-023-01/02/04/06/07 the DoD wording happened to fall below the threshold (e.g. SCN-023-01 says "Concurrent health checks are race-free" while the DoD says "`mlClient()` guarded by `sync.Once` — race detector clean"; the only shared significant words are "race" and "checks/check"), so the gate fails.

Two ancillary problems accumulated under the same root:

1. `scenario-manifest.json` was never generated for spec 023 (G057/G059), so the manifest cross-check fails immediately.
2. The Test Plan tables in all three scopes use a `Type | Test | Purpose | Scenarios Covered` schema with no `Location` column. Each row's "Test" cell carries a human description rather than a concrete file path, so the guard's "mapped row has no concrete test file path" check fails for every scenario-to-row mapping (9 of 9). Every behaviour is in fact backed by an existing test file (`internal/api/health_test.go`, `internal/config/validate_test.go`, `internal/connector/sync_interval_test.go`); the path strings were just never written into the Test Plan rows.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only preferred. No production code changes." — is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts:

1. **Trace-ID-bearing DoD bullets** appended to `scopes.md`:
   - Scope 1 DoD gains bullets for `SCN-023-01` (with raw `go test` output for `TestMLClient_ConcurrentAccess`, `TestMLClient_PreSet`, `TestHealthHandler_ConcurrentAccess`) and `SCN-023-02` (with `go build ./...` exit 0 plus `TestHealthHandler_AllHealthy` PASS as proof the typed `Dependencies` interfaces compile and run).
   - Scope 2 DoD gains bullets for `SCN-023-04` (raw output for `TestLoad_ConnectorPathFields`, `TestLoad_ConnectorPathFieldsOptional`), `SCN-023-06` (raw output for `TestCheckOllama_Healthy/_Down/_NotConfigured/_Unreachable`, `TestHealthHandler_OllamaUp/_OllamaNotConfigured`), and `SCN-023-07` (raw output for `TestHealthHandler_TelegramConnected/_TelegramDisconnected`).
   - SCN-023-03, SCN-023-05, SCN-023-08, SCN-023-09 are already faithfully mapped — no new DoD bullets required.

2. **Bridge Test Plan rows** added at the top of each scope's Test Plan table. These rows use the same column schema but their "Test" cell carries the form `<TestName> in <internal/.../foo_test.go>` so the row contains an existing concrete test file path that the guard finds before falling through to the legacy human-description rows. One bridge row is added per Gherkin scenario in the scope (3 in Scope 1, 4 in Scope 2, 2 in Scope 3 = 9 total).

3. **Scenario manifest** `specs/023-engineering-quality/scenario-manifest.json` is generated covering all 9 `SCN-023-*` scenarios. Each entry has `scenarioId`, `scope`, `requiredTestType`, `linkedTests` (with `file` + `function`), `evidenceRefs` (unit-test + source pointers), and `linkedDoD`.

4. **Report cross-reference** added to `specs/023-engineering-quality/report.md` documenting the bug, the per-scenario classification, and the raw verification evidence with full `internal/api/health_test.go`, `internal/config/validate_test.go`, and `internal/connector/sync_interval_test.go` paths so `report_mentions_path` succeeds.

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — race-free concurrent health, typed `Dependencies`, SST connector paths, live Ollama, live Telegram are all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (16 failures)`; post-fix it returns `RESULT: PASSED (0 warnings)`. The guard run is captured in `report.md` under "Validation Evidence". As an additional safety net, the 13 underlying behavior tests for the previously-flagged scenarios are re-executed and their PASS output is recorded inline.
