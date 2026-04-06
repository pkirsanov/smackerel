#!/usr/bin/env bash
set -euo pipefail

cd /workspace
python -m pip install --no-cache-dir -e ./ml[dev]

args=()
if [[ "${1:-}" == "--check" ]]; then
  args+=(--check)
fi

python -m ruff format "${args[@]}" ml/app ml/tests