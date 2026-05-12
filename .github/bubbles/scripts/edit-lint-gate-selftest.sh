#!/usr/bin/env bash
set -euo pipefail

# edit-lint-gate-selftest.sh — verify edit-lint-gate.sh behavior.
#
# Cases:
#   1. No config present                               → exit 0
#   2. Config present, editLintGate.enabled=false     → exit 0
#   3. Config matches file, fake linter returns 0     → exit 0
#   4. Config matches file, fake linter returns 1     → exit non-zero
#
# Bonus sanity:
#   5. Config present but no linter matches the file's extension → exit 0
#   6. Config matches multiple linters, one fails                 → exit non-zero

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="$SCRIPT_DIR/edit-lint-gate.sh"

if [[ ! -x "$GATE" ]]; then
  echo "FAIL: edit-lint-gate.sh is not executable at $GATE" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "FAIL: jq is required for edit-lint-gate-selftest.sh but not found in PATH" >&2
  exit 1
fi

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

failures=0

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

echo "Running edit-lint-gate selftest..."
echo "Scenario: framework supplies the gate, downstream registers linters via .specify/memory/bubbles.config.json. Default behavior is no-op (opt-in only)."

# --- Build a fake linter (passes) ------------------------------------------

FAKE_PASS="$TMP_ROOT/fake-linter-pass.sh"
cat > "$FAKE_PASS" <<'EOF'
#!/usr/bin/env bash
echo "fake-linter-pass: invoked on $*"
exit 0
EOF
chmod +x "$FAKE_PASS"

# --- Build a fake linter (fails) -------------------------------------------

FAKE_FAIL="$TMP_ROOT/fake-linter-fail.sh"
cat > "$FAKE_FAIL" <<'EOF'
#!/usr/bin/env bash
echo "fake-linter-fail: invoked on $*" >&2
exit 1
EOF
chmod +x "$FAKE_FAIL"

# ---- Case 1: no config present → exit 0 -----------------------------------

case1_root="$TMP_ROOT/case1"
mkdir -p "$case1_root/.specify/memory"
case1_target="$case1_root/foo.rs"
echo "// case1" > "$case1_target"

set +e
case1_out="$(BUBBLES_REPO_ROOT="$case1_root" bash "$GATE" "$case1_target" 2>&1)"
case1_exit=$?
set -e

if [[ "$case1_exit" -eq 0 ]]; then
  pass "Case 1: no config present → gate exits 0"
else
  fail "Case 1: no config present should exit 0 (got $case1_exit)"
  echo "  output: $case1_out"
fi

# ---- Case 2: config present, enabled=false → exit 0 -----------------------

case2_root="$TMP_ROOT/case2"
mkdir -p "$case2_root/.specify/memory"
cat > "$case2_root/.specify/memory/bubbles.config.json" <<JSON
{
  "editLintGate": {
    "enabled": false,
    "linters": [
      {
        "name": "would-fail",
        "match": "*.rs",
        "command": ["$FAKE_FAIL"]
      }
    ]
  }
}
JSON
case2_target="$case2_root/foo.rs"
echo "// case2" > "$case2_target"

set +e
case2_out="$(BUBBLES_REPO_ROOT="$case2_root" bash "$GATE" "$case2_target" 2>&1)"
case2_exit=$?
set -e

if [[ "$case2_exit" -eq 0 ]]; then
  pass "Case 2: config present but disabled → gate exits 0 without invoking any linter"
else
  fail "Case 2: disabled gate should exit 0 (got $case2_exit)"
  echo "  output: $case2_out"
fi

if printf '%s' "$case2_out" | grep -q 'fake-linter-fail'; then
  fail "Case 2: disabled gate must NOT invoke any linter"
else
  pass "Case 2: disabled gate correctly did NOT invoke any linter"
fi

# ---- Case 3: config matches file, fake linter returns 0 → exit 0 ----------

case3_root="$TMP_ROOT/case3"
mkdir -p "$case3_root/.specify/memory"
cat > "$case3_root/.specify/memory/bubbles.config.json" <<JSON
{
  "editLintGate": {
    "enabled": true,
    "linters": [
      {
        "name": "happy-linter",
        "match": "*.rs",
        "command": ["$FAKE_PASS"]
      }
    ]
  }
}
JSON
case3_target="$case3_root/lib.rs"
echo "// case3" > "$case3_target"

set +e
case3_out="$(BUBBLES_REPO_ROOT="$case3_root" bash "$GATE" "$case3_target" 2>&1)"
case3_exit=$?
set -e

