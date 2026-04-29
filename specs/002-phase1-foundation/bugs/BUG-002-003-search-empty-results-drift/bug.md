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
- [ ] Confirmed (targeted red-stage output must be captured by the fix owner)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

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

## Root Cause (initial analysis)
Root cause is unproven at packetization time. Candidate surfaces include broad-suite fixture isolation, unknown-query fixture design, search thresholding in text or vector fallback paths, stale artifacts leaking between E2E scenarios, or a mismatch between `test_search.sh` and the canonical `SCN-002-023` empty-results contract in spec 002.

## Related
- Feature: `specs/002-phase1-foundation/`
- Scenario: `SCN-002-023 Empty results handled gracefully`
- Scenario contract: `specs/002-phase1-foundation/scenario-manifest.json`
- E2E tests: `tests/e2e/test_search.sh`, `tests/e2e/test_search_empty.sh`
