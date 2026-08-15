#!/usr/bin/env bash
# Hermetic selftest for goal-contract.sh (IMP-038 SCOPE-1 / GF-1).
#
# Every case is adversarial: it fails if the guarantee it names regresses.
# In particular T3 (re-freeze refused), T7/T8 (substituted digest / revision
# rejected), and T9 (unapproved revision refused) are the three defects this
# script exists to prevent.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GC="$SCRIPT_DIR/goal-contract.sh"
BOUNDARY_RESOLVER="$SCRIPT_DIR/work-boundary-resolve.sh"
SCHEMA="$REPO_ROOT/bubbles/schemas/goal-contract.schema.json"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
skip() { echo "SKIP: $1"; }

if ! command -v jq >/dev/null 2>&1; then
  echo "goal-contract-selftest: SKIP (jq not installed)"
  exit 0
fi
[[ -f "$GC" ]] || { echo "FAIL: $GC not found" >&2; exit 1; }

# new_case <name> — a fresh workspace; echoes its directory.
new_case() {
  local d="$TMP_ROOT/$1"
  mkdir -p "$d"
  printf 'Freeze the operator outcome before planning begins.\n' > "$d/request.txt"
  echo "$d"
}

# freeze_default <dir> [extra goal-contract.sh args...]
freeze_default() {
  local d="$1"; shift
  bash "$GC" freeze \
    --session-file "$d/session.json" \
    --source-request-file "$d/request.txt" \
    --intent "Freeze one immutable Goal Contract per mutable run" \
    --success-signal "goal-contract-selftest exits 0" \
    --hard-constraint "no new gate id in SCOPE-1" \
    --non-goal "phase-relevance resolver" \
    --target "repository=bubbles" \
    --target "spec=specs/038-goal-fidelity" \
    --repository-root bubbles \
    --spec-target specs/038-goal-fidelity \
    --allowed-path 'bubbles/scripts/**' \
    --runner bubbles.goal \
    --session-id vscode-abc123 \
    --repository-alias bubbles \
    ${1+"$@"}
}

# expect_rc <label> <expected-rc> <command...>
expect_rc() {
  local label="$1" want="$2"; shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then
    pass "$label"
  else
    fail "$label (expected exit $want, got $rc)"
  fi
}

# expect_field <label> <session-file> <jq-path> <expected>
expect_field() {
  local label="$1" session="$2" path="$3" want="$4"
  local got rc=0
  got="$(bash "$GC" read --session-file "$session" --field "$path" 2>&1)" || rc=$?
  if [[ "$rc" -eq 0 && "$got" == "$want" ]]; then
    pass "$label"
  else
    fail "$label (rc=$rc, observed '$got', expected '$want')"
  fi
}

echo "Running goal-contract selftest..."

# ── T1 freeze produces a valid revision-1 auto-frozen contract ─────────────
d="$(new_case t1)"
if freeze_default "$d" >"$d/out.json" 2>"$d/err.txt"; then
  if [[ "$(jq -r '.revision' "$d/out.json")" == "1" ]] \
     && [[ "$(jq -r '.approval.state' "$d/out.json")" == "auto-frozen" ]] \
     && [[ "$(jq -r '.supersedes' "$d/out.json")" == "null" ]] \
     && [[ "$(jq -r '.goalId' "$d/out.json")" == "gc:vscode-abc123:1" ]] \
     && [[ "$(jq -r '.schemaVersion' "$d/out.json")" == "goal-contract/v1" ]]; then
    pass "T1 freeze -> revision 1, auto-frozen, supersedes=null, gc:<session>:1"
  else
    fail "T1 freeze produced the wrong shape: $(jq -c '{revision,approval,supersedes,goalId}' "$d/out.json")"
  fi
else
  fail "T1 freeze exited non-zero: $(cat "$d/err.txt")"
fi
expect_rc "T1b freeze result passes verify" 0 \
  bash "$GC" verify --session-file "$d/session.json"

# T1c the frozen contract validates against the real JSON Schema.
# jsonschema is OPTIONAL tooling: SKIP the assertion rather than hard-fail.
if python3 -c 'import jsonschema' >/dev/null 2>&1; then
  if python3 -c '
import json, sys, jsonschema
schema = json.load(open(sys.argv[1]))
doc = json.load(open(sys.argv[2]))
jsonschema.Draft202012Validator(schema).validate(doc)
' "$SCHEMA" "$d/out.json" >"$d/schema.err" 2>&1; then
    pass "T1c frozen contract validates against goal-contract.schema.json"
  else
    fail "T1c frozen contract failed schema validation: $(cat "$d/schema.err")"
  fi
else
  skip "T1c schema validation (python3 jsonschema not installed)"
fi

# T1d the emitted workBoundary is accepted by the REAL boundary resolver.
# This proves the schema mirrors work-boundary-resolve.sh rather than asserting it.
if [[ -f "$BOUNDARY_RESOLVER" ]]; then
  mkdir -p "$d/feature"
  jq '{ version: 3, status: "in_progress", workBoundary: .workBoundary }' "$d/out.json" \
    > "$d/feature/state.json"
  # Exit code and stdout are reported too: a resolver that fails without writing
  # stderr produced "rejected the frozen boundary:" with nothing after the colon.
  wb_rc=0
  bash "$BOUNDARY_RESOLVER" --feature-dir "$d/feature" --candidate-repo bubbles \
    --candidate-spec specs/038-goal-fidelity --candidate-path bubbles/scripts/x.sh \
    >"$d/wb.out" 2>"$d/wb.err" || wb_rc=$?
  if grep -qx 'disposition=in-boundary' "$d/wb.out"; then
    pass "T1d frozen workBoundary is accepted in-boundary by work-boundary-resolve.sh"
  else
    fail "T1d work-boundary-resolve.sh rejected the frozen boundary (exit=$wb_rc): stderr=$(tr '\n' ' ' <"$d/wb.err") stdout=$(tr '\n' ' ' <"$d/wb.out")"
  fi
else
  skip "T1d boundary cross-check (work-boundary-resolve.sh not found)"
fi

