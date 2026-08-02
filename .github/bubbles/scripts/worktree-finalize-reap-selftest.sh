#!/usr/bin/env bash
# worktree-finalize-reap-selftest.sh (IMP-107 / SCOPE-2 — gap WT-TEARDOWN)
# ---------------------------------------------------------------------------
# Proves that the SCOPE-1 SAFE reaper (worktree-reap.sh) already COVERS the
# gitIsolation / parallelScopes=dag *finalize* lifecycle end-to-end — so the
# "the parent drops that worktree and its branch" clause in
# agents/bubbles_shared/scope-workflow.md is now a MECHANIZED finalize-reap
# STEP, not narrative. SCOPE-2 adds NO reaper logic; it NAMES the teardown and
# this selftest verifies the existing reaper reaps BOTH finalize end-states
# while never eating un-merged / uncommitted work.
#
# The gitIsolation finalize lifecycle maps EXACTLY onto the reaper's scope:
#   * a COMPLETED scope  -> its branch is MERGED into the parent trunk
#                        -> its worktree becomes MERGED   -> finalize-reap removes it
#   * an ABANDONED scope -> the run is rolled back (worktree dropped)
#                        -> its worktree becomes PRUNABLE -> finalize-reap removes it
#   * an UNMERGED / DIRTY worktree (unfinished, un-merged work)
#                        -> is REFUSED -> the finalize-reap NEVER eats it
#
# Each fixture GENUINELY reaches its asserted finalize end-state (a real merge,
# a real rm -rf, a real unique commit, a real uncommitted change) — the
# assertions are non-tautological — and the reap outcome is checked against REAL
# `git worktree` / `git branch` state, not the reaper's own narration.
#
# Hermetic: a throwaway `mktemp -d` repo, torn down on exit. Reuses the shipped
# worktree-reap.sh + worktree-hygiene-report.sh UNCHANGED (SCOPE-1). Portable to
# bash 3.2 (macOS) + GNU/BSD git; uses only git + POSIX text tools.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_SH="$SCRIPT_DIR/worktree-hygiene-report.sh"
REAP_SH="$SCRIPT_DIR/worktree-reap.sh"

FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

# A missing dependency (or a non-git environment) is a real error, not an
# assertion miss — abort loudly.
for dep in "$REPORT_SH" "$REAP_SH"; do
  if [[ ! -f "$dep" ]]; then
    echo "SETUP-ABORT: required SCOPE-1 script not found: $dep" >&2
    exit 1
  fi
done
if ! command -v git >/dev/null 2>&1; then
  echo "SETUP-ABORT: git not available" >&2
  exit 1
fi

# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
REPO="$TMP_ROOT/repo"

# Abort setup loudly (a setup failure is a real error, not an assertion miss).
setup() { if ! "$@"; then echo "SETUP-ABORT: $*" >&2; exit 1; fi; }

echo "Running worktree finalize-reap selftest (IMP-107 / SCOPE-2 — WT-TEARDOWN)..."

# --- synthesize a repo with an initial commit pinned to `main` ---------------
setup git init -q "$REPO"
setup git -C "$REPO" config user.email "selftest@bubbles.local"
setup git -C "$REPO" config user.name "Bubbles Selftest"
setup git -C "$REPO" config commit.gpgsign false
# Pin the initial branch to `main` regardless of the host git default so the
# report's trunk detection is deterministic.
setup git -C "$REPO" symbolic-ref HEAD refs/heads/main
printf 'base\n' > "$REPO/base.txt"
setup git -C "$REPO" add -A
setup git -C "$REPO" commit -qm base

WT_COMPLETED="$TMP_ROOT/wt-completed"   # completed scope -> merged  -> MERGED   (reaped)
WT_ABANDONED="$TMP_ROOT/wt-abandoned"   # abandoned scope -> dropped -> PRUNABLE (reaped)
WT_UNMERGED="$TMP_ROOT/wt-unmerged"     # unfinished work -> UNMERGED            (refused)
WT_DIRTY="$TMP_ROOT/wt-dirty"           # merged branch + uncommitted -> DIRTY   (refused)

# COMPLETED scope: real scope work committed on its branch, THEN the parent
# merges that branch into trunk — the "merge" half of the lifecycle. Afterward
# the branch has 0 unique commits vs trunk and the worktree is clean -> MERGED.
setup git -C "$REPO" worktree add -q -b scope-completed "$WT_COMPLETED" main
printf 'scope deliverable\n' > "$WT_COMPLETED/deliverable.txt"
setup git -C "$WT_COMPLETED" add -A
setup git -C "$WT_COMPLETED" commit -qm "scope-completed: real work"
# Parent (on main) merges the completed scope branch into trunk (fast-forward).
setup git -C "$REPO" merge -q --no-edit scope-completed

# ABANDONED scope: the parent rolls back the run by dropping the worktree
# directory; git then marks the lingering admin entry PRUNABLE (dir-gone). The
# branch carries no unique work, mirroring a rolled-back/empty abandon.
setup git -C "$REPO" worktree add -q -b scope-abandoned "$WT_ABANDONED" main
rm -rf "$WT_ABANDONED"

# UNMERGED: genuine unfinished work — a unique commit NOT in trunk, clean tree.
setup git -C "$REPO" worktree add -q -b scope-unmerged "$WT_UNMERGED" main
printf 'still in progress\n' > "$WT_UNMERGED/wip.txt"
setup git -C "$WT_UNMERGED" add -A
setup git -C "$WT_UNMERGED" commit -qm "scope-unmerged: unfinished unique commit"

