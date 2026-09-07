#!/usr/bin/env bash
# Thin epoch facade. Only a schema-valid hostProof inside the input can prove a boundary.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SCRIPT_DIR/measured-budget-runtime.py"

usage() {
  cat >&2 <<'EOF'
Usage: session-epoch-authority.sh <boundary|open|verify|close> --store-root PATH --input FILE

Caller declarations such as fresh-context or compacted are intentionally not
accepted. Boundary authority is verified by the MBE engine from hostProof.
EOF
}

[[ $# -gt 0 ]] || { usage; exit 2; }
operation="$1"
shift
case "$operation" in
  open|verify|close) command_name="epoch-$operation" ;;
  boundary) command_name="epoch-boundary" ;;
  -h|--help) usage; exit 0 ;;
  *) echo "session-epoch-authority: unsupported operation '$operation'" >&2; exit 2 ;;
esac
store_root=""
input_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --store-root) [[ $# -ge 2 ]] || { usage; exit 2; }; store_root="$2"; shift 2 ;;
    --input) [[ $# -ge 2 ]] || { usage; exit 2; }; input_file="$2"; shift 2 ;;
    --fresh-context|--compacted|--model-declared-fresh)
      echo "session-epoch-authority: caller declarations are not host proof" >&2
      exit 2
      ;;
    *) echo "session-epoch-authority: unknown option '$1'" >&2; exit 2 ;;
  esac
done
[[ -n "$store_root" && -n "$input_file" ]] || { usage; exit 2; }
exec python3 "$ENGINE" "$command_name" --store-root "$store_root" --input "$input_file"