# ── T2 the digest is a deterministic function of the source-request bytes ──
d2="$(new_case t2)"
freeze_default "$d2" >"$d2/out.json" 2>/dev/null
digest_1="$(jq -r '.sourceRequestDigest' "$d/out.json")"
digest_2="$(jq -r '.sourceRequestDigest' "$d2/out.json")"
if [[ "$digest_1" == "$digest_2" && -n "$digest_1" ]]; then
  pass "T2 identical source-request bytes -> identical sourceRequestDigest"
else
  fail "T2 digest is not deterministic ('$digest_1' vs '$digest_2')"
fi

d2b="$(new_case t2b)"
printf 'A different operator request.\n' > "$d2b/request.txt"
freeze_default "$d2b" >"$d2b/out.json" 2>/dev/null
if [[ "$(jq -r '.sourceRequestDigest' "$d2b/out.json")" != "$digest_1" ]]; then
  pass "T2b different source-request bytes -> different digest"
else
  fail "T2b different bytes produced the same digest — the digest is not bound to the request"
fi

# ── T3 freezing twice is REFUSED (the silent-revision defect) ──────────────
expect_rc "T3 second freeze on an existing contract -> exit 3 (refused)" 3 \
  freeze_default "$d"
if [[ "$(jq -r '.goalContract.intent' "$d/session.json")" == "Freeze one immutable Goal Contract per mutable run" ]]; then
  pass "T3b the refused re-freeze left the stored contract untouched"
else
  fail "T3b the refused re-freeze mutated the stored contract"
fi

# ── T4 read (whole contract, and one field) ────────────────────────────────
if bash "$GC" read --session-file "$d/session.json" 2>/dev/null | jq -e '.goalId == "gc:vscode-abc123:1"' >/dev/null; then
  pass "T4 read returns the stored contract"
else
  fail "T4 read did not return the stored contract"
fi
expect_field "T4b read --field .intent returns just the intent" \
  "$d/session.json" ".intent" "Freeze one immutable Goal Contract per mutable run"

# ── T5 read on a session with no contract -> exit 4 ────────────────────────
d5="$(new_case t5)"
printf '{ "turnSnapshots": [] }\n' > "$d5/session.json"
expect_rc "T5 read with no .goalContract -> exit 4" 4 \
  bash "$GC" read --session-file "$d5/session.json"
expect_rc "T5b verify with no .goalContract -> exit 4" 4 \
  bash "$GC" verify --session-file "$d5/session.json"
expect_rc "T5c revise with no .goalContract -> exit 4" 4 \
  bash "$GC" revise --session-file "$d5/session.json" --approval-note "x"

# ── T6 verify passes for a matching goalId / revision / digest ─────────────
expect_rc "T6 verify with matching goalId+revision+digest -> exit 0" 0 \
  bash "$GC" verify --session-file "$d/session.json" \
    --expect-goal-id "gc:vscode-abc123:1" --expect-revision 1 --expect-digest "$digest_1"

# ── T7 verify FAILS on a substituted digest (adversarial) ──────────────────
expect_rc "T7 verify with a substituted --expect-digest -> exit 1" 1 \
  bash "$GC" verify --session-file "$d/session.json" \
    --expect-digest "sha256:$(printf '0%.0s' $(seq 1 64))"

d7="$(new_case t7)"
freeze_default "$d7" >/dev/null 2>&1
jq '.goalContract.sourceRequestDigest = "sha256:'"$(printf 'a%.0s' $(seq 1 64))"'"' \
  "$d7/session.json" > "$d7/session.tmp" && mv "$d7/session.tmp" "$d7/session.json"
expect_rc "T7b a digest substituted IN THE SESSION FILE fails verify -> exit 1" 1 \
  bash "$GC" verify --session-file "$d7/session.json" --expect-digest "$digest_1"

# ── T8 verify fails on a substituted revision ─────────────────────────────
d8="$(new_case t8)"
freeze_default "$d8" >/dev/null 2>&1
jq '.goalContract.revision = 7' "$d8/session.json" > "$d8/session.tmp" \
  && mv "$d8/session.tmp" "$d8/session.json"
expect_rc "T8 a revision substituted in the session file fails verify -> exit 1" 1 \
  bash "$GC" verify --session-file "$d8/session.json" --expect-revision 1
expect_rc "T8b the same substitution fails verify even with NO expectation flags" 1 \
  bash "$GC" verify --session-file "$d8/session.json"

# ── T9 revise without an approval note is REFUSED ─────────────────────────
d9="$(new_case t9)"
freeze_default "$d9" >/dev/null 2>&1
expect_rc "T9 revise without --approval-note -> exit 3 (refused)" 3 \
  bash "$GC" revise --session-file "$d9/session.json" --intent "a wider intent"
expect_field "T9b the refused revise did not change the intent" \
  "$d9/session.json" ".intent" "Freeze one immutable Goal Contract per mutable run"
expect_field "T9c the refused revise did not change the revision" \
  "$d9/session.json" ".revision" "1"

# ── T10 an approved revise increments, supersedes, and records approval ────
if bash "$GC" revise --session-file "$d9/session.json" \
     --approval-note "operator approved the wider intent" \
     --intent "a wider intent" >"$d9/rev.json" 2>"$d9/rev.err"; then
  if [[ "$(jq -r '.revision' "$d9/rev.json")" == "2" ]] \
     && [[ "$(jq -r '.goalId' "$d9/rev.json")" == "gc:vscode-abc123:2" ]] \
     && [[ "$(jq -r '.supersedes' "$d9/rev.json")" == "gc:vscode-abc123:1" ]] \
     && [[ "$(jq -r '.approval.state' "$d9/rev.json")" == "operator-approved" ]] \
     && [[ "$(jq -r '.approval.approvedAt' "$d9/rev.json")" != "null" ]] \
     && [[ "$(jq -r '.intent' "$d9/rev.json")" == "a wider intent" ]]; then
    pass "T10 revise -> revision 2, supersedes prior goalId, operator-approved"
  else
    fail "T10 revise produced the wrong shape: $(jq -c '{revision,goalId,supersedes,approval}' "$d9/rev.json")"
  fi
