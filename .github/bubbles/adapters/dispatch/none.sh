#!/usr/bin/env bash
# Honest default-off dispatch adapter. It does not intercept or issue permits.
set -euo pipefail

case "${1:-}" in
  describe)
    printf '%s\n' '{"adapterId":"none","hostEnforcement":"unsupported","measurement":"unmeasured","permitIssuance":false,"postures":["shadow","advisory"]}'
    ;;
  admit|permit|consume|dispatch)
    printf '%s\n' '{"code":"MBE-HOST-ENFORCEMENT-UNAVAILABLE","contractType":"measured-budget-error","errorClass":"unsupported","message":"dispatch adapter none cannot issue or enforce a permit","schemaVersion":1}' >&2
    exit 3
    ;;
  -h|--help|'')
    echo 'Usage: none.sh describe'
    ;;
  *) echo "dispatch none: unknown verb '${1:-}'" >&2; exit 2 ;;
esac