#!/usr/bin/env bash
# worktree-reap.sh (IMP-107 / SCOPE-1 — gap WT-TEARDOWN; SCOPE-5 marker note — gap WT-HARNESS)
# ---------------------------------------------------------------------------
# Explicit, SAFE-BY-CONSTRUCTION worktree reaper. It reaps ONLY the reapable set
# surfaced by worktree-hygiene-report.sh — `MERGED` and `PRUNABLE` worktrees —
# plus their fully-merged LOCAL branches. It is DRY-RUN BY DEFAULT: without
# `--yes` it prints exactly what it WOULD do and touches nothing.
#
# Hard safety invariants (IMP-107 R1-R6):
#   * REFUSES to reap UNMERGED / DIRTY / LEASE-HELD worktrees — it only reports
#     them. A live IMP-023 writer-lease (LEASE-HELD) is re-checked at action
#     time, so a concurrent live run can never be disturbed.
#   * Local branch deletion uses `git branch -d` (SAFE delete — git itself
#     refuses a non-merged branch). It NEVER uses `git branch -D`.
#   * MERGED / PRUNABLE worktree removal uses `git worktree remove` WITHOUT
#     `--force` (git itself refuses a dirty worktree) — the non-experiment path
#     NEVER passes `--force`.
#   * Remote branches are NEVER touched unless `--remote` is given, and even then
#     only AFTER the local safe-delete of that same branch has succeeded.
#   * There is NO `--skip` / bypass flag. `--yes` means "act"; it is not a safety
#     override. `--experiments` opts into the disposable-experiment path below.
#
# IMP-107 SCOPE-3 note: lingering EXPERIMENT (`.design-experiment`) worktrees are
# reaped ONLY under an explicit `--experiments` (which `cli.sh doctor --heal`
# passes) — report-only otherwise, preserving SCOPE-1 behavior. A
# `.design-experiment` marker DECLARES the worktree disposable/throwaway by
# construction, so its own untracked marker (and any throwaway probe content)
# would otherwise block a non-force `git worktree remove`; the experiment path is
# therefore the SOLE `--force` case — and even then it is skipped when LEASE-HELD
# (re-checked at action time) and its branch is removed only by the SAME safe
# `git branch -d` (never `-D`, so a branch with unique commits is retained, never
# lost). UNMERGED / DIRTY / LEASE-HELD stay report-only. The marked-worktree
# identity signal for HUMAN worktrees is SCOPE-5.
#
# IMP-107 SCOPE-5 note (gap WT-HARNESS): worktree-spawn.sh stamps a
# `.bubbles-worktree` marker { runId, mode, baseSha, createdAt, sessionId } on a
# framework-created worktree, which worktree-hygiene-report.sh surfaces as
# `framework-created=yes|no`. That marker is the SAFE IDENTITY SIGNAL for HUMAN
# worktrees: an UNMARKED, un-merged, non-prunable worktree is human-owned and
# stays REPORT-ONLY here — it is never reaped. The marker only REINFORCES this
# SCOPE-1 safety core; it is NEVER a new reason to force-reap an un-merged
# worktree. The reap set is unchanged: MERGED + PRUNABLE (+ lingering EXPERIMENT
# under --experiments). This note is documentation only; the reaper logic is
# SCOPE-1.
#
# Portable to bash 3.2 (macOS) + GNU/BSD git. Always exits 0.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

REPORT_SH="$SCRIPT_DIR/worktree-hygiene-report.sh"
LEASES_SH="$SCRIPT_DIR/runtime-leases.sh"

usage() {
  cat <<'EOF'
Usage: worktree-reap.sh [--yes] [--experiments] [--remote] [--help]

Safely reap MERGED + PRUNABLE git worktrees (and their fully-merged LOCAL
branches). DRY-RUN BY DEFAULT — pass --yes to actually act.

  --yes           Perform the reap. Without it, print what WOULD be reaped and stop.
  --experiments   ALSO reap lingering EXPERIMENT (`.design-experiment`) worktrees
                  (IMP-107 SCOPE-3) — disposable by construction, skipped when
                  LEASE-HELD. `cli.sh doctor --heal` passes this.
  --remote        ALSO delete the merged branch on origin, and ONLY after the
                  local safe-delete of that same branch succeeded (network opt-in).
  --help          Show this help and exit 0.

NEVER reaps UNMERGED / DIRTY / LEASE-HELD worktrees (report-only). Uses
`git branch -d` (never -D). MERGED/PRUNABLE use `git worktree remove` (no
--force); a lingering EXPERIMENT is the sole `--force` case (its marker declares
it disposable). A live writer-lease is re-checked at action time. No bypass flag.
EOF
}