else
  fail "T10 revise exited non-zero: $(cat "$d9/rev.err")"
fi
expect_rc "T10b the revised contract passes verify at revision 2" 0 \
  bash "$GC" verify --session-file "$d9/session.json" \
    --expect-goal-id "gc:vscode-abc123:2" --expect-revision 2

# ── T11 widening vs narrowing is classified, never silently reclassified ──
d11="$(new_case t11)"
freeze_default "$d11" >/dev/null 2>&1
bash "$GC" revise --session-file "$d11/session.json" --approval-note "add secondary-repo" \
  --repository-root bubbles --repository-root secondary-repo >/dev/null 2>&1
expect_field "T11 revise that ADDS a repositoryRoot -> 'widened: ' note prefix" \
  "$d11/session.json" ".approval.approvalNote" "widened: add secondary-repo"

d11b="$(new_case t11b)"
freeze_default "$d11b" \
  --spec-target specs/999-extra >/dev/null 2>&1
bash "$GC" revise --session-file "$d11b/session.json" --approval-note "drop the extra spec" \
  --spec-target specs/038-goal-fidelity >/dev/null 2>&1
expect_field "T11b revise that REMOVES a specTarget -> 'narrowed: ' note prefix" \
  "$d11b/session.json" ".approval.approvalNote" "narrowed: drop the extra spec"

d11c="$(new_case t11c)"
freeze_default "$d11c" >/dev/null 2>&1
bash "$GC" revise --session-file "$d11c/session.json" --approval-note "reword only" \
  --intent "same boundary, new wording" >/dev/null 2>&1
expect_field "T11c revise that leaves the boundary alone -> 'unchanged: ' prefix" \
  "$d11c/session.json" ".approval.approvalNote" "unchanged: reword only"

d11d="$(new_case t11d)"
freeze_default "$d11d" >/dev/null 2>&1
bash "$GC" revise --session-file "$d11d/session.json" --approval-note "swap roots" \
  --repository-root secondary-repo >/dev/null 2>&1
expect_field "T11d a mixed add+remove is 'widened: ' (an addition outranks a removal)" \
  "$d11d/session.json" ".approval.approvalNote" "widened: swap roots"

d11e="$(new_case t11e)"
freeze_default "$d11e" >/dev/null 2>&1
bash "$GC" revise --session-file "$d11e/session.json" --approval-note "allow cross-repo" \
  --cross-repo-policy authorized >/dev/null 2>&1
expect_field "T11e forbidden -> authorized is a boundary widening" \
  "$d11e/session.json" ".approval.approvalNote" "widened: allow cross-repo"

# ── T12 mirror writes ONLY the three ref fields and preserves .execution ───
d12="$(new_case t12)"
freeze_default "$d12" >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "status": "in_progress", "execution": { "currentScope": "SCOPE-1", "currentPhase": "implement" } }' \
  > "$d12/state.json"
if bash "$GC" mirror --session-file "$d12/session.json" --state-file "$d12/state.json" >/dev/null 2>"$d12/err.txt"; then
  ref_keys="$(jq -r '.execution.goalContractRef | keys | join(",")' "$d12/state.json")"
  if [[ "$ref_keys" == "goalId,revision,sourceRequestDigest" ]]; then
    pass "T12 mirror writes exactly goalId, revision, sourceRequestDigest"
  else
    fail "T12 mirror wrote the wrong key set: '$ref_keys'"
  fi
  if [[ "$(jq -r '.execution.currentScope' "$d12/state.json")" == "SCOPE-1" ]] \
     && [[ "$(jq -r '.execution.currentPhase' "$d12/state.json")" == "implement" ]] \
     && [[ "$(jq -r '.status' "$d12/state.json")" == "in_progress" ]]; then
    pass "T12b mirror preserved pre-existing .execution keys and siblings"
  else
    fail "T12b mirror dropped pre-existing state: $(jq -c . "$d12/state.json")"
  fi
  if [[ "$(jq -r '.execution.goalContractRef | has("intent") or has("successSignal") or has("hardConstraints") or has("workBoundary")' "$d12/state.json")" == "false" ]]; then
    pass "T12c mirror leaked no intent/successSignal/constraints/boundary into state.json (R5)"
  else
    fail "T12c mirror leaked contract content into state.json"
  fi
else
  fail "T12 mirror exited non-zero: $(cat "$d12/err.txt")"
fi
expect_rc "T12d mirror with a missing state file -> exit 2" 2 \
  bash "$GC" mirror --session-file "$d12/session.json" --state-file "$d12/absent.json"
expect_rc "T12e mirror with no contract -> exit 4" 4 \
  bash "$GC" mirror --session-file "$d5/session.json" --state-file "$d12/state.json"

# ── T13 a malformed workBoundary is refused at freeze ──────────────────────
d13="$(new_case t13)"
expect_rc "T13 freeze with no --repository-root (empty repositoryRoots) -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d13/session.json" \
    --source-request-file "$d13/request.txt" \
    --intent i --success-signal s --target "repository=bubbles" \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
if [[ -f "$d13/session.json" ]] && [[ "$(jq -r 'has("goalContract")' "$d13/session.json")" == "true" ]]; then
  fail "T13b the refused freeze still wrote a contract"
else
  pass "T13b the refused freeze wrote no contract"
fi
expect_rc "T13c freeze with an empty-string --repository-root -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d13/session2.json" \
    --source-request-file "$d13/request.txt" \
    --intent i --success-signal s --target "repository=bubbles" --repository-root "" \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
expect_rc "T13d freeze with an invalid --cross-repo-policy -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d13/session3.json" \
    --source-request-file "$d13/request.txt" \
    --intent i --success-signal s --target "repository=bubbles" --repository-root bubbles \
    --cross-repo-policy maybe \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles

# ── T14 remaining required-input and enum refusals ────────────────────────
d14="$(new_case t14)"
expect_rc "T14 freeze with no --target -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d14/session.json" \
    --source-request-file "$d14/request.txt" \
    --intent i --success-signal s --repository-root bubbles \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
