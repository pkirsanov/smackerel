#!/usr/bin/env bash
#
# tool-capture-shim-selftest.sh — hermetic selftest for the v6.1 (R2)
# auto-capture shim. Proves that sourcing the shim routes gate-relevant
# commands through tool-log.sh into .specify/runtime/tool-calls.jsonl.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHIM="$SCRIPT_DIR/tool-capture-shim.sh"

if ! command -v git >/dev/null 2>&1; then
  echo "tool-capture-shim-selftest: SKIP (git not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

# Hermetic repo so tool-log.sh resolves a repo root and we control the log path.
mkdir -p "$TMPDIR/bin"
cat > "$TMPDIR/bin/fakebuild" <<'EOF'
#!/usr/bin/env bash
echo "fakebuild ran with args: $*"
exit 0
EOF
cat > "$TMPDIR/bin/fakeother" <<'EOF'
#!/usr/bin/env bash
echo "fakeother ran"
exit 0
EOF
chmod +x "$TMPDIR/bin/fakebuild" "$TMPDIR/bin/fakeother"

LOG="$TMPDIR/tool-calls.jsonl"
export BUBBLES_TOOL_LOG_FILE="$LOG"
export PATH="$TMPDIR/bin:$PATH"

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# --- 1. explicit one-shot capture -------------------------------------------
# shellcheck disable=SC1090
source "$SHIM"
bubbles_capture -- fakebuild --explicit >/dev/null 2>&1
if [[ -f "$LOG" ]] && grep -q "fakebuild --explicit" "$LOG"; then
  pass "explicit bubbles_capture logged the command"
else
  fail "explicit capture did not append a tool-log entry"
fi

# --- 2. auto-capture via shadow wrapper -------------------------------------
rm -f "$LOG"
export BUBBLES_AUTOCAPTURE=1
export BUBBLES_CAPTURE_COMMANDS="fakebuild"
# shellcheck disable=SC1090
source "$SHIM"
fakebuild --auto >/dev/null 2>&1
if [[ -f "$LOG" ]] && grep -q "fakebuild --auto" "$LOG"; then
  pass "auto-capture shadow wrapper logged the command"
else
  fail "auto-capture wrapper did not append a tool-log entry"
fi

# --- 3. non-allowlisted command is NOT auto-captured ------------------------
rm -f "$LOG"
fakeother >/dev/null 2>&1
if [[ ! -s "$LOG" ]]; then
  pass "non-allowlisted command not auto-captured"
else
  fail "non-allowlisted command was captured (log non-empty)"
fi

# --- 4. wrapper preserves exit code -----------------------------------------
rm -f "$LOG"
cat > "$TMPDIR/bin/failcmd" <<'EOF'
#!/usr/bin/env bash
exit 7
EOF
chmod +x "$TMPDIR/bin/failcmd"
export BUBBLES_CAPTURE_COMMANDS="failcmd"
# shellcheck disable=SC1090
source "$SHIM"
failcmd >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 7 ]]; then
  pass "wrapper preserves wrapped command exit code (7)"
else
  fail "exit code not preserved (got $rc, expected 7)"
fi

# --- 5. uninstall restores plain resolution ---------------------------------
bubbles_capture_uninstall
if ! declare -F failcmd >/dev/null 2>&1; then
  pass "uninstall removed the shadow wrapper"
else
  fail "uninstall did not remove the wrapper function"
fi

echo ""
echo "[tool-capture-shim-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[tool-capture-shim-selftest] OK"
exit 0
