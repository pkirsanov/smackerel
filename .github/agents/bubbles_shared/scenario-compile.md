# Goal Scenario Compiler

Use this module when an orchestrator (`bubbles.goal` or `bubbles.sprint`) must turn a
single high-level operator outcome into an **ordered, typed, possibly cross-repo
execution graph** built entirely from EXISTING Bubbles workflow modes and specialist
agents. It is the canonical contract for "compile a scenario, then execute it" without
inventing new per-journey workflow modes and without nesting orchestrators.

A **goal scenario** is NOT a new workflow mode and NOT a `scenario-manifest.json` (that
file is the Gherkin scenario-contract registry — a different concept). A goal scenario is
a runtime execution plan: a dependency-ordered DAG whose nodes each resolve to one
already-defined mode or specialist, executed by the top-level orchestrator turn.

## Ownership Boundary (NON-NEGOTIABLE)

| Concern | Owner | Rule |
|---------|-------|------|
| Natural-language → structured intent | `bubbles.super` | Resolver only. Emits a `RESOLUTION-ENVELOPE` with the cross-repo/scenario fields below. MUST NOT compile or execute the DAG. |
| Single-outcome scenario compilation + execution | `bubbles.goal` | Compiles a DAG that converges ONE declared outcome; executes it in the top-level runtime. |
| Multi-outcome scenario compilation + execution | `bubbles.sprint` | Compiles a DAG of several converged outcomes in dependency order (NOT effort-reordering); executes each node's mode via the goal convergence loop. |
| Per-node delivery work | the node's resolved mode/owner | A delivery node runs `full-delivery`/`devops-to-doc`/etc.; an action node runs an OPS packet. |
| Per-repo completion certification | `bubbles.validate` (in that repo) | Certification is per repo. The scenario ledger NEVER certifies across repos. |

`bubbles.super` stays a `class: utility` resolver (`ownsArtifacts: []`). The compiler lives
in the orchestrators, not in `super`.

## When To Compile A Scenario

Compile a scenario when the operator's outcome is bigger than a single spec/mode and any of:

- the work spans more than one repo (e.g. a product repo plus a deployment-adapter repo);
- the work chains heterogeneous phase types (review → plan → deliver → deploy → operate);
- the work includes a real host-mutating action (deploy/promote/rollback) that must be
  gated behind explicit approval and live verification;
- the work includes ongoing operations setup (monitoring, backup, incident wiring) after a
  deploy.

If the outcome is a single spec through a single mode, do NOT compile a scenario — dispatch
the mode directly. Scenario compilation is for orchestrated, multi-phase missions only.

## The `RESOLUTION-ENVELOPE` Cross-Repo Fields (super → orchestrator)

When the operator's intent implies a scenario, `bubbles.super` adds these OPTIONAL fields to
its `RESOLUTION-ENVELOPE` (all absent for ordinary single-mode requests):

```markdown
## RESOLUTION-ENVELOPE
- invokedAs: subagent
- mode: autonomous-goal | autonomous-sprint
- specTargets: [ ... ]
- goalClass: <e.g. release-deployment-readiness | feature-delivery | hardening>
- primaryRepo: <repo id or workspace folder name>
- supportingRepos: [ <repo id>, ... ]          # empty for single-repo scenarios
- targetEnvironment: <target id>                # only when a deploy is in scope
- deploymentModel: <local-target | registry-pull | ...>   # only when a deploy is in scope
- constraints:
    - <hard constraint in plain English>
- compositionHint: single-outcome | multi-outcome
- rationale: <1 sentence>
- confidence: high | medium | low
```

`super` resolves intent to a PRIMARY mode (`autonomous-goal` for single-outcome,
`autonomous-sprint` for multi-outcome) plus these fields. The orchestrator compiles the DAG
from them. `super` MUST NOT emit a node list — node typing and dependency ordering are the
orchestrator's job.

## Scenario DAG Schema

The orchestrator compiles a scenario into this shape and writes it to the runtime ledger
(see "Runtime State" below). Use abstract placeholders in any documented example; concrete
repo names, host names, and target names belong in the operator's own runtime, NEVER in
framework docs.

