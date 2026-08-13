#!/usr/bin/env bash
set -euo pipefail

# state-snapshot-selftest.sh — verify state-snapshot.sh behavior.
#
# Cases:
#   1. Append to fresh session JSON → turnNumber=1, fields present.
#   2. Append to existing session JSON → turnNumber increments,
#      prior records preserved.
#   3. --mode end after --mode start → two records present with
#      matching scopeId and a turn-start/turn-end pair.
#   4. In-place caller packet mutation after private capture cannot redirect.
#   5. Atomic caller packet replacement after private capture cannot redirect.
#   6. Successful normal + convergence updates use same-directory renames and
#      leave no private packet capture or session-update temp file.
#   7. Binding validation failure cleans the private packet capture.
#   8. Normal update rename failure cleans its update temp and packet capture.
#   9. Convergence rename failure cleans its update temp and packet capture.
#  10. Missing required --phase flag → exit non-zero with error message.
#  11. --help exits zero and prints its usage banner.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SNAPSHOT="$SCRIPT_DIR/state-snapshot.sh"
BINDING="$SCRIPT_DIR/repository-binding.sh"

if [[ ! -x "$SNAPSHOT" ]]; then
  echo "FAIL: state-snapshot.sh is not executable at $SNAPSHOT" >&2
  exit 1
fi

if [[ ! -x "$BINDING" ]]; then
  echo "FAIL: repository-binding.sh is not executable at $BINDING" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "FAIL: jq is required for state-snapshot-selftest.sh but not found in PATH" >&2
  exit 1
fi

# macOS mktemp -d sits under the /var symlink, which repository-binding rejects.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

failures=0
assertions=0
cases=11

pass() { assertions=$((assertions + 1)); echo "PASS: $1"; }
fail() { assertions=$((assertions + 1)); echo "FAIL: $1"; failures=$((failures + 1)); }

shopt -s dotglob nullglob

directory_entry_count() {
  local directory="$1"
  local -a entries=("$directory"/*)
  printf '%s' "${#entries[@]}"
}

snapshot_update_temp_count() {
  local session_directory="$1"
  local -a temp_files=(
    "$session_directory"/.bubbles.session.json.update.*
    "$session_directory"/.bubbles.session.json.convergence.*
  )
  printf '%s' "${#temp_files[@]}"
}

file_line_count() {
  local file="$1"
  local count=0
  local line

  if [[ -f "$file" ]]; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      count=$((count + 1))
    done < "$file"
  fi
  printf '%s' "$count"
}

BOUND_CONTROL=""
BOUND_PACKET=""

prepare_bound_repo() {
  local root="$1"
  local session_id="$2"
  local control_dir="$TMP_ROOT/control-$session_id"
  local binding_output

  mkdir -p "$root/.specify/memory" "$root/bubbles/scripts" "$root/agents" "$control_dir"
  chmod 700 "$control_dir"
  printf 'test-version\n' > "$root/VERSION"
  printf '#!/usr/bin/env bash\n' > "$root/install.sh"
  printf '#!/usr/bin/env bash\n' > "$root/bubbles/scripts/cli.sh"
  git init -q "$root"

  BOUND_CONTROL="$control_dir/repository-binding.json"
  BOUND_PACKET="$TMP_ROOT/$session_id.packet.json"
  binding_output="$(bash "$BINDING" preflight \
    --session-id "$session_id" \
    --session-control-file "$BOUND_CONTROL" \
    --expected-control-revision 0 \
    --request-class TARGETLESS_MODE \
    --repository-root "$root" \
    --workspace-root "$root")"
  printf '%s\n' "$binding_output" | awk '/^\{.*"repositoryRoot"/ { packet = $0 } END { print packet }' > "$BOUND_PACKET"
  jq -e '.repositoryResolution.actionable == true' "$BOUND_PACKET" >/dev/null
}

REAL_BASH="$(command -v bash)"
REAL_MV="$(command -v mv)"
RACE_BIN="$TMP_ROOT/race-bin"
mkdir -p "$RACE_BIN"
cat > "$RACE_BIN/bash" <<EOF
#!$REAL_BASH
set -euo pipefail
if [[ "\${1:-}" == */repository-binding.sh && "\${2:-}" == "mirror-session" && -n "\${SNAPSHOT_ATTACK_KIND:-}" ]]; then
  packet_file=""
  next_is_packet=false
  for arg in "\$@"; do
    if [[ "\$next_is_packet" == true ]]; then
      packet_file="\$arg"
      break
    fi
    if [[ "\$arg" == "--packet-file" ]]; then
      next_is_packet=true
    fi
  done
  [[ -n "\$packet_file" && "\$packet_file" != "\$SNAPSHOT_CALLER_PACKET" ]]
  cmp -s "\$SNAPSHOT_CALLER_PACKET" "\$packet_file"
  packet_mode="\$(stat -c '%a' "\$packet_file" 2>/dev/null || stat -f '%Lp' "\$packet_file")"
  [[ "\$packet_mode" == "600" ]]
  printf 'byte-identical-private-copy-mode-0600\n' > "\$SNAPSHOT_CAPTURE_PROOF_FILE"
  case "\$SNAPSHOT_ATTACK_KIND" in
    in-place)
      cp "\$SNAPSHOT_ATTACKER_PACKET" "\$SNAPSHOT_CALLER_PACKET"
      ;;
    replace)
      mv "\$SNAPSHOT_ATTACKER_PACKET" "\$SNAPSHOT_CALLER_PACKET"
      ;;
    *)
      echo "unexpected snapshot attack kind: \$SNAPSHOT_ATTACK_KIND" >&2
      exit 2
      ;;
  esac
fi
exec "$REAL_BASH" "\$@"
EOF
chmod 700 "$RACE_BIN/bash"

ATOMIC_BIN="$TMP_ROOT/atomic-bin"
mkdir -p "$ATOMIC_BIN"
cat > "$ATOMIC_BIN/mv" <<EOF
#!$REAL_BASH
set -euo pipefail

path_device() {
  stat -c '%d' "\$1" 2>/dev/null || stat -f '%d' "\$1"
}

