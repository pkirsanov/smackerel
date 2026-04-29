# Scopes: BUG-039-001 Certification state drift

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Route validate-owned 039 state reconciliation

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-039-001 reconcile certification state without premature completion
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

### Implementation Plan
1. `bubbles.validate` confirms the 039 artifact-lint mismatch with current command output.
2. `bubbles.validate` updates only certification-owned state fields needed for active in-progress coherence.
3. Preserve Scope 1 as in progress and leave `completedScopes` empty until completion gates pass.
4. Re-run artifact lint for 039 and record evidence.
5. Keep broad E2E/product regressions tracked in their owning bug packets.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-039-001-01 | Artifact lint status coherence | artifact | `specs/039-recommendations-engine` | Artifact lint no longer fails on status/certification mismatch | BUG-039-001-SCN-001 |
| T-BUG-039-001-02 | Scope remains active | artifact | `specs/039-recommendations-engine/state.json` | Scope 1 remains in progress; no completion certification is added | BUG-039-001-SCN-001 |
| T-BUG-039-001-03 | Routed blockers preserved | artifact | bug packets under owning specs | Broad E2E blockers remain routed outside 039 completion state | BUG-039-001-SCN-002 |

### Definition of Done
- [ ] Validate owner confirms the artifact-lint status mismatch with raw evidence
- [ ] Validate owner reconciles certification-owned status fields without certifying 039 Scope 1 done
- [ ] Artifact lint for `specs/039-recommendations-engine` passes status coherence
- [ ] 039 Scope 1 remains in progress
- [ ] Broad E2E blockers remain represented by owning bug packets
- [ ] Bug marked as Fixed in bug.md by the validation owner
