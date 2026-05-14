#!/usr/bin/env bash
# spec-047 R7: trace mode (`x`) added to surface the exact failing
# command in CI logs even when only step-level metadata is available
# externally. No secrets pass through this script — only pip + pytest
# invocations — so command tracing is safe to leave on in CI.
set -euxo pipefail

cd /workspace
echo "[py-unit] starting pip install -e ./ml[dev]"
PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore python -m pip install --no-cache-dir -e ./ml[dev]
echo "[py-unit] pip install OK; starting pytest ml/tests"
pytest ml/tests -q
echo "[py-unit] pytest ml/tests finished OK"