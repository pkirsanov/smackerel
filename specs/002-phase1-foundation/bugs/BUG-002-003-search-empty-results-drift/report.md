# Execution Report: BUG-002-003 Search empty results drift

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore search empty-results live-stack contract - 2026-04-28

### Summary
- Bug packet created by `bubbles.bug` during 039 broad E2E failure classification.
- No production code, test code, parent spec 002 artifacts, or certification-owned fields were modified by this packetization pass.
- The packet routes implementation to the Phase 1 search owner because the failing behavior is `SCN-002-023`.

### Completion Statement
Bug packetization is complete for classification. The bug remains `in_progress`; fix, test, and validate evidence are intentionally absent from this triage packet.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the broad E2E failure signature. Workspace search confirmed `SCN-002-023` is an active spec 002 search scenario with linked E2E coverage. Runtime reproduction and red-stage output belong to the fix/test owner.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** No terminal command was executed in this packetization pass. The owner must capture current targeted red output before changing source or test code.

```text
Observed from workflow context:
test_search.sh SCN-002-023 unknown query expected 0 results, actual 5.

Source inspection notes:
- specs/002-phase1-foundation/scopes.md defines SCN-002-023 as "Empty results handled gracefully".
- specs/002-phase1-foundation/scenario-manifest.json links SCN-002-023 to internal/api/search_test.go and tests/e2e/test_search_empty.sh.
- The broad failure mentions tests/e2e/test_search.sh, so the owner must confirm which script is executing the protected empty-results scenario.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces depend on confirmed root cause:
- `tests/e2e/test_search.sh` and `tests/e2e/test_search_empty.sh` for fixture ownership and assertions
- `internal/api/search.go` and search-related tests only if targeted evidence proves runtime search behavior is returning unrelated matches

Protected surfaces for this bug:
- Recommendation engine feature 039 artifacts and certification fields
- Domain extraction and digest delivery code paths unless targeted evidence proves shared search-state interaction

## Implementation Evidence - 2026-04-28

### Root Cause
**Phase:** implement
**Command:** source inspection of `internal/api/search.go`
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The vector search path returned nearest pgvector candidates without a raw similarity confidence gate. In a shared live stack with embeddings from earlier scenarios, an unrelated unknown query could therefore return the closest artifacts instead of an honest empty result. The fix applies the confidence gate before annotation/domain boosts can promote weak semantic matches, and keeps explicit filtered searches allowed.

### Red Proof
**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh test unit`
**Exit Code:** 1
**Claim Source:** executed
**Interpretation:** New SCN-002-023 unit regressions failed before implementation because `vectorSearchConfidencePasses` and `minVectorSearchSimilarity` did not exist. A pre-edit broad E2E reproduction attempt did not reproduce the routed five-result count locally; the shell search scripts passed while unrelated E2E failures remained. The workflow-supplied failure signature remains the routed live-stack drift evidence.

### Implementation Changes
**Phase:** implement
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** `internal/api/search.go` now rejects low-confidence unfiltered vector matches using raw similarity, falls back to text search when vector search yields no confident matches, and preserves explicit-filter searches. `internal/api/search_test.go` now covers low-confidence rejection, raw-similarity-before-boost behavior, and explicit-filter pass-through.

### Unit And Check Evidence
**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh test unit`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** Go unit tests passed, including the new `internal/api` regressions; Python ML unit tests also passed with 348 passed and 2 warnings.

**Phase:** implement
**Command:** `timeout 120 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** Config SST, env drift guard, and scenario-lint checks passed.

### Live E2E Evidence
**Phase:** implement
**Command:** `timeout 1200 ./smackerel.sh --env test build`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** Fresh test images were built before live-stack verification. The core image was rebuilt as `smackerel-test-smackerel-core` with image digest prefix `sha256:afb0939671f34408ce3d5ac912c2d1e1b56582667612c`.

**Phase:** implement
**Command:** `timeout 3600 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed
**Interpretation:** The broad E2E run reached the shared shell search block after an initial non-Smackerel Chromium listener temporarily held test port `127.0.0.1:45001`. The protected search scripts passed post-fix: `test_search.sh`, `test_search_filters.sh`, and `test_search_empty.sh`. `test_search.sh` reported `PASS: SCN-002-023: Empty results handled gracefully`, returned the honest message `I don't have anything about that yet`, and preserved known-query behavior with one result for `pricing strategy`. `test_search_empty.sh` also reported `PASS: SCN-002-023: Empty results return graceful message: I don't have anything about that yet`. The broad suite still failed unrelated checks: shell failures in `test_persistence.sh`, `test_postgres_readiness_gate.sh`, `test_digest_telegram.sh`, and `test_topic_lifecycle.sh`; Go E2E failures in `TestE2E_DomainExtraction` and `TestOperatorStatus_RecommendationProvidersEmptyByDefault`; and `go-e2e (exit=1)`. Therefore the broader E2E suite is not claimed green by this implement pass.

### Open Validation Boundary
**Phase:** implement
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** Search-owned implementation and targeted regression evidence are complete, but validation-owned certification remains unset. The bug cannot be certified done by `bubbles.implement`, and the broader E2E suite still requires owner routing for non-search failures before the broad-suite DoD can be checked.
