# Session-Aware Runtime Coordination For Concurrent Bubbles Sessions

**Type:** Framework proposal  
**Area:** Workflow control plane, Docker lifecycle governance, runtime coordination  
**Severity:** High — concurrent sessions can stomp shared stacks, tear down each other's resources, or silently reuse incompatible runtime state  
**Filed:** 2026-04-01

<!-- GENERATED:CAPABILITY_LEDGER_STATUS_START -->
**Ledger Status:** proposed
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
  "repo": "guestHost",
  "sessionId": "chat_...",
  "agent": "bubbles.validate",
  "worktree": "/path/to/worktree",
  "branch": "feature/foo",
  "purpose": "validation",
  "environment": "dev",
  "composeProject": "guesthost-val-abc123",
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

- `guesthost-dev-validation-rlsabc123`
- `quantfinance-dev-fullstack-cmp9f84d2`

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
