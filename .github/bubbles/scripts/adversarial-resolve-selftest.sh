#!/usr/bin/env bash
set -uo pipefail

# adversarial-resolve-selftest.sh — IMP-020 S2 posture resolver contract.
#
# ADVERSARIAL by design: canonical samples must beat the deprecated passes
# alias at the same layer, while directive > env > config > default precedence
# remains observable. Invalid counts and bypass-shaped flags must fail closed.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SELF="$SCRIPT_DIR/adversarial-resolve-selftest.sh"
RESOLVE="$SCRIPT_DIR/adversarial-resolve.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"

# shellcheck source=/dev/null
source "$GUARD_LIB"

pass=0
fail=0
RUN_STDOUT=""
RUN_STDERR=""
RUN_STATUS=0

cleanup_fifo_writers() {
  local cleanup_root="$1"
  local pid_file=""
  local metadata_version=""
  local writer_pid=""
  local writer_shell=""
  local writer_script=""
  local writer_root=""
  local writer_fifo=""
  local writer_token=""
  local writer_ready=""
  local extra_line=""
  local writer_fifo_relative=""
  local writer_ready_relative=""
  local writer_command=""
  local expected_command=""

  [[ -d "$cleanup_root" ]] || return 0
  while IFS= read -r pid_file; do
    [[ -f "$pid_file" && ! -L "$pid_file" ]] || continue
    metadata_version=""
    writer_pid=""
    writer_shell=""
    writer_script=""
    writer_root=""
    writer_fifo=""
    writer_token=""
    writer_ready=""
    extra_line=""
    {
      IFS= read -r metadata_version &&
        IFS= read -r writer_pid &&
        IFS= read -r writer_shell &&
        IFS= read -r writer_script &&
        IFS= read -r writer_root &&
        IFS= read -r writer_fifo &&
        IFS= read -r writer_token &&
        IFS= read -r writer_ready &&
        ! IFS= read -r extra_line
    } < "$pid_file" || continue
      [[ -z "$extra_line" ]] || continue

    [[ "$metadata_version" == "version=1" ]] || continue
    [[ "$writer_pid" == pid=* ]] || continue
    writer_pid="${writer_pid#pid=}"
    [[ "$writer_pid" =~ ^[1-9][0-9]*$ ]] || continue
    [[ "$writer_shell" == shell=* ]] || continue
    writer_shell="${writer_shell#shell=}"
    [[ "$writer_shell" == "$BASH" ]] || continue
    [[ "$writer_script" == script=* ]] || continue
    writer_script="${writer_script#script=}"
    [[ "$writer_script" == "$HARNESS_FIFO_WRITER" ]] || continue
    [[ -f "$writer_script" && ! -L "$writer_script" ]] || continue
    [[ "$writer_root" == root=* ]] || continue
    writer_root="${writer_root#root=}"
    [[ "$writer_root/.bug014-fifo-writer.pid" == "$pid_file" ]] || continue
    [[ "$writer_root" == "$cleanup_root" || "$writer_root" == "$cleanup_root/"* ]] || continue
    [[ "$writer_fifo" == fifo=* ]] || continue
    writer_fifo="${writer_fifo#fifo=}"
    writer_fifo_relative="${writer_fifo#"$writer_root"/}"
    [[ "$writer_fifo_relative" != "$writer_fifo" ]] || continue
    case "$writer_fifo_relative" in
      ""|.|..|*/*) continue ;;
    esac
    [[ "$writer_token" == token=* ]] || continue
    writer_token="${writer_token#token=}"
    [[ "$writer_token" =~ ^bug014-[0-9]+-[0-9]+-[0-9]+-[0-9]+$ ]] || continue
    [[ "$writer_ready" == ready=* ]] || continue
    writer_ready="${writer_ready#ready=}"
    writer_ready_relative="${writer_ready#"$writer_root"/}"
    [[ "$writer_ready_relative" != "$writer_ready" ]] || continue
    case "$writer_ready_relative" in
      .bug014-fifo-writer.ready.*) ;;
      *) continue ;;
    esac

    expected_command="$writer_shell $writer_script write $writer_root $writer_fifo $pid_file $writer_token $writer_ready"
    writer_command="$(ps -ww -p "$writer_pid" -o command= 2>/dev/null)" || writer_command=""
    [[ "$writer_command" == "$expected_command" ]] || continue

    kill -TERM "$writer_pid" 2>/dev/null || continue
    wait "$writer_pid" 2>/dev/null || true
    if kill -0 "$writer_pid" 2>/dev/null; then
      kill -KILL "$writer_pid" 2>/dev/null || true
      bubbles_run_with_timeout 2 sh -c \
        "while kill -0 \"\$1\" 2>/dev/null; do :; done" sh "$writer_pid" \
        >/dev/null 2>&1 || true
    fi
  done < <(find "$cleanup_root" -type f \
    -name '.bug014-fifo-writer.pid' -print 2>/dev/null)
}

cleanup_harness() {
  local cleanup_status=$?
  local cleanup_root="$1"

  trap - EXIT INT TERM
  cleanup_fifo_writers "$cleanup_root"
  rm -rf "$cleanup_root" >/dev/null 2>&1 || true
  return "$cleanup_status"
}

cleanup_harness_signal() {
  local signal="$1"
  local cleanup_root="$2"
  local signal_status=0

  case "$signal" in
    INT) signal_status=130 ;;
    TERM) signal_status=143 ;;
    *) return 2 ;;
  esac

  trap - EXIT INT TERM
  cleanup_fifo_writers "$cleanup_root"
  rm -rf "$cleanup_root" >/dev/null 2>&1 || true
  exit "$signal_status"
}

run_f6_lifecycle_child() {
  local cleanup_root="$BUG014_F6_CLEANUP_ROOT"
  local ready_fifo="$BUG014_F6_READY_FIFO"
  local control_fifo="$BUG014_F6_CONTROL_FIFO"
  local sentinel="$BUG014_F6_SENTINEL"
  local action="$BUG014_F6_ACTION"
  local writer_fifo="$cleanup_root/blocked-writer.fifo"
  local writer_metadata="$cleanup_root/.bug014-fifo-writer.pid"
  local writer_pid=""
  local control=""

  case "$action" in
    EXIT|INT|TERM) ;;
    *) return 64 ;;
  esac

  trap 'cleanup_harness "$cleanup_root"' EXIT
  trap 'cleanup_harness_signal INT "$cleanup_root"' INT
  trap 'cleanup_harness_signal TERM "$cleanup_root"' TERM
  mkdir -p "$cleanup_root"
  mkfifo "$writer_fifo"
  writer_pid="$("$BASH" "$HARNESS_FIFO_WRITER" start \
    "$cleanup_root" "$writer_fifo" "$writer_metadata")" || return 65
  [[ "$writer_pid" =~ ^[1-9][0-9]*$ ]] || return 66
  printf '%s\n' "$writer_pid" > "$ready_fifo"

  if [[ "$action" == "EXIT" ]]; then
    IFS= read -r control < "$control_fifo"
    [[ "$control" == "exit" ]] || return 67
    exit 37
  fi

  IFS= read -r control 2>/dev/null < "$control_fifo"
  printf '%s\n' "post-signal-$action" > "$sentinel"
  exit 99
}

if [[ "${BUG014_F6_LIFECYCLE_CHILD:-0}" == "1" ]]; then
  : "${BUG014_F6_WRITER_SCRIPT:?missing F6 writer script}"
  : "${BUG014_F6_CLEANUP_ROOT:?missing F6 cleanup root}"
  : "${BUG014_F6_READY_FIFO:?missing F6 ready FIFO}"
  : "${BUG014_F6_CONTROL_FIFO:?missing F6 control FIFO}"
  : "${BUG014_F6_SENTINEL:?missing F6 sentinel}"
  : "${BUG014_F6_ACTION:?missing F6 action}"
  HARNESS_FIFO_WRITER="$BUG014_F6_WRITER_SCRIPT"
  run_f6_lifecycle_child
  exit $?
fi

# Keep fixtures under HOME so snap-confined binaries can read them. A tiny yq
# shim makes config-layer coverage hermetic and supports only this fixture shape.
tmp="$(mktemp -d "${HOME}/.bubbles-selftest-adversarial-resolve.XXXXXX")"
HARNESS_FIFO_WRITER="$tmp/bin/bug014-fifo-writer.sh"

trap 'cleanup_harness "$tmp"' EXIT
trap 'cleanup_harness_signal INT "$tmp"' INT
trap 'cleanup_harness_signal TERM "$tmp"' TERM
empty_repo="$tmp/empty"
config_repo="$tmp/config"
config_both_repo="$tmp/config-both"
config_alias_repo="$tmp/config-alias"
config_invalid_repo="$tmp/config-invalid"
config_over_max_repo="$tmp/config-over-max"
config_huge_repo="$tmp/config-huge"
config_alias_over_max_repo="$tmp/config-alias-over-max"
config_alias_huge_repo="$tmp/config-alias-huge"
config_empty_mode_repo="$tmp/config-empty-mode"
config_empty_samples_repo="$tmp/config-empty-samples"
config_empty_samples_alias_repo="$tmp/config-empty-samples-alias"
config_empty_passes_repo="$tmp/config-empty-passes"
config_empty_teeth_repo="$tmp/config-empty-teeth"
config_empty_canonical_repo="$tmp/config-empty-canonical"
config_empty_alias_repo="$tmp/config-empty-alias"
config_null_repo="$tmp/config-null"
mkdir -p \
  "$empty_repo/.github" \
  "$config_repo/.github" \
  "$config_both_repo/.github" \
  "$config_alias_repo/.github" \
  "$config_invalid_repo/.github" \
  "$config_over_max_repo/.github" \
  "$config_huge_repo/.github" \
  "$config_alias_over_max_repo/.github" \
  "$config_alias_huge_repo/.github" \
  "$config_empty_mode_repo/.github" \
  "$config_empty_samples_repo/.github" \
  "$config_empty_samples_alias_repo/.github" \
  "$config_empty_passes_repo/.github" \
  "$config_empty_teeth_repo/.github" \
  "$config_empty_canonical_repo/.github" \
  "$config_empty_alias_repo/.github" \
  "$config_null_repo/.github" \
  "$tmp/bin"
cat > "$HARNESS_FIFO_WRITER" <<'BUG014_FIFO_WRITER'
#!/usr/bin/env bash
set -uo pipefail

mode="${1:-}"
case "$mode" in
  start)
    [[ "$#" -eq 4 ]] || exit 64
    writer_root="$2"
    writer_fifo="$3"
    writer_metadata="$4"
    [[ -d "$writer_root" ]] || exit 65
    [[ -p "$writer_fifo" ]] || exit 66
    [[ "$writer_metadata" == "$writer_root/.bug014-fifo-writer.pid" ]] || exit 67
    writer_fifo_relative="${writer_fifo#"$writer_root"/}"
    [[ "$writer_fifo_relative" != "$writer_fifo" ]] || exit 68
    case "$writer_fifo_relative" in
      ""|.|..|*/*) exit 69 ;;
    esac

    writer_token="bug014-$PPID-$$-$RANDOM-$RANDOM"
    writer_ready="$writer_root/.bug014-fifo-writer.ready.$$.$RANDOM"
    writer_pid=""
    writer_started=0
    writer_ready_complete=0
    cleanup_start() {
      local cleanup_status=$?
      trap - EXIT
      rm -f "$writer_ready" >/dev/null 2>&1 || true
      if [[ "$writer_started" -eq 1 && "$writer_ready_complete" -eq 0 ]]; then
        kill -TERM "$writer_pid" 2>/dev/null || true
        wait "$writer_pid" 2>/dev/null || true
      fi
      return "$cleanup_status"
    }
    trap cleanup_start EXIT
    mkfifo "$writer_ready"
    "$BASH" "$0" write "$writer_root" "$writer_fifo" \
      "$writer_metadata" "$writer_token" "$writer_ready" \
      >/dev/null 2>&1 &
    writer_pid=$!
    writer_started=1
    {
      printf 'version=1\n'
      printf 'pid=%s\n' "$writer_pid"
      printf 'shell=%s\n' "$BASH"
      printf 'script=%s\n' "$0"
      printf 'root=%s\n' "$writer_root"
      printf 'fifo=%s\n' "$writer_fifo"
      printf 'token=%s\n' "$writer_token"
      printf 'ready=%s\n' "$writer_ready"
    } > "$writer_metadata"
    IFS= read -r writer_ready_line < "$writer_ready"
    [[ "$writer_ready_line" == "ready:$writer_token:$writer_pid" ]] || exit 70
    writer_ready_complete=1
    printf '%s\n' "$writer_pid"
    ;;
  write)
    [[ "$#" -eq 6 ]] || exit 71
    writer_root="$2"
    writer_fifo="$3"
    writer_metadata="$4"
    writer_token="$5"
    writer_ready="$6"
    [[ "$writer_metadata" == "$writer_root/.bug014-fifo-writer.pid" ]] || exit 72
    [[ "$writer_token" =~ ^bug014-[0-9]+-[0-9]+-[0-9]+-[0-9]+$ ]] || exit 73
    case "$writer_ready" in
      "$writer_root"/.bug014-fifo-writer.ready.*) ;;
      *) exit 74 ;;
    esac
    printf 'ready:%s:%s\n' "$writer_token" "$$" > "$writer_ready"
    printf 'samples\t3\n' > "$writer_fifo"
    ;;
  *) exit 75 ;;
esac
BUG014_FIFO_WRITER
chmod +x "$HARNESS_FIFO_WRITER"

cat > "$tmp/bin/bug014-unrelated-blocker.sh" <<'BUG014_UNRELATED_BLOCKER'
#!/usr/bin/env bash
printf 'ready\n' > "$1"
printf 'blocked\n' > "$2"
BUG014_UNRELATED_BLOCKER
chmod +x "$tmp/bin/bug014-unrelated-blocker.sh"
cat > "$tmp/bin/yq" <<'YQ'
#!/usr/bin/env bash
set -eu
query="$1"
file="$2"
key="${query#.adversarial.}"
key="${key%% *}:"
tag_query=0
case "$query" in
  *'| tag') tag_query=1 ;;
