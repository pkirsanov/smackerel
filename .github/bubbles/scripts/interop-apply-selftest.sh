#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

expect_path() {
  local target_path="$1"
  local label="$2"

  if [[ -e "$target_path" ]]; then
    pass "$label"
  else
    fail "$label"
  fi
}

expect_pattern() {
  local haystack="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" <<< "$haystack"; then
    pass "$label"
  else
    fail "$label"
  fi
}

expect_file_equals() {
  local target_path="$1"
  local expected_text="$2"
  local label="$3"

  if [[ -f "$target_path" ]] && grep -Fqx "$expected_text" "$target_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

DOWNSTREAM_FIXTURE="$TMP_ROOT/downstream-fixture"
mkdir -p "$DOWNSTREAM_FIXTURE"
git -C "$DOWNSTREAM_FIXTURE" init -q

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/CLAUDE.md"
# Claude Instructions

- Prefer explicit plans.
- Keep command changes reviewable.
EOF

mkdir -p "$DOWNSTREAM_FIXTURE/.claude/commands" "$DOWNSTREAM_FIXTURE/.claude/agents" "$DOWNSTREAM_FIXTURE/.roo/modes" "$DOWNSTREAM_FIXTURE/.cursor/rules"

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.claude/commands/review.md"
# Claude Command Surface

Document repo-local commands and keep them additive.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.claude/agents/tooling.md"
# Claude Tooling Notes

Use a small helper surface instead of mutating the framework layer.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.roo/modes/research.md"
# Roo Mode

Research mode requests a custom framework workflow.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.cursor/rules/safety.md"
# Cursor Rule

Prefer additive repo-local rules over global defaults.
EOF

(
  cd "$DOWNSTREAM_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap >/dev/null
)

mkdir -p "$DOWNSTREAM_FIXTURE/.github/instructions"
cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.github/instructions/imported-cursor.instructions.md"
collision-existing-file
EOF

echo "Running interop-apply selftest..."
echo "Scenario: supported interop apply generates only declared project-owned outputs, records manifest apply status, and falls back to proposals on collisions."

IMPORT_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && BUBBLES_INTEROP_FIXTURE_DIR="$DOWNSTREAM_FIXTURE" BUBBLES_INTEROP_TIMESTAMP='20260405T120000Z' bash .github/bubbles/scripts/cli.sh interop import --review-only 2>&1)"
APPLY_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && BUBBLES_INTEROP_FIXTURE_DIR="$DOWNSTREAM_FIXTURE" BUBBLES_INTEROP_TIMESTAMP='20260405T120000Z' bash .github/bubbles/scripts/cli.sh interop apply --safe 2>&1)"
STATUS_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && bash .github/bubbles/scripts/cli.sh interop status 2>&1)"
GUARD_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && bash .github/bubbles/scripts/cli.sh framework-write-guard 2>&1)"
set +e
INVALID_APPLY_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && BUBBLES_INTEROP_FIXTURE_DIR="$DOWNSTREAM_FIXTURE" BUBBLES_INTEROP_TIMESTAMP='../../../../../escape' bash .github/bubbles/scripts/cli.sh interop apply --safe 2>&1)"
INVALID_APPLY_STATUS=$?
set -e

IMPORT_ROOT="$DOWNSTREAM_FIXTURE/.github/bubbles-project/imports"
PROPOSAL_ROOT="$DOWNSTREAM_FIXTURE/.github/bubbles-project/proposals"

expect_pattern "$IMPORT_OUTPUT" 'Source: claude-code \(Claude Code\)' "Interop import detects Claude Code assets before apply"
expect_pattern "$APPLY_OUTPUT" 'Source: claude-code \(Claude Code\)' "Interop apply processes Claude Code packets"
expect_path "$DOWNSTREAM_FIXTURE/.github/instructions/imported-claude-code.instructions.md" "Interop apply writes project-owned imported instructions"
expect_path "$DOWNSTREAM_FIXTURE/scripts/imported-claude-code-tooling.md" "Interop apply writes project-owned helper tooling paths"
expect_pattern "$(cat "$DOWNSTREAM_FIXTURE/.specify/memory/agents.md")" 'GENERATED:INTEROP_CLAUDE_CODE_START' "Interop apply appends additive recommendations into .specify/memory/agents.md"
expect_pattern "$APPLY_OUTPUT" 'Collision fallback:' "Interop apply reports collision fallback targets"
expect_pattern "$STATUS_OUTPUT" 'cursor: reviewStatus=review-required applyStatus=applied-with-collisions' "Interop status reports apply collisions explicitly"
expect_pattern "$STATUS_OUTPUT" 'claude-code: reviewStatus=review-required applyStatus=applied' "Interop status reports successful apply status explicitly"
expect_pattern "$STATUS_OUTPUT" 'applied=.github/instructions/imported-claude-code.instructions.md, .specify/memory/agents.md, scripts/imported-claude-code-tooling.md' "Interop status reports the applied project-owned outputs"
expect_file_equals "$DOWNSTREAM_FIXTURE/.github/instructions/imported-cursor.instructions.md" 'collision-existing-file' "Interop apply leaves colliding project-owned files untouched"
expect_pattern "$STATUS_OUTPUT" 'proposals=.*cursor-apply-collision-review' "Interop status records apply-collision proposal routing"
expect_path "$IMPORT_ROOT/interop-manifest.json" "Interop manifest remains project-owned after apply"
expect_pattern "$GUARD_OUTPUT" 'Managed-file integrity:' "Framework write guard still passes after supported apply"

if [[ "$INVALID_APPLY_STATUS" -ne 0 ]]; then
  pass "Interop apply rejects unsafe timestamps before path construction"
else
  fail "Interop apply should reject unsafe timestamps before path construction"
fi
expect_pattern "$INVALID_APPLY_OUTPUT" 'BUBBLES_INTEROP_TIMESTAMP must match YYYYMMDDTHHMMSSZ' "Interop apply explains the timestamp validation rule"

if find "$DOWNSTREAM_FIXTURE/.github/bubbles" -path '*/imports/*' -o -path '*/proposals/*' | grep -q '.'; then
  fail "Interop apply never writes under framework-managed .github/bubbles surfaces"
else
  pass "Interop apply never writes under framework-managed .github/bubbles surfaces"
fi

if [[ -d "$PROPOSAL_ROOT" ]] && find "$PROPOSAL_ROOT" -maxdepth 1 -type f -name '*apply-collision-review.md' | grep -q '.'; then
  pass "Interop apply routes collisions into project-owned proposals"
else
  fail "Interop apply routes collisions into project-owned proposals"
fi

if [[ -e "$TMP_ROOT/escape" ]]; then
  fail "Interop apply should not create traversal-controlled paths outside the project-owned apply boundary"
else
  pass "Interop apply keeps unsafe timestamps from creating traversal-controlled paths"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "interop-apply selftest failed with $failures issue(s)."
  exit 1
fi

echo "interop-apply selftest passed."