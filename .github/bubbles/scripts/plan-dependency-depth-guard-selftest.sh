#!/usr/bin/env bash
# Hermetic selftest for plan-dependency-depth-guard.sh
# (IMP-100 Phase 4 / IMP-022 SCOPE-3 + SCOPE-4). macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/plan-dependency-depth-guard.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "plan-dependency-depth-guard-selftest: SKIP (jq not installed)"
  exit 0
fi

CONSUMER_BODY='### Implementation Plan
- API endpoints: GET /api/v1/thing wired via .route()
- Components/files: frontend dashboard page'
FOUNDATION_BODY='### Implementation Plan
- DB schema/migrations: add table
- service layer: repository business logic'

# mk_scope <feature-dir> <NN> <consumer|foundation>
mk_scope() {
  local fd="$1" nn="$2" cls="$3"
  mkdir -p "$fd/scopes/$nn"
  if [[ "$cls" == "consumer" ]]; then
    printf '%s\n' "$CONSUMER_BODY" > "$fd/scopes/$nn/scope.md"
  else
    printf '%s\n' "$FOUNDATION_BODY" > "$fd/scopes/$nn/scope.md"
  fi
}
mk_block() {
  mkdir -p "$1/.github"
  printf '%s\n' 'planDependencyDepthGuard: block' > "$1/.github/bubbles-project.yaml"
}
run() {
  local label="$1" exp="$2" dir="$3"
  local rc=0
  bash "$GUARD" "$dir" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running plan-dependency-depth-guard selftest..."

# T1: no state.json → no-op.
d="$TMP_ROOT/t1"
mkdir -p "$d"
run "T1 no state.json → exit 0" 0 "$d"

# T2: scopeProgress but no dependsOn edges → no-op.
d="$TMP_ROOT/t2"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run "T2 no dependsOn edges → no-op (exit 0)" 0 "$d"

# T3: edges present but scopeDir bodies missing → no-op (conservative).
d="$TMP_ROOT/t3"
mkdir -p "$d"
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run "T3 scopeDir bodies missing → no-op (exit 0)" 0 "$d"

# T4: EARLY-numbered but DAG-DEEP consumer (position guard misses this), block → exit 1.
#     scope 1 = consumer dependsOn [2,3,4]; 2,3,4 foundation → consumer needs 3 foundations.
d="$TMP_ROOT/t4"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2,3,4]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
  {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run "T4 early-numbered DAG-deep consumer, block → exit 1" 1 "$d"

# T5: same as T4 but advisory → exit 0 (warn only).
d="$TMP_ROOT/t5"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2,3,4]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
  {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
]}' > "$d/state.json"
run "T5 DAG-deep consumer, advisory → exit 0" 0 "$d"

# T6: transitive chain 1→2→3→4 where 4=consumer deps[3], 3 deps[2], 2 deps[1], 1 foundation.
#     consumer 4 transitive foundations = {1,2,3} = 3 → horizontal, block → exit 1.
d="$TMP_ROOT/t6"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
  {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
]}' > "$d/state.json"
mk_block "$d"
run "T6 transitive-chain deep consumer, block → exit 1" 1 "$d"

# T7: early usable increment exists — consumer 1 dependsOn [2] (1 foundation) + deep consumer 5.
#     min foundation-deps = 1 < 3 → OK even in block. (canary/early-slice preserved)
d="$TMP_ROOT/t7"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
mk_scope "$d" 05-e consumer
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
  {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]},
  {"scope":5,"scopeDir":"scopes/05-e","dependsOn":[2,3,4]}
]}' > "$d/state.json"
mk_block "$d"
run "T7 early usable consumer exists (min 1 foundation), block → exit 0" 0 "$d"

# T8: no consumer scope at all → no-op (position guard owns no-consumer).
d="$TMP_ROOT/t8"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]}
]}' > "$d/state.json"
mk_block "$d"
run "T8 no consumer scope → no-op (exit 0)" 0 "$d"

# T9: consumer needs only 2 foundations (below threshold 3) → OK.
d="$TMP_ROOT/t9"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]}
]}' > "$d/state.json"
mk_block "$d"
run "T9 consumer needs 2 foundations (below threshold), block → exit 0" 0 "$d"

# T10: missing feature dir → usage error.
run "T10 missing feature dir → exit 2" 2 "$TMP_ROOT/nope"

# T11: malformed JSON → runtime error.
d="$TMP_ROOT/t11"
mkdir -p "$d"
printf '%s\n' '{ bad json' > "$d/state.json"
run "T11 malformed state.json → exit 2" 2 "$d"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "plan-dependency-depth-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "plan-dependency-depth-guard-selftest: all cases passed."
