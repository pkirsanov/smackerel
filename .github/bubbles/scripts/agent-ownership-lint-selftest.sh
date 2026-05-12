#!/usr/bin/env bash
# agent-ownership-lint-selftest.sh
#
# Hermetic selftest for agent-ownership-lint.sh.
#
# Clones the bubbles framework surface (scripts + agents + manifests)
# into a temp directory so the lint can run against an isolated tree
# without touching the live repo. Then asserts:
#   - The clean clone passes (exit 0).
#   - Removing the `^version:` header from agent-ownership.yaml causes
#     the lint to report the missing header and exit non-zero.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/agent-ownership-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest agent-ownership-lint] FAIL: target script missing at $LINT" >&2
  exit 1
fi

REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
if [[ ! -d "$REPO_ROOT/agents" ]] || [[ ! -d "$REPO_ROOT/bubbles" ]]; then
  echo "[selftest agent-ownership-lint] FAIL: live repo layout missing agents/ or bubbles/" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

clone="$TMPDIR/clone"
mkdir -p "$clone"
cp -R "$REPO_ROOT/bubbles" "$clone/bubbles"
cp -R "$REPO_ROOT/agents" "$clone/agents"

CLONED_LINT="$clone/bubbles/scripts/agent-ownership-lint.sh"
if [[ ! -f "$CLONED_LINT" ]]; then
  echo "[selftest agent-ownership-lint] FAIL: lint script missing in clone at $CLONED_LINT" >&2
  exit 1
fi

# --- Case 1: clean clone → exit 0 ---
echo "[selftest agent-ownership-lint] Case 1: clean clone → exit 0"
log1="$TMPDIR/log1.txt"
set +e
bash "$CLONED_LINT" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "clean clone exits 0 (got $status1)"
else
  fail "clean clone should exit 0 (got $status1)"
  sed -n '1,120p' "$log1"
fi
if grep -Fq 'Agent ownership lint passed.' "$log1"; then
  pass "output reports 'Agent ownership lint passed.'"
else
  fail "expected 'Agent ownership lint passed.' in output"
  sed -n '1,120p' "$log1"
fi

# --- Case 2: strip `version:` from agent-ownership.yaml → exit non-zero ---
ownership_file="$clone/bubbles/agent-ownership.yaml"
if [[ ! -f "$ownership_file" ]]; then
  echo "[selftest agent-ownership-lint] FAIL: agent-ownership.yaml missing in clone" >&2
  exit 1
fi

# Remove the version header line. The lint requires `^version:` in this file.
violating_ownership="$TMPDIR/clone/bubbles/agent-ownership-noversion.yaml"
grep -v '^version:' "$ownership_file" > "$violating_ownership"
mv "$violating_ownership" "$ownership_file"

echo "[selftest agent-ownership-lint] Case 2: missing version header → exit non-zero"
log2="$TMPDIR/log2.txt"
set +e
bash "$CLONED_LINT" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -ne 0 ]]; then
  pass "violating clone exits non-zero (got $status2)"
else
  fail "violating clone should exit non-zero (got $status2)"
  sed -n '1,160p' "$log2"
fi
if grep -Fq 'agent ownership manifest missing version header' "$log2"; then
  pass "output surfaces 'missing version header' violation"
else
  fail "expected 'agent ownership manifest missing version header' in output"
  sed -n '1,160p' "$log2"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest agent-ownership-lint] PASS"
  exit 0
fi

echo "[selftest agent-ownership-lint] FAIL: $failures assertion(s)"
exit 1
