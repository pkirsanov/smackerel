#!/usr/bin/env bash
# Local deterministic experience-recall provider.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
INDEXER="$FRAMEWORK_ROOT/scripts/experience-recall-index.py"
VERB="${1:-}"

run_indexer() {
  command -v python3 >/dev/null 2>&1 || {
    echo "[experience-recall:local-lexical][ERROR] python3 is required" >&2
    exit 1
  }
  [[ -f "$INDEXER" ]] || {
    echo "[experience-recall:local-lexical][ERROR] indexer not found: $INDEXER" >&2
    exit 1
  }
  exec python3 "$INDEXER" "$@"
}

case "$VERB" in
  search | read | status | freshness | sync)
    shift
    run_indexer "$VERB" "$@"
    ;;
  capabilities)
    shift
    run_indexer capabilities "$@"
    ;;
  export)
    echo '[]'
    echo "[experience-recall:local-lexical][ERROR] export is unsupported" >&2
    exit 1
    ;;
  delete)
    echo '{"reason":"unsupported","supported":false}'
    echo "[experience-recall:local-lexical][ERROR] delete is unsupported" >&2
    exit 1
    ;;
  -h | --help | '')
    cat >&2 <<'EOF'
local-lexical.sh - deterministic experience-recall adapter
Usage: local-lexical.sh <verb> [args...]
Verbs: search | read | status | freshness | sync | export | delete |
       capabilities
EOF
    ;;
  *)
    echo "[experience-recall:local-lexical][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
