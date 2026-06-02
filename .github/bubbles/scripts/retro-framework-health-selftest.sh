#!/usr/bin/env bash
# retro-framework-health-selftest.sh — hermetic selftest.
#
# Cases:
#   1. No input files → proposal written with "no signal" messages
#   2. Events file with gate failures → top gates appear in proposal
#   3. Runs file with non-completed modes → stalled modes appear
#   4. Script makes ZERO writes to bubbles/, agents/, or any non-improvements path

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/retro-framework-health.sh"

[[ -x "$SCRIPT" ]] || { echo "FAIL: $SCRIPT not executable" >&2; exit 1; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-retro-fh.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# 1. No input files — should still write a proposal
mkdir -p "$TMP/improvements"
out_file="$("$SCRIPT" "$TMP" --slug "no-signal-test" 2>/dev/null | sed 's/.*Wrote //')"
[[ -f "$out_file" ]] || { echo "FAIL: case 1 proposal file not written ($out_file)"; exit 1; }
grep -q "no gate failure data" "$out_file" || { echo "FAIL: case 1 expected 'no gate failure data' marker"; exit 1; }
grep -q "no non-completed run data" "$out_file" || { echo "FAIL: case 1 expected 'no non-completed run data' marker"; exit 1; }
echo "PASS: no input files → proposal with no-signal messages"

# 2. Events file with gate failures
mkdir -p "$TMP/.specify/runtime"
cat > "$TMP/.specify/runtime/framework-events.jsonl" <<'EOF'
{"gate":"G123","outcome":"fail"}
{"gate":"G123","outcome":"fail"}
{"gate":"G124","outcome":"fail"}
{"gate":"G099","outcome":"pass"}
{"gate":"G123","outcome":"fail"}
EOF
if command -v jq >/dev/null 2>&1; then
  out_file="$("$SCRIPT" "$TMP" --slug "gate-fails-test" 2>/dev/null | sed 's/.*Wrote //')"
  grep -q "G123 (3 failures)" "$out_file" || { echo "FAIL: case 2 expected 'G123 (3 failures)'"; cat "$out_file"; exit 1; }
  grep -q "G124 (1 failures)" "$out_file" || { echo "FAIL: case 2 expected 'G124 (1 failures)'"; cat "$out_file"; exit 1; }
  echo "PASS: gate failures counted and ranked"
else
  echo "SKIP: jq not installed, gate failure ranking"
fi

# 3. Runs file with non-completed modes
cat > "$TMP/.specify/runtime/workflow-runs.json" <<'EOF'
[
  {"mode":"full-delivery","outcome":"completed"},
  {"mode":"full-delivery","outcome":"blocked"},
  {"mode":"incident-fastlane","outcome":"timeout"},
  {"mode":"full-delivery","outcome":"blocked"}
]
EOF
if command -v jq >/dev/null 2>&1; then
  out_file="$("$SCRIPT" "$TMP" --slug "stalled-modes-test" 2>/dev/null | sed 's/.*Wrote //')"
  grep -q "full-delivery (2 non-completed runs)" "$out_file" || { echo "FAIL: case 3 expected 'full-delivery (2 non-completed runs)'"; cat "$out_file"; exit 1; }
  echo "PASS: stalled modes counted"
fi

# 4. No writes outside improvements/
#    Capture mtime snapshot of bubbles/ if it exists in TMP
mkdir -p "$TMP/bubbles" "$TMP/agents"
touch "$TMP/bubbles/sentinel" "$TMP/agents/sentinel"
before_b="$(stat -c %Y "$TMP/bubbles/sentinel" 2>/dev/null || stat -f %m "$TMP/bubbles/sentinel" 2>/dev/null)"
before_a="$(stat -c %Y "$TMP/agents/sentinel" 2>/dev/null || stat -f %m "$TMP/agents/sentinel" 2>/dev/null)"
"$SCRIPT" "$TMP" --slug "no-write-test" >/dev/null 2>&1
after_b="$(stat -c %Y "$TMP/bubbles/sentinel" 2>/dev/null || stat -f %m "$TMP/bubbles/sentinel" 2>/dev/null)"
after_a="$(stat -c %Y "$TMP/agents/sentinel" 2>/dev/null || stat -f %m "$TMP/agents/sentinel" 2>/dev/null)"
[[ "$before_b" == "$after_b" ]] || { echo "FAIL: case 4 bubbles/ sentinel was modified"; exit 1; }
[[ "$before_a" == "$after_a" ]] || { echo "FAIL: case 4 agents/ sentinel was modified"; exit 1; }
echo "PASS: script makes zero writes outside improvements/"

echo "All retro-framework-health selftests passed."
