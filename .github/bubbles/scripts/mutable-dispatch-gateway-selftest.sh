#!/usr/bin/env bash
# mutable-dispatch-gateway-selftest.sh — hermetic selftest for
# mutable-dispatch-gateway.sh (IMP-056 SCOPE-4).
#
# Drives the REAL chain end to end: a real MBE store with a genuinely issued
# permit, a real repository packet and control file, a real frozen goal
# contract with a real pre-dispatch G134 receipt, and a real dispatchAdmission
# config -- then calls the gateway and inspects what actually happened,
# mirroring the discipline mutable-dispatch-authorization-selftest.sh already
# established: real fixtures over stubs, adversarial cases that break exactly
# one invariant at a time.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY="$SCRIPT_DIR/mutable-dispatch-gateway.sh"
REPO_BINDING="$SCRIPT_DIR/repository-binding.sh"
BOUNDARY_RECEIPT="$SCRIPT_DIR/goal-boundary-receipt.sh"
GC="$SCRIPT_DIR/goal-contract.sh"
ENGINE="$SCRIPT_DIR/measured-budget-runtime.py"
NAME="mutable-dispatch-gateway-selftest"

if ! command -v jq >/dev/null 2>&1; then
  echo "$NAME: SKIP (jq not installed)"
  exit 0
fi
for f in "$GATEWAY" "$REPO_BINDING" "$BOUNDARY_RECEIPT" "$GC" "$ENGINE"; do
  [[ -f "$f" ]] || { echo "$NAME: FAIL: $f not found" >&2; exit 1; }
done

# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root,
# same reason mutable-dispatch-authorization-selftest.sh does.
WORK="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$WORK"' EXIT INT TERM
checks=0
failures=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

# --- authority fixtures ------------------------------------------------------
make_authority() {
  local out="$1" purpose="$2" auth_id="$3" trust_id="$4"
  jq -n --arg p "$purpose" --arg aid "$auth_id" --arg tid "$trust_id" \
    --arg key "$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" \
    '{contractType:"security-hmac-authority",schemaVersion:1,purpose:$p,authorityId:$aid,trustRootId:$tid,keyHex:$key}' >"$out"
  chmod 600 "$out"
}
AUTHORITY_FILE="$WORK/mda-authority.json"
make_authority "$AUTHORITY_FILE" "mutable-dispatch-authorization" "auth-fixture" "trust-fixture"
HOST_PROOF_AUTHORITY="$WORK/host-proof-authority.json"
make_authority "$HOST_PROOF_AUTHORITY" "host-proof" "authority:selftest" "trust:selftest"

STORE_ROOT="$WORK/store"
mkdir -p "$STORE_ROOT"
chmod 700 "$STORE_ROOT"

# --- MBE fixture chain: a real store with one genuinely issued permit ------
# Mirrors measured-budget-runtime-v2-selftest.py's RuntimeCase.prepare()/
# dispatch() closely enough to issue one real dispatch-permit; kept as an
# embedded script rather than a new production file because this exact chain
# only exists to seed a test fixture.
issue_permit() {
  local action_digest="$1" nonce="$2"
  python3 - "$STORE_ROOT" "$HOST_PROOF_AUTHORITY" "$action_digest" "$nonce" "$ENGINE" <<'PY'
import sys, json, importlib.util

store_root, authority_path, action_digest, nonce, engine_path = sys.argv[1:6]
spec = importlib.util.spec_from_file_location("mbe", engine_path)
mbe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mbe)

runtime = mbe.MeasuredBudgetRuntime(store_root)
T = [f"2026-08-01T00:00:{s:02d}.000Z" for s in range(30)]
EXP = "2026-08-01T00:10:00.000Z"
D = lambda n: mbe.ecf.typed_digest("selftest", {"value": n})

identity = runtime.usage_record(record={"contractType":"host-session-identity","schemaVersion":2,
    "sessionIdentityId":"session:gw","adapterId":"adapter:selftest","hostInstanceId":"host:gw",
    "workspaceIdentity":"workspace:gw","hostSessionId":"session.gw","artifactSessionId":"artifact:gw",
    "repositoryDecisionId":"rb:selftest.7","hostSchemaId":"selftest-v2","proofDigest":D(1),"startedAt":T[0]})

