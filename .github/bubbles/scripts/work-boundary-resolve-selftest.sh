#!/usr/bin/env bash
# Hermetic selftest for work-boundary-resolve.sh (IMP-100 Phase 4 R6).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/work-boundary-resolve.sh"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if ! command -v jq >/dev/null 2>&1; then
  echo "work-boundary-resolve-selftest: SKIP (jq not installed)"
  exit 0
fi

# write_state <dir> <workBoundary-json|-> : write a v3 state.json; '-' = no workBoundary.
write_state() {
  local dir="$1" wb="$2"
  mkdir -p "$dir"
  if [[ "$wb" == "-" ]]; then
    printf '%s\n' '{ "version": 3, "status": "in_progress", "workflowMode": "full-delivery" }' > "$dir/state.json"
  else
    printf '{ "version": 3, "status": "in_progress", "workflowMode": "full-delivery", "workBoundary": %s }\n' "$wb" > "$dir/state.json"
  fi
}

# run_disp <label> <expected-disposition> <resolver-args...>
run_disp() {
  local label="$1" exp="$2"; shift 2
  local out rc=0
  out="$(bash "$RESOLVER" "$@" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -qx "disposition=$exp"; then
    pass "$label"
  else
    fail "$label (rc=$rc, got: $(printf '%s' "$out" | grep '^disposition=' || echo none))"
  fi
}

# run_fail <label> <resolver-args...> : expect exit 2 (usage / malformed / no parser).
run_fail() {
  local label="$1"; shift
  local rc=0
  bash "$RESOLVER" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    pass "$label"
  else
    fail "$label (expected exit 2, got $rc)"
  fi
}

echo "Running work-boundary-resolve selftest..."

# ── Backward-compatibility: nothing declared → permissive ──────────────────
d="$TMP_ROOT/no-wb"; write_state "$d" "-"
run_disp "T1 no workBoundary block → in-boundary (backward-compatible)" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/no-state"; mkdir -p "$d"   # no state.json
run_disp "T2 no state.json → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

# ── Repo dimension (the core anti-cross-repo-wandering contract) ────────────
d="$TMP_ROOT/repo-only"; write_state "$d" '{"repositoryRoots":["bubbles"]}'
run_disp "T3 same repo, no path/spec restriction → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles
run_disp "T4 different repo + default forbidden → refuse-cross-repo" refuse-cross-repo \
  --feature-dir "$d" --candidate-repo app-alpha

d="$TMP_ROOT/cross-auth"; write_state "$d" '{"repositoryRoots":["bubbles"],"crossRepoPolicy":"authorized"}'
run_disp "T5 different repo + crossRepoPolicy=authorized → route-cross-repo" route-cross-repo \
  --feature-dir "$d" --candidate-repo app-alpha
run_disp "T5b authorized policy, in-repo candidate still in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

# ── Spec dimension (unrelated same-repo work is routed, not inline-fixed) ───
d="$TMP_ROOT/spec"; write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"]}'
run_disp "T6 same repo, spec outside specTargets → route-same-repo" route-same-repo \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec specs/020-bar
run_disp "T7 same repo, spec inside specTargets → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec specs/010-foo
run_disp "T7b spec basename match → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec 010-foo

# ── Path dimension (glob prefix + trailing-slash + exact) ──────────────────
d="$TMP_ROOT/path"; write_state "$d" '{"repositoryRoots":["bubbles"],"allowedPaths":["services/gateway/**","libs/","README.md"]}'
run_disp "T8 same repo, path outside allowedPaths → route-same-repo" route-same-repo \
  --feature-dir "$d" --candidate-repo bubbles --candidate-path dashboard/src/App.tsx
run_disp "T9 path inside allowedPaths (prefix/**) → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-path services/gateway/src/main.rs
run_disp "T10 path inside allowedPaths (dir/) → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-path libs/util.rs
run_disp "T11 path exact match → in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-path README.md

# ── Fail-closed: a present-but-malformed boundary must NOT silently pass ────
d="$TMP_ROOT/m-empty"; write_state "$d" '{"repositoryRoots":[]}'
run_fail "T12 empty repositoryRoots → exit 2" --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/m-type"; write_state "$d" '{"repositoryRoots":"bubbles"}'
run_fail "T13 repositoryRoots not an array → exit 2" --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/m-policy"; write_state "$d" '{"repositoryRoots":["bubbles"],"crossRepoPolicy":"maybe"}'
run_fail "T14 invalid crossRepoPolicy → exit 2" --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/m-spec"; write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":"specs/010"}'
run_fail "T15 specTargets not an array → exit 2" --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/m-nonstr"; write_state "$d" '{"repositoryRoots":["bubbles",""]}'
run_fail "T16 repositoryRoots with an empty-string entry → exit 2" --feature-dir "$d" --candidate-repo bubbles

