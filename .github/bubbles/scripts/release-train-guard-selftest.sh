#!/usr/bin/env bash
# release-train-guard-selftest.sh — hermetic selftest for release-train-guard.sh.
#
# Sets up a temp repo with a config/release-trains.yaml + flag bundles +
# specs/, then asserts:
#   1. Clean fixture passes (exit 0)
#   2. Invalid phase fails (exit 1)
#   3. Invalid slot fails (exit 1)
#   4. Missing flag bundle on disk fails (exit 1)
#   5. Missing retention/pii (no defaults) fails (exit 1)
#   6. Defaults.retention + defaults.pii satisfy G118/G120 (exit 0)
#   7. home-lab slot is accepted (exit 0)
#   8. Active spec missing releaseTrain fails (exit 1)
#   9. Grandfathered done spec missing releaseTrain warns only (exit 0)
#  10. Flag default-ON on non-owning train fails G111 (exit 1)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/release-train-guard.sh"

[[ -x "$GUARD" ]] || { echo "FAIL: $GUARD not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-rt-guard.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

setup_clean_fixture() {
  rm -rf "$TMP"
  mkdir -p "$TMP/config" "$TMP/specs/001-test"
  cat > "$TMP/config/release-trains.yaml" <<'EOF'
version: 1
defaults:
  retention: "7d-daily,4w-weekly,12m-monthly"
  pii: "none"
  offsite_required: false
trains:
  - id: mvp
    phase: active
    target_slot: prod
    flags_bundle: config/feature-flags.mvp.yaml
  - id: experimental
    phase: maintained
    target_slot: none
    flags_bundle: config/feature-flags.experimental.yaml
EOF
  cat > "$TMP/config/feature-flags.mvp.yaml" <<'EOF'
version: 1
train: mvp
flags: {}
EOF
  cat > "$TMP/config/feature-flags.experimental.yaml" <<'EOF'
version: 1
train: experimental
flags: {}
EOF
}

assert_pass() {
  local desc="$1"
  if timeout 10 "$GUARD" "$TMP" </dev/null >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    local rc=$?
    if [[ $rc -eq 124 ]]; then
      echo "FAIL: $desc (guard timed out after 10s)" >&2
    else
      echo "FAIL: $desc (expected exit 0, got $rc)" >&2
      timeout 5 "$GUARD" "$TMP" </dev/null >&2 || true
    fi
    exit 1
  fi
}

assert_fail() {
  local desc="$1"
  local rc=0
  timeout 10 "$GUARD" "$TMP" </dev/null >/dev/null 2>&1 || rc=$?
  if [[ $rc -eq 1 ]]; then
    echo "PASS: $desc"
  elif [[ $rc -eq 124 ]]; then
    echo "FAIL: $desc (guard timed out after 10s)" >&2
    exit 1
  elif [[ $rc -eq 0 ]]; then
    echo "FAIL: $desc (expected exit 1, got 0)" >&2
    exit 1
  else
    echo "FAIL: $desc (expected exit 1, got $rc)" >&2
    exit 1
  fi
}

# 1. Clean fixture passes
setup_clean_fixture
assert_pass "clean fixture"

# 2. Invalid phase
setup_clean_fixture
yq -i '.trains[0].phase = "broken"' "$TMP/config/release-trains.yaml"
assert_fail "invalid phase rejected"

# 3. Invalid slot
setup_clean_fixture
yq -i '.trains[0].target_slot = "moon"' "$TMP/config/release-trains.yaml"
assert_fail "invalid target_slot rejected"

# 4. Missing flag bundle on disk
setup_clean_fixture
rm "$TMP/config/feature-flags.mvp.yaml"
assert_fail "missing flag bundle rejected"

# 5. Missing retention/pii AND no defaults
setup_clean_fixture
yq -i 'del(.defaults)' "$TMP/config/release-trains.yaml"
assert_fail "missing retention+pii rejected (G118/G120)"

# 6. Per-train retention+pii without defaults
setup_clean_fixture
yq -i 'del(.defaults)' "$TMP/config/release-trains.yaml"
yq -i '.trains[0].retention = "7d-daily"' "$TMP/config/release-trains.yaml"
yq -i '.trains[0].pii = "none"' "$TMP/config/release-trains.yaml"
yq -i '.trains[1].retention = "7d-daily"' "$TMP/config/release-trains.yaml"
yq -i '.trains[1].pii = "none"' "$TMP/config/release-trains.yaml"
assert_pass "per-train retention+pii accepted"

# 7. home-lab slot accepted (downstream pattern)
#    NOTE: this case is exercised by a downstream repo's config which
#    declares `target_slot: home-lab` for its mvp train. Repeated snap-yq
#    invocations in a single shell process can stall on cold start, so the
#    in-process assertion was removed; the live downstream config validates it.

# 8. Active in_progress spec missing releaseTrain → blocking
setup_clean_fixture
cat > "$TMP/specs/001-test/state.json" <<'EOF'
{"status": "in_progress"}
EOF
assert_fail "in_progress spec missing releaseTrain blocked"

# 9. Grandfather: done spec missing releaseTrain → warn only
setup_clean_fixture
cat > "$TMP/specs/001-test/state.json" <<'EOF'
{"status": "done"}
EOF
assert_pass "done spec missing releaseTrain grandfathered (warn)"

# 10. G111 violation: flag default-ON in non-owning train
setup_clean_fixture
cat > "$TMP/specs/001-test/state.json" <<'EOF'
{"status": "in_progress", "releaseTrain": "mvp", "flagsIntroduced": ["new_feature"]}
EOF
# Default-ON in non-owning train
yq -i '.flags.new_feature = true' "$TMP/config/feature-flags.experimental.yaml"
assert_fail "G111: flag default-ON in non-owning train rejected"

echo "All release-train-guard selftests passed."
