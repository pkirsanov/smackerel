#!/usr/bin/env bash
# intent-routes-lint-selftest.sh — hermetic selftest for intent-routes-lint.sh.
#
# Cases:
#   1. Clean fixture → exit 0
#   2. Missing version → exit 1
#   3. routes[] empty → exit 1
#   4. Duplicate phrase across two routes → exit 1
#   5. Uppercase phrase → exit 1
#   6. Unknown targetAgent → exit 1
#   7. Unknown targetMode → exit 1
#   8. Missing summary → exit 1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/intent-routes-lint.sh"

[[ -x "$LINT" ]] || { echo "FAIL: $LINT not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-intent.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

setup_clean_fixture() {
  rm -rf "$TMP"
  mkdir -p "$TMP/bubbles"

  cat > "$TMP/bubbles/agent-capabilities.yaml" <<'EOF'
version: 1
agents:
  bubbles.alpha:
    class: orchestrator
  bubbles.beta:
    class: utility
EOF

  cat > "$TMP/bubbles/workflows.yaml" <<'EOF'
modes:
  mode-a:
    description: "a"
  mode-b:
    description: "b"
EOF

  cat > "$TMP/bubbles/intent-routes.yaml" <<'EOF'
version: 1
routes:
  - phrases:
      - "do alpha thing"
      - "alpha please"
    targetAgent: bubbles.alpha
    targetMode: mode-a
    summary: alpha route
  - phrases:
      - "do beta thing"
    targetAgent: bubbles.beta
    targetMode: mode-b
    summary: beta route
EOF
}

assert_pass() {
  local desc="$1"
  if timeout 10 "$LINT" "$TMP" </dev/null >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    local rc=$?
    echo "FAIL: $desc (expected exit 0, got $rc)" >&2
    timeout 5 "$LINT" "$TMP" </dev/null >&2 || true
    exit 1
  fi
}

assert_fail() {
  local desc="$1"
  local rc=0
  timeout 10 "$LINT" "$TMP" </dev/null >/dev/null 2>&1 || rc=$?
  if [[ $rc -eq 1 ]]; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 1, got $rc)" >&2
    timeout 5 "$LINT" "$TMP" </dev/null >&2 || true
    exit 1
  fi
}

# 1. Clean fixture
setup_clean_fixture
assert_pass "clean fixture"

# 2. Missing version
setup_clean_fixture
yq -i 'del(.version)' "$TMP/bubbles/intent-routes.yaml"
assert_fail "missing version rejected"

# 3. routes[] empty
setup_clean_fixture
yq -i '.routes = []' "$TMP/bubbles/intent-routes.yaml"
assert_fail "routes[] empty rejected"

# 4. Duplicate phrase
setup_clean_fixture
yq -i '.routes[1].phrases += ["alpha please"]' "$TMP/bubbles/intent-routes.yaml"
assert_fail "duplicate phrase rejected"

# 5. Uppercase phrase
setup_clean_fixture
yq -i '.routes[0].phrases[0] = "Do Alpha Thing"' "$TMP/bubbles/intent-routes.yaml"
assert_fail "uppercase phrase rejected"

# 6. Unknown agent
setup_clean_fixture
yq -i '.routes[0].targetAgent = "bubbles.unknown"' "$TMP/bubbles/intent-routes.yaml"
assert_fail "unknown agent rejected"

# 7. Unknown mode
setup_clean_fixture
yq -i '.routes[0].targetMode = "mode-zzz"' "$TMP/bubbles/intent-routes.yaml"
assert_fail "unknown mode rejected"

# 8. Missing summary
setup_clean_fixture
yq -i 'del(.routes[0].summary)' "$TMP/bubbles/intent-routes.yaml"
assert_fail "missing summary rejected"

echo "All intent-routes-lint selftests passed."