esac
awk -v wanted="$key" -v tag_query="$tag_query" '
  $1 == wanted {
    found = 1
    value = $2
    if (tag_query == 1) {
      if (value == "null" || value == "~") print "!!null"
      else print "!!str"
    } else if (value == "\"\"" || value == "\047\047") {
      print ""
    } else if (value == "null" || value == "~") {
      print "null"
    } else {
      print value
    }
    exit
  }
  END {
    if (!found && tag_query == 1) print "!!null"
    else if (!found) print "null"
  }
' "$file"
YQ
chmod +x "$tmp/bin/yq"
printf 'adversarial:\n  mode: auto\n  samples: 4\n  teeth: blocking\n' \
  > "$config_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: 4\n  passes: 9\n' \
  > "$config_both_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  passes: 4\n' \
  > "$config_alias_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: 0\n' \
  > "$config_invalid_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: 6\n' \
  > "$config_over_max_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: 1000000000\n' \
  > "$config_huge_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  passes: 6\n' \
  > "$config_alias_over_max_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  passes: 1000000000\n' \
  > "$config_alias_huge_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  mode: ""\n' \
  > "$config_empty_mode_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: ""\n' \
  > "$config_empty_samples_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  samples: ""\n  passes: 4\n' \
  > "$config_empty_samples_alias_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  passes: ""\n' \
  > "$config_empty_passes_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  teeth: ""\n' \
  > "$config_empty_teeth_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  mode: ""\n  samples: ""\n  teeth: ""\n' \
  > "$config_empty_canonical_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  passes: ""\n' \
  > "$config_empty_alias_repo/.github/bubbles-project.yaml"
printf 'adversarial:\n  mode: null\n  samples: null\n  passes: null\n  teeth: null\n' \
  > "$config_null_repo/.github/bubbles-project.yaml"

TEST_PATH="$tmp/bin:$PATH"
REAL_AWK="$(command -v awk)"
REAL_CAT="$(command -v cat)"
REAL_CHMOD="$(command -v chmod)"
REAL_LN="$(command -v ln)"
REAL_MKFIFO="$(command -v mkfifo)"
REAL_MKTEMP="$(command -v mktemp)"
REAL_RM="$(command -v rm)"
record_pass() {
  pass=$((pass + 1))
}
record_fail() {
  echo "FAIL: $1"
  fail=$((fail + 1))
}

assert_status() { # label expected
  if [[ "$RUN_STATUS" -eq "$2" ]]; then
    record_pass
  else
  record_fail "$1 (expected status $2, got $RUN_STATUS)"
  printf '%s\n' "$RUN_STDOUT" | sed 's/^/    stdout: /'
  printf '%s\n' "$RUN_STDERR" | sed 's/^/    stderr: /'
  fi
}

assert_eq() { # label actual expected
  if [[ "$2" == "$3" ]]; then
    record_pass
  else
  record_fail "$1"
  printf '    expected: %s\n' "$3"
  printf '    actual:   %s\n' "$2"
  fi
}

assert_eq_redacted() { # label actual expected
  if [[ "$2" == "$3" ]]; then
    record_pass
  else
    record_fail "$1 (actual bytes=${#2}, expected bytes=${#3})"
  fi
}

assert_line() { # label output exact-line
  if printf '%s\n' "$2" | grep -Fqx -- "$3"; then
    record_pass
  else
    record_fail "$1 (missing line: $3)"
    printf '%s\n' "$2" | sed 's/^/    /'
  fi
}

assert_match() { # label output extended-regex
  if printf '%s\n' "$2" | grep -Eqi -- "$3"; then
    record_pass
  else
    record_fail "$1 (missing pattern: $3)"
    printf '%s\n' "$2" | sed 's/^/    /'
  fi
}

assert_not_match() { # label output extended-regex
  if printf '%s\n' "$2" | grep -Eqi -- "$3"; then
    record_fail "$1 (unexpected pattern: $3)"
    printf '%s\n' "$2" | sed 's/^/    /'
  else
    record_pass
  fi
}

run_resolver() {
  local stderr_file="$tmp/resolver.stderr"
  RUN_STDOUT=""
  RUN_STDERR=""
  RUN_STATUS=0
  if RUN_STDOUT="$(env -i PATH="$TEST_PATH" "$@" 2>"$stderr_file")"; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_STDERR="$(cat "$stderr_file")"
}

assert_alias_result() { # label expected-samples expected-source expected-layer
  local label="$1"
  assert_status "$label exits zero" 0
  assert_line "$label resolves samples" "$RUN_STDOUT" "samples=$2"
  assert_line "$label records source" "$RUN_STDOUT" "samplesSource=$3"
  assert_line "$label records deprecation" "$RUN_STDOUT" "deprecation=passes-alias"
  assert_match "$label warns on stderr" "$RUN_STDERR" "DEPRECATED: passes alias used at layer\\(s\\): $4; use samples instead"
  assert_not_match "$label emits no legacy passes key" "$RUN_STDOUT" '^passes='
}

assert_shadowed_alias_result() { # label expected-samples expected-source expected-layers
  local label="$1"
  assert_status "$label exits zero" 0
  assert_line "$label preserves selected samples" "$RUN_STDOUT" "samples=$2"
  assert_line "$label preserves selected source" "$RUN_STDOUT" "samplesSource=$3"
  assert_line "$label records deprecation" "$RUN_STDOUT" "deprecation=passes-alias"
  assert_eq "$label reports exact warning layer order" "$RUN_STDERR" \
    "adversarial-resolve: DEPRECATED: passes alias used at layer(s): $4; use samples instead"
  assert_not_match "$label does not validate a shadowed alias" "$RUN_STDERR" \
    'invalid .*passes'
  assert_not_match "$label emits no legacy passes key" "$RUN_STDOUT" '^passes='
}

assert_rejected_value() { # label diagnostic-pattern
  local label="$1"
  assert_status "$label exits one" 1
  assert_match "$label reports the complete selected value" "$RUN_STDERR" "$2"
  assert_eq "$label emits no resolved posture" "$RUN_STDOUT" ""
}

assert_usage_ambiguity() { # label diagnostic-pattern
  local label="$1"
  assert_status "$label exits two" 2
  assert_match "$label reports ambiguity" "$RUN_STDERR" "$2"
  assert_eq "$label emits no resolved posture" "$RUN_STDOUT" ""
}

# 1. The zero-config output is canonical and defaults to one correlated sample.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo"
expected_default="$(printf '%s\n' \
  'mode=off' \
  'samples=1' \
  'sampleSemantics=same-runtime-correlated' \
  'teeth=warn' \
  'source=default' \
  'samplesSource=default' \
  'deprecation=none')"
assert_status "default exits zero" 0
assert_eq "default output is canonical" "$RUN_STDOUT" "$expected_default"
assert_eq "default stderr is empty" "$RUN_STDERR" ""
assert_not_match "default emits no passes key" "$RUN_STDOUT" '^passes='

# 2. Explicit --samples is the canonical directive-layer flag.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --mode on --samples 3
assert_status "--samples exits zero" 0
assert_line "--samples resolves count" "$RUN_STDOUT" "samples=3"
assert_line "--samples records directive source" "$RUN_STDOUT" "samplesSource=directive"
assert_line "active output labels correlated samples" "$RUN_STDOUT" "sampleSemantics=same-runtime-correlated"
assert_line "canonical flag has no deprecation" "$RUN_STDOUT" "deprecation=none"
assert_not_match "active output has no independence or majority value" "$RUN_STDOUT" '(^|=).*(independent|vote|consensus|ensemble)'
assert_not_match "active output emits no passes key" "$RUN_STDOUT" '^passes='

# 3. Canonical samples resolve independently at env, config, and directive-string layers.
run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 bash "$RESOLVE" --repo-root "$empty_repo"
assert_status "env samples exits zero" 0
assert_line "env samples resolves count" "$RUN_STDOUT" "samples=5"
assert_line "env samples records source" "$RUN_STDOUT" "samplesSource=env"

run_resolver bash "$RESOLVE" --repo-root "$config_repo"
assert_status "config samples exits zero" 0
assert_line "config samples resolves count" "$RUN_STDOUT" "samples=4"
assert_line "config samples records source" "$RUN_STDOUT" "samplesSource=config"
assert_line "config mode resolves" "$RUN_STDOUT" "mode=auto"
assert_line "config teeth resolves" "$RUN_STDOUT" "teeth=blocking"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "adversarial: on samples: 4 teeth: blocking"
assert_status "directive-string samples exits zero" 0
assert_line "directive-string samples resolves count" "$RUN_STDOUT" "samples=4"
assert_line "directive-string samples records source" "$RUN_STDOUT" "samplesSource=directive"

# 4. The full precedence chain is directive > env > config > default.
run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 bash "$RESOLVE" --repo-root "$config_repo"
assert_status "env-over-config exits zero" 0
assert_line "env beats config" "$RUN_STDOUT" "samples=5"
assert_line "env-over-config source" "$RUN_STDOUT" "samplesSource=env"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 bash "$RESOLVE" --repo-root "$config_repo" --directive "samples: 3"
assert_status "directive-over-env-config exits zero" 0
assert_line "directive beats env and config" "$RUN_STDOUT" "samples=3"
assert_line "directive-over-env-config source" "$RUN_STDOUT" "samplesSource=directive"

# 5. Only the selected canonical count is validated; shadowed invalid counts
# cannot reject a valid higher-precedence value.
run_resolver BUBBLES_ADVERSARIAL_SAMPLES=0 bash "$RESOLVE" --repo-root "$empty_repo" --samples 3
assert_status "directive samples shadows invalid env samples" 0
assert_line "directive samples survives invalid env samples" "$RUN_STDOUT" "samples=3"
assert_line "directive samples owns shadowed-env result" "$RUN_STDOUT" "samplesSource=directive"
assert_line "directive samples over invalid env has no deprecation" "$RUN_STDOUT" "deprecation=none"
assert_eq "shadowed invalid env emits no diagnostic" "$RUN_STDERR" ""

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 bash "$RESOLVE" --repo-root "$config_invalid_repo"
assert_status "env samples shadows invalid config samples" 0
assert_line "env samples survives invalid config samples" "$RUN_STDOUT" "samples=5"
assert_line "env samples owns shadowed-config result" "$RUN_STDOUT" "samplesSource=env"
assert_line "env samples over invalid config has no deprecation" "$RUN_STDOUT" "deprecation=none"
assert_eq "shadowed invalid config emits no diagnostic" "$RUN_STDERR" ""

# 6. Canonical samples wins over passes at every same-layer collision. Under
# the current compatibility contract, a present but shadowed alias is still
# reported as deprecated even when its value is invalid and never selected.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 5 --passes 9
assert_alias_result "canonical flag over alias flag" 5 directive directive

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 3 --passes 0
assert_alias_result "canonical flag over invalid alias flag" 3 directive directive
assert_not_match "shadowed invalid alias is not validated" "$RUN_STDERR" "invalid passes alias '0'"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 BUBBLES_ADVERSARIAL_PASSES=9 bash "$RESOLVE" --repo-root "$empty_repo"
assert_alias_result "canonical env over alias env" 5 env env

run_resolver bash "$RESOLVE" --repo-root "$config_both_repo"
assert_alias_result "canonical config over alias config" 4 config config

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 4 passes: 9"
assert_alias_result "canonical directive token over alias token" 4 directive directive

# 6a. Deprecated alias presence is reported across every input layer even when
# a higher-precedence canonical value wins. Shadowed alias text is metadata
# only: valid, malformed, and out-of-range values must never be validated.
run_resolver BUBBLES_ADVERSARIAL_PASSES=2 \
  bash "$RESOLVE" --repo-root "$empty_repo" --samples 3
assert_shadowed_alias_result \
  "directive samples over valid env alias" 3 directive env

run_resolver BUBBLES_ADVERSARIAL_PASSES=not-a-count \
  bash "$RESOLVE" --repo-root "$empty_repo" --samples 3
assert_shadowed_alias_result \
  "directive samples over malformed env alias" 3 directive env

run_resolver bash "$RESOLVE" --repo-root "$config_alias_repo" --samples 3
assert_shadowed_alias_result \
  "directive samples over valid config alias" 3 directive config

run_resolver bash "$RESOLVE" --repo-root "$config_alias_huge_repo" --samples 3
assert_shadowed_alias_result \
  "directive samples over malformed config alias" 3 directive config

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 \
  bash "$RESOLVE" --repo-root "$config_alias_repo"
assert_shadowed_alias_result \
  "env samples over valid config alias" 5 env config

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 \
  bash "$RESOLVE" --repo-root "$config_alias_huge_repo"
assert_shadowed_alias_result \
  "env samples over malformed config alias" 5 env config

run_resolver BUBBLES_ADVERSARIAL_PASSES=2 \
  bash "$RESOLVE" --repo-root "$config_alias_huge_repo" --samples 3
assert_shadowed_alias_result \
  "directive samples over env and config aliases" 3 directive env,config

run_resolver BUBBLES_ADVERSARIAL_PASSES=not-a-count \
  bash "$RESOLVE" --repo-root "$config_alias_huge_repo" \
  --samples 3 --passes 0
assert_shadowed_alias_result \
  "directive samples reports all alias layers" 3 directive directive,env,config

# 7. Each deprecated alias resolves to samples and emits metadata plus warning.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes 2
assert_alias_result "deprecated --passes" 2 directive directive

run_resolver BUBBLES_ADVERSARIAL_PASSES=3 bash "$RESOLVE" --repo-root "$empty_repo"
assert_alias_result "deprecated env passes" 3 env env

run_resolver bash "$RESOLVE" --repo-root "$config_alias_repo"
assert_alias_result "deprecated config passes" 4 config config

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "passes: 5"
assert_alias_result "deprecated directive passes" 5 directive directive

# 8. Directive extraction accepts exact keys only. Longer identifiers,
# hyphenated identifiers, underscored identifiers, and prose without a key/value
# separator must leave the default count and deprecation metadata untouched.
for ignored_directive in \
  "resamples: 7 compasses: 9" \
  "my-samples: 7" \
  "passes_extra: 9" \
  "please use samples 7 for this run"; do
  run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "$ignored_directive"
  assert_status "non-key directive '$ignored_directive' exits zero" 0
  assert_line "non-key directive '$ignored_directive' keeps default samples" "$RUN_STDOUT" "samples=1"
  assert_line "non-key directive '$ignored_directive' keeps default source" "$RUN_STDOUT" "samplesSource=default"
  assert_line "non-key directive '$ignored_directive' has no deprecation" "$RUN_STDOUT" "deprecation=none"
  assert_eq "non-key directive '$ignored_directive' emits no warning" "$RUN_STDERR" ""
