#!/usr/bin/env bash
set -euo pipefail

: "${SMACKEREL_E2E_LEAK_MARKER:?SMACKEREL_E2E_LEAK_MARKER is required}"

bash -c 'exec -a "$0" bash -c '\''trap "" TERM INT HUP; while :; do sleep 1; done'\''' "$SMACKEREL_E2E_LEAK_MARKER" &
wait "$!"