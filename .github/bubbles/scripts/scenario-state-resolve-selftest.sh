#!/usr/bin/env bash
# scenario-state-resolve-selftest.sh — IMP-047 S-C.
#
# Covers the two things the outcome engine rests on:
#   1. One scenario fixture moves through EVERY applicable state, with every
#      state computed from receipts and receipt identity stable across runs.
#   2. Every adversarial substitution is REFUSED by name.
#
# Hermetic: every fixture is built under mktemp and removed on exit. Nothing
# reads or writes the repository's own runtime log.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the resolver or a dependency is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/scenario-state-resolve.sh"
REGISTRY="$SCRIPT_DIR/../registry/scenario-states.yaml"
NAME="scenario-state-resolve-selftest"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

[[ -f "$RESOLVER" ]] || {
  printf '%s: resolver not found: %s\n' "$NAME" "$RESOLVER" >&2
  exit 2
}
[[ -f "$REGISTRY" ]] || {
  printf '%s: registry not found: %s\n' "$NAME" "$REGISTRY" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-scenario-state.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

REV="$(printf '%040d' 1)"
OTHER_REV="$(printf '%040d' 2)"

# A manifest with one UI scenario (so GREEN_LIVE applies) that is also
# SLA-sensitive (so OBSERVED applies). Every applicable state is therefore in
# play for this single fixture.
write_manifest() {
  local dir="$1"
  local extra="${2:-}"
  mkdir -p "$dir"
  cat > "$dir/scenario-manifest.json" <<EOF
{
  "schemaVersion": 1,
  "spec": "fixture",
  "scenarios": [
    {
      "id": "SCN-999-001",
      "title": "Checkout total is recomputed after a coupon is applied",
      "requiredTestType": "e2e-ui",
      "behaviorTraits": ["user-visible-ui", "sla-sensitive"],
      "implementationRefs": ["src/checkout/total.ts"]${extra}
    }
  ]
}
EOF
}

receipt() {
  # phase exit ts [scenarioId] [testIdentity] [negativeControl] [revision]
  local phase="$1" exit_code="$2" ts="$3"
  local sid="${4:-SCN-999-001}"
  local test_id="${5:-tests/e2e/checkout.spec.ts::coupon recomputes total}"
  local control="${6:-drop the coupon multiplier; the asserted total stops changing}"
  local rev="${7:-$REV}"
  printf '{"schemaVersion":2,"ts":"%s","sessionId":"s-%s","cmd":"npx playwright test checkout","exitCode":%s,"stdoutHash":"%s","scenarioBinding":{"scenarioId":"%s","phase":"%s","testIdentity":"%s","sourceRevision":"%s","negativeControl":"%s","claim":"coupon recomputes the checkout total","implementationRefs":["src/checkout/total.ts"]}}\n' \
    "$ts" "$phase" "$exit_code" \
    "9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43" \
    "$sid" "$phase" "$test_id" "$rev" "$control"
}

resolve() {
  # dir logfile [extra args...]
  local dir="$1" log="$2"
  shift 2
  bash "$RESOLVER" --spec-dir "$dir" --log "$log" --source-revision "$REV" \
    --registry "$REGISTRY" --format json "$@" 2>&1
}

json_get() { printf '%s' "$1" | jq -r "$2" 2>/dev/null || printf 'ERR'; }

command -v jq >/dev/null 2>&1 || {
  printf '%s: jq is required\n' "$NAME" >&2
  exit 2
}

# ---------------------------------------------------------------------------
# LIFECYCLE. One fixture walks every applicable state, one receipt at a time,
# and the state is READ BACK from the resolver after each append. Nothing in the
# manifest ever changes.
# ---------------------------------------------------------------------------
life_dir="$TMP_DIR/lifecycle"
life_log="$TMP_DIR/lifecycle.jsonl"
write_manifest "$life_dir"
: > "$life_log"

out="$(resolve "$life_dir" "$life_log")"
if [[ "$(json_get "$out" '.scenarios[0].highestState')" == "PLANNED" ]]; then
  pass "lifecycle: a manifest with no receipts derives PLANNED and nothing further"
else
  fail "lifecycle: expected PLANNED with no receipts, observed $(json_get "$out" '.scenarios[0].highestState')"
fi
if [[ "$(json_get "$out" '.scenarios[0].applicableStates | index("GREEN_LIVE") != null')" == "true" &&
  "$(json_get "$out" '.scenarios[0].applicableStates | index("OBSERVED") != null')" == "true" ]]; then
  pass "lifecycle: traits make GREEN_LIVE and OBSERVED applicable for this fixture"
else
  fail "lifecycle: trait-derived applicability did not include GREEN_LIVE and OBSERVED"
fi

# An implement receipt BEFORE any red must not advance IMPLEMENTED.
receipt implement 0 "2026-08-17T10:00:00Z" >> "$life_log"
out="$(resolve "$life_dir" "$life_log")"
if [[ "$(json_get "$out" '.scenarios[0].highestState')" == "PLANNED" &&
  "$(json_get "$out" '.scenarios[0].blockedNotRun | index("IMPLEMENTED") != null')" == "true" ]]; then
  pass "lifecycle: implementation cannot start without an expected behavioral RED"
else
  fail "lifecycle: an implement receipt with no red advanced the scenario to $(json_get "$out" '.scenarios[0].highestState')"
fi

: > "$life_log"
receipt red 1 "2026-08-17T09:00:00Z" >> "$life_log"
out="$(resolve "$life_dir" "$life_log")"
if [[ "$(json_get "$out" '.scenarios[0].highestState')" == "RED_VERIFIED" ]]; then
  pass "lifecycle: a failing red receipt derives RED_VERIFIED"
else
  fail "lifecycle: expected RED_VERIFIED, observed $(json_get "$out" '.scenarios[0].highestState')"
fi

receipt implement 0 "2026-08-17T10:00:00Z" >> "$life_log"
out="$(resolve "$life_dir" "$life_log")"
if [[ "$(json_get "$out" '.scenarios[0].highestState')" == "IMPLEMENTED" ]]; then
  pass "lifecycle: an implement receipt after red derives IMPLEMENTED"
else
  fail "lifecycle: expected IMPLEMENTED, observed $(json_get "$out" '.scenarios[0].highestState')"
fi

receipt green 0 "2026-08-17T11:00:00Z" >> "$life_log"
out="$(resolve "$life_dir" "$life_log")"
if [[ "$(json_get "$out" '.scenarios[0].highestState')" == "GREEN_TARGETED" ]]; then
  pass "lifecycle: a same-scenario same-control green derives GREEN_TARGETED"
else
  fail "lifecycle: expected GREEN_TARGETED, observed $(json_get "$out" '.scenarios[0].highestState')"
fi

receipt live 0 "2026-08-17T12:00:00Z" >> "$life_log"
receipt regression 0 "2026-08-17T13:00:00Z" >> "$life_log"
receipt observed 0 "2026-08-17T14:00:00Z" >> "$life_log"
out="$(resolve "$life_dir" "$life_log")"
derived="$(json_get "$out" '.scenarios[0].derivedStates | join(" ")')"
if [[ "$derived" == "PLANNED RED_VERIFIED IMPLEMENTED GREEN_TARGETED GREEN_LIVE REGRESSION_GREEN OBSERVED" ]]; then
  pass "lifecycle: one fixture reached EVERY applicable state, each computed from a receipt"
else
  fail "lifecycle: expected all seven applicable states, observed [$derived]"
fi
if [[ "$(json_get "$out" '.scenarios[0].derivedStates | index("CERTIFIED")')" == "null" ]]; then
  pass "lifecycle: the resolver never emits CERTIFIED — certification stays validate-owned"
else
  fail "lifecycle: the resolver emitted CERTIFIED, creating a second certifying authority"
fi

# Receipt identity is stable: resolving twice over an unchanged log yields an
# identical payload. An unstable derivation could not be audited.
out2="$(resolve "$life_dir" "$life_log")"
if [[ "$out" == "$out2" ]]; then
  pass "lifecycle: repeated resolution over an unchanged log is byte-identical (stable, auditable)"
else
  fail "lifecycle: resolution is not stable across runs"
fi

# ---------------------------------------------------------------------------
# THE RULE: a hand-written state is REFUSED.
# ---------------------------------------------------------------------------
for key in state scenarioState derivedState currentState certified; do
  declared_dir="$TMP_DIR/declared-$key"
  write_manifest "$declared_dir" ",
      \"$key\": \"GREEN_TARGETED\""
  out="$(resolve "$declared_dir" "$life_log")"
  rc=$?
  if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -qF 'SCS-DECLARED-STATE'; then
    pass "declared state: a hand-written \`$key\` is refused with SCS-DECLARED-STATE"
  else
    fail "declared state: a hand-written \`$key\` was accepted (exit $rc)"
  fi
done

# The mirror: `lockdown.state` is a pre-existing approval flag with its own
# semantics, not a position in this progression. It must NOT be refused.
lockdown_dir="$TMP_DIR/lockdown"
write_manifest "$lockdown_dir" ',
      "lockdown": { "state": "locked" }'
out="$(resolve "$lockdown_dir" "$life_log")"
if ! printf '%s' "$out" | grep -qF 'SCS-DECLARED-STATE'; then
  pass "declared state: \`lockdown.state\` is not mistaken for a scenario state"
else
  fail "declared state: \`lockdown.state\` was wrongly refused as a hand-written scenario state"
fi

# ---------------------------------------------------------------------------
# ADVERSARIAL SUBSTITUTIONS. Each must be refused by its own code.
# ---------------------------------------------------------------------------
adv() {
  # label code build-log-fn
  local label="$1" code="$2" log="$3"
  local adv_out adv_rc
  adv_out="$(resolve "$life_dir" "$log")"
  adv_rc=$?
  if [[ "$adv_rc" -ne 0 ]] && printf '%s' "$adv_out" | grep -qF "$code"; then
    pass "$label refused with $code"
  else
    fail "$label was NOT refused with $code (exit $adv_rc)"
    printf '  output: %s\n' "$adv_out"
  fi
}

x_log="$TMP_DIR/cross-scenario.jsonl"
receipt red 1 "2026-08-17T09:00:00Z" > "$x_log"
receipt implement 0 "2026-08-17T10:00:00Z" >> "$x_log"
receipt green 0 "2026-08-17T11:00:00Z" "SCN-999-002" >> "$x_log"
adv "cross-scenario green" "SCS-CROSS-SCENARIO" "$x_log"

t_log="$TMP_DIR/test-substituted.jsonl"
receipt red 1 "2026-08-17T09:00:00Z" > "$t_log"
receipt implement 0 "2026-08-17T10:00:00Z" >> "$t_log"
receipt green 0 "2026-08-17T11:00:00Z" "SCN-999-001" "tests/unit/total.test.ts::adds numbers" >> "$t_log"
adv "green from a DIFFERENT test" "SCS-TEST-SUBSTITUTED" "$t_log"

c_log="$TMP_DIR/control-substituted.jsonl"
receipt red 1 "2026-08-17T09:00:00Z" > "$c_log"
receipt implement 0 "2026-08-17T10:00:00Z" >> "$c_log"
receipt green 0 "2026-08-17T11:00:00Z" "SCN-999-001" \
  "tests/e2e/checkout.spec.ts::coupon recomputes total" "delete the whole test file" >> "$c_log"
adv "green from a DIFFERENT negative control" "SCS-CONTROL-SUBSTITUTED" "$c_log"

d_log="$TMP_DIR/revision-drift.jsonl"
receipt red 1 "2026-08-17T09:00:00Z" "SCN-999-001" \
  "tests/e2e/checkout.spec.ts::coupon recomputes total" \
  "drop the coupon multiplier; the asserted total stops changing" "$OTHER_REV" > "$d_log"

# Drift is REPORTED and the receipt is excluded, but it does not block on its own.
# The receipt log is append-only, so a superseded receipt outlives every commit;
# treating drift as fatal made a spec permanently uncertifiable once it had
# recorded receipts and then committed anything at all.
d_out="$(resolve "$life_dir" "$d_log")"
d_rc=$?
if [[ "$d_rc" -eq 0 ]] &&
  [[ "$(json_get "$d_out" '.refusals[0].code')" == "SCS-REVISION-DRIFT" ]] &&
  [[ "$(json_get "$d_out" '.blockingRefusalCount')" == "0" ]]; then
  pass "source-revision drift is reported but does not block"
else
  fail "source-revision drift should report and not block (exit $d_rc)"
  printf '  output: %s\n' "$d_out"
fi

# Load-bearing half: exclusion still denies certification. A scenario whose only
# evidence is stale reaches no state, so the required state is unsatisfied. If
# this stops failing, drift has been made cosmetic rather than excluding.
d2_out="$(resolve "$life_dir" "$d_log" --require RED_VERIFIED --certifiable)"
d2_rc=$?
if [[ "$d2_rc" -ne 0 ]] && [[ "$(json_get "$d2_out" '.certifiable')" == "false" ]]; then
  pass "drift-only evidence still fails certification via unsatisfied"
else
  fail "drift-only evidence MUST NOT certify (exit $d2_rc)"
  printf '  output: %s\n' "$d2_out"
fi

# A genuine refusal alongside drift must still block, so the exemption is scoped
# to SCS-REVISION-DRIFT and did not neutralise the other codes.
d3_log="$TMP_DIR/drift-plus-blocking.jsonl"
cat "$d_log" > "$d3_log"
printf '{"schemaVersion":2,"ts":"2026-08-17T09:30:00Z","sessionId":"s-nc2","cmd":"npx playwright test checkout","exitCode":1,"scenarioBinding":{"scenarioId":"SCN-999-001","phase":"red","testIdentity":"tests/e2e/checkout.spec.ts::coupon recomputes total","sourceRevision":"%s","claim":"coupon recomputes the checkout total"}}\n' "$REV" >> "$d3_log"
adv "a blocking refusal alongside drift" "SCS-NO-NEGATIVE-CONTROL" "$d3_log"

n_log="$TMP_DIR/no-control.jsonl"
printf '{"schemaVersion":2,"ts":"2026-08-17T09:00:00Z","sessionId":"s-nc","cmd":"npx playwright test checkout","exitCode":1,"scenarioBinding":{"scenarioId":"SCN-999-001","phase":"red","testIdentity":"tests/e2e/checkout.spec.ts::coupon recomputes total","sourceRevision":"%s","claim":"coupon recomputes the checkout total"}}\n' "$REV" > "$n_log"
adv "receipt with no negative control" "SCS-NO-NEGATIVE-CONTROL" "$n_log"

g_log="$TMP_DIR/green-without-red.jsonl"
receipt green 0 "2026-08-17T11:00:00Z" > "$g_log"
adv "green with no prior red" "SCS-GREEN-WITHOUT-RED" "$g_log"

r_log="$TMP_DIR/red-passing.jsonl"
receipt red 0 "2026-08-17T09:00:00Z" > "$r_log"
adv "a red-phase receipt that exited 0" "SCS-RED-NOT-FAILING" "$r_log"

# A CHANGED implementation ref marks the scenario AFFECTED and its green stale.
impl_out="$(bash "$RESOLVER" --spec-dir "$life_dir" --log "$life_log" \
  --source-revision "$REV" --registry "$REGISTRY" --format json \
  --changed-file "src/checkout/total.ts" 2>&1)"
impl_rc=$?
if [[ "$impl_rc" -ne 0 ]] && printf '%s' "$impl_out" | grep -qF 'SCS-IMPL-REF-CHANGED'; then
  pass "a changed implementation ref marks the scenario AFFECTED"
else
  fail "a changed implementation ref did not mark the scenario AFFECTED (exit $impl_rc)"
fi
if [[ "$(json_get "$impl_out" '.scenarios[0].blockedNotRun | index("GREEN_TARGETED") != null')" == "true" ]]; then
  pass "an affected scenario's GREEN is demoted rather than silently retained"
else
  fail "an affected scenario kept its GREEN state"
fi

# An UNRELATED changed file must not mark anything affected. A rule that marks
# everything is the same as a rule that marks nothing.
unrelated_out="$(bash "$RESOLVER" --spec-dir "$life_dir" --log "$life_log" \
  --source-revision "$REV" --registry "$REGISTRY" --format json \
  --changed-file "docs/README.md" 2>&1)"
unrelated_rc=$?
if [[ "$unrelated_rc" -eq 0 ]] && ! printf '%s' "$unrelated_out" | grep -qF 'SCS-IMPL-REF-CHANGED'; then
  pass "an unrelated changed file does not mark the scenario affected"
else
  fail "an unrelated changed file wrongly marked the scenario affected (exit $unrelated_rc)"
fi

# ---------------------------------------------------------------------------
# GATES NEVER ADVANCE A STATE. A passing gate appended to the log with no
# scenarioBinding must leave every derived state exactly where it was.
# ---------------------------------------------------------------------------
gate_log="$TMP_DIR/gate-pass.jsonl"
receipt red 1 "2026-08-17T09:00:00Z" > "$gate_log"
before="$(json_get "$(resolve "$life_dir" "$gate_log")" '.scenarios[0].derivedStates | join(" ")')"
printf '{"schemaVersion":2,"ts":"2026-08-17T09:30:00Z","sessionId":"s-gate","cmd":"bash bubbles/scripts/artifact-lint.sh specs/fixture","exitCode":0,"tags":["validate"]}\n' >> "$gate_log"
after="$(json_get "$(resolve "$life_dir" "$gate_log")" '.scenarios[0].derivedStates | join(" ")')"
if [[ "$before" == "$after" && "$after" == "PLANNED RED_VERIFIED" ]]; then
  pass "a passing gate receipt does not advance any scenario state"
else
  fail "a passing gate changed the derived states from [$before] to [$after]"
fi

# ---------------------------------------------------------------------------
# CERTIFIABILITY is required-state driven, never checkbox driven.
# ---------------------------------------------------------------------------
cert_out="$(resolve "$life_dir" "$gate_log" --require GREEN_TARGETED --certifiable)"
cert_rc=$?
if [[ "$cert_rc" -eq 1 ]] && [[ "$(json_get "$cert_out" '.certifiable')" == "false" ]]; then
  pass "certifiability is refused while a required scenario state does not hold"
else
  fail "certifiability was granted with GREEN_TARGETED missing (exit $cert_rc)"
fi
cert_ok="$(resolve "$life_dir" "$life_log" --require GREEN_TARGETED --require REGRESSION_GREEN --certifiable)"
cert_ok_rc=$?
if [[ "$cert_ok_rc" -eq 0 ]] && [[ "$(json_get "$cert_ok" '.certifiable')" == "true" ]]; then
  pass "certifiability holds when every required scenario state is receipt-derived"
else
  fail "certifiability was refused for a fully receipt-backed scenario (exit $cert_ok_rc)"
fi

# ---------------------------------------------------------------------------
# MIGRATION AND ROLLBACK.
# ---------------------------------------------------------------------------
legacy_dir="$TMP_DIR/legacy-no-manifest"
mkdir -p "$legacy_dir"
legacy_out="$(bash "$RESOLVER" --spec-dir "$legacy_dir" --log "$life_log" \
  --source-revision "$REV" --registry "$REGISTRY" --format json 2>&1)"
legacy_rc=$?
if [[ "$legacy_rc" -eq 0 ]] && [[ "$(json_get "$legacy_out" '.manifestPresent')" == "false" ]]; then
  pass "migration: a spec with no manifest resolves cleanly and keeps the legacy basis"
else
  fail "migration: a manifest-less spec did not degrade cleanly (exit $legacy_rc)"
fi

boxes_dir="$TMP_DIR/checked-boxes"
write_manifest "$boxes_dir"
cat > "$boxes_dir/scopes.md" <<'EOF'
## Scope 1
- [x] Everything is done
- [x] All tests pass
EOF
boxes_out="$(resolve "$boxes_dir" "$TMP_DIR/empty.jsonl")"
if [[ "$(json_get "$boxes_out" '.scenarios[0].highestState')" == "PLANNED" ]]; then
  pass "migration: checked boxes never infer a later state"
else
  fail "migration: checked boxes inferred $(json_get "$boxes_out" '.scenarios[0].highestState')"
fi

rollback_out="$(BUBBLES_SCENARIO_STATE_ROLLBACK=1 bash "$RESOLVER" --spec-dir "$life_dir" \
  --log "$life_log" --source-revision "$REV" --registry "$REGISTRY" --format json 2>&1)"
rollback_rc=$?
if [[ "$rollback_rc" -eq 0 ]] &&
  [[ "$(json_get "$rollback_out" '.rollback')" == "true" ]] &&
  [[ "$(json_get "$rollback_out" '.scenarios[0].highestState')" == "PLANNED" ]] &&
  [[ "$(json_get "$rollback_out" '.scenarios[0].receiptCount')" == "6" ]]; then
  pass "rollback: advancement stops, and all 6 receipts are preserved and still counted"
else
  fail "rollback did not stop advancement while preserving receipts (exit $rollback_rc)"
  printf '  output: %s\n' "$rollback_out"
fi

# ---------------------------------------------------------------------------
# NO BYPASS.
# ---------------------------------------------------------------------------
for flag in --skip-red --force --ignore-drift --assume-green --allow-declared; do
  bypass_out="$(bash "$RESOLVER" --spec-dir "$life_dir" "$flag" 2>&1)"
  if [[ $? -eq 2 ]] && printf '%s' "$bypass_out" | grep -qF 'does not exist'; then
    pass "no bypass: \`$flag\` is rejected by name"
  else
    fail "no bypass: \`$flag\` was not rejected"
  fi
done

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
