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
PHASE_COORDINATOR="$SCRIPT_DIR/phase-coordinator.sh"
TRANSITION_GUARD="$SCRIPT_DIR/state-transition-guard.sh"
BUG_PACKET_DIR="$SCRIPT_DIR/../../bugs/BUG-042-same-revision-receipt-supersession"
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
[[ -f "$PHASE_COORDINATOR" && -f "$TRANSITION_GUARD" ]] || {
  printf '%s: production resolver consumers are required\n' "$NAME" >&2
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
  # phase exit ts [scenarioId] [testIdentity] [negativeControl] [revision] [schemaVersion]
  local phase="$1" exit_code="$2" ts="$3"
  local sid="${4:-SCN-999-001}"
  local test_id="${5:-tests/e2e/checkout.spec.ts::coupon recomputes total}"
  local control="${6:-drop the coupon multiplier; the asserted total stops changing}"
  local rev="${7:-$REV}"
  local schema_version="${8:-2}"
  printf '{"schemaVersion":%s,"ts":"%s","sessionId":"s-%s","cmd":"npx playwright test checkout","exitCode":%s,"stdoutHash":"%s","scenarioBinding":{"scenarioId":"%s","phase":"%s","testIdentity":"%s","sourceRevision":"%s","negativeControl":"%s","claim":"coupon recomputes the checkout total","implementationRefs":["src/checkout/total.ts"]}}\n' \
    "$schema_version" "$ts" "$phase" "$exit_code" \
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

# BUG-050 SCN-B050-005: RED is historical ordering proof. It may cite the
# pre-implementation source revision, while implement and GREEN remain bound to
# the current candidate. The old RED cannot itself buy GREEN.
historical_red_log="$TMP_DIR/bug050-historical-red-current-green.jsonl"
receipt red 1 "2026-09-02T08:30:00Z" "SCN-999-001" \
  "tests/e2e/checkout.spec.ts::coupon recomputes total" \
  "drop the coupon multiplier; the asserted total stops changing" "$OTHER_REV" > "$historical_red_log"
receipt implement 0 "2026-09-02T08:31:00Z" >> "$historical_red_log"
receipt green 0 "2026-09-02T08:32:00Z" >> "$historical_red_log"
historical_red_out="$(resolve "$life_dir" "$historical_red_log")"
historical_red_rc=$?
historical_red_states="$(json_get "$historical_red_out" '.scenarios[0].derivedStates | join(" ")')"
if [[ "$historical_red_rc" -eq 0 && "$historical_red_states" == "PLANNED RED_VERIFIED IMPLEMENTED GREEN_TARGETED" ]]; then
  pass "SCN-B050-005 historical RED plus current implement/GREEN derives the ordered proof chain"
else
  fail "SCN-B050-005 expected historical RED and current GREEN (exit $historical_red_rc states=[$historical_red_states])"
  printf '  output: %s\n' "$historical_red_out"
fi

stale_green_log="$TMP_DIR/bug050-historical-red-stale-green.jsonl"
receipt red 1 "2026-09-02T08:40:00Z" "SCN-999-001" \
  "tests/e2e/checkout.spec.ts::coupon recomputes total" \
  "drop the coupon multiplier; the asserted total stops changing" "$OTHER_REV" > "$stale_green_log"
receipt implement 0 "2026-09-02T08:41:00Z" >> "$stale_green_log"
receipt green 0 "2026-09-02T08:42:00Z" "SCN-999-001" \
  "tests/e2e/checkout.spec.ts::coupon recomputes total" \
  "drop the coupon multiplier; the asserted total stops changing" "$OTHER_REV" >> "$stale_green_log"
stale_green_out="$(resolve "$life_dir" "$stale_green_log")"
stale_green_rc=$?
stale_green_states="$(json_get "$stale_green_out" '.scenarios[0].derivedStates | join(" ")')"
if [[ "$stale_green_rc" -eq 0 && "$stale_green_states" == "PLANNED RED_VERIFIED IMPLEMENTED" ]] &&
  [[ "$(json_get "$stale_green_out" '[.refusals[] | select(.code == "SCS-REVISION-DRIFT")] | length')" -eq 1 ]]; then
  pass "SCN-B050-005 stale post-fix GREEN stays excluded while historical RED remains valid"
else
  fail "SCN-B050-005 stale GREEN was admitted or historical RED was lost (exit $stale_green_rc states=[$stale_green_states])"
  printf '  output: %s\n' "$stale_green_out"
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

# ---------------------------------------------------------------------------
# BUG-042 SAME-REVISION RECEIPT SUPERSESSION. These cases intentionally use
# physical JSONL order as evidence authority. T01-T03 pair each repair with a
# log that omits the correcting row; T05-T09 prove current conflicts remain
# blocking beside a selected historical chain.
# ---------------------------------------------------------------------------
B042_TEST_A="tests/e2e/checkout.spec.ts::coupon recomputes total"
B042_TEST_B="tests/unit/total.test.ts::adds numbers"
B042_CONTROL_A="drop the coupon multiplier; the asserted total stops changing"
B042_CONTROL_B="delete the whole test file"

write_two_scenario_manifest() {
  local dir="$1"
  mkdir -p "$dir"
  cat > "$dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "spec": "fixture",
  "scenarios": [
    {
      "id": "SCN-B042-A",
      "title": "First isolated scenario",
      "requiredTestType": "functional",
      "behaviorTraits": [],
      "implementationRefs": ["src/a.ts"]
    },
    {
      "id": "SCN-B042-B",
      "title": "Second isolated scenario",
      "requiredTestType": "functional",
      "behaviorTraits": [],
      "implementationRefs": ["src/b.ts"]
    }
  ]
}
EOF
}