done

# 9. Duplicate flags and duplicate exact directive tokens are usage errors, not
# last-value-wins. A usage error must not expose a partial resolved posture.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 2 --samples 3
assert_status "duplicate --samples rejected" 2
assert_match "duplicate --samples explains ambiguity" "$RUN_STDERR" 'duplicate --samples flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes 2 --passes 3
assert_status "duplicate --passes rejected" 2
assert_match "duplicate --passes explains ambiguity" "$RUN_STDERR" 'duplicate --passes flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 2 samples: 3"
assert_status "duplicate samples directive token rejected" 2
assert_match "duplicate samples directive token explains ambiguity" "$RUN_STDERR" 'duplicate samples directive token is ambiguous'
assert_eq "duplicate samples directive token emits no stdout" "$RUN_STDOUT" ""

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "passes: 2 passes: 3"
assert_status "duplicate passes directive token rejected" 2
assert_match "duplicate passes directive token explains ambiguity" "$RUN_STDERR" 'duplicate passes directive token is ambiguous'
assert_eq "duplicate passes directive token emits no stdout" "$RUN_STDOUT" ""

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "mode: on mode: off"
assert_status "duplicate mode directive token rejected" 2
assert_match "duplicate mode directive token explains ambiguity" "$RUN_STDERR" 'duplicate mode directive token is ambiguous'
assert_eq "duplicate mode directive token emits no stdout" "$RUN_STDOUT" ""

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "teeth: warn teeth: blocking"
assert_status "duplicate teeth directive token rejected" 2
assert_match "duplicate teeth directive token explains ambiguity" "$RUN_STDERR" 'duplicate teeth directive token is ambiguous'
assert_eq "duplicate teeth directive token emits no stdout" "$RUN_STDOUT" ""

# 10. Invalid counts fail at every canonical input layer and through the alias.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 0
assert_status "zero flag samples rejected" 1
assert_match "zero flag samples diagnostic" "$RUN_STDERR" "invalid samples '0'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples abc
assert_status "nonnumeric flag samples rejected" 1
assert_match "nonnumeric flag samples diagnostic" "$RUN_STDERR" "invalid samples 'abc'"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=0 bash "$RESOLVE" --repo-root "$empty_repo"
assert_status "zero env samples rejected" 1
assert_match "zero env samples diagnostic" "$RUN_STDERR" "invalid BUBBLES_ADVERSARIAL_SAMPLES '0'"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=0 bash "$RESOLVE" --repo-root "$config_repo"
assert_status "selected invalid env samples rejected over valid config" 1
assert_match "selected invalid env samples retains env diagnostic" "$RUN_STDERR" "invalid BUBBLES_ADVERSARIAL_SAMPLES '0'"
assert_eq "selected invalid env samples emits no stdout" "$RUN_STDOUT" ""

run_resolver bash "$RESOLVE" --repo-root "$config_invalid_repo"
assert_status "zero config samples rejected" 1
assert_match "zero config samples diagnostic" "$RUN_STDERR" "invalid config adversarial.samples '0'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
assert_status "zero directive samples rejected" 1
assert_match "zero directive samples diagnostic" "$RUN_STDERR" "invalid samples '0'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes 0
assert_status "zero passes alias rejected" 1
assert_match "zero passes alias diagnostic" "$RUN_STDERR" "invalid passes alias '0'"

# 11. BUG-014 malformed free-form values remain complete selected lexemes. They
# must fail validation rather than becoming a valid prefix or disappearing.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples:2.5"
assert_rejected_value "decimal directive samples" "invalid samples '2.5'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples:-2"
assert_rejected_value "signed directive samples" "invalid samples '-2'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "mode:on-call"
assert_rejected_value "suffixed directive mode" "invalid mode 'on-call'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "teeth:blocking-ish"
assert_rejected_value "suffixed directive teeth" "invalid teeth 'blocking-ish'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples:2|3"
assert_rejected_value "delimiter-shaped directive samples" \
  "invalid samples '2\|3'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "mode:on|call"
assert_rejected_value "delimiter-shaped directive mode" \
  "invalid mode 'on\|call'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "teeth:blocking|ish"
assert_rejected_value "delimiter-shaped directive teeth" \
  "invalid teeth 'blocking\|ish'"

# 12. BUG-014 duplicate directive-layer inputs are usage errors in both orders.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: 2" --directive "samples: 3"
assert_usage_ambiguity "duplicate --directive forward order" \
  'duplicate --directive flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: 3" --directive "samples: 2"
assert_usage_ambiguity "duplicate --directive reverse order" \
  'duplicate --directive flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --mode on --mode off
assert_usage_ambiguity "duplicate --mode forward order" \
  'duplicate --mode flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --mode off --mode on
assert_usage_ambiguity "duplicate --mode reverse order" \
  'duplicate --mode flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --teeth blocking --teeth warn
assert_usage_ambiguity "duplicate --teeth forward order" \
  'duplicate --teeth flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --teeth warn --teeth blocking
assert_usage_ambiguity "duplicate --teeth reverse order" \
  'duplicate --teeth flag is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "adversarial: on mode: off"
assert_usage_ambiguity "adversarial then mode synonym collision" \
  'duplicate mode directive token is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: off adversarial: on"
assert_usage_ambiguity "mode then adversarial synonym collision" \
  'duplicate mode directive token is ambiguous'

# 13. BUG-014 binds every selected canonical and deprecated count layer to the
# workflows registry range 1..5. Both boundaries remain valid.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 1
assert_status "minimum flag samples exits zero" 0
assert_line "minimum flag samples resolves one" "$RUN_STDOUT" "samples=1"
assert_line "minimum flag samples records directive source" "$RUN_STDOUT" "samplesSource=directive"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 5
assert_status "maximum flag samples exits zero" 0
assert_line "maximum flag samples resolves five" "$RUN_STDOUT" "samples=5"
assert_line "maximum flag samples records directive source" "$RUN_STDOUT" "samplesSource=directive"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 6
assert_rejected_value "over-max flag samples" "invalid samples '6'"
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples 1000000000
assert_rejected_value "huge flag samples" "invalid samples '1000000000'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 6"
assert_rejected_value "over-max directive samples" "invalid samples '6'"
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: 1000000000"
assert_rejected_value "huge directive samples" "invalid samples '1000000000'"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=6 \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "over-max env samples" \
  "invalid BUBBLES_ADVERSARIAL_SAMPLES '6'"
run_resolver BUBBLES_ADVERSARIAL_SAMPLES=1000000000 \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "huge env samples" \
  "invalid BUBBLES_ADVERSARIAL_SAMPLES '1000000000'"

run_resolver bash "$RESOLVE" --repo-root "$config_over_max_repo"
assert_rejected_value "over-max config samples" \
  "invalid config adversarial.samples '6'"
run_resolver bash "$RESOLVE" --repo-root "$config_huge_repo"
assert_rejected_value "huge config samples" \
  "invalid config adversarial.samples '1000000000'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes 6
assert_rejected_value "over-max flag passes alias" "invalid passes alias '6'"
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes 1000000000
assert_rejected_value "huge flag passes alias" \
  "invalid passes alias '1000000000'"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "passes: 6"
assert_rejected_value "over-max directive passes alias" \
  "invalid passes alias '6'"
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "passes: 1000000000"
assert_rejected_value "huge directive passes alias" \
  "invalid passes alias '1000000000'"

run_resolver BUBBLES_ADVERSARIAL_PASSES=6 \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "over-max env passes alias" \
  "invalid BUBBLES_ADVERSARIAL_PASSES alias '6'"
run_resolver BUBBLES_ADVERSARIAL_PASSES=1000000000 \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "huge env passes alias" \
  "invalid BUBBLES_ADVERSARIAL_PASSES alias '1000000000'"

run_resolver bash "$RESOLVE" --repo-root "$config_alias_over_max_repo"
assert_rejected_value "over-max config passes alias" \
  "invalid config adversarial.passes alias '6'"
run_resolver bash "$RESOLVE" --repo-root "$config_alias_huge_repo"
assert_rejected_value "huge config passes alias" \
  "invalid config adversarial.passes alias '1000000000'"

# 14. BUG-014 preserves explicit empty canonical samples as selected invalid
# input. Canonical samples still wins over passes, while true absence defaults.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --samples ""
assert_rejected_value "explicit empty samples" "invalid samples ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --samples "" --passes 2
assert_rejected_value "explicit empty samples over passes alias" \
  "invalid samples ''"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 \
  bash "$RESOLVE" --repo-root "$empty_repo" --samples ""
assert_rejected_value "explicit empty samples over env" "invalid samples ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo"
assert_status "true sample absence still exits zero" 0
assert_line "true sample absence still defaults count" "$RUN_STDOUT" "samples=1"
assert_line "true sample absence still records default source" \
  "$RUN_STDOUT" "samplesSource=default"

# 15. BUG014-REG-EMPTY-PRESENCE covers explicit empty values as selected input
# at directive, env, and config layers. Canonical fields and the deprecated
# passes alias retain field-specific diagnostics and never emit partial output.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --mode ""
assert_rejected_value "empty mode flag" "invalid mode ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --teeth ""
assert_rejected_value "empty teeth flag" "invalid teeth ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --passes ""
assert_rejected_value "empty passes flag alias" "invalid passes alias ''"
assert_not_match "empty passes flag alias cannot deprecate into success" \
  "$RUN_STDOUT" '^deprecation=passes-alias$'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "mode:"
assert_rejected_value "empty mode directive token" "invalid mode ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples:"
assert_rejected_value "empty samples directive token" "invalid samples ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "passes:"
assert_rejected_value "empty passes directive token alias" \
  "invalid passes alias ''"
assert_not_match "empty passes directive alias cannot deprecate into success" \
  "$RUN_STDOUT" '^deprecation=passes-alias$'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive "teeth:"
assert_rejected_value "empty teeth directive token" "invalid teeth ''"

run_resolver BUBBLES_ADVERSARIAL= \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "empty env mode" "invalid mode ''"

run_resolver BUBBLES_ADVERSARIAL_SAMPLES= \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "empty env samples" \
  "invalid BUBBLES_ADVERSARIAL_SAMPLES ''"

run_resolver BUBBLES_ADVERSARIAL_PASSES= \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "empty env passes alias" \
  "invalid BUBBLES_ADVERSARIAL_PASSES alias ''"
assert_not_match "empty env passes alias cannot deprecate into success" \
  "$RUN_STDOUT" '^deprecation=passes-alias$'

run_resolver BUBBLES_ADVERSARIAL_TEETH= \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "empty env teeth" "invalid teeth ''"

run_resolver bash "$RESOLVE" --repo-root "$config_empty_mode_repo"
assert_rejected_value "empty config mode" "invalid mode ''"

run_resolver bash "$RESOLVE" --repo-root "$config_empty_samples_repo"
assert_rejected_value "empty config samples" \
  "invalid config adversarial.samples ''"

run_resolver bash "$RESOLVE" --repo-root "$config_empty_passes_repo"
assert_rejected_value "empty config passes alias" \
  "invalid config adversarial.passes alias ''"
assert_not_match "empty config passes alias cannot deprecate into success" \
  "$RUN_STDOUT" '^deprecation=passes-alias$'

run_resolver bash "$RESOLVE" --repo-root "$config_empty_teeth_repo"
assert_rejected_value "empty config teeth" "invalid teeth ''"

# 15a. Empty canonical samples remains selected over a valid same-layer or
# lower-layer passes alias. Presence is precedence; emptiness never falls
# through to a valid compatibility value.
run_resolver BUBBLES_ADVERSARIAL_SAMPLES= BUBBLES_ADVERSARIAL_PASSES=2 \
  bash "$RESOLVE" --repo-root "$empty_repo"
assert_rejected_value "empty env samples over same-layer alias" \
  "invalid BUBBLES_ADVERSARIAL_SAMPLES ''"

run_resolver BUBBLES_ADVERSARIAL_PASSES=2 \
  bash "$RESOLVE" --repo-root "$empty_repo" --samples ""
assert_rejected_value "empty directive samples over lower env alias" \
  "invalid samples ''"

run_resolver bash "$RESOLVE" --repo-root "$config_empty_samples_alias_repo"
assert_rejected_value "empty config samples over same-layer alias" \
  "invalid config adversarial.samples ''"

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: passes:2"
assert_rejected_value "empty directive samples over adjacent alias" \
  "invalid samples ''"

# 15b. Valid higher canonical values shadow empty lower canonical values
# without selecting or validating them. Empty shadowed aliases remain
# observable deprecation metadata in deterministic layer order.
run_resolver BUBBLES_ADVERSARIAL= BUBBLES_ADVERSARIAL_SAMPLES= \
  BUBBLES_ADVERSARIAL_TEETH= \
  bash "$RESOLVE" --repo-root "$config_empty_canonical_repo" \
  --mode on --samples 3 --teeth blocking
assert_status "valid directive canonicals shadow empty lower canonicals" 0
assert_line "shadowed empty mode keeps directive mode" "$RUN_STDOUT" "mode=on"
assert_line "shadowed empty mode keeps directive source" \
  "$RUN_STDOUT" "source=directive"
assert_line "shadowed empty samples keeps directive count" \
  "$RUN_STDOUT" "samples=3"
assert_line "shadowed empty samples keeps directive source" \
  "$RUN_STDOUT" "samplesSource=directive"
assert_line "shadowed empty teeth keeps directive teeth" \
  "$RUN_STDOUT" "teeth=blocking"
assert_line "shadowed empty canonicals are not deprecations" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "shadowed empty canonicals emit no error" "$RUN_STDERR" ""

run_resolver BUBBLES_ADVERSARIAL=on BUBBLES_ADVERSARIAL_SAMPLES=5 \
  BUBBLES_ADVERSARIAL_TEETH=blocking \
  bash "$RESOLVE" --repo-root "$config_empty_canonical_repo"
assert_status "valid env canonicals shadow empty config canonicals" 0
assert_line "env mode shadows empty config mode" "$RUN_STDOUT" "mode=on"
assert_line "env mode owns shadowed config mode result" \
  "$RUN_STDOUT" "source=env"
assert_line "env samples shadows empty config samples" \
  "$RUN_STDOUT" "samples=5"
