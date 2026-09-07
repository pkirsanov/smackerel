#!/usr/bin/env bash
# python-env-selftest.sh — hermetic selftest for python-env.sh.
#
# Proves the RESOLUTION CONTRACT without touching the operator's real managed
# environment: every case points BUBBLES_PYTHON_HOME at a mktemp dir, and the
# "satisfying" interpreters are tiny fake python3 shims, so no network, no pip,
# and no dependency on whether this machine happens to have PyYAML installed.
#
# ADVERSARIAL CASES (each would pass if the logic regressed the obvious way):
#   A1  an interpreter satisfying only ONE of the two modules must NOT count as
#       satisfying. A resolver that stops at the first successful import passes
#       every other case here.
#   A2  activate() must NOT prepend when PATH's python3 already satisfies —
#       otherwise it silently hijacks an operator who provisioned their own way.
#   A3  the module list must stay in lockstep with dependency-posture.sh's
#       BUBBLES_REQUIRED_DEPS, so the documented duplication cannot drift.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_SH="$SCRIPT_DIR/python-env.sh"
SELFTEST_SCRIPT="$SCRIPT_DIR/python-env-selftest.sh"
POSTURE_SH="$SCRIPT_DIR/dependency-posture.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"

RUN_GENERAL=1
RUN_SECURITY=0
RUN_STRESS=0
RUN_FIXED_PERL=0
RUN_CLI_INTEGRATION=0
RUN_SIGNAL_READINESS=0
RUN_SIGNAL_TARGET=0
RUN_STRESS_COMPLETENESS_CONTROL=0
RUN_EXIT_TRAP_INTERRUPT=0
RUN_EXIT_TRAP_TARGET=0
RUN_BASH32_AGGREGATE=0
SIGNAL_TARGET_NAME=''
SIGNAL_TARGET_WORKER=''
SIGNAL_TARGET_READY_FIFO=''
SIGNAL_TARGET_HOLD_FIFO=''
SIGNAL_TARGET_RESULT_FILE=''
EXIT_TRAP_TARGET_WORKER=''
EXIT_TRAP_TARGET_CONTROL_ROOT=''
case "${1:-}" in
  '')
    if [[ "$#" -ne 0 ]]; then
      printf '%s\n' 'python-env selftest accepts no empty positional arguments' >&2
      exit 2
    fi
    ;;
  --internal-privileged-security)
    if [[ "$#" -ne 1 ]]; then
      printf '%s\n' 'python-env privileged security mode accepts no caller data' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_SECURITY=1
    ;;
  --internal-stress-lifecycle)
    if [[ "$#" -ne 2 || "${2:-}" != b039-native-supervisor-stress-v1 ]]; then
      printf '%s\n' 'python-env stress lifecycle mode requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_SECURITY=1
    RUN_STRESS=1
    ;;
  --internal-signal-readiness-controls)
    if [[ "$#" -ne 2 || "${2:-}" != b039-signal-readiness-v1 ]]; then
      printf '%s\n' 'python-env signal-readiness mode requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_SIGNAL_READINESS=1
    ;;
  --internal-signal-target)
    if [[ "$#" -ne 7 || "${2:-}" != b039-signal-target-v1 ]]; then
      printf '%s\n' 'python-env signal-target mode requires its explicit internal token and five target fields' >&2
      exit 2
    fi
    case "${3:-}" in HUP | INT | TERM) ;; *) exit 2 ;; esac
    if [[ "${4:-}" != /* || ! -x "${4:-}" || "${5:-}" != /* ||
      "${6:-}" != /* || "${7:-}" != /* ]]; then
      printf '%s\n' 'python-env signal-target mode requires absolute owned paths and an executable worker' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_SIGNAL_TARGET=1
    SIGNAL_TARGET_NAME="$3"
    SIGNAL_TARGET_WORKER="$4"
    SIGNAL_TARGET_READY_FIFO="$5"
    SIGNAL_TARGET_HOLD_FIFO="$6"
    SIGNAL_TARGET_RESULT_FILE="$7"
    ;;
  --internal-stress-completeness-control)
    if [[ "$#" -ne 2 || "${2:-}" != b039-stress-completeness-v1 ]]; then
      printf '%s\n' 'python-env stress-completeness control requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_STRESS_COMPLETENESS_CONTROL=1
    ;;
  --internal-exit-trap-interruption)
    if [[ "$#" -ne 2 || "${2:-}" != pre-har-01-exit-trap-v1 ]]; then
      printf '%s\n' 'python-env exit-trap interruption control requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_EXIT_TRAP_INTERRUPT=1
    ;;
  --reg-py-reap-01)
    if [[ "$#" -ne 1 ]]; then
      printf '%s\n' 'REG-PY-REAP-01 accepts no additional arguments' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_EXIT_TRAP_INTERRUPT=1
    ;;
  --reg-py-bash32-01)
    if [[ "$#" -ne 1 ]]; then
      printf '%s\n' 'REG-PY-BASH32-01 accepts no additional arguments' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_BASH32_AGGREGATE=1
    ;;
  --internal-exit-trap-target)
    if [[ "$#" -ne 4 || "${2:-}" != pre-har-01-target-v1 ||
      "${3:-}" != /* || ! -x "${3:-}" || "${4:-}" != /* || ! -d "${4:-}" ]]; then
      printf '%s\n' 'python-env exit-trap target requires its token, executable worker, and absolute control root' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_EXIT_TRAP_TARGET=1
    EXIT_TRAP_TARGET_WORKER="$3"
    EXIT_TRAP_TARGET_CONTROL_ROOT="$4"
    ;;
  --internal-fixed-perl-negative-controls)
    if [[ "$#" -ne 2 || "${2:-}" != b039-fixed-perl-negative-controls-v1 ]]; then
      printf '%s\n' 'python-env fixed-Perl negative-control mode requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_FIXED_PERL=1
    ;;
  --internal-cli-caller-integration)
    if [[ "$#" -ne 2 || "${2:-}" != b039-cli-caller-integration-v1 ]]; then
      printf '%s\n' 'python-env CLI caller integration mode requires its explicit internal token' >&2
      exit 2
    fi
    RUN_GENERAL=0
    RUN_CLI_INTEGRATION=1
    ;;
  *)
    printf 'python-env selftest mode is invalid: %s\n' "$1" >&2
    exit 2
    ;;
esac

if [[ ! -f "$ENV_SH" || ! -f "$GUARD_LIB" ]]; then
  echo "python-env-selftest: required surface missing (python-env.sh or guard-lib.sh)" >&2
  exit 2
fi

if [[ "$RUN_SECURITY" -eq 1 || "$RUN_FIXED_PERL" -eq 1 ||
  "$RUN_SIGNAL_READINESS" -eq 1 || "$RUN_SIGNAL_TARGET" -eq 1 ||
  "$RUN_EXIT_TRAP_TARGET" -eq 1 ]]; then
  case "$-" in
    *p*) ;;
    *)
      printf '%s\n' 'TP-S2-01_PRIVILEGED_CHILD_SETUP=FAIL reason=non-privileged-bash' >&2
      exit 2
      ;;
  esac
  if [[ -n "${BASH_ENV+x}" || -n "${CDPATH+x}" || -n "${GLOBIGNORE+x}" ]] ||
    declare -F source >/dev/null 2>&1 || declare -F kill >/dev/null 2>&1 ||
    declare -F wait >/dev/null 2>&1; then
    printf '%s\n' 'TP-S2-01_PRIVILEGED_CHILD_SETUP=FAIL reason=hostile-startup-state-present' >&2
    exit 2
  fi
  printf '%s\n' 'TP-S2-01_PRIVILEGED_CHILD_SETUP=PASS mode=env-i-/bin/bash-p hostileState=excluded'
fi

# Probe execution must use the same bounded watchdog API as production callers.
# shellcheck source=/dev/null
source "$GUARD_LIB"
# shellcheck source=/dev/null
source "$ENV_SH"

pass=0
fail=0
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
SELFTEST_COMPLETED=0
SCOPE2_SIGNAL_READINESS_COMPLETED=0
SCOPE2_STRESS_COMPLETED=0
SCOPE2_STRESS_ATTEMPT_COUNT=0
SCOPE2_STRESS_CLASS_COUNT=0
SELFTEST_ROOT_SUBSHELL=$BASH_SUBSHELL
SELFTEST_LIFECYCLE_PID=''
SECURITY_BOUNDARY_READY=0
SECURITY_BOUNDARY_RECORD=''

selftest_stop_exact_child() {
  if [[ "$SELFTEST_LIFECYCLE_PID" =~ ^[1-9][0-9]*$ ]]; then
    builtin kill -TERM "$SELFTEST_LIFECYCLE_PID" 2>/dev/null || true
    builtin kill -KILL "$SELFTEST_LIFECYCLE_PID" 2>/dev/null || true
    builtin wait "$SELFTEST_LIFECYCLE_PID" 2>/dev/null || true
  fi
  SELFTEST_LIFECYCLE_PID=''
}

selftest_cleanup() {
  local exit_status=$?
  if [[ "$BASH_SUBSHELL" -ne "$SELFTEST_ROOT_SUBSHELL" ]]; then
    return "$exit_status"
  fi
  builtin trap - EXIT HUP INT TERM
  if [[ ("$RUN_SECURITY" -eq 1 || "$RUN_FIXED_PERL" -eq 1 ||
    "$RUN_SIGNAL_READINESS" -eq 1 || "$RUN_SIGNAL_TARGET" -eq 1 ||
    "$RUN_EXIT_TRAP_TARGET" -eq 1) &&
    "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
    bubbles_python_security_cleanup || true
  fi
  selftest_stop_exact_child
  /bin/rm -rf "$TMP_ROOT"
  if [[ "$SELFTEST_COMPLETED" -ne 1 && "$exit_status" -eq 0 ]]; then
    echo "FAIL: python-env selftest exited before its completion summary" >&2
    exit_status=1
  fi
  exit "$exit_status"
}

selftest_signal() {
  local exit_status="$1"
  trap - HUP INT TERM
  exit "$exit_status"
}

trap selftest_cleanup EXIT
trap 'selftest_signal 129' HUP
trap 'selftest_signal 130' INT
trap 'selftest_signal 143' TERM

if [[ -n "${BUBBLES_PYTHON_SELFTEST_CHILD_MODE:-}" ]]; then
  if [[ -z "${BUBBLES_PYTHON_SELFTEST_READY_FILE:-}" ]]; then
    echo "python-env selftest child mode requires a ready file" >&2
    exit 2
  fi
  case "$BUBBLES_PYTHON_SELFTEST_CHILD_MODE" in
    premature-exit)
      printf '%s\n' "$TMP_ROOT" >"$BUBBLES_PYTHON_SELFTEST_READY_FILE"
      exit 0
      ;;
    timeout-exit)
      printf '%s\n' "$TMP_ROOT" >"$BUBBLES_PYTHON_SELFTEST_READY_FILE"
      exit 124
      ;;
    interrupt-hold)
      /usr/bin/mkfifo "$TMP_ROOT/interrupt-hold.fifo"
      exec 9<>"$TMP_ROOT/interrupt-hold.fifo"
      printf '%s\n' "$TMP_ROOT" >"$BUBBLES_PYTHON_SELFTEST_READY_FILE"
      builtin read -r -t 300 _selftest_hold <&9
      ;;
    *)
      echo "python-env selftest child mode is invalid" >&2
      exit 2
      ;;
  esac
fi

make_native_supervisor_mutation() {
  local mode="$1"
  local destination="$2"
  local source_mode=""

  /usr/bin/awk -v mode="$mode" '
    BEGIN { supervisor_wait_clear="BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=" sprintf("%c%c", 39, 39) }
    {
      lines[NR]=$0
      if ($0 ~ /^_bubbles_python_security_supervisor_program\(\) \{/) in_supervisor=1
      if (in_supervisor && waitpid_line == 0 && $0 ~ /waitpid[[:space:]]*\(/) waitpid_line=NR
      if (in_supervisor && complete_line == 0 && $0 ~ /COMPLETE\\tBPS1/) complete_line=NR
      if (in_supervisor && owner_clear_line == 0 &&
        $0 ~ /worker(_is)?_owned.*=[[:space:]]*0|owned_worker.*=[[:space:]]*0/) owner_clear_line=NR
      if (in_supervisor && worker_kill_line == 0 && $0 ~ /kill.*worker|kill.*worker_pid/) worker_kill_line=NR
      if (in_supervisor && control_close_line == 0 && $0 ~ /^[[:space:]]*close[[:space:]]*\(CONTROL\)/ && tolower($0) ~ /worker.*(control|bps).*descriptor/) control_close_line=NR
      if (in_supervisor && eof_line == 0 && $0 ~ /EOF|eof|read.*==[[:space:]]*0/) eof_line=NR
      if (in_supervisor && $0 ~ /^PERL$/) in_supervisor=0
      if ($0 ~ /^_bubbles_python_run_closed_operation\(\) \{/) in_run=1
      if (in_run && runtime_line == 0 && $0 ~ /print\("RUNTIME\\tPYSEC1/) runtime_line=NR
      if (in_run && supervisor_wait_line == 0 &&
        $0 ~ /builtin wait .*BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID/) supervisor_wait_line=NR
      if (in_run && supervisor_wait_line > 0 && supervisor_clear_line == 0 && index($0, supervisor_wait_clear) > 0) supervisor_clear_line=NR
      if (in_run && supervisor_clear_line > 0 && cleanup_line == 0 &&
        $0 ~ /bubbles_python_security_cleanup/) cleanup_line=NR
      if ($0 ~ /^bubbles_python_run_security_operation\(\) \{/) { in_run=0; in_api=1 }
      if (in_api && api_line == 0 && $0 ~ /apple-select[[:space:]]*\|/) api_line=NR
    }
    END {
      required=(waitpid_line > 0 && complete_line > 0 && owner_clear_line > 0 &&
        worker_kill_line > 0 && control_close_line > 0 && eof_line > 0 &&
        runtime_line > 0 && supervisor_wait_line > 0 && supervisor_clear_line > 0 &&
        cleanup_line > 0 && api_line > 0)
      if (!required) exit 42

      for (i=1; i<=NR; i++) {
        line=lines[i]
        if (mode == "signal-after-reap" && i == worker_kill_line) continue
        if (mode == "late-owner-clear" && i == owner_clear_line) continue
        if (mode == "cleanup-before-reap" && i == cleanup_line) continue
        if (mode == "worker-control-descriptor" && i == control_close_line) {
          print "# " line
          changed++
          continue
        }
        if (mode == "omit-waitpid" && line ~ /waitpid[[:space:]]*\(/) {
          gsub(/waitpid/, "b039_waitpid_omitted", line)
          changed++
        }
        if (mode == "caller-vector" && i == api_line) {
          sub(/apple-select/, "caller-program | apple-select", line)
          changed++
        }
        if (mode == "worker-forged-bps1" && i == runtime_line) {
          print "print(\"COMPLETE\\tBPS1\\truntime-probe\\t0\\tworker\\t0\\texit\\t0\\t0\")"
          changed++
        }
        if (mode == "eof-as-completion" && i == waitpid_line) {
          print lines[complete_line]
          changed++
        }
        if (mode == "cleanup-before-reap" && i == supervisor_wait_line) {
          print lines[cleanup_line]
          changed++
        }
        print line
        if (mode == "signal-after-reap" && i == owner_clear_line) {
          print lines[worker_kill_line]
          changed++
        }
        if (mode == "late-owner-clear" && i == complete_line) {
          print lines[owner_clear_line]
          changed++
        }
      }
      if (changed == 0) exit 42
    }
  ' "$ENV_SH" >"$destination" || return $?

  if source_mode="$(/usr/bin/stat -f '%Lp' "$ENV_SH" 2>/dev/null)"; then
    :
  elif source_mode="$(/usr/bin/stat -c '%a' "$ENV_SH" 2>/dev/null)"; then
    :
  else
    return 42
  fi
  /bin/chmod "$source_mode" "$destination"
}

ok() {
  echo "PASS: $1"
  pass=$((pass + 1))
}
bad() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

PRE_HAR_01_WAIT_COUNT=0
PRE_HAR_01_OWNER_PID=''

pre_har_01_wait_debug() {
  local command_text="$1"
  local supervisor_command=""

  [[ "$command_text" == "builtin wait \"\$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID\"" ]] || return 0
  PRE_HAR_01_WAIT_COUNT=$((PRE_HAR_01_WAIT_COUNT + 1))
  printf '%s\n' "$PRE_HAR_01_WAIT_COUNT" >"$EXIT_TRAP_TARGET_CONTROL_ROOT/wait-count"
  if [[ "$PRE_HAR_01_WAIT_COUNT" -eq 1 ]]; then
    printf '%s\n' "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" \
      >"$EXIT_TRAP_TARGET_CONTROL_ROOT/supervisor.pid"
    printf '%s\n' "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" \
      >"$EXIT_TRAP_TARGET_CONTROL_ROOT/private-root"
    supervisor_command="$(/bin/ps -p "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" -o command= 2>/dev/null || true)"
    printf '%s\n' "$supervisor_command" >"$EXIT_TRAP_TARGET_CONTROL_ROOT/supervisor.command"
    (
      /bin/sleep 0.2
      if pre_har_01_process_is_live "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID"; then
        printf '%s\n' yes >"$EXIT_TRAP_TARGET_CONTROL_ROOT/supervisor-live-before-term"
      fi
      /bin/kill -TERM "$PRE_HAR_01_OWNER_PID"
      printf '%s\n' SENT >"$EXIT_TRAP_TARGET_CONTROL_ROOT/signal-sent"
      /bin/sleep 0.5
      if pre_har_01_process_is_live "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID"; then
        printf '%s\n' yes >"$EXIT_TRAP_TARGET_CONTROL_ROOT/supervisor-live-before-usr1"
      fi
      /bin/kill -USR1 "$PRE_HAR_01_OWNER_PID"
      printf '%s\n' SENT >"$EXIT_TRAP_TARGET_CONTROL_ROOT/usr1-signal-sent"
    ) &
    printf '%s\n' "$!" >"$EXIT_TRAP_TARGET_CONTROL_ROOT/signal-sender.pid"
    # Leave the normal operation path at its first real supervisor wait. The
    # delayed sender interrupts both production EXIT waits while that same
    # supervisor remains live; Bash suppresses DEBUG while the EXIT trap runs.
    exit 77
  fi
}

pre_har_01_second_wait_interrupt() {
  local handle_present=no
  local private_root_present=no
  local supervisor_live=no

  if [[ "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" =~ ^[1-9][0-9]*$ ]]; then
    handle_present=yes
    if pre_har_01_process_is_live "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID"; then
      supervisor_live=yes
    fi
  fi
  [[ -n "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" \
    && -d "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" ]] && private_root_present=yes
  printf 'pendingSignal=%s pendingStatus=%s handlePresent=%s supervisorLive=%s privateRootPresent=%s\n' \
    "${BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL:-none}" \
    "$BUBBLES_PYTHON_SECURITY_PENDING_STATUS" "$handle_present" \
    "$supervisor_live" "$private_root_present" \
    >"$EXIT_TRAP_TARGET_CONTROL_ROOT/second-wait-interrupt.snapshot"
}

run_exit_trap_target() {
  local boundary_record=""
  local owner_identity_waited=0

  while [[ ! -s "$EXIT_TRAP_TARGET_CONTROL_ROOT/owner.pid" &&
    "$owner_identity_waited" -lt 20 ]]; do
    /bin/sleep 0.1
    owner_identity_waited=$((owner_identity_waited + 1))
  done
  PRE_HAR_01_OWNER_PID="$(/bin/cat "$EXIT_TRAP_TARGET_CONTROL_ROOT/owner.pid" 2>/dev/null || true)"
  [[ "$PRE_HAR_01_OWNER_PID" =~ ^[1-9][0-9]*$ ]] || return 125

  BUBBLES_SECURITY_ENTRY_MODE=direct
  export BUBBLES_SECURITY_ENTRY_MODE
  boundary_record="$(bubbles_python_security_require_boundary 2>/dev/null)" || return $?
  [[ "$boundary_record" == $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' ]] || return 125
  builtin trap 'pre_har_01_second_wait_interrupt' USR1
  set -T
  # BASH_COMMAND is intentionally expanded by the DEBUG trap.
  # shellcheck disable=SC2016
  builtin trap 'pre_har_01_wait_debug "$BASH_COMMAND"' DEBUG
  _bubbles_python_run_closed_operation general-probe "$EXIT_TRAP_TARGET_WORKER"
  return $?
}

pre_har_01_process_is_live() {
  local process_pid="${1:-}"
  local process_state=""

  [[ "$process_pid" =~ ^[1-9][0-9]*$ ]] || return 1
  builtin kill -0 "$process_pid" 2>/dev/null || return 1
  process_state="$(/bin/ps -p "$process_pid" -o state= 2>/dev/null || true)"
  process_state="${process_state//[[:space:]]/}"
  [[ "$process_state" != Z* ]]
}

pre_har_01_worker_processes() {
  local worker_path="$1"
  local process_line=""
  local process_pid=""
  local process_command=""

  while IFS= read -r process_line; do
    process_line="${process_line#"${process_line%%[![:space:]]*}"}"
    process_pid="${process_line%%[[:space:]]*}"
    process_command="${process_line#"$process_pid"}"
    if [[ "$process_pid" =~ ^[1-9][0-9]*$ && "$process_command" == *"$worker_path"* ]]; then
      printf '%s\n' "$process_pid"
    fi
  done < <(/bin/ps -axo pid=,command=)
}

run_exit_trap_interruption_control() {
  local case_root="$TMP_ROOT/pre-har-01-exit-trap"
  local output_file="$case_root/owner.output"
  local worker_file="$case_root/real-supervisor.worker"
  local supervisor_pid=""
  local supervisor_state="absent"
  local supervisor_command=""
  local supervisor_is_real="no"
  local supervisor_alive_initial="no"
  local supervisor_alive_final="no"
  local private_root=""
  local private_root_absent_initial="no"
  local private_root_absent_final="no"
  local signal_sent="no"
  local usr1_signal_sent="no"
  local first_wait_supervisor_live="no"
  local second_wait_supervisor_live="no"
  local second_wait_snapshot=""
  local signal_sender_pid=""
  local signal_sender_residue="no"
  local owner_pid=""
  local wait_count=0
  local owner_status=0
  local waited=0
  local worker_residue_count=0
  local worker_pid=""

  mkdir -p "$case_root"
  cat >"$worker_file" <<'PRE_HAR_01_WORKER'
#!/usr/bin/env bash
printf '%s' 'bubbles-python-runs'
/bin/sleep 5
PRE_HAR_01_WORKER
  /bin/chmod 700 "$worker_file"

  /usr/bin/env -i LC_ALL=C PATH=/usr/bin:/bin \
    BUBBLES_SECURITY_ENTRY_MODE=direct \
    "$BASH" -p "$SELFTEST_SCRIPT" --internal-exit-trap-target \
    pre-har-01-target-v1 "$worker_file" "$case_root" \
    >"$output_file" 2>&1 &
  owner_pid=$!
  printf '%s\n' "$owner_pid" >"$case_root/owner.pid"
  if builtin wait "$owner_pid"; then
    owner_status=0
  else
    owner_status=$?
  fi

  /bin/cat "$output_file"
  supervisor_pid="$(/bin/cat "$case_root/supervisor.pid" 2>/dev/null || true)"
  private_root="$(/bin/cat "$case_root/private-root" 2>/dev/null || true)"
  supervisor_command="$(/bin/cat "$case_root/supervisor.command" 2>/dev/null || true)"
  wait_count="$(/bin/cat "$case_root/wait-count" 2>/dev/null || printf '%s' 0)"
  [[ -f "$case_root/signal-sent" ]] && signal_sent=yes
  [[ -f "$case_root/usr1-signal-sent" ]] && usr1_signal_sent=yes
  [[ -f "$case_root/supervisor-live-before-term" ]] && first_wait_supervisor_live=yes
  [[ -f "$case_root/supervisor-live-before-usr1" ]] && second_wait_supervisor_live=yes
  second_wait_snapshot="$(/bin/cat "$case_root/second-wait-interrupt.snapshot" 2>/dev/null || true)"
  signal_sender_pid="$(/bin/cat "$case_root/signal-sender.pid" 2>/dev/null || true)"
  [[ "$supervisor_command" == *"/usr/bin/perl -T -w -e"* ]] && supervisor_is_real=yes
  if pre_har_01_process_is_live "$supervisor_pid"; then
    supervisor_alive_initial=yes
    supervisor_state="$(/bin/ps -p "$supervisor_pid" -o state= 2>/dev/null || true)"
    supervisor_state="${supervisor_state//[[:space:]]/}"
  fi
  [[ -n "$private_root" && ! -e "$private_root" ]] && private_root_absent_initial=yes

  printf 'PRE_HAR_01_CONTROL ownerExit=%s waitCount=%s signalSent=%s supervisorPid=%s supervisorKind=%s supervisorState=%s supervisorAliveAtOwnerExit=%s privateRootAbsentAtOwnerExit=%s\n' \
    "$owner_status" "$wait_count" "$signal_sent" \
    "${supervisor_pid:-missing}" "$supervisor_is_real" "${supervisor_state:-missing}" \
    "$supervisor_alive_initial" "$private_root_absent_initial"
  printf 'REG_PY_REAP_01_CONTROL termSent=%s usr1Sent=%s supervisorLiveBeforeTerm=%s supervisorLiveBeforeUsr1=%s secondWaitSnapshot=%q\n' \
    "$signal_sent" "$usr1_signal_sent" "$first_wait_supervisor_live" \
    "$second_wait_supervisor_live" "${second_wait_snapshot:-missing}"

  while pre_har_01_process_is_live "$supervisor_pid" && [[ "$waited" -lt 60 ]]; do
    /bin/sleep 0.1
    waited=$((waited + 1))
  done
  pre_har_01_process_is_live "$supervisor_pid" && supervisor_alive_final=yes
  while IFS= read -r worker_pid; do
    [[ -n "$worker_pid" ]] || continue
    worker_residue_count=$((worker_residue_count + 1))
    builtin kill -KILL "$worker_pid" 2>/dev/null || true
  done < <(pre_har_01_worker_processes "$worker_file")
  if pre_har_01_process_is_live "$signal_sender_pid"; then
    signal_sender_residue=yes
    builtin kill -KILL "$signal_sender_pid" 2>/dev/null || true
  fi
  [[ -n "$private_root" && ! -e "$private_root" ]] && private_root_absent_final=yes
  printf 'PRE_HAR_01_CLEANUP supervisorAliveFinal=%s workerResidue=%s signalSenderResidue=%s privateRootAbsentFinal=%s waitedTenths=%s\n' \
    "$supervisor_alive_final" "$worker_residue_count" "$signal_sender_residue" \
    "$private_root_absent_final" "$waited"

  if [[ "$owner_status" -eq 77 && "$wait_count" -eq 1 &&
    "$signal_sent" == yes && "$usr1_signal_sent" == yes &&
    "$first_wait_supervisor_live" == yes && "$second_wait_supervisor_live" == yes &&
    "$second_wait_snapshot" == 'pendingSignal=TERM pendingStatus=143 handlePresent=yes supervisorLive=yes privateRootPresent=yes' &&
    "$supervisor_pid" =~ ^[1-9][0-9]*$ &&
    "$supervisor_is_real" == yes && "$supervisor_alive_initial" == no &&
    "$private_root_absent_initial" == yes && "$supervisor_alive_final" == no &&
    "$worker_residue_count" -eq 0 && "$signal_sender_residue" == no &&
    "$private_root_absent_final" == yes ]]; then
    ok "REG-PY-REAP-01: EXIT survives both wait interruptions and reaps the supervisor before private-root cleanup"
  else
    printf 'RED-CONTROL: REG-PY-REAP-01 second EXIT wait returned while realSupervisorAlive=%s privateRootAbsent=%s snapshot=%q\n' \
      "$supervisor_alive_initial" "$private_root_absent_initial" "${second_wait_snapshot:-missing}"
    bad "REG-PY-REAP-01: EXIT must not clear the handle or clean the private root after two interrupted waits while its supervisor remains alive"
  fi

  if [[ "$supervisor_alive_final" == yes ]]; then
    builtin kill -KILL "$supervisor_pid" 2>/dev/null || true
  fi
  [[ -z "$private_root" || ! -e "$private_root" ]] || /bin/rm -rf "$private_root"
}

PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS=0
PRE_HAR_01_STOCK_SCENARIO_STATUS=125

run_reg_py_bash32_01_control() {
  local output_file="$TMP_ROOT/reg-py-bash32-01.output"
  local stock_without_bashpid=no
  local stock_status=0
  local scenario_markers=0
  local lifecycle_markers=0
  local lifecycle_execution_markers=0
  local result_markers=0
  local bashpid_diagnostics=0

  PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS=$((PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS + 1))
  if /bin/bash -c '[[ -z "${BASHPID+x}" ]]'; then
    stock_without_bashpid=yes
  fi
  if /usr/bin/env -i LC_ALL=C PATH=/usr/bin:/bin BUBBLES_SECURITY_ENTRY_MODE=direct \
    /bin/bash -p "$SELFTEST_SCRIPT" --reg-py-reap-01 >"$output_file" 2>&1; then
    stock_status=0
  else
    stock_status=$?
  fi
  PRE_HAR_01_STOCK_SCENARIO_STATUS="$stock_status"
  /bin/cat "$output_file"
  scenario_markers="$(/usr/bin/grep -c '^Scenario: PRE-HAR-01 ' "$output_file" 2>/dev/null || true)"
  lifecycle_markers="$(/usr/bin/grep -c '^REG_PY_REAP_01_CONTROL ' "$output_file" 2>/dev/null || true)"
  lifecycle_execution_markers="$(/usr/bin/awk '
    /^REG_PY_REAP_01_CONTROL / &&
      index($0, "termSent=yes usr1Sent=yes supervisorLiveBeforeTerm=yes supervisorLiveBeforeUsr1=yes") &&
      index($0, "secondWaitSnapshot=pendingSignal=TERM\\ pendingStatus=143\\ handlePresent=yes\\ supervisorLive=yes\\ privateRootPresent=yes") {
        count++
      }
    END { print count + 0 }
  ' "$output_file")"
  result_markers="$(/usr/bin/grep -c '^python-env PRE-HAR-01 selftest:' "$output_file" 2>/dev/null || true)"
  bashpid_diagnostics="$(/usr/bin/grep -c 'BASHPID.*unbound variable' "$output_file" 2>/dev/null || true)"
  printf 'REG_PY_BASH32_01_CONTROL aggregateInvocation=%s stockWithoutBASHPID=%s stockExit=%s scenarioMarkers=%s lifecycleMarkers=%s lifecycleExecutionMarkers=%s resultMarkers=%s bashpidDiagnostics=%s\n' \
    "$PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS" "$stock_without_bashpid" \
    "$stock_status" "$scenario_markers" "$lifecycle_markers" \
    "$lifecycle_execution_markers" "$result_markers" "$bashpid_diagnostics"
  if [[ "$stock_without_bashpid" == yes &&
    ( "$stock_status" -eq 0 || "$stock_status" -eq 1 ) &&
    "$scenario_markers" -eq 1 && "$lifecycle_markers" -eq 1 &&
    "$lifecycle_execution_markers" -eq 1 && "$result_markers" -eq 1 &&
    "$bashpid_diagnostics" -eq 0 ]]; then
    ok "REG-PY-BASH32-01: stock Bash without BASHPID executes and reports the PRE-HAR-01 lifecycle assertion"
  else
    bad "REG-PY-BASH32-01: stock Bash 3.2 must reach PRE-HAR-01 and the aggregate must account for its RED result"
  fi
}

if [[ "$RUN_EXIT_TRAP_TARGET" -eq 1 ]]; then
  run_exit_trap_target
  exit $?
fi

if [[ "$RUN_EXIT_TRAP_INTERRUPT" -eq 1 ]]; then
  echo "Scenario: PRE-HAR-01 two signal interruptions cannot make EXIT clear a live supervisor handle."
  run_exit_trap_interruption_control
  echo
  echo "python-env PRE-HAR-01 selftest: $pass passed, $fail failed"
  SELFTEST_COMPLETED=1
  [[ "$fail" -eq 0 ]]
  exit $?
fi

scope2_sha256_file() {
  local path="$1"
  local hash_tool=""
  if [[ -x /usr/bin/sha256sum ]]; then
    /usr/bin/sha256sum "$path" | /usr/bin/awk '{ print $1 }'
    return ${PIPESTATUS[0]}
  fi
  if [[ -x /usr/bin/shasum ]]; then
    /usr/bin/shasum -a 256 "$path" | /usr/bin/awk '{ print $1 }'
    return ${PIPESTATUS[0]}
  fi
  hash_tool="$(command -v sha256sum 2>/dev/null || true)"
  if [[ -n "$hash_tool" ]]; then
    "$hash_tool" "$path" | /usr/bin/awk '{ print $1 }'
    return ${PIPESTATUS[0]}
  fi
  hash_tool="$(command -v shasum 2>/dev/null || true)"
  if [[ -n "$hash_tool" ]]; then
    "$hash_tool" -a 256 "$path" | /usr/bin/awk '{ print $1 }'
    return ${PIPESTATUS[0]}
  fi
  return 1
}

scope2_file_mode() {
  local path="$1"
  local mode=""
  if mode="$(/usr/bin/stat -f '%Lp' "$path" 2>/dev/null)" &&
    [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  if mode="$(/usr/bin/stat -c '%a' "$path" 2>/dev/null)" &&
    [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  return 1
}

scope2_security_private_root_count() {
  local candidate=""
  local count=0
  for candidate in /tmp/bubbles-python-security.*; do
    [[ -e "$candidate" ]] || continue
    count=$((count + 1))
  done
  printf '%s\n' "$count"
}

SCOPE2_FIXED_PERL_SIBLING_COUNT=0
SCOPE2_FIXED_PERL_SIBLING_MISMATCH=''

scope2_verify_fixed_perl_candidate_siblings() {
  local framework_root="$1"
  local copied_framework="$2"
  local source_path=""
  local copied_path=""
  local relative_path=""
  local source_mode=""
  local copied_mode=""
  local source_count=0
  local copied_count=0
  local mismatch=""

  while IFS= read -r source_path || [[ -n "$source_path" ]]; do
    [[ -n "$source_path" ]] || continue
    relative_path="${source_path#"$framework_root"/}"
    [[ "$relative_path" == scripts/python-env.sh ]] && continue
    source_count=$((source_count + 1))
    copied_path="$copied_framework/$relative_path"
    if [[ ! -f "$copied_path" ]] || ! /usr/bin/cmp -s "$source_path" "$copied_path"; then
      mismatch="content:$relative_path"
      break
    fi
    source_mode="$(scope2_file_mode "$source_path" 2>/dev/null || true)"
    copied_mode="$(scope2_file_mode "$copied_path" 2>/dev/null || true)"
    if [[ -z "$source_mode" || "$copied_mode" != "$source_mode" ]]; then
      mismatch="mode:$relative_path:${source_mode:-missing}:${copied_mode:-missing}"
      break
    fi
  done < <(/usr/bin/find "$framework_root" -type f -print)

  while IFS= read -r copied_path || [[ -n "$copied_path" ]]; do
    [[ -n "$copied_path" ]] || continue
    relative_path="${copied_path#"$copied_framework"/}"
    [[ "$relative_path" == scripts/python-env.sh ]] && continue
    copied_count=$((copied_count + 1))
  done < <(/usr/bin/find "$copied_framework" -type f -print)

  SCOPE2_FIXED_PERL_SIBLING_COUNT=$source_count
  SCOPE2_FIXED_PERL_SIBLING_MISMATCH="$mismatch"
  [[ -z "$mismatch" && "$source_count" -gt 0 && "$copied_count" -eq "$source_count" ]]
}

scope2_fixed_perl_region_reference_count() {
  local path="$1"
  local needle="$2"
  /usr/bin/awk -v needle="$needle" '
    /^_bubbles_python_run_closed_operation\(\) \{/ { in_operation=1; regions++ }
    /^bubbles_python_run_security_operation\(\) \{/ { in_operation=0 }
    in_operation && index($0, needle) > 0 { references++ }
    END { print regions + 0, references + 0 }
  ' "$path"
}

SCOPE2_FIXED_PERL_SOURCE_HASH=''
SCOPE2_FIXED_PERL_MUTANT_HASH=''
SCOPE2_FIXED_PERL_SOURCE_MODE=''
SCOPE2_FIXED_PERL_MUTATION_MATCHES=0
SCOPE2_FIXED_PERL_REFERENCE_COUNT=0

scope2_mutate_fixed_perl_anchor() {
  local candidate="$1"
  local replacement="$2"
  local temporary="$candidate.b039-mutating"
  local reconstructed="$candidate.b039-reconstructed"
  local source_hash_before=""
  local source_hash_after=""
  local copied_hash_before=""
  local copied_hash_after=""
  local source_mode=""
  local copied_mode=""
  local source_shape=""
  local copied_shape=""

  [[ "$replacement" =~ ^/[A-Za-z0-9._/-]+$ ]] || return 42
  source_hash_before="$(scope2_sha256_file "$ENV_SH")" || return 42
  copied_hash_before="$(scope2_sha256_file "$candidate")" || return 42
  source_mode="$(scope2_file_mode "$ENV_SH")" || return 42
  copied_mode="$(scope2_file_mode "$candidate")" || return 42
  [[ "$source_hash_before" == "$copied_hash_before" && "$source_mode" == "$copied_mode" ]] || return 42
  source_shape="$(scope2_fixed_perl_region_reference_count "$ENV_SH" /usr/bin/perl)" || return 42
  [[ "$source_shape" == '1 3' ]] || return 42

  if ! /usr/bin/awk -v replacement="$replacement" '
    /^_bubbles_python_run_closed_operation\(\) \{/ { in_operation=1; regions++ }
    /^bubbles_python_run_security_operation\(\) \{/ { in_operation=0 }
    {
      line=$0
      if (in_operation) replacements += gsub(/\/usr\/bin\/perl/, replacement, line)
      print line
    }
    END { if (regions != 1 || replacements != 3) exit 42 }
  ' "$candidate" >"$temporary"; then
    /bin/rm -f "$temporary"
    return 42
  fi
  /bin/chmod "$source_mode" "$temporary" || {
    /bin/rm -f "$temporary"
    return 42
  }
  /bin/mv "$temporary" "$candidate" || return 42

  copied_shape="$(scope2_fixed_perl_region_reference_count "$candidate" "$replacement")" || return 42
  [[ "$copied_shape" == '1 3' ]] || return 42
  [[ "$(scope2_fixed_perl_region_reference_count "$candidate" /usr/bin/perl)" == '1 0' ]] || return 42

  /usr/bin/awk -v replacement="$replacement" '
    /^_bubbles_python_run_closed_operation\(\) \{/ { in_operation=1 }
    /^bubbles_python_run_security_operation\(\) \{/ { in_operation=0 }
    {
      line=$0
      if (in_operation && index(line, replacement) > 0) {
        prefix=substr(line, 1, index(line, replacement) - 1)
        suffix=substr(line, index(line, replacement) + length(replacement))
        line=prefix "/usr/bin/perl" suffix
      }
      print line
    }
  ' "$candidate" >"$reconstructed" || return 42
  if ! /usr/bin/cmp -s "$ENV_SH" "$reconstructed"; then
    /bin/rm -f "$reconstructed"
    return 42
  fi
  /bin/rm -f "$reconstructed"

  source_hash_after="$(scope2_sha256_file "$ENV_SH")" || return 42
  copied_hash_after="$(scope2_sha256_file "$candidate")" || return 42
  copied_mode="$(scope2_file_mode "$candidate")" || return 42
  [[ "$source_hash_after" == "$source_hash_before" &&
    "$copied_hash_after" != "$source_hash_before" &&
    "$copied_mode" == "$source_mode" ]] || return 42

  SCOPE2_FIXED_PERL_SOURCE_HASH="$source_hash_after"
  SCOPE2_FIXED_PERL_MUTANT_HASH="$copied_hash_after"
  SCOPE2_FIXED_PERL_SOURCE_MODE="$source_mode"
  SCOPE2_FIXED_PERL_MUTATION_MATCHES=1
  SCOPE2_FIXED_PERL_REFERENCE_COUNT=3
  return 0
}

scope2_assert_fixed_perl_negative() {
  local mode="$1"
  local expected_diagnostic="$2"
  local framework_root=""
  local candidate_root="$TMP_ROOT/fixed-perl-$mode-candidate"
  local copied_framework="$candidate_root/bubbles"
  local copied_env="$copied_framework/scripts/python-env.sh"
  local copied_scanner="$copied_framework/scripts/implementation-reality-scan.sh"
  local fallback_bin="$candidate_root/fallback-bin"
  local fallback_marker="$candidate_root/fallback-perl.executed"
  local anchor_marker="$candidate_root/anchor-perl.executed"
  local worker_marker="$candidate_root/worker.executed"
  local worker_path="$candidate_root/worker-probe"
  local anchor_path=""
  local module_output="$candidate_root/module-negative.output"
  local scanner_output="$candidate_root/scanner-negative.output"
  local module_status=0
  local scanner_status=0
  local private_before=0
  local private_after=0
  local mutation_residue_count=0
  local residue_path=""

  framework_root="$(cd "$SCRIPT_DIR/.." && pwd -P)"
  case "$mode" in
    absent)
      anchor_path=/usr/bin/bubbles-bug039-absent-perl
      if [[ -e "$anchor_path" || -L "$anchor_path" ]]; then
        bad "TP-S2-04 fixed-Perl absent SETUP: controlled absent anchor unexpectedly exists"
        return
      fi
      ;;
    untrusted)
      anchor_path="$candidate_root/caller-owned-perl"
      ;;
    *)
      bad "TP-S2-04 fixed-Perl SETUP: unknown negative-control mode '$mode'"
      return
      ;;
  esac

  /bin/mkdir -p "$candidate_root" "$fallback_bin"
  /bin/cp -Rp "$framework_root" "$copied_framework" || {
    bad "TP-S2-04 fixed-Perl $mode SETUP: production framework copy failed"
    return
  }
  /bin/mkdir -p "$candidate_root/specs/001-fixed-perl-negative" "$candidate_root/src"
  cat >"$candidate_root/specs/001-fixed-perl-negative/scopes.md" <<'EOF'
# Scope 1: Fixed Perl negative control

### Implementation Files

- `src/fixed-perl-negative.js`
EOF
  cat >"$candidate_root/src/fixed-perl-negative.js" <<'EOF'
export function persistForbiddenFixedPerlCredential(authToken) {
  localStorage.setItem("authToken", authToken);
}
EOF
  cat >"$fallback_bin/perl" <<EOF
#!/bin/bash
printf '%s\n' fallback-perl >"$fallback_marker"
exec /usr/bin/perl "\$@"
EOF
  /bin/chmod 700 "$fallback_bin/perl"
  cat >"$worker_path" <<EOF
#!/bin/bash
printf '%s\n' worker >"$worker_marker"
printf '%s' bubbles-python-runs
EOF
  /bin/chmod 700 "$worker_path"
  if [[ "$mode" == untrusted ]]; then
    cat >"$anchor_path" <<EOF
#!/bin/bash
printf '%s\n' anchor-perl >"$anchor_marker"
exec /usr/bin/perl "\$@"
EOF
    /bin/chmod 700 "$anchor_path"
  fi

  if ! scope2_verify_fixed_perl_candidate_siblings "$framework_root" "$copied_framework" ||
    ! /usr/bin/cmp -s "$ENV_SH" "$copied_env"; then
    bad "TP-S2-04 fixed-Perl $mode SETUP: copied candidate did not preserve source siblings and modes ($SCOPE2_FIXED_PERL_SIBLING_MISMATCH)"
    return
  fi
  if ! scope2_mutate_fixed_perl_anchor "$copied_env" "$anchor_path"; then
    bad "TP-S2-04 fixed-Perl $mode SETUP: fixed anchor mutation was not one reversible three-reference change"
    return
  fi
  if ! /bin/bash -n "$copied_env" ||
    ! scope2_verify_fixed_perl_candidate_siblings "$framework_root" "$copied_framework"; then
    bad "TP-S2-04 fixed-Perl $mode SETUP: mutated candidate changed a sibling, mode, or Bash syntax"
    return
  fi
  ok "TP-S2-04 fixed-Perl $mode copy changes one operation region, changes its hash, and preserves modes plus $SCOPE2_FIXED_PERL_SIBLING_COUNT siblings"

  private_before="$(scope2_security_private_root_count)"
  if /usr/bin/env -i \
    LC_ALL=C \
    PATH="$fallback_bin:/usr/bin:/bin" \
    BUBBLES_SECURITY_ENTRY_MODE=direct \
    /bin/bash -p -c '
      copied_env="$1"
      worker_path="$2"
      mode="$3"
      boundary_status=0
      operation_status=0
      boundary_record=""
      . "$copied_env"
      boundary_record="$(bubbles_python_security_require_boundary 2>/dev/null)" || boundary_status=$?
      if [[ "$boundary_status" -eq 0 &&
        "$boundary_record" == "$(printf "ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect")" ]]; then
        boundary_state=valid
      else
        boundary_state=invalid
      fi
      _bubbles_python_run_closed_operation general-probe "$worker_path" || operation_status=$?
      if [[ -z "$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" ]]; then control_state=empty; else control_state=present; fi
      if [[ -z "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" ]]; then private_state=empty; else private_state=present; fi
      printf "FIXED_PERL_MODULE mode=%s boundary=%s operationStatus=%s runStatus=%s diagnostic=%s rejection=%s operation=%s owner=%s timedOut=%s workerKind=%s stdoutBytes=%s stderrBytes=%s supervisor=%s supervisorProtocol=%s controlPath=%s privateRoot=%s\n" \
        "$mode" "$boundary_state" "$operation_status" \
        "$BUBBLES_PYTHON_SECURITY_RUN_STATUS" "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" \
        "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_REJECTION" "$BUBBLES_PYTHON_SECURITY_RUN_OPERATION" \
        "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" "$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT" \
        "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" \
        "$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES" "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_CONTRACT" \
        "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_PROTOCOL" "$control_state" "$private_state"
    ' _ "$copied_env" "$worker_path" "$mode" >"$module_output" 2>&1; then
    module_status=0
  else
    module_status=$?
  fi
  /bin/cat "$module_output"

  if [[ "$module_status" -eq 0 ]] &&
    /usr/bin/grep -Fq "FIXED_PERL_MODULE mode=$mode boundary=valid operationStatus=127 runStatus=127 diagnostic=$expected_diagnostic" "$module_output" &&
    /usr/bin/grep -Fq 'operation=general-probe owner=supervisor timedOut=0 workerKind=not-started stdoutBytes=0 stderrBytes=0 supervisor=none supervisorProtocol=none controlPath=empty privateRoot=empty' "$module_output"; then
    if [[ "$mode" == absent ]] && /usr/bin/grep -Fq 'rejection=ABSENT' "$module_output"; then
      ok "TP-S2-04 fixed-Perl absent real security operation returns 127/SUPERVISOR_UNAVAILABLE before worker launch"
    elif [[ "$mode" == untrusted ]] &&
      /usr/bin/grep -Eq 'rejection=(ANCESTOR_OWNER|ANCESTOR_MODE_WRITABLE|ANCESTOR_CALLER_WRITABLE|TARGET_OWNER|TARGET_MODE_WRITABLE|TARGET_CALLER_WRITABLE|TARGET_FORMAT)' "$module_output"; then
      ok "TP-S2-04 fixed-Perl untrusted real security operation returns 127/SUPERVISOR_UNTRUSTED before worker launch"
    else
      bad "TP-S2-04 fixed-Perl $mode returned the wrong closed path rejection"
    fi
  else
    bad "TP-S2-04 fixed-Perl $mode did not reach the real operation's exact fail-closed tuple"
  fi

  if (
    cd "$candidate_root" || exit 2
    /usr/bin/env -i \
      LC_ALL=C \
      PATH="$fallback_bin:/usr/bin:/bin" \
      DEVELOPER_DIR=/Library/Developer/CommandLineTools \
      BUBBLES_SECURITY_ENTRY_MODE=direct \
      /bin/bash -p "$copied_scanner" "$candidate_root/specs/001-fixed-perl-negative" \
      >"$scanner_output" 2>&1 </dev/null
  ); then
    scanner_status=0
  else
    scanner_status=$?
  fi

  if [[ "$scanner_status" -eq 1 ]] &&
    /usr/bin/grep -Fq $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' "$scanner_output" &&
    /usr/bin/grep -Fq 'sensitive-storage classifier unavailable: status=127 diagnostic=SECURITY_RUNTIME_UNAVAILABLE' "$scanner_output" &&
    /usr/bin/grep -Fq 'supervisor=none supervisorProtocol=none' "$scanner_output" &&
    /usr/bin/grep -Fq 'classifierProtocol=none' "$scanner_output" &&
    /usr/bin/grep -Fq 'VIOLATION [SENSITIVE_CLIENT_STORAGE]' "$scanner_output" &&
    ! /usr/bin/grep -Fq 'supervisorProtocol=BPS1' "$scanner_output" &&
    ! /usr/bin/grep -Fq 'classifierProtocol=SCS1' "$scanner_output"; then
    ok "TP-S2-04 fixed-Perl $mode copied scanner fails closed with no clean BPS1 or SCS1"
  else
    /bin/cat "$scanner_output"
    bad "TP-S2-04 fixed-Perl $mode copied scanner did not preserve fail-closed status and protocol absence (exit=$scanner_status)"
  fi

  if [[ ! -e "$fallback_marker" && ! -e "$anchor_marker" && ! -e "$worker_marker" ]]; then
    ok "TP-S2-04 fixed-Perl $mode executes no anchor, PATH fallback, or worker marker"
  else
    bad "TP-S2-04 fixed-Perl $mode executed a forbidden anchor, PATH fallback, or worker marker"
  fi

  private_after="$(scope2_security_private_root_count)"
  while IFS= read -r residue_path || [[ -n "$residue_path" ]]; do
    [[ -n "$residue_path" ]] || continue
    mutation_residue_count=$((mutation_residue_count + 1))
  done < <(/usr/bin/find "$candidate_root" \
    \( -type p -o -type d -name __pycache__ -o -type f -name '*.pyc' \
    -o -type f -name '*.b039-mutating' -o -type f -name '*.b039-reconstructed' \) -print)
  if [[ "$private_before" -eq "$private_after" && "$mutation_residue_count" -eq 0 ]] &&
    scope2_verify_fixed_perl_candidate_siblings "$framework_root" "$copied_framework" &&
    [[ "$(scope2_sha256_file "$ENV_SH")" == "$SCOPE2_FIXED_PERL_SOURCE_HASH" ]]; then
    ok "TP-S2-04 fixed-Perl $mode leaves production bytes, siblings, private roots, FIFOs, bytecode, and mutation temporaries unchanged"
  else
    bad "TP-S2-04 fixed-Perl $mode residue/integrity mismatch private=$private_before/$private_after mutationResidue=$mutation_residue_count siblings=$SCOPE2_FIXED_PERL_SIBLING_MISMATCH"
  fi

  printf 'TP-S2-04_FIXED_PERL_NEGATIVE mode=%s status=127 diagnostic=%s mutationMatches=%s anchorReferences=%s sourceHash=%s mutantHash=%s modeBits=%s siblingFiles=%s fallback=absent workerMarker=absent BPS1=absent SCS1=absent residue=0 retry=0\n' \
    "$mode" "$expected_diagnostic" "$SCOPE2_FIXED_PERL_MUTATION_MATCHES" \
    "$SCOPE2_FIXED_PERL_REFERENCE_COUNT" "$SCOPE2_FIXED_PERL_SOURCE_HASH" \
    "$SCOPE2_FIXED_PERL_MUTANT_HASH" "$SCOPE2_FIXED_PERL_SOURCE_MODE" \
    "$SCOPE2_FIXED_PERL_SIBLING_COUNT"
  /bin/rm -rf "$candidate_root"
  if [[ ! -e "$candidate_root" ]]; then
    ok "TP-S2-04 fixed-Perl $mode copied candidate is removed after verification"
  else
    bad "TP-S2-04 fixed-Perl $mode copied candidate remains after verification"
  fi
}

if [[ "$RUN_SECURITY" -eq 1 || "$RUN_FIXED_PERL" -eq 1 ||
  "$RUN_SIGNAL_READINESS" -eq 1 || "$RUN_SIGNAL_TARGET" -eq 1 ]]; then
  if ! declare -F bubbles_python_security_require_boundary >/dev/null 2>&1; then
    printf '%s\n' 'RED: TP-S2-01 SEC-R1 privilegedChild=ready BSEC1=missing boundaryApi=absent'
    bad "TP-S2-01 SEC-R1: actual privileged child cannot establish production BSEC1"
  else
    BUBBLES_SECURITY_ENTRY_MODE='direct'
    export BUBBLES_SECURITY_ENTRY_MODE
    if SECURITY_BOUNDARY_RECORD="$(bubbles_python_security_require_boundary 2>/dev/null)" &&
      [[ "$SECURITY_BOUNDARY_RECORD" == $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' ]]; then
      SECURITY_BOUNDARY_READY=1
      ok "TP-S2-01 SEC-R1: production boundary establishes direct BSEC1 inside env -i /bin/bash -p"
    else
      printf 'RED: TP-S2-01 SEC-R1 privilegedChild=ready BSEC1=%s boundaryApi=present\n' \
        "${SECURITY_BOUNDARY_RECORD:-invalid}"
      bad "TP-S2-01 SEC-R1: production boundary did not establish the exact direct BSEC1 record"
    fi
  fi
fi

if [[ ( "$RUN_SECURITY" -eq 1 || "$RUN_FIXED_PERL" -eq 1 ) &&
  "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
  echo "Scenario: TP-S2-04 fixed-Perl absence and untrusted-anchor controls fail closed without fallback."
  scope2_assert_fixed_perl_negative absent SUPERVISOR_UNAVAILABLE
  scope2_assert_fixed_perl_negative untrusted SUPERVISOR_UNTRUSTED
  printf '%s\n' 'TP-S2-04_FIXED_PERL_NEGATIVE_MATRIX_COMPLETED=1 cases=2 retries=0'
fi

SCOPE2_NATIVE_CONTRACT_MISSING=''

scope2_native_contract_ready() {
  local source_file="$1"
  local missing=''

  /usr/bin/grep -qE '/usr/bin/perl[[:space:]].*-T|-T[[:space:]].*/usr/bin/perl' "$source_file" || missing="$missing perl-taint"
  /usr/bin/grep -Fq 'root-protected-perl-supervisor-v1' "$source_file" || missing="$missing supervisor-contract"
  /usr/bin/grep -qE 'waitpid[[:space:]]*\(' "$source_file" || missing="$missing waitpid"
  /usr/bin/grep -Fq 'COMPLETE\tBPS1' "$source_file" || missing="$missing bps1-complete"
  /usr/bin/grep -qE 'worker(_is)?_owned.*=[[:space:]]*0|owned_worker.*=[[:space:]]*0' "$source_file" || missing="$missing owner-clear"
  /usr/bin/grep -qE 'kill.*worker|kill.*worker_pid' "$source_file" || missing="$missing owned-worker-signal"
  /usr/bin/grep -qEi 'close.*(control|bps)' "$source_file" || missing="$missing worker-control-close"
  /usr/bin/grep -qE 'EOF|eof|read.*==[[:space:]]*0' "$source_file" || missing="$missing eof-data-state"
  /usr/bin/grep -Fq 'BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID' "$source_file" || missing="$missing supervisor-wait-handle"
  /usr/bin/grep -qE '^bubbles_python_security_require_boundary\(\)' "$source_file" || missing="$missing boundary-api"
  /usr/bin/grep -qE '^bubbles_python_run_security_operation\(\)' "$source_file" || missing="$missing closed-operation-api"
  /usr/bin/grep -qE 'print\("RUNTIME\\tPYSEC1' "$source_file" || missing="$missing runtime-worker"

  SCOPE2_NATIVE_CONTRACT_MISSING="${missing# }"
  [[ -z "$SCOPE2_NATIVE_CONTRACT_MISSING" ]]
}

scope2_native_contract_structurally_valid() {
  local source_file="$1"
  /usr/bin/awk '
    BEGIN {
      supervisor_wait_clear="BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=" sprintf("%c%c", 39, 39)
      supervisor_wait_start="BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=$!"
    }
    /^_bubbles_python_security_supervisor_program\(\) \{/ { in_supervisor=1 }
    in_supervisor && waitpid_line == 0 && /waitpid[[:space:]]*\(/ { waitpid_line=NR }
    in_supervisor && complete_line == 0 && /COMPLETE\\tBPS1/ { complete_line=NR }
    in_supervisor && /worker(_is)?_owned.*=[[:space:]]*0|owned_worker.*=[[:space:]]*0/ {
      if (owner_clear_line == 0) owner_clear_line=NR
      if (complete_line > 0 && NR > complete_line) owner_clear_after_complete=1
    }
    in_supervisor && /kill.*worker|kill.*worker_pid/ {
      if (worker_kill_line == 0) worker_kill_line=NR
      if (owner_clear_line > 0 && NR > owner_clear_line) signal_after_reap=1
    }
    in_supervisor && control_close_line == 0 && /^[[:space:]]*close[[:space:]]*\(CONTROL\)/ && tolower($0) ~ /worker.*(control|bps).*descriptor/ { control_close_line=NR }
    in_supervisor && /^PERL$/ { in_supervisor=0 }
    /^_bubbles_python_run_closed_operation\(\) \{/ { in_run=1 }
    in_run && /print\("COMPLETE\\tBPS1/ { worker_bps1=1 }
    in_run && supervisor_launch_line == 0 && index($0, supervisor_wait_start) > 0 { supervisor_launch_line=NR }
    in_run && supervisor_wait_line == 0 && /builtin wait .*BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID/ { supervisor_wait_line=NR }
    in_run && supervisor_wait_line > 0 && supervisor_clear_line == 0 && index($0, supervisor_wait_clear) > 0 { supervisor_clear_line=NR }
    in_run && /bubbles_python_security_cleanup/ {
      if (supervisor_launch_line > 0 && supervisor_clear_line == 0) cleanup_before_reap=1
      if (supervisor_clear_line > 0 && cleanup_line == 0) cleanup_line=NR
    }
    /^bubbles_python_run_security_operation\(\) \{/ { in_run=0; in_api=1 }
    in_api && /caller-program/ { caller_vector=1 }
    END {
      valid=(waitpid_line > 0 && complete_line > waitpid_line &&
        owner_clear_line >= waitpid_line && owner_clear_line < complete_line &&
        owner_clear_after_complete == 0 && worker_kill_line > 0 && signal_after_reap == 0 &&
        control_close_line > 0 && control_close_line < complete_line &&
        supervisor_launch_line > 0 && supervisor_wait_line > supervisor_launch_line &&
        supervisor_clear_line > supervisor_wait_line && cleanup_before_reap == 0 &&
        cleanup_line > supervisor_clear_line && worker_bps1 == 0 && caller_vector == 0)
      exit valid ? 0 : 1
    }
  ' "$source_file"
}

SCOPE2_SIGNAL_BOUNDARY_TOKEN='supervisor-wait-trap-ready-v1'
SCOPE2_SIGNAL_READY_FIFO=''
SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT=0
SCOPE2_SIGNAL_BOUNDARY_INVALID=0
SCOPE2_SIGNAL_BOUNDARY_TRAP_READY=0
SCOPE2_SIGNAL_BOUNDARY_WAIT_READY=0
SCOPE2_SIGNAL_ARM_STATUS=125
SCOPE2_SIGNAL_SENDER_STATUS=125
SCOPE2_SIGNAL_RUN_STATUS=125
SCOPE2_SIGNAL_TARGET_STATUS=125
SCOPE2_SIGNAL_RESULT_COMPLETE=0
SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED=0
SCOPE2_SIGNAL_OUTER_TRAP_COUNT=0
SCOPE2_SIGNAL_HOLD_FIFO=''
SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED=0
SCOPE2_SIGNAL_RELEASE_STATUS=125

scope2_signal_sender() {
  local signal_name="$1"
  local target_pid="$2"
  local readiness_timeout="$3"
  local readiness_fifo="$4"
  local readiness_record=""
  local readiness_status=0

  # This descriptor opens only in the already-forked harness sender. Neither
  # the Perl supervisor nor its worker can inherit the readiness channel.
  exec 7<>"$readiness_fifo" || return 125
  builtin printf '%s\n' 'SENDER_ARMED'
  if builtin read -r -t "$readiness_timeout" readiness_record <&7; then
    readiness_status=0
  else
    readiness_status=$?
  fi
  exec 7>&-
  if [[ "$readiness_status" -ne 0 ||
    "$readiness_record" != "$SCOPE2_SIGNAL_BOUNDARY_TOKEN" ]]; then
    return 124
  fi
  builtin kill -"$signal_name" "$target_pid"
}

scope2_signal_wait_boundary_debug() {
  local command_text="$1"
  local active_signal_trap=""
  local expected_status=0
  local hold_record=""
  local hold_status=0

  # Observe the production wait command itself. This says nothing about shell
  # cleanliness before the privileged boundary and grants no worker authority.
  [[ "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" -eq 0 ]] || return 0
  [[ "$command_text" == "builtin wait \"\$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID\"" ]] || return 0
  SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT=1
  case "$SCOPE2_SIGNAL_EXPECTED_NAME" in
    HUP) expected_status=129 ;;
    INT) expected_status=130 ;;
    TERM) expected_status=143 ;;
    *)
      SCOPE2_SIGNAL_BOUNDARY_INVALID=1
      return 0
      ;;
  esac
  active_signal_trap="$(builtin trap -p "$SCOPE2_SIGNAL_EXPECTED_NAME")"
  if [[ "$BUBBLES_PYTHON_SECURITY_STATE" == SUPERVISOR_WAITING &&
    "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" =~ ^[1-9][0-9]*$ ]]; then
    SCOPE2_SIGNAL_BOUNDARY_WAIT_READY=1
  fi
  if [[ "$active_signal_trap" == *"_bubbles_python_security_record_signal $SCOPE2_SIGNAL_EXPECTED_NAME $expected_status"* ]]; then
    SCOPE2_SIGNAL_BOUNDARY_TRAP_READY=1
  fi
  if [[ "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" -ne 1 ||
    "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" -ne 1 ||
    -z "$SCOPE2_SIGNAL_READY_FIFO" || -z "$SCOPE2_SIGNAL_HOLD_FIFO" ]]; then
    SCOPE2_SIGNAL_BOUNDARY_INVALID=1
    return 0
  fi
  exec 8<>"$SCOPE2_SIGNAL_HOLD_FIFO" || {
    SCOPE2_SIGNAL_BOUNDARY_INVALID=1
    return 0
  }
  if ! builtin printf '%s\n' "$SCOPE2_SIGNAL_BOUNDARY_TOKEN" >"$SCOPE2_SIGNAL_READY_FIFO"; then
    SCOPE2_SIGNAL_BOUNDARY_INVALID=1
    exec 8>&-
    return 0
  fi
  if builtin read -r -t 10 hold_record <&8; then
    hold_status=0
  else
    hold_status=$?
  fi
  exec 8>&-
  if [[ ("$hold_status" -eq "$expected_status" ||
    "$hold_record" == SIGNAL_SENT) &&
    "$BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL" == "$SCOPE2_SIGNAL_EXPECTED_NAME" &&
    "$BUBBLES_PYTHON_SECURITY_PENDING_STATUS" -eq "$expected_status" ]]; then
    SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED=1
  else
    SCOPE2_SIGNAL_BOUNDARY_INVALID=1
  fi
}

scope2_signal_target_outer_trap() {
  SCOPE2_SIGNAL_TARGET_OUTER_TRAP_COUNT=$((SCOPE2_SIGNAL_TARGET_OUTER_TRAP_COUNT + 1))
  return 0
}

SCOPE2_SIGNAL_TARGET_FINALIZED=0
SCOPE2_SIGNAL_TARGET_FINALIZE_STATUS=125
SCOPE2_SIGNAL_TARGET_RESULT_PATH=''
SCOPE2_SIGNAL_TARGET_PRIVATE_BEFORE=0
SCOPE2_SIGNAL_TARGET_SAVED_DEBUG_TRAP=''
SCOPE2_SIGNAL_TARGET_HAD_FUNCTRACE=0
SCOPE2_SIGNAL_TARGET_BEFORE_HUP=''
SCOPE2_SIGNAL_TARGET_BEFORE_INT=''
SCOPE2_SIGNAL_TARGET_BEFORE_TERM=''

scope2_signal_target_finalize() {
  local cleanup_status=0
  local private_after=0
  local after_hup=""
  local after_int=""
  local after_term=""
  local outer_traps_restored=0
  local wait_pid_clear=0
  local state_idle=0

  if [[ "$SCOPE2_SIGNAL_TARGET_FINALIZED" -eq 1 ]]; then
    return "$SCOPE2_SIGNAL_TARGET_FINALIZE_STATUS"
  fi

  builtin trap - DEBUG
  if [[ -n "$SCOPE2_SIGNAL_TARGET_SAVED_DEBUG_TRAP" ]]; then
    builtin eval "$SCOPE2_SIGNAL_TARGET_SAVED_DEBUG_TRAP"
  fi
  if [[ "$SCOPE2_SIGNAL_TARGET_HAD_FUNCTRACE" -eq 0 ]]; then
    set +T
  fi

  bubbles_python_security_cleanup || cleanup_status=$?
  private_after="$(scope2_security_private_root_count)"
  after_hup="$(builtin trap -p HUP)"
  after_int="$(builtin trap -p INT)"
  after_term="$(builtin trap -p TERM)"
  if [[ "$SCOPE2_SIGNAL_TARGET_BEFORE_HUP" == "$after_hup" &&
    "$SCOPE2_SIGNAL_TARGET_BEFORE_INT" == "$after_int" &&
    "$SCOPE2_SIGNAL_TARGET_BEFORE_TERM" == "$after_term" ]]; then
    outer_traps_restored=1
  fi
  if [[ -z "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" ]]; then
    wait_pid_clear=1
  fi
  if [[ "$BUBBLES_PYTHON_SECURITY_STATE" == IDLE ]]; then
    state_idle=1
  fi

  [[ "$SCOPE2_SIGNAL_TARGET_RESULT_PATH" == /* ]] || return 125
  : >"$SCOPE2_SIGNAL_TARGET_RESULT_PATH" || return 125
  /bin/chmod 600 "$SCOPE2_SIGNAL_TARGET_RESULT_PATH" || return 125
  printf 'S2SIG1|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n' \
    "$BUBBLES_PYTHON_SECURITY_RUN_STATUS" \
    "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" \
    "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" "$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT" \
    "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" \
    "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" \
    "$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES" \
    "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" "$SCOPE2_SIGNAL_BOUNDARY_INVALID" \
    "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" \
    "$SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED" "$cleanup_status" \
    "$SCOPE2_SIGNAL_TARGET_PRIVATE_BEFORE" "$private_after" \
    "$outer_traps_restored" "$SCOPE2_SIGNAL_TARGET_OUTER_TRAP_COUNT" \
    "$wait_pid_clear" "$state_idle" \
    >"$SCOPE2_SIGNAL_TARGET_RESULT_PATH" || return 125
  SCOPE2_SIGNAL_TARGET_FINALIZED=1
  SCOPE2_SIGNAL_TARGET_FINALIZE_STATUS=0
  return 0
}

scope2_signal_target() {
  local signal_name="$1"
  local worker_program="$2"
  local readiness_fifo="$3"
  local hold_fifo="$4"
  local result_file="$5"

  SCOPE2_SIGNAL_EXPECTED_NAME="$signal_name"
  SCOPE2_SIGNAL_READY_FIFO="$readiness_fifo"
  SCOPE2_SIGNAL_HOLD_FIFO="$hold_fifo"
  SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT=0
  SCOPE2_SIGNAL_BOUNDARY_INVALID=0
  SCOPE2_SIGNAL_BOUNDARY_TRAP_READY=0
  SCOPE2_SIGNAL_BOUNDARY_WAIT_READY=0
  SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED=0
  SCOPE2_SIGNAL_TARGET_OUTER_TRAP_COUNT=0
  SCOPE2_SIGNAL_TARGET_FINALIZED=0
  SCOPE2_SIGNAL_TARGET_FINALIZE_STATUS=125
  SCOPE2_SIGNAL_TARGET_RESULT_PATH="$result_file"
  SCOPE2_SIGNAL_TARGET_PRIVATE_BEFORE=0
  SCOPE2_SIGNAL_TARGET_SAVED_DEBUG_TRAP=''
  SCOPE2_SIGNAL_TARGET_HAD_FUNCTRACE=0
  SCOPE2_SIGNAL_TARGET_BEFORE_HUP=''
  SCOPE2_SIGNAL_TARGET_BEFORE_INT=''
  SCOPE2_SIGNAL_TARGET_BEFORE_TERM=''

  builtin trap 'scope2_signal_target_outer_trap' HUP
  builtin trap 'scope2_signal_target_outer_trap' INT
  builtin trap 'scope2_signal_target_outer_trap' TERM
  SCOPE2_SIGNAL_TARGET_BEFORE_HUP="$(builtin trap -p HUP)"
  SCOPE2_SIGNAL_TARGET_BEFORE_INT="$(builtin trap -p INT)"
  SCOPE2_SIGNAL_TARGET_BEFORE_TERM="$(builtin trap -p TERM)"
  SCOPE2_SIGNAL_TARGET_PRIVATE_BEFORE="$(scope2_security_private_root_count)"

  SCOPE2_SIGNAL_TARGET_SAVED_DEBUG_TRAP="$(builtin trap -p DEBUG)"
  case "$-" in *T*) SCOPE2_SIGNAL_TARGET_HAD_FUNCTRACE=1 ;; esac
  set -T
  # BASH_COMMAND expands when the DEBUG trap runs.
  # shellcheck disable=SC2016
  builtin trap 'scope2_signal_wait_boundary_debug "$BASH_COMMAND"' DEBUG
  if _bubbles_python_run_closed_operation general-probe "$worker_program"; then
    :
  else
    :
  fi
  scope2_signal_target_finalize
}

scope2_run_synchronized_parent_signal() {
  local signal_name="$1"
  local worker_program="$2"
  local case_id="$3"
  local readiness_fifo="$TMP_ROOT/$case_id.readiness.fifo"
  local hold_fifo="$TMP_ROOT/$case_id.signal-hold.fifo"
  local result_file="$TMP_ROOT/$case_id.signal-result"
  local readiness_record=""
  local target_pid=""
  local read_status=0
  local send_status=0
  local release_status=0
  local target_status=0
  local protocol=""
  local result_status=125
  local result_diagnostic="NOT_RUN"
  local result_owner="supervisor"
  local result_timed_out=0
  local result_worker_kind="not-started"
  local result_stdout_bytes=0
  local result_stderr_bytes=0
  local result_boundary_count=0
  local result_boundary_invalid=1
  local result_trap_ready=0
  local result_wait_ready=0
  local result_hold_acknowledged=0
  local result_cleanup=1
  local result_private_before=0
  local result_private_after=0
  local result_outer_restored=0
  local result_outer_count=0
  local result_wait_clear=0
  local result_state_idle=0

  SCOPE2_SIGNAL_READY_FIFO="$readiness_fifo"
  SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT=0
  SCOPE2_SIGNAL_BOUNDARY_INVALID=0
  SCOPE2_SIGNAL_BOUNDARY_TRAP_READY=0
  SCOPE2_SIGNAL_BOUNDARY_WAIT_READY=0
  SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED=0
  SCOPE2_SIGNAL_RELEASE_STATUS=125
  SCOPE2_SIGNAL_ARM_STATUS=125
  SCOPE2_SIGNAL_SENDER_STATUS=125
  SCOPE2_SIGNAL_RUN_STATUS=125
  SCOPE2_SIGNAL_TARGET_STATUS=125
  SCOPE2_SIGNAL_RESULT_COMPLETE=0
  SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED=0
  SCOPE2_SIGNAL_OUTER_TRAP_COUNT=0

  case "$signal_name" in HUP | INT | TERM) ;; *) return 2 ;; esac
  [[ "$case_id" =~ ^[a-z0-9-]+$ && -x "$worker_program" ]] || return 2
  /usr/bin/mkfifo "$readiness_fifo" "$hold_fifo" || return 125
  # shellcheck disable=SC2016
  /usr/bin/env -i LC_ALL=C PATH=/usr/bin:/bin BUBBLES_SECURITY_ENTRY_MODE=direct \
    /usr/bin/perl -e \
    '$SIG{HUP}="DEFAULT"; $SIG{INT}="DEFAULT"; $SIG{TERM}="DEFAULT"; exec @ARGV or die "signal target exec failed: $!\n"' \
    /bin/bash -p "$SELFTEST_SCRIPT" --internal-signal-target \
    b039-signal-target-v1 "$signal_name" "$worker_program" \
    "$readiness_fifo" "$hold_fifo" "$result_file" &
  target_pid=$!
  SELFTEST_LIFECYCLE_PID="$target_pid"
  exec 7<>"$readiness_fifo" || return 125
  exec 9<>"$hold_fifo" || {
    exec 7>&-
    selftest_stop_exact_child
    /bin/rm -f "$readiness_fifo" "$hold_fifo" "$result_file"
    return 125
  }
  if builtin read -r -t 10 readiness_record <&7; then
    read_status=0
  else
    read_status=$?
  fi
  exec 7>&-
  if [[ "$read_status" -ne 0 ||
    "$readiness_record" != "$SCOPE2_SIGNAL_BOUNDARY_TOKEN" ]]; then
    exec 9>&-
    SCOPE2_SIGNAL_ARM_STATUS="$read_status"
    selftest_stop_exact_child
    /bin/rm -f "$readiness_fifo"
    /bin/rm -f "$hold_fifo"
    /bin/rm -f "$result_file"
    SCOPE2_SIGNAL_READY_FIFO=''
    return 1
  fi
  SCOPE2_SIGNAL_ARM_STATUS=0

  if builtin kill -"$signal_name" "$target_pid"; then
    send_status=0
  else
    send_status=$?
  fi
  SCOPE2_SIGNAL_SENDER_STATUS="$send_status"
  if builtin printf '%s\n' SIGNAL_SENT >&9; then
    release_status=0
  else
    release_status=$?
  fi
  exec 9>&-
  SCOPE2_SIGNAL_RELEASE_STATUS="$release_status"
  if builtin wait "$target_pid"; then
    target_status=0
  else
    target_status=$?
  fi
  SCOPE2_SIGNAL_TARGET_STATUS="$target_status"
  SELFTEST_LIFECYCLE_PID=''
  /bin/rm -f "$readiness_fifo"
  /bin/rm -f "$hold_fifo"
  SCOPE2_SIGNAL_READY_FIFO=''
  SCOPE2_SIGNAL_HOLD_FIFO=''

  if [[ "$target_status" -eq 0 && -f "$result_file" ]] \
    && IFS='|' builtin read -r protocol result_status result_diagnostic result_owner \
      result_timed_out result_worker_kind result_stdout_bytes result_stderr_bytes \
      result_boundary_count result_boundary_invalid result_trap_ready result_wait_ready \
      result_hold_acknowledged result_cleanup result_private_before result_private_after result_outer_restored \
      result_outer_count result_wait_clear result_state_idle <"$result_file" \
    && [[ "$protocol" == S2SIG1 ]]; then
    SCOPE2_SIGNAL_RESULT_COMPLETE=1
    SCOPE2_SIGNAL_RUN_STATUS="$result_status"
    BUBBLES_PYTHON_SECURITY_RUN_STATUS="$result_status"
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC="$result_diagnostic"
    BUBBLES_PYTHON_SECURITY_RUN_OWNER="$result_owner"
    BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT="$result_timed_out"
    BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND="$result_worker_kind"
    BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES="$result_stdout_bytes"
    BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES="$result_stderr_bytes"
    BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''
    BUBBLES_PYTHON_SECURITY_STATE='IDLE'
    BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT=''
    BUBBLES_PYTHON_SECURITY_STDOUT_PATH=''
    BUBBLES_PYTHON_SECURITY_STDERR_PATH=''
    BUBBLES_PYTHON_SECURITY_CONTROL_PATH=''
    SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT="$result_boundary_count"
    SCOPE2_SIGNAL_BOUNDARY_INVALID="$result_boundary_invalid"
    SCOPE2_SIGNAL_BOUNDARY_TRAP_READY="$result_trap_ready"
    SCOPE2_SIGNAL_BOUNDARY_WAIT_READY="$result_wait_ready"
    SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED="$result_hold_acknowledged"
    SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED="$result_outer_restored"
    SCOPE2_SIGNAL_OUTER_TRAP_COUNT="$result_outer_count"
  fi
  /bin/rm -f "$result_file"
  [[ "$SCOPE2_SIGNAL_ARM_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_SENDER_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_RELEASE_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_TARGET_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_RESULT_COMPLETE" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_INVALID" -eq 0 &&
    "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" -eq 1 &&
    "$SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED" -eq 1 &&
    "$result_cleanup" -eq 0 && "$result_private_before" -eq "$result_private_after" &&
    "$SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED" -eq 1 &&
    "$SCOPE2_SIGNAL_OUTER_TRAP_COUNT" -eq 0 &&
    "$result_wait_clear" -eq 1 && "$result_state_idle" -eq 1 ]]
}

scope2_assert_signal_sender_requires_readiness() {
  local readiness_fifo="$TMP_ROOT/missing-readiness.fifo"
  local armed_fifo="$TMP_ROOT/missing-readiness.armed.fifo"
  local armed_record=""
  local read_status=0
  local sender_pid=""
  local sender_status=0
  local target_pid=""
  local target_status=0

  (
    builtin trap 'builtin exit 88' HUP
    /bin/sleep 2
  ) &
  target_pid=$!
  /usr/bin/mkfifo "$readiness_fifo" "$armed_fifo"
  scope2_signal_sender HUP "$target_pid" 1 "$readiness_fifo" >"$armed_fifo" &
  sender_pid=$!
  SELFTEST_LIFECYCLE_PID="$sender_pid"
  if builtin read -r -t 5 armed_record <"$armed_fifo"; then
    read_status=0
  else
    read_status=$?
  fi
  /bin/rm -f "$armed_fifo"
  if [[ "$read_status" -ne 0 || "$armed_record" != SENDER_ARMED ]]; then
    selftest_stop_exact_child
    builtin kill -TERM "$target_pid" 2>/dev/null || true
    builtin wait "$target_pid" 2>/dev/null || true
    /bin/rm -f "$readiness_fifo"
    bad "TP-S2-05 signal sender reaches its bounded armed state"
    return
  fi
  if builtin wait "$sender_pid"; then sender_status=0; else sender_status=$?; fi
  SELFTEST_LIFECYCLE_PID=''
  /bin/rm -f "$readiness_fifo"
  if builtin wait "$target_pid"; then target_status=0; else target_status=$?; fi
  printf 'TP-S2-05_SIGNAL_READINESS readiness=missing senderStatus=%s targetStatus=%s signalSent=0 timeoutSeconds=1 retries=0 scope=direct-harness-target\n' \
    "$sender_status" "$target_status"
  if [[ "$sender_status" -eq 124 && "$target_status" -eq 0 ]]; then
    ok "TP-S2-05 missing wait-boundary readiness fails boundedly without signaling its direct harness target"
  else
    bad "TP-S2-05 missing wait-boundary readiness expected sender=124 target=0, observed sender=$sender_status target=$target_status"
  fi
}

scope2_assert_focused_parent_signal() {
  local signal_name="$1"
  local signal_slug=""
  local expected_status=0
  local expected_diagnostic=""
  local worker_path=""
  local private_before=0
  local private_after=0
  local synchronization_status=0
  local ordering_status=0
  local cleanup_status=0

  case "$signal_name" in
    HUP)
      signal_slug=hup
      expected_status=129
      expected_diagnostic=SIGNAL_HUP
      ;;
    INT)
      signal_slug=int
      expected_status=130
      expected_diagnostic=SIGNAL_INT
      ;;
    TERM)
      signal_slug=term
      expected_status=143
      expected_diagnostic=SIGNAL_TERM
      ;;
    *)
      bad "TP-S2-05 focused signal name '$signal_name' is invalid"
      return
      ;;
  esac
  worker_path="$TMP_ROOT/focused-signal-$signal_slug.worker"
  cat >"$worker_path" <<'EOF'
#!/bin/bash
printf '%s' 'bubbles-python-runs'
exec /bin/sleep 3
EOF
  /bin/chmod 700 "$worker_path"
  private_before="$(scope2_security_private_root_count)"
  if scope2_run_synchronized_parent_signal \
    "$signal_name" "$worker_path" "focused-signal-$signal_slug"; then
    synchronization_status=0
  else
    synchronization_status=$?
  fi
  scope2_native_contract_structurally_valid "$ENV_SH" || ordering_status=$?
  bubbles_python_security_cleanup || cleanup_status=$?
  private_after="$(scope2_security_private_root_count)"
  printf 'TP-S2-05_FOCUSED_SIGNAL signal=%s status=%s diagnostic=%s owner=%s workerKind=%s stdoutBytes=%s senderStatus=%s releaseStatus=%s targetStatus=%s resultComplete=%s armedStatus=%s boundaryNotifications=%s trapReady=%s waitHandleReady=%s holdAcknowledged=%s boundaryInvalid=%s outerTrapsRestored=%s outerTrapCount=%s ordering=%s cleanup=%s privateRoots=%s/%s retries=0\n' \
    "$signal_name" "$SCOPE2_SIGNAL_RUN_STATUS" "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" \
    "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" \
    "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" "$SCOPE2_SIGNAL_SENDER_STATUS" \
    "$SCOPE2_SIGNAL_RELEASE_STATUS" \
    "$SCOPE2_SIGNAL_TARGET_STATUS" "$SCOPE2_SIGNAL_RESULT_COMPLETE" \
    "$SCOPE2_SIGNAL_ARM_STATUS" "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" \
    "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" \
    "$SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED" "$SCOPE2_SIGNAL_BOUNDARY_INVALID" \
    "$SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED" \
    "$SCOPE2_SIGNAL_OUTER_TRAP_COUNT" "$ordering_status" "$cleanup_status" \
    "$private_before" "$private_after"
  if [[ "$synchronization_status" -eq 0 &&
    "$SCOPE2_SIGNAL_RUN_STATUS" -eq "$expected_status" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" == "$expected_diagnostic" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" == caller-signal &&
    "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" == exit &&
    "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" -eq 19 &&
    "$SCOPE2_SIGNAL_SENDER_STATUS" -eq 0 && "$SCOPE2_SIGNAL_ARM_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_RELEASE_STATUS" -eq 0 &&
    "$SCOPE2_SIGNAL_TARGET_STATUS" -eq 0 && "$SCOPE2_SIGNAL_RESULT_COMPLETE" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" -eq 1 &&
    "$SCOPE2_SIGNAL_HOLD_ACKNOWLEDGED" -eq 1 &&
    "$SCOPE2_SIGNAL_BOUNDARY_INVALID" -eq 0 &&
    "$SCOPE2_SIGNAL_OUTER_TRAPS_RESTORED" -eq 1 &&
    "$SCOPE2_SIGNAL_OUTER_TRAP_COUNT" -eq 0 && "$ordering_status" -eq 0 &&
    "$cleanup_status" -eq 0 && "$private_before" -eq "$private_after" &&
    -z "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" &&
    "$BUBBLES_PYTHON_SECURITY_STATE" == IDLE ]]; then
    ok "TP-S2-05 real parent $signal_name is delivered only at the production supervisor wait with production traps installed"
  else
    bad "TP-S2-05 focused synchronized parent $signal_name did not preserve the native-supervisor signal contract"
  fi
}

assert_native_supervisor_negative_control() {
  local finding="$1"
  local mode="$2"
  local label="$3"
  local mutation_dir="$TMP_ROOT/native-mutation-$mode"
  local mutant="$mutation_dir/python-env.sh"

  if ! scope2_native_contract_ready "$ENV_SH"; then
    printf 'RED: TP-S2-01 %s mutation=%s nativeContract=missing anchors=%s\n' \
      "$finding" "$mode" "${SCOPE2_NATIVE_CONTRACT_MISSING:-unknown}"
    bad "$label: authoritative BPS1/Perl contract is absent before mutation execution"
    return
  fi
  if ! scope2_native_contract_structurally_valid "$ENV_SH"; then
    printf 'RED: TP-S2-01 %s mutation=%s nativeContract=structurally-invalid\n' \
      "$finding" "$mode"
    bad "$label: unmutated native supervisor does not satisfy the authoritative lifecycle predicate"
    return
  fi

  mkdir -p "$mutation_dir"
  if ! make_native_supervisor_mutation "$mode" "$mutant"; then
    bad "$label SETUP: future-contract mutation could not be constructed"
    return
  fi
  if ! /bin/bash -n "$mutant"; then
    bad "$label SETUP: future-contract mutation is not valid Bash"
    return
  fi
  if scope2_native_contract_structurally_valid "$mutant"; then
    bad "$label TEST-CONTRACT: mutation did not turn the lifecycle predicate RED"
    return
  fi

  printf 'RED-CONTROL: TP-S2-01 %s mutation=%s authoritativePredicate=rejected\n' \
    "$finding" "$mode"
  ok "$label mutation turns the authoritative native-supervisor assertion RED"
}

if [[ "$RUN_SECURITY" -eq 1 ]]; then
  echo "Scenario: TP-S2-01 BPS1 and Perl-supervisor mutations must turn the authoritative assertions RED."
  assert_native_supervisor_negative_control SEC-R2 signal-after-reap \
    "TP-S2-01 SEC-R2 signal-after-reap"
  assert_native_supervisor_negative_control SEC-R2 late-owner-clear \
    "TP-S2-01 SEC-R2 late-owner-clear"
  assert_native_supervisor_negative_control SEC-R2 omit-waitpid \
    "TP-S2-01 SEC-R2 omitted-waitpid"
  assert_native_supervisor_negative_control HAR-R2 worker-forged-bps1 \
    "TP-S2-01 HAR-R2 worker-authored forged BPS1"
  assert_native_supervisor_negative_control SEC-R2 caller-vector \
    "TP-S2-01 SEC-R2 caller authority vector"
  assert_native_supervisor_negative_control HAR-R2 worker-control-descriptor \
    "TP-S2-01 HAR-R2 worker control descriptor"
  assert_native_supervisor_negative_control HAR-R2 eof-as-completion \
    "TP-S2-01 HAR-R2 EOF-as-completion"
  assert_native_supervisor_negative_control HAR-R1 cleanup-before-reap \
    "TP-S2-01 HAR-R1 cleanup-before-reap"
  printf '%s\n' 'TP-S2-01_NATIVE_MUTATION_MATRIX_COMPLETED=1'
fi

if [[ "$RUN_SIGNAL_TARGET" -eq 1 && "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
  signal_target_status=0
  if scope2_signal_target "$SIGNAL_TARGET_NAME" "$SIGNAL_TARGET_WORKER" \
    "$SIGNAL_TARGET_READY_FIFO" "$SIGNAL_TARGET_HOLD_FIFO" \
    "$SIGNAL_TARGET_RESULT_FILE"; then
    signal_target_status=0
  else
    signal_target_status=$?
  fi
  SELFTEST_COMPLETED=1
  exit "$signal_target_status"
fi

if [[ "$RUN_SIGNAL_READINESS" -eq 1 && "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
  echo "Scenario: TP-S2-05 parent signals wait for the production supervisor boundary."
  scope2_assert_signal_sender_requires_readiness
  scope2_assert_focused_parent_signal HUP
  scope2_assert_focused_parent_signal INT
  scope2_assert_focused_parent_signal TERM
  printf '%s\n' 'TP-S2-05_SIGNAL_READINESS_CONTROLS_COMPLETED=1 cases=4 signals=HUP,INT,TERM retries=0'
  SCOPE2_SIGNAL_READINESS_COMPLETED=1
fi

assert_selftest_lifecycle_fails_closed() {
  local mode="$1"
  local signal_name="$2"
  local expected_status="$3"
  local label="$4"
  local ready_fifo="$TMP_ROOT/lifecycle-$mode-$signal_name.ready.fifo"
  local output_file="$TMP_ROOT/lifecycle-$mode-$signal_name.log"
  local child_pid=""
  local child_tmp=""
  local child_status=0
  local read_status=0

  /usr/bin/mkfifo "$ready_fifo"
  BUBBLES_PYTHON_SELFTEST_CHILD_MODE="$mode" \
    BUBBLES_PYTHON_SELFTEST_READY_FILE="$ready_fifo" \
    /bin/bash "$SELFTEST_SCRIPT" >"$output_file" 2>&1 </dev/null &
  child_pid=$!
  SELFTEST_LIFECYCLE_PID="$child_pid"
  exec 6<"$ready_fifo"
  if builtin read -r -t 10 child_tmp <&6; then
    read_status=0
  else
    read_status=$?
  fi
  exec 6>&-
  if [[ "$read_status" -ne 0 || -z "$child_tmp" ]]; then
    selftest_stop_exact_child
    bad "$label reaches its bounded ready point"
    return
  fi

  if [[ ! "$SELFTEST_LIFECYCLE_PID" =~ ^[1-9][0-9]*$ ||
    "$SELFTEST_LIFECYCLE_PID" != "$child_pid" ]]; then
    bad "$label exact direct-child registration is invalid before builtin wait"
    SELFTEST_LIFECYCLE_PID="$child_pid"
    selftest_stop_exact_child
    return
  fi
  if [[ "$signal_name" != "NONE" ]]; then
    builtin kill -"$signal_name" "$SELFTEST_LIFECYCLE_PID" 2>/dev/null || true
  fi
  if builtin wait "$SELFTEST_LIFECYCLE_PID" 2>/dev/null; then child_status=0; else child_status=$?; fi
  SELFTEST_LIFECYCLE_PID=''
  if [[ "$child_status" -eq "$expected_status" ]]; then
    ok "$label preserves fatal exit $expected_status"
  else
    bad "$label expected exit $expected_status, got wait=$child_status"
  fi
  if [[ -n "$child_tmp" && ! -e "$child_tmp" ]]; then
    ok "$label removes its temporary tree"
  else
    bad "$label removes its temporary tree (still present: $child_tmp)"
    [[ -z "$child_tmp" ]] || rm -rf "$child_tmp"
  fi
  if /usr/bin/grep -Fq 'python-env selftest:' "$output_file"; then
    bad "$label must not emit a success summary"
  else
    ok "$label emits no success summary"
  fi
}

if [[ "$RUN_GENERAL" -eq 1 ]]; then
  echo "Scenario: premature and interrupted python-env selftests fail closed while cleaning up."
  assert_selftest_lifecycle_fails_closed premature-exit NONE 1 "Premature EXIT"
  assert_selftest_lifecycle_fails_closed timeout-exit NONE 124 "Timeout exit"
  assert_selftest_lifecycle_fails_closed interrupt-hold HUP 129 "HUP interruption"
  assert_selftest_lifecycle_fails_closed interrupt-hold TERM 143 "TERM interruption"

assert_exit() {
  local expected="$1" label="$2"
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label (exit $actual)"
  else
    bad "$label (expected exit $expected, got $actual)"
  fi
}

# make_fake_python <path> <module>... — a shim that imports ONLY the named modules.
# NOTE: fixture subdirectories are deliberately named "venvroot", never "home" —
# a literal Linux home-directory path in a portable surface trips the agnosticity lint.
make_fake_python() {
  local path="$1"
  shift
  mkdir -p "$(dirname "$path")"
  {
    echo '#!/usr/bin/env bash'
    echo '# fake python3 shim for python-env-selftest'
    echo 'if [[ "${1:-}" == "-c" ]]; then'
    echo '  case "${2:-}" in'
    local module
    for module in "$@"; do
      echo "    *\"import $module\"*) exit 0 ;;"
    done
    echo '    *) exit 1 ;;'
    echo '  esac'
    echo 'fi'
    echo 'exit 1'
  } >"$path"
  chmod +x "$path"
}

# ── Case 1: no managed venv, PATH python3 lacks the modules → unsatisfied ──
c1="$TMP_ROOT/c1"
mkdir -p "$c1/venvroot" "$c1/bin"
make_fake_python "$c1/bin/python3" nothingatall
assert_exit 1 "Case 1: nothing satisfies is exit 1" \
  env BUBBLES_PYTHON_HOME="$c1/venvroot" PATH="$c1/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── Case 2: a managed venv that satisfies resolves ─────────────────────────
c2="$TMP_ROOT/c2"
mkdir -p "$c2/venvroot/bin" "$c2/bin"
make_fake_python "$c2/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c2/bin/python3" nothingatall
assert_exit 0 "Case 2: satisfying managed venv is exit 0" \
  env BUBBLES_PYTHON_HOME="$c2/venvroot" PATH="$c2/bin:/usr/bin:/bin" bash "$ENV_SH" --check

resolved="$(env BUBBLES_PYTHON_HOME="$c2/venvroot" PATH="$c2/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c2/venvroot/bin/python3" ]]; then
  ok "Case 2b: --path prints the managed interpreter"
else
  bad "Case 2b: --path printed '$resolved', expected '$c2/venvroot/bin/python3'"
fi

# ── Case 3: PATH python3 that satisfies is accepted when no venv exists ────
c3="$TMP_ROOT/c3"
mkdir -p "$c3/venvroot" "$c3/bin"
make_fake_python "$c3/bin/python3" yaml jsonschema
assert_exit 0 "Case 3: satisfying PATH python3 is accepted" \
  env BUBBLES_PYTHON_HOME="$c3/venvroot" PATH="$c3/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── Case 4: $BUBBLES_PYTHON override wins ─────────────────────────────────
c4="$TMP_ROOT/c4"
mkdir -p "$c4/venvroot/bin" "$c4/bin" "$c4/override"
make_fake_python "$c4/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c4/bin/python3" nothingatall
make_fake_python "$c4/override/python3" yaml jsonschema
resolved="$(env BUBBLES_PYTHON_HOME="$c4/venvroot" BUBBLES_PYTHON="$c4/override/python3" \
  PATH="$c4/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c4/override/python3" ]]; then
  ok "Case 4: BUBBLES_PYTHON override takes precedence"
else
  bad "Case 4: --path printed '$resolved', expected the override"
fi

# ── Case 5: a NON-satisfying override is ignored, not trusted ─────────────
c5="$TMP_ROOT/c5"
mkdir -p "$c5/venvroot/bin" "$c5/bin" "$c5/override"
make_fake_python "$c5/venvroot/bin/python3" yaml jsonschema
make_fake_python "$c5/bin/python3" nothingatall
make_fake_python "$c5/override/python3" yaml
resolved="$(env BUBBLES_PYTHON_HOME="$c5/venvroot" BUBBLES_PYTHON="$c5/override/python3" \
  PATH="$c5/bin:/usr/bin:/bin" bash "$ENV_SH" --path 2>/dev/null || true)"
if [[ "$resolved" == "$c5/venvroot/bin/python3" ]]; then
  ok "Case 5: unsatisfying override falls through to the managed venv"
else
  bad "Case 5: --path printed '$resolved', expected the managed venv"
fi

# ── Case 6: usage errors ──────────────────────────────────────────────────
assert_exit 2 "Case 6: unknown mode is a usage error" bash "$ENV_SH" --bogus
assert_exit 2 "Case 6b: extra argument is a usage error" bash "$ENV_SH" --check extra
assert_exit 0 "Case 6c: --help exits 0" bash "$ENV_SH" --help

# ── Case 7: no bypass flag exists ─────────────────────────────────────────
for flag in --skip --force --ignore; do
  assert_exit 2 "Case 7: $flag is rejected (no bypass)" bash "$ENV_SH" "$flag"
done

# ── Case 8: provisioning refuses a missing requirements file ──────────────
assert_exit 2 "Case 8: missing requirements file is exit 2" \
  env BUBBLES_PYTHON_HOME="$TMP_ROOT/c8home" bash -c \
  '. "$1"; bubbles_python_provision "$2"' _ "$ENV_SH" "$TMP_ROOT/definitely-absent.txt"

# ── ADVERSARIAL A1: partial module satisfaction must NOT count ─────────────
a1="$TMP_ROOT/a1"
mkdir -p "$a1/venvroot/bin" "$a1/bin"
make_fake_python "$a1/venvroot/bin/python3" yaml
make_fake_python "$a1/bin/python3" nothingatall
assert_exit 1 "A1: interpreter with only ONE required module is unsatisfied" \
  env BUBBLES_PYTHON_HOME="$a1/venvroot" PATH="$a1/bin:/usr/bin:/bin" bash "$ENV_SH" --check

# ── ADVERSARIAL A2: activate must not hijack a satisfying PATH python3 ─────
a2="$TMP_ROOT/a2"
mkdir -p "$a2/venvroot/bin" "$a2/bin"
make_fake_python "$a2/venvroot/bin/python3" yaml jsonschema
make_fake_python "$a2/bin/python3" yaml jsonschema
a2_path="$(env BUBBLES_PYTHON_HOME="$a2/venvroot" PATH="$a2/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2_path" in
  "$a2/venvroot/bin":*) bad "A2: activate hijacked a PATH python3 that already satisfies" ;;
  *) ok "A2: activate leaves a satisfying PATH python3 alone" ;;
esac

# ── A2b: activate DOES prepend when PATH python3 does not satisfy ──────────
a2b_path="$(env BUBBLES_PYTHON_HOME="$a1/venvroot" PATH="$a1/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2b_path" in
  *"$a1/venvroot/bin"*) bad "A2b: activate prepended an UNSATISFYING managed venv" ;;
  *) ok "A2b: activate does not prepend an unsatisfying managed venv" ;;
esac

a2c="$TMP_ROOT/a2c"
mkdir -p "$a2c/venvroot/bin" "$a2c/bin"
make_fake_python "$a2c/venvroot/bin/python3" yaml jsonschema
make_fake_python "$a2c/bin/python3" nothingatall
a2c_path="$(env BUBBLES_PYTHON_HOME="$a2c/venvroot" PATH="$a2c/bin:/usr/bin:/bin" bash -c \
  '. "$1"; bubbles_python_activate >/dev/null 2>&1; printf "%s" "$PATH"' _ "$ENV_SH")"
case "$a2c_path" in
  "$a2c/venvroot/bin":*) ok "A2c: activate prepends the managed venv when PATH does not satisfy" ;;
  *) bad "A2c: activate failed to prepend; PATH=$a2c_path" ;;
esac

# ── ADVERSARIAL A3: module list must match dependency-posture.sh ───────────
if [[ -f "$POSTURE_SH" ]]; then
  declared="$(grep -oE '"python-module:[a-zA-Z0-9_]+:' "$POSTURE_SH" |
    sed 's/"python-module://; s/:$//' | LC_ALL=C sort | tr '\n' ' ')"
  owned="$(bash -c '. "$1"; printf "%s\n" "${BUBBLES_PYTHON_MODULES[@]}"' _ "$ENV_SH" |
    LC_ALL=C sort | tr '\n' ' ')"
  if [[ -n "$declared" && "$declared" == "$owned" ]]; then
    ok "A3: BUBBLES_PYTHON_MODULES matches dependency-posture.sh ($owned)"
  else
    bad "A3: module lists drifted — posture='$declared' python-env='$owned'"
  fi
else
  bad "A3: dependency-posture.sh not found at $POSTURE_SH"
fi

# ── Case 9: requirements.txt is pinned and single-index ───────────────────
# Directives only: a comment that NAMES --extra-index-url (the file documents
# why it has none) must not be mistaken for one being declared.
REQ="$SCRIPT_DIR/../requirements.txt"
if [[ -f "$REQ" ]]; then
  directives="$(grep -vE '^[[:space:]]*#' "$REQ" || true)"
  if [[ "$(printf '%s\n' "$directives" | grep -c -- '--extra-index-url' || true)" -eq 0 ]]; then
    ok "Case 9: requirements.txt declares no --extra-index-url"
  else
    bad "Case 9: requirements.txt declares an --extra-index-url"
  fi
  if [[ "$(printf '%s\n' "$directives" | grep -c -- '--index-url' || true)" -eq 1 ]]; then
    ok "Case 9b: requirements.txt pins exactly one index"
  else
    bad "Case 9b: requirements.txt must declare exactly one --index-url"
  fi
  unpinned="$(grep -vE '^[[:space:]]*(#|$|--)' "$REQ" | grep -vE '==' | tr -d ' ' || true)"
  if [[ -z "$unpinned" ]]; then
    ok "Case 9c: every requirement is == pinned"
  else
    bad "Case 9c: unpinned requirement(s): $unpinned"
  fi
else
  bad "Case 9: requirements.txt not found at $REQ"
fi
# ---------------------------------------------------------------------------
# Case 10 (adversarial A4): cli.sh MUST activate the managed interpreter early.
#
# This is the seam that covers the ~15 selftests which call `python3 -c "import
# yaml"` directly without sourcing dependency-posture.sh. If someone deletes it,
# those selftests silently regress to SKIP/empty-value failures instead of
# failing loudly here. Guard it structurally.
# ---------------------------------------------------------------------------
CLI="$SCRIPT_DIR/cli.sh"
if [[ -f "$CLI" ]]; then
  if grep -q 'bubbles_python_activate' "$CLI"; then
    ok "Case 10: cli.sh activates the managed interpreter"
  else
    bad "Case 10: cli.sh no longer calls bubbles_python_activate — every child selftest loses the managed interpreter"
  fi

  # It must activate BEFORE the command dispatch, otherwise children spawned by
  # earlier subcommand handling would miss the exported PATH.
  # NOTE: `|| true` is required — this script runs under `set -euo pipefail`, so
  # a non-matching grep inside a pipeline would abort the whole selftest.
  activate_line="$(grep -n 'bubbles_python_activate' "$CLI" | head -1 | cut -d: -f1 || true)"
  dispatch_line="$(grep -nE '^[[:space:]]*case "\$(command_name|first_word|COMMAND|1|\{1:-\})' "$CLI" | head -1 | cut -d: -f1 || true)"
  if [[ -n "$activate_line" && -n "$dispatch_line" ]]; then
    if [[ "$activate_line" -lt "$dispatch_line" ]]; then
      ok "Case 10b: activation (line $activate_line) precedes command dispatch (line $dispatch_line)"
    else
      bad "Case 10b: activation at line $activate_line runs AFTER dispatch at line $dispatch_line"
    fi
  else
    ok "Case 10b: dispatch marker not matched; Case 10 already guards presence"
  fi
else
  bad "Case 10: cli.sh not found at $CLI"
fi

# ---------------------------------------------------------------------------
# Case 11 (adversarial A5): the pinned closure must cover every required module.
# A requirements.txt that installs PyYAML but forgets jsonschema would provision
# "successfully" and then fail at first use.
# ---------------------------------------------------------------------------
if [[ -f "$REQ" ]]; then
  missing_mod=""
  for _m in "${BUBBLES_PYTHON_MODULES[@]}"; do
    case "$_m" in
      yaml) _pkg='PyYAML' ;;
      jsonschema) _pkg='jsonschema' ;;
      *) _pkg="$_m" ;;
    esac
    grep -qiE "^[[:space:]]*${_pkg}==" "$REQ" || missing_mod="$missing_mod $_m"
  done
  if [[ -z "$missing_mod" ]]; then
    ok "Case 11: requirements.txt pins a distribution for every required module"
  else
    bad "Case 11: no pinned distribution for required module(s):$missing_mod"
  fi
fi

# ---------------------------------------------------------------------------
# BUG-039 additions (Cases 12-15). python-env.sh is the module the framework
# designates to answer "which interpreter", and this packet added new API to it:
# bubbles_python_runs, bubbles_python_resolve_runnable, and a locator contract
# that can now DECLINE instead of always printing.
#
# These exist because of what the file did NOT cover. Every case above passed
# throughout, while the module published /bin/python3 — a path that does not
# exist — as a resolved interpreter whenever no locator was set:
# "${XDG_CACHE_HOME:-$HOME/.cache}" aborted inside a command substitution under
# set -u, the empty result was concatenated with "/bin/python3", and the caller
# then reported "no interpreter satisfies the required modules" — a sentence
# about interpreters, when nothing had been able to name one.
#
# ADVERSARIAL A6 is that exact defect: it goes red against the historical
# unguarded expansion and green against the guarded one.
# ---------------------------------------------------------------------------

# no_locator <command...> — run with every locator variable REMOVED (not
# emptied): this is the state in which the module used to fabricate a path.
no_locator() {
  env -u BUBBLES_PYTHON_HOME -u XDG_CACHE_HOME -u HOME "$@"
}

# Pinned here as a LITERAL rather than read back from python-env.sh. Sourcing
# the module for its own expected value would make the assertion agree with
# whatever the module currently says, which is the opposite of pinning it.
LOCATOR_VARS_EXPECTED='BUBBLES_PYTHON_HOME, XDG_CACHE_HOME, or HOME'

# make_probe_python <path> <mode> — a shim for the EXECUTION probe, which is a
# different question from make_fake_python's module imports. Modes mirror the
# ways a real interpreter can be present and still unusable.
make_probe_python() {
  local path="$1" mode="$2"
  mkdir -p "$(dirname "$path")"
  {
    echo '#!/bin/bash'
    echo '# probe shim for python-env-selftest'
    case "$mode" in
      healthy) echo "printf %s 'bubbles-python-runs'" ;;
      # exits 0 and emits nothing: the wrapper an exit-code-only check accepts.
      silent) echo 'exit 0' ;;
      # emits the payload but on a polluted stdout.
      noisy)
        echo "echo 'warning: deprecated interpreter'"
        echo "printf %s 'bubbles-python-runs'"
        ;;
      # the macOS Xcode-licence shape: resolves, is executable, exits 69.
      dead)
        echo "printf %s 'bubbles-python-runs'"
        echo 'exit 69'
        ;;
      xcode)
        echo "echo 'You have not agreed to the Xcode license agreements.' >&2"
        echo 'exit 69'
        ;;
      hang) echo 'exec /bin/sleep 300' ;;
      malformed) echo "printf %s 'not-the-probe-protocol'" ;;
    esac
  } >"$path"
  chmod +x "$path"
}

# ── SCN-B039-005 RED: caller-owned executables cannot grant security authority ──
# This case intentionally calls the production security resolver. The fake can
# forge every public protocol token used by the historical implementation, but
# a root-protected resolver must neither execute it nor publish it as authority.
fi
if [[ "$RUN_SECURITY" -eq 1 && "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
scope2_forged_root="$TMP_ROOT/scope2-forged-runtime"
scope2_forged_python="$scope2_forged_root/bin/python3"
scope2_forged_marker="$scope2_forged_root/executed.marker"
mkdir -p "$(dirname "$scope2_forged_python")"
cat >"$scope2_forged_python" <<'EOF'
#!/bin/bash
printf '%s\n' 'caller-owned runtime executed' >"${BUBBLES_SCOPE2_FORGED_MARKER:?marker required}"
printf '%s' 'bubbles-python-runs'
printf '\nRUNTIME\tPYSEC1\t3\t9\n'
printf 'COMPLETE\tPYMOD1\t9\n'
printf 'COMPLETE\tSCS1\t1\n'
exit 0
EOF
chmod +x "$scope2_forged_python"

scope2_resolution="$(
  builtin trap - EXIT
    PATH="$(dirname "$scope2_forged_python"):/usr/bin:/bin:/usr/sbin:/sbin"
    BUBBLES_PYTHON="$scope2_forged_python"
    BUBBLES_PYTHON_HOME="$scope2_forged_root"
    BUBBLES_SCOPE2_FORGED_MARKER="$scope2_forged_marker"
    export PATH BUBBLES_PYTHON BUBBLES_PYTHON_HOME BUBBLES_SCOPE2_FORGED_MARKER
    if ! declare -F bubbles_python_resolve_security_runtime >/dev/null 2>&1; then
      printf "API_MISSING"
    elif bubbles_python_resolve_security_runtime >/dev/null; then
      printf "RESOLVED|%s|%s|%s" \
        "$BUBBLES_PYTHON_SECURITY_STATUS" \
        "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" \
        "$BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT"
    else
      printf "DECLINED|%s|%s|%s" \
        "$BUBBLES_PYTHON_SECURITY_STATUS" \
        "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" \
        "$BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT"
    fi
)" 2>/dev/null || true
case "$scope2_resolution" in
  RESOLVED\|0\|OK\|root-protected-native-python-v1 | \
    DECLINED\|*\|*\|root-protected-native-python-v1)
    ok "SCN-B039-005: security resolution publishes only the root-protected trust contract"
    ;;
  *)
    bad "SCN-B039-005: security resolver contract result was '$scope2_resolution'"
    ;;
esac
if [[ ! -e "$scope2_forged_marker" ]]; then
  ok "SCN-B039-005: caller-owned override, managed, and PATH runtime is never executed"
else
  bad "SCN-B039-005: caller-owned runtime executed during security resolution"
fi
if ! declare -F bubbles_python_run_bounded >/dev/null 2>&1 &&
  ! declare -F bubbles_python_resolve_trusted_runnable >/dev/null 2>&1; then
  ok "SCN-B039-005: superseded generic runner and managed trust API are absent"
else
  bad "SCN-B039-005: superseded generic runner or managed trust API remains exposed"
fi
fi

# ── Case 12: the locator order is ordered, not incidental ─────────────────
if [[ "$RUN_GENERAL" -eq 1 ]]; then
c12="$TMP_ROOT/c12"
mkdir -p "$c12/explicit" "$c12/xdg" "$c12/venvroot"

c12a="$(env BUBBLES_PYTHON_HOME="$c12/explicit" XDG_CACHE_HOME="$c12/xdg" HOME="$c12/venvroot" \
  bash -c '. "$1"; bubbles_python_home' _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$c12a" == "$c12/explicit" ]]; then
  ok "Case 12: BUBBLES_PYTHON_HOME outranks XDG_CACHE_HOME and HOME"
else
  bad "Case 12: locator printed '$c12a', expected '$c12/explicit'"
fi

c12b="$(env -u BUBBLES_PYTHON_HOME XDG_CACHE_HOME="$c12/xdg/" HOME="$c12/venvroot" \
  bash -c '. "$1"; bubbles_python_home' _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$c12b" == "$c12/xdg/bubbles/python" ]]; then
  ok "Case 12b: XDG_CACHE_HOME outranks HOME, with the trailing slash normalized"
else
  bad "Case 12b: locator printed '$c12b', expected '$c12/xdg/bubbles/python'"
fi

c12c="$(env -u BUBBLES_PYTHON_HOME -u XDG_CACHE_HOME HOME="$c12/venvroot" \
  bash -c '. "$1"; bubbles_python_home' _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$c12c" == "$c12/venvroot/.cache/bubbles/python" ]]; then
  ok "Case 12c: HOME is the last locator, under .cache"
else
  bad "Case 12c: locator printed '$c12c', expected '$c12/venvroot/.cache/bubbles/python'"
fi

# ── ADVERSARIAL A6: an absent locator is a CONDITION, never a fabricated path ──
# This is the original defect. Under the historical unguarded expansion
# bubbles_python_home returned 0 with empty output and bubbles_python_venv_python
# then published "/bin/python3". Both halves are asserted: a resolver that
# reports success while printing nothing is just as wrong as one that prints a
# path it invented.
a6_home="$(no_locator bash -c '. "$1"; out="$(bubbles_python_home)" || { printf "DECLINED|%s" "$out"; exit 0; }; printf "RESOLVED|%s" "$out"' _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$a6_home" == "DECLINED|" ]]; then
  ok "A6: bubbles_python_home declines when no locator is set"
else
  bad "A6: bubbles_python_home returned '$a6_home', expected 'DECLINED|' (a fabricated or empty-but-successful home is the BUG-039 defect)"
fi

a6_venv="$(no_locator bash -c '. "$1"; out="$(bubbles_python_venv_python)" || { printf "DECLINED|%s" "$out"; exit 0; }; printf "RESOLVED|%s" "$out"' _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$a6_venv" == "DECLINED|" ]]; then
  ok "A6b: bubbles_python_venv_python declines instead of publishing a path"
else
  bad "A6b: bubbles_python_venv_python returned '$a6_venv', expected 'DECLINED|' — publishing a nonexistent interpreter path (historically '/bin/python3') is the BUG-039 defect"
fi

# ── ADVERSARIAL A7: usability is proven by PAYLOAD, not by exit code ──────
a7="$TMP_ROOT/a7"
make_probe_python "$a7/healthy/python3" healthy
make_probe_python "$a7/silent/python3" silent
make_probe_python "$a7/noisy/python3" noisy
make_probe_python "$a7/dead/python3" dead

# probe_runs <interpreter> — echo yes/no for bubbles_python_runs.
probe_runs() {
  no_locator bash -c '. "$1"; . "$2"; if bubbles_python_runs "$3"; then echo yes; else echo no; fi' \
    _ "$GUARD_LIB" "$ENV_SH" "$1" 2>/dev/null || echo no
}

assert_runs() {
  local mode="$1" want="$2" why="$3" got
  got="$(probe_runs "$a7/$mode/python3")"
  if [[ "$got" == "$want" ]]; then
    ok "A7 ($mode): $why"
  else
    bad "A7 ($mode): bubbles_python_runs said '$got', expected '$want' — $why"
  fi
}

assert_runs healthy yes "an interpreter that executes and returns the payload is usable"
assert_runs silent no "an interpreter that exits 0 while producing NOTHING is not usable"
assert_runs noisy no "an interpreter that pollutes stdout cannot pass the payload check"
assert_runs dead no "an interpreter that emits the payload and then exits 69 is not usable"

# ── ADVERSARIAL A7b: isolation utilities resolve only from the closed path ──
# Each hostile wrapper preserves the real utility's behavior after recording a
# marker. Without the function-local closed PATH in bubbles_python_runs, this
# test stays behaviorally green but the marker proves the trust boundary ran an
# ambient executable. That makes the assertion non-vacuous without racing a
# timeout or depending on the host's Python installation.
a7b="$TMP_ROOT/a7b"
a7b_bin="$a7b/bin"
a7b_marker="$a7b/ambient-utility-executed"
mkdir -p "$a7b_bin"
make_hostile_passthrough() {
  local tool="$1"
  local real_tool=""
  real_tool="$(PATH=/usr/bin:/bin:/usr/sbin:/sbin command -v "$tool" 2>/dev/null || true)"
  if [[ -z "$real_tool" ]]; then
    bad "A7b fixture requires $tool in the closed system path"
    return
  fi
  cat >"$a7b_bin/$tool" <<EOF
#!/bin/bash
printf '%s\n' '$tool' >>"\${BUBBLES_PYTHON_HOSTILE_MARKER:?marker required}"
exec '$real_tool' "\$@"
EOF
  chmod +x "$a7b_bin/$tool"
}
for isolation_tool in mktemp sleep wc grep cat rm; do
  make_hostile_passthrough "$isolation_tool"
done
for isolation_tool in timeout gtimeout; do
  cat >"$a7b_bin/$isolation_tool" <<'EOF'
#!/bin/bash
printf '%s\n' 'timeout-runner' >>"${BUBBLES_PYTHON_HOSTILE_MARKER:?marker required}"
printf '%s' 'bubbles-python-runs'
exit 0
EOF
  chmod +x "$a7b_bin/$isolation_tool"
done
a7b_result="$(PATH="$a7b_bin:/usr/bin:/bin:/usr/sbin:/sbin" \
  BUBBLES_PYTHON_HOSTILE_MARKER="$a7b_marker" /bin/bash -c \
  '. "$1"; . "$2"; if bubbles_python_runs "$3"; then printf "RUNS|%s|%s" "$BUBBLES_PYTHON_RUN_STATUS" "$BUBBLES_PYTHON_RUN_DIAGNOSTIC"; else printf "DECLINED|%s|%s" "$BUBBLES_PYTHON_RUN_STATUS" "$BUBBLES_PYTHON_RUN_DIAGNOSTIC"; fi' \
  _ "$GUARD_LIB" "$ENV_SH" "$a7/healthy/python3" 2>/dev/null || true)"
if [[ "$a7b_result" == "RUNS|0|OK" ]]; then
  ok "A7b: closed-path probe still accepts the real healthy interpreter"
else
  bad "A7b: closed-path probe returned '$a7b_result'"
fi
if [[ ! -e "$a7b_marker" ]]; then
  ok "A7b: interpreter isolation executes no ambient PATH utility"
else
  bad "A7b: interpreter isolation executed ambient utility: $(tr '\n' ' ' <"$a7b_marker")"
fi

a7b_mutant_result="$(PATH="$a7b_bin:/usr/bin:/bin:/usr/sbin:/sbin" \
  BUBBLES_PYTHON_ISOLATION_PATH="$a7b_bin:/usr/bin:/bin:/usr/sbin:/sbin" \
  BUBBLES_PYTHON_HOSTILE_MARKER="$a7b_marker" /bin/bash -c \
  '/bin/rm -f "$4"; . "$1"; . "$2"; if bubbles_python_runs "$3"; then result=RUNS; else result=DECLINED; fi; if [[ -e "$4" ]]; then marker=PRESENT; else marker=ABSENT; fi; printf "%s|%s|%s|%s" "$result" "$BUBBLES_PYTHON_RUN_STATUS" "$BUBBLES_PYTHON_RUN_DIAGNOSTIC" "$marker"' \
  _ "$GUARD_LIB" "$ENV_SH" "$a7/healthy/python3" "$a7b_marker" 2>/dev/null || true)"
if [[ "$a7b_mutant_result" == "RUNS|0|OK|ABSENT" ]]; then
  ok "A7b negative control: a legacy isolation-path override cannot inject a trusted helper"
else
  bad "A7b negative control: legacy isolation-path override reached a trusted helper ('$a7b_mutant_result')"
fi
fi

# ── SCN-B039-005: authenticated native runtime and hostile environment ─────
if [[ "$RUN_SECURITY" -eq 1 && "$SECURITY_BOUNDARY_READY" -eq 1 ]]; then
echo "Scenario: SCN-B039-005 resolves an authenticated native runtime independently of caller Python state."
security_status=0
if DEVELOPER_DIR=/Library/Developer/CommandLineTools bubbles_python_resolve_security_runtime; then
  security_status=0
else
  security_status=$?
fi
if [[ "$security_status" -eq 0 && "$BUBBLES_PYTHON_SECURITY_STATUS" -eq 0 &&
  "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == OK &&
  "$BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT" == root-protected-native-python-v1 &&
  "$BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL" == PYSEC1 &&
  "$BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL" == PYMOD1 ]]; then
  ok "SCN-B039-005: validated CLT/xcrun path authenticates PYSEC1 and PYMOD1"
else
  bad "SCN-B039-005: authenticated positive path failed status=$security_status runtimeStatus=$BUBBLES_PYTHON_SECURITY_STATUS diagnostic=$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC rejection=$BUBBLES_PYTHON_SECURITY_REJECTION"
fi
SECURITY_RUNTIME="$BUBBLES_PYTHON_SECURITY_RUNTIME"

hostile_env_root="$TMP_ROOT/security-hostile-env"
hostile_env_marker="$hostile_env_root/marker"
mkdir -p "$hostile_env_root"
cat >"$hostile_env_root/sitecustomize.py" <<EOF
from pathlib import Path
Path("$hostile_env_marker").write_text("executed", encoding="utf-8")
EOF
hostile_resolution="$(
  builtin trap - EXIT
    PYTHONPATH="$hostile_env_root"
    PYTHONHOME="$hostile_env_root"
    PYTHONSTARTUP="$hostile_env_root/sitecustomize.py"
    LD_PRELOAD="$hostile_env_root/missing.so"
    DYLD_INSERT_LIBRARIES="$hostile_env_root/missing.dylib"
    export PYTHONPATH PYTHONHOME PYTHONSTARTUP LD_PRELOAD DYLD_INSERT_LIBRARIES
    if bubbles_python_resolve_security_runtime; then result=RESOLVED; else result=DECLINED; fi
    printf "%s|%s|%s|%s|%s" "$result" "$BUBBLES_PYTHON_SECURITY_STATUS" \
      "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" "$BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL" \
      "$BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL"
)" 2>/dev/null || true
if [[ "$hostile_resolution" == "RESOLVED|0|OK|PYSEC1|PYMOD1" && ! -e "$hostile_env_marker" ]]; then
  ok "SCN-B039-005: PYTHON and loader environment cannot enter the isolated runtime"
else
  bad "SCN-B039-005: hostile environment result='$hostile_resolution' marker=$([[ -e "$hostile_env_marker" ]] && echo present || echo absent)"
fi

# ── SCN-B039-005 path predicates: ownership, modes, links, and native bytes ──
echo "Scenario: SCN-B039-005 path authentication rejects caller authority and unsafe link topology."
path_fixture="$TMP_ROOT/security-paths"
mkdir -p "$path_fixture/bin"
cat >"$path_fixture/bin/python3" <<'EOF'
#!/bin/bash
printf '%s\n' 'RUNTIME PYSEC1 forged'
EOF
chmod +x "$path_fixture/bin/python3"
if _bubbles_python_security_authenticate_path "$path_fixture/bin/python3" executable 1; then
  bad "SCN-B039-005: caller-owned text executable was authenticated"
else
  case "$BUBBLES_PYTHON_SECURITY_PATH_REJECTION" in
    ANCESTOR_OWNER | ANCESTOR_MODE_WRITABLE | ANCESTOR_CALLER_WRITABLE | TARGET_OWNER | TARGET_CALLER_WRITABLE | TARGET_FORMAT)
      ok "SCN-B039-005: caller-owned path is rejected before execution ($BUBBLES_PYTHON_SECURITY_PATH_REJECTION)"
      ;;
    *) bad "SCN-B039-005: caller-owned path rejection was '$BUBBLES_PYTHON_SECURITY_PATH_REJECTION'" ;;
  esac
fi
if ! _bubbles_python_security_mode_is_writable 755 && _bubbles_python_security_mode_is_writable 775 &&
  _bubbles_python_security_mode_is_writable 757 && _bubbles_python_security_mode_is_writable 777; then
  ok "SCN-B039-005: group/other writable ancestor and target modes fail closed"
else
  bad "SCN-B039-005: writable-mode predicate accepted an unsafe mode"
fi
if _bubbles_python_security_native_format "$path_fixture/bin/python3"; then
  bad "SCN-B039-005: text wrapper passed native ELF/Mach-O identity"
else
  ok "SCN-B039-005: text wrapper is rejected by native-format identity"
fi

ln -s target "$path_fixture/cycle-a"
ln -s cycle-a "$path_fixture/target"
depth_target="$path_fixture/depth-final"
printf '%s\n' x >"$depth_target"
depth_index=34
while [[ "$depth_index" -ge 1 ]]; do
  if [[ "$depth_index" -eq 34 ]]; then depth_next='depth-final'; else depth_next="depth-$((depth_index + 1))"; fi
  ln -s "$depth_next" "$path_fixture/depth-$depth_index"
  depth_index=$((depth_index - 1))
done
link_escape="$path_fixture/link-escape"
ln -s "$path_fixture/bin/python3" "$link_escape"

synthetic_link_rejection() {
  local requested="$1"
  local expected="$2"
  local observed=""
  observed="$(
    builtin trap - EXIT
    original_definition="$(declare -f _bubbles_python_security_stat)"
    original_definition="${original_definition/_bubbles_python_security_stat/_bubbles_python_security_stat_real}"
    eval "$original_definition"
    _bubbles_python_security_validate_directory() { return 0; }
    _bubbles_python_security_stat() {
      _bubbles_python_security_stat_real "$@" || return $?
      if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" == symlink ]]; then
        BUBBLES_PYTHON_SECURITY_META_OWNER=0
      fi
    }
    if _bubbles_python_security_authenticate_path "$requested" file 0; then
      printf RESOLVED
    else
      printf "%s" "$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
    fi
)" 2>/dev/null || true
  if [[ "$observed" == "$expected" ]]; then
    ok "SCN-B039-005: $expected is detected by the production link walker"
  else
    bad "SCN-B039-005: expected $expected, observed '${observed:-empty}'"
  fi
}
synthetic_link_rejection "$path_fixture/cycle-a" SYMLINK_CYCLE
synthetic_link_rejection "$path_fixture/depth-1" SYMLINK_DEPTH
synthetic_link_rejection "$link_escape" TARGET_OWNER

synthetic_final_rejection() {
  local mode="$1"
  local requested="$2"
  local expected="$3"
  local observed=""
  observed="$(
    builtin trap - EXIT
    expected_target="$requested"
    original_definition="$(declare -f _bubbles_python_security_stat)"
    original_definition="${original_definition/_bubbles_python_security_stat/_bubbles_python_security_stat_real}"
    eval "$original_definition"
    _bubbles_python_security_validate_directory() { return 0; }
    _bubbles_python_security_stat() {
      _bubbles_python_security_stat_real "$@" || return $?
      if [[ "$1" == "$expected_target" ]]; then
        # Stock Bash 3.2 misparses nested case patterns in command substitutions,
        # so keep these equivalent predicates free of case syntax.
        if [[ "$mode" == root-metadata-writable-mode ]]; then
          # shellcheck disable=SC2034
          BUBBLES_PYTHON_SECURITY_META_OWNER=0
          BUBBLES_PYTHON_SECURITY_META_MODE=777
          BUBBLES_PYTHON_SECURITY_META_TYPE='file'
        elif [[ "$mode" == root-metadata-caller-writable ]]; then
          # shellcheck disable=SC2034
          BUBBLES_PYTHON_SECURITY_META_OWNER=0
          # shellcheck disable=SC2034
          BUBBLES_PYTHON_SECURITY_META_MODE=755
          BUBBLES_PYTHON_SECURITY_META_TYPE='file'
        fi
      fi
    }
    if _bubbles_python_security_authenticate_path "$requested" file 0; then
      printf RESOLVED
    else
      printf "%s" "$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
    fi
)" 2>/dev/null || true
  if [[ "$observed" == "$expected" ]]; then
    ok "SCN-B039-005: $expected rejects $mode through the production path walker"
  else
    bad "SCN-B039-005: $mode expected $expected, observed '${observed:-empty}'"
  fi
}
synthetic_final_rejection caller-owned-target "$path_fixture/bin/python3" TARGET_OWNER
synthetic_final_rejection root-metadata-writable-mode "$path_fixture/bin/python3" TARGET_MODE_WRITABLE
synthetic_final_rejection root-metadata-caller-writable "$path_fixture/bin/python3" TARGET_CALLER_WRITABLE
synthetic_final_rejection caller-owned-symlink "$link_escape" SYMLINK_OWNER

# ── SCN-B039-005 protocol corruption and untrusted closure mutations ───────
echo "Scenario: SCN-B039-005 malformed PYSEC1/PYMOD1 and untrusted origins fail closed."
runtime_capture="$TMP_ROOT/runtime-probe.capture"
module_capture="$TMP_ROOT/module-probe.capture"
if bubbles_python_run_security_operation runtime-probe; then
  /bin/cp "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" "$runtime_capture"
  bubbles_python_security_cleanup || true
else
  bad "SCN-B039-005: authenticated runtime-probe operation failed"
  bubbles_python_security_cleanup || true
fi
if bubbles_python_run_security_operation module-probe; then
  /bin/cp "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" "$module_capture"
  bubbles_python_security_cleanup || true
else
  bad "SCN-B039-005: authenticated module-probe operation failed"
  bubbles_python_security_cleanup || true
fi

malformed_runtime="$TMP_ROOT/runtime-malformed.capture"
printf 'RUNTIME\tPYSEC1\t3\n' >"$malformed_runtime"
if _bubbles_python_security_validate_runtime_protocol "$malformed_runtime" "$SECURITY_RUNTIME"; then
  bad "SCN-B039-005: malformed PYSEC1 was accepted"
else
  ok "SCN-B039-005: malformed PYSEC1 is rejected"
fi
unsupported_runtime="$TMP_ROOT/runtime-unsupported.capture"
/usr/bin/awk 'NR == 1 { print "RUNTIME\tPYSEC1\t3\t8"; next } { print }' \
  "$runtime_capture" >"$unsupported_runtime"
BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NOT_RUN'
if _bubbles_python_security_validate_runtime_protocol "$unsupported_runtime" "$SECURITY_RUNTIME"; then
  bad "SCN-B039-005: unsupported Python 3.8 protocol was accepted"
elif [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == PYTHON_VERSION_UNSUPPORTED ]]; then
  ok "SCN-B039-005: unsupported Python version receives its closed diagnostic"
else
  bad "SCN-B039-005: unsupported-version diagnostic was '$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC'"
fi
untrusted_search_root="$TMP_ROOT/untrusted-search-root"
mkdir -p "$untrusted_search_root"
untrusted_runtime="$TMP_ROOT/runtime-untrusted-root.capture"
/usr/bin/awk -v bad="$untrusted_search_root" '
  BEGIN { changed=0 }
  /^PATH\tPYSEC1\t/ && changed == 0 { print "PATH\tPYSEC1\t" bad; changed=1; next }
  { print }
' "$runtime_capture" >"$untrusted_runtime"
BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NOT_RUN'
if _bubbles_python_security_validate_runtime_protocol "$untrusted_runtime" "$SECURITY_RUNTIME"; then
  bad "SCN-B039-005: caller-owned PYSEC1 search root was accepted"
elif [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == RUNTIME_CLOSURE_UNTRUSTED ]]; then
  ok "SCN-B039-005: caller-owned PYSEC1 search root fails closure authentication"
else
  bad "SCN-B039-005: untrusted PYSEC1 root diagnostic was '$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC'"
fi
malformed_module="$TMP_ROOT/module-malformed.capture"
printf 'COMPLETE\tPYMOD1\t8\n' >"$malformed_module"
if _bubbles_python_security_validate_module_protocol "$malformed_module"; then
  bad "SCN-B039-005: malformed PYMOD1 was accepted"
else
  ok "SCN-B039-005: malformed PYMOD1 is rejected"
fi
module_record_count="$(/usr/bin/grep -c $'^MODULE\tPYMOD1\t' "$module_capture" || true)"
module_declared_count="$(/usr/bin/awk -F '\t' '$1 == "COMPLETE" && $2 == "PYMOD1" { print $3 }' "$module_capture")"
if [[ "$module_record_count" =~ ^[0-9]+$ && "$module_declared_count" == "$module_record_count" &&
  "$module_record_count" -gt 9 ]]; then
  ok "SCN-B039-005: PYMOD1 authenticates the complete loaded-module closure ($module_record_count records)"
else
  bad "SCN-B039-005: PYMOD1 closure count records=$module_record_count declared=${module_declared_count:-missing}"
fi
untrusted_module="$TMP_ROOT/module-untrusted-origin.capture"
printf '%s\n' 'untrusted module fixture' >"$untrusted_search_root/module.py"
/usr/bin/awk -v bad="$untrusted_search_root/module.py" '
  BEGIN { changed=0 }
  /^MODULE\tPYMOD1\t/ && $4 == "file" && changed == 0 { $5=bad; OFS="\t"; print $1,$2,$3,$4,$5; changed=1; next }
  { print }
' "$module_capture" >"$untrusted_module"
BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NOT_RUN'
if _bubbles_python_security_validate_module_protocol "$untrusted_module"; then
  bad "SCN-B039-005: caller-owned PYMOD1 origin was accepted"
elif [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == MODULE_CLOSURE_UNTRUSTED ]]; then
  ok "SCN-B039-005: caller-owned PYMOD1 origin fails closure authentication"
else
  bad "SCN-B039-005: untrusted PYMOD1 origin diagnostic was '$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC'"
fi
BUBBLES_PYTHON_SECURITY_RUNTIME="$SECURITY_RUNTIME"
BUBBLES_PYTHON_SECURITY_STATUS=0
BUBBLES_PYTHON_SECURITY_DIAGNOSTIC=OK
BUBBLES_PYTHON_SECURITY_REJECTION=NONE
# Fixture input is consumed by functions sourced from python-env.sh.
# shellcheck disable=SC2034
BUBBLES_PYTHON_SECURITY_PROVENANCE='root-protected-path'
BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL=PYSEC1
BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL=PYMOD1

# ── SCN-B039-007/008 native-supervisor status and cleanup assertions ───────

success_shadow_result="$(
  builtin trap - EXIT
  BUBBLES_PYTHON_SECURITY_RUNTIME="$SECURITY_RUNTIME"
  BUBBLES_PYTHON_SECURITY_STATUS=0
  BUBBLES_PYTHON_SECURITY_DIAGNOSTIC=OK
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_PROVENANCE='root-protected-path'
  BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL=PYSEC1
  BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL=PYMOD1
  shadow_kill=0; shadow_wait=0
  kill() { shadow_kill=$((shadow_kill + 1)); return 99; }
  wait() { shadow_wait=$((shadow_wait + 1)); return 99; }
  trap 'return 91' HUP
  trap 'return 92' INT
  trap 'return 93' TERM
  before_hup="$(trap -p HUP)"; before_int="$(trap -p INT)"; before_term="$(trap -p TERM)"
  operation_status=0
  cleanup_status=0
  bubbles_python_run_security_operation runtime-probe || operation_status=$?
  root="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT"
  bubbles_python_security_cleanup || cleanup_status=$?
  after_hup="$(trap -p HUP)"; after_int="$(trap -p INT)"; after_term="$(trap -p TERM)"
  if [[ "$before_hup" == "$after_hup" && "$before_int" == "$after_int" && "$before_term" == "$after_term" ]]; then traps=restored; else traps=changed; fi
  if [[ "$cleanup_status" -eq 0 && -n "$root" && ! -e "$root" ]]; then removed=yes; else removed=no; fi
  printf "%s|%s|%s|%s|%s|%s" "$operation_status" "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" "$shadow_kill" "$shadow_wait" "$traps" "$removed"
)" 2>/dev/null || true
if [[ "$success_shadow_result" == "0|OK|0|0|restored|yes" ]]; then
  ok "SCN-B039-007/008: success uses builtin kill/wait, restores traps, and removes captures"
else
  bad "SCN-B039-007/008: success/shadow result '$success_shadow_result'"
fi

# EXIT cleanup: intentionally leave a successful operation registered, then
# exit the sourcing shell. Its restored outer process verifies the private root.
exit_root_file="$TMP_ROOT/exit-cleanup-root"
exit_case_status=0
(
  BUBBLES_PYTHON_SECURITY_RUNTIME="$SECURITY_RUNTIME"
  BUBBLES_PYTHON_SECURITY_STATUS=0
  BUBBLES_PYTHON_SECURITY_DIAGNOSTIC=OK
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_PROVENANCE='root-protected-path'
  BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL=PYSEC1
  BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL=PYMOD1
  bubbles_python_run_security_operation runtime-probe
  printf "%s\n" "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" >"$exit_root_file"
  exit 77
) 2>/dev/null || exit_case_status=$?
exit_private_root="$(/bin/cat "$exit_root_file" 2>/dev/null || true)"
if [[ "$exit_case_status" -eq 77 && -n "$exit_private_root" && ! -e "$exit_private_root" ]]; then
  ok "SCN-B039-008: EXIT cleanup removes an intentionally unconsumed operation root"
else
  bad "SCN-B039-008: EXIT cleanup status=$exit_case_status root='${exit_private_root:-missing}'"
fi

# The repeated native-supervisor matrix has no retries. Each named iteration
# executes once against the same authenticated runtime.
scope2_private_root_count() {
  local candidate=""
  local count=0
  for candidate in /tmp/bubbles-python-security.*; do
    [[ -e "$candidate" ]] || continue
    count=$((count + 1))
  done
  printf '%s\n' "$count"
}

scope2_control_line_count() {
  local control_path="${1:-}"
  local line=""
  local count=0
  [[ -f "$control_path" ]] || {
    printf '%s\n' 0
    return 0
  }
  while IFS= read -r line || [[ -n "$line" ]]; do
    count=$((count + 1))
  done <"$control_path"
  printf '%s\n' "$count"
}

matrix_iteration=1
matrix_failures=0
matrix_attempts=0
while [[ "$matrix_iteration" -le 30 ]]; do
  matrix_attempts=$((matrix_attempts + 1))
  matrix_private_before="$(scope2_private_root_count)"
  matrix_status=0
  bubbles_python_run_security_operation runtime-probe || matrix_status=$?
  matrix_root="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT"
  matrix_diagnostic="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC"
  matrix_owner="$BUBBLES_PYTHON_SECURITY_RUN_OWNER"
  matrix_timed_out="$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT"
  matrix_worker_kind="$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND"
  matrix_stdout_bytes="$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES"
  matrix_stderr_bytes="$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES"
  matrix_stdout_actual="$(/usr/bin/wc -c <"$BUBBLES_PYTHON_SECURITY_STDOUT_PATH")"
  matrix_stderr_actual="$(/usr/bin/wc -c <"$BUBBLES_PYTHON_SECURITY_STDERR_PATH")"
  matrix_stdout_actual="${matrix_stdout_actual//[[:space:]]/}"
  matrix_stderr_actual="${matrix_stderr_actual//[[:space:]]/}"
  matrix_control_lines="$(scope2_control_line_count "$BUBBLES_PYTHON_SECURITY_CONTROL_PATH")"
  matrix_ordering=0
  scope2_native_contract_structurally_valid "$ENV_SH" || matrix_ordering=$?
  matrix_cleanup=0
  bubbles_python_security_cleanup || matrix_cleanup=$?
  matrix_private_after="$(scope2_private_root_count)"
  if [[ "$matrix_status" -eq 0 && "$matrix_diagnostic" == OK &&
    "$matrix_owner" == worker && "$matrix_timed_out" -eq 0 && "$matrix_worker_kind" == exit &&
    "$matrix_stdout_bytes" -gt 0 && "$matrix_stdout_bytes" -eq "$matrix_stdout_actual" &&
    "$matrix_stderr_bytes" -eq "$matrix_stderr_actual" && "$matrix_control_lines" -eq 1 &&
    "$matrix_ordering" -eq 0 && "$matrix_cleanup" -eq 0 &&
    -z "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" &&
    "$BUBBLES_PYTHON_SECURITY_STATE" == IDLE && -n "$matrix_root" && ! -e "$matrix_root" &&
    "$matrix_private_before" -eq "$matrix_private_after" ]]; then
    printf 'TP-S2-05_MATRIX class=success iteration=%s/30 status=%s owner=%s timedOut=%s workerKind=%s stdoutBytes=%s stderrBytes=%s completionRecords=1 ordering=waitpid-owner-clear-bps1 oneReap=1 postReapSignals=0 residue=0 retry=0\n' \
      "$matrix_iteration" "$matrix_status" "$matrix_owner" "$matrix_timed_out" \
      "$matrix_worker_kind" "$matrix_stdout_bytes" "$matrix_stderr_bytes"
    ok "SCN-B039-007 stress iteration $matrix_iteration/30 native-supervisor success"
  else
    bad "SCN-B039-007 stress iteration $matrix_iteration/30 status=$matrix_status diagnostic=$matrix_diagnostic owner=$matrix_owner timedOut=$matrix_timed_out workerKind=$matrix_worker_kind stdout=$matrix_stdout_bytes/$matrix_stdout_actual stderr=$matrix_stderr_bytes/$matrix_stderr_actual completionRecords=$matrix_control_lines ordering=$matrix_ordering cleanup=$matrix_cleanup roots=$matrix_private_before/$matrix_private_after"
    matrix_failures=$((matrix_failures + 1))
  fi
  matrix_iteration=$((matrix_iteration + 1))
done
if [[ "$RUN_STRESS" -eq 1 && "$matrix_attempts" -eq 30 ]]; then
  SCOPE2_STRESS_ATTEMPT_COUNT=$matrix_attempts
  SCOPE2_STRESS_CLASS_COUNT=1
fi
if [[ "$matrix_failures" -eq 0 ]]; then
  ok "TP-S2-05 success matrix completed 30/30 with exact BPS1 bytes, one reap, zero post-reap signals, no residue, and no retry substitution"
else
  bad "TP-S2-05 success matrix had $matrix_failures failed iteration(s)"
fi

scope2_write_lifecycle_worker() {
  local lifecycle_class="$1"
  local destination="$2"
  case "$lifecycle_class" in
    timeout)
      cat >"$destination" <<'EOF'
#!/bin/bash
trap '' TERM
exec /bin/sleep 300
EOF
      ;;
    output-limit)
      cat >"$destination" <<'EOF'
#!/bin/bash
exec /usr/bin/yes x
EOF
      ;;
    signal-hup | signal-int | signal-term)
      cat >"$destination" <<'EOF'
#!/bin/bash
printf '%s' 'bubbles-python-runs'
exec /bin/sleep 3
EOF
      ;;
    *) return 2 ;;
  esac
  /bin/chmod 700 "$destination"
}

scope2_run_lifecycle_iteration() {
  local lifecycle_class="$1"
  local iteration="$2"
  local worker_path="$TMP_ROOT/tp-s2-05-$lifecycle_class.worker"
  local worker_program=""
  local expected_status=0
  local expected_diagnostic=""
  local expected_owner=""
  local expected_timed_out=0
  local expected_worker_kind=""
  local expected_stdout_bytes=0
  local signal_name=""
  local sender_status=0
  local run_status=0
  local synchronization_status=0
  local signal_sync_valid=1
  local private_before=0
  local private_after=0
  local root=""
  local control_path=""
  local control_lines=0
  local stdout_actual=0
  local stderr_actual=0
  local ordering_status=0
  local cleanup_status=0
  local root_removed=0

  case "$lifecycle_class" in
    fast-exit)
      worker_program=/usr/bin/false
      expected_status=1
      expected_diagnostic=WORKER_EXIT_NONZERO
      expected_owner=worker
      expected_timed_out=0
      expected_worker_kind='exit'
      expected_stdout_bytes=0
      ;;
    timeout)
      scope2_write_lifecycle_worker "$lifecycle_class" "$worker_path" || return 2
      worker_program="$worker_path"
      expected_status=124
      expected_diagnostic=SUPERVISOR_TIMEOUT
      expected_owner=supervisor
      expected_timed_out=1
      expected_worker_kind=signal
      expected_stdout_bytes=0
      ;;
    output-limit)
      scope2_write_lifecycle_worker "$lifecycle_class" "$worker_path" || return 2
      worker_program="$worker_path"
      expected_status=125
      expected_diagnostic=OUTPUT_LIMIT
      expected_owner=supervisor
      expected_timed_out=0
      expected_worker_kind=signal
      expected_stdout_bytes=16384
      ;;
    signal-hup | signal-int | signal-term)
      scope2_write_lifecycle_worker "$lifecycle_class" "$worker_path" || return 2
      worker_program="$worker_path"
      expected_owner=caller-signal
      expected_timed_out=0
      expected_worker_kind='exit'
      expected_stdout_bytes=19
      case "$lifecycle_class" in
        signal-hup) signal_name=HUP; expected_status=129; expected_diagnostic=SIGNAL_HUP ;;
        signal-int) signal_name=INT; expected_status=130; expected_diagnostic=SIGNAL_INT ;;
        signal-term) signal_name=TERM; expected_status=143; expected_diagnostic=SIGNAL_TERM ;;
      esac
      ;;
    *) return 2 ;;
  esac

  private_before="$(scope2_private_root_count)"
  if [[ -n "$signal_name" ]]; then
    if scope2_run_synchronized_parent_signal \
      "$signal_name" "$worker_program" "$lifecycle_class-$iteration"; then
      synchronization_status=0
    else
      synchronization_status=$?
    fi
    run_status="$SCOPE2_SIGNAL_RUN_STATUS"
    sender_status="$SCOPE2_SIGNAL_SENDER_STATUS"
    if [[ "$synchronization_status" -ne 0 ||
      "$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT" -ne 1 ||
      "$SCOPE2_SIGNAL_BOUNDARY_INVALID" -ne 0 ||
      "$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY" -ne 1 ||
      "$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY" -ne 1 ]]; then
      signal_sync_valid=0
    fi
  else
    _bubbles_python_run_closed_operation general-probe "$worker_program" || run_status=$?
  fi

  root="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT"
  control_path="$BUBBLES_PYTHON_SECURITY_CONTROL_PATH"
  if [[ -n "$control_path" ]]; then
    control_lines="$(scope2_control_line_count "$control_path")"
    stdout_actual="$(/usr/bin/wc -c <"$BUBBLES_PYTHON_SECURITY_STDOUT_PATH")"
    stderr_actual="$(/usr/bin/wc -c <"$BUBBLES_PYTHON_SECURITY_STDERR_PATH")"
    stdout_actual="${stdout_actual//[[:space:]]/}"
    stderr_actual="${stderr_actual//[[:space:]]/}"
  else
    control_lines=1
    stdout_actual="$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES"
    stderr_actual="$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES"
  fi
  scope2_native_contract_structurally_valid "$ENV_SH" || ordering_status=$?
  bubbles_python_security_cleanup || cleanup_status=$?
  private_after="$(scope2_private_root_count)"
  if [[ -z "$root" || ! -e "$root" ]]; then
    root_removed=1
  fi

  if [[ "$run_status" -eq "$expected_status" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" == "$expected_diagnostic" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" == "$expected_owner" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT" -eq "$expected_timed_out" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" == "$expected_worker_kind" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" -eq "$expected_stdout_bytes" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" -eq "$stdout_actual" &&
    "$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES" -eq "$stderr_actual" &&
    "$control_lines" -eq 1 && "$ordering_status" -eq 0 && "$cleanup_status" -eq 0 &&
    "$sender_status" -eq 0 && "$signal_sync_valid" -eq 1 &&
    -z "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" &&
    "$BUBBLES_PYTHON_SECURITY_STATE" == IDLE && "$private_before" -eq "$private_after" &&
    "$root_removed" -eq 1 ]]; then
    printf 'TP-S2-05_MATRIX class=%s iteration=%s/30 status=%s owner=%s timedOut=%s workerKind=%s stdoutBytes=%s stderrBytes=%s completionRecords=1 ordering=waitpid-owner-clear-bps1 oneReap=1 postReapSignals=0 residue=0 retry=0\n' \
      "$lifecycle_class" "$iteration" "$run_status" \
      "$BUBBLES_PYTHON_SECURITY_RUN_OWNER" "$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT" \
      "$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND" \
      "$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES" "$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES"
    ok "TP-S2-05 $lifecycle_class iteration $iteration/30 preserves the complete native-supervisor lifecycle contract"
    return 0
  fi

  bad "TP-S2-05 $lifecycle_class iteration $iteration/30 observed status=$run_status diagnostic=$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC owner=$BUBBLES_PYTHON_SECURITY_RUN_OWNER timedOut=$BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT workerKind=$BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND stdout=$BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES/$stdout_actual stderr=$BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES/$stderr_actual completionRecords=$control_lines ordering=$ordering_status cleanup=$cleanup_status sender=$sender_status synchronization=$synchronization_status boundaryNotifications=$SCOPE2_SIGNAL_BOUNDARY_NOTIFY_COUNT trapReady=$SCOPE2_SIGNAL_BOUNDARY_TRAP_READY waitHandleReady=$SCOPE2_SIGNAL_BOUNDARY_WAIT_READY boundaryInvalid=$SCOPE2_SIGNAL_BOUNDARY_INVALID roots=$private_before/$private_after"
  return 1
}

if [[ "$RUN_STRESS" -eq 1 ]]; then
  scope2_assert_signal_sender_requires_readiness
  printf '%s\n' 'TP-S2-05_STRESS_MATRIX_BEGIN classes=success,fast-exit,timeout,output-limit,HUP,INT,TERM iterations=30 retry=forbidden'
  for lifecycle_class in fast-exit timeout output-limit signal-hup signal-int signal-term; do
    lifecycle_iteration=1
    lifecycle_failures=0
    lifecycle_attempts=0
    while [[ "$lifecycle_iteration" -le 30 ]]; do
      lifecycle_attempts=$((lifecycle_attempts + 1))
      if scope2_run_lifecycle_iteration "$lifecycle_class" "$lifecycle_iteration"; then
        :
      else
        lifecycle_failures=$((lifecycle_failures + 1))
      fi
      lifecycle_iteration=$((lifecycle_iteration + 1))
    done
    SCOPE2_STRESS_ATTEMPT_COUNT=$((SCOPE2_STRESS_ATTEMPT_COUNT + lifecycle_attempts))
    if [[ "$lifecycle_attempts" -eq 30 ]]; then
      SCOPE2_STRESS_CLASS_COUNT=$((SCOPE2_STRESS_CLASS_COUNT + 1))
    else
      bad "TP-S2-05 $lifecycle_class matrix attempted $lifecycle_attempts/30 iterations"
    fi
    if [[ "$lifecycle_failures" -eq 0 ]]; then
      ok "TP-S2-05 $lifecycle_class matrix completed 30/30 with fixed production limits and no retry substitution"
    else
      bad "TP-S2-05 $lifecycle_class matrix had $lifecycle_failures failed iteration(s)"
    fi
  done
  printf '%s\n' 'TP-S2-05_STRESS_MATRIX_END attemptedPerClass=30 retries=0'
  if [[ "$SCOPE2_STRESS_ATTEMPT_COUNT" -eq 210 &&
    "$SCOPE2_STRESS_CLASS_COUNT" -eq 7 ]]; then
    printf 'TP-S2-05_STRESS_COMPLETENESS=PASS classes=%s/7 iterations=%s/210 endMarker=1 retries=0\n' \
      "$SCOPE2_STRESS_CLASS_COUNT" "$SCOPE2_STRESS_ATTEMPT_COUNT"
    SCOPE2_STRESS_COMPLETED=1
  else
    bad "TP-S2-05 stress matrix incomplete classes=$SCOPE2_STRESS_CLASS_COUNT/7 iterations=$SCOPE2_STRESS_ATTEMPT_COUNT/210"
  fi
fi

# Structural assertions are scoped to the production security module. Strings
# in this selftest's labeled negative controls do not authorize production use.
if /usr/bin/grep -Eq '^[[:space:]]*set -m|builtin kill[[:space:]].*"-\$|kill -0|BUBBLES_PYTHON_.*DESCENDANT|bubbles_python_run_bounded|bubbles_python_resolve_trusted_runnable' "$ENV_SH"; then
  bad "SCN-B039-007: forbidden process-group, polling, descendant, or removed API remains in production"
else
  ok "SCN-B039-007: production security module contains no forbidden supervision mechanism"
fi
scope2_vector_status=0
bubbles_python_run_security_operation caller-program /bin/true 1 1 >/dev/null 2>&1 || scope2_vector_status=$?
scope2_extra_status=0
bubbles_python_run_security_operation runtime-probe /bin/true 1 1 >/dev/null 2>&1 || scope2_extra_status=$?
if [[ "$scope2_vector_status" -eq 2 && "$scope2_extra_status" -eq 2 &&
  "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" == ARGUMENT_INVALID ]]; then
  ok "TP-S2-01 SEC-R2: callers cannot supply a program, helper, wall, grace, or output-limit vector"
else
  bad "TP-S2-01 SEC-R2: closed operation API accepted a caller authority vector (program=$scope2_vector_status extra=$scope2_extra_status diagnostic=$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC)"
fi
bubbles_python_security_cleanup || true
if scope2_native_contract_structurally_valid "$ENV_SH"; then
  ok "SCN-B039-007/008: BPS1 completion follows waitpid, owner clear, and supervisor wait-handle clear"
else
  bad "SCN-B039-007/008: native supervisor lifecycle structure is incomplete"
fi
fi

run_scope2_privileged_security_suite() {
  local hostile_bash_env="$TMP_ROOT/privileged-hostile-bash-env.sh"
  local hostile_marker="$TMP_ROOT/privileged-hostile.marker"
  local output_file="$TMP_ROOT/privileged-security.output"
  local child_status=0
  local setup_ready=0

  printf 'printf %%s\\n BASH_ENV_EXECUTED >>%q\n' "$hostile_marker" >"$hostile_bash_env"
  if (
    source() { return 73; }
    kill() { return 73; }
    wait() { return 73; }
    export -f source kill wait
    BASH_ENV="$hostile_bash_env"
    export BASH_ENV
    /usr/bin/env -i \
      LC_ALL=C \
      PATH=/usr/bin:/bin \
      DEVELOPER_DIR=/Library/Developer/CommandLineTools \
      /bin/bash -p "$SELFTEST_SCRIPT" --internal-privileged-security
  ) >"$output_file" 2>&1 </dev/null; then
    child_status=0
  else
    child_status=$?
  fi
  /bin/cat "$output_file"

  if /usr/bin/grep -Fq 'TP-S2-01_PRIVILEGED_CHILD_SETUP=PASS mode=env-i-/bin/bash-p hostileState=excluded' "$output_file" &&
    ! /usr/bin/grep -Fq 'TP-S2-01_PRIVILEGED_CHILD_SETUP=FAIL' "$output_file" &&
    [[ ! -e "$hostile_marker" ]]; then
    setup_ready=1
    ok "TP-S2-01 SEC-R1 SETUP: env -i /bin/bash -p excludes BASH_ENV and exported functions"
  else
    bad "TP-S2-01 SEC-R1 SETUP: privileged child did not prove hostile-startup exclusion"
  fi

  if [[ "$child_status" -eq 0 && "$setup_ready" -eq 1 ]]; then
    ok "TP-S2-01 privileged security suite satisfies every current native-supervisor assertion"
  elif [[ "$child_status" -eq 1 && "$setup_ready" -eq 1 ]] &&
    /usr/bin/grep -Fq 'RED: TP-S2-01 SEC-R1' "$output_file" &&
    /usr/bin/grep -Fq 'RED: TP-S2-01 SEC-R2' "$output_file" &&
    /usr/bin/grep -Fq 'RED: TP-S2-01 HAR-R1' "$output_file" &&
    /usr/bin/grep -Fq 'RED: TP-S2-01 HAR-R2' "$output_file" &&
    ! /usr/bin/grep -Fq ' SETUP:' "$output_file" &&
    ! /usr/bin/grep -Fq ' TEST-CONTRACT:' "$output_file"; then
    printf 'RED: TP-S2-01 privilegedSecurityExit=%s setup=PASS contradictionFailures=0\n' "$child_status"
    bad "TP-S2-01 privileged security suite remains RED only for missing production invariants"
  else
    bad "TP-S2-01 TEST-CONTRACT: privileged security suite exit=$child_status setup=$setup_ready"
  fi
}

# ── Case 13: the absent-locator reason names the cause, not the interpreters ──
if [[ "$RUN_GENERAL" -eq 1 ]]; then
# The failure text is the deliverable here. "no interpreter satisfies" sends a
# reader to inspect interpreters; the honest text says none could be named.
c13_empty="$TMP_ROOT/c13/emptypath"
mkdir -p "$c13_empty"
c13_reason="$(env -i PATH="$c13_empty" /bin/bash -c \
  '. "$1"; if bubbles_python_resolve_runnable >/dev/null 2>&1; then printf "UNEXPECTED_RESOLVE"; else printf "%s" "$BUBBLES_PYTHON_RUNNABLE_REASON"; fi' \
  _ "$ENV_SH" 2>/dev/null || true)"
if [[ "$c13_reason" == *"the managed venv has no locator"* &&
  "$c13_reason" == *"$LOCATOR_VARS_EXPECTED"* ]]; then
  ok "Case 13: absent-locator reason names the locator variables"
else
  bad "Case 13: reason was '$c13_reason', expected it to name 'the managed venv has no locator' and '$LOCATOR_VARS_EXPECTED'"
fi
# Teeth: the reason must not describe a venv it never located. Under the
# historical defect this branch reported "the managed venv (/bin/python3) is
# absent or does not execute", which is a claim about a path nothing chose.
if [[ "$c13_reason" == *"the managed venv ("* ]]; then
  bad "Case 13b: reason names a concrete venv path while no locator is set: '$c13_reason'"
else
  ok "Case 13b: reason claims no venv path when none could be named"
fi
fi

# ---------------------------------------------------------------------------
# BUG-039 Scope 2 TDD RED contract.
#
# These controls target the authoritative privileged-native-supervision-v2
# epoch while retaining root-protected-native-python-v1 for the worker. They
# intentionally fail against clean successor 72bbb987 until production adds
# privileged entry and native waitpid supervision. Fixture-construction errors
# use a distinct SETUP label and never masquerade as a contract RED.
# ---------------------------------------------------------------------------

scope2_fixed_modern_bash() {
  local candidate=""
  local major=""
  for candidate in /opt/homebrew/bin/bash /opt/local/bin/bash /usr/local/bin/bash /usr/bin/bash /bin/bash; do
    [[ -x "$candidate" ]] || continue
    major="$("$candidate" -c 'printf "%s" "${BASH_VERSINFO[0]:-0}"' 2>/dev/null || true)"
    if [[ "$major" =~ ^[0-9]+$ && "$major" -ge 4 ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

scope2_prepare_entry_fixture() {
  local fixture_root="$1"
  local framework_root=""
  local relative_path=""
  framework_root="$(cd "$SCRIPT_DIR/.." && pwd -P)"
  mkdir -p "$fixture_root/specs/001-scope2-entry" "$fixture_root/src"
  /bin/cp -Rp "$framework_root" "$fixture_root/bubbles" || return 2
  for relative_path in \
    scripts/cli.sh \
    scripts/implementation-reality-scan.sh \
    scripts/guard-lib.sh \
    scripts/python-env.sh \
    scripts/fun-mode.sh \
    scripts/aliases.sh; do
    if [[ ! -f "$fixture_root/bubbles/$relative_path" ]] ||
      ! /usr/bin/cmp -s "$framework_root/$relative_path" "$fixture_root/bubbles/$relative_path" ||
      { [[ -x "$framework_root/$relative_path" ]] && [[ ! -x "$fixture_root/bubbles/$relative_path" ]]; }; then
      printf 'TP-S2-01_ENTRY_FIXTURE_SETUP_FAILED path=%s\n' "$relative_path" >&2
      return 2
    fi
  done
  cat >"$fixture_root/specs/001-scope2-entry/scopes.md" <<'EOF'
# Scope 1: BUG-039 privileged entry fixture

### Implementation Files

- `src/scope2-entry.js`
EOF
  cat >"$fixture_root/src/scope2-entry.js" <<'EOF'
export function scope2EntryLabel(value) {
  return String(value);
}
EOF
  return 0
}

scope2_write_hostile_startup() {
  local destination="$1"
  cat >"$destination" <<'EOF'
source() {
  case "${0:-}:${1:-}" in
    */implementation-reality-scan.sh:*/guard-lib.sh)
      printf '%s\n' SCANNER_GUARD_SOURCE_INTERCEPTED >>"${BUBBLES_SCOPE2_ENTRY_MARKER:?marker required}"
      builtin source "$@"
      return 73
      ;;
  esac
  builtin source "$@"
}
kill() {
  printf '%s\n' HOSTILE_KILL_CALLED >>"${BUBBLES_SCOPE2_ENTRY_MARKER:?marker required}"
  return 73
}
wait() {
  printf '%s\n' HOSTILE_WAIT_CALLED >>"${BUBBLES_SCOPE2_ENTRY_MARKER:?marker required}"
  return 73
}
export -f source kill wait
EOF
}

scope2_assert_cli_entry_case() {
  local mode="$1"
  local modern_bash="$2"
  local fixture_root="$TMP_ROOT/scope2-entry-$mode"
  local hostile_startup="$fixture_root/hostile-startup.sh"
  local marker_file="$fixture_root/hostile.marker"
  local output_file="$fixture_root/caller.output"
  local copied_cli="$fixture_root/bubbles/scripts/cli.sh"
  local entry_status=0
  local marker_count=0
  local fixed_path="/opt/local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

  if ! scope2_prepare_entry_fixture "$fixture_root"; then
    bad "TP-S2-01 SEC-R1 SETUP: $mode copied caller fixture did not preserve every required sibling and mode"
    return
  fi
  scope2_write_hostile_startup "$hostile_startup"

  case "$mode" in
    bash-env)
      if /usr/bin/env \
        PATH="$fixed_path" \
        DEVELOPER_DIR=/Library/Developer/CommandLineTools \
        BASH_ENV="$hostile_startup" \
        BUBBLES_SCOPE2_ENTRY_MARKER="$marker_file" \
        "$modern_bash" "$copied_cli" scan specs/001-scope2-entry \
        >"$output_file" 2>&1 </dev/null; then
        entry_status=0
      else
        entry_status=$?
      fi
      ;;
    exported-functions)
      if /usr/bin/env -u BASH_ENV \
        PATH="$fixed_path" \
        DEVELOPER_DIR=/Library/Developer/CommandLineTools \
        BUBBLES_SCOPE2_ENTRY_MARKER="$marker_file" \
        /bin/bash -c '
          . "$1"
          export -f source kill wait
          exec "$2" "$3" scan specs/001-scope2-entry
        ' _ "$hostile_startup" "$modern_bash" "$copied_cli" \
        >"$output_file" 2>&1 </dev/null; then
        entry_status=0
      else
        entry_status=$?
      fi
      ;;
    *)
      bad "TP-S2-01 SEC-R1 SETUP: unknown hostile entry mode '$mode'"
      return
      ;;
  esac

  marker_count="$(/usr/bin/grep -cF SCANNER_GUARD_SOURCE_INTERCEPTED "$marker_file" 2>/dev/null || true)"
  if [[ "$entry_status" -eq 73 && "$marker_count" -eq 1 ]] &&
    ! /usr/bin/grep -Fq $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' "$output_file"; then
    printf 'RED: TP-S2-01 SEC-R1 mode=%s callerExit=%s hostileScannerSources=%s missing=BSEC1/direct\n' \
      "$mode" "$entry_status" "$marker_count"
    bad "TP-S2-01 SEC-R1: $mode crossed the canonical CLI caller before env -i /bin/bash -p"
  elif [[ "$entry_status" -eq 0 && "$marker_count" -eq 0 ]] &&
    /usr/bin/grep -Fq $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' "$output_file"; then
    ok "TP-S2-01/TP-S2-06 SEC-R1: $mode is excluded before the direct BSEC1 privileged child"
  else
    /bin/cat "$output_file"
    bad "TP-S2-01 SEC-R1 SETUP/CONTRACT: $mode unexpected caller result exit=$entry_status hostileScannerSources=$marker_count"
  fi
}

