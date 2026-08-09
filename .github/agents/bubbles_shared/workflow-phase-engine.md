## Workflow Phase Engine

Use this module as the canonical source for the sequential per-spec execution engine and workflow closeout contract in `bubbles.workflow`.

## Repository Binding Consumer Contract

The phase engine MUST validate the current actionable repository-binding packet and require `PREFLIGHT_COMMITTED` before repository-local state reads, relative-path expansion, candidate scans, work selection, repository-owned commands, or specialist dispatch. A missing, stale, redacted, root-substituted, or field-incomplete packet refuses before side effects.

Every repository-sensitive consumer requires all of these fields unchanged:

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable

When a phase requires targetless stochastic or iterate discovery, it MUST call `bubbles/scripts/repository-binding.sh discover-specs` with the current actionable packet after preflight. It may consume candidates only after the exact event `DISCOVERY SCOPE mode=<mode> root=<resolvedRepositoryRoot>/specs`; it must never reconstruct a discovery root from ambient state.

### Phase 1: Per-Spec Orchestration Loop

This section owns the full sequential single-spec execution contract, including:

- batch exclusion checks and sequential-only routing
- the Pre-Spec Advancement Gate (G019)
- the Cross-Agent Output Verification Protocol (G020/G021)
- the full phase-to-agent dispatch table
- per-spec run-record initialization
- grill preflight handling
- phase execution via `runSubagent`
- G033 pre-implementation readiness enforcement
- phase result processing, retries, and failure classification
- **finding-owned closure after trigger phases** (see Finding-Owned Closure Protocol below)
- auto-escalation before terminal blocking
- handoff escalation as last resort
- promotion rules, state-transition guard enforcement, specialist completion checks, anti-fabrication checks, execution-history writes, and per-spec commit transaction rules

Retained workflow-agent anchors that must still be honored:

- Phase 1 is for sequential single-spec execution. Batch work belongs in Phase 0.8.
- This agent MUST actively invoke specialist agents for every phase via `runSubagent`.
- The orchestrator MUST enforce the Pre-Spec Advancement Gate (Gate G019) before advancing to the next spec.
- The orchestrator MUST enforce Cross-Agent Output Verification (G020) and Anti-Fabrication heuristics (G021) after every specialist run.
- The orchestrator MUST enforce Gate G033 before any `implement` phase.
- **The orchestrator MUST enforce the Planning-Only Mode Gate (Gate G070) before any `implement` phase:** If the active workflow mode has `statusCeiling` below `done`, the `implement` phase MUST NOT be invoked. Instead, mark the spec as `route_required` with `nextRequiredOwner: bubbles.implement`. This gate also applies when the user's original request contained planning-only intent language (see workflow-mode-resolution.md → Reciprocal Status Ceiling Warning).
- The state transition guard (G023) remains the first blocking check before any `done` promotion.

#### Phase Relevance Resolution (MANDATORY — one verdict, every runner)

Before dispatching each phase, obtain the skip/run verdict from `bubbles/scripts/phase-relevance-resolve.sh` rather than deciding here. The contract — invocation, the four authorized runners sharing one verdict, the three fail-to-`run` properties, and the absence of any skip-forcing flag — is [operating-baseline.md → Phase Relevance Resolution](operating-baseline.md).

Two obligations belong to this phase loop specifically:

- **Record every decision** in `executionHistory` using the registry's `skipRecordSchema` (`phase`, `outcome: skipped`, `reason`, `changedSurface`, `reevaluated`, `reevaluationTrigger`). A skip that leaves no record is indistinguishable from a phase that was never considered.
- **Re-evaluate every skip** when a `reevaluateTriggers` event occurs — artifact modified, scope surface expanded, gate failure, or a prior phase routing new work. A phase skipped earlier MUST be dispatched if the new surface makes it relevant.

#### Finding-Owned Closure Protocol (MANDATORY — NON-NEGOTIABLE)

**Reference:** [workflow-fix-cycle-protocol.md](workflow-fix-cycle-protocol.md) for the full closure contract.

**Trigger phases** are: `chaos`, `harden`, `gaps`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `test`, and `improve`. When ANY trigger phase returns findings (bugs, gaps, regressions, improvements, operational issues), the orchestrator MUST NOT simply proceed to the next phase in `phaseOrder`. Instead, the orchestrator MUST execute the finding-owned closure chain BEFORE the next phaseOrder step.

**⚠️ FINDING-ONLY OUTPUT IS NOT SUCCESS.** A trigger phase that returns findings without those findings being remediated is a NON-TERMINAL result. The orchestrator MUST NOT advance past the trigger phase, report summary-only output, or return `completed_owned` until every finding has been closed through the full chain below.

**Step-by-step finding closure procedure (execute for EACH finding):**

1. **Parse findings.** Extract each distinct finding from the trigger agent's result. Each finding gets its own closure path.

2. **Classify each finding.** Determine whether it is:
   - A bug under an existing spec → create bug artifacts via `bubbles.bug`
   - A design/spec gap → route to planning chain
   - An operational issue → route via `bubbles.devops`
   - A new capability need → create new spec folder

