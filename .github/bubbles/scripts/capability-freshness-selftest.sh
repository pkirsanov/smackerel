#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

rewrite_once() {
  local file_path="$1"
  local from_text="$2"
  local to_text="$3"
  local temp_file
  temp_file="$(mktemp)"
  awk -v from_text="$from_text" -v to_text="$to_text" '
    BEGIN { replaced = 0 }
    index($0, from_text) && replaced == 0 {
      prefix = substr($0, 1, index($0, from_text) - 1)
      suffix = substr($0, index($0, from_text) + length(from_text))
      $0 = prefix to_text suffix
      replaced = 1
    }
    { print }
    END { if (replaced == 0) exit 2 }
  ' "$file_path" > "$temp_file"
  mv "$temp_file" "$file_path"
}

expect_check_failure() {
  local label="$1"
  if BUBBLES_REPO_ROOT="$TMP_ROOT" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >/tmp/bubbles-capability-check.out 2>&1; then
    fail "$label"
  else
    pass "$label"
    cat /tmp/bubbles-capability-check.out
  fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT" /tmp/bubbles-capability-check.out' EXIT

mkdir -p "$TMP_ROOT/bubbles" "$TMP_ROOT/docs/issues" "$TMP_ROOT/docs/generated"
cp "$ROOT_DIR/bubbles/capability-ledger.yaml" "$TMP_ROOT/bubbles/capability-ledger.yaml"
cp "$ROOT_DIR/bubbles/interop-registry.yaml" "$TMP_ROOT/bubbles/interop-registry.yaml"
cp "$ROOT_DIR/README.md" "$TMP_ROOT/README.md"
cp "$ROOT_DIR/docs/issues/session-aware-runtime-coordination.md" "$TMP_ROOT/docs/issues/session-aware-runtime-coordination.md"
cp "$ROOT_DIR/docs/issues/G068-word-overlap-threshold.md" "$TMP_ROOT/docs/issues/G068-word-overlap-threshold.md"

echo "Running capability-freshness selftest..."
echo "Scenario: generated docs or issue status drift must fail loudly before release or publication."

BUBBLES_REPO_ROOT="$TMP_ROOT" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null
pass "Fresh fixture generated from the capability ledger"

rewrite_once "$TMP_ROOT/docs/generated/competitive-capabilities.md" 'State summary: 4 shipped, 1 partial, 2 proposed.' 'State summary: 2 shipped, 1 partial, 4 proposed.'
expect_check_failure "Generated capability guide drift is detected"

BUBBLES_REPO_ROOT="$TMP_ROOT" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null
rewrite_once "$TMP_ROOT/docs/issues/G068-word-overlap-threshold.md" '**Ledger Status:** proposed' '**Ledger Status:** shipped'
expect_check_failure "Issue status block drift is detected"

if [[ "$failures" -gt 0 ]]; then
  echo "capability-freshness selftest failed with $failures issue(s)."
  exit 1
fi

echo "capability-freshness selftest passed."