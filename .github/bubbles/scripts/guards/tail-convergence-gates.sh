# shellcheck shell=bash
# shellcheck disable=SC2154  # sourced fragment: all referenced vars are set in state-transition-guard.sh's scope before sourcing
# =============================================================================
# guards/tail-convergence-gates.sh  (M4 guard split)
# =============================================================================
# Checks 23-25: convergence cap (G082), compaction discipline (G083), and
# pre-existing deferral block (G084), plus Check 40: session cap (G128) — the
# AGGREGATE sibling of G082 added with IMP-003. Sourced by
# state-transition-guard.sh in the same shell scope, so pass/fail/warn/info, the
# failures/warnings counters, and computed vars ($SCRIPT_DIR, $feature_dir,
# fixture_gate_skip, run_guard_in_feature_repo) are all in scope exactly as
# before extraction. Checks 23-25 remain byte-identical to the previous inline
# blocks; Check 40 is additive and is a NO-OP unless a `sessionBudget` is
# recorded in `.specify/memory/bubbles.session.json`.
# =============================================================================

b037_validate_blocking_authority() {
  B037_AUTH_SESSION=""
  local session_id="${BUBBLES_SESSION_ID:-}"
  local control_file="${BUBBLES_SESSION_CONTROL_FILE:-}"
  local packet_file="${BUBBLES_BINDING_PACKET_FILE:-}"
  local scenario_file="${BUBBLES_BINDING_SCENARIO_FILE:-}"
  local node_id="${BUBBLES_BINDING_NODE_ID:-}"
  local validator="$SCRIPT_DIR/repository-binding.sh"
  local validator_output=""
  local validator_args=()

  [[ -n "$session_id" && -n "$control_file" && -n "$packet_file" ]] || return 2
  if [[ -n "$scenario_file" || -n "$node_id" ]]; then
    [[ -n "$scenario_file" && -n "$node_id" ]] || return 2
  fi
  [[ -f "$validator" && ! -L "$validator" && -x "$validator" ]] || return 2

  validator_args=(validate-packet
    --session-id "$session_id"
    --session-control-file "$control_file"
    --packet-file "$packet_file")
  if [[ -n "$scenario_file" ]]; then
    validator_args+=(--scenario-file "$scenario_file" --node-id "$node_id")
  fi
  validator_args+=(--emit-redacted-projection)

  if ! validator_output="$("$BASH" "$validator" "${validator_args[@]}" 2>&1)"; then
    return 2
  fi
  case "$validator_output" in
    *$'\n'*) return 2 ;;
  esac
  if ! jq -e --arg sessionId "$session_id" '
      type == "object"
      and .repositoryRoot == "<redacted-local-root>"
      and .repositoryResolution.sessionId == $sessionId
      and .repositoryResolution.pathVisibility == "redacted"
      and .repositoryResolution.actionable == false
    ' <<<"$validator_output" >/dev/null 2>&1; then
    return 2
  fi

  B037_AUTH_SESSION="$session_id"
}

