#!/usr/bin/env bash
# Capability: framework-self-observation, impact-aware-validation-trace-contracts,
# Capability: linter-on-edit-gate, observability-posture-and-slo-gates,
# Capability: session-aware-runtime-coordination, workflow-runner-authorization
set -euo pipefail

# IMP-102 SCOPE-5: Bubbles requires bash 4.0+ — the framework uses associative
# arrays (declare -A) pervasively (12+ scripts, plus many selftests below). On
# stock macOS bash 3.2 these constructs fail; the shipped command surface must
# fail LOUDLY and EARLY (before sourcing any helper or running any declare -A
# selftest) instead of silently masking the breakage from installers/doctor/CI.
if [[ -z "${BASH_VERSINFO:-}" ]] || ((${BASH_VERSINFO[0]:-0} < 4)); then
  printf 'ERROR: Bubbles requires bash 4.0+ (found %s). Install a newer bash (e.g. `brew install bash` on macOS) and re-run.\n' "${BASH_VERSION:-unknown}" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/guard-lib.sh"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

# Concurrency guard. framework-validate is NOT safe to run twice at once.
#
# The original reason was fixed scratch paths with no per-run suffix, measured at
# 8 spurious failures from one overlapping run. That specific hazard is gone:
# capability-freshness and generate-installer both mktemp their fixture roots
# now, and /tmp/bubbles-agent-ownership-lint no longer exists at all. The guard
# stays because two concurrent runs still share one working tree and its
# generated files (release manifest, framework stats, gate coverage map), and
# regenerating those under another run is the same class of interference. The
# 8-failure measurement predates the mktemp fixes and is not evidence about the
# state of the tree today.
#
# Atomic `mkdir` is available on both GNU and stock macOS userlands. The owner
# record makes a dead holder reclaimable, while a missing/malformed record fails
# closed instead of guessing. Cleanup removes a lock only when its token still
# matches, so a stale process cannot remove a replacement owner's lock.
#
# The guard MUST be re-entrant. Several selftests legitimately run a NESTED
# framework-validate as part of their fixture (v5.3-selftest runs one against a
# synthesized downstream install; repo-drift-report-selftest runs one to capture
# drift output). Those are children of an outer run that already owns the lock,
# so a naive guard refuses them and fails the very suite it is protecting —
# measured: 3 red checks (v5.3 G1, tiering IMP-012, repo-drift IMP-027). The
# exported marker below is inherited only by descendants of a holding run, so
# nested invocations pass through only when the recorded owner is a real process
# ancestor and the inherited token matches. A copied environment marker alone
# cannot bypass contention.
#
# IMP-049 SCOPE-3. The lock protects the SHARED SCRATCH FIXTURES that executing
# checks build. An invocation that executes no check touches none of them, so
# making it wait is protection against nothing. Before this, the lock was taken
# here and arguments were parsed ~250 lines later, which meant `--help`,
# `--list-tier` AND an ordinary typo all exited 1 with a lock error whenever any
# run was in flight — observed while preparing IMP-049 by the review that needed
# `--list-tier=full` and could not get an answer.
#
# The predicate is "will this invocation execute checks?", not "is it one of the
# read-only flags". That direction matters: an argument this pre-scan does not
# recognise means the parser below is about to reject it with exit 2, so holding
# the lock for it would replace a precise usage error with a misleading lock
# error. No arguments at all is the default full run, which does execute and
# therefore does lock. `framework-validate-tier-selftest.sh` asserts that the
# executing-flag list here still matches the parser's own case arms, so the two
# cannot drift apart silently.
#
# IMP-049 SCOPE-2. Whether THIS invocation is the outermost one is decided here,
# before anything can set the marker, and it governs the run receipt below.
# Several selftests run a NESTED framework-validate as part of their fixture, and
# a nested run that wrote a receipt would describe a synthesized fixture while
# appearing to describe this tree. The marker is deliberately separate from the
# lock marker because lock ownership and receipt ownership are different claims.
_fv_outermost=false
[[ -z "${BUBBLES_FRAMEWORK_VALIDATE_DEPTH:-}" ]] && _fv_outermost=true
export BUBBLES_FRAMEWORK_VALIDATE_DEPTH=$((${BUBBLES_FRAMEWORK_VALIDATE_DEPTH:-0} + 1))

_fv_lock_dir=""
_fv_lock_token=""
_fv_lock_identity=""
_bubbles_compat_dir=""
_fv_cleanup() {
  if [[ -n "$_fv_lock_dir" && -n "$_fv_lock_token" && -f "$_fv_lock_dir/owner" ]]; then
    local _fv_cleanup_pid="" _fv_cleanup_token="" _fv_cleanup_identity="" _fv_cleanup_extra=""
    read -r _fv_cleanup_pid _fv_cleanup_token _fv_cleanup_identity _fv_cleanup_extra <"$_fv_lock_dir/owner" || true
    if [[ -z "$_fv_cleanup_extra" && "$_fv_cleanup_pid" == "$$" &&
      "$_fv_cleanup_token" == "$_fv_lock_token" &&
      "$_fv_cleanup_identity" == "$_fv_lock_identity" ]]; then
      rm -rf "$_fv_lock_dir"
    fi
  fi
  [[ -z "$_bubbles_compat_dir" ]] || rm -rf "$_bubbles_compat_dir"
}
trap _fv_cleanup EXIT

_fv_process_identity() {
  local pid="$1" stat_line="" stat_tail="" start_ticks="" started=""
  [[ "$pid" =~ ^[1-9][0-9]*$ ]] || return 1

  # Linux exposes a kernel-assigned start tick in field 22. Split after the
  # final ") " because the comm field itself may contain spaces or parentheses.
  if [[ -r "/proc/$pid/stat" ]]; then
    stat_line="$(cat "/proc/$pid/stat" 2>/dev/null)" || return 1
    [[ "$stat_line" == *") "* ]] || return 1
    stat_tail="${stat_line##*) }"
    read -r -a _fv_stat_fields <<<"$stat_tail"
    start_ticks="${_fv_stat_fields[19]:-}"
    [[ "$start_ticks" =~ ^[0-9]+$ ]] || return 1
    printf 'proc:%s\n' "$start_ticks"
    return 0
  fi

  # BSD/macOS has no /proc. `ps -o lstart=` is capability-probed instead of
  # OS-detected. Its normalized creation timestamp distinguishes incarnations.
  # If the capability is absent or ambiguous, fail closed rather than minting a
  # weak identity that could make PID reuse look like the original owner.
  started="$(ps -o lstart= -p "$pid" 2>/dev/null)" || return 1
  started="${started#${started%%[![:space:]]*}}"
  started="${started%${started##*[![:space:]]}}"
  [[ -n "$started" ]] || return 1
  started="${started//[[:space:]]/_}"
  [[ "$started" =~ ^[A-Za-z0-9_.:+-]+$ ]] || return 1
  printf 'ps:%s\n' "$started"
}

_fv_pid_is_ancestor() {
  local candidate="$1" current="$$" parent=""
  [[ "$candidate" =~ ^[1-9][0-9]*$ ]] || return 1
  while [[ "$current" =~ ^[1-9][0-9]*$ && "$current" -gt 1 ]]; do
    [[ "$current" == "$candidate" ]] && return 0
    parent="$(ps -o ppid= -p "$current" 2>/dev/null)" || return 1
    parent="${parent//[[:space:]]/}"
    [[ "$parent" =~ ^[1-9][0-9]*$ ]] || return 1
    current="$parent"
  done
  return 1
}

_fv_lock_refuse() {
  printf 'ERROR: another framework-validate run is already in progress on this machine.\n' >&2
  printf '       Concurrent runs corrupt each other'"'"'s shared scratch fixtures and produce\n' >&2
  printf '       false failures. Wait for the other run to finish, then re-run.\n' >&2
  exit 1
}

_fv_lock_acquire() {
  _fv_lock_dir="${TMPDIR:-/tmp}/bubbles-framework-validate.lock.d"
  _fv_lock_token="$$.$RANDOM.$SECONDS"
  _fv_lock_identity="$(_fv_process_identity "$$")" || {
    printf 'ERROR: framework-validate could not establish process-incarnation identity.\n' >&2
    exit 1
  }
  local owner_pid="" owner_token="" owner_identity="" owner_extra="" live_identity=""

  if mkdir "$_fv_lock_dir" 2>/dev/null; then
    printf '%s %s %s\n' "$$" "$_fv_lock_token" "$_fv_lock_identity" >"$_fv_lock_dir/owner" || {
      rm -rf "$_fv_lock_dir"
      printf 'ERROR: framework-validate could not create its lock owner record.\n' >&2
      exit 1
    }
    export BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD="$_fv_lock_token"
    return 0
  fi

  [[ -f "$_fv_lock_dir/owner" ]] || _fv_lock_refuse
  read -r owner_pid owner_token owner_identity owner_extra <"$_fv_lock_dir/owner" || _fv_lock_refuse
  [[ -z "$owner_extra" && "$owner_pid" =~ ^[1-9][0-9]*$ && -n "$owner_token" &&
    "$owner_identity" =~ ^(proc:[0-9]+|ps:[A-Za-z0-9_.:+-]+)$ ]] || _fv_lock_refuse

  if kill -0 "$owner_pid" 2>/dev/null; then
    live_identity="$(_fv_process_identity "$owner_pid")" || _fv_lock_refuse
    if [[ "$live_identity" == "$owner_identity" &&
      "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" == "$owner_token" ]] &&
      _fv_pid_is_ancestor "$owner_pid"; then
      _fv_lock_dir=""
      _fv_lock_token=""
      _fv_lock_identity=""
      return 0
    fi
    [[ "$live_identity" != "$owner_identity" ]] || _fv_lock_refuse
  fi

  # Re-read before atomically claiming a dead owner's directory. If either field
  # changed, another process repaired/replaced it and this invocation fails
  # closed. The unique rename has no unlocked deletion window: exactly one
  # contender can move this inode, then exactly one contender can mkdir the
  # canonical path.
  local confirm_pid="" confirm_token="" confirm_identity="" confirm_extra=""
  read -r confirm_pid confirm_token confirm_identity confirm_extra <"$_fv_lock_dir/owner" || _fv_lock_refuse
  [[ -z "$confirm_extra" && "$confirm_pid" == "$owner_pid" &&
    "$confirm_token" == "$owner_token" && "$confirm_identity" == "$owner_identity" ]] || _fv_lock_refuse
  local stale_dir="${_fv_lock_dir}.stale.${_fv_lock_token}"
  mv "$_fv_lock_dir" "$stale_dir" 2>/dev/null || _fv_lock_refuse
  if ! mkdir "$_fv_lock_dir" 2>/dev/null; then
    rm -rf "$stale_dir"
    _fv_lock_refuse
  fi
  printf '%s %s %s\n' "$$" "$_fv_lock_token" "$_fv_lock_identity" >"$_fv_lock_dir/owner" || {
    rm -rf "$_fv_lock_dir"
    printf 'ERROR: framework-validate could not replace a stale lock owner record.\n' >&2
    exit 1
  }
  rm -rf "$stale_dir"
  export BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD="$_fv_lock_token"
}

_fv_executes_checks=true
if [[ $# -gt 0 ]]; then
  _fv_executes_checks=false
  for _fv_arg in "$@"; do
    case "$_fv_arg" in
      --tier=core | --tier=full | --changed-only | --no-changed-only | --cache | --no-cache | --record-debt)
        _fv_executes_checks=true
        ;;
      -h | --help | --list-tier=core | --list-tier=full) ;;
      *)
        _fv_executes_checks=false
        break
        ;;
    esac
  done
fi

_fv_lockfile="${TMPDIR:-/tmp}/bubbles-framework-validate.lock"
_fv_flock_path=""
_fv_resolve_flock() {
  local candidate=""

  for candidate in \
    /usr/bin/flock \
    /bin/flock \
    /usr/local/bin/flock \
    /opt/homebrew/bin/flock \
    /opt/local/bin/flock; do
    if [[ -f "$candidate" && -x "$candidate" ]]; then
      _fv_flock_path="$candidate"
      return 0
    fi
  done
  return 1
}

if [[ "$_fv_executes_checks" == "true" ]]; then
  _fv_resolve_flock || true
fi

_fv_flock() {
  local flock_status=0
  local restore_errexit=false

  [[ -n "$_fv_flock_path" ]] || return 127
  [[ "$-" == *e* ]] && restore_errexit=true
  builtin set +e
  builtin command "$_fv_flock_path" "$@"
  flock_status=$?
  [[ "$restore_errexit" == "false" ]] || builtin set -e
  return "$flock_status"
}

