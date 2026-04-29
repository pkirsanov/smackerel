# Feature: BUG-003-001 Topic lifecycle duplicate seed

## Problem Statement
Spec 003 protects topic lifecycle behavior through `tests/e2e/test_topic_lifecycle.sh`. Broad E2E now fails before lifecycle behavior can be proven because the fixture inserts a duplicate `pricing` topic into the unique `topics.name` surface.

## Outcome Contract
**Intent:** Topic lifecycle E2E fixtures are repeatable and isolated, so lifecycle behavior is tested instead of failing on duplicate seed data.
**Success Signal:** `test_topic_lifecycle.sh` can run in the broad suite even when a `pricing` topic already exists, and it reaches the lifecycle assertions for the owned fixture topic.
**Hard Constraints:** The regression must exercise the real database and topic lifecycle path. It must not remove the unique topic-name constraint or silently ignore setup failures without proving lifecycle behavior.
**Failure Condition:** The scenario fails with `topics_name_key`, or passes without verifying topic lifecycle behavior after fixture setup.

## Goals
- Preserve topic name uniqueness as a production invariant.
- Make the E2E fixture idempotent, uniquely owned, or isolated.
- Capture red-stage evidence showing the existing `pricing` row and the failing insert path.

## Non-Goals
- Removing the `topics_name_key` constraint.
- Disabling the topic lifecycle E2E.
- Changing digest, search, recommendation, or domain extraction behavior.

## Requirements
- Test-owned topic fixtures must use unique names or idempotent setup.
- The regression must prove lifecycle behavior still executes after setup.
- The adversarial case must run with a pre-existing `pricing` topic and must not collide.
- Cleanup or fixture ownership must preserve disposable-stack isolation.

## User Scenarios (Gherkin)

```gherkin
Scenario: Topic lifecycle E2E setup is idempotent
  Given the disposable live stack already contains a topic named pricing
  When the topic lifecycle E2E prepares its fixture data
  Then fixture setup does not violate the unique topic-name constraint
  And the lifecycle assertions execute against a test-owned topic

Scenario: Duplicate topic regression fails loudly before lifecycle proof
  Given fixture setup attempts to insert an already-existing topic name without ownership or upsert semantics
  When the topic lifecycle E2E runs
  Then the test fails with diagnostics instead of treating setup as lifecycle success
```

## Acceptance Criteria
- Targeted pre-fix evidence records the duplicate insert path and existing `pricing` row context.
- The fixed `test_topic_lifecycle.sh` passes when the topic name already exists before setup.
- Broad `./smackerel.sh test e2e` no longer reports the `topics_name_key` collision once all routed blockers are fixed.
