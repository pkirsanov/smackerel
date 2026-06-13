#!/usr/bin/env bash
# env-pollution-scan-selftest.sh — hermetic selftest for env-pollution-scan.sh.
#
# Asserts that env-pollution-scan.sh:
#   1. Passes on a clean repo (no test code writes to prod surfaces)
#   2. Flags writes to /srv/backups/* paths
#   3. Flags writes to knb manifest paths (regex + not \+)
#   4. Flags writes to config/release-trains.yaml from test code
#   5. Flags writes to config/feature-flags.<train>.yaml from test code
#   6. Allows comment-only mentions (no write verb)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAN="$SCRIPT_DIR/env-pollution-scan.sh"

[[ -x "$SCAN" ]] || { echo "FAIL: $SCAN not executable" >&2; exit 1; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-pollution.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# Initialize: ensure tests dir is fresh each time via helper
reset_tmp() {
  rm -rf "$TMP" && mkdir -p "$TMP/tests"
}

assert_pass() {
  local desc="$1"
  if "$SCAN" "$TMP" </dev/null >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 0)" >&2
    "$SCAN" "$TMP" </dev/null >&2 || true
    exit 1
  fi
}

assert_fail() {
  local desc="$1" pattern="$2"
  local output
  output="$("$SCAN" "$TMP" </dev/null 2>&1)" && {
    echo "FAIL: $desc (expected exit 1, got 0)" >&2
    exit 1
  }
  if echo "$output" | grep -q "$pattern"; then
    echo "PASS: $desc (matched $pattern)"
  else
    echo "FAIL: $desc (output did not match $pattern)" >&2
    echo "$output" >&2
    exit 1
  fi
}

# 1. Clean repo
reset_tmp
echo "# nothing here" > "$TMP/tests/clean_test.py"
assert_pass "clean test code"

# 2. Write to /srv/backups/
reset_tmp
cat > "$TMP/tests/bad_backup_test.py" <<'EOF'
def test_bad():
    write_to("/srv/backups/test-fixture")
EOF
assert_fail "writes to /srv/backups/ flagged" "/srv/backups/"

# 3. Write to knb manifest path (regression for + vs \+ regex bug)
reset_tmp
cat > "$TMP/tests/bad_manifest_test.py" <<'EOF'
def test_bad():
    update("knb/exampleproduct/home-lab/manifest.yaml", {})
EOF
assert_fail "writes to knb manifest path flagged" "knb"

# 4. Write to config/release-trains.yaml
reset_tmp
cat > "$TMP/tests/bad_trains_test.py" <<'EOF'
def test_bad():
    patch("config/release-trains.yaml", value="x")
EOF
assert_fail "writes to release-trains.yaml flagged" "release-trains"

# 5. Write to config/feature-flags.<train>.yaml
reset_tmp
cat > "$TMP/tests/bad_flags_test.py" <<'EOF'
def test_bad():
    write("config/feature-flags.mvp.yaml", "flags:")
EOF
assert_fail "writes to feature-flags bundle flagged" "feature-flags"

# 6. Comment-only mention (no write verb anywhere on line) → allowed
rm -rf "$TMP" && mkdir -p "$TMP/tests"
cat > "$TMP/tests/comment_test.py" <<'EOF'
# This test references /srv/backups/ in a comment only.
# It also references knb/exampleproduct/home-lab/manifest.yaml as documentation.
def test_clean():
    assert True
EOF
assert_pass "comment-only mention allowed"

# 7. Regression: the scanner must NOT self-match framework '*selftest.sh' files.
#    Before the glob dot-escaping fix, the '**/*.test.*' glob compiled to the
#    regex '.test.', whose unescaped dots matched '...selftest.sh' and flagged
#    this scanner's own fixtures during a full-repo pre-push scan (G115 false
#    positive). A '*selftest.sh' file outside a test dir must pass clean.
reset_tmp
cat > "$TMP/pipeline-selftest.sh" <<'EOF'
def deploy():
    write_to("/srv/backups/should-not-be-scanned")
EOF
assert_pass "framework *selftest.sh not self-matched (glob dot-escaping)"

# 8. ADVERSARIAL telemetry isolation (SCOPE-3 T3.6): an in-test env=prod
#    telemetry WRITE must BLOCK (the scan still protects prod monitoring). This
#    proves the guard fails on a real test-to-prod-telemetry regression.
reset_tmp
cat > "$TMP/tests/telemetry_prod_test.py" <<'EOF'
def test_emits_prod_telemetry():
    # Pushing a metric tagged env="prod" from test code is forbidden.
    push_metric("gateway.request.p99", env="prod")
EOF
assert_fail "env=prod telemetry write blocked" 'prod'

# 9. ADVERSARIAL telemetry isolation (SCOPE-3 T3.6): the SAME write tagged
#    env=test MUST pass — the validate plane writes only to env=test* surfaces.
reset_tmp
cat > "$TMP/tests/telemetry_test_env_test.py" <<'EOF'
def test_emits_test_telemetry():
    # The validate-plane test stack uses env="test*" labels — allowed.
    push_metric("gateway.request.p99", env="test")
EOF
assert_pass "env=test telemetry write allowed"

echo "All env-pollution-scan selftests passed."
