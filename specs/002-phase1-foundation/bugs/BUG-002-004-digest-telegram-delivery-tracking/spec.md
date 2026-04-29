# Feature: BUG-002-004 Digest Telegram delivery tracking

## Problem Statement
Spec 002 protects the Phase 1 daily digest contract that generated digests are delivered through Telegram when configured. Broad E2E now reports the Telegram digest delivery proof as untracked, so `SCN-002-032` cannot be certified green.

## Outcome Contract
**Intent:** A generated digest sent through Telegram leaves an observable delivery tracking record in the live stack.
**Success Signal:** `tests/e2e/test_digest_telegram.sh` observes the generated digest and its Telegram delivery tracking signal for the configured chat/channel.
**Hard Constraints:** The regression must exercise real digest generation, delivery routing, persistence/tracking, and Telegram delivery abstraction used by the live stack. It must not replace the behavior with canned responses or skip delivery assertions.
**Failure Condition:** The scenario passes without proving delivery tracking, or broad E2E still reports `Digest delivery not tracked`.

## Goals
- Preserve `SCN-002-032` as the protected digest-via-Telegram contract.
- Capture red-stage evidence identifying which delivery tracking signal is missing.
- Restore strict live-stack proof that Telegram delivery is tracked.

## Non-Goals
- Removing Telegram delivery from the digest contract.
- Treating digest generation alone as sufficient proof of delivery.
- Changing unrelated search, topic lifecycle, domain extraction, or recommendation behavior.

## Requirements
- The E2E fixture must generate a uniquely identifiable digest for the test run.
- Telegram delivery must be observable through the same tracking surface used by production/operator diagnostics.
- The regression must fail if generation succeeds but delivery tracking is absent.
- The fix must preserve `/api/digest` retrieval and quiet-day digest behavior.

## User Scenarios (Gherkin)

```gherkin
Scenario: Generated digest is tracked after Telegram delivery
  Given Telegram digest delivery is configured for the disposable live stack
  When a digest is generated for the E2E fixture
  Then the digest is delivered to the configured chat
  And the delivery is tracked with the generated digest identity

Scenario: Digest delivery regression fails when tracking is absent
  Given a digest exists but no Telegram delivery tracking record exists
  When the Telegram digest E2E verifies delivery
  Then the test fails with diagnostics instead of accepting generation-only proof
```

## Acceptance Criteria
- Targeted pre-fix failure output identifies the missing delivery tracking surface.
- The fixed `test_digest_telegram.sh` scenario passes for `SCN-002-032` against the live stack.
- Broad `./smackerel.sh test e2e` no longer reports this digest delivery tracking failure once all routed blockers are fixed.
