# Workflow Orchestration Core

Purpose: shared orchestration rules for `bubbles.workflow` and other agents that coordinate planning, implementation, and recovery across specialist phases.

## Planning-First Recovery Protocol

When execution discovers undocumented or improperly documented work, repair the planning layer before continuing:

1. Missing classified work folder: classify the work item and create the correct feature, bug, or ops artifact set via the owning agent chain.
2. Existing folder but missing artifacts: invoke the owner chain to create the missing artifacts instead of letting downstream agents continue on partial docs.
3. Existing artifacts but empty or skeletal content: treat that state as missing planning, not as a valid prerequisite.
4. Placeholder, TODO, or stub behavior uncovered during execution: if the behavior is not already owned by an active feature, bug, or ops packet, promote it into one before allowing implementation or hardening to claim progress.
5. UI-bearing work: when the promoted work has user-facing behavior, include `bubbles.ux` in the planning chain before design and plan.

This protocol is mandatory for feature work, bug work, hardening, gaps, stabilize, improve-existing, redesign-existing, and iterate-triggered execution. Orchestrators must repair the planning deficit instead of stopping with advice to the user.

## Baseline Workflow Law

These are baseline workflow laws, not optional tags:

- Implementation must not start until `spec.md`, `design.md`, and planning artifacts are present and coherent.
- Changed behavior must map to explicit Gherkin scenarios before coding starts.
- Scenario-specific tests must be identified in the scope plan before coding starts.
- E2E or integration proof must be driven from those planned scenarios.

These requirements are enforced by planning readiness, G033 design readiness, Gherkin/Test Plan/DoD checks, and planning-first recovery. They are not what `tdd: true` turns on.

## Auto-Escalation Protocol

When a phase fails and retry limits are approaching or exhausted, orchestration must attempt bounded inline recovery before handing off or blocking:

1. Identify the unmet prerequisite or blocker.
2. Invoke the owning specialist inline.
3. Resume the original phase after that repair.
4. Mark the spec `blocked` only if the inline repair path also exhausts its own retry limits.

Default routing map:

- Weak planning: `bubbles.design` + `bubbles.plan`
- Weak scenarios or DoD: `bubbles.harden`
- Implementation gaps: `bubbles.gaps` + `bubbles.implement`
- Defect packets: `bubbles.bug` + `bubbles.implement`
- State drift: inline reconciliation of `state.json`, stale execution claims, and stale certification claims
- Post-implementation test failures: `bubbles.implement` + `bubbles.test`