# Bubbles Control Plane Design

This document proposes the next-step architecture for Bubbles so delegation, state transitions, behavior contracts, and workflow modes are enforced by a machine-readable control plane instead of being scattered across prompt prose.

Related documents:
- [Control Plane Rollout](CONTROL_PLANE_ROLLOUT.md)
- [Control Plane Schemas](CONTROL_PLANE_SCHEMAS.md)
- [Workflow Modes](WORKFLOW_MODES.md)
- [Agent Manual](AGENT_MANUAL.md)

## Why This Change Exists

Bubbles already has strong specialist-agent boundaries, gate-driven workflows, state-transition checks, and optional tags such as `grillMode`, `tdd`, and `autoCommit`. The current framework still has seven systemic weaknesses:

1. Delegation is described in prose more often than enforced through a registry.
2. State authority is fragmented across workflow, specialists, and guard scripts.
3. User-visible behavior contracts are tracked as prose Gherkin instead of stable machine-readable scenario identities.
4. Optional execution tags are not yet a first-class repo-level policy surface with default provenance.
5. Framework-level self-validation and release hygiene are spread across individual scripts rather than described as first-class control-plane operations.
6. Run-state and framework event history are still more narrative than typed, which makes resume and postmortem analysis harder than they should be.
7. Risk level is implicit in many framework actions even though runtime teardown, external side effects, and owned mutations should be visibly classified.

The requested changes all point to the same architectural direction: Bubbles needs a real control plane.

## Design Goals

The target architecture must satisfy all of the following:

1. Every agent delegates work to the correct specialist whenever ownership crosses a specialist boundary.
2. Subagent execution returns durable work packets, not vague handoff prose.
3. Runtime defaults such as TDD, grill, auto-commit, and lockdown come from one mutable central policy source.
4. Completion state transitions are certified only by `bubbles.validate` after gates pass.
5. `bubbles.grill` can act as a bounded interactive interrogation gate when ambiguity matters.
6. Gherkin scenarios become strict, live-system, BDD-aligned acceptance contracts.
7. Lockdown behavior protects approved UI and behavior contracts from silent drift.
8. TDD becomes scenario-first by default where it materially reduces risk.
9. Bug and chaos follow-up flows default to TDD and persistent regression coverage.
10. Regression tests become protected behavior contracts that cannot drift without spec invalidation.
11. Every user-visible or externally observable behavior change must have explicit Gherkin and passing live-system BDD evidence.
12. Existing repos and active specs can adopt the control plane incrementally without blind rewrites or mixed authoritative state.
13. Concurrent sessions must coordinate shared Docker or runtime resources through an explicit lease registry so reuse is safe and teardown blast radius is bounded.
14. The framework must expose a first-class self-validation surface so shipping or upgrading Bubbles does not depend on tribal knowledge of which scripts to chain.
15. Repo-readiness guidance must stay separate from completion certification so repos can be assessed without weakening validate-owned authority.
16. Framework actions should advertise their risk class so advice, automation, and future approvals can reason about blast radius explicitly.

## Non-Goals

This design does not propose:

- replacing specialist agents with a single super-agent
- moving product-code implementation into the workflow agent
- making `bubbles.validate` responsible for every transient execution field in `state.json`
- weakening existing ownership boundaries in `bubbles/agent-ownership.yaml`

## Architecture Overview

The new control plane has eleven cooperating parts.

### 1. Agent Capability Registry

Bubbles already has a narrow ownership manifest in [bubbles/agent-ownership.yaml](../../bubbles/agent-ownership.yaml). That should evolve into a generated capability registry that answers four runtime questions:

1. Which agent owns this artifact or workflow phase?
2. Which state fields may this agent claim versus certify?
3. Which user interactions may this agent perform directly?
4. Which specialist must be invoked when work crosses ownership?

The registry should be generated from:
- agent frontmatter in `agents/bubbles.*.agent.md`
- `bubbles/agent-ownership.yaml`
- workflow phase definitions in `bubbles/workflows.yaml`

