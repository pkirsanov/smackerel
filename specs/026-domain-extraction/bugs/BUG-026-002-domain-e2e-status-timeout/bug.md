# Bug: BUG-026-002 Domain E2E status timeout

## Summary
The domain extraction E2E times out with empty processing/domain status after capturing recipe-like content, leaving the domain extraction live-stack scenario red in the broad E2E suite.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Domain extraction live-stack certification blocked
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (targeted red-stage output captured: focused domain E2E timed out with `processing=processed domain=`)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the full E2E suite through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` to execute.
3. The test posts recipe-like text to `/api/capture`.
4. The test polls `/api/artifact/{artifact_id}` for `processing_status`, `domain_extraction_status`, and `domain_data` for up to 90 seconds.
5. The test fails when processing/domain status remains empty or never reaches the expected completed state.

## Expected Behavior
Recipe-like captured content should be processed, domain extraction should complete, `domain_data` should include structured recipe fields, and ingredient search should return the captured artifact.

## Actual Behavior
The E2E scenario times out with empty processing/domain status, so the domain data and search assertions never become reliable live-stack proof.

## Environment
- Service: Go core, domain extraction pipeline, Python ML sidecar, NATS, PostgreSQL
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Workflow context from bubbles.stabilize: Domain extraction e2e times out with empty processing/domain status.
Relevant test path: tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction.

Targeted red-stage reproduction:
timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction
Captured artifact 01KQA420VP2JP3ZT5KZF5WZZMN.
Poll output reached processing=processed but domain_extraction_status stayed empty.
Final failure: domain extraction not completed within 90s timeout -- last domain_status=.
```

## Root Cause
The failure had four cooperating causes along the live-stack path:

1. `/api/artifact/{id}` did not expose persisted `domain_extraction_status` or `domain_data`, so the E2E poller could not observe completed extraction even when data existed.
2. The plain-text recipe fixture entered the pipeline as broad `generic`/`note` content under degraded ML fallback, while the recipe prompt contract matched `recipe` content or recipe-site URLs.
3. The Go processor and domain dispatch path trusted transient broad ML result types instead of preserving the stored domain-specific artifact type for contract matching.
4. The decisive container wiring bug: `smackerel-core` mounted `./config/prompt_contracts` at `/app/prompt_contracts` but did not override `PROMPT_CONTRACTS_DIR` to that in-container path. The core container therefore could not load domain prompt contracts, so dispatch never wrote `domain_extraction_status=pending` and never published `domain.extract`.

The implemented fix exposes domain fields, preserves recipe typing through degraded processing, provides a gated recipe-domain fallback when the LLM is unavailable, matches domain contracts from the stored artifact type, and aligns the core container prompt-contract path with the mounted path.

## Related
- Feature: `specs/026-domain-extraction/`
- E2E test: `tests/e2e/domain_e2e_test.go`
- Existing related but non-covering bug: `BUG-026-001-dod-scenario-fidelity-gap`
