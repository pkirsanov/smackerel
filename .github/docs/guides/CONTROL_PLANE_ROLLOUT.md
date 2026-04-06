# Bubbles Control Plane Rollout

This document turns the control-plane design into an executable rollout plan and records the sequencing behind the active control-plane surfaces.

Related documents:
- [Control Plane Design](CONTROL_PLANE_DESIGN.md)
- [Control Plane Schemas](CONTROL_PLANE_SCHEMAS.md)
- [Existing Repo Adoption](CONTROL_PLANE_ADOPTION.md)

## Rollout Strategy

The changes should land in phases. The order matters because later changes depend on central registries and validate-owned certification.

Implementation principle:
- establish machine-readable control surfaces first
- move authority second
- tighten behavior enforcement third
- flip defaults only after enforcement exists

## Workstreams

The rollout covers the control-plane changes across ten workstreams. Several of these surfaces are already active and mechanically enforced; the remaining value of this document is sequencing, adoption guidance, and alignment work.

1. Specialist delegation, capability discovery, and no-hybrid role enforcement
2. Central default policy and mode provenance
3. Validate-owned certification state
4. Result envelope, transition packet, and rework packet enforcement
5. Scenario contract, BDD enforcement, and regression immutability
6. Grill mode and lockdown behavior governance
7. TDD-default workflow behavior for bug, chaos, and then broader delivery modes
8. Framework self-validation and release hygiene
9. Typed run-state and framework event surfaces
10. Action-risk classification and repo-readiness boundary enforcement

## Implementation Surface Matrix

The rollout phases describe sequencing. The matrix below translates the design into concrete implementation surfaces so the work can be scheduled and reviewed by file family.

### CLI And Bootstrap Surfaces

Primary files:
- `bubbles/scripts/cli.sh`
- `install.sh`
- `docs/guides/INSTALLATION.md`

Tasks:
- add `bubbles policy status|get|set|reset` support
- add bootstrap checks for missing `.specify/memory/bubbles.config.json`
- distinguish framework-managed refresh from repo-owned artifact migration
- expose a doctor/status path that reports missing policy registry, missing version 3 state, and missing `scenario-manifest.json` for active changed specs
- keep `framework-validate` and `release-check` visible as first-class framework-source operations

### Guard And Lint Surfaces

Primary files:
- `bubbles/scripts/state-transition-guard.sh`
- `bubbles/scripts/traceability-guard.sh`
- `bubbles/scripts/artifact-freshness-guard.sh`
- `bubbles/scripts/agent-ownership-lint.sh`

Tasks:
- enforce validate-owned certification as the sole promotion authority
- fail migrated specs that still mix legacy completion authority with version 3 certification
- require `policySnapshot` provenance on control-plane-aware runs
- require stable `SCN-*` entries plus linked live tests for active changed behavior
- block stale scopes from remaining executable after freshness reconciliation

### Orchestrator Prompt Surfaces

Primary files:
- `agents/bubbles.workflow.agent.md`
- `agents/bubbles.iterate.agent.md`
- `agents/bubbles.super.agent.md`
- `agents/bubbles.bug.agent.md`

Tasks:
- teach workflow selection to route existing-feature work through `reconcile-to-doc`, `improve-existing`, or `redesign-existing` based on freshness and redesign intent
- make `bubbles.super` surface repo-default policy registry values rather than prompt-implied defaults
- require orchestrators to seed `policySnapshot` on the first migrated run
- require orchestrators to treat missing `scenario-manifest.json` for active changed behavior as bootstrap debt, not optional follow-up

### Planning And Certification Prompt Surfaces

Primary files:
- `agents/bubbles.plan.agent.md`
- `agents/bubbles.validate.agent.md`
- `agents/bubbles.audit.agent.md`
- `agents/bubbles.regression.agent.md`

Tasks:
- make `bubbles.plan` perform selective scenario lift for active changed scopes instead of bulk scenario generation
- make `bubbles.validate` downgrade stale certification before recertifying migrated specs
- make audit/regression consume the same scenario contract source and freshness rules
- make validate reject any active migrated spec whose `certification.*` state is not coherent with actual scope status and evidence

### Shared Templates And Governance Surfaces

Primary files:
- `agents/bubbles_shared/feature-templates.md`
- `agents/bubbles_shared/scope-workflow.md`
- `agents/bubbles_shared/artifact-freshness.md`
- `agents/bubbles_shared/completion-governance.md`