scope2_assert_cli_status_preservation() {
  local modern_bash="$1"
  local fixture_root="$TMP_ROOT/scope2-entry-status-preservation"
  local output_file="$fixture_root/caller.output"
  local copied_cli="$fixture_root/bubbles/scripts/cli.sh"
  local entry_status=0
  local fixed_path="/opt/local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

  if ! scope2_prepare_entry_fixture "$fixture_root"; then
    bad "TP-S2-06 SETUP: copied CLI status fixture did not preserve every required sibling and mode"
    return
  fi
  cat >"$fixture_root/src/scope2-entry.js" <<'EOF'
export function persistForbiddenCredential(authToken) {
  localStorage.setItem("authToken", authToken);
}
EOF

  if /usr/bin/env \
    PATH="$fixed_path" \
    DEVELOPER_DIR=/Library/Developer/CommandLineTools \
    "$modern_bash" "$copied_cli" scan specs/001-scope2-entry \
    >"$output_file" 2>&1 </dev/null; then
    entry_status=0
  else
    entry_status=$?
  fi

  if [[ "$entry_status" -eq 1 ]] &&
    /usr/bin/grep -Fq $'ENTRY\tBSEC1\tprivileged-bash-entry-v1\tdirect' "$output_file" &&
    /usr/bin/grep -Fq 'supervisorProtocol=BPS1' "$output_file" &&
    /usr/bin/grep -Fq 'VIOLATION [SENSITIVE_CLIENT_STORAGE]' "$output_file" &&
    ! /usr/bin/grep -Fq 'entryMode=compat-reexec' "$output_file"; then
    ok "TP-S2-06 CLI scan preserves scanner exit 1 through direct BSEC1 with valid BPS1 and no ordinary-Bash authority"
  else
    /bin/cat "$output_file"
    bad "TP-S2-06 CLI scan status preservation expected exit 1 with direct BSEC1, BPS1, and the real sensitive-storage finding; observed exit=$entry_status"
  fi
}