assert_line "env samples owns shadowed config samples result" \
  "$RUN_STDOUT" "samplesSource=env"
assert_line "env teeth shadows empty config teeth" \
  "$RUN_STDOUT" "teeth=blocking"
assert_line "shadowed empty config canonicals have no deprecation" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "env canonicals over empty config canonicals emit no error" \
  "$RUN_STDERR" ""

run_resolver BUBBLES_ADVERSARIAL_PASSES= \
  bash "$RESOLVE" --repo-root "$config_empty_alias_repo" --samples 3
assert_shadowed_alias_result \
  "valid directive samples over empty env and config aliases" \
  3 directive env,config

run_resolver BUBBLES_ADVERSARIAL_SAMPLES=5 \
  bash "$RESOLVE" --repo-root "$config_empty_alias_repo"
assert_shadowed_alias_result \
  "valid env samples over empty config alias" 5 env config

run_resolver bash "$RESOLVE" --repo-root "$config_empty_samples_repo" \
  --samples 3
assert_status "valid directive samples shadows empty config canonical" 0
assert_line "shadowed empty config canonical keeps count" \
  "$RUN_STDOUT" "samples=3"
assert_line "shadowed empty config canonical keeps source" \
  "$RUN_STDOUT" "samplesSource=directive"
assert_line "shadowed empty config canonical has no deprecation" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "shadowed empty config canonical emits no diagnostic" \
  "$RUN_STDERR" ""

# 15c. An empty exact directive key never consumes the following exact key.
# The empty selected field owns the failure even when the next field is valid.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: teeth:blocking"
assert_rejected_value "empty samples before teeth" "invalid samples ''"
assert_not_match "empty samples does not fail as teeth" \
  "$RUN_STDERR" 'invalid teeth'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: samples:3"
assert_rejected_value "empty mode before samples" "invalid mode ''"
assert_not_match "empty mode does not consume samples" \
  "$RUN_STDERR" 'invalid samples'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "passes: teeth:blocking"
assert_rejected_value "empty passes before teeth" "invalid passes alias ''"
assert_not_match "empty passes does not fail as teeth" \
  "$RUN_STDERR" 'invalid teeth'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "teeth: mode:on"
assert_rejected_value "empty teeth before mode" "invalid teeth ''"
assert_not_match "empty teeth does not consume mode" \
  "$RUN_STDERR" 'invalid mode'

# 15d. Empty exact tokens remain distinct from absence and non-key prose.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" --directive ""
assert_status "empty directive string is absence" 0
assert_line "empty directive string keeps default samples" \
  "$RUN_STDOUT" "samples=1"
assert_line "empty directive string keeps default mode" "$RUN_STDOUT" "mode=off"
assert_eq "empty directive string emits no warning" "$RUN_STDERR" ""

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples without a separator"
assert_status "non-key samples prose is absence" 0
assert_line "non-key samples prose keeps default samples" \
  "$RUN_STDOUT" "samples=1"
assert_line "non-key samples prose has no deprecation" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "non-key samples prose emits no warning" "$RUN_STDERR" ""

# 15e. Duplicate empty exact directive keys remain usage ambiguities.
run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: samples:"
assert_usage_ambiguity "duplicate empty samples tokens" \
  'duplicate samples directive token is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "passes: passes:"
assert_usage_ambiguity "duplicate empty passes tokens" \
  'duplicate passes directive token is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: adversarial:"
assert_usage_ambiguity "duplicate empty mode synonym tokens" \
  'duplicate mode directive token is ambiguous'

run_resolver bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "teeth: teeth:"
assert_usage_ambiguity "duplicate empty teeth tokens" \
  'duplicate teeth directive token is ambiguous'

# 15f. YAML empty strings are present invalid values, while explicit null and
# missing keys remain absent and resolve through the normal defaults.
run_resolver bash "$RESOLVE" --repo-root "$config_null_repo"
assert_status "null config values are absent" 0
assert_line "null config mode keeps default" "$RUN_STDOUT" "mode=off"
assert_line "null config samples keeps default" "$RUN_STDOUT" "samples=1"
assert_line "null config teeth keeps default" "$RUN_STDOUT" "teeth=warn"
assert_line "null config mode source is default" "$RUN_STDOUT" "source=default"
assert_line "null config samples source is default" \
  "$RUN_STDOUT" "samplesSource=default"
assert_line "null config alias has no deprecation" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "null config values emit no warning" "$RUN_STDERR" ""

run_resolver bash "$RESOLVE" --repo-root "$empty_repo"
assert_status "missing config values remain absent" 0
assert_line "missing config mode keeps default" "$RUN_STDOUT" "mode=off"
assert_line "missing config samples keeps default" "$RUN_STDOUT" "samples=1"
assert_line "missing config teeth keeps default" "$RUN_STDOUT" "teeth=warn"
assert_line "missing config alias has no deprecation" \
  "$RUN_STDOUT" "deprecation=none"
assert_eq "missing config values emit no warning" "$RUN_STDERR" ""

# 16. Bypass-shaped flags do not exist.
for bypass_flag in --force --skip --ignore; do
  run_resolver bash "$RESOLVE" --repo-root "$empty_repo" "$bypass_flag"
  assert_status "$bypass_flag rejected" 2
  assert_match "$bypass_flag reported unknown" "$RUN_STDERR" "unknown option: $bypass_flag"
done

# Preserve the complete F1-F4 and BUG-010 matrix before extending it with the
# BUG014-F5/F6 hardening cases below.
assert_eq "pre-F5/F6 matrix retains all 463 existing assertions" \
  "$pass:$fail" "463:0"

count_parser_records() {
  local parser_dir="$1"
  local record
  local count=0

  for record in "$parser_dir"/bubbles-adversarial-resolve.*; do
    [[ -e "$record" || -L "$record" ]] || continue
    count=$((count + 1))
  done
  printf '%s\n' "$count"
}

# 17. BUG014-F6: a parser failure is a stable usage failure, never token
# absence followed by the default posture. The injected awk is parser-only:
# this case uses the already-created empty fixture and therefore never invokes
# the yq fixture shim.
fault_bin="$tmp/fault-bin"
fault_parser_tmp="$tmp/fault-parser-tmp"
mkdir -p "$fault_bin" "$fault_parser_tmp"
cat > "$fault_bin/awk" <<'AWK_FAILURE'
#!/usr/bin/env bash
echo "raw injected awk parser failure" >&2
exit 42
AWK_FAILURE
chmod +x "$fault_bin/awk"

base_test_path="$TEST_PATH"
fault_records_before="$(count_parser_records "$fault_parser_tmp")"
TEST_PATH="$fault_bin:$base_test_path"
run_resolver TMPDIR="$fault_parser_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples:0"
TEST_PATH="$base_test_path"
fault_records_after="$(count_parser_records "$fault_parser_tmp")"

assert_status "injected parser failure exits two" 2
assert_eq "injected parser failure emits no posture stdout" "$RUN_STDOUT" ""
assert_eq "injected parser failure has stable stderr" "$RUN_STDERR" \
  "adversarial-resolve: directive parser failed"
assert_not_match "injected parser failure cannot return fallback posture" \
  "$RUN_STDOUT" '(^samples=1$|^deprecation=none$)'
assert_eq "injected parser failure preserves parser-record count" \
  "$fault_records_after" "$fault_records_before"
assert_eq "injected parser failure leaves no parser record" \
  "$fault_records_after" "0"

# 17a. BUG014-F6: parser-record creation and post-production consumption are
# independent checked operations. Every shim targets only the parser record
# operation named by the case and delegates all unrelated commands.
mktemp_failure_bin="$tmp/mktemp-failure-bin"
mktemp_failure_tmp="$tmp/mktemp-failure-tmp"
mkdir -p "$mktemp_failure_bin" "$mktemp_failure_tmp"
cat > "$mktemp_failure_bin/mktemp" <<'MKTEMP_FAILURE'
#!/usr/bin/env bash
set -u

: "${BUG014_REAL_MKTEMP:?missing real mktemp path}"
case "${1:-}" in
  */bubbles-adversarial-resolve.XXXXXXXX)
    echo "raw injected mktemp parser failure: ${1:-missing-path}" >&2
    exit 71
    ;;
esac
exec "$BUG014_REAL_MKTEMP" "$@"
MKTEMP_FAILURE
chmod +x "$mktemp_failure_bin/mktemp"

TEST_PATH="$mktemp_failure_bin:$base_test_path"
run_resolver TMPDIR="$mktemp_failure_tmp" \
  BUG014_REAL_MKTEMP="$REAL_MKTEMP" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 3"
TEST_PATH="$base_test_path"
assert_status "mktemp parser-record creation failure exits two" 2
assert_eq "mktemp parser-record creation failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "mktemp parser-record creation failure has stable stderr" \
  "$RUN_STDERR" "adversarial-resolve: directive parser failed"
assert_not_match "mktemp parser-record creation failure emits no default posture" \
  "$RUN_STDOUT" '(^samples=1$|^deprecation=none$)'
assert_eq "mktemp parser-record creation failure leaves no record" \
  "$(count_parser_records "$mktemp_failure_tmp")" "0"

record_fault_bin="$tmp/record-fault-bin"
record_fault_tmp="$tmp/record-fault-tmp"
mkdir -p "$record_fault_bin" "$record_fault_tmp"
cat > "$record_fault_bin/awk" <<'AWK_RECORD_FAULT'
#!/usr/bin/env bash
set -u

: "${BUG014_RECORD_FAULT:?missing record fault}"
: "${BUG014_RECORD_DIR:?missing record directory}"
: "${BUG014_REAL_AWK:?missing real awk path}"
: "${BUG014_REAL_CAT:?missing real cat path}"
: "${BUG014_REAL_CHMOD:?missing real chmod path}"
: "${BUG014_REAL_LN:?missing real ln path}"
: "${BUG014_REAL_MKFIFO:?missing real mkfifo path}"
: "${BUG014_REAL_RM:?missing real rm path}"
: "${BUG014_FIFO_WRITER:?missing FIFO writer path}"
: "${BUG014_WRITER_BASH:?missing FIFO writer Bash path}"

emit_fault_record() {
  case "$BUG014_RECORD_FAULT" in
    no-tab) printf 'samples 3\n' ;;
    multiple-tab) printf 'samples\t3\textra\n' ;;
    unknown-key) printf 'unknown\t3\n' ;;
    incomplete) printf 'samples\t3' ;;
    cr-bearing) printf 'samples\t3\r\n' ;;
    control-bearing) printf 'samples\t3\001\n' ;;
    *) return 1 ;;
  esac
}

case "$BUG014_RECORD_FAULT" in
  no-tab|multiple-tab|unknown-key|incomplete|cr-bearing|control-bearing)
    emit_fault_record
    exit 0
    ;;
esac

awk_output="$BUG014_RECORD_DIR/.bug014-awk-output.$$"
"$BUG014_REAL_AWK" "$@" > "$awk_output"
awk_status=$?
if [[ "$awk_status" -ne 0 ]]; then
  "$BUG014_REAL_RM" -f "$awk_output"
  exit "$awk_status"
fi
"$BUG014_REAL_CAT" "$awk_output"
"$BUG014_REAL_RM" -f "$awk_output"

record_path=""
for candidate in "$BUG014_RECORD_DIR"/bubbles-adversarial-resolve.*; do
  [[ -e "$candidate" || -L "$candidate" ]] || continue
  record_path="$candidate"
  break
done
[[ -n "$record_path" ]] || exit 98

case "$BUG014_RECORD_FAULT" in
  unlink)
    "$BUG014_REAL_RM" -f "$record_path"
    ;;
  unreadable)
    "$BUG014_REAL_CHMOD" 000 "$record_path"
    ;;
  symlink)
    link_target="$BUG014_RECORD_DIR/.bug014-link-target.$$"
    printf 'samples\t3\n' > "$link_target"
    "$BUG014_REAL_RM" -f "$record_path"
    "$BUG014_REAL_LN" -s "$link_target" "$record_path"
    ;;
  fifo)
    "$BUG014_REAL_RM" -f "$record_path"
    "$BUG014_REAL_MKFIFO" "$record_path"
    "$BUG014_WRITER_BASH" "$BUG014_FIFO_WRITER" start \
      "$BUG014_RECORD_DIR" "$record_path" \
      "$BUG014_RECORD_DIR/.bug014-fifo-writer.pid" >/dev/null
    ;;
  *) exit 99 ;;
esac
AWK_RECORD_FAULT
chmod +x "$record_fault_bin/awk"

assert_f6_parser_failure() { # label parser-dir records-before
  local label="$1"
  local parser_dir="$2"
  local records_before="$3"
  local records_after
  records_after="$(count_parser_records "$parser_dir")"

  assert_status "$label exits two" 2
  assert_eq "$label emits no posture stdout" "$RUN_STDOUT" ""
  assert_eq "$label has exact stable stderr" \
    "$RUN_STDERR" "adversarial-resolve: directive parser failed"
  assert_not_match "$label emits no default or deprecation posture" \
    "$RUN_STDOUT" '(^samples=1$|^deprecation=)'
  assert_eq "$label preserves parser-record count" \
    "$records_after" "$records_before"
  assert_eq "$label leaves no parser record" "$records_after" "0"
}

