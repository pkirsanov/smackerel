# Bubbles Existing Repo Adoption

This guide explains how to adopt the Bubbles control plane in a repo that already has active specs, local governance files, and in-flight work.

Related documents:
- [Control Plane Design](CONTROL_PLANE_DESIGN.md)
- [Control Plane Rollout](CONTROL_PLANE_ROLLOUT.md)
- [Control Plane Schemas](CONTROL_PLANE_SCHEMAS.md)
- [Workflow Modes](WORKFLOW_MODES.md)
- [Interop Migration Guide](INTEROP_MIGRATION.md)
- [Generated Interop Migration Matrix](../generated/interop-migration-matrix.md)

## Purpose

The control plane is not just a greenfield template system. Existing repos need a safe path that:

- preserves repo-owned artifacts
- avoids blanket rewrites in dirty worktrees
- separates execution claims from authoritative completion state
- lifts active changed behavior into stable scenario contracts
- uses artifact freshness to decide whether to reconcile, improve, or redesign

## Adoption Rules

1. Refresh framework-managed assets freely.
2. Never blanket-overwrite repo-owned artifacts such as `constitution.md`, `agents.md`, feature specs, or project docs.
3. Add missing control-plane bootstrap surfaces first, but only through the Bubbles installer/bootstrap path or framework refresh workflow.
4. Migrate only active specs immediately; untouched historical specs can convert later.
5. Downgrade stale certification before claiming progress.

Downstream framework-authoring rule:
- Consumer repos must not hand-edit framework-managed Bubbles files in place.
- Record requested framework changes under `.github/bubbles-project/proposals/` or via `bubbles framework-proposal <slug>`.
- Implement the framework change in the Bubbles source repo, then refresh downstream installs.

Bootstrap provenance rule:
- `.specify/memory/bubbles.config.json` is a Bubbles bootstrap artifact and must be created by install/bootstrap, not by hand.
- `.github` framework drift should be reconciled through `bubbles.setup`, not ad hoc edits in downstream repos.
- `.github/bubbles/.checksums` is the downstream checksum snapshot for installed framework-managed files. If it reports drift, fix the framework upstream instead of patching local copies.

## Phase A: Framework Baseline

Checklist:
- Confirm `.github/bubbles/.version` matches the intended upstream version.
- Confirm framework-managed registries and manifests exist.
- Confirm the repo has the latest shared agents, prompts, instructions, and skills.

Expected outcome:
- the shared framework layer is current before project-owned artifact migration begins

## Phase B: Repo Policy Bootstrap

Checklist:
- Ensure `.specify/memory/bubbles.config.json` exists.
- If it is missing, rerun install/bootstrap from the Bubbles repo instead of creating the file manually.
- Ensure repo-default grill, TDD, lockdown, regression, auto-commit, and certification behavior live there.
- Ensure the first control-plane-aware workflow run records `policySnapshot` provenance in `state.json`.

Expected outcome:
- runtime defaults come from one repo-local registry instead of prompt prose

## Adoption Profiles

Adoption profiles are maturity-tier guidance presets. They change bootstrap emphasis, doctor messaging, and repo-readiness severity without changing certification authority, scenario contracts, or artifact ownership.

| Profile | Intended Audience | Guidance Posture | Required Docs | Suggested First Commands |
| --- | --- | --- | --- | --- |
| `foundation` | Brownfield repos adopting Bubbles with active local work and partial framework bootstrap | Advisory-first onboarding and smaller first-step cleanup | `docs/guides/CONTROL_PLANE_ADOPTION.md`, `docs/recipes/ask-the-super-first.md` | `bash bubbles/scripts/cli.sh repo-readiness .`, `bash bubbles/scripts/cli.sh doctor` |
| `delivery` | Teams already ready to operate normal Bubbles packets and workflow surfaces | Standard readiness checklist and default control-plane posture | `docs/guides/CONTROL_PLANE_ADOPTION.md`, `docs/guides/WORKFLOW_MODES.md`, `docs/recipes/ask-the-super-first.md` | `bash bubbles/scripts/cli.sh profile show`, `bash bubbles/scripts/cli.sh repo-readiness .` |
| `assured` | Teams that want stronger early guardrail visibility before scaling delivery | Guardrail-forward readiness and earlier hygiene pressure | `docs/guides/CONTROL_PLANE_ADOPTION.md`, `docs/guides/WORKFLOW_MODES.md`, `docs/recipes/ask-the-super-first.md` | `bash bubbles/scripts/cli.sh profile show`, `bash bubbles/scripts/cli.sh repo-readiness . --profile assured` |

