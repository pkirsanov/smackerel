#!/usr/bin/env bash
# observability-adapter-lint-selftest.sh — hermetic selftest.
#
# Cases:
#   1. Real bubbles adapter dir (none + prometheus) → exit 0
#   2. Missing none.sh → exit 1
#   3. Adapter missing verb in case statement → exit 1
#   4. Adapter not executable → exit 1
#   5. none.sh returns wrong output → exit 1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LINT="$SCRIPT_DIR/observability-adapter-lint.sh"

[[ -x "$LINT" ]] || { echo "FAIL: $LINT not executable" >&2; exit 1; }

# 1. Real bubbles adapter dir should pass
out="$("$LINT" "$REPO_ROOT" 2>&1)" || { echo "FAIL: real adapter dir should pass"; echo "$out"; exit 1; }
echo "PASS: real adapter dir (none + prometheus) passes"

# Now build a fixture for the failure cases.
TMP="$(mktemp -d "${HOME}/.bubbles-selftest-obs-adapter.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

setup_fixture() {
  rm -rf "$TMP"
  mkdir -p "$TMP/bubbles/adapters/observability"
  cat > "$TMP/bubbles/adapters/observability/none.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "$TMP/bubbles/adapters/observability/none.sh"
}

assert_pass() {
  local desc="$1"
  if "$LINT" "$TMP" >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 0)"; "$LINT" "$TMP"; exit 1
  fi
}
assert_fail() {
  local desc="$1"
  local rc=0
  "$LINT" "$TMP" >/dev/null 2>&1 || rc=$?
  if [[ $rc -eq 1 ]]; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 1, got $rc)"; "$LINT" "$TMP"; exit 1
  fi
}

# 2. Missing none.sh
setup_fixture
rm "$TMP/bubbles/adapters/observability/none.sh"
assert_fail "missing none.sh rejected"

# 3. Adapter missing verb in case statement
setup_fixture
cat > "$TMP/bubbles/adapters/observability/incomplete.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts) echo '{}' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$TMP/bubbles/adapters/observability/incomplete.sh"
assert_fail "adapter missing verb rejected"

# 4. Adapter not executable
setup_fixture
cat > "$TMP/bubbles/adapters/observability/notexec.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
esac
EOF
# Intentionally NOT chmod +x
assert_fail "adapter not executable rejected"

# 5. none.sh returns wrong output
setup_fixture
cat > "$TMP/bubbles/adapters/observability/none.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo 'WRONG' ;;
esac
EOF
chmod +x "$TMP/bubbles/adapters/observability/none.sh"
assert_fail "none.sh returning wrong output rejected"

echo "All observability-adapter-lint selftests passed."
