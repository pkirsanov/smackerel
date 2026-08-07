#!/usr/bin/env bash
# Neutral experience-recall adapter. It never reads, stores, or recalls data.

set -euo pipefail

VERB="${1:-}"

case "$VERB" in
  search | export)
    echo '[]'
    ;;
  read | status | freshness | sync | delete | capabilities)
    echo '{}'
    ;;
  selftest)
    case "${2:-}" in
      search | export) echo '[]' ;;
      read | status | freshness | sync | delete | capabilities) echo '{}' ;;
      *)
        echo "[experience-recall:none][ERROR] selftest requires a known verb" >&2
        exit 1
        ;;
    esac
    ;;
  -h | --help | '')
    cat >&2 <<'EOF'
none.sh - neutral experience-recall adapter
Usage: none.sh <verb> [args...]
Verbs: search | read | status | freshness | sync | export | delete |
       capabilities | selftest <verb>
EOF
    ;;
  *)
    echo "[experience-recall:none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