_fv_lock_identity() {
  local path="$1" identity=""
  [[ -f "$path" ]] || return 1
  if identity="$(/usr/bin/stat -Lc '%d:%i' "$path" 2>/dev/null)" \
    && [[ "$identity" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf 'device-inode:%s\n' "$identity"
    return 0
  fi
  if identity="$(/usr/bin/stat -f '%d:%i' "$path" 2>/dev/null)" \
    && [[ "$identity" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf 'device-inode:%s\n' "$identity"
    return 0
  fi
  return 1
}

_fv_darwin_descriptor_identity() {
  local lsof_path="" lsof_output="" field="" device="" inode=""
  local device_digits="" device_decimal=""

  if [[ -x /usr/sbin/lsof ]]; then
    lsof_path=/usr/sbin/lsof
  elif [[ -x /usr/bin/lsof ]]; then
    lsof_path=/usr/bin/lsof
  else
    return 1
  fi
  lsof_output="$("$lsof_path" -a -p "$$" -d 9 -FDi 2>/dev/null)" || return 1
  while IFS= read -r field; do
    case "$field" in
      D*) device="${field#D}" ;;
      i*) inode="${field#i}" ;;
    esac
  done <<<"$lsof_output"
  if [[ "$device" =~ ^0[xX][0-9a-fA-F]+$ ]]; then
    device_digits="${device#0x}"
    device_digits="${device_digits#0X}"
    printf -v device_decimal '%u' "$((16#$device_digits))" || return 1
  elif [[ "$device" =~ ^[0-9]+$ ]]; then
    device_decimal="$device"
  else
    return 1
  fi
  [[ "$inode" =~ ^[0-9]+$ ]] || return 1
  printf 'device-inode:%s:%s\n' "$device_decimal" "$inode"
}

_fv_lock_descriptor_matches_path() {
  local descriptor_path="$1" lock_identity="" descriptor_identity=""
  local opened_identity=""

  lock_identity="$(_fv_lock_identity "$_fv_lockfile")" || return 1
  descriptor_identity="$(_fv_lock_identity "$descriptor_path")" || return 1
  [[ "$descriptor_identity" == "$lock_identity" ]] && return 0
  [[ "$descriptor_path" == /dev/fd/9 ]] || return 1
  opened_identity="$(_fv_darwin_descriptor_identity)" || return 1
  [[ "$opened_identity" == "$lock_identity" ]]
}

_fv_open_lock_descriptor() {
  local descriptor_path=""

  [[ ! -L "$_fv_lockfile" ]] || return 1
  if [[ ! -e "$_fv_lockfile" ]]; then
    if ! (set -o noclobber; : >"$_fv_lockfile") 2>/dev/null; then
      [[ -e "$_fv_lockfile" ]] || return 1
    fi
  fi
  [[ -f "$_fv_lockfile" && ! -L "$_fv_lockfile" ]] || return 1
  exec 9<"$_fv_lockfile" || return 1
  if [[ -L "$_fv_lockfile" ]]; then
    exec 9<&-
    return 1
  fi
  if [[ -e "/proc/$$/fd/9" ]]; then
    descriptor_path="/proc/$$/fd/9"
  elif [[ -e /dev/fd/9 ]]; then
    descriptor_path=/dev/fd/9
  else
    exec 9<&-
    return 1
  fi
  if ! _fv_lock_descriptor_matches_path "$descriptor_path" \
    || [[ -L "$_fv_lockfile" ]]; then
    exec 9<&-
    return 1
  fi
}

_fv_inherited_lock_is_owned() {
  local descriptor_path=""
  [[ "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" == 1 ]] || return 1
  if [[ -e "/proc/$$/fd/9" ]]; then
    descriptor_path="/proc/$$/fd/9"
  elif [[ -e /dev/fd/9 ]]; then
    descriptor_path=/dev/fd/9
  else
    return 1
  fi
  _fv_lock_descriptor_matches_path "$descriptor_path" || return 1
  [[ ! -L "$_fv_lockfile" ]] || return 1
  _fv_flock -n 9 >/dev/null 2>&1
}

if [[ "$_fv_executes_checks" == "true" && -n "$_fv_flock_path" ]]; then
  if [[ -n "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" ]]; then
    if ! _fv_inherited_lock_is_owned; then
      printf 'ERROR: inherited framework-validate lock marker is not backed by the owned lock descriptor.\n' >&2
      exit 1
    fi
  else
    if ! _fv_open_lock_descriptor; then
      printf 'ERROR: framework-validate lock path is not a stable regular file; refusing unsafe acquisition.\n' >&2
      exit 1
    fi
    if ! _fv_flock -n 9; then
      printf 'ERROR: another framework-validate run is already in progress on this machine.\n' >&2
      printf '       Concurrent runs corrupt each other'"'"'s shared scratch fixtures and produce\n' >&2
      printf '       false failures. Wait for the other run to finish, then re-run.\n' >&2
      exit 1
    fi
    export BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD=1
  fi
elif [[ "$_fv_executes_checks" == "true" ]] && [[ -n "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" ]]; then
  printf 'ERROR: inherited framework-validate lock ownership cannot be authenticated because flock is unavailable.\n' >&2
  exit 1
elif [[ "$_fv_executes_checks" == "true" ]]; then
  # flock absent (stock macOS ships none). The guard degrades to a no-op, so say
  # so — a silent degrade lets an operator believe concurrent-run protection is
  # active when it is not.
  printf 'NOTE: flock not found — concurrent-run protection is OFF for this run.\n' >&2
  printf '      Two independent framework-validate runs would corrupt each other'"'"'s\n' >&2
  printf '      shared scratch fixtures. Run only one at a time on this machine.\n' >&2
fi

# macOS portability shim. BSD userland diverges from GNU coreutils on `sed -i`
# (BSD needs `sed -i ''`) and lacks `timeout` (coreutils ships `gsed`/`gtimeout`).
# Several selftests below invoke `sed -i` / `timeout` in GNU form. When the GNU
# binaries are present under their `g`-prefixed names, expose them as plain `sed`
# / `timeout` on PATH for THIS process and every selftest subprocess it spawns,
# so the whole validation runs unchanged on Linux + macOS. On Linux (unprefixed
# GNU tools already present) every probe short-circuits and this is a no-op.
if ! sed --version >/dev/null 2>&1 && command -v gsed >/dev/null 2>&1; then
  _bubbles_compat_dir="$(mktemp -d)"
  ln -sf "$(command -v gsed)" "$_bubbles_compat_dir/sed"
fi
if ! command -v timeout >/dev/null 2>&1 && command -v gtimeout >/dev/null 2>&1; then
  [[ -n "$_bubbles_compat_dir" ]] || _bubbles_compat_dir="$(mktemp -d)"
  ln -sf "$(command -v gtimeout)" "$_bubbles_compat_dir/timeout"
fi
if [[ -n "$_bubbles_compat_dir" ]]; then
  PATH="$_bubbles_compat_dir:$PATH"
  export PATH
fi

# v5.3 / G1: install-mode detection. Many selftests below were authored
# inside the framework source repo and assume `install.sh`, `VERSION`, or
# the framework's own `README.md`/`docs/` layout are present. From a
# downstream install tree (which only carries `.github/bubbles/...`), those
# assertions cannot hold. Detect the install mode once here and use it to
# drive run_check_self_only below.
#
# Override: BUBBLES_FRAMEWORK_VALIDATE_MODE=source|downstream forces a mode
# (useful for selftests that synthesize either tree).
INSTALL_MODE="${BUBBLES_FRAMEWORK_VALIDATE_MODE:-}"
if [[ -z "$INSTALL_MODE" ]]; then
  if [[ -f "$REPO_ROOT/install.sh" && -f "$REPO_ROOT/VERSION" && -f "$REPO_ROOT/bubbles/scripts/cli.sh" ]]; then
    INSTALL_MODE="source"
  elif [[ -f "$REPO_ROOT/.github/bubbles/.install-source.json" ]]; then
    INSTALL_MODE="downstream"
  else
    INSTALL_MODE="unknown"
  fi
fi

failures=0
skipped=0
# Skips are counted per REASON. A single aggregate previously described every
# skip as framework-source-only and told the operator to "run from a
# framework-source tree" -- self-contradictory advice for the tier skips that
# dominate a --tier=core run inside a source tree.
skipped_tier=0
skipped_changed_only=0
skipped_self_only=0
skipped_denied=0
declare -a failed_check_labels=()
# PERF. This suite is serial and its membership grows automatically via the
# discovery sweep, so its wall clock only ever goes up. Recording each check's
# cost is what makes that cost attributable instead of a single opaque number.
declare -a check_durations=()

# IMP-027 SCOPE-7 support: the hermetic-selftest result cache, and the
# changed-surface filter both live outside this script so they can be tested on
# their own. Absence degrades to the previous behaviour (run everything).
FRAMEWORK_VERSION="unknown"
[[ -f "$REPO_ROOT/VERSION" ]] && FRAMEWORK_VERSION="$(tr -d '[:space:]' <"$REPO_ROOT/VERSION" 2>/dev/null || echo unknown)"
# shellcheck source=bubbles/scripts/validate-cache.sh
# IMP-027 SCOPE-7 cache helpers. Sourced ONLY when the file actually defines the
# cache API. `source` runs in THIS shell, so a sibling that merely exits would
# terminate framework-validate mid-run — and because it exits 0, the run would
# look like a silent PASS that validated nothing. Confirming the function is
# defined before sourcing keeps a malformed or stubbed sibling from being able
# to end the validation run.
#
# The check uses ONLY bash builtins (`$(<file)` + glob compare). An external
# `grep` here runs before the portability harness has a full PATH and shows up
# as an unexpected command invocation in the BUG-021 deadline regression, which
# asserts the canonical success path shells out to nothing optional.
if [[ -f "$SCRIPT_DIR/validate-cache.sh" ]]; then
  _validate_cache_src="$(<"$SCRIPT_DIR/validate-cache.sh")"
  if [[ "$_validate_cache_src" == *"validate_cache_key()"* ]]; then
    source "$SCRIPT_DIR/validate-cache.sh"
  fi
  unset _validate_cache_src
fi

# Paths this run should treat as changed, resolved once.
CHANGED_PATHS=""
changed_paths_load() {
  [[ -n "$CHANGED_PATHS" ]] && return 0
  if command -v git >/dev/null 2>&1 && git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
    CHANGED_PATHS="$(
      {
        git -C "$REPO_ROOT" diff --name-only HEAD 2>/dev/null || true
        git -C "$REPO_ROOT" ls-files --others --exclude-standard 2>/dev/null || true
      } | sort -u
    )"
    # At push time the work is already committed, so the working-tree diff is
    # empty and --changed-only would degrade to the full suite. Fall back to
    # the commits that are not yet upstream -- what a push actually carries.
    if [[ -z "$CHANGED_PATHS" ]]; then
      local _base="${BUBBLES_CHANGED_BASE:-}"
      [[ -n "$_base" ]] || _base="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
      if [[ -n "$_base" ]]; then
        CHANGED_PATHS="$(git -C "$REPO_ROOT" diff --name-only "$_base...HEAD" 2>/dev/null || true)"
      fi
    fi
  fi
  # No detectable change set must run everything, never skip everything.
  [[ -n "$CHANGED_PATHS" ]] || CHANGED_PATHS="__NO_GIT__"
}

# changed_surface_touches <selftest-path>
#
# IMP-047 S-E. This used to answer from a BASENAME PAIR: `X-selftest.sh` was
# assumed to own `X.sh` and nothing else. Under that rule, changing
# `bubbles/registry/gates.yaml` SKIPPED the 97 checks that read it and changing
# `guard-lib.sh` SKIPPED nothing that sources it, because no basename moved. A
# selection that answers "unaffected" about inputs it never looked at is a
# skip dressed as a decision.
#
# The answer now comes from the DECLARED INPUT CLOSURE in
# `bubbles/registry/validation-checks.yaml`, derived by tracing real references.
# A check is affected when the change touches ANY declared input. A check with
# an UNKNOWN closure is affected by every change, so it always runs.
#
# The retired basename rule is kept only as the report-only fallback below, for
# the migration window, and only when the closure map cannot be loaded at all.
changed_surface_touches() {
  local selftest="$1"
  changed_paths_load
  [[ "$CHANGED_PATHS" == "__NO_GIT__" ]] && return 0

  if declare -F vclosure_load >/dev/null 2>&1; then
    if [[ -z "${VCLOSURE_LOADED:-}" ]]; then
      vclosure_load >/dev/null 2>&1 || true
    fi
    if [[ -n "${VCLOSURE_LOADED:-}" ]]; then
      local rel="$selftest"
      [[ "$rel" == "$VCLOSURE_LOADED/"* ]] && rel="${rel#"$VCLOSURE_LOADED"/}"
      local id
      id="$(vclosure_id_for_script "$rel")"
      # A check the closure map does not know is an unknown dependency: run it.
      [[ -n "$id" ]] || return 0
      vclosure_affected "$id" "$CHANGED_PATHS" && return 0
      return 1
    fi
  fi

  # Report-only fallback (migration): the closure map is unreadable, so fall
  # back to the retired basename rule and SAY SO, because a silent fallback to
  # the defective rule is how the defect survives a migration.
  echo "NOTE: closure map unavailable; falling back to the retired basename selection for $(basename "$selftest")" >&2
  local base subject
  base="$(basename "$selftest")"
  subject="${base%-selftest.sh}.sh"

  printf '%s\n' "$CHANGED_PATHS" | grep -qxF "bubbles/scripts/$base" && return 0
  printf '%s\n' "$CHANGED_PATHS" | grep -qxF "bubbles/scripts/$subject" && return 0
  return 1
}

# IMP-012 tiering (opt-in, non-breaking). Default tier=full runs EVERY check
# exactly as before. `--tier=core` runs only the fast, high-signal structural
# subset (registry/lint/generator/scan selftests) for a quick local signal;
# the pre-push / release-check path passes no flag, so it is unchanged.
# `--list-tier=core` DRY-LISTS which checks the core tier would run/skip and
# exits 0 (no execution) — used by the tiering selftest and by operators.
VALIDATE_TIER="${BUBBLES_VALIDATE_TIER:-full}"
LIST_TIER_ONLY="false"

# IMP-027 SCOPE-7 (PERF-1). --tier=core took 260s for 16 of 209 checks. A
# ~25-minute serial pre-push is the strongest practical incentive toward the
# bypass behaviour this framework exists to prevent, so wall clock is a
# governance concern.
#
# WHICH TIER RUNS WHERE (keep this accurate -- a stale claim here sends a new
# check into a tier that never executes in the gate people actually hit):
#   pre-push  -> --tier=core   (see hooks/pre-push.sh; BUBBLES_PREPUSH_TIER=full
#                               opts into the full gate locally)
#   CI        -> the full tier
# So a regression guard that must block a push has to be in core_check_label().
#
# WHICH CALLER FORCES --no-changed-only (IMP-058 SCOPE-4 -- keep this accurate
# too, for the same reason): release-check.sh and hooks/pre-push.sh's
# full-validation branch both pass --no-changed-only explicitly, so neither
# can silently inherit the bare-invocation default below. Anything else that
# invokes this script with no flags gets that default.
#
# --changed-only  restrict to checks whose owned surface the working tree
#                 actually touched. Ownership is DERIVED (a selftest owns the
#                 script it tests), never a hand-maintained manifest -- that
#                 enumeration habit is what COV-2 was.
# --no-cache      ignore the hermetic-selftest result cache for this run.
#
# IMP-058 SCOPE-4 / PERF-13: a bare working-tree invocation now defaults to
# --changed-only, so the dev-loop cost tracks what actually changed instead of
# re-running the whole suite on every call. This default must NEVER reach a
# release path by accident, so it is a SEPARATE opt-in for release-critical
# callers: BUBBLES_VALIDATE_CHANGED_ONLY=false (or the explicit
# --no-changed-only flag) forces the full, unfiltered run regardless of this
# default, and release-check.sh / hooks/pre-push.sh's full-validation branch
# both set it explicitly rather than relying on flag absence. A caller that
# passes neither the env var nor either flag gets the new default; an explicit
# --changed-only or --no-changed-only always wins over both the env var and
# this default.
CHANGED_ONLY="${BUBBLES_VALIDATE_CHANGED_ONLY:-true}"
# IMP-027 SCOPE-7: the result cache is OPT-IN (--cache), never on by default.
#
# The original CORRECTNESS objection is now closed. validate_cache_key() used to
# hash only the SELFTEST file, so a `foo-selftest.sh` kept returning a cached
# PASS after `foo.sh` changed — the same staleness class guard Check 43 exists
# to catch in evidence. The key now covers the selftest AND the script it tests,
# so editing either one invalidates the entry.
#
# One reason to stay opt-in survives, and it was found by a test rather than by
# reasoning: IT DEFEATS DEADLINE ENFORCEMENT. tests/regression/test_28 mutates
# this validator to prove an overdue target gets killed by the watchdog. A
# cached result returns instantly, so the target never runs long enough to be
# killed, `mac.finished` appears, and the deadline assertion fails. A cache that
# can suppress a safety mechanism must not be the default.
#
# Speed is worth having, but only when explicitly requested by someone who knows
# the run is not exercising timing.
CACHE_ENABLED="false"
cache_hits=0
# IMP-047 S-E. --record-debt turns a --changed-only deferral into a real
# obligation in the append-only validation debt ledger. It is opt-in for the
# migration window: the default keeps deferrals report-only so the selection can
# be compared against a cold full run before anything depends on it.
RECORD_DEBT="false"
declare -a deferred_check_ids=()
declare -a deferred_check_labels=()
declare -a deferred_check_cmds=()

# validation_check_id_for <script-path> — the stable id from the closure map, or
# empty when the map does not know this script (which is itself the reason such a
# check is never deferred silently).
validation_check_id_for() {
  local script="${1:-}"
  [[ -n "$script" ]] || return 0
  declare -F vclosure_id_for_script >/dev/null 2>&1 || return 0
  [[ -n "${VCLOSURE_LOADED:-}" ]] || return 0
  local rel="$script"
  [[ "$rel" == "$VCLOSURE_LOADED/"* ]] && rel="${rel#"$VCLOSURE_LOADED"/}"
  vclosure_id_for_script "$rel"
}
for _arg in "$@"; do
  case "$_arg" in
    --tier=core | --tier=full) VALIDATE_TIER="${_arg#--tier=}" ;;
    --list-tier=core | --list-tier=full)
      VALIDATE_TIER="${_arg#--list-tier=}"
      LIST_TIER_ONLY="true"
      ;;
    --changed-only) CHANGED_ONLY="true" ;;
    --no-changed-only) CHANGED_ONLY="false" ;;
    --cache) CACHE_ENABLED="true" ;;
    --no-cache) CACHE_ENABLED="false" ;;
    --record-debt) RECORD_DEBT="true" ;;
    -h | --help)
      echo "Usage: framework-validate.sh [--tier=core|full] [--list-tier=core|full] [--changed-only|--no-changed-only] [--cache] [--no-cache] [--record-debt]"
      echo "  (no flag)        full tier, --changed-only filtering (unless BUBBLES_VALIDATE_CHANGED_ONLY=false)"
      echo "  --tier=core      run only the fast structural/lint/generator subset"
      echo "  --list-tier=core dry-list what the core tier runs/skips, then exit 0"
      echo "  --changed-only   run only checks whose DECLARED CLOSURE the tree touched (default for a bare working-tree invocation; set BUBBLES_VALIDATE_CHANGED_ONLY=false or pass --no-changed-only to force the full run)"
      echo "  --no-changed-only  force the full, unfiltered run even under the new default — release-check.sh and pre-push.sh's full-validation branch both pass this explicitly"
      echo "  --cache          OPT IN to the closure-keyed result cache (off by default)"
      echo "  --no-cache       ignore the result cache"
      echo "  --record-debt    write each --changed-only deferral to the validation debt ledger"
      exit 0
      ;;
    *)
      echo "framework-validate: unknown argument '$_arg'." >&2
      exit 2
      ;;
  esac
done

# IMP-049 SCOPE-2: the run receipt. release-check.sh runs this entire suite as
# its first check — 3743s across 338 checks, measured — on a tree a validate run
# may have proven minutes earlier. A receipt lets that consumer re-derive the
# tree digest itself and decide, rather than re-running on faith.
#
# THE PREVIOUS RECEIPT DIES HERE, before the first check. The tree can be
# byte-identical to the one that passed last time while THIS run is on its way
# to discovering a failure, so a surviving pass-receipt would be consumed on a
# tree whose verdict has just changed. Removing it first is what makes a receipt
# describe only runs that reached an end.
_fv_receipt_ready=false
if [[ "$_fv_outermost" == "true" && "$_fv_executes_checks" == "true" && "$LIST_TIER_ONLY" == "false" ]] \
  && [[ -f "$SCRIPT_DIR/validation-receipt.sh" ]]; then
  # Same defence as the validate-cache source above, and for the same reason:
  # `source` runs in THIS shell, so a sibling that merely exits would terminate
  # framework-validate mid-run — and because it exits 0, the run would look like
  # a silent PASS that validated nothing. Confirming the API is actually defined
  # keeps a stubbed or truncated sibling from being able to end the run. Checked
  # with builtins only, before the portability harness has a full PATH.
  _fv_receipt_src="$(<"$SCRIPT_DIR/validation-receipt.sh")"
  if [[ "$_fv_receipt_src" == *"validation_receipt_invalidate()"* ]]; then
    # shellcheck source=bubbles/scripts/validation-receipt.sh
    source "$SCRIPT_DIR/validation-receipt.sh"
  fi
  unset _fv_receipt_src
  if declare -F validation_receipt_invalidate >/dev/null 2>&1; then
    if validation_receipt_invalidate "$REPO_ROOT"; then
      _fv_receipt_ready=true
    else
      # A receipt we cannot delete is a receipt a later release-check might
      # consume against a tree this run has not finished judging. Say so; the
      # consumer still re-derives the digest, but the operator should know the
      # invalidation did not take.
      printf 'NOTE: could not clear the previous validation receipt; no receipt will be written this run.\n' >&2
    fi
  fi
fi

# fv_write_receipt <verdict> — best-effort, never fails the run. A missing
# receipt costs a consumer nothing but a re-run, which is the safe direction.
fv_write_receipt() {
  [[ "$_fv_receipt_ready" == "true" ]] || return 0
  declare -F validation_receipt_write >/dev/null 2>&1 || return 0
  validation_receipt_write "$REPO_ROOT" "$VALIDATE_TIER" "$1" \
    "${#check_durations[@]}" "$SECONDS" "$CHANGED_ONLY" "$CACHE_ENABLED" 2>/dev/null || true
}

# A check is CORE (fast, high-signal, deterministic) when its label matches one
# of these substrings. The set is intentionally small — structural registry/lint
# consistency + the cheap generator/scan selftests.
core_check_label() {
  case "$1" in
    *"Repository drift report"* | *"Gate-catalog freshness"* | \
      *"Portable surface agnosticity"* | *"Shellcheck lint"* | \
      *"Registry consistency"* | *"YAML schema"* | \
      *"Cheatsheet generator selftest"* | *"Modes split"* | \
      *"Scan-lib"* | *"Derived-artifact regen"* | *"Gate scaffolder"* | \
      *"drift-check selftest"* | *"hub-report selftest"* | \
      *"guard-lib timeout fallback"*) # portable-ok: case pattern matching a check NAME, not a timeout invocation
      return 0
      ;;
    # The LIVE checks in core are cheap blockers: release-manifest freshness
    # catches a stale generated artifact, while G128 enforces the authoritative
    # host-session budget on every executing tier.
    *"Release manifest freshness"* | *"Session cap guard (live, G128)"*)
      return 0
      ;;
    *) return 1 ;;
  esac
}

_fv_resolve_lsof_path() {
  local _candidate=""

  for _candidate in /usr/bin/lsof /usr/sbin/lsof; do
    if [[ -x "$_candidate" ]]; then
      printf '%s\n' "$_candidate"
      return 0
    fi
  done
  return 1
}

