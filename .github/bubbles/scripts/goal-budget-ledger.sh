#!/usr/bin/env bash
# Thin CLI facade over the immutable IMP-055 measured-budget domain engine.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SCRIPT_DIR/measured-budget-runtime.py"

usage() {
  cat >&2 <<'EOF'
Usage: goal-budget-ledger.sh <open|snapshot|reserve|debit|release|hold|correct|settle|retry-decide|close> --store-root PATH --input FILE

The input is a JSON object accepted by the corresponding MBE-1 operation.
Canonical output and stable exit codes come directly from the domain engine.
EOF
}

[[ $# -gt 0 ]] || { usage; exit 2; }
operation="$1"
shift
case "$operation" in
  open) command_name="budget-open" ;;
  snapshot) command_name="snapshot" ;;
  reserve|debit|release|hold|correct|close) command_name="$operation" ;;
  settle) command_name="budget-settle" ;;
  retry-decide) command_name="retry-decide" ;;
  -h|--help) usage; exit 0 ;;
  *) echo "goal-budget-ledger: unsupported operation '$operation'" >&2; exit 2 ;;
esac

store_root=""
input_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --store-root) [[ $# -ge 2 ]] || { usage; exit 2; }; store_root="$2"; shift 2 ;;
    --input) [[ $# -ge 2 ]] || { usage; exit 2; }; input_file="$2"; shift 2 ;;
    *) echo "goal-budget-ledger: unknown option '$1'" >&2; exit 2 ;;
  esac
done
[[ -n "$store_root" && -n "$input_file" ]] || { usage; exit 2; }
[[ -f "$ENGINE" ]] || { echo "goal-budget-ledger: measured-budget engine unavailable" >&2; exit 2; }
exec python3 "$ENGINE" "$command_name" --store-root "$store_root" --input "$input_file"