f6_unlinked_fifo_writer_after="not-run"
for record_fault in unlink unreadable symlink fifo; do
  record_fault_case_tmp="$record_fault_tmp/$record_fault"
  mkdir -p "$record_fault_case_tmp"
  record_fault_before="$(count_parser_records "$record_fault_case_tmp")"
  TEST_PATH="$record_fault_bin:$base_test_path"
  if [[ "$record_fault" == "unlink" ]]; then
    run_resolver TMPDIR="$record_fault_case_tmp" \
      BUG014_RECORD_FAULT="$record_fault" \
      BUG014_RECORD_DIR="$record_fault_case_tmp" \
      BUG014_REAL_AWK="$REAL_AWK" BUG014_REAL_CAT="$REAL_CAT" \
      BUG014_REAL_CHMOD="$REAL_CHMOD" BUG014_REAL_LN="$REAL_LN" \
      BUG014_REAL_MKFIFO="$REAL_MKFIFO" BUG014_REAL_RM="$REAL_RM" \
      BUG014_FIFO_WRITER="$HARNESS_FIFO_WRITER" BUG014_WRITER_BASH="$BASH" \
      BUBBLES_ADVERSARIAL=auto BUBBLES_ADVERSARIAL_SAMPLES=5 \
      BUBBLES_ADVERSARIAL_TEETH=warn \
      bash "$RESOLVE" --repo-root "$config_repo" \
      --directive "mode: on samples: 3 teeth: blocking"
  else
    run_resolver TMPDIR="$record_fault_case_tmp" \
      BUG014_RECORD_FAULT="$record_fault" \
      BUG014_RECORD_DIR="$record_fault_case_tmp" \
      BUG014_REAL_AWK="$REAL_AWK" BUG014_REAL_CAT="$REAL_CAT" \
      BUG014_REAL_CHMOD="$REAL_CHMOD" BUG014_REAL_LN="$REAL_LN" \
      BUG014_REAL_MKFIFO="$REAL_MKFIFO" BUG014_REAL_RM="$REAL_RM" \
      BUG014_FIFO_WRITER="$HARNESS_FIFO_WRITER" BUG014_WRITER_BASH="$BASH" \
      bash "$RESOLVE" --repo-root "$empty_repo" \
      --directive "mode: on samples: 3 teeth: blocking"
  fi
  TEST_PATH="$base_test_path"
  assert_f6_parser_failure \
    "post-AWK $record_fault parser-record fault" \
    "$record_fault_case_tmp" "$record_fault_before"
  if [[ -f "$record_fault_case_tmp/.bug014-fifo-writer.pid" ]]; then
    fifo_writer_pid=""
    if [[ "$record_fault" == "fifo" ]]; then
      {
        IFS= read -r fifo_metadata_version
        IFS= read -r fifo_writer_pid
      } < "$record_fault_case_tmp/.bug014-fifo-writer.pid" || fifo_writer_pid=""
      [[ "$fifo_metadata_version" == "version=1" && "$fifo_writer_pid" == pid=* ]] || \
        fifo_writer_pid=""
      fifo_writer_pid="${fifo_writer_pid#pid=}"
      [[ "$fifo_writer_pid" =~ ^[1-9][0-9]*$ ]] || fifo_writer_pid=""
    fi
    cleanup_fifo_writers "$record_fault_case_tmp"
    if [[ "$record_fault" == "fifo" ]]; then
      if [[ -n "$fifo_writer_pid" ]] && kill -0 "$fifo_writer_pid" 2>/dev/null; then
        f6_unlinked_fifo_writer_after="alive"
      else
        f6_unlinked_fifo_writer_after="not-alive"
      fi
      assert_eq "post-AWK fifo cleanup terminates writer after FIFO unlink" \
        "$f6_unlinked_fifo_writer_after" "not-alive"
    fi
  fi
  "$REAL_RM" -f "$record_fault_case_tmp"/.bug014-link-target.*
done

# F6 lifecycle cleanup is exercised under the system and default Bash roles.
# Separate ready/control FIFOs prove the writer exists before each outcome and
# hold the harness before signal delivery without timing sleeps.
f6_default_bash="$BASH"
for f6_bash_candidate in /opt/homebrew/bin/bash /usr/local/bin/bash; do
  if [[ -x "$f6_bash_candidate" && "$f6_bash_candidate" != "/bin/bash" ]]; then
    f6_default_bash="$f6_bash_candidate"
    break
  fi
done

f6_lifecycle_cases=0
f6_lifecycle_ready_failures=0
f6_lifecycle_status_failures=0
f6_lifecycle_sentinel_failures=0
f6_lifecycle_writer_before_failures=0
f6_lifecycle_writer_after_failures=0
f6_lifecycle_fixture_failures=0
f6_lifecycle_pid_residue_failures=0
f6_lifecycle_timeout_failures=0
f6_lifecycle_idempotency_failures=0
f6_lifecycle_status_rows=""
f6_harness_pid=""

launch_f6_harness() {
  local harness_bash="$1"
  local cleanup_root="$2"
  local ready_fifo="$3"
  local control_fifo="$4"
  local sentinel="$5"
  local action="$6"
  local monitor_was_on=0

  case "$-" in
    *m*) monitor_was_on=1 ;;
    *) set -m ;;
  esac
  BUG014_F6_LIFECYCLE_CHILD=1 \
    BUG014_F6_WRITER_SCRIPT="$HARNESS_FIFO_WRITER" \
    BUG014_F6_CLEANUP_ROOT="$cleanup_root" \
    BUG014_F6_READY_FIFO="$ready_fifo" \
    BUG014_F6_CONTROL_FIFO="$control_fifo" \
    BUG014_F6_SENTINEL="$sentinel" \
    BUG014_F6_ACTION="$action" \
    "$harness_bash" "$SELF" &
  f6_harness_pid=$!
  if [[ "$monitor_was_on" -eq 0 ]]; then
    set +m
  fi
}

run_f6_lifecycle_case() {
  local shell_role="$1"
  local harness_bash="$2"
  local action="$3"
  local expected_status="$4"
  local case_root="$tmp/f6-lifecycle-$shell_role-${action}-root"
  local ready_fifo="$tmp/f6-lifecycle-$shell_role-${action}-ready"
  local control_fifo="$tmp/f6-lifecycle-$shell_role-${action}-control"
  local sentinel="$tmp/f6-lifecycle-$shell_role-${action}-sentinel"
  local timeout_marker="$tmp/f6-lifecycle-$shell_role-${action}-timeout"
  local writer_pid=""
  local ready_status=0
  local harness_status=0
  local watchdog_pid=""

  "$REAL_MKFIFO" "$ready_fifo" "$control_fifo"
  launch_f6_harness "$harness_bash" "$case_root" "$ready_fifo" \
    "$control_fifo" "$sentinel" "$action"
  writer_pid="$(bubbles_run_with_timeout 5 "$REAL_CAT" "$ready_fifo" 2>/dev/null)"
  ready_status=$?
  f6_lifecycle_cases=$((f6_lifecycle_cases + 1))
  if [[ "$ready_status" -ne 0 || ! "$writer_pid" =~ ^[1-9][0-9]*$ ]]; then
    f6_lifecycle_ready_failures=$((f6_lifecycle_ready_failures + 1))
  fi
  if [[ "$writer_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$writer_pid" 2>/dev/null; then
    :
  else
    f6_lifecycle_writer_before_failures=$((f6_lifecycle_writer_before_failures + 1))
  fi

  if [[ "$ready_status" -eq 0 && "$writer_pid" =~ ^[1-9][0-9]*$ ]]; then
    if [[ "$action" == "EXIT" ]]; then
      printf 'exit\n' > "$control_fifo"
    else
      kill -"$action" "$f6_harness_pid" 2>/dev/null || true
    fi
  fi

  (
    if ! bubbles_run_with_timeout 5 sh -c \
      "while kill -0 \"\$1\" 2>/dev/null; do :; done" sh "$f6_harness_pid" \
      >/dev/null 2>&1; then
      : > "$timeout_marker"
      kill -KILL "$f6_harness_pid" 2>/dev/null || true
    fi
  ) &
  watchdog_pid=$!
  wait "$f6_harness_pid" 2>/dev/null
  harness_status=$?
  wait "$watchdog_pid" 2>/dev/null || true

  if [[ -n "$f6_lifecycle_status_rows" ]]; then
    f6_lifecycle_status_rows="$f6_lifecycle_status_rows,"
  fi
  f6_lifecycle_status_rows="$f6_lifecycle_status_rows$shell_role-$action:$harness_status"

  [[ "$harness_status" -eq "$expected_status" ]] || \
    f6_lifecycle_status_failures=$((f6_lifecycle_status_failures + 1))
  [[ ! -e "$sentinel" ]] || \
    f6_lifecycle_sentinel_failures=$((f6_lifecycle_sentinel_failures + 1))
  if [[ "$writer_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$writer_pid" 2>/dev/null; then
    f6_lifecycle_writer_after_failures=$((f6_lifecycle_writer_after_failures + 1))
  fi
  [[ ! -e "$case_root" ]] || \
    f6_lifecycle_fixture_failures=$((f6_lifecycle_fixture_failures + 1))
  [[ ! -e "$case_root/.bug014-fifo-writer.pid" ]] || \
    f6_lifecycle_pid_residue_failures=$((f6_lifecycle_pid_residue_failures + 1))
  [[ ! -e "$timeout_marker" ]] || \
    f6_lifecycle_timeout_failures=$((f6_lifecycle_timeout_failures + 1))
  cleanup_fifo_writers "$case_root" || \
    f6_lifecycle_idempotency_failures=$((f6_lifecycle_idempotency_failures + 1))
  cleanup_fifo_writers "$case_root" || \
    f6_lifecycle_idempotency_failures=$((f6_lifecycle_idempotency_failures + 1))
  if [[ "$writer_pid" =~ ^[1-9][0-9]*$ ]]; then
    kill -KILL "$writer_pid" 2>/dev/null || true
  fi
  "$REAL_RM" -rf "$case_root"
  "$REAL_RM" -f "$ready_fifo" "$control_fifo" "$sentinel" "$timeout_marker"
}

for f6_shell_role in system default; do
  case "$f6_shell_role" in
    system) f6_harness_bash="/bin/bash" ;;
    default) f6_harness_bash="$f6_default_bash" ;;
  esac
  run_f6_lifecycle_case "$f6_shell_role" "$f6_harness_bash" EXIT 37
  run_f6_lifecycle_case "$f6_shell_role" "$f6_harness_bash" INT 130
  run_f6_lifecycle_case "$f6_shell_role" "$f6_harness_bash" TERM 143
done

assert_eq "F6 lifecycle matrix runs EXIT INT TERM under two Bash roles" \
  "$f6_lifecycle_cases" "6"
assert_eq "F6 lifecycle matrix synchronizes every blocked writer" \
  "$f6_lifecycle_ready_failures:$f6_lifecycle_writer_before_failures" "0:0"
assert_eq "F6 lifecycle matrix preserves EXIT INT TERM statuses" \
  "$f6_lifecycle_status_failures" "0"
assert_eq "F6 INT and TERM execute no post-signal sentinel" \
  "$f6_lifecycle_sentinel_failures" "0"
assert_eq "F6 EXIT INT TERM terminate every owned writer" \
  "$f6_lifecycle_writer_after_failures" "0"
assert_eq "F6 lifecycle cleanup removes fixtures and PID metadata" \
  "$f6_lifecycle_fixture_failures:$f6_lifecycle_pid_residue_failures" "0:0"
assert_eq "F6 lifecycle matrix needs no watchdog recovery" \
  "$f6_lifecycle_timeout_failures" "0"
assert_eq "F6 lifecycle cleanup remains idempotent" \
  "$f6_lifecycle_idempotency_failures" "0"

# A reused PID with the cleanup root only as an inert argv value must survive
# both legacy PID-only metadata and a well-formed stale writer identity.
reused_pid_root="$tmp/f6-reused-pid-root"
reused_pid_ready="$tmp/f6-reused-pid-ready"
reused_pid_block="$tmp/f6-reused-pid-block"
reused_pid_metadata="$reused_pid_root/.bug014-fifo-writer.pid"
reused_pid_fifo="$reused_pid_root/stale-writer.fifo"
reused_pid_ready_identity="$reused_pid_root/.bug014-fifo-writer.ready.stale.1"
mkdir -p "$reused_pid_root"
"$REAL_MKFIFO" "$reused_pid_ready" "$reused_pid_block" "$reused_pid_fifo"
"$BASH" "$tmp/bin/bug014-unrelated-blocker.sh" \
  "$reused_pid_ready" "$reused_pid_block" "$reused_pid_root/inert-only" &
unrelated_pid=$!
IFS= read -r unrelated_ready < "$reused_pid_ready"
unrelated_command="$(ps -ww -p "$unrelated_pid" -o command= 2>/dev/null)"
case "$unrelated_command" in
  *"$reused_pid_root/inert-only"*) unrelated_has_inert_root="yes" ;;
  *) unrelated_has_inert_root="no" ;;
esac
if kill -0 "$unrelated_pid" 2>/dev/null; then
  unrelated_before="alive"
else
  unrelated_before="not-alive"
fi

printf '%s\n' "$unrelated_pid" > "$reused_pid_metadata"
cleanup_fifo_writers "$reused_pid_root"
if kill -0 "$unrelated_pid" 2>/dev/null; then
  unrelated_after_pid_only="alive"
else
  unrelated_after_pid_only="not-alive"
fi

{
  printf 'version=1\n'
  printf 'pid=%s\n' "$unrelated_pid"
  printf 'shell=%s\n' "$BASH"
  printf 'script=%s\n' "$HARNESS_FIFO_WRITER"
  printf 'root=%s\n' "$reused_pid_root"
  printf 'fifo=%s\n' "$reused_pid_fifo"
  printf 'token=bug014-1-2-3-4\n'
  printf 'ready=%s\n' "$reused_pid_ready_identity"
} > "$reused_pid_metadata"
cleanup_fifo_writers "$reused_pid_root"
cleanup_fifo_writers "$reused_pid_root"
if kill -0 "$unrelated_pid" 2>/dev/null; then
  unrelated_after_stale_identity="alive"
else
  unrelated_after_stale_identity="not-alive"
fi
kill -TERM "$unrelated_pid" 2>/dev/null || true
wait "$unrelated_pid" 2>/dev/null || true
if kill -0 "$unrelated_pid" 2>/dev/null; then
  unrelated_after_teardown="alive"
else
  unrelated_after_teardown="not-alive"
fi
"$REAL_RM" -rf "$reused_pid_root"
"$REAL_RM" -f "$reused_pid_ready" "$reused_pid_block"

assert_eq "F6 reused-PID fixture starts an unrelated blocked process" \
  "$unrelated_ready:$unrelated_before" "ready:alive"
assert_eq "F6 reused-PID fixture carries cleanup root only as inert argv" \
  "$unrelated_has_inert_root" "yes"
assert_eq "F6 PID-only malformed metadata never signals unrelated PID" \
  "$unrelated_after_pid_only" "alive"
assert_eq "F6 stale exact-identity metadata never signals reused PID" \
  "$unrelated_after_stale_identity" "alive"
assert_eq "F6 unrelated process stops only during explicit teardown" \
  "$unrelated_after_teardown" "not-alive"