The registry is not just documentation. It becomes the runtime source for:
- workflow delegation
- illegal cross-owner write detection
- super-agent routing recommendations
- handoff-cycle validation

### 1.5. No-Hybrid Role Model

The current agent surface is easiest to reason about when every agent fits one primary operating class.

The control plane should adopt a strict no-hybrid model:

- orchestrators pick work, dispatch specialists, consume packets, and continue execution
- owners update only their owned artifacts or owned execution surfaces
- diagnostics detect, classify, and packetize findings, but do not remediate foreign-owned work inline
- certification agents certify, reopen, invalidate, and route, but do not implement fixes

Recommended classes:

- `orchestrator`: `bubbles.workflow`, `bubbles.iterate`, `bubbles.bug`
- `planning-owner`: `bubbles.analyst`, `bubbles.ux`, `bubbles.design`, `bubbles.plan`
- `execution-owner`: `bubbles.implement`, `bubbles.test`, `bubbles.docs`, `bubbles.chaos`, `bubbles.simplify`
- `diagnostic`: `bubbles.gaps`, `bubbles.harden`, `bubbles.stabilize`, `bubbles.regression`, `bubbles.security`, `bubbles.code-review`, `bubbles.system-review`, `bubbles.spec-review`
- `certification`: `bubbles.validate`, `bubbles.audit`
- `utility`: `bubbles.super`, `bubbles.status`, `bubbles.handoff`, `bubbles.commands`, `bubbles.create-skill`, `bubbles.setup`, `bubbles.recap`

This removes the ambiguous "small inline fix" loophole from diagnostic agents. If a tiny fix is warranted, the orchestrator should dispatch a narrow packet to the owning execution or planning agent instead of letting the diagnostic agent perform the repair itself.

### 2. Execution Policy Registry

Bubbles already uses `.specify/memory/bubbles.config.json` for metrics toggles. That file should become the central mutable execution policy store for repo-local defaults.

`bubbles/workflows.yaml` remains the framework capability catalog and policy schema. The mutable repo-local defaults belong in `.specify/memory/bubbles.config.json` and must be managed by:
- `bubbles/scripts/cli.sh`
- `bubbles.super`

Policy examples:
- default TDD mode
- default grill mode
- default auto-commit mode
- lockdown defaults
- regression strictness
- whether validate certification is mandatory before any promotion
- whether bug and chaos flows force scenario-first red to green

Every workflow run must record a policy snapshot plus provenance for each active mode:
- `user-request`
- `repo-default`
- `workflow-forced`
- `spec-lockdown`

The runtime coordination defaults also belong here:
- runtime lease TTL
- stale-lease detection threshold
- reuse policy for shared-compatible stacks

### 2.5. Runtime Lease Registry

Execution isolation is not enough when multiple sessions can start or reuse Docker containers, Compose projects, networks, volumes, or mutable test stores. The control plane therefore needs a runtime lease registry under `.specify/runtime/`.

The registry is responsible for:

1. recording which session currently owns or is attached to a runtime stack
2. classifying the stack as `shared-compatible`, `exclusive`, `disposable`, or `persistent-protected`
3. storing a compatibility fingerprint derived from repo state and declared runtime inputs
4. generating safe Compose project names for compatible reuse or isolated duplication
5. surfacing stale leases and active conflicts in `bubbles status`, `bubbles doctor`, and `bubbles runtime ...`

This keeps source-level parallelism (`gitIsolation`, worktrees, parallel scopes) from accidentally colliding at the container/runtime layer.

### 2.6. Workflow Run-State

The framework already records execution and certification state, but long-running or resumed work also needs a typed run-state surface that answers simpler operational questions:

- what workflow run is active right now
- which packet or continuation target is pending
- what runtime lease or shared stack the run is attached to
- whether a run is resuming, retrying, or recovering from a routed packet

