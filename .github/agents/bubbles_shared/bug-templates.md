# Bug Artifact Templates

Use these templates when creating bug artifacts.

## bug.md Template

```markdown
# Bug: [BUG-NNN] Short Description

## Summary
One-line summary of the bug.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [ ] Reported
- [ ] Confirmed (reproduced)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Step 1
2. Step 2
3. ...

## Expected Behavior
What should happen.

## Actual Behavior
What actually happens.

## Environment
- Service: [service-name]
- Version: [commit/version]
- Platform: [OS/browser/device]

## Error Output
```
[stack trace or error message]
```

## Root Cause (filled after analysis)
[Description of root cause]

## Related
- Feature: `specs/[NNN-feature-name]/`
- Related bugs: [links]
- Related PRs: [links]

## Deferred Reason (if mode: document)
[Why this bug is being deferred, priority, when to fix]
```

## design.md Template (Bug Fix)

```markdown
# Bug Fix Design: [BUG-NNN]

## Root Cause Analysis

### Investigation Summary
[What was investigated]

### Root Cause
[Precise technical root cause]

### Impact Analysis
- Affected components: [list]
- Affected data: [if any]
- Affected users: [scope]

## Fix Design

### Solution Approach
[Chosen solution and why]

### Alternative Approaches Considered
1. [Alternative 1] - Why rejected
2. [Alternative 2] - Why rejected
```


## Additional Bug Artifact Expectations

Bug folders do not stop at `bug.md` and `design.md`. Before any bug can be promoted, the bug packet must also include the standard execution artifacts from [feature-templates.md](feature-templates.md):

- `scopes.md` — bug scope with Gherkin, Test Plan, and DoD
- `report.md` — evidence with raw terminal output
- `uservalidation.md` — checked-by-default validation checklist
- `scenario-manifest.json` — required before any completion claim when the bug scope defines Gherkin or changes observable behavior
- `state.json` — version 3 control-plane state with `workflowMode`, `execution`, `certification`, and `policySnapshot`

Minimum bug-state expectations:

- `workflowMode: "bugfix-fastlane"`
- `status: "in_progress"` until validate-owned certification promotes it
- `certification.status: "in_progress"` while work is active
- `certification.completedScopes`, `certification.certifiedCompletedPhases`, `certification.scopeProgress`, and `certification.lockdownState` present
- `policySnapshot` present with grill, TDD, auto-commit, lockdown, regression, and validation provenance
- `transitionRequests` and `reworkQueue` present, even when empty

Minimum regression planning expectations for bug scopes:

- Test Plan includes at least one row whose label literally contains `Regression E2E`
- DoD includes the exact checkbox items below so the transition guard can prove the regression contract mechanically:
  - `- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior`
  - `- [ ] Broader E2E regression suite passes`
