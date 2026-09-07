#!/usr/bin/env bash
#
# autonomy-resolve-selftest.sh — proves the autonomy precedence chain.
#
# The load-bearing assertion is that each layer WINS OVER THE ONE BELOW IT, and
# that the two durable layers (env, project config) resolve without any per-run
# directive. That second property is the whole point of the resolver: it is what
# lets an interrupted session resume at the same posture without the operator
# re-asserting it in the prompt.

set -uo pipefail

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
RESOLVE="$SCRIPT_DIR/autonomy-resolve.sh"
BINDING="$SCRIPT_DIR/repository-binding.sh"

ISSUES=0
TMPS=()
trap '[[ ${#TMPS[@]} -gt 0 ]] && rm -rf "${TMPS[@]}" 2>/dev/null || true' EXIT INT TERM

pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  ISSUES=$((ISSUES + 1))
}

check() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label"
  else
    fail "$label"
    echo "  expected: $expected"
    echo "  actual:   $actual"
  fi
}

# A repo root with no bubbles-project.yaml, so the config layer is genuinely absent.
BARE_ROOT="$(mktemp -d)"
TMPS+=("$BARE_ROOT")

# A repo root whose project config pins the posture.
CONF_ROOT="$(mktemp -d)"
TMPS+=("$CONF_ROOT")
mkdir -p "$CONF_ROOT/.github"
printf 'autonomy: interactive\n' >"$CONF_ROOT/.github/bubbles-project.yaml"

