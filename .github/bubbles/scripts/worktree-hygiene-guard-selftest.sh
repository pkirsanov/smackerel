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

# IMP-033 SCOPE-1: the reaper's input contract must be BYTE-IDENTICAL after the
# new local/remote line was added. Structural proof (a "before" binary is not
# available inside a hermetic fixture): every emitted line is a 7-field worktree
# record and NONE of the three summary lines leaks in.
if printf '%s\n' "$porc" | grep -q 'worktree-hygiene-local'; then
  fail "f9 --porcelain leaked the IMP-033 local/remote line into the reaper contract"
else
  pass "f9 --porcelain carries no worktree-hygiene-local line"
fi
bad_field_lines="$(printf '%s\n' "$porc" | sed '/^$/d' | awk -F'\t' 'NF != 7' | wc -l | tr -d ' ')"
if [[ "$bad_field_lines" == "0" ]]; then
  pass "f10 --porcelain emits only 7-field worktree records (byte-shape unchanged)"
else
  fail "f10 --porcelain emitted $bad_field_lines line(s) without exactly 7 tab fields"
fi

# IMP-033 SCOPE-1 on the RICH fixture, read off the pristine report: the main
# repo is clean (its WIP was stashed), has no remote, and owns 8 non-trunk local
# branches. The branch count is deliberately NOT the stale count on line 2 (1) —
# proving the two counters are distinct rather than a duplicated metric.
loc_sum="$(printf '%s\n' "$rep_default" | sed -n 's/^worktree-hygiene-local: //p' | tail -1)"
if [[ "$loc_sum" == "0 dirty files (primary), 0 untracked, remote=none, 8 non-trunk local branches" ]]; then
  pass "f11 local/remote summary correct on the rich fixture (branch count distinct from the stale count)"
else
  fail "f11 local/remote summary mismatch: '$loc_sum'"
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

# =====================================================================
# (h) IMP-033 SCOPE-1 (gap COV-5): the primary-worktree + remote line.
#     Each case gets its OWN throwaway repo so the rich fixture's counts above
#     stay untouched. These are the two leak vectors the linked-worktree
#     enumeration is structurally blind to: uncommitted work in the checkout
#     the operator types in, and commits that exist only on this machine.
#     Every degraded state is asserted by NAME — the whole point is that a
#     failed or impossible lookup must never render as a reassuring `0 ahead`.
# =====================================================================
local_line() {
  BUBBLES_REPO_ROOT="$1" bash "$REPORT_SH" "${@:2}" 2>/dev/null \
    | sed -n 's/^worktree-hygiene-local: //p' | tail -1
}

mk_repo() {
  local d="$1"
  setup git init -q "$d"
  setup git -C "$d" config user.email "selftest@bubbles.local"
  setup git -C "$d" config user.name "Bubbles Selftest"
  setup git -C "$d" config commit.gpgsign false
  setup git -C "$d" symbolic-ref HEAD refs/heads/main
  printf 'base\n' > "$d/base.txt"
  setup git -C "$d" add -A
  setup git -C "$d" commit -qm base
}

assert_local() {
  local label="$1" want="$2" got="$3"
  if [[ "$got" == "$want" ]]; then pass "$label"; else fail "$label — expected '$want', got '$got'"; fi
}

# h1 — clean tree, no remote. The baseline the other cases must differ from.
H_CLEAN="$TMP_ROOT/h-clean"
mk_repo "$H_CLEAN"
assert_local "h1 clean + no remote reports remote=none (never a fabricated 0 ahead)" \
  "0 dirty files (primary), 0 untracked, remote=none, 0 non-trunk local branches" \
  "$(local_line "$H_CLEAN")"

