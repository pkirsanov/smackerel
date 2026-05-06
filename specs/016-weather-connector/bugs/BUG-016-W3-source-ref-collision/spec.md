# Feature: [BUG-016-W3] Weather sync SourceRef uniqueness and panic-free sync tests

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Date:** 2026-05-04
> **Status:** Draft - ready for implementation owner

---

## Problem Statement

Weather sync artifacts use `SourceRef` values that are intended to identify a specific upstream observation for downstream artifact processing and deduplication. A previous improvement, `IMP-016-R4-001`, changed date-only `SourceRef` formatting to `time.RFC3339`. The 2026-05-03 broad regression run shows that this fix was incomplete: `time.RFC3339` has second-level granularity, so two `Sync()` calls for the same location inside the same second can still produce identical values such as `current-City-2026-05-03T21:16:37Z`.

The same unit run also exposed unsafe test synchronization in two weather tests. Their `httptest.Server` handlers close a shared channel on every request, but weather sync can make more than one request to the same handler. Closing an already closed channel panics, making the test harness itself unstable during the regression run.

## Outcome Contract

**Intent:** Weather connector sync output is safe for pipeline deduplication under rapid repeated syncs, and the regression tests remain stable when sync internals make multiple HTTP requests or retry a request.

**Success Signal:** Running the weather connector unit regression through `./smackerel.sh test unit` produces no duplicate same-location `SourceRef` values for same-second syncs and no `panic: close of closed channel` messages from the weather test server handlers.

**Failure Condition:** If two distinct `Sync()` calls for the same configured location can emit the same `SourceRef`, or if repeated handler invocation in the health/config-generation guard tests can panic by closing the same channel twice, this bug is not fixed.

## Requirements

| # | Requirement |
|---|-------------|
| R1 | A weather `SourceRef` for a current or forecast artifact MUST identify the sync event with more precision or entropy than second-level wall-clock time. |
| R2 | Two successful consecutive `Sync()` calls for the same location MUST emit distinct `SourceRef` values even when both calls occur within the same wall-clock second. |
| R3 | Regression tests MUST NOT rely on sleeping across a second boundary to prove SourceRef uniqueness. |
| R4 | `TestSync_SourceRefUniquePerSync` or its replacement MUST fail if the implementation uses only `time.RFC3339` second-level granularity. |
| R5 | Weather test HTTP handlers that signal `syncStarted` MUST be idempotent under repeated handler invocation. |
| R6 | Regression coverage MUST include a repeated-handler case that would panic if the synchronization channel could be closed twice. |
| R7 | Regression tests MUST contain no silent-pass bailout paths, including conditional returns that skip assertions after observing the failure condition. |
| R8 | Implementation MUST preserve existing weather artifact content types, metadata fields, source ID, and cursor behavior. |
| R9 | Runtime validation MUST use repo-standard commands through `./smackerel.sh`. |

## User Scenarios (Gherkin)

```gherkin
Feature: Weather sync artifacts remain unique and tests remain stable

  Scenario: SCN-BUG016W3-001 Same-second syncs produce unique SourceRefs
    Given a weather connector configured with one location named "City"
    And the upstream current weather endpoint returns valid current conditions
    When two successful Sync calls run for that location within the same wall-clock second
    Then the current weather artifacts from the two syncs have different SourceRef values
    And both SourceRef values still identify the location and artifact type

  Scenario: SCN-BUG016W3-002 Adversarial seconds-only SourceRef fails
    Given the SourceRef implementation only formats the sync time with second-level RFC3339 granularity
    When two successful Sync calls for the same location run inside the same second
    Then the regression test fails because the SourceRef values collide

  Scenario: SCN-BUG016W3-003 Health-sync test handler tolerates repeated requests
    Given the health transition test server handler is invoked more than once during a sync
    When the handler signals that sync has started
    Then the signal path does not panic on the second invocation
    And the test still asserts that Health reports syncing while the sync is blocked

  Scenario: SCN-BUG016W3-004 Config-generation guard handler tolerates repeated requests
    Given the config-generation guard test server handler is invoked more than once during a sync
    When Connect runs while Sync is blocked on HTTP
    Then the signal path does not panic on the second invocation
    And the test still asserts that the stale Sync does not clobber the Connect health state

  Scenario: SCN-BUG016W3-005 No silent-pass bailout in bug regressions
    Given the regression tests observe duplicate SourceRefs or repeated handler invocation
    When the failure condition occurs
    Then the tests fail by assertion or panic recovery evidence instead of returning early
```

## Acceptance Criteria

- [ ] SCN-BUG016W3-001 through SCN-BUG016W3-005 have executable regression coverage.
- [ ] The same-second SourceRef regression fails against the current broken behavior and passes after the fix.
- [ ] The repeated-handler regression fails or panics against the unsafe double-close behavior and passes after the test harness fix.
- [ ] `./smackerel.sh test unit` exits 0 with the weather connector package green.
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and `./smackerel.sh test stress` are run or honestly blocked with raw evidence by the owning validation agent.
- [ ] No generated files under `config/generated/` are edited by hand.

## Non-Goals

- Changing weather artifact schemas or content types.
- Changing NATS subjects, stream contracts, connector registration, or config SST behavior.
- Reclassifying this as a 039 recommendations bug.
- Hiding the unit failure by weakening `TestSync_SourceRefUniquePerSync` or by adding sleeps that avoid same-second execution.