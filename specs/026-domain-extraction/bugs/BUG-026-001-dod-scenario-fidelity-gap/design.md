# Design: BUG-026-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [026 spec](../../spec.md) | [026 scopes](../../scopes.md) | [026 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 026 was authored before Gate G068 (Gherkin → DoD Content Fidelity) was tightened. The DoD bullets accurately described the delivered behavior (validation, registry matching, NATS dispatch, ML retry, parallel synthesis, multi-ingredient parsing, JSONB filters, Telegram domain card rendering, etc.) but did not embed the `SCN-026-N-M` trace ID that the guard's `scenario_matches_dod` matcher tries first; the fuzzy "≥3 significant words shared" fallback also did not satisfy 17 of 44 scenarios because the DoD wording paraphrased the Gherkin instead of restating it.

Three ancillary problems accumulated under the same root:

1. `scenario-manifest.json` was never generated for spec 026 (Gates G057/G059).
2. Several Test Plan rows reference paths that do not exist on disk — the actual tests live at related-but-different paths added during the implementation/test-gap rounds:
   - `internal/db/migrations_test.go` → `tests/integration/db_migration_test.go`
   - `internal/pipeline/subscriber_test.go` (for domain-specific tests) → `internal/pipeline/domain_subscriber_test.go`
   - `tests/integration/nats_contract_test.go` → `internal/nats/contract_test.go`
   - `tests/integration/domain_extraction_test.go` → `tests/e2e/domain_e2e_test.go` (in-process unit-side covered in `internal/pipeline/domain_subscriber_test.go`)
   - `internal/api/search_test.go` (for domain intent/filter tests) → `internal/api/domain_intent_test.go` and `internal/api/domain_filter_test.go`
   - `tests/e2e/domain_search_test.go` → `tests/e2e/domain_e2e_test.go`
3. The Gherkin scenario `SCN-026-8-8 SearchResult includes domain_data when present` had no Test Plan row even though `TestSearchResult_DomainDataSerialization` already covers it.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The boundary clause from the user prompt — "artifact-only preferred. No production code changes." — is honored: gap analysis proved every behavior is delivered and tested, so no production change is justified.

The fix has four parts:

1. **Trace-ID-bearing DoD bullets** added to `scopes.md` for the 17 unmapped scenarios. Each new bullet:
   - Embeds the `SCN-026-N-M` ID as the first words of the bullet text (so the trace-ID matcher succeeds before the fuzzy-word fallback ever runs)
   - Restates the Gherkin scenario name verbatim (so the fuzzy matcher would also succeed)
   - Names the concrete test functions and the source pointer
   - Carries inline raw `go test` / `pytest` evidence in fenced code blocks
2. **Scenario manifest** `specs/026-domain-extraction/scenario-manifest.json` is generated covering all 44 `SCN-026-N-M` scenarios with `scenarioId`, `scope`, `requiredTestType`, `linkedTests` (`file` + `function`), `evidenceRefs` (unit-test/source pointers), and `linkedDoD`.
3. **Test Plan path corrections** to clear the "mapped row references no existing concrete test file" failures and the "report is missing evidence reference for concrete test file" failures. The corrected rows still satisfy the Gherkin → Test Plan trace requirement; only the file path is updated.
4. **Report cross-reference** added to `specs/026-domain-extraction/report.md` documenting the bug, the per-scenario classification, the raw verification evidence, and the full relative path of every concrete test file the guard checks (so `report_mentions_path` succeeds for all 12 affected files).

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets added by this fix preserve the original DoD claims (the implemented behavior matches the Gherkin scenario verbatim — request/response validation, registry matching, ML retry/exhaustion, parallel synthesis dispatch, multi-ingredient parsing, JSONB recipe filters, recipe/product extraction structured output, Telegram nil/empty rendering are all genuinely delivered and tested) and only add the trace ID and raw test evidence the gate requires. No DoD bullet was deleted or weakened. No Gherkin scenario was edited. The behavior the Gherkin describes is the behavior the production code implements; the only thing being fixed is the documentation linkage.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself. Pre-fix it returned `RESULT: FAILED (43 failures, 0 warnings)` with `DoD fidelity: 44 scenarios checked, 27 mapped to DoD, 17 unmapped`; post-fix it returns `RESULT: PASSED (0 warnings)` with `DoD fidelity: 44 scenarios checked, 44 mapped to DoD, 0 unmapped`. The guard run is captured in `report.md` under "Validation Evidence".

The 14+ underlying behavior tests (Go) plus 6 ML-sidecar tests for the previously-flagged scenarios are run inline in `report.md > Test Evidence` to prove the artifact change does not silently rely on tests that have since regressed.
