#!/usr/bin/env bash
# propagation-policy-guard-selftest.sh — hermetic selftest for
# propagation-policy-guard.sh.
#
# Cases:
#   1. No policy file → exit 0 with skip message
#   1b. No policy file with --require-policy → exit 1 (G121)
#   2. Clean fixture → exit 0
#   3. version != 1 → exit 1
#   4. trains[] empty → exit 1
#   5. Train id not in release-trains.yaml → exit 1
#   6. Edge missing receivingTrainValidationMode → exit 1 (G122)
#   7. Edge mode=none without validationSkipReason → exit 1 (G122)
#   8. backportRequiresApproval missing → exit 1
#   9. Absolute ledgerPath → exit 1
#  10. Invalid JSONL ledger line → exit 1 (G123)
#  11. Edge from/to references undeclared train → exit 1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/propagation-policy-guard.sh"

[[ -x "$GUARD" ]] || { echo "FAIL: $GUARD not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-prop-guard.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
SELFTEST_TIMEOUT="${BUBBLES_PROPAGATION_GUARD_SELFTEST_TIMEOUT_SECONDS:-60}"

setup_clean_fixture() {
  rm -rf "$TMP"
  mkdir -p "$TMP/config"
  cat > "$TMP/config/release-trains.yaml" <<'EOF'
version: 1
defaults:
  retention: "7d-daily"
  pii: "none"
trains:
  - id: experimental
    phase: active
    target_slot: none
    flags_bundle: config/feature-flags.experimental.yaml
  - id: mvp
    phase: active
    target_slot: staging
    flags_bundle: config/feature-flags.mvp.yaml
  - id: prod
    phase: active
    target_slot: prod
    flags_bundle: config/feature-flags.prod.yaml
EOF
  for t in experimental mvp prod; do
    cat > "$TMP/config/feature-flags.$t.yaml" <<EOF
version: 1
train: $t
flags: {}
EOF
  done

  cat > "$TMP/propagation-policy.yaml" <<'EOF'
version: 1
trains:
  - id: experimental
    role: incoming
  - id: mvp
    role: staging
  - id: prod
    role: production
defaultFlow:
  - from: experimental
    to: mvp
    auto: true
    receivingTrainValidationMode: validate-only
  - from: mvp
    to: prod
    auto: false
    receivingTrainValidationMode: full-delivery
backportRequiresApproval: true
ledgerPath: propagation-ledger.yaml
EOF
}

assert_pass() {
  local desc="$1"
  if timeout "$SELFTEST_TIMEOUT" "$GUARD" "$TMP" </dev/null >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    local rc=$?
    echo "FAIL: $desc (expected exit 0, got $rc)" >&2
    timeout 5 "$GUARD" "$TMP" </dev/null >&2 || true
    exit 1
  fi
}

assert_fail() {
  local desc="$1"
  local rc=0
  timeout "$SELFTEST_TIMEOUT" "$GUARD" "$TMP" </dev/null >/dev/null 2>&1 || rc=$?
  if [[ $rc -eq 1 ]]; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 1, got $rc)" >&2
    timeout 5 "$GUARD" "$TMP" </dev/null >&2 || true
    exit 1
  fi
}

# 1. No policy file → skip
setup_clean_fixture
rm "$TMP/propagation-policy.yaml"
assert_pass "no policy file → skip"

# 1b. No policy file with --require-policy → blocking G121
setup_clean_fixture
rm "$TMP/propagation-policy.yaml"
rc=0
timeout "$SELFTEST_TIMEOUT" "$GUARD" --require-policy "$TMP" </dev/null >/dev/null 2>&1 || rc=$?
if [[ $rc -eq 1 ]]; then
  echo "PASS: no policy file with --require-policy rejected (G121)"
else
  echo "FAIL: no policy file with --require-policy rejected (expected exit 1, got $rc)" >&2
  exit 1
fi

# 2. Clean fixture
setup_clean_fixture
assert_pass "clean fixture"

# 3. version != 1
setup_clean_fixture
yq -i '.version = 2' "$TMP/propagation-policy.yaml"
assert_fail "version != 1 rejected"

# 4. trains[] empty
setup_clean_fixture
yq -i '.trains = []' "$TMP/propagation-policy.yaml"
assert_fail "trains[] empty rejected"

# 5. Policy train not in release-trains.yaml
setup_clean_fixture
yq -i '.trains[0].id = "ghost-train"' "$TMP/propagation-policy.yaml"
yq -i '.defaultFlow[0].from = "ghost-train"' "$TMP/propagation-policy.yaml"
assert_fail "policy train not in release-trains.yaml rejected"

# 6. Edge missing receivingTrainValidationMode
setup_clean_fixture
yq -i 'del(.defaultFlow[0].receivingTrainValidationMode)' "$TMP/propagation-policy.yaml"
assert_fail "edge missing receivingTrainValidationMode rejected (G122)"

# 7. Edge mode=none without validationSkipReason
setup_clean_fixture
yq -i '.defaultFlow[0].receivingTrainValidationMode = "none"' "$TMP/propagation-policy.yaml"
assert_fail "edge mode=none without validationSkipReason rejected (G122)"

# 8. backportRequiresApproval missing
setup_clean_fixture
yq -i 'del(.backportRequiresApproval)' "$TMP/propagation-policy.yaml"
assert_fail "backportRequiresApproval missing rejected"

# 9. Absolute ledgerPath
setup_clean_fixture
yq -i '.ledgerPath = "/srv/ledger.yaml"' "$TMP/propagation-policy.yaml"
assert_fail "absolute ledgerPath rejected"

# 10. Invalid JSONL ledger line
if command -v jq >/dev/null 2>&1; then
  setup_clean_fixture
  printf 'this is not json\n' > "$TMP/propagation-ledger.yaml"
  assert_fail "invalid JSONL ledger line rejected (G123)"

  setup_clean_fixture
  cat > "$TMP/propagation-ledger.yaml" <<'EOF'
{"timestamp":"2026-06-03T00:00:00Z","operator":"tester","operation":"backport","fromTrain":"prod","toTrain":"mvp","commits":["abc123"],"validationMode":"validate-only","validationOutcome":"passed","approvalToken":null}
EOF
  assert_fail "backport ledger without approvalToken rejected (G123)"
else
  echo "SKIP: jq not installed, ledger format check"
fi

# 11. Edge from/to references undeclared train
setup_clean_fixture
yq -i '.defaultFlow[0].to = "phantom"' "$TMP/propagation-policy.yaml"
assert_fail "edge referencing undeclared train rejected"

echo "All propagation-policy-guard selftests passed."
