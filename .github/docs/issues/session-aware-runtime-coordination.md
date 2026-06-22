# Session-Aware Runtime Coordination For Concurrent Bubbles Sessions

**Type:** Framework proposal  
**Area:** Workflow control plane, Docker lifecycle governance, runtime coordination  
**Severity:** High — concurrent sessions can stomp shared stacks, tear down each other's resources, or silently reuse incompatible runtime state  
**Filed:** 2026-04-01

<!-- GENERATED:CAPABILITY_LEDGER_STATUS_START -->
**Ledger Status:** shipped
**Related Capability:** Session-aware runtime coordination
**Competitive Pressure:** claude-code, roo-code, cursor
**Source Of Truth:** [Issue Status](../generated/issue-status.md)
<!-- GENERATED:CAPABILITY_LEDGER_STATUS_END -->

## Problem

Bubbles already has two useful but incomplete protections:

1. **Source isolation** via `gitIsolation` and `parallelScopes: dag`, which protects code changes by using branches and worktrees.
2. **Docker isolation guidance** via Compose project names, profiles, labels, and disposable validation storage.

Those are necessary, but they do **not** yet provide a session-aware runtime coordination layer.

Today, two independent Bubbles sessions can still collide when they:

- start the same repo CLI stack against the same Docker namespace
- rebuild or restart containers/images that another session is actively validating against
- reuse mutable volumes or databases from a different branch/worktree with different code or migrations
- run cleanup/teardown commands that remove another session's resources
- attach to an already-running stack without proving compatibility with the current workspace state

The current framework knows how to isolate **code execution**, but it does not yet coordinate **runtime ownership, compatibility, reuse, or teardown safety** across sessions.

## Current Gap

The framework currently provides:

- worktree-level parallel scope execution
- `gitIsolation: true`
- Docker lifecycle guidance that recommends isolated Compose project names for validation stacks
- label-aware cleanup guidance
- shared-infrastructure blast-radius planning for risky refactors

What it does **not** provide:

- a runtime lease registry
- compatibility fingerprints for safe stack reuse
- session-aware rules for shared vs exclusive Docker resources
- heartbeat/TTL ownership of running stacks
- lease-aware cleanup and teardown
- control-plane visibility into active runtime sessions

## Failure Modes

### 1. Destructive teardown across sessions

Session A starts a validation stack. Session B runs a repo CLI stop/down command that targets the same Compose project name or cleanup group. Session A loses its runtime mid-validation.

### 2. Incompatible runtime reuse

Session A is on branch X with one schema/config/image set. Session B is on branch Y with different changes. Session B reuses Session A's running stack because it exists, but the stack is no longer compatible with B's workspace.

### 3. Mutable store contamination

Two sessions unknowingly share a database or volume. One session runs migrations, seed data, or destructive tests that invalidate the other's expectations.

### 4. Cleanup blast radius

Project-scoped cleanup is safer than global cleanup, but if multiple sessions intentionally or accidentally share a project name, cleanup can still delete live resources that belong to another active session.

### 5. Hidden contention

Even when nothing is destroyed, multiple sessions can fight over ports, build caches, or restarts with no framework-level visibility into who owns what.

## Proposed Fix

Add a **session-aware runtime coordination layer** to Bubbles.

This should be built around four concepts:

1. **Runtime leases** — explicit ownership records for stacks and mutable resources
2. **Compatibility fingerprints** — proof that reuse is safe
3. **Resource share modes** — reusable vs exclusive vs disposable runtime classes
4. **Lease-aware orchestration** — agents and repo CLI entrypoints honor the same runtime contract

## Proposed Design

### 1. Runtime Lease Registry

Add a control-plane artifact such as:

`/.specify/runtime/resource-leases.json`

Each lease record should include fields like:

```json
{
  "leaseId": "rls_20260401_abc123",
  "repo": "example-app",
  "sessionId": "chat_...",
  "agent": "bubbles.validate",
  "worktree": "/path/to/worktree",
  "branch": "feature/foo",
  "purpose": "validation",
  "environment": "dev",
  "composeProject": "example-app-val-abc123",
  "stackGroup": "validation",
  "shareMode": "exclusive",
  "compatibilityFingerprint": "sha256:...",
  "resources": {
    "containers": ["..."],
    "volumes": ["..."],
    "networks": ["..."],
    "images": ["..."]
  },
  "startedAt": "2026-04-01T12:00:00Z",
  "lastHeartbeatAt": "2026-04-01T12:05:00Z",
  "expiresAt": "2026-04-01T12:20:00Z",
  "status": "active"
}
```