write_consumer_fixture() {
  local dir="$1"
  local artifact
  mkdir -p "$dir/.specify/runtime"
  for artifact in bug.md spec.md design.md scopes.md report.md uservalidation.md state.json test-plan.json; do
    cp "$BUG_PACKET_DIR/$artifact" "$dir/$artifact"
  done
  cat > "$dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "spec": "fixture",
  "scenarios": [
    {
      "id": "SCN-999-001",
      "title": "Consumer observes corrected same-revision evidence",
      "requiredTestType": "functional",
      "behaviorTraits": [],
      "implementationRefs": ["src/checkout/total.ts"]
    }
  ]
}
EOF
}

sha256_text() {
  python3 -c 'import hashlib, sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())' <<<"$1"
}

# B042-T01 substituted-test replacement.
t01_log="$TMP_DIR/b042-t01.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t01_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t01_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t01_log"
receipt green 0 "2026-08-18T12:00:00Z" >> "$t01_log"
t01_out="$(resolve "$life_dir" "$t01_log" --require GREEN_TARGETED --certifiable)"
t01_rc=$?
t01_text_out="$(bash "$RESOLVER" --spec-dir "$life_dir" --log "$t01_log" \
  --source-revision "$REV" --registry "$REGISTRY" --format text \
  --require GREEN_TARGETED --certifiable 2>&1)"
