#!/usr/bin/env bash
# gate-enforcement-selftest.sh — hermetic selftest for gate-enforcement.sh.
#
# Builds throwaway registries under mktemp and asserts the lint's verdict.
# Every dangling-declaration class gets an adversarial fixture, because a lint
# that has never been observed failing is not enforcement.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/gate-enforcement.sh"

if [[ ! -f "$TOOL" ]]; then
  echo "gate-enforcement-selftest: tool not found: $TOOL" >&2
  exit 2
fi

# Resolve the same managed interpreter the tool resolves. Without this the
# selftest probed the bare python3 and skipped on hosts where the tool itself
# runs fine — a selftest that never executes cannot witness a regression.
if [[ -f "$SCRIPT_DIR/dependency-posture.sh" ]]; then
  # shellcheck source=/dev/null
  . "$SCRIPT_DIR/dependency-posture.sh"
fi

if ! command -v python3 >/dev/null 2>&1 || ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-enforcement-selftest: SKIP (python3 + PyYAML not installed)"
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

# make_repo <dir> <enforcedBy-value>
make_repo() {
  local d="$1" value="$2"
  mkdir -p "$d/bubbles/registry" "$d/bubbles/scripts" "$d/agents"
  cat >"$d/bubbles/registry/gates.yaml" <<EOF
gates:
  G001:
    name: example_gate
    enforcedBy: [$value]
    description: An example gate.
EOF
  cat >"$d/bubbles/scripts/state-transition-guard.sh" <<'EOF'
#!/usr/bin/env bash
echo "--- Check 1: Something ---"
EOF
  cat >"$d/bubbles/scripts/real-guard.sh" <<'EOF'
#!/usr/bin/env bash
# Gate G001 — example_gate.
echo real
EOF
  cat >"$d/bubbles/scripts/silent-guard.sh" <<'EOF'
#!/usr/bin/env bash
echo silent
EOF
  cat >"$d/agents/bubbles.workflow.agent.md" <<'EOF'
---
description: example
---
EOF
}

# --- GREEN cases ------------------------------------------------------------
g1="$WORK/g1"
make_repo "$g1" "guard-check:1"
assert "green: resolvable guard-check" 0 "$g1"

g2="$WORK/g2"
make_repo "$g2" "script:bubbles/scripts/real-guard.sh"
assert "green: resolvable script that names the gate" 0 "$g2"

g3="$WORK/g3"
make_repo "$g3" "behavioral:bubbles.workflow"
assert "green: resolvable agent" 0 "$g3"

g4="$WORK/g4"
make_repo "$g4" "mode-required"
assert "green: mode-required needs no target" 0 "$g4"

g5="$WORK/g5"
make_repo "$g5" "unbound"
assert "green: unbound is honest, not a lint failure" 0 "$g5"

# --- RED cases --------------------------------------------------------------
r1="$WORK/r1"
make_repo "$r1" "guard-check:99Z"
assert "red: dangling guard-check label" 1 "$r1"

r2="$WORK/r2"
make_repo "$r2" "script:bubbles/scripts/does-not-exist.sh"
assert "red: dangling script path" 1 "$r2"

# IMP-049 SCOPE-5: existence alone is too weak. A gate may name a real script
# that never mentions it, which is a declaration nothing verifies.
r2b="$WORK/r2b"
make_repo "$r2b" "script:bubbles/scripts/silent-guard.sh"
assert "red: declared script exists but never names the gate id" 1 "$r2b"

r3="$WORK/r3"
make_repo "$r3" "behavioral:bubbles.nonexistent"
assert "red: dangling agent reference" 1 "$r3"

r4="$WORK/r4"
make_repo "$r4" "sorcery:magic"
assert "red: vocabulary outside the constrained set" 1 "$r4"

r5="$WORK/r5"
mkdir -p "$r5/bubbles/registry" "$r5/bubbles/scripts"
cat >"$r5/bubbles/registry/gates.yaml" <<'EOF'
gates:
  G001:
    name: undeclared_gate
    description: A gate with no enforcedBy at all.
EOF
assert "red: gate missing enforcedBy entirely" 1 "$r5"

# --- bind is idempotent and never overwrites --------------------------------
b1="$WORK/b1"
make_repo "$b1" "mode-required"
assert "green: bind is a no-op when every gate declares" 0 "$b1" bind
if grep -q 'enforcedBy: \[mode-required\]' "$b1/bubbles/registry/gates.yaml"; then
  echo "PASS  bind preserved the existing declaration"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  bind overwrote an existing declaration"
  fail_count=$((fail_count + 1))
fi

# bind seeds a missing declaration
b2="$WORK/b2"
mkdir -p "$b2/bubbles/registry" "$b2/bubbles/scripts"
cat >"$b2/bubbles/registry/gates.yaml" <<'EOF'
gates:
  G001:
    name: seeded_gate
    description: No enforcer anywhere.
EOF
cat >"$b2/bubbles/scripts/state-transition-guard.sh" <<'EOF'
#!/usr/bin/env bash
echo none
EOF
assert "green: bind runs on an unseeded registry" 0 "$b2" bind
if grep -q 'enforcedBy: \[unbound\]' "$b2/bubbles/registry/gates.yaml"; then
  echo "PASS  bind marked an unenforceable gate 'unbound' rather than inventing a binding"
  pass_count=$((pass_count + 1))
else
  echo "FAIL  bind did not mark the unenforceable gate 'unbound'"
  fail_count=$((fail_count + 1))
fi

echo ""
echo "gate-enforcement selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All gate-enforcement selftests passed."
exit 0
