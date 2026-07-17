# Recipe: Cross-Repo Goal Scenario

> *"One plan, boys. From the napkin to the park bein' online — and nobody deploys till I say go."*

Use this when an operator outcome is bigger than one spec and one mode: it spans more than
one repo, chains heterogeneous phases (review → plan → deliver → deploy → operate), and/or
ends in a real host-mutating deploy. Instead of inventing a new workflow mode per mission,
`bubbles.goal` / `bubbles.sprint` **compile a goal scenario**: a typed, dependency-ordered
DAG whose nodes each resolve to an EXISTING mode or specialist.

This is NOT a new workflow mode and NOT `scenario-manifest.json` (that's the Gherkin
scenario-contract registry — a different thing). It is a runtime execution plan. See the
canonical contract: [`agents/bubbles_shared/scenario-compile.md`](../../agents/bubbles_shared/scenario-compile.md).

## Who Does What

```
User → /bubbles.goal (or /bubbles.sprint)
         │  (vague? → runSubagent(bubbles.super) → RESOLUTION-ENVELOPE, intent only)
         ▼
   compile scenario DAG  →  validate (scenario-compile-lint.sh)  →  preview + approval
         ▼
  execute nodes in dependency order (each = existing mode/agent, directly executed by the active runner)
         ▼
   verify rootOutcome (successSignal proven, hardConstraints held)
```

- `bubbles.super` resolves natural language to intent ONLY — it never compiles or runs the DAG.
- `bubbles.goal` compiles + converges a **single** declared outcome.
- `bubbles.sprint` compiles + executes a **multi-outcome** ordered mission.
- Each node runs in its own repo's command surface and is certified by `bubbles.validate`
  **in that repo**. The scenario ledger aggregates per-repo results; it never certifies across repos.

## Quick Start — Natural Language

```
/bubbles.goal  make <product-repo> ready for deployment to <target> and ship it; <adapter-repo> owns the target details; deliver all required work with no skips, deploy to <target>, verify the running service, and set up ongoing devops/monitoring/backup
```

or, when the mission is an explicit ordered list of outcomes:

```
/bubbles.sprint  review deliver deploy and operate: get <product-repo> MVP ready for <target>, plan required specs in <product-repo> and <adapter-repo>, deliver everything, deploy, then stand up ongoing ops
```

If you're not sure which to use, ask the super:

```
/bubbles.super  I want to take this whole repo from "specs exist" to "live and operated on my target" — what do I run?
```

## The Scenario DAG

The orchestrator compiles a plan to `.specify/runtime/scenario-plan-<id>.json` and validates
it with `bash bubbles/scripts/scenario-compile-lint.sh <plan>`. A typical
delivery-to-deployment scenario:

| Node | type | repo | resolves to | depends on |
|------|------|------|-------------|------------|
| readiness | diagnostic | product | `bubbles.system-review` | — |
| product-plan | planning | product | `product-to-planning` | readiness |
| adapter-plan | planning | adapter | `product-to-planning` | readiness |
| product-deliver | delivery | product | `full-delivery` | product-plan |
| adapter-deliver | delivery | adapter | `devops-to-doc` | adapter-plan |
| deploy-verify | verification | product | `validate-only` | product-deliver, adapter-deliver |
| deploy | **action** | adapter | OPS packet → existing deploy surface | deploy-verify |
| live-ops | ongoing-ops | product | `stabilize-to-doc` OPS packet | deploy |

Node `type` is routing metadata — it does NOT add a new completion model. `action` and
`ongoing-ops` nodes are OPS packets under `specs/_ops/OPS-*` in the target repo, delivered
through an existing mode and certified by `bubbles.validate`.

## The Approval Gate (host mutation)

A host-mutating `action` node (deploy/promote/rollback) is gated exactly like a propagate
backport:

1. The orchestrator reaches the node and emits `route_required` with `action: human-approval`,
   naming the target, the command, the rollback path, and the `riskClass`. It STOPS.