if [[ "$RUN_GENERAL" -eq 1 || "$RUN_CLI_INTEGRATION" -eq 1 ]]; then
  echo "Scenario: TP-S2-01/TP-S2-06 canonical CLI callers enter direct BSEC1 and preserve scanner status."
  scope2_modern_bash="$(scope2_fixed_modern_bash || true)"
  if [[ -n "$scope2_modern_bash" ]]; then
    scope2_assert_cli_entry_case bash-env "$scope2_modern_bash"
    scope2_assert_cli_entry_case exported-functions "$scope2_modern_bash"
    scope2_assert_cli_status_preservation "$scope2_modern_bash"
  else
    bad "TP-S2-06 SETUP: no fixed Bash 4+ path is available for the canonical CLI caller"
  fi
  printf '%s\n' 'TP-S2-06_CLI_INTEGRATION_COMPLETED=1 cases=3 retries=0'
fi

if [[ "$RUN_GENERAL" -eq 1 ]]; then
scope2_scanner="$SCRIPT_DIR/implementation-reality-scan.sh"
scope2_compat_line="$(/usr/bin/grep -nF 'BUBBLES_SECURITY_ENTRY_MODE=compat-reexec' "$scope2_scanner" 2>/dev/null | /usr/bin/awk -F: 'NR == 1 { print $1 }' || true)"
scope2_first_source_line="$(/usr/bin/grep -nE '^[[:space:]]*(source|\.)[[:space:]]+' "$scope2_scanner" 2>/dev/null | /usr/bin/awk -F: 'NR == 1 { print $1 }' || true)"
if [[ "$scope2_compat_line" =~ ^[1-9][0-9]*$ && "$scope2_first_source_line" =~ ^[1-9][0-9]*$ &&
  "$scope2_compat_line" -lt "$scope2_first_source_line" ]] &&
  /usr/bin/grep -Fq '/usr/bin/env -i' "$scope2_scanner" &&
  /usr/bin/grep -Fq '/bin/bash -p' "$scope2_scanner"; then
  ok "TP-S2-01 SEC-R1: ordinary direct scanner compatibility re-execs before every framework source"
