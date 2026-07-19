#!/usr/bin/env bash
# spec-047 R7: trace mode (`x`) added to surface the exact failing
# command in CI logs even when only step-level metadata is available
# externally. No secrets pass through this script — only pip + pytest
# invocations — so command tracing is safe to leave on in CI.
set -euxo pipefail

cd /workspace
pytest_args=(-m "not integration and not live_ollama")
while [[ $# -gt 0 ]]; do
	case "$1" in
		--k)
			if [[ $# -lt 2 || -z "$2" ]]; then
				echo "ERROR: --k requires a non-empty pytest expression" >&2
				exit 1
			fi
			pytest_args+=(-k "$2")
			shift 2
			;;
		*)
			echo "Unknown python-unit option: $1" >&2
			exit 1
			;;
	esac
done
echo "[py-unit] starting pip install -e ./ml[dev]"
PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_ROOT_USER_ACTION=ignore python -m pip install --no-cache-dir -e ./ml[dev]
echo "[py-unit] pip install OK; starting unit-only pytest ml/tests"
pytest -q "${pytest_args[@]}" ml/tests
echo "[py-unit] pytest ml/tests finished OK"
