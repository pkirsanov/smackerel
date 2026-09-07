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
run_block_diagnostic() {
  local label="$1" dir="$2" consumer_id="$3" foundation_count="$4"
  local output rc=0
  output="$(bash "$GUARD" "$dir" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 1 ]]; then
    fail "$label (expected deliberate block exit 1, got $rc)"
  elif [[ "$output" != *"DEPENDENCY-GRAPH HORIZONTAL PLAN"* ]]; then
    fail "$label (exit 1 lacked the horizontal-plan diagnostic)"
  elif [[ "$output" != *"least-blocked consumer (scope $consumer_id)"* ]]; then
    fail "$label (diagnostic lacked symbolic consumer $consumer_id)"
  elif [[ "$output" != *"transitively depends on $foundation_count foundation scope(s)"* ]]; then
    fail "$label (diagnostic lacked foundation count $foundation_count)"
  else
    pass "$label"
  fi
}
run_horizontal_without_authority_conflict() {
  local label="$1" dir="$2"
  local output rc=0
  output="$(bash "$GUARD" "$dir" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 1 ]]; then
    fail "$label (expected deliberate block exit 1, got $rc: $output)"
  elif [[ "$output" != *"DEPENDENCY-GRAPH HORIZONTAL PLAN"* ]]; then
    fail "$label (exit 1 lacked the horizontal-plan diagnostic: $output)"
  elif [[ "$output" == *"INCOMPLETE DAG SIGNAL"* || "$output" == *"deprecated top-level scopeProgress conflicts with certification.scopeProgress"* ]]; then
    fail "$label (authority-conflict/incomplete-DAG diagnostic vetoed canonical evaluation: $output)"
  else
    pass "$label"
  fi
}
run_collision_diagnostic() {
  local label="$1" dir="$2" expected="$3"
  local output rc=0
  output="$(bash "$GUARD" "$dir" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 1 ]]; then
    fail "$label (expected block-posture collision exit 1, got $rc)"
  elif [[ "$output" != *"INCOMPLETE DAG SIGNAL — ambiguous scope alias — $expected"* ]]; then
    fail "$label (exit 1 lacked deterministic collision diagnostic: $output)"
  else
    pass "$label"
  fi
}
run_duplicate_diagnostic() {
  local label="$1" dir="$2" expected="$3"
  local output rc=0
  output="$(bash "$GUARD" "$dir" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 1 ]]; then
    fail "$label (expected block-posture duplicate exit 1, got $rc)"
  elif [[ "$output" != *"INCOMPLETE DAG SIGNAL"* || "$output" != *"$expected"* ]]; then
    fail "$label (exit 1 lacked deterministic duplicate diagnostic: $output)"
  else
    pass "$label"
  fi
}
run_signal_diagnostic() {
  local label="$1" exp="$2" dir="$3" expected="$4"
  local output rc=0
  output="$(bash "$GUARD" "$dir" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne "$exp" ]]; then
    fail "$label (expected exit $exp, got $rc: $output)"
  elif [[ "$output" != *"INCOMPLETE DAG SIGNAL"* || "$output" != *"$expected"* ]]; then
    fail "$label (missing incomplete-signal diagnostic: $output)"
  else
    pass "$label"
  fi
}