Tasks:
- add explicit migration recipes for legacy state to version 3
- document selective adoption of `scenario-manifest.json`
- document when additive bootstrap is sufficient versus when redesign workflow is required
- keep certification, freshness, and scenario-contract rules aligned across templates and guard docs

### Documentation Surfaces

Primary files:
- `docs/guides/CONTROL_PLANE_DESIGN.md`
- `docs/guides/CONTROL_PLANE_SCHEMAS.md`
- `docs/guides/CONTROL_PLANE_ADOPTION.md`
- `docs/guides/WORKFLOW_MODES.md`
- `docs/guides/AGENT_MANUAL.md`

Tasks:
- keep design, rollout, schema, and adoption guidance aligned
- document the no-destructive migration rule for dirty downstream repos
- document the difference between framework refresh and project-owned artifact migration
- document freshness-aware workflow selection for existing features
- document repo-readiness as advisory framework ops rather than delivery certification

## Phase 0: Baseline And Inventory

### Goal

Freeze the current framework surface so the redesign can be landed without guessing.

### Deliverables

- current-state inventory of:
  - agent frontmatter
  - `bubbles/agent-ownership.yaml`
  - workflow phases and optional tags in `bubbles/workflows.yaml`
  - guard scripts under `bubbles/scripts/`
- explicit compatibility matrix showing which current files consume:
  - `state.json`
  - `workflowMode`
  - `completedPhases`
  - `completedScopes`
  - optional tags

### Exit Criteria

- the current state shape is documented
- consumers of `state.json` are enumerated before schema changes begin

## Phase 1: Agent Capability Registry

### Goal

Replace delegation-by-prose with delegation-by-registry.

### Build

- add generated `bubbles/agent-capabilities.yaml`
- generate it from:
  - `agents/bubbles.*.agent.md`
  - `bubbles/agent-ownership.yaml`
  - `bubbles/workflows.yaml`
- update orchestrators and super to consult the registry at runtime
- add `G042 artifact_ownership_enforcement_gate (absorbs former G042)`
- extend ownership lint to validate:
  - every workflow phase has an owning agent
  - every specialist declares exactly one primary class: orchestrator, planning-owner, execution-owner, diagnostic, certification, or utility
  - every foreign-owned artifact route has a target owner
  - only orchestrators may invoke child workflows
  - diagnostic agents are not allowed to advertise inline remediation rights

### Files Likely Touched

- `bubbles/agent-ownership.yaml`
- new `bubbles/agent-capabilities.yaml`
- `agents/bubbles.workflow.agent.md`
- `agents/bubbles.iterate.agent.md`
- `agents/bubbles.super.agent.md`
- `bubbles/scripts/agent-ownership-lint.sh`
- `bubbles/workflows.yaml`

### Exit Criteria

- workflow and iterate reject illegal foreign-owner execution
- super recommendations come from registry lookup, not only free-form prompt heuristics
- handoff-cycle and ownership lint include capability validation
- no-hybrid role classification is mechanically enforced

## Phase 2: Execution Policy Registry And CLI Surface

### Goal

Move runtime defaults out of scattered prompt defaults and into one mutable repo-local policy store.

### Build

- expand `.specify/memory/bubbles.config.json` into the central execution policy registry
- extend `bubbles/scripts/cli.sh` with a `policy` command family:
  - `policy status`
  - `policy get <key>`
  - `policy set <key> <value>`
  - `policy reset <key>`
- teach `bubbles.super` to surface and modify policy defaults
- add mode provenance recording to workflow outputs and state snapshots
- add `G055 policy_provenance_gate`

### Policy Defaults To Add

- `tdd.mode`
- `grill.mode`
- `autoCommit.mode`
- `lockdown.default`
- `regression.immutability`
- `validate.certificationRequired`
- `bugfixFastlane.forceTdd`
- `chaos.forceTdd`

### Exit Criteria

- policy defaults can be managed by CLI and `bubbles.super`
- every run records active mode provenance
- no specialist relies on hardcoded repo-level defaults for grill, TDD, or auto-commit

## Phase 2.5: Existing-Repo Bootstrap And Safe Adoption

### Goal

Bring already-active repos onto the control plane without blind rewrites, mixed authority, or blanket regeneration of project-owned artifacts.

### Build