APPLY=false
REMOTE=false
EXPERIMENTS=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes) APPLY=true; shift ;;
    --experiments) EXPERIMENTS=true; shift ;;
    --remote) REMOTE=true; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "worktree-reap: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if ! git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  echo "[worktree-reap] $REPO_ROOT is not a git repository (nothing to do)."
  exit 0
fi

# Re-check safety at action time (defense in depth beyond the report's class).
still_dirty() {
  local p="$1" n
  [[ -d "$p" ]] || { return 1; }
  n="$(git -C "$p" status --porcelain 2>/dev/null | sed '/^$/d' | wc -l | tr -d ' ')"
  [[ "$n" =~ ^[0-9]+$ ]] || n=0
  [[ "$n" -gt 0 ]]
}

still_lease_held() {
  local p="$1" active
  [[ -f "$p/.specify/runtime/resource-leases.json" ]] || return 1
  [[ -x "$LEASES_SH" ]] || return 1
  active="$(BUBBLES_REPO_ROOT="$p" bash "$LEASES_SH" summary 2>/dev/null | sed -nE 's/.*active=([0-9]+).*/\1/p' | head -1)"
  [[ "$active" =~ ^[0-9]+$ ]] || active=0
  [[ "$active" -gt 0 ]]
}

delete_local_branch() {
  # Safe-delete a merged local branch; echo an outcome word. Never uses -D.
  local branch="$1"
  [[ -n "$branch" ]] || { echo "no-branch"; return 0; }
  git -C "$REPO_ROOT" show-ref --verify --quiet "refs/heads/$branch" || { echo "branch-absent"; return 0; }
  if git -C "$REPO_ROOT" branch -d "$branch" >/dev/null 2>&1; then
    echo "branch-deleted"
  else
    echo "branch-retained"
  fi
}

maybe_delete_remote_branch() {
  # Only after a successful local safe-delete, and only with --remote.
  local branch="$1" local_outcome="$2"
  [[ "$REMOTE" == true ]] || return 0
  [[ -n "$branch" ]] || return 0
  [[ "$local_outcome" == "branch-deleted" ]] || return 0
  git -C "$REPO_ROOT" show-ref --verify --quiet "refs/remotes/origin/$branch" || return 0
  if git -C "$REPO_ROOT" push origin --delete "$branch" >/dev/null 2>&1; then
    echo "    remote: deleted origin/$branch"
  else
    echo "    remote: origin/$branch delete declined (left intact)"
  fi
}

machine="$(BUBBLES_REPO_ROOT="$REPO_ROOT" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"

reaped=0 skipped=0

if [[ "$APPLY" == true ]]; then
  if [[ "$EXPERIMENTS" == true ]]; then
    echo "[worktree-reap] APPLYING — reaping MERGED + PRUNABLE + lingering EXPERIMENT worktrees under $REPO_ROOT"
  else
    echo "[worktree-reap] APPLYING — reaping MERGED + PRUNABLE worktrees under $REPO_ROOT"
  fi
  # Clear dir-gone (prunable) admin entries first, so a lingering PRUNABLE
  # branch is no longer reported as "checked out" and can be safely deleted.
  git -C "$REPO_ROOT" worktree prune >/dev/null 2>&1 || true
else
  echo "[worktree-reap] DRY-RUN (no --yes) — nothing will be modified. Under $REPO_ROOT:"
fi

if [[ -z "$machine" ]]; then
  echo "  (no linked worktrees — nothing to reap)"
  echo "[worktree-reap] done: 0 reaped, 0 skipped."
  exit 0
fi

