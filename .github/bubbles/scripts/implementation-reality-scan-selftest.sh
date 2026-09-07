#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAN_SCRIPT="$SCRIPT_DIR/implementation-reality-scan.sh"
SELFTEST_SCRIPT="$SCRIPT_DIR/implementation-reality-scan-selftest.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
SELFTEST_ENTRYPOINT=full

case "${1:-}" in
  '')
    if [[ "$#" -ne 0 ]]; then
      printf '%s\n' 'implementation-reality-scan selftest accepts no empty positional arguments' >&2
      exit 2
    fi
    ;;
  --internal-dispatch-probe)
    if [[ "$#" -ne 1 ]]; then
      printf '%s\n' 'implementation-reality-scan selftest dispatch probe accepts no additional arguments' >&2
      exit 2
    fi
    printf '%s\n' 'IMPLEMENTATION_REALITY_SELFTEST_ZERO_ARGUMENT_ENTRY=FULL_SUITE'
    exit 0
    ;;
  --internal-authority-bypass-control)
    if [[ "$#" -ne 2 || "${2:-}" != b039-authority-bypass-v1 ]]; then
      printf '%s\n' 'implementation-reality-scan authority control requires its explicit internal token' >&2
      exit 2
    fi
    SELFTEST_ENTRYPOINT=authority-bypass
    ;;
  --internal-mutation-runner-control)
    if [[ "$#" -ne 2 || "${2:-}" != b039-mutation-runner-v1 ]]; then
      printf '%s\n' 'implementation-reality-scan mutation runner control requires its explicit internal token' >&2
      exit 2
    fi
    SELFTEST_ENTRYPOINT=mutation-runner
    ;;
  --internal-mutation-runner-focused-control)
    if [[ "$#" -ne 2 || "${2:-}" != b039-mutation-runner-focused-v1 ]]; then
      printf '%s\n' 'implementation-reality-scan focused mutation runner control requires its explicit internal token' >&2
      exit 2
    fi
    SELFTEST_ENTRYPOINT=mutation-runner-focused
    ;;
  --internal-lifecycle-wait-boundary-control)
    if [[ "$#" -ne 2 || "${2:-}" != b039-lifecycle-wait-boundary-v1 ]]; then
      printf '%s\n' 'implementation-reality-scan lifecycle wait-boundary control requires its explicit internal token' >&2
      exit 2
    fi
    SELFTEST_ENTRYPOINT=lifecycle-wait-boundary
    ;;
  *)
    printf 'implementation-reality-scan selftest argument is invalid: %s\n' "$1" >&2
    exit 2
    ;;
esac

TMPDIR="$(mktemp -d)"
FIXTURE_ROOT="$TMPDIR/fixtures"
CLASSIFIER_HELPER_CACHE_DIR="$SCRIPT_DIR/guards/__pycache__"
SELFTEST_COMPLETED=0
SELFTEST_LIFECYCLE_PID=''
SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS=0
SELFTEST_LIFECYCLE_SIGNAL_MODE=normal
SELFTEST_LIFECYCLE_BOUNDARY_CONTROL=0
SELFTEST_LIFECYCLE_BOUNDARY_OBSERVED=0
SELFTEST_MUTATION_TIMED_OUT=0
SELFTEST_MUTATION_RUN_DIAGNOSTIC=NOT_RUN
SELFTEST_MUTATION_SUPERVISOR_PROTOCOL=none
SELFTEST_MUTATION_SUPERVISOR_COMPLETED=0
SELFTEST_MUTATION_SUPERVISOR_OWNER=none
SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND=none
SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED=0
SELFTEST_MUTATION_SUPERVISOR_EVENTS=none
SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION=none
SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal

selftest_stop_exact_child() {
  local child_pid=''

  child_pid="$SELFTEST_LIFECYCLE_PID" SELFTEST_LIFECYCLE_PID=''
  if [[ "$child_pid" =~ ^[1-9][0-9]*$ ]]; then
    if [[ "$SELFTEST_LIFECYCLE_SIGNAL_MODE" == record-only ]]; then
      SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS=$((SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS + 2))
      return 97
    fi
    SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS=$((SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS + 1))
    builtin kill -TERM "$child_pid" 2>/dev/null || true
    SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS=$((SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS + 1))
    builtin kill -KILL "$child_pid" 2>/dev/null || true
    builtin wait "$child_pid" 2>/dev/null || true
  fi
  return 0
}

selftest_lifecycle_post_wait_adversary() {
  local child_pid="$1"
  local child_status="$2"
  local observed_handle="$SELFTEST_LIFECYCLE_PID"
  local process_text=''
  local process_status=0
  local attempts_before="$SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS"
  local attempts_after=0
  local cleanup_status=0
  local observed_handle_state=invalid

  if [[ -z "$observed_handle" ]]; then
    observed_handle_state=absent
  elif [[ "$observed_handle" =~ ^[1-9][0-9]*$ ]]; then
    observed_handle_state=numeric
  fi

  if process_text="$(/bin/ps -p "$child_pid" -o pid= 2>&1)"; then
    process_status=0
  else
    process_status=$?
  fi
  SELFTEST_LIFECYCLE_SIGNAL_MODE=record-only
  if selftest_stop_exact_child; then
    cleanup_status=0
  else
    cleanup_status=$?
  fi
  SELFTEST_LIFECYCLE_SIGNAL_MODE=normal
  attempts_after="$SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS"
  printf 'LIFECYCLE_WAIT_BOUNDARY_CASE waitStatus=%s processStatus=%s processText=%q observedHandleState=%s cleanupStatus=%s cleanupSignalsBefore=%s cleanupSignalsAfter=%s\n' \
    "$child_status" "$process_status" "$process_text" "$observed_handle_state" \
    "$cleanup_status" "$attempts_before" "$attempts_after"
  if [[ "$observed_handle" =~ ^[1-9][0-9]*$ ]]; then
    if [[ "$cleanup_status" -eq 97 && -z "$SELFTEST_LIFECYCLE_PID" &&
      "$attempts_after" -eq $((attempts_before + 2)) ]]; then
      printf '%s\n' 'RED: NEG-B039-LIFECYCLE-WAIT-BOUNDARY cleanup reached stale numeric handle and recorded two suppressed signals' >&2
      return 97
    fi
    printf '%s\n' 'FAIL: lifecycle wait-boundary stale-handle adversary did not execute the safe cleanup branch' >&2
    return 96
  fi
  if [[ "$child_status" -eq 0 && "$process_status" -eq 1 && -z "$process_text" &&
    -z "$observed_handle" && "$cleanup_status" -eq 0 && -z "$SELFTEST_LIFECYCLE_PID" &&
    "$attempts_before" -eq "$attempts_after" ]]; then
    SELFTEST_LIFECYCLE_BOUNDARY_OBSERVED=1
    printf '%s\n' 'PASS: lifecycle wait clears cleanup authority before definitive reap returns to Bash'
    return 0
  fi
  printf '%s\n' 'FAIL: lifecycle wait-boundary adversary did not reach a reaped child with empty cleanup authority' >&2
  return 96
}

selftest_wait_exact_child() {
  local expected_pid="$1"
  local child_pid=''
  local child_status=0
  local boundary_status=0

  child_pid="$SELFTEST_LIFECYCLE_PID" SELFTEST_LIFECYCLE_PID='' # B039-LIFECYCLE-ATOMIC-TAKE
  if [[ ! "$child_pid" =~ ^[1-9][0-9]*$ || "$child_pid" != "$expected_pid" ]]; then
    return 125
  fi
  if builtin wait "$child_pid" 2>/dev/null; then
    child_status=0
  else
    child_status=$?
  fi
  if [[ "$SELFTEST_LIFECYCLE_BOUNDARY_CONTROL" -eq 1 ]]; then
    if selftest_lifecycle_post_wait_adversary "$child_pid" "$child_status"; then
      boundary_status=0
    else
      boundary_status=$?
    fi
    [[ "$boundary_status" -eq 0 ]] || return "$boundary_status"
  fi
  return "$child_status"
}

selftest_mutation_supervisor_program() {
  /bin/cat <<'PERL'
sub untaint_blob {
  my ($raw, $maximum) = @_;
  return undef if !defined($raw) || length($raw) > $maximum;
  return $1 if $raw =~ /\A([^\0]*)\z/s;
  return undef;
}

sub untaint_absolute_path {
  my ($raw) = @_;
  return undef if !defined($raw);
  return $1 if $raw =~ m{\A(/[^\0\r\n\t]{0,4095})\z};
  return undef;
}

sub emit_control {
  my ($control, $status, $owner, $timed_out, $worker_kind, $ownership_registered, $events) = @_;
  my $record = join("\t", "COMPLETE", "BMR1", $status, $owner, $timed_out,
    $worker_kind, $ownership_registered, $events) . "\n";
  my $offset = 0;
  while ($offset < length($record)) {
    my $written = syswrite($control, $record, length($record) - $offset, $offset);
    return 0 if !defined($written) || $written <= 0;
    $offset += $written;
  }
  return 1;
}

my $output_path = untaint_absolute_path(shift @ARGV);
my $raw_wall_seconds = shift @ARGV;
my $raw_test_mode = shift @ARGV;
exit 2 if !defined($output_path) || !defined($raw_wall_seconds) ||
  $raw_wall_seconds !~ /\A([1-9][0-9]{0,2})\z/;
my $wall_seconds = 0 + $1;
exit 2 if !defined($raw_test_mode) || $raw_test_mode !~ /\A(normal|deadline-edge)\z/;
my $test_mode = $1;
my @command = ();
my $aggregate_bytes = 0;
for my $raw (@ARGV) {
  my $value = untaint_blob($raw, 262144);
  exit 2 if !defined($value);
  $aggregate_bytes += length($value);
  exit 2 if $aggregate_bytes > 1048576;
  push @command, $value;
}
exit 2 if !@command || !defined(untaint_absolute_path($command[0]));

open(my $control_writer, ">&", \*STDOUT) or exit 125;
open(my $worker_output, ">", $output_path) or exit 125;
my $worker_pid = fork();
if (!defined($worker_pid)) {
  emit_control($control_writer, 125, "supervisor", 0, "not-started", 0, "SETUP_FAILED");
  close($control_writer);
  close($worker_output);
  exit 125;
}
if ($worker_pid == 0) {
  close($control_writer);
  open(STDIN, "<", "/dev/null") or exit 126;
  open(STDOUT, ">&", $worker_output) or exit 126;
  open(STDERR, ">&", $worker_output) or exit 126;
  close($worker_output);
  $SIG{"HUP"} = "DEFAULT";
  $SIG{"INT"} = "DEFAULT";
  $SIG{"TERM"} = "DEFAULT";
  $SIG{"PIPE"} = "DEFAULT";
  %ENV = ("LC_ALL" => "C", "PATH" => "/usr/bin:/bin");
  exec { $command[0] } @command or do {
    print STDERR "MUTATION_WORKER_EXEC_FAILED\n";
    exit 126;
  };
}
close($worker_output);

my @events = ("FORK");
my $worker_is_owned = 1;
my $ownership_registered = 1; # B039-MUTATION-OWNERSHIP-REGISTRATION
push @events, $ownership_registered ? "OWNERSHIP_REGISTER" : "OWNERSHIP_MISSING";
my $raw_wait_status = 0;
my $wait_failed = 0;
my $wall_expired = 0;
my $grace_expired = 0;
my $alarm_phase = "wall";
my $termination_reason = "";
my $term_sent = 0;
my $kill_sent = 0;
my $pending_signal = "";
my $pending_signal_status = 0;
my $grace_seconds = 2;

$SIG{"HUP"} = sub { if ($pending_signal eq "") { $pending_signal = "HUP"; $pending_signal_status = 129; } };
$SIG{"INT"} = sub { if ($pending_signal eq "") { $pending_signal = "INT"; $pending_signal_status = 130; } };
$SIG{"TERM"} = sub { if ($pending_signal eq "") { $pending_signal = "TERM"; $pending_signal_status = 143; } };
$SIG{"ALRM"} = sub {
  if ($alarm_phase eq "wall") { $wall_expired = 1; }
  else { $grace_expired = 1; }
};

my $signal_owned_worker = sub {
  my ($signal_number) = @_;
  if (!$worker_is_owned) { # B039-MUTATION-POST-REAP-GUARD
    push @events, "SIGNAL_SKIPPED_UNOWNED";
    return 0;
  }
  if ($test_mode eq "deadline-edge") {
    push @events, $worker_is_owned ? "SIGNAL_WOULD_SEND_OWNED" : "SIGNAL_WOULD_SEND_UNOWNED";
    return 1;
  }
  my $sent = kill($signal_number, $worker_pid);
  push @events, $signal_number == 15 ? "TERM_SIGNAL_OWNED" : "KILL_SIGNAL_OWNED";
  return $sent;
};

alarm($wall_seconds); # B039-MUTATION-WALL-BOUND
while ($worker_is_owned) {
  my $waited_pid = waitpid($worker_pid, 1);
  if ($waited_pid == $worker_pid) {
    $raw_wait_status = $?;
    push @events, "WAITPID";
    $worker_is_owned = 0;
    push @events, "OWNERSHIP_CLEAR";
    last;
  }
  if ($waited_pid == -1) {
    $wait_failed = 1;
    push @events, "WAITPID_ERROR";
    $worker_is_owned = 0;
    push @events, "OWNERSHIP_CLEAR";
    last;
  }

  if ($termination_reason eq "") {
    if ($pending_signal ne "") {
      $termination_reason = "caller-signal";
    } elsif ($wall_expired) {
      $termination_reason = "timeout";
    }
  }
  if ($termination_reason ne "" && !$term_sent) {
    $signal_owned_worker->(15);
    $term_sent = 1;
    $alarm_phase = "grace";
    $grace_expired = 0;
    alarm($grace_seconds);
  } elsif ($term_sent && $grace_expired && !$kill_sent) {
    $signal_owned_worker->(9);
    $kill_sent = 1;
  }
  select(undef, undef, undef, 0.01);
}
alarm(0);

if ($test_mode eq "deadline-edge" && !$wait_failed) {
  $wall_expired = 1;
  push @events, "DEADLINE_EDGE";
  $signal_owned_worker->(15);
}

my $final_status = 0;
my $final_owner = "worker";
my $timed_out = 0;
my $worker_kind = "exit";
if ($wait_failed) {
  $final_status = 125;
  $final_owner = "supervisor";
  $worker_kind = "not-started";
} elsif ($termination_reason eq "caller-signal") {
  $final_status = $pending_signal_status;
  $final_owner = "caller-signal";
  $worker_kind = ($raw_wait_status & 127) ? "signal" : "exit";
} elsif ($termination_reason eq "timeout") {
  $final_status = 124;
  $final_owner = "supervisor";
  $timed_out = 1;
  $worker_kind = ($raw_wait_status & 127) ? "signal" : "exit";
} elsif ($raw_wait_status & 127) {
  $final_status = 128 + ($raw_wait_status & 127);
  $worker_kind = "signal";
} else {
  $final_status = ($raw_wait_status >> 8) & 255;
}
push @events, "COMPLETE";
my $event_record = join(",", @events);
emit_control($control_writer, $final_status, $final_owner, $timed_out, $worker_kind,
  $ownership_registered, $event_record) or exit 125;
close($control_writer);
exit $final_status;
PERL
}