```yaml
version: 1
scenarioId: <stable-kebab-id>
rootOutcome:                       # IS an Outcome Contract (Gate G070 shape)
  intent: <what must become true>
  successSignal: <observable proof the mission succeeded>
  hardConstraints:
    - <constraint that MUST hold, e.g. "local-target build, not cloud">
  failureCondition: <what means the mission failed>
  targetReleasePacket: <phase>     # OPTIONAL — when set, the DAG MUST cover every
                                   # delivery=required feature in
                                   # docs/releases/<phase>/features.md (Gate G101)
repos:
  - id: <product>
    role: product
  - id: <adapter>
    role: deployment-adapter
nodes:
  - id: <node-id>
    type: diagnostic | planning | delivery | verification | action | ongoing-ops
    repo: <repos[].id>             # which repo this node executes in
    mode: <existing workflow mode> # OR:
    agent: <existing specialist>   # exactly one of mode/agent (diagnostic may use agent)
    opsPacket: specs/_ops/OPS-<id> # REQUIRED for action and ongoing-ops nodes
    approvalRequired: true|false   # MUST be true for action nodes
    riskClass: <action-risk-registry class>   # REQUIRED for action nodes
    dependsOn: [ <node-id>, ... ]  # forms the DAG; must reference existing node ids
    coversFeatures: [ <feature-id>, ... ]   # delivery nodes only; the release-packet
                                            # feature ids this node delivers. REQUIRED
                                            # coverage when rootOutcome.targetReleasePacket
                                            # is set (Gate G101).
```

### Node Types And Their Completion Proof

Node `type` is routing metadata. It does NOT introduce a new completion model — completion
stays spec/scope/DoD/validate-owned per [completion-governance.md](completion-governance.md).

| type | runs | completion proof |
|------|------|------------------|
| `diagnostic` | a review/analysis agent (`bubbles.system-review`, `bubbles.code-review`, `bubbles.stabilize`, …) | findings ledger; promoted findings routed to owners. Read-only. |
| `planning` | `product-to-planning` / `spec-scope-hardening` | spec/design/scopes created or hardened to the mode's ceiling (`specs_hardened`, Gate G073). |
| `delivery` | `full-delivery` / `devops-to-doc` / `bugfix-fastlane` / `stabilize-to-doc` | validate-certified `done` per repo (Gates G024/G025/G056). |
| `verification` | `validate-only` / `audit-only` | raw evidence, zero blockers. |
| `action` | an OPS packet whose DoD encodes the host-mutating command (deploy/promote/rollback via the repo's existing deploy surface) | approval token recorded; apply + live verify green; rollback path proven; validate-certified. |
| `ongoing-ops` | an OPS packet for monitoring/backup/incident-workflow wiring | telemetry adapter wired, runbook + DoD certified; backup/upkeep cadence declared. |

`action` and `ongoing-ops` nodes are NOT "done because a command exited 0". They are OPS
packets under `specs/_ops/OPS-*` in the target repo, delivered through an existing delivery
mode and certified by `bubbles.validate` in that repo.

## Hard Rules (mechanically enforced by `scenario-compile-lint.sh`)

1. **No depth-2 nodes.** A node's `mode` MUST NOT resolve to any mode whose
   `constraints.requiresTopLevelRuntime` is true in `bubbles/workflows/modes.yaml`
   (currently `iterate`, `autonomous-goal`, `autonomous-sprint`,
   `stochastic-quality-sweep`, `retro-quality-sweep`, `idea-to-release-completion`). The
   scenario executor is ALREADY the top-level orchestrator; nesting a fan-out mode as a
   node would force orchestrator-inside-orchestrator and break Gate **G064**
  (workflow-runner authorization and fan-out depth). The lint derives the forbidden set from `modes.yaml` so it
   never drifts.
2. **Every node references a real mode or agent.** `mode` must exist in `modes.yaml`;
   `agent` must exist in `agent-capabilities.yaml`. Exactly one of `mode`/`agent` per node.
3. **Every node declares a repo** that exists in `repos[]`.
4. **`action` nodes are fully gated.** `approvalRequired: true` AND `riskClass` set AND
   `opsPacket` set. Approval is PRE-mutation (see "Approval" below).
5. **`ongoing-ops` nodes declare an `opsPacket`.**
6. **`dependsOn` forms a DAG.** Every referenced id exists, no self-reference, no cycles.
7. **Node ids are unique.**
8. **`rootOutcome` is a complete Outcome Contract** (intent, successSignal, hardConstraints,
   failureCondition all present).
9. **Release coverage (Gate G101).** When `rootOutcome.targetReleasePacket` names a phase
   whose `docs/releases/<phase>/features.md` is reachable, every `delivery=required` feature
   MUST be covered by some `delivery`-type node's `coversFeatures[]`. An under-scoped DAG (a
   promised required feature with no delivery node) is rejected at compile time. When the
   packet is not reachable (a supporting-repo packet, or the source-repo `framework-validate`
   run), coverage is enforced at convergence by `release-delivery-reconciliation-guard.sh`.

