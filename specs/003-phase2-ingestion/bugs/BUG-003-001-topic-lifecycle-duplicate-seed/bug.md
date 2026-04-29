# Bug: BUG-003-001 Topic lifecycle duplicate seed

## Summary
`test_topic_lifecycle.sh` fails on a duplicate `pricing` topic insert, blocking broad E2E certification for the Phase 2 topic lifecycle regression surface.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Broad E2E certification blocked by non-idempotent topic lifecycle fixture/state handling
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
2. Allow `tests/e2e/test_topic_lifecycle.sh` to execute.
3. The scenario attempts to create or seed a topic named `pricing`.
4. Observe the database unique constraint failure.

## Expected Behavior
The topic lifecycle E2E should be isolated or idempotent: a pre-existing `pricing` topic should be reused, namespaced, or cleaned through the disposable test fixture without violating `topics_name_key`.

## Actual Behavior
The broad E2E failure reports `duplicate key value violates unique constraint "topics_name_key"`, with `Key (name)=(pricing) already exists.`

## Environment
- Service: Go core topic lifecycle, PostgreSQL, E2E stack
- Version: Workspace state on 2026-04-28 during 039 broad E2E failure classification
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Broad E2E classification input:
test_topic_lifecycle.sh duplicate topic insert:
duplicate key value violates unique constraint "topics_name_key"
Key (name)=(pricing) already exists.
```

## Root Cause (initial analysis)
Root cause is unproven at packetization time. Candidate surfaces include non-unique E2E fixture names, missing cleanup, non-idempotent seed SQL/API calls, broad-suite ordering leaks, or the topic repository lacking an upsert/reuse path for test-owned topics.

## Related
- Feature: `specs/003-phase2-ingestion/`
- Scenario: `SCN-003-022 Topic momentum calculation`
- E2E test: `tests/e2e/test_topic_lifecycle.sh`
- Prior classification note: spec 038 report described this as pre-existing fixture isolation around `topics_name_key`
