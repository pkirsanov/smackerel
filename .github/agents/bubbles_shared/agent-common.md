<!-- governance-version: 3.0.0 -->
# Shared Agent Patterns (Common to all bubbles.* agents)

This file is the governance index for the Bubbles framework. It exists to route agents and reviewers to the smallest authoritative module set that fits the task.

Use this file as an index and compatibility reference, not as the default full-context load.

## Core Principle

Load the smallest authoritative module set that matches the role and current phase. Do not recreate shared rules inside prompts or secondary docs.

## Governance Module Index

| Need | Load |
|------|------|
| Hard non-negotiables | `critical-requirements.md` |
| Operating baseline | `operating-baseline.md`, `execution-ops.md` |
| Artifact ownership | `artifact-ownership.md` |
| Artifact freshness and supersession | `artifact-freshness.md` |
| Completion chain and state integrity | `completion-governance.md` |
| Validation model | `validation-core.md`, `validation-profiles.md` |
| Test, evidence, and quality gates | `quality-gates.md` |
| Artifact lifecycle and scope structure | `artifact-lifecycle.md` |
| Scope templates | `scope-templates.md` |
| Workflow rules and phase sequencing | `scope-workflow.md`, `state-gates.md` |
| Planning bootstrap (includes test-plan.json) | `plan-bootstrap.md`, `planning-core.md` |
| Implementation bootstrap (includes regression auto-gen) | `implement-bootstrap.md`, `execution-core.md` |
| Testing bootstrap (includes test-plan.json consumption) | `test-bootstrap.md`, `test-core.md` |
| Audit bootstrap | `audit-bootstrap.md`, `audit-core.md` |
| Analyst bootstrap | `analysis-bootstrap.md` |
| Design bootstrap | `design-bootstrap.md` |
| Docs bootstrap | `docs-bootstrap.md` |
| Clarify bootstrap | `clarify-bootstrap.md` |
| UX bootstrap | `ux-bootstrap.md` |
| Consumer rename/removal rules | `consumer-trace.md` |
| Persistent regression expectations | `e2e-regression.md` |
| Evidence-specific rules | `evidence-rules.md` |
| Shared test-substance rules | `test-fidelity.md` |

## Quick Routing Guide

| Question | Authoritative Source |
|----------|----------------------|
| Who owns this artifact? | `artifact-ownership.md` |
| How do I invalidate stale spec/design/scopes safely? | `artifact-freshness.md` |
| Can this scope/spec be marked complete? | `completion-governance.md` |
| What does `done_with_concerns` mean? | `completion-governance.md` |
| What is the Honesty Incentive? | `critical-requirements.md` |
| What are the evidence provenance rules? | `evidence-rules.md` → Evidence Provenance Taxonomy |
| What is an Uncertainty Declaration? | `evidence-rules.md` → Uncertainty Declaration Protocol |
| What are Spot-Check Recommendations? | `audit-core.md` → Spot-Check Recommendations |
| What Tier 2 checks apply to this agent? | `validation-profiles.md` |
| What test categories and evidence rules apply? | `quality-gates.md`, `evidence-rules.md`, `test-fidelity.md` |
| What artifacts must exist and how are scopes structured? | `artifact-lifecycle.md`, `scope-templates.md`, `scope-workflow.md` |
| What is the role loading baseline? | `operating-baseline.md` |
| What happens on retries, timeouts, or auto-commit? | `execution-ops.md` |
| What is the 3-strike escalation protocol? | `execution-ops.md` |
| How do workflow phases and state transitions work? | `scope-workflow.md`, `state-gates.md` |
| How does smart phase routing (skip/re-evaluate) work? | `workflows.yaml` → `phaseRelevance` section |
| How do mechanical vs taste decisions work? | `workflows.yaml` → `decisionPolicy` section |
| How does cross-model review work? | `workflows.yaml` → `crossModelReview` section |
| How does test-plan.json handoff work? | `planning-core.md`, `test-bootstrap.md` |
| How does regression test auto-generation work (bug fixes)? | `implement-bootstrap.md` |
| How do I run a retrospective? | `bubbles.retro.agent.md` |
| How does the v3 control plane work (execution vs certification, policy defaults, scenario contracts)? | `feature-templates.md`, [CONTROL_PLANE_DESIGN.md](../../docs/guides/CONTROL_PLANE_DESIGN.md), [CONTROL_PLANE_SCHEMAS.md](../../docs/guides/CONTROL_PLANE_SCHEMAS.md) |
| What are gates G042–G068 (capability delegation, policy provenance, validate certification, scenario manifest, lockdown, regression contract, scenario TDD, rework packets, owner-only remediation, concrete results, child-workflow depth, etc.)? | `workflows.yaml` gate definitions, [CONTROL_PLANE_DESIGN.md](../../docs/guides/CONTROL_PLANE_DESIGN.md) |
| Who owns state.json certification vs execution claims? | `agent-ownership.yaml`, `agent-capabilities.yaml`, `scope-workflow.md` |