b037_run_stable_guard() {
  local guard_repo_root="$1"
  local guard_name="$2"
  local output_file="$3"
  shift 3
  local state_helper="$SCRIPT_DIR/session-state-io.py"
  local python_bin=""
  local source_path="$SCRIPT_DIR/$guard_name"
  local capture_dir=""
  local before_path=""
  local preflight_path=""
  local after_path=""
  local shim_dir=""
  local dirname_shim=""
  local real_dirname=""
  local before_meta=""
  local preflight_meta=""
  local after_meta=""
  local child_rc=9

  python_bin="$(command -v python3 2>/dev/null || true)"
  real_dirname="$(command -v dirname 2>/dev/null || true)"
  [[ -n "$python_bin" && -n "$real_dirname" ]] || return 9
  [[ -f "$state_helper" && ! -L "$state_helper" ]] || return 9
  [[ -f "$source_path" && ! -L "$source_path" && -x "$source_path" ]] || return 9
  [[ -f "$output_file" && ! -L "$output_file" ]] || return 9

  capture_dir="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-stable-guard.XXXXXX" 2>/dev/null || true)"
  [[ -n "$capture_dir" && -d "$capture_dir" ]] || return 9
  before_path="$capture_dir/guard.before.sh"
  preflight_path="$capture_dir/guard.preflight.sh"
  after_path="$capture_dir/guard.after.sh"
  shim_dir="$capture_dir/bin"
  dirname_shim="$shim_dir/dirname"
  mkdir "$shim_dir" || {
    rm -rf "$capture_dir"
    return 9
  }

  if ! before_meta="$("$python_bin" "$state_helper" capture \
      --root "$SCRIPT_DIR" --relative-path "$guard_name" \
      --destination "$before_path" 2>/dev/null)" ||
    [[ ! -x "$source_path" || -L "$source_path" ]] ||
    ! preflight_meta="$("$python_bin" "$state_helper" capture \
      --root "$SCRIPT_DIR" --relative-path "$guard_name" \
      --destination "$preflight_path" 2>/dev/null)" ||
    [[ ! -x "$source_path" || -L "$source_path" ]] ||
    ! jq -e --argjson other "$preflight_meta" '
      type == "object" and .status == "captured"
      and ($other | type == "object" and .status == "captured")
      and .revision == $other.revision
      and .device == $other.device
      and .inode == $other.inode
    ' <<<"$before_meta" >/dev/null 2>&1; then
    rm -rf "$capture_dir"
    return 9
  fi

  cat >"$dirname_shim" <<'EOF'
#!/bin/sh
if [ "$#" -eq 1 ] && [ "$1" = "$BUBBLES_STABLE_SCRIPT_PATH" ]; then
  printf '.\n'
else
  exec "$BUBBLES_STABLE_REAL_DIRNAME" "$@"
fi
EOF
  chmod 700 "$dirname_shim" || {
    rm -rf "$capture_dir"
    return 9
  }

  (
    cd "$SCRIPT_DIR" || exit 9
    PATH="$shim_dir:$PATH" \
      BUBBLES_STABLE_SCRIPT_PATH="$before_path" \
      BUBBLES_STABLE_REAL_DIRNAME="$real_dirname" \
      BUBBLES_REPO_ROOT="$guard_repo_root" \
      "$BASH" "$before_path" "$@"
  ) >"$output_file" 2>&1
  child_rc=$?

  if ! after_meta="$("$python_bin" "$state_helper" capture \
      --root "$SCRIPT_DIR" --relative-path "$guard_name" \
      --destination "$after_path" 2>/dev/null)" ||
    [[ ! -x "$source_path" || -L "$source_path" ]] ||
    ! jq -e --argjson other "$after_meta" '
      type == "object" and .status == "captured"
      and ($other | type == "object" and .status == "captured")
      and .revision == $other.revision
      and .device == $other.device
      and .inode == $other.inode
    ' <<<"$before_meta" >/dev/null 2>&1; then
    rm -rf "$capture_dir"
    return 9
  fi

  rm -rf "$capture_dir"
  return "$child_rc"
}