configured = {"modelRequestCount": 4}
policies = [{"dimension": name, "state": "configured" if name in configured else "unconfigured",
             "limit": configured.get(name), "unit": mbe.DIMENSION_DEFINITIONS[name]["unit"],
             "currency": "USD" if name == "monetaryMinorUnits" and name in configured else None,
             "scale": 2 if name == "monetaryMinorUnits" and name in configured else None}
            for name in mbe.DIMENSIONS]
budget = runtime.budget_open(goal_id="goal:gw", policy_digest=D(0), rollout_posture="reference-enforce",
    dimensions=policies, opened_at=T[0], goal_deadline_at=EXP, max_occurrences=2, max_attempts_per_occurrence=2)

snapshot = runtime.budget_snapshot(budget_id=budget["budgetId"], at=T[1])
proof = {"contractType":"authenticated-host-proof","schemaVersion":1,"proofType":"host-checkpoint",
    "proofId":"proof:1","issuer":"issuer:selftest","verifierId":"authority:selftest","trustRootId":"trust:selftest",
    "repositoryDecisionId":"rb:selftest.7","previousSessionIdentity":identity["sessionIdentityId"],
    "nextSessionIdentity":identity["sessionIdentityId"],"continuationDigest":D(2),"issuedAt":T[2],"expiresAt":T[4]}
proof["authenticator"] = mbe.security.authenticator(mbe.security.load(authority_path, "host-proof"), proof)
boundary_digest = mbe.ecf.typed_digest("authenticated-host-proof", proof)
boundary = runtime.epoch_boundary(goal_id=budget["goalId"], from_epoch_id="epoch:none", to_epoch_class=mbe.EPOCH_CLASSES[0],
    boundary_kind="initial-host-checkpoint", previous_session_identity_id=identity["sessionIdentityId"],
    next_session_identity_id=identity["sessionIdentityId"], continuation_digest=D(2),
    budget_snapshot_id=snapshot["snapshotId"], host_proof_digest=boundary_digest, observed_at=T[2])
verification = runtime.epoch_verify(boundary_id=boundary["boundaryId"], host_proof=proof,
    authority_path=authority_path, verified_at=T[3])
epoch = runtime.epoch_open(goal_id=budget["goalId"], budget_id=budget["budgetId"], epoch_class=mbe.EPOCH_CLASSES[0],
    sequence=1, host_session_identity_id=identity["sessionIdentityId"], opened_by_boundary_id=boundary["boundaryId"],
    epoch_verification_id=verification["epochVerificationId"], continuation_digest=D(2), opened_at=T[3])

occurrence, attempt = "occ:gw", "att:gw"
intent = runtime.dispatch_intent(goal_id=budget["goalId"], budget_id=budget["budgetId"], epoch_id=epoch["epochId"],
    session_identity_id=identity["sessionIdentityId"], occurrence_id=occurrence, attempt_id=attempt,
    agent="bubbles.test", phase="verification", action_class="subagent", action_family="agent",
    input_digest=D(4), action_digest=action_digest, work_boundary_id="scope:selftest",
    repository_decision_id="rb:selftest.7", created_at=T[5])
negotiation = {"contractType":"usage-negotiation","schemaVersion":2,"negotiationId":"neg:gw","intentId":intent["intentId"],
    "adapterId":"adapter:selftest","adapterContractVersion":2,"hostSchemaId":"selftest-v2",
    "sessionIdentityId":identity["sessionIdentityId"],"mappingDigest":D(5),"capabilitiesDigest":D(6),"negotiatedAt":T[6]}
