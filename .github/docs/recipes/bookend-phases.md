# Recipe: Bookend Phases for Long Workflows

> *"You can't have a party without setup, and you can't leave without takin' the cans back, Ricky."*

---

## The Problem

Long workflows accumulate side effects: spawned containers, locked databases, half-applied migrations, captured leases, downloaded fixtures, log handles. When the core phases (`implement`, `test`) exit early — whether from a real failure, a timeout, or a routed packet — those side effects leak into the next run. The next agent inherits a dirty world and either:

- Mistakes leftover state for fresh state (false-positive completion), or
- Crashes on a port collision / locked file / orphaned process and reports the wrong root cause.

A workflow that touches real infrastructure cannot rely on the optimistic path. The early-exit case is the common case, not the edge case.

## The Pattern

Bookend the core phases with two dedicated phases:

- **`setup`** — pre-flight that establishes the world the core phases assume. Idempotent; safe to re-run.
- **`teardown`** — post-flight that releases everything `setup` and the core phases acquired. Runs on success **and** on failure.

The two MUST be:

1. **Idempotent** — second invocation with unchanged inputs is a no-op, never an error.
2. **Trap-guaranteed** — `teardown` runs even when the core phases exit non-zero. If the workflow runtime supports it, register `teardown` in a `trap '...' EXIT INT TERM` so an interrupted run still cleans up.
3. **Owner-scoped** — only release resources the same workflow run acquired (lease tokens, run-id-tagged containers, scoped tmp dirs). Never blindly nuke a shared host.

## When to Use

Add `setup` + `teardown` phases when the workflow:

- Runs DB migrations against a real instance (lock acquisition, schema changes).
- Spawns real containers, processes, or serverless invocations.
- Acquires runtime leases (see `runtime-leases.sh`).
- Downloads sizeable fixtures or pulls registry images.
- Mutates shared host state (Caddy fragment, ufw rule, systemd unit) on a deployment target.

Skip them when the workflow is purely in-memory, read-only, or single-file editing. Bookends are not free — they add two phases worth of state-snapshot surface and gate evaluation. Use them where the dirty-state risk is real.

## Example Workflow Snippet

```yaml
modes:
  migrate-and-verify:
    description: "DB migration with bookend phases"
    statusCeiling: done
    phaseOrder:
      - setup        # ← bookend: acquire migration lease, snapshot schema
      - implement    # apply migrations
      - test         # verify schema + data invariants
      - teardown     # ← bookend: release lease, drop temp tables, restore observability
    requiredGates:
      - G023
      - G024
    constraints:
      teardownAlwaysRuns: true   # phase runs on success AND failure
```

The `teardownAlwaysRuns` flag (or its runtime equivalent) is what makes the bookend a real safety net rather than just another optimistic phase.

## Anti-Patterns

- **Cleanup-only-in-teardown without trap guarantees.** If `teardown` is skipped on a panic, timeout, or lost runtime, it provides zero protection. Always wire trap-style execution at the runtime layer; do not assume the orchestrator will reach the next phase on its own.
- **Assuming `setup` always succeeds.** A failing `setup` MUST short-circuit the core phases AND still run a partial-`teardown` to release whatever it managed to acquire. Do not let a half-built world leak forward.
- **Mixing teardown with feature work.** `teardown` is for releasing what the run acquired — not for migrating data, not for sending notifications, not for opportunistic cleanup of unrelated leftovers (see `bubbles/workflows.yaml` Check 8D, "Mixed-purpose cleanup is forbidden").
- **Sharing teardown across runs.** Each run owns its own resources via a unique run-id or lease token. A teardown that "cleans everything that looks like ours" will eventually delete a sibling run's state.
- **Treating workflow `setup` as one-shot bootstrap.** Project-init bootstrap is the [`bubbles.setup`](../../agents/bubbles.setup.agent.md) agent's territory and runs once per repo. Workflow `setup` runs every invocation and MUST be idempotent against the existing world.

## References

- [`bubbles/workflows.yaml`](../../bubbles/workflows.yaml) — workflow modes, phase declarations, and the `state-transition-guard.sh` Check 8D ("Mixed-purpose cleanup is forbidden") gate.
- [`bubbles.setup` agent](../../agents/bubbles.setup.agent.md) — project-level bootstrap (different from workflow `setup` phase).
- [`bubbles.devops` agent](../../agents/bubbles.devops.agent.md) — owns ops surface and runtime cleanup patterns; `teardown` phases often delegate here.
- [`runtime-leases.sh`](../../bubbles/scripts/runtime-leases.sh) — the canonical lease registry that bookend phases acquire and release.
