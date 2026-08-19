#!/usr/bin/env bash
#
# guard-lib.sh — shared helpers for state-transition-guard.sh and its sub-guards
# (extracted in v6.1 / R1 as the first step of the guard split + the BUG-001
# reliability fix). Sourced, not executed.
#
# Provides:
#   bubbles_run_with_timeout <secs> <cmd...>   portable timeout (124 on timeout)
#   bubbles_pruned_find       <root> <pred...> find that prunes generated dirs
#
# These convert two BUG-001 failure modes into bounded, observable behavior:
#   - sub-guard invocations that hang with no timeout
#   - whole-repo find walks that traverse .git / node_modules / target / build
#
# Idempotent: guarded against double-source.

[[ -n "${_BUBBLES_GUARD_LIB_SOURCED:-}" ]] && return 0
_BUBBLES_GUARD_LIB_SOURCED=1

# Portable command timeout. Prefers GNU `timeout`, then `gtimeout`, else a bash
# watchdog fallback. Returns the command's exit code; 124 on timeout (matching
# GNU timeout). Caller-supplied stdout/stderr redirections are inherited.
bubbles_run_with_timeout() {
  local secs="$1"; shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "${secs}s" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout "${secs}s" "$@"
    return $?
  fi
  # Fallback watchdog (rare: only hosts without coreutils timeout, e.g. a stock
  # macOS PATH). Two properties are load-bearing; do not simplify either away:
  #
  #   1. The watchdog's stdout/stderr go to /dev/null. A background subshell
  #      inherits the caller's pipe, so when the caller is a command
  #      substitution -- `out="$(bubbles_run_with_timeout 120 ...)"` -- the
  #      substitution reads until EOF, and EOF does not arrive while the
  #      watchdog still holds the write end. That made an instantly-returning
  #      command block for the FULL timeout.
  #
  #   2. The watchdog polls in 1s steps instead of one long `sleep "$secs"`.
  #      Killing the subshell does not reap a `sleep` grandchild, so a single
  #      long sleep survives as an orphan for its full duration. Polling lets
  #      the watchdog notice the command finished and exit within ~1s.
  # POSIX shells start asynchronous commands with SIGINT/SIGQUIT ignored when
  # job control is off. That disposition survives exec and cannot be trapped by
  # a nested non-interactive Bash, so signal-cleanup tests (and real children)
  # hang until the watchdog kills them. Enable monitor mode only for the launch,
  # then restore the caller's state.
  local monitor_was_on=0
  case "$-" in
    *m*) monitor_was_on=1 ;;
    *) set -m ;;
  esac
  "$@" &
  local cmd_pid=$!
  if [[ "$monitor_was_on" -eq 0 ]]; then
    set +m
  fi
  (
    waited=0
    while [ "$waited" -lt "$secs" ]; do
      sleep 1
      kill -0 "$cmd_pid" 2>/dev/null || exit 0
      waited=$((waited + 1))
    done
    kill -TERM "$cmd_pid" 2>/dev/null
  ) >/dev/null 2>&1 &
  local watch_pid=$!
  local rc=0
  wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true
  wait "$watch_pid" 2>/dev/null || true
  # Normalize a watchdog SIGTERM (143) to GNU timeout's 124.
  [[ "$rc" -eq 143 ]] && rc=124
  return $rc
}

# find that prunes high-fan-out generated directories so whole-repo walks do not
# traverse .git / node_modules / target / build caches. Usage:
#   bubbles_pruned_find <root> <find-predicate...>   # predicate SHOULD end -print
bubbles_pruned_find() {
  local root="$1"; shift
  find "$root" \
    \( -type d \( -name .git -o -name node_modules -o -name target -o -name vendor \
       -o -name dist -o -name build -o -name .venv -o -name venv -o -name __pycache__ \
       -o -name coverage -o -name .bubbles-cache -o -name .next -o -name .gradle \) -prune \) \
    -o \( "$@" \)
}

# =============================================================================
# Cross-platform (Linux + macOS) portability helpers
# =============================================================================
# GNU coreutils (Linux) and BSD userland (macOS) diverge on several flags the
# guards rely on. These helpers pick the working form at runtime so a single
# code path runs on both. Prefer them over raw `sed -i`, `date -d`, `stat -c`.

