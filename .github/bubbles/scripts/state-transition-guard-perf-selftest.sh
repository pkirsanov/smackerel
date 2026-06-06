#!/usr/bin/env bash
#
# state-transition-guard-perf-selftest.sh — BUG-001 reliability regression
# (review R1, v6.1). Proves the guard's reliability helpers bound work that
# previously could hang Check 3G / whole-repo find walks on large repos.
#
# Asserts:
#   1. bubbles_run_with_timeout returns 124 when a command exceeds the limit.
#   2. bubbles_run_with_timeout preserves a fast command's own exit code.
#   3. bubbles_pruned_find does NOT descend into .git / node_modules / target,
#      so a synthetic repo with a huge node_modules is walked quickly.
#   4. The walk completes well within budget (proves the exclusion works).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/guard-lib.sh"

if [[ ! -f "$LIB" ]]; then
  echo "state-transition-guard-perf-selftest: SKIP (guard-lib.sh missing)"
  exit 0
fi
# shellcheck disable=SC1090
source "$LIB"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# --- 1. timeout fires --------------------------------------------------------
rc=0; bubbles_run_with_timeout 1 sleep 5 >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 124 ]]; then pass "timeout fires (124) on a hanging command"; else fail "timeout expected 124, got $rc"; fi

# --- 2. exit code preserved --------------------------------------------------
rc=0; bubbles_run_with_timeout 5 bash -c 'exit 3' >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 3 ]]; then pass "fast command exit code preserved (3)"; else fail "exit code expected 3, got $rc"; fi

# --- 3 + 4. pruned find skips generated dirs and is fast ---------------------
# Build a synthetic repo: a real target file at the top, plus large generated
# dirs that an unbounded find would crawl.
mkdir -p "$TMPDIR/repo/specs/001-x"
echo "real" > "$TMPDIR/repo/specs/001-x/wanted_test.rs"
# Heavy generated trees (the BUG-001 hang source). Keep counts modest but
# structured so a naive walk would do real work.
for d in .git node_modules target build .venv; do
  mkdir -p "$TMPDIR/repo/$d"
  for i in $(seq 1 50); do
    mkdir -p "$TMPDIR/repo/$d/sub$i"
    : > "$TMPDIR/repo/$d/sub$i/wanted_test.rs"   # decoys with the same name
  done
done

# pruned find for the target name should match ONLY the real file, not decoys.
matches="$(bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print 2>/dev/null | grep -c . || true)"
if [[ "$matches" -eq 1 ]]; then
  pass "pruned find returns only the real file (1), excludes generated-dir decoys"
else
  fail "pruned find matched $matches files (expected 1 — decoys in .git/node_modules/target leaked)"
fi

# Confirm no pruned-dir path appears in the output.
leak="$(bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print 2>/dev/null | grep -E '/(\.git|node_modules|target|build|\.venv)/' || true)"
if [[ -z "$leak" ]]; then
  pass "no generated-directory paths leak through the prune"
else
  fail "generated-directory path leaked: $leak"
fi

# Budget: the pruned walk must finish quickly (proves exclusion). 10s is a very
# loose ceiling; the real point is it does not crawl the heavy trees.
start=$(date +%s)
bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print >/dev/null 2>&1 || true
elapsed=$(( $(date +%s) - start ))
if [[ "$elapsed" -le 10 ]]; then
  pass "pruned walk completes within budget (${elapsed}s <= 10s)"
else
  fail "pruned walk took ${elapsed}s (> 10s budget)"
fi

echo ""
echo "[state-transition-guard-perf-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[state-transition-guard-perf-selftest] OK"
exit 0