Invariant for every profile:
- `bubbles.validate` remains the only certification authority.
- Scenario contracts and completion gates stay full-strength.
- Profile changes are repo-local state in `.specify/memory/bubbles.config.json`, not framework-owned trust metadata.

To inspect or change the active profile:

```bash
bash bubbles/scripts/cli.sh profile show
bash bubbles/scripts/cli.sh profile list
bash bubbles/scripts/cli.sh profile set foundation
bash bubbles/scripts/cli.sh policy status
```

## Phase C: Active Spec Inventory

Checklist:
- List the specs currently in progress, blocked, or recently reopened.
- Identify which scopes are actively changing user-visible or externally observable behavior.
- Identify which specs still use legacy completion state or lack version 3 `state.json` structure.

Expected outcome:
- migration effort is focused on active truth, not every historical artifact at once

## Phase D: Certification Migration

Checklist:
- Convert active specs to `state.json` version 3.
- Move claims of executed work into `execution.*`.
- Move authoritative completion into `certification.*`.
- Ensure top-level compatibility `status` mirrors `certification.status`.
- If prior completion is stale or unsupported, reopen it instead of preserving a false green state.

Expected outcome:
- only `bubbles.validate` certifies completion state

## Phase E: Scenario Contract Lift

Checklist:
- Create `scenario-manifest.json` for each active feature that is changing user-visible or externally observable behavior.
- Add stable `SCN-*` entries only for active changed behavior.
- Link each scenario to the required live-system tests and evidence refs.
- Mark regression-required and lockdown-protected scenarios explicitly.

Expected outcome:
- changed behavior has durable machine-readable acceptance contracts without forcing bulk migration of untouched history

## Phase F: Artifact Freshness Triage

Checklist:
- Reconcile active `spec.md`, `design.md`, and `scopes.md` so they present one active truth.
- Lower invalid sections into clearly marked superseded appendices when historical context still matters.
- Remove stale scopes from executable scope inventories.
- Choose the right workflow mode:
  - `reconcile-to-doc` for stale truth cleanup
  - `improve-existing` for evolutionary enhancement
  - `redesign-existing` for major flow, UX, or architecture rewrites

Expected outcome:
- active specs stop mixing current intent with stale executable instructions

## Phase G: Validation And Promotion

Checklist:
- Run freshness, traceability, and state-transition guards.
- Confirm `policySnapshot`, `scenario-manifest.json`, and version 3 `state.json` are coherent.
- Confirm open transition requests and rework packets are closed before certification.
- Route final completion through `bubbles.validate`.

Expected outcome:
- migrated specs can promote safely under the new control-plane rules

## Repo Assessment Worksheet

Use this for each downstream repo you migrate.

```markdown
Repo: <name>
Framework version current: yes/no
Policy registry present: yes/no
Active specs inventoried: yes/no
Version 3 state migrated for active specs: yes/no
Scenario contracts added for active changed behavior: yes/no
Freshness triage complete: yes/no
Validate-owned certification active: yes/no
Main blockers:
- ...
```

## Migration Strategy Guidance

Prefer additive migration when:
- the repo is dirty with unrelated local changes
- framework files are already current
- the main gap is missing control-plane bootstrap or missing version 3 state in active specs

Prefer redesign workflow when:
- active requirements, UX, design, and scopes all contradict intended behavior
- stale scopes would otherwise remain executable after migration
- bulk scenario invalidation is required because approved behavior is changing substantially

## Interop Migration Path

Use the generated migration matrix and the interop migration guide when a repo already carries Claude Code, Roo Code, Cursor, or Cline assets.

1. Start with review-only intake so the repo snapshots external assets into `.github/bubbles-project/imports/**` without mutating framework-managed files.
2. Use supported apply only for explicit project-owned targets recorded in the import manifest: imported instructions, additive recommendations in `.specify/memory/agents.md`, project-owned helper paths under `scripts/`, and any generated project-owned migration skill.
3. Treat workflow-mode requests, framework-surface edits, and file collisions as proposal-only outcomes. Those stay reviewable under `.github/bubbles-project/proposals/**` and never become a back door into `.github/bubbles/**`.

The benchmark narrative lives in generated docs, not hand-maintained competitor prose:
- [Competitive Capabilities](../generated/competitive-capabilities.md)
- [Interop Migration Matrix](../generated/interop-migration-matrix.md)