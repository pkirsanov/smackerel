#!/usr/bin/env bash
# Hermetic selftest for the IMP-107 worktree-hygiene report + reaper (SCOPE-1),
# the lingering design-experiment detection + reap (SCOPE-3), and the
# stale-branch + stash surfacing (SCOPE-4).
# ---------------------------------------------------------------------------
# Synthesizes a throwaway git repo with one linked worktree in EACH hygiene
# state (merged / unmerged / dirty / prunable / lease-held / experiment) — each
# materially different, so the assertions are non-tautological — where the
# lease-held worktree ALSO carries a `.design-experiment` marker so it doubles
# as a LEASE-HELD (still-live) design-experiment. For SCOPE-4 it also creates two
# standalone local branches (a fresh one and an old/diverged one) plus a real
# stash. It asserts:
#   (a) worktree-hygiene-report.sh classifies every worktree correctly;
#   (d) design-experiment-guard.sh --lingering (SCOPE-3) flags a marked,
#       NON-lease-held worktree, --strict exits 1, a LEASE-HELD experiment is
#       NOT flagged (still live), and the DEFAULT leakage-REFUSE mode is
#       byte-unchanged (a regression guard);
#   (f) SCOPE-4 stale-branch + stash SURFACING: the old/diverged branch is
#       flagged with age+ahead, the fresh branch is NOT (non-tautological),
#       trunk is excluded, the stash is surfaced, the thresholds are
#       configurable, and --porcelain is UNCHANGED (no branch/stash leakage);
#   (b) worktree-reap.sh in DRY-RUN lists only merged+prunable and removes
#       nothing;
#   (c) worktree-reap.sh --yes removes ONLY merged+prunable (and their merged
#       local branches) and LEAVES unmerged/dirty/lease-held/experiment intact;
#   (e) worktree-reap.sh --experiments --yes (SCOPE-3) removes the lingering
#       EXPERIMENT worktree + its branch but LEAVES a lease-held experiment;
#   (g) SCOPE-4 REPORT-ONLY proof: the (c)/(e) reaper runs drop NEITHER the
#       stale/fresh local branches NOR the stash.
# The lease-held worktree is synthesized by GENUINELY acquiring an IMP-023
# writer-lease via runtime-leases.sh (with a hand-written-registry fallback,
# clearly WARNed, if that environment lacks a dependency).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_SH="$SCRIPT_DIR/worktree-hygiene-report.sh"
REAP_SH="$SCRIPT_DIR/worktree-reap.sh"
GUARD_SH="$SCRIPT_DIR/design-experiment-guard.sh"
LEASES_SH="$SCRIPT_DIR/runtime-leases.sh"

FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
REPO="$TMP_ROOT/repo"

# Abort setup loudly (a setup failure is a real error, not an assertion miss).
setup() { if ! "$@"; then echo "SETUP-ABORT: $*" >&2; exit 1; fi; }

echo "Running worktree-hygiene guard selftest..."

# --- synthesize a repo with an initial commit on `main` ----------------------
setup git init -q "$REPO"
setup git -C "$REPO" config user.email "selftest@bubbles.local"
setup git -C "$REPO" config user.name "Bubbles Selftest"
setup git -C "$REPO" config commit.gpgsign false
# Pin the initial branch name to `main` regardless of the host git default.
setup git -C "$REPO" symbolic-ref HEAD refs/heads/main
printf 'base\n' > "$REPO/base.txt"
setup git -C "$REPO" add -A
setup git -C "$REPO" commit -qm base

WT_MERGED="$TMP_ROOT/wt-merged"
WT_UNMERGED="$TMP_ROOT/wt-unmerged"
WT_DIRTY="$TMP_ROOT/wt-dirty"
WT_GONE="$TMP_ROOT/wt-gone"
WT_LEASE="$TMP_ROOT/wt-lease"
WT_EXP="$TMP_ROOT/wt-exp"

# MERGED: a fresh branch at main's tip — 0 unique commits, clean.
setup git -C "$REPO" worktree add -q -b merged-wt "$WT_MERGED" main

