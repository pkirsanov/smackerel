#!/usr/bin/env bash
# File: scan-lib-selftest.sh
#
# Hermetic selftest for scan-lib.sh. Proves the three scan helpers honor the
# contracts that prevent the recurring guard false-positive class (IMP-009):
#   * bubbles_scan_files EXCLUDES *selftest* fixtures + generated dirs;
#   * bubbles_strip_comments drops pure-comment lines (so a code-evidence grep
#     does not count a mechanism named only in a comment);
#   * bubbles_status_lines yields canonical '**Status:**' lines but EXCLUDES
#     '>'-prefixed blockquote summary lines (BUG-006), while keeping real ones.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/scan-lib.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# --- bubbles_scan_files: excludes selftest fixtures + generated dirs ----------
mkdir -p "$work/tree/sub" "$work/tree/node_modules" "$work/tree/target"
printf 'x\n' >"$work/tree/real-guard.sh"
printf 'x\n' >"$work/tree/real-guard-selftest.sh"
printf 'x\n' >"$work/tree/sub/another.sh"
printf 'x\n' >"$work/tree/sub/some-selftest.sh"
printf 'x\n' >"$work/tree/node_modules/dep.sh"
printf 'x\n' >"$work/tree/target/built.sh"

found="$(bubbles_scan_files "$work/tree" '*.sh' | sort)"
if grep -q '/real-guard.sh$' <<<"$found" && grep -q '/sub/another.sh$' <<<"$found"; then
  pass "bubbles_scan_files includes real source files"
else
  fail "bubbles_scan_files should include real source files"
  echo "$found"
fi
if ! grep -q 'selftest' <<<"$found"; then
  pass "bubbles_scan_files EXCLUDES *selftest* fixtures (anti-self-match)"
else
  fail "bubbles_scan_files must exclude *selftest* fixtures"
  echo "$found"
fi
if ! grep -qE 'node_modules|/target/' <<<"$found"; then
  pass "bubbles_scan_files EXCLUDES generated/vendor dirs"
else
  fail "bubbles_scan_files must exclude generated/vendor dirs"
  echo "$found"
fi

# --- bubbles_strip_comments: drops pure-comment lines -------------------------
stripped="$(printf '%s\n' \
  '# TODO: PKCE' \
  '  // bearer only here' \
  'real_code_line=1' \
  '<!-- html comment -->' \
  '   actual_code()' | bubbles_strip_comments)"
if grep -q 'real_code_line=1' <<<"$stripped" && grep -q 'actual_code()' <<<"$stripped"; then
  pass "bubbles_strip_comments keeps real code lines"
else
  fail "bubbles_strip_comments should keep real code lines"
  echo "$stripped"
fi
if ! grep -qE 'PKCE|bearer only|html comment' <<<"$stripped"; then
  pass "bubbles_strip_comments drops pure-comment lines (anti-comment-match)"
else
  fail "bubbles_strip_comments must drop pure-comment lines"
  echo "$stripped"
fi

# --- bubbles_status_lines: excludes blockquote, keeps real status -------------
cat >"$work/scopes.md" <<'EOF'
> **Status:** all scopes Not Started (planning refreshed 2026-06-17)

## Scope 1
**Status:** Done

## Scope 2
**Status:** In Progress
EOF
status_lines="$(bubbles_status_lines "$work/scopes.md")"
if grep -q '\*\*Status:\*\* Done' <<<"$status_lines" && grep -q '\*\*Status:\*\* In Progress' <<<"$status_lines"; then
  pass "bubbles_status_lines keeps real per-scope '**Status:**' lines"
else
  fail "bubbles_status_lines should keep real status lines"
  echo "$status_lines"
fi
if ! grep -q 'all scopes Not Started' <<<"$status_lines"; then
  pass "bubbles_status_lines EXCLUDES the '>'-blockquote summary line (BUG-006)"
else
  fail "bubbles_status_lines must exclude the blockquote summary line"
  echo "$status_lines"
fi
# exactly two real status lines
n="$(printf '%s\n' "$status_lines" | grep -c '\*\*Status:\*\*' || true)"
[[ "$n" -eq 2 ]] \
  && pass "bubbles_status_lines yields exactly the 2 real status lines (blockquote not counted)" \
  || fail "bubbles_status_lines should yield exactly 2 real status lines (got $n)"

if [[ "$failures" -eq 0 ]]; then
  echo "[scan-lib-selftest] OK"
else
  echo "[scan-lib-selftest] $failures failed"
  exit 1
fi
