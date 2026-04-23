## Workflow Phase Engine

Use this module as the canonical source for the sequential per-spec execution engine and workflow closeout contract in `bubbles.workflow`.

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

3. **Invoke the finding-owned planning chain** (when mode constraint `requireFindingOwnedPlanningWorkflow` is true):
   - `bubbles.analyst` — analyze the finding's impact and requirements
   - `bubbles.ux` — ONLY when the finding touches UI or a user-visible journey
   - `bubbles.design` — update design.md with the fix/change design
   - `bubbles.plan` — update scopes.md with new/modified scope, Gherkin scenarios, test plan, and DoD

4. **Invoke the finding-owned delivery chain** (when mode constraint `requireTerminalFindingClosure` is true):
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