run_check() {
  local label="$1"
  shift

  if [[ "$VALIDATE_TIER" == "core" ]] && ! core_check_label "$label"; then
    if [[ "$LIST_TIER_ONLY" == "true" ]]; then
      echo "WOULD-SKIP (non-core): $label"
    else
      echo "==> $label"
      echo "SKIP: $label (tier=core)"
      skipped=$((skipped + 1))
      skipped_tier=$((skipped_tier + 1))
      echo
    fi
    return 0
  fi

  # IMP-027 SCOPE-7. Both filters below apply ONLY to hermetic selftests, which
  # the framework's own contract says build their own fixtures and depend on
  # nothing outside their source. A live guard reads the working tree -- the
  # very thing that changes between runs -- so skipping one would report a
  # verdict about a tree that was never inspected.
  local _script=""
  if [[ "${1:-}" == "bash" && "$#" -eq 2 ]]; then
    case "$(basename "${2:-}")" in
      *-selftest.sh) _script="$2" ;;
      test_33_traceability_current_scope_universe.sh) _script="$2" ;;
    esac
  fi

  # Decided before the dry-list return so `--list-tier` reflects it too.
  local _changed_skip="false"
  if [[ -n "$_script" && "$CHANGED_ONLY" == "true" ]] && ! changed_surface_touches "$_script"; then
    _changed_skip="true"
  fi

  if [[ "$LIST_TIER_ONLY" == "true" ]]; then
    if [[ "$_changed_skip" == "true" ]]; then
      echo "WOULD-SKIP (--changed-only): $label"
    else
      echo "WOULD-RUN: $label"
    fi
    return 0
  fi

  if [[ "$_changed_skip" == "true" ]]; then
    # IMP-047 S-E vocabulary. This is a DEFERRAL, not a pass and not a silent
    # skip: the check has not run and something must still run it. The word
    # stays distinct from PASS and REUSED so a scrollback cannot blur the three
    # into "it was fine".
    echo "==> $label"
    echo "DEFERRED: $label (--changed-only; no declared closure input was touched)"
    skipped=$((skipped + 1))
    skipped_changed_only=$((skipped_changed_only + 1))
    if [[ "$RECORD_DEBT" == "true" ]]; then
      local _did
      _did="$(validation_check_id_for "$_script")"
      deferred_check_ids+=("${_did:-unknown}")
      deferred_check_labels+=("$label")
      deferred_check_cmds+=("$(printf '%q ' "$@")")
    fi
    echo
    return 0
  fi

  local _cache_key=""
  if [[ -n "$_script" && "$CACHE_ENABLED" == "true" ]] && declare -F validate_cache_key >/dev/null 2>&1; then
    _cache_key="$(validate_cache_key "$_script" "$FRAMEWORK_VERSION" 2>/dev/null || true)"
    if [[ -z "$_cache_key" && "$(basename "$_script")" == "test_33_traceability_current_scope_universe.sh" ]] \
      && declare -F vclosure_digest >/dev/null 2>&1; then
      [[ -n "${VCLOSURE_LOADED:-}" ]] || vclosure_load >/dev/null 2>&1 || true
      _cr02_check_id="$(validation_check_id_for "$_script")"
      if [[ -n "$_cr02_check_id" ]]; then
        _cr02_digest="$(vclosure_digest "$_cr02_check_id" "$FRAMEWORK_VERSION" 2>/dev/null || true)"
        [[ -n "$_cr02_digest" ]] && _cache_key="$FRAMEWORK_VERSION-$_cr02_digest"
      fi
    fi
    if [[ -n "$_cache_key" ]] && validate_cache_get "$_cache_key"; then
      # IMP-047 S-E: REUSED is its own result, and it NEVER prints PASS. PASS
      # means this run executed the check and watched it succeed. Reuse means
      # this run executed nothing and is citing an earlier verdict, which is a
      # weaker claim, and printing the stronger word for the weaker claim is how
      # a suite reports work it did not do.
      local _receipt="unknown"
      declare -F validate_cache_receipt >/dev/null 2>&1 \
        && _receipt="$(validate_cache_receipt "$_cache_key" 2>/dev/null || echo unknown)"
      echo "==> $label"
      echo "REUSED: $label (receipt $_receipt — every declared closure input unchanged)"
      cache_hits=$((cache_hits + 1))
      echo
      return 0
    fi
  fi

  echo "==> $label"
  local _started="$SECONDS"
  local _rc=0 _cap="" _ci_tree_rc=0 _ci_escape_detected=0
  _fv_process_identity() {
    local _identity_pid="$1" _proc_stat="" _proc_tail="" _proc_start=""
    local _ps_path="" _ps_output="" _observed_pid="" _weekday=""
    local _month="" _day="" _started_at="" _year="" _extra=""
    local -a _proc_fields=()

    [[ "$_identity_pid" =~ ^[1-9][0-9]*$ ]] || return 1
    if [[ -r "/proc/$_identity_pid/stat" ]]; then
      IFS= builtin read -r _proc_stat <"/proc/$_identity_pid/stat" || return 1
      _proc_tail="${_proc_stat##*) }"
      [[ "$_proc_tail" != "$_proc_stat" ]] || return 1
      IFS=' ' builtin read -r -a _proc_fields <<<"$_proc_tail"
      [[ "${#_proc_fields[@]}" -gt 19 ]] || return 1
      _proc_start="${_proc_fields[19]}"
      [[ "$_proc_start" =~ ^[0-9]+$ ]] || return 1
      printf 'proc-start:%s:%s\n' "$_identity_pid" "$_proc_start"
      return 0
    fi
    if [[ -x /bin/ps ]]; then
      _ps_path=/bin/ps
    elif [[ -x /usr/bin/ps ]]; then
      _ps_path=/usr/bin/ps
    else
      return 1
    fi
    _ps_output="$(LC_ALL=C "$_ps_path" -o pid= -o lstart= -p "$_identity_pid" 2>/dev/null)" || return 1
    [[ -n "$_ps_output" && "$_ps_output" != *$'\n'* ]] || return 1
    IFS=' ' builtin read -r _observed_pid _weekday _month _day _started_at _year _extra <<<"$_ps_output"
    [[ "$_observed_pid" == "$_identity_pid" && -n "$_weekday" \
      && -n "$_month" && -n "$_day" && -n "$_started_at" \
      && -n "$_year" && -z "$_extra" ]] || return 1
    printf 'ps-lstart:%s:%s:%s:%s:%s:%s\n' \
      "$_observed_pid" "$_weekday" "$_month" "$_day" "$_started_at" "$_year"
  }
  _fv_observe_capture_holders() {
    local _observer_label="$1"
    local _observer_path="" _observer_output="" _observer_status=0 _observer_pid=""
    shift

    if ! _observer_path="$(_fv_resolve_lsof_path)"; then
      printf 'ERROR: detached descriptor observer lsof is unavailable for %s\n' \
        "$_observer_label" >&2
      return 2
    fi
    if _observer_output="$(builtin command "$_observer_path" -t "$@" 2>&1)"; then
      _observer_status=0
    else
      _observer_status=$?
    fi
    # lsof reports a clean no-match as status 1 with no output on BSD and GNU
    # implementations. Any output in that state is an observation failure.
    if [[ "$_observer_status" -eq 1 && -z "$_observer_output" ]]; then
      return 0
    fi
    if [[ "$_observer_status" -ne 0 ]]; then
      printf 'ERROR: detached descriptor observer lsof failed with status %s for %s\n' \
        "$_observer_status" "$_observer_label" >&2
      return 2
    fi
    [[ -n "$_observer_output" ]] || return 0
    while IFS= read -r _observer_pid; do
      if [[ ! "$_observer_pid" =~ ^[1-9][0-9]*$ ]]; then
        printf 'ERROR: detached descriptor observer lsof returned malformed output for %s\n' \
          "$_observer_label" >&2
        return 2
      fi
    done <<<"$_observer_output"
    printf '%s' "$_observer_output"
  }
  _fv_capture_holder_matches() {
    local _holder_label="$1" _holder_pid="$2" _holder_cap="$3"
    local _expected_identity="${4:-}" _identity_before="" _identity_after=""
    local _holder_observation=""

    _capture_holder_verified_identity=""
    if ! _identity_before="$(_fv_process_identity "$_holder_pid")"; then
      if builtin kill -0 "$_holder_pid" 2>/dev/null; then
        printf 'ERROR: could not establish stable process identity for detached holder %s in %s\n' \
          "$_holder_pid" "$_holder_label" >&2
        return 2
      fi
      return 1
    fi
    [[ -z "$_expected_identity" || "$_identity_before" == "$_expected_identity" ]] || return 1
    if ! _holder_observation="$(_fv_observe_capture_holders \
      "$_holder_label" -a -p "$_holder_pid" "$_holder_cap")"; then
      return 2
    fi
    [[ "$_holder_observation" == "$_holder_pid" ]] || return 1
    if ! _identity_after="$(_fv_process_identity "$_holder_pid")"; then
      if builtin kill -0 "$_holder_pid" 2>/dev/null; then
        printf 'ERROR: could not revalidate stable process identity for detached holder %s in %s\n' \
          "$_holder_pid" "$_holder_label" >&2
        return 2
      fi
      return 1
    fi
    [[ "$_identity_after" == "$_identity_before" ]] || return 1
    [[ -z "$_expected_identity" || "$_identity_after" == "$_expected_identity" ]] || return 1
    _capture_holder_verified_identity="$_identity_after"
  }
  # Only CI captures output. A private regular file is deliberate: a pipeline
  # reader waits for EOF from every inherited writer, so an arbitrary orphaned
  # descendant can deadlock the validator after the direct check has returned.
  # The regular file lets us wait for the direct command, preserve its exact
  # status, then replay every line captured by that point without making
  # completion depend on descendant-held descriptors. Locally the invocation
  # stays exactly as it was, preserving stdout/stderr separation.
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    if _cap="$(mktemp "${TMPDIR:-/tmp}/bubbles-framework-validate-capture.XXXXXXXX")"; then
      # Force the direct check into its own process group. Do not depend on the
      # caller's job-control state: enable monitor mode for the launch, then
      # restore it immediately. With a single-command background job, $! is
      # both the direct child PID and the owned process-group ID, so negative
      # signals below can never target this validator's process group.
      local _monitor_was_enabled=0 _check_pid="" _termination_waited=0
      local _capture_holders="" _capture_holder_pid="" _capture_holder_count=0
      local _capture_observer_failed=0 _capture_holder_match_status=0
      local _capture_cleanup_round=0 _capture_cleanup_pending=0
      local _capture_holder_verified_identity=""
      local -A _capture_holder_identities=()
      [[ "$-" == *m* ]] && _monitor_was_enabled=1
      if set -m; then
        "$@" >"$_cap" 2>&1 &
        _check_pid=$!
        if [[ "$_monitor_was_enabled" -eq 0 ]] && ! set +m; then
          printf 'ERROR: could not restore job-control state after launching %s\n' "$label" >&2
          _ci_tree_rc=1
        fi

        # Wait for the direct command first and retain its exact status. A
        # completed parent can leave descendants alive in the same owned group,
        # so close that tree before replaying the regular-file capture.
        if wait "$_check_pid" 2>/dev/null; then _rc=0; else _rc=$?; fi
        if [[ "$_check_pid" =~ ^[1-9][0-9]*$ ]] \
          && [[ "$_check_pid" != "$$" ]] \
          && kill -0 -- "-$_check_pid" 2>/dev/null; then
          kill -TERM -- "-$_check_pid" 2>/dev/null || true
          _termination_waited=0
          while kill -0 -- "-$_check_pid" 2>/dev/null \
            && [[ "$_termination_waited" -lt 3 ]]; do
            sleep 1
            _termination_waited=$((_termination_waited + 1))
          done
          if kill -0 -- "-$_check_pid" 2>/dev/null; then
            kill -KILL -- "-$_check_pid" 2>/dev/null || true
            _termination_waited=0
            while kill -0 -- "-$_check_pid" 2>/dev/null \
              && [[ "$_termination_waited" -lt 3 ]]; do
              sleep 1
              _termination_waited=$((_termination_waited + 1))
            done
          fi
          # The direct child was already reaped above. A second wait is the
          # only portable reap available if Bash still retained job state.
          wait "$_check_pid" 2>/dev/null || true
          if kill -0 -- "-$_check_pid" 2>/dev/null; then
            printf 'ERROR: process group %s for %s survived bounded TERM/KILL cleanup\n' \
              "$_check_pid" "$label" >&2
            _ci_tree_rc=1
          fi
        fi

        # A descendant can call setsid(2) and leave the owned process group
        # while retaining both the validator lock and this check's private
        # capture descriptor. The random capture path is a per-check ownership
        # witness. Bind each observed PID to its process start identity, then
        # revalidate that identity around a targeted descriptor observation
        # immediately before bounded TERM and KILL attempts.
        if _capture_holders="$(_fv_observe_capture_holders "$label" "$_cap")"; then
          while IFS= read -r _capture_holder_pid; do
            [[ "$_capture_holder_pid" =~ ^[1-9][0-9]*$ ]] || continue
            [[ "$_capture_holder_pid" != "$$" && "$_capture_holder_pid" != "$_check_pid" ]] || continue
            _capture_holder_match_status=0
            if _fv_capture_holder_matches "$label" "$_capture_holder_pid" "$_cap"; then
              _capture_holder_identities["$_capture_holder_pid"]="$_capture_holder_verified_identity"
              _capture_holder_count=$((_capture_holder_count + 1))
              if _fv_capture_holder_matches "$label" "$_capture_holder_pid" "$_cap" \
                "${_capture_holder_identities[$_capture_holder_pid]}"; then
                kill -TERM "$_capture_holder_pid" 2>/dev/null || true
              else
                _capture_holder_match_status=$?
                if [[ "$_capture_holder_match_status" -eq 2 ]]; then
                  _capture_observer_failed=1
                  _ci_tree_rc=1
                  break
                fi
              fi
            else
              _capture_holder_match_status=$?
              if [[ "$_capture_holder_match_status" -eq 2 ]]; then
                _capture_observer_failed=1
                _ci_tree_rc=1
                break
              fi
            fi
          done <<<"$_capture_holders"
          if [[ "$_capture_observer_failed" -eq 0 && "$_capture_holder_count" -gt 0 ]]; then
            _ci_escape_detected=1
            _ci_tree_rc=1
            _capture_cleanup_round=0
            while [[ "$_capture_cleanup_round" -lt 30 ]]; do
              _capture_cleanup_pending=0
              while IFS= read -r _capture_holder_pid; do
                [[ "$_capture_holder_pid" =~ ^[1-9][0-9]*$ ]] || continue
                [[ -n "${_capture_holder_identities[$_capture_holder_pid]:-}" ]] || continue
                _capture_holder_match_status=0
                if _fv_capture_holder_matches "$label" "$_capture_holder_pid" "$_cap" \
                  "${_capture_holder_identities[$_capture_holder_pid]}"; then
                  _capture_cleanup_pending=1
                else
                  _capture_holder_match_status=$?
                  if [[ "$_capture_holder_match_status" -eq 2 ]]; then
                    _capture_observer_failed=1
                    _capture_cleanup_pending=1
                    _ci_tree_rc=1
                    break
                  fi
                fi
              done <<<"$_capture_holders"
              [[ "$_capture_observer_failed" -eq 0 ]] || break
              [[ "$_capture_cleanup_pending" -eq 1 ]] || break
              sleep 0.1
              _capture_cleanup_round=$((_capture_cleanup_round + 1))
            done
            if [[ "$_capture_observer_failed" -eq 0 ]]; then
              while IFS= read -r _capture_holder_pid; do
                [[ "$_capture_holder_pid" =~ ^[1-9][0-9]*$ ]] || continue
                [[ -n "${_capture_holder_identities[$_capture_holder_pid]:-}" ]] || continue
                _capture_holder_match_status=0
                if _fv_capture_holder_matches "$label" "$_capture_holder_pid" "$_cap" \
                  "${_capture_holder_identities[$_capture_holder_pid]}"; then
                  kill -KILL "$_capture_holder_pid" 2>/dev/null || true
                else
                  _capture_holder_match_status=$?
                  if [[ "$_capture_holder_match_status" -eq 2 ]]; then
                    _capture_observer_failed=1
                    _ci_tree_rc=1
                    break
                  fi
                fi
              done <<<"$_capture_holders"
            fi
            if [[ "$_capture_observer_failed" -eq 0 ]]; then
              _capture_cleanup_round=0
              while [[ "$_capture_cleanup_round" -lt 30 ]]; do
                _capture_cleanup_pending=0
                while IFS= read -r _capture_holder_pid; do
                  [[ "$_capture_holder_pid" =~ ^[1-9][0-9]*$ ]] || continue
                  [[ -n "${_capture_holder_identities[$_capture_holder_pid]:-}" ]] || continue
                  _capture_holder_match_status=0
                  if _fv_capture_holder_matches "$label" "$_capture_holder_pid" "$_cap" \
                    "${_capture_holder_identities[$_capture_holder_pid]}"; then
                    _capture_cleanup_pending=1
                  else
                    _capture_holder_match_status=$?
                    if [[ "$_capture_holder_match_status" -eq 2 ]]; then
                      _capture_observer_failed=1
                      _capture_cleanup_pending=1
                      _ci_tree_rc=1
                      break
                    fi
                  fi
                done <<<"$_capture_holders"
                [[ "$_capture_observer_failed" -eq 0 ]] || break
                [[ "$_capture_cleanup_pending" -eq 1 ]] || break
                sleep 0.1
                _capture_cleanup_round=$((_capture_cleanup_round + 1))
              done
            fi
            if [[ "$_capture_observer_failed" -eq 1 ]]; then
              printf 'ERROR: detached descriptor observation failed during bounded cleanup for %s\n' \
                "$label" >&2
            elif [[ "$_capture_cleanup_pending" -eq 1 ]]; then
              printf 'ERROR: %s detached process(es) for %s retained the private CI capture after bounded cleanup\n' \
                "$_capture_holder_count" "$label" >&2
            else
              printf 'ERROR: %s detached process(es) escaped %s; bounded cleanup completed and validation is refused\n' \
                "$_capture_holder_count" "$label" >&2
            fi
          fi
        else
          _capture_observer_failed=1
          _ci_tree_rc=1
        fi
      else
        printf 'ERROR: could not create a distinct process group for %s\n' "$label" >&2
        _rc=1
        _ci_tree_rc=1
      fi
      if ! cat "$_cap"; then
        echo "ERROR: could not replay the captured output for $label" >&2
        _ci_tree_rc=1
      fi
    else
      echo "ERROR: could not create a private CI capture file for $label" >&2
      _rc=1
      _ci_tree_rc=1
    fi
  else
    if "$@"; then _rc=0; else _rc=$?; fi
  fi
  if [[ "$_rc" -eq 0 && "$_ci_tree_rc" -eq 0 ]]; then
    echo "PASS: $label"
    [[ -n "$_cache_key" ]] && validate_cache_put "$_cache_key" 0
  else
    # Additive, GitHub-gated (OW-002): also surface the failing check as a
    # check-run annotation, which is readable UNAUTHENTICATED even though the
    # raw job log needs admin (403). Local output is unchanged. The captured
    # assertion lines make the failure diagnosable without the 403-gated log.
    echo "FAIL: $label"
    local _detail=""
    [[ -n "$_cap" ]] && _detail="$(bubbles_ci_failure_detail "$_cap")"
    if [[ -n "$_detail" ]]; then
      bubbles_ci_annotate_failure "FAIL: ${label}"$'\n'"${_detail}"
    else
      bubbles_ci_annotate_failure "FAIL: $label"
    fi
    failures=$((failures + 1))
    failed_check_labels+=("$label")
  fi
  [[ -n "$_cap" ]] && rm -f "$_cap"
  check_durations+=("$((SECONDS - _started))|$label")
  echo
  if [[ "$_ci_escape_detected" -eq 1 ]]; then
    return 1
  fi
}

# Focused, internal harness for the framework-validation wiring selftest. This
# deliberately enters through the production run_check implementation above,
# including its cache lookup/write behavior, but exits before the normal
# schedule so the wiring selftest can prove fail-closed and cache semantics
# without recursively scheduling itself or running the full framework suite.
if [[ -n "${BUBBLES_FRAMEWORK_VALIDATE_READER_PROBE:-}" ]]; then
  _reader_probe_script="$SCRIPT_DIR/scenario-reference-reader-"'selftest.sh'
  run_check "Scenario reference reader selftest (IMP-040 / COV-8)" \
    bash "$_reader_probe_script"
  if [[ "$failures" -ne 0 ]]; then
    printf 'framework-validate reader probe: FAILED (%s failure(s))\n' "$failures" >&2
    exit 1
  fi
  printf 'framework-validate reader probe: PASSED\n'
  exit 0
fi

# Wrapper for selftests that only make sense when run inside the framework
# source tree (those that invoke install.sh, walk VERSION, or assert the
# framework's own README/docs layout). When INSTALL_MODE != "source", emit
# a SKIP line instead of running them so downstream framework-validate
# exits 0 with explicit accounting instead of FAIL'ing on
# expected-to-be-missing files.
run_check_self_only() {
  local label="$1"
  shift

  if [[ "$INSTALL_MODE" != "source" ]]; then
    echo "==> $label"
    echo "SKIP: $label (framework-source-only; install-mode=$INSTALL_MODE)"
    skipped=$((skipped + 1))
    skipped_self_only=$((skipped_self_only + 1))
    echo
    return 0
  fi
  run_check "$label" "$@"
}