The registry must be advisory for humans but authoritative for Bubbles-controlled runtime actions.

### 2. Share Modes

Every runtime target should be categorized into one of these modes:

- `shared-compatible` — may be reused only when fingerprints match and the target is explicitly marked shareable
- `exclusive` — may only be used by one active session at a time
- `disposable` — must be created fresh and destroyed at the end of the run
- `persistent-protected` — never auto-deleted, never used as validation/test scratch state

Examples:

- Build cache: usually `shared-compatible`
- Validation database: usually `disposable`
- Dev primary database: usually `persistent-protected`
- Live mutable integration stack: often `exclusive`

### 3. Compatibility Fingerprint

Before reusing a running stack, Bubbles should compute a compatibility fingerprint from relevant inputs such as:

- Dockerfiles
- Compose files and active profiles
- lockfiles / dependency manifests
- generated config inputs
- migration directories or schema hash
- selected repo CLI command + environment
- optionally the current git revision or a reduced source-hash set

If the fingerprint differs, reuse is forbidden.

### 4. Lease-Aware Decision Matrix

When a session needs a runtime resource:

1. If no lease exists, create a new stack and lease it.
2. If a lease exists and the target is `shared-compatible` and fingerprint matches, attach and renew.
3. If a lease exists but fingerprint differs, allocate a new isolated stack.
4. If a lease exists and the target is `exclusive`, block with a concrete explanation unless the lease is stale.
5. If the lease is stale, offer takeover or automated recovery under explicit rules.

### 5. Session Heartbeats And Stale-Lease Recovery

Active Bubbles sessions should renew leases while runtime work is ongoing.

If a session disappears without cleanup:

- leases age out by TTL
- `bubbles.super doctor` surfaces stale leases
- `bubbles.status` shows which resources are active, stale, or orphaned
- teardown may reclaim stale validation/disposable resources only after timeout policy is satisfied

### 6. Lease-Aware Cleanup

Any Bubbles-controlled cleanup, stop, or teardown action must be filtered by lease ownership.

Required behavior:

- only touch resources owned by the current lease, or
- explicitly target stale leases that are eligible for reclaim, or
- ask for a force/takeover path when a live foreign lease exists

This prevents one session from tearing down another session's containers or deleting compatible reusable resources unexpectedly.

### 7. Compose Project Naming Convention

Standardize a lease-derived Compose project naming pattern, for example:

`<repo>-<env>-<purpose>-<leaseOrCompatGroup>`

Examples:

- `example-app-dev-validation-rlsabc123`
- `example-app-dev-fullstack-cmp9f84d2`

This keeps current Compose-grouping guidance, but makes it deterministic and session-safe.

## Agent Integration Points

The following agents should honor runtime leases:

- `bubbles.workflow` — acquire or attach before runtime-bearing phases
- `bubbles.devops` — own start/stop/build/deploy/runtime coordination surfaces
- `bubbles.validate` — lease validation stacks and avoid live-session collisions
- `bubbles.chaos` — never attack a stack owned by another active incompatible session
- `bubbles.stabilize` — detect contention, stale leases, restart thrash, and port conflicts
- `bubbles.super` — expose doctor/status/recovery UX for runtime leases

## CLI / Framework Surface Additions

Possible CLI additions:

- `bubbles runtime leases`
- `bubbles runtime attach <lease-id>`
- `bubbles runtime release <lease-id>`
- `bubbles runtime reclaim-stale`
- `bubbles runtime doctor`

Possible user-facing outcomes:

- show active runtime owners before destructive operations
- explain why a stack was reused, isolated, or blocked
- show the fingerprint mismatch that forced isolation

## Rollout Plan

### Phase 1: Policy And Metadata

- define runtime lease schema
- define share modes and compatibility inputs
- update Docker lifecycle governance docs
- update `bubbles.super` recommendations for parallel work

### Phase 2: Observability Without Enforcement

- record leases and show them in status/doctor output
- label resources consistently
- surface stale/orphaned stacks

### Phase 3: Soft Enforcement

- warn before attaching to incompatible stacks
- warn before tearing down foreign active leases
- prefer isolated stack allocation automatically

### Phase 4: Hard Enforcement

- block incompatible reuse
- block foreign lease teardown by default
- require explicit takeover for stale recovery

## Acceptance Criteria