# h2 — dirty AND untracked, with DIFFERENT counts (1 vs 2). A detector that
#      lumped every porcelain line into both counters would report 3/3 here.
H_DIRTY="$TMP_ROOT/h-dirty"
mk_repo "$H_DIRTY"
printf 'modified\n' >> "$H_DIRTY/base.txt"
printf 'a\n' > "$H_DIRTY/untracked-a.txt"
printf 'b\n' > "$H_DIRTY/untracked-b.txt"
assert_local "h2 dirty(1) and untracked(2) counted separately" \
  "1 dirty files (primary), 2 untracked, remote=none, 0 non-trunk local branches" \
  "$(local_line "$H_DIRTY")"

# h3 — ahead>0 against a real (local, no-network) remote.
H_BARE="$TMP_ROOT/h-bare.git"
setup git init -q --bare "$H_BARE"
H_AHEAD="$TMP_ROOT/h-ahead"
mk_repo "$H_AHEAD"
setup git -C "$H_AHEAD" remote add origin "$H_BARE"
setup git -C "$H_AHEAD" push -q origin main
printf 'one\n' > "$H_AHEAD/one.txt"
setup git -C "$H_AHEAD" add -A
setup git -C "$H_AHEAD" commit -qm "unpushed one"
printf 'two\n' > "$H_AHEAD/two.txt"
setup git -C "$H_AHEAD" add -A
setup git -C "$H_AHEAD" commit -qm "unpushed two"
assert_local "h3 two unpushed commits reported as 2 ahead" \
  "0 dirty files (primary), 0 untracked, 2 ahead / 0 behind origin/main (unfetched), 0 non-trunk local branches" \
  "$(local_line "$H_AHEAD")"

# h4 — ahead AND behind, with DIFFERENT counts (1 vs 1 would be tautological, so
#      diverge by 1 ahead / 2 behind). Proves the left/right fields are not swapped.
H_BARE2="$TMP_ROOT/h-bare2.git"
setup git init -q --bare "$H_BARE2"
H_DIV="$TMP_ROOT/h-diverged"
mk_repo "$H_DIV"
setup git -C "$H_DIV" remote add origin "$H_BARE2"
setup git -C "$H_DIV" push -q origin main
div_base="$(git -C "$H_DIV" rev-parse HEAD)"
printf 'r1\n' > "$H_DIV/r1.txt"; setup git -C "$H_DIV" add -A; setup git -C "$H_DIV" commit -qm "remote one"
printf 'r2\n' > "$H_DIV/r2.txt"; setup git -C "$H_DIV" add -A; setup git -C "$H_DIV" commit -qm "remote two"
setup git -C "$H_DIV" push -q origin main
setup git -C "$H_DIV" reset -q --hard "$div_base"
printf 'l1\n' > "$H_DIV/l1.txt"; setup git -C "$H_DIV" add -A; setup git -C "$H_DIV" commit -qm "local one"
assert_local "h4 diverged tree reports 1 ahead / 2 behind (fields not transposed)" \
  "0 dirty files (primary), 0 untracked, 1 ahead / 2 behind origin/main (unfetched), 0 non-trunk local branches" \
  "$(local_line "$H_DIV")"

# h5 — a remote is configured but its trunk ref was never fetched. Reporting
#      `0 ahead` here would be the exact false-clean this scope exists to close.
H_NOREF="$TMP_ROOT/h-noref"
H_BARE3="$TMP_ROOT/h-bare3.git"
setup git init -q --bare "$H_BARE3"
mk_repo "$H_NOREF"
setup git -C "$H_NOREF" remote add origin "$H_BARE3"
assert_local "h5 configured-but-never-fetched remote reports remote-untracked" \
  "0 dirty files (primary), 0 untracked, remote-untracked (origin/main never fetched), 0 non-trunk local branches" \
  "$(local_line "$H_NOREF")"

# h6 — detached HEAD: there is no branch to compare, and the report says so.
H_DET="$TMP_ROOT/h-detached"
mk_repo "$H_DET"
setup git -C "$H_DET" checkout -q --detach HEAD
assert_local "h6 detached HEAD named explicitly" \
  "0 dirty files (primary), 0 untracked, detached-HEAD, 0 non-trunk local branches" \
  "$(local_line "$H_DET")"