# Focused CR-02 harness. Like the reader probe above, this enters through the
# production source-only wrapper and run_check/cache implementation, then exits
# before the normal schedule. The wiring selftest can therefore prove blocking
# and cache behavior without recursively invoking itself or the full suite.
if [[ -n "${BUBBLES_FRAMEWORK_VALIDATE_CR02_PROBE:-}" ]]; then
  _cr02_probe_script="$REPO_ROOT/tests/regression/test_33_traceability_current_scope_"'universe.sh'
  run_check_self_only "CR-02 traceability current-scope universe regression" \
    bash "$_cr02_probe_script"
  if [[ "$failures" -ne 0 ]]; then
    printf 'framework-validate CR-02 probe: FAILED (%s failure(s))\n' "$failures" >&2
    exit 1
  fi
  printf 'framework-validate CR-02 probe: PASSED\n'
  exit 0
fi

echo "Bubbles Framework Validation"
echo "Repository: $REPO_ROOT"
echo "Install mode: $INSTALL_MODE"
echo

# Resolve the managed interpreter for every check this run spawns.
#
# cli.sh does this before dispatch, so `cli.sh framework-validate` has satisfied
# python3 while `bash framework-validate.sh` does not -- and the latter is a real
# entry point: v5.3-selftest runs the DOWNSTREAM copy exactly that way. Without
# it, every python-dependent check reads an empty registry and reports a content
# mismatch instead of a missing module. That discrepancy produced a check that
# passed through one entry point and failed through the other, with nothing
# naming the difference.
#
# python-env.sh is EXECUTED, never sourced. cli.sh can source it safely because
# cli.sh is not the script that repo-drift-report-selftest stages against a tree
# of stubs -- and in that tree every sibling is a stub ending in `exit 0`, which
# in a SOURCED file terminates this validator's own shell and reports a silent
# pass that validated nothing. That fixture exists precisely to catch this, and
# it did. A subprocess cannot end this run no matter what the file contains.
#
# The interpreter is only prepended when the one already on PATH cannot satisfy
# the framework's declared requirements, so an operator's working environment is
# never displaced. The import list duplicates dependency-posture.sh's declared
# set; that is a deliberate two-line duplication rather than a second source of
# truth, because resolving it through the helper would mean sourcing it.
if [[ -f "$SCRIPT_DIR/python-env.sh" ]] \
  && ! python3 -c 'import yaml, jsonschema' >/dev/null 2>&1; then
  bubbles_managed_python="$(bash "$SCRIPT_DIR/python-env.sh" --path 2>/dev/null || true)"
  if [[ -n "$bubbles_managed_python" && -x "$bubbles_managed_python" ]]; then
    PATH="$(dirname "$bubbles_managed_python"):$PATH"
    export PATH
  fi
fi

run_check "Repository drift report (informational)" bash "$SCRIPT_DIR/repo-drift-report.sh" --repo-root "$REPO_ROOT"
run_check "Gate-catalog freshness advisory (informational, IMP-005)" bash "$SCRIPT_DIR/gate-catalog-freshness.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Portable surface agnosticity" bash "$SCRIPT_DIR/agnosticity-lint.sh" --quiet
# Cheap structural checks run BEFORE the whole-tree ShellCheck scan. A broken
# registry or malformed schema is a one-second answer, and making the operator
# wait ~42s to hear it is the difference between a fast loop and a slow one.
run_check "Registry consistency selftest" bash "$SCRIPT_DIR/registry-consistency-selftest.sh"
run_check "YAML schema validate" bash "$SCRIPT_DIR/yaml-schema-validate.sh"
run_check_self_only "Git hook environment sanitization selftest" bash "$SCRIPT_DIR/hooks/git-env-sanitize-selftest.sh"
run_check_self_only "Shellcheck lint (v7.0.2, -S warning, zero findings)" bash "$SCRIPT_DIR/shellcheck-lint.sh" --quiet
run_check_self_only "Shellcheck lint selftest (v7.0.2)" bash "$SCRIPT_DIR/shellcheck-lint-selftest.sh"
run_check_self_only "Cheatsheet generator selftest (v6.0 / B7)" bash "$SCRIPT_DIR/generate-cheatsheet-selftest.sh"
run_check_self_only "Agent roster coverage (v7.18.0)" bash "$SCRIPT_DIR/agent-roster-coverage.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Agent roster coverage selftest (v7.18.0)" bash "$SCRIPT_DIR/agent-roster-coverage-selftest.sh"
run_check "Tool-log selftest (v5.1 / M1)" bash "$SCRIPT_DIR/tool-log-selftest.sh"
run_check "Evidence-tool-log bridge selftest (v6.0 / B1)" bash "$SCRIPT_DIR/evidence-tool-log-bridge-selftest.sh"
run_check "Diff-evidence guard selftest (v6.0 / B2)" bash "$SCRIPT_DIR/diff-evidence-guard-selftest.sh"
run_check "Result-envelope validate selftest (v6.0 / B3)" bash "$SCRIPT_DIR/result-envelope-validate-selftest.sh"
run_check "Artifact-lint certifying-window selftest (v7.17.0)" bash "$SCRIPT_DIR/artifact-lint-selftest.sh"
run_check "Skill-evolution selftest (v7.16.0 / IMP-016)" bash "$SCRIPT_DIR/skill-evolution-selftest.sh"
run_check "Inventory parity check selftest (IMP-005)" bash "$SCRIPT_DIR/inventory-parity-check-selftest.sh"
# Live parity check is framework-source-only: skills/INVENTORY.md is a source-repo
# artifact and is not vendored into downstream install trees.
run_check_self_only "Inventory parity check (live, IMP-005)" bash "$SCRIPT_DIR/inventory-parity-check.sh" "$REPO_ROOT"
# Framework-source-only for the same reason (IMP-042 SCOPE-12): this selftest reads
# the LIVE .specify/memory/bubbles.config.json and drives developer-profile.sh set
# against it, so downstream it asserts against whatever profile that repo chose and
# fails on a correct configuration. Enumerating it here also removes it from the
# discovery sweep, which ran it as portable.
run_check_self_only "Profile transition selftest" bash "$SCRIPT_DIR/profile-transition-selftest.sh"
# Skill invocation/description-load classification (IMP-021 SCOPE-5): a hermetic
# selftest proving the report sums auto-discovery description bytes and flags a
# class-less skill row, PLUS a live source-only report (skills/INVENTORY.md is a
# source-repo artifact) that prints the aggregate always-loaded description load
# report-only (exit 0, no threshold) and fails only if a real row omits its class.
run_check "Skill description-load report selftest (IMP-021 SCOPE-5)" bash "$SCRIPT_DIR/skill-description-load-selftest.sh"
run_check_self_only "Skill description-load report (live, IMP-021 SCOPE-5)" bash "$SCRIPT_DIR/skill-description-load.sh" --repo-root "$REPO_ROOT" --summary
# Case-collision guard (IMP-017): the hermetic selftest PLUS a live scan of the
# repo's tracked files. The live check is deliberately NOT source-only — a
# case-only duplicate path is a defect in ANY git repo (downstream installs
# included), and the guard no-ops gracefully outside a git work tree.
run_check "Case-collision guard selftest (IMP-017)" bash "$SCRIPT_DIR/case-collision-guard-selftest.sh"
run_check "Case-collision guard (live, IMP-017)" bash "$SCRIPT_DIR/case-collision-guard.sh" --repo-root "$REPO_ROOT"
# Workflow YAML validity (IMP-102 / SCOPE-3): live scan of every
# .github/workflows/*.yml|*.yaml PLUS an always-on adversarial red fixture
# reproducing the col-0 python-continuation defect that silently disabled the
# CI state-transition anti-fabrication chain. Not source-only — a workflow
# GitHub cannot load is a defect in ANY repo; it no-ops when no workflows /
# no PyYAML are present.
run_check "Workflow YAML validity selftest (IMP-102 / SCOPE-3)" bash "$SCRIPT_DIR/workflow-yaml-validity-selftest.sh"
# macOS/WSL portability guard: run its HERMETIC selftest (green + one red fixture
# per class + self-portability), NOT a scan of the framework's own scripts (which
# intentionally use raw timeout/sed -i mediated by guard-lib + the PATH shim).
macos_portability_guard_timeout_seconds="${BUBBLES_MACOS_PORTABILITY_GUARD_SELFTEST_TIMEOUT_SECONDS:-120}"
run_check "macOS portability guard selftest (bubbles-cross-platform-shell)" bubbles_run_with_timeout "$macos_portability_guard_timeout_seconds" bash "$SCRIPT_DIR/macos-portability-guard-selftest.sh"
# BSD-userland simulator (OW-002): the PATH shim at the top of this file only
# works in the macOS-to-GNU direction -- it lets a Mac run GNU-shaped code.
# Nothing let a Linux host run BSD-shaped userland, so a macOS-only failure could
# not be reproduced without a Mac, and the release-hygiene-macos job's logs are
# not readable from a workstation. That is why OW-002's macOS failures sat
# unattributed. Run the simulator's HERMETIC selftest, never a live scan: the
# simulator is opt-in tooling that nothing executes unless a caller assigns its
# output to PATH. The selftest asserts TRANSLATION rather than mere acceptance,
# because a shim that only rejected GNU spellings would break the correct BSD
# branch too and produce false attributions -- worse than having no simulator.
run_check "BSD-userland simulator selftest (OW-002)" bubbles_run_with_timeout 120 bash "$SCRIPT_DIR/bsd-userland-sim-selftest.sh"
# guard-lib timeout fallback (OW-009): on a host with no coreutils timeout the
# fallback watchdog must NOT inherit the caller's stdout pipe, or a command
# substitution blocks for the FULL timeout even after the command already
# exited. That is the 129s vs 9s state-transition-guard run reported on a stock
# macOS PATH. The selftest forces the fallback branch on EVERY platform, so
# Linux CI protects macOS.
run_check "guard-lib timeout fallback selftest (OW-009)" bubbles_run_with_timeout 120 bash "$SCRIPT_DIR/guard-lib-timeout-selftest.sh"
# CI annotation emitter (OW-002): raw job logs need repo ADMIN and answer 403,
# so a red macOS release-hygiene job was unattributable from an unprivileged
# machine. Check-run annotations ARE readable unauthenticated, so every FAIL
# also emits `::error::` under GITHUB_ACTIONS. The selftest pins that the
# annotation is additive, gated (no local noise), and correctly escaped.
if [[ -x "$SCRIPT_DIR/ci-annotation-emitter-selftest.sh" ]]; then
  run_check_self_only "CI annotation emitter selftest (OW-002)" bubbles_run_with_timeout 120 bash "$SCRIPT_DIR/ci-annotation-emitter-selftest.sh"
fi
# Bash baseline guard (IMP-102 / SCOPE-5): proves the shipped command surface
# (cli.sh, framework-validate.sh) fails LOUDLY and EARLY on bash < 4 instead of
# silently masking declare -A breakage — positive static + functional + an
# adversarial guard-removed fixture that must break the static check.
run_check "Bash baseline guard selftest (IMP-102 / SCOPE-5)" bash "$SCRIPT_DIR/bash-baseline-guard-selftest.sh"
# Evidence-Backed Experience Recall (IMP-037). Registration is explicit here
# because these carry per-suite timeouts a glob cannot infer; the discovery
# sweep near the end of this file now runs any *-selftest.sh that no enumerated
# check already scheduled. These six were green but unwired through SCOPE-5,
# which meant ~516 assertions guarded nothing in pre-push or release-check.
# Timeouts are ~4x the measured runtime (1s/1s/11s/45s/16s/7s) so a normal run
# never trips them but a hang still fails instead of blocking the gate forever.
run_check "Experience-recall resolver selftest (IMP-037 / SCOPE-1)" bubbles_run_with_timeout 60 bash "$SCRIPT_DIR/experience-recall-resolve-selftest.sh"
run_check "Experience-recall adapter-contract selftest (IMP-037 / SCOPE-1)" bubbles_run_with_timeout 60 bash "$SCRIPT_DIR/experience-recall-adapter-contract-selftest.sh"
run_check "Experience-recall indexer selftest (IMP-037 / SCOPE-2)" bubbles_run_with_timeout 120 bash "$SCRIPT_DIR/experience-recall-index-selftest.sh"
run_check "Experience-recall CLI selftest (IMP-037 / SCOPE-3)" bubbles_run_with_timeout 300 bash "$SCRIPT_DIR/experience-recall-cli-selftest.sh"
run_check "Experience-recall lifecycle selftest (IMP-037 / SCOPE-4)" bubbles_run_with_timeout 180 bash "$SCRIPT_DIR/experience-recall-lifecycle-selftest.sh"
# The authority firewall: recalled experience is tier 4 and advisory, so a
# recall id/index path/export can never be cited as evidence -- refused in
# EVERY mode including --advisory, because that is an authority breach, not a
# schema nit. Also pins the validator's fallback constants to the indexer's.
run_check "Experience-recall authority firewall selftest (IMP-037 / SCOPE-6)" bubbles_run_with_timeout 120 bash "$SCRIPT_DIR/experience-recall-authority-selftest.sh"
# Retrieval QUALITY, not just correctness: a labeled corpus measures macro
# precision/recall at the result bound and attacks repository isolation, anchor
# validity, freshness, lifecycle, corpus admission, and prompt injection. A
# provider that retrieves nothing useful passes every other selftest.
# Source-only: the corpus lives under bubbles/eval/, which does not ship, so
# downstream this could only ever SKIP.
run_check_self_only "Experience-recall evaluation selftest (IMP-037 / SCOPE-7)" bubbles_run_with_timeout 300 bash "$SCRIPT_DIR/experience-recall-eval-selftest.sh"
run_check_self_only "Installer manifest check (v6.0 / B9)" bash "$SCRIPT_DIR/generate-installer.sh"
run_check_self_only "Installer manifest selftest (v6.0 / B9)" bash "$SCRIPT_DIR/generate-installer-selftest.sh"
run_check_self_only "Payload integrity verifier selftest (IMP-101 / SCOPE-8)" bash "$SCRIPT_DIR/verify-payload-integrity-selftest.sh"
run_check_self_only "Upgrade transactionality selftest (IMP-102 / SCOPE-6)" bash "$SCRIPT_DIR/upgrade-transactionality-selftest.sh"
if [[ -x "$SCRIPT_DIR/migrate-modes-v5-to-v6.sh" ]]; then
  run_check_self_only "Migrate-modes-v5-to-v6 selftest (v6.0 / C1)" bash "$SCRIPT_DIR/migrate-modes-v5-to-v6-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/generate-modes-block.sh" ]]; then
  run_check "Modes split no-duplication (v6.1 / S2)" bash "$SCRIPT_DIR/generate-modes-block.sh" --check
fi
# IMP-047 S-A: the `gates-block-reader` apparatus retired here. It guarded a
# generated `gates:` block in workflows.yaml that IMP-042 SCOPE-13 removed, so
# the lint had been computing an empty inventory and printing "the block is safe
# to remove" on every run while still exiting 0 — which is why two checks stayed
# scheduled against nothing. The corollary is now mechanical: a lint that reports
# its own removal precondition must refuse, so the next orphaned guard fails
# loudly instead of running forever. The live scan is source-only because it
# reads this repository's own script tree.
if [[ -x "$SCRIPT_DIR/orphaned-scaffolding-guard.sh" ]]; then
  run_check "Orphaned-scaffolding rule selftest (IMP-047 S-A)" bash "$SCRIPT_DIR/orphaned-scaffolding-guard-selftest.sh"
  run_check_self_only "Orphaned scaffolding (live, IMP-047 S-A)" bash "$SCRIPT_DIR/orphaned-scaffolding-guard.sh" --repo-root "$REPO_ROOT"
fi
# IMP-102 / SCOPE-9: gate-coverage map — advisory generated doc mapping every
# gate to its enforcing surface(s) (modes / state-transition-guard / framework-
# validate scripts / CI). --check keeps the committed doc fresh; the selftest
# proves the freshness check catches drift. Source-only: the map reflects THIS
# repo's own scripts/guard/CI surfaces, so it is meaningful only in the source
# checkout (the generator + selftest SKIP gracefully when inputs are absent).
if [[ -x "$SCRIPT_DIR/generate-gate-coverage-map.sh" ]]; then
  run_check_self_only "Gate-coverage map drift (IMP-102 / SCOPE-9)" bash "$SCRIPT_DIR/generate-gate-coverage-map.sh" --check
fi
# IMP-027 SCOPE-2a: the coverage map is now generated from the registry's
# declared `enforcedBy` field. These verify that no gate declares an enforcer
# that does not resolve, which is what made the previous grep-derived map
# untrustworthy in both directions.
if [[ -x "$SCRIPT_DIR/gate-enforcement.sh" ]]; then
  run_check_self_only "Gate enforcement bindings resolve (IMP-027 SCOPE-2a)" bash "$SCRIPT_DIR/gate-enforcement.sh" lint --repo-root "$REPO_ROOT"
fi
if [[ -x "$SCRIPT_DIR/gate-enforcement-selftest.sh" ]]; then
  run_check "Gate enforcement selftest (IMP-027 SCOPE-2a)" bash "$SCRIPT_DIR/gate-enforcement-selftest.sh"
fi
# IMP-027 SCOPE-2c: every gate must declare whether it compensates for model
# unreliability (and can retire as models improve) or encodes a business
# invariant (and never can). 99 of 112 were unclassified.
if [[ -x "$SCRIPT_DIR/gate-classification.sh" ]]; then
  run_check_self_only "Gate classification complete (IMP-027 SCOPE-2c)" bash "$SCRIPT_DIR/gate-classification.sh" lint --repo-root "$REPO_ROOT"
fi
# IMP-027 SCOPE-2d: the documented gate bands were hand-written and wrong, and
# customGatesDiscovery advertised G100+ for project gates while the framework
# itself occupies G110-G131. Both are now derived and checked.
if [[ -x "$SCRIPT_DIR/gate-bands.sh" ]]; then
  run_check_self_only "Gate-band strings current (IMP-027 SCOPE-2d)" bash "$SCRIPT_DIR/gate-bands.sh" --check --repo-root "$REPO_ROOT"
fi
# The generated `gateEnforcement:` block shipped a --check mode that nothing
# invoked, so a stale block could only be noticed by hand. That is the same
# shape as the unrun enforcement IMP-051 SCOPE-5 closed: a check that exists but
# never executes is indistinguishable from no check at all. Registering a gate
# without regenerating leaves declaredEnforcedBy describing the previous state,
# which is precisely the "declared vs observed" drift this block exists to make
# legible.
if [[ -x "$SCRIPT_DIR/generate-gate-enforcement.sh" ]]; then
  run_check_self_only "Generated gate-enforcement block current" bash "$SCRIPT_DIR/generate-gate-enforcement.sh" --check
fi
# IMP-058 SCOPE-6 (REG-23): the derived block above can drift stale without
# failing anything if a NEW gate starts disagreeing with its own declaration
# -- this ratchet is what actually refuses that, rather than merely detecting
# that the block needs regenerating.
if [[ -x "$SCRIPT_DIR/gate-enforcement-agreement-ratchet-selftest.sh" ]]; then
  run_check "Gate enforcement agreement ratchet selftest (IMP-058 SCOPE-6)" bash "$SCRIPT_DIR/gate-enforcement-agreement-ratchet-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/gate-enforcement-agreement-ratchet.sh" ]]; then
  run_check_self_only "Gate enforcement agreement ratchet" bash "$SCRIPT_DIR/gate-enforcement-agreement-ratchet.sh"
