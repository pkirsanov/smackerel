#!/usr/bin/env bash
# bubbles/scripts/generate-validation-checks-selftest.sh
#
# Hermetic selftest for the IMP-047 S-E closure-map generator.
#
# Capability: declared-input-closure-validation
#
# WHAT THIS PINS
# `bubbles/registry/validation-checks.yaml` is the input set that change
# selection and result reuse are both decided from. Two properties make it
# trustworthy, and neither is self-evident:
#
#   1. IT IS DERIVED, NOT AUTHORED. A hand edit must be REFUSED by `--check`,
#      or the map becomes a place to widen reuse by typing `closureComplete:
#      true` next to a check nobody traced.
#   2. IT IS DETERMINISTIC. `--check` compares the committed file against a
#      fresh derivation. If two derivations of an unchanged tree can differ,
#      `--check` fails at random, and a check that cries wolf gets regenerated
#      reflexively — which is indistinguishable from not having it.
#
# Every case builds its own miniature repository under `mktemp`, so nothing here
# reads or depends on the state of the real closure map.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="generate-validation-checks-selftest"
GEN="$SCRIPT_DIR/generate-validation-checks.sh"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

if [[ ! -f "$GEN" ]]; then
  printf '%s: generator not found: %s\n' "$NAME" "$GEN" >&2
  exit 2
fi

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

FIX="$WORK/repo"
mkdir -p "$FIX/bubbles/scripts" "$FIX/bubbles/registry"

# A miniature framework: one selftest with a fully resolvable closure, and one
# check that walks the tree and therefore has none.
cat >"$FIX/bubbles/scripts/framework-validate.sh" <<'EOF'
#!/usr/bin/env bash
run_check "Alpha selftest" bash "$SCRIPT_DIR/alpha-selftest.sh"
run_check_self_only "Tree walker (live)" bash "$SCRIPT_DIR/tree-walker.sh"
EOF

cat >"$FIX/bubbles/scripts/alpha-selftest.sh" <<'EOF'
#!/usr/bin/env bash
source "$SCRIPT_DIR/guard-lib.sh"
REG="$SCRIPT_DIR/../registry/gates.yaml"
printf '%s\n' "$REG"
EOF

cat >"$FIX/bubbles/scripts/guard-lib.sh" <<'EOF'
#!/usr/bin/env bash
guard_lib_noop() { return 0; }
EOF

cat >"$FIX/bubbles/scripts/tree-walker.sh" <<'EOF'
#!/usr/bin/env bash
find "$REPO_ROOT" -name 'anything' -print
EOF

printf 'gates: []\n' >"$FIX/bubbles/registry/gates.yaml"

TARGET="$FIX/bubbles/registry/validation-checks.yaml"

# The block of a single check id, so an assertion about one entry cannot be
# satisfied by a line that belongs to a different entry.
entry_block() {
  LC_ALL=C awk -v want="  $2:" '
    $0 == want { inblock = 1; next }
    inblock && /^  vc-/ { inblock = 0 }
    inblock && /^[a-zA-Z]/ { inblock = 0 }
    inblock { print }
  ' "$1"
}

# --- P1. the derivation traces real references ------------------------------
write_out="$(bash "$GEN" --repo-root "$FIX" 2>&1)"
write_rc=$?
if [[ "$write_rc" -eq 0 ]] && [[ -f "$TARGET" ]]; then
  ok "P1 the generator writes a closure map (rc=0)"
else
  bad "P1 generator writes" "rc=$write_rc out=$(printf '%s' "$write_out" | tr '\n' '|')"
fi

alpha_block="$(entry_block "$TARGET" "vc-bubbles-scripts-alpha-selftest")"
if printf '%s\n' "$alpha_block" | grep -q '^    - bubbles/scripts/guard-lib\.sh$' &&
  printf '%s\n' "$alpha_block" | grep -q '^    - bubbles/registry/gates\.yaml$' &&
  printf '%s\n' "$alpha_block" | grep -q '^    closureComplete: true$'; then
  ok "P2 a sourced lib and a read registry both land in the traced closure"
else
  bad "P2 traced closure members" "block=$(printf '%s' "$alpha_block" | tr '\n' '|')"
fi

# --- P3. honest incompleteness ---------------------------------------------
# A check that walks the tree has no enumerable input set. Recording `true` here
# would be the staleness the whole map exists to remove.
walker_block="$(entry_block "$TARGET" "vc-bubbles-scripts-tree-walker")"
if printf '%s\n' "$walker_block" | grep -q '^    closureComplete: false$'; then
  ok "P3 a tree-walking check is recorded closureComplete: false"
else
  bad "P3 tree walker incomplete" "block=$(printf '%s' "$walker_block" | tr '\n' '|')"
fi