# Portable in-place sed. GNU `sed -i <prog>` and BSD `sed -i '' <prog>` are
# mutually incompatible, so neither raw form is cross-platform. This rewrites via
# a temp file, preserving the EXACT sed program (no Perl-regex dialect shift).
# The FILE MUST be the LAST argument; all preceding args are sed flags/program.
# Usage: bubbles_sed_inplace [sed-flags] <program> <file>
bubbles_sed_inplace() {
  local argc=$#
  local file="${!argc}"
  local tmp
  tmp="$(mktemp)" || return 1
  if sed "${@:1:argc-1}" "$file" >"$tmp" 2>/dev/null; then
    mv "$tmp" "$file"
  else
    rm -f "$tmp"
    return 1
  fi
}

# Portable ISO-8601-UTC ("YYYY-MM-DDThh:mm:ssZ") -> epoch seconds. GNU date uses
# `-d`; BSD/macOS date uses `-j -f`. Prints the epoch on success; prints nothing
# and returns 1 on failure (caller decides how to treat an unparseable stamp).
# Usage: epoch="$(bubbles_iso_to_epoch "2026-06-30T14:00:00Z")" || epoch=""
bubbles_iso_to_epoch() {
  local ts="$1" epoch
  if epoch="$(date -d "$ts" +%s 2>/dev/null)" && [[ -n "$epoch" ]]; then
    printf '%s' "$epoch"
    return 0
  fi
  if epoch="$(date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$ts" +%s 2>/dev/null)" && [[ -n "$epoch" ]]; then
    printf '%s' "$epoch"
    return 0
  fi
  # BSD/macOS bare date ("YYYY-MM-DD") -> midnight UTC.
  if epoch="$(date -u -j -f "%Y-%m-%d %H:%M:%S" "${ts} 00:00:00" +%s 2>/dev/null)" && [[ -n "$epoch" ]]; then
    printf '%s' "$epoch"
    return 0
  fi
  return 1
}

# Portable "now, in milliseconds since epoch". GNU date supports %N (nanoseconds);
# BSD/macOS date may not (it echoes a literal 'N'), so detect a non-numeric result
# and fall back to second resolution (x1000). Usage: ms="$(bubbles_now_ms)"
bubbles_now_ms() {
  local ns
  ns="$(date +%s%N 2>/dev/null)"
  if [[ "$ns" =~ ^[0-9]+$ ]]; then
    printf '%s' "$(( ns / 1000000 ))"
  else
    printf '%s' "$(( $(date +%s) * 1000 ))"
  fi
}

# Portable file mtime (epoch seconds). GNU: `stat -c %Y`; BSD/macOS: `stat -f %m`.
# Prints the mtime, or nothing (rc 1) if the file is unstattable.
# Usage: mtime="$(bubbles_file_mtime_epoch "$file")"
bubbles_file_mtime_epoch() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null
}

# =============================================================================
# Control-plane policy activation (control-plane policy-activation fix)
# =============================================================================
# resolve_effective_policy / resolve_effective_policy_source ACTIVATE the
# Single-Source-of-Truth control-plane defaults so the v3 gates G055-G060 are no
# longer INERT when a spec omits policySnapshot. Before this, the control-plane
# checks sourced effective policy ONLY from each spec's state.json policySnapshot,
# so a missing snapshot left the SST defaults (grill/tdd/lockdown/regression/...)
# declared-but-unenforced. Precedence (highest first):
#   1. state.json.policySnapshot.<section>.<key|mode|value>   (per-spec snapshot)
#   2. <repo_root>/.specify/memory/bubbles.config.json defaults.<section>.<key>  (SST)
#   3. <framework_default>                                     (hardcoded fallback)
# The repo config file being ABSENT is handled gracefully (falls through to the
# framework default). python3 is required for the JSON precedence; without it the
# resolver falls back to the framework default (never silently skips).

