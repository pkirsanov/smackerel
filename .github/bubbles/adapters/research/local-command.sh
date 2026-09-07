#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
runtime="$script_dir/../../scripts/research-runtime.py"

if ! command -v python3 >/dev/null 2>&1; then
  printf '%s\n' '{"code":"RER-ROUTE-UNAVAILABLE","contractType":"research-error","errorClass":"dependency","message":"python3 is unavailable","schemaVersion":1}' >&2
  exit 2
fi

exec python3 "$runtime" adapter-local-command "$@"