# UNMERGED: a real unique commit not in trunk.
setup git -C "$REPO" worktree add -q -b feature-wt "$WT_UNMERGED" main
printf 'unique\n' > "$WT_UNMERGED/feature.txt"
setup git -C "$WT_UNMERGED" add -A
setup git -C "$WT_UNMERGED" commit -qm "unique feature commit"

# DIRTY: 0 unique commits but a real uncommitted modification to a tracked file.
setup git -C "$REPO" worktree add -q -b dirty-wt "$WT_DIRTY" main
printf 'uncommitted change\n' >> "$WT_DIRTY/base.txt"

# PRUNABLE: worktree directory really deleted; admin entry lingers.
setup git -C "$REPO" worktree add -q -b gone-wt "$WT_GONE" main
rm -rf "$WT_GONE"

# EXPERIMENT: a real .design-experiment marker at the worktree root.
setup git -C "$REPO" worktree add -q -b exp-wt "$WT_EXP" main
: > "$WT_EXP/.design-experiment"

# LEASE-HELD: genuinely acquire an IMP-023 writer-lease scoped to this worktree.
setup git -C "$REPO" worktree add -q -b lease-wt "$WT_LEASE" main
lease_mode="genuine"
if ! BUBBLES_REPO_ROOT="$WT_LEASE" BUBBLES_SESSION_ID="wt-selftest-session" \
     bash "$LEASES_SH" acquire --purpose wt-selftest-lease --environment dev \
       --share-mode exclusive --ttl-minutes 60 >/dev/null 2>&1; then
  lease_mode="approximated"
fi
# Verify the lease system reports it active; otherwise approximate by writing a
# minimal active-lease registry directly (no runtime-leases.sh dependency).
lease_active="$(BUBBLES_REPO_ROOT="$WT_LEASE" bash "$LEASES_SH" summary 2>/dev/null \
  | sed -nE 's/.*active=([0-9]+).*/\1/p' | head -1)"
if [[ "${lease_active:-0}" -lt 1 ]]; then
  lease_mode="approximated"
  mkdir -p "$WT_LEASE/.specify/runtime"
  future="$(date -u -d '+60 minutes' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
    || date -u -v+60M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
  now_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  {
    printf '{\n  "version": 1,\n  "leases": [\n'
    printf '    {"leaseId":"rls_selftest_0001","repo":"wt-lease","sessionId":"wt-selftest-session","agent":"cli","worktree":"%s","branch":"lease-wt","purpose":"wt-selftest-lease","environment":"dev","composeProject":"wt-selftest-cp","stackGroup":"validation","shareMode":"exclusive","compatibilityFingerprint":"fp","resources":"","attachedSessions":"wt-selftest-session","startedAt":"%s","lastHeartbeatAt":"%s","expiresAt":"%s","status":"active","weight":0}\n' \
      "$WT_LEASE" "$now_ts" "$now_ts" "$future"
    printf '  ]\n}\n'
  } > "$WT_LEASE/.specify/runtime/resource-leases.json"
fi
if [[ "$lease_mode" == "approximated" ]]; then
  echo "WARN: could not cleanly acquire a genuine writer-lease in this environment;" \
       "approximated the LEASE-HELD worktree by writing a minimal active-lease registry" \
       "(the report still consumes runtime-leases.sh summary to classify it)."
fi

# Also mark WT_LEASE as a design-experiment: it now doubles as a LEASE-HELD
# design-experiment (marker + live lease) — the fixture SCOPE-3 needs to prove a
# lease-held experiment is NOT lingering and is NEVER reaped. Lease precedence in
# the report keeps it classified LEASE-HELD (not EXPERIMENT), so the summary
# counts below are unchanged (still 1 lease-held, 1 experiment).
: > "$WT_LEASE/.design-experiment"

# --- SCOPE-4 (gap WT-STALE) fixtures: standalone local branches + a stash ------
# LOCAL branches NOT checked out in any worktree (created via a temp worktree
# that is immediately removed, leaving the branch with a unique commit), plus a
# real WIP stash. They must be SURFACED by the report and NEVER touched by the
# reaper. Distinct age/ahead make the flagging assertions non-tautological. The
# temp worktrees are created AND removed here (before the report runs), so the
# "6 worktrees" summary below is unchanged; `git worktree remove` is targeted
# (never `git worktree prune`, which would clear the PRUNABLE fixture).
WT_TMP_FRESH="$TMP_ROOT/wt-tmp-fresh"
WT_TMP_OLD="$TMP_ROOT/wt-tmp-old"