else
  printf 'RED: TP-S2-01 SEC-R1 scannerCompatLine=%s firstSourceLine=%s missing=compat-reexec/env-i/bash-p\n' \
    "${scope2_compat_line:-absent}" "${scope2_first_source_line:-absent}"
  bad "TP-S2-01 SEC-R1: scanner lacks first-executable-statement compat-reexec before framework sourcing"
fi

echo "Scenario: TP-S2-01 SEC-R2 fixed native supervisor owns one worker through waitpid and BPS1."
scope2_perl_count="$(/usr/bin/grep -cE '/usr/bin/perl[[:space:]].*-T|-T[[:space:]].*/usr/bin/perl' "$ENV_SH" || true)"
scope2_supervisor_contract_count="$(/usr/bin/grep -cF 'root-protected-perl-supervisor-v1' "$ENV_SH" || true)"
scope2_waitpid_count="$(/usr/bin/grep -cE 'waitpid[[:space:]]*\(' "$ENV_SH" || true)"
scope2_bps1_count="$(/usr/bin/grep -cF 'BPS1' "$ENV_SH" || true)"
scope2_fork_count="$(/usr/bin/grep -cE '(^|[^[:alnum:]_])fork[[:space:]]*\(' "$ENV_SH" || true)"
scope2_boundary_api_count="$(/usr/bin/grep -cE '^bubbles_python_security_require_boundary\(\)' "$ENV_SH" || true)"
scope2_waitpid_line="$(/usr/bin/grep -nE 'waitpid[[:space:]]*\(' "$ENV_SH" 2>/dev/null | /usr/bin/awk -F: 'NR == 1 { print $1 }' || true)"
scope2_bps1_complete_line="$(/usr/bin/grep -nF 'COMPLETE\tBPS1' "$ENV_SH" 2>/dev/null | /usr/bin/awk -F: 'NR == 1 { print $1 }' || true)"
scope2_unreaped_signal_guard_count="$(/usr/bin/grep -ciE 'unreaped[[:space:]_-]+(direct[[:space:]_-]+)?worker|worker[[:space:]_-]+owned' "$ENV_SH" || true)"
if [[ "$scope2_perl_count" -gt 0 && "$scope2_supervisor_contract_count" -gt 0 &&
  "$scope2_waitpid_count" -gt 0 && "$scope2_bps1_count" -gt 0 && "$scope2_fork_count" -eq 1 &&
  "$scope2_boundary_api_count" -eq 1 && "$scope2_waitpid_line" =~ ^[1-9][0-9]*$ &&
  "$scope2_bps1_complete_line" =~ ^[1-9][0-9]*$ &&
  "$scope2_waitpid_line" -lt "$scope2_bps1_complete_line" &&
  "$scope2_unreaped_signal_guard_count" -gt 0 ]]; then
  ok "TP-S2-01 SEC-R2: fixed taint-mode Perl, one direct-worker fork, waitpid, BPS1, and boundary API are present"
