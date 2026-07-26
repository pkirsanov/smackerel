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

TMP_ROOT="$(mktemp -d)"
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

if [[ "$failures" -gt 0 ]]; then
  echo "state-snapshot selftest failed with $failures issue(s) across $assertions assertions in $cases cases."
  exit 1
fi
echo "state-snapshot selftest passed with $assertions assertions across $cases cases."