# fresh-feature: 1 unique commit dated NOW -> ahead=1, age~0 (NOT flagged at defaults).
setup git -C "$REPO" worktree add -q -b fresh-feature "$WT_TMP_FRESH" main
printf 'fresh unique\n' > "$WT_TMP_FRESH/fresh.txt"
setup git -C "$WT_TMP_FRESH" add -A
setup git -C "$WT_TMP_FRESH" commit -qm "fresh unique commit"
setup git -C "$REPO" worktree remove --force "$WT_TMP_FRESH"

# old-feature: 1 unique commit dated 2020 -> ahead=1, age huge (FLAGGED by age).
setup git -C "$REPO" worktree add -q -b old-feature "$WT_TMP_OLD" main
printf 'old unique\n' > "$WT_TMP_OLD/old.txt"
setup git -C "$WT_TMP_OLD" add -A
setup env GIT_AUTHOR_DATE="2020-01-01T00:00:00 +0000" GIT_COMMITTER_DATE="2020-01-01T00:00:00 +0000" \
  git -C "$WT_TMP_OLD" commit -qm "old unique commit"
setup git -C "$REPO" worktree remove --force "$WT_TMP_OLD"

# A real WIP stash in the main repo (surfaced report-only; never dropped).
printf 'wip change\n' >> "$REPO/base.txt"
setup git -C "$REPO" stash push -q -m "selftest-wip-stash"

# class_of <machine-output> <path>  ->  classification token (or empty)
class_of() {
  printf '%s\n' "$1" | awk -F'\t' -v p="$2" '$2 == p { print $1; exit }'
}

# =====================================================================
# (a) report classifies every worktree correctly
# =====================================================================
machine="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"

assert_class() {
  local label="$1" path="$2" want="$3" got
  got="$(class_of "$machine" "$path")"
  if [[ "$got" == "$want" ]]; then pass "$label ($want)"; else fail "$label (expected $want, got '${got:-<none>}')"; fi
}

assert_class "a1 merged worktree"     "$WT_MERGED"   "MERGED"
assert_class "a2 unmerged worktree"   "$WT_UNMERGED" "UNMERGED"
assert_class "a3 dirty worktree"      "$WT_DIRTY"    "DIRTY"
assert_class "a4 prunable worktree"   "$WT_GONE"     "PRUNABLE"
assert_class "a5 lease-held worktree" "$WT_LEASE"    "LEASE-HELD"
assert_class "a6 experiment worktree" "$WT_EXP"      "EXPERIMENT"

# The human summary line must count each state exactly once (non-tautological).
summary="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" 2>/dev/null | sed -n 's/^worktree-hygiene: //p' | tail -1)"
if [[ "$summary" == "6 worktrees (1 merged, 1 unmerged, 1 prunable, 1 dirty, 1 lease-held, 1 experiment)" ]]; then
  pass "a7 summary line counts each state once"
else
  fail "a7 summary line mismatch: '$summary'"
fi

# Report must exit 0 (advisory).
BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "a8 report exits 0 (advisory)"; else fail "a8 report exit $rc (expected 0)"; fi

# =====================================================================
# (f) SCOPE-4 (gap WT-STALE): stale-branch + stash SURFACING (report-only),
#     on the pristine repo (before any reaping). Non-tautological: the fresh
#     branch is NOT flagged at defaults; lowering --branch-ahead flags BOTH.
# =====================================================================
rep_default="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" 2>/dev/null || true)"

if printf '%s\n' "$rep_default" | grep -qE '^  STALE-BRANCH old-feature .*age=[0-9]+d  ahead=1 '; then
  pass "f1 old/diverged branch flagged with age+ahead"
