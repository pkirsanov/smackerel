#!/usr/bin/env bash
# teardown.sh — removes ONLY what bootstrap/apply created on this host.
#
# Adapter contract:
#   - MUST NOT touch host singletons not owned by this target (Caddy, ufw, system Docker daemon)
#   - MUST be reversible: re-running bootstrap then apply must restore a working install
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARAMS="$SCRIPT_DIR/params.yaml"

[[ -f "$PARAMS" ]] || { echo "ERROR: $PARAMS missing" >&2; exit 1; }

yaml_get() {
  local key="$1"
  awk -v k="$key" '
    /^[[:space:]]*#/ { next }
    {
      line=$0
      sub(/^[[:space:]]+/, "", line)
      if (line == k":" || line ~ "^"k":[[:space:]]") {
        sub(/^[^:]+:[[:space:]]*/, "", line)
        sub(/[[:space:]]*#.*$/, "", line)
        print line
        exit
      }
    }
  ' "$PARAMS"
}

INSTALL_ROOT="$(yaml_get installRoot)"
MANIFEST_DIR="$(yaml_get manifestDir)"
COMPOSE_DIR="$(yaml_get composeDir)"

[[ -n "$INSTALL_ROOT" ]] || { echo "ERROR: host.installRoot missing in params.yaml" >&2; exit 1; }

echo "▶ teardown: stopping stack (best-effort)"
if [[ -d "$COMPOSE_DIR" ]] && [[ -f "$COMPOSE_DIR/docker-compose.yml" ]]; then
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" --project-name smackerel-home-lab down -v --remove-orphans || true
fi

echo "▶ teardown: removing install root: $INSTALL_ROOT"
rm -rf "$INSTALL_ROOT"

echo "teardown OK"