b037_parse_guard_result() {
  local guard_name="$1"
  local process_exit="$2"
  local session_id="$3"
  local spec_dir="$4"
  local output_file="$5"
  local python_bin=""

  python_bin="$(command -v python3 2>/dev/null || true)"
  [[ -n "$python_bin" && -f "$output_file" && ! -L "$output_file" ]] || return 9
  "$python_bin" -c '
import json
import re
import sys
from pathlib import Path

guard, process_exit_raw, session_id, spec_dir, output_path = sys.argv[1:]
try:
    process_exit = int(process_exit_raw)
    raw = Path(output_path).read_bytes()
    if b"\x00" in raw or b"\r" in raw:
        raise ValueError
    text = raw.decode("utf-8")
    candidates = [line for line in text.split("\n") if line.startswith(guard + " status")]
    if len(candidates) != 1:
        raise ValueError
    json_string = "\"(?:[^\"\\\\]|\\\\.)*\""
    if guard == "G128":
        pattern = re.compile(
            rf"^G128 status=(?P<status>[A-Z][A-Z-]*) exit=(?P<exit>[0-9]+)"
            rf"(?: session=(?P<session>{json_string}))?"
            r"(?: reason=(?P<reason>[a-z0-9][a-z0-9-]*))?$"
        )
        matrix = {0: {"NO-ACTIVE-BUDGET", "PASS", "SOFT-BOUNDARY"}, 1: {"BREACH"}, 2: {"INPUT-ERROR"}}
    elif guard == "G082":
        pattern = re.compile(
            rf"^G082 status=(?P<status>[A-Z][A-Z-]*) exit=(?P<exit>[0-9]+)"
            rf" session=(?P<session>{json_string})"
            rf" spec=(?P<spec>{json_string})"
            r"(?: reason=(?P<reason>[a-z0-9][a-z0-9-]*))?$"
        )
        matrix = {0: {"PASS"}, 1: {"BREACH"}, 2: {"INPUT-ERROR"}}
    else:
        raise ValueError
    match = pattern.fullmatch(candidates[0])
    if match is None:
        raise ValueError
    status = match.group("status")
    declared_exit = int(match.group("exit"))
    if declared_exit != process_exit or status not in matrix.get(declared_exit, set()):
        raise ValueError
    session_token = match.group("session")
    if session_token is not None:
        if json.loads(session_token) != session_id:
            raise ValueError
    elif status not in {"NO-ACTIVE-BUDGET", "INPUT-ERROR"}:
        raise ValueError
    reason = match.group("reason")
    if guard == "G082":
        if json.loads(match.group("spec")) != spec_dir:
            raise ValueError
        if (status == "INPUT-ERROR") != (reason is not None):
            raise ValueError
    elif (status in {"NO-ACTIVE-BUDGET", "INPUT-ERROR"}) != (reason is not None):
        raise ValueError
except (OSError, UnicodeDecodeError, ValueError, json.JSONDecodeError):
    sys.exit(2)
print(json.dumps({"exit": declared_exit, "status": status}, separators=(",", ":"), sort_keys=True))
' "$guard_name" "$process_exit" "$session_id" "$spec_dir" "$output_file"
}

# =============================================================================
# CHECK 23: Convergence Cap Enforcement (Gate G082)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/convergence-cap-guard.sh.
# The guard reads `.specify/memory/bubbles.session.json` and checks every
# `convergenceLoops[]` entry whose `specDir` matches the spec under
# inspection. If the highest observed `iterationCount` exceeds
# `maxConvergenceIterations` (default 10, from `bubbles/workflows.yaml`),
# the guard exits 1 and this check fails. Missing session.json, missing
# convergenceLoops[], or entries scoped to other specs all pass cleanly.
echo "--- Check 23: Convergence Cap Enforcement (Gate G082) ---"
if fixture_gate_skip "convergence cap enforcement (Gate G082)"; then
  :