runtime.usage_record(record=negotiation)
maximums = [{"dimension":"modelRequestCount","amount":4,"unit":mbe.DIMENSION_DEFINITIONS["modelRequestCount"]["unit"],"currency":None,"scale":None}]
dimension_digest = mbe.ecf.typed_digest("dimension-set", sorted(mbe.amount_key(row) for row in maximums))
quote = {"contractType":"usage-quote","schemaVersion":2,"quoteId":"quote:gw","goalId":budget["goalId"],
    "budgetId":budget["budgetId"],"epochId":epoch["epochId"],"sessionIdentityId":identity["sessionIdentityId"],
    "occurrenceId":occurrence,"attemptId":attempt,"actionDigest":action_digest,"adapterId":"adapter:selftest",
    "intentId":intent["intentId"],"negotiationId":negotiation["negotiationId"],"mappingDigest":D(5),"ruleDigest":D(7),
    "dimensionSetDigest":dimension_digest,"maximums":maximums,"expiresAt":EXP,"quotedAt":T[7]}
runtime.usage_record(record=quote)
snapshot2 = runtime.budget_snapshot(budget_id=budget["budgetId"], at=T[8])
reservation = runtime.budget_reserve(goal_id=budget["goalId"], budget_id=budget["budgetId"], epoch_id=epoch["epochId"],
    session_identity_id=identity["sessionIdentityId"], intent_id=intent["intentId"], occurrence_id=occurrence,
    attempt_id=attempt, action_digest=action_digest, adapter_id="adapter:selftest",
    negotiation_id=negotiation["negotiationId"], quote_id=quote["quoteId"],
    quote_digest=mbe.ecf.typed_digest("usage-quote", quote), dimension_set_digest=dimension_digest,
    amounts=maximums, expires_at=EXP, funding_reservation_id=None, retry_decision_id=None,
    expected_ecf_sequence=snapshot2["ecfSequence"], expected_ecf_head_digest=snapshot2["ecfHeadDigest"], at=T[9])
facts = []
for index, fact_type in enumerate(("phase-relevance","risk-tier","model-class","tool-grant","action-authorization","retry-eligibility")):
    facts.append(runtime.admission_fact(intent_id=intent["intentId"], fact_type=fact_type, source_record_id=f"source:{index}",
        source_digest=D(8), value_digest=mbe.ecf.typed_digest("fact-value", {"type": fact_type, "allowed": True}),
        state="verified", issued_at=T[10], expires_at=EXP))
decision = runtime.admission_evaluate(intent_id=intent["intentId"], budget_id=budget["budgetId"], epoch_id=epoch["epochId"],
    session_identity_id=identity["sessionIdentityId"], occurrence_id=occurrence, attempt_id=attempt,
    action_digest=action_digest, negotiation_id=negotiation["negotiationId"], quote_id=quote["quoteId"],
    reservation_id=reservation["reservationId"], epoch_verification_id=verification["epochVerificationId"],
    fact_ids=[row["factId"] for row in facts], evaluated_at=T[11])
permit = runtime.permit_issue(decision_id=decision["decisionId"], nonce=nonce, expires_at=EXP, issued_at=T[12],
    enforcement_kind="repository-reference")
print(json.dumps(permit))
PY
}

