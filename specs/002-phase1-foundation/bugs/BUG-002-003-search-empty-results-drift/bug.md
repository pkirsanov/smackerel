# Bug: BUG-002-003 Search empty results drift

## Summary
SCN-002-023 reports an unknown search query returning 5 results instead of the expected zero-result response, blocking broad E2E certification for the Phase 1 search contract.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Broad E2E certification blocked for a protected Phase 1 search scenario
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (red-stage and root-cause evidence captured in report.md)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Run the broad E2E command through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/test_search.sh` to reach scenario `SCN-002-023`.
3. Submit the unknown-query case expected to have no matching artifacts.
4. Observe the result count returned by the live search API.

## Expected Behavior
The protected empty-results scenario should return no matching artifacts and the honest message defined by spec 002: `I don't have anything about that yet`.

## Actual Behavior
The broad E2E failure reports `SCN-002-023`: expected `0` results, actual `5`.

## Environment
- Service: Go core search API, PostgreSQL/pgvector search store, NATS/ML search pipeline
- Version: Workspace state on 2026-04-28 during 039 broad E2E failure classification
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Broad E2E classification input:
tests/e2e/test_search.sh SCN-002-023 unknown query expected 0 results, actual 5.
```

## Root Cause
The vector search path returned nearest pgvector candidates without a raw similarity confidence gate. In a broad live E2E run with unrelated prior artifacts, the unknown-query case could therefore return low-confidence nearest neighbors instead of the honest empty result.

## Verification
The fix adds raw vector-confidence gating before annotation/domain boosts, preserves explicit filtered searches, and keeps text fallback behavior for low-confidence vector matches. Post-fix evidence records `SCN-002-023` passing in both `test_search.sh` and `test_search_empty.sh`; the later c6d2b26 broad E2E baseline records `./smackerel.sh test e2e` exit 0 with 34/34 shell E2E scripts passed and Go E2E packages passed.

## Related
- Feature: `specs/002-phase1-foundation/`
- Scenario: `SCN-002-023 Empty results handled gracefully`
- Scenario contract: `specs/002-phase1-foundation/scenario-manifest.json`
- E2E tests: `tests/e2e/test_search.sh`, `tests/e2e/test_search_empty.sh`
