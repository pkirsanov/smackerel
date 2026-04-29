# Bug Fix Design: BUG-002-003

## Root Cause Analysis

### Investigation Summary
The broad E2E suite reports `tests/e2e/test_search.sh` failing `SCN-002-023` because an unknown query expected zero results but received five. Existing spec 002 artifacts define `SCN-002-023` as the empty-results search contract and map it to `internal/api/search_test.go::TestSCN002023_EmptyResults_GracefulMessage` plus `tests/e2e/test_search_empty.sh`. No matching bug packet existed before this classification pass.

### Root Cause
Unproven at packetization time. The fix owner must capture the targeted red-stage response body and determine whether the five results come from fixture leakage, broad-suite ordering, search fallback overmatching, threshold drift, or test/scenario mismatch.

### Impact Analysis
- Affected components: search API, E2E search fixtures, broad-suite data isolation, possible vector/text fallback thresholds.
- Affected data: disposable E2E artifacts created during the broad suite.
- Affected users: search users may see unrelated results for queries with no true matches if the runtime behavior mirrors the E2E failure.

## Fix Design

### Solution Approach
Start with a targeted E2E reproduction that logs the unknown query, result count, result identifiers, and response message. Repair the first broken contract discovered: fixture isolation if leaked data is returned, threshold/ranking behavior if unrelated artifacts score as valid matches, or test alignment if `test_search.sh` is executing a scenario different from the canonical `SCN-002-023` contract. Preserve the strict zero-result assertion.

### Alternative Approaches Considered
1. Accepting five fuzzy results for the unknown query - rejected because it violates the spec 002 empty-results scenario.
2. Removing `SCN-002-023` from broad E2E - rejected because it weakens a protected search regression contract.
3. Replacing live E2E proof with unit-only evidence - rejected because the failure appears in the broad live stack and must be proven there.

## Regression Test Design
- Targeted E2E: the `SCN-002-023` unknown-query case passes with zero results and the honest message.
- Adversarial E2E: run the unknown-query case after searchable broad-suite artifacts exist and assert no leaked artifacts are returned.
- Broad E2E: `./smackerel.sh test e2e` no longer reports the search empty-results failure.