# --- fixture builders reused from mutable-dispatch-authorization-selftest.sh
physical_path() { (cd -P -- "$1" 2>/dev/null && pwd -P); }
create_eligible_repo() {
  local root="$1"
  mkdir -p "$root/bubbles/scripts" "$root/agents"
  printf 'fixture-version\n' >"$root/VERSION"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$root/install.sh"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$root/bubbles/scripts/cli.sh"
  printf '%s\n' '---' 'name: fixture-workflow' '---' >"$root/agents/bubbles.workflow.agent.md"
  chmod +x "$root/install.sh" "$root/bubbles/scripts/cli.sh"
  git init -q "$root"
  git -C "$root" config user.name "Bubbles Fixture"
  git -C "$root" config user.email "fixture@example.invalid"
  git -C "$root" add VERSION install.sh bubbles/scripts/cli.sh agents/bubbles.workflow.agent.md
  git -C "$root" commit -q -m "fixture repository"
  physical_path "$root"
}
write_valid_control() {
  local control_file="$1" session_id="$2" repository_root="$3" repository_alias="$4"
  local decision_id="rb:$session_id:1" control_digest
  control_digest="$(printf '%s' "$control_file" | sha256sum | awk '{print $1}')"
  jq -n --arg session "$session_id" --arg controlPathDigest "sha256:$control_digest" \
    --arg root "$repository_root" --arg alias "$repository_alias" --arg decision "$decision_id" \
    '{schemaVersion:1,sessionId:$session,controlPathDigest:$controlPathDigest,revision:1,
      currentBinding:{repositoryRoot:$root,repositoryAlias:$alias,establishedDecisionId:$decision,
        establishedAuthority:"explicit-repository-root",establishedAt:"2026-01-01T00:00:00Z",lastDecisionId:$decision},
      transitionHistory:[{revision:1,decisionId:$decision,fromRepositoryRoot:null,toRepositoryRoot:$root,
        fromRepositoryAlias:null,toRepositoryAlias:$alias,authority:"explicit-repository-root",
        transition:"established",targetKind:"repository-root",timestamp:"2026-01-01T00:00:00Z"}]}' \
    >"$control_file"
  chmod 600 "$control_file"
}
write_actionable_packet() {
  local packet_file="$1" session_id="$2" revision="$3" repository_root="$4" repository_alias="$5" control_file="$6"
  local decision_id="rb:$session_id:$revision" control_path_digest
  control_path_digest="$(jq -r '.controlPathDigest' "$control_file")"
  jq -n --arg root "$repository_root" --arg alias "$repository_alias" --arg session "$session_id" \
    --arg decision "$decision_id" --arg controlPathDigest "$control_path_digest" --argjson revision "$revision" \
    '{repositoryRoot:$root,repositoryAlias:$alias,
      repositoryResolution:{sessionId:$session,decisionId:$decision,controlRevision:$revision,
        controlPathDigest:$controlPathDigest,authority:"explicit-repository-root",transition:"established",
        scopeKind:"command",scopeId:null,targetKind:"repository-root",pathVisibility:"local",actionable:true}}' \
    >"$packet_file"
}
new_goal_case() {
  local name="$1" d="$WORK/goal-$1"
  mkdir -p "$d/spec"
  printf 'Deliver mutable-dispatch-gateway.\n' >"$d/request.txt"
  echo '{}' >"$d/session.json"
  bash "$GC" freeze \
    --session-file "$d/session.json" --source-request-file "$d/request.txt" \
    --intent "Dispatch through the canonical gateway" \
    --success-signal "mutable-dispatch-gateway-selftest exits 0" \
    --hard-constraint "no bypass flag exists" \
    --target "spec=specs/056-mutable-dispatch" --repository-root bubbles \
    --spec-target specs/056-mutable-dispatch --allowed-path 'bubbles/scripts/**' \
    --runner bubbles.goal --session-id "sess-$name" --repository-alias bubbles >/dev/null 2>&1
  echo '{ "version": 3, "status": "in_progress" }' >"$d/spec/state.json"
  bash "$GC" sync-boundary --session-file "$d/session.json" --state-file "$d/spec/state.json" >/dev/null 2>&1
  bash "$GC" mirror --session-file "$d/session.json" --state-file "$d/spec/state.json" >/dev/null 2>&1
  cat >"$d/spec/spec.md" <<'MD'
# Feature

## Outcome Contract

- **Intent**: Dispatch through the canonical gateway.
- **Success Signal**: mutable-dispatch-gateway-selftest exits 0.
- **Hard Constraints**: no bypass flag exists.
- **Failure Condition**: the broker launches without every invariant holding.
MD
  cat >"$d/spec/report.md" <<'MD'
# Report

## Summary

Delivered the canonical gateway.

## Test Evidence

The declared Success Signal was demonstrated by running the selftest.
MD
  printf '%s' "$d"
}
emit_pre_dispatch_receipt() {
  local goal_dir="$1" out="$2"
  bash "$GC" ref --session-file "$goal_dir/session.json" >"$goal_dir/ref.json" 2>/dev/null
  bash "$BOUNDARY_RECEIPT" emit --boundary pre-dispatch --session-file "$goal_dir/session.json" \
    --spec-dir "$goal_dir/spec" --candidate-repo bubbles --candidate-spec specs/056-mutable-dispatch \
    --candidate-path 'bubbles/scripts/x.sh' --ref-file "$goal_dir/ref.json" --mutable >"$out" 2>/dev/null
}