if [[ -n "\${SNAPSHOT_RENAME_PROOF_FILE:-}" && "\$#" -eq 2 && "\${2:-}" == */.specify/memory/bubbles.session.json ]]; then
  source_path="\$1"
  destination_path="\$2"
  case "\$(basename "\$source_path")" in
    .bubbles.session.json.update.*) rename_kind="update" ;;
    .bubbles.session.json.convergence.*) rename_kind="convergence" ;;
    *) rename_kind="" ;;
  esac

  if [[ -n "\$rename_kind" ]]; then
    source_directory="\$(cd "\$(dirname "\$source_path")" && pwd -P)"
    destination_directory="\$(cd "\$(dirname "\$destination_path")" && pwd -P)"
    source_device="\$(path_device "\$source_path")"
    destination_device="\$(path_device "\$destination_directory")"

    if [[ "\$source_directory" != "\$destination_directory" ]]; then
      echo "state-snapshot-selftest: cross-directory session rename: \$source_path -> \$destination_path" >&2
      exit 96
    fi
    if [[ "\$source_device" != "\$destination_device" ]]; then
      echo "state-snapshot-selftest: cross-device session rename: \$source_path -> \$destination_path" >&2
      exit 96
    fi

    printf '%s\n' "\$rename_kind:same-directory:same-device" >> "\$SNAPSHOT_RENAME_PROOF_FILE"
    if [[ "\${SNAPSHOT_RENAME_FAIL_KIND:-}" == "\$rename_kind" ]]; then
      exit 97
    fi
  fi
fi

exec "$REAL_MV" "\$@"
EOF
chmod 700 "$ATOMIC_BIN/mv"

run_post_capture_attack() {
  local case_name="$1"
  local attack_kind="$2"
  local captured_root="$TMP_ROOT/$case_name-captured"
  local attacker_root="$TMP_ROOT/$case_name-attacker"
  local session_id="snapshot-$case_name"
  local phase="phase_${case_name}_execute"
  local control_file
  local binding_packet
  local attacker_packet="$TMP_ROOT/$case_name-attacker.packet.json"
  local packet_hardlink="$TMP_ROOT/$case_name-original.packet.json"
  local capture_proof="$TMP_ROOT/$case_name-capture-proof"
  local snapshot_output
  local snapshot_rc

  prepare_bound_repo "$captured_root" "$session_id"
  control_file="$BOUND_CONTROL"
  binding_packet="$BOUND_PACKET"
  mkdir -p "$attacker_root/.specify/memory"
  jq --arg attacker_root "$attacker_root" '.repositoryRoot = $attacker_root' \
    "$binding_packet" > "$attacker_packet"
  ln "$binding_packet" "$packet_hardlink"

  set +e
  snapshot_output="$(PATH="$RACE_BIN:$PATH" \
    SNAPSHOT_ATTACK_KIND="$attack_kind" \
    SNAPSHOT_CALLER_PACKET="$binding_packet" \
    SNAPSHOT_ATTACKER_PACKET="$attacker_packet" \
    SNAPSHOT_CAPTURE_PROOF_FILE="$capture_proof" \
    BUBBLES_AGENT_NAME="bubbles.workflow" \
    "$RACE_BIN/bash" "$SNAPSHOT" \
    --session-id "$session_id" --session-control-file "$control_file" \
    --binding-packet-file "$binding_packet" \
    --phase "$phase" --mode start 2>&1)"
  snapshot_rc=$?
  set -e

  if [[ "$snapshot_rc" -eq 0 ]]; then
    pass "$attack_kind caller packet attack does not invalidate the captured snapshot"
  else
    fail "$attack_kind caller packet attack should preserve a successful snapshot (exit=$snapshot_rc)"
    echo "  output: $snapshot_output"
  fi

  if [[ "$(cat "$capture_proof" 2>/dev/null || true)" == "byte-identical-private-copy-mode-0600" ]]; then
    pass "$attack_kind packet capture is distinct, byte-identical, and mode 0600"
  else
    fail "$attack_kind packet capture did not satisfy private-copy invariants"
  fi

  if [[ "$(jq -r '.repositoryRoot' "$binding_packet")" == "$attacker_root" ]]; then
    pass "$attack_kind caller packet attack executes after private capture"
  else
    fail "$attack_kind caller packet attack did not select the attacker root"
  fi

  if { [[ "$attack_kind" == "in-place" && "$binding_packet" -ef "$packet_hardlink" ]] ||
       [[ "$attack_kind" == "replace" && ! "$binding_packet" -ef "$packet_hardlink" ]]; }; then
    pass "$attack_kind caller packet attack uses the intended inode semantics"
  else
    fail "$attack_kind caller packet attack did not use the intended inode semantics"
  fi

  if [[ -f "$captured_root/.specify/memory/bubbles.session.json" ]] &&
     jq -e --arg phase "$phase" \
       '.turnSnapshots | length == 1 and .[0].phase == $phase' \
       "$captured_root/.specify/memory/bubbles.session.json" >/dev/null; then
    pass "$attack_kind caller packet attack writes only to the captured repository root"
  else
    fail "$attack_kind caller packet attack did not write the expected captured snapshot"
  fi

  if [[ ! -e "$attacker_root/.specify/memory/bubbles.session.json" ]]; then
    pass "$attack_kind caller packet attack cannot write attacker-selected repository state"
  else
    fail "$attack_kind caller packet attack wrote attacker-selected repository state"
  fi
}

