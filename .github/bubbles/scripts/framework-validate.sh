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
if [[ -z "${BASH_VERSINFO:-}" ]] || (( ${BASH_VERSINFO[0]:-0} < 4 )); then
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
# `flock` holds the lock on fd 9 and the kernel releases it when the process
# dies, so this cannot leave a stale lock behind and needs no bypass flag. Where
# `flock` is absent (stock macOS), the guard degrades to a no-op rather than
# risking a lock we cannot reliably reap — matching how the optional-dependency
# selftests degrade elsewhere in this suite.
#
# The guard MUST be re-entrant. Several selftests legitimately run a NESTED
# framework-validate as part of their fixture (v5.3-selftest runs one against a
# synthesized downstream install; repo-drift-report-selftest runs one to capture
# drift output). Those are children of an outer run that already owns the lock,
# so a naive guard refuses them and fails the very suite it is protecting —
# measured: 3 red checks (v5.3 G1, tiering IMP-012, repo-drift IMP-027). The
# exported marker below is inherited only by descendants of a holding run, so
# nested invocations pass through while two INDEPENDENT top-level runs still
# contend.
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
# lock marker, which is never set when `flock` is absent (stock macOS) and would
# therefore report every nested run on a Mac as outermost.
_fv_outermost=false
[[ -z "${BUBBLES_FRAMEWORK_VALIDATE_DEPTH:-}" ]] && _fv_outermost=true
export BUBBLES_FRAMEWORK_VALIDATE_DEPTH=$((${BUBBLES_FRAMEWORK_VALIDATE_DEPTH:-0} + 1))