run_equal_cost_permutation() {
  local label="$1" first="$2" second="$3" expected_consumer="$4"
  local first_output second_output first_rc=0 second_rc=0
  first_output="$(bash "$GUARD" "$first" 2>&1)" && first_rc=0 || first_rc=$?
  second_output="$(bash "$GUARD" "$second" 2>&1)" && second_rc=0 || second_rc=$?
  if [[ "$first_rc" -ne 0 || "$second_rc" -ne 0 ]]; then
    fail "$label (expected clean exits, got $first_rc and $second_rc)"
  elif [[ "$first_output" != *"consumer scope $expected_consumer"* || "$second_output" != *"consumer scope $expected_consumer"* ]]; then
    fail "$label (equal-cost winner was not canonical scope $expected_consumer)"
  elif [[ "${first_output##*consumer scope }" != "${second_output##*consumer scope }" ]]; then
    fail "$label (permuted declarations changed the deterministic verdict)"
  else
    pass "$label"
  fi
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

# T3: edges present but scopeDir bodies missing fail closed under block posture.
d="$TMP_ROOT/t3"
mkdir -p "$d"
printf '%s\n' '{"scopeProgress":[
  {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2]},
  {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T3 missing scope body refuses under block posture" 1 "$d" "does not claim a readable regular non-symlink scopeDir/scope.md"

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

# T12: counts-summary OBJECT scopeProgress → no-op, NOT a crash.
# Regression case. Single-file scopes.md plans carry scopeProgress as
# {total,done,inProgress,notStarted} rather than the per-scope array. Before the
# type check this fell through the length test (jq `length` on an object returns
# its KEY COUNT — 4 — not 0), then `.[] | .dependsOn` iterated the counts VALUES
# and died with "Cannot index number with string \"dependsOn\"" (jq exit 5).
# Under `block` that non-zero exit was reported as a substantive verdict: a
# one-scope plan was told every consumer-visible scope sat behind >=3 foundation
# scopes. A crash rendered as a confident false BLOCK is worse than a crash,
# which is why this asserts exit 0 under the BLOCK posture specifically.
d="$TMP_ROOT/t12"
mkdir -p "$d"
printf '%s\n' '{"scopeProgress":{"total":1,"done":1,"inProgress":0,"notStarted":0}}' > "$d/state.json"
mk_block "$d"
run "T12 counts-summary object scopeProgress, block → no-op (exit 0)" 0 "$d"

# T13: same shape nested under certification (the completed-spec read path).
# The guard resolves `.scopeProgress // .certification.scopeProgress`, so the
# fallback branch needs its own case; a fix applied to only one branch would
# still crash every certified spec.
d="$TMP_ROOT/t13"
mkdir -p "$d"
printf '%s\n' '{"certification":{"scopeProgress":{"total":2,"done":2,"inProgress":0,"notStarted":0}}}' > "$d/state.json"
mk_block "$d"
run "T13 counts-summary under certification, block → no-op (exit 0)" 0 "$d"

# T14: array whose entries are not objects is an incomplete signal in block posture.
d="$TMP_ROOT/t14"
mkdir -p "$d"
printf '%s\n' '{"scopeProgress":[1,2,3]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T14 non-object scopeProgress entries refuse in block posture" 1 "$d" "scopeProgress entries are not all objects"

# T15: ADVERSARIAL PARTNER — the type check must not blunt real detection.
# Identical block posture to T12-T14, but the genuine horizontal-plan shape in
# the per-scope ARRAY form. If a future edit made the guard no-op too eagerly
# (say by widening the type test), T12 would still pass while the guard stopped
# working entirely. This case is what makes the no-op cases meaningful.
d="$TMP_ROOT/t15"
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
run "T15 real horizontal chain still BLOCKS under block posture (exit 1)" 1 "$d"

# T16 / SCN-B052-001: a deprecated empty top-level array must not shadow the
# canonical certification graph. The canonical chain is horizontal and must
# therefore block under block posture.
d="$TMP_ROOT/t16"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{
  "scopeProgress": [],
  "certification": {"scopeProgress":[
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
  ]}
}' > "$d/state.json"
mk_block "$d"
run_horizontal_without_authority_conflict "T16 SCN-B052-001 legacy empty array cannot shadow canonical deep graph" "$d"

# T16A: canonical symbolic scopeId chain. This preserves the adversarial
# regression for symbolic IDs through map construction and fixed-point closure.
d="$TMP_ROOT/t16a"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T16A symbolic horizontal chain deliberately BLOCKS" "$d" "SCOPE-consumer" 3

# T17 / SCN-B052-002: canonical shallow data wins over a deprecated deep graph.
d="$TMP_ROOT/t17"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
printf '%s\n' '{
  "scopeProgress": [
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2,3,4]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
  ],
  "certification": {"scopeProgress":[
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
  ]}
}' > "$d/state.json"
mk_block "$d"
run "T17 SCN-B052-002 canonical shallow graph wins over legacy deep graph (exit 0)" 0 "$d"

# T17A: canonical symbolic IDs preserve the early-increment exception. The
# first consumer needs one foundation while a second consumer is deep in the DAG.
d="$TMP_ROOT/t17a"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
mk_scope "$d" 05-e consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-early","scopeDir":"scopes/01-a","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-late","scopeDir":"scopes/05-e","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run "T17A symbolic early usable increment, block → exit 0" 0 "$d"

# T18 / SCN-B052-003: a legacy-only deep graph remains supported.
d="$TMP_ROOT/t18"
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
run "T18 SCN-B052-003 legacy-only deep graph remains blocking (exit 1)" 1 "$d"