run_atomic_success_case() {
  local root="$TMP_ROOT/case6"
  local session_id="snapshot-case6"
  local capture_tmp="$TMP_ROOT/case6-private-captures"
  local rename_proof="$TMP_ROOT/case6-rename-proof"
  local control_file
  local binding_packet
  local session_file="$root/.specify/memory/bubbles.session.json"
  local snapshot_output
  local snapshot_rc

  prepare_bound_repo "$root" "$session_id"
  control_file="$BOUND_CONTROL"
  binding_packet="$BOUND_PACKET"
  mkdir -p "$capture_tmp"

  set +e
  snapshot_output="$(PATH="$ATOMIC_BIN:$PATH" \
    TMPDIR="$capture_tmp" \
    SNAPSHOT_RENAME_PROOF_FILE="$rename_proof" \
    BUBBLES_AGENT_NAME="bubbles.workflow" \
    bash "$SNAPSHOT" \
    --session-id "$session_id" --session-control-file "$control_file" \
    --binding-packet-file "$binding_packet" \
    --phase phase_case6_execute --mode start \
    --convergence-iteration 2 --spec-dir specs/case6 2>&1)"
  snapshot_rc=$?
  set -e

  if [[ "$snapshot_rc" -eq 0 ]]; then
    pass "normal and convergence snapshot updates complete through instrumented renames"
  else
    fail "normal and convergence snapshot updates should succeed (exit=$snapshot_rc)"
    echo "  output: $snapshot_output"
  fi

  # SCOPE-6: the snapshot must be self-describing about the posture that
  # produced it, so an audit never reconstructs the operator's shell.
  posture_recorded="$(jq -r '.autonomyPosture // "null"' "$session_file" 2>/dev/null)"
  if [[ -n "$posture_recorded" && "$posture_recorded" != "null" ]]; then
    pass "session state records the resolved autonomy posture (got: $posture_recorded)"
  else
    fail "session state should record a non-null autonomyPosture"
  fi

  turn_posture="$(jq -r '.turnSnapshots[-1].posture // "null"' "$session_file" 2>/dev/null)"
  if [[ "$turn_posture" == "$posture_recorded" ]]; then
    pass "the turn snapshot carries the same posture as the session record"
  else
    fail "turn snapshot posture ($turn_posture) should match session posture ($posture_recorded)"
  fi

  if [[ "$(file_line_count "$rename_proof")" == "2" ]]; then
    pass "exactly two session-file renames occur for a convergence snapshot"
  else
    fail "convergence snapshot should perform exactly two session-file renames"
  fi

  if grep -Fxq 'update:same-directory:same-device' "$rename_proof" 2>/dev/null; then
    pass "normal session update temp is renamed from the destination directory and device"
  else
    fail "normal session update did not prove same-directory same-device rename"
  fi

  if grep -Fxq 'convergence:same-directory:same-device' "$rename_proof" 2>/dev/null; then
    pass "convergence update temp is renamed from the destination directory and device"
  else
    fail "convergence update did not prove same-directory same-device rename"
  fi

  if jq -e \
    '.turnSnapshots | length == 1' "$session_file" >/dev/null 2>&1 &&
     jq -e \
       '.convergenceLoops | length == 1 and .[0].iterationCount == 2 and .[0].specDir == "specs/case6"' \
       "$session_file" >/dev/null 2>&1; then
    pass "instrumented atomic renames persist both snapshot and convergence state"
  else
    fail "instrumented atomic renames did not persist expected session state"
  fi

  if [[ "$(directory_entry_count "$capture_tmp")" == "0" ]]; then
    pass "successful snapshot removes its private packet capture"
  else
    fail "successful snapshot left private packet capture residue"
  fi

  if [[ "$(snapshot_update_temp_count "$(dirname "$session_file")")" == "0" ]]; then
    pass "successful snapshot leaves no session update temp files"
  else
    fail "successful snapshot left session update temp files"
  fi

  # SCOPE-5: an auto-resolved decision is recorded with the principle that fired.
  # Runs LAST in this case, and without the instrumented mv shim, so neither the
  # rename count nor the turnSnapshots==1 assertion above is disturbed.
  set +e
  BUBBLES_AGENT_NAME="bubbles.workflow" \
    bash "$SNAPSHOT" \
    --session-id "$session_id" --session-control-file "$control_file" \
    --binding-packet-file "$binding_packet" \
    --phase phase_case6_decision --mode start \
    --decision "chose the POSIX rewrite over installing gawk" \
    --decision-principle prefer_correct_over_green \
    --decision-chose posix-rewrite \
    --decision-considered "posix-rewrite, install-gawk, fail-loud-shim" >/dev/null 2>&1
  local decision_rc=$?
  set -e

  if [[ "$decision_rc" -eq 0 ]]; then
    pass "a snapshot carrying an auto-resolved decision succeeds"
  else
    fail "decision snapshot should succeed (exit=$decision_rc)"
  fi

  if [[ "$(jq -r '.autonomyDecisions | length' "$session_file" 2>/dev/null)" == "1" ]]; then
    pass "the auto-resolved decision is appended to autonomyDecisions[]"
  else
    fail "autonomyDecisions[] should hold exactly one entry"
  fi

  if [[ "$(jq -r '.autonomyDecisions[-1].principle' "$session_file" 2>/dev/null)" == "prefer_correct_over_green" ]]; then
    pass "the decision records the principle that fired"
  else
    fail "the decision should record its principle"
  fi

  # The discarded alternatives are what make the ledger auditable.
  if [[ "$(jq -r '.autonomyDecisions[-1].considered | length' "$session_file" 2>/dev/null)" == "3" ]]; then
    pass "the decision records every option considered, not just the winner"
  else
    fail "considered[] should hold 3 options"
  fi

  # A turn with no decision must not grow the ledger.
  if [[ "$(jq -r '.turnSnapshots | length' "$session_file" 2>/dev/null)" -gt "$(jq -r '.autonomyDecisions | length' "$session_file" 2>/dev/null)" ]]; then
    pass "turns without a decision leave autonomyDecisions[] untouched"
  else
    fail "a decision-free turn must not append to autonomyDecisions[]"
  fi

  set +e
  BUBBLES_AGENT_NAME="bubbles.workflow" \
    bash "$SNAPSHOT" \
    --session-id "$session_id" --session-control-file "$control_file" \
    --binding-packet-file "$binding_packet" \
    --phase phase_case6_orphan --mode start \
    --decision-principle orphaned >/dev/null 2>&1
  local orphan_rc=$?
  set -e
  if [[ "$orphan_rc" -eq 2 ]]; then
    pass "decision metadata without a decision is a usage error (exit 2)"
  else
    fail "decision metadata without --decision should exit 2 (got: $orphan_rc)"
  fi
}