# DIRTY: branch is fully merged (0 unique commits) BUT the worktree carries a
# real uncommitted change — proves the finalize-reap refuses even a
# merged-branch worktree while uncommitted work is present (DIRTY precedence).
setup git -C "$REPO" worktree add -q -b scope-dirty "$WT_DIRTY" main
printf 'uncommitted edit\n' >> "$WT_DIRTY/base.txt"

# =====================================================================
# Phase A — prove each fixture GENUINELY reached its finalize end-state
#           (non-tautological): the report must classify each correctly BEFORE
#           any reaping. --porcelain emits CLASS<TAB>PATH<TAB>... per worktree.
# =====================================================================
porc="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"
class_of() { printf '%s\n' "$porc" | awk -F'\t' -v p="$1" '$2==p {print $1; exit}'; }

a_completed="$(class_of "$WT_COMPLETED")"
a_abandoned="$(class_of "$WT_ABANDONED")"
a_unmerged="$(class_of "$WT_UNMERGED")"
a_dirty="$(class_of "$WT_DIRTY")"

if [[ "$a_completed" == "MERGED" ]]; then
  pass "A1 completed scope reached MERGED end-state (branch merged into trunk)"
else
  fail "A1 completed scope class='$a_completed' (expected MERGED)"
fi
if [[ "$a_abandoned" == "PRUNABLE" ]]; then
  pass "A2 abandoned scope reached PRUNABLE end-state (worktree dropped)"
else
  fail "A2 abandoned scope class='$a_abandoned' (expected PRUNABLE)"
fi
if [[ "$a_unmerged" == "UNMERGED" ]]; then
  pass "A3 unfinished scope is UNMERGED (unique un-merged commit)"
else
  fail "A3 unfinished scope class='$a_unmerged' (expected UNMERGED)"
fi
if [[ "$a_dirty" == "DIRTY" ]]; then
  pass "A4 merged-branch worktree with uncommitted work is DIRTY"
else
  fail "A4 dirty worktree class='$a_dirty' (expected DIRTY)"
fi

# =====================================================================
# Phase B — the finalize-reap step: worktree-reap.sh --yes reaps BOTH finalize
#           end-states (MERGED completed + PRUNABLE abandoned) and REFUSES the
#           un-merged / dirty work. Assertions check REAL git state, not the
#           reaper's narration.
# =====================================================================
reap_out="$(BUBBLES_REPO_ROOT="$REPO" bash "$REAP_SH" --yes 2>&1)"
reap_rc=$?
if [[ "$reap_rc" -eq 0 ]]; then
  pass "B0 finalize-reap (worktree-reap.sh --yes) exits 0"
else
  fail "B0 finalize-reap exited $reap_rc"
fi

# B1 completed -> MERGED finalize path: worktree dir + merged branch removed.
if [[ ! -d "$WT_COMPLETED" ]] && ! git -C "$REPO" show-ref --verify --quiet refs/heads/scope-completed; then
  pass "B1 completed-scope worktree reaped via MERGED finalize path (dir + branch gone)"
else
  fail "B1 completed-scope worktree NOT fully reaped (dir or branch remains)"
fi

# B2 abandoned -> PRUNABLE finalize path: admin entry pruned + branch removed.
if ! git -C "$REPO" worktree list --porcelain 2>/dev/null | grep -q "^worktree $WT_ABANDONED$" \
  && ! git -C "$REPO" show-ref --verify --quiet refs/heads/scope-abandoned; then
  pass "B2 abandoned-scope worktree reaped via PRUNABLE finalize path (entry pruned + branch gone)"
else
  fail "B2 abandoned-scope worktree NOT fully reaped (entry or branch remains)"
fi

# B3 UNMERGED refused: worktree + unique branch left intact — the finalize-reap
# NEVER eats un-merged work.
if [[ -d "$WT_UNMERGED" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/scope-unmerged; then
  pass "B3 unfinished (UNMERGED) worktree left intact — finalize-reap never eats un-merged work"
else
  fail "B3 unfinished (UNMERGED) worktree was disturbed"
fi

# B4 DIRTY refused: worktree + branch left intact even though the branch itself
# is fully merged — uncommitted work is never eaten.
if [[ -d "$WT_DIRTY" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/scope-dirty; then
  pass "B4 dirty worktree left intact — finalize-reap never eats uncommitted work"
else
  fail "B4 dirty worktree was disturbed"
fi

# B5 the reaper's own narration corroborates the real-state checks above
# (whitespace-tolerant so alignment changes never break the assertion).
if printf '%s\n' "$reap_out" | grep -qE "REAPED[[:space:]]+MERGED[[:space:]]+${WT_COMPLETED}([[:space:]]|$)" \
  && printf '%s\n' "$reap_out" | grep -qE "REAPED[[:space:]]+PRUNABLE[[:space:]]+${WT_ABANDONED}([[:space:]]|$)"; then
  pass "B5 reaper narration reports REAPED MERGED (completed) + REAPED PRUNABLE (abandoned)"
else
  fail "B5 reaper narration missing a REAPED finalize line:
$reap_out"
fi

# B6 the reaper NEVER narrates reaping a refused (un-merged/dirty) worktree.
if printf '%s\n' "$reap_out" | grep -qE "REAPED[[:space:]].*(${WT_UNMERGED}|${WT_DIRTY})"; then
  fail "B6 reaper narrated reaping an un-merged/dirty worktree:
$reap_out"
else
  pass "B6 reaper never narrates reaping an un-merged/dirty worktree"
fi

# =====================================================================
echo ""
if [[ "$FAILURES" -eq 0 ]]; then
  echo "all cases passed."
  exit 0
fi
echo "$FAILURES case(s) failed."
exit 1
