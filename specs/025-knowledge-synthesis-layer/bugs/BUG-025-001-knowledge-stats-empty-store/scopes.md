# Scopes: BUG-025-001 Knowledge stats empty-store 500

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Return valid stats for an empty knowledge store

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-025-001 knowledge stats handles empty stores
  Scenario: Knowledge stats returns zero values for an empty store
    Given the knowledge store contains no concepts, entities, or synthesized artifacts
    When an authenticated caller requests knowledge stats
    Then the response is successful
    And the stats counts are zero
    And prompt_contract_version is an explicit empty value

  Scenario: Knowledge stats regression fails on unhandled empty prompt contract version
    Given knowledge_concepts has no rows
    When the stats query computes the latest prompt contract version
    Then the empty result is handled explicitly without scanning NULL into a string
```

### Implementation Plan
1. Capture targeted red-stage evidence for empty-store `GET /api/knowledge/stats`.
2. Adjust `GetStats` so empty `knowledge_concepts` produces an explicit empty prompt contract version without suppressing real DB errors.
3. Add adversarial regression coverage for empty concepts, empty lint reports, and no synthesized artifacts.
4. Run targeted unit/integration/E2E coverage through the repo CLI.
5. Re-run the broader E2E suite when routed blockers are ready.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-025-001-01 | Empty-store GetStats returns zero values | unit/integration | `internal/knowledge/store_test.go` or `tests/integration` | Empty knowledge tables return zero counts and empty prompt contract version without error | BUG-025-001-SCN-001 |
| T-BUG-025-001-02 | Regression E2E: empty stats endpoint succeeds | e2e-api | `tests/e2e/knowledge_synthesis_test.go` or focused E2E stats test | Fresh disposable stack returns HTTP 200 from `/api/knowledge/stats` | BUG-025-001-SCN-001 |
| T-BUG-025-001-03 | Adversarial empty concepts query | unit/integration | `internal/knowledge/store_test.go` | No `knowledge_concepts` rows cannot scan NULL into a string | BUG-025-001-SCN-002 |
| T-BUG-025-001-04 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Broad suite no longer reports the empty-store stats 500 | BUG-025-001-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence — **Phase:** implement. **Claim Source:** executed.
  > Evidence:
  > ```text
  > $ timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
  > === RUN   TestKnowledgeStore_TablesExist
  > knowledge_store_test.go: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
  > --- FAIL: TestKnowledgeStore_TablesExist
  > FAIL
  > Exit Code: 1
  > ```
  > Root cause: `internal/knowledge/store.go::GetStats` scanned the scalar subquery for latest `prompt_contract_version` into a Go string. The old query coalesced inside the subquery, so an empty `knowledge_concepts` table still produced a NULL scalar result and pgx failed the string scan.
- [x] Empty knowledge store stats return HTTP 200 with zero-valued content — **Phase:** implement. **Claim Source:** executed.
  > Evidence:
  > ```text
  > $ timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
  > go-e2e: applying -run selector: TestKnowledgeStore_TablesExist
  > === RUN   TestKnowledgeStore_TablesExist
  >     knowledge_store_test.go:43: knowledge stats: concepts=0 entities=0 synthesized=0 pending=0 contract=
  > --- PASS: TestKnowledgeStore_TablesExist (0.06s)
  > PASS
  > ok      github.com/smackerel/smackerel/tests/e2e        0.069s
  > ```
- [x] Prompt contract version empty result is handled explicitly without masking real DB errors — **Phase:** implement. **Claim Source:** executed.
  > Evidence: `GetStats` now uses `COALESCE((SELECT prompt_contract_version FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1), '')`, so the no-row scalar subquery is converted to an explicit empty string. The lint report branch only treats `pgx.ErrNoRows` as empty/default; other DB errors still return `get knowledge lint stats`.
- [x] Pre-fix regression test fails for empty-store stats — **Phase:** implement. **Claim Source:** executed.
  > Evidence: focused E2E red proof above exited 1 before the implementation change, with `/api/knowledge/stats` returning HTTP 500 for a fresh empty store.
- [x] Adversarial regression case exists for no `knowledge_concepts` rows — **Phase:** implement. **Claim Source:** executed.
  > Evidence:
  > ```text
  > $ timeout 1200 ./smackerel.sh test integration
  > === RUN   TestKnowledgeStats_EmptyStoreReturnsZeroValues
  > --- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues (0.55s)
  > ```
  > The new integration test truncates `knowledge_concepts`, `knowledge_entities`, `knowledge_lint_reports`, `edges`, and `artifacts`, then calls `knowledge.NewKnowledgeStore(pool).GetStats(ctx)` and asserts zero counts, nil `LastSynthesisAt`, zero lint counts, and `PromptContractVersion == ""`.
- [x] Post-fix targeted stats regression passes — **Phase:** implement. **Claim Source:** executed.
  > Evidence:
  > ```text
  > $ timeout 600 ./smackerel.sh test unit --go
  > ok      github.com/smackerel/smackerel/internal/knowledge 0.011s
  > Exit Code: 0
  >
  > $ timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
  > --- PASS: TestKnowledgeStore_TablesExist (0.06s)
  > PASS
  > ok      github.com/smackerel/smackerel/tests/e2e        0.069s
  > Exit Code: 0
  > ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement. **Claim Source:** executed.
  > Evidence: `tests/integration/knowledge_stats_test.go::TestKnowledgeStats_EmptyStoreReturnsZeroValues` preserves the isolated empty-store regression that would fail if the NULL scan returned. `tests/e2e/knowledge_store_test.go::TestKnowledgeStore_TablesExist` keeps the HTTP 200 assertion and now verifies the live endpoint response contract without assuming broad-suite global state is empty: required numeric fields must be present and non-negative, `last_synthesis_at` must be present as `null` or an RFC3339 timestamp, and `prompt_contract_version` must be present and non-null.
- [ ] Broader E2E regression suite passes
  > Evidence blocker — **Phase:** implement. **Claim Source:** executed.
  > ```text
  > $ timeout 3600 ./smackerel.sh test e2e
  > === RUN   TestKnowledgeStore_TablesExist
  >     knowledge_store_test.go:77: knowledge stats: concepts=0 entities=0 edges=3 completed=0 pending=0 failed=2 contract=
  > --- PASS: TestKnowledgeStore_TablesExist (0.05s)
  > --- FAIL: TestE2E_DomainExtraction (90.29s)
  > --- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.30s)
  > --- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.05s)
  > FAIL    github.com/smackerel/smackerel/tests/e2e        168.493s
  > BROAD_E2E_STATUS=1
  > Exit Code: 1
  > ```
  > The broad command proves the repaired `/api/knowledge/stats` assertion is valid after earlier broad E2E scenarios have seeded knowledge edges and synthesis failures. No completion claim is made for the broad E2E DoD item because the suite still exits 1 on unrelated E2E failures outside BUG-025-001.
- [x] Regression tests contain no silent-pass bailout patterns — **Phase:** implement. **Claim Source:** executed.
  > Evidence: workspace search over `tests/e2e/knowledge_store_test.go` and `tests/integration/knowledge_stats_test.go` for `route()`, `intercept()`, `msw`, `nock`, `wiremock`, `t.Skip`, and failure-condition early returns returned no matches. Both tests use direct assertions and `t.Fatal`/`t.Error` failures.
- [ ] Bug marked as Fixed in bug.md by the validation owner
  > Evidence pending validation owner certification; this implementation pass does not edit certification-owned status.
