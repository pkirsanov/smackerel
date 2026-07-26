#!/usr/bin/env bash
# Bubbles gate-coverage map generator selftest (IMP-102 / SCOPE-9).
#
# Keeps docs/generated/gate-coverage-map.md honest:
#   1. The committed doc MUST match what the generator emits from the current
#      sources (gates registry, modes requiredGates, guard checks, scripts, CI)
#      — a stale map fails --check.
#   2. The doc carries the GENERATED marker (drift-by-deletion guard).
#   3. --print is deterministic (byte-identical across runs).
#   4. ADVERSARIAL: a deliberately staled doc MUST make --check exit non-zero,
#      proving the freshness check actually detects drift (not a tautology).
#
# The adversarial case only ever overwrites the tracked doc via `cp` and is
# trap-protected, so the working tree is restored even if the test aborts.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GENERATOR="$SCRIPT_DIR/generate-gate-coverage-map.sh"
DOC="$REPO_ROOT/docs/generated/gate-coverage-map.md"

tmp_dir="$(mktemp -d)"
backup="$tmp_dir/gate-coverage-map.backup"
stale="$tmp_dir/gate-coverage-map.stale"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

restore_doc() {
  if [[ -f "$backup" ]]; then
    cp "$backup" "$DOC"
  fi
}

cleanup() {
  restore_doc
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

if [[ ! -x "$GENERATOR" ]]; then
  echo "generate-gate-coverage-map-selftest: missing or non-executable generator at $GENERATOR" >&2
  exit 2
fi

# PyYAML is required for a meaningful run; if absent, the generator SKIPs and so
# does this selftest (minimal-host graceful degradation, matching the family).
if ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "generate-gate-coverage-map-selftest: SKIP (PyYAML not installed)"
  exit 0
fi

# 1. --check is green against the committed tree.
check_log="$tmp_dir/check.log"
if bash "$GENERATOR" --check >"$check_log" 2>&1; then
  pass "generator --check matches the committed gate-coverage map"
else
  fail "generator --check reports drift; rerun: bash bubbles/scripts/generate-gate-coverage-map.sh"
  sed -n '1,40p' "$check_log" | sed 's/^/    /' >&2
fi

# 2. The doc exists and carries the GENERATED marker.
if [[ -f "$DOC" ]] && grep -Fq "GENERATED — do not edit by hand." "$DOC"; then
  pass "gate-coverage map exists and carries the GENERATED marker"
else
  fail "gate-coverage map missing or lacks the GENERATED marker"
fi

# 3. --print is deterministic.
print_a="$tmp_dir/print-a.md"
print_b="$tmp_dir/print-b.md"
bash "$GENERATOR" --print >"$print_a" 2>/dev/null
bash "$GENERATOR" --print >"$print_b" 2>/dev/null
if diff -q "$print_a" "$print_b" >/dev/null 2>&1; then
  pass "generator --print is deterministic across runs"
else
  fail "generator --print differs across runs (non-deterministic)"
fi

# 3b. The printed content matches the committed doc (write == print == committed).
if diff -q "$print_a" "$DOC" >/dev/null 2>&1; then
  pass "committed doc equals fresh --print output"
else
  fail "committed doc differs from fresh --print output"
fi

# 4. ADVERSARIAL: a staled doc MUST make --check fail.
cp "$DOC" "$backup"
cp "$DOC" "$stale"
printf '\n<!-- selftest adversarial drift marker -->\n' >>"$stale"
cp "$stale" "$DOC"
if bash "$GENERATOR" --check >/dev/null 2>&1; then
  fail "--check must DETECT a stale doc (adversarial drift not caught)"
else
  pass "--check detects a stale doc (adversarial drift caught)"
fi
# Restore and confirm green again — proves the failure was the drift, not noise.
restore_doc
if bash "$GENERATOR" --check >/dev/null 2>&1; then
  pass "--check is green again after restoring the fresh doc"
else
  fail "--check should be green after restoring the fresh doc"
fi

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "generate-gate-coverage-map selftest failed with $failures issue(s)."
  exit 1
fi
echo "generate-gate-coverage-map selftest passed."