## Cross-Repo Execution Boundary (NON-NEGOTIABLE)

A scenario MAY span repos, but each node executes inside its target repo's OWN Bubbles
install, command registry (`.specify/memory/agents.md`), policies, `state.json`, and
artifact ownership rules. Concretely:

- Run each node's build/test/lint/deploy commands from the OWNING repo's command surface;
  never invoke one repo's CLI against another repo's tree.
- The scenario ledger records **per-repo sub-results**; it MUST NOT make one repo's
  `state.json` certify another repo's work. Certification stays validate-owned per repo.
- In a multi-root workspace, the per-repo MCP server id (`bubbles-<repo-slug>`) is what lets
  the top-level session reach each repo's tools. If a supporting repo's tools are not
  reachable, the scenario node for that repo is `blocked` — do NOT fabricate cross-repo work.
- Framework-managed files in a downstream repo are READ-ONLY (see
  [operating-baseline.md](operating-baseline.md) → Framework File Immutability).

## Approval For Action Nodes (reuse the propagate token pattern)

A host-mutating `action` node (deploy/promote/rollback) MUST gate exactly like
`bubbles.propagate` backports:

1. On first reach, the orchestrator emits `route_required` with `action: human-approval`,
   naming the target, the command, the rollback path, and the `riskClass`. It STOPS at that
   node.
2. The operator re-invokes with an approval token. Without the token the node MUST refuse —
   never silently proceed.
3. The approval token is recorded in the scenario ledger entry for that node (audit trail).
4. Approval is PRE-mutation. "All specs done" only means "ready to CONSIDER deploy" — it is
   NOT permission to mutate the host. The approval gate is the permission.

Per-action-node approval is the default for any scenario whose `targetEnvironment` is a real
host. Do not batch one token across multiple action nodes.

## Execution Model (depth-safe)

The operator invokes `bubbles.goal` or `bubbles.sprint` DIRECTLY, so the orchestrator IS the
top-level runtime that owns the operator turn (`requiresTopLevelRuntime` is satisfied because
it is not itself a subagent). Inside the loop:

- `super` is a depth-1 resolver subagent (envelope-only; no nested dispatch) — safe.
- each scenario node is a single specialist OR a single-spec mode (`devops-to-doc`,
  `product-to-planning`, `validate-only`, …) executed directly by the active runner per
  [workflow-delegation-core.md](workflow-delegation-core.md).
- nodes NEVER resolve to a fan-out `requiresTopLevelRuntime` mode (Hard Rule 1).

This keeps effective nesting depth ≤ 1 (Gate G064) and avoids the Failure-Mode-4
cross-role fabrication described in [workflow-execution-loops.md](workflow-execution-loops.md)
→ Top-level-runtime modes. If the orchestrator is itself running as a subagent without
`runSubagent`, it MUST emit `route_required` with `routingReason: top-level-runtime-required`
and `nextOwner: user-session` rather than collapsing roles.

## Preview Before Execution

Before executing the first node, the orchestrator MUST present the compiled DAG to the
operator: node order, per-node repo + mode, the aggregate risk class (the highest node
`riskClass`), and which nodes require approval. For scenarios containing an `action` node, the
orchestrator waits for the approval token at that node — it does not pre-authorize the whole
scenario.

## Runtime State

- Compiled plan (the artifact the lint validates): a per-run JSON under
  `.specify/runtime/` (e.g. `.specify/runtime/scenario-plan-<scenarioId>.json`). This path
  is runtime-gitignored; it never enters the committed tree.
- Per-attempt ledger: append one record per node attempt to
  `.specify/runtime/scenario-runs.jsonl` carrying `scenarioId`, `node`, `repo`, `mode`,
  `stepIndex`, `total`, `outcome`, `approvalToken` (action nodes only), and `evidenceRef`.
  Append-only; never rewrite past entries.
- Resume: a half-finished scenario (delivered but not yet deployed) resumes at the next
  unfinished node, trusting prior nodes' per-repo certifications instead of re-running them.

Orchestrators MUST NOT author files outside their session/runtime surface (the goal/sprint
TOOL ALLOWLIST); ledger writes go through the runtime/session tooling, not ad-hoc edits.

