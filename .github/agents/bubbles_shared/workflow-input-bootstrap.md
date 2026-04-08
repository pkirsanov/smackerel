## Workflow Input And Bootstrap

Use this module as the canonical source for the non-batch input-enrichment, research, bootstrap, and pre-implementation readiness contracts in `bubbles.workflow`.

### Phase 0.3: Analysis Loop

**Scope:** This section owns the single-spec analysis pipeline when `batch` is false and the selected mode includes `analyze`.

Authoritative requirements:

- Run the upstream business analysis and UX pipeline for the single target spec only.
- Invoke `bubbles.analyst` to create or enrich `spec.md` with actors, scenarios, competitive analysis, and improvement proposals.
- Invoke `bubbles.ux` for UI-bearing work to add ASCII wireframes and journey flows.
- Preserve the existing skip logic for pre-populated `## Actors & Personas`, `## UI Wireframes`, `skip_analysis: true`, and all batch-enabled runs.
- Enforce Gate G032 before continuing.
- When `effectiveSpecReview != off`, route next into Phase 0.35 before any implementation-capable work.

### Phase 0.35: One-Shot Spec Review Hook

This section owns the one-shot `bubbles.spec-review` contract, including:

- exactly-once-per-target invocation for the current workflow run
- post-analysis timing for modes that include `analyze`
- pre-implementation timing for modes that do not include `analyze`
- batch and delivery-lockdown placement rules
- stale-artifact routing rules for `CURRENT`, `MINOR_DRIFT`, `PARTIAL`, `MAJOR_DRIFT`, `OBSOLETE`, and `route_required`
- rerouting to `reconcile-to-doc`, `product-to-delivery`, or the narrower planning owners when the active artifacts are not trustworthy

### Phase 0.65: Validation Reconciliation Loop

This section owns the validate-first reconciliation contract for modes that set `requireArtifactStateReconciliation: true`, including:

- baseline `validate` as the authority for claimed-versus-implemented drift
- stale completion claim parsing and artifact/state reconciliation before new implementation work
- required resets to `state.json`, scope status, and stale certification/execution claims when drift is detected
- pass-through of the reconciled finding bundle into downstream `implement`, `harden`, and `gaps` phases

### Phase 0.5: Value-First Work Discovery

This section owns the deterministic value-first ranking contract for `mode: value-first-e2e-batch`, including:

- candidate discovery across planned scopes, bugs, gaps, hardening, missing planning, and new work
- scoring using `bubbles/workflows.yaml` `priorityScoring`
- per-dimension scoring for `userImpact`, `deliveryBlocker`, `complianceRisk`, `regressionRisk`, `readiness`, and `effortInverse`
- weighted ranking, tie-breaker handling, and the required ranked-candidate output payload

### Phase 0.55: Objective Research Pass

This section owns the two-pass brownfield current-truth protocol, including:

- question generation via a solution-aware subagent pass
- solution-blind codebase fact gathering via a fresh subagent pass
- the required `## Current Truth` insertion at the top of `design.md`
- skip rules for greenfield work or specs explicitly marked as net-new

### Phase 0.6: Bootstrap Loop

This section owns the iterative bootstrap contract for underspecified work, including:

- optional analyst and UX setup when the mode requires analysis and those sections are missing
- design creation/refinement via `bubbles.design`
- ambiguity routing via `bubbles.clarify` only when needed
- scope generation via `bubbles.plan`
- repeat-until-ready exit criteria for coherent design, actionable spec, and execution-ready scopes

### Pre-Implementation Readiness Check (Gate G033)

This section owns the mandatory pre-implement readiness gate, including:

- substantive `design.md` requirements
- `scopes.md` Gherkin, Test Plan, and DoD checkbox requirements
- re-verification of `spec.md`
- inline auto-escalation to `bubbles.design`, `bubbles.clarify`, and `bubbles.plan` when artifacts are incomplete
- three-iteration retry budget before terminal `blocked`
- the documented exemptions for `bugfix-fastlane` and modes whose analyze/bootstrap phases run first

### Phase 0.7: Spec/Scope Hardening Loop

This section owns the docs/spec hardening contract for `mode: spec-scope-hardening`, including:

- iterative refinement of `spec.md`, `design.md`, and `scopes.md` via `bubbles.harden`
- required hardening outcomes for user stories, Gherkin coverage, explicit E2E mapping, and DoD expansion
- hard enforcement of `G015/G016`
- the `specs_hardened` status ceiling and prohibition on promoting directly to `done`