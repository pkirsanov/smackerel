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

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "work-boundary-resolve-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "work-boundary-resolve-selftest: all cases passed."