2. You re-invoke with an approval token. Without it, the node refuses — never silently proceeds.
3. The token is recorded in the scenario ledger.

"All specs done" only means "ready to CONSIDER deploy". The approval token — PRE-mutation,
per-action-node — is the permission.

## Hard Rules (mechanically enforced by `scenario-compile-lint.sh`)

- **No fan-out node.** A node MUST NOT resolve to a `requiresTopLevelRuntime` mode
  (`iterate`, `autonomous-goal`, `autonomous-sprint`, `stochastic-quality-sweep`,
  `retro-quality-sweep`, `idea-to-release-completion`). The scenario executor is already the
  top-level orchestrator; nesting a fan-out mode breaks Gate **G064**. The lint derives the
  forbidden set from `modes.yaml` so it never drifts.
- **Every node references a real mode or agent**, declares a repo in `repos[]`, and uses
  exactly one of `mode`/`agent`.
- **Action nodes are fully gated** (`approvalRequired: true` + `riskClass` + `opsPacket`).
- **`dependsOn` forms a DAG** (no cycles, no dangling references).
- **`rootOutcome` is a complete Outcome Contract** (Gate **G070** shape: intent,
  successSignal, hardConstraints, failureCondition).

Validate any compiled plan:

```bash
bash bubbles/scripts/scenario-compile-lint.sh .specify/runtime/scenario-plan-<id>.json
# downstream: bash .github/bubbles/scripts/scenario-compile-lint.sh <plan>
```

## Anti-Patterns (FORBIDDEN)

| Anti-pattern | Why it's wrong | Fix |
|--------------|---------------|-----|
| Adding a new `mode:` per mission | Mode sprawl | Compose existing modes as DAG nodes; the scenario is data |
| A node resolving to `autonomous-goal`/`iterate`/a sweep | Orchestrator-inside-orchestrator (G064) | Single-spec modes or specialists only |
| `super` compiling/executing the DAG | Role violation | `super` resolves intent; goal/sprint compile + execute |
| One repo's `state.json` certifying another | Cross-repo certification leak | Per-repo validate-owned certification; ledger aggregates only |
| Deploy because "all specs are done" | Skips the approval gate | Approval token at the action node, PRE-mutation |
| `action` node "done because exit 0" | No real completion proof | OPS packet + apply + live verify + rollback proof + validate certification |

## Related Recipes

- [Autonomous Goal](autonomous-goal.md) — single-outcome autonomous convergence (the engine per node)
- [Autonomous Sprint](autonomous-sprint.md) — multi-outcome time-bounded execution
- [DevOps Work](devops-work.md) — the delivery lane action nodes route through
- [Build-Once Deploy-Many](build-once-deploy-many.md) — the deploy surface an action node invokes
- [Add A Deployment Target](add-deployment-target.md) — authoring the adapter an action node deploys through
- [Live Deployment Convergence](live-deployment-convergence.md) — connector, seed-data, and live-journey convergence on a real target
- [Ask the Super First](ask-the-super-first.md) — when you don't know whether it's goal or sprint

## Related Skills & Modules

- [`agents/bubbles_shared/scenario-compile.md`](../../agents/bubbles_shared/scenario-compile.md) — the authoritative DAG contract
- [`bubbles-deployment-target-adapter`](../../skills/bubbles-deployment-target-adapter/SKILL.md) — adapter contract for action nodes
- [`bubbles-upkeep-cadence`](../../skills/bubbles-upkeep-cadence/SKILL.md) — backup/monitoring for ongoing-ops nodes
- [`bubbles-result-envelope`](../../skills/bubbles-result-envelope/SKILL.md) — per-node result shape

## Related Gates

- **G064** — authorized top-level workflow runner; no nested runner or fan-out node nesting
- **G070** — Outcome Contract (the `rootOutcome` shape verified at the end)
- **G056** — validate-owned certification (per repo)
