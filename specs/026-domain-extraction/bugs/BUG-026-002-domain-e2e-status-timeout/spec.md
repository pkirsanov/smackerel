# Feature: BUG-026-002 Domain E2E status timeout

## Problem Statement
The domain extraction feature has a live-stack E2E scenario that should prove recipe content becomes structured domain data. The current broad E2E suite reports empty processing/domain status until timeout, so domain extraction cannot be treated as a healthy regression surface for dependent work.

## Outcome Contract
**Intent:** Captured recipe-like content completes both processing and domain extraction in the live stack.
**Success Signal:** `TestE2E_DomainExtraction` observes processed status, completed domain extraction status, non-empty domain data with recipe structure, and search returns the artifact.
**Hard Constraints:** The test must exercise real capture, NATS, ML sidecar, persistence, artifact detail, and search paths with no request interception or silent skip.
**Failure Condition:** The test passes without completed domain status and non-empty domain data, or still times out with empty status after the fix.

## Goals
- Capture targeted red-stage evidence for the empty processing/domain status.
- Identify whether the failure is status persistence, domain publish/consume, ML result handling, or detail response mapping.
- Restore live-stack domain extraction proof for recipe-like content.

## Non-Goals
- Changing 039 recommendation engine behavior.
- Weakening the E2E assertion to accept missing domain data.
- Replacing the live E2E with a unit-only proof.

## Requirements
- The regression must assert completed processing and completed domain extraction status.
- The regression must assert structured domain data contains recipe-relevant fields.
- The regression must fail if processing/domain status is empty or missing.
- The regression must use unique disposable-stack fixture content.

## User Scenarios (Gherkin)

```gherkin
Scenario: Recipe capture completes domain extraction in the live stack
  Given the disposable live stack is healthy
  When an E2E test captures recipe-like text
  Then artifact detail eventually reports processing_status as processed or completed
  And domain_extraction_status as completed
  And domain_data contains recipe structure

Scenario: Domain extraction regression fails on empty statuses
  Given artifact detail returns empty processing or domain extraction status
  When the E2E polling loop evaluates the captured artifact
  Then the test fails with diagnostic output instead of treating the extraction as complete
```

## Acceptance Criteria
- Targeted pre-fix failure output includes last observed processing and domain status values.
- The fixed flow passes `TestE2E_DomainExtraction` against the live stack.
- Broad `./smackerel.sh test e2e` no longer reports this domain status timeout once all routed blockers are addressed.

## Current Evidence Status

- Targeted pre-fix failure captured: focused domain E2E reached `processing=processed` with empty `domain_extraction_status` until timeout.
- Focused post-fix proof captured: `TestE2E_DomainExtraction` passed and observed `domain=completed` with recipe `domain_data`.
- Broad post-fix proof captured: the domain extraction test passed inside `./smackerel.sh --env test test e2e`; the command still exited 1 on an unrelated operator status E2E failure.