else
  fail "f1 old-feature not flagged with age+ahead: $(printf '%s\n' "$rep_default" | grep -i 'stale-branch' || echo '<no STALE-BRANCH lines>')"
fi

if printf '%s\n' "$rep_default" | grep -qE '^  STALE-BRANCH fresh-feature '; then
  fail "f2 fresh branch was wrongly flagged at default thresholds"
else
  pass "f2 fresh branch not flagged at default thresholds"
fi

if printf '%s\n' "$rep_default" | grep -qE '^  STALE-BRANCH main '; then
  fail "f3 trunk branch was wrongly flagged"
else
  pass "f3 trunk branch excluded from the stale scan"
fi

if printf '%s\n' "$rep_default" | grep -q "selftest-wip-stash"; then
  pass "f4 stash surfaced in the report"
else
  fail "f4 stash not surfaced"
fi

br_sum="$(printf '%s\n' "$rep_default" | sed -n 's/^worktree-hygiene-branches: //p' | tail -1)"
if [[ "$br_sum" == "1 stale local branches (age>=14d or ahead>=200), 1 stashes" ]]; then
  pass "f5 branch/stash summary line correct"
else
  fail "f5 branch/stash summary mismatch: '$br_sum'"
fi

wt_sum2="$(printf '%s\n' "$rep_default" | sed -n 's/^worktree-hygiene: //p' | tail -1)"
if [[ "$wt_sum2" == "6 worktrees (1 merged, 1 unmerged, 1 prunable, 1 dirty, 1 lease-held, 1 experiment)" ]]; then
  pass "f6 worktree summary line unchanged by SCOPE-4"
else
  fail "f6 worktree summary line perturbed: '$wt_sum2'"
fi

rep_cfg="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" --branch-age-days 100000 --branch-ahead 1 2>/dev/null || true)"
cfg_sum="$(printf '%s\n' "$rep_cfg" | sed -n 's/^worktree-hygiene-branches: //p' | tail -1)"
if [[ "$cfg_sum" == "2 stale local branches (age>=100000d or ahead>=1), 1 stashes" ]] \
  && printf '%s\n' "$rep_cfg" | grep -qE '^  STALE-BRANCH fresh-feature ' \
  && printf '%s\n' "$rep_cfg" | grep -qE '^  STALE-BRANCH old-feature '; then
  pass "f7 thresholds configurable (--branch-ahead 1 flags both; --branch-age-days 100000 disables age)"
else
  fail "f7 configurable thresholds failed: '$cfg_sum'"
fi

porc="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"
if printf '%s\n' "$porc" | grep -qiE 'STALE-BRANCH|stash|worktree-hygiene-branches'; then
  fail "f8 --porcelain leaked branch/stash lines into the reaper contract"
else
  pass "f8 --porcelain unchanged (no branch/stash lines; reaper contract intact)"
fi

# =====================================================================
# (d) design-experiment-guard.sh --lingering (IMP-107 SCOPE-3) + a regression
#     proving the DEFAULT (non --lingering) leakage-REFUSE mode is unchanged.
#     Runs BEFORE any reaping, so WT_EXP / WT_LEASE are still pristine.
# =====================================================================

# d1: --lingering flags a marked, NON-lease-held worktree (WT_EXP) and, being
#     advisory, exits 0.
ling_out="$(bash "$GUARD_SH" --lingering --worktree "$WT_EXP" 2>&1)"; ling_rc=$?
if printf '%s\n' "$ling_out" | grep -q "LINGERING" && [[ "$ling_rc" -eq 0 ]]; then
  pass "d1 --lingering flags a marked non-lease worktree (advisory exit 0)"
else
  fail "d1 --lingering on marked non-lease worktree (rc=$ling_rc): $ling_out"
fi

# d2: --lingering --strict turns the same finding into a hard failure (exit 1).
bash "$GUARD_SH" --lingering --strict --worktree "$WT_EXP" >/dev/null 2>&1; ling_strict_rc=$?
if [[ "$ling_strict_rc" -eq 1 ]]; then
  pass "d2 --lingering --strict exits 1 on a lingering experiment"
else
  fail "d2 --lingering --strict expected exit 1, got $ling_strict_rc"
