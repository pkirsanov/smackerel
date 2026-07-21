#!/usr/bin/env bash
# Hermetic selftest for scope-context-fit-lint.sh
# (IMP-100 Phase 4 / IMP-026 SCOPE-6 — contextFit). macOS+WSL portable.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/scope-context-fit-lint.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

# --- fixtures -----------------------------------------------------------------
SELF_CONTAINED='# Scopes

## Scope 1: Add endpoint
Implement GET /api/widgets per spec.md FR-3. Wire the handler in the router.
The operator selects a widget; the user asked for filtering by date.
See design.md section Widgets and scenario S-01.

## Scope 2: Frontend list
Render the widget list page referencing the design.md wireframe.'

CHAT_DEP='# Scopes

## Scope 1: Add endpoint
Implement the endpoint as discussed above in our conversation.
Wire it per what we agreed earlier.

## Scope 2: Frontend
Render the list page per spec.md.'

ORDINARY_REQ='# Scopes

## Scope 1: Requirements
The user asked for a date filter. The operator selects a range.
Implement per spec.md FR-3. The user mentioned wanting CSV export (tracked in spec.md).'

mk_single() {
  local dir="$1" content="$2"
  mkdir -p "$dir"
  printf '%s\n' "$content" > "$dir/scopes.md"
}
mk_block_config() {
  mkdir -p "$1/.github"
  printf '%s\n' 'scopeContextFitGuard: block' > "$1/.github/bubbles-project.yaml"
}

run() {
  local label="$1" exp="$2" dir="$3"
  local rc=0
  bash "$GUARD" "$dir" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running scope-context-fit-lint selftest..."

# T1: no scopes → nothing to check.
d="$TMP_ROOT/t1"
mkdir -p "$d"
run "T1 no scopes → exit 0" 0 "$d"

# T2: self-contained scopes → OK.
d="$TMP_ROOT/t2"
mk_single "$d" "$SELF_CONTAINED"
run "T2 self-contained → OK (exit 0)" 0 "$d"

# T3: chat-dependent scope, advisory (default) → warn but exit 0.
d="$TMP_ROOT/t3"
mk_single "$d" "$CHAT_DEP"
run "T3 chat-dep, advisory → exit 0" 0 "$d"

# T4: chat-dependent scope, block posture → exit 1.
d="$TMP_ROOT/t4"
mk_single "$d" "$CHAT_DEP"
mk_block_config "$d"
run "T4 chat-dep, block → exit 1" 1 "$d"

# T5: ordinary requirement language must NOT false-positive.
d="$TMP_ROOT/t5"
mk_single "$d" "$ORDINARY_REQ"
mk_block_config "$d"
run "T5 ordinary requirement language, block → no false-positive (exit 0)" 0 "$d"

# T6: block posture but self-contained → exit 0.
d="$TMP_ROOT/t6"
mk_single "$d" "$SELF_CONTAINED"
mk_block_config "$d"
run "T6 self-contained, block → exit 0" 0 "$d"

# T7: per-scope-dir layout with a chat-dep scope, block → exit 1.
d="$TMP_ROOT/t7"
mkdir -p "$d/scopes/01-endpoint" "$d/scopes/02-frontend"
printf '%s\n' 'Implement per our chat from the chat history.' > "$d/scopes/01-endpoint/scope.md"
printf '%s\n' 'Render the page per design.md.' > "$d/scopes/02-frontend/scope.md"
mk_block_config "$d"
run "T7 per-scope-dir chat-dep, block → exit 1" 1 "$d"

# T8: missing feature dir → usage error.
run "T8 missing feature dir → exit 2" 2 "$TMP_ROOT/does-not-exist"

# T9: 'the conversation above' variant flagged under block.
d="$TMP_ROOT/t9"
mk_single "$d" '# Scopes

## Scope 1: X
Follow the plan from the conversation above.'
mk_block_config "$d"
run "T9 conversation-above variant, block → exit 1" 1 "$d"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "scope-context-fit-lint-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "scope-context-fit-lint-selftest: all cases passed."
