#!/usr/bin/env bash
# Hermetic selftest for work-tracker-project.sh (IMP-100 Phase 4 / IMP-026 SCOPE-7).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/work-tracker-project.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "work-tracker-project-selftest: SKIP (jq not installed)"
  exit 0
fi

mkfeat() { mkdir -p "$1"; [[ -n "${2:-}" ]] && printf '%s\n' "$2" > "$1/state.json"; return 0; }

# assert_jq <label> <feature-dir> <jq-filter-returning-boolean>
assert_jq() {
  local label="$1" dir="$2" filter="$3"
  local out rc=0
  out="$(bash "$GUARD" --feature-dir "$dir" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | jq -e "$filter" >/dev/null 2>&1; then
    pass "$label"
  else
    fail "$label (rc=$rc, filter failed; out=$(printf '%s' "$out" | head -c 200))"
  fi
}
run_exit() {
  local label="$1" exp="$2"; shift 2
  local rc=0
  bash "$GUARD" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running work-tracker-project selftest..."

# T1: no state.json → empty projection, provider-neutral shape.
d="$TMP_ROOT/t1"; mkfeat "$d"
assert_jq "T1 no state.json → empty items, source=bubbles" "$d" '.source == "bubbles" and (.items | length) == 0'

# T2: feature with status + no scopes → one epic.
d="$TMP_ROOT/t2"; mkfeat "$d" '{ "status": "in_progress", "title": "My Feature" }'
assert_jq "T2 epic only (no scopes)" "$d" '(.items | length) == 1 and .items[0].type == "epic" and .items[0].status == "in_progress" and .items[0].title == "My Feature" and .items[0].parent == null'

# T3: feature + 2 scopes → epic + 2 tasks with correct parent linkage.
d="$TMP_ROOT/t3"; mkfeat "$d" '{ "status": "done", "scopes": [ { "id": "01-a", "status": "done" }, { "id": "02-b", "status": "in_progress" } ] }'
assert_jq "T3 epic + 2 tasks" "$d" '(.items | length) == 3 and ([.items[] | select(.type=="task")] | length) == 2'
assert_jq "T3 task parent linkage + id namespacing" "$d" '([.items[] | select(.type=="task")] | all(.parent == "'"$(basename "$d")"'")) and (.items[1].id | test("/01-a$"))'
assert_jq "T3 task status carried through" "$d" '([.items[] | select(.type=="task" and .status=="in_progress")] | length) == 1'

# T4: idempotency — two runs are byte-identical.
d="$TMP_ROOT/t4"; mkfeat "$d" '{ "status": "done", "scopes": [ { "id": "01-a", "status": "done" } ] }'
o1="$(bash "$GUARD" --feature-dir "$d" 2>/dev/null)"
o2="$(bash "$GUARD" --feature-dir "$d" 2>/dev/null)"
if [[ "$o1" == "$o2" ]]; then pass "T4 idempotent (two runs byte-identical)"; else fail "T4 not idempotent"; fi

# T5: read-only — state.json is not mutated by projection.
d="$TMP_ROOT/t5"; mkfeat "$d" '{ "status": "done", "scopes": [ { "id": "01-a", "status": "done" } ] }'
before="$(shasum -a 256 "$d/state.json" | awk '{print $1}')"
bash "$GUARD" --feature-dir "$d" >/dev/null 2>&1 || true
after="$(shasum -a 256 "$d/state.json" | awk '{print $1}')"
if [[ "$before" == "$after" ]]; then pass "T5 read-only (state.json unchanged)"; else fail "T5 mutated state.json"; fi

# T6: scope id falls back to name/scope when .id absent.
d="$TMP_ROOT/t6"; mkfeat "$d" '{ "status": "in_progress", "scopes": [ { "name": "alpha", "status": "in_progress" } ] }'
assert_jq "T6 scope id falls back to name" "$d" '[.items[] | select(.type=="task")][0].id | test("/alpha$")'

# T7-T10: usage.
run_exit "T7 missing --feature-dir → exit 2" 2
run_exit "T8 nonexistent dir → exit 2" 2 --feature-dir "$TMP_ROOT/nope"
run_exit "T9 unknown flag → exit 2" 2 --feature-dir "$TMP_ROOT/t2" --push
run_exit "T10 --help → exit 0" 0 --help

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "work-tracker-project-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "work-tracker-project-selftest: all cases passed."