resolve() { (unset BUBBLES_AUTONOMY; bash "$RESOLVE" "$@") 2>/dev/null; }
posture() { resolve "$@" | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p'; }
layer() { resolve "$@" | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p'; }

echo "Running autonomy-resolve selftest..."
echo "Scenario: the posture dial resolves from a documented precedence chain and survives a restart."

# --- Layer 4: framework default ---
check "Layer 4: no input resolves to the framework default" "full" "$(posture --repo-root "$BARE_ROOT")"
check "Layer 4: default names itself as the winning layer" "framework-default" "$(layer --repo-root "$BARE_ROOT")"

# --- Layer 3: project config beats default ---
if command -v yq >/dev/null 2>&1; then
  check "Layer 3: project config beats the framework default" "interactive" "$(posture --repo-root "$CONF_ROOT")"
  check "Layer 3: names project-config as the winning layer" "project-config" "$(layer --repo-root "$CONF_ROOT")"
else
  echo "SKIP: layer-3 config assertions require yq (absent); env/directive/default layers still asserted below."
fi

# --- Layer 2: env beats project config AND default ---
check "Layer 2: env beats the framework default" "guarded" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
check "Layer 2: names env as the winning layer" "env" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p')"
if command -v yq >/dev/null 2>&1; then
  check "Layer 2: env beats the project config" "guarded" \
    "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --repo-root "$CONF_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
fi

# --- Layer 1: directive beats everything ---
check "Layer 1: --autonomy beats env" "interactive" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --autonomy interactive --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"
check "Layer 1: names directive as the winning layer" "directive" \
  "$(BUBBLES_AUTONOMY=guarded bash "$RESOLVE" --autonomy interactive --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY_SOURCE=//p')"
check "Layer 1: an autonomy: token inside --directive is extracted" "guarded" \
  "$(posture --directive 'mode: full-delivery autonomy:guarded specs: x' --repo-root "$BARE_ROOT")"
check "Layer 1: explicit --autonomy overrides the --directive token" "interactive" \
  "$(posture --autonomy interactive --directive 'autonomy:guarded' --repo-root "$BARE_ROOT")"

# --- Durability: the restart property the resolver exists to provide ---
check "Durability: env alone resolves with NO per-run directive" "interactive" \
  "$(BUBBLES_AUTONOMY=interactive bash "$RESOLVE" --repo-root "$BARE_ROOT" 2>/dev/null | sed -n 's/^BUBBLES_RESOLVED_AUTONOMY=//p')"

# --- Invalid values exit 1 ---
resolve --autonomy bogus --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid --autonomy exits 1" "1" "$?"
BUBBLES_AUTONOMY=bogus bash "$RESOLVE" --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid BUBBLES_AUTONOMY exits 1" "1" "$?"
resolve --directive 'autonomy:bogus' --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid autonomy token in --directive exits 1" "1" "$?"

if command -v yq >/dev/null 2>&1; then
  BAD_ROOT="$(mktemp -d)"
  TMPS+=("$BAD_ROOT")
  mkdir -p "$BAD_ROOT/.github"
  printf 'autonomy: bogus\n' >"$BAD_ROOT/.github/bubbles-project.yaml"
  resolve --repo-root "$BAD_ROOT" >/dev/null 2>&1
  check "Invalid autonomy in project config exits 1" "1" "$?"
fi

# --- Usage errors exit 2 ---
resolve --autonomy --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "--autonomy with no value exits 2" "2" "$?"
resolve --autonomy full --autonomy guarded --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Duplicate --autonomy exits 2" "2" "$?"
resolve --bogus-flag >/dev/null 2>&1
check "Unknown flag exits 2" "2" "$?"
resolve --format yaml --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "Invalid --format exits 2" "2" "$?"

# --- No bypass exists (the posture never waives verification) ---
for flag in --skip --force --ignore --no-verify; do
  resolve "$flag" >/dev/null 2>&1
  check "Bypass flag $flag is refused (exit 2)" "2" "$?"
done

# --- Help + json shape ---
resolve --help >/dev/null 2>&1
check "--help exits 0" "0" "$?"
help_out="$(bash "$RESOLVE" --help 2>/dev/null)"
if printf '%s' "$help_out" | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi
check "--format json emits a single-line record" '{"autonomy":"full","source":"framework-default"}' \
  "$(posture_json=$(resolve --format json --repo-root "$BARE_ROOT"); printf '%s' "$posture_json")"

# --- SCOPE-3: `unattended` is opt-in AND may not be unbounded ---
check "unattended resolves when the budget is bounded" "unattended" \
  "$(posture --autonomy unattended --session-budget bounded)"

resolve --autonomy unattended --session-budget bounded >/dev/null 2>&1
check "unattended with a bounded budget exits 0" "0" "$?"

resolve --autonomy unattended --session-budget unbounded >/dev/null 2>&1
check "unattended with an unbounded budget is refused (exit 3)" "3" "$?"

unbounded_err="$(
  (unset BUBBLES_AUTONOMY
    bash "$RESOLVE" --autonomy unattended --session-budget unbounded) 2>&1 >/dev/null
)"
if printf '%s' "$unbounded_err" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
  pass "The unbounded refusal names its code (E039-UNATTENDED-UNBOUNDED)"
else
  fail "The unbounded refusal must name E039-UNATTENDED-UNBOUNDED"
fi

# The explicit override remains available for non-repository posture checks.
BUDGET_ROOT="$(mktemp -d)"
TMPS+=("$BUDGET_ROOT")
mkdir -p "$BUDGET_ROOT/.specify/memory"
printf '%s\n' '{"sessionBudget":{"maxToolCalls":250,"maxWallClockMinutes":null}}' >"$BUDGET_ROOT/.specify/memory/bubbles.session.json"
resolve --autonomy unattended --session-budget bounded --repo-root "$BUDGET_ROOT" >/dev/null 2>&1
check "unattended is allowed with an explicit bounded posture input" "0" "$?"

printf '%s\n' '{"sessionBudget":{"maxToolCalls":null,"maxWallClockMinutes":null}}' >"$BUDGET_ROOT/.specify/memory/bubbles.session.json"
resolve --autonomy unattended --session-budget unbounded --repo-root "$BUDGET_ROOT" >/dev/null 2>&1
check "unattended is refused with an explicit unbounded posture input" "3" "$?"

resolve --autonomy unattended --session-budget bogus >/dev/null 2>&1
check "--session-budget with a bogus value is a usage error (exit 2)" "2" "$?"

# Adding a fourth value must not shift the default, and must not constrain the others.
check "the framework default is still full, not unattended" "full" "$(posture --repo-root "$BARE_ROOT")"

resolve --autonomy full --session-budget unbounded --repo-root "$BARE_ROOT" >/dev/null 2>&1
check "full is unaffected by an unbounded budget (exit 0)" "0" "$?"

# --- BUG-037 Scope 1: exact validated session policy head only -------------

echo "SCENARIO: SCN-B037-011 exact-session policy controls unattended boundedness"

POLICY_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
TMPS+=("$POLICY_ROOT")
mkdir -p "$POLICY_ROOT/.specify/memory" "$POLICY_ROOT/bubbles/scripts" "$POLICY_ROOT/agents"
printf '%s\n' 'test-version' > "$POLICY_ROOT/VERSION"
printf '%s\n' '#!/usr/bin/env bash' > "$POLICY_ROOT/install.sh"
printf '%s\n' '#!/usr/bin/env bash' > "$POLICY_ROOT/bubbles/scripts/cli.sh"
git init -q "$POLICY_ROOT"

POLICY_CONTROL_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
TMPS+=("$POLICY_CONTROL_ROOT")

prepare_policy_binding() {
  local session_id="$1"
  local control_file="$POLICY_CONTROL_ROOT/$session_id.control.json"
  local packet_file="$POLICY_CONTROL_ROOT/$session_id.packet.json"
  local binding_output

  binding_output="$(/bin/bash "$BINDING" preflight \
    --session-id "$session_id" \
    --session-control-file "$control_file" \
    --expected-control-revision 0 \
    --request-class TARGETLESS_MODE \
    --repository-root "$POLICY_ROOT" \
    --workspace-root "$POLICY_ROOT")"
  printf '%s\n' "$binding_output" |
    awk '/^\{.*"repositoryRoot"/ { packet = $0 } END { print packet }' > "$packet_file"
  printf '%s\n%s' "$control_file" "$packet_file"
}

policy_binding_a="$(prepare_policy_binding host-a)"
policy_control_a="${policy_binding_a%%$'\n'*}"
policy_packet_a="${policy_binding_a#*$'\n'}"
policy_binding_b="$(prepare_policy_binding host-b)"
policy_control_b="${policy_binding_b%%$'\n'*}"
policy_packet_b="${policy_binding_b#*$'\n'}"
policy_binding_c="$(prepare_policy_binding host-c)"
policy_control_c="${policy_binding_c%%$'\n'*}"
policy_packet_c="${policy_binding_c#*$'\n'}"
policy_binding_d="$(prepare_policy_binding host-d)"
policy_control_d="${policy_binding_d%%$'\n'*}"
policy_packet_d="${policy_binding_d#*$'\n'}"

cat > "$POLICY_ROOT/.specify/memory/bubbles.session.json" <<'JSON'
{
  "legacyMarker": "preserve-root",
  "sessionBudget": {
    "maxToolCalls": 999,
    "legacyMarker": "must-not-activate-a-session"
  },
  "sessionBudgetHistory": [
    {
      "recordSchemaVersion": 1,
      "hostSessionId": "host-a",
      "revision": 1,
      "supersedesRevision": null,
      "recordedAt": "2026-09-01T00:00:00Z",
      "budget": {
        "schemaVersion": 1,
        "maxTotalConvergenceIterations": 2,
        "maxWallClockMinutes": 180,
        "maxToolCalls": 350,
        "maxSingleToolResultBytes": 50000,
        "maxCumulativeToolResultBytes": 250000,
        "maxPromptTokensPerRequest": null,
        "maxCumulativePromptTokens": null
      }
    },
    {
      "recordSchemaVersion": 1,
      "hostSessionId": "host-b",
      "revision": 1,
      "supersedesRevision": null,
      "recordedAt": "2026-09-01T00:00:01Z",
      "budget": {
        "schemaVersion": 1,
        "maxTotalConvergenceIterations": null,
        "maxWallClockMinutes": null,
        "maxToolCalls": null,
        "maxSingleToolResultBytes": null,
        "maxCumulativeToolResultBytes": null,
        "maxPromptTokensPerRequest": null,
        "maxCumulativePromptTokens": null
      }
    },
    {
      "recordSchemaVersion": 1,
      "hostSessionId": "host-d",
      "revision": 1,
      "supersedesRevision": null,
      "recordedAt": "2026-09-01T00:00:02Z",
      "budget": {
        "schemaVersion": 1,
        "maxTotalConvergenceIterations": null,
        "maxWallClockMinutes": null,
        "maxToolCalls": null,
        "maxSingleToolResultBytes": null,
        "maxCumulativeToolResultBytes": null,
        "maxPromptTokensPerRequest": null,
        "maxCumulativePromptTokens": 11
      }
    }
  ]
}
JSON

resolve_exact_policy() {
  local session_id="$1"
  local control_file="$2"
  local packet_file="$3"
  env -u BUBBLES_AUTONOMY /bin/bash "$RESOLVE" \
    --autonomy unattended \
    --repo-root "$POLICY_ROOT" \
    --session-id "$session_id" \
    --session-control-file "$control_file" \
    --binding-packet-file "$packet_file" 2>&1
}

policy_a_output="$(resolve_exact_policy host-a "$policy_control_a" "$policy_packet_a")"
policy_a_rc=$?
check "SCN-B037-011 bounded host-a exact policy head allows unattended" "0" "$policy_a_rc"
if printf '%s\n' "$policy_a_output" | grep -q '^BUBBLES_RESOLVED_AUTONOMY=unattended$'; then
  pass "SCN-B037-011 host-a result comes from the production resolver"
else
  fail "SCN-B037-011 host-a production resolver did not emit unattended"
fi

policy_b_output="$(resolve_exact_policy host-b "$policy_control_b" "$policy_packet_b")"
policy_b_rc=$?
check "SCN-B037-011 all-null host-b exact policy head remains default-off" "3" "$policy_b_rc"
if printf '%s\n' "$policy_b_output" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
  pass "SCN-B037-011 host-b refusal names the unattended boundedness contract"
else
  fail "SCN-B037-011 host-b all-null policy should emit E039-UNATTENDED-UNBOUNDED"
fi

policy_c_output="$(resolve_exact_policy host-c "$policy_control_c" "$policy_packet_c")"
policy_c_rc=$?
check "SCN-B037-011 bounded sibling and legacy policy cannot activate absent host-c" "3" "$policy_c_rc"
if printf '%s\n' "$policy_c_output" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
  pass "SCN-B037-011 absent host-c reports the exact-session default-off result"
else
  fail "SCN-B037-011 absent host-c should report E039-UNATTENDED-UNBOUNDED"
fi

policy_d_output="$(resolve_exact_policy host-d "$policy_control_d" "$policy_packet_d")"
policy_d_rc=$?
check "SCN-B037-011 final declared cap remains eligible for exact-session boundedness" "0" "$policy_d_rc"
if printf '%s\n' "$policy_d_output" | grep -q '^BUBBLES_RESOLVED_AUTONOMY=unattended$'; then
  pass "SCN-B037-011 host-d preserves a non-null maxCumulativePromptTokens value"
else
  fail "SCN-B037-011 host-d should remain bounded when only the final cap is non-null"
fi

cp "$POLICY_ROOT/.specify/memory/bubbles.session.json" "$POLICY_CONTROL_ROOT/policy-valid.json"
jq --arg hostA host-a '
  .sessionBudgetHistory += [
    (.sessionBudgetHistory[] | select(.hostSessionId == $hostA and .revision == 1)
     | .revision = 2
     | .supersedesRevision = 1
     | .recordedAt = "2026-09-01T00:00:03Z"
     | .budget |= with_entries(if .key == "schemaVersion" then . else .value = null end))
  ]
' "$POLICY_CONTROL_ROOT/policy-valid.json" > "$POLICY_ROOT/.specify/memory/bubbles.session.json"
policy_latest_null_output="$(resolve_exact_policy host-a "$policy_control_a" "$policy_packet_a")"
policy_latest_null_rc=$?
check "SCN-B037-011 only the unique latest exact-session head controls boundedness" "3" "$policy_latest_null_rc"
if printf '%s\n' "$policy_latest_null_output" | grep -q 'E039-UNATTENDED-UNBOUNDED'; then
  pass "SCN-B037-011 an all-null correction disables only its exact session"
else
  fail "SCN-B037-011 resolver should not inherit an older bounded revision"
fi

jq --arg hostA host-a '
  .sessionBudgetHistory += [
    (.sessionBudgetHistory[] | select(.hostSessionId == $hostA and .revision == 1)
     | .revision = 2
     | .supersedesRevision = 1
     | .recordedAt = "2026-09-01T00:00:04Z"),
    (.sessionBudgetHistory[] | select(.hostSessionId == $hostA and .revision == 1)
     | .revision = 3
     | .supersedesRevision = 1
     | .recordedAt = "2026-09-01T00:00:05Z")
  ]
' "$POLICY_CONTROL_ROOT/policy-valid.json" > "$POLICY_ROOT/.specify/memory/bubbles.session.json"
policy_branch_output="$(resolve_exact_policy host-a "$policy_control_a" "$policy_packet_a")"
policy_branch_rc=$?
check "SCN-B037-011 branching exact-session policy history fails closed" "3" "$policy_branch_rc"
if printf '%s\n' "$policy_branch_output" | grep -q 'E039-SESSION-POLICY-INVALID'; then
  pass "SCN-B037-011 branch rejection uses the stable policy failure code"
else
  fail "SCN-B037-011 branching policy history should emit E039-SESSION-POLICY-INVALID"
fi

jq '.sessionBudgetHistory += [.sessionBudgetHistory[0]]' \
  "$POLICY_CONTROL_ROOT/policy-valid.json" > "$POLICY_ROOT/.specify/memory/bubbles.session.json"
policy_duplicate_output="$(resolve_exact_policy host-a "$policy_control_a" "$policy_packet_a")"
policy_duplicate_rc=$?
check "SCN-B037-011 duplicate exact-session policy revision fails closed" "3" "$policy_duplicate_rc"
if printf '%s\n' "$policy_duplicate_output" | grep -q 'E039-SESSION-POLICY-INVALID'; then
  pass "SCN-B037-011 malformed policy chain emits its stable failure code"
else
  fail "SCN-B037-011 malformed policy chain should emit E039-SESSION-POLICY-INVALID"
fi

cp "$POLICY_CONTROL_ROOT/policy-valid.json" "$POLICY_ROOT/.specify/memory/bubbles.session.json"
mv "$POLICY_ROOT/.specify/memory/bubbles.session.json" "$POLICY_CONTROL_ROOT/policy-symlink-target.json"
ln -s "$POLICY_CONTROL_ROOT/policy-symlink-target.json" "$POLICY_ROOT/.specify/memory/bubbles.session.json"
policy_symlink_output="$(resolve_exact_policy host-a "$policy_control_a" "$policy_packet_a")"
policy_symlink_rc=$?
if [[ "$policy_symlink_rc" -eq 3 ]] &&
  printf '%s\n' "$policy_symlink_output" | grep -q 'E039-SESSION-POLICY-INVALID'; then
  pass "SCN-B037-011 resolver rejects a symlink session-state capture"
else
  fail "SCN-B037-011 resolver should fail closed on a symlink session-state capture"
fi
rm -f "$POLICY_ROOT/.specify/memory/bubbles.session.json"
mv "$POLICY_CONTROL_ROOT/policy-symlink-target.json" "$POLICY_ROOT/.specify/memory/bubbles.session.json"

jq '.repositoryResolution.controlRevision += 1' "$policy_packet_a" > "$POLICY_CONTROL_ROOT/host-a.stale.packet.json"
policy_stale_output="$(resolve_exact_policy host-a "$policy_control_a" "$POLICY_CONTROL_ROOT/host-a.stale.packet.json")"
policy_stale_rc=$?
if [[ "$policy_stale_rc" -ne 0 ]] &&
  printf '%s\n' "$policy_stale_output" | grep -q 'packet authority is invalid'; then
  pass "SCN-B037-011 stale authority cannot select a session policy head"
else
  fail "SCN-B037-011 stale authority should fail before policy selection"
fi

for agent_contract in \
  "$SCRIPT_DIR/../../agents/bubbles.goal.agent.md" \
  "$SCRIPT_DIR/../../agents/bubbles.workflow.agent.md" \
  "$SCRIPT_DIR/../../agents/bubbles.iterate.agent.md" \
  "$SCRIPT_DIR/../../agents/bubbles.sprint.agent.md"; do
  contract_cap_count=0
  for cap_name in \
    maxTotalConvergenceIterations \
    maxWallClockMinutes \
    maxToolCalls \
    maxSingleToolResultBytes \
    maxCumulativeToolResultBytes \
    maxPromptTokensPerRequest \
    maxCumulativePromptTokens; do
    if grep -Fq "\`$cap_name\`" "$agent_contract"; then
      contract_cap_count=$((contract_cap_count + 1))
    fi
  done
  if grep -Fq 'state-snapshot.sh --session-budget-json <object> --expected-session-budget-revision 0' "$agent_contract" &&
     grep -Fq 'validated session, control file, and packet' "$agent_contract" &&
    grep -Fq "explicit \`null\`" "$agent_contract" &&
     [[ "$contract_cap_count" -eq 7 ]]; then
    pass "SCN-B037-011 $(basename "$agent_contract") seeds all seven exact-session values and explicit nulls"
  else
    fail "SCN-B037-011 $(basename "$agent_contract") lacks the complete exact-session seven-cap writer contract"
  fi
done

workflow_budget_block="$(awk '
  /^  sessionBudget:/ { active = 1 }
  active { print }
  active && /^  contextBoundary:/ { exit }
' "$SCRIPT_DIR/../workflows.yaml")"
workflow_cap_count=0
for cap_name in \
  maxTotalConvergenceIterations \
  maxWallClockMinutes \
  maxToolCalls \
  maxSingleToolResultBytes \
  maxCumulativeToolResultBytes \
  maxPromptTokensPerRequest \
  maxCumulativePromptTokens; do
  if printf '%s\n' "$workflow_budget_block" | grep -q "^    $cap_name:$"; then
    workflow_cap_count=$((workflow_cap_count + 1))
  fi
done
workflow_null_count="$(printf '%s\n' "$workflow_budget_block" | grep -c '^      default: null$' || true)"
if [[ "$workflow_cap_count" -eq 7 && "$workflow_null_count" -eq 7 ]]; then
  pass "SCN-B037-011 workflow policy retains all seven cap names and null defaults"
else
  fail "SCN-B037-011 workflow policy cap inventory changed (caps=$workflow_cap_count nulls=$workflow_null_count)"
fi

echo
if [[ $ISSUES -eq 0 ]]; then
  echo "autonomy-resolve selftest passed."
  exit 0
fi
echo "autonomy-resolve selftest failed with $ISSUES issue(s)."
exit 1
