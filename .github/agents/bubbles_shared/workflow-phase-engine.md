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
- findings handling after harden/gaps/stabilize/security
- auto-escalation before terminal blocking
- handoff escalation as last resort
- promotion rules, state-transition guard enforcement, specialist completion checks, anti-fabrication checks, execution-history writes, and per-spec commit transaction rules

Retained workflow-agent anchors that must still be honored:

- Phase 1 is for sequential single-spec execution. Batch work belongs in Phase 0.8.
- This agent MUST actively invoke specialist agents for every phase via `runSubagent`.
- The orchestrator MUST enforce the Pre-Spec Advancement Gate (Gate G019) before advancing to the next spec.
- The orchestrator MUST enforce Cross-Agent Output Verification (G020) and Anti-Fabrication heuristics (G021) after every specialist run.
- The orchestrator MUST enforce Gate G033 before any `implement` phase.
- The state transition guard (G023) remains the first blocking check before any `done` promotion.

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
- strict-mode and delivery-lockdown stop restrictions
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