# --- P4. a clean regeneration verifies ---------------------------------------
check_out="$(bash "$GEN" --repo-root "$FIX" --check 2>&1)"
check_rc=$?
if [[ "$check_rc" -eq 0 ]] && printf '%s' "$check_out" | grep -q 'OK'; then
  ok "P4 --check accepts the file the generator just wrote"
else
  bad "P4 clean --check" "rc=$check_rc out=$(printf '%s' "$check_out" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: a hand edit inside the generated region is REFUSED -----
# The exact edit an author would reach for to widen reuse: flip an honest
# `false` to `true`. If --check tolerated this, the map would stop being a
# derivation and start being an assertion.
LC_ALL=C awk '
  { if ($0 == "    closureComplete: false") print "    closureComplete: true"; else print }
' "$TARGET" >"$WORK/handedited.yaml"
cp "$WORK/handedited.yaml" "$TARGET"
edit_out="$(bash "$GEN" --repo-root "$FIX" --check 2>&1)"
edit_rc=$?
if [[ "$edit_rc" -eq 1 ]] && printf '%s' "$edit_out" | grep -q 'DRIFT'; then
  ok "A1 a hand edit inside the generated file is refused as DRIFT"
else
  bad "A1 hand edit refused" "rc=$edit_rc out=$(printf '%s' "$edit_out" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: the derivation is DETERMINISTIC ------------------------
# Two derivations of an identical tree must be byte-identical. A generator whose
# closure member order wandered would make --check a permanent false alarm, and
# a permanent false alarm is regenerated without reading it.
run_a="$WORK/run-a.yaml"
run_b="$WORK/run-b.yaml"
bash "$GEN" --repo-root "$FIX" --stdout >"$run_a" 2>/dev/null
rc_a=$?
bash "$GEN" --repo-root "$FIX" --stdout >"$run_b" 2>/dev/null
rc_b=$?
det_diff="$(LC_ALL=C diff "$run_a" "$run_b" 2>&1 || true)"
if [[ "$rc_a" -eq 0 && "$rc_b" -eq 0 && -z "$det_diff" && -s "$run_a" ]]; then
  ok "A2 two derivations of an unchanged tree are byte-identical"
else
  bad "A2 deterministic derivation" "rc=$rc_a/$rc_b diff=$(printf '%s' "$det_diff" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: determinism is not vacuous -----------------------------
# A2 would also pass against a generator that emitted nothing at all. Prove the
# compared output actually carries the derived entries.
if grep -q '^  vc-bubbles-scripts-alpha-selftest:$' "$run_a" &&
  grep -q '^  vc-bubbles-scripts-tree-walker:$' "$run_a"; then
  ok "A3 the compared derivation is non-empty and names both derived checks"
else
  bad "A3 determinism non-vacuity" "run_a=$(wc -l <"$run_a" | tr -d ' ') lines"
fi

# --- A4. ADVERSARIAL: a changed input changes the derivation -----------------
# The inverse of A2. If the output were constant regardless of the tree, both
# determinism and drift detection would be meaningless.
cat >>"$FIX/bubbles/scripts/alpha-selftest.sh" <<'EOF'
EXTRA="$SCRIPT_DIR/../registry/extra.yaml"
printf '%s\n' "$EXTRA"
EOF
printf 'extra: []\n' >"$FIX/bubbles/registry/extra.yaml"
bash "$GEN" --repo-root "$FIX" --stdout >"$WORK/run-c.yaml" 2>/dev/null
if ! LC_ALL=C diff -q "$run_a" "$WORK/run-c.yaml" >/dev/null 2>&1 &&
  grep -q '^    - bubbles/registry/extra\.yaml$' "$WORK/run-c.yaml"; then
  ok "A4 adding a real reference changes the derivation and appears in it"
else
  bad "A4 changed input changes derivation" "extra recorded=$(grep -c 'extra\.yaml' "$WORK/run-c.yaml" | tr -d ' ')"
fi

# --- A5. ADVERSARIAL: --check on a missing map refuses, not passes -----------
rm -f "$TARGET"
missing_out="$(bash "$GEN" --repo-root "$FIX" --check 2>&1)"
missing_rc=$?
if [[ "$missing_rc" -eq 1 ]] && printf '%s' "$missing_out" | grep -q 'missing'; then
  ok "A5 --check refuses when the closure map is absent"
else
  bad "A5 missing map refused" "rc=$missing_rc out=$(printf '%s' "$missing_out" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: no bypass flag exists ----------------------------------
bypass_out="$(bash "$GEN" --repo-root "$FIX" --force 2>&1)"
bypass_rc=$?
if [[ "$bypass_rc" -eq 2 ]] && printf '%s' "$bypass_out" | grep -q 'no bypass'; then
  ok "A6 an unsupported bypass-shaped flag is refused with exit 2"
else
  bad "A6 no bypass" "rc=$bypass_rc out=$(printf '%s' "$bypass_out" | tr '\n' '|')"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