run_binding_failure_cleanup_case() {
  local root="$TMP_ROOT/case7"
  local session_id="snapshot-case7"
  local capture_tmp="$TMP_ROOT/case7-private-captures"
  local binding_packet
  local snapshot_output
  local snapshot_rc

  prepare_bound_repo "$root" "$session_id"
  binding_packet="$BOUND_PACKET"
  mkdir -p "$capture_tmp"

  set +e
  snapshot_output="$(TMPDIR="$capture_tmp" \
    BUBBLES_AGENT_NAME="bubbles.workflow" \
    bash "$SNAPSHOT" \
    --session-id "$session_id" \
    --session-control-file "$TMP_ROOT/case7-missing-control.json" \
    --binding-packet-file "$binding_packet" \
    --phase phase_case7_execute --mode start 2>&1)"
  snapshot_rc=$?
  set -e

  if [[ "$snapshot_rc" -ne 0 ]]; then
    pass "binding validation failure exits non-zero after private capture"
  else
    fail "missing session control should fail binding validation"
    echo "  output: $snapshot_output"
  fi

  if [[ "$(directory_entry_count "$capture_tmp")" == "0" ]]; then
    pass "binding validation failure removes its private packet capture"
  else
    fail "binding validation failure left private packet capture residue"
  fi

  if [[ ! -e "$root/.specify/memory/bubbles.session.json" ]]; then
    pass "binding validation failure performs no repository-local session write"
  else
    fail "binding validation failure wrote repository-local session state"
  fi
}

run_rename_failure_cleanup_case() {
  local case_name="$1"
  local fail_kind="$2"
  local root="$TMP_ROOT/$case_name"
  local session_id="snapshot-$case_name"
  local capture_tmp="$TMP_ROOT/$case_name-private-captures"
  local rename_proof="$TMP_ROOT/$case_name-rename-proof"
  local control_file
  local binding_packet
  local session_directory="$root/.specify/memory"
  local snapshot_output
  local snapshot_rc
  local -a convergence_args=()

  prepare_bound_repo "$root" "$session_id"
  control_file="$BOUND_CONTROL"
  binding_packet="$BOUND_PACKET"
  mkdir -p "$capture_tmp"
  if [[ "$fail_kind" == "convergence" ]]; then
    convergence_args=(--convergence-iteration 3 --spec-dir "specs/$case_name")
  fi

  set +e
  snapshot_output="$(PATH="$ATOMIC_BIN:$PATH" \
    TMPDIR="$capture_tmp" \
    SNAPSHOT_RENAME_PROOF_FILE="$rename_proof" \
    SNAPSHOT_RENAME_FAIL_KIND="$fail_kind" \
    BUBBLES_AGENT_NAME="bubbles.workflow" \
    bash "$SNAPSHOT" \
    --session-id "$session_id" --session-control-file "$control_file" \
    --binding-packet-file "$binding_packet" \
    --phase "phase_${case_name}_execute" --mode start \
    "${convergence_args[@]}" 2>&1)"
  snapshot_rc=$?
  set -e

  if [[ "$snapshot_rc" -eq 97 ]]; then
    pass "$fail_kind rename failure propagates the injected non-zero status"
  else
    fail "$fail_kind rename failure should exit 97 (got exit=$snapshot_rc)"
    echo "  output: $snapshot_output"
  fi

  if grep -Fxq "$fail_kind:same-directory:same-device" "$rename_proof" 2>/dev/null; then
    pass "$fail_kind failure is injected after same-directory same-device placement proof"
  else
    fail "$fail_kind failure did not reach the instrumented session rename"
  fi

  if [[ "$(directory_entry_count "$capture_tmp")" == "0" ]]; then
    pass "$fail_kind rename failure removes its private packet capture"
  else
    fail "$fail_kind rename failure left private packet capture residue"
  fi

  if [[ "$(snapshot_update_temp_count "$session_directory")" == "0" ]]; then
    pass "$fail_kind rename failure removes all session update temp files"
  else
    fail "$fail_kind rename failure left session update temp files"
  fi

  if [[ "$fail_kind" == "convergence" ]]; then
    if jq -e \
      '(.turnSnapshots | length) == 1 and (((.convergenceLoops // []) | length) == 0)' \
      "$session_directory/bubbles.session.json" >/dev/null 2>&1; then
      pass "convergence rename failure preserves the completed normal update only"
    else
      fail "convergence rename failure did not preserve the expected normal update boundary"
    fi
  fi
}

echo "Running state-snapshot selftest..."
echo "Scenario: orchestrator agents must record a per-turn snapshot in .specify/memory/bubbles.session.json without ever losing prior records."

# ---- Case 1: append to fresh session JSON ---------------------------------

case1_root="$TMP_ROOT/case1"
case1_session_id="snapshot-case1"
prepare_bound_repo "$case1_root" "$case1_session_id"
case1_control="$BOUND_CONTROL"
case1_packet="$BOUND_PACKET"
# Note: empty/missing session file should be created by the snapshot script.

case1_out="$(BUBBLES_AGENT_NAME="bubbles.workflow" bash "$SNAPSHOT" \
  --session-id "$case1_session_id" --session-control-file "$case1_control" \
  --binding-packet-file "$case1_packet" \
  --phase phase_1_plan --mode start --note "starting batch 2A" 2>&1)"

case1_session="$case1_root/.specify/memory/bubbles.session.json"

if [[ -f "$case1_session" ]]; then
  pass "Snapshot creates session JSON file when missing"
else
  fail "Snapshot must create session JSON file when missing"
  echo "  output: $case1_out"
fi

case1_count="$(jq '.turnSnapshots | length' "$case1_session" 2>/dev/null || echo -1)"
if [[ "$case1_count" == "1" ]]; then
  pass "First snapshot creates exactly one turnSnapshots entry"
else
  fail "First snapshot should create exactly one turnSnapshots entry (got $case1_count)"
fi

case1_turn="$(jq '.turnSnapshots[0].turnNumber' "$case1_session" 2>/dev/null || echo -1)"
if [[ "$case1_turn" == "1" ]]; then
  pass "First snapshot turnNumber is 1"