fi
# IMP-027 SCOPE-11: a modelCompensation gate with no recorded retirement
# criterion carries unbounded cost in time — nobody can say what would have to
# be true to turn it off, so it is carried forever by default. `lint` keeps
# that backlog visible; it does not (and cannot) retire anything.
if [[ -x "$SCRIPT_DIR/gate-retirement-selftest.sh" ]]; then
  run_check "Gate retirement selftest (IMP-027 SCOPE-11)" bash "$SCRIPT_DIR/gate-retirement-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/gate-retirement.sh" ]]; then
  run_check_self_only "Gate retirement criteria recorded (IMP-027 SCOPE-11)" bash "$SCRIPT_DIR/gate-retirement.sh" lint
fi
# IMP-058 SCOPE-3 / COV-24: a preventionEvidence criterion is only honest if
# the gate carrying it can actually accumulate the telemetry it depends on.
# Refuses the exact defect class this scope found and fixed (a gate's own id
# missing from its pass/fail message text), without requiring every
# modelCompensation gate to be telemetry-capable today.
if [[ -x "$SCRIPT_DIR/gate-telemetry-capability-lint-selftest.sh" ]]; then
  run_check "Gate telemetry capability lint selftest (IMP-058 SCOPE-3)" bash "$SCRIPT_DIR/gate-telemetry-capability-lint-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/gate-telemetry-capability-lint.sh" ]]; then
  run_check_self_only "Gate telemetry capability (IMP-058 SCOPE-3)" bash "$SCRIPT_DIR/gate-telemetry-capability-lint.sh"
fi
# IMP-027 SCOPE-4 / SEC-3: G034 was a businessInvariant gate with no enforcer
# and no agent reference — its entire enforcement was "appears in a mode's
# requiredGates list". These give it a mechanical surface.
if [[ -x "$SCRIPT_DIR/security-gate.sh" ]]; then
  run_check_self_only "Security gate (G034, IMP-027 SCOPE-4)" bash "$SCRIPT_DIR/security-gate.sh" --repo-root "$REPO_ROOT"
fi
if [[ -x "$SCRIPT_DIR/security-gate-selftest.sh" ]]; then
  # self-only, like its sibling above: a selftest validates framework SOURCE and
  # has no meaning in a downstream/fixture tree. Registering it as a plain
  # run_check made it execute inside minimal fixture repos and changed their
  # aggregate failure count, which broke the BUG-021 deadline regression's
  # "exactly 1 failing check" contract.
  run_check_self_only "Security gate selftest (G034, IMP-027 SCOPE-4)" bash "$SCRIPT_DIR/security-gate-selftest.sh"
fi
# IMP-027 SCOPE-6 / COST-1: distance-to-target and the dispatch-weighted cost
# proxy. The report itself is advisory (a ratchet stops growth but never states
# a destination); only its selftest is blocking, so a broken closure walk cannot
# silently report a healthy repo.
if [[ -x "$SCRIPT_DIR/bundle-cost-report-selftest.sh" ]]; then
  run_check "Bundle cost report selftest (COST-1, IMP-027 SCOPE-6)" bash "$SCRIPT_DIR/bundle-cost-report-selftest.sh"
fi
# IMP-027 SCOPE-5: the golden-task corpus. The live run proves the reference
# output still satisfies every task (the regression baseline); the selftest
# proves the corpus can FAIL, which is what stops it becoming a rubber stamp.
if [[ -x "$SCRIPT_DIR/eval-harness.sh" ]] && [[ -d "$REPO_ROOT/bubbles/eval/tasks" ]]; then
  run_check_self_only "Golden-task corpus baseline (IMP-027 SCOPE-5)" bash "$SCRIPT_DIR/eval-harness.sh" run \
    --suite "$REPO_ROOT/bubbles/eval/tasks" \
    --output "$REPO_ROOT/bubbles/eval/fixtures/positive/corpus-output"
fi
if [[ -x "$SCRIPT_DIR/eval-corpus-selftest.sh" ]]; then
  run_check_self_only "Golden-task corpus discriminates (IMP-027 SCOPE-5)" bash "$SCRIPT_DIR/eval-corpus-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/generate-gate-coverage-map-selftest.sh" ]]; then
  run_check_self_only "Gate-coverage map generator selftest (IMP-102 / SCOPE-9)" bash "$SCRIPT_DIR/generate-gate-coverage-map-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mode-family-inventory-selftest.sh" ]]; then
  run_check "Mode-family inventory selftest (v6.1 / R5)" bash "$SCRIPT_DIR/mode-family-inventory-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/model-tier-advisory-selftest.sh" ]]; then
  run_check "Model-tier floor selftest (v6.1 / S9 / G126)" bash "$SCRIPT_DIR/model-tier-advisory-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/parallel-fanout-determinism-selftest.sh" ]]; then
  run_check "Parallel fan-out determinism selftest (v6.1 / B10 / R8)" bash "$SCRIPT_DIR/parallel-fanout-determinism-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/pre-tool-risk-gate-selftest.sh" ]]; then
  run_check "Pre-tool risk gate selftest (v6.1 / R10)" bash "$SCRIPT_DIR/pre-tool-risk-gate-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/adversarial-resolve-selftest.sh" ]]; then
  run_check "Adversarial-resolve control plane selftest (IMP-002 / S0)" bash "$SCRIPT_DIR/adversarial-resolve-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/autonomy-resolve-selftest.sh" ]]; then
  run_check "Autonomy-resolve posture selftest (IMP-039 / SCOPE-1)" bash "$SCRIPT_DIR/autonomy-resolve-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/autonomy-posture-guard-selftest.sh" ]]; then
  run_check "Autonomy posture consistency selftest (G135 / IMP-039 SCOPE-7)" bash "$SCRIPT_DIR/autonomy-posture-guard-selftest.sh"
fi
if [[ -f "$SCRIPT_DIR/adversarial-aggregate-selftest.sh" ]]; then
  # The selftest validates the source-only eval schema and canonical source surfaces.
  run_check_self_only "Adversarial aggregate selftest (IMP-020 / S2)" bash "$SCRIPT_DIR/adversarial-aggregate-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/control-plane-policy-activation-selftest.sh" ]]; then
  run_check "Control-plane policy-activation selftest (G055-G060 SST precedence + G060 red->green ordering)" bash "$SCRIPT_DIR/control-plane-policy-activation-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/control-plane-rce-selftest.sh" ]]; then
  run_check "Control-plane RCE selftest (IMP-102 / SCOPE-4 — no shell interpolation into python3 -c)" bash "$SCRIPT_DIR/control-plane-rce-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/evidence-admission-hardening-selftest.sh" ]]; then
  run_check "Evidence-admission hardening selftest (IMP-102 / SCOPE-1)" bash "$SCRIPT_DIR/evidence-admission-hardening-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/tool-capture-shim-selftest.sh" ]]; then
  run_check "Tool-capture shim selftest (v6.1 / R2)" bash "$SCRIPT_DIR/tool-capture-shim-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/eval-harness-selftest.sh" ]]; then
  run_check_self_only "Golden-task eval harness selftest (v6.1 / R11)" bash "$SCRIPT_DIR/eval-harness-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/eval-heldout-guard-selftest.sh" ]]; then
  run_check "Held-out eval isolation guard selftest (IMP-100 Phase 6 / IMP-020 S4)" bash "$SCRIPT_DIR/eval-heldout-guard-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/effective-bundle-measure-selftest.sh" ]]; then
  run_check "Effective prompt-bundle measurement selftest (IMP-100 Phase 6 / IMP-020 S5)" bash "$SCRIPT_DIR/effective-bundle-measure-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/forecast-eval-check-selftest.sh" ]]; then
  run_check "Forecast-eval check selftest (IMP-100 Phase 6 / IMP-020 S6)" bash "$SCRIPT_DIR/forecast-eval-check-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/state-transition-guard-perf-selftest.sh" ]]; then
  run_check "Guard reliability perf selftest (v6.1 / R1 / BUG-001)" bash "$SCRIPT_DIR/state-transition-guard-perf-selftest.sh"
fi
run_check "Result-envelope validate (v6.0 / B3, malformed blocks)" bash "$SCRIPT_DIR/result-envelope-validate.sh"
run_check "v5.2 aggregate selftest (F1, F3, F6, F7)" bash "$SCRIPT_DIR/v5.2-selftest.sh"
if [[ -x "$SCRIPT_DIR/v5.3-selftest.sh" ]]; then
  run_check_self_only "v5.3 downstream-install selftest (G1)" bash "$SCRIPT_DIR/v5.3-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mcp-server-selftest.sh" ]]; then
  run_check "v6 MCP server selftest (A5)" bash "$SCRIPT_DIR/mcp-server-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mcp-http-transport-selftest.sh" ]]; then
  run_check "MCP HTTP transport selftest (v6.1 / R9)" bash "$SCRIPT_DIR/mcp-http-transport-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mcp-trust-boundary-selftest.sh" ]]; then
  run_check "MCP trust-boundary selftest (IMP-102 / SCOPE-7)" bash "$SCRIPT_DIR/mcp-trust-boundary-selftest.sh"
fi
run_check "Workflow registry consistency" bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet
run_check "Mode resolver validate" bash "$SCRIPT_DIR/mode-resolver.sh" --validate
run_check "Mode resolver selftest" bash "$SCRIPT_DIR/mode-resolver-selftest.sh"
run_check "Mode-resolver phase-multiplicity selftest (IMP-102 / SCOPE-2)" bash "$SCRIPT_DIR/mode-resolver-phase-multiplicity-selftest.sh"
run_check "Risk-tier resolver selftest (BFW-01 / IMP-021)" bash "$SCRIPT_DIR/risk-tier-resolve-selftest.sh"
run_check "Rapid-tool-delivery mode selftest (IMP-100 Phase 1)" bash "$SCRIPT_DIR/rapid-tool-delivery-mode-selftest.sh"
run_check "Work-boundary resolver selftest (IMP-100 Phase 4 R6)" bash "$SCRIPT_DIR/work-boundary-resolve-selftest.sh"
run_check "Goal-contract selftest (IMP-038 SCOPE-1 / GF-1)" bash "$SCRIPT_DIR/goal-contract-selftest.sh"
run_check "Goal-fidelity guard selftest (IMP-038 SCOPE-6 / G134)" bash "$SCRIPT_DIR/goal-fidelity-guard-selftest.sh"
run_check "Goal-boundary receipt selftest (IMP-041 SCOPE-3 / GF-7)" bash "$SCRIPT_DIR/goal-boundary-receipt-selftest.sh"
run_check "Mutable-dispatch authorization selftest (IMP-056 SCOPE-3)" bash "$SCRIPT_DIR/mutable-dispatch-authorization-selftest.sh"
run_check "Mutable-dispatch gateway selftest (IMP-056 SCOPE-4)" bash "$SCRIPT_DIR/mutable-dispatch-gateway-selftest.sh"
run_check_self_only "Mutable-dispatch caller-coverage lint (live, IMP-056 SCOPE-6)" bash "$SCRIPT_DIR/mutable-dispatch-caller-coverage-lint.sh" --repo-root "$REPO_ROOT"
run_check "Mutable-dispatch caller-coverage lint selftest (IMP-056 SCOPE-6)" bash "$SCRIPT_DIR/mutable-dispatch-caller-coverage-lint-selftest.sh"
run_check "Expansion-approval selftest (IMP-041 SCOPE-4 / GF-10)" bash "$SCRIPT_DIR/expansion-approval-selftest.sh"
run_check "Convergence-materiality selftest (IMP-041 SCOPE-7 / GF-13)" bash "$SCRIPT_DIR/convergence-materiality-selftest.sh"
run_check "IMP-041 evaluation corpus (SCOPE-8 / COV-13)" bash "$SCRIPT_DIR/imp041-evaluation-corpus.sh"
run_check "Run-state abandoned-run reaper selftest" bash "$SCRIPT_DIR/run-state-reaper-selftest.sh"
run_check "Run-state registry read-modify-write selftest" bash "$SCRIPT_DIR/run-state-registry-selftest.sh"
run_check "Goal-fidelity telemetry selftest (IMP-038 SCOPE-7 / GF-5)" bash "$SCRIPT_DIR/goal-fidelity-telemetry-selftest.sh"
run_check "Phase-relevance resolver selftest (IMP-038 SCOPE-5 / GF-4)" bash "$SCRIPT_DIR/phase-relevance-resolve-selftest.sh"
run_check "Assurance deploy-eligibility resolver selftest (IMP-100 Phase 2 R4d / Phase 3)" bash "$SCRIPT_DIR/assurance-resolve-selftest.sh"
run_check "Assurance level derivation resolver selftest (IMP-100 Phase 3 choke #1)" bash "$SCRIPT_DIR/assurance-derive-selftest.sh"
run_check "Assurance certification consistency guard selftest (IMP-100 Phase 3 choke #1)" bash "$SCRIPT_DIR/assurance-certification-check-selftest.sh"
run_check "Deploy-manifest assurance lint selftest (IMP-100 Phase 3 chokes #4/#5)" bash "$SCRIPT_DIR/deploy-manifest-assurance-lint-selftest.sh"
run_check "Transition contract resolver selftest (BUG-009 S02)" bash "$SCRIPT_DIR/transition-contract-resolver-selftest.sh"
run_check "Audit result contract lint selftest (BUG-009 S04)" bash "$SCRIPT_DIR/audit-result-contract-lint-selftest.sh"
run_check "Mode alias selftest (v6.0 / B4)" bash "$SCRIPT_DIR/mode-alias-selftest.sh"
if [[ -x "$SCRIPT_DIR/v7-selftest.sh" ]]; then
  run_check "v7 mode-name removal + grandfather selftest (v7.0)" bash "$SCRIPT_DIR/v7-selftest.sh"
fi
run_check "Spec-review handoff selftest" bash "$SCRIPT_DIR/spec-review-handoff-selftest.sh"
if [[ -d "$REPO_ROOT/agents" ]]; then
  agents_dir="$REPO_ROOT/agents"
else
  agents_dir="$REPO_ROOT/.github/agents"
fi
run_check "Instruction budget lint" bash "$SCRIPT_DIR/instruction-budget-lint.sh" "$agents_dir"
run_check "Agent ownership lint" bash "$SCRIPT_DIR/agent-ownership-lint.sh"
run_check "Orchestrator tool frontmatter lint (v7.0.3)" bash "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint.sh"
run_check "Workflow runner grants lint (G064)" bash "$SCRIPT_DIR/workflow-runner-grants-lint.sh"
run_check "Workflow runner grants lint selftest (G064)" bash "$SCRIPT_DIR/workflow-runner-grants-lint-selftest.sh"
if [[ -x "$SCRIPT_DIR/mcp-grant-selftest.sh" ]]; then
  # Source-only: asserts the canonical 'bubbles' MCP token; downstream installs
  # carry a per-repo 'bubbles-<slug>' token, so this can only hold in source.
  run_check_self_only "MCP tool grant selftest (v7.1)" bash "$SCRIPT_DIR/mcp-grant-selftest.sh"
fi
run_check "Action risk registry lint" bash "$SCRIPT_DIR/action-risk-registry-lint.sh"
run_check_self_only "Capability ledger selftest" bash "$SCRIPT_DIR/capability-ledger-selftest.sh"
run_check "Capability consumer freshness selftest (G127)" bash "$SCRIPT_DIR/capability-consumer-freshness-selftest.sh"
run_check_self_only "Capability consumer freshness (live, G127)" bash "$SCRIPT_DIR/capability-consumer-freshness.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Capability freshness selftest" bash "$SCRIPT_DIR/capability-freshness-selftest.sh"
run_check_self_only "Competitive docs selftest" bash "$SCRIPT_DIR/competitive-docs-selftest.sh"
run_check_self_only "Interop apply selftest" bash "$SCRIPT_DIR/interop-apply-selftest.sh"
run_check_self_only "Interop import selftest" bash "$SCRIPT_DIR/interop-import-selftest.sh"
run_check_self_only "Release manifest selftest" bash "$SCRIPT_DIR/release-manifest-selftest.sh"
# Source-only: it drives the release-check receipt-consumption branch, and
# release-check is itself source-only, so downstream there is nothing to test.
run_check_self_only "Validation run receipt selftest (IMP-049 SCOPE-2)" bash "$SCRIPT_DIR/validation-receipt-selftest.sh"
run_check_self_only "Release manifest purity selftest" bash "$SCRIPT_DIR/release-manifest-purity-selftest.sh"
run_check_self_only "Payload closure guard (IMP-042 / REG-11)" bash "$SCRIPT_DIR/payload-closure-guard.sh"
run_check_self_only "Payload closure guard selftest (IMP-042 / REG-11)" bash "$SCRIPT_DIR/payload-closure-guard-selftest.sh"
run_check_self_only "Derived-artifact regen wrapper selftest (IMP-007)" bash "$SCRIPT_DIR/regen-derived-selftest.sh"
run_check "Gate-hit telemetry selftest (IMP-036)" bash "$SCRIPT_DIR/gate-hit-log-selftest.sh"
run_check "Micro-fix admission selftest (IMP-042 SCOPE-9)" bash "$SCRIPT_DIR/micro-fix-admission-selftest.sh"
# IMP-043 SCOPE-6: the loop as a loop. Every component of the learning loop
# already passes its own selftest while lessons.md stays empty everywhere, so the
# defect lives in the seams the component tests never cross.
if [[ -x "$SCRIPT_DIR/learning-loop-selftest.sh" ]]; then
  run_check "Learning-loop selftest (IMP-043 SCOPE-6 / COV-18)" bash "$SCRIPT_DIR/learning-loop-selftest.sh"
fi
# IMP-044 SCOPE-3: the config writer rendered a fixed template, so every key it
# did not name was destroyed on the next mutation, silently and with a success
# message.
if [[ -x "$SCRIPT_DIR/control-plane-config-merge-selftest.sh" ]]; then
  run_check "Control-plane config merge selftest (IMP-044 SCOPE-3 / REG-15)" bash "$SCRIPT_DIR/control-plane-config-merge-selftest.sh"
