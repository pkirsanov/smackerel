#!/usr/bin/env bash
# agnosticity-lint-selftest.sh
#
# Hermetic selftest for agnosticity-lint.sh.
#
# Clones the bubbles framework surface into a temp directory and runs
# the lint against synthetic fixtures placed inside the clone. Asserts:
#   - A portable-surface file containing a real project name (e.g.
#     "wanderaide") triggers a PROJECT_NAME violation and exits non-zero.
#   - A portable-surface file with no banned tokens passes (exit 0).
#
# Note: agnosticity-lint resolves REPO_ROOT from its own location, so
# we must invoke the cloned copy to keep the test isolated from the
# live repo.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/agnosticity-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest agnosticity-lint] FAIL: target script missing at $LINT" >&2
  exit 1
fi

REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
if [[ ! -d "$REPO_ROOT/bubbles/scripts" ]]; then
  echo "[selftest agnosticity-lint] FAIL: live repo layout missing bubbles/scripts" >&2
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
mkdir -p "$clone/agents/bubbles_shared"

CLONED_LINT="$clone/bubbles/scripts/agnosticity-lint.sh"
if [[ ! -f "$CLONED_LINT" ]]; then
  echo "[selftest agnosticity-lint] FAIL: lint script missing in clone" >&2
  exit 1
fi

# Drop the .manifest from the clone so the framework-drift check (which
# scans the live scripts dir against the manifest) doesn't fire on our
# synthetic fixture file.
rm -f "$clone/bubbles/.manifest"

# Build a portable-surface fixture file path. agents/bubbles_shared/*.md
# is a portable surface scanned by agnosticity-lint.
clean_fixture_rel="agents/bubbles_shared/_selftest-clean.md"
violating_fixture_rel="agents/bubbles_shared/_selftest-violating.md"
clean_fixture="$clone/$clean_fixture_rel"
violating_fixture="$clone/$violating_fixture_rel"

cat > "$clean_fixture" <<'EOF'
# Selftest Clean Fixture

This file uses only generic, project-agnostic language. It demonstrates
acceptable phrasing for portable Bubbles surfaces and does not reference
any specific project name or absolute machine path.
EOF

cat > "$violating_fixture" <<'EOF'
# Selftest Violating Fixture

This file mentions wanderaide which is a real project name and must be
caught by the PROJECT_NAME rule.
EOF

# --- Case 1: clean fixture → exit 0 ---
echo "[selftest agnosticity-lint] Case 1: clean fixture → exit 0"
log1="$TMPDIR/log1.txt"
set +e
bash "$CLONED_LINT" --quiet "$clean_fixture_rel" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "clean fixture exits 0 (got $status1)"
else
  fail "clean fixture should exit 0 (got $status1)"
  sed -n '1,80p' "$log1"
fi

# --- Case 2: violating fixture → exit non-zero ---
echo "[selftest agnosticity-lint] Case 2: violating fixture → exit non-zero"
log2="$TMPDIR/log2.txt"
set +e
bash "$CLONED_LINT" "$violating_fixture_rel" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -ne 0 ]]; then
  pass "violating fixture exits non-zero (got $status2)"
else
  fail "violating fixture should exit non-zero (got $status2)"
  sed -n '1,80p' "$log2"
fi
if grep -Fq 'PROJECT_NAME' "$log2"; then
  pass "output surfaces PROJECT_NAME rule"
else
  fail "expected PROJECT_NAME violation token in output"
  sed -n '1,80p' "$log2"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest agnosticity-lint] PASS"
  exit 0
fi

echo "[selftest agnosticity-lint] FAIL: $failures assertion(s)"
exit 1
