#!/usr/bin/env bash
set -euo pipefail

cd /workspace
echo "[py-integration] starting pip install -e ./ml[dev]"
PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore python -m pip install --no-cache-dir -e ./ml[dev]
echo "[py-integration] pip install OK; starting live integration pytest"
pytest -q -m integration ml/tests/integration
echo "[py-integration] live integration pytest finished OK"