fi
run_check "Agent-id enum lint selftest (IMP-036)" bash "$SCRIPT_DIR/agent-id-enum-lint-selftest.sh"
run_check "Phase-name enum lint selftest (IMP-052)" bash "$SCRIPT_DIR/phase-name-enum-lint-selftest.sh"
run_check "Phase-name enum lint (live, IMP-052)" bash "$SCRIPT_DIR/phase-name-enum-lint.sh" "$REPO_ROOT"
run_check "Collected-test-count guard selftest (IMP-036)" bash "$SCRIPT_DIR/collected-test-count-guard-selftest.sh"
run_check "Gate-vintage selftest (IMP-036)" bash "$SCRIPT_DIR/gate-vintage-selftest.sh"
run_check "Evidence-capture selftest (IMP-036)" bash "$SCRIPT_DIR/evidence-capture-selftest.sh"
# The gap ID is carried alongside because the identifier IMP-039 is ALSO held by
# the delivered autonomy-posture work (gate G135), which has its own SCOPE-1 and
# SCOPE-7. COST-*/EV-* disambiguate; the bare scope number does not.
run_check "Output-policy coherence selftest (IMP-039 / EV-7)" bash "$SCRIPT_DIR/output-policy-coherence-guard-selftest.sh"
run_check "Usage-adapter contract selftest (IMP-039 / COST-4)" bash "$SCRIPT_DIR/usage-adapter-contract-selftest.sh"
run_check "Test-inventory adapter contract selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/test-inventory-adapter-contract-selftest.sh"
run_check "Scenario linked-test resolution selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-test-resolve-selftest.sh"
run_check "Scenario reference reader selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-reference-reader-selftest.sh"
run_check "Scenario manifest v2 schema selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-manifest-v2-schema-selftest.sh"
run_check "Scenario manifest migration selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-manifest-migrate-selftest.sh"
run_check "YAML schema dispatch selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/yaml-schema-validate-selftest.sh"
run_check "Execution-control store selftest (IMP-054/055 / ECF-01)" bash "$SCRIPT_DIR/execution-control-selftest.sh"
run_check "Measured-budget runtime selftest (IMP-055 / MBE-01)" python3 "$SCRIPT_DIR/measured-budget-runtime-selftest.py"
run_check "Research runtime selftest (IMP-054 / RESEARCH-01)" python3 "$SCRIPT_DIR/research-runtime-selftest.py"
run_check "Research adapter contract selftest (IMP-054 / RESEARCH-02)" python3 "$SCRIPT_DIR/research-adapter-contract-selftest.py"
run_check "Usage adapter v2 selftest (IMP-055 / USAGE-02)" bash "$SCRIPT_DIR/usage-adapter-v2-selftest.sh"
run_check "Admission contract selftest (IMP-055 / ADMISSION-01)" bash "$SCRIPT_DIR/admission-contract-selftest.sh"
run_check "Research and admission CLI integration selftest (IMP-054/055)" bash "$SCRIPT_DIR/research-admission-cli-selftest.sh"
run_check "Framework validation wiring selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/framework-validation-wiring-selftest.sh"
run_check "Report-section contract selftest (IMP-047 / S-B)" bash "$SCRIPT_DIR/report-sections-selftest.sh"
# Framework-source-only: check P3 requires the repo-root BUGS.md, which the
# release manifest classifies as neither managed nor source-only-shipped, so it
# does not exist in an installed tree. Scheduling it as portable made every
# downstream install fail a check about a file it can never have.
run_check_self_only "Bug-packet contract selftest (IMP-047 / S-B)" bash "$SCRIPT_DIR/bug-packet-selftest.sh"
# Framework-source-only for the same reason, one directory over: check P1 builds
# every behavioural fixture from a SHIPPED bugs/BUG-*/state.json that declares
# the compact form. The framework's own bugs/ tree is a source-repo artifact and
# is never installed, so downstream the selftest refuses at P1 with "no
# bugs/BUG-*/state.json declares the compact form" -- a verdict about a fixture
# the installed tree cannot have, not about the guard behaviour under test.
# Wiring it here also removes it from the discovered sweep, which has only
# run_check and therefore no way to express this.
run_check_self_only "Compact-packet obligation basis selftest (BUG-042)" bash "$SCRIPT_DIR/compact-obligation-basis-selftest.sh"
# IMP-047 S-E. These four cover the apparatus that decides what a run may SKIP:
# the derived closure map, its consumer, the debt ledger that records every
# deferral, and the batch executor that settles them. Each one removes work from
# a run, so each one needs a guard that fails when its honesty rule is dropped.
run_check "Closure-map generator selftest (IMP-047 S-E)" bash "$SCRIPT_DIR/generate-validation-checks-selftest.sh"
run_check "Declared-input closure selftest (IMP-047 S-E)" bash "$SCRIPT_DIR/validation-closure-selftest.sh"
run_check "Validation debt ledger selftest (IMP-047 S-E)" bash "$SCRIPT_DIR/validation-debt-selftest.sh"
run_check "Batched obligation settlement selftest (IMP-047 S-E)" bash "$SCRIPT_DIR/validation-batch-selftest.sh"
run_check "Scenario obligation matrix selftest (IMP-040 / COV-9)" bash "$SCRIPT_DIR/scenario-obligation-lint-selftest.sh"
run_check "Test-mechanism declaration selftest (IMP-040 / COV-10)" bash "$SCRIPT_DIR/test-mechanism-lint-selftest.sh"
run_check "Mutation adapter contract selftest (IMP-040 / COV-11)" bash "$SCRIPT_DIR/mutation-adapter-contract-selftest.sh"
run_check "Scenario impact resolution selftest (IMP-040 / REG-8)" bash "$SCRIPT_DIR/scenario-impact-resolve-selftest.sh"
run_check "Changed-spec verification selftest (IMP-040 / COV-12)" bash "$SCRIPT_DIR/verify-changed-specs-selftest.sh"
run_check "IMP-040 evaluation corpus (8 repository shapes)" bash "$SCRIPT_DIR/imp040-evaluation-corpus.sh"
run_check "Tool-grant lint selftest (IMP-039 / COST-6)" bash "$SCRIPT_DIR/tool-grant-lint-selftest.sh"
run_check "Always-on instruction budget selftest (IMP-039 / COST-6)" bash "$SCRIPT_DIR/always-on-instruction-budget-selftest.sh"
# Advisory by design: the grant frontmatter is runtime-enforced, so an
# over-narrow grant breaks dispatch silently. Report the delta, narrow one agent
# at a time, and only then flip a repo to --strict.
run_check_self_only "Tool-grant lint (IMP-039 / COST-6, advisory)" bash "$SCRIPT_DIR/tool-grant-lint.sh" --quiet
# IMP-049 SCOPE-6 / COST-1. Exact documentation wording that used to gate a
# release: generated cheatsheet markup, rendered table rows, and literal English
# sentences. The owning selftests now assert the structure underneath each one,
# so rewording a doc no longer breaks a build. This reports drift and always
# exits 0. Self-only: it reads this repo's own docs, prompts, and README.
run_check_self_only "Documentation wording (IMP-049 / COST-1, advisory)" bash "$SCRIPT_DIR/docs-wording-advisory.sh" --quiet
# Self-only: it reads this repo's own instruction surfaces. A downstream repo
# gets the coherent text from the template on upgrade, so running it there would
# report the framework's own upgrade lag as a consumer defect.
run_check_self_only "Output-policy coherence (IMP-039 / EV-7)" bash "$SCRIPT_DIR/output-policy-coherence-guard.sh" --quiet
# Self-only for the same reason: a downstream repo's own always-on instructions
# are its governance call, not the framework's.
run_check_self_only "Always-on instruction budget (IMP-039 / COST-6)" bash "$SCRIPT_DIR/always-on-instruction-budget.sh" --quiet
run_check_self_only "Gate-vintage annotation freshness (IMP-036)" bash "$SCRIPT_DIR/gate-vintage-annotate.sh" --check
run_check_self_only "Gate scaffolder selftest (IMP-011)" bash "$SCRIPT_DIR/scaffold-gate-selftest.sh"
run_check_self_only "Framework drift-check selftest (IMP-013)" bash "$SCRIPT_DIR/bubbles-drift-check-selftest.sh"
run_check_self_only "Spec dashboard selftest (portfolio-count correctness)" bash "$SCRIPT_DIR/spec-dashboard-selftest.sh"
run_check_self_only "Governance hub-report selftest (IMP-014)" bash "$SCRIPT_DIR/bubbles-hub-report-selftest.sh"
run_check_self_only "Scan-lib helpers selftest (IMP-009)" bash "$SCRIPT_DIR/scan-lib-selftest.sh"
run_check_self_only "DoD section lib selftest (BUG-026)" bash "$SCRIPT_DIR/dod-section-lib-selftest.sh"
run_check_self_only "Scenario-match lib selftest (BUG-004)" bash "$SCRIPT_DIR/scenario-match-lib-selftest.sh"
run_check_self_only "Scope universe resolver selftest (BUG-026)" bash "$SCRIPT_DIR/scope-universe-resolver-selftest.sh"
run_check_self_only "Framework-validate tiering selftest (IMP-012)" bash "$SCRIPT_DIR/framework-validate-tier-selftest.sh"
# IMP-042 SCOPE-2: core_check_label() selects the push-blocking tier by substring
# match on check LABELS, so renaming a check silently drops it from that tier with
# nothing reporting the loss. Source-only because it reads this validator's own
# text, which downstream carries under a different path prefix.
if [[ -x "$SCRIPT_DIR/core-tier-pattern-lint.sh" ]]; then
  run_check_self_only "Core-tier pattern lint (IMP-042 SCOPE-2)" bash "$SCRIPT_DIR/core-tier-pattern-lint.sh"
fi
if [[ -x "$SCRIPT_DIR/core-tier-pattern-lint-selftest.sh" ]]; then
  run_check_self_only "Core-tier pattern lint selftest (IMP-042 SCOPE-2)" bash "$SCRIPT_DIR/core-tier-pattern-lint-selftest.sh"
fi
run_check_self_only "Framework-validate changed-only selftest (IMP-027 SCOPE-7)" bash "$SCRIPT_DIR/framework-validate-changed-only-selftest.sh"
run_check_self_only "Install provenance selftest" bash "$SCRIPT_DIR/install-provenance-selftest.sh"
run_check_self_only "Trust doctor selftest" bash "$SCRIPT_DIR/trust-doctor-selftest.sh"
run_check "Repo-binding preflight selftest (BFW-05 / IMP-025)" bash "$SCRIPT_DIR/repo-binding-preflight-selftest.sh"
# The aggregate reports each focused IMP-103 suite, including the conformance
# guard selftest. Do not add a second standalone conformance-selftest run here.
run_check "Repository work-boundary aggregate selftest (IMP-103 / G129)" \
  bash "$SCRIPT_DIR/cli.sh" repository-binding-selftest --suite=all
run_check "Repository host-context bridge selftest (IMP-103 / G129)" \
  bash "$SCRIPT_DIR/repository-binding-host-context-selftest.sh"
# The live guard checks canonical source consumers and therefore has no valid
# downstream equivalent; hermetic aggregate coverage still runs downstream.
run_check_self_only "Repository work-boundary conformance guard (live, G129)" \
  bash "$SCRIPT_DIR/repository-binding-conformance-guard.sh" --root "$REPO_ROOT"
run_check "Finding closure selftest" bash "$SCRIPT_DIR/finding-closure-selftest.sh"
run_check "Super surface selftest" bash "$SCRIPT_DIR/super-surface-selftest.sh"
run_check "Workflow delegation selftest" bash "$SCRIPT_DIR/workflow-delegation-selftest.sh"
run_check "Top-level-runtime routing selftest" bash "$SCRIPT_DIR/top-level-runtime-routing-selftest.sh"
run_check "Continuation intent resolver selftest" bash "$SCRIPT_DIR/continuation-intent-resolve-selftest.sh"
run_check "Continuation routing selftest" bash "$SCRIPT_DIR/continuation-routing-selftest.sh"
planning_provenance_timeout_seconds="${BUBBLES_WORKFLOW_PLANNING_PROVENANCE_SELFTEST_TIMEOUT_SECONDS:-120}"
run_check "Workflow planning provenance selftest" bubbles_run_with_timeout "$planning_provenance_timeout_seconds" bash "$SCRIPT_DIR/workflow-planning-provenance-selftest.sh"
run_check "Transition guard selftest" bash "$SCRIPT_DIR/state-transition-guard-selftest.sh"
run_check "Required-specialists fallback selftest (IMP-105 / SCOPE-3 — Check 6 fail-open closure)" bash "$SCRIPT_DIR/state-transition-required-specialists-selftest.sh"
run_check "Required-specialists registry selftest (IMP-042 SCOPE-13)" bash "$SCRIPT_DIR/required-specialists-registry-selftest.sh"
run_check_self_only "BUG-009 planning audit contract regression" bash "$REPO_ROOT/tests/regression/test_23_planning_audit_contract.sh"
run_check_self_only "BUG-013 sensitive client storage regression" bash "$REPO_ROOT/tests/regression/test_24_g028_sensitive_client_storage.sh"
run_check_self_only "BUG-018 traceability Test Plan heading-depth regression" bash "$REPO_ROOT/tests/regression/test_25_traceability_test_plan_heading_depth.sh"
run_check_self_only "CR-02 traceability current-scope universe regression" bash "$REPO_ROOT/tests/regression/test_33_traceability_current_scope_universe.sh"
run_check_self_only "BUG-019 state-transition compound MJS test-path regression" bash "$REPO_ROOT/tests/regression/test_26_state_transition_spec_mjs_path.sh"
run_check_self_only "BUG-021 portable framework deadline regression" bash "$REPO_ROOT/tests/regression/test_28_framework_validate_portable_timeout.sh"
run_check_self_only "BUG-029 human acceptance terminal regression (G136)" bash "$REPO_ROOT/tests/regression/test_35_human_acceptance_terminal.sh"
run_check_self_only "BUG-045 framework-validate tier-lock lifecycle regression" bash "$REPO_ROOT/tests/regression/test_36_framework_validate_tier_lock_lifecycle.sh"
run_check "Convergence cap guard selftest" bash "$SCRIPT_DIR/convergence-cap-guard-selftest.sh"
run_check "Session cap guard selftest (G128)" bash "$SCRIPT_DIR/session-cap-guard-selftest.sh"
fv_run_stable_session_guard() {
  local output_file="$1"
  shift
  local state_helper="$SCRIPT_DIR/session-state-io.py"
  local python_bin=""
  local source_path="$SCRIPT_DIR/session-cap-guard.sh"
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
      --root "$SCRIPT_DIR" --relative-path 'session-cap-guard.sh' \
      --destination "$before_path" 2>/dev/null)" ||
    [[ ! -x "$source_path" || -L "$source_path" ]] ||
    ! preflight_meta="$("$python_bin" "$state_helper" capture \
      --root "$SCRIPT_DIR" --relative-path 'session-cap-guard.sh' \
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
      BUBBLES_REPO_ROOT="$REPO_ROOT" \
      "$BASH" "$before_path" "$@"
  ) >"$output_file" 2>&1
  child_rc=$?

  if ! after_meta="$("$python_bin" "$state_helper" capture \
      --root "$SCRIPT_DIR" --relative-path 'session-cap-guard.sh' \
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

fv_parse_session_guard_result() {
  local process_exit="$1"
  local session_id="$2"
  local output_file="$3"
  local python_bin=""

  python_bin="$(command -v python3 2>/dev/null || true)"
  [[ -n "$python_bin" && -f "$output_file" && ! -L "$output_file" ]] || return 9
  "$python_bin" -c '
import json
import re
import sys
from pathlib import Path

process_exit_raw, session_id, output_path = sys.argv[1:]
try:
    process_exit = int(process_exit_raw)
    raw = Path(output_path).read_bytes()
    if b"\x00" in raw or b"\r" in raw:
        raise ValueError
    text = raw.decode("utf-8")
    candidates = [line for line in text.split("\n") if line.startswith("G128 status")]
    if len(candidates) != 1:
        raise ValueError
    json_string = "\"(?:[^\"\\\\]|\\\\.)*\""
    pattern = re.compile(
        rf"^G128 status=(?P<status>[A-Z][A-Z-]*) exit=(?P<exit>[0-9]+)"
        rf"(?: session=(?P<session>{json_string}))?"
        r"(?: reason=(?P<reason>[a-z0-9][a-z0-9-]*))?$"
    )
    matrix = {0: {"NO-ACTIVE-BUDGET", "PASS", "SOFT-BOUNDARY"}, 1: {"BREACH"}, 2: {"INPUT-ERROR"}}
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
    if (status in {"NO-ACTIVE-BUDGET", "INPUT-ERROR"}) != (reason is not None):
        raise ValueError
except (OSError, UnicodeDecodeError, ValueError, json.JSONDecodeError):
    sys.exit(2)
print(json.dumps({"exit": declared_exit, "status": status}, separators=(",", ":"), sort_keys=True))
' "$process_exit" "$session_id" "$output_file"
}