if [[ "${BUG014_F6_TRACE:-0}" == "1" ]]; then
  printf '%s\n' \
    "BUG014_F6_LIFECYCLE_CASES=$f6_lifecycle_cases" \
    "BUG014_F6_LIFECYCLE_STATUS_ROWS=$f6_lifecycle_status_rows" \
    "BUG014_F6_LIFECYCLE_READY_FAILURES=$f6_lifecycle_ready_failures" \
    "BUG014_F6_LIFECYCLE_STATUS_FAILURES=$f6_lifecycle_status_failures" \
    "BUG014_F6_LIFECYCLE_SENTINEL_FAILURES=$f6_lifecycle_sentinel_failures" \
    "BUG014_F6_LIFECYCLE_WRITER_BEFORE_FAILURES=$f6_lifecycle_writer_before_failures" \
    "BUG014_F6_LIFECYCLE_WRITER_AFTER_FAILURES=$f6_lifecycle_writer_after_failures" \
    "BUG014_F6_LIFECYCLE_FIXTURE_FAILURES=$f6_lifecycle_fixture_failures" \
    "BUG014_F6_LIFECYCLE_PID_RESIDUE_FAILURES=$f6_lifecycle_pid_residue_failures" \
    "BUG014_F6_LIFECYCLE_TIMEOUT_FAILURES=$f6_lifecycle_timeout_failures" \
    "BUG014_F6_LIFECYCLE_IDEMPOTENCY_FAILURES=$f6_lifecycle_idempotency_failures" \
    "BUG014_F6_REUSED_PID_INERT_ROOT=$unrelated_has_inert_root" \
    "BUG014_F6_REUSED_PID_PID_ONLY_AFTER=$unrelated_after_pid_only" \
    "BUG014_F6_REUSED_PID_STALE_IDENTITY_AFTER=$unrelated_after_stale_identity" \
    "BUG014_F6_REUSED_PID_TEARDOWN_AFTER=$unrelated_after_teardown" \
    "BUG014_F6_UNLINKED_FIFO_WRITER_AFTER=$f6_unlinked_fifo_writer_after"
fi

read_failure_bin="$tmp/read-failure-bin"
read_failure_tmp="$tmp/read-failure-tmp"
mkdir -p "$read_failure_bin" "$read_failure_tmp"
cat > "$read_failure_bin/cat" <<'CAT_READ_FAILURE'
#!/usr/bin/env bash
set -u

: "${BUG014_FAIL_RECORD_DIR:?missing parser record directory}"
: "${BUG014_REAL_CAT:?missing real cat path}"
for candidate in "$@"; do
  case "$candidate" in
    "$BUG014_FAIL_RECORD_DIR"/bubbles-adversarial-resolve.*)
      echo "raw injected parser-record read failure: $candidate" >&2
      exit 74
      ;;
  esac
done
exec "$BUG014_REAL_CAT" "$@"
CAT_READ_FAILURE
chmod +x "$read_failure_bin/cat"

read_failure_before="$(count_parser_records "$read_failure_tmp")"
TEST_PATH="$read_failure_bin:$base_test_path"
run_resolver TMPDIR="$read_failure_tmp" \
  BUG014_FAIL_RECORD_DIR="$read_failure_tmp" BUG014_REAL_CAT="$REAL_CAT" \
  BUBBLES_ADVERSARIAL=auto BUBBLES_ADVERSARIAL_SAMPLES=5 \
  BUBBLES_ADVERSARIAL_TEETH=warn \
  bash "$RESOLVE" --repo-root "$config_repo" \
  --directive "mode: on samples: 3 teeth: blocking"
TEST_PATH="$base_test_path"
assert_f6_parser_failure "explicit parser-record read failure" \
  "$read_failure_tmp" "$read_failure_before"

for framing_fault in \
  no-tab multiple-tab unknown-key incomplete cr-bearing control-bearing; do
  framing_fault_tmp="$record_fault_tmp/framing-$framing_fault"
  mkdir -p "$framing_fault_tmp"
  framing_fault_before="$(count_parser_records "$framing_fault_tmp")"
  TEST_PATH="$record_fault_bin:$base_test_path"
  run_resolver TMPDIR="$framing_fault_tmp" \
    BUG014_RECORD_FAULT="$framing_fault" \
    BUG014_RECORD_DIR="$framing_fault_tmp" \
    BUG014_REAL_AWK="$REAL_AWK" BUG014_REAL_CAT="$REAL_CAT" \
    BUG014_REAL_CHMOD="$REAL_CHMOD" BUG014_REAL_LN="$REAL_LN" \
    BUG014_REAL_MKFIFO="$REAL_MKFIFO" BUG014_REAL_RM="$REAL_RM" \
    bash "$RESOLVE" --repo-root "$empty_repo" \
    --directive "mode: on samples: 3 teeth: blocking"
  TEST_PATH="$base_test_path"
  assert_f6_parser_failure "successful producer $framing_fault record" \
    "$framing_fault_tmp" "$framing_fault_before"
done

# 18. BUG014-F5: one multi-key directive is tokenized exactly once. The PATH
# wrapper increments an external counter before delegating to the real awk, so
# reparsing cannot hide behind canonical output.
count_bin="$tmp/count-bin"
count_parser_tmp="$tmp/count-parser-tmp"
awk_counter="$tmp/awk-counter"
mkdir -p "$count_bin" "$count_parser_tmp"
printf '0\n' > "$awk_counter"
cat > "$count_bin/awk" <<'AWK_COUNTER'
#!/usr/bin/env bash
set -u

: "${BUG014_AWK_COUNTER:?missing counter path}"
: "${BUG014_REAL_AWK:?missing real awk path}"

counter=0
if [[ -f "$BUG014_AWK_COUNTER" ]]; then
  IFS= read -r counter < "$BUG014_AWK_COUNTER" || true
fi
case "$counter" in
  ''|*[!0-9]*) exit 97 ;;
esac
counter=$((counter + 1))
printf '%s\n' "$counter" > "$BUG014_AWK_COUNTER"
exec "$BUG014_REAL_AWK" "$@"
AWK_COUNTER
chmod +x "$count_bin/awk"

count_records_before="$(count_parser_records "$count_parser_tmp")"
TEST_PATH="$count_bin:$base_test_path"
run_resolver TMPDIR="$count_parser_tmp" \
  BUG014_AWK_COUNTER="$awk_counter" BUG014_REAL_AWK="$REAL_AWK" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "adversarial: on samples: 4 teeth: blocking"
TEST_PATH="$base_test_path"
count_records_after="$(count_parser_records "$count_parser_tmp")"
awk_invocations="$(cat "$awk_counter")"
expected_parse_once="$(printf '%s\n' \
  'mode=on' \
  'samples=4' \
  'sampleSemantics=same-runtime-correlated' \
  'teeth=blocking' \
  'source=directive' \
  'samplesSource=directive' \
  'deprecation=none')"

assert_status "multi-key parse-once directive exits zero" 0
assert_eq "multi-key parse-once output is canonical" \
  "$RUN_STDOUT" "$expected_parse_once"
assert_eq "multi-key parse-once stderr is empty" "$RUN_STDERR" ""
assert_eq "multi-key directive invokes awk exactly once" "$awk_invocations" "1"
assert_eq "multi-key parse preserves parser-record count" \
  "$count_records_after" "$count_records_before"
assert_eq "multi-key parse leaves no parser record" "$count_records_after" "0"

printf '0\n' > "$awk_counter"
empty_count_records_before="$(count_parser_records "$count_parser_tmp")"
TEST_PATH="$count_bin:$base_test_path"
run_resolver TMPDIR="$count_parser_tmp" \
  BUG014_AWK_COUNTER="$awk_counter" BUG014_REAL_AWK="$REAL_AWK" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive ""
TEST_PATH="$base_test_path"
empty_count_records_after="$(count_parser_records "$count_parser_tmp")"
empty_awk_invocations="$(cat "$awk_counter")"
assert_status "empty directive parse-once control exits zero" 0
assert_eq "empty directive parse-once control keeps canonical default" \
  "$RUN_STDOUT" "$expected_default"
assert_eq "empty directive parse-once control has empty stderr" \
  "$RUN_STDERR" ""
assert_eq "empty directive invokes awk exactly once" \
  "$empty_awk_invocations" "1"
assert_eq "empty directive preserves parser-record count" \
  "$empty_count_records_after" "$empty_count_records_before"
assert_eq "empty directive leaves no parser record" \
  "$empty_count_records_after" "0"

# 19. BUG014-F5: a 128 KiB whitespace run before a malformed selected value
# must finish under a generous watchdog. A file-backed source runner avoids the
# Linux single-argument size ceiling while executing the exact resolver source.
directive_runner="$tmp/directive-runner.sh"
cat > "$directive_runner" <<'DIRECTIVE_RUNNER'
#!/usr/bin/env bash
set -euo pipefail

directive_file="$1"
resolver="$2"
repo_root="$3"
directive="$(cat "$directive_file")"
source "$resolver" --repo-root "$repo_root" --directive "$directive"
DIRECTIVE_RUNNER
chmod +x "$directive_runner"

run_directive_file_with_timeout() {
  local seconds="$1"
  local directive_file="$2"
  local parser_tmp="$3"
  local stdout_file="$tmp/resolver-timeout.stdout"
  local stderr_file="$tmp/resolver-timeout.stderr"

  RUN_STDOUT=""
  RUN_STDERR=""
  RUN_STATUS=0
  if bubbles_run_with_timeout "$seconds" \
    env -i PATH="$TEST_PATH" TMPDIR="$parser_tmp" \
    bash "$directive_runner" "$directive_file" "$RESOLVE" "$empty_repo" \
    > "$stdout_file" 2> "$stderr_file"; then
    RUN_STATUS=0
  else
    RUN_STATUS=$?
  fi
  RUN_STDOUT="$(cat "$stdout_file")"
  RUN_STDERR="$(cat "$stderr_file")"
}

small_directive_file="$tmp/small-whitespace.directive"
long_directive_file="$tmp/long-whitespace.directive"
whitespace_parser_tmp="$tmp/whitespace-parser-tmp"
mkdir -p "$whitespace_parser_tmp"
printf 'samples:' > "$small_directive_file"
"$REAL_AWK" 'BEGIN { for (i = 0; i < 4096; i++) printf " " }' \
  >> "$small_directive_file"
printf '0' >> "$small_directive_file"
printf 'samples:' > "$long_directive_file"
"$REAL_AWK" 'BEGIN { for (i = 0; i < 128 * 1024; i++) printf " " }' \
  >> "$long_directive_file"
printf '0' >> "$long_directive_file"

small_total_bytes="$(LC_ALL=C wc -c < "$small_directive_file")"
small_total_bytes=$((small_total_bytes + 0))
long_total_bytes="$(LC_ALL=C wc -c < "$long_directive_file")"
long_total_bytes=$((long_total_bytes + 0))
assert_eq "small whitespace control has the intended byte size" \
  "$small_total_bytes" "$((4096 + 9))"
assert_eq "long directive contains exactly 128 KiB of whitespace" \
  "$long_total_bytes" "$((128 * 1024 + 9))"

run_directive_file_with_timeout 5 \
  "$small_directive_file" "$whitespace_parser_tmp"
small_status="$RUN_STATUS"
small_stdout="$RUN_STDOUT"
small_stderr="$RUN_STDERR"

whitespace_records_before="$(count_parser_records "$whitespace_parser_tmp")"
run_directive_file_with_timeout 5 \
  "$long_directive_file" "$whitespace_parser_tmp"
whitespace_records_after="$(count_parser_records "$whitespace_parser_tmp")"

assert_status "128 KiB whitespace directive finishes and exits one" 1
assert_eq_redacted "128 KiB whitespace directive emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq_redacted "128 KiB whitespace directive reports only the selected value" \
  "$RUN_STDERR" \
  "adversarial-resolve: invalid samples '0' (expected integer 1..5)"
assert_eq "128 KiB and small control exits are identical" \
  "$RUN_STATUS" "$small_status"
assert_eq_redacted "128 KiB and small control stdout is byte-identical" \
  "$RUN_STDOUT" "$small_stdout"
assert_eq_redacted "128 KiB and small control stderr is byte-identical" \
  "$RUN_STDERR" "$small_stderr"
assert_eq "whitespace cases preserve parser-record count" \
  "$whitespace_records_after" "$whitespace_records_before"
assert_eq "whitespace cases leave no parser record" \
  "$whitespace_records_after" "0"

# 20. BUG014-F5/F6 deterministic cleanup: repeated valid and invalid
# directives must preserve byte-for-byte output and exit behavior, and every
# parser record must be removed across both success and failure paths.
determinism_parser_tmp="$tmp/determinism-parser-tmp"
mkdir -p "$determinism_parser_tmp"
determinism_records_before="$(count_parser_records "$determinism_parser_tmp")"

run_resolver TMPDIR="$determinism_parser_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: on samples: 3 teeth: blocking"
valid_status_first="$RUN_STATUS"
valid_stdout_first="$RUN_STDOUT"
valid_stderr_first="$RUN_STDERR"
run_resolver TMPDIR="$determinism_parser_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: on samples: 3 teeth: blocking"
assert_eq "repeated valid directive exit is deterministic" \
  "$RUN_STATUS" "$valid_status_first"
assert_eq "repeated valid directive stdout is byte-identical" \
  "$RUN_STDOUT" "$valid_stdout_first"
assert_eq "repeated valid directive stderr is byte-identical" \
  "$RUN_STDERR" "$valid_stderr_first"

run_resolver TMPDIR="$determinism_parser_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
invalid_status_first="$RUN_STATUS"
invalid_stdout_first="$RUN_STDOUT"
invalid_stderr_first="$RUN_STDERR"
run_resolver TMPDIR="$determinism_parser_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
assert_eq "repeated invalid directive exit is deterministic" \
  "$RUN_STATUS" "$invalid_status_first"
assert_eq "repeated invalid directive stdout is byte-identical" \
  "$RUN_STDOUT" "$invalid_stdout_first"
assert_eq "repeated invalid directive stderr is byte-identical" \
  "$RUN_STDERR" "$invalid_stderr_first"

determinism_records_after="$(count_parser_records "$determinism_parser_tmp")"
assert_eq "repeated directives preserve parser-record count" \
  "$determinism_records_after" "$determinism_records_before"
assert_eq "repeated directives leave no parser record" \
  "$determinism_records_after" "0"