fi

# d3: a LEASE-HELD experiment (WT_LEASE: marker + a live lease) is NOT flagged
#     as lingering — it is still live. Reuses the same runtime-leases.sh path.
lease_ling_out="$(bash "$GUARD_SH" --lingering --worktree "$WT_LEASE" 2>&1)"; lease_ling_rc=$?
if printf '%s\n' "$lease_ling_out" | grep -q "LEASE-HELD" \
  && ! printf '%s\n' "$lease_ling_out" | grep -q "LINGERING" && [[ "$lease_ling_rc" -eq 0 ]]; then
  pass "d3 lease-held experiment is NOT flagged lingering (still live)"
else
  fail "d3 lease-held experiment lingering check (rc=$lease_ling_rc): $lease_ling_out"
fi

# d4 (regression): the DEFAULT (non --lingering) leakage-REFUSE behavior is
#     unchanged — (i) no marker -> no-op exit 0, (ii) marked+clean -> PASS exit
#     0, (iii) marked + a leaked terminal status -> REFUSE exit 1.
LEAK_DIR="$TMP_ROOT/exp-leak"
mkdir -p "$LEAK_DIR"
: > "$LEAK_DIR/.design-experiment"
printf '{ "status": "done", "completedScopes": [] }\n' > "$LEAK_DIR/state.json"
bash "$GUARD_SH" --worktree "$REPO"      >/dev/null 2>&1; def_nomark_rc=$?
bash "$GUARD_SH" --worktree "$WT_EXP"    >/dev/null 2>&1; def_clean_rc=$?
bash "$GUARD_SH" --worktree "$LEAK_DIR"  >/dev/null 2>&1; def_leak_rc=$?
if [[ "$def_nomark_rc" -eq 0 && "$def_clean_rc" -eq 0 && "$def_leak_rc" -eq 1 ]]; then
  pass "d4 default leakage-REFUSE behavior unchanged (no-op=0, clean=0, leak=1)"
else
  fail "d4 default behavior regressed (no-op=$def_nomark_rc clean=$def_clean_rc leak=$def_leak_rc)"
fi

# =====================================================================
# (b) DRY-RUN reaper lists only merged+prunable and removes nothing
# =====================================================================
dry="$(BUBBLES_REPO_ROOT="$REPO" bash "$REAP_SH" 2>/dev/null || true)"

if printf '%s\n' "$dry" | grep -q "would reap MERGED   $WT_MERGED"; then
  pass "b1 dry-run lists MERGED"
else
  fail "b1 dry-run missing MERGED line"
fi
if printf '%s\n' "$dry" | grep -q "would reap PRUNABLE $WT_GONE"; then
  pass "b2 dry-run lists PRUNABLE"
else
  fail "b2 dry-run missing PRUNABLE line"
fi
# Must NOT propose reaping the protected states.
if printf '%s\n' "$dry" | grep -qE "would reap .*($WT_UNMERGED|$WT_DIRTY|$WT_LEASE|$WT_EXP)"; then
  fail "b3 dry-run proposed reaping a protected worktree"
else
  pass "b3 dry-run never proposes a protected worktree"
fi
# Dry-run mutated nothing: every present worktree dir still exists, branches intact.
if [[ -d "$WT_MERGED" && -d "$WT_UNMERGED" && -d "$WT_DIRTY" && -d "$WT_LEASE" && -d "$WT_EXP" ]] \
  && git -C "$REPO" show-ref --verify --quiet refs/heads/merged-wt \
  && git -C "$REPO" show-ref --verify --quiet refs/heads/gone-wt; then
  pass "b4 dry-run modified nothing"
else
  fail "b4 dry-run unexpectedly modified the repo"
fi

# =====================================================================
# (c) --yes reaps ONLY merged+prunable; leaves the rest intact
# =====================================================================
BUBBLES_REPO_ROOT="$REPO" bash "$REAP_SH" --yes >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "c0 reaper --yes exits 0"; else fail "c0 reaper --yes exit $rc"; fi