- [ ] Bubbles can record active runtime leases for agent-started Docker/stack work
- [ ] A session can reuse a stack only when compatibility fingerprint matches and the stack is marked shareable
- [ ] Exclusive stacks cannot be silently reused by another active session
- [ ] Cleanup and teardown actions do not delete resources owned by another active session
- [ ] `bubbles.status` or equivalent shows active runtime owners and stale leases
- [ ] `bubbles.super doctor` can detect orphaned or stale runtime resources
- [ ] Repo CLI integration can adopt lease-aware Compose project naming without breaking current single-session flows

## Affected Framework Paths

- `bubbles/scripts/cli.sh`
- `bubbles/workflows.yaml`
- `agents/bubbles.workflow.agent.md`
- `agents/bubbles.devops.agent.md`
- `agents/bubbles.validate.agent.md`
- `agents/bubbles.chaos.agent.md`
- `agents/bubbles.stabilize.agent.md`
- `agents/bubbles.super.agent.md`
- `agents/bubbles_shared/docker-lifecycle-governance.md`
- `docs/recipes/parallel-scopes.md`
- `docs/recipes/framework-ops.md`
- `docs/guides/CONTROL_PLANE_DESIGN.md`

## Non-Goals

- Replacing repo-specific CLI entrypoints for build/test/deploy
- Forcing all stacks to be isolated all the time
- Sharing mutable validation databases across sessions
- Solving every port allocation problem automatically without repo participation

## Why This Must Be Upstream

This behavior must be centralized in Bubbles because downstream repos cannot solve cross-session safety consistently on their own without reimplementing the same control-plane logic in multiple repo CLIs and agent prompts.

The framework already owns the orchestration layer, parallel-scope semantics, and Docker lifecycle governance. Runtime lease coordination belongs beside those capabilities, not as a one-off downstream patch.

## Resolution

**Shipped in v3.8.0.** The session-aware runtime coordination layer is now a first-class control-plane capability.

### What Was Shipped

- **`bubbles/scripts/runtime-leases.sh`** — runtime lease registry implementation with subcommands:
  - `acquire` — allocate or reuse a lease (compatibility-fingerprint-aware) with explicit `--share-mode` (`shared-compatible` / `exclusive` / `disposable` / `persistent-protected`), `--ttl-minutes`, `--compose-project`, `--fingerprint-file`/`--fingerprint-input`, and `--resource <kind:name>` ownership recording
  - `attach <lease-id> [--takeover]` — join a compatible lease or take over a stale one
  - `release <lease-id> [--session-id <id>]` — mark released or detach a single attached session
  - `heartbeat <lease-id>` — renew TTL for active sessions
  - `reclaim-stale` — mark expired active leases as stale
  - `doctor [--quiet]` — detect stale leases and active conflicts
  - `summary` — aggregate active/stale/released/conflict counts
  - `lookup [filters]` — locate leases by compose project, purpose, session, or status
  - `leases | list` — show recorded runtime leases
- **`bubbles/scripts/runtime-lease-selftest.sh`** — 19-case selftest covering acquire, compatible reuse, incompatible isolation, exclusive blocking, stale detection, doctor reporting, share-mode release semantics, summary aggregation, and downstream installation/CLI integration
- **CLI integration** in `bubbles/scripts/cli.sh`:
  - `cmd_runtime` (passthrough subcommand): `bubbles/scripts/cli.sh runtime <subcommand>`
  - `cmd_status`: surfaces runtime lease summary alongside the spec dashboard
  - `cmd_doctor`: includes runtime conflict/stale detection
- **Framework validation**: `bubbles/scripts/framework-validate.sh` runs the runtime lease selftest as part of the standard validation suite
- **Schema and design docs**: `docs/guides/CONTROL_PLANE_DESIGN.md` and `docs/guides/CONTROL_PLANE_SCHEMAS.md` document the lease record shape, share modes, fingerprint inputs, and the lease-aware decision matrix

### Acceptance Criteria — Status

