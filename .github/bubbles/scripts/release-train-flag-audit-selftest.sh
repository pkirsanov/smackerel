#!/usr/bin/env bash
# release-train-flag-audit-selftest.sh — hermetic selftest.
#
# Asserts:
#   1. Missing release-trains.yaml → exits 0 with skip
#   2. Flags on active train → not overdue
#   3. Flags on frozen train → overdue (grace)
#   4. Flags on retired train → VIOLATION counted

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUDIT="$SCRIPT_DIR/release-train-flag-audit.sh"

[[ -x "$AUDIT" ]] || { echo "FAIL: $AUDIT not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-flag-audit.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# 1. Missing config
output="$("$AUDIT" "$TMP" </dev/null 2>&1)"
echo "$output" | grep -q "skipping" && echo "PASS: missing config handled" || { echo "FAIL: missing config handling" >&2; exit 1; }

# Set up fixture
rm -rf "$TMP" && mkdir -p "$TMP/config" "$TMP/specs/001-active" "$TMP/specs/002-frozen" "$TMP/specs/003-retired"
cat > "$TMP/config/release-trains.yaml" <<'EOF'
version: 1
defaults:
  retention: "7d-daily"
  pii: "none"
trains:
  - id: mvp
    phase: active
    target_slot: prod
    flags_bundle: config/feature-flags.mvp.yaml
  - id: old
    phase: frozen
    target_slot: none
    flags_bundle: config/feature-flags.old.yaml
  - id: ancient
    phase: retired
    target_slot: none
    flags_bundle: config/feature-flags.ancient.yaml
EOF

cat > "$TMP/specs/001-active/state.json" <<'EOF'
{"status": "in_progress", "releaseTrain": "mvp", "flagsIntroduced": ["feature_a"]}
EOF
cat > "$TMP/specs/002-frozen/state.json" <<'EOF'
{"status": "done", "releaseTrain": "old", "flagsIntroduced": ["feature_b"]}
EOF
cat > "$TMP/specs/003-retired/state.json" <<'EOF'
{"status": "done", "releaseTrain": "ancient", "flagsIntroduced": ["feature_c"]}
EOF

output="$("$AUDIT" "$TMP" 2>&1)"

# Active train flag should NOT appear
if echo "$output" | grep -q "feature_a"; then
  echo "FAIL: active-train flag should not be overdue" >&2
  echo "$output" >&2
  exit 1
fi
echo "PASS: active-train flag not flagged"

# Frozen train flag should appear as grace
if echo "$output" | grep -q "feature_b.*grace"; then
  echo "PASS: frozen-train flag flagged as grace"
else
  echo "FAIL: frozen-train flag not flagged as grace" >&2
  echo "$output" >&2
  exit 1
fi

# Retired train flag should appear as VIOLATION
if echo "$output" | grep -q "feature_c.*VIOLATION"; then
  echo "PASS: retired-train flag flagged as VIOLATION"
else
  echo "FAIL: retired-train flag not flagged as VIOLATION" >&2
  echo "$output" >&2
  exit 1
fi

# Overall violation count
if echo "$output" | grep -q "VIOLATION: 1 flag"; then
  echo "PASS: violation count correct"
else
  echo "FAIL: violation count not reported" >&2
  echo "$output" >&2
  exit 1
fi

echo "All release-train-flag-audit selftests passed."
