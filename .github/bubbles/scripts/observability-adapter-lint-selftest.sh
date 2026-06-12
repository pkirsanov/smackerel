#!/usr/bin/env bash
# observability-adapter-lint-selftest.sh — hermetic selftest.
#
# Cases:
#   1. Real bubbles adapter dir (none + prometheus) → exit 0
#      (also proves prometheus `selftest` normalizes the raw envelope to an
#      array and none emits []/{} per-verb — the R2-D shapes).
#   2. Missing none.sh → exit 1
#   3. Adapter missing verb in case statement → exit 1
#   4. Adapter not executable → exit 1
#   5. none.sh returns non-JSON output → exit 1
#   6. ADVERSARIAL (R2-D/R2-G): none.sh returns `{}` (object) for fetch-alerts
#      — wrong shape, must be an array → exit 1
#   7. ADVERSARIAL (R2-D/R2-G): a provider adapter whose `selftest fetch-alerts`
#      emits the RAW PROVIDER ENVELOPE (object) instead of the normalized array
#      → exit 1. Uses the shipped negative fixture
#      bubbles/tests/fixtures/observability/payload-alerts-raw-envelope.invalid.json.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LINT="$SCRIPT_DIR/observability-adapter-lint.sh"
RAW_ENVELOPE_FIXTURE="$REPO_ROOT/bubbles/tests/fixtures/observability/payload-alerts-raw-envelope.invalid.json"

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
  # Baseline none.sh emits the CANONICAL per-verb shapes (R2-D): [] for
  # fetch-alerts, {} for the other three. Failure cases below introduce a
  # SINGLE defect each so they fail for exactly their intended reason.
  cat > "$TMP/bubbles/adapters/observability/none.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts) echo '[]' ;;
  fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
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

# 5. none.sh returns non-JSON output
setup_fixture
cat > "$TMP/bubbles/adapters/observability/none.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo 'WRONG' ;;
esac
EOF
chmod +x "$TMP/bubbles/adapters/observability/none.sh"
assert_fail "none.sh returning non-JSON output rejected"

# 6. ADVERSARIAL (R2-D): none.sh returns `{}` (object) for fetch-alerts — wrong
#    shape; fetch-alerts MUST be a JSON array. Proves the per-verb shape check
#    would FAIL on a regression to the old "{} for every verb" contract.
setup_fixture
cat > "$TMP/bubbles/adapters/observability/none.sh" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  fetch-alerts) echo '{}' ;;
  fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$TMP/bubbles/adapters/observability/none.sh"
assert_fail "none.sh '{}' for fetch-alerts rejected (must be array)"

# 7. ADVERSARIAL (R2-D/R2-G): a provider adapter whose `selftest fetch-alerts`
#    emits the RAW PROVIDER ENVELOPE (an object) instead of the normalized
#    array. Uses the shipped negative fixture so the lint is proven to reject a
#    real un-normalized Prometheus response.
setup_fixture
if [[ -f "$RAW_ENVELOPE_FIXTURE" ]]; then
  cp "$RAW_ENVELOPE_FIXTURE" "$TMP/raw-envelope.json"
else
  printf '%s' '{"status":"success","data":{"alerts":[]}}' > "$TMP/raw-envelope.json"
fi
cat > "$TMP/bubbles/adapters/observability/badprovider.sh" <<EOF
#!/usr/bin/env bash
case "\${1:-}" in
  selftest)
    case "\${2:-}" in
      fetch-alerts) cat "$TMP/raw-envelope.json" ;;  # RAW envelope (object) — WRONG, must be array
      fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
      *) exit 1 ;;
    esac ;;
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact) echo '{}' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$TMP/bubbles/adapters/observability/badprovider.sh"
assert_fail "selftest emitting raw provider envelope for fetch-alerts rejected"

echo "All observability-adapter-lint selftests passed."