expect_rc "T14b freeze with an out-of-enum --target kind -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d14/session.json" \
    --source-request-file "$d14/request.txt" \
    --intent i --success-signal s --target "database=main" --repository-root bubbles \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
expect_rc "T14c freeze with no --success-signal -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d14/session.json" \
    --source-request-file "$d14/request.txt" \
    --intent i --target "repository=bubbles" --repository-root bubbles \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
expect_rc "T14d freeze with a missing --source-request-file -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d14/session.json" \
    --source-request-file "$d14/absent.txt" \
    --intent i --success-signal s --target "repository=bubbles" --repository-root bubbles \
    --runner bubbles.goal --session-id vscode-abc123 --repository-alias bubbles
# A ':' in the session id would make gc:<sessionId>:<revision> unparseable.
expect_rc "T14e freeze with a ':' in --session-id -> exit 2" 2 \
  bash "$GC" freeze --session-file "$d14/session.json" \
    --source-request-file "$d14/request.txt" \
    --intent i --success-signal s --target "repository=bubbles" --repository-root bubbles \
    --runner bubbles.goal --session-id "vscode:abc:123" --repository-alias bubbles
expect_rc "T14f unknown subcommand -> exit 2" 2 bash "$GC" explode
expect_rc "T14g no subcommand -> exit 2" 2 bash "$GC"
expect_rc "T14h --help -> exit 0" 0 bash "$GC" --help

# ── T15 there is no bypass flag ───────────────────────────────────────────
for bypass in --force --skip --ignore --no-verify; do
  expect_rc "T15 '$bypass' is not accepted (no bypass exists)" 2 \
    bash "$GC" freeze --session-file "$TMP_ROOT/t15.json" "$bypass"
done
if grep -qE '^\s*--(force|skip|ignore|no-verify|unsafe)\)' "$GC"; then
  fail "T15b goal-contract.sh declares a bypass-shaped flag"
else
  pass "T15b goal-contract.sh declares no bypass-shaped flag"
fi

# ── T16 the shared outcome model ──────────────────────────────────────────
# SCOPE-1 claims a compiled scenario's `rootOutcome` and the Goal Contract now
# derive from ONE `$defs.outcomeCore` instead of two outcome models. That claim
# is only worth anything if a rootOutcome the EXISTING lint accepts also
# validates against `$defs.scenarioRootOutcome` — otherwise "unified" would
# just mean a second model was written next to the first one.
#
# The fixture is copied from scenario-compile-lint-selftest.sh's clean case, so
# it drifts loudly rather than silently if that lint's contract changes.
if python3 -c 'import jsonschema' >/dev/null 2>&1; then
  d16="$(new_case t16-shared-outcome)"
  cat > "$d16/root-outcome.json" <<'EOF'
{
  "intent": "Product is live and operable on the target environment",
  "successSignal": "Service health endpoint green on the target after deploy",
  "hardConstraints": ["local-target build, not cloud"],
  "failureCondition": "Any node blocked or health check red after deploy"
}
EOF
  # validate_as <schema-$defs-name> <doc> -> rc 0 valid, rc 1 invalid
  validate_as() {
    # shellcheck disable=SC2016  # "#/$defs/" is a literal JSON pointer, not a shell var
    python3 -c '
import json, sys, jsonschema
schema = json.load(open(sys.argv[1]))
doc = json.load(open(sys.argv[2]))
sub = dict(schema)
sub["$ref"] = "#/$defs/" + sys.argv[3]
jsonschema.Draft202012Validator(sub).validate(doc)
' "$SCHEMA" "$2" "$1" 2>/dev/null
  }

  if validate_as scenarioRootOutcome "$d16/root-outcome.json"; then
    pass "T16 a lint-accepted scenario rootOutcome validates against the shared schema"
  else
    fail "T16 a lint-accepted scenario rootOutcome was REJECTED by the shared schema"
  fi
  if validate_as outcomeCore "$d16/root-outcome.json"; then
    pass "T16b the same rootOutcome satisfies the shared outcomeCore mixin"
  else
    fail "T16b the shared outcomeCore rejected a valid scenario rootOutcome"
  fi

  # Adversarial: the two scenario-only tightenings must actually bite. Dropping
  # failureCondition is legal for a Goal Contract and illegal for a scenario, so
  # a schema that merely aliased the two models would pass both of these.
  jq 'del(.failureCondition)' "$d16/root-outcome.json" > "$d16/no-fc.json"
  if validate_as scenarioRootOutcome "$d16/no-fc.json"; then
    fail "T16c scenarioRootOutcome accepted a missing failureCondition (lint rejects it)"
  else
    pass "T16c scenarioRootOutcome rejects a missing failureCondition, matching the lint"
  fi
  if validate_as outcomeCore "$d16/no-fc.json"; then
    pass "T16d outcomeCore still allows an absent failureCondition (Goal Contract case)"
  else
    fail "T16d outcomeCore wrongly requires failureCondition, which would break freeze"
  fi

  jq '.hardConstraints = []' "$d16/root-outcome.json" > "$d16/no-hc.json"
  if validate_as scenarioRootOutcome "$d16/no-hc.json"; then
    fail "T16e scenarioRootOutcome accepted empty hardConstraints (lint rejects it)"
  else
    pass "T16e scenarioRootOutcome rejects empty hardConstraints, matching the lint"
  fi
else
  skip "T16 shared outcome model (python3 jsonschema not installed)"
fi

# ═══════════════════════════════════════════════════════════════════════════
# T17-T21 sync-boundary (IMP-038 SCOPE-2 / GF-2)
#
# The contract's workBoundary and the boundary work-boundary-resolve.sh actually
# enforces must be ONE fact. T17c/T17d are the load-bearing pair: they prove the
# sync is what turns a strict REFUSAL into a decision, so a regression that makes
# sync a no-op fails loudly instead of leaving a silently unbounded run.
# ═══════════════════════════════════════════════════════════════════════════

d17="$(new_case t17)"
freeze_default "$d17" >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "status": "in_progress", "execution": { "currentScope": "SCOPE-2" } }' \
  > "$d17/state.json"

