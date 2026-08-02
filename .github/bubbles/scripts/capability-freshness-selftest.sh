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

# Count capabilities in a ledger fixture whose state equals <want>. Exact
# four-space match so capability ids and narrative prose are never miscounted.
count_state() {
  awk -v want="$2" '$0 == "    state: " want { n++ } END { print n + 0 }' "$1"
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
  ' "$file_path" >"$temp_file"
  mv "$temp_file" "$file_path"
}

expect_check_failure() {
  local label="$1"
  if BUBBLES_REPO_ROOT="$TMP_ROOT" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >"$CHECK_OUT" 2>&1; then
    fail "$label"
  else
    pass "$label"
    cat "$CHECK_OUT"
  fi
}

TMP_ROOT="$(mktemp -d)"
# Per-run file: the previous fixed /tmp path meant a second concurrent run's EXIT
# trap deleted this one's output mid-read.
CHECK_OUT="$(mktemp "${TMPDIR:-/tmp}/bubbles-capability-check.XXXXXX")"
trap 'rm -rf "$TMP_ROOT" "$CHECK_OUT"' EXIT

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

# Exact counts computed from the fixture ledger source (never an arbitrary
# [0-9]+ regex). The regenerated summary must match these precisely.
fixture_shipped="$(count_state "$TMP_ROOT/bubbles/capability-ledger.yaml" shipped)"
fixture_partial="$(count_state "$TMP_ROOT/bubbles/capability-ledger.yaml" partial)"
fixture_proposed="$(count_state "$TMP_ROOT/bubbles/capability-ledger.yaml" proposed)"
expected_summary="State summary: ${fixture_shipped} shipped, ${fixture_partial} partial, ${fixture_proposed} proposed."

if grep -qxF "$expected_summary" "$TMP_ROOT/docs/generated/competitive-capabilities.md"; then
  pass "Generated summary matches the fixture-derived counts (${fixture_shipped}/${fixture_partial}/${fixture_proposed})"
else
  fail "Generated summary matches the fixture-derived counts (${fixture_shipped}/${fixture_partial}/${fixture_proposed})"
fi

# Drift fixture derived from the exact counts: bump shipped by one so the summary
# no longer matches the regenerated ledger truth (no arbitrary magic numbers).
drift_summary="State summary: $((fixture_shipped + 1)) shipped, ${fixture_partial} partial, ${fixture_proposed} proposed."
rewrite_once "$TMP_ROOT/docs/generated/competitive-capabilities.md" \
  "$expected_summary" "$drift_summary"
expect_check_failure "Generated capability guide drift is detected"

BUBBLES_REPO_ROOT="$TMP_ROOT" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null
# G068 issue MD now ships as 'Ledger Status: shipped' (resolved in v3.8.0).
# Flip the drift fixture direction: rewrite 'shipped' -> 'proposed' so it no
# longer matches the regenerated YAML state and the freshness check fires.
rewrite_once "$TMP_ROOT/docs/issues/G068-word-overlap-threshold.md" '**Ledger Status:** shipped' '**Ledger Status:** proposed'
expect_check_failure "Issue status block drift is detected"

if [[ "$failures" -gt 0 ]]; then
  echo "capability-freshness selftest failed with $failures issue(s)."
  exit 1
fi

echo "capability-freshness selftest passed."