run_live_session_cap_guard() {
  local session_id="${BUBBLES_SESSION_ID:-}"
  local control_file="${BUBBLES_SESSION_CONTROL_FILE:-}"
  local packet_file="${BUBBLES_BINDING_PACKET_FILE:-}"
  local scenario_file="${BUBBLES_BINDING_SCENARIO_FILE:-}"
  local node_id="${BUBBLES_BINDING_NODE_ID:-}"
  local validator="$SCRIPT_DIR/repository-binding.sh"
  local state_helper="$SCRIPT_DIR/session-state-io.py"
  local python_bin=""
  local validator_output=""
  local output_file=""
  local parsed=""
  local status=""
  local reason=""
  local rc=2
  local validator_args=()

  if [[ -z "$session_id" || -z "$control_file" || -z "$packet_file" ]]; then
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
    return 2
  fi
  if [[ -n "$scenario_file" || -n "$node_id" ]]; then
    if [[ -z "$scenario_file" || -z "$node_id" ]]; then
      printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
      return 2
    fi
  fi
  if [[ ! -f "$validator" || -L "$validator" || ! -x "$validator" ]]; then
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
    return 2
  fi

  validator_args=(validate-packet
    --session-id "$session_id"
    --session-control-file "$control_file"
    --packet-file "$packet_file")
  if [[ -n "$scenario_file" ]]; then
    validator_args+=(--scenario-file "$scenario_file" --node-id "$node_id")
  fi
  validator_args+=(--emit-redacted-projection)

  if ! validator_output="$("$BASH" "$validator" "${validator_args[@]}" 2>&1)"; then
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
    return 2
  fi
  case "$validator_output" in
    *$'\n'*)
      printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
      return 2
      ;;
  esac
  if ! jq -e --arg sessionId "$session_id" '
      type == "object"
      and .repositoryRoot == "<redacted-local-root>"
      and .repositoryResolution.sessionId == $sessionId
      and .repositoryResolution.pathVisibility == "redacted"
      and .repositoryResolution.actionable == false
    ' <<<"$validator_output" >/dev/null 2>&1; then
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority\n'
    return 2
  fi

  python_bin="$(command -v python3 2>/dev/null || true)"
  if [[ -z "$python_bin" || ! -f "$state_helper" || -L "$state_helper" ]]; then
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=guard-unavailable\n'
    return 2
  fi

  output_file="$(mktemp "${TMPDIR:-/tmp}/bubbles-framework-g128-output.XXXXXX")"
  chmod 600 "$output_file"
  if fv_run_stable_session_guard "$output_file" \
      --session-id "$session_id" --quiet; then
    rc=0
  else
    rc=$?
  fi

  if ! parsed="$(fv_parse_session_guard_result "$rc" \
      "$session_id" "$output_file" 2>/dev/null)"; then
    reason="invalid-child-result"
    if [[ "$rc" -gt 2 ]]; then
      reason="guard-unavailable"
    fi
    printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=%s\n' "$reason"
    rm -f "$output_file"
    return 2
  fi

  cat "$output_file"
  rm -f "$output_file"
  status="$(jq -r '.status' <<<"$parsed")"
  case "$status" in
    NO-ACTIVE-BUDGET | PASS | SOFT-BOUNDARY)
      printf 'Framework check PASS G128 status=%s exit=0 source=guard\n' "$status"
      return 0
      ;;
    BREACH)
      printf 'Framework check FAIL G128 status=BREACH exit=1 source=guard\n'
      return 1
      ;;
    INPUT-ERROR)
      printf 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=guard\n'
      return 2
      ;;
  esac
}
run_check "Session cap guard (live, G128)" run_live_session_cap_guard
run_check "Compaction discipline guard selftest" bash "$SCRIPT_DIR/compaction-discipline-guard-selftest.sh"
run_check "Pre-existing deferral guard selftest" bash "$SCRIPT_DIR/pre-existing-deferral-guard-selftest.sh"
run_check "Discovered-issue disposition guard selftest (G095)" bash "$SCRIPT_DIR/discovered-issue-disposition-guard-selftest.sh"
run_check "Requirement-mechanism guard selftest (G097)" bash "$SCRIPT_DIR/requirement-mechanism-guard-selftest.sh"
run_check "Domain-invariant guard selftest (G130)" bash "$SCRIPT_DIR/domain-invariant-guard-selftest.sh"
run_check "Domain-model consistency guard selftest (G131)" bash "$SCRIPT_DIR/domain-model-consistency-selftest.sh"
run_check "Framework dogfood guard selftest" bash "$SCRIPT_DIR/framework-dogfood-guard-selftest.sh"
run_check "Orchestrator persistence lint selftest" bash "$SCRIPT_DIR/orchestrator-persistence-lint-selftest.sh"
run_check_self_only "Orchestrator persistence persistent regression" bash "$REPO_ROOT/tests/regression/test_05_orchestrator_persistence.sh"
run_check "Validation latency report selftest" bash "$SCRIPT_DIR/validation-latency-report-selftest.sh"
run_check "Retro convergence health selftest" bash "$SCRIPT_DIR/retro-convergence-health-selftest.sh"
run_check "Planning workflow chain guard selftest" bash "$SCRIPT_DIR/planning-workflow-chain-guard-selftest.sh"
run_check "Capability foundation guard selftest" bash "$SCRIPT_DIR/capability-foundation-guard-selftest.sh"
run_check "State linkage backfill selftest" bash "$SCRIPT_DIR/state-linkage-backfill-selftest.sh"
run_check "State certification reconcile selftest (IMP-032 SCOPE-4b)" bash "$SCRIPT_DIR/state-certification-reconcile-selftest.sh"
run_check "Planning packet linkage guard selftest" bash "$SCRIPT_DIR/planning-packet-linkage-guard-selftest.sh"
run_check "Vertical-delivery plan guard selftest (BFW-02 / IMP-022)" bash "$SCRIPT_DIR/vertical-delivery-plan-guard-selftest.sh"
run_check "Surface reachability guard selftest (IMP-031 SCOPE-3)" bash "$SCRIPT_DIR/surface-reachability-guard-selftest.sh"
run_check_self_only "Surface reachability report (live, IMP-031 SCOPE-3)" bash "$SCRIPT_DIR/surface-reachability-guard.sh" --repo-root "$REPO_ROOT"
run_check "Technical prose lint selftest (IMP-030 SCOPE-3)" bash "$SCRIPT_DIR/technical-prose-lint-selftest.sh"
run_check_self_only "Technical prose report (live, report-only, IMP-030 SCOPE-3)" bash "$SCRIPT_DIR/technical-prose-lint.sh" "$REPO_ROOT/skills"
run_check "Plan dependency-depth guard selftest (IMP-100 Phase 4 / IMP-022 SCOPE-3+4)" bash "$SCRIPT_DIR/plan-dependency-depth-guard-selftest.sh"
run_check "Execution substate guard selftest (IMP-100 Phase 2 / IMP-024 SCOPE-3)" bash "$SCRIPT_DIR/execution-substate-guard-selftest.sh"
run_check "Evidence receipt check selftest (IMP-100 Phase 2 / IMP-024 SCOPE-1+2)" bash "$SCRIPT_DIR/evidence-receipt-check-selftest.sh"
run_check "Design-experiment guard selftest (IMP-100 Phase 4 / IMP-026 SCOPE-8)" bash "$SCRIPT_DIR/design-experiment-guard-selftest.sh"
run_check "Worktree hygiene guard selftest (IMP-107 / SCOPE-1; IMP-033 / SCOPE-1)" bash "$SCRIPT_DIR/worktree-hygiene-guard-selftest.sh"
run_check "Doctor hygiene surface selftest (IMP-033 / SCOPE-2 — EV-5)" bash "$SCRIPT_DIR/doctor-hygiene-surface-selftest.sh"
run_check "Open-work register selftest (IMP-033 / SCOPE-3 — WIP-1, WIP-2)" bash "$SCRIPT_DIR/open-work-report-selftest.sh"
run_check_self_only "Open-work register lint (live)" bash "$SCRIPT_DIR/open-work-report.sh" --repo-root "$REPO_ROOT" --lint
run_check "Closeout safety-contract selftest (IMP-033 / SCOPE-4 — WIP-3)" bash "$SCRIPT_DIR/closeout-report-selftest.sh"
run_check "Open-work surface selftest (IMP-033 / SCOPE-6 — WIP-1)" bash "$SCRIPT_DIR/open-work-surface-selftest.sh"
run_check "Multi-root honesty selftest (IMP-033 / SCOPE-7 — WIP-3)" bash "$SCRIPT_DIR/multi-root-honesty-selftest.sh"
run_check "Worktree finalize-reap selftest (IMP-107 / SCOPE-2 — WT-TEARDOWN)" bash "$SCRIPT_DIR/worktree-finalize-reap-selftest.sh"
run_check "Worktree spawn selftest (IMP-107 / SCOPE-5 — WT-HARNESS)" bash "$SCRIPT_DIR/worktree-spawn-selftest.sh"
run_check "Work-tracker projection selftest (IMP-100 Phase 4 / IMP-026 SCOPE-7)" bash "$SCRIPT_DIR/work-tracker-project-selftest.sh"
run_check "Scope context-fit lint selftest (IMP-100 Phase 4 / IMP-026 SCOPE-6)" bash "$SCRIPT_DIR/scope-context-fit-lint-selftest.sh"
run_check "Expand-migrate-contract guard selftest (IMP-100 Phase 4 / IMP-026 SCOPE-2)" bash "$SCRIPT_DIR/expand-migrate-contract-guard-selftest.sh"
run_check "IMP-021 interaction-discipline contracts selftest (SCOPE-1/3/4 + SCOPE-2 wiring)" bash "$SCRIPT_DIR/imp021-interaction-contracts-selftest.sh"
run_check "Post-certification spec edit guard selftest" bash "$SCRIPT_DIR/post-cert-spec-edit-guard-selftest.sh"
run_check "Inter-spec dependency guard selftest" bash "$SCRIPT_DIR/inter-spec-dependency-guard-selftest.sh"
run_check "Strict terminal status guard selftest" bash "$SCRIPT_DIR/strict-terminal-status-guard-selftest.sh"
run_check "Delivery implementation delta guard selftest" bash "$SCRIPT_DIR/delivery-implementation-delta-guard-selftest.sh"
run_check "Batch promotion lint selftest" bash "$SCRIPT_DIR/batch-promotion-lint-selftest.sh"
run_check "Done-spec audit selftest" bash "$SCRIPT_DIR/done-spec-audit-selftest.sh"
run_check "Test impact plan selftest" bash "$SCRIPT_DIR/test-impact-plan-selftest.sh"
run_check "Trace contract guard selftest" bash "$SCRIPT_DIR/trace-contract-guard-selftest.sh"

if [[ -x "$SCRIPT_DIR/runtime-lease-selftest.sh" ]]; then
  run_check_self_only "Runtime lease selftest" bash "$SCRIPT_DIR/runtime-lease-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/context-compactor-selftest.sh" ]]; then
  run_check "Context compactor selftest" bash "$SCRIPT_DIR/context-compactor-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/state-snapshot-selftest.sh" ]]; then
  run_check "State snapshot selftest" bash "$SCRIPT_DIR/state-snapshot-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/runtime-concurrency-selftest.sh" ]]; then
  run_check "Runtime concurrency selftest (IMP-102 / SCOPE-8)" bash "$SCRIPT_DIR/runtime-concurrency-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/implementation-reality-scan-selftest.sh" ]]; then
  run_check "Implementation reality scan selftest" bash "$SCRIPT_DIR/implementation-reality-scan-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/edit-lint-gate-selftest.sh" ]]; then
  run_check "Edit lint gate selftest" bash "$SCRIPT_DIR/edit-lint-gate-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/gate-id-grep-selftest.sh" ]]; then
  run_check "Gate ID grep selftest" bash "$SCRIPT_DIR/gate-id-grep-selftest.sh"
fi
# IMP-027 SCOPE-10: run the LIVE scan too, not only its selftest. Previously a
# retired gate ID could sit in README/docs prose indefinitely because nothing
# executed the scanner against the real tree.
if [[ -x "$SCRIPT_DIR/gate-id-grep.sh" ]]; then
  run_check_self_only "Gate ID grep (live, IMP-027 SCOPE-10)" bash "$SCRIPT_DIR/gate-id-grep.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/release-packet-location-guard-selftest.sh" ]]; then
  run_check "Release packet location guard selftest" bash "$SCRIPT_DIR/release-packet-location-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-packet-completeness-guard-selftest.sh" ]]; then
  run_check "Release packet completeness guard selftest (G138)" bash "$SCRIPT_DIR/release-packet-completeness-guard-selftest.sh"
fi

# The live guard runs too, not only its selftest. IMP-050 SCOPE-3: the location
# guard's own live path was never wired here, so it was exercised solely against
# synthetic fixtures. A repo with no docs/releases/ auto-exempts at exit 0.
if [[ -x "$SCRIPT_DIR/release-packet-completeness-guard.sh" ]]; then
  run_check "Release packet completeness (live, G138)" bash "$SCRIPT_DIR/release-packet-completeness-guard.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/release-delivery-reconciliation-guard-selftest.sh" ]]; then
  run_check "Release delivery reconciliation guard selftest (G101)" bash "$SCRIPT_DIR/release-delivery-reconciliation-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-delivery-reconciliation-guard.sh" ]]; then
  run_check "Release delivery reconciliation guard (live, G101)" bash "$SCRIPT_DIR/release-delivery-reconciliation-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/release-ladder-schema-guard-selftest.sh" ]]; then
  run_check "Release ladder schema guard selftest (G137)" bash "$SCRIPT_DIR/release-ladder-schema-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-ladder-schema-guard.sh" ]]; then
  run_check "Release ladder schema guard (live, G137)" bash "$SCRIPT_DIR/release-ladder-schema-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/workflow-surface-selftest.sh" ]]; then
  run_check "Workflow surface selftest" bash "$SCRIPT_DIR/workflow-surface-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/agent-ownership-lint-selftest.sh" ]]; then
  run_check "Agent ownership lint selftest" bash "$SCRIPT_DIR/agent-ownership-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/agnosticity-lint-selftest.sh" ]]; then
  run_check "Agnosticity lint selftest" bash "$SCRIPT_DIR/agnosticity-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/artifact-freshness-guard-selftest.sh" ]]; then
  run_check "Artifact freshness guard selftest" bash "$SCRIPT_DIR/artifact-freshness-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/instruction-budget-lint-selftest.sh" ]]; then
  run_check "Instruction budget lint selftest" bash "$SCRIPT_DIR/instruction-budget-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/regression-baseline-guard-selftest.sh" ]]; then
  run_check "Regression baseline guard selftest" bash "$SCRIPT_DIR/regression-baseline-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/regression-quality-guard-selftest.sh" ]]; then
  run_check "Regression quality guard selftest" bash "$SCRIPT_DIR/regression-quality-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/traceability-guard-selftest.sh" ]]; then
  run_check "Traceability guard selftest" bash "$SCRIPT_DIR/traceability-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/value-selection-lint-selftest.sh" ]]; then
  run_check "Value selection lint selftest" bash "$SCRIPT_DIR/value-selection-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/governance-index-lint-selftest.sh" ]]; then
  run_check "Governance index lint selftest" bash "$SCRIPT_DIR/governance-index-lint-selftest.sh"
fi

# Source-only: the consulted indexes are framework-layout paths that do not
# exist in a downstream install, where this would report false orphans.
if [[ -x "$SCRIPT_DIR/governance-index-lint.sh" ]]; then
  run_check_self_only "Governance index lint (live)" bash "$SCRIPT_DIR/governance-index-lint.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint-selftest.sh" ]]; then
  run_check "Orchestrator tool frontmatter lint selftest" bash "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/trajectory-inspector-selftest.sh" ]]; then
  run_check "Trajectory inspector selftest" bash "$SCRIPT_DIR/trajectory-inspector-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/propagation-policy-guard-selftest.sh" ]]; then
  run_check "Propagation policy guard selftest" bash "$SCRIPT_DIR/propagation-policy-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-train-rollup-selftest.sh" ]]; then
  run_check "Release train rollup selftest" bash "$SCRIPT_DIR/release-train-rollup-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-assurance-gate-selftest.sh" ]]; then
  run_check "Release assurance gate selftest (IMP-100 Phase 3 choke #3)" bash "$SCRIPT_DIR/release-assurance-gate-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-adapter-lint-selftest.sh" ]]; then
  run_check "Observability adapter lint selftest" bash "$SCRIPT_DIR/observability-adapter-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-adapter-lint.sh" && -d "$REPO_ROOT/bubbles/adapters/observability" ]]; then
  run_check "Observability adapter lint (live)" bash "$SCRIPT_DIR/observability-adapter-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/prometheus-adapter-fetch-selftest.sh" ]]; then
  run_check "Prometheus adapter live-fetch selftest (P2)" bash "$SCRIPT_DIR/prometheus-adapter-fetch-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-posture-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree fixtures under tests/fixtures/observability/
  # which the installer does not vendor downstream. The live G098 guard below
  # still runs everywhere.
  run_check_self_only "Observability posture guard selftest (G098)" bash "$SCRIPT_DIR/observability-posture-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-opt-out-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live G099 guard below still runs everywhere.
  run_check_self_only "Observability opt-out guard selftest (G099)" bash "$SCRIPT_DIR/observability-opt-out-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-posture-guard.sh" ]]; then
  run_check "Observability posture guard (live, G098)" bash "$SCRIPT_DIR/observability-posture-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-opt-out-guard.sh" ]]; then
  run_check "Observability opt-out guard (live, G099)" bash "$SCRIPT_DIR/observability-opt-out-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-slo-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live G100 guard below still runs everywhere.
  run_check_self_only "Observability SLO guard selftest (G100)" bash "$SCRIPT_DIR/observability-slo-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-slo-guard.sh" ]]; then
  run_check "Observability SLO guard (live, G100)" bash "$SCRIPT_DIR/observability-slo-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-endpoint-resolve-selftest.sh" ]]; then
  run_check "Observability endpoint resolver selftest (SCOPE-3)" bash "$SCRIPT_DIR/observability-endpoint-resolve-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-check-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live check twin below still runs everywhere.
  run_check_self_only "Observability check twin selftest (wired fixture)" bash "$SCRIPT_DIR/observability-check-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-check.sh" ]]; then
  run_check "Observability check twin (live, posture+SLO+trace+endpoints)" bash "$SCRIPT_DIR/observability-check.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/env-pollution-scan-selftest.sh" ]]; then
  run_check "Env pollution scan selftest (G115)" bash "$SCRIPT_DIR/env-pollution-scan-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/scenario-compile-lint-selftest.sh" ]]; then
  run_check "Scenario compile lint selftest" bash "$SCRIPT_DIR/scenario-compile-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/retro-framework-health-selftest.sh" ]]; then
  run_check "Retro framework-health selftest" bash "$SCRIPT_DIR/retro-framework-health-selftest.sh"
fi

# IMP-027 SCOPE-9: G125 had ZERO enforcer scripts. retro-framework-health.sh is
# the generator, not a verifier. These two run the independent check.
if [[ -x "$SCRIPT_DIR/framework-health-evidence-lint-selftest.sh" ]]; then
  run_check "Framework-health evidence lint selftest (G125, IMP-027 SCOPE-9)" bash "$SCRIPT_DIR/framework-health-evidence-lint-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/framework-health-evidence-lint.sh" ]]; then
  run_check_self_only "Framework-health evidence lint (live, G125)" bash "$SCRIPT_DIR/framework-health-evidence-lint.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/intent-routes-lint-selftest.sh" ]]; then
  run_check "Intent routes lint selftest" bash "$SCRIPT_DIR/intent-routes-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/intent-routes-lint.sh" && -f "$REPO_ROOT/bubbles/intent-routes.yaml" ]]; then
  run_check "Intent routes lint (live)" bash "$SCRIPT_DIR/intent-routes-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/stale-deferral-lint-selftest.sh" ]]; then
  run_check "Stale-deferral lint selftest" bash "$SCRIPT_DIR/stale-deferral-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/stale-deferral-lint.sh" ]]; then
  # Live scan is source-only: it reads the framework VERSION and scans the
  # framework's own managed docs. Downstream repos have their own VERSION +
  # product docs, so the lapsed-promise comparison is meaningful only here.
  run_check_self_only "Stale-deferral lint (live)" bash "$SCRIPT_DIR/stale-deferral-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/management-truth-lint-selftest.sh" ]]; then
  run_check "Management-truth lint selftest" bash "$SCRIPT_DIR/management-truth-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/managed-docs-existence-lint-selftest.sh" ]]; then
  run_check "Managed-doc existence lint selftest (IMP-042 SCOPE-13)" bash "$SCRIPT_DIR/managed-docs-existence-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/adoption-profile-lib-selftest.sh" ]]; then
  run_check "Adoption-profile library selftest (IMP-042 SCOPE-13)" bash "$SCRIPT_DIR/adoption-profile-lib-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/capability-consumer-naming-selftest.sh" ]]; then
  run_check "Capability consumer naming selftest (IMP-042 SCOPE-11)" bash "$SCRIPT_DIR/capability-consumer-naming-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/capability-consumer-naming.sh" ]]; then
  run_check_self_only "Capability consumer naming (live, IMP-042 SCOPE-11)" bash "$SCRIPT_DIR/capability-consumer-naming.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/managed-docs-existence-lint.sh" ]]; then
  # Skips in a framework source tree; the managed-doc contract governs product
  # repositories, which is where this has something to check.
  run_check "Managed-doc existence lint (live, IMP-042 SCOPE-13)" bash "$SCRIPT_DIR/managed-docs-existence-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/management-truth-lint.sh" ]]; then
  # Live scan is source-only: it checks the framework's OWN recipe catalog
  # (docs/recipes/README.md) and installer --profile help against the live
  # adoption-profiles.yaml. Downstream repos have their own docs, so the
  # catalog-completeness comparison is meaningful only here.
  run_check_self_only "Management-truth lint (live)" bash "$SCRIPT_DIR/management-truth-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/gate-strength-lint-selftest.sh" ]]; then
  run_check "Gate-strength lint selftest" bash "$SCRIPT_DIR/gate-strength-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/gate-strength-lint.sh" ]]; then
  # Source-only: publishes the enforcement-strength taxonomy of the framework's
  # OWN gate registry and fails only if a gate cannot be classified (a registry
  # parse problem). Downstream repos carry the same registry via managed sync.
  run_check_self_only "Gate-strength taxonomy (live)" bash "$SCRIPT_DIR/gate-strength-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/claim-source-lint-selftest.sh" ]]; then
  run_check "Claim-Source lint selftest" bash "$SCRIPT_DIR/claim-source-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/claim-source-lint.sh" ]]; then
  # Source-only: scans the framework's OWN report.md evidence blocks for the
  # G072 Claim-Source provenance tag. Advisory-until-opt-in, so it never blocks
  # here; the state-transition-guard invokes the same lint on every transition.
  run_check_self_only "Claim-Source provenance lint (live)" bash "$SCRIPT_DIR/claim-source-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/reference-existence-lint-selftest.sh" ]]; then
  run_check "Reference-existence lint selftest (G132)" bash "$SCRIPT_DIR/reference-existence-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/reference-existence-lint.sh" ]]; then
  # Source-only: scans the framework's OWN claim-bearing governance surfaces for
  # phantom path references (G132). The surface is named explicitly because the
  # lint has no default surface. docs/examples/ is EXCLUDED on purpose: an
  # example artifact legitimately renders self-referential links such as
  # [bug.md](bug.md) that describe a hypothetical spec folder rather than
  # claiming a path in this repo. Advisory-until-opt-in, so it never blocks here.
  run_check_self_only "Reference-existence lint (live, G132)" bash "$SCRIPT_DIR/reference-existence-lint.sh" \
    "$REPO_ROOT/agents" \
    "$REPO_ROOT/skills" \
    "$REPO_ROOT/instructions" \
    "$REPO_ROOT/prompts" \
    "$REPO_ROOT/docs/guides" \
    "$REPO_ROOT/docs/recipes" \
    "$REPO_ROOT/docs/generated" \
    "$REPO_ROOT/docs/governance-index.md" \
    "$REPO_ROOT/README.md"