# Internal: print either the resolved value ('value') or its provenance source
# ('source': snapshot | repo-default | framework-default) for one policy key.
_resolve_effective_policy_field() {
  local field="$1" state_file="$2" section="$3" key="$4" framework_default="$5"
  local repo_root="${6:-}"
  local config_file=""
  local sf_dir=""

  if [[ -z "$repo_root" ]]; then
    if [[ -n "$state_file" ]]; then
      sf_dir="$(cd "$(dirname "$state_file")" 2>/dev/null && pwd -P)" || sf_dir=""
    fi
    if [[ -n "$sf_dir" ]] && command -v git >/dev/null 2>&1 \
      && git -C "$sf_dir" rev-parse --show-toplevel >/dev/null 2>&1; then
      repo_root="$(git -C "$sf_dir" rev-parse --show-toplevel 2>/dev/null)" || repo_root=""
    fi
    if [[ -z "$repo_root" ]]; then
      if [[ -n "$sf_dir" ]]; then
        repo_root="$sf_dir"
      else
        repo_root="$PWD"
      fi
    fi
  fi
  config_file="$repo_root/.specify/memory/bubbles.config.json"

  if command -v python3 >/dev/null 2>&1; then
    if python3 -c '
import json, sys

state_file, config_file, section, key, default, field = (
    sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6])


def load(path):
    try:
        with open(path) as handle:
            return json.load(handle)
    except Exception:
        return {}


def nonempty(value):
    return value is not None and str(value).strip() != ""


value = None
source = None

snapshot = load(state_file).get("policySnapshot") or {}
section_snap = snapshot.get(section) if isinstance(snapshot, dict) else None
if isinstance(section_snap, dict):
    for candidate in (key, "mode", "value"):
        if candidate in section_snap and nonempty(section_snap.get(candidate)):
            value = section_snap.get(candidate)
            source = "snapshot"
            break

if value is None:
    defaults = load(config_file).get("defaults") or {}
    section_cfg = defaults.get(section) if isinstance(defaults, dict) else None
    if isinstance(section_cfg, dict) and key in section_cfg and nonempty(section_cfg.get(key)):
        value = section_cfg.get(key)
        source = "repo-default"

if value is None:
    value = default
    source = "framework-default"

if field == "source":
    print(source)
elif isinstance(value, bool):
    print("true" if value else "false")
else:
    print(str(value))
' "$state_file" "$config_file" "$section" "$key" "$framework_default" "$field" 2>/dev/null; then
      return 0
    fi
  fi

  # No python3 (or python failed): fall back to the framework default — never a
  # silent skip.
  if [[ "$field" == "source" ]]; then
    printf '%s\n' "framework-default"
  else
    printf '%s\n' "$framework_default"
  fi
}

# resolve_effective_policy <state_file> <section> <key> <framework_default> [repo_root]
# Echoes the effective value for the policy key using the snapshot → SST config →
# framework-default precedence chain.
resolve_effective_policy() {
  _resolve_effective_policy_field "value" "$@"
}

# resolve_effective_policy_source <state_file> <section> <key> <framework_default> [repo_root]
# Echoes the provenance of the resolved value: snapshot | repo-default | framework-default.
resolve_effective_policy_source() {
  _resolve_effective_policy_field "source" "$@"
}

# policy_snapshot_present <state_file>
# Returns 0 when state.json carries a policySnapshot object, 1 otherwise.
policy_snapshot_present() {
  local state_file="$1"
  [[ -f "$state_file" ]] || return 1
  grep -qE '"policySnapshot"[[:space:]]*:[[:space:]]*\{' "$state_file"
}