# h7 — --fetch SUCCEEDS against the local bare remote -> labelled (fetched).
#      Built deterministically (never `git clone`, whose trunk depends on the
#      host's init.defaultBranch): pin main, fetch, hard-reset onto origin/main
#      so the histories genuinely share a base, then add one local commit.
H_FETCH_OK="$TMP_ROOT/h-fetch-ok"
mk_repo "$H_FETCH_OK"
setup git -C "$H_FETCH_OK" remote add origin "$H_BARE"
setup git -C "$H_FETCH_OK" fetch -q origin
setup git -C "$H_FETCH_OK" reset -q --hard origin/main
printf 'local only\n' > "$H_FETCH_OK/local.txt"
setup git -C "$H_FETCH_OK" add -A
setup git -C "$H_FETCH_OK" commit -qm "local only commit"
assert_local "h7 --fetch success labelled (fetched)" \
  "0 dirty files (primary), 0 untracked, 1 ahead / 0 behind origin/main (fetched), 0 non-trunk local branches" \
  "$(local_line "$H_FETCH_OK" --fetch)"

# h8 — --fetch FAILS (remote URL points nowhere) but the last-known ref still
#      exists: the comparison falls back to it AND is labelled remote-unverified.
H_FETCH_FAIL="$TMP_ROOT/h-fetch-fail"
mk_repo "$H_FETCH_FAIL"
setup git -C "$H_FETCH_FAIL" remote add origin "$H_BARE2"
setup git -C "$H_FETCH_FAIL" fetch -q origin
setup git -C "$H_FETCH_FAIL" remote set-url origin "$TMP_ROOT/definitely-not-a-repo.git"
ff_line="$(local_line "$H_FETCH_FAIL" --fetch)"
if printf '%s' "$ff_line" | grep -q '(remote-unverified)'; then
  pass "h8 failed/offline fetch labelled (remote-unverified), not silently reported as fresh"
else
  fail "h8 failed fetch was not labelled remote-unverified: '$ff_line'"
fi

# h9 — the DEFAULT run must NOT fetch (this script is read-only; fetch writes
#      refs/remotes/*). Proven by pointing the remote at nothing and observing
#      the default run still succeeds and is labelled (unfetched).
def_line="$(local_line "$H_FETCH_FAIL")"
if printf '%s' "$def_line" | grep -q '(unfetched)'; then
  pass "h9 default run does not fetch (read-only contract preserved)"
else
  fail "h9 default run label wrong: '$def_line'"
fi

# h10 — --porcelain on a DIRTY primary emits nothing at all (no linked
#       worktrees), proving the new facts never reach the reaper's contract.
h10_porc="$(BUBBLES_REPO_ROOT="$H_DIRTY" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"
if [[ -z "$(printf '%s' "$h10_porc" | tr -d '[:space:]')" ]]; then
  pass "h10 --porcelain silent on a dirty primary with no linked worktrees"
else
  fail "h10 --porcelain emitted output for a dirty primary: '$h10_porc'"
fi

# h11 — advisory contract holds in every degraded state (always exit 0).
h11_rc=0
for h11_repo in "$H_CLEAN" "$H_DIRTY" "$H_AHEAD" "$H_DIV" "$H_NOREF" "$H_DET" "$H_FETCH_FAIL"; do
  BUBBLES_REPO_ROOT="$h11_repo" bash "$REPORT_SH" >/dev/null 2>&1 || h11_rc=$?
done
if [[ "$h11_rc" -eq 0 ]]; then
  pass "h11 report exits 0 in every degraded state (advisory, never a gate)"
else
  fail "h11 report exited $h11_rc in a degraded state"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "worktree-hygiene-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "worktree-hygiene-guard-selftest: all cases passed."