else
  printf 'RED: TP-S2-01 SEC-R2 perl=%s supervisorContract=%s fork=%s waitpid=%s waitpidLine=%s BPS1=%s BPS1CompleteLine=%s unreapedSignalGuard=%s boundaryApi=%s\n' \
    "$scope2_perl_count" "$scope2_supervisor_contract_count" "$scope2_fork_count" \
    "$scope2_waitpid_count" "${scope2_waitpid_line:-absent}" "$scope2_bps1_count" \
    "${scope2_bps1_complete_line:-absent}" "$scope2_unreaped_signal_guard_count" "$scope2_boundary_api_count"
  bad "TP-S2-01 SEC-R2: fixed /usr/bin/perl waitpid supervision is missing before BPS1 completion"
fi

echo "Scenario: TP-S2-01 HAR-R1 Bash retains only a wait-only supervisor handle."
scope2_legacy_protocol="$(printf '%s%s' 'BP' 'Y1')"
scope2_bash_worker_authority_count="$(/usr/bin/grep -cE "BUBBLES_PYTHON_SECURITY_ACTIVE_PID|_bubbles_python_security_stop_active_child|BUBBLES_PYTHON_SECURITY_FIFO_PATH|READY\\\\t$scope2_legacy_protocol|ERROR\\\\t$scope2_legacy_protocol" "$ENV_SH" || true)"
scope2_supervisor_wait_count="$(/usr/bin/grep -cF 'BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID' "$ENV_SH" || true)"
scope2_forbidden_lifecycle_count="$(/usr/bin/grep -cE '^[[:space:]]*set -m|builtin kill[[:space:]].*"-\$|kill -0|^[[:space:]]*jobs([[:space:]]|$)|^[[:space:]]*disown([[:space:]]|$)|BUBBLES_PYTHON_.*DESCENDANT.*PID' "$ENV_SH" || true)"
scope2_supervisor_wait_line="$(/usr/bin/grep -nE 'builtin wait .*BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID' "$ENV_SH" 2>/dev/null | /usr/bin/awk -F: 'NR == 1 { print $1 }' || true)"
scope2_supervisor_clear_line="$(/usr/bin/grep -nF "BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''" "$ENV_SH" 2>/dev/null | /usr/bin/awk -F: -v wait_line="${scope2_supervisor_wait_line:-0}" '$1 > wait_line { print $1; exit }' || true)"
scope2_pending_after_reap_line="$(/usr/bin/awk -v clear_line="${scope2_supervisor_clear_line:-0}" '
  clear_line > 0 && NR > clear_line && /BUBBLES_PYTHON_SECURITY_PENDING_(SIGNAL|STATUS)/ { print NR; exit }
