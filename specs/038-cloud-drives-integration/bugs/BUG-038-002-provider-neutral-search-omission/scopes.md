# Scopes: BUG-038-002 Provider-neutral Drive search omission

## Scope 1: Restore deterministic multi-provider search convergence

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** backend search and live E2E bugfix

### Gherkin Scenarios

```gherkin
Feature: Provider-neutral Drive artifacts remain discoverable

  Scenario: Both providers surface immediately after completed extraction
    Given one Google file and one memdrive file contain the same query terms
    And both scans and extraction passes complete successfully
    When the authenticated client searches the live canonical API
    Then both exact artifact IDs are returned
    And each result carries its exact provider identity

  Scenario: Earlier generic contenders cannot suppress isolated provider rows
    Given earlier packages already produced enough exact-title generic contenders to fill the result limit
    And two newly extracted Drive artifacts share a collision-resistant per-run term
    When the authenticated client searches for that isolated term
    Then the final bounded candidate set includes both Drive artifacts
    And duplicate artifact IDs are not returned

  Scenario: Provider and audience policy remains strict
    Given provider and audience metadata are persisted for both Drive artifacts
    When search filters are applied
    Then only policy-eligible rows are returned
    And no provider or audience assertion is weakened
```

### Implementation Plan

1. Reproduce the exact live failure on a freshly created disposable stack and compare it with shared-corpus contamination.
2. Add the adversarial regression before changing production code and prove it is RED.
3. Isolate the fixture with one collision-resistant term shared by both providers while preserving generic contenders.
4. Re-run focused unit, integration, live E2E, and the full serialized Drive E2E package.
5. Run impacted Go/Python units, check, lint, format, packet gates, and regression guards.
6. Commit and push the isolated branch through normal hooks while leaving certification in progress.

### Implementation Files

- `tests/e2e/drive/drive_cross_feature_e2e_test.go`
- `specs/038-cloud-drives-integration/bugs/BUG-038-002-provider-neutral-search-omission/`

### Change Boundary

**Allowed file families:** `tests/e2e/drive/drive_cross_feature_e2e_test.go` and this BUG-038-002 packet.

**Excluded surfaces:** production search/runtime behavior, policy weakening, arbitrary sleeps, package de-serialization, synthesis/assistant packets, target deployment, `knb`, and release-train configuration.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Earlier generic contenders cannot suppress isolated provider rows | `e2e-api` | `tests/e2e/drive/drive_cross_feature_e2e_test.go` | Twenty exact-title generic contenders fill the bounded window, while both providers remain discoverable by the per-run term | `./smackerel.sh test e2e --go-run '^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$'` | Yes; disposable stack |
| Search contract units | `unit` | `internal/api/search_test.go` | Existing bounded search and response contracts remain green | `./smackerel.sh test unit --go --go-run 'TestSearch' --verbose` | No |
| Multi-provider retrieval | `integration` | `tests/integration/drive/drive_cross_feature_test.go`, `drive_multi_provider_search_test.go` | Real PostgreSQL rows from both providers remain discoverable under shared search logic | `./smackerel.sh test integration --go-run '^(TestDriveArtifactsFeedRecipesExpensesListsAnnotationsMealPlanDigest|TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters)$'` | Yes; disposable stack |
| Regression E2E provider-neutral search | `e2e-api` | `tests/e2e/drive/drive_cross_feature_e2e_test.go` | Exact broad-run failure with twenty generic contenders, per-run query, exact IDs, and provider metadata | `./smackerel.sh test e2e --go-run '^TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers$'` | Yes; disposable stack |
| Broader E2E regression | `e2e-api` | `tests/e2e/drive/` | Entire serialized Drive package proves no provider/policy/cleanup regression and cascade recovery | `./smackerel.sh test e2e --go-run '^(TestDrive|TestMultiProviderDrive|TestLowConfidenceConfirmation|TestTelegramRetrieval|TestFolderMove|TestSkippedAndBlocked|TestSaveRulesList|TestTelegramReceipt)'` | Yes; disposable stack |
| Impacted Go/Python units | `unit` | affected packages plus `ml/tests/` | Search and ML request-path regressions remain green | repository CLI focused unit commands | No |
| Static quality | `lint` | changed source/tests | Check, lint, and format report zero warnings | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh format --check` | No |
| Governance | `artifact` | packet and changed files | Artifact, traceability, implementation-reality, state, and regression guards | committed Bubbles guard scripts | No |

### Definition of Done

- [ ] Root cause confirmed with current-session live RED evidence.
- [ ] Pre-fix regression test fails on the exact provider-neutral omission.
- [ ] Both providers surface immediately after completed extraction with exact IDs and metadata.
- [ ] Earlier generic contenders cannot suppress isolated provider rows.
- [ ] Fix implemented at the first confirmed owning layer without sleeps or weakened assertions.
- [ ] Identity, tenant, audience, provider, and cleanup behavior remain strict and isolated.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Focused unit and integration tests pass.
- [ ] Impacted Go and Python unit suites pass.
- [ ] Drive package cascade-noise scenarios recover on the same disposable stack.
- [ ] Regression tests contain no mock/interception, skip/only, bailout, or tautological patterns.
- [ ] Check, lint, and format pass with zero warnings.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Packet artifact, traceability, implementation-reality, state-transition, and regression guards pass at `in_progress`.
- [ ] Source branch is committed and pushed through normal hooks; validate-owned certification remains `in_progress`.