This is distinct from completion authority. Run-state is about safe continuation and inspection, not promotion.

### 2.7. Framework Event Stream

Guard scripts, packets, runtime lease transitions, and policy provenance changes should be representable as typed framework events instead of only prompt prose or loose terminal output. A typed event stream enables better resume diagnostics, metrics, and later automation without weakening the existing raw-evidence rules.

Examples of useful event classes:

- `gate_passed`
- `gate_failed`
- `packet_emitted`
- `packet_consumed`
- `runtime_lease_acquired`
- `runtime_lease_released`
- `policy_snapshot_recorded`
- `certification_downgraded`

### 3. Validate-Owned Certification State

Current Bubbles guidance allows specialists to append their own phase completion metadata. That is useful for execution traceability but not strong enough for the stricter model requested here.

The control plane should split state into two layers:

- `execution`: transient claims and in-flight status written by the running workflow or specialist
- `certification`: authoritative promotion state written only by `bubbles.validate`

Examples:

Execution-owned fields:
- `currentPhase`
- `currentScope`
- `activeAgent`
- `runStartedAt`
- `pendingTransitionRequests`

Validate-certified fields:
- `status`
- `completedScopes`
- `certifiedCompletedPhases`
- `scopeProgress[*].status`
- `lockdownState`
- `invalidationLedger`

This preserves execution velocity while making promotion authority explicit.

### 4. Scenario Contract Manifest

Gherkin is the true behavior contract, but today it mostly lives in markdown. The framework needs a generated scenario manifest with stable scenario IDs.

Each scenario record should include:
- stable `scenarioId`
- owning spec and scope
- normalized Gherkin text hash
- whether it is new, changed, regression, bugfix, or lockdown-protected
- required live test type: `e2e-api` or `e2e-ui`
- linked test files and test identifiers
- last passing evidence references
- invalidation and replacement history

This makes the following enforceable:
- exact scenario-to-test mapping
- exact changed-behavior regression coverage
- immutable regression tests for non-invalidated scenarios
- lockdown protection at scenario granularity

### 5. Transition Request And Rework Packet Protocol

Subagent interactions must stop being purely narrative when they surface failed DoD, missing scenarios, or invalid state transitions.

Every specialist should return one of four structured outcomes:
- `completed_owned`
- `completed_diagnostic`
- `route_required`
- `blocked`

Rules:

- owners and execution agents return `completed_owned` when they changed their owned surface and produced concrete evidence
- diagnostic and certification agents return `completed_diagnostic` when they completed their inspection/certification responsibility without opening new work
- diagnostics MUST NOT silently repair foreign-owned artifacts and then return `completed_owned`
- `route_required` is mandatory whenever the next action belongs to another owner or executor
- `blocked` is valid only when the agent updated the relevant blocked state or packet with a concrete reason and evidence

If `route_required`, the result must include a machine-readable packet containing:
- target scope and DoD item
- target scenario IDs
- owning specialist
- required files or artifacts to touch
- required gates before the work can re-enter validation
- narrow execution hints when the fix can be isolated to a specific file, function, route, or test

If `bubbles.validate` reopens work, it should never just uncheck a box and stop. It must emit a rework packet tied to concrete scenarios, tests, and scope items. The workflow then routes the packet to the right owner and keeps running.

If a diagnostic agent finds a tiny, obvious fix, it should still emit a narrow packet. The orchestrator may then immediately dispatch that packet to the correct owner with tightly scoped context. This preserves micro-fix speed without creating hybrid agents.

### 5.5. Action Risk Classification

Framework operations are easier to reason about when each action advertises its risk class up front. The control plane should classify framework actions using a small, stable vocabulary:

- `read_only`
- `owned_mutation`
- `destructive_mutation`
- `external_side_effect`
- `runtime_teardown`

Examples:

- `doctor` is mostly `read_only`, with `owned_mutation` only when `--heal` is explicitly requested
- `framework-validate` is `read_only`
- `release-check` is `read_only`
- runtime stack cleanup is `runtime_teardown`
- hook installation is `owned_mutation`

This gives the super-agent, CLI, and future approval flows a shared safety vocabulary.

### 6. Grill Mode As An Interactive Ambiguity Gate

`bubbles.grill` should remain distinct from `bubbles.clarify`.

- `bubbles.clarify` classifies ambiguity, identifies the blocked decision, and routes the artifact change to the owning specialist.
- `bubbles.grill` pressure-tests assumptions and, when enabled, interrogates the user before irreversible design or behavior moves.

The control plane should support these grill modes:
- `off`
- `on-demand`
- `required-on-ambiguity`
- `required-for-lockdown`

`off` remains the framework default. The repo policy registry can elevate it.

### 7. Lockdown And Invalidation Model

Lockdown must operate on scenarios, not just broad spec status.

A locked scenario means:
- its linked BDD live-system tests are immutable without invalidation
- behavior and UI changes are blocked until user approval is captured through grill
- analyst-owned spec invalidation or replacement updates must exist before replacement tests are accepted
- validate must demote or invalidate certified completion before the replacement behavior can be promoted

This gives Bubbles a safe way to protect already-approved product behavior.

### 8. Existing-Spec Adoption Model

The control plane is only useful if existing repos can adopt it without corrupting in-flight work. The framework therefore needs an explicit adoption model for active specs, not just greenfield templates.

The adoption model has four required behaviors.

### 8.5. Repo-Readiness Boundary

Repo-readiness is useful, but it is not the same thing as delivery certification. The control plane should treat repo-readiness as an advisory framework-ops surface that answers questions like:

- are the command entrypoints documented and real
- do the framework-owned surfaces appear intact
- are the local instructions and operational assumptions legible enough for agents to work safely

It must not be allowed to certify scope completion, satisfy `bubbles.validate`, or silently replace scenario- and evidence-based delivery gates.

#### 8.1. Validate-Owned Certification Migration

Existing specs frequently carry stale or inflated completion claims because execution agents and manual edits both touched the same completion fields.

The migration rule is:

- legacy completion claims move into `execution.completedPhaseClaims` only when they describe work that actually ran
- authoritative completion moves into `certification.*` and is written only by `bubbles.validate`
- the top-level compatibility `status` mirrors `certification.status` and is never treated as an independent source of truth
- if a previously "done" scope or spec fails freshness, scenario, or evidence checks, validate downgrades certification first and only then routes rework

This makes reopen and invalidate behavior deterministic. Execution agents may still record what they did, but only validate decides what is actually complete.

#### 8.2. Scenario Contracts For Changed Behavior

Existing repos already contain Gherkin in `scopes.md`, but that is not enough for durable regression protection. The adoption rule is that every changed user-visible or externally observable behavior in an active scope receives a stable `SCN-*` entry in `scenario-manifest.json` before the work can be certified.

Each adopted scenario contract must define:

- a stable scenario ID that survives implementation churn until invalidated
- the owning scope and current Gherkin hash
- whether the scenario is new, changed, regression, bugfix, or lockdown-protected
- the required live test class (`e2e-ui` or `e2e-api`)
- the linked live tests and evidence references

For existing features, adoption should be selective rather than bulk-generated. Only active or newly changed behavior must be lifted into scenario contracts immediately. Historical untouched behavior can remain prose-only until a workflow reopens it.

#### 8.3. Repo-Default Policy Registry As The Sole Runtime Default Source

The framework must stop encoding repo-default grill, TDD, lockdown, regression, or certification behavior in prompt prose. Prompts may describe how policies behave, but they must not invent effective defaults.

The design rule is:

- `.specify/memory/bubbles.config.json` is the only repo-local source for mutable execution defaults
- `bubbles/workflows.yaml` defines framework capabilities and forced overrides, not repo-local preferences
- every workflow run records a `policySnapshot` with both the effective value and provenance for grill, TDD, auto-commit, lockdown, regression, and validation certification
- workflow selection, validate checks, and downstream status reporting all read from the same recorded snapshot rather than re-deriving defaults from prompts

This gives repos one place to raise discipline gradually without forking framework prompts.

#### 8.4. Artifact Freshness And Redesign Triage

Existing repos are more likely to fail because stale requirements, UX, design, and planning artifacts remain mixed into active truth than because code is missing outright. The control plane therefore needs freshness-aware workflow selection, not just stronger validation.

The design rule is:

- if active artifacts disagree but the feature direction is still fundamentally the same, use `reconcile-to-doc` or `improve-existing`
- if active artifacts no longer describe the intended feature and major flow, UX, or architecture changes are required, use `redesign-existing`
- stale active sections must be lowered into clearly marked superseded sections before new active truth is written
- stale scopes must be removed from executable scope inventories and may survive only as archival prose, never as active Test Plan or DoD structures

This turns artifact freshness from a passive lint concern into an explicit planning and workflow-selection discipline.

#### 8.5. Adoption Boundary For Dirty Repos

Because many downstream repos will already have unrelated local changes, the adoption design must be non-destructive:

- framework-managed assets may be refreshed in place
- repo-owned artifacts such as `constitution.md`, `agents.md`, existing specs, and feature docs require targeted migration, not blanket overwrite
- missing control-plane bootstrap surfaces such as `.specify/memory/bubbles.config.json` may be added safely
- migration work should prefer additive introduction of `policySnapshot`, `scenario-manifest.json`, and validate-owned certification over wholesale artifact regeneration unless `redesign-existing` is chosen

## Behavioral Laws

The control plane should enforce these framework laws.

### Delegation Law

If work crosses a registered ownership boundary, the current agent must delegate to the owning specialist. It may not quietly perform the foreign-owned action itself.

### Owner-Only Remediation Law

Only the owning planning or execution specialist may modify its owned surface.

- diagnostic agents do not repair code, tests, planning artifacts, docs, or state directly
- certification agents do not implement fixes
- orchestrators do not implement fixes directly; they dispatch the correct owner

### Certification Law

Only `bubbles.validate` may certify completion state transitions such as:
- DoD item completed
- scope done
- spec done
- lockdown invalidated or re-certified

### Scenario Law

Every user-visible or externally observable behavior change must:
- exist as one or more Gherkin scenarios
- map to one or more live-system BDD tests
- pass those tests before certification

### Regression Law

Regression tests attached to a non-invalidated scenario are protected artifacts. They cannot be rewritten to fit new behavior until the spec and scenario contract are invalidated and replaced.

### TDD Law

When TDD is active, the required order is:
1. scenario exists
2. targeted failing proof exists
3. implementation changes
4. targeted proof goes green
5. broader regression stays green
6. validate certifies

For bug and chaos flows, this should become the default rather than an opt-in.

### Micro-Fix Dispatch Law

Tiny fixes remain allowed, but only through orchestrator dispatch.

Required pattern:

1. a diagnostic agent emits a narrow route packet
2. the orchestrator immediately invokes the correct owner with that packet
3. the owner performs the change and returns owned evidence
4. validation or audit resumes from the packet context

This keeps the loop fast while preserving strict ownership.

### Child Workflow Law

Only orchestrators may invoke child workflows, and child workflow depth must be bounded.

- allowed callers: `bubbles.workflow`, `bubbles.iterate`, `bubbles.bug`
- non-orchestrator agents may emit packets but may not spawn workflows directly
- child workflow depth should be limited to 1
- child workflows inherit policy snapshot, target context, and packet references from the parent orchestrator

## Mapping The Requested Changes To The Design