else
  conv_output_file=""
  conv_rc=2
  conv_result=""
  conv_status=""
  if b037_validate_blocking_authority; then
    g082_validated_session="$B037_AUTH_SESSION"
    readonly g082_validated_session
    conv_output_file="$(mktemp "${TMPDIR:-/tmp}/bubbles-g082-output.XXXXXX")"
    chmod 600 "$conv_output_file"
    if b037_run_stable_guard "$guard_repo_root" \
        "convergence-cap-guard.sh" "$conv_output_file" "$feature_dir" \
        --session-id "$g082_validated_session" --quiet; then
      conv_rc=0
    else
      conv_rc=$?
    fi
    if conv_result="$(b037_parse_guard_result G082 "$conv_rc" \
        "$g082_validated_session" "$feature_dir" "$conv_output_file" 2>/dev/null)"; then
      cat "$conv_output_file"
      conv_status="$(jq -r '.status' <<<"$conv_result")"
      case "$conv_status" in
        PASS)
          pass "Convergence cap not exceeded (Gate G082)"
          printf 'Check 23 PASS G082 status=PASS exit=0 source=guard\n'
          ;;
        BREACH)
          fail "Convergence cap exceeded — Gate G082 violation"
          printf 'Check 23 FAIL G082 status=BREACH exit=1 source=guard\n'
          ;;
        INPUT-ERROR)
          fail "Convergence cap guard input is invalid — Gate G082"
          printf 'Check 23 FAIL G082 status=INPUT-ERROR exit=2 source=guard\n'
          ;;
      esac
    else
      conv_reason="invalid-child-result"
      if [[ "$conv_rc" -gt 2 ]]; then
        conv_reason="guard-unavailable"
      fi
      fail "Convergence cap enforcement is unavailable or returned an invalid result — Gate G082"
      printf 'Check 23 FAIL G082 status=INPUT-ERROR exit=2 source=caller reason=%s\n' "$conv_reason"
    fi
    rm -f "$conv_output_file"
  else
    fail "Convergence cap authority is invalid — Gate G082"
    printf 'Check 23 FAIL G082 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
  fi
fi
echo ""

# =============================================================================
# CHECK 24: Compaction Discipline Enforcement (Gate G083)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/compaction-discipline-guard.sh.
# The guard reads `.specify/memory/bubbles.session.json`, isolates
# `envelopesReceived[]` entries whose `specDir` matches the spec under
# inspection, sorts by `receivedAt`, drops the latest 2 (kept raw by
# policy), then checks the eligible slice for BOTH `count <= 3` AND
# `cumulative rawSizeBytes <= 8192` UNLESS each over-budget envelope
# carries a `compactedAt` timestamp. Thresholds are framework constants
# (NOT workflows.yaml-configurable). Missing session.json or no
# envelopesReceived[] entries for this spec both pass cleanly.
echo "--- Check 24: Compaction Discipline Enforcement (Gate G083) ---"
comp_guard="$SCRIPT_DIR/compaction-discipline-guard.sh"
if fixture_gate_skip "compaction discipline enforcement (Gate G083)"; then
  :
elif [[ -x "$comp_guard" ]]; then
  if run_guard_in_feature_repo bash "$comp_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "Compaction discipline respected (Gate G083)"
  else
    fail "Compaction discipline violation — Gate G083. Run 'bash $comp_guard $feature_dir' for full diagnostic"
    info "Eligible slice (envelopes except latest 2) MUST satisfy count<=3 AND rawSizeBytes<=8192 UNLESS each over-budget envelope has compactedAt"
    info "Orchestrator agents MUST run bubbles/scripts/context-compactor.sh on over-budget envelopes (additively stamps compactedAt) BEFORE the next dispatch"
    info "Thresholds are framework constants; see agents/bubbles_shared/operating-baseline.md → 'Context Compaction Discipline'"
  fi
else
  info "compaction-discipline-guard.sh not present at $comp_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 25: Pre-Existing Deferral Block Enforcement (Gate G084)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/pre-existing-deferral-guard.sh.
