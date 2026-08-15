#!/usr/bin/env bash
# capability-consumer-naming-selftest.sh — prove the naming guard can fail.
#
# IMP-042 SCOPE-13 / COV-15.
#
# The live ledger passes, which is exactly when a guard is easiest to get wrong
# without noticing. These fixtures pin each decision the guard makes.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/capability-consumer-naming.sh"
LABEL="capability-consumer-naming-selftest"

pass_count=0
fail_count=0
pass() {
  printf '[%s] PASS %s\n' "$LABEL" "$*"
  pass_count=$((pass_count + 1))
}
fail() {
  printf '[%s] FAIL %s\n' "$LABEL" "$*" >&2
  fail_count=$((fail_count + 1))
}

# $1 = root, $2 = ledger body
build() {
  local root="$1" body="$2"
  mkdir -p "$root/bubbles/scripts"
  printf '%s\n' "$body" >"$root/bubbles/capability-ledger.yaml"
}

run_case() {
  local name="$1" expected="$2" root="$3"
  local actual
  bash "$GUARD" "$root" >/dev/null 2>&1
  actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    pass "$name (exit $actual)"
  else
    fail "$name (expected $expected, got $actual)"
  fi
  rm -rf "$root"
}

named_ledger='version: 1
capabilities:
  demo-capability:
    state: shipped
    consumers:
    - bubbles/scripts/demo-consumer.sh
'

# --- GREEN: the consumer names its capability -------------------------------
root="$(mktemp -d)"
build "$root" "$named_ledger"
printf '#!/usr/bin/env bash\n# Capability: demo-capability\necho ok\n' >"$root/bubbles/scripts/demo-consumer.sh"
run_case "T1 a consumer naming its capability passes" 0 "$root"

# --- RED: the consumer does not name it -------------------------------------
root="$(mktemp -d)"
build "$root" "$named_ledger"
printf '#!/usr/bin/env bash\necho ok\n' >"$root/bubbles/scripts/demo-consumer.sh"
run_case "T2 a consumer that never names its capability fails" 1 "$root"

# --- Non-shipped capabilities are out of scope ------------------------------
root="$(mktemp -d)"
build "$root" 'version: 1
capabilities:
  demo-capability:
    state: partial
    consumers:
    - bubbles/scripts/demo-consumer.sh
  other-capability:
    state: shipped
    consumers:
    - bubbles/scripts/other-consumer.sh
'
printf '#!/usr/bin/env bash\necho unnamed\n' >"$root/bubbles/scripts/demo-consumer.sh"
printf '#!/usr/bin/env bash\n# Capability: other-capability\necho ok\n' >"$root/bubbles/scripts/other-consumer.sh"
run_case "T3 a partial capability's unnamed consumer is ignored" 0 "$root"

# --- Non-executable consumers are out of scope ------------------------------
root="$(mktemp -d)"
build "$root" 'version: 1
capabilities:
  demo-capability:
    state: shipped
    consumers:
    - agents/bubbles.demo.agent.md
    - bubbles/scripts/demo-consumer.sh
'
mkdir -p "$root/agents"
printf '# an agent that never says the id\n' >"$root/agents/bubbles.demo.agent.md"
printf '#!/usr/bin/env bash\n# Capability: demo-capability\necho ok\n' >"$root/bubbles/scripts/demo-consumer.sh"
run_case "T4 a markdown consumer is not required to name the capability" 0 "$root"

# --- A ledger with no executable consumers is an error, not a silent pass ----
root="$(mktemp -d)"
build "$root" 'version: 1
capabilities:
  demo-capability:
    state: shipped
    consumers:
    - agents/bubbles.demo.agent.md
'
run_case "T5 no executable consumers at all exits 2 rather than reporting OK" 2 "$root"

# --- A missing ledger must refuse -------------------------------------------
root="$(mktemp -d)"
run_case "T6 a missing ledger exits 2" 2 "$root"

printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
