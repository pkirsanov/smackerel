#!/usr/bin/env bash
set -euo pipefail

cd /workspace
python -m pip install --no-cache-dir -e ./ml[dev]
python -m ruff check ml/app ml/tests