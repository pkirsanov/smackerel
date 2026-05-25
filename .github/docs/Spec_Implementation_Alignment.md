# Spec Implementation Alignment

This document defines the durable contract for keeping hardened specs, implementation work, and maintenance reviews aligned. It is framework behavior, not a Bubbles source-repo execution packet; the source repo itself does not keep persistent `specs/` artifacts.

## Alignment Model

Bubbles treats planning truth, implementation truth, and published docs as separate but linked surfaces:

| Surface | Role | Durable Location |
|---------|------|------------------|
| Planning packet | Declares requirements, design, scopes, and scenario contracts while work is active | Downstream or fixture `specs/` packets, not the Bubbles source repo |
| Implementation evidence | Proves code, runtime, config, contract, test, or docs changed outside planning bookkeeping | Git diff, report Code Diff Evidence, guard output |
| Published truth | Explains current framework behavior for maintainers and downstream users | `docs/`, `README.md`, shared governance docs, scripts, workflows, release manifest |

The Bubbles source repository publishes durable behavior into docs and framework assets. Downstream product repositories may keep persistent `specs/` execution packets for their own work.

## State Linkage Fields

State schema v3 includes explicit linkage and revalidation fields. Missing linkage must be represented explicitly, not inferred through hidden defaults.

```json
{
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": null,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": [],
  "certifiedAt": null,
  "requiresRevalidation": false
}
```

| Field | Meaning |
|-------|---------|
| `linkedImplementationSpec` | Planning packets use this to name the implementation spec that consumes them. |
| `linkedPlanningPacket` | Implementation specs use this to name the planning packet they implement. |
| `planningOnly` | `true` only when a packet intentionally produces no implementation delivery. |
| `planningOnlyJustification` | Required when `planningOnly:true`; otherwise null. |
| `specDependsOn` | Explicit dependency list. Empty array means no declared dependencies. |
| `certifiedAt` | Certification timestamp used by post-certification edit checks. |
| `requiresRevalidation` | Set when dependencies or certified planning contracts changed. |

`state-linkage-backfill.sh` adds these fields to existing state files and is idempotent.

## G087 Planning Packet Linkage

`planning-packet-linkage-guard.sh` enforces:

- `specs_hardened` packets with `planningOnly:false` must have a non-empty `linkedImplementationSpec`.
- Linked implementation paths must resolve to real spec directories with `state.json`.
- `planningOnly:true` requires a non-empty `planningOnlyJustification`.
- Done implementation specs linked from a planning packet must point back through `linkedPlanningPacket`.
- Archived implementation targets are not valid delivery links.

## G088 Post-Certification Edit Guard

`post-cert-spec-edit-guard.sh` enforces:

- Done specs must carry a parseable top-level `certifiedAt`.
- Post-certification edits to `spec.md`, `design.md`, `scopes.md`, `scopes/_index.md`, or `scopes/*/scope.md` are violations unless the spec is demoted, marked `requiresRevalidation:true`, or recertified by a current spec-review pass.
- Legacy read-only `done_with_concerns` may be inspected only for migration compatibility; touched or recertified legacy specs must normalize to `done` plus observations or `blocked`.

## G089 Inter-Spec Dependency Guard

`inter-spec-dependency-guard.sh` enforces:

- Every `specDependsOn[]` entry resolves to a real spec directory with `state.json`.
- Stable dependencies are `done` specs. Old untouched legacy `done_with_concerns` specs are accepted only with `legacyStatusCompatibility:true`.
- Missing, malformed, unfinished, archived, cyclic, or newly recertified legacy statuses fail unless the dependent is already marked `requiresRevalidation:true` for the dependency drift.
- `inter-spec-dependency-revalidation.sh` propagates demotions by marking direct and transitive dependents with `requiresRevalidation:true`.

## Spec Review Default And Improve-Existing Route

Done-ceiling delivery modes run a spec-review pass before implementation by default unless a mode explicitly opts out. When `bubbles.spec-review` returns `MAJOR_DRIFT` or `OBSOLETE` for a `done` spec, the orchestrator routes to the existing `improve-existing` workflow for that exact spec.

This route is mandatory because severe drift on a certified spec is a stale contract problem, not a report-only docs note.

## G091 Planning Workflow Chain

`planning-workflow-chain-guard.sh` enforces the canonical planning chain for delivery-capable planning/bootstrap/fallback paths:

```text
bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan
```

The guard inspects `workflows.yaml`, bootstrap agents, improvement prelude profiles, delivery-quality constraints, auto-escalation inline actions, and active prompt/shared documentation text. `design -> plan` shortcuts are valid only when explicitly marked as forbidden examples or when machine-readable mode metadata proves the mode is docs-only, validate-only, or spec-review-only and does not mutate planning truth.

## G092 Strict Terminal Status

New certification writes may use only:

- `done`
- `blocked`

Non-blocking notes live in `observations[]` or `certification.observations[]`. They do not change status and cannot bypass evidence, tests, or gates. High-severity and remediation-required findings force `blocked`.

Legacy `done_with_concerns` is read-only compatibility for old specs only. It is not a valid new terminal outcome, workflow outcome, dependency-stability state, or certification status.

## G093 Delivery Implementation Delta

`delivery-implementation-delta-guard.sh` prevents done-ceiling delivery modes from certifying planning-only diffs as delivered work.

The guard:

- Resolves the active workflow mode and status ceiling.
- Reads changed paths from git diff or report Code Diff Evidence.
- Classifies paths as planning-only, source, runtime, config, contract, test, docs, or other.
- Passes done-ceiling delivery only when at least one source/runtime/config/contract/test/docs path changed outside `specs/` and `.specify/`.
- Exempts lower-ceiling planning modes while preserving G087 for hardened planning packets.
- Emits a blocked finding with path classification and owner routing when delivery delta is missing.

## Repository Drift Visibility

`repo-drift-report.sh` is informational. It reports:

- Orphan hardened planning packets older than the threshold.
- Done specs with post-certification planning edits.
- Done specs with post-certification source edits referenced by Code Diff Evidence.
- Specs marked `requiresRevalidation:true`.
- Dependency revalidation findings from G089.

In the Bubbles source repository, a missing `specs/` directory is expected and reports as a clean no-op.

## Evidence Provenance

- **Claim Source:** interpreted
- **Interpretation:** This contract was derived from `planning-packet-linkage-guard.sh`, `post-cert-spec-edit-guard.sh`, `inter-spec-dependency-guard.sh`, `planning-workflow-chain-guard.sh`, `strict-terminal-status-guard.sh`, `delivery-implementation-delta-guard.sh`, `repo-drift-report.sh`, `state-transition-guard.sh`, `bubbles/workflows.yaml`, and the migrated execution packet.
