# Bug Fix Design: BUG-039-001

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 workflow context reports 039 artifact lint failing because top-level `state.json.status` is `in_progress` while `certification.status` remains `not_started`. Source inspection of `specs/039-recommendations-engine/state.json` confirmed that state shape.

### Root Cause
Execution metadata advanced into active full-delivery work while validate-owned certification metadata did not mirror the active in-progress status. This is a control-plane reconciliation issue, not a production-code defect.

### Impact Analysis
- Affected components: `specs/039-recommendations-engine/state.json`, artifact lint status coherence checks.
- Affected data: control-plane metadata only.
- Affected users: workflow agents trying to advance 039 Scope 1 through valid gates.

## Fix Design

### Solution Approach
Route to `bubbles.validate` to reconcile certification-owned status fields according to the control-plane model. The reconciliation must not mark Scope 1 done, must not add completed scopes, and must not certify phases that have not passed evidence gates.

### Alternative Approaches Considered
1. `bubbles.bug` directly edits certification.status. Rejected because certification fields are validate-owned.
2. Mark 039 done to satisfy lint. Rejected because Scope 1 remains active and broad E2E blockers are unresolved.

## Affected Files
- `specs/039-recommendations-engine/state.json` is the expected validate-owned reconciliation target.
- This bug packet documents the route and must not modify 039 `spec.md` or `design.md`.

## Regression Test Design
- Artifact validation: artifact lint for `specs/039-recommendations-engine` passes status coherence.
- State assertion: Scope 1 remains in progress and no completion certification is fabricated.
- Route assertion: broad E2E blockers remain represented by their owning bug packets.

## Ownership
- Owning feature/spec: `specs/039-recommendations-engine`
- Required owner: `bubbles.validate`
- Current scope: `scope-01-foundation-schema`