# The guard recursively scans every `scope.md` and `report.md` under
# `<feature_dir>/scopes/*/` for two classes of pre-existing deferral
# markers:
#   - Forbidden phrases (case-insensitive substring):
#       "pre-existing failure", "pre-existing test failure",
#       "carried forward", "out of session scope",
#       "previous-session failure", "not introduced by this spec"
#   - Forbidden markers (colon-anchored, case-sensitive):
#       TODO:  FIXME:  HACK:  STUB:
# H2 subsections named `## Superseded Decisions`, `## Historical Notes`,
# and `## Out of Scope` are exempt (allowed to discuss historical
# deferrals for traceability). Inline `...` backticked spans and
# ```fenced code blocks``` are also exempt so the guard never
# self-triggers when the language is used as enumeration prose or
# captured raw terminal output. Any active hit produces exit 1 and
# blocks promotion to `done`.
echo "--- Check 25: Pre-Existing Deferral Block Enforcement (Gate G084) ---"
pre_guard="$SCRIPT_DIR/pre-existing-deferral-guard.sh"
if [[ -x "$pre_guard" ]]; then
  if run_guard_in_feature_repo bash "$pre_guard" "$feature_dir" --quiet > /dev/null 2>&1; then
    pass "No active pre-existing-deferral markers in scope.md / report.md (Gate G084)"
  else
    fail "Pre-existing deferral marker detected — Gate G084. Run 'bash $pre_guard $feature_dir' for full diagnostic"
    info "Forbidden phrases: 'pre-existing failure', 'pre-existing test failure', 'carried forward', 'out of session scope', 'previous-session failure', 'not introduced by this spec'"
    info "Forbidden markers (colon-anchored): TODO:  FIXME:  HACK:  STUB:"
    info "Move historical language under '## Superseded Decisions', '## Historical Notes', or '## Out of Scope', OR wrap enumeration prose in inline backticks"
    info "Pre-existing failures MUST be fixed inline; deferring to a follow-up session is forbidden by Gate G084"
  fi
else
  info "pre-existing-deferral-guard.sh not present at $pre_guard; skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 40: Session Cap Enforcement (Gate G128)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/session-cap-guard.sh — the
# whole-host-session sibling of the per-(specDir, agent) convergence cap
# (Check 23 / G082). The guard reads retained repository history but measures
# only records exactly attributed to the host ID forwarded from the validated
# caller. A missing ID remains legal for the default-off no-op; an active budget
# makes it an INPUT-ERROR. The guard takes no specDir because it aggregates all
# specs and agents inside the selected host session.
echo "--- Check 40: Session Cap Enforcement (Gate G128) ---"
if fixture_gate_skip "session cap enforcement (Gate G128)"; then
  :
else
  sess_args=(--quiet)
  sess_output_file=""
  sess_rc=2
  sess_result=""
  sess_status=""
  if b037_validate_blocking_authority; then
    g128_validated_session="$B037_AUTH_SESSION"
    readonly g128_validated_session
    sess_args+=(--session-id "$g128_validated_session")
    sess_output_file="$(mktemp "${TMPDIR:-/tmp}/bubbles-g128-output.XXXXXX")"
    chmod 600 "$sess_output_file"
    if b037_run_stable_guard "$guard_repo_root" \
      "session-cap-guard.sh" "$sess_output_file" "${sess_args[@]}"; then
      sess_rc=0
    else
      sess_rc=$?
    fi
    if sess_result="$(b037_parse_guard_result G128 "$sess_rc" \
        "$g128_validated_session" "" "$sess_output_file" 2>/dev/null)"; then
      cat "$sess_output_file"
      sess_status="$(jq -r '.status' <<<"$sess_result")"
      case "$sess_status" in
        NO-ACTIVE-BUDGET|PASS|SOFT-BOUNDARY)
          pass "Host-session budget evaluation completed (Gate G128)"
          printf 'Check 40 PASS G128 status=%s exit=0 source=guard\n' "$sess_status"
          ;;
        BREACH)
          fail "Host-session budget breached — Gate G128"
          printf 'Check 40 FAIL G128 status=BREACH exit=1 source=guard\n'
          ;;
        INPUT-ERROR)
          fail "Host-session budget evaluation input is invalid — Gate G128"
          printf 'Check 40 FAIL G128 status=INPUT-ERROR exit=2 source=guard\n'
          ;;
      esac
    else
      sess_reason="invalid-child-result"
      if [[ "$sess_rc" -gt 2 ]]; then
        sess_reason="guard-unavailable"
      fi
      fail "Session cap enforcement is unavailable or returned an invalid result — Gate G128"
      printf 'Check 40 FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=%s\n' "$sess_reason"
    fi
    rm -f "$sess_output_file"
  else
    fail "Session cap authority is invalid — Gate G128"
    printf 'Check 40 FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
  fi
fi
echo ""
