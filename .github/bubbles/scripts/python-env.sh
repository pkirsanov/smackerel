#!/usr/bin/env bash
# python-env.sh — managed Python interpreter resolution and dependency
# provisioning for Bubbles.
#
# WHY THIS EXISTS
# ---------------
# dependency-posture.sh declares python3 + PyYAML + jsonschema REQUIRED and
# refuses to let a guard skip silently ("a guard that skips is a guard that
# lies"). But the framework shipped no supported way to OBTAIN those modules, so
# the declaration had no reachable remedy. Observed consequences on a real
# operator machine:
#
#   * `pip install --user --break-system-packages` — forces past PEP 668 on a
#     Homebrew interpreter, and lands in whichever python3 PATH resolved that
#     day.
#   * a virtualenv under /tmp, referenced by nothing in the repo, wiped on
#     reboot.
#
# Both are shortcuts, and both are silently undone by the next PATH change. A
# machine can carry several python3 installs (Homebrew, python.org, Xcode,
# conda); `command -v python3` is therefore NOT a stable identity, and a
# successful install can stop applying without anything being edited.
#
# THE CONTRACT
#   The managed virtualenv owns its own interpreter, so once provisioned it
#   keeps satisfying regardless of later PATH or conda changes. Resolution is
#   explicit and ordered, never ambient:
#
#     1. $BUBBLES_PYTHON            — operator override, honored only if it satisfies
#     2. the managed venv           — bubbles_python_home()
#     3. python3 from PATH          — honored only if it already satisfies
#
#   Nothing is auto-installed. Provisioning is an explicit operator act:
#     bash bubbles/scripts/python-env.sh --provision
#
# SOURCEABLE. Sourcing defines functions and changes nothing else — it does NOT
# set shell options, because callers source this from scripts with their own
# `set` posture.
#
# Usage (executed):
#   python-env.sh --check       posture report; exit 0 satisfied, 1 not
#   python-env.sh --provision   create/repair the managed venv from requirements.txt
#   python-env.sh --path        print the resolved interpreter; exit 1 if none
#   python-env.sh --help
#
# Exit codes: 0 ok · 1 unsatisfied · 2 usage or provisioning error.

# The python modules Bubbles requires. Kept in lockstep with the
# `python-module:` entries of BUBBLES_REQUIRED_DEPS in dependency-posture.sh;
# python-env-selftest.sh asserts the two agree, so the duplication cannot drift.
BUBBLES_PYTHON_MODULES=(yaml jsonschema)

# bubbles_python_home — durable root of the managed virtualenv.
# NOT under /tmp: a temp-dir venv is erased on reboot, which is exactly the
# shortcut this module replaces.
#
# Every locator is expanded GUARDED, and "there is no locator" is returned as a
# condition rather than printed as a path. An unguarded $HOME inside a `:-`
# default still aborts under `set -u`; because resolution reads this through a
# command substitution, that abort arrived at the caller as an EMPTY candidate
# and was reported as "no interpreter satisfies the required modules" — a
# sentence about interpreters, when the actual subject was an absent locator.
# Naming the two apart is the whole point of returning 1 here.
BUBBLES_PYTHON_LOCATOR_VARS='BUBBLES_PYTHON_HOME, XDG_CACHE_HOME, or HOME'

bubbles_python_home() {
  if [[ -n "${BUBBLES_PYTHON_HOME:-}" ]]; then
    printf '%s\n' "${BUBBLES_PYTHON_HOME%/}"
    return 0
  fi
  if [[ -n "${XDG_CACHE_HOME:-}" ]]; then
    printf '%s\n' "${XDG_CACHE_HOME%/}/bubbles/python"
    return 0
  fi
  if [[ -n "${HOME:-}" ]]; then
    printf '%s\n' "${HOME%/}/.cache/bubbles/python"
    return 0
  fi
  return 1
}

bubbles_python_venv_python() {
  local home
  home="$(bubbles_python_home)" || return 1
  printf '%s\n' "$home/bin/python3"
}

# bubbles_python_satisfies <interpreter> — true when it imports every required module.
bubbles_python_satisfies() {
  local py="${1:-}" module
  [[ -n "$py" && -x "$py" ]] || return 1
  for module in "${BUBBLES_PYTHON_MODULES[@]}"; do
    "$py" -c "import $module" >/dev/null 2>&1 || return 1
  done
  return 0
}

# bubbles_python_resolve — print the first interpreter that satisfies, in the
# documented order. Prints nothing and returns 1 when none does.
bubbles_python_resolve() {
  local candidate
  if [[ -n "${BUBBLES_PYTHON:-}" ]] && bubbles_python_satisfies "$BUBBLES_PYTHON"; then
    printf '%s\n' "$BUBBLES_PYTHON"
    return 0
  fi
  if candidate="$(bubbles_python_venv_python)" && bubbles_python_satisfies "$candidate"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  candidate="$(command -v python3 2>/dev/null || true)"
  if bubbles_python_satisfies "$candidate"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  return 1
}

# bubbles_python_runs remains a GENERAL usability probe. It can execute a
# caller-selected interpreter, so its result never grants Scan 2B authority.
# Security authority starts at bubbles_python_resolve_security_runtime below.
BUBBLES_PYTHON_RUN_SENTINEL='bubbles-python-runs'
BUBBLES_PYTHON_RUN_STATUS=127
BUBBLES_PYTHON_RUN_DIAGNOSTIC='NOT_RUN'

# Security runtime identity. These values are diagnostic outputs, not bearer
# credentials: every Python launch re-authenticates the stored path.
BUBBLES_PYTHON_SECURITY_RUNTIME=''
BUBBLES_PYTHON_SECURITY_STATUS=127
BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NOT_RUN'
BUBBLES_PYTHON_SECURITY_REJECTION='NONE'
BUBBLES_PYTHON_SECURITY_CANDIDATE_COUNT=0
BUBBLES_PYTHON_SECURITY_PROVENANCE='none'
BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT='root-protected-native-python-v1'
BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL='none'
BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL='none'
BUBBLES_PYTHON_SECURITY_DEVELOPER_DIR=''

# Privileged entry, native-supervisor result, and cleanup registry. Bash retains
# only the direct Perl supervisor as a wait handle. The worker PID remains
# lexical Perl state and is never exported, printed, persisted, or signaled by
# Bash. This contract makes no recursive-descendant containment claim.
BUBBLES_PYTHON_SECURITY_ENTRY_STATUS=127
BUBBLES_PYTHON_SECURITY_ENTRY_DIAGNOSTIC='NOT_RUN'
BUBBLES_PYTHON_SECURITY_ENTRY_PROTOCOL='none'
BUBBLES_PYTHON_SECURITY_ENTRY_MODE='none'
BUBBLES_PYTHON_SECURITY_SUPERVISOR_CONTRACT='none'
BUBBLES_PYTHON_SECURITY_SUPERVISOR_PROTOCOL='none'
BUBBLES_PYTHON_SECURITY_SUPERVISOR_REJECTION='NONE'
BUBBLES_PYTHON_SECURITY_RUN_STATUS=125
BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='NOT_RUN'
BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT=0
BUBBLES_PYTHON_SECURITY_RUN_OPERATION=''
BUBBLES_PYTHON_SECURITY_RUN_OWNER='supervisor'
BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND='not-started'
BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES=0
BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES=0
BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT=''
BUBBLES_PYTHON_SECURITY_STDOUT_PATH=''
BUBBLES_PYTHON_SECURITY_STDERR_PATH=''
BUBBLES_PYTHON_SECURITY_CONTROL_PATH=''
BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''
BUBBLES_PYTHON_SECURITY_STATE='IDLE'
BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL=''
BUBBLES_PYTHON_SECURITY_PENDING_STATUS=0
BUBBLES_PYTHON_SECURITY_SAVED_TRAP_EXIT=''
BUBBLES_PYTHON_SECURITY_SAVED_TRAP_HUP=''
BUBBLES_PYTHON_SECURITY_SAVED_TRAP_INT=''
BUBBLES_PYTHON_SECURITY_SAVED_TRAP_TERM=''
BUBBLES_PYTHON_SECURITY_SAVED_UMASK=''
BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE=0
BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS=0
BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT=''
BUBBLES_PYTHON_SECURITY_CAPTURE_BYTES=0

# Path-authentication scratch outputs. No function below prints a candidate.
BUBBLES_PYTHON_SECURITY_PATH_RESOLVED=''
BUBBLES_PYTHON_SECURITY_PATH_REJECTION='NONE'
BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='OK'
BUBBLES_PYTHON_SECURITY_META_OWNER=''
BUBBLES_PYTHON_SECURITY_META_MODE=''
BUBBLES_PYTHON_SECURITY_META_TYPE=''
BUBBLES_PYTHON_SECURITY_NORMALIZED_PATH=''

_bubbles_python_security_path_text_safe() {
  local value="${1:-}"
  [[ -n "$value" ]] || return 1
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* && "$value" != *$'\t'* ]] || return 1
  return 0
}

