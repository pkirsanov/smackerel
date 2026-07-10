# Workflow Orchestration Core

Purpose: shared orchestration rules for `bubbles.workflow` and other agents that coordinate planning, implementation, and recovery across specialist phases.

## Planning-First Recovery Protocol

When execution discovers undocumented or improperly documented work, repair the planning layer before continuing:

1. Missing classified work folder: classify the work item and create the correct feature, bug, or ops artifact set via the owning agent chain.
2. Existing folder but missing artifacts: invoke the owner chain to create the missing artifacts instead of letting downstream agents continue on partial docs.
3. Existing artifacts but empty or skeletal content: treat that state as missing planning, not as a valid prerequisite.
4. Placeholder, TODO, or stub behavior uncovered during execution: if the behavior is not already owned by an active feature, bug, or ops packet, promote it into one before allowing implementation or hardening to claim progress.
5. All implementation-capable planning work: invoke the canonical planning chain `bubbles.analyst` → `bubbles.ux` → `bubbles.design` → `bubbles.plan`. UX is mandatory even for framework/operator/non-UI work; non-UI UX defines workflow behavior, status language, blocked envelopes, and exception handling.

This protocol is mandatory for feature work, bug work, hardening, gaps, stabilize, improve-existing, redesign-existing, and iterate-triggered execution. Orchestrators must repair the planning deficit instead of stopping with advice to the user.

## Conditional Clarify Consistency Gate

`bubbles.clarify` is a CONDITIONAL planning-chain consistency gate — it is NOT a mandatory phase in every planning run and stays default-off. It is triggered only by ambiguity or taste-decision overflow: `decisionPolicy.tasteDecisionHandling` `overflowAction: route_to_clarify` fires when a phase accumulates more than `maxPerPhase` (5) taste decisions, and the `autonomy: guarded`/`interactive` dial arms it on genuine ambiguity. When triggered, `clarify` performs structured consistency routing across the planning chain (`spec.md` → `design.md` → `scopes.md`) so the planning layer stays internally coherent before implementation.

Dedupe from grill (they are distinct and MUST NOT be conflated): `clarify` = structured consistency routing (reconcile the planning artifacts); `grill` (the `interrogate` phase) = adversarial pressure-test (challenge the plan). Under `autonomy: full` (the default) neither runs — the run stays 100% autonomous. Do NOT force `clarify` into every planning run; it remains conditional and default-off.

## Baseline Workflow Law

These are baseline workflow laws, not optional tags:

- Implementation must not start until `spec.md`, `design.md`, and planning artifacts are present and coherent.
- Changed behavior must map to explicit Gherkin scenarios before coding starts.
- Scenario-specific tests must be identified in the scope plan before coding starts.
- E2E or integration proof must be driven from those planned scenarios.

These requirements are enforced by planning readiness, G033 design readiness, Gherkin/Test Plan/DoD checks, and planning-first recovery. They are not what `tdd: true` turns on.

## Review-To-Delivery Transition (MANDATORY)

When a review agent (`bubbles.system-review`, `bubbles.code-review`, `bubbles.spec-review`) produces findings that require code changes, the transition from diagnostic findings to delivery work MUST follow this chain:

**Spec-review severe-drift exception:** When `bubbles.spec-review` classifies a spec whose `state.json` status is `done` or legacy read-only `done_with_concerns` as `MAJOR_DRIFT` or `OBSOLETE`, the finding is a certified-spec freshness failure rather than an ordinary code-review defect. The active authorized runner MUST execute `mode=improve-existing` directly for that reviewed spec, resume only after the mapped mode reaches a terminal result, and preserve any `blocked` or `route_required` outcome. Read-only docs-only/spec-review-only modes MUST emit a `mode=improve-existing`, `spec: <reviewed-spec-dir>` packet without making code changes.

1. **Every finding that requires a code change MUST be tracked as a bug.** Invoke `bubbles.bug` to create the full 6-artifact bug packet (`bug.md`, `spec.md`, `design.md`, `scopes.md`, `report.md`, `state.json`) before any implementation begins.
2. **The `directFix` follow-up tag does NOT exempt findings from bug artifact creation.** It only indicates that the fix design is straightforward and does not require new feature-level spec work. Bug-level artifacts are still mandatory.
3. **After bug artifacts are created, deliver via `bugfix-fastlane` mode** (or the appropriate delivery mode) using the standard specialist chain: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs`.
4. **The workflow agent MUST NOT make code changes directly** when processing review findings. All code changes flow through `bubbles.implement` via `runSubagent`.
5. **Batch review findings into individual bug packets** — each distinct finding gets its own bug folder, not a single combined bug for multiple unrelated issues.

This protocol applies regardless of fix complexity. A one-line `go.mod` change and a multi-file architectural refactor both go through the same artifact-first pipeline.

## Auto-Escalation Protocol

When a phase fails and retry limits are approaching or exhausted, orchestration must attempt bounded inline recovery before handing off or blocking:

1. Identify the unmet prerequisite or blocker.
2. Invoke the owning specialist inline.
3. Resume the original phase after that repair.
4. Mark the spec `blocked` only if the inline repair path also exhausts its own retry limits.

**⚠️ PLANNING-MODE CONSTRAINT (NON-NEGOTIABLE):** When the active workflow mode has `statusCeiling` below `done` (i.e., planning-only modes like `spec-scope-hardening`):
- Auto-escalation MUST NOT escalate into implementation specialists (`bubbles.implement`, `bubbles.simplify` code changes, `bubbles.gaps` code changes)
- If the recovery path requires implementation → set status to `route_required` with `nextRequiredOwner: bubbles.implement` and STOP
- Planning-only auto-escalation is limited to: `bubbles.analyst`, `bubbles.ux`, `bubbles.design`, `bubbles.plan`, `bubbles.clarify`, `bubbles.harden` (artifact-only)
- This constraint also applies when the user's original request implied planning-only intent (see workflow-mode-resolution.md → Reciprocal Status Ceiling Warning)

Default routing map:

- Weak planning: `bubbles.analyst` + `bubbles.ux` + `bubbles.design` + `bubbles.plan`
- Weak scenarios or DoD: `bubbles.harden`
- Severe spec-review drift on `done` / legacy read-only `done_with_concerns`: execute `improve-existing` directly in the active authorized runner
- Implementation gaps: `bubbles.gaps` + `bubbles.implement`
- Defect packets: `bubbles.bug` + `bubbles.implement`
- State drift: inline reconciliation of `state.json`, stale execution claims, and stale certification claims
- Post-implementation test failures: `bubbles.implement` + `bubbles.test`

When both `bubbles.gaps` and `bubbles.harden` are dispatched for the same spec, order them gaps → harden: gaps audits spec/design ↔ implementation fidelity first, then harden verifies DoD/tests/policy compliance.