# ── Usage errors ───────────────────────────────────────────────────────────
run_fail "T17 missing --candidate-repo → exit 2" --feature-dir "$TMP_ROOT/repo-only"
run_fail "T18 missing --feature-dir → exit 2" --candidate-repo bubbles

# ═══════════════════════════════════════════════════════════════════════════
# STRICT MODE (IMP-038 SCOPE-2 / GF-2)
#
# The permissive default is the GF-2 hole: a run could enter MUTABLE execution
# with no declared repository, spec, or path boundary and nothing refused. Every
# case below fails if that refusal regresses to a pass.
# ═══════════════════════════════════════════════════════════════════════════

# run_rc <label> <expected-rc> <resolver-args...>
run_rc() {
  local label="$1" want="$2"; shift 2
  local rc=0
  bash "$RESOLVER" "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then
    pass "$label"
  else
    fail "$label (expected exit $want, got $rc)"
  fi
}

# run_both <label> <expected-disposition> <resolver-args...>
# Strict must change the handling of ABSENCE, never the classification: both
# modes must decide, and both must decide the SAME thing.
run_both() {
  local label="$1" exp="$2"; shift 2
  local plain strict_out d1 d2 rc1=0 rc2=0
  plain="$(bash "$RESOLVER" "$@" 2>/dev/null)" || rc1=$?
  strict_out="$(bash "$RESOLVER" --strict "$@" 2>/dev/null)" || rc2=$?
  d1="$(printf '%s\n' "$plain" | sed -n 's/^disposition=//p')"
  d2="$(printf '%s\n' "$strict_out" | sed -n 's/^disposition=//p')"
  if [[ "$rc1" -eq 0 && "$rc2" -eq 0 && "$d1" == "$exp" && "$d2" == "$exp" ]]; then
    pass "$label"
  else
    fail "$label (non-strict rc=$rc1 disp='$d1'; strict rc=$rc2 disp='$d2'; expected '$exp')"
  fi
}

# ── Absence is refused in strict, and STILL permissive in the default mode ──
# The same fixture answered two ways is what proves the modes genuinely differ
# AND that the legacy read-only contract survived.
d="$TMP_ROOT/no-state"   # created for T2; still has no state.json
run_rc "S1 strict + no state.json → exit 3 (refused)" 3 \
  --feature-dir "$d" --candidate-repo bubbles --strict
run_disp "S1b the SAME fixture without --strict is still in-boundary (legacy survives)" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/no-wb"      # created for T1; state.json with no workBoundary
run_rc "S2 strict + state.json with no workBoundary → exit 3 (refused)" 3 \
  --feature-dir "$d" --candidate-repo bubbles --strict
run_disp "S2b the SAME fixture without --strict is still in-boundary" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

# ── Each planning-dispatch minimum key, absent on its own ──────────────────
d="$TMP_ROOT/s-no-spec"; write_state "$d" '{"repositoryRoots":["bubbles"],"crossRepoPolicy":"forbidden"}'
run_rc "S3 strict + no specTargets → exit 3" 3 --feature-dir "$d" --candidate-repo bubbles --strict
run_disp "S3b the same boundary is still decided without --strict" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles

d="$TMP_ROOT/s-no-policy"; write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"]}'
run_rc "S4 strict + no crossRepoPolicy → exit 3" 3 --feature-dir "$d" --candidate-repo bubbles --strict

d="$TMP_ROOT/s-no-roots"; write_state "$d" '{"specTargets":["specs/010-foo"],"crossRepoPolicy":"forbidden"}'
run_rc "S5 strict + no repositoryRoots → exit 3" 3 --feature-dir "$d" --candidate-repo bubbles --strict

# An empty list declares nothing, so strict must treat it exactly like an absent
# key — otherwise `"specTargets": []` would be a one-character bypass.
d="$TMP_ROOT/s-empty-spec"; write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":[],"crossRepoPolicy":"forbidden"}'
run_rc "S6 strict + EMPTY specTargets → exit 3 (an empty list is not a declaration)" 3 \
  --feature-dir "$d" --candidate-repo bubbles --strict

