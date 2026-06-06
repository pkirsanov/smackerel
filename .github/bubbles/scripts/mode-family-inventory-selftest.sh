#!/usr/bin/env bash
#
# mode-family-inventory-selftest.sh — hermetic selftest for the v6.1 (R5)
# per-family mode inventory + structural validator.
#
# Asserts:
#   1. --check PASSES on the real repo (every mode maps to one canonical family).
#   2. --check FAILS on a fixture whose workflows.yaml has a mode missing from
#      aliases.yaml (unmapped mode).
#   3. --check FAILS on a fixture whose alias maps a mode to a non-canonical
#      primitive.
#   4. --family <p> lists only that family's modes.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/mode-family-inventory.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "mode-family-inventory-selftest: SKIP (python3 not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# --- 1. real repo passes -----------------------------------------------------
rc=0; bash "$TARGET" --check >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 0 ]]; then pass "real repo --check passes (every mode mapped)"; else fail "real repo --check failed (exit $rc)"; fi

# --- fixtures ----------------------------------------------------------------
mk_aliases() {
  cat > "$1" <<'YAML'
version: 1
v6Primitives:
- analyze
- plan
- implement
- test
- validate
v5Aliases:
  full-delivery:
    primitive: implement
    tags: { action: full-delivery }
    description: x
  audit-only:
    primitive: validate
    tags: { action: audit }
    description: x
YAML
}

mk_workflows() {
  # $1 = path, $2... = mode names
  {
    echo "version: 1"
    echo "modes:"
    shift
    for m in "$@"; do
      echo "  $m:"
      echo "    description: fixture"
    done
    echo "phases:"
    echo "  implement:"
    echo "    agent: x"
  } > "$WF_OUT"
}

# 2. unmapped mode -> FAIL
ALIASES_OK="$TMPDIR/aliases-ok.yaml"; mk_aliases "$ALIASES_OK"
WF_OUT="$TMPDIR/wf-unmapped.yaml"; mk_workflows "$WF_OUT" full-delivery audit-only orphan-mode
rc=0; BUBBLES_WORKFLOWS_FILE="$WF_OUT" BUBBLES_ALIASES_FILE="$ALIASES_OK" bash "$TARGET" --check >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "unmapped mode fails --check (exit 1)"; else fail "unmapped mode should fail (exit $rc)"; fi

# 3. non-canonical primitive -> FAIL
ALIASES_BAD="$TMPDIR/aliases-bad.yaml"
cat > "$ALIASES_BAD" <<'YAML'
version: 1
v6Primitives:
- analyze
- plan
- implement
v5Aliases:
  full-delivery:
    primitive: implement
    tags: { action: full-delivery }
    description: x
  audit-only:
    primitive: not-a-primitive
    tags: { action: audit }
    description: x
YAML
WF_OUT2="$TMPDIR/wf-ok.yaml"; WF_OUT="$WF_OUT2"; mk_workflows "$WF_OUT2" full-delivery audit-only
rc=0; BUBBLES_WORKFLOWS_FILE="$WF_OUT2" BUBBLES_ALIASES_FILE="$ALIASES_BAD" bash "$TARGET" --check >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "non-canonical primitive fails --check (exit 1)"; else fail "non-canonical primitive should fail (exit $rc)"; fi

# 4. --family lists only that family's modes (real repo: 'fix' family)
fixmodes="$(bash "$TARGET" --family fix 2>/dev/null)"
if printf '%s' "$fixmodes" | grep -q "bugfix-fastlane" && ! printf '%s' "$fixmodes" | grep -q "full-delivery"; then
  pass "--family fix lists only fix-family modes"
else
  fail "--family fix output unexpected: $fixmodes"
fi

echo ""
echo "[mode-family-inventory-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[mode-family-inventory-selftest] OK"
exit 0