' "$ENV_SH" || true)"
scope2_bash_signals_supervisor_count="$(/usr/bin/grep -cE 'builtin kill .*BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID' "$ENV_SH" || true)"
if [[ "$scope2_bash_worker_authority_count" -eq 0 && "$scope2_supervisor_wait_count" -gt 0 &&
  "$scope2_forbidden_lifecycle_count" -eq 0 && "$scope2_bash_signals_supervisor_count" -eq 0 &&
  "$scope2_supervisor_wait_line" =~ ^[1-9][0-9]*$ && "$scope2_supervisor_clear_line" =~ ^[1-9][0-9]*$ &&
  "$scope2_pending_after_reap_line" =~ ^[1-9][0-9]*$ &&
  "$scope2_supervisor_wait_line" -lt "$scope2_supervisor_clear_line" &&
  "$scope2_supervisor_clear_line" -lt "$scope2_pending_after_reap_line" ]]; then
  ok "TP-S2-01 HAR-R1: Bash has no worker/watchdog signaling, PID probing, process-group, job-control, or descendant-cleanup authority"
else
  printf 'RED: TP-S2-01 HAR-R1 bashWorkerAuthority=%s supervisorWaitHandle=%s waitLine=%s clearLine=%s pendingAfterReapLine=%s bashSignalsSupervisor=%s forbiddenLifecycle=%s\n' \
    "$scope2_bash_worker_authority_count" "$scope2_supervisor_wait_count" \
    "${scope2_supervisor_wait_line:-absent}" "${scope2_supervisor_clear_line:-absent}" \
    "${scope2_pending_after_reap_line:-absent}" "$scope2_bash_signals_supervisor_count" \
    "$scope2_forbidden_lifecycle_count"
  bad "TP-S2-01 HAR-R1: active Bash lifecycle still stores or signals worker/FIFO authority instead of waiting only for the supervisor"
