#!/usr/bin/env bash
# upkeep-calendar-selftest.sh — hermetic selftest for upkeep-calendar.sh.
#
# Asserts:
#   1. Missing calendar → exits 0 with skip message
#   2. Calendar present + no ledger → all tasks marked DUE
#   3. Calendar present + recent ledger entry → task marked ok

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CAL="$SCRIPT_DIR/upkeep-calendar.sh"

[[ -x "$CAL" ]] || { echo "FAIL: $CAL not executable" >&2; exit 1; }
command -v yq >/dev/null 2>&1 || { echo "SKIP: yq not installed"; exit 0; }
command -v jq >/dev/null 2>&1 || { echo "SKIP: jq not installed"; exit 0; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-upkeep.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# 1. Missing calendar → exits 0 with skip message
output="$("$CAL" "$TMP" </dev/null 2>&1)"
echo "$output" | grep -q "no upkeep tasks configured" && echo "PASS: missing calendar handled" || { echo "FAIL: missing calendar handling" >&2; echo "$output" >&2; exit 1; }

# 2. Calendar present + no ledger → all DUE
rm -rf "$TMP" && mkdir -p "$TMP/config"
cat > "$TMP/config/upkeep-calendar.yaml" <<'EOF'
version: 1
tasks:
  - id: backup
    cadence: daily
  - id: bcdr-drill
    cadence: quarterly
EOF
UPKEEP_LEDGER="$TMP/no-ledger.jsonl" output="$("$CAL" "$TMP" </dev/null 2>&1)"
if echo "$output" | grep -q "backup.*DUE" && echo "$output" | grep -q "bcdr-drill.*DUE"; then
  echo "PASS: tasks marked DUE when no ledger"
else
  echo "FAIL: tasks not marked DUE when no ledger" >&2
  echo "$output" >&2
  exit 1
fi

# 3. Calendar present + recent successful ledger → task marked ok
NOW_ISO="$(date -u +%FT%TZ)"
printf '{"task":"backup","outcome":"success","finished_at":"%s"}\n' "$NOW_ISO" > "$TMP/ledger.jsonl"
UPKEEP_LEDGER="$TMP/ledger.jsonl" output="$("$CAL" "$TMP" </dev/null 2>&1)"
if echo "$output" | grep -q "backup.*ok"; then
  echo "PASS: recent backup marked ok"
else
  echo "FAIL: recent backup not marked ok" >&2
  echo "$output" >&2
  exit 1
fi

echo "All upkeep-calendar selftests passed."