_fv_executes_checks=true
if [[ $# -gt 0 ]]; then
  _fv_executes_checks=false
  for _fv_arg in "$@"; do
    case "$_fv_arg" in
      --tier=core | --tier=full | --changed-only | --cache | --no-cache | --record-debt)
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
if [[ "$_fv_executes_checks" == "true" ]] && [[ -z "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" ]] && command -v flock >/dev/null 2>&1; then
  _fv_lockfile="${TMPDIR:-/tmp}/bubbles-framework-validate.lock"
  # Probe writability on a THROWAWAY command first. `exec 9>file` with no command
  # applies its redirections to the shell PERMANENTLY, so appending an error
  # suppressor there (`exec 9>file 2>/dev/null`) silences stderr for the entire
  # run — every selftest's diagnostics included. Keep the exec line bare.
  if : >"$_fv_lockfile" 2>/dev/null; then
    exec 9>"$_fv_lockfile"
    if ! flock -n 9; then
      printf 'ERROR: another framework-validate run is already in progress on this machine.\n' >&2
      printf '       Concurrent runs corrupt each other'"'"'s shared scratch fixtures and produce\n' >&2
      printf '       false failures. Wait for the other run to finish, then re-run.\n' >&2
      exit 1
    fi
    export BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD=1
  fi
elif [[ "$_fv_executes_checks" == "true" ]] && [[ -z "${BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD:-}" ]]; then
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
_bubbles_compat_dir=""
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
  trap 'rm -rf "$_bubbles_compat_dir"' EXIT
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
# --changed-only  restrict to checks whose owned surface the working tree
#                 actually touched. Ownership is DERIVED (a selftest owns the
#                 script it tests), never a hand-maintained manifest -- that
#                 enumeration habit is what COV-2 was.
# --no-cache      ignore the hermetic-selftest result cache for this run.
CHANGED_ONLY="false"
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
    --cache) CACHE_ENABLED="true" ;;
    --no-cache) CACHE_ENABLED="false" ;;
    --record-debt) RECORD_DEBT="true" ;;
    -h | --help)
      echo "Usage: framework-validate.sh [--tier=core|full] [--list-tier=core|full] [--changed-only] [--cache] [--no-cache] [--record-debt]"
      echo "  (no flag)        run every check (full tier — unchanged default)"
      echo "  --tier=core      run only the fast structural/lint/generator subset"
      echo "  --list-tier=core dry-list what the core tier runs/skips, then exit 0"
      echo "  --changed-only   run only checks whose DECLARED CLOSURE the tree touched"
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
if [[ "$_fv_outermost" == "true" && "$_fv_executes_checks" == "true" && "$LIST_TIER_ONLY" == "false" ]] &&
  [[ -f "$SCRIPT_DIR/validation-receipt.sh" ]]; then
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
    # The one LIVE check in core: 14s, and the cheapest detector of a stale
    # committed release manifest. Without it a stale manifest survives the core
    # tier and only surfaces in a full validate, where one root cause presents
    # as many unrelated-looking failures (interop, install provenance, trust doctor).
    *"Release manifest freshness"*)
      return 0
      ;;
    *) return 1 ;;
  esac
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
    if [[ -n "$_cache_key" ]] && validate_cache_get "$_cache_key"; then
      # IMP-047 S-E: REUSED is its own result, and it NEVER prints PASS. PASS
      # means this run executed the check and watched it succeed. Reuse means
      # this run executed nothing and is citing an earlier verdict, which is a
      # weaker claim, and printing the stronger word for the weaker claim is how
      # a suite reports work it did not do.
      local _receipt="unknown"
      declare -F validate_cache_receipt >/dev/null 2>&1 &&
        _receipt="$(validate_cache_receipt "$_cache_key" 2>/dev/null || echo unknown)"
      echo "==> $label"
      echo "REUSED: $label (receipt $_receipt — every declared closure input unchanged)"
      cache_hits=$((cache_hits + 1))
      echo
      return 0
    fi
  fi

  echo "==> $label"
  local _started="$SECONDS"
  local _rc=0 _cap=""
  # Only CI captures output. Locally the invocation stays exactly as it was, so
  # stdout/stderr separation and byte-for-byte output are preserved.
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    _cap="$(mktemp)"
    if "$@" 2>&1 | tee "$_cap"; then _rc=0; else _rc=$?; fi
  else
    if "$@"; then _rc=0; else _rc=$?; fi
  fi
  if [[ "$_rc" -eq 0 ]]; then
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
}

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
if [[ -f "$SCRIPT_DIR/python-env.sh" ]] &&
  ! python3 -c 'import yaml, jsonschema' >/dev/null 2>&1; then
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
# Evidence-Backed Experience Recall (IMP-037). Registration is explicit here --
# nothing in this file auto-discovers `*-selftest.sh`, so an unregistered suite
# is simply never executed. These six were green but unwired through SCOPE-5,
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
run_check "Validation run receipt selftest (IMP-049 SCOPE-2)" bash "$SCRIPT_DIR/validation-receipt-selftest.sh"
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
run_check "Report-section contract selftest (IMP-047 / S-B)" bash "$SCRIPT_DIR/report-sections-selftest.sh"
# Framework-source-only: check P3 requires the repo-root BUGS.md, which the
# release manifest classifies as neither managed nor source-only-shipped, so it
# does not exist in an installed tree. Scheduling it as portable made every
# downstream install fail a check about a file it can never have.
run_check_self_only "Bug-packet contract selftest (IMP-047 / S-B)" bash "$SCRIPT_DIR/bug-packet-selftest.sh"
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
run_check_self_only "BUG-019 state-transition compound MJS test-path regression" bash "$REPO_ROOT/tests/regression/test_26_state_transition_spec_mjs_path.sh"
run_check_self_only "BUG-021 portable framework deadline regression" bash "$REPO_ROOT/tests/regression/test_28_framework_validate_portable_timeout.sh"
run_check_self_only "BUG-029 human acceptance terminal regression (G136)" bash "$REPO_ROOT/tests/regression/test_35_human_acceptance_terminal.sh"
run_check "Convergence cap guard selftest" bash "$SCRIPT_DIR/convergence-cap-guard-selftest.sh"
run_check "Session cap guard selftest (G128)" bash "$SCRIPT_DIR/session-cap-guard-selftest.sh"
run_check "Session cap guard (live, G128)" env BUBBLES_REPO_ROOT="$REPO_ROOT" bash "$SCRIPT_DIR/session-cap-guard.sh" --quiet
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
    if [[ -x "$debt_tool" || -f "$debt_tool" ]] &&
      bash "$debt_tool" record \
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
