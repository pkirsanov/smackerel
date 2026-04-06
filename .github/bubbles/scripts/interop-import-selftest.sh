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

mkdir -p "$DOWNSTREAM_FIXTURE/.roo/rules" "$DOWNSTREAM_FIXTURE/.roo/modes" "$DOWNSTREAM_FIXTURE/.cursor/rules" "$DOWNSTREAM_FIXTURE/.cline/modes"

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.roo/rules/review.md"
# Roo Rule

Always explain ownership boundaries before editing framework-managed files.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.roo/modes/research.md"
# Roo Mode

Research mode requests a custom framework workflow.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.cursor/rules/safety.md"
# Cursor Rule

Prefer additive repo-local rules over global defaults.
EOF

cat <<'EOF' > "$DOWNSTREAM_FIXTURE/.clinerules"
Use terse plans and preserve existing user-owned conventions.
EOF

(
  cd "$DOWNSTREAM_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap >/dev/null
)

echo "Running interop-import selftest..."
echo "Scenario: review-only interop intake snapshots supported external assets inside project-owned paths and proposals workflow-mode changes instead of mutating framework-managed files."

IMPORT_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && BUBBLES_INTEROP_FIXTURE_DIR="$DOWNSTREAM_FIXTURE" BUBBLES_INTEROP_TIMESTAMP='20260404T120000Z' bash .github/bubbles/scripts/cli.sh interop import --review-only 2>&1)"
STATUS_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && bash .github/bubbles/scripts/cli.sh interop status 2>&1)"
GUARD_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && bash .github/bubbles/scripts/cli.sh framework-write-guard 2>&1)"
set +e
INVALID_TIMESTAMP_OUTPUT="$(cd "$DOWNSTREAM_FIXTURE" && BUBBLES_INTEROP_FIXTURE_DIR="$DOWNSTREAM_FIXTURE" BUBBLES_INTEROP_TIMESTAMP='../../../../../escape' bash .github/bubbles/scripts/cli.sh interop import --review-only 2>&1)"
INVALID_TIMESTAMP_STATUS=$?
set -e

IMPORT_ROOT="$DOWNSTREAM_FIXTURE/.github/bubbles-project/imports"
PROPOSAL_ROOT="$DOWNSTREAM_FIXTURE/.github/bubbles-project/proposals"

expect_pattern "$IMPORT_OUTPUT" 'Source: claude-code \(Claude Code\)' "Interop import detects Claude Code assets"
expect_pattern "$IMPORT_OUTPUT" 'Source: roo-code \(Roo Code\)' "Interop import detects Roo Code assets"
expect_pattern "$IMPORT_OUTPUT" 'Generated targets:' "Interop import reports generated project-owned targets"
expect_path "$IMPORT_ROOT/interop-manifest.json" "Interop import writes the project-owned interop manifest"
expect_path "$IMPORT_ROOT/claude-code/20260404T120000Z/raw/CLAUDE.md" "Interop import snapshots Claude Code raw assets"
expect_path "$IMPORT_ROOT/roo-code/20260404T120000Z/normalized.json" "Interop import writes normalized output"
expect_path "$IMPORT_ROOT/cursor/20260404T120000Z/translation-report.md" "Interop import writes translation reports"
expect_path "$IMPORT_ROOT/claude-code/20260404T120000Z/proposed-overrides/.github/instructions/imported-claude-code.instructions.md" "Interop import stages review-only candidate outputs under proposed-overrides"
expect_pattern "$STATUS_OUTPUT" 'roo-code: reviewStatus=review-required' "Interop status reports review-required imports"
expect_pattern "$STATUS_OUTPUT" 'claude-code: reviewStatus=review-required' "Interop status includes supported source records"
expect_pattern "$IMPORT_OUTPUT" 'Proposal refs:' "Interop import reports workflow-mode proposal routing"
expect_pattern "$GUARD_OUTPUT" 'Managed-file integrity:' "Framework write guard still passes after interop import"

if [[ "$INVALID_TIMESTAMP_STATUS" -ne 0 ]]; then
  pass "Interop import rejects unsafe timestamps before path construction"
else
  fail "Interop import should reject unsafe timestamps before path construction"
fi
expect_pattern "$INVALID_TIMESTAMP_OUTPUT" 'BUBBLES_INTEROP_TIMESTAMP must match YYYYMMDDTHHMMSSZ' "Interop import explains the timestamp validation rule"

if find "$DOWNSTREAM_FIXTURE/.github/bubbles" -path '*/imports/*' -o -path '*/proposals/*' | grep -q '.'; then
  fail "Interop import never writes under framework-managed .github/bubbles surfaces"
else
  pass "Interop import never writes under framework-managed .github/bubbles surfaces"
fi

if [[ -e "$TMP_ROOT/escape" ]]; then
  fail "Interop import should not create traversal-controlled paths outside the project-owned intake root"
else
  pass "Interop import keeps unsafe timestamps from creating traversal-controlled paths"
fi

if [[ -d "$PROPOSAL_ROOT" ]] && find "$PROPOSAL_ROOT" -maxdepth 1 -type f -name '*roo-code-framework-workflow-review.md' | grep -q '.'; then
  pass "Interop import routes workflow-mode requests into project-owned proposals"
else
  fail "Interop import routes workflow-mode requests into project-owned proposals"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "interop-import selftest failed with $failures issue(s)."
  exit 1
fi

echo "interop-import selftest passed."