t01_text_rc=$?
t01_negative_log="$TMP_DIR/b042-t01-negative.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t01_negative_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t01_negative_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t01_negative_log"
t01_negative_out="$(resolve "$life_dir" "$t01_negative_log" --require GREEN_TARGETED --certifiable)"
t01_negative_rc=$?
if [[ "$t01_rc" -eq 0 ]] &&
  [[ "$(json_get "$t01_out" '.scenarios[0].selectionStatus')" == "SELECTED" ]] &&
  [[ "$(json_get "$t01_out" '.scenarios[0].selectedChain.testIdentity')" == "$B042_TEST_A" ]] &&
  [[ "$(json_get "$t01_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "4" ]] &&
  [[ "$(json_get "$t01_out" '[.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .disposition == "SUPERSEDED" and .blocking == false)] | length')" == "1" ]] &&
  [[ "$t01_negative_rc" -eq 1 ]] &&
  [[ "$(json_get "$t01_negative_out" '[.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T01 substituted-test replacement"
else
  fail "B042-T01 substituted-test replacement (corrected exit $t01_rc, negative exit $t01_negative_rc)"
fi
if [[ "$t01_rc" -eq 0 ]] &&
  [[ "$(json_get "$t01_out" '.scenarios[0].derivedStates | index("IMPLEMENTED") != null')" == "true" ]] &&
  [[ "$(json_get "$t01_out" '.scenarios[0].blockedNotRun | index("IMPLEMENTED")')" == "null" ]]; then
  pass "B042-T01 selected IMPLEMENTED is not also blocked-not-run"
else
  fail "B042-T01 selected IMPLEMENTED is not also blocked-not-run"
fi
if [[ "$t01_text_rc" -eq 0 ]] &&
  printf '%s' "$t01_text_out" | grep -qF 'SUPERSEDED SCS-TEST-SUBSTITUTED' &&
  ! printf '%s' "$t01_text_out" | grep -qF 'refusals are SCS-REVISION-DRIFT'; then
  pass "B042-T01 superseded same-revision text is not labeled drift-only"
else
  fail "B042-T01 superseded same-revision text is not labeled drift-only (exit $t01_text_rc)"
  printf '  output: %s\n' "$t01_text_out"
fi

# B042-T02 substituted-control replacement.
t02_log="$TMP_DIR/b042-t02.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t02_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t02_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_B" >> "$t02_log"
receipt green 0 "2026-08-18T12:00:00Z" >> "$t02_log"
t02_out="$(resolve "$life_dir" "$t02_log" --require GREEN_TARGETED --certifiable)"
t02_rc=$?
t02_negative_log="$TMP_DIR/b042-t02-negative.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t02_negative_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t02_negative_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_B" >> "$t02_negative_log"
t02_negative_out="$(resolve "$life_dir" "$t02_negative_log" --require GREEN_TARGETED --certifiable)"
t02_negative_rc=$?
if [[ "$t02_rc" -eq 0 ]] &&
  [[ "$(json_get "$t02_out" '.scenarios[0].selectedChain.negativeControl')" == "$B042_CONTROL_A" ]] &&
  [[ "$(json_get "$t02_out" '[.refusals[] | select(.code == "SCS-CONTROL-SUBSTITUTED" and .disposition == "SUPERSEDED" and .blocking == false)] | length')" == "1" ]] &&
  [[ "$t02_negative_rc" -eq 1 ]] &&
  [[ "$(json_get "$t02_negative_out" '[.refusals[] | select(.code == "SCS-CONTROL-SUBSTITUTED" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T02 substituted-control replacement"
else
  fail "B042-T02 substituted-control replacement (corrected exit $t02_rc, negative exit $t02_negative_rc)"
fi

# B042-T03 exit-zero RED replacement.
t03_log="$TMP_DIR/b042-t03.jsonl"
receipt red 0 "2026-08-18T09:00:00Z" > "$t03_log"
receipt red 1 "2026-08-18T10:00:00Z" >> "$t03_log"
receipt implement 0 "2026-08-18T11:00:00Z" >> "$t03_log"
receipt green 0 "2026-08-18T12:00:00Z" >> "$t03_log"
t03_out="$(resolve "$life_dir" "$t03_log" --require GREEN_TARGETED --certifiable)"
t03_rc=$?
t03_negative_log="$TMP_DIR/b042-t03-negative.jsonl"
receipt red 0 "2026-08-18T09:00:00Z" > "$t03_negative_log"
receipt implement 0 "2026-08-18T11:00:00Z" >> "$t03_negative_log"
receipt green 0 "2026-08-18T12:00:00Z" >> "$t03_negative_log"
t03_negative_out="$(resolve "$life_dir" "$t03_negative_log" --require GREEN_TARGETED --certifiable)"
t03_negative_rc=$?
if [[ "$t03_rc" -eq 0 ]] &&
  [[ "$(json_get "$t03_out" '.scenarios[0].selectedChain.red.appendOrdinal')" == "2" ]] &&
  [[ "$(json_get "$t03_out" '[.refusals[] | select(.code == "SCS-RED-NOT-FAILING" and .disposition == "SUPERSEDED" and .blocking == false)] | length')" == "1" ]] &&
  [[ "$t03_negative_rc" -eq 1 ]] &&
  [[ "$(json_get "$t03_negative_out" '[.refusals[] | select(.code == "SCS-RED-NOT-FAILING" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T03 exit-zero RED replacement"
else
  fail "B042-T03 exit-zero RED replacement (corrected exit $t03_rc, negative exit $t03_negative_rc)"
fi

# B042-T04 equal timestamps use append ordinal.
t04_log="$TMP_DIR/b042-t04.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t04_log"
receipt implement 0 "2026-08-18T09:00:00Z" >> "$t04_log"
receipt green 0 "2026-08-18T09:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t04_log"
receipt green 0 "2026-08-18T09:00:00Z" >> "$t04_log"
t04_out_one="$(resolve "$life_dir" "$t04_log")"
t04_rc_one=$?
t04_out_two="$(resolve "$life_dir" "$t04_log")"
t04_rc_two=$?
if [[ "$t04_rc_one" -eq 0 && "$t04_rc_two" -eq 0 ]] &&
  [[ "$t04_out_one" == "$t04_out_two" ]] &&
  [[ "$(json_get "$t04_out_one" '.scenarios[0].selectedChain.green.appendOrdinal')" == "4" ]] &&
  [[ "$(json_get "$t04_out_one" '.scenarios[0].selectedChain.chainId | startswith("sha256:")')" == "true" ]]; then
  pass "B042-T04 equal timestamps use append ordinal"
else
  fail "B042-T04 equal timestamps use append ordinal (exits $t04_rc_one/$t04_rc_two)"
fi

# B042-T05 later substituted-test remains unresolved.
t05_log="$TMP_DIR/b042-t05.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t05_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t05_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t05_log"
receipt green 0 "2026-08-18T12:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t05_log"
t05_out="$(resolve "$life_dir" "$t05_log" --require GREEN_TARGETED --certifiable)"
t05_rc=$?
if [[ "$t05_rc" -eq 1 ]] &&
  [[ "$(json_get "$t05_out" '.scenarios[0].selectionStatus')" == "SELECTED_WITH_UNRESOLVED" ]] &&
  [[ "$(json_get "$t05_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "3" ]] &&
  [[ "$(json_get "$t05_out" '[.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T05 later substituted-test remains unresolved"
else
  fail "B042-T05 later substituted-test remains unresolved (exit $t05_rc)"
fi

# B042-T06 later substituted-control remains unresolved.
t06_log="$TMP_DIR/b042-t06.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t06_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t06_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t06_log"
receipt green 0 "2026-08-18T12:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_B" >> "$t06_log"
t06_out="$(resolve "$life_dir" "$t06_log" --require GREEN_TARGETED --certifiable)"
t06_rc=$?
if [[ "$t06_rc" -eq 1 ]] &&
  [[ "$(json_get "$t06_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "3" ]] &&
  [[ "$(json_get "$t06_out" '[.refusals[] | select(.code == "SCS-CONTROL-SUBSTITUTED" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T06 later substituted-control remains unresolved"
else
  fail "B042-T06 later substituted-control remains unresolved (exit $t06_rc)"
fi

# B042-T07 later exit-zero RED remains unresolved.
t07_log="$TMP_DIR/b042-t07.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t07_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t07_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t07_log"
receipt red 0 "2026-08-18T12:00:00Z" >> "$t07_log"
t07_out="$(resolve "$life_dir" "$t07_log" --require GREEN_TARGETED --certifiable)"
t07_rc=$?
if [[ "$t07_rc" -eq 1 ]] &&
  [[ "$(json_get "$t07_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "3" ]] &&
  [[ "$(json_get "$t07_out" '[.refusals[] | select(.code == "SCS-RED-NOT-FAILING" and .disposition == "UNRESOLVED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T07 later exit-zero RED remains unresolved"
else
  fail "B042-T07 later exit-zero RED remains unresolved (exit $t07_rc)"
fi

# B042-T08 later RED-only campaign is partial.
t08_log="$TMP_DIR/b042-t08.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t08_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t08_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t08_log"
receipt red 1 "2026-08-18T12:00:00Z" >> "$t08_log"
t08_out="$(resolve "$life_dir" "$t08_log" --require GREEN_TARGETED --certifiable)"
t08_rc=$?
if [[ "$t08_rc" -eq 1 ]] &&
  [[ "$(json_get "$t08_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "3" ]] &&
  [[ "$(json_get "$t08_out" '[.refusals[] | select(.code == "SCS-CHAIN-PARTIAL" and .disposition == "UNRESOLVED" and .blocking == true and (.detail | contains("IMPLEMENT")))] | length')" == "1" ]]; then
  pass "B042-T08 later RED-only campaign is partial"
else
  fail "B042-T08 later RED-only campaign is partial (exit $t08_rc)"
fi

# B042-T09 later RED-IMPLEMENT campaign is partial.
t09_log="$TMP_DIR/b042-t09.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t09_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t09_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t09_log"
receipt red 1 "2026-08-18T12:00:00Z" >> "$t09_log"
receipt implement 0 "2026-08-18T13:00:00Z" >> "$t09_log"
t09_out="$(resolve "$life_dir" "$t09_log" --require GREEN_TARGETED --certifiable)"
t09_rc=$?
if [[ "$t09_rc" -eq 1 ]] &&
  [[ "$(json_get "$t09_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "3" ]] &&
  [[ "$(json_get "$t09_out" '[.refusals[] | select(.code == "SCS-CHAIN-PARTIAL" and .disposition == "UNRESOLVED" and .blocking == true and (.detail | contains("GREEN")))] | length')" == "1" ]]; then
  pass "B042-T09 later RED-IMPLEMENT campaign is partial"
else
  fail "B042-T09 later RED-IMPLEMENT campaign is partial (exit $t09_rc)"
fi

# B042-T10 still-later coherent chain closes conflict.
t10_log="$TMP_DIR/b042-t10.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t10_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t10_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t10_log"
receipt green 0 "2026-08-18T12:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t10_log"
receipt red 1 "2026-08-18T13:00:00Z" >> "$t10_log"
receipt implement 0 "2026-08-18T14:00:00Z" >> "$t10_log"
receipt green 0 "2026-08-18T15:00:00Z" >> "$t10_log"
t10_out="$(resolve "$life_dir" "$t10_log" --require GREEN_TARGETED --certifiable)"
t10_rc=$?
if [[ "$t10_rc" -eq 0 ]] &&
  [[ "$(json_get "$t10_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "7" ]] &&
  [[ "$(json_get "$t10_out" '[.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .ledgerIdentity.appendOrdinal == 4 and .disposition == "SUPERSEDED" and .blocking == false)] | length')" == "1" ]]; then
  pass "B042-T10 still-later coherent chain closes conflict"
else
  fail "B042-T10 still-later coherent chain closes conflict (exit $t10_rc)"
fi

# B042-T11 duplicate rows retain distinct ordinals.
t11_log="$TMP_DIR/b042-t11.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t11_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t11_log"
t11_duplicate="$(receipt green 0 "2026-08-18T11:00:00Z")"
printf '%s\n%s\n' "$t11_duplicate" "$t11_duplicate" >> "$t11_log"
t11_out="$(resolve "$life_dir" "$t11_log")"
t11_rc=$?
if [[ "$t11_rc" -eq 0 ]] &&
  [[ "$(json_get "$t11_out" '.scenarios[0].selectedChain.green.appendOrdinal')" == "4" ]] &&
  [[ "$(json_get "$t11_out" '[.scenarios[0].receiptDispositions[] | select(.phase == "green") | .ledgerIdentity.rowSha256] | unique | length')" == "1" ]] &&
  [[ "$(json_get "$t11_out" '[.scenarios[0].receiptDispositions[] | select(.phase == "green") | .ledgerIdentity.appendOrdinal] | join(" ")')" == "3 4" ]] &&
  [[ "$(json_get "$t11_out" '[.scenarios[0].receiptDispositions[] | select(.phase == "green") | .disposition] | sort | join(" ")')" == "SELECTED SUPERSEDED" ]]; then
  pass "B042-T11 duplicate rows retain distinct ordinals"
else
  fail "B042-T11 duplicate rows retain distinct ordinals (exit $t11_rc)"
fi

# B042-T12 cross-scenario isolation.
t12_dir="$TMP_DIR/b042-t12"
write_two_scenario_manifest "$t12_dir"
t12_log="$TMP_DIR/b042-t12.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" "SCN-B042-A" "$B042_TEST_A" > "$t12_log"
receipt implement 0 "2026-08-18T10:00:00Z" "SCN-B042-A" "$B042_TEST_A" >> "$t12_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-B042-A" "$B042_TEST_B" >> "$t12_log"
receipt red 1 "2026-08-18T12:00:00Z" "SCN-B042-B" "$B042_TEST_A" >> "$t12_log"
receipt implement 0 "2026-08-18T13:00:00Z" "SCN-B042-B" "$B042_TEST_A" >> "$t12_log"
receipt green 0 "2026-08-18T14:00:00Z" "SCN-B042-B" "$B042_TEST_A" >> "$t12_log"
t12_out="$(resolve "$t12_dir" "$t12_log" --require GREEN_TARGETED --certifiable)"
t12_rc=$?
if [[ "$t12_rc" -eq 1 ]] &&
  [[ "$(json_get "$t12_out" '[.scenarios[] | select(.scenarioId == "SCN-B042-A") | .selectionStatus][0]')" == "NO_COMPLETE_CHAIN" ]] &&
  [[ "$(json_get "$t12_out" '[.scenarios[] | select(.scenarioId == "SCN-B042-B") | .selectionStatus][0]')" == "SELECTED" ]] &&
  [[ "$(json_get "$t12_out" '[.refusals[] | select(.scenarioId == "SCN-B042-A" and .code == "SCS-TEST-SUBSTITUTED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T12 cross-scenario isolation"
else
  fail "B042-T12 cross-scenario isolation (exit $t12_rc)"
fi

# B042-T13 BUG-034 revision isolation.
B042_T13_DRIFT_EXIT=0
B042_T13_REQUIRED_EXIT=1
B042_T13_MIXED_EXIT=1
B042_T13_DRIFT_SHA256="7a505dd15e59cb94eece826408a55e2eabdea33aed8b143a23dea24da33254c3"
B042_T13_REQUIRED_SHA256="4d90f6a135ffe8abd8b9e6724e77a057e5acd095ca716d85b37fe877336d11fb"
B042_T13_MIXED_SHA256="59e94858ceb7b79e2b1ee6f0c41f7c1a81a243ee91a0d4b9775355e365f8bd00"
t13_drift_out="$(resolve "$life_dir" "$d_log")"
t13_drift_rc=$?
t13_drift_repeat="$(resolve "$life_dir" "$d_log")"
t13_drift_repeat_rc=$?
t13_required_out="$(resolve "$life_dir" "$d_log" --require RED_VERIFIED --certifiable)"
t13_required_rc=$?
t13_mixed_out="$(resolve "$life_dir" "$d3_log")"
t13_mixed_rc=$?
t13_drift_bytes="${t13_drift_out//$TMP_DIR/<TMP_DIR>}"
t13_required_bytes="${t13_required_out//$TMP_DIR/<TMP_DIR>}"
t13_mixed_bytes="${t13_mixed_out//$TMP_DIR/<TMP_DIR>}"
t13_drift_hash="$(sha256_text "$t13_drift_bytes")"
t13_required_hash="$(sha256_text "$t13_required_bytes")"
t13_mixed_hash="$(sha256_text "$t13_mixed_bytes")"
t13_checks=0
[[ "$t13_drift_rc" -eq 0 && "$t13_drift_repeat_rc" -eq 0 ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_drift_out" == "$t13_drift_repeat" ]] && t13_checks=$((t13_checks + 1))
[[ "$(json_get "$t13_drift_out" '.refusals[0].code')" == "SCS-REVISION-DRIFT" ]] && t13_checks=$((t13_checks + 1))
[[ "$(json_get "$t13_drift_out" '.blockingRefusalCount')" == "0" ]] && t13_checks=$((t13_checks + 1))
[[ "$(json_get "$t13_drift_out" '.scenarios[0] | has("selectionStatus")')" == "false" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_required_rc" -eq 1 ]] && t13_checks=$((t13_checks + 1))
[[ "$(json_get "$t13_required_out" '.certifiable')" == "false" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_mixed_rc" -eq 1 ]] && t13_checks=$((t13_checks + 1))
[[ "$(json_get "$t13_mixed_out" '[.refusals[] | select(.code == "SCS-NO-NEGATIVE-CONTROL")] | length')" == "1" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_drift_rc" -eq "$B042_T13_DRIFT_EXIT" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_required_rc" -eq "$B042_T13_REQUIRED_EXIT" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_mixed_rc" -eq "$B042_T13_MIXED_EXIT" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_drift_hash" == "$B042_T13_DRIFT_SHA256" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_required_hash" == "$B042_T13_REQUIRED_SHA256" ]] && t13_checks=$((t13_checks + 1))
[[ "$t13_mixed_hash" == "$B042_T13_MIXED_SHA256" ]] && t13_checks=$((t13_checks + 1))
if [[ "$t13_checks" -eq 15 ]]; then
  pass "B042-T13 BUG-034 revision isolation"
  printf 'PROTECTION: B042-T13 drift-output-sha256=%s required-output-sha256=%s mixed-output-sha256=%s\n' \
    "$t13_drift_hash" "$t13_required_hash" "$t13_mixed_hash"
else
  fail "B042-T13 BUG-034 revision isolation ($t13_checks/15 checks; exits $t13_drift_rc/$t13_required_rc/$t13_mixed_rc)"
  printf '  repeatEqual=%s driftCode=%s blockingCount=%s selectionField=%s certifiable=%s noControlCount=%s hashes=%s/%s/%s\n' \
    "$([[ "$t13_drift_out" == "$t13_drift_repeat" ]] && printf true || printf false)" \
    "$(json_get "$t13_drift_out" '.refusals[0].code')" \
    "$(json_get "$t13_drift_out" '.blockingRefusalCount')" \
    "$(json_get "$t13_drift_out" '.scenarios[0] | has("selectionStatus")')" \
    "$(json_get "$t13_required_out" '.certifiable')" \
    "$(json_get "$t13_mixed_out" '[.refusals[] | select(.code == "SCS-NO-NEGATIVE-CONTROL")] | length')" \
    "$t13_drift_hash" "$t13_required_hash" "$t13_mixed_hash"
fi

# B042-T14 proof identity cannot borrow phases.
t14_log="$TMP_DIR/b042-t14.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t14_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t14_log"
receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_B" >> "$t14_log"
t14_out="$(resolve "$life_dir" "$t14_log" --require GREEN_TARGETED --certifiable)"
t14_rc=$?
if [[ "$t14_rc" -eq 1 ]] &&
  [[ "$(json_get "$t14_out" '.scenarios[0].selectionStatus')" == "NO_COMPLETE_CHAIN" ]] &&
  [[ "$(json_get "$t14_out" '.scenarios[0].selectedChain')" == "null" ]] &&
  [[ "$(json_get "$t14_out" '[.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T14 proof identity cannot borrow phases"
else
  fail "B042-T14 proof identity cannot borrow phases (exit $t14_rc)"
fi

# B042-T15 duplicate model ordinal refuses order. The copied resolver is
# mutated only inside the disposable fixture; the repository source is never
# changed by this case.
t15_log="$TMP_DIR/b042-t15.jsonl"
receipt red 1 "2026-08-18T09:00:00Z" > "$t15_log"
receipt implement 0 "2026-08-18T10:00:00Z" >> "$t15_log"
receipt green 0 "2026-08-18T11:00:00Z" >> "$t15_log"
receipt red 1 "2026-08-18T12:00:00Z" >> "$t15_log"
receipt implement 0 "2026-08-18T13:00:00Z" >> "$t15_log"
receipt green 0 "2026-08-18T14:00:00Z" >> "$t15_log"
t15_resolver="$TMP_DIR/b042-t15-resolver.sh"
cp "$RESOLVER" "$t15_resolver"
B042_MUTATED_RESOLVER="$t15_resolver" python3 - <<'PY'
import os
from pathlib import Path

path = Path(os.environ['B042_MUTATED_RESOLVER'])
text = path.read_text(encoding='utf-8')
needle = 'append_ordinal = physical_ordinal'
replacement = 'append_ordinal = 3 if physical_ordinal == 6 else physical_ordinal'
if text.count(needle) != 1:
    raise SystemExit(3)
path.write_text(text.replace(needle, replacement), encoding='utf-8')
PY
t15_mutation_rc=$?
t15_out=""
t15_rc=2
if [[ "$t15_mutation_rc" -eq 0 ]]; then
  t15_out="$(bash "$t15_resolver" --spec-dir "$life_dir" --log "$t15_log" \
    --source-revision "$REV" --registry "$REGISTRY" --format json 2>&1)"
  t15_rc=$?
fi
if [[ "$t15_mutation_rc" -eq 0 && "$t15_rc" -eq 1 ]] &&
  [[ "$(json_get "$t15_out" '.scenarios[0].selectionStatus')" == "ORDER_REFUSED" ]] &&
  [[ "$(json_get "$t15_out" '[.refusals[] | select(.code == "SCS-APPEND-ORDER-CONFLICT" and .blocking == true)] | length')" == "1" ]]; then
  pass "B042-T15 duplicate model ordinal refuses order"
else
  fail "B042-T15 duplicate model ordinal refuses order (mutation exit $t15_mutation_rc, resolver exit $t15_rc)"
fi

# B042-T16 legacy receipts derive local identities.
t16_ok=0
t16_projection=""
for schema_version in 1 2 3; do
  t16_log="$TMP_DIR/b042-t16-v${schema_version}.jsonl"
  receipt red 1 "2026-08-18T09:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_A" "$REV" "$schema_version" > "$t16_log"
  receipt implement 0 "2026-08-18T10:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_A" "$REV" "$schema_version" >> "$t16_log"
  receipt green 0 "2026-08-18T11:00:00Z" "SCN-999-001" "$B042_TEST_A" "$B042_CONTROL_A" "$REV" "$schema_version" >> "$t16_log"
  t16_out="$(resolve "$life_dir" "$t16_log" --require GREEN_TARGETED --certifiable)"
  t16_rc=$?
  projection="$(json_get "$t16_out" '[.scenarios[0].selectionStatus, .scenarios[0].selectedChain.red.appendOrdinal, .scenarios[0].selectedChain.implement.appendOrdinal, .scenarios[0].selectedChain.green.appendOrdinal, .scenarios[0].selectedChain.testIdentity] | join("|")')"
  if [[ "$t16_rc" -eq 0 && "$projection" == "SELECTED|1|2|3|$B042_TEST_A" ]] &&
    [[ "$(json_get "$t16_out" '[.scenarios[0].receiptDispositions[].ledgerIdentity.rowSha256 | startswith("sha256:")] | all')" == "true" ]]; then
    t16_ok=$((t16_ok + 1))
    t16_projection="$projection"
  else
    printf '  B042-T16 schemaVersion=%s exit=%s projection=%s digestCheck=%s\n' \
      "$schema_version" "$t16_rc" "$projection" \
      "$(json_get "$t16_out" '[.scenarios[0].receiptDispositions[].ledgerIdentity.rowSha256 | startswith("sha256:")] | all')"
  fi
done
if [[ "$t16_ok" -eq 3 && "$t16_projection" == "SELECTED|1|2|3|$B042_TEST_A" ]]; then
  pass "B042-T16 legacy receipts derive local identities"
else
  fail "B042-T16 legacy receipts derive local identities ($t16_ok/3 envelopes passed)"
fi

# B042-T17 bypass arguments remain rejected.
t17_rejected=0
for flag in --skip-red --force --ignore-drift --assume-green --allow-declared; do
  t17_out="$(bash "$RESOLVER" --spec-dir "$life_dir" "$flag" 2>&1)"
  t17_rc=$?
  if [[ "$t17_rc" -eq 2 ]] && printf '%s' "$t17_out" | grep -qF 'does not exist'; then
    t17_rejected=$((t17_rejected + 1))
  fi
done
if [[ "$t17_rejected" -eq 5 ]]; then
  pass "B042-T17 bypass arguments remain rejected"
else
  fail "B042-T17 bypass arguments remain rejected ($t17_rejected/5 rejected)"
fi

# B042-T18 rollback preserves current ledger identities.
t18_source_before="$(cksum "$life_log")"
t18_out="$(BUBBLES_SCENARIO_STATE_ROLLBACK=1 bash "$RESOLVER" --spec-dir "$life_dir" \
  --log "$life_log" --source-revision "$REV" --registry "$REGISTRY" --format json 2>&1)"
t18_rc=$?
t18_source_after="$(cksum "$life_log")"
if [[ "$t18_rc" -eq 0 ]] &&
  [[ "$t18_source_before" == "$t18_source_after" ]] &&
  [[ "$(json_get "$t18_out" '.rollback')" == "true" ]] &&
  [[ "$(json_get "$t18_out" '.scenarios[0].highestState')" == "PLANNED" ]] &&
  [[ "$(json_get "$t18_out" '.scenarios[0].receiptCount')" == "6" ]] &&
  [[ "$(json_get "$t18_out" '.scenarios[0].receiptDispositions | length')" == "6" ]] &&
  [[ "$(json_get "$t18_out" '[.scenarios[0].receiptDispositions[].ledgerIdentity.appendOrdinal] | join(" ")')" == "1 2 3 4 5 6" ]] &&
  [[ "$(json_get "$t18_out" '[.scenarios[0].receiptDispositions[].ledgerIdentity.rowSha256 | startswith("sha256:")] | all')" == "true" ]]; then
  pass "B042-T18 rollback preserves current ledger identities"
else
  fail "B042-T18 rollback preserves current ledger identities (exit $t18_rc)"
  printf '  output: %s\n' "$t18_out"
fi

# Production consumer canaries. Both consumers execute from their repository
# paths; only fixture artifacts and cursor data live under the temporary root.
consumer_source_before="$(cksum "$PHASE_COORDINATOR" "$TRANSITION_GUARD")"

consumer_success_dir="$TMP_DIR/b042-consumer-success"
write_consumer_fixture "$consumer_success_dir"
cp "$t01_log" "$consumer_success_dir/.specify/runtime/tool-calls.jsonl"
receipt regression 0 "2026-08-18T13:00:00Z" >> "$consumer_success_dir/.specify/runtime/tool-calls.jsonl"
coordinator_success_out="$(bash "$PHASE_COORDINATOR" --spec-dir "$consumer_success_dir" \
  --cursor "$consumer_success_dir/.phase-cursor.json" \
  --independent "consumer-probe=/usr/bin/true" --max-iterations 1 --format json 2>&1)"
coordinator_success_rc=$?
if [[ "$coordinator_success_rc" -eq 0 ]] &&
  [[ "$(json_get "$coordinator_success_out" '.complete')" == "true" ]] &&
  [[ "$(json_get "$coordinator_success_out" '.scenarioStates.scenarios[0].selectedChain.green.appendOrdinal')" == "4" ]] &&
  [[ "$(json_get "$coordinator_success_out" '[.scenarioStates.refusals[] | select(.code == "SCS-TEST-SUBSTITUTED" and .disposition == "SUPERSEDED" and .blocking == false)] | length')" == "1" ]]; then
  pass "consumer: phase coordinator preserves the corrected additive scenario-state payload"
else
  fail "consumer: phase coordinator rejected the corrected additive payload (exit $coordinator_success_rc)"
fi

transition_success_out="$(bash "$TRANSITION_GUARD" "$consumer_success_dir" \
  --target-status "done" --expect-workflow-mode "bugfix-fastlane" 2>&1)"
transition_success_rc=$?
if printf '%s' "$transition_success_out" | grep -qF 'Every required scenario state is receipt-derived for every applicable scenario' &&
  ! printf '%s' "$transition_success_out" | grep -qF 'Required scenario states are NOT receipt-derived'; then
  pass "consumer: transition guard accepts corrected same-revision scenario evidence at Check 4"
else
  fail "consumer: transition guard did not accept corrected scenario evidence at Check 4 (exit $transition_success_rc)"
fi

consumer_unresolved_dir="$TMP_DIR/b042-consumer-unresolved"
write_consumer_fixture "$consumer_unresolved_dir"
cp "$t05_log" "$consumer_unresolved_dir/.specify/runtime/tool-calls.jsonl"
receipt regression 0 "2026-08-18T13:00:00Z" >> "$consumer_unresolved_dir/.specify/runtime/tool-calls.jsonl"
coordinator_unresolved_out="$(bash "$PHASE_COORDINATOR" --spec-dir "$consumer_unresolved_dir" \
  --cursor "$consumer_unresolved_dir/.phase-cursor.json" \
  --independent "consumer-probe=/usr/bin/true" --max-iterations 1 --format json 2>&1)"
coordinator_unresolved_rc=$?
if [[ "$coordinator_unresolved_rc" -eq 0 ]] &&
  [[ "$(json_get "$coordinator_unresolved_out" '.complete')" == "true" ]] &&
  [[ "$(json_get "$coordinator_unresolved_out" '.scenarioStates')" == "null" ]]; then
  pass "consumer: phase coordinator preserves its null boundary for unresolved resolver output"
else
  fail "consumer: phase coordinator changed its unresolved-output boundary (exit $coordinator_unresolved_rc)"
fi

transition_unresolved_out="$(bash "$TRANSITION_GUARD" "$consumer_unresolved_dir" \
  --target-status "done" --expect-workflow-mode "bugfix-fastlane" 2>&1)"
transition_unresolved_rc=$?
if [[ "$transition_unresolved_rc" -ne 0 ]] &&
  printf '%s' "$transition_unresolved_out" | grep -qF 'Required scenario states are NOT receipt-derived' &&
  printf '%s' "$transition_unresolved_out" | grep -qF 'REFUSED SCS-TEST-SUBSTITUTED'; then
  pass "consumer: transition guard relays and blocks the current unresolved receipt"
else
  fail "consumer: transition guard did not relay the current unresolved receipt (exit $transition_unresolved_rc)"
fi

consumer_source_after="$(cksum "$PHASE_COORDINATOR" "$TRANSITION_GUARD")"
if [[ "$consumer_source_before" == "$consumer_source_after" ]]; then
  pass "consumer: production coordinator and transition guard source bytes remain unchanged"
else
  fail "consumer: a production consumer changed during hermetic execution"
fi

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