# 21. BUG014-F7: every required query made through an available yq must fail
# closed. The shim delegates all non-selected calls to the normal fixture yq,
# while its selected failure emits raw stderr that the resolver must hide.
failing_yq_bin="$tmp/failing-yq-bin"
failing_yq_parser_tmp="$tmp/failing-yq-parser-tmp"
real_fixture_yq="$tmp/bin/yq"
mkdir -p "$failing_yq_bin" "$failing_yq_parser_tmp"
cat > "$failing_yq_bin/yq" <<'YQ_FAILURE'
#!/usr/bin/env bash
set -u

: "${BUG014_FAIL_YQ_QUERY:?missing selected yq query}"
: "${BUG014_REAL_YQ:?missing fixture yq path}"

if [[ "$1" == "$BUG014_FAIL_YQ_QUERY" ]]; then
  echo "partial injected yq parser output"
  echo "raw injected yq parser failure" >&2
  exit 86
fi
exec "$BUG014_REAL_YQ" "$@"
YQ_FAILURE
chmod +x "$failing_yq_bin/yq"

f7_queries=(
  '.adversarial.mode'
  '.adversarial.mode | tag'
  '.adversarial.samples'
  '.adversarial.samples | tag'
  '.adversarial.passes'
  '.adversarial.passes | tag'
  '.adversarial.teeth'
  '.adversarial.teeth | tag'
)
f7_labels=(
  'mode value'
  'mode tag'
  'samples value'
  'samples tag'
  'passes value'
  'passes tag'
  'teeth value'
  'teeth tag'
)

f7_index=0
while [[ "$f7_index" -lt "${#f7_queries[@]}" ]]; do
  f7_query="${f7_queries[$f7_index]}"
  f7_label="${f7_labels[$f7_index]}"
  f7_records_before="$(count_parser_records "$failing_yq_parser_tmp")"
  TEST_PATH="$failing_yq_bin:$base_test_path"
  case "$f7_index" in
    0)
      run_resolver TMPDIR="$failing_yq_parser_tmp" \
        BUG014_FAIL_YQ_QUERY="$f7_query" BUG014_REAL_YQ="$real_fixture_yq" \
        bash "$RESOLVE" --repo-root "$config_repo" \
        --directive "mode: on samples: 3 teeth: blocking"
      ;;
    1)
      run_resolver TMPDIR="$failing_yq_parser_tmp" \
        BUG014_FAIL_YQ_QUERY="$f7_query" BUG014_REAL_YQ="$real_fixture_yq" \
        BUBBLES_ADVERSARIAL=on BUBBLES_ADVERSARIAL_SAMPLES=5 \
        BUBBLES_ADVERSARIAL_TEETH=blocking \
        bash "$RESOLVE" --repo-root "$config_repo"
      ;;
    *)
      run_resolver TMPDIR="$failing_yq_parser_tmp" \
        BUG014_FAIL_YQ_QUERY="$f7_query" BUG014_REAL_YQ="$real_fixture_yq" \
        bash "$RESOLVE" --repo-root "$config_repo"
      ;;
  esac
  TEST_PATH="$base_test_path"
  f7_records_after="$(count_parser_records "$failing_yq_parser_tmp")"

  assert_status "failed yq $f7_label query exits two" 2
  assert_eq "failed yq $f7_label query emits no posture stdout" \
    "$RUN_STDOUT" ""
  assert_eq "failed yq $f7_label query has stable stderr" \
    "$RUN_STDERR" "adversarial-resolve: config parser failed"
  assert_not_match "failed yq $f7_label query emits no mode posture" \
    "$RUN_STDOUT" '^mode='
  assert_not_match "failed yq $f7_label query emits no default samples" \
    "$RUN_STDOUT" '^samples=1$'
  assert_not_match "failed yq $f7_label query emits no default deprecation" \
    "$RUN_STDOUT" '^deprecation=none$'
  assert_eq "failed yq $f7_label query preserves parser-record count" \
    "$f7_records_after" "$f7_records_before"
  assert_eq "failed yq $f7_label query leaves no parser record" \
    "$f7_records_after" "0"

  f7_index=$((f7_index + 1))
done

# A non-selected shim delegates all eight calls and retains valid config
# behavior. A separate PATH with no yq preserves warning-and-skip semantics.
f7_control_parser_tmp="$tmp/f7-control-parser-tmp"
mkdir -p "$f7_control_parser_tmp"
TEST_PATH="$failing_yq_bin:$base_test_path"
run_resolver TMPDIR="$f7_control_parser_tmp" \
  BUG014_FAIL_YQ_QUERY='not-a-production-query' \
  BUG014_REAL_YQ="$real_fixture_yq" \
  bash "$RESOLVE" --repo-root "$config_repo"
TEST_PATH="$base_test_path"
expected_valid_config="$(printf '%s\n' \
  'mode=auto' \
  'samples=4' \
  'sampleSemantics=same-runtime-correlated' \
  'teeth=blocking' \
  'source=config' \
  'samplesSource=config' \
  'deprecation=none')"
assert_status "available yq valid-config control exits zero" 0
assert_eq "available yq valid-config control is canonical" \
  "$RUN_STDOUT" "$expected_valid_config"
assert_eq "available yq valid-config control has empty stderr" \
  "$RUN_STDERR" ""
assert_eq "available yq valid-config control leaves no parser record" \
  "$(count_parser_records "$f7_control_parser_tmp")" "0"

missing_yq_bin="$tmp/missing-yq-bin"
missing_yq_parser_tmp="$tmp/missing-yq-parser-tmp"
mkdir -p "$missing_yq_bin" "$missing_yq_parser_tmp"
for required_command in bash awk cat mktemp rm tr; do
  ln -s "$(command -v "$required_command")" \
    "$missing_yq_bin/$required_command"
done
TEST_PATH="$missing_yq_bin"
run_resolver TMPDIR="$missing_yq_parser_tmp" \
  bash "$RESOLVE" --repo-root "$config_repo" \
  --directive "mode: on samples: 2 teeth: blocking"
TEST_PATH="$base_test_path"
expected_missing_yq="$(printf '%s\n' \
  'mode=on' \
  'samples=2' \
  'sampleSemantics=same-runtime-correlated' \
  'teeth=blocking' \
  'source=directive' \
  'samplesSource=directive' \
  'deprecation=none')"
assert_status "missing yq compatibility control exits zero" 0
assert_eq "missing yq compatibility control remains canonical" \
  "$RUN_STDOUT" "$expected_missing_yq"
assert_eq "missing yq compatibility control retains exact warning" \
  "$RUN_STDERR" \
  "adversarial-resolve: yq not found — skipping config layer (directive/env/default still apply)"
assert_eq "missing yq compatibility control leaves no parser record" \
  "$(count_parser_records "$missing_yq_parser_tmp")" "0"

# 21a. BUG014-F7: successful yq status is insufficient. This shim preserves
# output framing for the selected value/tag pair and delegates every other
# production query to the normal fixture yq.
shape_yq_bin="$tmp/shape-yq-bin"
shape_yq_parser_tmp="$tmp/shape-yq-parser-tmp"
mkdir -p "$shape_yq_bin" "$shape_yq_parser_tmp"
cat > "$shape_yq_bin/yq" <<'YQ_SHAPE'
#!/usr/bin/env bash
set -u

: "${BUG014_SHAPE_FIELD:?missing selected shape field}"
: "${BUG014_VALUE_SHAPE:?missing selected value shape}"
: "${BUG014_TAG_SHAPE:?missing selected tag shape}"
: "${BUG014_REAL_YQ:?missing fixture yq path}"

value_query=".adversarial.$BUG014_SHAPE_FIELD"
tag_query="$value_query | tag"

emit_value() {
  case "$BUG014_VALUE_SHAPE" in
    delegate) exec "$BUG014_REAL_YQ" "$@" ;;
    zero) exit 0 ;;
    empty) printf '\n' ;;
    multiline) printf 'shape-value-one\nshape-value-two\n' ;;
    cr) printf 'shape-value\r\n' ;;
    map) printf '{key: value}\n' ;;
    seq) printf '[one, two]\n' ;;
    null) printf 'null\n' ;;
    integer) printf '4\n' ;;
    noninteger) printf 'four\n' ;;
    nonnull)
      case "$BUG014_SHAPE_FIELD" in
        mode) printf 'on\n' ;;
        samples|passes) printf '4\n' ;;
        teeth) printf 'blocking\n' ;;
      esac
      ;;
    *) exit 96 ;;
  esac
}

emit_tag() {
  case "$BUG014_TAG_SHAPE" in
    delegate) exec "$BUG014_REAL_YQ" "$@" ;;
    zero) exit 0 ;;
    empty) printf '\n' ;;
    multiline) printf '!!str\n!!null\n' ;;
    null) printf '!!null\n' ;;
    str) printf '!!str\n' ;;
    int) printf '!!int\n' ;;
    map) printf '!!map\n' ;;
    seq) printf '!!seq\n' ;;
    bool) printf '!!bool\n' ;;
    float) printf '!!float\n' ;;
    timestamp) printf '!!timestamp\n' ;;
    custom) printf '!bug014-custom\n' ;;
    unknown) printf '!!binary\n' ;;
    *) exit 97 ;;
  esac
}

if [[ "$1" == "$value_query" ]]; then
  emit_value "$@"
elif [[ "$1" == "$tag_query" ]]; then
  emit_tag "$@"
else
  exec "$BUG014_REAL_YQ" "$@"
fi
YQ_SHAPE
chmod +x "$shape_yq_bin/yq"

assert_f7_shape_failure() { # label parser-dir records-before
  local label="$1"
  local parser_dir="$2"
  local records_before="$3"
  local records_after
  records_after="$(count_parser_records "$parser_dir")"

  assert_status "$label exits two" 2
  assert_eq "$label emits no posture stdout" "$RUN_STDOUT" ""
  assert_eq "$label has exact stable stderr" \
    "$RUN_STDERR" "adversarial-resolve: config parser failed"
  assert_not_match "$label emits no presence or deprecation posture" \
    "$RUN_STDOUT" '^(mode|samples|source|samplesSource|deprecation)='
  assert_eq "$label preserves parser-record count" \
    "$records_after" "$records_before"
  assert_eq "$label leaves no parser record" "$records_after" "0"
}

run_f7_shape_case() { # label field value-shape tag-shape repo-root context
  local label="$1"
  local field="$2"
  local value_shape="$3"
  local tag_shape="$4"
  local repo_root="$5"
  local context="$6"
  local case_tmp="$shape_yq_parser_tmp/${label//[^A-Za-z0-9]/-}"
  local records_before

  mkdir -p "$case_tmp"
  records_before="$(count_parser_records "$case_tmp")"
  TEST_PATH="$shape_yq_bin:$base_test_path"
  case "$context" in
    none)
      run_resolver TMPDIR="$case_tmp" \
        BUG014_SHAPE_FIELD="$field" BUG014_VALUE_SHAPE="$value_shape" \
        BUG014_TAG_SHAPE="$tag_shape" BUG014_REAL_YQ="$real_fixture_yq" \
        bash "$RESOLVE" --repo-root "$repo_root"
      ;;
    directive)
      run_resolver TMPDIR="$case_tmp" \
        BUG014_SHAPE_FIELD="$field" BUG014_VALUE_SHAPE="$value_shape" \
        BUG014_TAG_SHAPE="$tag_shape" BUG014_REAL_YQ="$real_fixture_yq" \
        bash "$RESOLVE" --repo-root "$repo_root" \
        --directive "mode: on samples: 3 teeth: blocking"
      ;;
    env)
      run_resolver TMPDIR="$case_tmp" \
        BUG014_SHAPE_FIELD="$field" BUG014_VALUE_SHAPE="$value_shape" \
        BUG014_TAG_SHAPE="$tag_shape" BUG014_REAL_YQ="$real_fixture_yq" \
        BUBBLES_ADVERSARIAL=on BUBBLES_ADVERSARIAL_SAMPLES=5 \
        BUBBLES_ADVERSARIAL_TEETH=blocking \
        bash "$RESOLVE" --repo-root "$repo_root"
      ;;
    *) record_fail "$label has unknown test context $context" ;;
  esac
  TEST_PATH="$base_test_path"
  assert_f7_shape_failure "$label" "$case_tmp" "$records_before"
}

for shape_field in mode samples passes teeth; do
  shape_repo="$config_repo"
  [[ "$shape_field" != "passes" ]] || shape_repo="$config_alias_repo"

  run_f7_shape_case "$shape_field zero-line value" \
    "$shape_field" zero str "$shape_repo" none
  run_f7_shape_case "$shape_field multi-line value" \
    "$shape_field" multiline str "$shape_repo" none
  run_f7_shape_case "$shape_field CR-bearing value" \
    "$shape_field" cr str "$shape_repo" none
  run_f7_shape_case "$shape_field zero-line tag" \
    "$shape_field" nonnull zero "$shape_repo" none
  run_f7_shape_case "$shape_field empty tag" \
    "$shape_field" nonnull empty "$shape_repo" none
  run_f7_shape_case "$shape_field multi-line tag" \
    "$shape_field" nonnull multiline "$shape_repo" none
  for rejected_tag in map seq bool float timestamp custom unknown; do
    run_f7_shape_case "$shape_field rejected $rejected_tag tag" \
      "$shape_field" nonnull "$rejected_tag" "$shape_repo" none
  done
  run_f7_shape_case "$shape_field null tag with non-null value" \
    "$shape_field" nonnull null "$shape_repo" none
done

run_f7_shape_case "mode rejected int tag" \
  mode nonnull int "$config_repo" none
run_f7_shape_case "teeth rejected int tag" \
  teeth nonnull int "$config_repo" none
run_f7_shape_case "samples int tag with non-integer value" \
  samples noninteger int "$config_repo" none
run_f7_shape_case "passes int tag with non-integer value" \
  passes noninteger int "$config_alias_repo" none
run_f7_shape_case "samples int tag with empty value" \
  samples empty int "$config_repo" none
run_f7_shape_case "passes int tag with empty value" \
  passes empty int "$config_alias_repo" none
run_f7_shape_case "directive cannot bypass malformed mode tag" \
  mode nonnull multiline "$config_repo" directive
run_f7_shape_case "env cannot bypass malformed samples value" \
  samples multiline str "$config_repo" env
run_f7_shape_case "canonical samples cannot bypass malformed passes tag" \
  passes nonnull unknown "$config_both_repo" directive