fi

echo "Scenario: TP-S2-01 HAR-R2 only supervisor waitpid decides completion."
scope2_worker_control_count="$(/usr/bin/grep -cE "exec 9>.*FIFO|BUBBLES_PYTHON_SECURITY_FIFO_PATH|READY\\\\t$scope2_legacy_protocol|ERROR\\\\t$scope2_legacy_protocol" "$ENV_SH" || true)"
if [[ "$scope2_worker_control_count" -eq 0 && "$scope2_waitpid_count" -gt 0 && "$scope2_bps1_count" -gt 0 ]]; then
  ok "TP-S2-01 HAR-R2: worker-held FIFO, EOF, and text are absent from completion authority"
else
  printf 'RED: TP-S2-01 HAR-R2 workerControl=%s waitpid=%s BPS1=%s\n' \
    "$scope2_worker_control_count" "$scope2_waitpid_count" "$scope2_bps1_count"
  bad "TP-S2-01 HAR-R2: worker-held FIFO/EOF still participates in completion and no supervisor BPS1 authority exists"
fi

printf '%s\n' 'TP-S2-01_EPOCH=privileged-native-supervision-v2'
printf '%s\n' 'TP-S2-01_RETAINED_WORKER_TRUST=root-protected-native-python-v1'

run_scope2_privileged_security_suite
fi

