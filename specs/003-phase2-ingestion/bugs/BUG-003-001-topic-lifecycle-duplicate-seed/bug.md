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
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

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

## Root Cause
The duplicate seed was caused by shared-stack E2E fixture ownership drift. `test_graph_entities.sh` seeded a topic named `pricing` with id `e2e-topic-pricing`; the pre-fix topic lifecycle fixture then attempted to insert another `pricing` row using a different id and `ON CONFLICT (id) DO NOTHING`, which did not protect the unique `topics.name` constraint.

## Resolution
`tests/e2e/test_topic_lifecycle.sh` now preserves the pre-existing `pricing` row as the adversarial fixture, seeds lifecycle-owned topics with unique `topic-lifecycle-*` names, and verifies lifecycle assertions against the owned fixture topic instead of the shared `pricing` topic.

## Verification
- Focused pre-fix evidence reproduced `topics_name_key` for `pricing` after `test_graph_entities.sh` seeded the shared topic.
- Focused post-fix topic lifecycle E2E passed twice on the same shared stack with an existing `pricing` topic.
- Broad implementation-stage shell E2E reached 34/34 passing and `test_topic_lifecycle.sh` passed; remaining broad failures were unrelated Go E2E failures.
- The later `c6d2b26` baseline recorded full `./smackerel.sh test e2e` pass with shell E2E 34/34 and Go E2E packages passed, so the broad suite no longer reports the `topics_name_key` collision.

## Related
- Feature: `specs/003-phase2-ingestion/`
- Scenario: `SCN-003-022 Topic momentum calculation`
- E2E test: `tests/e2e/test_topic_lifecycle.sh`
- Prior classification note: spec 038 report described this as pre-existing fixture isolation around `topics_name_key`