## Root-Outcome Verification

After all nodes reach terminal state, the orchestrator MUST verify the `rootOutcome` Outcome
Contract — demonstrate the `successSignal` with real evidence and confirm every
`hardConstraint` held — NOT merely that each node returned success. This reuses Gate **G070**
(Outcome Contract) semantics; a scenario whose nodes all passed but whose `successSignal` is
unproven or whose `hardConstraint` was violated is NOT complete.

For a **release-phase scenario** (`rootOutcome.targetReleasePacket` set), root-outcome
verification MUST additionally run
`release-delivery-reconciliation-guard.sh --phase <phase> --require-coverage` in the target
repo and treat a non-zero exit as a NON-terminal convergence state — continue (create/route
the missing specs) or end `blocked` with the guard's report, NEVER EXIT_SUCCESS. The scenario
is NOT complete while any `delivery=required` feature is unspecced, non-terminal, blocked, or
implement-self-certified (Gate **G101**). This is the teeth behind "MVP delivered": a
phase's promised required-feature set must each map to a terminal, validate-certified spec —
not merely that the nodes the orchestrator chose to create all passed.

## Forbidden Patterns

| ❌ Forbidden | ✅ Required |
|---|---|
| Adding a new `mode:` per business journey | Compose existing modes as DAG nodes; the scenario is data, not a mode |
| A node resolving to `autonomous-goal`/`autonomous-sprint`/`iterate`/`stochastic-quality-sweep`/`retro-quality-sweep`/`idea-to-release-completion` | Single-spec modes or specialists only (Hard Rule 1, Gate G064) |
| `super` compiling or executing the DAG | `super` resolves intent only; goal/sprint compile + execute |
| One repo's `state.json` certifying another repo | Per-repo validate-owned certification; ledger aggregates only |
| Treating an `action` node as "done because exit 0" | OPS packet + apply + live verify + rollback proof + validate certification |
| Deploying because "all specs are done" | Explicit approval token at the action node, PRE-mutation |
| Overloading `scenario-manifest.json` for the plan | `scenario-manifest.json` is Gherkin contracts; scenario plan lives under `.specify/runtime/` |
| Concrete repo/host/target names in framework docs | Abstract placeholders (`<product-repo>`, `<adapter-repo>`, `<target>`) per `docs/SCOPE_POLICY.md` |

## Mechanical Enforcement

- `bubbles/scripts/scenario-compile-lint.sh <scenario-json> [repo-root]` validates a
  compiled scenario DAG against every Hard Rule above. Exit 0 clean, 1 violation, 2 usage.
- `bubbles/scripts/scenario-compile-lint-selftest.sh` is the hermetic selftest (clean DAG +
  adversarial fixtures: fan-out node, ungated action node, cyclic/dangling `dependsOn`,
  duplicate id, unknown repo, missing Outcome Contract, and an under-scoped release-phase DAG
  whose `targetReleasePacket` leaves a required feature uncovered). Wired into
  `framework-validate.sh`.
- `bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root <dir> [--phase <p>] [--require-coverage]`
  reconciles `docs/releases/<phase>/features.md` `delivery=required` features against the
  delivered (terminal + validate-certified) spec truth (Gate **G101**). Run by
  `bubbles.goal`/`bubbles.sprint` at convergence for release-phase scenarios; wired into
  `framework-validate.sh` (selftest + live guard). Hermetic selftest:
  `bubbles/scripts/release-delivery-reconciliation-guard-selftest.sh`.

## Cross-References

- [workflow-delegation-core.md](workflow-delegation-core.md) — NL routing + parent-expansion + runtime depth compatibility
- [workflow-execution-loops.md](workflow-execution-loops.md) — top-level-runtime modes + Failure-Mode-4 prohibition
- [completion-governance.md](completion-governance.md) — per-repo sequential completion, validate-owned certification
- [artifact-ownership.md](artifact-ownership.md) — owner-only remediation; route foreign-artifact work
- `instructions/bubbles-deployment-target.instructions.md` — deploy adapter contract for action nodes
- `instructions/bubbles-upkeep-operations.instructions.md` — backup/monitoring cadence for ongoing-ops nodes
- `bubbles/action-risk-registry.yaml` — riskClass vocabulary for action nodes
- `bubbles/workflows/modes.yaml` — the `requiresTopLevelRuntime` set the lint forbids as nodes