else
  fail "First snapshot turnNumber should be 1 (got $case1_turn)"
fi

case1_fields_ok=true
for field in phase mode agent note timestamp; do
  v="$(jq -r ".turnSnapshots[0].$field" "$case1_session" 2>/dev/null || echo "")"
  if [[ -z "$v" || "$v" == "null" ]]; then
    case1_fields_ok=false
    fail "Required field '$field' missing or null in first snapshot record"
  fi
done
if $case1_fields_ok; then
  pass "All required fields (phase, mode, agent, note, timestamp) present in first snapshot"
fi

case1_phase="$(jq -r '.turnSnapshots[0].phase' "$case1_session")"
case1_mode="$(jq -r '.turnSnapshots[0].mode' "$case1_session")"
case1_agent="$(jq -r '.turnSnapshots[0].agent' "$case1_session")"
case1_scope="$(jq -r '.turnSnapshots[0].scopeId' "$case1_session")"
if [[ "$case1_phase" == "phase_1_plan" \
   && "$case1_mode" == "start" \
   && "$case1_agent" == "bubbles.workflow" \
   && "$case1_scope" == "null" ]]; then
  pass "First snapshot record carries the supplied phase/mode/agent and null scopeId"
else
  fail "First snapshot record fields did not match supplied values"
  echo "  phase=$case1_phase mode=$case1_mode agent=$case1_agent scopeId=$case1_scope"
fi

# ---- Case 2: append to existing session JSON ------------------------------

case2_root="$TMP_ROOT/case2"
case2_session_id="snapshot-case2"
prepare_bound_repo "$case2_root" "$case2_session_id"
case2_control="$BOUND_CONTROL"
case2_packet="$BOUND_PACKET"
case2_session="$case2_root/.specify/memory/bubbles.session.json"

# Seed an existing session JSON with a non-snapshot field that MUST be
# preserved across the append, plus a pre-existing turnSnapshots entry.
cat > "$case2_session" <<'JSON'
{
  "sessionId": "session-existing",
  "turnSnapshots": [
    {
      "turnNumber": 1,
      "timestamp": "2026-01-01T00:00:00Z",
      "phase": "prior_phase",
      "scopeId": "scope-prior",
      "mode": "start",
      "note": "pre-existing record",
      "agent": "bubbles.goal"
    }
  ]
}
JSON

BUBBLES_AGENT_NAME="bubbles.sprint" bash "$SNAPSHOT" \
  --session-id "$case2_session_id" --session-control-file "$case2_control" \
  --binding-packet-file "$case2_packet" \
  --phase phase_2_plan --scope-id scope-A --mode start >/dev/null

case2_count="$(jq '.turnSnapshots | length' "$case2_session")"
if [[ "$case2_count" == "2" ]]; then
  pass "Append to existing session yields exactly 2 turnSnapshots entries"
else
  fail "Append to existing session should yield 2 entries (got $case2_count)"
fi

case2_turn_new="$(jq '.turnSnapshots[1].turnNumber' "$case2_session")"
if [[ "$case2_turn_new" == "2" ]]; then
  pass "Appended record turnNumber correctly increments to 2"
else
  fail "Appended record turnNumber should be 2 (got $case2_turn_new)"
fi

case2_prior_phase="$(jq -r '.turnSnapshots[0].phase' "$case2_session")"
case2_prior_note="$(jq -r '.turnSnapshots[0].note' "$case2_session")"
if [[ "$case2_prior_phase" == "prior_phase" && "$case2_prior_note" == "pre-existing record" ]]; then
  pass "Pre-existing turnSnapshots[0] record is preserved verbatim"
else
  fail "Pre-existing turnSnapshots[0] record was modified"
  echo "  phase=$case2_prior_phase note=$case2_prior_note"
fi

case2_session_id="$(jq -r '.sessionId' "$case2_session")"
if [[ "$case2_session_id" == "session-existing" ]]; then
  pass "Non-snapshot session fields (sessionId) preserved across append"
else
  fail "Non-snapshot session fields should be preserved (got sessionId=$case2_session_id)"
fi

# ---- Case 3: --mode end after --mode start --------------------------------

case3_root="$TMP_ROOT/case3"
case3_session_id="snapshot-case3"
prepare_bound_repo "$case3_root" "$case3_session_id"
case3_control="$BOUND_CONTROL"
case3_packet="$BOUND_PACKET"
case3_session="$case3_root/.specify/memory/bubbles.session.json"

BUBBLES_AGENT_NAME="bubbles.iterate" bash "$SNAPSHOT" \
  --session-id "$case3_session_id" --session-control-file "$case3_control" \
  --binding-packet-file "$case3_packet" \
  --phase phase_3_execute --scope-id scope-X --mode start >/dev/null
BUBBLES_AGENT_NAME="bubbles.iterate" bash "$SNAPSHOT" \
  --session-id "$case3_session_id" --session-control-file "$case3_control" \
  --binding-packet-file "$case3_packet" \
  --phase phase_3_execute --scope-id scope-X --mode end >/dev/null

case3_count="$(jq '.turnSnapshots | length' "$case3_session")"
if [[ "$case3_count" == "2" ]]; then
  pass "start+end snapshot pair produces exactly 2 records"
else
  fail "start+end pair should produce 2 records (got $case3_count)"
fi

case3_mode_a="$(jq -r '.turnSnapshots[0].mode' "$case3_session")"
case3_mode_b="$(jq -r '.turnSnapshots[1].mode' "$case3_session")"
case3_scope_a="$(jq -r '.turnSnapshots[0].scopeId' "$case3_session")"
case3_scope_b="$(jq -r '.turnSnapshots[1].scopeId' "$case3_session")"

if [[ "$case3_mode_a" == "start" && "$case3_mode_b" == "end" ]]; then
  pass "start+end pair records modes in the correct order"
else
  fail "start+end pair should record mode start then end (got $case3_mode_a, $case3_mode_b)"
fi

if [[ "$case3_scope_a" == "scope-X" && "$case3_scope_b" == "scope-X" ]]; then
  pass "start+end pair preserves matching scopeId across both records"