| Requested Change | Design Response |
|---|---|
| 1. Always delegate to specialists | Agent capability registry plus a new delegation gate |
| 2. Subagents must not leave unfinished work | Structured transition and rework packets routed by workflow |
| 3. Central defaults and mode reporting | Execution policy registry in `.specify/memory/bubbles.config.json` with provenance recording |
| 4. Only validate changes spec state | Split execution state from validate-certified promotion state |
| 5. Grill mode for ambiguity/user confirmation | Grill mode with `required-on-ambiguity` and `required-for-lockdown` values |
| 6. Strict Gherkin-to-E2E validation | Scenario contract manifest plus live BDD enforcement |
| 7. Lockdown mode | Scenario-level lockdown plus invalidation ledger and grill confirmation |
| 8. Default TDD for changed Gherkin | Policy default for scenario-first red to green |
| 9. TDD main mode for bug/chaos | Mode-level forced TDD defaults for `bugfix-fastlane` and chaos follow-up |
| 10. Regression tests cannot drift without spec change | Scenario-linked regression contract protection |
| 11. All behavior changes need Gherkin and BDD E2E | Scenario law plus certification guard |
| 12. No hybrid agents | Owner-only remediation plus orchestrator-driven micro-fix dispatch |
| 13. Child workflows only where safe | Orchestrator-only child workflow invocation with bounded depth |
| 14. Existing active specs adopt safely | Existing-spec adoption model with selective scenario lift, repo-default policy registry, and freshness triage |

## Proposed New Gates

The current gate registry ends at G061. This design would add the following framework gates:

- `G042 artifact_ownership_enforcement_gate (absorbs former G042)` — foreign-owned work must route through the registered specialist
- `G055 policy_provenance_gate` — active modes must record value plus source
- `G056 validate_certification_gate` — only validate may certify promotion state
- `G057 scenario_manifest_gate` — every changed behavior must resolve to stable scenario IDs and live tests
- `G058 lockdown_gate` — locked scenarios cannot change without approval and invalidation
- `G059 regression_contract_gate` — protected regression tests cannot drift without scenario invalidation
- `G060 scenario_tdd_gate` — when TDD is active, targeted red evidence must exist before green certification
- `G061 rework_packet_gate` — route-required findings must produce structured packets, not narrative-only handoffs
- `G042 artifact_ownership_enforcement_gate` (absorbs former G042) — only owning planning/execution specialists may modify their surfaces; diagnostics must route
- `G063 concrete_result_gate` — every agent invocation must end with `completed_owned`, `completed_diagnostic`, `route_required`, or `blocked` plus the required concrete payload
- `G064 child_workflow_depth_gate` — only orchestrators may invoke child workflows and nesting depth may not exceed 1

## Tradeoffs And Guardrails

### What Gets Better

- More deterministic agent behavior
- Less hidden cross-owner work
- Stronger auditability of why a mode was active
- Better preservation of approved behavior contracts
- Stronger regression discipline
- Cleaner separation between diagnosis, execution, and certification

### What Gets More Expensive

- More schema and generation logic
- More state-management complexity
- More explicit invalidation workflow for approved behavior changes
- Some additional friction when teams want to intentionally change locked behavior quickly
- More orchestrator logic because micro-fix speed now flows through packet dispatch instead of inline diagnostic edits

### Required Guardrail

Do not move everything into validate. Validate should certify promotion state and invalidation decisions, not become a giant universal executor.

## Recommended End State

Bubbles should operate as a policy-driven, registry-backed workflow system with these properties:
- specialist delegation is enforced
- hybrid remediators are eliminated in favor of owner-only remediation
- workflow defaults are centrally managed
- scenario contracts are durable and machine-readable
- validate certifies completion and reopening
- lockdown protects approved behavior from silent drift
- bug and chaos default to scenario-first TDD
- regression suites act as spec-backed behavior contracts
- tiny-fix speed is preserved through narrow orchestrator dispatch rather than diagnostic inline edits

That architecture satisfies the full requested direction without collapsing the current specialist model.