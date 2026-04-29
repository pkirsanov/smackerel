# Bug: BUG-031-002 — DoD scenario fidelity gap (SCN-LST-001/002/003/004)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 031 — Live Stack Testing
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard, after recently being upgraded to run to completion against `specs/031-live-stack-testing`, reported 7 failures (3 Test-Plan-row mapping failures + 3 Gherkin→DoD content-fidelity gaps + 1 rollup) for 4 underlying scenarios in Scopes 2, 5, and 6:

- `SCN-LST-001` All migrations apply cleanly (Scope 2) — G068 unmapped
- `SCN-LST-002` Schema DDL resilience (Scope 2) — Test Plan row unmapped
- `SCN-LST-003` Full pipeline flow (Scope 5) — Test Plan row unmapped + G068 unmapped
- `SCN-LST-004` Search works after cold start (Scope 6) — Test Plan row unmapped + G068 unmapped

The guard's `scenario_matches_row` and `scenario_matches_dod` functions try `extract_trace_ids` (SCN/AC/FR/UC pattern) equality first and fall back to a fuzzy "≥2 / ≥3 significant words shared" check. The four affected Gherkin scenario names did not embed their `SCN-LST-NNN` IDs, so the trace-ID fast path could not fire; the fuzzy matcher's significant-word threshold was below 2 (or 3 for DoD) for these scenarios because their titles ("Schema DDL resilience", "Full pipeline flow", "Search works after cold start", "All migrations apply cleanly") share too few ≥4-character non-stop-words with the existing Test Plan rows / DoD bullets that describe the delivered tests.

## Reproduction (Pre-fix)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing 2>&1 | tail -10
❌ Scope 2: Database Migration Integration Tests scenario has no traceable Test Plan row: Schema DDL resilience
❌ Scope 5: E2E Capture → Process → Search scenario has no traceable Test Plan row: Full pipeline flow
❌ Scope 6: ML Sidecar Readiness Gate scenario has no traceable Test Plan row: Search works after cold start
❌ Scope 2: Database Migration Integration Tests Gherkin scenario has no faithful DoD item preserving its behavioral claim: All migrations apply cleanly
❌ Scope 5: E2E Capture → Process → Search Gherkin scenario has no faithful DoD item preserving its behavioral claim: Full pipeline flow
❌ Scope 6: ML Sidecar Readiness Gate Gherkin scenario has no faithful DoD item preserving its behavioral claim: Search works after cold start
❌ DoD content fidelity gap: 3 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
ℹ️  DoD fidelity: 12 scenarios checked, 9 mapped to DoD, 3 unmapped
RESULT: FAILED (7 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each flagged scenario the bug investigator inspected the production code (`internal/db/migrations/`, `internal/api/ml_readiness.go`, `internal/api/health.go`, `cmd/core/services.go`) and the test files under `tests/integration/` and `tests/e2e/`. All four behaviors are genuinely **delivered-and-tested**; the guard failed only because the Gherkin scenario titles did not embed the `SCN-LST-NNN` IDs and the fuzzy fallback's word overlap was below threshold.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-LST-001 | Yes — consolidated migrations 001/018/019 apply against fresh PostgreSQL; all 12 expected tables, 11 indexes, vector + pg_trgm extensions, schema_version count ≥ 3 | Yes — `TestMigrations_AllTablesExist`, `TestMigrations_SchemaVersionCount`, `TestMigrations_ExtensionsLoaded`, `TestMigrations_ArtifactsColumns`, `TestMigrations_IndexesExist`, `TestMigrations_AnnotationsConstraints` | `tests/integration/db_migration_test.go` | `internal/db/migrations/` |
| SCN-LST-002 | Yes — DDL drop + recreate of `lists`/`list_items` leaves other tables intact; resilience verified via in-test DDL roundtrip | Yes — `TestMigrations_TableDropAndRecreate` | `tests/integration/db_migration_test.go::TestMigrations_TableDropAndRecreate` | `internal/db/migrations/018_consolidated_baseline.sql` |
| SCN-LST-003 | Yes — full pipeline flow: POST `/api/capture` → NATS ARTIFACTS WorkQueue → ML processing → poll `/api/artifact/{id}` until `processing_status=processed` → POST `/api/search` returns the artifact | Yes — `TestE2E_CaptureProcessSearch` | `tests/e2e/capture_process_search_test.go::TestE2E_CaptureProcessSearch` | `internal/api/capture.go`, `internal/api/search.go`, ML sidecar `ml/app/` |
| SCN-LST-004 | Yes — `WaitForMLReady` polls `/health` every 500ms until healthy or timeout; on timeout sets `mlHealthy=false` so search falls back to text mode until the next health check | Yes — `TestMLReadiness_WaitForHealthy`, `TestMLReadiness_TimeoutFallback`, `TestMLReadiness_EmptyURL`, `TestMLReadiness_ZeroTimeout` | `tests/integration/ml_readiness_test.go` | `internal/api/ml_readiness.go`, `internal/api/health.go`, `cmd/core/services.go` |

**Disposition:** All four scenarios are **delivered-but-undocumented at the trace-ID level** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/031-live-stack-testing/scopes.md` Scope 2 Gherkin block prefixes the scenario name with `SCN-LST-001` and `SCN-LST-002`
- [x] Parent `specs/031-live-stack-testing/scopes.md` Scope 5 Gherkin block prefixes the scenario name with `SCN-LST-003`
- [x] Parent `specs/031-live-stack-testing/scopes.md` Scope 6 Gherkin block prefixes the scenario name with `SCN-LST-004`
- [x] Parent `specs/031-live-stack-testing/scopes.md` Scope 2 / 5 / 6 DoD entries include faithful trace-ID-bearing bullets (`Scenario SCN-LST-001`, `Scenario SCN-LST-002`, `Scenario SCN-LST-003`, `Scenario SCN-LST-004`)
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-002-dod-scenario-fidelity-gap` PASS
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing` PASS
- [x] No production code changed (boundary preserved — only `specs/031-live-stack-testing/scopes.md` and the new bug folder)