# T19 / SCN-B052-004: execution metadata cannot replace canonical authority.
d="$TMP_ROOT/t19"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
printf '%s\n' '{
  "certification": {"scopeProgress":[
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
  ]},
  "execution": {"scopeProgress":[
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[2,3,4]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[]}
  ]}
}' > "$d/state.json"
mk_block "$d"
run "T19 SCN-B052-004 canonical graph wins over execution deep graph (exit 0)" 0 "$d"

# T18A: when both identities exist, canonical scopeId wins over legacy scope.
d="$TMP_ROOT/t18a"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":41,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":42,"scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scope":43,"scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scope":44,"scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T18A scopeId precedes conflicting legacy scope and deliberately BLOCKS" "$d" "SCOPE-consumer" 3

# T19A: an entry with neither canonical nor usable legacy identity cannot be
# represented in the local DAG map and must fail closed in block posture.
d="$TMP_ROOT/t19a"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"","scope":0,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-b","dependsOn":["SCOPE-missing"]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T19A unusable identity refuses in block posture" 1 "$d" "not every scope has a usable scopeId/scope identity"

# T20: a present canonical empty array is authoritative; only absent or null
# canonical data may select the deprecated top-level fallback.
d="$TMP_ROOT/t20"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{
  "scopeProgress": [
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
  ],
  "certification": {"scopeProgress":[]}
}' > "$d/state.json"
mk_block "$d"
run "T20 canonical empty array remains authoritative over legacy deep graph (exit 0)" 0 "$d"

