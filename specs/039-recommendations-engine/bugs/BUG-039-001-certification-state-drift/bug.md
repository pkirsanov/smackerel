# Bug: BUG-039-001 Certification state drift

## Summary
039 artifact lint fails because top-level `state.json` status is `in_progress` while `certification.status` is `not_started`; certification state is validate-owned and must be reconciled without marking 039 Scope 1 done.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - 039 delivery workflow gate blocked by control-plane state drift
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (artifact-lint mismatch evidence exists in the parent 039 report)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Inspect `specs/039-recommendations-engine/state.json` during Scope 1 full-delivery.
2. Observe top-level `status` is `in_progress`.
3. Observe `certification.status` is `not_started`.
4. Run artifact lint for 039 through the Bubbles validation command.
5. Artifact lint reports status/certification mismatch.

## Expected Behavior
During active 039 delivery, top-level workflow status and validate-owned certification status should be coherent without promoting Scope 1 to done.

## Actual Behavior
Top-level status is `in_progress`, but validate-owned certification status remains `not_started`, causing artifact lint to fail.

## Environment
- Service: Bubbles control-plane artifacts for feature 039
- Version: Workspace state on 2026-04-27 during 039 full-delivery Scope 1
- Platform: Linux

## Error Output
```text
Workflow context from bubbles.stabilize: 039 artifact lint fails because top-level state.json status is in_progress while certification.status is not_started; this is validate-owned state reconciliation.
```

## Root Cause (initial analysis)
The feature entered active implementation state while certification metadata remained at initial status. Because certification fields are owned by `bubbles.validate`, `bubbles.bug` only documents and routes this finding; it does not edit 039 certification-owned fields.

## Resolution
The parent 039 control-plane state now has top-level `status: in_progress` and `certification.status: in_progress`, so artifact lint no longer reports the original status/certification mismatch. Current parent certification still has `completedScopes: []` and Scope 1 certification progress remains `In Progress`, so this fix does not certify feature 039 as done.

Verification on 2026-04-30:
- `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine` exited 0 and reported `Top-level status matches certification.status`.
- `cat specs/039-recommendations-engine/state.json` showed `certification.completedScopes` as an empty array and `certification.scopeProgress[0].status` as `In Progress`.
- `ls specs/039-recommendations-engine/bugs` showed `BUG-039-002-operator-status-provider-block` remains represented separately from this certification-state packet.

## Related
- Feature: `specs/039-recommendations-engine/`
- Active scope: `scope-01-foundation-schema`
- Required owner: `bubbles.validate`