if [[ "$case3_exit" -eq 0 ]]; then
  pass "Case 3: enabled gate with passing linter → exit 0"
else
  fail "Case 3: passing linter should exit 0 (got $case3_exit)"
  echo "  output: $case3_out"
fi

if printf '%s' "$case3_out" | grep -q "fake-linter-pass: invoked on $case3_target"; then
  pass "Case 3: matching linter was invoked with the changed file path appended"
else
  fail "Case 3: linter should have been invoked with the changed file path"
  echo "  output: $case3_out"
fi

if printf '%s' "$case3_out" | grep -q "happy-linter' PASSED"; then
  pass "Case 3: gate reports linter PASS via stdout"
else
  fail "Case 3: gate should report linter PASS"
  echo "  output: $case3_out"
fi

# ---- Case 4: config matches file, fake linter returns 1 → exit non-zero ---

case4_root="$TMP_ROOT/case4"
mkdir -p "$case4_root/.specify/memory"
cat > "$case4_root/.specify/memory/bubbles.config.json" <<JSON
{
  "editLintGate": {
    "enabled": true,
    "linters": [
      {
        "name": "angry-linter",
        "match": "*.rs",
        "command": ["$FAKE_FAIL"]
      }
    ]
  }
}
JSON
case4_target="$case4_root/main.rs"
echo "// case4" > "$case4_target"

set +e
case4_out="$(BUBBLES_REPO_ROOT="$case4_root" bash "$GATE" "$case4_target" 2>&1)"
case4_exit=$?
set -e

if [[ "$case4_exit" -ne 0 ]]; then
  pass "Case 4: enabled gate with failing linter → exit non-zero (got $case4_exit)"
else
  fail "Case 4: failing linter should exit non-zero (got $case4_exit)"
  echo "  output: $case4_out"
fi

if printf '%s' "$case4_out" | grep -q "angry-linter' FAILED"; then
  pass "Case 4: gate reports linter FAILED via stderr"
else
  fail "Case 4: gate should report linter FAILED"
  echo "  output: $case4_out"
fi

# ---- Bonus Case 5: enabled, but no linter matches extension → exit 0 ------

case5_root="$TMP_ROOT/case5"
mkdir -p "$case5_root/.specify/memory"
cat > "$case5_root/.specify/memory/bubbles.config.json" <<JSON
{
  "editLintGate": {
    "enabled": true,
    "linters": [
      {
        "name": "would-fail",
        "match": "*.rs",
        "command": ["$FAKE_FAIL"]
      }
    ]
  }
}
JSON
case5_target="$case5_root/notes.md"
echo "# case5" > "$case5_target"

set +e
case5_out="$(BUBBLES_REPO_ROOT="$case5_root" bash "$GATE" "$case5_target" 2>&1)"
case5_exit=$?
set -e

if [[ "$case5_exit" -eq 0 ]]; then
  pass "Case 5: enabled gate with no matching linter → exit 0 silently"
else
  fail "Case 5: no matching linter should exit 0 (got $case5_exit)"
  echo "  output: $case5_out"
fi

# ---- Bonus Case 6: multiple linters, one fails → exit non-zero ------------

case6_root="$TMP_ROOT/case6"
mkdir -p "$case6_root/.specify/memory"
cat > "$case6_root/.specify/memory/bubbles.config.json" <<JSON
{
  "editLintGate": {
    "enabled": true,
    "linters": [
      {
        "name": "happy",
        "match": "*.rs",
        "command": ["$FAKE_PASS"]
      },
      {
        "name": "angry",
        "match": "*.rs",
        "command": ["$FAKE_FAIL"]
      }
    ]
  }
}
JSON
case6_target="$case6_root/lib.rs"
echo "// case6" > "$case6_target"

set +e
case6_out="$(BUBBLES_REPO_ROOT="$case6_root" bash "$GATE" "$case6_target" 2>&1)"
case6_exit=$?
set -e

if [[ "$case6_exit" -ne 0 ]]; then
  pass "Case 6: multiple linters, any failure → exit non-zero (got $case6_exit)"
else
  fail "Case 6: any failing linter should exit non-zero (got $case6_exit)"
  echo "  output: $case6_out"
fi

# ---- Sanity: --help exits 0 ----------------------------------------------

if "$GATE" --help >/dev/null 2>&1; then
  pass "--help exits 0"
else
  fail "--help should exit 0"
fi

if "$GATE" --help 2>/dev/null | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "edit-lint-gate selftest failed with $failures issue(s)."
  exit 1
fi
echo "edit-lint-gate selftest passed."