# ── A complete boundary classifies IDENTICALLY in both modes (all four) ────
d="$TMP_ROOT/s-complete"
write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"],"allowedPaths":["bubbles/scripts/**"],"crossRepoPolicy":"forbidden"}'
run_both "S7 complete boundary → in-boundary in BOTH modes" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec specs/010-foo --candidate-path bubbles/scripts/x.sh
run_both "S7b complete boundary, out-of-scope spec → route-same-repo in BOTH modes" route-same-repo \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec specs/020-bar
run_both "S7c complete boundary, out-of-scope path → route-same-repo in BOTH modes" route-same-repo \
  --feature-dir "$d" --candidate-repo bubbles --candidate-spec specs/010-foo --candidate-path dashboard/src/App.tsx
run_both "S7d complete boundary, foreign repo + forbidden → refuse-cross-repo in BOTH modes" refuse-cross-repo \
  --feature-dir "$d" --candidate-repo app-alpha

d="$TMP_ROOT/s-complete-auth"
write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"],"allowedPaths":["bubbles/scripts/**"],"crossRepoPolicy":"authorized"}'
run_both "S7e complete boundary, foreign repo + authorized → route-cross-repo in BOTH modes" route-cross-repo \
  --feature-dir "$d" --candidate-repo app-alpha

# ── --require-allowed-paths: the FIRST-SOURCE-MUTATION requirement ─────────
d="$TMP_ROOT/s-no-paths"
write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"],"crossRepoPolicy":"forbidden"}'
run_rc "S8 --require-allowed-paths + absent allowedPaths → exit 4" 4 \
  --feature-dir "$d" --candidate-repo bubbles --require-allowed-paths
run_disp "S8b the same boundary still decides under plain --strict (planning needs no paths)" in-boundary \
  --feature-dir "$d" --candidate-repo bubbles --strict

d="$TMP_ROOT/s-empty-paths"
write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"],"allowedPaths":[],"crossRepoPolicy":"forbidden"}'
run_rc "S8c --require-allowed-paths + EMPTY allowedPaths → exit 4" 4 \
  --feature-dir "$d" --candidate-repo bubbles --require-allowed-paths

run_disp "S8d --require-allowed-paths + a concrete allowedPath → decides" in-boundary \
  --feature-dir "$TMP_ROOT/s-complete" --candidate-repo bubbles \
  --candidate-spec specs/010-foo --candidate-path bubbles/scripts/x.sh --require-allowed-paths

# --require-allowed-paths implies --strict, so it can never be the WEAKER check.
run_rc "S9 --require-allowed-paths alone still refuses an undeclared boundary → exit 3" 3 \
  --feature-dir "$TMP_ROOT/no-wb" --candidate-repo bubbles --require-allowed-paths
run_rc "S9b --require-allowed-paths alone still refuses a missing state.json → exit 3" 3 \
  --feature-dir "$TMP_ROOT/no-state" --candidate-repo bubbles --require-allowed-paths

# ── Strict must not RECLASSIFY malformed as merely-undeclared ──────────────
# Each fixture DECLARES all three required keys, so the strict presence check is
# satisfied and only the value is broken. If strict ever reported these as
# exit 3 ("undeclared"), a malformed boundary would be indistinguishable from an
# absent one and the fail-closed contract would read as a paperwork problem.
d="$TMP_ROOT/s-bad-roots-type"
write_state "$d" '{"repositoryRoots":"bubbles","specTargets":["specs/010-foo"],"crossRepoPolicy":"forbidden"}'
run_rc "S10 strict + declared-but-malformed repositoryRoots → still exit 2 (malformed)" 2 \
  --feature-dir "$d" --candidate-repo bubbles --strict

d="$TMP_ROOT/s-bad-policy"
write_state "$d" '{"repositoryRoots":["bubbles"],"specTargets":["specs/010-foo"],"crossRepoPolicy":"maybe"}'
run_rc "S10b strict + declared-but-invalid crossRepoPolicy → still exit 2 (malformed)" 2 \
  --feature-dir "$d" --candidate-repo bubbles --strict

d="$TMP_ROOT/s-empty-roots"
write_state "$d" '{"repositoryRoots":[],"specTargets":["specs/010-foo"],"crossRepoPolicy":"forbidden"}'
run_rc "S10c strict + EMPTY repositoryRoots → exit 2 (a hard error in BOTH modes)" 2 \
  --feature-dir "$d" --candidate-repo bubbles --strict
run_rc "S10d the same empty repositoryRoots is exit 2 without --strict too" 2 \
  --feature-dir "$d" --candidate-repo bubbles

# ── There is no bypass flag ────────────────────────────────────────────────
for bypass in --force --skip --ignore --no-verify; do
  run_rc "S11 '$bypass' is not accepted (no bypass exists)" 2 \
    --feature-dir "$TMP_ROOT/s-complete" --candidate-repo bubbles "$bypass"
done
if grep -qE '^[[:space:]]*--(force|skip|ignore|no-verify|unsafe)\)' "$RESOLVER"; then
  fail "S11b work-boundary-resolve.sh declares a bypass-shaped flag"
else
  pass "S11b work-boundary-resolve.sh declares no bypass-shaped flag"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "work-boundary-resolve-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "work-boundary-resolve-selftest: all cases passed."
