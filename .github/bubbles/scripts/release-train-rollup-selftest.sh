#!/usr/bin/env bash
# release-train-rollup-selftest.sh — hermetic selftest for release-train-rollup.sh.
#
# Cases:
#   1. No trains file → exit 0, informational message, no table
#   2. 3 declared trains → exit 0, table with 3 data rows
#   3. flagsIntroduced in spec state.json adds to OPEN_FLAGS count
#   4. Per-train retention/pii override defaults

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROLLUP="$SCRIPT_DIR/release-train-rollup.sh"

[[ -x "$ROLLUP" ]] || { echo "FAIL: $ROLLUP not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-rollup.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

setup_three_trains() {
  rm -rf "$TMP"
  mkdir -p "$TMP/config" "$TMP/specs/001-a" "$TMP/specs/002-b"
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
    retention: "30d-daily"
  - id: prod
    phase: active
    target_slot: prod
    flags_bundle: config/feature-flags.prod.yaml
    pii: "encrypted-only"
EOF
  cat > "$TMP/specs/001-a/state.json" <<'EOF'
{"status":"in_progress","releaseTrain":"mvp","flagsIntroduced":["alpha","beta"]}
EOF
  cat > "$TMP/specs/002-b/state.json" <<'EOF'
{"status":"in_progress","releaseTrain":"experimental","flagsIntroduced":["gamma"]}
EOF
}

# 1. No trains file
rm -rf "$TMP" && mkdir -p "$TMP"
out="$("$ROLLUP" "$TMP" 2>&1)"
echo "$out" | grep -q "No config/release-trains.yaml" || { echo "FAIL: case 1 missing informational message"; echo "$out"; exit 1; }
echo "PASS: no trains file → informational"

# 2. Three trains
setup_three_trains
out="$("$ROLLUP" "$TMP")"
echo "$out" | grep -q '^| experimental ' || { echo "FAIL: case 2 experimental row missing"; echo "$out"; exit 1; }
echo "$out" | grep -q '^| mvp ' || { echo "FAIL: case 2 mvp row missing"; echo "$out"; exit 1; }
echo "$out" | grep -q '^| prod ' || { echo "FAIL: case 2 prod row missing"; echo "$out"; exit 1; }
echo "PASS: 3 trains report 3 rows"

# 3. Open flags counted
echo "$out" | grep '^| mvp ' | grep -q ' 2 ' || { echo "FAIL: case 3 mvp should have 2 open flags"; echo "$out"; exit 1; }
echo "$out" | grep '^| experimental ' | grep -q ' 1 ' || { echo "FAIL: case 3 experimental should have 1 open flag"; echo "$out"; exit 1; }
echo "PASS: open_flags counted (mvp=2, experimental=1)"

# 4. Per-train override
echo "$out" | grep '^| mvp ' | grep -q '30d-daily' || { echo "FAIL: case 4 per-train retention override"; echo "$out"; exit 1; }
echo "$out" | grep '^| prod ' | grep -q 'encrypted-only' || { echo "FAIL: case 4 per-train pii override"; echo "$out"; exit 1; }
echo "$out" | grep '^| experimental ' | grep -q '7d-daily' || { echo "FAIL: case 4 default retention applied"; echo "$out"; exit 1; }
echo "PASS: per-train override and defaults applied"

echo "All release-train-rollup selftests passed."
