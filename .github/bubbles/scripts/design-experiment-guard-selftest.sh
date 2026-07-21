#!/usr/bin/env bash
# Hermetic selftest for design-experiment-guard.sh (IMP-100 Phase 4 / IMP-026 SCOPE-8).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/design-experiment-guard.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

# run <label> <expected-exit> <guard-args...>
run() {
  local label="$1" exp="$2"; shift 2
  local rc=0
  bash "$GUARD" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running design-experiment-guard selftest..."

# T1: no marker → no-op PASS (even though a terminal status is present, it is ignored).
d="$TMP_ROOT/t1"; mkdir -p "$d"
printf '%s\n' '{ "status": "done" }' > "$d/state.json"
run "T1 no .design-experiment marker → no-op (exit 0)" 0 --worktree "$d"

# T2: marker + clean empty state → PASS.
d="$TMP_ROOT/t2"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '{}' > "$d/state.json"
run "T2 marked + clean exploration → pass (exit 0)" 0 --worktree "$d"

# T3: marker + terminal status done → REFUSE.
d="$TMP_ROOT/t3"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '{ "status": "done" }' > "$d/state.json"
run "T3 marked + terminal status done → refuse (exit 1)" 1 --worktree "$d"

# T4: marker + non-empty completedScopes → REFUSE.
d="$TMP_ROOT/t4"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '{ "status": "in_progress", "completedScopes": ["01-a"] }' > "$d/state.json"
run "T4 marked + non-empty completedScopes → refuse (exit 1)" 1 --worktree "$d"

# T5: marker + checked DoD in scope.md → REFUSE.
d="$TMP_ROOT/t5"; mkdir -p "$d/scopes/01-a"; touch "$d/.design-experiment"
printf '%s\n' '- [x] Implemented the thing' > "$d/scopes/01-a/scope.md"
run "T5 marked + checked DoD in scope.md → refuse (exit 1)" 1 --worktree "$d"

# T6: marker + checked DoD (uppercase X, indented) in scopes.md → REFUSE.
d="$TMP_ROOT/t6"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '  - [X] done item' > "$d/scopes.md"
run "T6 marked + checked DoD uppercase [X] → refuse (exit 1)" 1 --worktree "$d"

# T7: marker + in-progress + empty completedScopes + unchecked DoD → PASS (clean exploration).
d="$TMP_ROOT/t7"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '{ "status": "in_progress", "completedScopes": [] }' > "$d/state.json"
printf '%s\n' '- [ ] not yet' > "$d/scopes.md"
run "T7 marked + in-progress, empty scopes, unchecked DoD → pass (exit 0)" 0 --worktree "$d"

# T8: marker + terminal alias delivered_fast → REFUSE.
d="$TMP_ROOT/t8"; mkdir -p "$d"; touch "$d/.design-experiment"
printf '%s\n' '{ "status": "delivered_fast" }' > "$d/state.json"
run "T8 marked + delivered_fast → refuse (exit 1)" 1 --worktree "$d"

# T9: marker + nested specs/feat/state.json specs_hardened → REFUSE (find recurses).
d="$TMP_ROOT/t9"; mkdir -p "$d/specs/feat"; touch "$d/.design-experiment"
printf '%s\n' '{ "status": "specs_hardened" }' > "$d/specs/feat/state.json"
run "T9 marked + nested terminal specs_hardened → refuse (exit 1)" 1 --worktree "$d"

# T10-T13: usage / help.
run "T10 missing --worktree → exit 2" 2
run "T11 --worktree nonexistent → exit 2" 2 --worktree "$TMP_ROOT/nope"
run "T12 unknown flag → exit 2" 2 --worktree "$TMP_ROOT/t2" --ship-it
run "T13 --help → exit 0" 0 --help

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "design-experiment-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "design-experiment-guard-selftest: all cases passed."
