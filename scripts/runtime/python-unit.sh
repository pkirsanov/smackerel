#!/usr/bin/env bash
set -euo pipefail

cd /workspace
PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore python -m pip install --no-cache-dir -e ./ml[dev]
pytest ml/tests -q