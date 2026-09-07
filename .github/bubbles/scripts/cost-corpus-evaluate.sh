#!/usr/bin/env bash
# Thin operator facade for IMP-055 frozen-corpus sealing and evaluation.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNTIME="$SCRIPT_DIR/measured-budget-runtime.py"

usage() {
  printf 'Usage: %s <seal|evaluate> --store-root <absolute-path> --input <absolute-json-path>\n' "${0##*/}"
}

[[ $# -eq 5 ]] || {
  usage >&2
  exit 2
}

operation="$1"
shift
[[ "$1" == "--store-root" && "$3" == "--input" ]] || {
  usage >&2
  exit 2
}
store_root="$2"
input="$4"

case "$operation" in
  seal) command_name="corpus-seal" ;;
  evaluate) command_name="corpus-evaluate" ;;
  *)
    printf 'cost-corpus-evaluate: unsupported operation: %s\n' "$operation" >&2
    exit 2
    ;;
esac

[[ "$store_root" == /* && "$input" == /* ]] || {
  printf 'cost-corpus-evaluate: store and input paths must be absolute\n' >&2
  exit 2
}

exec python3 "$RUNTIME" "$command_name" --store-root "$store_root" --input "$input"