else
  fail "start+end pair scopeId mismatch (start=$case3_scope_a, end=$case3_scope_b)"
fi

case3_turn_a="$(jq '.turnSnapshots[0].turnNumber' "$case3_session")"
case3_turn_b="$(jq '.turnSnapshots[1].turnNumber' "$case3_session")"
if [[ "$case3_turn_a" == "1" && "$case3_turn_b" == "2" ]]; then
  pass "start+end pair turnNumbers increment correctly (1 → 2)"
else
  fail "start+end pair turnNumbers should be 1 and 2 (got $case3_turn_a, $case3_turn_b)"
fi

# ---- Case 4: caller mutates packet bytes after private capture -------------

run_post_capture_attack "case4" "in-place"

# ---- Case 5: caller atomically replaces packet after private capture -------

run_post_capture_attack "case5" "replace"

# ---- Case 6: successful cleanup and same-directory atomic renames ----------

run_atomic_success_case

# ---- Case 7: binding validation failure cleanup ----------------------------

run_binding_failure_cleanup_case

# ---- Case 8: normal update rename failure cleanup --------------------------

run_rename_failure_cleanup_case "case8" "update"

# ---- Case 9: convergence update rename failure cleanup ---------------------

run_rename_failure_cleanup_case "case9" "convergence"

# ---- Case 10: missing required --phase flag -------------------------------

case10_root="$TMP_ROOT/case10"
mkdir -p "$case10_root/.specify/memory"
set +e
case10_out="$(bash "$SNAPSHOT" --mode start 2>&1)"
case10_exit=$?
set -e

if [[ "$case10_exit" -ne 0 ]]; then
  pass "Missing --phase flag exits non-zero (exit=$case10_exit)"
else
  fail "Missing --phase flag should exit non-zero (got exit=$case10_exit)"
  echo "  output: $case10_out"
fi

if printf '%s' "$case10_out" | grep -q -- '--phase is required'; then
  pass "Missing --phase flag prints a clear error message on stderr"
else
  fail "Missing --phase flag should print a clear error message"
  echo "  output: $case10_out"
fi

# ---- Case 12: goal identity is DERIVED, never caller-supplied (IMP-038 S3) --
#
# A snapshot must record the goal the turn actually ran under. Deriving it from
# `.goalContract` in the same atomic read makes that unforgeable; a flag would
# let a caller record a ref that disagrees with the contract in force, which is
# exactly the substitution this field exists to expose.

cases=$((cases + 1))
case12_root="$TMP_ROOT/case12"
case12_session_id="snapshot-case12"
prepare_bound_repo "$case12_root" "$case12_session_id"
case12_control="$BOUND_CONTROL"
case12_packet="$BOUND_PACKET"
case12_session="$case12_root/.specify/memory/bubbles.session.json"

# No contract yet: a read-only or pre-IMP-038 run must still snapshot cleanly.
BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case12_session_id" --session-control-file "$case12_control" \
  --binding-packet-file "$case12_packet" \
  --phase phase_1_understand --mode start >/dev/null

if [[ "$(jq -r '.turnSnapshots[-1].goalRef' "$case12_session")" == "null" ]]; then
  pass "A turn with no frozen Goal Contract records goalRef as null"
else
  fail "goalRef must be null when no contract is frozen (got: $(jq -c '.turnSnapshots[-1].goalRef' "$case12_session"))"
fi

# Freeze a contract, then snapshot again: the ref must appear, derived.
case12_request="$TMP_ROOT/case12-request.txt"
printf 'freeze then snapshot\n' > "$case12_request"
bash "$SCRIPT_DIR/goal-contract.sh" freeze \
  --session-file "$case12_session" \
  --source-request-file "$case12_request" \
  --intent "thread goal identity through snapshots" \
  --success-signal "state-snapshot-selftest exits 0" \
  --target "spec=specs/038-goal-fidelity" \
  --repository-root bubbles \
  --spec-target specs/038-goal-fidelity \
  --allowed-path 'bubbles/scripts/**' \
  --runner bubbles.goal \
  --session-id "$case12_session_id" \
  --repository-alias bubbles >/dev/null 2>&1

BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case12_session_id" --session-control-file "$case12_control" \
  --binding-packet-file "$case12_packet" \
  --phase phase_3_execute --mode start \
  --convergence-iteration 1 --spec-dir specs/038-goal-fidelity >/dev/null

if jq -e '
  .turnSnapshots[-1].goalRef as $r
  | $r.goalId == .goalContract.goalId
    and $r.revision == .goalContract.revision
    and $r.sourceRequestDigest == .goalContract.sourceRequestDigest
    and $r.workBoundary == .goalContract.workBoundary
' "$case12_session" >/dev/null 2>&1; then
  pass "A turn snapshot derives goalRef from the frozen contract, boundary included"
else
  fail "turn snapshot goalRef must match the frozen contract (got: $(jq -c '.turnSnapshots[-1].goalRef' "$case12_session"))"
fi

if jq -e '
  .turnSnapshots[-1].goalRef
  | (has("intent") or has("successSignal") or has("hardConstraints") or has("targets")) | not
' "$case12_session" >/dev/null 2>&1; then
  pass "A turn snapshot carries identity + boundary only, no contract prose (R5)"
else
  fail "turn snapshot goalRef leaked contract prose: $(jq -c '.turnSnapshots[-1].goalRef | keys_unsorted' "$case12_session")"
fi

if jq -e '
  .convergenceLoops[-1].goalRef as $r
  | $r.goalId == .goalContract.goalId and $r.revision == .goalContract.revision
' "$case12_session" >/dev/null 2>&1; then
  pass "A convergence-loop entry carries the same derived goalRef"
else
  fail "convergenceLoops goalRef must match the frozen contract (got: $(jq -c '.convergenceLoops[-1].goalRef' "$case12_session"))"
fi

# The earlier no-contract snapshot must NOT be retroactively rewritten: turn
# history records what was true at each turn, not what is true now.
if [[ "$(jq -r '.turnSnapshots[0].goalRef' "$case12_session")" == "null" ]]; then
  pass "Freezing a contract does not retroactively rewrite earlier snapshots"