# T20B / SCN-B052-005: canonical null selects the deprecated compatibility
# graph, which remains subject to the normal horizontal-plan verdict.
d="$TMP_ROOT/t20b"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{
  "scopeProgress": [
    {"scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
    {"scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
    {"scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
    {"scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
  ],
  "certification": {"scopeProgress":null}
}' > "$d/state.json"
mk_block "$d"
run_horizontal_without_authority_conflict "T20B SCN-B052-005 canonical null falls back to legacy deep graph" "$d"

# T20A: canonical IDs connected exclusively through positive numeric legacy
# aliases remain a real horizontal chain.
d="$TMP_ROOT/t20a"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":41,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":42,"scopeDir":"scopes/02-b","dependsOn":[41]},
  {"scopeId":"SCOPE-foundation-c","scope":43,"scopeDir":"scopes/03-c","dependsOn":[42]},
  {"scopeId":"SCOPE-consumer","scope":44,"scopeDir":"scopes/04-d","dependsOn":[43]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T20A numeric legacy aliases form a real horizontal chain" "$d" "SCOPE-consumer" 3

# T21: canonical IDs connected exclusively through nonblank string legacy
# aliases. Whitespace around a legacy alias is identity padding, not part of
# the alias itself, matching the closed resolver semantics.
d="$TMP_ROOT/t21"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":" legacy-a ","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":"legacy-b","scopeDir":"scopes/02-b","dependsOn":["legacy-a"]},
  {"scopeId":"SCOPE-foundation-c","scope":"legacy-c","scopeDir":"scopes/03-c","dependsOn":["legacy-b"]},
  {"scopeId":"SCOPE-consumer","scope":"legacy-consumer","scopeDir":"scopes/04-d","dependsOn":["legacy-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T21 string legacy aliases form a real horizontal chain" "$d" "SCOPE-consumer" 3

# T22: whitespace-only scopeId falls back to a valid positive numeric scope.
d="$TMP_ROOT/t22"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"\t","scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scopeId":"\n","scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
  {"scopeId":"  ","scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T22 whitespace scopeId falls back to numeric legacy identity" "$d" "4" 3

# T23: whitespace-only scopeId also falls back to a nonblank string scope.
d="$TMP_ROOT/t23"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":"legacy-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"   ","scope":"legacy-b","scopeDir":"scopes/02-b","dependsOn":["legacy-a"]},
  {"scopeId":"   ","scope":"legacy-c","scopeDir":"scopes/03-c","dependsOn":["legacy-b"]},
  {"scopeId":"   ","scope":"legacy-consumer","scopeDir":"scopes/04-d","dependsOn":["legacy-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T23 whitespace scopeId falls back to string legacy identity" "$d" "legacy-consumer" 3

# T24: fractional numeric scope is not a valid legacy identity and refuses.
d="$TMP_ROOT/t24"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":1.5,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scope":2,"scopeDir":"scopes/02-b","dependsOn":[1.5]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T24 fractional legacy identity refuses" 1 "$d" "not every scope has a usable scopeId/scope identity"

# T25: one alias cannot identify two canonical nodes. Ambiguity must fail loud
# instead of silently choosing whichever record happened to be processed last.
d="$TMP_ROOT/t25"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":"shared-legacy","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":"shared-legacy","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scope":"consumer-legacy","scopeDir":"scopes/03-c","dependsOn":["shared-legacy"]}
]}' > "$d/state.json"
mk_block "$d"
run_collision_diagnostic "T25 colliding legacy alias fails closed" "$d" 'alias "shared-legacy" identifies canonical scopes: SCOPE-foundation-a, SCOPE-foundation-b'

# T26: alias normalization must not blunt the early-increment exception. The
# early consumer uses a numeric alias while the late chain mixes canonical and
# string aliases; the least-blocked consumer still has only one foundation.
d="$TMP_ROOT/t26"
mk_scope "$d" 01-a consumer
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d foundation
mk_scope "$d" 05-e consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-early","scope":11,"scopeDir":"scopes/01-a","dependsOn":[12]},
  {"scopeId":"SCOPE-foundation-a","scope":12,"scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":"legacy-b","scopeDir":"scopes/03-c","dependsOn":[12]},
  {"scopeId":"SCOPE-foundation-c","scope":"legacy-c","scopeDir":"scopes/04-d","dependsOn":["legacy-b"]},
  {"scopeId":"SCOPE-late","scope":15,"scopeDir":"scopes/05-e","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run "T26 mixed aliases preserve early usable increment (exit 0)" 0 "$d"

# T27: zero is not a positive integral legacy identity. Apart from the zero
# node, this is a complete mixed symbolic/legacy horizontal chain. The block
# posture must refuse because the local DAG cannot represent every node.
d="$TMP_ROOT/t27"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":0,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":2,"scopeDir":"scopes/02-b","dependsOn":[0]},
  {"scopeId":"SCOPE-foundation-c","scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
  {"scopeId":"SCOPE-consumer","scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T27 zero numeric legacy identity refuses" 1 "$d" "not every scope has a usable scopeId/scope identity"

# T28: a negative integer is likewise unusable. This differs from T27 only in
# the invalid numeric class, so accepting signed integers as aliases would turn
# the fixture into the same real horizontal chain and produce a false block.
d="$TMP_ROOT/t28"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":-1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":2,"scopeDir":"scopes/02-b","dependsOn":[-1]},
  {"scopeId":"SCOPE-foundation-c","scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
  {"scopeId":"SCOPE-consumer","scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T28 negative numeric legacy identity refuses" 1 "$d" "not every scope has a usable scopeId/scope identity"

# T29: adversarial positive-integral partner for T27 and T28. The first node
# uses a legacy-only positive integer while the remaining canonical symbolic
# nodes are reached through positive numeric aliases. All three foundations
# must resolve into the consumer closure, yielding a deliberate policy exit 1.
d="$TMP_ROOT/t29"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"   ","scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scopeId":"SCOPE-foundation-c","scope":3,"scopeDir":"scopes/03-c","dependsOn":[2]},
  {"scopeId":"SCOPE-consumer","scope":4,"scopeDir":"scopes/04-d","dependsOn":[3]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T29 positive-integral mixed symbolic/legacy chain deliberately BLOCKS" "$d" "SCOPE-consumer" 3

# T30: T25's colliding declarations in reverse order. Registry construction
# must report the same exit code and the same sorted collision diagnostic,
# rather than selecting a winner based on declaration order.
d="$TMP_ROOT/t30"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-b","scope":"shared-legacy","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-a","scope":"shared-legacy","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scope":"consumer-legacy","scopeDir":"scopes/03-c","dependsOn":["shared-legacy"]}
]}' > "$d/state.json"
mk_block "$d"
run_collision_diagnostic "T30 reversed alias collision is deterministic" "$d" 'alias "shared-legacy" identifies canonical scopes: SCOPE-foundation-a, SCOPE-foundation-b'

# T31: duplicate canonical records previously overwrote depmap entries. In this
# order the second SCOPE-foundation-b record erased its dependency on
# SCOPE-foundation-a, reducing the consumer closure from three foundations to
# two and producing a false-clean exit 0. Duplicate nodes are malformed input,
# so the guard must refuse before classmap/depmap construction.
d="$TMP_ROOT/t31"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
duplicate_diagnostic='duplicate canonical scope identity — canonical scope "SCOPE-foundation-b" is declared by 2 records'
run_duplicate_diagnostic "T31 duplicate canonical records refuse instead of false-clean" "$d" "$duplicate_diagnostic"

# T32: reversing only the duplicate declarations used to change depmap's
# winner and turn T31 into a deliberate policy block. The malformed-input
# diagnostic must be byte-for-byte identical regardless of record order.
d="$TMP_ROOT/t32"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_duplicate_diagnostic "T32 reversed duplicate records keep the same diagnostic" "$d" "$duplicate_diagnostic"

# T33: distinct dependency tokens that resolve to one canonical node are a
# duplicate effective edge. Counting them once would conceal malformed graph
# input, so alias normalization must precede duplicate-target rejection.
d="$TMP_ROOT/t33"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation","scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scope":2,"scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation",1,"01"]}
]}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T33 duplicate effective dependency target refuses" 1 "$d" "duplicate effective dependency target SCOPE-foundation"

# T34: unique canonical IDs are the positive partner for T31/T32. With no
# duplicate record, the same three-foundation chain must still reach the
# deliberate policy verdict rather than being weakened to a blanket no-op.
d="$TMP_ROOT/t34"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T34 unique-ID partner deliberately BLOCKS" "$d" "SCOPE-consumer" 3

# T35: duplicate canonical identities are outside this guard's jurisdiction
# when the plan has no dependency edges. Applicability must be established
# before malformed-graph diagnostics that only matter to DAG construction.
d="$TMP_ROOT/t35"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-b","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run "T35 duplicate canonical records without edges → no-op (exit 0)" 0 "$d"

# T36: an edge plus a missing body is incomplete and refuses in block posture.
d="$TMP_ROOT/t36"
mk_scope "$d" 01-a foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-missing","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-missing","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]}
]}' > "$d/state.json"
mk_block "$d"
run_duplicate_diagnostic "T36 duplicate canonical records refuse despite missing body" "$d" "$duplicate_diagnostic"

# T37: alias ambiguity is likewise irrelevant when no dependency token needs
# resolution because the plan has no edges.
d="$TMP_ROOT/t37"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":"shared-legacy","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":"shared-legacy","scopeDir":"scopes/02-b","dependsOn":[]}
]}' > "$d/state.json"
mk_block "$d"
run "T37 alias collision without edges → no-op (exit 0)" 0 "$d"

