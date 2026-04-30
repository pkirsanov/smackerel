# Scopes: BUG-039-001 Certification state drift

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Route validate-owned 039 state reconciliation

**Status:** Done
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
- [x] Validate owner confirms the artifact-lint status mismatch with raw evidence
  - **Phase:** validate
  - **Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine`
  - **Exit Code:** 1
  - **Claim Source:** interpreted
  - **Interpretation:** Existing parent 039 report evidence records the pre-fix artifact-lint failure signature for this bug: top-level `status` was `in_progress` while `certification.status` was `not_started`.
  ```text
  ❌ Top-level status 'in_progress' does not match certification.status 'not_started'
  Artifact lint FAILED with 1 issue(s).
  ```
- [x] Validate owner reconciles certification-owned status fields without certifying 039 Scope 1 done
  - **Phase:** validate
  - **Command:** `cat specs/039-recommendations-engine/state.json`
  - **Exit Code:** 0
  - **Claim Source:** executed
  ```json
  "status": "in_progress",
  "certification": {
    "status": "in_progress",
    "completedScopes": [],
    "certifiedCompletedPhases": [],
    "scopeProgress": [
      {
        "scope": 1,
        "name": "scope-01-foundation-schema",
        "status": "In Progress",
        "certifiedAt": null
      }
    ]
  }
  ```
- [x] Artifact lint for `specs/039-recommendations-engine` passes status coherence
  - **Phase:** validate
  - **Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine`
  - **Exit Code:** 0
  - **Claim Source:** executed
  ```text
  ✅ Detected state.json status: in_progress
  ✅ Detected state.json workflowMode: full-delivery
  ✅ state.json v3 has required field: status
  ✅ state.json v3 has required field: execution
  ✅ state.json v3 has required field: certification
  ✅ state.json v3 has required field: policySnapshot
  ✅ Top-level status matches certification.status
  Artifact lint PASSED.
  ```
- [x] 039 Scope 1 remains in progress
  - **Phase:** validate
  - **Command:** `cat specs/039-recommendations-engine/state.json`
  - **Exit Code:** 0
  - **Claim Source:** executed
  ```json
  "certification": {
    "completedScopes": [],
    "scopeProgress": [
      {
        "scope": 1,
        "name": "scope-01-foundation-schema",
        "status": "In Progress",
        "certifiedAt": null
      }
    ]
  }
  ```
- [x] Broad E2E blockers remain represented by owning bug packets
  - **Phase:** validate
  - **Command:** `ls specs/039-recommendations-engine/bugs`
  - **Exit Code:** 0
  - **Claim Source:** executed
  ```text
  BUG-039-001-certification-state-drift
  BUG-039-002-operator-status-provider-block
  ```
- [x] Bug marked as Fixed in bug.md by the validation owner
  - **Phase:** validate
  - **Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine/bugs/BUG-039-001-certification-state-drift`
  - **Exit Code:** 0
  - **Claim Source:** executed
  ```text
  ✅ Detected state.json status: done
  ✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
  ✅ Workflow mode 'bugfix-fastlane' allows status 'done'
  ✅ All 1 scope(s) in scopes.md are marked Done
  ✅ Required specialist phase 'implement' found in execution/certification phase records
  ✅ Required specialist phase 'test' found in execution/certification phase records
  ✅ Required specialist phase 'validate' found in execution/certification phase records
  ✅ Required specialist phase 'audit' found in execution/certification phase records
  Artifact lint PASSED.
  ```