if [[ -f "$BOUNDARY_RESOLVER" ]]; then
  expect_rc "T17 BEFORE sync, --strict refuses the spec (exit 3) — the GF-2 hole" 3 \
    bash "$BOUNDARY_RESOLVER" --feature-dir "$d17" --candidate-repo bubbles --strict
else
  skip "T17 pre-sync strict refusal (work-boundary-resolve.sh not found)"
fi

if bash "$GC" sync-boundary --session-file "$d17/session.json" --state-file "$d17/state.json" \
     >/dev/null 2>"$d17/sync.err"; then
  contract_wb="$(bash "$GC" read --session-file "$d17/session.json" --field '.workBoundary | tojson' 2>/dev/null)"
  state_wb="$(jq -c '.workBoundary' "$d17/state.json")"
  if [[ "$contract_wb" == "$state_wb" ]]; then
    pass "T17b sync-boundary wrote the contract's workBoundary verbatim"
  else
    fail "T17b sync wrote a different boundary (contract: $contract_wb, state: $state_wb)"
  fi
  if [[ "$(jq -r '.execution.currentScope' "$d17/state.json")" == "SCOPE-2" ]] \
     && [[ "$(jq -r '.status' "$d17/state.json")" == "in_progress" ]] \
     && [[ "$(jq -r '.version' "$d17/state.json")" == "3" ]]; then
    pass "T17c sync-boundary preserved every pre-existing state.json key"
  else
    fail "T17c sync-boundary dropped pre-existing state: $(jq -c . "$d17/state.json")"
  fi
else
  fail "T17b sync-boundary exited non-zero: $(cat "$d17/sync.err")"
fi

if [[ -f "$BOUNDARY_RESOLVER" ]]; then
  expect_rc "T17d AFTER sync, --strict --require-allowed-paths DECIDES (exit 0)" 0 \
    bash "$BOUNDARY_RESOLVER" --feature-dir "$d17" --candidate-repo bubbles \
      --candidate-spec specs/038-goal-fidelity --candidate-path bubbles/scripts/x.sh \
      --strict --require-allowed-paths
else
  skip "T17d post-sync strict decision (work-boundary-resolve.sh not found)"
fi

# ── T18 narrowing succeeds: a planner may shrink reach without approval ────
d18="$(new_case t18)"
freeze_default "$d18" >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "workBoundary": { "repositoryRoots": ["bubbles","secondary-repo"], "specTargets": ["specs/038-goal-fidelity","specs/999-extra"], "allowedPaths": ["bubbles/scripts/**","docs/**"], "crossRepoPolicy": "forbidden" } }' \
  > "$d18/state.json"
expect_rc "T18 sync-boundary that NARROWS an existing boundary -> exit 0" 0 \
  bash "$GC" sync-boundary --session-file "$d18/session.json" --state-file "$d18/state.json"
if [[ "$(jq -c '.workBoundary.repositoryRoots' "$d18/state.json")" == '["bubbles"]' ]] \
   && [[ "$(jq -r '.workBoundary.specTargets | index("specs/999-extra") == null' "$d18/state.json")" == "true" ]]; then
  pass "T18b the narrowed boundary actually replaced the wider one"
else
  fail "T18b narrowing did not take effect: $(jq -c '.workBoundary' "$d18/state.json")"
fi

# ── T19 widening is REFUSED and leaves state.json byte-identical ───────────
d19="$(new_case t19)"
freeze_default "$d19" >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "status": "in_progress", "workBoundary": { "repositoryRoots": ["bubbles"], "specTargets": ["specs/038-goal-fidelity"], "crossRepoPolicy": "forbidden" } }' \
  > "$d19/state.json"
cp "$d19/state.json" "$d19/state.before"
expect_rc "T19 sync-boundary that would WIDEN an existing boundary -> exit 3 (refused)" 3 \
  bash "$GC" sync-boundary --session-file "$d19/session.json" --state-file "$d19/state.json"
if cmp -s "$d19/state.before" "$d19/state.json"; then
  pass "T19b the refused widen left state.json byte-identical"
else
  fail "T19b the refused widen MUTATED state.json: $(jq -c '.workBoundary' "$d19/state.json")"
fi

d19b="$(new_case t19b)"
freeze_default "$d19b" --cross-repo-policy authorized >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "workBoundary": { "repositoryRoots": ["bubbles"], "specTargets": ["specs/038-goal-fidelity"], "allowedPaths": ["bubbles/scripts/**"], "crossRepoPolicy": "forbidden" } }' \
  > "$d19b/state.json"
expect_rc "T19c forbidden -> authorized is a widening, so sync refuses it too" 3 \
  bash "$GC" sync-boundary --session-file "$d19b/session.json" --state-file "$d19b/state.json"

# ── T20 an unchanged re-sync is idempotent, not a refusal ──────────────────
expect_rc "T20 re-syncing an already-identical boundary -> exit 0 (idempotent)" 0 \
  bash "$GC" sync-boundary --session-file "$d17/session.json" --state-file "$d17/state.json"

# ── T21 sync-boundary input handling ──────────────────────────────────────
expect_rc "T21 sync-boundary with no --state-file -> exit 2" 2 \
  bash "$GC" sync-boundary --session-file "$d17/session.json"
expect_rc "T21b sync-boundary with a missing state file -> exit 2" 2 \
  bash "$GC" sync-boundary --session-file "$d17/session.json" --state-file "$d17/absent.json"
expect_rc "T21c sync-boundary with no contract -> exit 4" 4 \
  bash "$GC" sync-boundary --session-file "$d5/session.json" --state-file "$d17/state.json"
d21="$(new_case t21)"
freeze_default "$d21" >/dev/null 2>&1
printf '%s\n' '{ "version": 3, "workBoundary": "bubbles" }' > "$d21/state.json"
expect_rc "T21d sync-boundary onto a non-object existing workBoundary -> exit 2" 2 \
  bash "$GC" sync-boundary --session-file "$d21/session.json" --state-file "$d21/state.json"
