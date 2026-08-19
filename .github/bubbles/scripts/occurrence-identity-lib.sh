#!/usr/bin/env bash
# occurrence-identity-lib.sh — the ONE positional occurrence-identity rule.
#
# WHY THIS EXISTS
# `phase-coordinator.sh` established occurrence identity for PHASES: repeated
# names get DISTINCT positional ids (`validate#1`, `validate#2`), and an
# occurrence is resolved by running it rather than by asserting it. IMP-048
# SCOPE-3 extends those same guarantees one level DOWN, to the individual test
# leaves a phase is made of.
#
# Two consumers of one rule is fine. Two IMPLEMENTATIONS of one rule is BUG-004,
# which this repository has already paid for once. So the rule lives here and
# both consumers source it: there is exactly one implementation of what an
# occurrence id is and of which outcomes resolve one.
#
# Provides:
#   occurrence_ids_for <name>...       prints "<name>#<n>", one per line, in the
#                                      order given. Identity is POSITIONAL, so
#                                      the second run of a name is a different
#                                      thing from the first.
#   occurrence_resolving_outcome <o>   returns 0 when the outcome RESOLVES an
#                                      occurrence. A failure, a timeout, and a
#                                      blocked dependent all leave it
#                                      outstanding, which is what makes resume
#                                      land on them.
#
# Sourced, not executed. Bash 3.2 safe (macOS ships 3.2, so no associative
# arrays and no `mapfile`). Idempotent: guarded against double-source.

[ -n "${_BUBBLES_OCCURRENCE_IDENTITY_LIB_SOURCED:-}" ] && return 0
_BUBBLES_OCCURRENCE_IDENTITY_LIB_SOURCED=1

# occurrence_ids_for <name>...
# Assigns each name its 1-based ordinal among prior occurrences of the same
# name. Emits nothing for an empty argument list.
occurrence_ids_for() {
  local _oi_name _oi_prior _oi_count _oi_seen=''
  for _oi_name in "$@"; do
    _oi_count=1
    # Herestring, never a discarding pipe: `... | grep -q` on unbounded input
    # under pipefail is the BUG-009 SIGPIPE race.
    while IFS= read -r _oi_prior; do
      [ -n "$_oi_prior" ] || continue
      [ "$_oi_prior" = "$_oi_name" ] && _oi_count=$((_oi_count + 1))
    done <<< "$_oi_seen"
    _oi_seen="${_oi_seen}${_oi_seen:+
}${_oi_name}"
    printf '%s#%d\n' "$_oi_name" "$_oi_count"
  done
  return 0
}

# occurrence_resolving_outcome <outcome>
# The closed set of outcomes that RESOLVE an occurrence. Everything else --
# RAN_FAIL, UNRESOLVED, BLOCKED_NOT_RUN, PENDING, or an unknown token -- leaves
# the occurrence outstanding.
occurrence_resolving_outcome() {
  case "${1:-}" in
    RAN_PASS | SKIPPED_IRRELEVANT | ACCEPTED) return 0 ;;
    *) return 1 ;;
  esac
}