# T38: alias ambiguity refuses when an edge exists, even if a body is missing.
d="$TMP_ROOT/t38"
mk_scope "$d" 01-a foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scope":"shared-legacy","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scope":"shared-legacy","scopeDir":"scopes/02-missing","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-c","dependsOn":["shared-legacy"]}
]}' > "$d/state.json"
mk_block "$d"
run_collision_diagnostic "T38 alias collision with missing body refuses" "$d" 'alias "shared-legacy" identifies canonical scopes: SCOPE-foundation-a, SCOPE-foundation-b'

# T39: applicable duplicate partner. Complete bodies plus a real edge restore
# graph applicability, so duplicate refusal must still precede map building.
d="$TMP_ROOT/t39"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]}
]}' > "$d/state.json"
mk_block "$d"
run_duplicate_diagnostic "T39 applicable duplicate partner still refuses" "$d" "$duplicate_diagnostic"

# T40: applicable unique-ID partner proves that applicability staging does not
# weaken horizontal-plan detection.
d="$TMP_ROOT/t40"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-foundation-a"]},
  {"scopeId":"SCOPE-foundation-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-foundation-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-d","dependsOn":["SCOPE-foundation-c"]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T40 applicable unique horizontal partner deliberately BLOCKS" "$d" "SCOPE-consumer" 3

# T41: canonical scopeId uses the same edge-whitespace normalization as its
# derived alias. Unpadded dependency tokens must resolve to trimmed canonical
# IDs, and diagnostics must never expose identity padding.
d="$TMP_ROOT/t41"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-d consumer
printf '%s\n' '{"scopeProgress":[
  {"scopeId":" foundation-a ","scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":" foundation-b ","scope":2,"scopeDir":"scopes/02-b","dependsOn":[" foundation-a "]},
  {"scopeId":" foundation-c ","scope":3,"scopeDir":"scopes/03-c","dependsOn":[" foundation-b "]},
  {"scopeId":" consumer ","scope":4,"scopeDir":"scopes/04-d","dependsOn":[" foundation-c "]}
]}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T41 edge-whitespace canonical IDs normalize before graph analysis" "$d" "consumer" 3