while IFS=$'\t' read -r cls path branch _; do
  [[ -n "${cls:-}" ]] || continue
  case "$cls" in
    MERGED)
      if [[ "$APPLY" == false ]]; then
        echo "  [dry-run] would reap MERGED   $path (branch=${branch:-<detached>})"
        reaped=$((reaped + 1))
        continue
      fi
      # Defense in depth: never touch a worktree that turned dirty/lease-held.
      if still_lease_held "$path"; then
        echo "  SKIP  $path — became LEASE-HELD (live run); left intact"
        skipped=$((skipped + 1)); continue
      fi
      if still_dirty "$path"; then
        echo "  SKIP  $path — became DIRTY; left intact"
        skipped=$((skipped + 1)); continue
      fi
      if git -C "$REPO_ROOT" worktree remove "$path" >/dev/null 2>&1; then
        local_outcome="$(delete_local_branch "$branch")"
        echo "  REAPED MERGED   $path (branch=${branch:-<detached>}, $local_outcome)"
        maybe_delete_remote_branch "$branch" "$local_outcome"
        reaped=$((reaped + 1))
      else
        echo "  SKIP  $path — 'git worktree remove' declined (left intact)"
        skipped=$((skipped + 1))
      fi
      ;;
    PRUNABLE)
      if [[ "$APPLY" == false ]]; then
        echo "  [dry-run] would reap PRUNABLE $path (branch=${branch:-<detached>}; git worktree prune)"
        reaped=$((reaped + 1))
      else
        # The directory is already gone and its admin entry was pruned above.
        # Safe-delete the branch if it lingers.
        local_outcome="$(delete_local_branch "$branch")"
        echo "  REAPED PRUNABLE $path (branch=${branch:-<detached>}, $local_outcome)"
        maybe_delete_remote_branch "$branch" "$local_outcome"
        reaped=$((reaped + 1))
      fi
      ;;
    EXPERIMENT)
      # Lingering design-experiment (IMP-107 SCOPE-3): reaped ONLY when the
      # operator explicitly opts in via --experiments (doctor --heal passes it).
      # Report-only otherwise, preserving SCOPE-1 behavior.
      if [[ "$EXPERIMENTS" != true ]]; then
        echo "  keep  EXPERIMENT $path — report-only (pass --experiments or 'doctor --heal' to reap lingering experiments)"
        skipped=$((skipped + 1)); continue
      fi
      if [[ "$APPLY" == false ]]; then
        echo "  [dry-run] would reap EXPERIMENT $path (branch=${branch:-<detached>}; lingering .design-experiment, disposable)"
        reaped=$((reaped + 1))
        continue
      fi
      # NEVER disturb a live run: a lease can appear after the report ran.
      if still_lease_held "$path"; then
        echo "  SKIP  $path — became LEASE-HELD (live run); left intact"
        skipped=$((skipped + 1)); continue
      fi
      # A `.design-experiment` marker declares the worktree disposable/throwaway
      # by construction; its own untracked marker would block a non-force
      # removal, so the experiment path is the SOLE `--force` case. The branch
      # is still removed only by SAFE `git branch -d` (a branch with unique
      # exploration commits is retained, never lost).
      if git -C "$REPO_ROOT" worktree remove --force "$path" >/dev/null 2>&1; then
        local_outcome="$(delete_local_branch "$branch")"
        echo "  REAPED EXPERIMENT $path (branch=${branch:-<detached>}, $local_outcome)"
        maybe_delete_remote_branch "$branch" "$local_outcome"
        reaped=$((reaped + 1))
      else
        echo "  SKIP  $path — 'git worktree remove --force' declined (left intact)"
        skipped=$((skipped + 1))
      fi
      ;;
    *)
      echo "  keep  $cls $path — report-only (never auto-reaped)"
      skipped=$((skipped + 1))
      ;;
  esac
done <<EOF
$machine
EOF

if [[ "$APPLY" == true ]]; then
  echo "[worktree-reap] done: $reaped reaped, $skipped skipped."
else
  echo "[worktree-reap] dry-run: $reaped would be reaped, $skipped would be kept. Re-run with --yes to act."
fi
exit 0