selftest_fixed_perl_is_trusted_for_harness() {
  local metadata="" mode="" _links="" uid="" _gid="" _remainder=""
  [[ -x /usr/bin/perl ]] || return 1
  metadata="$(LC_ALL=C /bin/ls -Lldn /usr/bin/perl 2>/dev/null)" || return 1
  read -r mode _links uid _gid _remainder <<<"$metadata"
  [[ ${#mode} -ge 10 && "${mode:0:1}" == - && "$uid" == 0 ]] || return 1
  [[ "${mode:5:1}" != w && "${mode:8:1}" != w ]] || return 1
  return 0
}

selftest_run_mutation_bounded() {
  local output_file="$1"
  local wall_seconds="$2"
  shift 2
  local supervisor_program=""
  local control_record=""
  local record_type="" protocol="" record_status="" owner="" timed_out=""
  local worker_kind="" ownership_registered="" events="" extra=""
  local supervisor_status=0
  local test_mode="$SELFTEST_MUTATION_SUPERVISOR_TEST_MODE"

  SELFTEST_MUTATION_TIMED_OUT=0
  SELFTEST_MUTATION_RUN_DIAGNOSTIC=NOT_RUN
  SELFTEST_MUTATION_SUPERVISOR_PROTOCOL=none
  SELFTEST_MUTATION_SUPERVISOR_COMPLETED=0
  SELFTEST_MUTATION_SUPERVISOR_OWNER=none
  SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND=none
  SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED=0
  SELFTEST_MUTATION_SUPERVISOR_EVENTS=none
  SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION=none
  if [[ "$output_file" != /* || "$output_file" == *$'\n'* ||
    ! "$wall_seconds" =~ ^[1-9][0-9]{0,2}$ || "$#" -eq 0 ||
    "$1" != /* || ( "$test_mode" != normal && "$test_mode" != deadline-edge ) ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=ARGUMENT_INVALID
    return 2
  fi
  if [[ ! -x /usr/bin/perl ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_UNAVAILABLE
    return 127
  fi
  if ! selftest_fixed_perl_is_trusted_for_harness; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_UNTRUSTED
    return 127
  fi

  # BMR1 is test-harness infrastructure, not security authority. Its fixed
  # /usr/bin/perl program may execute arbitrary absolute argv only so copied
  # scanner mutations can run. The native supervisor alone forks, signals,
  # and reaps that worker. Bash receives only the post-waitpid control record.
  supervisor_program="$(selftest_mutation_supervisor_program)"
  if control_record="$(/usr/bin/perl -T -w -e "$supervisor_program" \
    "$output_file" "$wall_seconds" "$test_mode" "$@")"; then
    supervisor_status=0
  else
    supervisor_status=$?
  fi
  IFS=$'\t' read -r record_type protocol record_status owner timed_out \
    worker_kind ownership_registered events extra <<<"$control_record"
  if [[ -n "$extra" || "$record_type" != COMPLETE || "$protocol" != BMR1 ||
    ! "$record_status" =~ ^[0-9]+$ || "$record_status" -ne "$supervisor_status" ||
    ( "$owner" != worker && "$owner" != supervisor && "$owner" != caller-signal ) ||
    ! "$timed_out" =~ ^[01]$ ||
    ( "$worker_kind" != exit && "$worker_kind" != signal && "$worker_kind" != not-started ) ||
    ! "$ownership_registered" =~ ^[01]$ || -z "$events" ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_PROTOCOL_INVALID
    return 125
  fi

  SELFTEST_MUTATION_SUPERVISOR_PROTOCOL=BMR1
  SELFTEST_MUTATION_SUPERVISOR_COMPLETED=1
  SELFTEST_MUTATION_SUPERVISOR_OWNER="$owner"
  SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND="$worker_kind"
  SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED="$ownership_registered"
  SELFTEST_MUTATION_SUPERVISOR_EVENTS="$events"
  SELFTEST_MUTATION_TIMED_OUT="$timed_out"
  case "$events" in
    *SIGNAL_WOULD_SEND_UNOWNED*) SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION='forbidden-post-reap' ;;
    *SIGNAL_SKIPPED_UNOWNED*) SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION='skipped-post-reap' ;;
    *TERM_SIGNAL_OWNED* | *KILL_SIGNAL_OWNED*) SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION='owned-worker' ;;
    *) SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION=none ;;
  esac

  if [[ "$events" == SETUP_FAILED && "$owner" == supervisor &&
    "$record_status" -eq 125 && "$ownership_registered" -eq 0 ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_SETUP_FAILED
    return 125
  fi
  if [[ "$ownership_registered" -ne 1 || "$events" == *OWNERSHIP_MISSING* ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=OWNERSHIP_REGISTRATION_INVALID
    return 125
  fi
  if [[ "$events" != FORK,OWNERSHIP_REGISTER,*WAITPID,OWNERSHIP_CLEAR,*COMPLETE ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_EVENT_ORDER_INVALID
    return 125
  fi
  if [[ "$events" == *SIGNAL_WOULD_SEND_UNOWNED* ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=POST_REAP_SIGNAL_DECISION
    return 125
  fi
  if [[ "$test_mode" == deadline-edge &&
    "$events" != FORK,OWNERSHIP_REGISTER,WAITPID,OWNERSHIP_CLEAR,DEADLINE_EDGE,SIGNAL_SKIPPED_UNOWNED,COMPLETE ]]; then
    SELFTEST_MUTATION_RUN_DIAGNOSTIC=DEADLINE_EDGE_ORDER_INVALID
    return 125
  fi

  case "$owner:$record_status:$timed_out:$worker_kind" in
    worker:0:0:exit) SELFTEST_MUTATION_RUN_DIAGNOSTIC=OK ;;
    worker:*:0:signal) SELFTEST_MUTATION_RUN_DIAGNOSTIC=CHILD_SIGNAL ;;
    worker:*:0:exit) SELFTEST_MUTATION_RUN_DIAGNOSTIC=CHILD_EXIT_NONZERO ;;
    supervisor:124:1:*) SELFTEST_MUTATION_RUN_DIAGNOSTIC=TIMEOUT ;;
    caller-signal:129:0:*) SELFTEST_MUTATION_RUN_DIAGNOSTIC=SIGNAL_HUP ;;
    caller-signal:130:0:*) SELFTEST_MUTATION_RUN_DIAGNOSTIC=SIGNAL_INT ;;
    caller-signal:143:0:*) SELFTEST_MUTATION_RUN_DIAGNOSTIC=SIGNAL_TERM ;;
    *) SELFTEST_MUTATION_RUN_DIAGNOSTIC=SUPERVISOR_PROTOCOL_INVALID; return 125 ;;
  esac
  return "$supervisor_status"
}

selftest_run_internal_mutation_runner_control() {
  local output_dir="$TMPDIR/internal-mutation-runner"
  local output_file=""
  local runner_status=0
  local control_failures=0

  mkdir -p "$output_dir"

  if [[ -n "${BUBBLES_MUTATION_RUNNER_ROOT_RECORD:-}" ]]; then
    printf '%s\n' "$TMPDIR" >"$BUBBLES_MUTATION_RUNNER_ROOT_RECORD"
  fi

  output_file="$output_dir/timeout.output"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal
  if selftest_run_mutation_bounded "$output_file" 1 /bin/sleep 2; then
    runner_status=0
  else
    runner_status=$?
  fi
  printf 'MUTATION_RUNNER_CASE name=timeout status=%s timedOut=%s diagnostic=%s protocol=%s completed=%s ownership=%s owner=%s kind=%s events=%s\n' \
    "$runner_status" "$SELFTEST_MUTATION_TIMED_OUT" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" \
    "$SELFTEST_MUTATION_SUPERVISOR_PROTOCOL" "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" \
    "$SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED" "$SELFTEST_MUTATION_SUPERVISOR_OWNER" \
    "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" "$SELFTEST_MUTATION_SUPERVISOR_EVENTS"
  if [[ "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == OWNERSHIP_REGISTRATION_INVALID ]]; then
    printf '%s\n' 'RED: NEG-B039-MUTATION-REGISTRATION native supervisor omitted ownership registration' >&2
    return 1
  fi
  if [[ "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 && "$runner_status" -eq 0 &&
    "$SELFTEST_MUTATION_TIMED_OUT" -eq 0 && "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == OK ]]; then
    printf '%s\n' 'RED: NEG-B039-MUTATION-BOUND native supervisor exceeded the declared wall' >&2
    return 1
  fi
  if [[ "$runner_status" -eq 124 && "$SELFTEST_MUTATION_TIMED_OUT" -eq 1 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == TIMEOUT &&
    "$SELFTEST_MUTATION_SUPERVISOR_PROTOCOL" == BMR1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == supervisor ]]; then
    printf '%s\n' 'PASS: mutation runner native supervisor enforces timeout status 124'
  else
    printf '%s\n' 'FAIL: mutation runner timeout control returned an unexpected native-supervisor result' >&2
    control_failures=$((control_failures + 1))
  fi

  output_file="$output_dir/child-124.output"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal
  if selftest_run_mutation_bounded "$output_file" 3 /bin/sh -c 'exit 124'; then
    runner_status=0
  else
    runner_status=$?
  fi
  printf 'MUTATION_RUNNER_CASE name=child-124 status=%s timedOut=%s diagnostic=%s owner=%s kind=%s completion=%s\n' \
    "$runner_status" "$SELFTEST_MUTATION_TIMED_OUT" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" \
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" \
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED"
  if [[ "$runner_status" -eq 124 && "$SELFTEST_MUTATION_TIMED_OUT" -eq 0 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == CHILD_EXIT_NONZERO &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == worker &&
    "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" == exit ]]; then
    printf '%s\n' 'PASS: mutation runner distinguishes child exit 124 from supervisor timeout 124'
  else
    printf '%s\n' 'FAIL: mutation runner child-124 distinction failed' >&2
    control_failures=$((control_failures + 1))
  fi

  output_file="$output_dir/fast-nonzero.output"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal
  if selftest_run_mutation_bounded "$output_file" 3 /bin/sh -c \
    'printf "COMPLETE\tBMR1\t0\tworker\t0\texit\t1\tFORGED\n"; exit 73'; then
    runner_status=0
  else
    runner_status=$?
  fi
  printf 'MUTATION_RUNNER_CASE name=fast-nonzero status=%s timedOut=%s diagnostic=%s owner=%s kind=%s completion=%s\n' \
    "$runner_status" "$SELFTEST_MUTATION_TIMED_OUT" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" \
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" \
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED"
  if [[ "$runner_status" -eq 73 && "$SELFTEST_MUTATION_TIMED_OUT" -eq 0 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == CHILD_EXIT_NONZERO &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == worker &&
    "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" == exit ]] &&
    /usr/bin/grep -Fq $'COMPLETE\tBMR1\t0\tworker\t0\texit\t1\tFORGED' "$output_file"; then
    printf '%s\n' 'PASS: mutation runner preserves fast nonzero status and rejects worker-authored control text'
  else
    printf '%s\n' 'FAIL: mutation runner fast-nonzero or private-control proof failed' >&2
    control_failures=$((control_failures + 1))
  fi

  output_file="$output_dir/real-signal.output"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal
  if selftest_run_mutation_bounded "$output_file" 3 /bin/sh -c 'kill -TERM "$$"'; then
    runner_status=0
  else
    runner_status=$?
  fi
  printf 'MUTATION_RUNNER_CASE name=real-signal status=%s timedOut=%s diagnostic=%s owner=%s kind=%s completion=%s\n' \
    "$runner_status" "$SELFTEST_MUTATION_TIMED_OUT" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" \
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" \
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED"
  if [[ "$runner_status" -eq 143 && "$SELFTEST_MUTATION_TIMED_OUT" -eq 0 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == CHILD_SIGNAL &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == worker &&
    "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" == signal ]]; then
    printf '%s\n' 'PASS: mutation runner preserves a real child signal as status 143'
  else
    printf '%s\n' 'FAIL: mutation runner real-signal status ownership failed' >&2
    control_failures=$((control_failures + 1))
  fi

  output_file="$output_dir/deadline-edge.output"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=deadline-edge
  if selftest_run_mutation_bounded "$output_file" 3 /bin/sh -c 'exit 0'; then
    runner_status=0
  else
    runner_status=$?
  fi
  printf 'MUTATION_RUNNER_CASE name=deadline-edge status=%s timedOut=%s diagnostic=%s owner=%s kind=%s completion=%s signalDecision=%s events=%s\n' \
    "$runner_status" "$SELFTEST_MUTATION_TIMED_OUT" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" \
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" "$SELFTEST_MUTATION_SUPERVISOR_WORKER_KIND" \
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" "$SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION" \
    "$SELFTEST_MUTATION_SUPERVISOR_EVENTS"
  SELFTEST_MUTATION_SUPERVISOR_TEST_MODE=normal
  if [[ "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == POST_REAP_SIGNAL_DECISION ]]; then
    printf '%s\n' 'RED: NEG-B039-MUTATION-GUARD post-reap deadline made a forbidden signal decision' >&2
    return 1
  fi
  if [[ "$runner_status" -eq 0 && "$SELFTEST_MUTATION_TIMED_OUT" -eq 0 &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == OK &&
    "$SELFTEST_MUTATION_SUPERVISOR_SIGNAL_DECISION" == skipped-post-reap &&
    "$SELFTEST_MUTATION_SUPERVISOR_EVENTS" == FORK,OWNERSHIP_REGISTER,WAITPID,OWNERSHIP_CLEAR,DEADLINE_EDGE,SIGNAL_SKIPPED_UNOWNED,COMPLETE ]]; then
    printf '%s\n' 'PASS: mutation runner deadline edge orders waitpid then ownership clear then no signal'
  else
    printf '%s\n' 'FAIL: mutation runner deadline-edge post-reap ownership order failed' >&2
    control_failures=$((control_failures + 1))
  fi

  printf 'MUTATION_RUNNER_FOCUSED_SUMMARY failures=%s cases=5 protocol=BMR1 bashWorkerPidState=absent bashWatchdogPidState=absent\n' \
    "$control_failures"
  [[ "$control_failures" -eq 0 ]]
}

make_mutation_runner_selftest() {
  local mode="$1"
  local destination="$2"
  /usr/bin/awk -v mode="$mode" '
    mode == "bound" && /^[[:space:]]*alarm\(\$wall_seconds\); # B039-MUTATION-WALL-BOUND$/ {
      sub(/alarm\(\$wall_seconds\)/, "alarm($wall_seconds + 2)")
      print
      changed=changed + 1
      next
    }
    mode == "registration" && /^[[:space:]]*my \$ownership_registered = 1; # B039-MUTATION-OWNERSHIP-REGISTRATION$/ {
      sub(/my \$ownership_registered = 1/, "my $ownership_registered = 0")
      print
      changed=changed + 1
      next
    }
    mode == "guard" && /^[[:space:]]*if \(!\$worker_is_owned\) \{ # B039-MUTATION-POST-REAP-GUARD$/ {
      sub(/if \(!\$worker_is_owned\)/, "if (0)")
      print
      changed=changed + 1
      next
    }
    { print }
    END { if (changed != 1) exit 42 }
  ' "$SELFTEST_SCRIPT" >"$destination"
}

selftest_check_mutation_runner_negative_control() {
  local mode="$1"
  local expected_assertion="$2"
  local mutation_dir="$TMPDIR/mutation-runner-$mode"
  local mutant_script="$mutation_dir/implementation-reality-scan-selftest.sh"
  local output_file="$mutation_dir/mutant.output"
  local internal_tmp_parent="$TMPDIR/mutation-runner-inner-$mode"
  local root_record="$mutation_dir/internal-root.record"
  local control_status=0
  local internal_root=""

  mkdir -p "$mutation_dir" "$internal_tmp_parent"
  if ! make_mutation_runner_selftest "$mode" "$mutant_script"; then
    printf 'FAIL: NEG-B039-MUTATION-%s copied mutation construction did not match exactly once\n' "$mode" >&2
    return 1
  fi
  if selftest_run_mutation_bounded "$output_file" 10 /usr/bin/env \
    TMPDIR="$internal_tmp_parent" BUBBLES_MUTATION_RUNNER_ROOT_RECORD="$root_record" \
    /bin/bash "$mutant_script" \
    --internal-mutation-runner-control b039-mutation-runner-v1; then
    control_status=0
  else
    control_status=$?
  fi
  /bin/cat "$output_file"
  internal_root="$(/bin/cat "$root_record" 2>/dev/null || true)"

  if [[ "$control_status" -eq 1 && "${internal_root##*/}" == tmp.* &&
    ! -e "$internal_root" && "$SELFTEST_MUTATION_SUPERVISOR_PROTOCOL" == BMR1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == worker &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == CHILD_EXIT_NONZERO ]] &&
    /usr/bin/grep -Fq "$expected_assertion" "$output_file"; then
    printf 'RED: NEG-B039-MUTATION-%s mutantExit=1 rootAbsent=yes outerProtocol=BMR1 outerOwnership=registered bashWorkerPidState=absent bashWatchdogPidState=absent\n' "$mode"
    printf 'PASS: NEG-B039-MUTATION-%s copied mutation turns its owning lifecycle assertion RED without residue\n' "$mode"
    return 0
  fi
  printf 'FAIL: NEG-B039-MUTATION-%s expected exit 1, native-supervisor completion, and absent residue; got exit=%s root=%s diagnostic=%s\n' \
    "$mode" "$control_status" "${internal_root:-missing}" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" >&2
  return 1
}

make_lifecycle_wait_boundary_mutant() {
  local destination="$1"
  /usr/bin/awk '
    /B039-LIFECYCLE-ATOMIC-TAKE$/ {
      sub(/ SELFTEST_LIFECYCLE_PID=.*/, " # B039-LIFECYCLE-ATOMIC-TAKE-MUTANT")
      print
      changed=changed + 1
      next
    }
    { print }
    END { if (changed != 1) exit 42 }
  ' "$SELFTEST_SCRIPT" >"$destination"
}

selftest_run_lifecycle_wait_boundary_control() {
  local child_pid=''
  local child_status=0

  if [[ -n "${BUBBLES_MUTATION_RUNNER_ROOT_RECORD:-}" ]]; then
    printf '%s\n' "$TMPDIR" >"$BUBBLES_MUTATION_RUNNER_ROOT_RECORD"
  fi
  SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS=0
  SELFTEST_LIFECYCLE_SIGNAL_MODE=normal
  SELFTEST_LIFECYCLE_BOUNDARY_OBSERVED=0
  /bin/sh -c 'exit 0' &
  child_pid=$!
  SELFTEST_LIFECYCLE_PID="$child_pid"
  SELFTEST_LIFECYCLE_BOUNDARY_CONTROL=1
  if selftest_wait_exact_child "$child_pid"; then
    child_status=0
  else
    child_status=$?
  fi
  SELFTEST_LIFECYCLE_BOUNDARY_CONTROL=0
  printf 'LIFECYCLE_WAIT_BOUNDARY_SUMMARY status=%s observed=%s cleanupSignals=%s handleState=%s\n' \
    "$child_status" "$SELFTEST_LIFECYCLE_BOUNDARY_OBSERVED" \
    "$SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS" \
    "$([[ -z "$SELFTEST_LIFECYCLE_PID" ]] && printf absent || printf present)"
  if [[ "$child_status" -ne 0 ]]; then
    return "$child_status"
  fi
  if [[ "$SELFTEST_LIFECYCLE_BOUNDARY_OBSERVED" -eq 1 &&
    "$SELFTEST_LIFECYCLE_CLEANUP_SIGNAL_ATTEMPTS" -eq 0 &&
    -z "$SELFTEST_LIFECYCLE_PID" ]]; then
    printf '%s\n' 'PASS: exact wait-to-clear boundary invokes cleanup with no numeric signal authority'
    return 0
  fi
  printf '%s\n' 'FAIL: exact wait-to-clear boundary control was vacuous or retained cleanup signal authority' >&2
  return 96
}

selftest_check_lifecycle_wait_boundary_negative_control() {
  local mutation_dir="$TMPDIR/lifecycle-wait-boundary-mutant"
  local mutant_script="$mutation_dir/implementation-reality-scan-selftest.sh"
  local output_file="$mutation_dir/mutant.output"
  local internal_tmp_parent="$TMPDIR/lifecycle-wait-boundary-inner"
  local root_record="$mutation_dir/internal-root.record"
  local control_status=0
  local internal_root=''

  mkdir -p "$mutation_dir" "$internal_tmp_parent"
  if ! make_lifecycle_wait_boundary_mutant "$mutant_script"; then
    printf '%s\n' 'FAIL: NEG-B039-LIFECYCLE-WAIT-BOUNDARY copied mutation construction did not match exactly once' >&2
    return 1
  fi
  if selftest_run_mutation_bounded "$output_file" 10 /usr/bin/env \
    TMPDIR="$internal_tmp_parent" BUBBLES_MUTATION_RUNNER_ROOT_RECORD="$root_record" \
    /bin/bash "$mutant_script" \
    --internal-lifecycle-wait-boundary-control b039-lifecycle-wait-boundary-v1; then
    control_status=0
  else
    control_status=$?
  fi
  /bin/cat "$output_file"
  internal_root="$(/bin/cat "$root_record" 2>/dev/null || true)"

  if [[ "$control_status" -eq 97 && "${internal_root##*/}" == tmp.* &&
    ! -e "$internal_root" && "$SELFTEST_MUTATION_SUPERVISOR_PROTOCOL" == BMR1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_COMPLETED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNERSHIP_REGISTERED" -eq 1 &&
    "$SELFTEST_MUTATION_SUPERVISOR_OWNER" == worker &&
    "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" == CHILD_EXIT_NONZERO ]] &&
    /usr/bin/grep -Fq \
      'RED: NEG-B039-LIFECYCLE-WAIT-BOUNDARY cleanup reached stale numeric handle and recorded two suppressed signals' \
      "$output_file"; then
    printf '%s\n' 'RED: NEG-B039-LIFECYCLE-WAIT-BOUNDARY mutantExit=97 rootAbsent=yes cleanupSignalsRecorded=2 osSignalsSent=0 outerProtocol=BMR1'
    printf '%s\n' 'PASS: NEG-B039-LIFECYCLE-WAIT-BOUNDARY copied mutation executes cleanup and catches post-reap numeric authority without signaling it'
    return 0
  fi
  printf 'FAIL: NEG-B039-LIFECYCLE-WAIT-BOUNDARY expected safe exit 97 and absent residue; got exit=%s root=%s diagnostic=%s\n' \
    "$control_status" "${internal_root:-missing}" "$SELFTEST_MUTATION_RUN_DIAGNOSTIC" >&2
  return 1
}

selftest_run_internal_mutation_runner_focused_control() {
  local focused_failures=0

  if selftest_run_lifecycle_wait_boundary_control; then
    printf '%s\n' 'PASS: lifecycle exact-boundary positive control is green'
  else
    printf '%s\n' 'FAIL: lifecycle exact-boundary positive control failed' >&2
    focused_failures=$((focused_failures + 1))
  fi
  if ! selftest_check_lifecycle_wait_boundary_negative_control; then
    focused_failures=$((focused_failures + 1))
  fi
  if selftest_run_internal_mutation_runner_control; then
    printf '%s\n' 'PASS: focused mutation runner positive lifecycle matrix is green'
  else
    printf '%s\n' 'FAIL: focused mutation runner positive lifecycle matrix failed' >&2
    focused_failures=$((focused_failures + 1))
  fi
  if ! selftest_check_mutation_runner_negative_control \
    registration \
    'RED: NEG-B039-MUTATION-REGISTRATION native supervisor omitted ownership registration'; then
    focused_failures=$((focused_failures + 1))
  fi
  if ! selftest_check_mutation_runner_negative_control \
    bound \
    'RED: NEG-B039-MUTATION-BOUND native supervisor exceeded the declared wall'; then
    focused_failures=$((focused_failures + 1))
  fi
  if ! selftest_check_mutation_runner_negative_control \
    guard \
    'RED: NEG-B039-MUTATION-GUARD post-reap deadline made a forbidden signal decision'; then
    focused_failures=$((focused_failures + 1))
  fi
  printf 'MUTATION_RUNNER_COPIED_MUTATION_SUMMARY failures=%s mutations=3 exactConstruction=required setupFailureIsRed=no\n' \
    "$focused_failures"
  [[ "$focused_failures" -eq 0 ]]
}

selftest_cleanup() {
  local status=$?
  builtin trap - EXIT HUP INT TERM
  if declare -F bubbles_python_security_cleanup >/dev/null 2>&1; then
    bubbles_python_security_cleanup || true
  fi
  selftest_stop_exact_child
  /bin/rm -rf "$TMPDIR" "$CLASSIFIER_HELPER_CACHE_DIR"
  if [[ "$SELFTEST_COMPLETED" -ne 1 && "$status" -eq 0 ]]; then
    echo "FAIL: implementation-reality-scan selftest exited before its completion verdict" >&2
    status=1
  fi
  exit "$status"
}

selftest_signal() {
  local status="$1"
  trap - HUP INT TERM
  exit "$status"
}

trap selftest_cleanup EXIT
trap 'selftest_signal 129' HUP
trap 'selftest_signal 130' INT
trap 'selftest_signal 143' TERM

if [[ "$SELFTEST_ENTRYPOINT" == mutation-runner ]]; then
  mutation_runner_status=0
  SELFTEST_COMPLETED=1
  if selftest_run_internal_mutation_runner_control; then
    mutation_runner_status=0
  else
    mutation_runner_status=$?
  fi
  exit "$mutation_runner_status"
fi
if [[ "$SELFTEST_ENTRYPOINT" == mutation-runner-focused ]]; then
  mutation_runner_status=0
  SELFTEST_COMPLETED=1
  if selftest_run_internal_mutation_runner_focused_control; then
    mutation_runner_status=0
  else
    mutation_runner_status=$?
  fi
  exit "$mutation_runner_status"
fi
if [[ "$SELFTEST_ENTRYPOINT" == lifecycle-wait-boundary ]]; then
  lifecycle_boundary_status=0
  SELFTEST_COMPLETED=1
  if selftest_run_lifecycle_wait_boundary_control; then
    lifecycle_boundary_status=0
  else
    lifecycle_boundary_status=$?
  fi
  exit "$lifecycle_boundary_status"
fi

# Private child modes exercise this script's own EXIT/INT/TERM contract. They
# are entered only by the bounded parent regression below. The ready file lives
# outside the child's TMPDIR so the parent can prove that the EXIT cleanup
# removed the exact temporary tree the child created.
if [[ -n "${BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_CHILD_MODE:-}" ]]; then
  if [[ -z "${BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_READY_FILE:-}" ]]; then
    echo "implementation-reality-scan selftest child mode requires a ready file" >&2
    exit 2
  fi
  case "$BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_CHILD_MODE" in
    premature-exit)
      printf '%s\n' "$TMPDIR" >"$BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_READY_FILE"
      exit 0
      ;;
    timeout-exit)
      printf '%s\n' "$TMPDIR" >"$BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_READY_FILE"
      exit 124
      ;;
    interrupt-hold)
      /usr/bin/mkfifo "$TMPDIR/interrupt-hold.fifo"
      exec 9<>"$TMPDIR/interrupt-hold.fifo"
      printf '%s\n' "$TMPDIR" >"$BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_READY_FILE"
      builtin read -r -t 300 _selftest_hold <&9
      ;;
    *)
      echo "implementation-reality-scan selftest child mode is invalid" >&2
      exit 2
      ;;
  esac