REPO_A="$WORK/repo-a"
mkdir -p "$REPO_A"
REPO_A_PHYSICAL="$(create_eligible_repo "$REPO_A")"
SESSION_ID="gw-session"
CONTROL_FILE="$WORK/control.json"
PACKET_FILE="$WORK/packet.json"
write_valid_control "$CONTROL_FILE" "$SESSION_ID" "$REPO_A_PHYSICAL" "bubbles"
write_actionable_packet "$PACKET_FILE" "$SESSION_ID" 1 "$REPO_A_PHYSICAL" "bubbles" "$CONTROL_FILE"

GOAL_DIR="$(new_goal_case happy)"
RECEIPT_FILE="$WORK/receipt.json"
emit_pre_dispatch_receipt "$GOAL_DIR" "$RECEIPT_FILE"

# --- dispatchAdmission config -------------------------------------------------
mkdir -p "$REPO_A_PHYSICAL/.github"
cat >"$REPO_A_PHYSICAL/.github/bubbles-project.yaml" <<'EOF'
dispatchAdmission:
  adapter: reference-broker
EOF

ARGV_JSON='["/usr/bin/true"]'
ACTION_DIGEST="sha256:$(printf '{"argv":%s}' "$ARGV_JSON" | sha256sum | awk '{print $1}')"
ACTION_FILE="$WORK/action.json"
jq -n --argjson argv "$ARGV_JSON" --arg ad "$ACTION_DIGEST" '{argv:$argv,actionDigest:$ad}' >"$ACTION_FILE"

PERMIT_JSON="$(issue_permit "$ACTION_DIGEST" "nonce:gw-happy")"
PERMIT_ID="$(jq -r '.permitId' <<<"$PERMIT_JSON")"
CONSUMPTION_FILE="$WORK/consumption.json"
jq -n --arg pid "$PERMIT_ID" --arg nonce "nonce:gw-happy" --arg ad "$ACTION_DIGEST" \
  '{permit_id:$pid,nonce:$nonce,consumed_at:"2026-08-01T00:00:13.000Z",action_digest:$ad}' >"$CONSUMPTION_FILE"

dispatch_args=(dispatch --repo-root "$REPO_A_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$PACKET_FILE" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles \
  --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE")

# --- G1. happy path: every invariant holds, the broker actually launches ----
set +e
OUT="$(bash "$GATEWAY" "${dispatch_args[@]}" 2>"$WORK/g1.err")"
g1_rc=$?
set -e
if [[ "$g1_rc" -eq 0 ]] && jq -e '.contractType == "reference-dispatch-result" and .childExitCode == 0' <<<"$OUT" >/dev/null 2>&1; then
  ok "G1 the gateway dispatches through the broker when every invariant holds"
else
  bad "G1 happy-path dispatch" "rc=$g1_rc err=$(cat "$WORK/g1.err") out=$OUT"
fi

# --- G2. dispatchAdmission.adapter absent -> refused, broker never runs ----
NOADAPTER_REPO="$WORK/repo-noadapter"
mkdir -p "$NOADAPTER_REPO"
NOADAPTER_PHYSICAL="$(create_eligible_repo "$NOADAPTER_REPO")"
set +e
out2="$(bash "$GATEWAY" dispatch --repo-root "$NOADAPTER_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$PACKET_FILE" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE" 2>&1)"
rc2=$?
set -e
if [[ "$rc2" -eq 1 ]] && grep -qi "none" <<<"$out2"; then
  ok "G2 an absent dispatchAdmission.adapter (defaults to none) refuses"
else
  bad "G2 absent adapter refuses" "rc=$rc2 out=$out2"
fi

# --- G3. dispatchAdmission.adapter explicitly none -> refused ---------------
NONE_REPO="$WORK/repo-none"
mkdir -p "$NONE_REPO/.github"
NONE_PHYSICAL="$(create_eligible_repo "$NONE_REPO")"
mkdir -p "$NONE_PHYSICAL/.github"
cat >"$NONE_PHYSICAL/.github/bubbles-project.yaml" <<'EOF'
dispatchAdmission:
  adapter: none