if [[ "$RUN_SIGNAL_TARGET" -eq 1 ]]; then
  signal_target_status=0
  scope2_signal_target_finalize || signal_target_status=$?
  SELFTEST_COMPLETED=1
  exit "$signal_target_status"
fi
if [[ "$RUN_STRESS_COMPLETENESS_CONTROL" -eq 1 ]]; then
  printf '%s\n' 'TP-S2-05_STRESS_COMPLETENESS_NEGATIVE_CONTROL classes=0/7 iterations=0/210 endMarker=0 retries=0'
  RUN_STRESS=1
fi
if [[ "$RUN_SIGNAL_READINESS" -eq 1 &&
  "$SCOPE2_SIGNAL_READINESS_COMPLETED" -ne 1 ]]; then
  bad "TP-S2-05 focused signal controls exited before HUP, INT, TERM, and the completion sentinel"
fi
if [[ "$RUN_STRESS" -eq 1 && "$SCOPE2_STRESS_COMPLETED" -ne 1 ]]; then
  bad "TP-S2-05 stress lifecycle exited before all seven classes, 210 iterations, and the end sentinel"
fi
if [[ "$RUN_GENERAL" -eq 1 || "$RUN_BASH32_AGGREGATE" -eq 1 ]]; then
  echo "Scenario: REG-PY-BASH32-01 stock Bash executes PRE-HAR-01 and the default aggregate accounts for it."
  run_reg_py_bash32_01_control
  printf 'REG_PY_BASH32_01_AGGREGATE attempts=%s expected=1\n' \
    "$PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS"
  if [[ "$PRE_HAR_01_DEFAULT_AGGREGATE_ATTEMPTS" -ne 1 ]]; then
    bad "REG-PY-BASH32-01: PRE-HAR-01 aggregate invocation count must be exactly one"
  fi
  if [[ "$RUN_GENERAL" -eq 1 && "$PRE_HAR_01_STOCK_SCENARIO_STATUS" -ne 0 ]]; then
    bad "REG-PY-REAP-01: the default aggregate must propagate the PRE-HAR-01 lifecycle RED result"
  fi
fi
echo
echo "python-env selftest: $pass passed, $fail failed"
SELFTEST_COMPLETED=1
[[ "$fail" -eq 0 ]]
