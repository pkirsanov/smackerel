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
#   - (BUG-004) An installer-substituted MCP id (`bubbles-<slug>`) on an agent
#     `tools:` line is EXEMPT (exit 0) — it is legitimate framework output.
#   - (BUG-004) The same `bubbles-<slug>` token in PROSE (no `tools:` line) is
#     STILL flagged — the exemption is scoped to tools declarations only.
#   - (BUG-004) A bare project name that is the repo's OWN derived slug (from
#     .vscode/mcp.json) is flagged even when it is NOT in the hardcoded list,
#     proving the derived-slug union closes the drift hole.
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

# --- Case 3: installer bubbles-<slug> on a tools: line → exempt (exit 0) ---
# (BUG-004) install.sh rewrites the canonical `bubbles` MCP id to a per-repo
# `bubbles-<slug>` token on agent tools: lines. That is legitimate framework
# output and must NOT be flagged as PROJECT_NAME drift.
installer_fixture_rel="agents/bubbles_shared/_selftest-installer-token.md"
installer_fixture="$clone/$installer_fixture_rel"
cat > "$installer_fixture" <<'EOF'
# Selftest Installer-Token Fixture

tools: [read, search, edit, agent, bubbles-wanderaide, playwright]
EOF

echo "[selftest agnosticity-lint] Case 3: installer bubbles-<slug> on tools: line → exit 0"
log3="$TMPDIR/log3.txt"
set +e
bash "$CLONED_LINT" --quiet "$installer_fixture_rel" >"$log3" 2>&1
status3=$?
set -e
if [[ "$status3" -eq 0 ]]; then
  pass "installer MCP-id token on tools: line is exempt (exit 0)"
else
  fail "installer MCP-id token should be exempt (got $status3)"
  sed -n '1,80p' "$log3"
fi

# --- Case 4: bubbles-<slug> in PROSE (no tools: line) → still flagged ---
# (BUG-004) The exemption is scoped to tools declarations. The same token in
# prose is a real leak and must still be flagged.
prose_token_fixture_rel="agents/bubbles_shared/_selftest-prose-token.md"
prose_token_fixture="$clone/$prose_token_fixture_rel"
cat > "$prose_token_fixture" <<'EOF'
# Selftest Prose-Token Fixture

This prose names the bubbles-wanderaide server id outside any tools line,
which is a real project-name leak and must still be flagged.
EOF

echo "[selftest agnosticity-lint] Case 4: bubbles-<slug> in prose → exit non-zero"
log4="$TMPDIR/log4.txt"
set +e
bash "$CLONED_LINT" "$prose_token_fixture_rel" >"$log4" 2>&1
status4=$?
set -e
if [[ "$status4" -ne 0 ]]; then
  pass "bubbles-<slug> in prose is still flagged (exit $status4)"
else
  fail "bubbles-<slug> in prose should be flagged (got $status4)"
  sed -n '1,80p' "$log4"
fi
if grep -Fq 'PROJECT_NAME' "$log4"; then
  pass "prose token surfaces PROJECT_NAME rule"
else
  fail "expected PROJECT_NAME violation token in prose-token output"
  sed -n '1,80p' "$log4"
fi

# --- Case 5: derived own-slug leak (NOT hardcoded) → flagged ---
# (BUG-004 fix #2) A product whose name is absent from the hardcoded list is
# still caught when it is the repo's OWN slug, derived from .vscode/mcp.json.
# This proves the derived-slug union closes the drift hole.
mkdir -p "$clone/.vscode"
cat > "$clone/.vscode/mcp.json" <<'EOF'
{
  "servers": {
    "bubbles-exampleproduct": {
      "type": "stdio",
      "command": "python3",
      "args": ["${workspaceFolder}/bubbles/mcp/server.py"]
    }
  }
}
EOF
derived_leak_fixture_rel="agents/bubbles_shared/_selftest-derived-leak.md"
derived_leak_fixture="$clone/$derived_leak_fixture_rel"
cat > "$derived_leak_fixture" <<'EOF'
# Selftest Derived-Slug Leak Fixture

This prose leaks the bare project name exampleproduct, which is the repo's
own derived slug and must be flagged even though it is not hardcoded.
EOF

echo "[selftest agnosticity-lint] Case 5: derived own-slug leak (not hardcoded) → exit non-zero"
log5="$TMPDIR/log5.txt"
set +e
bash "$CLONED_LINT" "$derived_leak_fixture_rel" >"$log5" 2>&1
status5=$?
set -e
if [[ "$status5" -ne 0 ]]; then
  pass "derived own-slug leak is flagged (exit $status5)"
else
  fail "derived own-slug leak should be flagged (got $status5)"
  sed -n '1,80p' "$log5"
fi
if grep -Fq 'PROJECT_NAME' "$log5"; then
  pass "derived own-slug leak surfaces PROJECT_NAME rule"
else
  fail "expected PROJECT_NAME violation token in derived-leak output"
  sed -n '1,80p' "$log5"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest agnosticity-lint] PASS"
  exit 0
fi

echo "[selftest agnosticity-lint] FAIL: $failures assertion(s)"
exit 1
