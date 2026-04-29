# Feature: BUG-031-003 Capture process/search E2E processing timeout

## Problem Statement
The live-stack capture pipeline scenario is supposed to prove that a captured text artifact is processed and searchable. The current broad E2E suite reports that the artifact never reaches the expected processed status within the test window, so 039 cannot rely on broad E2E as a clean regression gate.

## Outcome Contract
**Intent:** The live-stack E2E pipeline reliably advances a captured text artifact to a processed state and returns it from search.
**Success Signal:** `TestE2E_CaptureProcessSearch` observes `processing_status` as `processed` or `completed`, then finds the captured artifact in `/api/search` results.
**Hard Constraints:** The test must use the real disposable stack, real NATS pipeline, and real API responses; no internal request interception or silent skip is allowed.
**Failure Condition:** The test passes without observing processed status, or still times out waiting for processing status after the fix.

## Goals
- Reproduce the timeout with targeted red-stage evidence.
- Identify whether the failure is pipeline completion, status persistence, detail response mapping, or E2E harness readiness.
- Restore scenario-specific live-stack proof for capture -> process -> search.

## Non-Goals
- Changing recommendation engine scope 039.
- Weakening the E2E assertion to accept missing processing status.
- Reclassifying this live-stack scenario as a unit or mocked test.

## Requirements
- The regression must assert the exact user-visible/API-visible pipeline behavior from SCN-LST-003.
- The regression must fail if processing status stays empty, pending forever, or absent from artifact detail.
- The regression must search only after processing status is observed as complete.
- Test data must remain isolated to the disposable E2E stack.

## User Scenarios (Gherkin)

```gherkin
Scenario: Capture process search pipeline reaches processed status
  Given the disposable live stack is healthy
  When an E2E test captures a unique text artifact
  Then artifact detail eventually reports processing_status as processed or completed
  And searching for content from that artifact returns the artifact

Scenario: Capture process search regression fails on empty processing status
  Given the artifact detail response omits or empties processing_status
  When the E2E polling loop evaluates the captured artifact
  Then the test fails with diagnostic output instead of treating the artifact as processed
```

## Acceptance Criteria
- Targeted pre-fix failure output identifies the last observed artifact detail status.
- The fixed flow passes `TestE2E_CaptureProcessSearch` against the live stack.
- Broad `./smackerel.sh test e2e` no longer reports this capture processing timeout once all routed blockers are addressed.
