#!/usr/bin/env bash
#
# validate-cache.sh — content-addressed result cache for hermetic selftests
# (IMP-027 / SCOPE-7).
#
# WHY THIS EXISTS
# ---------------
# framework-validate.sh --tier=core took 260 seconds to run 16 of 209 checks.
# The full tier is what pre-push runs. A ~25-minute serial pre-push is the
# single strongest practical incentive toward the bypass behaviour the whole
# framework exists to prevent, so the wall clock is a governance problem, not
# an ergonomics one.
#
# WHAT IS SAFE TO CACHE
# ---------------------
# ONLY hermetic selftests. The framework's selftest contract is that they build
# their own fixtures under mktemp and assert behaviour that depends on nothing
# outside their own source. That makes sha256(script) a sound cache key: if the
# script has not changed, its verdict cannot have changed.
#
# LIVE GUARDS ARE NEVER CACHED. They read the working tree, which is exactly
# what changes between runs. Caching one would mean reporting a verdict about a
# tree that was never inspected — the same "reported success for a check that
# never ran" failure this proposal's SEC-2 work removed. The cache therefore
# refuses any command that is not a bare hermetic selftest invocation.
#
# The entry also records the framework version, so a version bump invalidates
# every entry rather than silently carrying verdicts across releases.
#
# Usage:
#   validate_cache_key <script-path>          -> prints a key, or empty if unsafe
#   validate_cache_get <key>                  -> exit 0 if a PASS is cached
#   validate_cache_put <key> <exit-code>      -> record a verdict (pass only)
#   validate_cache_prune                      -> drop entries older than 30 days

set -uo pipefail

: "${BUBBLES_VALIDATE_CACHE_DIR:=${XDG_CACHE_HOME:-$HOME/.cache}/bubbles/validate}"

# validate_cache_enabled — the cache is opt-OUT, but never active in CI, where a
# cold, complete run is the entire point.
validate_cache_enabled() {
  [[ "${BUBBLES_VALIDATE_CACHE:-1}" == "1" ]] || return 1
  [[ -z "${CI:-}" ]] || return 1
  command -v sha256sum >/dev/null 2>&1 || return 1
  return 0
}

# validate_cache_key <script-path> [framework-version]
validate_cache_key() {
  local script="$1"
  local version="${2:-unknown}"

  [[ -f "$script" ]] || return 1
  # Hermetic selftests only. Anything else reads the tree.
  case "$(basename "$script")" in
    *-selftest.sh) ;;
    *) return 1 ;;
  esac

  local digest
  digest="$(sha256sum "$script" 2>/dev/null | cut -d' ' -f1)" || return 1
  [[ -n "$digest" ]] || return 1
  printf '%s-%s' "$version" "$digest"
}

validate_cache_get() {
  local key="$1"
  validate_cache_enabled || return 1
  [[ -n "$key" ]] || return 1
  [[ -f "$BUBBLES_VALIDATE_CACHE_DIR/$key" ]] || return 1
  return 0
}

# validate_cache_put <key> <exit-code>
#
# Only PASSES are cached. Caching a failure would let a fixed script keep
# reporting its old failure until something invalidated the entry, and a stale
# red is just as dishonest as a stale green.
validate_cache_put() {
  local key="$1" code="$2"
  validate_cache_enabled || return 0
  [[ -n "$key" ]] || return 0
  [[ "$code" -eq 0 ]] || return 0
  mkdir -p "$BUBBLES_VALIDATE_CACHE_DIR" 2>/dev/null || return 0
  : >"$BUBBLES_VALIDATE_CACHE_DIR/$key" 2>/dev/null || return 0
  return 0
}

validate_cache_prune() {
  [[ -d "$BUBBLES_VALIDATE_CACHE_DIR" ]] || return 0
  find "$BUBBLES_VALIDATE_CACHE_DIR" -type f -mtime +30 -delete 2>/dev/null || true
}

validate_cache_clear() {
  [[ -d "$BUBBLES_VALIDATE_CACHE_DIR" ]] || return 0
  rm -rf "${BUBBLES_VALIDATE_CACHE_DIR:?}"/* 2>/dev/null || true
}
