#!/usr/bin/env bash
# bubbles/adapters/codeindex/none.sh — no-op code-index adapter.
#
# DEFAULT adapter. A repository that has not opted in to a code-index provider
# resolves here, so every consumer runs unchanged and the framework NEVER gains
# a hard dependency on an external indexer binary.
#
# Each verb returns the NEUTRAL EMPTY VALUE for its canonical normalized shape
# and exits 0. Consumers MUST treat a neutral value as "no structural facts
# available" and gracefully skip — never as a finding, and never as a failure.
#
# Canonical per-verb shapes (validated by codeindex-adapter shape selftests):
#   symbols   <query>        → JSON array → neutral empty value: []
#   impact    <symbol>       → JSON array → neutral empty value: []
#   affected  <file>...      → JSON array → neutral empty value: []
#   routes                   → JSON array → neutral empty value: []
#   status                   → JSON map   → neutral empty value: {}
#   freshness                → JSON map   → neutral empty value: {}
#   capabilities             → JSON map   → neutral empty value: {}
#
# `status` and `freshness` are MAPS (index health / staleness key -> value);
# the other four are ARRAYS of records. This mirrors the observability adapter
# contract, where fetch-alerts is an array and the remaining verbs are maps.
#
# NOTE on `capabilities`: an OPTIONAL declaration of which verbs a provider
# actually supports (native / derived / unsupported). This adapter declares
# NOTHING — the neutral empty map — because it supports every verb neutrally.
# A consumer MUST treat an absent or empty declaration as "no restrictions
# claimed", exactly as it did before the verb existed.
#
# NOTE on `freshness`: with no index configured there is nothing to be stale,
# so this returns the neutral map and exits 0. It does NOT claim "fresh" — a
# consumer that reached here has already been told by every other verb that no
# structural facts exist, and must skip rather than trust an emptiness.

set -euo pipefail

VERB="${1:-}"

case "$VERB" in
  symbols|impact|affected|routes|indexed)
    # Record-list verbs normalize to a JSON ARRAY; neutral empty is [].
    echo '[]'
    exit 0
    ;;
  status|freshness|capabilities)
    # Index health / staleness / capability declaration normalize to a JSON
    # MAP; neutral empty is {}.
    echo '{}'
    exit 0
    ;;
  sync)
    # The only mutating verb. With no index configured there is nothing to
    # sync, so this is a deliberate no-op that SUCCEEDS: it lets a repo wire
    # `freshness || sync` into its CLI or a git hook unconditionally, and stay
    # correct whether or not a provider is ever adopted.
    echo '{}'
    exit 0
    ;;
  selftest)
    # Shape selftest: emit the canonical neutral shape for <verb> with no
    # provider installed. Lets a shape lint validate this adapter offline.
    case "${2:-}" in
      symbols|impact|affected|routes|indexed) echo '[]'; exit 0 ;;
      status|freshness|sync|capabilities) echo '{}'; exit 0 ;;
      *) echo "[none][ERROR] selftest requires a known verb" >&2; exit 1 ;;
    esac
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
none.sh — no-op code-index adapter (framework default)
Usage: none.sh <verb> [args...]
Verbs: symbols <query> (-> []) | impact <symbol> (-> []) |
       affected <file>... (-> []) | routes (-> []) | indexed (-> []) |
       status (-> {}) | freshness (-> {}) | sync (-> {}, no-op) |
       capabilities (-> {}, no restrictions claimed)
       selftest <verb> (-> canonical neutral shape)
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
