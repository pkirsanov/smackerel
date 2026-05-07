#!/usr/bin/env bash
set -euo pipefail

cd /workspace
PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore python -m pip install --no-cache-dir -e ./ml[dev]

args=()
if [[ "${1:-}" == "--check" ]]; then
  args+=(--check)
fi

python -m ruff format "${args[@]}" ml/app ml/tests