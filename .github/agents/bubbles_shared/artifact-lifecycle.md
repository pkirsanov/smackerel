# Artifact Lifecycle Governance

Use this file for work classification, required artifacts, scope structure, and lifecycle expectations for execution packets.

## Work Classification Gate

All work must be classified as one of:

- feature work under `specs/NNN-feature-name/`
- feature-bound bug work under `specs/.../bugs/BUG-NNN-description/`
- cross-cutting bug work under `specs/_bugs/BUG-NNN-description/`
- ops work under `specs/_ops/OPS-NNN-description/`

Ad-hoc implementation outside that structure is not valid completion work.

## Required Feature Artifacts

Feature directories require:

- `spec.md`
- `design.md`
- `scopes.md` or `scopes/_index.md` plus per-scope files
- `report.md` or per-scope reports
- `uservalidation.md`
- `state.json`

## Required Bug Artifacts

Bug directories require:

- `bug.md`
- `spec.md`
- `design.md`
- `scopes.md`
- `report.md`
- `state.json`

## Required Ops Artifacts

Ops directories require:

- `objective.md`
- `design.md`
- `scopes.md`
- `report.md`
- `runbook.md`
- `state.json`

## User Validation Gate

Unchecked items in `uservalidation.md` represent user-reported regressions and block unrelated forward progress until addressed or explicitly reclassified by the owning workflow.

## Scope Structure

Each scope must contain:

- status
- dependency declaration when applicable
- Gherkin scenarios
- implementation plan
- Test Plan
- DoD checkboxes

When UI behavior changes, add a UI scenario matrix.

When ops behavior changes, add publication targets for the managed docs that must be updated before closeout.

## Tiered DoD Expectations

Every scope must include checks that prove:

- implementation behavior is complete
- scenario-specific tests pass
- regression coverage exists for changed behavior
- grouped build quality gate passes

Project-specific additions may extend this, but cannot weaken it.

## Scope Isolation And Pickup

Implement and iterate agents work from the next eligible scope only:

- respect declared dependencies
- do not start later scopes before earlier required scopes are done
- when six or more scopes exist, prefer per-scope directory mode

## Bug Awareness

Before starting new feature work, review unresolved bug folders under the same feature area and surface those to the user or orchestrator instead of silently ignoring them.

## Artifact Cross-Linking

Artifacts must cross-reference each other so a reviewer can move between:

- spec
- design
- scopes
- report
- user validation
- state

Use the templates in `scope-templates.md` as the single source of truth for artifact shapes. Use `managed-docs.md` and the effective managed-doc registry for published-doc targets.

## Artifact Freshness And Supersession

Existing artifacts must be reconciled when current truth changes.

- `spec.md`, `design.md`, and planning artifacts may preserve history, but they must expose only one active truth.
- Invalid legacy content must be removed from active sections immediately.
- If history matters, preserve it under clearly labeled superseded or suppressed sections.
- Default behavior for existing artifacts is `reconcile`; use `redesign` for major behavioral or structural changes and `replace` when most of the prior artifact is no longer valid.

### Active Scope Inventory Rule

When requirements or design changes invalidate scopes:

- stale scopes must be removed from the active execution inventory
- stale scopes may be preserved only in a clearly marked superseded appendix
- stale scopes must not remain executable, eligible, or status-bearing

## Documentation Ownership Boundary

Artifact ownership remains defined in `artifact-ownership.md`. Diagnostic agents may identify required changes, but they do not directly rewrite foreign-owned planning or design artifacts except through the execution-only exception.

Managed docs are the published truth surfaces. Feature, bug, and ops packets remain execution truth while work is active.