# T42: a large deep graph with many consumers sharing the same transitive tail
# exercises closure reuse. This is deliberately an outcome check, not an SLA:
# elapsed time is printed only as a regression-observation aid, while the
# surrounding focused-suite timeout supplies the hang bound.
d="$TMP_ROOT/t42"
foundation_count=300
consumer_count=60
i=1
while [[ "$i" -le "$foundation_count" ]]; do
  nn="$(printf '%03d-foundation' "$i")"
  mk_scope "$d" "$nn" foundation
  i=$((i + 1))
done
i=1
while [[ "$i" -le "$consumer_count" ]]; do
  nn="$(printf '%03d-consumer' "$((foundation_count + i))")"
  mk_scope "$d" "$nn" consumer
  i=$((i + 1))
done
{
  printf '{"scopeProgress":['
  i=1
  while [[ "$i" -le "$foundation_count" ]]; do
    [[ "$i" -gt 1 ]] && printf ','
    nn="$(printf '%03d-foundation' "$i")"
    if [[ "$i" -eq 1 ]]; then deps='[]'; else deps="[$((i - 1))]"; fi
    printf '{"scope":%s,"scopeDir":"scopes/%s","dependsOn":%s}' "$i" "$nn" "$deps"
    i=$((i + 1))
  done
  i=1
  while [[ "$i" -le "$consumer_count" ]]; do
    nn="$(printf '%03d-consumer' "$((foundation_count + i))")"
    printf ',{"scope":%s,"scopeDir":"scopes/%s","dependsOn":[%s]}' "$((foundation_count + i))" "$nn" "$foundation_count"
    i=$((i + 1))
  done
  printf ']}\n'
} > "$d/state.json"
mk_block "$d"
started_at=$SECONDS
run_block_diagnostic "T42 large deep shared-tail graph deliberately BLOCKS" "$d" "$((foundation_count + 1))" "$foundation_count"
echo "T42 observation: $((SECONDS - started_at)) second(s) for $foundation_count foundations and $consumer_count shared-tail consumers"

# T43: every scope-universe-resolver alias form resolves to the same canonical
# foundation: canonical ID, numeric legacy, zero-padded numeric, normalized
# scopeDir, basename, and scopeDir/scope.md.
d="$TMP_ROOT/t43"
aliases=("SCOPE-foundation" "1" "01" "scopes/01-foundation" "01-foundation" "scopes/01-foundation/scope.md")
i=1
for dependency_alias in "${aliases[@]}"; do
  case_dir="$d/$i"
  mk_scope "$case_dir" 01-foundation foundation
  mk_scope "$case_dir" 02-consumer consumer
  printf '%s\n' '{"scopeProgress":[' \
    '  {"scopeId":"SCOPE-foundation","scope":1,"scopeDir":"scopes/01-foundation/","dependsOn":[]},' \
    "  {\"scopeId\":\"SCOPE-consumer\",\"scope\":2,\"scopeDir\":\"scopes/02-consumer\",\"dependsOn\":[\"$dependency_alias\"]}" \
    ']}' > "$case_dir/state.json"
  mk_block "$case_dir"
  run "T43.$i closed alias '$dependency_alias' resolves" 0 "$case_dir"
  i=$((i + 1))
done

# T44: lexical traversal cannot escape the feature directory to claim a body.
d="$TMP_ROOT/t44"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 02-consumer consumer
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-foundation","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/../scopes/02-consumer","dependsOn":["SCOPE-foundation"]}' \
  ']}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T44 traversal scopeDir refuses" 1 "$d" "has an invalid or escaping scopeDir"

# T45: a scopeDir symlink to an external body is never followed.
d="$TMP_ROOT/t45"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 02-consumer consumer
outside="$TMP_ROOT/t45-outside"
mk_scope "$outside" escaped consumer
rm -rf "$d/scopes/02-consumer"
ln -s "$outside/scopes/escaped" "$d/scopes/02-consumer"
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-foundation","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":["SCOPE-foundation"]}' \
  ']}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T45 symlink scopeDir escape refuses" 1 "$d" "does not claim a readable regular non-symlink scopeDir/scope.md"

# T46: an unknown dependency is incomplete graph input, not an empty closure.
d="$TMP_ROOT/t46"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 02-consumer consumer
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-foundation","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":["SCOPE-unknown"]}' \
  ']}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T46 unknown edge refuses" 1 "$d" "depends on unknown alias SCOPE-unknown"

