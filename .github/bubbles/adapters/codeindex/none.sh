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
#
# `status` is a MAP (index freshness/health key -> value); the other four are
# ARRAYS of records. This mirrors the observability adapter contract, where
# fetch-alerts is an array and the remaining verbs are maps.

set -euo pipefail

VERB="${1:-}"

case "$VERB" in
  symbols|impact|affected|routes)
    # Record-list verbs normalize to a JSON ARRAY; neutral empty is [].
    echo '[]'
    exit 0
    ;;
  status)
    # Index freshness/health normalizes to a JSON MAP; neutral empty is {}.
    echo '{}'
    exit 0
    ;;
  selftest)
    # Shape selftest: emit the canonical neutral shape for <verb> with no
    # provider installed. Lets a shape lint validate this adapter offline.
    case "${2:-}" in
      symbols|impact|affected|routes) echo '[]'; exit 0 ;;
      status) echo '{}'; exit 0 ;;
      *) echo "[none][ERROR] selftest requires a known verb" >&2; exit 1 ;;
    esac
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
none.sh — no-op code-index adapter (framework default)
Usage: none.sh <verb> [args...]
Verbs: symbols <query> (-> []) | impact <symbol> (-> []) |
       affected <file>... (-> []) | routes (-> []) | status (-> {})
       selftest <verb> (-> canonical neutral shape)
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