else
  fail "earlier snapshot was rewritten: $(jq -c '.turnSnapshots[0].goalRef' "$case12_session")"
fi

# There is no flag to supply a ref, so a caller cannot record a wrong one.
case12_flag_exit=0
BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case12_session_id" --session-control-file "$case12_control" \
  --binding-packet-file "$case12_packet" \
  --phase phase_3_execute --mode start --goal-ref '{"goalId":"gc:forged:1"}' >/dev/null 2>&1 \
  || case12_flag_exit=$?
if [[ "$case12_flag_exit" -eq 2 ]]; then
  pass "state-snapshot accepts no --goal-ref flag, so a ref cannot be forged"
else
  fail "state-snapshot must reject --goal-ref (got exit=$case12_flag_exit)"
fi

# ---- Case 13: goal-node snapshot carries its compiled declaration ----------

cases=$((cases + 1))
case13_root="$TMP_ROOT/case13"
case13_session_id="snapshot-case13"
case13_node_id="fix-goal-node-state-snapshot-binding"
case13_spec_dir="specs/bug-015"
prepare_bound_repo "$case13_root" "$case13_session_id"
case13_control="$BOUND_CONTROL"
case13_packet="$BOUND_PACKET"
case13_goal_packet="$TMP_ROOT/case13-goal.packet.json"
case13_scenario="$TMP_ROOT/case13-scenario.json"
case13_session="$case13_root/.specify/memory/bubbles.session.json"
case13_control_baseline="$TMP_ROOT/case13-control-baseline.json"

jq --arg session "$case13_session_id" --arg scope "$case13_node_id" '
  .repositoryResolution.authority = "scoped-scenario-node"
  | .repositoryResolution.transition = "scoped-override"
  | .repositoryResolution.scopeKind = "goal-node"
  | .repositoryResolution.scopeId = $scope
  | .repositoryResolution.targetKind = "goal-node"
  | .repositoryResolution.decisionId = (
      "rb:" + $session + ":" + (.repositoryResolution.controlRevision | tostring)
      + ":node:" + $scope
    )
' "$case13_packet" > "$case13_goal_packet"

case13_alias="$(jq -r '.repositoryAlias' "$case13_goal_packet")"
jq -n \
  --arg root "$case13_root" \
  --arg alias "$case13_alias" \
  --arg node "$case13_node_id" \
  --slurpfile packet "$case13_goal_packet" \
  '{
    version: 1,
    scenarioId: "bug-015-goal-node-snapshot",
    repos: [{
      id: "bubbles-source",
      role: "framework",
      repositoryRoot: $root,
      repositoryAlias: $alias
    }],
    nodes: [{
      id: $node,
      type: "delivery",
      repo: "bubbles-source",
      mode: "bugfix-fastlane",
      dependsOn: [],
      repositoryResolution: $packet[0].repositoryResolution
    }]
  }' > "$case13_scenario"

jq -n \
  --arg spec "$case13_spec_dir" \
  '{
    turnSnapshots: [{
      turnNumber: 1,
      timestamp: "2026-08-10T00:00:00Z",
      phase: "phase_before_bug_015",
      scopeId: "prior-scope",
      mode: "start",
      note: "preserve this snapshot",
      agent: "bubbles.goal",
      goalRef: null
    }],
    convergenceLoops: [
      {
        specDir: $spec,
        agent: "bubbles.goal",
        iterationCount: 1,
        goalRef: null
      },
      {
        specDir: "specs/unrelated",
        agent: "bubbles.workflow",
        iterationCount: 9,
        goalRef: {goalId: "unrelated", revision: 3},
        marker: "preserve-byte-equivalent"
      }
    ]
  }' > "$case13_session"

case13_prior_snapshot="$(jq -c '.turnSnapshots[0]' "$case13_session")"
case13_unrelated_loop="$(jq -c '.convergenceLoops[] | select(.specDir == "specs/unrelated")' "$case13_session")"
cp "$case13_control" "$case13_control_baseline"

set +e
case13_out="$(BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case13_session_id" --session-control-file "$case13_control" \
  --binding-packet-file "$case13_goal_packet" \
  --scenario-file "$case13_scenario" --node-id "$case13_node_id" \
  --phase phase_5_remediate --scope-id bug-015 --mode start \
  --convergence-iteration 4 --spec-dir "$case13_spec_dir" 2>&1)"
case13_exit=$?
set -e

if [[ "$case13_exit" -eq 0 ]]; then
  pass "Goal-node snapshot accepts and forwards the matching scenario/node pair"
else
  fail "Goal-node snapshot should accept the matching scenario/node pair (exit=$case13_exit)"
  echo "  output: $case13_out"
fi

if jq -e --argjson prior "$case13_prior_snapshot" '
  (.turnSnapshots | length) == 2
  and .turnSnapshots[0] == $prior
  and .turnSnapshots[1].turnNumber == 2
  and .turnSnapshots[1].phase == "phase_5_remediate"
  and .turnSnapshots[1].scopeId == "bug-015"
  and .turnSnapshots[1].agent == "bubbles.goal"
' "$case13_session" >/dev/null 2>&1; then
  pass "Goal-node snapshot preserves existing turns and appends exactly one record"
else
  fail "Goal-node snapshot must preserve existing turns and append exactly one record"
fi

if jq -e \
  --arg spec "$case13_spec_dir" \
  --argjson unrelated "$case13_unrelated_loop" '
  (.convergenceLoops | length) == 2
  and ([.convergenceLoops[] | select(.specDir == $spec and .agent == "bubbles.goal")] | length) == 1
  and (.convergenceLoops[] | select(.specDir == $spec and .agent == "bubbles.goal") | .iterationCount) == 4
  and (.convergenceLoops[] | select(.specDir == "specs/unrelated")) == $unrelated
' "$case13_session" >/dev/null 2>&1; then
  pass "Goal-node convergence updates only the matching entry and preserves unrelated entries"
else
  fail "Goal-node convergence must update only the matching entry and preserve unrelated entries"
fi