EOF
set +e
out3="$(bash "$GATEWAY" dispatch --repo-root "$NONE_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$PACKET_FILE" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE" 2>&1)"
rc3=$?
set -e
if [[ "$rc3" -eq 1 ]] && grep -qF "admits no mutable child dispatch" <<<"$out3"; then
  ok "G3 dispatchAdmission.adapter: none refuses explicitly, with its own named reason"
else
  bad "G3 explicit none refuses with its own message" "rc=$rc3 out=$out3"
fi

# --- G4. ADVERSARIAL: repository authority fails -> refused, no permit consumed
BAD_PACKET="$WORK/bad-packet.json"
jq '.repositoryAlias = "not-bubbles"' "$PACKET_FILE" >"$BAD_PACKET"
set +e
out4="$(bash "$GATEWAY" dispatch --repo-root "$REPO_A_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$BAD_PACKET" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE" 2>&1)"
rc4=$?
set -e
consumed_after_g4="$(python3 -c "
import importlib.util, sys
spec = importlib.util.spec_from_file_location('mbe', '$ENGINE')
mbe = importlib.util.module_from_spec(spec); spec.loader.exec_module(mbe)
runtime = mbe.MeasuredBudgetRuntime('$STORE_ROOT')
records = runtime.records()
print(sum(1 for r in records if r.get('contractType') == 'permit-consumption' and r.get('permitId') == '$PERMIT_ID'))
")"
if [[ "$rc4" -eq 1 ]] && [[ "$consumed_after_g4" == "0" ]]; then
  ok "G4 a failed repository-authority check refuses before the permit is ever consumed"
else
  bad "G4 repository authority failure refuses pre-consumption" "rc=$rc4 consumed=$consumed_after_g4 out=$out4"
fi

# --- G5. ADVERSARIAL: the broker's OWN snapshot refusal still refuses the gateway
BAD_ACTION_FILE="$WORK/bad-action.json"
jq -n '{argv:["/nonexistent/executable"],actionDigest:"sha256:0000000000000000000000000000000000000000000000000000000000000000"}' >"$BAD_ACTION_FILE"
set +e
out5="$(bash "$GATEWAY" dispatch --repo-root "$REPO_A_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$PACKET_FILE" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$BAD_ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE" 2>&1)"
rc5=$?
set -e
if [[ "$rc5" -eq 1 ]]; then
  ok "G5 the broker's own snapshot refusal (bad action) still refuses via the gateway"
else
  bad "G5 snapshot refusal propagates" "rc=$rc5 out=$out5"
fi

# --- G6. ADVERSARIAL: a resolved adapter that is neither none nor reference-broker
# is refused too. Only two adapters exist in this tree today (none,
# reference-broker), so this stubs the RESOLVER, not a third real adapter, to
# prove the gateway's own allowlist check -- not just dispatch-adapter-
# resolve.sh's file-existence check -- is what is actually load-bearing here.
FAKE_RESOLVER="$WORK/fake-adapter-resolve.sh"
cat >"$FAKE_RESOLVER" <<'EOF'
#!/usr/bin/env bash
echo "adapter=some-future-adapter"
EOF
chmod +x "$FAKE_RESOLVER"
set +e
out6="$(BUBBLES_DISPATCH_ADAPTER_RESOLVE="$FAKE_RESOLVER" bash "$GATEWAY" dispatch --repo-root "$REPO_A_PHYSICAL" --store-root "$STORE_ROOT" --authority-file "$AUTHORITY_FILE" \
  --session-id "$SESSION_ID" --session-control-file "$CONTROL_FILE" --packet-file "$PACKET_FILE" \
  --feature-dir "$GOAL_DIR/spec" --candidate-repo bubbles --receipt-file "$RECEIPT_FILE" --session-file "$GOAL_DIR/session.json" \
  --permit-id "$PERMIT_ID" --action "$ACTION_FILE" --permit-consumption "$CONSUMPTION_FILE" 2>&1)"
rc6=$?
set -e
if [[ "$rc6" -eq 1 ]] && grep -qF "some-future-adapter" <<<"$out6"; then
  ok "G6 an adapter that is neither none nor reference-broker is refused"
else
  bad "G6 unsupported adapter name refused" "rc=$rc6 out=$out6"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
