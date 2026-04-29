# Feature: BUG-039-001 Certification state drift

## Problem Statement
Feature 039 is active in full-delivery Scope 1, but control-plane certification metadata is not coherent with top-level execution state. This blocks artifact lint and prevents the workflow from cleanly routing real product/e2e blockers.

## Outcome Contract
**Intent:** 039 control-plane state reflects active in-progress delivery without certifying Scope 1 as done.
**Success Signal:** Artifact lint for `specs/039-recommendations-engine` no longer fails on top-level status versus certification status mismatch, and Scope 1 remains in progress until validate certifies completion evidence.
**Hard Constraints:** Only `bubbles.validate` may modify certification-owned fields; this packet must not edit 039 `spec.md`, `design.md`, or certification fields.
**Failure Condition:** 039 is promoted to done prematurely, or artifact lint still reports status/certification mismatch.

## Goals
- Route validation-owned state reconciliation to `bubbles.validate`.
- Preserve 039 Scope 1 as in progress.
- Keep product/e2e blockers tracked separately from certification-state reconciliation.

## Non-Goals
- Implementing 039 recommendation functionality.
- Editing 039 `spec.md` or `design.md`.
- Certifying Scope 1 completion.

## Requirements
- Validate-owned reconciliation must preserve `status: in_progress` until completion gates pass.
- Certification status must no longer contradict the active workflow status.
- Artifact lint must be re-run by the validate owner after reconciliation.
- The bug packet must remain separate from production-code blockers.

## User Scenarios (Gherkin)

```gherkin
Scenario: Validate reconciles active 039 state without certifying completion
  Given 039 Scope 1 is in progress
  And top-level state status is in_progress
  When validate reconciles certification-owned status fields
  Then certification status reflects active in-progress work
  And Scope 1 is not marked done

Scenario: Artifact lint passes the 039 status coherence check
  Given validate has reconciled certification-owned status fields
  When artifact lint runs for specs/039-recommendations-engine
  Then status coherence passes
  And unrelated broad E2E blockers remain routed to their owning bug packets
```

## Acceptance Criteria
- Validate-owned status reconciliation is recorded in 039 evidence.
- 039 Scope 1 remains in progress after reconciliation.
- Artifact lint for 039 no longer fails on status/certification mismatch.