# MERGED worktree + branch removed.
if [[ ! -d "$WT_MERGED" ]] && ! git -C "$REPO" show-ref --verify --quiet refs/heads/merged-wt; then
  pass "c1 MERGED worktree and branch reaped"
else
  fail "c1 MERGED worktree/branch not fully reaped"
fi
# PRUNABLE admin entry cleared + branch removed.
if ! git -C "$REPO" worktree list --porcelain 2>/dev/null | grep -q "^worktree $WT_GONE$" \
  && ! git -C "$REPO" show-ref --verify --quiet refs/heads/gone-wt; then
  pass "c2 PRUNABLE entry pruned and branch reaped"
else
  fail "c2 PRUNABLE entry/branch not fully reaped"
fi
# UNMERGED intact (worktree + unique branch).
if [[ -d "$WT_UNMERGED" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/feature-wt; then
  pass "c3 UNMERGED worktree and branch left intact"
else
  fail "c3 UNMERGED worktree/branch was disturbed"
fi
# DIRTY intact.
if [[ -d "$WT_DIRTY" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/dirty-wt; then
  pass "c4 DIRTY worktree and branch left intact"
else
  fail "c4 DIRTY worktree/branch was disturbed"
fi
# LEASE-HELD intact (never disturb a live run).
if [[ -d "$WT_LEASE" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/lease-wt; then
  pass "c5 LEASE-HELD worktree and branch left intact"
else
  fail "c5 LEASE-HELD worktree/branch was disturbed"
fi
# EXPERIMENT intact (SCOPE-1 is report-only for experiments).
if [[ -d "$WT_EXP" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/exp-wt; then
  pass "c6 EXPERIMENT worktree and branch left intact"
else
  fail "c6 EXPERIMENT worktree/branch was disturbed"
fi

# =====================================================================
# (e) --experiments --yes reaps the lingering EXPERIMENT (WT_EXP) + its branch
#     but LEAVES the lease-held experiment (WT_LEASE) intact. Runs AFTER the
#     SCOPE-1 reap in (c), which already cleared merged+prunable.
# =====================================================================
BUBBLES_REPO_ROOT="$REPO" bash "$REAP_SH" --experiments --yes >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "e0 reaper --experiments --yes exits 0"; else fail "e0 reaper --experiments --yes exit $rc"; fi

# Lingering EXPERIMENT worktree + its (merged) branch removed.
if [[ ! -d "$WT_EXP" ]] && ! git -C "$REPO" show-ref --verify --quiet refs/heads/exp-wt; then
  pass "e1 lingering EXPERIMENT worktree and branch reaped"
else
  fail "e1 lingering EXPERIMENT worktree/branch not fully reaped"
fi
# Lease-held experiment left intact (never disturb a live run).
if [[ -d "$WT_LEASE" ]] && git -C "$REPO" show-ref --verify --quiet refs/heads/lease-wt; then
  pass "e2 lease-held experiment worktree and branch left intact"
else
  fail "e2 lease-held experiment worktree/branch was disturbed"
fi

# =====================================================================
# (g) SCOPE-4 REPORT-ONLY proof: the reaper runs in (c) --yes and (e)
#     --experiments --yes must have dropped NEITHER the stale/fresh local
#     branches NOR the stash. The reaper consumes only worktree porcelain
#     lines, so a branch/stash is structurally never reaped.
# =====================================================================
if git -C "$REPO" show-ref --verify --quiet refs/heads/old-feature; then
  pass "g1 stale branch (old-feature) survived --yes + --experiments --yes"
else
  fail "g1 stale branch old-feature was reaped (REPORT-ONLY violated)"
fi
if git -C "$REPO" show-ref --verify --quiet refs/heads/fresh-feature; then
  pass "g2 fresh branch (fresh-feature) survived the reaper"
else
  fail "g2 fresh branch fresh-feature was reaped (REPORT-ONLY violated)"
fi
if git -C "$REPO" stash list 2>/dev/null | grep -q "selftest-wip-stash"; then
  pass "g3 stash survived the reaper (never auto-dropped)"
else
  fail "g3 stash was dropped (REPORT-ONLY violated)"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "worktree-hygiene-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "worktree-hygiene-guard-selftest: all cases passed."