- require `.specify/memory/bubbles.config.json` in every adopted repo before control-plane-aware workflow execution
- add bootstrap checks that distinguish framework-managed surfaces from repo-owned artifacts
- document selective adoption rules for active specs:
  - add `policySnapshot` on first control-plane-aware run
  - create `scenario-manifest.json` only for changed or active behavior, not for untouched historical scope inventory
  - migrate completion authority to validate-owned `certification.*` before allowing new promotion
- teach workflow mode selection to prefer:
  - `improve-existing` when the feature is directionally correct and needs enhancement
  - `reconcile-to-doc` when stale truth must be cleaned up before further execution
  - `redesign-existing` when active requirements, UX, design, and scopes all need reconciliation before delivery

### Enforcement Changes

- dirty repos may receive additive control-plane bootstrap surfaces, but repo-owned artifacts must not be overwritten implicitly
- validate must downgrade stale completion claims before recertifying migrated specs
- artifact-freshness checks run before certification for migrated active specs so stale scopes cannot remain executable

### Exit Criteria

- repos can adopt the control plane by adding missing bootstrap surfaces instead of rerendering all local artifacts
- workflow mode selection reflects freshness and redesign intent instead of treating all existing-feature work as ordinary implementation
- migrated active specs no longer mix execution claims and authoritative completion state

## Phase 3: Validate-Owned Certification State

### Goal

Separate execution claims from certified completion state.

### Build

- evolve `state.json` to version 3
- split state into:
  - `execution`
  - `certification`
  - `transitionRequests`
  - `reworkQueue`
  - `policySnapshot`
- update specialists so they can submit claims and transition requests but not certify completion
- update `bubbles.validate` to become the only certification writer
- update workflow finalize logic to wait for validate certification before promotions
- add `G056 validate_certification_gate` and `G061 rework_packet_gate`

### Migration Rule

Maintain backward readers temporarily so existing scripts can read legacy state while the new fields are introduced. Once all core scripts are updated, make version 3 mandatory.

### Exit Criteria

- specialists no longer directly promote scopes or specs
- validate certifies done, reopened, invalidated, and locked states
- state-transition-guard reads certification state, not just raw execution claims

## Phase 4: Result Envelope And Packet Enforcement

### Goal

Make every agent invocation return a concrete machine-readable outcome and remove narrative-only handoffs.

### Build

- add a specialist result envelope schema shared by agents and child workflows
- normalize allowed outcomes:
  - `completed_owned`
  - `completed_diagnostic`
  - `route_required`
  - `blocked`
- upgrade existing `ROUTE-REQUIRED` output into the canonical packet contract
- teach orchestrators to consume envelopes and continue automatically
- implement orchestrator-owned micro-fix dispatch: diagnostics emit narrow packets, orchestrators immediately invoke the correct owner
- add `G042 artifact_ownership_enforcement_gate` (absorbs former G042), `G063 concrete_result_gate`, and `G064 child_workflow_depth_gate`

### Exit Criteria

- every specialist and child workflow returns a concrete result envelope
- diagnostics never fix foreign-owned work inline
- orchestrators can preserve tiny-fix speed by dispatching narrow packets immediately
- child workflow invocation is limited to orchestrators and bounded depth

## Phase 5: Scenario Contract Manifest And BDD Enforcement

### Goal

Make Gherkin scenarios durable machine-readable contracts.

### Build

- add generated `scenario-manifest.json` per feature
- assign stable scenario IDs
- store:
  - normalized scenario hash
  - owning spec and scope
  - required live test class
  - linked tests and evidence
  - regression-required flag
  - lockdown flag
- update planning guidance so new scopes generate scenario IDs
- update traceability and state-transition guards to consume scenario manifests
- add `G057 scenario_manifest_gate`

### Enforcement Changes

- changed behavior is invalid unless it has scenario IDs
- scenario IDs must map to live-system BDD tests
- validate cannot certify without passing scenario-linked evidence

### Exit Criteria

- scenario-to-test linkage is machine-checked
- live-system BDD evidence is required for changed or new user-visible behavior
- non-scenario behavior changes fail validation

## Phase 6: Regression Immutability And Bug/Chaos TDD Default

### Goal

Make scenario-linked regression tests durable contracts and force safer red-to-green behavior where risk is highest.

### Build

- mark scenario-linked regression tests as protected by default
- add scenario invalidation requirements before protected regression tests can change
- strengthen `bubbles.regression` to detect protected-test drift
- force TDD for:
  - `bugfix-fastlane`
  - chaos-discovered defects routed into deterministic follow-up
