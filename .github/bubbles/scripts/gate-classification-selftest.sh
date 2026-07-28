#!/usr/bin/env bash
# gate-classification-selftest.sh — hermetic selftest.
#
# Every gate must be classified, every value must be legal, and the
# workflows.yaml block must be regenerable from the registry so the two
# cannot drift apart.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/gate-classification.sh"

if [[ ! -f "$TOOL" ]]; then
  echo "gate-classification-selftest: tool not found: $TOOL" >&2
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1 || ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-classification-selftest: SKIP (python3 + PyYAML not installed)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

pass_count=0
fail_count=0

assert() {
  local name="$1" expected="$2" repo="$3" sub="${4:-lint}" actual=0 output=""
  output="$(bash "$TOOL" "$sub" --repo-root "$repo" 2>&1)" || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS  $name (exit $actual)"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name — expected exit $expected, got $actual"
    echo "      output: $output"
    fail_count=$((fail_count + 1))
  fi
}

# make_repo <dir> <classification-line-or-empty>
make_repo() {
  local d="$1" cls="$2"
  mkdir -p "$d/bubbles/registry"
  {
    echo "gates:"
    echo "  G001:"
    echo "    name: example_gate"
    [[ -n "$cls" ]] && echo "    classification: $cls"
    echo "    description: Models fabricate evidence for this one."
  } >"$d/bubbles/registry/gates.yaml"
  cat >"$d/bubbles/workflows.yaml" <<'EOF'
gateClassification:
  modelCompensation:
  - G001 # stale
priorityScoring:
  model: weighted_sum
EOF
}

# --- GREEN ------------------------------------------------------------------
g1="$WORK/g1"
make_repo "$g1" "modelCompensation"
assert "green: modelCompensation is legal" 0 "$g1"

g2="$WORK/g2"
make_repo "$g2" "businessInvariant"
assert "green: businessInvariant is legal" 0 "$g2"

g3="$WORK/g3"
make_repo "$g3" "hybrid"
assert "green: hybrid is legal" 0 "$g3"

# --- RED --------------------------------------------------------------------
r1="$WORK/r1"
make_repo "$r1" ""
assert "red: unclassified gate is caught" 1 "$r1"

r2="$WORK/r2"
make_repo "$r2" "someMadeUpClass"
assert "red: illegal classification value is caught" 1 "$r2"

# --- bind seeds only what is missing, and never overwrites ------------------
b1="$WORK/b1"
make_repo "$b1" ""
assert "green: bind seeds a missing classification" 0 "$b1" bind
if grep -q 'classification: modelCompensation' "$b1/bubbles/registry/gates.yaml"; then
  echo "PASS  bind derived modelCompensation from the 'fabricate' signal"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  bind did not derive the expected class from the description signal"
  fail_count=$((fail_count + 1))
fi

b2="$WORK/b2"
make_repo "$b2" "businessInvariant"
assert "green: bind is a no-op when already classified" 0 "$b2" bind
if grep -q 'classification: businessInvariant' "$b2/bubbles/registry/gates.yaml"; then
  echo "PASS  bind preserved a hand-made classification"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  bind overwrote a hand-made classification"
  fail_count=$((fail_count + 1))
fi

# --- emit regenerates the workflows block from the registry -----------------
e1="$WORK/e1"
make_repo "$e1" "businessInvariant"
assert "green: emit regenerates the block" 0 "$e1" emit
if grep -q 'businessInvariant:' "$e1/bubbles/workflows.yaml" &&
  ! grep -q '# stale' "$e1/bubbles/workflows.yaml" &&
  grep -q '^priorityScoring:' "$e1/bubbles/workflows.yaml"; then
  echo "PASS  emit replaced the stale block and preserved the following key"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  emit did not correctly replace the gateClassification block"
  fail_count=$((fail_count + 1))
fi

e2="$WORK/e2"
make_repo "$e2" ""
assert "red: emit refuses while a gate is unclassified" 1 "$e2" emit

echo ""
echo "gate-classification selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All gate-classification selftests passed."
exit 0