run_f7_shape_case "env teeth cannot bypass CR-bearing config value" \
  teeth cr str "$config_repo" env

# Allowed closed-set controls prove that validation does not overcorrect.
run_f7_allowed_case() { # label field value-shape tag-shape repo-root
  local label="$1"
  local field="$2"
  local value_shape="$3"
  local tag_shape="$4"
  local repo_root="$5"
  local case_tmp="$shape_yq_parser_tmp/${label//[^A-Za-z0-9]/-}"

  mkdir -p "$case_tmp"
  TEST_PATH="$shape_yq_bin:$base_test_path"
  run_resolver TMPDIR="$case_tmp" \
    BUG014_SHAPE_FIELD="$field" BUG014_VALUE_SHAPE="$value_shape" \
    BUG014_TAG_SHAPE="$tag_shape" BUG014_REAL_YQ="$real_fixture_yq" \
    bash "$RESOLVE" --repo-root "$repo_root"
  TEST_PATH="$base_test_path"
}

run_f7_allowed_case "allowed mode string" \
  mode nonnull str "$config_repo"
assert_status "allowed mode string exits zero" 0
assert_line "allowed mode string remains present" "$RUN_STDOUT" "mode=on"
assert_eq "allowed mode string emits no diagnostic" "$RUN_STDERR" ""

run_f7_allowed_case "allowed teeth string" \
  teeth nonnull str "$config_repo"
assert_status "allowed teeth string exits zero" 0
assert_line "allowed teeth string remains present" \
  "$RUN_STDOUT" "teeth=blocking"
assert_eq "allowed teeth string emits no diagnostic" "$RUN_STDERR" ""

run_f7_allowed_case "allowed samples integer" \
  samples integer int "$config_repo"
assert_status "allowed samples integer exits zero" 0
assert_line "allowed samples integer remains present" "$RUN_STDOUT" "samples=4"
assert_line "allowed samples integer remains config sourced" \
  "$RUN_STDOUT" "samplesSource=config"
assert_eq "allowed samples integer emits no diagnostic" "$RUN_STDERR" ""

run_f7_allowed_case "allowed passes integer" \
  passes integer int "$config_alias_repo"
assert_alias_result "allowed passes integer" 4 config config

run_f7_allowed_case "allowed quoted samples string" \
  samples integer str "$config_repo"
assert_status "allowed quoted samples string exits zero" 0
assert_line "allowed quoted samples string remains present" \
  "$RUN_STDOUT" "samples=4"
assert_line "allowed quoted samples string remains config sourced" \
  "$RUN_STDOUT" "samplesSource=config"
assert_eq "allowed quoted samples string emits no diagnostic" \
  "$RUN_STDERR" ""

run_f7_allowed_case "allowed quoted passes string" \
  passes integer str "$config_alias_repo"
assert_alias_result "allowed quoted passes string" 4 config config

for empty_string_field in mode samples passes teeth; do
  empty_string_repo="$config_empty_mode_repo"
  case "$empty_string_field" in
    samples) empty_string_repo="$config_empty_samples_repo" ;;
    passes) empty_string_repo="$config_empty_passes_repo" ;;
    teeth) empty_string_repo="$config_empty_teeth_repo" ;;
  esac
  run_f7_allowed_case "allowed empty $empty_string_field string shape" \
    "$empty_string_field" empty str "$empty_string_repo"
  assert_status "allowed empty $empty_string_field reaches selected validation" 1
  assert_eq "allowed empty $empty_string_field emits no posture" \
    "$RUN_STDOUT" ""
  assert_match "allowed empty $empty_string_field reports selected value" \
    "$RUN_STDERR" "invalid .*${empty_string_field}.*''"
  assert_not_match "allowed empty $empty_string_field is not parser failure" \
    "$RUN_STDERR" 'config parser failed'
done

assert_eq "F7 shape matrix leaves no parser records" \
  "$(count_parser_records "$shape_yq_parser_tmp")" "0"

# 22. BUG014-F8: removal failure is injected only for the production parser
# record. All other rm calls delegate to the real command, and the harness
# removes each intentionally retained record after asserting exact accounting.
cleanup_failure_bin="$tmp/cleanup-failure-bin"
real_rm="$REAL_RM"
mkdir -p "$cleanup_failure_bin"
cat > "$cleanup_failure_bin/rm" <<'RM_FAILURE'
#!/usr/bin/env bash
set -u

: "${BUG014_FAIL_RECORD_DIR:?missing parser record directory}"
: "${BUG014_REAL_RM:?missing real rm path}"

for candidate in "$@"; do
  case "$candidate" in
    "$BUG014_FAIL_RECORD_DIR"/bubbles-adversarial-resolve.*) exit 73 ;;
  esac
done
exec "$BUG014_REAL_RM" "$@"
RM_FAILURE
chmod +x "$cleanup_failure_bin/rm"

cleanup_success_tmp="$tmp/cleanup-success-tmp"
mkdir -p "$cleanup_success_tmp"
cleanup_success_before="$(count_parser_records "$cleanup_success_tmp")"
TEST_PATH="$cleanup_failure_bin:$base_test_path"
run_resolver TMPDIR="$cleanup_success_tmp" \
  BUG014_FAIL_RECORD_DIR="$cleanup_success_tmp" BUG014_REAL_RM="$real_rm" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: on samples: 3 teeth: blocking"
TEST_PATH="$base_test_path"
cleanup_success_after="$(count_parser_records "$cleanup_success_tmp")"
assert_status "valid resolution cleanup failure exits two" 2
assert_eq "valid resolution cleanup failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "valid resolution cleanup failure has stable stderr" \
  "$RUN_STDERR" \
  "adversarial-resolve: directive parser record cleanup failed"
assert_not_match "valid resolution cleanup failure emits no mode posture" \
  "$RUN_STDOUT" '^mode='
assert_not_match "valid resolution cleanup failure emits no samples posture" \
  "$RUN_STDOUT" '^samples='
assert_not_match "valid resolution cleanup failure emits no deprecation posture" \
  "$RUN_STDOUT" '^deprecation='
assert_eq "valid resolution cleanup failure started without residue" \
  "$cleanup_success_before" "0"
assert_eq "valid resolution cleanup failure retains exactly one record" \
  "$cleanup_success_after" "1"
"$real_rm" -f "$cleanup_success_tmp"/bubbles-adversarial-resolve.*
assert_eq "valid resolution test-owned cleanup removes retained record" \
  "$(count_parser_records "$cleanup_success_tmp")" "0"

cleanup_validation_tmp="$tmp/cleanup-validation-tmp"
mkdir -p "$cleanup_validation_tmp"
TEST_PATH="$cleanup_failure_bin:$base_test_path"
run_resolver TMPDIR="$cleanup_validation_tmp" \
  BUG014_FAIL_RECORD_DIR="$cleanup_validation_tmp" BUG014_REAL_RM="$real_rm" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
TEST_PATH="$base_test_path"
expected_validation_cleanup="$(printf '%s\n' \
  "adversarial-resolve: invalid samples '0' (expected integer 1..5)" \
  'adversarial-resolve: directive parser record cleanup failed')"
assert_status "validation plus cleanup failure preserves exit one" 1
assert_eq "validation plus cleanup failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "validation plus cleanup failure preserves both diagnostics" \
  "$RUN_STDERR" "$expected_validation_cleanup"
assert_eq "validation plus cleanup failure retains exactly one record" \
  "$(count_parser_records "$cleanup_validation_tmp")" "1"
"$real_rm" -f "$cleanup_validation_tmp"/bubbles-adversarial-resolve.*
assert_eq "validation failure test-owned cleanup removes retained record" \
  "$(count_parser_records "$cleanup_validation_tmp")" "0"

cleanup_parser_tmp="$tmp/cleanup-parser-tmp"
mkdir -p "$cleanup_parser_tmp"
TEST_PATH="$cleanup_failure_bin:$fault_bin:$base_test_path"
run_resolver TMPDIR="$cleanup_parser_tmp" \
  BUG014_FAIL_RECORD_DIR="$cleanup_parser_tmp" BUG014_REAL_RM="$real_rm" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
TEST_PATH="$base_test_path"
expected_parser_cleanup="$(printf '%s\n' \
  'adversarial-resolve: directive parser failed' \
  'adversarial-resolve: directive parser record cleanup failed')"
assert_status "parser plus cleanup failure preserves exit two" 2
assert_eq "parser plus cleanup failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "parser plus cleanup failure preserves both diagnostics" \
  "$RUN_STDERR" "$expected_parser_cleanup"
assert_eq "parser plus cleanup failure retains exactly one record" \
  "$(count_parser_records "$cleanup_parser_tmp")" "1"
"$real_rm" -f "$cleanup_parser_tmp"/bubbles-adversarial-resolve.*
assert_eq "parser failure test-owned cleanup removes retained record" \
  "$(count_parser_records "$cleanup_parser_tmp")" "0"

cleanup_config_tmp="$tmp/cleanup-config-tmp"
mkdir -p "$cleanup_config_tmp"
TEST_PATH="$cleanup_failure_bin:$failing_yq_bin:$base_test_path"
run_resolver TMPDIR="$cleanup_config_tmp" \
  BUG014_FAIL_RECORD_DIR="$cleanup_config_tmp" BUG014_REAL_RM="$real_rm" \
  BUG014_FAIL_YQ_QUERY='.adversarial.samples' \
  BUG014_REAL_YQ="$real_fixture_yq" \
  bash "$RESOLVE" --repo-root "$config_repo"
TEST_PATH="$base_test_path"
expected_config_cleanup="$(printf '%s\n' \
  'adversarial-resolve: config parser failed' \
  'adversarial-resolve: directive parser record cleanup failed')"
assert_status "config parser plus cleanup failure preserves exit two" 2
assert_eq "config parser plus cleanup failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "config parser plus cleanup failure preserves both diagnostics" \
  "$RUN_STDERR" "$expected_config_cleanup"
assert_eq "config parser plus cleanup failure retains exactly one record" \
  "$(count_parser_records "$cleanup_config_tmp")" "1"
"$REAL_RM" -f "$cleanup_config_tmp"/bubbles-adversarial-resolve.*
assert_eq "config parser failure test-owned cleanup removes retained record" \
  "$(count_parser_records "$cleanup_config_tmp")" "0"

cleanup_usage_tmp="$tmp/cleanup-usage-tmp"
mkdir -p "$cleanup_usage_tmp"
TEST_PATH="$cleanup_failure_bin:$base_test_path"
run_resolver TMPDIR="$cleanup_usage_tmp" \
  BUG014_FAIL_RECORD_DIR="$cleanup_usage_tmp" BUG014_REAL_RM="$real_rm" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "samples: 2 samples: 3"
TEST_PATH="$base_test_path"
expected_usage_cleanup="$(printf '%s\n' \
  'adversarial-resolve: duplicate samples directive token is ambiguous' \
  'adversarial-resolve: directive parser record cleanup failed')"
assert_status "duplicate token plus cleanup failure preserves exit two" 2
assert_eq "duplicate token plus cleanup failure emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "duplicate token plus cleanup failure preserves both diagnostics" \
  "$RUN_STDERR" "$expected_usage_cleanup"
assert_eq "duplicate token plus cleanup failure retains exactly one record" \
  "$(count_parser_records "$cleanup_usage_tmp")" "1"
"$REAL_RM" -f "$cleanup_usage_tmp"/bubbles-adversarial-resolve.*
assert_eq "duplicate token failure test-owned cleanup removes retained record" \
  "$(count_parser_records "$cleanup_usage_tmp")" "0"

symlink_cleanup_target="$tmp/symlink-cleanup-target"
symlink_cleanup_tmp="$tmp/symlink-cleanup-tmp"
mkdir -p "$symlink_cleanup_target"
"$REAL_LN" -s "$symlink_cleanup_target" "$symlink_cleanup_tmp"
run_resolver TMPDIR="$symlink_cleanup_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" \
  --directive "mode: on samples: 3 teeth: blocking"
expected_symlink_cleanup="$(printf '%s\n' \
  'mode=on' \
  'samples=3' \
  'sampleSemantics=same-runtime-correlated' \
  'teeth=blocking' \
  'source=directive' \
  'samplesSource=directive' \
  'deprecation=none')"
assert_status "symlink TMPDIR success exits zero" 0
assert_eq "symlink TMPDIR success output is canonical" \
  "$RUN_STDOUT" "$expected_symlink_cleanup"
assert_eq "symlink TMPDIR success emits no diagnostic" "$RUN_STDERR" ""
assert_eq "symlink TMPDIR success leaves no link-facing parser record" \
  "$(count_parser_records "$symlink_cleanup_tmp")" "0"
assert_eq "symlink TMPDIR success leaves no physical parser record" \
  "$(count_parser_records "$symlink_cleanup_target")" "0"

# Normal success, validation failure, and parser failure remain zero-residue
# controls independent of the earlier F5/F6 cases.
normal_cleanup_tmp="$tmp/normal-cleanup-tmp"
mkdir -p "$normal_cleanup_tmp"
run_resolver TMPDIR="$normal_cleanup_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 2"
assert_status "normal cleanup success control exits zero" 0
assert_eq "normal cleanup success control leaves no parser record" \
  "$(count_parser_records "$normal_cleanup_tmp")" "0"

run_resolver TMPDIR="$normal_cleanup_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
assert_status "normal cleanup validation control exits one" 1
assert_eq "normal cleanup validation control emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "normal cleanup validation control leaves no parser record" \
  "$(count_parser_records "$normal_cleanup_tmp")" "0"

TEST_PATH="$fault_bin:$base_test_path"
run_resolver TMPDIR="$normal_cleanup_tmp" \
  bash "$RESOLVE" --repo-root "$empty_repo" --directive "samples: 0"
TEST_PATH="$base_test_path"
assert_status "normal cleanup parser control exits two" 2
assert_eq "normal cleanup parser control emits no posture stdout" \
  "$RUN_STDOUT" ""
assert_eq "normal cleanup parser control retains parser diagnostic" \
  "$RUN_STDERR" "adversarial-resolve: directive parser failed"
assert_eq "normal cleanup parser control leaves no parser record" \
  "$(count_parser_records "$normal_cleanup_tmp")" "0"

echo "adversarial-resolve-selftest: $pass passed, $fail failed"
if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "PASS"
