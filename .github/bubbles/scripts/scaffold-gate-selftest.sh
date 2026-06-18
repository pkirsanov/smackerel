#!/usr/bin/env bash
# File: scaffold-gate-selftest.sh
#
# Hermetic selftest for scaffold-gate.sh. Proves next-ID computation (skipping the
# former burned G096 and the reserved G102–G109 gap), skeleton creation, --dry-run
# writes nothing, clobber refusal, and bad-name/usage exits.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAFFOLD="$SCRIPT_DIR/scaffold-gate.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# Build a fake repo root with a minimal gates.yaml fixture + tests/regression.
make_root() {
  local root="$1"
  shift
  mkdir -p "$root/bubbles/registry" "$root/bubbles/scripts" "$root/tests/regression"
  {
    echo "gates:"
    local id
    for id in "$@"; do
      echo "  $id:"
      echo "    name: ${id}_probe_gate"
    done
  } >"$root/bubbles/registry/gates.yaml"
}

run_scaffold() {
  # $1 = root, rest = args
  local root="$1"
  shift
  BUBBLES_SCAFFOLD_ROOT="$root" BUBBLES_SCAFFOLD_GATES_FILE="$root/bubbles/registry/gates.yaml" \
    bash "$SCAFFOLD" "$@"
}

# --- Case 1: next id after a G127 ceiling is G128 ------------------------------
r1="$work/r1"
make_root "$r1" G090 G101 G127
out1="$(run_scaffold "$r1" my_new_thing_gate 2>&1)"
grep -q "gate id      : G128" <<<"$out1" \
  && pass "next id after G127 is G128" \
  || {
    fail "next id after G127 should be G128"
    echo "$out1"
  }
[[ -x "$r1/bubbles/scripts/my-new-thing-guard.sh" ]] \
  && pass "guard skeleton created and executable" \
  || fail "guard skeleton missing"
[[ -x "$r1/bubbles/scripts/my-new-thing-guard-selftest.sh" ]] \
  && pass "selftest skeleton created" \
  || fail "selftest skeleton missing"
[[ -f "$r1/tests/regression/test_01_my-new-thing.sh" ]] \
  && pass "regression skeleton created with next NN (01 on empty dir)" \
  || fail "regression skeleton missing"
grep -q "Gate G128 — my_new_thing_gate" "$r1/bubbles/scripts/my-new-thing-guard.sh" \
  && pass "guard skeleton carries the computed gate id + name" \
  || fail "guard skeleton missing gate id/name stamp"
grep -q "exit 1" "$r1/bubbles/scripts/my-new-thing-guard.sh" \
  && pass "guard skeleton defaults to a fail-loud finding (exit 1) until implemented" \
  || fail "guard skeleton should default to exit 1"

# --- Case 2: the former burned G096 is skipped when it would be next ----------
r2="$work/r2"
make_root "$r2" G090 G095
out2="$(run_scaffold "$r2" another_probe_gate --dry-run 2>&1)"
grep -q "gate id      : G097" <<<"$out2" \
  && pass "next id after G095 skips the former burned G096 → G097" \
  || {
    fail "next id after G095 should skip the former burned G096 and be G097"
    echo "$out2"
  }

# --- Case 3: the reserved G102–G109 gap is skipped ----------------------------
r3="$work/r3"
make_root "$r3" G100 G101
out3="$(run_scaffold "$r3" gap_probe_gate --dry-run 2>&1)"
grep -q "gate id      : G110" <<<"$out3" \
  && pass "next id after G101 skips the reserved G102–G109 gap → G110" \
  || {
    fail "next id after G101 should skip the reserved gap and be G110"
    echo "$out3"
  }

# --- Case 4: --dry-run writes nothing -----------------------------------------
r4="$work/r4"
make_root "$r4" G127
run_scaffold "$r4" dryrun_probe_gate --dry-run >/dev/null 2>&1
if [[ -z "$(find "$r4/bubbles/scripts" -name 'dryrun-probe-*' 2>/dev/null)" ]]; then
  pass "--dry-run writes no skeleton files"
else
  fail "--dry-run must not write files"
fi

# --- Case 5: clobber refusal --------------------------------------------------
r5="$work/r5"
make_root "$r5" G127
run_scaffold "$r5" clobber_probe_gate >/dev/null 2>&1
set +e
run_scaffold "$r5" clobber_probe_gate >"$work/c5.log" 2>&1
c5=$?
set -e
if [[ "$c5" -eq 1 ]] && grep -q "refusing to clobber" "$work/c5.log"; then
  pass "re-running refuses to clobber existing skeleton files (exit 1)"
else
  fail "re-running should refuse to clobber (exit 1)"
  cat "$work/c5.log"
fi

# --- Case 6: bad gate name → exit 1 -------------------------------------------
r6="$work/r6"
make_root "$r6" G127
set +e
run_scaffold "$r6" NotSnakeCase >"$work/c6.log" 2>&1
c6=$?
set -e
[[ "$c6" -eq 1 ]] \
  && pass "a non-snake_case / non-_gate name is refused (exit 1)" \
  || {
    fail "bad gate name should exit 1"
    cat "$work/c6.log"
  }

# --- Case 7: no gate name → usage exit 2 --------------------------------------
r7="$work/r7"
make_root "$r7" G127
set +e
run_scaffold "$r7" >/dev/null 2>&1
c7=$?
set -e
[[ "$c7" -eq 2 ]] \
  && pass "missing gate name exits 2 (usage)" \
  || fail "missing gate name should exit 2"

if [[ "$failures" -eq 0 ]]; then
  echo "[scaffold-gate-selftest] OK"
else
  echo "[scaffold-gate-selftest] $failures failed"
  exit 1
fi