_bubbles_python_security_normalize_absolute() {
  local value="${1:-}"
  local part=""
  local normalized=""
  local -a input_parts=()
  local -a output_parts=()

  BUBBLES_PYTHON_SECURITY_NORMALIZED_PATH=''
  _bubbles_python_security_path_text_safe "$value" || return 1
  [[ "$value" == /* ]] || return 1
  IFS='/' read -r -a input_parts <<<"${value#/}"
  for part in "${input_parts[@]+"${input_parts[@]}"}"; do
    case "$part" in
      '' | .) ;;
      ..)
        if [[ ${#output_parts[@]} -gt 0 ]]; then
          unset 'output_parts[${#output_parts[@]}-1]'
        fi
        ;;
      *) output_parts+=("$part") ;;
    esac
  done
  normalized='/'
  for part in "${output_parts[@]+"${output_parts[@]}"}"; do
    if [[ "$normalized" == '/' ]]; then
      normalized="/$part"
    else
      normalized="$normalized/$part"
    fi
  done
  BUBBLES_PYTHON_SECURITY_NORMALIZED_PATH="$normalized"
  return 0
}

_bubbles_python_security_parse_stat_record() {
  local record="${1:-}"
  local owner="" mode="" type="" extra=""

  IFS='|' read -r owner mode type extra <<<"$record"
  [[ -z "$extra" && "$owner" =~ ^[0-9]+$ && "$mode" =~ ^[0-7]+$ && -n "$type" ]] || return 1
  BUBBLES_PYTHON_SECURITY_META_OWNER="$owner"
  BUBBLES_PYTHON_SECURITY_META_MODE="$mode"
  case "$type" in
    Directory | directory) BUBBLES_PYTHON_SECURITY_META_TYPE='directory' ;;
    'Regular File' | 'regular file' | 'regular empty file') BUBBLES_PYTHON_SECURITY_META_TYPE='file' ;;
    'Symbolic Link' | 'symbolic link') BUBBLES_PYTHON_SECURITY_META_TYPE='symlink' ;;
    *) BUBBLES_PYTHON_SECURITY_META_TYPE="$type" ;;
  esac
  return 0
}

_bubbles_python_security_stat() {
  local path="${1:-}"
  local record=""

  BUBBLES_PYTHON_SECURITY_META_OWNER=''
  BUBBLES_PYTHON_SECURITY_META_MODE=''
  BUBBLES_PYTHON_SECURITY_META_TYPE=''

  # GNU stat accepts -f too, but with different semantics. A successful exit
  # is therefore not a capability result: accept a dialect only when the full
  # owner/mode/type record validates.
  if record="$(/usr/bin/stat -f '%u|%Lp|%HT' "$path" 2>/dev/null)" &&
    _bubbles_python_security_parse_stat_record "$record"; then
    return 0
  fi
  if record="$(/usr/bin/stat -c '%u|%a|%F' "$path" 2>/dev/null)" &&
    _bubbles_python_security_parse_stat_record "$record"; then
    return 0
  fi
  BUBBLES_PYTHON_SECURITY_META_OWNER=''
  BUBBLES_PYTHON_SECURITY_META_MODE=''
  BUBBLES_PYTHON_SECURITY_META_TYPE=''
  return 1
}

_bubbles_python_security_mode_is_writable() {
  local mode="${1:-}"
  local length=0 permissions="" group_digit="" other_digit=""
  [[ "$mode" =~ ^[0-7]+$ ]] || return 0
  length=${#mode}
  [[ "$length" -ge 3 ]] || return 0
  permissions="${mode:$((length - 3)):3}"
  group_digit="${permissions:1:1}"
  other_digit="${permissions:2:1}"
  case "$group_digit" in 2 | 3 | 6 | 7) return 0 ;; esac
  case "$other_digit" in 2 | 3 | 6 | 7) return 0 ;; esac
  return 1
}

_bubbles_python_security_mode_has_setid() {
  local mode="${1:-}"
  local length=${#mode}
  local special=0
  if [[ "$length" -ge 4 ]]; then
    special="${mode:$((length - 4)):1}"
  fi
  case "$special" in 2 | 3 | 4 | 5 | 6 | 7) return 0 ;; esac
  return 1
}

_bubbles_python_security_validate_directory() {
  local path="${1:-}"
  if ! _bubbles_python_security_stat "$path"; then
    BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='METADATA_UNAVAILABLE'
    return 2
  fi
  if [[ "$BUBBLES_PYTHON_SECURITY_META_OWNER" != 0 ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='ANCESTOR_OWNER'
    return 1
  fi
  if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" != directory ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_TYPE'
    return 1
  fi
  if _bubbles_python_security_mode_is_writable "$BUBBLES_PYTHON_SECURITY_META_MODE"; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='ANCESTOR_MODE_WRITABLE'
    return 1
  fi
  if [[ -w "$path" ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='ANCESTOR_CALLER_WRITABLE'
    return 1
  fi
  return 0
}

_bubbles_python_security_native_format() {
  local path="${1:-}"
  local magic=""
  magic="$(/usr/bin/od -An -tx1 -N4 "$path" 2>/dev/null | /usr/bin/tr -d '[:space:]')" || return 1
  case "$magic" in
    7f454c46 | feedface | cefaedfe | feedfacf | cffaedfe | cafebabe | bebafeca | cafebabf | bfbafeca) return 0 ;;
    *) return 1 ;;
  esac
}

# Authenticate one absolute path without executing it. Symlinks are inspected
# and resolved lexically, with a fixed depth and repeated-target rejection.
_bubbles_python_security_authenticate_path() {
  local requested="${1:-}"
  local expected_kind="${2:-file}"
  local require_native="${3:-0}"
  local current="" component_path="" parent="" target="" combined="" suffix=""
  local part="" seen='|'
  local link_count=0 index=0 remainder_index=0 restarted=0
  local -a components=()

  BUBBLES_PYTHON_SECURITY_PATH_RESOLVED=''
  BUBBLES_PYTHON_SECURITY_PATH_REJECTION='NONE'
  BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='OK'
  if ! _bubbles_python_security_path_text_safe "$requested"; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='PATH_TEXT_UNSAFE'
    return 1
  fi
  if [[ "$requested" != /* ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='NOT_ABSOLUTE'
    return 1
  fi
  if ! _bubbles_python_security_normalize_absolute "$requested"; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='PATH_TEXT_UNSAFE'
    return 1
  fi
  current="$BUBBLES_PYTHON_SECURITY_NORMALIZED_PATH"
  seen="|$current|"

  while :; do
    _bubbles_python_security_validate_directory / || return $?
    IFS='/' read -r -a components <<<"${current#/}"
    component_path=''
    restarted=0
    index=0
    for part in "${components[@]+"${components[@]}"}"; do
      index=$((index + 1))
      if [[ -z "$component_path" ]]; then
        component_path="/$part"
      else
        component_path="$component_path/$part"
      fi
      if [[ ! -e "$component_path" && ! -L "$component_path" ]]; then
        BUBBLES_PYTHON_SECURITY_PATH_REJECTION='ABSENT'
        return 1
      fi
      if ! _bubbles_python_security_stat "$component_path"; then
        BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='METADATA_UNAVAILABLE'
        return 2
      fi
      if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" == symlink ]]; then
        if [[ "$BUBBLES_PYTHON_SECURITY_META_OWNER" != 0 ]]; then
          BUBBLES_PYTHON_SECURITY_PATH_REJECTION='SYMLINK_OWNER'
          return 1
        fi
        link_count=$((link_count + 1))
        if [[ "$link_count" -gt 32 ]]; then
          BUBBLES_PYTHON_SECURITY_PATH_REJECTION='SYMLINK_DEPTH'
          return 1
        fi
        target="$(/usr/bin/readlink "$component_path" 2>/dev/null)" || {
          BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='METADATA_UNAVAILABLE'
          return 2
        }
        if ! _bubbles_python_security_path_text_safe "$target"; then
          BUBBLES_PYTHON_SECURITY_PATH_REJECTION='PATH_TEXT_UNSAFE'
          return 1
        fi
        parent="${component_path%/*}"
        [[ -n "$parent" ]] || parent='/'
        if [[ "$target" == /* ]]; then
          combined="$target"
        elif [[ "$parent" == '/' ]]; then
          combined="/$target"
        else
          combined="$parent/$target"
        fi
        suffix=''
        remainder_index=$index
        while [[ "$remainder_index" -lt "${#components[@]}" ]]; do
          suffix="$suffix/${components[$remainder_index]}"
          remainder_index=$((remainder_index + 1))
        done
        if ! _bubbles_python_security_normalize_absolute "$combined$suffix"; then
          BUBBLES_PYTHON_SECURITY_PATH_REJECTION='PATH_TEXT_UNSAFE'
          return 1
        fi
        current="$BUBBLES_PYTHON_SECURITY_NORMALIZED_PATH"
        if [[ "$seen" == *"|$current|"* ]]; then
          BUBBLES_PYTHON_SECURITY_PATH_REJECTION='SYMLINK_CYCLE'
          return 1
        fi
        seen="$seen$current|"
        restarted=1
        break
      fi
      if [[ "$index" -lt "${#components[@]}" ]]; then
        _bubbles_python_security_validate_directory "$component_path" || return $?
      fi
    done
    [[ "$restarted" -eq 1 ]] || break
  done

  if [[ ! -e "$current" ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='ABSENT'
    return 1
  fi
  if ! _bubbles_python_security_stat "$current"; then
    BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='METADATA_UNAVAILABLE'
    return 2
  fi
  if [[ "$BUBBLES_PYTHON_SECURITY_META_OWNER" != 0 ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_OWNER'
    return 1
  fi
  if _bubbles_python_security_mode_is_writable "$BUBBLES_PYTHON_SECURITY_META_MODE"; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_MODE_WRITABLE'
    return 1
  fi
  if [[ -w "$current" ]]; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_CALLER_WRITABLE'
    return 1
  fi
  case "$expected_kind" in
    directory)
      if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" != directory ]]; then
        BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_TYPE'
        return 1
      fi
      ;;
    file)
      if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" != file ]]; then
        BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_TYPE'
        return 1
      fi
      ;;
    executable)
      if [[ "$BUBBLES_PYTHON_SECURITY_META_TYPE" != file || ! -x "$current" ]]; then
        BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_TYPE'
        return 1
      fi
      if _bubbles_python_security_mode_has_setid "$BUBBLES_PYTHON_SECURITY_META_MODE"; then
        BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_MODE_WRITABLE'
        return 1
      fi
      ;;
    *)
      # Sourced callers inspect this path-authentication diagnostic.
      # shellcheck disable=SC2034
      BUBBLES_PYTHON_SECURITY_PATH_DIAGNOSTIC='METADATA_UNAVAILABLE'
      return 2
      ;;
  esac
  if [[ "$require_native" -eq 1 ]] && ! _bubbles_python_security_native_format "$current"; then
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TARGET_FORMAT'
    return 1
  fi
  # The traversal above authenticates every caller-visible prefix before it
  # follows a link, then authenticates the complete resolved target chain.
  # Re-walking the original parent as directories would incorrectly reject a
  # protected symlink in an intermediate component.
  BUBBLES_PYTHON_SECURITY_PATH_RESOLVED="$current"
  return 0
}

_bubbles_python_security_validate_search_root() {
  local path="${1:-}"
  local parent=""
  _bubbles_python_security_path_text_safe "$path" || {
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='PATH_TEXT_UNSAFE'
    return 1
  }
  [[ "$path" == /* ]] || {
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='NOT_ABSOLUTE'
    return 1
  }
  if [[ -e "$path" || -L "$path" ]]; then
    if [[ -d "$path" ]]; then
      _bubbles_python_security_authenticate_path "$path" directory 0
    else
      _bubbles_python_security_authenticate_path "$path" file 0
    fi
    return $?
  fi
  parent="$path"
  while [[ ! -e "$parent" && ! -L "$parent" ]]; do
    [[ "$parent" != '/' ]] || break
    parent="${parent%/*}"
    [[ -n "$parent" ]] || parent='/'
  done
  _bubbles_python_security_authenticate_path "$parent" directory 0
}

bubbles_python_security_require_boundary() {
  local function_name=""
  local environment_line=""
  local environment_name=""

  BUBBLES_PYTHON_SECURITY_ENTRY_STATUS=127
  BUBBLES_PYTHON_SECURITY_ENTRY_DIAGNOSTIC='SECURITY_BOUNDARY_REQUIRED'
  BUBBLES_PYTHON_SECURITY_ENTRY_PROTOCOL='none'
  BUBBLES_PYTHON_SECURITY_ENTRY_MODE='none'

  case "$-" in
    *p*) ;;
    *) return 1 ;;
  esac
  case "${BUBBLES_SECURITY_ENTRY_MODE:-}" in
    direct | compat-reexec)
      BUBBLES_PYTHON_SECURITY_ENTRY_MODE="$BUBBLES_SECURITY_ENTRY_MODE"
      ;;
    *) return 1 ;;
  esac
  if [[ -n "${BASH_ENV+x}" || -n "${ENV+x}" || -n "${CDPATH+x}" ||
    -n "${GLOBIGNORE+x}" || -n "${PYTHONPATH+x}" || -n "${PYTHONHOME+x}" ||
    -n "${PYTHONSTARTUP+x}" || -n "${PERL5LIB+x}" || -n "${PERLLIB+x}" ||
    -n "${LD_PRELOAD+x}" || -n "${DYLD_INSERT_LIBRARIES+x}" ]]; then
    return 1
  fi
  for function_name in source builtin return exec /bin/bash /usr/bin/env /usr/bin/perl kill wait; do
    if declare -F "$function_name" >/dev/null 2>&1; then
      return 1
    fi
  done
  if ! _bubbles_python_security_authenticate_path /bin/bash executable 1 ||
    [[ "$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED" != /bin/bash ]]; then
    return 1
  fi
  if ! _bubbles_python_security_authenticate_path /usr/bin/env executable 1 ||
    [[ "$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED" != /usr/bin/env ]]; then
    return 1
  fi
  while IFS= read -r environment_line || [[ -n "$environment_line" ]]; do
    environment_name="${environment_line%%=*}"
    case "$environment_name" in
      BASH_FUNC_* | BASH_ENV | ENV | CDPATH | GLOBIGNORE | PYTHONPATH | PYTHONHOME | \
        PYTHONSTARTUP | PERL5LIB | PERLLIB | LD_PRELOAD | DYLD_INSERT_LIBRARIES)
        return 1
        ;;
    esac
  done < <(/usr/bin/env)

  # The complete BSEC1 result tuple is a sourced-module API consumed by the
  # privileged scanner after this function returns; it is intentionally not
  # exported into worker environments.
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_ENTRY_STATUS=0
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_ENTRY_DIAGNOSTIC='OK'
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_ENTRY_PROTOCOL='BSEC1'
  printf 'ENTRY\tBSEC1\tprivileged-bash-entry-v1\t%s\n' "$BUBBLES_PYTHON_SECURITY_ENTRY_MODE"
  return 0
}

_bubbles_python_security_record_signal() {
  [[ -z "$BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL" ]] || return 0
  BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL="$1"
  BUBBLES_PYTHON_SECURITY_PENDING_STATUS="$2"
  builtin trap '' HUP INT TERM
}

_bubbles_python_security_install_traps() {
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_EXIT="$(builtin trap -p EXIT)"
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_HUP="$(builtin trap -p HUP)"
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_INT="$(builtin trap -p INT)"
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_TERM="$(builtin trap -p TERM)"
  builtin trap '_bubbles_python_security_exit_trap' EXIT
  builtin trap '_bubbles_python_security_record_signal HUP 129' HUP
  builtin trap '_bubbles_python_security_record_signal INT 130' INT
  builtin trap '_bubbles_python_security_record_signal TERM 143' TERM
}

_bubbles_python_security_restore_trap() {
  local saved="$1"
  local signal_name="$2"
  builtin trap - "$signal_name"
  if [[ -n "$saved" ]]; then
    builtin eval "$saved"
  fi
}

# Idempotent file cleanup. It never signals or waits on a worker. A non-empty
# supervisor wait handle means the reap boundary has not completed, so cleanup
# refuses rather than racing the child relationship.
bubbles_python_security_cleanup() {
  local saved_exit="$BUBBLES_PYTHON_SECURITY_SAVED_TRAP_EXIT"
  local saved_hup="$BUBBLES_PYTHON_SECURITY_SAVED_TRAP_HUP"
  local saved_int="$BUBBLES_PYTHON_SECURITY_SAVED_TRAP_INT"
  local saved_term="$BUBBLES_PYTHON_SECURITY_SAVED_TRAP_TERM"
  local saved_umask="$BUBBLES_PYTHON_SECURITY_SAVED_UMASK"
  local private_root="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT"
  local cleanup_status=0

  if [[ "$BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE" -eq 1 ]]; then
    return "$BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS"
  fi
  if [[ -n "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" ]]; then
    BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS=1
    return 1
  fi
  if [[ "$BUBBLES_PYTHON_SECURITY_STATE" == IDLE &&
    -z "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" &&
    -z "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" &&
    -z "$BUBBLES_PYTHON_SECURITY_STDERR_PATH" &&
    -z "$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" ]]; then
    return "$BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS"
  fi
  BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE=1
  builtin trap - EXIT HUP INT TERM
  if [[ -n "$private_root" ]]; then
    case "$private_root" in
      /tmp/bubbles-python-security.*) /bin/rm -rf "$private_root" || cleanup_status=1 ;;
      *) cleanup_status=1 ;;
    esac
  fi
  if [[ -n "$saved_umask" && "$saved_umask" =~ ^[0-7]+$ ]]; then
    umask "$saved_umask" || cleanup_status=1
  fi

  BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT=''
  BUBBLES_PYTHON_SECURITY_STDOUT_PATH=''
  BUBBLES_PYTHON_SECURITY_STDERR_PATH=''
  BUBBLES_PYTHON_SECURITY_CONTROL_PATH=''
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''
  BUBBLES_PYTHON_SECURITY_STATE='IDLE'
  BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL=''
  BUBBLES_PYTHON_SECURITY_PENDING_STATUS=0
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_EXIT=''
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_HUP=''
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_INT=''
  BUBBLES_PYTHON_SECURITY_SAVED_TRAP_TERM=''
  BUBBLES_PYTHON_SECURITY_SAVED_UMASK=''
  BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS=$cleanup_status
  BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE=0

  _bubbles_python_security_restore_trap "$saved_hup" HUP || cleanup_status=1
  _bubbles_python_security_restore_trap "$saved_int" INT || cleanup_status=1
  _bubbles_python_security_restore_trap "$saved_term" TERM || cleanup_status=1
  _bubbles_python_security_restore_trap "$saved_exit" EXIT || cleanup_status=1
  BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS=$cleanup_status
  BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE=0
  return "$cleanup_status"
}

_bubbles_python_security_exit_trap() {
  local exit_status=$?
  local supervisor_wait_status=0
  if [[ "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" =~ ^[1-9][0-9]*$ ]]; then
    # A trapped signal can interrupt wait with a status above 128 without
    # reaping the supervisor. Retry the blocking wait until it returns a
    # definitive child status, or 127 after a signal-shaped child status was
    # already reaped. This retains the wait handle and private root without
    # adding PID probes or a polling loop.
    while true; do
      if builtin wait "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID" 2>/dev/null; then
        supervisor_wait_status=0
        break
      else
        supervisor_wait_status=$?
      fi
      [[ "$supervisor_wait_status" -gt 128 ]] || break
    done
    BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''
  fi
  bubbles_python_security_cleanup || true
  builtin exit "$exit_status"
}

_bubbles_python_security_runtime_for_operation() {
  local expected=""
  if [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == OK &&
    "$BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT" == root-protected-native-python-v1 &&
    "$BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL" == PYSEC1 &&
    "$BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL" == PYMOD1 ]]; then
    expected="$BUBBLES_PYTHON_SECURITY_RUNTIME"
  elif [[ "$BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE" -eq 1 ]]; then
    expected="$BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME"
  fi
  [[ -n "$expected" ]] || return 1
  if ! _bubbles_python_security_authenticate_path "$expected" executable 1; then
    return 1
  fi
  [[ "$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED" == "$expected" ]] || return 1
  return 0
}

_bubbles_python_security_scan_driver() {
  /bin/cat <<'PY'
import hashlib
import os
import sys
import types
from pathlib import Path

sys.dont_write_bytecode = True

def abort(token, status=125):
    sys.stderr.write("PYDRIVER1\t" + token + "\n")
    raise SystemExit(status)

helper_path = Path(sys.argv[1])
expected_digest = sys.argv[2]
maximum_bytes = int(sys.argv[3])
try:
    with helper_path.open("rb") as stream:
        helper_bytes = stream.read(maximum_bytes + 1)
except FileNotFoundError:
    abort("HELPER_MISSING")
except OSError:
    abort("HELPER_MISSING")
if len(helper_bytes) > maximum_bytes:
    abort("HELPER_TOO_LARGE")
if hashlib.sha256(helper_bytes).hexdigest() != expected_digest:
    abort("HELPER_DIGEST_MISMATCH")
try:
    helper_text = helper_bytes.decode("utf-8")
except UnicodeDecodeError:
    abort("HELPER_DECODE_INVALID")
try:
    helper_code = compile(helper_text, str(helper_path), "exec", dont_inherit=True)
except (SyntaxError, ValueError, TypeError):
    abort("HELPER_COMPILE_INVALID")

try:
    repository = Path(sys.argv[4]).resolve(strict=True)
    if not repository.is_dir():
        abort("DATA_PATH_INVALID")
    config_path = Path(sys.argv[5]).resolve(strict=False)
    config_path.relative_to(repository)
    source_paths = []
    for raw_path in sys.argv[6:]:
        source_path = Path(raw_path).resolve(strict=True)
        source_path.relative_to(repository)
        if not source_path.is_file():
            abort("DATA_PATH_INVALID")
        source_paths.append(source_path)
except (OSError, RuntimeError, ValueError):
    abort("DATA_PATH_INVALID")

module_name = "bubbles_sensitive_client_storage_scan"
module = types.ModuleType(module_name)
module.__file__ = str(helper_path)
module.__package__ = ""
sys.modules[module_name] = module
try:
    exec(helper_code, module.__dict__)
except BaseException:
    abort("HELPER_EXECUTION_FAILED")
if not callable(getattr(module, "parse_project_config", None)) or not callable(getattr(module, "analyze_file", None)):
    abort("HELPER_CONTRACT_INVALID")

try:
    approvals, _ = module.parse_project_config(config_path, repository)
except module.ConfigError as exc:
    module.Finding(
        path=os.path.relpath(config_path, repository).replace(os.sep, "/"),
        line=exc.line,
        reason="SENSITIVE_STORAGE_CONFIG_INVALID",
        storage="configuration",
        operation="parse",
        key="unresolved",
        provider="unresolved",
        config_match="invalid",
    ).emit()
    approvals = []
scanned = 0
for source_path in source_paths:
    for finding in module.analyze_file(source_path, repository, approvals):
        finding.emit()
    scanned += 1
print("COMPLETE\tSCS1\t%d" % scanned)
PY
}

_bubbles_python_security_module_probe() {
  /bin/cat <<'PY'
import ast, dataclasses, hashlib, os, pathlib, re, sys, types, typing

required = ("ast", "dataclasses", "hashlib", "os", "pathlib", "re", "sys", "types", "typing")
modules = []
for name, module in sorted(sys.modules.items()):
  if name == "__main__" or module is None:
    continue
  spec = getattr(module, "__spec__", None)
  if spec is None:
    continue
  origin = getattr(spec, "origin", None)
  if origin == "built-in":
    print("MODULE\tPYMOD1\t%s\tbuilt-in\t-" % name)
  elif origin == "frozen":
    print("MODULE\tPYMOD1\t%s\tfrozen\t-" % name)
  elif isinstance(origin, str):
    print("MODULE\tPYMOD1\t%s\tfile\t%s" % (name, origin))
  else:
    print("MODULE\tPYMOD1\t%s\tinvalid\t-" % name)
  modules.append(name)
if not all(name in modules for name in required):
  raise SystemExit(125)
print("COMPLETE\tPYMOD1\t%d" % len(modules))
PY
}

_bubbles_python_security_supervisor_program() {
  /bin/cat <<'PERL'
open(CONTROL, ">&=3") or exit 125;

sub emit_completion {
  my ($operation, $status, $owner, $timed_out, $worker_kind, $stdout_bytes, $stderr_bytes) = @_;
  my $record = join("\t", "COMPLETE", "BPS1", $operation, $status, $owner,
    $timed_out, $worker_kind, $stdout_bytes, $stderr_bytes) . "\n";
  return 0 if length($record) > 512;
  my $offset = 0;
  while ($offset < length($record)) {
    my $written = syswrite(CONTROL, $record, length($record) - $offset, $offset);
    return 0 if !defined($written) || $written <= 0;
    $offset += $written;
  }
  return 1;
}

sub finish_without_worker {
  my ($operation, $status) = @_;
  emit_completion($operation, $status, "supervisor", 0, "not-started", 0, 0);
  close(CONTROL);
  exit 0;
}

sub untaint_blob {
  my ($raw, $maximum) = @_;
  return undef if !defined($raw) || length($raw) > $maximum;
  return $1 if $raw =~ /\A([^\0]*)\z/s;
  return undef;
}

sub absolute_path {
  my ($value) = @_;
  return defined($value) && $value =~ m{\A/[^\0\r\n\t]{0,4095}\z};
}

my $raw_operation = shift @ARGV;
if (!defined($raw_operation) ||
  $raw_operation !~ /\A(general-probe|apple-select|apple-find-python|runtime-probe|module-probe|scan2b-classify)\z/) {
  finish_without_worker("invalid", 2);
}
my $operation = $1;
my @arguments = ();
my $aggregate_bytes = 0;
for my $raw (@ARGV) {
  my $value = untaint_blob($raw, 262144);
  finish_without_worker($operation, 2) if !defined($value);
  $aggregate_bytes += length($value);
  finish_without_worker($operation, 2) if $aggregate_bytes > 524288;
  push @arguments, $value;
}

my $stdout_limit = 0;
my $stderr_limit = 0;
my %worker_environment = ("LC_ALL" => "C");
my @worker_argv = ();

if ($operation eq "general-probe") {
  finish_without_worker($operation, 2) unless @arguments == 3 &&
    length($arguments[0]) > 0 && $arguments[1] eq "-c" &&
    $arguments[2] eq "import sys; sys.stdout.write('bubbles-python-runs')";
  @worker_argv = @arguments;
  $stdout_limit = 16384;
  $stderr_limit = 16384;
} elsif ($operation eq "apple-select") {
  finish_without_worker($operation, 2) unless @arguments == 2 &&
    $arguments[0] eq "/usr/bin/xcode-select" && $arguments[1] eq "-p";
  @worker_argv = @arguments;
  $stdout_limit = 4096;
  $stderr_limit = 16384;
} elsif ($operation eq "apple-find-python") {
  finish_without_worker($operation, 2) unless @arguments == 4 &&
    absolute_path($arguments[0]) && $arguments[1] eq "/usr/bin/xcrun" &&
    $arguments[2] eq "--find" && $arguments[3] eq "python3";
  $worker_environment{"DEVELOPER_DIR"} = $arguments[0];
  @worker_argv = @arguments[1 .. 3];
  $stdout_limit = 4096;
  $stderr_limit = 16384;
} elsif ($operation eq "runtime-probe") {
  finish_without_worker($operation, 2) unless @arguments == 6 &&
    absolute_path($arguments[0]) && $arguments[1] eq "-I" &&
    $arguments[2] eq "-S" && $arguments[3] eq "-B" &&
    $arguments[4] eq "-c" && $arguments[5] =~ /RUNTIME\\tPYSEC1/ &&
    $arguments[5] =~ /COMPLETE\\tPYSEC1/;
  @worker_argv = @arguments;
  $stdout_limit = 16384;
  $stderr_limit = 16384;
} elsif ($operation eq "module-probe") {
  finish_without_worker($operation, 2) unless @arguments == 6 &&
    absolute_path($arguments[0]) && $arguments[1] eq "-I" &&
    $arguments[2] eq "-S" && $arguments[3] eq "-B" &&
    $arguments[4] eq "-c" && $arguments[5] =~ /MODULE\\tPYMOD1/ &&
    $arguments[5] =~ /COMPLETE\\tPYMOD1/;
  @worker_argv = @arguments;
  $stdout_limit = 65536;
  $stderr_limit = 16384;
} else {
  finish_without_worker($operation, 2) unless @arguments >= 12 &&
    absolute_path($arguments[0]) && $arguments[1] eq "-I" &&
    $arguments[2] eq "-S" && $arguments[3] eq "-B" &&
    $arguments[4] eq "-c" &&
    absolute_path($arguments[6]) &&
    $arguments[6] =~ m{/bubbles/scripts/guards/sensitive-client-storage-scan[.]py\z} &&
    $arguments[7] eq "77a02ff179d529812d75cfa223bef5f9f171a9169dce050ab46fb2f1f0834df3" &&
    $arguments[8] eq "262144" && absolute_path($arguments[9]) &&
    absolute_path($arguments[10]) && @arguments <= 4107;
  my $path_bytes = 0;
  for my $index (9 .. $#arguments) {
    finish_without_worker($operation, 2) unless absolute_path($arguments[$index]);
    $path_bytes += length($arguments[$index]);
  }
  finish_without_worker($operation, 2) if $path_bytes > 65536;
  @worker_argv = @arguments;
  $stdout_limit = 4194304;
  $stderr_limit = 65536;
}

my ($stdout_reader, $stdout_writer, $stderr_reader, $stderr_writer);
finish_without_worker($operation, 125) unless pipe($stdout_reader, $stdout_writer);
finish_without_worker($operation, 125) unless pipe($stderr_reader, $stderr_writer);

my $worker_pid = fork();
finish_without_worker($operation, 125) unless defined($worker_pid);
if ($worker_pid == 0) {
  close($stdout_reader);
  close($stderr_reader);
  close(CONTROL); # close worker BPS control descriptor before exec
  open(STDIN, "<", "/dev/null") or exit 126;
  open(STDOUT, ">&", $stdout_writer) or exit 126;
  open(STDERR, ">&", $stderr_writer) or exit 126;
  close($stdout_writer);
  close($stderr_writer);
  $SIG{"HUP"} = "DEFAULT";
  $SIG{"INT"} = "DEFAULT";
  $SIG{"TERM"} = "DEFAULT";
  $SIG{"PIPE"} = "DEFAULT";
  %ENV = %worker_environment;
  exec { $worker_argv[0] } @worker_argv or do {
    print STDERR "WORKER_EXEC_FAILED\n";
    exit 126;
  };
}

close($stdout_writer);
close($stderr_writer);
my $stdout_open = 1;
my $stderr_open = 1;
my $stdout_bytes = 0;
my $stderr_bytes = 0;
my $output_limit_reached = 0;
my $stream_failure = 0;

sub write_stdout {
  my ($data) = @_;
  my $offset = 0;
  while ($offset < length($data)) {
    my $written = syswrite(STDOUT, $data, length($data) - $offset, $offset);
    return 0 if !defined($written) || $written <= 0;
    $offset += $written;
  }
  return 1;
}

sub write_stderr {
  my ($data) = @_;
  my $offset = 0;
  while ($offset < length($data)) {
    my $written = syswrite(STDERR, $data, length($data) - $offset, $offset);
    return 0 if !defined($written) || $written <= 0;
    $offset += $written;
  }
  return 1;
}

my $consume_ready_stream = sub {
  my ($stream_name, $stream_handle) = @_;
  my $chunk = "";
  my $read_count = sysread($stream_handle, $chunk, 8192);
  return if !defined($read_count);
  if ($read_count == 0) { # Pipe EOF is data state only, never completion authority.
    if ($stream_name eq "stdout") {
      $stdout_open = 0;
    } else {
      $stderr_open = 0;
    }
    close($stream_handle);
    return;
  }
  if ($stream_name eq "stdout") {
    my $remaining = $stdout_limit - $stdout_bytes;
    my $captured = $read_count <= $remaining ? $read_count : $remaining;
    if ($captured > 0) {
      $stream_failure = 1 unless write_stdout(substr($chunk, 0, $captured));
      $stdout_bytes += $captured;
    }
    $output_limit_reached = 1 if $read_count > $remaining;
  } else {
    my $remaining = $stderr_limit - $stderr_bytes;
    my $captured = $read_count <= $remaining ? $read_count : $remaining;
    if ($captured > 0) {
      $stream_failure = 1 unless write_stderr(substr($chunk, 0, $captured));
      $stderr_bytes += $captured;
    }
    $output_limit_reached = 1 if $read_count > $remaining;
  }
};

my $worker_is_owned = 1;
my $raw_wait_status = 0;
my $wait_failure = 0;
my $pending_signal = "";
my $pending_signal_status = 0;
my $wall_expired = 0;
my $grace_expired = 0;
my $alarm_phase = "wall";
my $termination_reason = "";
my $term_sent = 0;
my $kill_sent = 0;

$SIG{"HUP"} = sub { if ($pending_signal eq "") { $pending_signal = "HUP"; $pending_signal_status = 129; } };
$SIG{"INT"} = sub { if ($pending_signal eq "") { $pending_signal = "INT"; $pending_signal_status = 130; } };
$SIG{"TERM"} = sub { if ($pending_signal eq "") { $pending_signal = "TERM"; $pending_signal_status = 143; } };
$SIG{"ALRM"} = sub {
  if ($alarm_phase eq "wall") { $wall_expired = 1; }
  else { $grace_expired = 1; }
};
$SIG{"PIPE"} = "IGNORE";

my $signal_owned_worker = sub {
  my ($signal_number) = @_;
  return 0 unless $worker_is_owned;
  return kill($signal_number, $worker_pid); # signal only the unreaped direct worker
};

alarm(30);
while ($worker_is_owned) {
  my $waited_pid = waitpid($worker_pid, 1);
  if ($waited_pid == $worker_pid) {
    $raw_wait_status = $?;
    $worker_is_owned = 0;
    last;
  }
  if ($waited_pid == -1) {
    $wait_failure = 1;
    $worker_is_owned = 0;
    last;
  }

  if ($termination_reason eq "") {
    if ($pending_signal ne "") {
      $termination_reason = "caller-signal";
    } elsif ($wall_expired) {
      $termination_reason = "timeout";
    } elsif ($output_limit_reached) {
      $termination_reason = "output-limit";
    } elsif ($stream_failure) {
      $termination_reason = "internal";
    }
  }
  if ($termination_reason ne "" && !$term_sent) {
    $signal_owned_worker->(15);
    $term_sent = 1;
    $alarm_phase = "grace";
    $grace_expired = 0;
    alarm(2);
  } elsif ($term_sent && $grace_expired && !$kill_sent) {
    $signal_owned_worker->(9);
    $kill_sent = 1;
  }

  my $read_mask = "";
  vec($read_mask, fileno($stdout_reader), 1) = 1 if $stdout_open;
  vec($read_mask, fileno($stderr_reader), 1) = 1 if $stderr_open;
  if ($stdout_open || $stderr_open) {
    my $ready_mask = $read_mask;
    my $selected = select($ready_mask, undef, undef, 0.05);
    if (defined($selected) && $selected > 0) {
      $consume_ready_stream->("stdout", $stdout_reader)
        if $stdout_open && vec($ready_mask, fileno($stdout_reader), 1);
      $consume_ready_stream->("stderr", $stderr_reader)
        if $stderr_open && vec($ready_mask, fileno($stderr_reader), 1);
    }
  } else {
    select(undef, undef, undef, 0.05);
  }
}
alarm(0);

# Drain only bytes already ready after reap. Descendant-held descriptors and
# later worker text cannot delay or authorize completion.
for my $drain_round (1 .. 600) {
  last unless $stdout_open || $stderr_open;
  my $read_mask = "";
  vec($read_mask, fileno($stdout_reader), 1) = 1 if $stdout_open;
  vec($read_mask, fileno($stderr_reader), 1) = 1 if $stderr_open;
  my $ready_mask = $read_mask;
  my $selected = select($ready_mask, undef, undef, 0);
  last if !defined($selected) || $selected <= 0;
  $consume_ready_stream->("stdout", $stdout_reader)
    if $stdout_open && vec($ready_mask, fileno($stdout_reader), 1);
  $consume_ready_stream->("stderr", $stderr_reader)
    if $stderr_open && vec($ready_mask, fileno($stderr_reader), 1);
}
close($stdout_reader) if $stdout_open;
close($stderr_reader) if $stderr_open;

if ($termination_reason eq "" && $pending_signal ne "") {
  $termination_reason = "caller-signal";
}
if ($termination_reason eq "" && $output_limit_reached) {
  $termination_reason = "output-limit";
}
if ($termination_reason eq "" && ($stream_failure || $wait_failure)) {
  $termination_reason = "internal";
}

my $final_status = 0;
my $final_owner = "worker";
my $timed_out = 0;
my $worker_kind = "exit";
if ($termination_reason eq "caller-signal") {
  $final_status = $pending_signal_status;
  $final_owner = "caller-signal";
  $worker_kind = ($raw_wait_status & 127) ? "signal" : "exit";
} elsif ($termination_reason eq "timeout") {
  $final_status = 124;
  $final_owner = "supervisor";
  $timed_out = 1;
  $worker_kind = ($raw_wait_status & 127) ? "signal" : "exit";
} elsif ($termination_reason eq "output-limit" || $termination_reason eq "internal") {
  $final_status = 125;
  $final_owner = "supervisor";
  $worker_kind = ($raw_wait_status & 127) ? "signal" : "exit";
} elsif ($raw_wait_status & 127) {
  $final_status = 128 + ($raw_wait_status & 127);
  $worker_kind = "signal";
} else {
  $final_status = ($raw_wait_status >> 8) & 255;
}

my $completion_anchor = "COMPLETE\tBPS1";
if ($completion_anchor ne join("\t", "COMPLETE", "BPS1")) {
  $final_status = 125;
  $final_owner = "supervisor";
}
emit_completion($operation, $final_status, $final_owner, $timed_out,
  $worker_kind, $stdout_bytes, $stderr_bytes) or exit 125;
close(CONTROL);
exit 0;
PERL
}

_bubbles_python_security_validate_supervisor_protocol() {
  local path="$1"
  local expected_operation="$2"
  local stdout_limit="$3"
  local stderr_limit="$4"
  local record="" record_type="" protocol="" operation="" status=""
  local owner="" timed_out="" worker_kind="" stdout_bytes="" stderr_bytes="" extra=""
  local byte_count="" line_count=0

  BUBBLES_PYTHON_SECURITY_SUPERVISOR_PROTOCOL='none'
  [[ -f "$path" ]] || return 1
  byte_count="$(/usr/bin/wc -c <"$path" 2>/dev/null)" || return 1
  byte_count="${byte_count//[[:space:]]/}"
  [[ "$byte_count" =~ ^[1-9][0-9]*$ && "$byte_count" -le 512 ]] || return 1
  while IFS= read -r record || [[ -n "$record" ]]; do
    line_count=$((line_count + 1))
  done <"$path"
  [[ "$line_count" -eq 1 ]] || return 1
  record="$(/bin/cat "$path")" || return 1
  IFS=$'\t' read -r record_type protocol operation status owner timed_out \
    worker_kind stdout_bytes stderr_bytes extra <<<"$record"
  [[ -z "$extra" && "$record_type" == COMPLETE && "$protocol" == BPS1 &&
    "$operation" == "$expected_operation" && "$status" =~ ^[0-9]+$ && "$status" -le 255 &&
    "$timed_out" =~ ^[01]$ && "$stdout_bytes" =~ ^[0-9]+$ &&
    "$stderr_bytes" =~ ^[0-9]+$ && "$stdout_bytes" -le "$stdout_limit" &&
    "$stderr_bytes" -le "$stderr_limit" ]] || return 1
  case "$owner" in worker | supervisor | caller-signal) ;; *) return 1 ;; esac
  case "$worker_kind" in exit | signal | not-started) ;; *) return 1 ;; esac

  BUBBLES_PYTHON_SECURITY_SUPERVISOR_PROTOCOL='BPS1'
  BUBBLES_PYTHON_SECURITY_RUN_STATUS=$status
  BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT=$timed_out
  BUBBLES_PYTHON_SECURITY_RUN_OWNER="$owner"
  BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND="$worker_kind"
  BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES=$stdout_bytes
  BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES=$stderr_bytes
  case "$owner:$status:$timed_out:$worker_kind" in
    worker:0:0:exit) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='OK' ;;
    worker:126:0:exit) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='WORKER_EXEC_FAILED' ;;
    worker:*:*:signal) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='WORKER_SIGNAL' ;;
    worker:*:*:*) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='WORKER_EXIT_NONZERO' ;;
    supervisor:2:0:not-started) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID' ;;
    supervisor:124:1:*) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_TIMEOUT' ;;
    supervisor:125:0:not-started) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_SETUP_FAILED' ;;
    supervisor:125:0:*)
      if [[ "$stdout_bytes" -eq "$stdout_limit" || "$stderr_bytes" -eq "$stderr_limit" ]]; then
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='OUTPUT_LIMIT'
      else
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'
      fi
      ;;
    caller-signal:129:*:*) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SIGNAL_HUP' ;;
    caller-signal:130:*:*) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SIGNAL_INT' ;;
    caller-signal:143:*:*) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SIGNAL_TERM' ;;
    *) BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_PROTOCOL_INVALID'; return 1 ;;
  esac
  return 0
}

# Internal fixed-operation dispatcher. It accepts a closed operation enum, not
# an executable vector. Only general-probe accepts an interpreter path, and that
# operation is never exposed by the security API below.
_bubbles_python_run_closed_operation() {
  local operation="${1:-}"
  local stdout_limit=0 stderr_limit=0
  local runtime="" developer_dir="" module_source="" module_dir="" helper_path=""
  local runtime_program="" module_program="" scan_driver=""
  local supervisor_program="" supervisor_status=0 wait_status=0 old_umask=""
  local -a security_command_args=()
  shift 2>/dev/null || true

  BUBBLES_PYTHON_SECURITY_RUN_STATUS=125
  BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='NOT_RUN'
  BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT=0
  # Sourced callers inspect the operation associated with the run result.
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_RUN_OPERATION="$operation"
  BUBBLES_PYTHON_SECURITY_RUN_OWNER='supervisor'
  # These native-supervisor result fields form a sourced-module API. They stay
  # in the caller shell and are not exported to the supervisor or worker.
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_RUN_WORKER_KIND='not-started'
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_RUN_STDOUT_BYTES=0
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_RUN_STDERR_BYTES=0
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_CONTRACT='none'
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_PROTOCOL='none'
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_REJECTION='NONE'
  BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL=''
  BUBBLES_PYTHON_SECURITY_PENDING_STATUS=0
  BUBBLES_PYTHON_SECURITY_CLEANUP_ACTIVE=0
  BUBBLES_PYTHON_SECURITY_CLEANUP_STATUS=0

  case "$operation" in
    general-probe)
      [[ $# -eq 1 && -n "${1:-}" && -x "$1" ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      security_command_args=("$1" -c "import sys; sys.stdout.write('$BUBBLES_PYTHON_RUN_SENTINEL')")
      stdout_limit=16384
      stderr_limit=16384
      ;;
    apple-select)
      [[ $# -eq 0 ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      _bubbles_python_security_authenticate_path /usr/bin/env executable 1 || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      _bubbles_python_security_authenticate_path /usr/bin/xcode-select executable 1 || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      security_command_args=(/usr/bin/xcode-select -p)
      stdout_limit=4096
      stderr_limit=16384
      ;;
    apple-find-python)
      [[ $# -eq 1 ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      developer_dir="$1"
      _bubbles_python_security_authenticate_path /usr/bin/env executable 1 || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      _bubbles_python_security_authenticate_path /usr/bin/xcrun executable 1 || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      _bubbles_python_security_authenticate_path "$developer_dir" directory 0 || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      developer_dir="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      security_command_args=("$developer_dir" /usr/bin/xcrun --find python3)
      stdout_limit=4096
      stderr_limit=16384
      ;;
    runtime-probe)
      [[ $# -eq 0 ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      _bubbles_python_security_runtime_for_operation || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      runtime="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      runtime_program='import sys
print("RUNTIME\tPYSEC1\t%d\t%d" % (sys.version_info[0], sys.version_info[1]))
print("FLAGS\tPYSEC1\t%d\t%d\t%d\t%d" % (sys.flags.isolated, sys.flags.no_site, sys.flags.ignore_environment, sys.flags.dont_write_bytecode))
print("EXECUTABLE\tPYSEC1\t" + sys.executable)
print("PREFIX\tPYSEC1\tbase\t" + sys.base_prefix)
print("PREFIX\tPYSEC1\texec\t" + sys.exec_prefix)
for value in sys.path:
    print("PATH\tPYSEC1\t" + value)
print("COMPLETE\tPYSEC1\t%d" % len(sys.path))'
      security_command_args=("$runtime" -I -S -B -c "$runtime_program")
      stdout_limit=16384
      stderr_limit=16384
      ;;
    module-probe)
      [[ $# -eq 0 ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      _bubbles_python_security_runtime_for_operation || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      runtime="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      module_program="$(_bubbles_python_security_module_probe)" || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      security_command_args=("$runtime" -I -S -B -c "$module_program")
      stdout_limit=65536
      stderr_limit=16384
      ;;
    scan2b-classify)
      [[ $# -ge 3 ]] || {
        BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
        return 2
      }
      _bubbles_python_security_runtime_for_operation || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      runtime="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      for developer_dir in "$@"; do
        if ! _bubbles_python_security_path_text_safe "$developer_dir" || [[ "$developer_dir" != /* ]]; then
          BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
          BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
          return 2
        fi
      done
      module_source="${BASH_SOURCE[0]}"
      module_dir="${module_source%/*}"
      [[ "$module_dir" != "$module_source" ]] || module_dir='.'
      module_dir="$(cd "$module_dir" 2>/dev/null && pwd -P)" || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      helper_path="$module_dir/guards/sensitive-client-storage-scan.py"
      scan_driver="$(_bubbles_python_security_scan_driver)" || {
        BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'; return 125;
      }
      security_command_args=("$runtime" -I -S -B -c "$scan_driver" \
        "$helper_path" 77a02ff179d529812d75cfa223bef5f9f171a9169dce050ab46fb2f1f0834df3 262144 "$@")
      stdout_limit=4194304
      stderr_limit=65536
      ;;
    *)
      BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
      BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
      return 2
      ;;
  esac

  if ! _bubbles_python_security_authenticate_path /usr/bin/env executable 1 ||
    [[ "$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED" != /usr/bin/env ]]; then
    # shellcheck disable=SC2034  # sourced-module rejection detail
    BUBBLES_PYTHON_SECURITY_SUPERVISOR_REJECTION="$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
    BUBBLES_PYTHON_SECURITY_RUN_STATUS=127
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_UNTRUSTED'
    return 127
  fi
  if ! _bubbles_python_security_authenticate_path /usr/bin/perl executable 1 ||
    [[ "$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED" != /usr/bin/perl ]]; then
    # shellcheck disable=SC2034  # sourced-module rejection detail
    BUBBLES_PYTHON_SECURITY_SUPERVISOR_REJECTION="$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
    BUBBLES_PYTHON_SECURITY_RUN_STATUS=127
    if [[ "$BUBBLES_PYTHON_SECURITY_PATH_REJECTION" == ABSENT ]]; then
      BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_UNAVAILABLE'
    else
      BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_UNTRUSTED'
    fi
    return 127
  fi
  # shellcheck disable=SC2034  # sourced-module supervisor identity
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_CONTRACT='root-protected-perl-supervisor-v1'
  supervisor_program="$(_bubbles_python_security_supervisor_program)" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='INTERNAL'
    return 125
  }

  old_umask="$(umask)"
  BUBBLES_PYTHON_SECURITY_SAVED_UMASK="$old_umask"
  _bubbles_python_security_install_traps
  umask 077
  BUBBLES_PYTHON_SECURITY_STATE='SETUP'
  BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT="$(/usr/bin/mktemp -d /tmp/bubbles-python-security.XXXXXXXX 2>/dev/null)" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'
    bubbles_python_security_cleanup || true
    return 125
  }
  /bin/chmod 700 "$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'
    bubbles_python_security_cleanup || true
    return 125
  }
  BUBBLES_PYTHON_SECURITY_STDOUT_PATH="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT/stdout.capture"
  BUBBLES_PYTHON_SECURITY_STDERR_PATH="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT/stderr.capture"
  BUBBLES_PYTHON_SECURITY_CONTROL_PATH="$BUBBLES_PYTHON_SECURITY_PRIVATE_ROOT/supervisor.control"
  : >"$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'; bubbles_python_security_cleanup || true; return 125;
  }
  : >"$BUBBLES_PYTHON_SECURITY_STDERR_PATH" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'; bubbles_python_security_cleanup || true; return 125;
  }
  : >"$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'; bubbles_python_security_cleanup || true; return 125;
  }
  /bin/chmod 600 "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" \
    "$BUBBLES_PYTHON_SECURITY_STDERR_PATH" "$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" || {
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'; bubbles_python_security_cleanup || true; return 125;
  }

  BUBBLES_PYTHON_SECURITY_STATE='SUPERVISOR_LAUNCHING'
  /usr/bin/env -i LC_ALL=C /usr/bin/perl -T -w -e "$supervisor_program" \
    "$operation" "${security_command_args[@]}" \
    >"$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" \
    2>"$BUBBLES_PYTHON_SECURITY_STDERR_PATH" \
    3>"$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" </dev/null &
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=$!
  BUBBLES_PYTHON_SECURITY_STATE='SUPERVISOR_WAITING'

  while :; do
    if builtin wait "$BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID"; then
      supervisor_status=0
      break
    fi
    wait_status=$?
    if [[ -n "$BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL" &&
      "$wait_status" -eq "$BUBBLES_PYTHON_SECURITY_PENDING_STATUS" ]]; then
      continue
    fi
    supervisor_status=$wait_status
    break
  done
  BUBBLES_PYTHON_SECURITY_SUPERVISOR_WAIT_PID=''
  BUBBLES_PYTHON_SECURITY_STATE='SUPERVISOR_REAPED'

  if [[ "$supervisor_status" -ne 0 ]] ||
    ! _bubbles_python_security_validate_supervisor_protocol \
      "$BUBBLES_PYTHON_SECURITY_CONTROL_PATH" "$operation" "$stdout_limit" "$stderr_limit"; then
    BUBBLES_PYTHON_SECURITY_RUN_STATUS=125
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='SUPERVISOR_PROTOCOL_INVALID'
    bubbles_python_security_cleanup || true
    return 125
  fi
  if [[ -n "$BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL" ]]; then
    BUBBLES_PYTHON_SECURITY_RUN_STATUS=$BUBBLES_PYTHON_SECURITY_PENDING_STATUS
    # shellcheck disable=SC2034  # sourced-module completion tuple
    BUBBLES_PYTHON_SECURITY_RUN_TIMED_OUT=0
    # shellcheck disable=SC2034  # sourced-module completion tuple
    BUBBLES_PYTHON_SECURITY_RUN_OWNER='caller-signal'
    BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC="SIGNAL_$BUBBLES_PYTHON_SECURITY_PENDING_SIGNAL"
    wait_status=$BUBBLES_PYTHON_SECURITY_PENDING_STATUS
    bubbles_python_security_cleanup || true
    return "$wait_status"
  fi
  return "$BUBBLES_PYTHON_SECURITY_RUN_STATUS"
}

# Security API: closed operations only. No executable, module, helper path,
# Python program, timeout, output limit, or generic command vector is accepted.
bubbles_python_run_security_operation() {
  local operation="${1:-}"
  case "$operation" in
    apple-select | apple-find-python | runtime-probe | module-probe | scan2b-classify) ;;
    *)
      # Sourced callers inspect the closed-operation rejection status.
      # shellcheck disable=SC2034
      BUBBLES_PYTHON_SECURITY_RUN_STATUS=2
      BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC='ARGUMENT_INVALID'
      return 2
      ;;
  esac
  _bubbles_python_run_closed_operation "$@"
}

_bubbles_python_security_capture() {
  local path="${1:-}"
  local maximum="${2:-0}"
  local bytes=""
  BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT=''
  BUBBLES_PYTHON_SECURITY_CAPTURE_BYTES=0
  [[ -f "$path" && "$maximum" =~ ^[1-9][0-9]*$ ]] || return 1
  bytes="$(/usr/bin/wc -c <"$path" 2>/dev/null)" || return 1
  bytes="${bytes//[[:space:]]/}"
  [[ "$bytes" =~ ^[0-9]+$ && "$bytes" -le "$maximum" ]] || return 1
  BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT="$(/bin/cat "$path")" || return 1
  BUBBLES_PYTHON_SECURITY_CAPTURE_BYTES=$bytes
  return 0
}

_bubbles_python_security_single_line_capture() {
  local path="${1:-}"
  local maximum="${2:-0}"
  local line="" value="" count=0
  _bubbles_python_security_capture "$path" "$maximum" || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    count=$((count + 1))
    value="$line"
  done <"$path"
  [[ "$count" -eq 1 ]] || return 1
  _bubbles_python_security_path_text_safe "$value" || return 1
  BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT="$value"
  return 0
}

bubbles_python_runs() {
  local py="${1:-}"
  local run_status=0 probe="" stdout_path="" stderr_path=""
  BUBBLES_PYTHON_RUN_STATUS=127
  BUBBLES_PYTHON_RUN_DIAGNOSTIC='INTERPRETER_ABSENT'
  [[ -n "$py" && -x "$py" ]] || return 1

  if _bubbles_python_run_closed_operation general-probe "$py"; then
    run_status=0
  else
    run_status=$?
  fi
  stdout_path="$BUBBLES_PYTHON_SECURITY_STDOUT_PATH"
  stderr_path="$BUBBLES_PYTHON_SECURITY_STDERR_PATH"
  BUBBLES_PYTHON_RUN_STATUS=$run_status
  case "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" in
    SUPERVISOR_TIMEOUT)
      BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_TIMEOUT'
      ;;
    OUTPUT_LIMIT)
      BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_OUTPUT_LIMIT'
      ;;
    CAPTURE_UNAVAILABLE | SUPERVISOR_UNAVAILABLE | SUPERVISOR_UNTRUSTED | \
      SUPERVISOR_SETUP_FAILED | SUPERVISOR_PROTOCOL_INVALID | INTERNAL | ARGUMENT_INVALID)
      BUBBLES_PYTHON_RUN_STATUS=125
      BUBBLES_PYTHON_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'
      ;;
    WORKER_EXIT_NONZERO | WORKER_EXEC_FAILED | WORKER_SIGNAL)
      if [[ -f "$stderr_path" ]] && /usr/bin/grep -Eiq 'Xcode (license|licence)|license agreements' "$stderr_path" 2>/dev/null; then
        BUBBLES_PYTHON_RUN_DIAGNOSTIC='XCODE_LICENSE_UNACCEPTED'
      else
        BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_EXIT_NONZERO'
      fi
      ;;
    OK)
      if ! _bubbles_python_security_capture "$stdout_path" 16384; then
        BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_OUTPUT_LIMIT'
      else
        probe="$BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT"
        if [[ "$BUBBLES_PYTHON_SECURITY_CAPTURE_BYTES" -eq 0 ]]; then
          BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_EMPTY'
        elif [[ "$BUBBLES_PYTHON_SECURITY_CAPTURE_BYTES" -eq "${#BUBBLES_PYTHON_RUN_SENTINEL}" &&
          "$probe" == "$BUBBLES_PYTHON_RUN_SENTINEL" ]]; then
          BUBBLES_PYTHON_RUN_DIAGNOSTIC='OK'
        else
          BUBBLES_PYTHON_RUN_DIAGNOSTIC='PROBE_PROTOCOL_INVALID'
        fi
      fi
      ;;
    SIGNAL_HUP | SIGNAL_INT | SIGNAL_TERM)
      BUBBLES_PYTHON_RUN_DIAGNOSTIC="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC"
      ;;
    *) BUBBLES_PYTHON_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE' ;;
  esac
  if ! bubbles_python_security_cleanup; then
    # Sourced callers inspect the general probe's final status.
    # shellcheck disable=SC2034
    BUBBLES_PYTHON_RUN_STATUS=125
    BUBBLES_PYTHON_RUN_DIAGNOSTIC='CAPTURE_UNAVAILABLE'
  fi
  [[ "$BUBBLES_PYTHON_RUN_DIAGNOSTIC" == OK ]]
}

# bubbles_python_resolve_runnable — the SAME ordered, non-ambient contract as
# bubbles_python_resolve, for consumers whose helper needs only a working
# stdlib interpreter.
#
# Why a second resolver rather than reusing the first: the required-module set
# (yaml, jsonschema) belongs to the guards that parse YAML, not to every python
# consumer. A helper that imports nothing outside the stdlib would be refused an
# interpreter that runs it perfectly well, which trades one wrong answer for
# another. What every consumer DOES share is the part `command -v python3` gets
# wrong: presence is not usability, and PATH is not a stable identity.
#
# On failure BUBBLES_PYTHON_RUNNABLE_REASON names each candidate and how it
# declined, so the caller can report the absent prerequisite instead of
# inventing a finding about the code it was asked to inspect. Both the reason
# and the resolved path are published as GLOBALS as well as printed: a caller
# that reads the path with `$(...)` runs this in a subshell, where an assignment
# to the reason is discarded and the diagnostic arrives empty — which is the
# same lost-cause failure this function exists to prevent. Read the globals
# after calling it directly:
#
#   if bubbles_python_resolve_runnable >/dev/null; then
#     py="$BUBBLES_PYTHON_RUNNABLE"
#   else
#     echo "unavailable: $BUBBLES_PYTHON_RUNNABLE_REASON"
#   fi
BUBBLES_PYTHON_RUNNABLE=''
BUBBLES_PYTHON_RUNNABLE_REASON=''

bubbles_python_resolve_runnable() {
  local candidate declined=''
  BUBBLES_PYTHON_RUNNABLE=''
  BUBBLES_PYTHON_RUNNABLE_REASON=''

  if [[ -n "${BUBBLES_PYTHON:-}" ]]; then
    if bubbles_python_runs "$BUBBLES_PYTHON"; then
      BUBBLES_PYTHON_RUNNABLE="$BUBBLES_PYTHON"
      printf '%s\n' "$BUBBLES_PYTHON"
      return 0
    fi
    declined="${declined}BUBBLES_PYTHON ($BUBBLES_PYTHON) does not execute; "
  fi

  if candidate="$(bubbles_python_venv_python)"; then
    if bubbles_python_runs "$candidate"; then
      BUBBLES_PYTHON_RUNNABLE="$candidate"
      printf '%s\n' "$candidate"
      return 0
    fi
    declined="${declined}the managed venv ($candidate) is absent or does not execute; "
  else
    declined="${declined}the managed venv has no locator ($BUBBLES_PYTHON_LOCATOR_VARS are all unset); "
  fi

  candidate="$(command -v python3 2>/dev/null || true)"
  if [[ -n "$candidate" ]]; then
    if bubbles_python_runs "$candidate"; then
      # shellcheck disable=SC2034  # cross-file output; read by implementation-reality-scan.sh
      BUBBLES_PYTHON_RUNNABLE="$candidate"
      printf '%s\n' "$candidate"
      return 0
    fi
    declined="${declined}python3 on PATH ($candidate) resolves but exits without running; "
  else
    declined="${declined}no python3 on PATH; "
  fi

  # shellcheck disable=SC2034  # cross-file output; read by implementation-reality-scan.sh
  BUBBLES_PYTHON_RUNNABLE_REASON="${declined%; }"
  return 1
}

_bubbles_python_security_validate_runtime_protocol() {
  local path="${1:-}"
  local expected_runtime="${2:-}"
  local line="" kind="" version="" major="" minor="" extra=""
  local isolated="" no_site="" ignore_environment="" dont_write_bytecode=""
  local label="" value="" resolved="" seen_paths='|' path_count=0 declared_count=""
  local line_number=0 complete=0

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    kind="${line%%$'\t'*}"
    case "$line_number:$kind" in
      1:RUNTIME)
        IFS=$'\t' read -r kind version major minor extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 && "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ ]] || return 1
        if [[ "$major" -lt 3 || "$major" -eq 3 && "$minor" -lt 9 ]]; then
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PYTHON_VERSION_UNSUPPORTED'
          return 2
        fi
        ;;
      2:FLAGS)
        IFS=$'\t' read -r kind version isolated no_site ignore_environment dont_write_bytecode extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 && "$isolated" == 1 && "$no_site" == 1 &&
          "$ignore_environment" == 1 && "$dont_write_bytecode" == 1 ]] || return 1
        ;;
      3:EXECUTABLE)
        IFS=$'\t' read -r kind version value extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 ]] || return 1
        _bubbles_python_security_authenticate_path "$value" executable 1 || {
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='RUNTIME_CLOSURE_UNTRUSTED'; return 2;
        }
        resolved="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
        [[ "$resolved" == "$expected_runtime" ]] || {
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='RUNTIME_CLOSURE_UNTRUSTED'; return 2;
        }
        ;;
      4:PREFIX | 5:PREFIX)
        IFS=$'\t' read -r kind version label value extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 ]] || return 1
        if [[ "$line_number" -eq 4 ]]; then [[ "$label" == base ]] || return 1; else [[ "$label" == exec ]] || return 1; fi
        _bubbles_python_security_authenticate_path "$value" directory 0 || {
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='RUNTIME_CLOSURE_UNTRUSTED'; return 2;
        }
        ;;
      *:PATH)
        [[ "$complete" -eq 0 && "$line_number" -gt 5 ]] || return 1
        IFS=$'\t' read -r kind version value extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 && -n "$value" && "$seen_paths" != *"|$value|"* ]] || return 1
        _bubbles_python_security_validate_search_root "$value" || {
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='RUNTIME_CLOSURE_UNTRUSTED'; return 2;
        }
        seen_paths="$seen_paths$value|"
        path_count=$((path_count + 1))
        ;;
      *:COMPLETE)
        [[ "$complete" -eq 0 && "$line_number" -gt 5 ]] || return 1
        IFS=$'\t' read -r kind version declared_count extra <<<"$line"
        [[ -z "$extra" && "$version" == PYSEC1 && "$declared_count" =~ ^[0-9]+$ &&
          "$declared_count" -eq "$path_count" ]] || return 1
        complete=1
        ;;
      *) return 1 ;;
    esac
    if [[ "$complete" -eq 1 && "$kind" != COMPLETE ]]; then
      return 1
    fi
  done <"$path"
  [[ "$complete" -eq 1 ]] || return 1
  return 0
}

_bubbles_python_security_validate_module_protocol() {
  local path="${1:-}"
  local line="" kind="" version="" name="" origin_kind="" origin="" extra=""
  local declared_count="" count=0 complete=0
  local seen='|'
  while IFS= read -r line || [[ -n "$line" ]]; do
    kind="${line%%$'\t'*}"
    case "$kind" in
      MODULE)
        [[ "$complete" -eq 0 ]] || return 1
        IFS=$'\t' read -r kind version name origin_kind origin extra <<<"$line"
        [[ -z "$extra" && "$version" == PYMOD1 && "$name" =~ ^[A-Za-z0-9_.]+$ &&
          "$seen" != *"|$name|"* ]] || return 1
        case "$origin_kind" in
          built-in | frozen) [[ "$origin" == - ]] || return 1 ;;
          file)
            _bubbles_python_security_authenticate_path "$origin" file 0 || {
              BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='MODULE_CLOSURE_UNTRUSTED'; return 2;
            }
            ;;
          *) return 1 ;;
        esac
        seen="$seen$name|"
        count=$((count + 1))
        ;;
      COMPLETE)
        [[ "$complete" -eq 0 ]] || return 1
        IFS=$'\t' read -r kind version declared_count extra <<<"$line"
        [[ -z "$extra" && "$version" == PYMOD1 && "$declared_count" =~ ^[0-9]+$ &&
          "$declared_count" -eq "$count" ]] || return 1
        complete=1
        ;;
      *) return 1 ;;
    esac
  done <"$path"
  [[ "$complete" -eq 1 ]] || return 1
  for name in ast dataclasses hashlib os pathlib re sys types typing; do
    [[ "$seen" == *"|$name|"* ]] || return 1
  done
  return 0
}

_bubbles_python_security_reset_identity() {
  BUBBLES_PYTHON_SECURITY_RUNTIME=''
  BUBBLES_PYTHON_SECURITY_STATUS=127
  BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NOT_RUN'
  BUBBLES_PYTHON_SECURITY_REJECTION='NONE'
  BUBBLES_PYTHON_SECURITY_CANDIDATE_COUNT=0
  BUBBLES_PYTHON_SECURITY_PROVENANCE='none'
  BUBBLES_PYTHON_SECURITY_TRUST_CONTRACT='root-protected-native-python-v1'
  BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL='none'
  BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL='none'
  BUBBLES_PYTHON_SECURITY_DEVELOPER_DIR=''
  BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
  BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
}

_bubbles_python_security_record_prelaunch_failure() {
  local status="$1" diagnostic="$2" rejection="$3"
  if [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == NOT_RUN ||
    "$diagnostic" == XCODE_LICENSE_UNACCEPTED ||
    "$diagnostic" == METADATA_UNAVAILABLE ]]; then
    BUBBLES_PYTHON_SECURITY_STATUS=$status
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC="$diagnostic"
    BUBBLES_PYTHON_SECURITY_REJECTION="$rejection"
  fi
}

# Resolve the Apple launcher to its Python target without executing
# /usr/bin/python3. The returned target has already passed full path checks.
_bubbles_python_security_resolve_apple_target() {
  local developer_dir="" target="" run_status=0 stderr_path=""
  _bubbles_python_security_authenticate_path /usr/bin/python3 executable 1 || return 1
  _bubbles_python_security_authenticate_path /usr/bin/xcode-select executable 1 || {
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TOOL_UNTRUSTED'; return 1;
  }
  _bubbles_python_security_authenticate_path /usr/bin/xcrun executable 1 || {
    BUBBLES_PYTHON_SECURITY_PATH_REJECTION='TOOL_UNTRUSTED'; return 1;
  }
  if [[ -n "${DEVELOPER_DIR:-}" ]]; then
    if ! _bubbles_python_security_authenticate_path "$DEVELOPER_DIR" directory 0; then
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='DEVELOPER_DIR_UNTRUSTED'
      return 2
    fi
    developer_dir="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
  else
    if bubbles_python_run_security_operation apple-select; then run_status=0; else run_status=$?; fi
    if [[ "$run_status" -ne 0 ]] ||
      ! _bubbles_python_security_single_line_capture "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" 4096; then
      BUBBLES_PYTHON_SECURITY_STATUS=$run_status
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='APPLE_RESOLUTION_UNAVAILABLE'
      bubbles_python_security_cleanup || true
      return 2
    fi
    developer_dir="$BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT"
    bubbles_python_security_cleanup || return 2
    if ! _bubbles_python_security_authenticate_path "$developer_dir" directory 0; then
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='DEVELOPER_DIR_UNTRUSTED'
      return 2
    fi
    developer_dir="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
  fi
  # Sourced callers inspect the authenticated Apple developer directory.
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_DEVELOPER_DIR="$developer_dir"

  if bubbles_python_run_security_operation apple-find-python "$developer_dir"; then run_status=0; else run_status=$?; fi
  stderr_path="$BUBBLES_PYTHON_SECURITY_STDERR_PATH"
  if [[ "$run_status" -ne 0 ]]; then
    BUBBLES_PYTHON_SECURITY_STATUS=$run_status
    if [[ "$run_status" -eq 69 && -f "$stderr_path" ]] &&
      /usr/bin/grep -Eiq 'Xcode (license|licence)|license agreements' "$stderr_path" 2>/dev/null; then
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='XCODE_LICENSE_UNACCEPTED'
    else
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='APPLE_RESOLUTION_UNAVAILABLE'
    fi
    bubbles_python_security_cleanup || true
    return 2
  fi
  if ! _bubbles_python_security_single_line_capture "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" 4096; then
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='APPLE_RESOLUTION_UNAVAILABLE'
    bubbles_python_security_cleanup || true
    return 2
  fi
  target="$BUBBLES_PYTHON_SECURITY_CAPTURE_TEXT"
  bubbles_python_security_cleanup || return 2
  if ! _bubbles_python_security_authenticate_path "$target" executable 1; then
    return $?
  fi
  return 0
}

# Security resolver. It accepts no candidate and ignores every general
# interpreter override and managed-venv locator.
bubbles_python_resolve_security_runtime() {
  local candidate="" resolved="" first_rejection='NONE' run_status=0 protocol_status=0
  local path_entry="" candidate_seen='|' candidate_result=0
  local -a path_entries=()
  local -a candidates=()

  _bubbles_python_security_reset_identity
  if [[ "$EUID" -eq 0 ]]; then
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='CALLER_WRITE_AUTHORITY_UNSEPARATED'
    return 1
  fi

  if [[ -e /usr/bin/python3 || -L /usr/bin/python3 ]]; then
    candidates+=(/usr/bin/python3)
    candidate_seen="${candidate_seen}/usr/bin/python3|"
  fi
  IFS=':' read -r -a path_entries <<<"${PATH:-}"
  for path_entry in "${path_entries[@]+"${path_entries[@]}"}"; do
    [[ -n "$path_entry" && "$path_entry" == /* ]] || continue
    candidate="${path_entry%/}/python3"
    [[ -e "$candidate" || -L "$candidate" ]] || continue
    [[ "$candidate_seen" == *"|$candidate|"* ]] && continue
    candidates+=("$candidate")
    candidate_seen="$candidate_seen$candidate|"
  done
  # Sourced callers inspect the finite authenticated-candidate count.
  # shellcheck disable=SC2034
  BUBBLES_PYTHON_SECURITY_CANDIDATE_COUNT=${#candidates[@]}
  if [[ ${#candidates[@]} -eq 0 ]]; then
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NO_CANDIDATE'
    BUBBLES_PYTHON_SECURITY_REJECTION='ABSENT'
    return 1
  fi

  for candidate in "${candidates[@]}"; do
    candidate_result=0
    if [[ "$candidate" == /usr/bin/python3 && -x /usr/bin/xcode-select && -x /usr/bin/xcrun ]]; then
      if _bubbles_python_security_resolve_apple_target; then
        resolved="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      else
        candidate_result=$?
        if [[ "$candidate_result" -eq 2 ]]; then
          _bubbles_python_security_record_prelaunch_failure \
            "$BUBBLES_PYTHON_SECURITY_STATUS" "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" \
            "$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
        else
          [[ "$first_rejection" != NONE ]] || first_rejection="$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
        fi
        continue
      fi
    else
      if _bubbles_python_security_authenticate_path "$candidate" executable 1; then
        resolved="$BUBBLES_PYTHON_SECURITY_PATH_RESOLVED"
      else
        candidate_result=$?
        if [[ "$candidate_result" -eq 2 ]]; then
          _bubbles_python_security_record_prelaunch_failure 127 METADATA_UNAVAILABLE NONE
        else
          [[ "$first_rejection" != NONE ]] || first_rejection="$BUBBLES_PYTHON_SECURITY_PATH_REJECTION"
        fi
        continue
      fi
    fi

    BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=1
    BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME="$resolved"
    if bubbles_python_run_security_operation runtime-probe; then run_status=0; else run_status=$?; fi
    if [[ "$run_status" -ne 0 ]]; then
      BUBBLES_PYTHON_SECURITY_STATUS=$run_status
      case "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" in
        SUPERVISOR_TIMEOUT) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_TIMEOUT' ;;
        SUPERVISOR_UNAVAILABLE | SUPERVISOR_UNTRUSTED)
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC"
          ;;
        SIGNAL_HUP | SIGNAL_INT | SIGNAL_TERM) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" ;;
        *) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_EXIT_NONZERO' ;;
      esac
      bubbles_python_security_cleanup || true
      BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
      BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
      return 1
    fi
    protocol_status=0
    _bubbles_python_security_validate_runtime_protocol "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" "$resolved" || protocol_status=$?
    if [[ "$protocol_status" -ne 0 ]]; then
      if [[ "$protocol_status" -eq 1 ]]; then
        BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_PROTOCOL_INVALID'
      fi
      BUBBLES_PYTHON_SECURITY_STATUS=125
      bubbles_python_security_cleanup || true
      BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
      BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
      return 1
    fi
    bubbles_python_security_cleanup || {
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_PROTOCOL_INVALID'; return 2;
    }
    BUBBLES_PYTHON_SECURITY_PATH_PROTOCOL='PYSEC1'

    if bubbles_python_run_security_operation module-probe; then run_status=0; else run_status=$?; fi
    if [[ "$run_status" -ne 0 ]]; then
      BUBBLES_PYTHON_SECURITY_STATUS=$run_status
      case "$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" in
        SUPERVISOR_TIMEOUT) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_TIMEOUT' ;;
        SUPERVISOR_UNAVAILABLE | SUPERVISOR_UNTRUSTED)
          BUBBLES_PYTHON_SECURITY_DIAGNOSTIC="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC"
          ;;
        SIGNAL_HUP | SIGNAL_INT | SIGNAL_TERM) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC="$BUBBLES_PYTHON_SECURITY_RUN_DIAGNOSTIC" ;;
        *) BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='PROBE_EXIT_NONZERO' ;;
      esac
      bubbles_python_security_cleanup || true
      BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
      BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
      return 1
    fi
    protocol_status=0
    _bubbles_python_security_validate_module_protocol "$BUBBLES_PYTHON_SECURITY_STDOUT_PATH" || protocol_status=$?
    if [[ "$protocol_status" -ne 0 ]]; then
      if [[ "$protocol_status" -eq 1 ]]; then
        BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='MODULE_PROTOCOL_INVALID'
      fi
      BUBBLES_PYTHON_SECURITY_STATUS=125
      bubbles_python_security_cleanup || true
      BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
      BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
      return 1
    fi
    bubbles_python_security_cleanup || {
      BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='MODULE_PROTOCOL_INVALID'; return 2;
    }
    BUBBLES_PYTHON_SECURITY_MODULE_PROTOCOL='PYMOD1'
    BUBBLES_PYTHON_SECURITY_RUNTIME="$resolved"
    BUBBLES_PYTHON_SECURITY_STATUS=0
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='OK'
    BUBBLES_PYTHON_SECURITY_REJECTION='NONE'
    # Sourced callers inspect the provenance of the selected runtime.
    # shellcheck disable=SC2034
    BUBBLES_PYTHON_SECURITY_PROVENANCE='root-protected-path'
    BUBBLES_PYTHON_SECURITY_RESOLUTION_ACTIVE=0
    BUBBLES_PYTHON_SECURITY_CANDIDATE_RUNTIME=''
    return 0
  done

  if [[ "$BUBBLES_PYTHON_SECURITY_DIAGNOSTIC" == NOT_RUN ]]; then
    BUBBLES_PYTHON_SECURITY_STATUS=127
    BUBBLES_PYTHON_SECURITY_DIAGNOSTIC='NO_AUTHENTICATED_CANDIDATE'
    # Sourced callers inspect the final candidate rejection reason.
    # shellcheck disable=SC2034
    BUBBLES_PYTHON_SECURITY_REJECTION="$first_rejection"
  fi
  return 1
}

# bubbles_python_activate — make a bare `python3` call resolve to a satisfying
# interpreter, so the ~77 scripts that invoke `python3` directly need no edit.
# Only prepends when PATH's python3 does NOT already satisfy, so an operator who
# provisioned deps their own way keeps their interpreter. Idempotent.
bubbles_python_activate() {
  local venv_python bin_dir path_python
  venv_python="$(bubbles_python_venv_python)" || return 1
  bubbles_python_satisfies "$venv_python" || return 1
  path_python="$(command -v python3 2>/dev/null || true)"
  bubbles_python_satisfies "$path_python" && return 0
  bin_dir="${venv_python%/python3}"
  case ":${PATH:-}:" in
    *":$bin_dir:"*) ;;
    *)
      PATH="$bin_dir:${PATH:-}"
      export PATH
      ;;
  esac
  return 0
}

# bubbles_python_base — an interpreter able to CREATE the venv. Distinct from
# bubbles_python_resolve: the base need not have the modules yet.
bubbles_python_base() {
  local candidate resolved
  for candidate in "${BUBBLES_PYTHON_BASE:-}" python3 python3.13 python3.12 python3.11; do
    [[ -n "$candidate" ]] || continue
    resolved="$(command -v "$candidate" 2>/dev/null || true)"
    [[ -n "$resolved" ]] || continue
    if "$resolved" -c 'import sys, venv; sys.exit(0 if sys.version_info[:2] >= (3, 9) else 1)' >/dev/null 2>&1; then
      printf '%s\n' "$resolved"
      return 0
    fi
  done
  return 1
}

bubbles_python_requirements_default() {
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  printf '%s\n' "$here/../requirements.txt"
}

# bubbles_python_provision <requirements-file> — idempotent create-or-repair.
bubbles_python_provision() {
  local requirements="${1:-}" venv_dir venv_python base
  if ! venv_dir="$(bubbles_python_home)"; then
    echo "python-env: cannot locate the managed environment: $BUBBLES_PYTHON_LOCATOR_VARS are all unset" >&2
    echo "  set one of them, then re-run --provision." >&2
    return 2
  fi
  venv_python="$venv_dir/bin/python3"

  if [[ ! -f "$requirements" ]]; then
    echo "python-env: requirements file not found: $requirements" >&2
    return 2
  fi

  # A half-built or interpreter-less venv is repaired, not worked around.
  if [[ ! -x "$venv_python" ]]; then
    if ! base="$(bubbles_python_base)"; then
      echo "python-env: no python3 (>= 3.9, with venv) available to build the managed environment" >&2
      echo "  set BUBBLES_PYTHON_BASE to an interpreter, or install python3." >&2
      return 2
    fi
    echo "python-env: creating managed environment at $venv_dir (base: $base)"
    rm -rf "$venv_dir"
    if ! "$base" -m venv "$venv_dir"; then
      echo "python-env: failed to create the virtualenv at $venv_dir" >&2
      return 2
    fi
  else
    echo "python-env: reusing managed environment at $venv_dir"
  fi

  # Record the toolchain identity on EVERY run, pass or fail. A provisioning
  # failure that reports only "it failed" leaves the next reader guessing, and
  # WHICH interpreter ran — on WHICH platform — is the first fact any diagnosis
  # needs. Asked of the interpreter itself, never inferred from the host.
  local venv_identity pip_status=0
  venv_identity="$("$venv_python" -c 'import sys, platform; print(sys.version.split()[0], platform.system() + "/" + platform.machine())' 2>/dev/null)" ||
    venv_identity="unreported (the interpreter did not answer)"

  echo "python-env: interpreter $venv_python ($venv_identity)"
  echo "python-env: installing pinned requirements from $requirements"
  "$venv_python" -m pip install \
    --disable-pip-version-check \
    --no-input \
    --requirement "$requirements" || pip_status=$?
  if [[ "$pip_status" -ne 0 ]]; then
    # Report what was OBSERVED. The previous message asserted a cause
    # ("network unavailable, or a pin no longer resolves") that nothing here
    # measured, which is how a CI leg goes dark: the log named a diagnosis
    # instead of the data needed to reach one.
    {
      echo "python-env: dependency installation FAILED (pip rc=$pip_status)"
      echo "  interpreter  : $venv_python"
      echo "  python       : $venv_identity"
      echo "  requirements : $requirements"
      echo "  The cause is NOT determined here; pip's own output above is the record."
      echo "  Common causes: no route to the package index, an index that requires"
      echo "  auth, or a pin with no distribution for this python/platform."
    } >&2
    return 2
  fi

  if ! bubbles_python_satisfies "$venv_python"; then
    echo "python-env: environment still does not import every required module after install" >&2
    return 1
  fi
  return 0
}

bubbles_python_report() {
  local resolved module status_line satisfied=0 home
  echo "Bubbles managed Python posture"
  if home="$(bubbles_python_home)"; then
    echo "  managed venv : $home"
  else
    echo "  managed venv : NONE — no locator ($BUBBLES_PYTHON_LOCATOR_VARS are all unset)"
  fi
  if resolved="$(bubbles_python_resolve)"; then
    satisfied=1
    echo "  resolved     : $resolved"
  else
    echo "  resolved     : NONE — no interpreter imports every required module"
  fi
  for module in "${BUBBLES_PYTHON_MODULES[@]}"; do
    if [[ "$satisfied" -eq 1 ]] && "$resolved" -c "import $module" >/dev/null 2>&1; then
      status_line="ok"
    else
      status_line="MISSING"
    fi
    printf '  module %-12s %s\n' "$module" "$status_line"
  done
  [[ "$satisfied" -eq 1 ]] || return 1
  return 0
}

# ── executed (not sourced) ────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  set -euo pipefail

  bubbles_python_usage() {
    cat <<'EOF'
Usage: bash bubbles/scripts/python-env.sh [--check | --provision | --path]

  --check       Report interpreter/module posture. Exit 0 satisfied, 1 not.
  --provision   Create or repair the managed virtualenv and install the pinned
                requirements from bubbles/requirements.txt.
  --path        Print the resolved interpreter. Exit 1 when none satisfies.
  --help        This text.

Resolution order: $BUBBLES_PYTHON, then the managed venv, then python3 on PATH.
The managed venv is located from BUBBLES_PYTHON_HOME, else XDG_CACHE_HOME, else
HOME. With none of those set the venv cannot be named at all, which is reported
as an absent locator, not as an interpreter that failed its modules.
There is no --skip/--force: an unsatisfied posture is reported, never bypassed.
EOF
  }

  mode="--check"
  if [[ $# -gt 0 ]]; then
    mode="$1"
    shift
  fi
  if [[ $# -gt 0 ]]; then
    echo "python-env: unexpected argument: $1" >&2
    bubbles_python_usage >&2
    exit 2
  fi

  case "$mode" in
    --check)
      if bubbles_python_report; then
        exit 0
      fi
      echo >&2
      if bubbles_python_home >/dev/null; then
        echo "Remediate with: bash bubbles/scripts/python-env.sh --provision" >&2
      else
        echo "Remediate by setting one of $BUBBLES_PYTHON_LOCATOR_VARS, then --provision." >&2
      fi
      exit 1
      ;;
    --provision)
      requirements="$(bubbles_python_requirements_default)"
      bubbles_python_provision "$requirements"
      provision_status=$?
      if [[ "$provision_status" -ne 0 ]]; then
        exit "$provision_status"
      fi
      echo
      bubbles_python_report
      ;;
    --path)
      if resolved_path="$(bubbles_python_resolve)"; then
        printf '%s\n' "$resolved_path"
        exit 0
      fi
      # Two failures reach here and they call for different operator actions.
      # Reporting both as "no interpreter satisfies" sent the reader to inspect
      # interpreters when nothing had been able to name one.
      if bubbles_python_home >/dev/null; then
        echo "python-env: no interpreter satisfies the required modules" >&2
      else
        echo "python-env: cannot locate the managed environment: $BUBBLES_PYTHON_LOCATOR_VARS are all unset" >&2
        echo "  no interpreter was rejected for missing modules; none could be named." >&2
      fi
      exit 1
      ;;
    --help | -h)
      bubbles_python_usage
      exit 0
      ;;
    *)
      echo "python-env: unknown mode: $mode" >&2
      bubbles_python_usage >&2
      exit 2
      ;;
  esac
fi