fi

# Importing the production classifier must never leave generated state beside
# a security helper. Remove residue from an earlier run before testing, and the
# EXIT trap repeats this cleanup even when an assertion fails.
rm -rf "$CLASSIFIER_HELPER_CACHE_DIR"

# shellcheck source=/dev/null
source "$GUARD_LIB"

# The scan resolves its classifier interpreter through python-env.sh. This
# selftest's skip decision has to be made by the SAME resolver, or it can skip
# coverage the scan would have run.
# shellcheck source=/dev/null
source "$SCRIPT_DIR/python-env.sh"

# Successful producer tests need a real interpreter independently of the
# managed-path trust fixture used by the scanner. Resolve it through the
# production API, then pass the exact resolved executable to that wrapper. A
# machine with no runnable Python cannot execute this required contract and
# therefore fails the selftest prerequisite instead of reporting a skip.
CLASSIFIER_TEST_PYTHON="${BUBBLES_SELFTEST_REAL_PYTHON:-}"
if [[ -n "$CLASSIFIER_TEST_PYTHON" && -x "$CLASSIFIER_TEST_PYTHON" ]] &&
  bubbles_python_runs "$CLASSIFIER_TEST_PYTHON"; then
  :
elif bubbles_python_resolve_runnable >/dev/null; then
  CLASSIFIER_TEST_PYTHON="$BUBBLES_PYTHON_RUNNABLE"