fi

if [[ -x "$SCRIPT_DIR/effective-bundle-budget-selftest.sh" ]]; then
  run_check "Effective-bundle budget selftest" bash "$SCRIPT_DIR/effective-bundle-budget-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/effective-bundle-budget.sh" ]]; then
  # Source-only: measures the framework's OWN agent bundles against an OPTIONAL
  # effectiveBundleMaxBytes budget. With no budget configured it is purely
  # informational (never blocks); opt-in blocking is per .github/bubbles-project.yaml.
  run_check_self_only "Effective-bundle budget (live)" bash "$SCRIPT_DIR/effective-bundle-budget.sh" "$REPO_ROOT"
fi

# IMP-102 / SCOPE-10: ratcheting PER-AGENT effective-bundle size budget. The
# hermetic selftest runs everywhere (it builds its own fixtures + guards the
# real-tree sync case). The live --check is source-only: it measures the
# framework's OWN agents against the committed per-agent ceilings in
# bubbles/agent-bundle-budgets.json, which is a source-repo artifact (classified
# source-only in the release manifest, not shipped downstream where agents/ lives
# under .github/agents). Mirrors the effective-bundle-budget wiring above.
if [[ -x "$SCRIPT_DIR/agent-bundle-size-budget-selftest.sh" ]]; then
  run_check "Agent bundle-size budget selftest (IMP-102 / SCOPE-10)" bash "$SCRIPT_DIR/agent-bundle-size-budget-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/agent-bundle-size-budget.sh" ]]; then
  run_check_self_only "Agent bundle-size budget (ratcheting per-agent, IMP-102 / SCOPE-10)" bash "$SCRIPT_DIR/agent-bundle-size-budget.sh" --check --repo-root "$REPO_ROOT"
fi

# ---------------------------------------------------------------------------
# IMP-047 S-C — the RED-to-GREEN outcome engine.
#
# Enumerated rather than left to the discovery sweep below, because these four
# carry the outcome model itself: every scenario state derived from receipts,
# every substitution refused by name, tool-log evidence admitted on semantics
# instead of token overlap, and phase work resumed by occurrence identity.
# ---------------------------------------------------------------------------
if [[ -f "$SCRIPT_DIR/scenario-state-resolve-selftest.sh" ]]; then
  run_check "Scenario-state resolver selftest (IMP-047 S-C)" bash "$SCRIPT_DIR/scenario-state-resolve-selftest.sh"
fi

if [[ -f "$SCRIPT_DIR/receipt-identity-selftest.sh" ]]; then
  run_check "Receipt identity selftest (IMP-047 S-C)" bash "$SCRIPT_DIR/receipt-identity-selftest.sh"
fi

if [[ -f "$SCRIPT_DIR/tool-log-semantic-admission-selftest.sh" ]]; then
  run_check "Tool-log semantic admission selftest (IMP-047 S-C / AC13)" bash "$SCRIPT_DIR/tool-log-semantic-admission-selftest.sh"
fi

if [[ -f "$SCRIPT_DIR/phase-coordinator-selftest.sh" ]]; then
  run_check "Phase coordinator selftest (IMP-047 S-C / AC8-AC12)" bash "$SCRIPT_DIR/phase-coordinator-selftest.sh"
fi

# Dispatch receipts extend the same occurrence model UP one level, from the
# phase to the dispatch that was supposed to resolve it, so they are enumerated
# beside the coordinator rather than left to the sweep below.
if [[ -f "$SCRIPT_DIR/dispatch-receipt-selftest.sh" ]]; then
  run_check "Dispatch receipt selftest (IMP-048 SCOPE-2 / HO-4)" bash "$SCRIPT_DIR/dispatch-receipt-selftest.sh"
fi

# Leaf receipts extend that same occurrence model DOWN one level, from the phase
# to the individual test commands it is made of, so all three sit together.
if [[ -f "$SCRIPT_DIR/test-leaf-receipt-selftest.sh" ]]; then
  run_check "Test-leaf receipt selftest (IMP-048 SCOPE-3 / PERF-9)" bash "$SCRIPT_DIR/test-leaf-receipt-selftest.sh"
fi

# The review loop CONSUMES the signals the two receipt surfaces above produce —
# a repeated failure signature, a dispatch that returned no envelope — so it is
# enumerated with them rather than left to the sweep below.
if [[ -f "$SCRIPT_DIR/session-review-selftest.sh" ]]; then
  run_check "Session review selftest (IMP-048 SCOPE-1 / LRN-8)" bash "$SCRIPT_DIR/session-review-selftest.sh"
fi

# Liveness is what makes the session controls above readable at all: G083, G128
# and trajectory health all read the session store, so it is enumerated beside
# the cap guard whose store it keeps honest.
if [[ -f "$SCRIPT_DIR/session-liveness-selftest.sh" ]]; then
  run_check "Session liveness selftest (IMP-048 SCOPE-7 / WIP-5)" bash "$SCRIPT_DIR/session-liveness-selftest.sh"
fi

# Mutation receipts are the same earned-receipt rule applied to the strongest
# negative control: test-mechanism-lint.sh checks the declaration EXISTS, this
# checks the declared mutation actually RAN, in isolation, and was restored.
if [[ -f "$SCRIPT_DIR/mutation-receipt-selftest.sh" ]]; then
  run_check "Mutation receipt selftest (IMP-048 SCOPE-4 / EV-11)" bash "$SCRIPT_DIR/mutation-receipt-selftest.sh"
fi

# ---------------------------------------------------------------------------
# IMP-027 SCOPE-2b — selftest discovery sweep
#
# Everything above is an ENUMERATED check: a human wired it by hand. That
# enumeration is exactly how COV-2 happened — 8 selftests existed in the tree
# and were never executed by anything, because adding a file and adding its
# run_check line are two separate acts and only the first is required to
# commit.
#
# This sweep closes the class rather than the instances. It globs every
# bubbles/scripts/*-selftest.sh, skips the ones already named by an enumerated
# check above, skips anything explicitly denied in
# bubbles/registry/selftest-denylist.txt, and runs the remainder. A newly
# committed selftest is therefore executed with no wiring step at all.
#
# The enumerated checks are deliberately left in place: they carry tier
# assignments, install-mode gating, and arguments that a glob cannot infer.
# ---------------------------------------------------------------------------
if [[ "$LIST_TIER_ONLY" != "true" ]]; then
  # Resolved from the FRAMEWORK directory, not the repository root. The framework
  # is bubbles/ in the source tree and .github/bubbles/ in an installed
  # downstream, so a repo-root path resolved to nothing downstream, the denylist
  # read as EMPTY, and the sweep ran two suites it is contracted not to run --
  # repository-binding-selftest.sh asserts it was entered through the cli.sh
  # boundary, so a direct second run fails BY CONSTRUCTION. That is exactly the
  # fail-open shape the note below exists to prevent, one layer up, and it is
  # refused just as loudly.
  selftest_denylist="$SCRIPT_DIR/../registry/selftest-denylist.txt"

  # BUG-021. This sweep decides WHAT RUNS by matching each discovered name
  # against this validator's own source, so reading that source is control
  # flow, not decoration. The first version read it with `$(cat ... 2>/dev/null
  # || true)`, which made an EXTERNAL tool load-bearing. Under a minimal PATH --
  # exactly what tests/regression/test_28_framework_validate_portable_timeout.sh
  # constructs, and what a hardened CI image can present -- `cat` is absent, the
  # error was swallowed, `fv_source` collapsed to empty, no name ever matched,
  # and every already-enumerated selftest was silently run a SECOND time outside
  # the watchdog that bounds it. That is a wrong verdict reported as a pass.
  #
  # Read with the bash builtin so the sweep depends on nothing but bash, and
  # refuse LOUDLY if the source cannot be read at all, rather than degrading
  # into "re-run everything". The denylist is parsed with builtins for the same
  # reason: no external tool may decide what this validator executes.
  fv_source=""
  if [[ -r "$SCRIPT_DIR/framework-validate.sh" ]]; then
    fv_source="$(<"$SCRIPT_DIR/framework-validate.sh")"
  fi

  if [[ -z "$fv_source" ]]; then
    echo "==> Discovered selftest sweep (IMP-027 SCOPE-2b)"
    echo "FAIL: cannot read $SCRIPT_DIR/framework-validate.sh to identify already-enumerated selftests"
    bubbles_ci_annotate_failure "FAIL: cannot read $SCRIPT_DIR/framework-validate.sh to identify already-enumerated selftests"
    echo "      refusing to re-run every selftest unbounded; fix the install rather than ignoring this"
    failures=$((failures + 1))
    failed_check_labels+=("Discovered selftest sweep (IMP-027 SCOPE-2b)")
    echo
  elif [[ ! -r "$selftest_denylist" && -d "$SCRIPT_DIR/../registry" ]]; then
    echo "==> Discovered selftest sweep (IMP-027 SCOPE-2b)"
    echo "FAIL: cannot read $selftest_denylist to identify denied selftests"
    bubbles_ci_annotate_failure "FAIL: cannot read $selftest_denylist to identify denied selftests"
    echo "      refusing to run suites contracted to run only through their own entry point"
    failures=$((failures + 1))
    failed_check_labels+=("Discovered selftest sweep (IMP-027 SCOPE-2b)")
    echo
  else
    # Newline-delimited denied names, read ONCE with builtins. Semantics match
    # the previous grep pair exactly: blank lines and lines whose first
    # non-space character is '#' are ignored, and a name must match a whole
    # line exactly.
    selftest_denied_names=$'\n'
    if [[ -r "$selftest_denylist" ]]; then
      while IFS= read -r selftest_deny_line || [[ -n "$selftest_deny_line" ]]; do
        selftest_deny_trimmed="${selftest_deny_line#"${selftest_deny_line%%[![:space:]]*}"}"
        [[ -z "$selftest_deny_trimmed" || "$selftest_deny_trimmed" == '#'* ]] && continue
        selftest_denied_names+="$selftest_deny_line"$'\n'
      done <"$selftest_denylist"
    else
      # No registry directory at all: a deliberately partial tree, such as the
      # stub fixture repo-drift-report-selftest stages to prove this validator
      # runs non-blockingly. Proceeding is correct there, but it must be SAID --
      # the whole point of the branch above is that an empty deny-list silently
      # decides what runs.
      echo "NOTE: no registry directory at $SCRIPT_DIR/../registry; running the sweep with an EMPTY deny-list"
    fi

    # Which selftests are ALREADY wired. Decided from real run_check invocations
    # rather than from raw source text: a substring match treats ANY mention as
    # "already covered", so a single comment naming a selftest would remove it
    # from the run with nothing reporting the loss.
    #
    # The trailing newline is re-appended because command substitution strips
    # every trailing newline, which would leave the LAST scheduled entry without
    # its delimiter and re-run it a second time.
    fv_scheduled="$(bubbles_scheduled_selftests "$fv_source")"$'\n'

    for selftest_path in "$SCRIPT_DIR"/*-selftest.sh; do
      [[ -f "$selftest_path" ]] || continue
      selftest_name="${selftest_path##*/}"

      # Already wired by an enumerated check above.
      case "$fv_scheduled" in
        *$'\n'"$selftest_name"$'\n'*) continue ;;
      esac

      # Explicitly denied, with a documented reason.
      case "$selftest_denied_names" in
        *$'\n'"$selftest_name"$'\n'*)
          echo "==> Discovered selftest: $selftest_name"
          echo "SKIP: $selftest_name (denied in bubbles/registry/selftest-denylist.txt)"
          skipped=$((skipped + 1))
          skipped_denied=$((skipped_denied + 1))
          echo
          continue
          ;;
      esac

      run_check "Discovered selftest: $selftest_name (IMP-027 SCOPE-2b)" bash "$selftest_path"
    done
  fi
fi

if [[ -x "$SCRIPT_DIR/selftest-coverage-lint.sh" ]]; then
  run_check_self_only "Selftest coverage lint (IMP-027 SCOPE-2b)" bash "$SCRIPT_DIR/selftest-coverage-lint.sh" --repo-root "$REPO_ROOT"
fi
if [[ -x "$SCRIPT_DIR/selftest-coverage-lint-selftest.sh" ]]; then
  run_check "Selftest coverage lint selftest (IMP-027 SCOPE-2b)" bash "$SCRIPT_DIR/selftest-coverage-lint-selftest.sh"
fi

# Manifest freshness runs LAST, and deliberately so. Several checks above
# regenerate derived artifacts, so a freshness verdict computed mid-run
# describes a tree that the rest of the run can still change. Asking the
# question at the end is the only placement that answers it about the tree the
# operator is actually about to ship.
run_check_self_only "Release manifest freshness" bash "$SCRIPT_DIR/generate-release-manifest.sh" --check

if [[ "$LIST_TIER_ONLY" == "true" ]]; then
  echo "Tier listing complete (tier=$VALIDATE_TIER). No checks were executed."
  exit 0
fi

# PERF report. Printed on success AND on failure, because the run costs the
# same either way and the total is only actionable next to the checks that
# bought it. Ranking is done in-shell: this is the canonical success path, so
# it must not depend on any external tool (BUG-021).
if [[ ${#check_durations[@]} -gt 0 ]]; then
  perf_remaining=("${check_durations[@]}")
  perf_lines=""
  perf_rank=0
  while [[ "$perf_rank" -lt 10 && ${#perf_remaining[@]} -gt 0 ]]; do
    perf_best_idx=-1
    perf_best_secs=0
    for perf_i in "${!perf_remaining[@]}"; do
      perf_secs="${perf_remaining[$perf_i]%%|*}"
      if [[ "$perf_secs" -gt "$perf_best_secs" ]]; then
        perf_best_secs="$perf_secs"
        perf_best_idx="$perf_i"
      fi
    done
    # Everything left cost under a second; there is nothing more to report.
    [[ "$perf_best_idx" -ge 0 ]] || break
    perf_lines+="$(printf '  %4ds  %s' "$perf_best_secs" "${perf_remaining[$perf_best_idx]#*|}")"$'\n'
    unset "perf_remaining[$perf_best_idx]"
    perf_remaining=("${perf_remaining[@]}")
    perf_rank=$((perf_rank + 1))
  done

  echo "Wall clock: ${SECONDS}s across ${#check_durations[@]} executed check(s)."
  if [[ -n "$perf_lines" ]]; then
    echo "Slowest checks (>=1s):"
    printf '%s' "$perf_lines"
  fi
  echo
fi

# Reports skips per reason so the operator can tell tier filtering apart from
# a genuinely unavailable framework-source check.
skip_summary() {
  local parts=""
  if [[ "$skipped_tier" -gt 0 ]]; then
    parts="$skipped_tier tier=$VALIDATE_TIER"
  fi
  if [[ "$skipped_changed_only" -gt 0 ]]; then
    parts="${parts:+$parts, }$skipped_changed_only --changed-only"
  fi
  if [[ "$skipped_self_only" -gt 0 ]]; then
    parts="${parts:+$parts, }$skipped_self_only framework-source-only (install-mode=$INSTALL_MODE)"
  fi
  if [[ "$skipped_denied" -gt 0 ]]; then
    parts="${parts:+$parts, }$skipped_denied denylisted"
  fi
  printf '%s' "$parts"
}

if [[ "$failures" -gt 0 ]]; then
  fv_write_receipt fail
  if [[ "$skipped" -gt 0 ]]; then
    echo "Framework validation failed with $failures failing check(s) ($skipped skipped: $(skip_summary))."
  else
    echo "Framework validation failed with $failures failing check(s)."
  fi
  echo "Failed checks:"
  for failed_label in "${failed_check_labels[@]}"; do
    echo "  - $failed_label"
  done
  exit 1
fi

# ---------------------------------------------------------------------------
# IMP-047 S-E: settle the deferrals into the append-only ledger.
#
# ORDER IS THE POINT. `basic` is the immediate affected-validation floor INSIDE
# `fast`, not a fourth assurance level, so the affected set must ALREADY HAVE RUN
# before any obligation may be deferred. That is why this block sits after the
# failure gate: a run with failures never reaches it, and debt is therefore never
# taken out against a floor that did not hold.
#
# A MISSING LEDGER WRITE FORCES IMMEDIATE EXECUTION. If `record` refuses, the
# check is run right here instead of being treated as deferred, because a
# deferral nobody recorded is a silent skip.
if [[ "$RECORD_DEBT" == "true" && "${#deferred_check_ids[@]}" -gt 0 ]]; then
  echo "==> Validation debt (IMP-047 S-E)"
  debt_tool="$SCRIPT_DIR/validation-debt.sh"
  debt_dir="${BUBBLES_VALIDATION_DEBT_DIR:-$REPO_ROOT/.specify/runtime/validation-debt}"
  source_revision="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"

  floor_receipt="$debt_dir/floor-$source_revision.json"
  if mkdir -p "$debt_dir" 2>/dev/null; then
    printf '{"floor":"basic","sourceRevision":"%s","executed":"%s","failures":"0","writtenAt":"%s"}\n' \
      "$source_revision" "${#check_durations[@]}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$floor_receipt"
  fi

  debt_idx=0
  for deferred_id in "${deferred_check_ids[@]}"; do
    deferred_label="${deferred_check_labels[$debt_idx]}"
    deferred_cmd="${deferred_check_cmds[$debt_idx]}"
    debt_idx=$((debt_idx + 1))
    if [[ -x "$debt_tool" || -f "$debt_tool" ]] \
      && bash "$debt_tool" record \
        --check "$deferred_id" \
        --class heavy-selftest \
        --source-revision "$source_revision" \
        --spec "framework-validate" \
        --reason closure-unaffected \
        --boundary release \
        --floor-receipt "$floor_receipt" >/dev/null 2>&1; then
      echo "DEFERRED: $deferred_label (obligation recorded, due at the release boundary)"
      continue
    fi
    echo "LEDGER-WRITE-FAILED: $deferred_label — executing it now rather than deferring silently."
    eval "$deferred_cmd" || {
      echo "FAIL: $deferred_label (forced execution after a failed ledger write)"
      bubbles_ci_annotate_failure "FAIL: $deferred_label (forced execution after a failed ledger write)"
      failures=$((failures + 1))
      failed_check_labels+=("$deferred_label")
    }
  done
  echo

  if [[ "$failures" -gt 0 ]]; then
    fv_write_receipt fail
    echo "Framework validation failed with $failures failing check(s) after forced execution."
    exit 1
  fi
fi

if [[ "$skipped" -gt 0 ]]; then
  echo "Framework validation passed ($skipped skipped: $(skip_summary))."
  if [[ "$skipped_self_only" -gt 0 ]]; then
    echo "Run from a framework-source tree to execute the framework-source-only check(s)."
  fi
fi

# The receipt is written LAST, for the same reason the manifest freshness check
# runs last: several checks above regenerate derived artifacts, so a digest taken
# any earlier would describe a tree the rest of the run could still change.
fv_write_receipt pass

echo "Framework validation passed."