# T47: malformed identity and body findings are report-only only under the
# explicit default report posture; the same fixtures block when configured.
d="$TMP_ROOT/t47-report"
mk_scope "$d" 02-consumer consumer
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"","scope":0,"scopeDir":"scopes/01-missing","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":["missing"]}' \
  ']}' > "$d/state.json"
run_signal_diagnostic "T47 malformed identity reports under report posture" 0 "$d" "not every scope has a usable scopeId/scope identity"
mk_block "$d"
run_signal_diagnostic "T47 malformed identity blocks under block posture" 1 "$d" "not every scope has a usable scopeId/scope identity"

d="$TMP_ROOT/t47-body-report"
mk_scope "$d" 02-consumer consumer
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-missing","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":["SCOPE-foundation"]}' \
  ']}' > "$d/state.json"
run_signal_diagnostic "T47 missing body reports under report posture" 0 "$d" "does not claim a readable regular non-symlink scopeDir/scope.md"
mk_block "$d"
run_signal_diagnostic "T47 missing body blocks under block posture" 1 "$d" "does not claim a readable regular non-symlink scopeDir/scope.md"

# T48: two records cannot claim one physical body through lexical spellings.
d="$TMP_ROOT/t48"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 03-consumer consumer
printf '%s\n' '{"scopeProgress":[' \
  '  {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-foundation","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/01-foundation/","dependsOn":[]},' \
  '  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-consumer","dependsOn":["SCOPE-foundation-a"]}' \
  ']}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T48 duplicate physical body claim refuses" 1 "$d" "multiple scope records claim physical body"

# T49: divergent deprecated data cannot veto canonical evaluation.
d="$TMP_ROOT/t49"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 02-consumer consumer
printf '%s\n' '{
  "scopeProgress":[
    {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-foundation","dependsOn":[]},
    {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":[]}
  ],
  "certification":{"scopeProgress":[
    {"scopeId":"SCOPE-foundation","scopeDir":"scopes/01-foundation","dependsOn":[]},
    {"scopeId":"SCOPE-consumer","scopeDir":"scopes/02-consumer","dependsOn":["SCOPE-foundation"]}
  ]}
}' > "$d/state.json"
mk_block "$d"
run "T49 divergent legacy data cannot veto canonical scopeProgress (exit 0)" 0 "$d"

# T50: semantically equal authorities may coexist during migration. Object-key,
# record, and dependency order are representation details, not graph meaning.
d="$TMP_ROOT/t50"
mk_scope "$d" 01-foundation foundation
mk_scope "$d" 02-foundation foundation
mk_scope "$d" 03-consumer consumer
printf '%s\n' '{
  "scopeProgress":[
    {"dependsOn":["SCOPE-foundation-b","SCOPE-foundation-a"],"scopeDir":"scopes/03-consumer","scopeId":"SCOPE-consumer"},
    {"dependsOn":[],"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-foundation"},
    {"scopeDir":"scopes/01-foundation","dependsOn":[],"scopeId":"SCOPE-foundation-a"}
  ],
  "certification":{"scopeProgress":[
    {"scopeId":"SCOPE-foundation-a","scopeDir":"scopes/01-foundation","dependsOn":[]},
    {"scopeId":"SCOPE-foundation-b","scopeDir":"scopes/02-foundation","dependsOn":[]},
    {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-consumer","dependsOn":["SCOPE-foundation-a","SCOPE-foundation-b"]}
  ]}
}' > "$d/state.json"
mk_block "$d"
run "T50 semantically equal scopeProgress authorities remain valid" 0 "$d"

# T51: a direct self-loop must be rejected before consumer classification.
d="$TMP_ROOT/t51"
mk_scope "$d" 01-consumer consumer
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-consumer","scope":1,"scopeDir":"scopes/01-consumer","dependsOn":[1]}
]}}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T51 self-loop refuses" 1 "$d" "self dependency on canonical scope SCOPE-consumer"

# T52: a two-node cycle formerly converged to a closure and was classified.
d="$TMP_ROOT/t52"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-consumer consumer
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-a","scopeDir":"scopes/01-a","dependsOn":["SCOPE-b"]},
  {"scopeId":"SCOPE-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-a"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/03-consumer","dependsOn":["SCOPE-a"]}
]}}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T52 two-node cycle refuses" 1 "$d" "dependency cycle includes canonical scopes: SCOPE-a, SCOPE-b"

