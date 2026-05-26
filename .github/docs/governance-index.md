# Bubbles Governance Index

> Auto-tracked by `bubbles/scripts/governance-index-lint.sh` (T2E-B6).
>
> This file is the canonical roll-up of every governance doc that ships with
> the Bubbles framework. Adding a new governance doc without linking it
> from this index (or another well-known index) will be flagged as an
> orphan by `governance-index-lint.sh` on the next `framework-validate`
> run.
>
> Each entry is a stub link. Open the file directly to read its purpose,
> scope, and authority level — the source files are the source of truth.

---

## Agent Shared Docs (`agents/bubbles_shared/`)

Shared governance, contracts, templates, and bootstrap modules that every
agent reads.

- [agent-common.md](../agents/bubbles_shared/agent-common.md)
- [analysis-bootstrap.md](../agents/bubbles_shared/analysis-bootstrap.md)
- [artifact-freshness.md](../agents/bubbles_shared/artifact-freshness.md)
- [artifact-lifecycle.md](../agents/bubbles_shared/artifact-lifecycle.md)
- [artifact-ownership.md](../agents/bubbles_shared/artifact-ownership.md)
- [audit-bootstrap.md](../agents/bubbles_shared/audit-bootstrap.md)
- [audit-core.md](../agents/bubbles_shared/audit-core.md)
- [bug-templates.md](../agents/bubbles_shared/bug-templates.md)
- [capability-foundation.md](../agents/bubbles_shared/capability-foundation.md)
- [clarify-bootstrap.md](../agents/bubbles_shared/clarify-bootstrap.md)
- [completion-governance.md](../agents/bubbles_shared/completion-governance.md)
- [consumer-trace.md](../agents/bubbles_shared/consumer-trace.md)
- [critical-requirements.md](../agents/bubbles_shared/critical-requirements.md)
- [design-bootstrap.md](../agents/bubbles_shared/design-bootstrap.md)
- [docker-lifecycle-governance.md](../agents/bubbles_shared/docker-lifecycle-governance.md)
- [docs-bootstrap.md](../agents/bubbles_shared/docs-bootstrap.md)
- [e2e-regression.md](../agents/bubbles_shared/e2e-regression.md)
- [evidence-rules.md](../agents/bubbles_shared/evidence-rules.md)
- [execution-core.md](../agents/bubbles_shared/execution-core.md)
- [execution-ops.md](../agents/bubbles_shared/execution-ops.md)
- [feature-templates.md](../agents/bubbles_shared/feature-templates.md)
- [implement-bootstrap.md](../agents/bubbles_shared/implement-bootstrap.md)
- [managed-docs.md](../agents/bubbles_shared/managed-docs.md)
- [operating-baseline.md](../agents/bubbles_shared/operating-baseline.md)
- [plan-bootstrap.md](../agents/bubbles_shared/plan-bootstrap.md)
- [planning-core.md](../agents/bubbles_shared/planning-core.md)
- [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md)
- [quality-gates.md](../agents/bubbles_shared/quality-gates.md)
- [review-core.md](../agents/bubbles_shared/review-core.md)
- [scope-templates.md](../agents/bubbles_shared/scope-templates.md)
- [scope-workflow.md](../agents/bubbles_shared/scope-workflow.md)
- [state-gates.md](../agents/bubbles_shared/state-gates.md)
- [test-bootstrap.md](../agents/bubbles_shared/test-bootstrap.md)
- [test-core.md](../agents/bubbles_shared/test-core.md)
- [test-fidelity.md](../agents/bubbles_shared/test-fidelity.md)
- [ux-bootstrap.md](../agents/bubbles_shared/ux-bootstrap.md)
- [validation-core.md](../agents/bubbles_shared/validation-core.md)
- [validation-profiles.md](../agents/bubbles_shared/validation-profiles.md)
- [workflow-delegation-core.md](../agents/bubbles_shared/workflow-delegation-core.md)
- [workflow-execution-loops.md](../agents/bubbles_shared/workflow-execution-loops.md)
- [workflow-fix-cycle-protocol.md](../agents/bubbles_shared/workflow-fix-cycle-protocol.md)
- [workflow-input-bootstrap.md](../agents/bubbles_shared/workflow-input-bootstrap.md)
- [workflow-mode-resolution.md](../agents/bubbles_shared/workflow-mode-resolution.md)
- [workflow-orchestration-core.md](../agents/bubbles_shared/workflow-orchestration-core.md)
- [workflow-phase-engine.md](../agents/bubbles_shared/workflow-phase-engine.md)

---

## Framework Maintainer Docs (`docs/`)

Durable source-repo framework behavior and maintainer contracts.

- [Framework_Convergence_Health.md](Framework_Convergence_Health.md)
- [Spec_Implementation_Alignment.md](Spec_Implementation_Alignment.md)

---

## Instructions (`instructions/`)

Project-installable instruction modules consumed by IDE agents via
`applyTo` patterns.

- [bubbles-agents.instructions.md](../instructions/bubbles-agents.instructions.md)
- [bubbles-config-sst.instructions.md](../instructions/bubbles-config-sst.instructions.md)
- [bubbles-deployment-target.instructions.md](../instructions/bubbles-deployment-target.instructions.md)
- [bubbles-docker-lifecycle-governance.instructions.md](../instructions/bubbles-docker-lifecycle-governance.instructions.md)
- [bubbles-skills.instructions.md](../instructions/bubbles-skills.instructions.md)
- [bubbles-test-environment-isolation.instructions.md](../instructions/bubbles-test-environment-isolation.instructions.md)