if cmp -s "$case13_control_baseline" "$case13_control"; then
  pass "Goal-node snapshot leaves command-level external control byte-identical"
else
  fail "Goal-node snapshot must leave command-level external control byte-identical"
fi

case13_session_baseline="$TMP_ROOT/case13-session-baseline.json"
cp "$case13_session" "$case13_session_baseline"
case13_entry_count="$(directory_entry_count "$(dirname "$case13_session")")"

set +e
case13_wrong_node_out="$(BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case13_session_id" --session-control-file "$case13_control" \
  --binding-packet-file "$case13_goal_packet" \
  --scenario-file "$case13_scenario" --node-id absent-node \
  --phase phase_5_remediate --scope-id bug-015 --mode start \
  --convergence-iteration 5 --spec-dir "$case13_spec_dir" 2>&1)"
case13_wrong_node_exit=$?
set -e
if [[ "$case13_wrong_node_exit" -eq 1 ]] && \
   printf '%s' "$case13_wrong_node_out" | grep -q 'GOAL_NODE_DECLARATION_INVALID'; then
  pass "Goal-node snapshot refuses a node absent from the compiled scenario"
else
  fail "Goal-node snapshot should refuse an absent node with GOAL_NODE_DECLARATION_INVALID"
  echo "  exit=$case13_wrong_node_exit output: $case13_wrong_node_out"
fi
if cmp -s "$case13_session_baseline" "$case13_session"; then
  pass "Wrong-node snapshot refusal writes no mirror, turn snapshot, or convergence entry"
else
  fail "Wrong-node snapshot refusal must leave repository session state byte-identical"
fi

set +e
case13_scenario_only_out="$(BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case13_session_id" --session-control-file "$case13_control" \
  --binding-packet-file "$case13_goal_packet" \
  --scenario-file "$case13_scenario" \
  --phase phase_5_remediate --scope-id bug-015 --mode start 2>&1)"
case13_scenario_only_exit=$?
set -e
if [[ "$case13_scenario_only_exit" -eq 2 ]] && \
   printf '%s' "$case13_scenario_only_out" | grep -q -- '--scenario-file requires --node-id'; then
  pass "Scenario-only snapshot returns usage status 2 before repository writes"
else
  fail "Scenario-only snapshot should return usage status 2 with a paired-argument error"
fi
if cmp -s "$case13_session_baseline" "$case13_session" && \
   [[ "$(directory_entry_count "$(dirname "$case13_session")")" == "$case13_entry_count" ]]; then
  pass "Scenario-only snapshot leaves repository session files byte-identical"
else
  fail "Scenario-only snapshot must not create or modify repository session files"
fi

set +e
case13_node_only_out="$(BUBBLES_AGENT_NAME="bubbles.goal" bash "$SNAPSHOT" \
  --session-id "$case13_session_id" --session-control-file "$case13_control" \
  --binding-packet-file "$case13_goal_packet" \
  --node-id "$case13_node_id" \
  --phase phase_5_remediate --scope-id bug-015 --mode start 2>&1)"
case13_node_only_exit=$?
set -e
if [[ "$case13_node_only_exit" -eq 2 ]] && \
   printf '%s' "$case13_node_only_out" | grep -q -- '--node-id requires --scenario-file'; then
  pass "Node-only snapshot returns usage status 2 before repository writes"
else
  fail "Node-only snapshot should return usage status 2 with a paired-argument error"
fi
if cmp -s "$case13_session_baseline" "$case13_session" && \
   [[ "$(directory_entry_count "$(dirname "$case13_session")")" == "$case13_entry_count" ]]; then
  pass "Node-only snapshot leaves repository session files byte-identical"
else
  fail "Node-only snapshot must not create or modify repository session files"
fi

if cmp -s "$case13_control_baseline" "$case13_control"; then
  pass "All goal-node snapshot refusals leave external control byte-identical"
else
  fail "Goal-node snapshot refusals must leave external control byte-identical"
fi

# ---- Case 11: --help contract ----------------------------------------------

if "$SNAPSHOT" --help >/dev/null 2>&1; then
  pass "--help exits 0"
else
  fail "--help should exit 0"
fi

if "$SNAPSHOT" --help 2>/dev/null | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi

# ---- Case 12: --context-boundary argument contract (IMP-039 SCOPE-4) --------
#
# Argument validation runs before any repository binding, so these cases need no
# fixture. They are the adversarial half: an unrecognized kind and a checkpoint
# claim with no id are precisely the two shapes a fabricated boundary takes, and
# Gate G083 would otherwise be the only thing standing between them and the
# session file.
cases=$((cases + 1))

set +e
cb_missing_out="$("$SNAPSHOT" --context-boundary 2>&1)"; cb_missing_rc=$?
cb_badkind_out="$("$SNAPSHOT" --context-boundary handled --phase p 2>&1)"; cb_badkind_rc=$?
cb_noid_out="$("$SNAPSHOT" --context-boundary host-checkpoint --phase p 2>&1)"; cb_noid_rc=$?
set -e

if [[ "$cb_missing_rc" -eq 2 ]] && printf '%s' "$cb_missing_out" | grep -q 'requires a value'; then
  pass "--context-boundary without a value is a usage error"
else
  fail "--context-boundary without a value should exit 2 (got $cb_missing_rc)"
fi

if [[ "$cb_badkind_rc" -eq 2 ]] &&
  printf '%s' "$cb_badkind_out" | grep -q 'host-checkpoint, fresh-context or unavailable'; then
  pass "an unrecognized boundary kind is rejected with the allowed set"
else
  fail "an unrecognized boundary kind should exit 2 naming the allowed kinds (got $cb_badkind_rc)"
fi

if [[ "$cb_noid_rc" -eq 2 ]] &&
  printf '%s' "$cb_noid_out" | grep -q 'requires a checkpoint id'; then
  pass "host-checkpoint without a checkpoint id is rejected"
else
  fail "host-checkpoint without an id should exit 2 (got $cb_noid_rc)"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "state-snapshot selftest failed with $failures issue(s) across $assertions assertions in $cases cases."
  exit 1
fi
echo "state-snapshot selftest passed with $assertions assertions across $cases cases."