else
  printf 'implementation-reality-scan selftest prerequisite failed: runnable Python required: %s\n' \
    "$BUBBLES_PYTHON_RUNNABLE_REASON" >&2
  exit 2
fi

failures=0
skips=0
B039_AUTHORIZED_CLASSIFIER_MUTATION_VERIFIED=0
RUN_OUTPUT=""
RUN_STATUS=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

skip() {
  echo "SKIP: $1"
  skips=$((skips + 1))
}

selftest_fixture_absolute_path_safe() {
  local value="${1:-}"
  [[ "$value" == /* ]] || return 1
  [[ ${#value} -le 4096 ]] || return 1
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* && "$value" != *$'\t'* ]] || return 1
  return 0
}

assert_legacy_target_environment_is_inert() {
  local output_file="$TMPDIR/legacy-target-dispatch-probe.output"
  local probe_status=0

  if bubbles_run_with_timeout 10 env \
    BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_TARGET=authority-bypass \
    "$BASH" "$SELFTEST_SCRIPT" --internal-dispatch-probe >"$output_file" 2>&1 </dev/null; then
    probe_status=0
  else
    probe_status=$?
  fi
  /bin/cat "$output_file"
  if [[ "$probe_status" -eq 0 ]] &&
    /usr/bin/grep -Fqx 'IMPLEMENTATION_REALITY_SELFTEST_ZERO_ARGUMENT_ENTRY=FULL_SUITE' "$output_file"; then
    pass "TEST-B039-001 legacy SELFTEST_TARGET cannot select an ambient subset"
  else
    fail "TEST-B039-001 legacy SELFTEST_TARGET changed dispatch (exit=$probe_status)"
  fi
}

echo "Scenario: TEST-B039-001 inherited subset selectors cannot replace the full-suite entrypoint."
assert_legacy_target_environment_is_inert

assert_selftest_lifecycle_fails_closed() {
  local mode="$1"
  local signal_name="$2"
  local expected_status="$3"
  local label="$4"
  local ready_fifo="$TMPDIR/lifecycle-$mode-$signal_name.ready.fifo"
  local output_file="$TMPDIR/lifecycle-$mode-$signal_name.log"
  local child_pid=""
  local child_tmp=""
  local child_status=0
  local read_status=0

  /usr/bin/mkfifo "$ready_fifo"
  BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_CHILD_MODE="$mode" \
    BUBBLES_IMPLEMENTATION_REALITY_SELFTEST_READY_FILE="$ready_fifo" \
    /bin/bash "$SELFTEST_SCRIPT" >"$output_file" 2>&1 </dev/null &
  child_pid=$!
  SELFTEST_LIFECYCLE_PID="$child_pid"
  exec 6<"$ready_fifo"
  if builtin read -r -t 10 child_tmp <&6; then read_status=0; else read_status=$?; fi
  exec 6>&-
  if [[ "$read_status" -ne 0 || -z "$child_tmp" ]]; then
    selftest_stop_exact_child
    fail "$label reaches its bounded ready point"
    return
  fi

  if [[ ! "$SELFTEST_LIFECYCLE_PID" =~ ^[1-9][0-9]*$ ||
    "$SELFTEST_LIFECYCLE_PID" != "$child_pid" ]]; then
    fail "$label exact direct-child registration is invalid before builtin wait"
    SELFTEST_LIFECYCLE_PID="$child_pid"
    selftest_stop_exact_child
    return
  fi
  if [[ "$signal_name" != "NONE" ]]; then
    builtin kill -"$signal_name" "$child_pid" 2>/dev/null || true
  fi
  if selftest_wait_exact_child "$child_pid"; then child_status=0; else child_status=$?; fi

  if [[ "$child_status" -eq "$expected_status" ]]; then
    pass "$label preserves fatal exit $expected_status"
  else
    fail "$label expected exit $expected_status, got wait=$child_status"
  fi
  if [[ -n "$child_tmp" && ! -e "$child_tmp" ]]; then
    pass "$label removes its temporary tree"
  else
    fail "$label removes its temporary tree (still present: $child_tmp)"
    [[ -z "$child_tmp" ]] || rm -rf "$child_tmp"
  fi
  if /usr/bin/grep -Fq 'implementation-reality-scan selftest passed' "$output_file"; then
    fail "$label must not emit a success summary"
  else
    pass "$label emits no success summary"
  fi
}

echo "Scenario: premature and interrupted selftest exits fail closed while cleaning up."
assert_selftest_lifecycle_fails_closed premature-exit NONE 1 "Premature EXIT"
assert_selftest_lifecycle_fails_closed timeout-exit NONE 124 "Timeout exit"
assert_selftest_lifecycle_fails_closed interrupt-hold HUP 129 "HUP interruption"
assert_selftest_lifecycle_fails_closed interrupt-hold TERM 143 "TERM interruption"

echo "Scenario: native mutation-runner focused lifecycle and copied negative controls stay green."
if selftest_run_internal_mutation_runner_focused_control; then
  pass "Mutation runner focused native-supervisor suite covers lifecycle, deadline edge, and copied mutations"
else
  fail "Mutation runner focused native-supervisor suite failed"
fi

# Is the Scan 2B classifier's interpreter USABLE -- not merely present?
#
# Those are different questions and on macOS they diverge. /usr/bin/python3 is a
# shim that dispatches through the ACTIVE developer directory, so when Xcode.app
# is selected and its licence has not been accepted the shim resolves (satisfying
# `command -v python3`) and then exits 69 without executing a line. The scan then
# fails closed to SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED for every candidate
# line -- correct behaviour -- and assertions on exact classifier tuples fail
# while naming the code under scan, when the real subject is the absent
# prerequisite.
#
# This asks the SAME resolver the scan asks: python-env.sh's authenticated
# root-protected-native-python-v1 trust contract. BUBBLES_PYTHON, managed-venv
# locators, and caller PATH entries cannot become classifier authority.
CLASSIFIER_UNAVAILABLE_REASON=""
CLASSIFIER_REMEDIATION=""

sensitive_storage_classifier_usable() {
  CLASSIFIER_UNAVAILABLE_REASON=""
  CLASSIFIER_REMEDIATION=""

  # Not a command substitution: the resolver's numeric status and closed
  # diagnostic globals must survive for the skip contract below.
  if bubbles_python_resolve_security_runtime; then
    return 0
  fi

  CLASSIFIER_UNAVAILABLE_REASON="status=$BUBBLES_PYTHON_SECURITY_STATUS diagnostic=$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC rejection=$BUBBLES_PYTHON_SECURITY_REJECTION candidates=$BUBBLES_PYTHON_SECURITY_CANDIDATE_COUNT trust=$BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT provenance=$BUBBLES_PYTHON_SECURITY_PROVENANCE"
  case "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" in
    XCODE_LICENSE_UNACCEPTED)
      CLASSIFIER_REMEDIATION="run 'sudo xcodebuild -license accept', select accepted Command Line Tools, or set the validated Command Line Tools DEVELOPER_DIR for one invocation"
      ;;
    *)
      CLASSIFIER_REMEDIATION="install a root-protected native Python 3.9 or newer with protected import roots"
      ;;
  esac
  return 1
}

assert_output_contains() {
  local expected="$1"
  local label="$2"
  if grep -Fq -- "$expected" <<<"$RUN_OUTPUT"; then
    pass "$label"
  else
    fail "$label (missing: $expected)"
  fi
}

assert_output_not_contains() {
  local forbidden="$1"
  local label="$2"
  if grep -Fq -- "$forbidden" <<<"$RUN_OUTPUT"; then
    fail "$label (unexpected: $forbidden)"
  else
    pass "$label"
  fi
}

run_scan_in_repo() {
  local repo_root="$1"
  local feature_dir="$2"
  local output_file="$TMPDIR/run-scan-in-repo.txt"
  RUN_OUTPUT=""
  RUN_STATUS=0
  if (
    cd "$repo_root" || exit 2
    export DEVELOPER_DIR=/Library/Developer/CommandLineTools
    bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose
  ) >"$output_file" 2>&1; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
}

run_scan_in_repo_with_home() {
  local repo_root="$1"
  local feature_dir="$2"
  local python_home="$3"
  local output_file="$TMPDIR/run-scan-with-home.txt"
  local started_at=$SECONDS
  local elapsed_seconds=0
  RUN_OUTPUT=""
  RUN_STATUS=0
  if (
    cd "$repo_root" || exit 2
    export BUBBLES_PYTHON=""
    export BUBBLES_PYTHON_HOME="$python_home"
    export BUBBLES_SELFTEST_REAL_PYTHON="$CLASSIFIER_TEST_PYTHON"
    export DEVELOPER_DIR=/Library/Developer/CommandLineTools
    bubbles_run_with_timeout 180 /bin/bash "$SCAN_SCRIPT" "$feature_dir" --verbose
  ) >"$output_file" 2>&1; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
  elapsed_seconds=$((SECONDS - started_at))
  printf 'SELFTEST_SCAN_METRIC fixture=%s status=%s elapsed_seconds=%s\n' \
    "${python_home##*/}" "$RUN_STATUS" "$elapsed_seconds"
}

run_scan_in_repo_with_hostile_env() {
  local repo_root="$1"
  local feature_dir="$2"
  local python_home="$3"
  local hostile_path="$4"
  local marker_file="$5"
  local output_file="$TMPDIR/run-scan-with-hostile-env.txt"
  local started_at=$SECONDS
  local elapsed_seconds=0
  RUN_OUTPUT=""
  RUN_STATUS=0
  if (
    cd "$repo_root" || exit 2
    unset BUBBLES_PYTHON BUBBLES_PYTHON_HOME XDG_CACHE_HOME HOME
    export PATH="$hostile_path:${PATH:-}"
    export BUBBLES_PYTHON_HOME="$python_home"
    export BUBBLES_SELFTEST_REAL_PYTHON="$CLASSIFIER_TEST_PYTHON"
    export BUBBLES_HOSTILE_ENV_MARKER="$marker_file"
    export DEVELOPER_DIR=/Library/Developer/CommandLineTools
    bubbles_run_with_timeout 180 /bin/bash "$SCAN_SCRIPT" "$feature_dir" --verbose
  ) >"$output_file" 2>&1; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
  elapsed_seconds=$((SECONDS - started_at))
  printf 'SELFTEST_SCAN_METRIC fixture=hostile-env status=%s elapsed_seconds=%s\n' \
    "$RUN_STATUS" "$elapsed_seconds"
}

run_scan_in_repo_without_locator() {
  local repo_root="$1"
  local feature_dir="$2"
  local output_file="$TMPDIR/run-scan-without-locator.txt"
  RUN_OUTPUT=""
  RUN_STATUS=0
  if (
    cd "$repo_root" || exit 2
    unset BUBBLES_PYTHON BUBBLES_PYTHON_HOME XDG_CACHE_HOME HOME
    export DEVELOPER_DIR=/Library/Developer/CommandLineTools
    bubbles_run_with_timeout 180 /bin/bash "$SCAN_SCRIPT" "$feature_dir" --verbose
  ) >"$output_file" 2>&1; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
}

run_expect_success() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-success.txt"

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    pass "$label"
  else
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
  fi
}

run_expect_zero_files_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-zero-files.txt"
  local status=0

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
    return
  else
    status=$?
    output="$(cat "$output_file")"
    echo "$output"
  fi

  if [[ "$status" -eq 1 ]] && grep -Fq 'ZERO_FILES_RESOLVED' <<< "$output"; then
    pass "$label"
  else
    fail "$label"
  fi
}

run_expect_fake_integration_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local output_file="$TMPDIR/run-expect-fake-integration.txt"
  local status=0

  if bubbles_run_with_timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose >"$output_file" 2>&1; then
    output="$(cat "$output_file")"
    echo "$output"
    fail "$label"
    return
  else
    status=$?
    output="$(cat "$output_file")"
    echo "$output"
  fi

  if [[ "$status" -eq 1 ]] && grep -Fq 'FAKE_INTEGRATION' <<< "$output"; then
    pass "$label"
  else
    fail "$label"
  fi
}