# T53: the same rejection must hold for cycles longer than two nodes.
d="$TMP_ROOT/t53"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-consumer consumer
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-a","scopeDir":"scopes/01-a","dependsOn":["SCOPE-b"]},
  {"scopeId":"SCOPE-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-c"]},
  {"scopeId":"SCOPE-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-a"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-consumer","dependsOn":["SCOPE-a"]}
]}}' > "$d/state.json"
mk_block "$d"
run_signal_diagnostic "T53 three-node cycle refuses" 1 "$d" "dependency cycle includes canonical scopes: SCOPE-a, SCOPE-b, SCOPE-c"

# T54: valid symbolic and numeric aliases still resolve after the graph is
# normalized into canonical targets once.
d="$TMP_ROOT/t54"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-consumer consumer
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-a","scope":1,"scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-b","scope":2,"scopeDir":"scopes/02-b","dependsOn":[1]},
  {"scopeId":"SCOPE-c","scope":"legacy-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-b"]},
  {"scopeId":"SCOPE-consumer","scope":4,"scopeDir":"scopes/04-consumer","dependsOn":["legacy-c"]}
]}}' > "$d/state.json"
mk_block "$d"
run_block_diagnostic "T54 mixed valid aliases still form a horizontal chain" "$d" "SCOPE-consumer" 3

# T55: classification must force the C locale rather than inherit caller state.
# A PATH shim makes inherited locale use observable without depending on which
# optional locales the host installed.
d="$TMP_ROOT/t55"
mk_scope "$d" 01-a foundation
mk_scope "$d" 02-b foundation
mk_scope "$d" 03-c foundation
mk_scope "$d" 04-consumer consumer
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-b","scopeDir":"scopes/02-b","dependsOn":["SCOPE-a"]},
  {"scopeId":"SCOPE-c","scopeDir":"scopes/03-c","dependsOn":["SCOPE-b"]},
  {"scopeId":"SCOPE-consumer","scopeDir":"scopes/04-consumer","dependsOn":["SCOPE-c"]}
]}}' > "$d/state.json"
mk_block "$d"
mkdir -p "$d/fake-bin"
real_grep="$(command -v grep)"
printf '%s\n' '#!/usr/bin/env bash' \
  "if [[ \"\${LC_ALL:-}\" != \"C\" ]]; then exit 97; fi" \
  "exec \"\${REAL_GREP:?}\" \"\$@\"" > "$d/fake-bin/grep"
chmod +x "$d/fake-bin/grep"
locale_output=""
locale_rc=0
locale_output="$(LC_ALL=POSIX REAL_GREP="$real_grep" PATH="$d/fake-bin:$PATH" bash "$GUARD" "$d" 2>&1)" && locale_rc=0 || locale_rc=$?
if [[ "$locale_rc" -eq 1 && "$locale_output" == *"DEPENDENCY-GRAPH HORIZONTAL PLAN"* ]]; then
  pass "T55 inherited locale cannot alter classification or posture"
else
  fail "T55 inherited locale changed classification/posture (exit $locale_rc: $locale_output)"
fi

# T56: equal-cost consumers use canonical scope ID as a total tie-breaker,
# independent of declaration order.
first="$TMP_ROOT/t56-first"
second="$TMP_ROOT/t56-second"
for d in "$first" "$second"; do
  mk_scope "$d" 01-a foundation
  mk_scope "$d" 02-b foundation
  mk_scope "$d" 03-alpha consumer
  mk_scope "$d" 04-zeta consumer
  mk_block "$d"
done
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-zeta","scopeDir":"scopes/04-zeta","dependsOn":["SCOPE-b"]},
  {"scopeId":"SCOPE-b","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-alpha","scopeDir":"scopes/03-alpha","dependsOn":["SCOPE-a"]},
  {"scopeId":"SCOPE-a","scopeDir":"scopes/01-a","dependsOn":[]}
]}}' > "$first/state.json"
printf '%s\n' '{"certification":{"scopeProgress":[
  {"scopeId":"SCOPE-a","scopeDir":"scopes/01-a","dependsOn":[]},
  {"scopeId":"SCOPE-alpha","scopeDir":"scopes/03-alpha","dependsOn":["SCOPE-a"]},
  {"scopeId":"SCOPE-b","scopeDir":"scopes/02-b","dependsOn":[]},
  {"scopeId":"SCOPE-zeta","scopeDir":"scopes/04-zeta","dependsOn":["SCOPE-b"]}
]}}' > "$second/state.json"
run_equal_cost_permutation "T56 equal-cost permutation has a stable canonical winner" "$first" "$second" "SCOPE-alpha"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "plan-dependency-depth-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "plan-dependency-depth-guard-selftest: all cases passed."