# policy_spec_grandfathered <state_file> <cutoff_YYYY-MM-DD>
# Grandfather clause for the newly-activated control-plane enforcement: a spec
# that carries NO policySnapshot AND whose createdAt is missing or strictly
# before <cutoff> is exempt (returns 0). A spec that DOES carry a policySnapshot,
# or whose createdAt is on/after the cutoff, gets full enforcement (returns 1).
# Mirrors the G094 createdAt comparison in capability-foundation-guard.sh.
policy_spec_grandfathered() {
  local state_file="$1" cutoff="$2"
  # A spec that carries policySnapshot always gets full enforcement.
  if policy_snapshot_present "$state_file"; then
    return 1
  fi
  local created_at="" created_date=""
  created_at="$(grep -Eo '"createdAt"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null \
    | head -n1 | sed -E 's/.*"createdAt"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' || true)"
  created_date="${created_at:0:10}"
  # Missing createdAt → grandfathered (mirrors G094 missing-createdAt handling).
  if [[ -z "$created_date" ]]; then
    return 0
  fi
  # Malformed createdAt → fail closed to enforcement (NOT grandfathered).
  if [[ ! "$created_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    return 1
  fi
  # createdAt strictly before the cutoff → grandfathered.
  if [[ "$created_date" < "$cutoff" ]]; then
    return 0
  fi
  return 1
}

# detect_red_green_ordering <file...>
# Layer-2 G060 evidence integrity. Returns 0 only when a RED-stage (failing-proof)
# marker appears on an EARLIER line than the FIRST GREEN-stage (passing-proof)
# marker within the SAME file — proving red-was-captured-before-green ordering.
# The literal word 'tdd'/'scenario-first' alone never matches (it is a rubber
# stamp, not ordering evidence). Returns 1 when no such ordering exists.
detect_red_green_ordering() {
  local red_pattern green_pattern f red_line green_line
  red_pattern='red[ -]?stage|(^|[^a-z])red:|failing targeted|failing proof|failing test first|required red-stage|test fails|test result:[[:space:]]*failed|--- fail|[1-9][0-9]*[[:space:]]+(tests?[[:space:]]+)?failed'
  green_pattern='green[ -]?stage|(^|[^a-z])green:|now passes|passing|test result:[[:space:]]*ok|0[[:space:]]+failed|--- pass|[1-9][0-9]*[[:space:]]+(tests?[[:space:]]+)?passed'
  for f in "$@"; do
    [[ -f "$f" ]] || continue
    red_line="$(grep -niE "$red_pattern" "$f" 2>/dev/null | head -n1 | cut -d: -f1 || true)"
    [[ -n "$red_line" ]] || continue
    green_line="$(grep -niE "$green_pattern" "$f" 2>/dev/null | head -n1 | cut -d: -f1 || true)"
    [[ -n "$green_line" ]] || continue
    if [[ "$red_line" =~ ^[0-9]+$ && "$green_line" =~ ^[0-9]+$ && "$red_line" -lt "$green_line" ]]; then
      return 0
    fi
  done
  return 1
}

# bubbles_ci_annotate_failure <label>
# Emit a GitHub Actions `::error::` workflow command naming a failing check.
#
# WHY (OW-002). Raw CI logs are NOT readable without privilege: `GET
# /actions/jobs/<id>/logs` requires repo admin and answers 403 to an
# unprivileged caller, so a red macOS release-hygiene job could not be
# attributed to a specific check from a machine without an admin token.
# Check-run ANNOTATIONS carry no such requirement — `GET
# /repos/<owner>/<repo>/check-runs/<id>/annotations` answered unauthenticated on
# 2026-08-07. Emitting one annotation per failing check therefore makes every
# future failure attributable with ZERO credentials, from any machine.
#
# ADDITIVE, never a replacement. Callers keep printing their existing plain
# `FAIL: <label>` line — other tooling and the selftests parse it. This only
# ADDS a line.
#
# GATED on GitHub's own signal. Nothing is emitted unless GITHUB_ACTIONS is
# exactly "true", so local runs stay byte-identical to before.
#
# ESCAPED per GitHub's workflow-command rules, so a label carrying `%` or an
# embedded newline cannot truncate the annotation or forge a second command.
# `%` MUST be substituted FIRST — otherwise the `%` that %0D/%0A introduce
# would itself be re-escaped into %250D/%250A.
bubbles_ci_annotate_failure() {
  [[ "${GITHUB_ACTIONS:-}" == "true" ]] || return 0
  local msg="${1-}"
  msg="${msg//%/%25}"
  msg="${msg//$'\r'/%0D}"
  msg="${msg//$'\n'/%0A}"
  printf '::error::%s\n' "$msg"
}

# bubbles_ci_failure_detail <file>
# Print the failure-shaped lines from a captured check log, at most 10 lines.
# Empty output means the log carried no recognizable failure line, in which case
# the caller MUST fall back to the bare label rather than annotating an empty body.
#
# THREE SHAPES ARE RECOGNIZED, in ONE grep pass (so ordering and the 10-line cap
# stay trivially correct):
#   1. assertion shapes  — FAIL / ERROR / AssertionError / Traceback / not ok / ✗ / ❌
#   2. tool-error shapes — a line whose FIRST token is `<tool>:` (awk, sed, jq, …)
#   3. shell/interpreter phrases ANYWHERE in the line (command not found, …)
#
# Shapes 2 and 3 exist because of a measured mis-signal: a macOS check failed
# with `awk: line 27: syntax error at or near ,`, which starts with none of the
# shape-1 tokens and was therefore filtered out. The annotation then showed only
# assertions carrying empty values, making a CRASHED INTERPRETER look like code
# that runs but produces wrong content. Interpreter-level errors MUST be visible.
#
# POSIX ERE ONLY — this also runs against macOS BSD grep. No \s, \+, \?, no
# `grep -P`. The required `:` right after the tool token is what keeps shape 2
# bounded: `shellcheck:` and `python-ish:` do not match.
#
# Always returns 0. "No failure line found" is a NORMAL outcome of this helper,
# not an error, and the caller runs under `set -euo pipefail` — a bare
# `grep | head | cut` would return 1 there (grep finds nothing, pipefail
# propagates it) and abort the caller mid-run on exactly the fallback path this
# helper documents. The trailing `|| true` is LOAD-BEARING.
# See ci-annotation-emitter-selftest.sh cases I4/I5/I15.
bubbles_ci_failure_detail() {
  local file="${1-}"
  [[ -f "$file" ]] || return 0
  local _tools='awk|gawk|mawk|sed|grep|jq|date|stat|sort|python3|python|bash|sh|node'
  local _phrases='[Cc]ommand not found|[Ss]yntax error|[Uu]nbound variable|[Nn]o such file or directory|[Pp]ermission denied|[Ii]llegal option'
  local _re="^[[:space:]]*(FAIL|ERROR|AssertionError|Traceback|not ok|✗|❌|(${_tools}):)|(${_phrases})"
  grep -aE "$_re" "$file" 2>/dev/null | head -n 10 | cut -c1-300 || true
}

# Emit the selftest basenames a validator actually SCHEDULES, as a newline-
# wrapped block suitable for exact `*$'\n'name$'\n'*` matching.
#
# Detection is INVOCATION-based on purpose. Matching raw source text means any
# mention removes a selftest from the run -- a comment, a rationale, even a
# denylist note -- and nothing reports that it stopped executing. That is a
# coverage hole that looks exactly like a passing suite.
#
# run_check invocations are frequently backslash-continued, so logical lines are
# reassembled before matching; a line-at-a-time reader would miss the continued
# form and re-run an already-wired selftest a second time.
#
# Builtins only: per BUG-021, no external tool may decide what a validator
# executes, because a minimal PATH would silently collapse the answer to empty.
bubbles_scheduled_selftests() {
  local source_text="${1-}"
  local line pending="" logical token out=$'\n'
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == *\\ ]]; then
      pending+="${line%\\} "
      continue
    fi
    logical="$pending$line"
    pending=""
    logical="${logical#"${logical%%[![:space:]]*}"}"
    case "$logical" in
      'run_check '* | 'run_check_self_only '*) ;;
      *) continue ;;
    esac
    while [[ "$logical" == *-selftest.sh* ]]; do
      token="${logical%%-selftest.sh*}"
      logical="${logical#*-selftest.sh}"
      token="${token##*/}"
      token="${token##*\"}"
      token="${token##*[[:space:]]}"
      out+="${token}-selftest.sh"$'\n'
    done
  done <<<"$source_text"
  printf '%s' "$out"
}

# IMP-047 S-A. The orphaned-scaffolding rule, made mechanical.
#
# `gates-block-reader-lint.sh` computed that its own guarded surface was gone,
# printed "the block is safe to remove" and "the inventory is empty; SCOPE-13
# removal precondition is met", and then exited 0. Because it PASSED, every
# scheduler kept running it, and it survived long after the thing it guarded had
# been deleted. A check that announces its own obsolescence and reports success
# is indistinguishable from a check that is still doing work.
#
# A lint reaching this helper is stating a fact about ITSELF, not about the tree,
# so it must stop being green. The helper prints the declaration and returns 3 --
# a distinct code, because this is neither a finding in the guarded surface nor a
# usage error -- and the caller propagates it. There is no suppression flag;
# suppression would recreate the exact silence this removes.
bubbles_removal_precondition_met() {
  local name="${1:-lint}"
  local detail="${2:-its guarded surface is absent}"
  printf '%s: REMOVAL PRECONDITION MET: %s\n' "$name" "$detail" >&2
  printf '%s: this check now guards nothing and must be deleted, not scheduled.\n' "$name" >&2
  printf '%s: refusing to PASS a check that has reported its own obsolescence.\n' "$name" >&2
  return 3
}