- add `G059 regression_contract_gate` and `G060 scenario_tdd_gate`

### Enforcement Changes

- bug work must start with failing scenario proof
- chaos findings must be converted into deterministic regression scenarios before completion
- protected regression tests cannot be weakened unless the scenario contract is invalidated first

### Exit Criteria

- bug and chaos flows default to scenario-first TDD
- regression test drift without invalidation is blocked
- validate and regression share the same scenario contract source

## Phase 7: Grill Mode And Lockdown Approval Flow

### Goal

Protect approved behavior and require explicit user interrogation where ambiguity or lockdown applies.

### Build

- introduce grill policy modes:
  - `off`
  - `on-demand`
  - `required-on-ambiguity`
  - `required-for-lockdown`
- add scenario-level lockdown metadata and approval records
- add invalidation ledger entries for changed locked behavior
- teach workflow to call `bubbles.grill` when:
  - policy requires it
  - a locked scenario is changing
  - behavior ambiguity blocks safe design
- add `G058 lockdown_gate`

### Enforcement Changes

For locked behavior:
1. workflow routes to grill for user confirmation
2. analyst updates or invalidates the scenario contract
3. plan updates scopes/tests
4. implementation proceeds
5. validate certifies the replacement behavior

### Exit Criteria

- locked scenarios cannot drift silently
- grill is the explicit approval gate for locked or ambiguous behavior changes
- replacement behavior requires invalidation plus new scenario-linked BDD evidence

## Phase 8: Default Expansion Across Delivery Modes

### Goal

Once enforcement is real, move from opt-in modes to safer defaults.

### Build

- make scenario-first TDD the default for:
  - bugfix-fastlane
  - chaos follow-up
  - regression-driven rework
- evaluate making TDD default for all modes that change Gherkin scenarios
- keep grill off by framework default but allow repo-level elevation via policy registry
- document the new control-plane laws in:
  - `README.md`
  - `docs/guides/WORKFLOW_MODES.md`
  - `docs/guides/AGENT_MANUAL.md`
  - `docs/CHEATSHEET.md`

### Exit Criteria

- defaults align with the new enforcement model
- docs describe provenance-driven defaults, validate certification, and protected scenario contracts

## Proposed Gate Rollout Order

| Phase | Gates To Add |
|---|---|
| 1 | G042 |
| 2 | G055 |
| 3 | G056, G061 |
| 4 | G057 |
| 5 | G059, G060 |
| 6 | G058 |

## Mapping Requested Changes To Phases

| Requested Change | Phase Coverage |
|---|---|
| 1. Specialist delegation | Phases 1 and 3 |
| 2. Finish work through structured handoffs | Phases 3 and 4 |
| 3. Central defaults and reporting | Phases 2 and 2.5 |
| 4. Only validate certifies state | Phases 2.5 and 3 |
| 5. Grill mode | Phase 6 |
| 6. Strict Gherkin and live BDD E2E | Phase 4 |
| 7. Lockdown mode | Phase 6 |
| 8. TDD default for changed Gherkin | Phases 5 and 7 |
| 9. TDD for bug and chaos | Phase 5 |
| 10. Regression immutability | Phase 5 |
| 11. Every behavior change gets scenarios and live BDD | Phase 4 |
| 12. Safe adoption for active specs and redesign triage | Phase 2.5 |

## Recommended Delivery Sequence

Deliver the phases in this order:

1. Phase 0 and Phase 1 together
2. Phase 2
3. Phase 2.5
4. Phase 3
5. Phase 4
6. Phase 5
7. Phase 6
8. Phase 7

Do not flip repo defaults before the corresponding gates exist. For example, do not make TDD the default everywhere until scenario manifests and protected regression enforcement are already in place.

## Success Criteria

The rollout is complete when all of the following are true:

- delegation is resolved through a registry
- defaults are managed centrally by CLI and super
- existing repos can adopt missing control-plane surfaces without blanket rewrite of repo-owned artifacts
- validate alone certifies promotion state
- changed behavior is represented as stable scenario contracts
- live BDD evidence is mandatory for changed observable behavior
- bug and chaos flows are scenario-first by default
- locked scenarios cannot change without approval and invalidation
- regression tests for approved behavior are immutable until the spec changes
- workflow selection uses freshness-aware triage between reconcile, improve, and redesign for existing features

At that point, the requested DNA change is real rather than aspirational.