create_shell_heavy_fixture() {
  local feature_dir="$FIXTURE_ROOT/shell-heavy-feature"
  mkdir -p "$feature_dir/scripts" "$feature_dir/config" "$feature_dir/docs"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Shell Heavy Fixture

## Scope 1: Inventory Discovery

### Implementation Files

- \`$feature_dir/scripts/validate.sh\`
- \`$feature_dir/config/service.yaml\`
- \`$feature_dir/config/service.yml\`
- \`$feature_dir/config/schema.json\`
- \`$feature_dir/docs/operator.md\`
EOF

  cat > "$feature_dir/scripts/validate.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "fixture validation complete"
EOF

  cat > "$feature_dir/config/service.yaml" <<'EOF'
service: shell-heavy
mode: explicit
EOF

  cat > "$feature_dir/config/service.yml" <<'EOF'
service: shell-heavy-yml
mode: explicit
EOF

  cat > "$feature_dir/config/schema.json" <<'EOF'
{"service":"shell-heavy","mode":"explicit"}
EOF

  cat > "$feature_dir/docs/operator.md" <<'EOF'
# Operator Notes

This fixture proves non-code implementation inventories are still resolved.
EOF
}

create_missing_inventory_fixture() {
  local feature_dir="$FIXTURE_ROOT/missing-inventory-feature"
  mkdir -p "$feature_dir"

  cat > "$feature_dir/scopes.md" <<'EOF'
# Scopes: Missing Inventory Fixture

## Scope 1: Missing Inventory

This scope intentionally has no backtick-wrapped implementation file paths.
EOF
}

create_go_connector_package_fixture() {
  local feature_dir="$FIXTURE_ROOT/go-connector-package-feature"
  local package_dir="$feature_dir/internal/connector/honest"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Go Connector Package Fixture

## Scope 1: Honest Connector Helpers

### Implementation Files

- \`$package_dir/client.go\`
- \`$package_dir/capability.go\`
- \`$package_dir/normalizer.go\`
EOF

  cat > "$package_dir/client.go" <<'EOF'
package honest

import (
	"context"
	"net/http"
)

type Client struct {
	httpClient *http.Client
}

func (c *Client) Fetch(ctx context.Context, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
EOF

  cat > "$package_dir/capability.go" <<'EOF'
package honest

import "fmt"

func ValidateCapability(version string) error {
	if version == "" {
		return fmt.Errorf("capability version is required")
	}
	return nil
}
EOF

  cat > "$package_dir/normalizer.go" <<'EOF'
package honest

type Artifact struct {
	ID string
}

type DegradedDiagnostic struct {
	Reason string
}

func Normalize(raw string) (*Artifact, *DegradedDiagnostic) {
	if raw == "" {
		return nil, &DegradedDiagnostic{Reason: "missing trusted artifact"}
	}
	return &Artifact{ID: raw}, nil
}
EOF
}

create_fake_connector_fixture() {
  local feature_dir="$FIXTURE_ROOT/fake-connector-feature"
  local package_dir="$feature_dir/internal/connector/external"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Fake Connector Fixture

## Scope 1: No-op Connector

### Implementation Files

- \`$package_dir/connector.go\`
EOF

  cat > "$package_dir/connector.go" <<'EOF'
package external

type Connector struct{}

func (c *Connector) Sync() error {
	return nil
}
EOF
}

# NEGATIVE (must NOT flag): legitimate OpenTelemetry no-op tracer fallback plus
# closed-vocabulary span-status literals ("noop"). These are observability
# constructs, not faked upstream integration. Proves the Scan 1D telemetry
# refinement exempts them. Mirrors the real assistant_adapter package shape: a
# sibling with a real external call so the package carries an external signal.
create_telemetry_noop_adapter_fixture() {
  local feature_dir="$FIXTURE_ROOT/telemetry-noop-adapter-feature"
  local package_dir="$feature_dir/internal/adapter/telemetry"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Telemetry No-op Adapter Fixture

## Scope 1: OpenTelemetry no-op tracer fallback + closed-vocab span status

### Implementation Files

- \`$package_dir/tracer_fallback.go\`
EOF

  # Real upstream transport lives in a sibling (unlisted) so the package has a
  # genuine external-call signal, exactly like the real adapter package.
  cat > "$package_dir/sender.go" <<'EOF'
package telemetry

import "net/http"

func send(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
EOF

  # The scanned file: OTel no-op tracer fallback + "noop" span-status literals,
  # mirroring internal/telegram/assistant_adapter/adapter.go lines 147/152/154/
  # 214/338. With the Scan 1D refinement these MUST NOT be flagged.
  cat > "$package_dir/tracer_fallback.go" <<'EOF'
package telemetry

// buildTracer returns the real tracer, or a no-op tracer fallback when a
// caller omits one so span-emission sites stay unconditional.
func buildTracer() (Tracer, error) {
	noopTr, _, err := tracing.NewTracer(ctx, tracing.Config{Enabled: false, ServiceName: "svc"})
	if err != nil {
		return nil, fmt.Errorf("build noop tracer fallback: %w", err)
	}
	tr = noopTr
	return tr, nil
}

// endTranslate ends the root span with the closed-vocabulary status literal
// "noop" (contract: status is one of ok|error|noop).
func endTranslate(span Span) {
	tracing.EndSpan(span, "noop", "not_assistant_message")
	rootStatus := "noop"
	_ = rootStatus
}
EOF
}

# ADVERSARIAL (must STILL flag): a genuinely faked no-op integration. 'Relay'
# is supposed to reach an upstream bus, but the body is a bare, non-telemetry,
# non-quoted no-op with no external call. The Scan 1D refinement MUST NOT exempt
# this — proves the exclusion opens no hole for real fakes.
create_fake_noop_integration_fixture() {
  local feature_dir="$FIXTURE_ROOT/fake-noop-integration-feature"
  local package_dir="$feature_dir/internal/connector/relay"
  mkdir -p "$package_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scopes: Fake No-op Integration Fixture

## Scope 1: Bare no-op integration (must STILL flag)

### Implementation Files

- \`$package_dir/relay.go\`
EOF

  cat > "$package_dir/relay.go" <<'EOF'
package relay

// Relay is supposed to reach the upstream notification bus.
func Relay(payload string) error {
	outcome := noop
	_ = outcome
	return nil
}

func noop() {}
EOF
}

create_sensitive_storage_fixture() {
  SENSITIVE_REPO="$FIXTURE_ROOT/sensitive-storage-repo"
  SENSITIVE_FEATURE="$SENSITIVE_REPO/specs/001-sensitive-storage"
  SENSITIVE_SOURCE="$SENSITIVE_REPO/src/provider-client.js"
  SENSITIVE_DART_SOURCE="$SENSITIVE_REPO/src/provider-preferences.dart"
  SENSITIVE_CONFIG="$SENSITIVE_REPO/.github/bubbles-project.yaml"
  mkdir -p "$SENSITIVE_FEATURE" "$(dirname "$SENSITIVE_SOURCE")" "$(dirname "$SENSITIVE_CONFIG")"

  cat > "$SENSITIVE_FEATURE/scopes.md" <<'EOF'
# Scope 1: Sensitive Storage Selftest

### Implementation Files

- `src/provider-client.js`
- `src/provider-preferences.dart`
EOF

  cat > "$SENSITIVE_SOURCE" <<'EOF'
const KEY = "marketProvider:twelvedata:apiKey";
const KEY_ALIAS = KEY;
const UNKNOWN_KEY = "marketProvider:unknown-vendor:apiKey";
const CACHE_KEY = "marketCache:latest";
localStorage.setItem(KEY_ALIAS, providerCredential);
sessionStorage.setItem(KEY, providerCredential);
sessionStorage.setItem(UNKNOWN_KEY, providerCredential);
sessionStorage.setItem(`marketProvider:${provider}:apiKey`, providerCredential);
localStorage.setItem(KEY, providerCredential);
sessionStorage.setItem(KEY, authBearerToken);
localStorage.setItem(CACHE_KEY, marketSnapshot); // auth token and payment secret are comments only
localStorage.removeItem("legacyAuthToken");
const beforeScrub = { apiKey: providerCredential, price: 42 };
localStorage.setItem("marketCache:before", JSON.stringify(beforeScrub));
const afterScrub = { apiKey: providerCredential, authToken: authBearerToken, price: 42 };
delete afterScrub.apiKey;
delete afterScrub.authToken;
localStorage.setItem("marketCache:after", JSON.stringify(afterScrub));
indexedDB.open("authCredentialDatabase");
SharedPreferences.putString("refreshToken", refreshToken);
AsyncStorage.multiSet("paymentCard", paymentCardNumber);
const transaction = providerDatabase.transaction("credentials", "readwrite");
const credentialStore = transaction.objectStore("credentials");
credentialStore.put(providerCredential, KEY);
EOF

  cat > "$SENSITIVE_DART_SOURCE" <<'EOF'
Future<void> persistProviderCredential(
  SharedPreferences preferences,
  String providerCredential,
) async {
  await preferences.setString(
    "marketProvider:twelvedata:apiKey",
    providerCredential,
  );
}
EOF

  write_sensitive_valid_config
}

write_sensitive_valid_config() {
  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
}

create_classifier_protocol_fixture() {
  PROTOCOL_REPO="$FIXTURE_ROOT/classifier-protocol-repo"
  PROTOCOL_FEATURE="$PROTOCOL_REPO/specs/001-classifier-protocol"
  PROTOCOL_SOURCE="$PROTOCOL_REPO/src/view.js"
  mkdir -p "$PROTOCOL_FEATURE" "$(dirname "$PROTOCOL_SOURCE")"

  cat > "$PROTOCOL_FEATURE/scopes.md" <<'EOF'
# Scope 1: Classifier Protocol Selftest

### Implementation Files

- `src/view.js`
EOF

  write_protocol_boundary_source
}

write_protocol_boundary_source() {
  cat > "$PROTOCOL_SOURCE" <<'EOF'
export function cacheSnapshot(snapshot) {
  localStorage.setItem("marketCache:latest", JSON.stringify(snapshot));
}
EOF
}

write_protocol_zero_source() {
  cat > "$PROTOCOL_SOURCE" <<'EOF'
export function formatLabel(value) {
  return String(value);
}
EOF
}

write_protocol_finding_source() {
  cat > "$PROTOCOL_SOURCE" <<'EOF'
export function persistCredential(providerCredential) {
  localStorage.setItem("marketProvider:twelvedata:apiKey", providerCredential);
}
EOF
}

# A managed-interpreter fixture is production input, not a substitute parser.
# Every assertion below observes implementation-reality-scan.sh. The fixture
# only controls what the executable does at the probe/helper trust boundary.
# The real-forward mode execs the independently resolved Python with every
# production argument unchanged. Mutation modes alter only a copied driver
# string in the temporary fixture so the assertions prove they have teeth.
make_classifier_python_fixture() {
  local home="$1"
  local mode="$2"
  local path="$home/bin/python3"
  mkdir -p "$(dirname "$path")"
  cat > "$path" <<EOF
#!/bin/bash
mode='$mode'
real_python="\${BUBBLES_SELFTEST_REAL_PYTHON:-}"
if [[ "\${1:-}" == "-B" ]]; then
  shift
fi
if [[ "\${1:-}" == "-c" && "\${2:-}" == *bubbles-python-runs* ]]; then
  case "\$mode" in
    forge-all)
      printf '%s\n' 'forged managed runtime executed' >"\${BUBBLES_SCOPE2_FORGED_MARKER:?marker required}"
      printf 'COMPLETE\tSCS1\t1\n'
      exit 0
      ;;
    probe-silent) exit 0 ;;
    probe-malformed) printf '%s' 'not-the-probe-protocol'; exit 0 ;;
    probe-hang) exec /bin/sleep 300 ;;
    xcode) printf '%s\n' 'You have not agreed to the Xcode license agreements. SECRET_MUST_NOT_LEAK' >&2; exit 69 ;;
    *) printf '%s' 'bubbles-python-runs'; exit 0 ;;
  esac
fi
case "\$mode" in
  real-forward)
    [[ -n "\$real_python" && -x "\$real_python" ]] || exit 75
    exec "\$real_python" "\$@"
    ;;
  mutate-completion)
    [[ -n "\$real_python" && -x "\$real_python" ]] || exit 75
    if [[ "\${1:-}" == "-c" && "\${2:-}" == *COMPLETE* && "\${2:-}" == *SCS1* ]]; then
      mutation_prefix='import builtins
_bubbles_original_print = builtins.print
def _bubbles_mutated_print(*args, **kwargs):
    if args and isinstance(args[0], str) and args[0].startswith("COMPLETE\\tSCS1\\t"):
        return None
    return _bubbles_original_print(*args, **kwargs)
print = _bubbles_mutated_print
'
      mutated_driver="\$mutation_prefix
\$2"
      shift 2
      exec "\$real_python" -c "\$mutated_driver" "\$@"
    fi
    exec "\$real_python" "\$@"
    ;;
  mutate-classification)
    [[ -n "\$real_python" && -x "\$real_python" ]] || exit 75
    if [[ "\${1:-}" == "-c" && "\${2:-}" == *'for finding in module.analyze_file(source_path, repo_root, approvals):'* ]]; then
      original_driver="\$2"
      classification_line='for finding in module.analyze_file(source_path, repo_root, approvals):'
      mutated_driver="\${original_driver//\$classification_line/for finding in ():}"
      [[ "\$mutated_driver" != "\$original_driver" ]] || exit 76
      shift 2
      exec "\$real_python" -c "\$mutated_driver" "\$@"
    fi
    exec "\$real_python" "\$@"
    ;;
esac
case "\$mode" in
  helper-hang) exec /bin/sleep 300 ;;
  helper-failure)
    printf '%s\n' 'SECRET_MUST_NOT_LEAK helper failure bytes' >&2
    exit 73
    ;;
  helper-empty) exit 0 ;;
  helper-malformed) printf '%s\n' 'NOT_A_CLASSIFIER_RECORD SECRET_MUST_NOT_LEAK' ;;
  helper-missing-completion)
    printf 'FINDING\tsrc/view.js\t2\tDURABLE_CREDENTIAL_STORAGE\tlocalStorage\tpersist\tmarketProvider:twelvedata:apiKey\ttwelvedata\tabsent\n'
    ;;
  helper-duplicate-completion)
    printf 'COMPLETE\tSCS1\t1\nCOMPLETE\tSCS1\t1\n'
    ;;
  helper-count-mismatch) printf 'COMPLETE\tSCS1\t2\n' ;;
  *) exit 74 ;;
esac
EOF
  chmod +x "$path"
}

assert_classifier_boundary_failure() {
  local mode="$1"
  local expected_status="$2"
  local expected_diagnostic="$3"
  local probe_timeout_seconds="${4:-30}"
  local home="$FIXTURE_ROOT/classifier-python-$mode"
  local observed_diagnostic=""
  make_classifier_python_fixture "$home" "$mode"
  BUBBLES_SELFTEST_PROBE_TIMEOUT_SECONDS="$probe_timeout_seconds" \
    run_scan_in_repo_with_home "$PROTOCOL_REPO" "$PROTOCOL_FEATURE" "$home"
  if [[ "$RUN_STATUS" -eq 1 ]]; then
    pass "$mode fails closed"
  else
    fail "$mode fails closed (expected scanner exit 1, got $RUN_STATUS)"
  fi
  if grep -Fq -- "status=$expected_status diagnostic=$expected_diagnostic" <<<"$RUN_OUTPUT"; then
    pass "$mode reports bounded numeric status and closed diagnostic"
  else
    observed_diagnostic="$(grep -F 'sensitive-storage classifier' <<<"$RUN_OUTPUT" || true)"
    fail "$mode expected status=$expected_status diagnostic=$expected_diagnostic; observed: $observed_diagnostic"
  fi
  assert_output_contains "reason=SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED" "$mode produces unresolved source findings"
  assert_output_not_contains "SECRET_MUST_NOT_LEAK" "$mode never replays executable output"
}

assert_classifier_boundary_failure_with_timeout() {
  local mode="$1"
  local expected_status="$2"
  local expected_diagnostic="$3"
  local timeout_seconds="$4"
  local home="$FIXTURE_ROOT/classifier-python-$mode-timeout-$timeout_seconds"
  local observed_diagnostic=""
  make_classifier_python_fixture "$home" "$mode"
  BUBBLES_SELFTEST_CLASSIFIER_TIMEOUT_SECONDS="$timeout_seconds" \
    run_scan_in_repo_with_home "$PROTOCOL_REPO" "$PROTOCOL_FEATURE" "$home"
  if [[ "$RUN_STATUS" -eq 1 ]]; then
    pass "$mode timeout control fails closed"
  else
    fail "$mode timeout control fails closed (expected scanner exit 1, got $RUN_STATUS)"
  fi
  if grep -Fq -- "status=$expected_status diagnostic=$expected_diagnostic" <<<"$RUN_OUTPUT"; then
    pass "$mode timeout control reports bounded numeric status and closed diagnostic"
  else
    observed_diagnostic="$(grep -F 'sensitive-storage classifier' <<<"$RUN_OUTPUT" || true)"
    fail "$mode timeout control expected status=$expected_status diagnostic=$expected_diagnostic; observed: $observed_diagnostic"
  fi
  assert_output_contains "reason=SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED" "$mode timeout control produces unresolved source findings"
  assert_output_not_contains "SECRET_MUST_NOT_LEAK" "$mode timeout control never replays executable output"
}

assert_classifier_helper_cache_absent() {
  local label="$1"
  if [[ -e "$CLASSIFIER_HELPER_CACHE_DIR" ]]; then
    fail "$label (unexpected helper cache: $CLASSIFIER_HELPER_CACHE_DIR)"
  else
    pass "$label"
  fi
}

real_finding_contract_holds() {
  [[ "$RUN_STATUS" -eq 1 ]] || return 1
  grep -Fq -- "classifier protocol complete: version=SCS1 scanned=1 findings=1" <<<"$RUN_OUTPUT" || return 1
  grep -Fq -- "reason=DURABLE_CREDENTIAL_STORAGE storage=localStorage operation=persist key=marketProvider:twelvedata:apiKey provider=twelvedata configMatch=absent" <<<"$RUN_OUTPUT" || return 1
  return 0
}

assert_sensitive_invalid_config() {
  local label="$1"
  run_scan_in_repo "$SENSITIVE_REPO" "$SENSITIVE_FEATURE"
  if [[ "$RUN_STATUS" -eq 1 ]]; then
    pass "$label blocks"
  else
    fail "$label blocks (expected exit 1, got $RUN_STATUS)"
  fi
  assert_output_contains "reason=SENSITIVE_STORAGE_CONFIG_INVALID" "$label reports config integrity"
}

create_shell_heavy_fixture
create_missing_inventory_fixture
create_go_connector_package_fixture
create_fake_connector_fixture
create_telemetry_noop_adapter_fixture
create_fake_noop_integration_fixture
create_sensitive_storage_fixture
create_classifier_protocol_fixture

echo "Scenario: SCN-B039-005 caller-owned managed/PATH Python cannot authorize a clean Scan 2B verdict."
write_protocol_finding_source
scope2_forged_home="$FIXTURE_ROOT/classifier-python-forge-all"
scope2_forged_marker="$FIXTURE_ROOT/classifier-python-forge-all.marker"
make_classifier_python_fixture "$scope2_forged_home" forge-all
export BUBBLES_SCOPE2_FORGED_MARKER="$scope2_forged_marker"
run_scan_in_repo_with_home "$PROTOCOL_REPO" "$PROTOCOL_FEATURE" "$scope2_forged_home"
unset BUBBLES_SCOPE2_FORGED_MARKER
if [[ "$RUN_STATUS" -eq 1 ]]; then
  pass "SCN-B039-005 forged managed runtime cannot earn a clean scanner verdict"
else
  fail "SCN-B039-005 forged managed runtime earned scanner exit $RUN_STATUS"
fi
if [[ ! -e "$scope2_forged_marker" ]]; then
  pass "SCN-B039-005 forged managed runtime marker remains absent"
else
  fail "SCN-B039-005 forged managed runtime executed before authentication"
fi
assert_output_contains "trust=root-protected-native-python-v1" "SCN-B039-005 scanner reports the root-protected trust contract"
assert_output_not_contains "classifier protocol complete: version=SCS1 scanned=1 findings=0" "SCN-B039-005 forged clean SCS1 cannot satisfy Scan 2B"

echo "Scenario: SCN-B039-005 copied authentication bypass turns the forged-runtime assertions red."
authority_live_hashes="$(/usr/bin/shasum -a 256 "$SCRIPT_DIR/python-env.sh" "$SCAN_SCRIPT" "$SCRIPT_DIR/fun-mode.sh" "$SCRIPT_DIR/guards/sensitive-client-storage-scan.py")"
authority_root="$FIXTURE_ROOT/authority-bypass-mutation"
authority_scripts="$authority_root/bubbles/scripts"
authority_env="$authority_scripts/python-env.sh"
authority_scanner="$authority_scripts/implementation-reality-scan.sh"
authority_fake_bin="$authority_root/caller-bin"
authority_fake="$authority_fake_bin/python3"
authority_marker="$authority_root/caller-runtime-executed.marker"
authority_untrusted_developer="$authority_root/untrusted-developer"
authority_trace="$authority_root/authentication-bypass.trace"
authority_output="$TMPDIR/authority-bypass-mutation.output"
authority_mutation_status=0
authority_mutation_preconditions=0
authority_source_hash=""
authority_mutant_hash=""
authority_auth_marker_count=0
authority_candidate_marker_count=0
authority_fixture_path_status=0
authority_fake_literal=""
authority_marker_literal=""
authority_trace_literal=""
mkdir -p "$authority_scripts/guards" "$authority_fake_bin" "$authority_untrusted_developer"
/bin/cp "$SCRIPT_DIR/python-env.sh" "$authority_env"
/bin/cp "$GUARD_LIB" "$authority_scripts/guard-lib.sh"
/bin/cp "$SCAN_SCRIPT" "$authority_scanner"
/bin/cp "$SCRIPT_DIR/fun-mode.sh" "$authority_scripts/fun-mode.sh"
/bin/cp "$SCRIPT_DIR/guards/sensitive-client-storage-scan.py" "$authority_scripts/guards/sensitive-client-storage-scan.py"
if selftest_fixture_absolute_path_safe "$authority_fake" &&
  selftest_fixture_absolute_path_safe "$authority_marker" &&
  selftest_fixture_absolute_path_safe "$authority_trace"; then
  printf -v authority_fake_literal '%q' "$authority_fake"
  printf -v authority_marker_literal '%q' "$authority_marker"
  printf -v authority_trace_literal '%q' "$authority_trace"
  pass "SCN-B039-005 authority mutation fixture paths satisfy the closed absolute path grammar"
else
  authority_fixture_path_status=1
  fail "SCN-B039-005 authority mutation fixture paths satisfy the closed absolute path grammar"
fi
authority_source_hash="$(/usr/bin/shasum -a 256 "$SCRIPT_DIR/python-env.sh" | /usr/bin/awk '{print $1}')"
if [[ "$authority_fixture_path_status" -eq 0 ]] &&
  B039_AUTHORITY_FAKE_LITERAL="$authority_fake_literal" \
  B039_AUTHORITY_TRACE_LITERAL="$authority_trace_literal" \
  /usr/bin/awk '
  /^_bubbles_python_security_authenticate_path\(\) \{/ {
    print
    print "  # B039-NEG-AUTHENTICATION-BYPASS"
    print "  if [[ -n \"${BUBBLES_AUTHORITY_BYPASS_CANDIDATE+x}\" || -n \"${BUBBLES_AUTHORITY_BYPASS_TRACE+x}\" ]]; then"
    print "    printf \"%s\\n\" B039_AUTH_ENV_PRESENT >>" ENVIRON["B039_AUTHORITY_TRACE_LITERAL"]
    print "    return 97"
    print "  fi"
    print "  printf \"%s\\n\" B039_AUTH_ENV_ABSENT >>" ENVIRON["B039_AUTHORITY_TRACE_LITERAL"]
    print "  printf \"%s|%s|%s|%s\\n\" B039_AUTH_BYPASS \"${1:-}\" \"${2:-}\" \"${3:-}\" >>" ENVIRON["B039_AUTHORITY_TRACE_LITERAL"]
    print "  BUBBLES_PYTHON_SECURITY_PATH_RESOLVED=\"$1\""
    print "  BUBBLES_PYTHON_SECURITY_PATH_REJECTION=NONE"
    print "  BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC=OK"
    print "  return 0"
    print "}"
    authentication_bypass=authentication_bypass + 1
    skipping=1
    next
  }
  skipping && /^}/ { skipping=0; next }
  !skipping && index($0, "for candidate in \"${candidates[@]}\"; do") {
    print "  # B039-NEG-FORCE-CALLER-CANDIDATE"
    print "  candidates=(" ENVIRON["B039_AUTHORITY_FAKE_LITERAL"] ")"
    print "  BUBBLES_PYTHON_SECURITY_CANDIDATE_COUNT=1"
    candidate_force=candidate_force + 1
  }
  !skipping { print }
  END {
    if (authentication_bypass != 1 || candidate_force != 1 || skipping) exit 42
  }
' "$SCRIPT_DIR/python-env.sh" >"$authority_env"; then
  authority_mutation_status=0
else
  authority_mutation_status=$?
fi
authority_mutant_hash="$(/usr/bin/shasum -a 256 "$authority_env" | /usr/bin/awk '{print $1}')"
authority_auth_marker_count="$(/usr/bin/grep -cF 'B039-NEG-AUTHENTICATION-BYPASS' "$authority_env" || true)"
authority_candidate_marker_count="$(/usr/bin/grep -cF 'B039-NEG-FORCE-CALLER-CANDIDATE' "$authority_env" || true)"
if [[ "$authority_mutation_status" -eq 0 &&
  "$authority_source_hash" != "$authority_mutant_hash" &&
  "$authority_auth_marker_count" -eq 1 &&
  "$authority_candidate_marker_count" -eq 1 ]]; then
  authority_mutation_preconditions=1
  pass "SCN-B039-005 authority mutation changes exactly one authentication branch and forces one caller-owned candidate branch"
else
  fail "SCN-B039-005 authority mutation preconditions failed (status=$authority_mutation_status authMarkers=$authority_auth_marker_count candidateMarkers=$authority_candidate_marker_count sourceHash=$authority_source_hash mutantHash=$authority_mutant_hash)"
fi
cat >"$authority_fake" <<EOF
#!/bin/bash
program=''
previous=''
for argument in "\$@"; do
  if [[ "\$previous" == -c ]]; then program="\$argument"; break; fi
  previous="\$argument"
done
case "\$program" in
  *'RUNTIME\tPYSEC1'*)
    printf 'RUNTIME\tPYSEC1\t3\t9\n'
    printf 'FLAGS\tPYSEC1\t1\t1\t1\t1\n'
    printf 'EXECUTABLE\tPYSEC1\t%s\n' $authority_fake_literal
    printf 'PREFIX\tPYSEC1\tbase\t/\nPREFIX\tPYSEC1\texec\t/\n'
    printf 'PATH\tPYSEC1\t/\nCOMPLETE\tPYSEC1\t1\n'
    ;;
  *'MODULE\tPYMOD1'*)
    for name in ast dataclasses hashlib os pathlib re sys types typing; do
      printf 'MODULE\tPYMOD1\t%s\tbuilt-in\t-\n' "\$name"
    done
    printf 'COMPLETE\tPYMOD1\t9\n'
    ;;
  *)
    printf '%s\n' 'forged caller-owned runtime executed' >$authority_marker_literal
    printf 'COMPLETE\tSCS1\t1\n'
    ;;
esac
exit 0
EOF
chmod +x "$authority_fake"
unset BUBBLES_AUTHORITY_BYPASS_CANDIDATE BUBBLES_AUTHORITY_BYPASS_TRACE
RUN_OUTPUT=""
RUN_STATUS=0
if [[ "$authority_mutation_preconditions" -eq 1 ]]; then
  if selftest_run_mutation_bounded "$authority_output" 180 /usr/bin/env \
    PATH="$authority_fake_bin:/usr/bin:/bin:/usr/sbin:/sbin" \
    DEVELOPER_DIR="$authority_untrusted_developer" \
    "$BASH" -c 'repo_root="$1"; shift; cd "$repo_root" || exit 2; exec "$@"' \
    _ "$PROTOCOL_REPO" "$BASH" "$authority_scanner" "$PROTOCOL_FEATURE" --verbose; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(/bin/cat "$authority_output")"
else
  RUN_STATUS=125
  RUN_OUTPUT='authority-bypass copied scanner was not run because mutation preconditions failed'
fi
if [[ "$SELFTEST_ENTRYPOINT" == authority-bypass ]]; then
  printf '%s\n' '=== SCN-B039-005 authority-bypass copied scanner output ==='
  printf '%s\n' "$RUN_OUTPUT"
  printf '%s\n' '=== SCN-B039-005 authority-bypass authentication trace ==='
  if [[ -f "$authority_trace" ]]; then
    /bin/cat "$authority_trace"
  else
    printf '%s\n' 'trace absent'
  fi
fi
authority_after_hashes="$(/usr/bin/shasum -a 256 "$SCRIPT_DIR/python-env.sh" "$SCAN_SCRIPT" "$SCRIPT_DIR/fun-mode.sh" "$SCRIPT_DIR/guards/sensitive-client-storage-scan.py")"
if [[ -f "$authority_trace" ]] &&
  /usr/bin/grep -Fqx 'B039_AUTH_ENV_ABSENT' "$authority_trace" &&
  ! /usr/bin/grep -Fq 'B039_AUTH_ENV_PRESENT' "$authority_trace"; then
  pass "SCN-B039-005 privileged copied scanner receives no authority-bypass path variables"
else
  fail "SCN-B039-005 privileged copied scanner received an authority-bypass path variable"
fi
if [[ -f "$authority_trace" ]] &&
  /usr/bin/grep -Fq "B039_AUTH_BYPASS|$authority_fake|executable|1" "$authority_trace"; then
  pass "SCN-B039-005 forced caller-owned runtime reaches the copied authentication bypass"
else
  fail "SCN-B039-005 forced caller-owned runtime did not reach the copied authentication bypass"
fi
if [[ "$RUN_STATUS" -eq 0 && -e "$authority_marker" ]] &&
  grep -Fq 'classifier protocol complete: version=SCS1 scanned=1 findings=0' <<<"$RUN_OUTPUT" &&
  grep -Fq 'candidates=1' <<<"$RUN_OUTPUT"; then
  pass "SCN-B039-005 authority-bypass mutation makes forged clean output and marker assertions red"
else
  printf '%s\n' '=== SCN-B039-005 failed authority-bypass copied scanner output ==='
  /bin/cat "$authority_output"
  printf '%s\n' '=== SCN-B039-005 failed authority-bypass authentication trace ==='
  if [[ -f "$authority_trace" ]]; then
    /bin/cat "$authority_trace"
  else
    printf '%s\n' 'trace absent'
  fi
  fail "SCN-B039-005 authority-bypass mutation did not expose the expected compromise (status=$RUN_STATUS marker=$([[ -e "$authority_marker" ]] && echo present || echo absent))"
fi
if [[ "$authority_live_hashes" == "$authority_after_hashes" ]]; then
  pass "SCN-B039-005 authority mutation leaves live production bytes identical"
else
  fail "SCN-B039-005 authority mutation changed live production bytes"
fi
if [[ "$SELFTEST_ENTRYPOINT" == authority-bypass ]]; then
  printf 'implementation-reality-scan authority-bypass control summary: failures=%s skips=%s\n' "$failures" "$skips"
  SELFTEST_COMPLETED=1
  if [[ "$failures" -gt 0 ]]; then
    exit 1
  fi
  exit 0
fi
# The production path must have an authenticated positive control on this host
# before helper-identity tests can claim anything about classifier execution.
if DEVELOPER_DIR=/Library/Developer/CommandLineTools bubbles_python_resolve_security_runtime; then
  SECURITY_RUNTIME="$BUBBLES_PYTHON_SECURITY_RUNTIME"
  pass "SCN-B039-005 authenticated CLT/xcrun runtime is available to scanner tests"
else
  SECURITY_RUNTIME=""
  fail "SCN-B039-005 authenticated CLT/xcrun runtime unavailable: status=$BUBBLES_PYTHON_SECURITY_STATUS diagnostic=$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC rejection=$BUBBLES_PYTHON_SECURITY_REJECTION"
fi

run_copied_scan_with_helper() {
  local mode="$1"
  local payload="$2"
  local expected_digest_mode="$3"
  local copy_root="$FIXTURE_ROOT/helper-$mode"
  local copy_scripts="$copy_root/bubbles/scripts"
  local copy_scanner="$copy_scripts/implementation-reality-scan.sh"
  local copy_helper="$copy_scripts/guards/sensitive-client-storage-scan.py"
  local output_file="$TMPDIR/helper-$mode.output"
  local helper_digest=""
  mkdir -p "$copy_scripts/guards"
  /bin/cp "$SCRIPT_DIR/python-env.sh" "$copy_scripts/python-env.sh"
  /bin/cp "$GUARD_LIB" "$copy_scripts/guard-lib.sh"
  /bin/cp "$SCAN_SCRIPT" "$copy_scanner"
  /bin/cp "$SCRIPT_DIR/guards/sensitive-client-storage-scan.py" "$copy_helper"
  printf '%s\n' "$payload" >>"$copy_helper"
  if [[ "$expected_digest_mode" == updated ]]; then
    helper_digest="$(/usr/bin/shasum -a 256 "$copy_helper" | /usr/bin/awk '{print $1}')"
    /usr/bin/perl -pi -e "s/77a02ff179d529812d75cfa223bef5f9f171a9169dce050ab46fb2f1f0834df3/$helper_digest/g" "$copy_scripts/python-env.sh"
  fi
  RUN_OUTPUT=""
  RUN_STATUS=0
  if selftest_run_mutation_bounded "$output_file" 180 /usr/bin/env \
    DEVELOPER_DIR=/Library/Developer/CommandLineTools \
    /bin/bash -c 'repo_root="$1"; shift; cd "$repo_root" || exit 2; exec "$@"' \
    _ "$PROTOCOL_REPO" /bin/bash "$copy_scanner" "$PROTOCOL_FEATURE" --verbose; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_OUTPUT="$(/bin/cat "$output_file")"
  printf '%s\n' "$RUN_OUTPUT"
}

echo "Scenario: SCN-B039-006 altered helper payload classes fail digest authentication before execution."
for helper_payload_mode in marker subprocess setsid double-fork dynamic-import ctypes eval exec; do
  helper_marker="$TMPDIR/helper-$helper_payload_mode.marker"
  case "$helper_payload_mode" in
    marker) helper_payload="open('$helper_marker', 'w').write('executed')" ;;
    subprocess) helper_payload="__import__('subprocess').run(['/usr/bin/touch', '$helper_marker'])" ;;
    setsid) helper_payload="__import__('os').setsid(); open('$helper_marker', 'w').write('executed')" ;;
    double-fork) helper_payload="(__import__('os').fork() == 0 and __import__('os').fork() == 0 and open('$helper_marker', 'w').write('executed'))" ;;
    dynamic-import) helper_payload="__import__('pathlib').Path('$helper_marker').write_text('executed')" ;;
    ctypes) helper_payload="__import__('ctypes'); open('$helper_marker', 'w').write('executed')" ;;
    eval) helper_payload="eval(\"open('$helper_marker', 'w').write('executed')\")" ;;
    exec) helper_payload="exec(\"open('$helper_marker', 'w').write('executed')\")" ;;
  esac
  run_copied_scan_with_helper "$helper_payload_mode" "$helper_payload" original
  if [[ "$RUN_STATUS" -eq 1 ]] &&
    grep -Fq 'diagnostic=HELPER_DIGEST_MISMATCH' <<<"$RUN_OUTPUT" &&
    [[ ! -e "$helper_marker" ]]; then
    pass "SCN-B039-006 $helper_payload_mode payload is rejected before execution"
  else
    fail "SCN-B039-006 $helper_payload_mode payload status=$RUN_STATUS marker=$([[ -e "$helper_marker" ]] && echo present || echo absent)"
  fi
done

echo "Scenario: SCN-B039-006 copied-driver digest update executes reviewed classifier mutation and semantic assertions bite."
run_copied_scan_with_helper classification-mutation \
  '# copied candidate marker: digest updated, production classifier bytes intentionally changed below' updated
classification_copy="$FIXTURE_ROOT/helper-classification-mutation/bubbles/scripts/guards/sensitive-client-storage-scan.py"
classification_env="$FIXTURE_ROOT/helper-classification-mutation/bubbles/scripts/python-env.sh"
classification_scanner="$FIXTURE_ROOT/helper-classification-mutation/bubbles/scripts/implementation-reality-scan.sh"
classification_previous_digest="$(/usr/bin/shasum -a 256 "$classification_copy" | /usr/bin/awk '{print $1}')"
/usr/bin/perl -pi -e 's/DURABLE_CREDENTIAL_STORAGE/SESSION_CREDENTIAL_UNAPPROVED/g' "$classification_copy"
classification_digest="$(/usr/bin/shasum -a 256 "$classification_copy" | /usr/bin/awk '{print $1}')"
/usr/bin/perl -pi -e "s/$classification_previous_digest/$classification_digest/g" "$classification_env"
RUN_OUTPUT=""
RUN_STATUS=0
if selftest_run_mutation_bounded "$TMPDIR/classification-mutation.output" 180 /usr/bin/env \
  DEVELOPER_DIR=/Library/Developer/CommandLineTools \
  /bin/bash -c 'repo_root="$1"; shift; cd "$repo_root" || exit 2; exec "$@"' \
  _ "$SENSITIVE_REPO" /bin/bash "$classification_scanner" "$SENSITIVE_FEATURE" --verbose; then
  RUN_STATUS=0
else
  RUN_STATUS=$?
fi
RUN_OUTPUT="$(/bin/cat "$TMPDIR/classification-mutation.output")"
if [[ "$RUN_STATUS" -eq 1 ]] && ! grep -Fq 'diagnostic=HELPER_DIGEST_MISMATCH' <<<"$RUN_OUTPUT" &&
  grep -Fq 'classifier protocol complete: version=SCS1' <<<"$RUN_OUTPUT" &&
  grep -Fq 'reason=SESSION_CREDENTIAL_UNAPPROVED' <<<"$RUN_OUTPUT" &&
  ! grep -Fq 'reason=DURABLE_CREDENTIAL_STORAGE storage=localStorage operation=persist key=marketProvider:twelvedata:apiKey provider=twelvedata configMatch=absent' <<<"$RUN_OUTPUT"; then
  B039_AUTHORIZED_CLASSIFIER_MUTATION_VERIFIED=1
  pass "SCN-B039-003 copied classifier mutation executes after explicit digest review and changes semantic tuples"
  pass "Corrupting production classification makes the real-finding contract red"
else
  fail "SCN-B039-003 copied classifier mutation did not reach semantic classification (status=$RUN_STATUS)"
fi

echo "Scenario: SCN-B039-003 copied-driver completion mutation is rejected by the production protocol parser."
completion_root="$FIXTURE_ROOT/driver-completion-mutation"
completion_scripts="$completion_root/bubbles/scripts"
completion_scanner="$completion_scripts/implementation-reality-scan.sh"
completion_env="$completion_scripts/python-env.sh"
completion_helper="$completion_scripts/guards/sensitive-client-storage-scan.py"
completion_output="$TMPDIR/driver-completion-mutation.output"
mkdir -p "$completion_scripts/guards"
/bin/cp "$SCRIPT_DIR/python-env.sh" "$completion_env"
/bin/cp "$GUARD_LIB" "$completion_scripts/guard-lib.sh"
/bin/cp "$SCAN_SCRIPT" "$completion_scanner"
/bin/cp "$SCRIPT_DIR/guards/sensitive-client-storage-scan.py" "$completion_helper"
completion_record_before="$(/usr/bin/grep -cF 'print("COMPLETE\tSCS1\t%d" % scanned)' "$completion_env" || true)"
/usr/bin/perl -pi -e 's/^print\("COMPLETE\\tSCS1\\t%d" % scanned\)$/pass # copied completion-emission mutation/' "$completion_env"
completion_record_after="$(/usr/bin/grep -cF 'print("COMPLETE\tSCS1\t%d" % scanned)' "$completion_env" || true)"
RUN_OUTPUT=""
RUN_STATUS=0
if selftest_run_mutation_bounded "$completion_output" 180 /usr/bin/env \
  DEVELOPER_DIR=/Library/Developer/CommandLineTools \
  /bin/bash -c 'repo_root="$1"; shift; cd "$repo_root" || exit 2; exec "$@"' \
  _ "$PROTOCOL_REPO" /bin/bash "$completion_scanner" "$PROTOCOL_FEATURE" --verbose; then
  RUN_STATUS=0
else
  RUN_STATUS=$?
fi
RUN_OUTPUT="$(/bin/cat "$completion_output")"
if [[ "$completion_record_before" -eq 1 && "$completion_record_after" -eq 0 &&
  "$RUN_STATUS" -eq 1 &&
  "$RUN_OUTPUT" == *"diagnostic=CLASSIFIER_COMPLETION_MISSING"* ]] &&
  ! real_finding_contract_holds; then
  pass "Deleting production completion emission makes the real-finding contract red"
  pass "Completion-emission mutant fails through the production scanner path"
else
  fail "SCN-B039-003 completion mutation result before=$completion_record_before after=$completion_record_after status=$RUN_STATUS"
fi

echo "Scenario: SCN-B039-006 same-byte helper execution resists a post-read path replacement."
same_byte_marker="$TMPDIR/same-byte-original.marker"
replacement_marker="$TMPDIR/same-byte-replacement.marker"
same_byte_helper="$TMPDIR/same-byte-helper.py"
same_byte_replacement="$TMPDIR/same-byte-replacement.py"
same_byte_driver_raw="$TMPDIR/same-byte-driver.raw.py"
same_byte_driver="$TMPDIR/same-byte-driver.py"
same_byte_reopen="$TMPDIR/same-byte-reopen.py"
same_byte_output="$TMPDIR/same-byte-driver.output"
same_byte_reopen_output="$TMPDIR/same-byte-reopen.output"
same_byte_status=0
same_byte_reopen_status=0
same_byte_live_hash="$(/usr/bin/shasum -a 256 "$SCRIPT_DIR/python-env.sh")"
cat >"$same_byte_helper" <<EOF
from pathlib import Path
Path("$same_byte_marker").write_text("original", encoding="utf-8")
class ConfigError(Exception):
    line = 0
class Finding:
    def __init__(self, **kwargs):
        pass
    def emit(self):
        pass
def parse_project_config(config_path, repository):
    return [], None
def analyze_file(source_path, repository, approvals):
    return []
EOF
cat >"$same_byte_replacement" <<EOF
from pathlib import Path
Path("$replacement_marker").write_text("replacement", encoding="utf-8")
class ConfigError(Exception):
    line = 0
class Finding:
    def __init__(self, **kwargs):
        pass
    def emit(self):
        pass
def parse_project_config(config_path, repository):
    return [], None
def analyze_file(source_path, repository, approvals):
    return []
EOF
same_byte_digest="$(/usr/bin/shasum -a 256 "$same_byte_helper" | /usr/bin/awk '{print $1}')"
_bubbles_python_security_scan_driver >"$same_byte_driver_raw"
/usr/bin/awk -v replacement="$same_byte_replacement" '
  { print }
  /helper_code = compile\(helper_text/ {
    printf "    os.replace(Path(\"%s\"), helper_path)\n", replacement
  }
' "$same_byte_driver_raw" >"$same_byte_driver"
if selftest_run_mutation_bounded "$same_byte_output" 30 "$SECURITY_RUNTIME" -I -S -B "$same_byte_driver" \
  "$same_byte_helper" "$same_byte_digest" 262144 \
  "$PROTOCOL_REPO" "$PROTOCOL_REPO/.github/bubbles-project.yaml" "$PROTOCOL_SOURCE"; then
  same_byte_status=0
else
  same_byte_status=$?
fi
/bin/cat "$same_byte_output"
if [[ "$same_byte_status" -eq 0 && -e "$same_byte_marker" && ! -e "$replacement_marker" ]]; then
  pass "SCN-B039-006 production driver executes its checked byte buffer after path replacement"
else
  fail "SCN-B039-006 production driver did not preserve the checked byte buffer (status=$same_byte_status)"
fi
/bin/rm -f "$same_byte_marker" "$replacement_marker"
cat >"$same_byte_helper" <<EOF
from pathlib import Path
Path("$same_byte_marker").write_text("original", encoding="utf-8")
class ConfigError(Exception):
    line = 0
class Finding:
    def __init__(self, **kwargs):
        pass
    def emit(self):
        pass
def parse_project_config(config_path, repository):
    return [], None
def analyze_file(source_path, repository, approvals):
    return []
EOF
cat >"$same_byte_replacement" <<EOF
from pathlib import Path
Path("$replacement_marker").write_text("replacement", encoding="utf-8")
class ConfigError(Exception):
    line = 0
class Finding:
    def __init__(self, **kwargs):
        pass
    def emit(self):
        pass
def parse_project_config(config_path, repository):
    return [], None
def analyze_file(source_path, repository, approvals):
    return []
EOF
same_byte_digest="$(/usr/bin/shasum -a 256 "$same_byte_helper" | /usr/bin/awk '{print $1}')"
/usr/bin/awk '
  /exec\(helper_code, module.__dict__\)/ {
    print "    helper_code = compile(helper_path.read_text(encoding=\"utf-8\"), str(helper_path), \"exec\", dont_inherit=True)"
  }
  { print }
' "$same_byte_driver" >"$same_byte_reopen"
if selftest_run_mutation_bounded "$same_byte_reopen_output" 30 "$SECURITY_RUNTIME" -I -S -B "$same_byte_reopen" \
  "$same_byte_helper" "$same_byte_digest" 262144 \
  "$PROTOCOL_REPO" "$PROTOCOL_REPO/.github/bubbles-project.yaml" "$PROTOCOL_SOURCE"; then
  same_byte_reopen_status=0
else
  same_byte_reopen_status=$?
fi
/bin/cat "$same_byte_reopen_output"
if [[ "$same_byte_reopen_status" -eq 0 && ! -e "$same_byte_marker" && -e "$replacement_marker" ]]; then
  pass "SCN-B039-006 copied production-driver reopen mutation turns the same-byte assertion red"
else
  fail "SCN-B039-006 copied production-driver reopen mutation did not execute replacement bytes (status=$same_byte_reopen_status)"
fi
same_byte_after_hash="$(/usr/bin/shasum -a 256 "$SCRIPT_DIR/python-env.sh")"
if [[ "$same_byte_live_hash" == "$same_byte_after_hash" ]]; then
  pass "SCN-B039-006 same-byte and reopen mutations leave live production bytes identical"
else
  fail "SCN-B039-006 same-byte mutation changed live production bytes"
fi

echo "Running implementation-reality-scan discovery selftest..."
echo "Scenario: shell-heavy fixtures resolve honest implementation inventory."
run_expect_success "$FIXTURE_ROOT/shell-heavy-feature" "Shell-heavy fixture resolves .sh/.yaml/.yml/.json/docs-backed inventory"

echo "Scenario: missing inventories still fail with ZERO_FILES_RESOLVED."
run_expect_zero_files_failure "$FIXTURE_ROOT/missing-inventory-feature" "Missing-inventory fixture fails honestly without shim files"

echo "Scenario: Go connector helper nil returns are not fake when the package has a real transport client."
run_expect_success "$FIXTURE_ROOT/go-connector-package-feature" "Go connector helper return nil lines pass when a sibling client performs external calls"

echo "Scenario: no-op connector still fails external integration authenticity."
run_expect_fake_integration_failure "$FIXTURE_ROOT/fake-connector-feature" "No-op connector without an external call is still flagged as FAKE_INTEGRATION"

echo "Scenario: OpenTelemetry no-op tracer fallback + quoted 'noop' span-status literals are NOT flagged as fake integrations."
run_expect_success "$FIXTURE_ROOT/telemetry-noop-adapter-feature" "Telemetry no-op tracer fallback + quoted 'noop' span-status literals pass Scan 1D (BUG-064-001 false-positive class)"

echo "Scenario: a bare non-telemetry no-op integration body is STILL flagged (exclusion opens no hole)."
run_expect_fake_integration_failure "$FIXTURE_ROOT/fake-noop-integration-feature" "Bare non-telemetry, non-quoted no-op integration body is still flagged as FAKE_INTEGRATION"

assert_classifier_helper_cache_absent "Selftest removes prior bytecode from the real helper directory"

write_protocol_zero_source
run_scan_in_repo "$PROTOCOL_REPO" "$PROTOCOL_FEATURE"
if [[ "$RUN_STATUS" -eq 0 ]]; then
  pass "Real zero-finding producer executes the production driver and helper"
else
  fail "Real zero-finding producer executes the production driver and helper (scanner exit $RUN_STATUS)"
fi
assert_output_contains "classifier protocol complete: version=SCS1 scanned=1 findings=0" "Real zero-finding completion reports the honest scanned count"
assert_output_not_contains "VIOLATION [SENSITIVE_CLIENT_STORAGE]" "Source without a sensitive operation has no storage finding"
assert_classifier_helper_cache_absent "Real zero-finding producer creates no helper-side bytecode cache"

first_real_zero_status="$RUN_STATUS"
first_real_zero_summary="$(grep -F 'classifier protocol complete:' <<<"$RUN_OUTPUT" || true)"
run_scan_in_repo "$PROTOCOL_REPO" "$PROTOCOL_FEATURE"
second_real_zero_summary="$(grep -F 'classifier protocol complete:' <<<"$RUN_OUTPUT" || true)"
if [[ "$first_real_zero_status" -eq 0 && "$RUN_STATUS" -eq 0 &&
  "$first_real_zero_summary" == "$second_real_zero_summary" &&
  "$second_real_zero_summary" == *"version=SCS1 scanned=1 findings=0"* ]]; then
  pass "Consecutive real zero-finding production runs have identical verdict summaries"
else
  fail "Consecutive real zero-finding production runs diverged (first=$first_real_zero_status/$first_real_zero_summary second=$RUN_STATUS/$second_real_zero_summary)"
fi
assert_classifier_helper_cache_absent "Second consecutive real zero-finding producer creates no helper-side bytecode cache"

write_protocol_finding_source
run_scan_in_repo "$PROTOCOL_REPO" "$PROTOCOL_FEATURE"
if [[ "$RUN_STATUS" -eq 1 ]]; then
  pass "Real finding producer remains blocking after protocol completion"
else
  fail "Real finding producer remains blocking after protocol completion (scanner exit $RUN_STATUS)"
fi
assert_output_contains "classifier protocol complete: version=SCS1 scanned=1 findings=1" "Real finding completion reports one scanned file and one finding"
assert_output_contains "reason=DURABLE_CREDENTIAL_STORAGE storage=localStorage operation=persist key=marketProvider:twelvedata:apiKey provider=twelvedata configMatch=absent" "Real classifier emits the exact durable-credential finding tuple"
assert_classifier_helper_cache_absent "Real finding producer creates no helper-side bytecode cache"

echo "Scenario: a hostile PATH env cannot replace the trusted classifier launch."
hostile_env_path="$FIXTURE_ROOT/hostile-env-path"
hostile_env_marker="$FIXTURE_ROOT/hostile-env-executed"
mkdir -p "$hostile_env_path"
cat >"$hostile_env_path/env" <<'EOF'
#!/bin/bash
printf '%s\n' 'hostile env executed' >"$BUBBLES_HOSTILE_ENV_MARKER"
printf 'COMPLETE\tSCS1\t1\n'
exit 0
EOF
chmod +x "$hostile_env_path/env"
run_scan_in_repo_with_hostile_env "$PROTOCOL_REPO" "$PROTOCOL_FEATURE" \
  "$scope2_forged_home" "$hostile_env_path" "$hostile_env_marker"
if real_finding_contract_holds; then
  pass "Hostile PATH env cannot suppress the real classifier finding"
else
  hostile_env_diagnostic="$(grep -F 'sensitive-storage classifier' <<<"$RUN_OUTPUT" || true)"
  fail "Hostile PATH env cannot suppress the real classifier finding (scanner exit $RUN_STATUS; observed: $hostile_env_diagnostic)"
fi
if [[ ! -e "$hostile_env_marker" ]]; then
  pass "Trusted classifier launch never executes hostile PATH env"
else
  fail "Trusted classifier launch never executes hostile PATH env (marker exists)"
fi
assert_classifier_helper_cache_absent "Hostile PATH env scenario leaves the helper directory clean"

if sensitive_storage_classifier_usable; then
  echo "Scenario: semantic Scan 2B distinguishes storage operations and exact session classification."
  run_scan_in_repo "$SENSITIVE_REPO" "$SENSITIVE_FEATURE"
  if [[ "$RUN_STATUS" -eq 1 ]]; then
    pass "Sensitive storage matrix retains blocking findings"
  else
    fail "Sensitive storage matrix retains blocking findings (expected exit 1, got $RUN_STATUS)"
  fi
  assert_output_contains "reason=DURABLE_CREDENTIAL_STORAGE storage=localStorage operation=persist key=marketProvider:twelvedata:apiKey provider=twelvedata" "Literal and alias-resolved durable credentials are blocked"
  assert_output_not_contains "src/provider-client.js:6" "Exact configured session credential is allowed"
  assert_output_contains "reason=SESSION_PROVIDER_UNKNOWN" "Unknown session provider is blocked distinctly"
  assert_output_contains "reason=SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED" "Dynamic session provider is blocked unresolved"
  assert_output_contains "reason=FORBIDDEN_SECRET_CLASS storage=sessionStorage" "High-trust session material cannot use approval"
  assert_output_not_contains "src/provider-client.js:11" "Inline comment vocabulary does not taint cache"
  assert_output_not_contains "src/provider-client.js:12" "removeItem remains cleanup"
  assert_output_contains "src/provider-client.js:14" "Credential object before scrub remains blocking"
  assert_output_not_contains "src/provider-client.js:18" "Proven scrubbed rewrite remains clear"
  assert_output_contains "storage=indexedDB operation=read" "IndexedDB credential access remains covered"
  assert_output_contains "storage=SharedPreferences operation=persist" "SharedPreferences credential persistence remains covered"
  assert_output_contains "storage=AsyncStorage operation=persist" "AsyncStorage credential persistence remains covered"
  assert_output_contains "storage=indexedDB operation=persist key=marketProvider:twelvedata:apiKey" "IndexedDB object-store credential persistence remains covered"
  assert_output_contains "src/provider-preferences.dart" "SharedPreferences instance credential persistence remains covered"

  echo "Scenario: sensitive storage project configuration fails closed."
  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: ../src/*.js
        storage: sessionStorage
        key: marketProvider:*:apiKey
        provider: '*'
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
  assert_sensitive_invalid_config "Traversal and wildcard approval"

  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
      - path: src/provider-client.js
        storage: sessionStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: third-party-market-data
        privilege: low
        lifetime: same-tab
EOF
  assert_sensitive_invalid_config "Duplicate approval tuple"

  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    unknownField: true
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage: localStorage
        key: marketProvider:twelvedata:apiKey
        provider: twelvedata
        credentialClass: auth-token
        privilege: high
        lifetime: durable
EOF
  assert_sensitive_invalid_config "Unknown field and enum values"

  cat > "$SENSITIVE_CONFIG" <<'EOF'
scans:
  sensitiveClientStorage:
    approvedSessionCredentials:
      - path: src/provider-client.js
        storage sessionStorage
        key: marketProvider:twelvedata:apiKey
EOF
  assert_sensitive_invalid_config "Malformed sensitive storage YAML"
else
  # Machine-readable for consumers (tests/regression/test_24_...) so a skip can
  # never be scraped as a pass.
  echo "SENSITIVE_STORAGE_CLASSIFIER_UNAVAILABLE=1"
  skip "semantic Scan 2B classification and sensitive-storage config integrity — $CLASSIFIER_UNAVAILABLE_REASON"
  echo "      remediation: $CLASSIFIER_REMEDIATION"
  echo "      not run: 15 semantic classification assertions, 8 config-integrity assertions."
  echo "      Both groups assert exact classifier tuples. With the classifier unable to start, the scan"
  echo "      fails closed to SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED for every candidate line and"
  echo "      emits SENSITIVE_STORAGE_CONFIG_INVALID for any config declaring the key, so neither a pass"
  echo "      nor a failure from these assertions would carry information about the classifier."
fi

write_sensitive_valid_config
NO_PARSER_PATH="$TMPDIR/no-parser-path"
mkdir -p "$NO_PARSER_PATH"
for tool_name in awk basename cat cut dirname find grep head sed sort tr wc; do
  tool_path="$(command -v "$tool_name" 2>/dev/null || true)"
  [[ -z "$tool_path" ]] || ln -s "$tool_path" "$NO_PARSER_PATH/$tool_name"
done
parser_output=""
parser_status=0
if parser_output="$(
  cd "$SENSITIVE_REPO" || exit 2
  env -i PATH="$NO_PARSER_PATH" /bin/bash "$SCAN_SCRIPT" "$SENSITIVE_FEATURE" --verbose 2>&1
)"; then
  parser_status=0
else
  parser_status=$?
fi
printf '%s\n' "$parser_output"
if [[ "$parser_status" -eq 1 ]] && printf '%s\n' "$parser_output" | grep -Fq 'reason=SENSITIVE_STORAGE_CONFIG_INVALID'; then
  pass "Parser-unavailable configured approval fails closed"
else
  fail "Parser-unavailable configured approval fails closed"
fi

echo "Scenario: portable watchdog preserves exit 124 without GNU coreutils."
NO_TIMEOUT_PATH="$TMPDIR/no-timeout-path"
mkdir -p "$NO_TIMEOUT_PATH"
ln -s "$(command -v sleep)" "$NO_TIMEOUT_PATH/sleep"
portable_timeout_status=0
if (
  PATH="$NO_TIMEOUT_PATH"
  hash -r
  bubbles_run_with_timeout 1 /bin/sleep 5
); then
  portable_timeout_status=0
else
  portable_timeout_status=$?
fi
if [[ "$portable_timeout_status" -eq 124 ]]; then
  echo "PORTABLE_WATCHDOG_FALLBACK=124"
  pass "Portable watchdog preserves exit 124"
else
  echo "PORTABLE_WATCHDOG_FALLBACK=$portable_timeout_status"
  fail "Portable watchdog preserves exit 124"
fi

echo "Scenario: fixed-operation timeout recovery leaves the next classifier run healthy."
write_protocol_zero_source
run_scan_in_repo "$PROTOCOL_REPO" "$PROTOCOL_FEATURE"
if [[ "$RUN_STATUS" -eq 0 ]]; then
  pass "Classifier remains reusable after both watchdog timeouts"
else
  fail "Classifier remains reusable after both watchdog timeouts (scanner exit $RUN_STATUS)"
fi
assert_output_contains "classifier protocol complete: version=SCS1 scanned=1 findings=0" "Post-timeout classifier completes its protocol"
assert_classifier_helper_cache_absent "Post-timeout real producer leaves the helper directory clean"

# Only a run that reaches this point may return its verdict. The EXIT trap
# converts every earlier zero-status exit into failure, while preserving any
# original nonzero status and the dedicated INT/TERM statuses.
SELFTEST_COMPLETED=1

echo "implementation-reality-scan selftest summary: failures=$failures skips=$skips"
echo "B039_AUTHORIZED_CLASSIFIER_MUTATION_VERIFIED=$B039_AUTHORIZED_CLASSIFIER_MUTATION_VERIFIED"

if [[ "$skips" -gt 0 ]]; then
  echo "implementation-reality-scan selftest skipped $skips scenario group(s) for an absent prerequisite."
fi

if [[ "$failures" -gt 0 ]]; then
  echo "implementation-reality-scan selftest failed with $failures issue(s)."
  exit 1
fi

if [[ "$skips" -gt 0 ]]; then
  echo "implementation-reality-scan selftest passed the scenarios it could run ($skips skipped)."
  exit 0
fi

echo "IMPLEMENTATION_REALITY_SELFTEST_FULL_SUITE_COMPLETED=1"
echo "implementation-reality-scan selftest passed."
