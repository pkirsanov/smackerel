# <img src="../../icons/bubbles-glasses.svg" width="28"> Parallel Worktrees

> *"Alright boys, in and out. Everybody gets their own room, and we clean up before we leave."*

Spawn isolated git worktrees the SUPPORTED way — with a `.bubbles-worktree`
identity marker — so parallel operator/agent work never leaves untracked debris,
and every spawned worktree has a matching **safe reap**.

---

## The Situation

You want to run **several independent workflows on the same repo at once** — one
worktree per concurrent task (the `<repo>-<role>-<date>` pattern), on branches
like `bug/…`, `governance/plan-…`, `release/…`. Ad-hoc `git worktree add` does
this, but those worktrees are created **outside** any `parallelScopes=dag`
parent, so the framework's drop-on-complete contract never applies to them.
Merged/abandoned worktrees then **linger invisibly** until a `doctor` finally
surfaces the mess (gap `WT-HARNESS`).

`worktree-spawn.sh` closes that loop. It is the canonical create path: it stamps
a `.bubbles-worktree` marker recording `{ runId, mode, baseSha, createdAt,
sessionId }`, which makes each worktree **framework-created and identifiable** —
so the hygiene report can tag it and the safe reaper can clear it when it is
merged, while **never touching an unmarked human worktree**.

## First: Prefer In-Tree-on-`main` (the zero-sprawl default)

Isolated worktrees are **not** the default. The clean repos parallelize the
cheap way: small scopes committed in-tree on `main` (trunk-based development,
per `instructions/bubbles-release-trains.instructions.md`). Reach for an isolated
worktree ONLY when you need genuine isolation — e.g. a long parallel task whose
uncommitted state would collide with other work in the same tree, or a throwaway
design-experiment probe. If in doubt, stay in-tree on `main`.

## The Spawn → Work → Reap Loop

**1. Spawn** a marked worktree off a base (default `HEAD`):

```
bash bubbles/scripts/worktree-spawn.sh \
  --path ../myrepo-plan-20260727 --branch governance/plan-foo \
  --mode full-delivery --run-id run-abc --session-id vscode-xxxx
```

Add `--experiment` for a throwaway probe — it ALSO stamps a `.design-experiment`
marker, so the probe is both framework-created AND declared disposable.

**2. Work** in the new worktree exactly as usual — commit, run workflows, merge
your branch into trunk when the task is done.

**3. Reap** when finished. The safe reaper removes only what is provably done:

```
# See what WOULD be reaped (dry-run by default):
bash bubbles/scripts/worktree-reap.sh

# Actually reap MERGED + PRUNABLE worktrees (and their merged local branches):
bash bubbles/scripts/worktree-reap.sh --yes

# Or let doctor heal the whole set (merged/prunable + lingering experiments):
bash bubbles/scripts/cli.sh doctor --heal
```

To see which worktrees the framework created at any time, read the hygiene
report — each worktree line carries a `framework-created=yes|no` tag:

```
bash bubbles/scripts/worktree-hygiene-report.sh
```

## The Reap Guarantee

Every **spawned (marked)** worktree has a matching **safe reap** — but the safety
core never widens:

- The reaper reaps **only** `MERGED` + `PRUNABLE` worktrees (plus lingering
  `EXPERIMENT` worktrees under `--experiments` / `doctor --heal`). It **refuses**
  `UNMERGED` / `DIRTY` / `LEASE-HELD` — those are report-only.
- The `.bubbles-worktree` marker is a **safe identity signal**, not a reap
  trigger. It lets an operator SEE which worktrees the framework created; it is
  **never** a new reason to force-reap an un-merged worktree.
- An **UNMARKED** (human-owned) un-merged, non-prunable worktree is **always
  report-only** — the reaper never touches it.
- Local branch deletion uses `git branch -d` (safe delete — git refuses a
  non-merged branch), never `-D`. Remote branches are untouched without an
  explicit `--remote`. A live IMP-023 writer-lease (`LEASE-HELD`) is re-checked
  at action time, so a concurrent live run is never disturbed.

The result: parallelize freely, and the debris the framework used to leave behind
becomes impossible to miss (surfaced every `doctor`) and trivial to clear
(`doctor --heal`) — with zero risk to unmerged or human-owned work.

## See Also

- [Parallel Scope Execution](parallel-scopes.md) — run DAG-independent scopes of ONE spec concurrently via `parallelScopes: dag`
- [Coordinate Runtime Leases](runtime-coordination.md) — shared Docker/Compose ownership across parallel sessions
- [Framework Ops](framework-ops.md) — `doctor` / `doctor --heal` and other framework health commands