2a. **Classify its goal impact** (IMP-038 SCOPE-4 / GF-3). Steps 3 and 4 — the mandatory planning and delivery chains — apply to `required` findings: those inside the work boundary that the Goal Contract cannot be satisfied while open. They are NOT the closure path for every finding:

   - `required` → run steps 3 and 4 in full before advancing.
   - `blocking-external` → BLOCK the parent. File the disposition, then request the operator-approved expansion or external repair. Do not deliver it inline; it is outside the approved boundary.
   - `independent` → discharge through its `G095` disposition under a SEPARATE scoped packet, then continue the parent. Filing plus routing IS the closure here; inline delivery is not required and would be unrequested work.

   Use `work-boundary-resolve.sh` for the in/out-of-boundary split, then ask whether the success signal and every hard constraint survive with the finding open. The full contract is [operating-baseline.md → Goal Impact](operating-baseline.md). This does NOT permit cherry-picking: an in-boundary finding that blocks the contract is `required`, and a `required` finding is completed, never routed away.

3. **Invoke the finding-owned planning chain** — for `required` findings — (when mode constraint `requireFindingOwnedPlanningWorkflow` is true):
   - `bubbles.analyst` — analyze the finding's impact and requirements
   - `bubbles.ux` — ONLY when the finding touches UI or a user-visible journey
   - `bubbles.design` — update design.md with the fix/change design
   - `bubbles.plan` — update scopes.md with new/modified scope, Gherkin scenarios, test plan, and DoD

4. **Invoke the finding-owned delivery chain** — for `required` findings — (when mode constraint `requireTerminalFindingClosure` is true):
   - `bubbles.implement` — implement the fix/change (pass the full finding ledger, require one-to-one closure)
   - `bubbles.test` — execute all tests for the changed scope
   - `bubbles.validate` — validate the fix against the spec
   - `bubbles.audit` — audit the change for compliance
   - `bubbles.docs` — sync managed docs

5. **Verify closure.** Every finding from step 1 MUST have been addressed. If ANY finding remains unresolved, the phase is NOT complete — retry or mark blocked.

6. **Resume phaseOrder.** Only after ALL findings are closed, continue with the remaining phases in `phaseOrder`.

**One-to-one accounting rule:** The orchestrator MUST maintain a finding ledger. Every finding is tracked individually. The implement prompt MUST include the full finding list. The implement result MUST account for every finding. Unaccounted findings block advancement.

**⛔ PROHIBITED PATTERNS:**
- ❌ Returning a findings summary table without invoking the planning + delivery chain
- ❌ Proceeding to `implement` in phaseOrder without first running `bubbles.design` → `bubbles.plan` for findings
- ❌ Treating the trigger phase as the entire workflow (finding-only = failure)
- ❌ Skipping the planning chain because "the fix is obvious" — planning is ALWAYS required when `requireFindingOwnedPlanningWorkflow: true`
- ❌ Reporting `completed_owned` while findings remain in `route_required` state

### Phase 2: Optional Global Final Pass

This section owns the full global-final-pass contract, including:

- optional global chaos, validate, and docs passes
- value-first extra priority re-scan behavior
- spec-scope-hardening global verification requirements
- unresolved-issues ledger requirements

### Phase 3: Finalize

This section owns the workflow final summary contract, including:

- execution summary table requirements
- final status reporting by spec
- failed-gate and resume-command reporting rules

#### Registry-Bound Finalize Boundary

Before requesting any terminal transition, the workflow runner MUST
independently execute `transition-contract-resolver.sh` against current state.
It compares the fresh `workflowMode`, target status, `contractDigest`, and
`targetRevision` with its own frozen assertions and with the current
`AUDIT_RESULT_V1`; no prior resolver output is reusable at this boundary.

When the resolved `phaseOrder` contains `audit`, the runner MUST resolve
`execution.audit.currentAttemptId` to exactly one ACTIVE attempt, resolve that
attempt's evidence ref to exactly one complete result transcript, and run the
canonical audit-result contract lint. Attempt ID, result state, audit profile,
target, digest, revision, verdict, outcome, evidence ref, and one-to-one
`addressedFindings`/`unresolvedFindings` accounting MUST match current runner
state. Missing, stale, duplicate, dangling, `INCOMPLETE`, `SUPERSEDED`,
over-ceiling, contradictory, or finding-incomplete evidence is terminally
blocked with no prior-result reuse or guessed profile.

A pre-audit validate pass may report checks but cannot certify the ceiling.
After exactly one matching `planning-maturity-v1` attempt with
`PLANNING_AUDIT_CLEAN`, the runner sends the frozen assertions to
`bubbles.validate`. Finalize itself writes no certification or status. Validate
independently repeats the boundary check and may mirror only top-level `status`
and `certification.status` to exactly `specs_hardened`; scope statuses, DoD,
completed scopes, scope progress, delivery evaluation/evidence, and audit
history remain unchanged. The existing `delivery-completion-v1` done path keeps
its all-scopes-Done, all-DoD-complete, evidence-backed strictness unchanged.

### Failure Routing Contract

This section owns the failure-routing contract, including:

- failure class to specialist mapping
- required re-invocation behavior
- routed-phase re-entry expectations

### Stop Conditions

This section owns the truly terminal stop-condition contract, including:

- the only valid workflow stop reasons
- invalid stop reasons that must instead trigger inline escalation
- strict-mode and full-delivery stop restrictions
- resume-envelope behavior for genuinely blocked specs only

### Agent Completion Validation

This section owns the workflow-level completion validation contract, including:

- Tier 1 + Tier 2 validation requirements
- blocked-result handling when completion checks fail

### Output Requirements

This section owns the final response contract, including:

- execution summary requirements
- blocked-spec reporting requirements
- invocation audit requirements
- continuation-envelope requirements for non-terminal workflow results