- [x] Bubbles can record active runtime leases for agent-started Docker/stack work — `runtime-leases.sh acquire` records leases under `.specify/runtime/resource-leases.json`
- [x] A session can reuse a stack only when compatibility fingerprint matches and the stack is marked shareable — covered by selftest cases "Compatible shared runtime is reused across sessions" and "Incompatible shared runtime gets a new lease"
- [x] Exclusive stacks cannot be silently reused by another active session — covered by selftest case "Exclusive runtime blocks concurrent acquisition"
- [x] Cleanup and teardown actions do not delete resources owned by another active session — share-mode + lease-aware release semantics enforce this; downstream Compose project naming is lease-derived
- [x] `bubbles.status` or equivalent shows active runtime owners and stale leases — `bubbles/scripts/cli.sh status` calls `runtime-leases.sh summary`
- [x] `bubbles.super doctor` can detect orphaned or stale runtime resources — `bubbles/scripts/cli.sh doctor` invokes `runtime-leases.sh doctor`
- [x] Repo CLI integration can adopt lease-aware Compose project naming without breaking current single-session flows — selftest "Downstream CLI runtime summary works from installed .github layout" / "Downstream CLI can acquire a runtime lease" / "Downstream CLI can release a runtime lease" prove the downstream installation path

### Resource-Weighted Admission (shipped extension)

The original lease model coordinated ownership, compatibility, and exclusivity, but it had **no host-capacity dimension**: nothing stopped two *different* heavy builds (different purpose, different fingerprint, no compose collision) from starting at once and OOM-killing the host (exit 137), nor from orphan-hanging when one session removed another's mid-build container. That is the unaddressed half of Failure Mode #5 ("fight over … build caches" / hidden contention) and the recurring real-world failure that hand-rolled watchdog + memory-polling push-grab loops were papering over.

Weighted admission closes that gap by adding a single resource-weight budget on top of the existing lease registry. It is **opt-in and fully backward-compatible**:

- **Config (`bubbles.config.json` → `runtime`):**
  - `capacityWeight` (number, default **0**). `0` = admission **disabled** — a fresh install behaves exactly as before until a host operator sets a budget. When `> 0`, weighted admission is active and represents the host's total concurrent heavy-work budget.
  - `weightClasses` (optional object; built-in default `{ "light": 1, "medium": 4, "heavy": 8 }`) maps a named class to its unit cost.
- **`acquire` options:**
  - `--weight <light|medium|heavy>` — resolves to units via `weightClasses` (default `light`).
  - `--weight-units <N>` — explicit integer units; takes precedence over `--weight`.
  - `--wait <seconds>` — if admission would refuse, block and re-poll (releasing the registry lock between polls) until capacity frees or the timeout elapses, then refuse. Omitted = immediate structured refusal.
- **Lease record:** a numeric `weight` field is persisted per lease and shown by `format_lease_line` / `lookup`. A legacy lease with no `weight` reads as `0`, so pre-existing registries keep parsing.
- **Admission semantics:** before creating a new lease, Bubbles sums `weight` over **effectively-active** leases only. Stale/expired/released leases do **not** count — so a dead session's heavy lease frees its budget automatically via TTL/stale downgrade (the orphan-hang fix). If `active_sum + new_weight > capacityWeight`, the acquire is refused (or waits, with `--wait`) with a structured message and a `runtime_lease_capacity_refused` framework event.
- **Observability:** `summary` and `doctor` print `Runtime capacity: <active_sum>/<capacityWeight> weight units` (or `disabled` when `capacityWeight=0`).

Selftest coverage (in `runtime-lease-selftest.sh`) includes a heavy acquire under budget, an **adversarial** second-heavy refusal (which fails if the gate is removed), capacity freeing on release, a stale-lease-frees-capacity case (orphan-hang analog), `--wait` immediate-refuse / wait-loop-timeout / post-release-success, `--weight-units` precedence + exact-boundary admission, and a backward-compat case proving two heavy leases both acquire when `capacityWeight` is unset.

Downstream wiring (product repo CLIs invoking `runtime acquire --purpose build --weight heavy` before a heavy build, and an `exclusive` land lease before a push) is intentionally a **separate later task** — the framework primitive is shipped here.

### Known Follow-Ups (Not Blocking This Flip)

These rollout-plan items remain optional enhancements rather than gaps in the shipped capability:

- Phase 4 hard enforcement in agent prompts — individual agents (`bubbles.workflow`, `bubbles.devops`, `bubbles.validate`, `bubbles.chaos`, `bubbles.stabilize`) can adopt explicit lease acquisition in their phase contracts. The framework primitive is shipped; agent-prompt integration can land incrementally.
- `bubbles/workflows.yaml` mode-level `runtimeLease` declarations are not yet defined — modes can opt into lease-aware orchestration once the agent-prompt integration above lands.

These are tracked as future work, not as blockers — the underlying lease registry, fingerprinting, share modes, doctor surface, and downstream installation path are all shipped, tested, and integrated into the framework validation suite.
