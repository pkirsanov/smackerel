#!/usr/bin/env bash
# Pure admission facade over measured-budget-runtime.py. It performs no prose inference.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE="$SCRIPT_DIR/measured-budget-runtime.py"

usage() {
  cat >&2 <<'EOF'
Usage: dispatch-admission.sh evaluate --store-root PATH --input FILE
       dispatch-admission.sh issue-permit --store-root PATH --input FILE
  dispatch-admission.sh <record-usage|record-intent|record-fact|consume-permit> --store-root PATH --input FILE

FILE must contain the complete typed, digest-bound fact object required by the
MBE engine. Missing facts fail loud; this facade supplies no inferred defaults.
EOF
}

[[ $# -gt 0 ]] || { usage; exit 2; }
operation="$1"
shift
case "$operation" in
  evaluate) command_name="admission-evaluate" ;;
  issue-permit) command_name="permit-issue" ;;
  record-usage) command_name="usage-record" ;;
  record-intent) command_name="dispatch-intent" ;;
  record-fact) command_name="admission-fact" ;;
  consume-permit) command_name="permit-consume" ;;
  -h|--help) usage; exit 0 ;;
  *) echo "dispatch-admission: unsupported operation '$operation'" >&2; exit 2 ;;
esac
store_root=""
input_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --store-root) [[ $# -ge 2 ]] || { usage; exit 2; }; store_root="$2"; shift 2 ;;
    --input) [[ $# -ge 2 ]] || { usage; exit 2; }; input_file="$2"; shift 2 ;;
    *) echo "dispatch-admission: unknown option '$1'" >&2; exit 2 ;;
  esac
done
[[ -n "$store_root" && -n "$input_file" ]] || { usage; exit 2; }
exec python3 "$ENGINE" "$command_name" --store-root "$store_root" --input "$input_file"