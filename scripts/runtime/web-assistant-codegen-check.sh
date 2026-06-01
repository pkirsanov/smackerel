#!/usr/bin/env bash
# Spec 073 Scope 1c — runs `cmd/web-assistant-codegen --check` inside
# the Go tooling container. Fails fail-loud if
# web/pwa/generated/assistant_turn_v1.{js,d.ts} differ from the bytes
# regenerated from internal/assistant/schema/assistant_turn_v1.json.
#
# Invoked by `./smackerel.sh check` and by `./smackerel.sh build` to
# keep the committed artifacts in lockstep with the canonical
# wire-schema. Operators regenerate with:
#   go run ./cmd/web-assistant-codegen
set -euo pipefail

cd /workspace

go run ./cmd/web-assistant-codegen --check
