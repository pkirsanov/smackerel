# Scopes: BUG-025-001 Knowledge stats empty-store 500

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Return valid stats for an empty knowledge store

**Status:** Done
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
5. Validate the broader E2E baseline after sibling fixes landed.

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
  > **Command:** `grep -nE "COALESCE\(\(SELECT prompt_contract_version|pgx.ErrNoRows|get knowledge lint stats" internal/knowledge/store.go`
  > **Exit Code:** 0
  > Evidence:
  > ```text
  > 131:            if err == pgx.ErrNoRows {
  > 477:                    COALESCE((SELECT prompt_contract_version FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1), '')`).Scan(
  > 488:    if err != nil && !errors.Is(err, pgx.ErrNoRows) {
  > 489:            return nil, fmt.Errorf("get knowledge lint stats: %w", err)
  > ```
  > `GetStats` converts the no-row scalar subquery to an explicit empty string, and the lint report branch only treats `pgx.ErrNoRows` as empty/default while preserving other database errors.
- [x] Pre-fix regression test fails for empty-store stats — **Phase:** implement. **Claim Source:** executed.
  > **Command:** `timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist`
  > **Exit Code:** 1
  > Evidence:
  > ```text
  > === RUN   TestKnowledgeStore_TablesExist
  > knowledge_store_test.go: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
  > --- FAIL: TestKnowledgeStore_TablesExist
  > FAIL
  > Exit Code: 1
  > ```
  > Focused E2E red proof exited 1 before the implementation change, with `/api/knowledge/stats` returning HTTP 500 for a fresh empty store.
- [x] Adversarial regression case exists for no `knowledge_concepts` rows — **Phase:** implement. **Claim Source:** executed.
  > **Command:** `timeout 1200 ./smackerel.sh test integration`
  > **Exit Code:** 1 overall; BUG-025-001 regression passed before unrelated suite failures.
  > Evidence:
  > ```text
  > $ timeout 1200 ./smackerel.sh test integration
  > === RUN   TestKnowledgeStats_EmptyStoreReturnsZeroValues
  > --- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues (0.55s)
  > --- FAIL: TestNATS_PublishSubscribe_Artifacts
  > --- FAIL: TestNATS_PublishSubscribe_Domain
  > --- FAIL: TestNATS_Chaos_MaxDeliverExhaustion
  > --- FAIL: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList
  > Exit Code: 1
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
  > **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/knowledge_store_test.go tests/integration/knowledge_stats_test.go`
  > **Exit Code:** 0
  > Evidence:
  > ```text
  > Scanning tests/e2e/knowledge_store_test.go
  > Adversarial signal detected in tests/e2e/knowledge_store_test.go
  > Scanning tests/integration/knowledge_stats_test.go
  > Adversarial signal detected in tests/integration/knowledge_stats_test.go
  > REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  > Files scanned: 2
  > Files with adversarial signals: 2
  > ```
  > `tests/integration/knowledge_stats_test.go::TestKnowledgeStats_EmptyStoreReturnsZeroValues` preserves the isolated empty-store regression that would fail if the NULL scan returned. `tests/e2e/knowledge_store_test.go::TestKnowledgeStore_TablesExist` keeps the HTTP 200 assertion and verifies the live endpoint response contract without assuming broad-suite global state is empty.
- [x] Broader E2E regression suite passes — **Phase:** validate. **Claim Source:** interpreted.
  > **Command:** existing BUG-025-001 report evidence review plus c6d2b26 broad E2E baseline evidence from `specs/039-recommendations-engine/report.md`
  > **Exit Code:** c6d2b26 broad baseline 0; not rerun during metadata-only closeout.
  > **Interpretation:** BUG-025-001 implementation evidence proves the fixed behavior directly: the focused E2E stats endpoint regression passed on a fresh stack, the adversarial live PostgreSQL regression passed for no `knowledge_concepts` rows, and the earlier broad implementation-stage E2E run showed `TestKnowledgeStore_TablesExist` passing before unrelated sibling failures. The later c6d2b26 baseline records full `./smackerel.sh test e2e` exit 0, proving the broad suite no longer reports the BUG-025-001 empty-store stats 500.
  > ```text
  > BUG-025-001 implementation broad-order stats evidence:
  > === RUN   TestKnowledgeStore_TablesExist
  >     knowledge_store_test.go:77: knowledge stats: concepts=0 entities=0 edges=3 completed=0 pending=0 failed=2 contract=
  > --- PASS: TestKnowledgeStore_TablesExist (0.05s)
  >
  > c6d2b26 broad E2E baseline evidence from specs/039-recommendations-engine/report.md:
  > Command: timeout 3600 ./smackerel.sh test e2e
  > Exit Code: 0
  > Shell e2e phase: Total: 34, Passed: 34, Failed: 0
  > Go e2e packages passed.
  > ```
- [x] Regression tests contain no silent-pass bailout patterns — **Phase:** implement. **Claim Source:** executed.
  > **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/knowledge_store_test.go tests/integration/knowledge_stats_test.go`
  > **Exit Code:** 0
  > Evidence:
  > ```text
  > ============================================================
  >   BUBBLES REGRESSION QUALITY GUARD
  >   Repo: <home>/smackerel
  >   Timestamp: 2026-04-30T02:42:26Z
  >   Bugfix mode: true
  > ============================================================
  > Scanning tests/e2e/knowledge_store_test.go
  > Adversarial signal detected in tests/e2e/knowledge_store_test.go
  > Scanning tests/integration/knowledge_stats_test.go
  > Adversarial signal detected in tests/integration/knowledge_stats_test.go
  > REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  > Files scanned: 2
  > Files with adversarial signals: 2
  > ```
- [x] Bug marked as Fixed in bug.md by the validation owner — **Phase:** validate. **Claim Source:** executed.
  > **Command:** `grep -nE "Fixed|Verified|Closed|\"status\": \"done\"|\*\*Status:\*\* Done" specs/025-knowledge-synthesis-layer/bugs/BUG-025-001-knowledge-stats-empty-store/bug.md specs/025-knowledge-synthesis-layer/bugs/BUG-025-001-knowledge-stats-empty-store/scopes.md specs/025-knowledge-synthesis-layer/bugs/BUG-025-001-knowledge-stats-empty-store/state.json`
  > **Exit Code:** 0
  > Evidence:
  > ```text
  > bug.md:16:- [x] Fixed (current HEAD contains the `GetStats` outer `COALESCE` fix; later `c6d2b26` broad E2E baseline was GREEN)
  > bug.md:17:- [x] Verified
  > bug.md:18:- [x] Closed
  > scopes.md:7:**Status:** Done
  > scopes.md:112:- [x] Bug marked as Fixed in bug.md by the validation owner — **Phase:** validate. **Claim Source:** executed.
  > state.json:7:  "status": "done",
  > state.json:51:    "status": "done",
  > ```