for bypass in --force --skip --ignore --no-verify; do
  expect_rc "T21e sync-boundary rejects '$bypass' (no bypass exists)" 2 \
    bash "$GC" sync-boundary --session-file "$d17/session.json" --state-file "$d17/state.json" "$bypass"
done

# ── SCOPE-3 / GF-1, GF-5: goal identity threaded through every transition ────
# `ref` is the ONE producer and `verify-ref` the ONE comparator. The three
# defects below all look like a well-formed payload to a reader: an omitted
# field asserts nothing, a substituted field re-points the work at a different
# goal, and a widened boundary grants unapproved reach.

d22="$(new_case t22)"
freeze_default "$d22" >/dev/null 2>&1
ref22="$d22/ref.json"
bash "$GC" ref --session-file "$d22/session.json" > "$ref22" 2>/dev/null

if [[ "$(jq -r '[keys_unsorted[]] | join(",")' "$ref22")" == "goalId,revision,sourceRequestDigest,workBoundary" ]]; then
  pass "T22 ref emits exactly goalId, revision, sourceRequestDigest, workBoundary"
else
  fail "T22 ref emitted unexpected keys: $(jq -c 'keys_unsorted' "$ref22")"
fi
# A transition ref carries the boundary; the durable spec mirror deliberately
# does not (R5). Asserting the difference keeps the two from converging.
if [[ "$(jq -r 'has("intent") or has("successSignal") or has("hardConstraints") or has("targets")' "$ref22")" == "false" ]]; then
  pass "T22b ref leaked no intent/successSignal/constraints/targets"
else
  fail "T22b ref leaked contract prose: $(jq -c 'keys_unsorted' "$ref22")"
fi
expect_rc "T22c a freshly produced ref verifies, boundary included" 0 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$ref22" --require-boundary

for omitted in goalId revision sourceRequestDigest; do
  jq --arg k "$omitted" 'del(.[$k])' "$ref22" > "$d22/omit-$omitted.json"
  expect_rc "T23 a ref omitting $omitted -> exit 1" 1 \
    bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/omit-$omitted.json"
done

jq '.sourceRequestDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"' \
  "$ref22" > "$d22/sub-digest.json"
expect_rc "T24 a substituted digest -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/sub-digest.json"
jq '.revision = 99' "$ref22" > "$d22/sub-rev.json"
expect_rc "T24b a substituted revision -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/sub-rev.json"
jq '.goalId = "gc:someone-elses-session:1"' "$ref22" > "$d22/sub-id.json"
expect_rc "T24c a substituted goalId -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/sub-id.json"

jq '.workBoundary.repositoryRoots += ["secondary-repo"]' "$ref22" > "$d22/widen-repo.json"
expect_rc "T25 a ref claiming an extra repositoryRoot -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/widen-repo.json" --require-boundary
jq '.workBoundary.allowedPaths = ["docs/**"]' "$ref22" > "$d22/widen-path.json"
expect_rc "T25b a ref claiming a path outside the frozen glob -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/widen-path.json" --require-boundary
jq '.workBoundary.crossRepoPolicy = "authorized"' "$ref22" > "$d22/widen-policy.json"
expect_rc "T25c a ref upgrading crossRepoPolicy to authorized -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/widen-policy.json" --require-boundary

# Narrowing is legitimate: a specialist reports back a subset of what it was
# given. These two cases are the reason the comparator tests COVERAGE rather
# than set difference — under set difference both read as additions and were
# falsely refused.
jq '.workBoundary.allowedPaths = ["bubbles/scripts/goal-contract.sh"]' "$ref22" > "$d22/narrow-path.json"
expect_rc "T26 a ref narrowing a glob to one file inside it -> exit 0" 0 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/narrow-path.json" --require-boundary
jq '.workBoundary.specTargets = ["038-goal-fidelity"]' "$ref22" > "$d22/narrow-spec.json"
expect_rc "T26b a ref using the resolver's basename spec form -> exit 0" 0 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/narrow-spec.json" --require-boundary

jq 'del(.workBoundary)' "$ref22" > "$d22/no-boundary.json"
expect_rc "T27 a ref with no workBoundary and --require-boundary -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/no-boundary.json" --require-boundary
expect_rc "T27b the same ref WITHOUT --require-boundary still verifies" 0 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/no-boundary.json"

printf '%s\n' '["not","an","object"]' > "$d22/array-ref.json"
expect_rc "T28 a non-object ref -> exit 1" 1 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/array-ref.json"
printf '%s\n' 'not json at all' > "$d22/bad.json"
expect_rc "T28b a non-JSON ref file -> exit 2" 2 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/bad.json"
expect_rc "T28c verify-ref with no --ref-file -> exit 2" 2 \
  bash "$GC" verify-ref --session-file "$d22/session.json"
expect_rc "T28d verify-ref with a missing ref file -> exit 2" 2 \
  bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$d22/absent.json"
expect_rc "T28e verify-ref with no contract -> exit 4" 4 \
  bash "$GC" verify-ref --session-file "$d5/session.json" --ref-file "$ref22"
expect_rc "T28f ref with no contract -> exit 4" 4 \
  bash "$GC" ref --session-file "$d5/session.json"
for bypass in --force --skip --ignore --no-verify; do
  expect_rc "T28g verify-ref rejects '$bypass' (no bypass exists)" 2 \
    bash "$GC" verify-ref --session-file "$d22/session.json" --ref-file "$ref22" "$bypass"
done

# A revision moves the identity. Every ref minted against the prior revision
# must stop verifying, or resume after an approved expansion would silently
# keep running against stale planning.
d29="$(new_case t29)"
freeze_default "$d29" >/dev/null 2>&1
bash "$GC" ref --session-file "$d29/session.json" > "$d29/ref-r1.json" 2>/dev/null
bash "$GC" revise --session-file "$d29/session.json" --approval-note "operator widened to secondary-repo" \
  --repository-root bubbles --repository-root secondary-repo >/dev/null 2>&1
expect_rc "T29 a ref minted at revision 1 fails after an approved revision" 1 \
  bash "$GC" verify-ref --session-file "$d29/session.json" --ref-file "$d29/ref-r1.json"
