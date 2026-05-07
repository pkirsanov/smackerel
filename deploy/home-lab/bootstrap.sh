#!/usr/bin/env bash
# bootstrap.sh — one-time host preparation. Idempotent: safe to re-run.
#
# Creates the install root, manifest dir, and compose dir. Does NOT pull artifacts and does
# NOT start the stack. After bootstrap, an operator runs `apply` to install a specific
# release.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARAMS="$SCRIPT_DIR/params.yaml"

[[ -f "$PARAMS" ]] || { echo "ERROR: $PARAMS missing" >&2; exit 1; }

# Minimal yaml-get for a flat sub-tree (key.path: value)
yaml_get() {
  local key="$1"
  awk -v k="$key" '
    BEGIN { found=0 }
    /^[[:space:]]*#/ { next }
    {
      line=$0
      sub(/^[[:space:]]+/, "", line)
      if (line == k":" || line ~ "^"k":[[:space:]]") {
        sub(/^[^:]+:[[:space:]]*/, "", line)
        sub(/[[:space:]]*#.*$/, "", line)
        print line
        found=1
        exit
      }
    }
  ' "$PARAMS"
}

INSTALL_ROOT="$(yaml_get installRoot)"
MANIFEST_DIR="$(yaml_get manifestDir)"
COMPOSE_DIR="$(yaml_get composeDir)"

[[ -n "$INSTALL_ROOT" ]] || { echo "ERROR: host.installRoot missing in params.yaml" >&2; exit 1; }
[[ -n "$MANIFEST_DIR" ]] || { echo "ERROR: host.manifestDir missing in params.yaml" >&2; exit 1; }
[[ -n "$COMPOSE_DIR" ]] || { echo "ERROR: host.composeDir missing in params.yaml" >&2; exit 1; }

mkdir -p "$INSTALL_ROOT" "$MANIFEST_DIR" "$COMPOSE_DIR"

echo "bootstrap OK"
echo "  installRoot:  $INSTALL_ROOT"
echo "  manifestDir:  $MANIFEST_DIR"
echo "  composeDir:   $COMPOSE_DIR"
echo ""
echo "Next: ./smackerel.sh deploy-target home-lab apply --image-core=<digest> --image-ml=<digest> --config-bundle=<bundle>"
