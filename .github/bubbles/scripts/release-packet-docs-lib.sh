#!/usr/bin/env bash
# release-packet-docs-lib.sh — ONE authority for the canonical release-packet doc set.
#
# Sourceable and Bash-3.2-safe (no associative arrays, no mapfile).
#
# The canonical contract lives in agents/bubbles.releases.agent.md:
#   "Exactly 8 docs per phase, no more and no fewer."
#
# Before this lib the eight names were duplicated across the location guard
# header, its CANONICAL_DOCS array, its path regex, its selftest loop, the
# releases agent, and the release-packet-template skill. A rename therefore had
# six places to diverge silently. IMP-050 SCOPE-2 requires a single authority;
# consumers source this file instead of re-listing the set.
#
# Exports:
#   RELEASE_PACKET_DOCS        — array of the eight canonical basenames
#   release_packet_docs_alternation  — prints a regex alternation of the stems
#                                      (no .md suffix), e.g. vision|features|...

if [ -n "${_RELEASE_PACKET_DOCS_LIB_SOURCED:-}" ]; then
  return 0
fi
_RELEASE_PACKET_DOCS_LIB_SOURCED=1

# Order is the authoring order documented in bubbles.releases.agent.md, not
# alphabetical; the reporting order of a completeness failure follows it so an
# operator reads absences in the same sequence the agent writes them.
RELEASE_PACKET_DOCS=(
  "vision.md"
  "features.md"
  "actions.md"
  "business-plan.md"
  "deployment.md"
  "marketing.md"
  "monetization.md"
  "ops-scalability.md"
)

release_packet_docs_alternation() {
  local out="" name stem
  for name in "${RELEASE_PACKET_DOCS[@]}"; do
    stem="${name%.md}"
    if [ -z "$out" ]; then
      out="$stem"
    else
      out="$out|$stem"
    fi
  done
  printf '%s' "$out"
}