---

## Skills (`skills/*/SKILL.md`)

Discoverable procedural workflows packaged as model skills.

- [bubbles-config-sst](../skills/bubbles-config-sst/SKILL.md)
- [bubbles-capability-foundation-design](../skills/bubbles-capability-foundation-design/SKILL.md)
- [bubbles-deployment-target-adapter](../skills/bubbles-deployment-target-adapter/SKILL.md)
- [bubbles-docker-lifecycle-governance](../skills/bubbles-docker-lifecycle-governance/SKILL.md)
- [bubbles-docker-port-standards](../skills/bubbles-docker-port-standards/SKILL.md)
- [bubbles-product-principle-discovery](../skills/bubbles-product-principle-discovery/SKILL.md)
- [bubbles-repo-readiness](../skills/bubbles-repo-readiness/SKILL.md)
- [bubbles-skill-authoring](../skills/bubbles-skill-authoring/SKILL.md)
- [bubbles-spec-template-bdd](../skills/bubbles-spec-template-bdd/SKILL.md)
- [bubbles-tailnet-edge-pattern](../skills/bubbles-tailnet-edge-pattern/SKILL.md)
- [bubbles-test-environment-isolation](../skills/bubbles-test-environment-isolation/SKILL.md)
- [bubbles-test-integrity](../skills/bubbles-test-integrity/SKILL.md)

---

## Recipes (`docs/recipes/`)

Operator-facing workflow recipes that compose agents and prompts into
end-to-end flows.

- [README.md](recipes/README.md)
- [add-deployment-target.md](recipes/add-deployment-target.md)
- [ask-the-super-first.md](recipes/ask-the-super-first.md)
- [autonomous-goal.md](recipes/autonomous-goal.md)
- [autonomous-sprint.md](recipes/autonomous-sprint.md)
- [bookend-phases.md](recipes/bookend-phases.md)
- [brainstorm-idea.md](recipes/brainstorm-idea.md)
- [build-once-deploy-many.md](recipes/build-once-deploy-many.md)
- [chaos-testing.md](recipes/chaos-testing.md)
- [check-status.md](recipes/check-status.md)
- [choose-review-path.md](recipes/choose-review-path.md)
- [code-health-analysis.md](recipes/code-health-analysis.md)
- [cross-model-review.md](recipes/cross-model-review.md)
- [custom-gates.md](recipes/custom-gates.md)
- [devops-release-coordination.md](recipes/devops-release-coordination.md)
- [devops-work.md](recipes/devops-work.md)
- [design-a-capability.md](recipes/design-a-capability.md)
- [end-of-day.md](recipes/end-of-day.md)
- [explore-idea.md](recipes/explore-idea.md)
- [fix-a-bug.md](recipes/fix-a-bug.md)
- [framework-dogfood.md](recipes/framework-dogfood.md)
- [framework-ops.md](recipes/framework-ops.md)
- [grill-an-idea.md](recipes/grill-an-idea.md)
- [idea-to-release.md](recipes/idea-to-release.md)
- [just-tell-bubbles.md](recipes/just-tell-bubbles.md)
- [new-feature.md](recipes/new-feature.md)
- [ops-packet-work.md](recipes/ops-packet-work.md)
- [outcome-first-specs.md](recipes/outcome-first-specs.md)
- [parallel-scopes.md](recipes/parallel-scopes.md)
- [plan-only.md](recipes/plan-only.md)
- [post-impl-hardening.md](recipes/post-impl-hardening.md)
- [quality-sweep.md](recipes/quality-sweep.md)
- [reconcile-redesign-existing-feature.md](recipes/reconcile-redesign-existing-feature.md)
- [regression-check.md](recipes/regression-check.md)
- [release-planning.md](recipes/release-planning.md)
- [resume-work.md](recipes/resume-work.md)
- [retro-driven-harden.md](recipes/retro-driven-harden.md)
- [retro-driven-review.md](recipes/retro-driven-review.md)
- [retro-driven-simplify.md](recipes/retro-driven-simplify.md)
- [retro-quality-sweep.md](recipes/retro-quality-sweep.md)
- [retro.md](recipes/retro.md)
- [review-code-directly.md](recipes/review-code-directly.md)
- [review-then-improve.md](recipes/review-then-improve.md)
- [runtime-coordination.md](recipes/runtime-coordination.md)
- [safe-shared-infrastructure-refactor.md](recipes/safe-shared-infrastructure-refactor.md)
- [security-review.md](recipes/security-review.md)
- [setup-project.md](recipes/setup-project.md)
- [simplify-existing-code.md](recipes/simplify-existing-code.md)
- [spec-freshness-review.md](recipes/spec-freshness-review.md)
- [structured-commits.md](recipes/structured-commits.md)
- [system-review.md](recipes/system-review.md)
- [tdd-first-execution.md](recipes/tdd-first-execution.md)
- [update-docs.md](recipes/update-docs.md)
- [ux-single-file-sweep.md](recipes/ux-single-file-sweep.md)
- [validation-latency-budgets.md](recipes/validation-latency-budgets.md)

---

## Capability-First Design Governance

- Validation IDs: `AN5`, `DE4`, `UX9`, `P4` in [validation-profiles.md](../agents/bubbles_shared/validation-profiles.md)
- Gate: `G094 capability_foundation_gate` in [workflows.yaml](../bubbles/workflows.yaml)
- Guard: [capability-foundation-guard.sh](../bubbles/scripts/capability-foundation-guard.sh)