## Command Prefix Convention (NON-NEGOTIABLE)

When any agent emits a command recommendation, prompt example, next-step instruction, or continuation option that references a Bubbles agent, it MUST use the `/` slash prefix: `/bubbles.workflow`, `/bubbles.validate`, `/bubbles.test`. The `@` prefix is NEVER correct for Bubbles agent invocations — `@bubbles.*` references are wrong and must not appear in agent output.

**This applies to ALL agents** — including recap, status, handoff, and any agent that generates suggested next commands. Every agent reference in output MUST start with `/bubbles.`, never `@bubbles.`.

## Workflow-Only Continuation Convention (NON-NEGOTIABLE)

When a read-only or advisory surface suggests how to continue stateful work, it MUST default to a workflow command, not a raw specialist command.

Required behavior:
- Recap, status, handoff, and recommendation-first uses of `bubbles.super` MUST recommend `/bubbles.workflow ...` with the appropriate mode by default.
- Direct specialist continuation commands such as `/bubbles.implement`, `/bubbles.test`, `/bubbles.validate`, or `/bubbles.audit` are allowed only when the user explicitly asks for a surgical direct-agent invocation.
- Read-only continuation surfaces may emit a `## CONTINUATION-ENVELOPE` carrying `target`, `intent`, `preferredWorkflowMode`, `tags`, and `reason` so `bubbles.workflow` can consume the recommendation safely.
- If a continuation recommendation came from recap, status, handoff, or another advisory surface, treat it as intent-routing metadata, not as permission to bypass workflow orchestration.

## Artifact Ownership And Delegation Contract

Use [artifact-ownership.md](artifact-ownership.md) as the single source of truth for ownership boundaries and foreign-artifact routing.

## Artifact Freshness And Supersession

Use [artifact-freshness.md](artifact-freshness.md) when requirements, UX, design, or planning artifacts drift from current truth.

## Absolute Completion Hierarchy

Use [completion-governance.md](completion-governance.md) as the authoritative source for:

- per-DoD validation
- scope completion
- spec completion
- deferral blocking
- red/green traceability
- consumer trace
- scope size discipline
- live-stack authenticity
- state claim integrity

## Per-Agent Completion Validation Protocol

Use [validation-core.md](validation-core.md) for the shared Tier 1/Tier 2 model and [validation-profiles.md](validation-profiles.md) for role-specific Tier 2 checks.

Prompt files should reference the matching profile instead of embedding duplicate validation tables.

## Operating Baseline

Use [operating-baseline.md](operating-baseline.md) as the source for:

- context-loading profiles
- loop guard behavior
- indirection rules
- **framework file immutability** — agents MUST NEVER modify files in `.github/bubbles/scripts/`, `.github/agents/bubbles_shared/`, `.github/agents/bubbles.*.agent.md`, or other framework-managed paths
- action-first execution
- role baselines

Use [execution-ops.md](execution-ops.md) for bounded retries, timeout expectations, lessons-learned memory, and optional atomic commit behavior.

## Universal Truthfulness And Test Substance

Use [quality-gates.md](quality-gates.md), [evidence-rules.md](evidence-rules.md), and [test-fidelity.md](test-fidelity.md) as the authoritative source for:

- evidence standards
- anti-fabrication behavior
- test substance expectations
- live-stack classification
- completion checkpoints

## Artifact Lifecycle And Scope Structure

Use [artifact-lifecycle.md](artifact-lifecycle.md), [scope-templates.md](scope-templates.md), and [scope-workflow.md](scope-workflow.md) as the authoritative source for:

- work classification
- required feature and bug artifacts
- user validation expectations
- scope structure
- DoD shape
- scope isolation and pickup
- artifact cross-linking

## Quality Gates And Completion-State Integrity

Use [quality-gates.md](quality-gates.md) plus [state-gates.md](state-gates.md) as the authoritative source for:

- canonical test taxonomy
- implementation reality
- integration completeness
- vertical slice completeness
- sequential completion
- specialist completion chain
- phase-scope coherence
- mandatory completion checkpoint

## Compatibility Note

Older prompts, instructions, or downstream repos may still refer to `agent-common.md` as the universal governance file. That remains valid because this file is the stable index into the shared governance modules. New prompt work should prefer the smallest specific module set instead of treating this file as the default full load.
