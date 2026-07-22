#!/usr/bin/env bash
# gate-strength-lint-selftest.sh — hermetic selftest for gate-strength-lint.sh.
#
# Builds a throwaway gate registry with one gate per enforcement class and
# asserts the deterministic classifier assigns the expected class, plus the
# completeness (publish) path and the graceful-skip path. Requires yq (the
# classifier's only dependency); skips cleanly if absent.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/gate-strength-lint.sh"

if ! command -v yq >/dev/null 2>&1; then
  echo "SKIP: gate-strength-lint selftest (yq not available)"
  exit 0
fi

pass=0
fail=0
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

mkdir -p "$TMP_ROOT/bubbles/registry"
cat > "$TMP_ROOT/bubbles/registry/gates.yaml" <<'YAML'
gates:
  G001:
    name: block_gate
    description: ABSOLUTE BLOCKING gate. Artifacts MUST exist.
  G002:
    name: adv_gate
    description: Advisory informational check only.
  G003:
    name: behav_gate
    description: Enforced by bubbles.workflow agent behavior, not by script.
  G004:
    name: cond_gate
    description: Conditional gate; a no-op when no UI is touched.
  G005:
    name: ext_gate
    description: Enforced by the CI pipeline (external system).
YAML

out="$(bash "$LINT" --list "$TMP_ROOT")"

check() {
  local gid="$1" exp="$2" got
  got="$(printf '%s\n' "$out" | awk -v g="$gid" '$1==g{print $2}')"
  if [[ "$got" == "$exp" ]]; then
    echo "PASS: $gid -> $exp"
    pass=$((pass + 1))
  else
    echo "FAIL: $gid expected $exp got '$got'"
    fail=$((fail + 1))
  fi
}

check G001 mechanical-blocking
check G002 mechanical-advisory
check G003 behavioral-contract
check G004 conditional-noop
check G005 external-enforcement

if bash "$LINT" "$TMP_ROOT" >/dev/null 2>&1; then
  echo "PASS: publish mode exits 0 on a complete registry"
  pass=$((pass + 1))
else
  echo "FAIL: publish mode returned nonzero on a complete registry"
  fail=$((fail + 1))
fi

mkdir -p "$TMP_ROOT/empty"
if bash "$LINT" "$TMP_ROOT/empty" >/dev/null 2>&1; then
  echo "PASS: absent registry skips gracefully"
  pass=$((pass + 1))
else
  echo "FAIL: absent registry returned nonzero"
  fail=$((fail + 1))
fi

echo
echo "gate-strength-lint selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