bash "$GC" ref --session-file "$d29/session.json" > "$d29/ref-r2.json" 2>/dev/null
expect_rc "T29b a ref re-minted at revision 2 verifies" 0 \
  bash "$GC" verify-ref --session-file "$d29/session.json" --ref-file "$d29/ref-r2.json" --require-boundary
if [[ "$(jq -r '.revision' "$d29/ref-r2.json")" == "2" ]]; then
  pass "T29c the re-minted ref carries the new revision"
else
  fail "T29c re-minted ref revision: $(jq -r '.revision' "$d29/ref-r2.json")"
fi

# ── Cross-script agreement: one matching rule, two implementations ──────────
# classify_boundary_change() mirrors work-boundary-resolve.sh's path matching.
# This table drives BOTH through their real entry points — the resolver via a
# candidate path, the comparator via `verify-ref --require-boundary` on a ref
# that narrows allowedPaths to exactly that candidate. A change to either side
# alone fails here rather than silently diverging, which is how a boundary
# starts meaning one thing to the resolver and another to the comparator.
if [[ -f "$BOUNDARY_RESOLVER" ]]; then
  d30="$(new_case t30)"
  freeze_default "$d30" --allowed-path 'docs/guides/' --allowed-path 'README.md' >/dev/null 2>&1
  bash "$GC" ref --session-file "$d30/session.json" > "$d30/ref.json" 2>/dev/null
  bash "$GC" sync-boundary --session-file "$d30/session.json" --state-file "$d30/state.json" >/dev/null 2>&1 \
    || printf '%s\n' "{ \"version\": 3, \"workBoundary\": $(jq -c '.workBoundary' "$d30/ref.json") }" > "$d30/state.json"
  # candidate|expected-in-boundary. The frozen allowedPaths are
  # bubbles/scripts/** (glob), docs/guides/ (dir prefix), README.md (exact).
  agreement_rows=(
    'bubbles/scripts/goal-contract.sh|yes'
    'bubbles/scripts|yes'
    'docs/guides/CONTROL_PLANE_DESIGN.md|yes'
    'README.md|yes'
    'docs/other.md|no'
    'bubbles/schemas/x.json|no'
    'README.md.bak|no'
  )
  agreement_ok="true"
  for row in "${agreement_rows[@]}"; do
    cand="${row%%|*}"; want="${row##*|}"

    resolver_disposition="$(bash "$BOUNDARY_RESOLVER" --feature-dir "$d30" \
      --candidate-repo bubbles --candidate-path "$cand" 2>/dev/null \
      | sed -n 's/^disposition=//p')"
    resolver_says="no"; [[ "$resolver_disposition" == "in-boundary" ]] && resolver_says="yes"

    # A ref that declares ONLY this candidate is a NARROWING exactly when the
    # frozen list already covers it, so verify-ref's exit code IS the
    # comparator's coverage verdict — reached through the real caller, not by
    # reaching into the function.
    jq --arg p "$cand" '.workBoundary.allowedPaths = [$p]' "$d30/ref.json" > "$d30/probe.json"
    comparator_rc=0
    bash "$GC" verify-ref --session-file "$d30/session.json" \
      --ref-file "$d30/probe.json" --require-boundary >/dev/null 2>&1 || comparator_rc=$?
    comparator_says="no"; [[ "$comparator_rc" -eq 0 ]] && comparator_says="yes"

    if [[ "$resolver_says" != "$want" || "$comparator_says" != "$want" ]]; then
      fail "T30 agreement for '$cand': want $want, resolver=$resolver_says ($resolver_disposition), comparator=$comparator_says (rc=$comparator_rc)"
      agreement_ok="false"
    fi
  done
  [[ "$agreement_ok" == "true" ]] && pass "T30 resolver and comparator agree on every path-matching row"
else
  skip "T30 cross-script matching agreement (work-boundary-resolve.sh not found)"
fi

# --- IMP-041 SCOPE-1: v2 semantic boundary ----------------------------------
# The v1 boundary is path-shaped, so a goal could grow from a bounded test into
# a platform with every path still in-boundary. These cases exist to prove the
# second, shape-shaped layer refuses that — and that adding it did not disturb
# v1, which every existing frozen contract still uses.

# freeze_semantic <dir> <session-id> [extra args...]
freeze_semantic() {
  local d="$1" sid="$2"; shift 2
  bash "$GC" freeze \
    --session-file "$d/session.json" \
    --source-request-file "$d/request.txt" \
    --intent "evaluate the installed model through existing settings" \
    --success-signal "the existing suite reports a score" \
    --target "spec=specs/041-x" \
    --repository-root bubbles \
    --runner bubbles.goal \
    --session-id "$sid" \
    --repository-alias bubbles \
    ${1+"$@"}
}

# T31 — every execution shape freezes and verifies.
t31_ok="true"
for shape in one-off existing-capability-change reusable-capability; do
  d31="$(new_case "t31-$shape")"
  if ! freeze_semantic "$d31" "sess31" --execution-shape "$shape" >/dev/null 2>&1; then
    t31_ok="false"; fail "T31 freeze rejected execution shape '$shape'"
  elif ! bash "$GC" verify --session-file "$d31/session.json" >/dev/null 2>&1; then
    t31_ok="false"; fail "T31 verify rejected execution shape '$shape'"
  elif [[ "$(jq -r '.goalContract.schemaVersion' "$d31/session.json")" != "goal-contract/v2" ]]; then
    t31_ok="false"; fail "T31 a semantic freeze must write goal-contract/v2 (shape '$shape')"
  fi
done
[[ "$t31_ok" == "true" ]] && pass "T31 every execution shape freezes, verifies, and writes v2"

# T32..T35 — caller errors are exit 2 at the flag, not invalid contracts later.
d32="$(new_case t32)"
expect_rc "T32 an unknown change class is refused at the flag" 2 \
  freeze_semantic "$d32" sess32 --execution-shape one-off --allow-change-class new-quantum-thing
d33="$(new_case t33)"
expect_rc "T33 a negative delta budget is refused" 2 \
  freeze_semantic "$d33" sess33 --execution-shape one-off --delta-budget maxNewFiles=-1
d34="$(new_case t34)"
expect_rc "T34 an unknown execution shape is refused" 2 \
  freeze_semantic "$d34" sess34 --execution-shape platform
d35="$(new_case t35)"
expect_rc "T35 semantic detail with no --execution-shape is refused, not dropped" 2 \
  freeze_semantic "$d35" sess35 --allow-change-class existing-test

# T36 — ADVERSARIAL: a class cannot be both pre-approved and approval-gated.
# Without this check the approval requirement is decorative: a planner could
# read the class off allowedChangeClasses and never reach the approval gate.
d36="$(new_case t36)"
expect_rc "T36 overlapping allowed/approval-required classes are refused" 1 \
  freeze_semantic "$d36" sess36 --execution-shape one-off \
  --allow-change-class new-runner --approval-change-class new-runner

# T37 — ADVERSARIAL: a semanticBoundary smuggled into a v1 contract is refused.
# A v1 reader would ignore the field entirely, so accepting it would create a
# contract whose declared boundary no consumer enforces.
d37="$(new_case t37)"
freeze_default "$d37" >/dev/null 2>&1
jq '.goalContract.semanticBoundary = {executionShape:"one-off",allowedChangeClasses:[],approvalRequiredChangeClasses:[],deltaBudget:{}}' \
  "$d37/session.json" > "$d37/tampered.json" && mv "$d37/tampered.json" "$d37/session.json"
expect_rc "T37 a semanticBoundary inside a v1 contract is refused" 1 \
  bash "$GC" verify --session-file "$d37/session.json"

# T38 — revise carries the boundary forward and names the direction it moved.
d38="$(new_case t38)"
freeze_semantic "$d38" sess38 --execution-shape one-off \
  --allow-change-class existing-test --delta-budget maxNewFiles=2 >/dev/null 2>&1
bash "$GC" revise --session-file "$d38/session.json" --approval-note "reword" --intent "i2" >/dev/null 2>&1
t38_shape="$(jq -r '.goalContract.semanticBoundary.executionShape' "$d38/session.json")"
t38_note="$(jq -r '.goalContract.approval.approvalNote' "$d38/session.json")"
if [[ "$t38_shape" == "one-off" && "$t38_note" == *"semantic-unchanged"* ]]; then
  pass "T38 a revise with no semantic flag carries the boundary forward unchanged"
else
  fail "T38 semantic carry-forward (shape='$t38_shape', note='$t38_note')"
fi

# T38b/T38c — the direction classifier is the audit record of what was approved.
bash "$GC" revise --session-file "$d38/session.json" --approval-note "approved a foundation" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --allow-change-class new-shared-library --delta-budget maxNewFiles=2 >/dev/null 2>&1
t38b_note="$(jq -r '.goalContract.approval.approvalNote' "$d38/session.json")"
if [[ "$t38b_note" == *"semantic-widened"* ]]; then
  pass "T38b promoting the shape and adding a class records semantic-widened"
else
  fail "T38b widening classification (note='$t38b_note')"
fi

bash "$GC" revise --session-file "$d38/session.json" --approval-note "scope back down" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --delta-budget maxNewFiles=2 >/dev/null 2>&1
t38c_note="$(jq -r '.goalContract.approval.approvalNote' "$d38/session.json")"
if [[ "$t38c_note" == *"semantic-narrowed"* ]]; then
  pass "T38c dropping a class records semantic-narrowed"
else
  fail "T38c narrowing classification (note='$t38c_note')"
fi

# T39 — an unapproved semantic revision is still refused (exit 3), so widening
# cannot happen without a recorded operator note.
expect_rc "T39 a semantic revision with no approval note is refused" 3 \
  bash "$GC" revise --session-file "$d38/session.json" --execution-shape one-off

# T40 — a ref minted before an approved semantic revision no longer verifies.
d40="$(new_case t40)"
freeze_semantic "$d40" sess40 --execution-shape one-off --allow-change-class existing-test >/dev/null 2>&1
bash "$GC" ref --session-file "$d40/session.json" > "$d40/ref.json" 2>/dev/null
if [[ "$(jq -r 'has("semanticBoundary")' "$d40/ref.json")" == "true" ]]; then
  pass "T40 the emitted ref carries the semantic boundary"
else
  fail "T40 ref omits semanticBoundary (a consumer could not check it)"
fi
if bash "$GC" verify-ref --session-file "$d40/session.json" --ref-file "$d40/ref.json" >/dev/null 2>&1; then
  pass "T40b a fresh ref verifies"
else
  fail "T40b a fresh ref should verify"
fi
bash "$GC" revise --session-file "$d40/session.json" --approval-note "widen" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --allow-change-class new-shared-library >/dev/null 2>&1
expect_rc "T40c a ref minted before an approved semantic revision is invalidated" 1 \
  bash "$GC" verify-ref --session-file "$d40/session.json" --ref-file "$d40/ref.json"

# T41 — ADVERSARIAL non-vacuity for the whole scope: v1 must be untouched.
# Every case above could pass while v1 silently became v2, which would break
# every already-frozen contract in the field.
d41="$(new_case t41)"
freeze_default "$d41" >/dev/null 2>&1
t41_ver="$(jq -r '.goalContract.schemaVersion' "$d41/session.json")"
t41_has="$(jq -r '.goalContract | has("semanticBoundary")' "$d41/session.json")"
bash "$GC" revise --session-file "$d41/session.json" --approval-note "reword only" --intent "x" >/dev/null 2>&1
t41_note="$(jq -r '.goalContract.approval.approvalNote' "$d41/session.json")"
if [[ "$t41_ver" == "goal-contract/v1" && "$t41_has" == "false" && "$t41_note" == "unchanged: reword only" ]]; then
  pass "T41 a freeze with no semantic flag stays v1 and keeps the exact v1 note format"
else
  fail "T41 v1 compatibility (version='$t41_ver', hasSemantic='$t41_has', note='$t41_note')"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "goal-contract-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "goal-contract-selftest: all cases passed."
