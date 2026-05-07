#!/usr/bin/env bash
# scripts/commands/deploy_target.sh — dispatcher for ./smackerel.sh deploy-target
#
# Usage:
#   ./smackerel.sh deploy-target <target> <action> [--flags...]
#
# Targets: home-lab (and any future deploy/<target>/ folder)
# Actions: preconditions | bootstrap | apply | rollback | verify | teardown |
#          status | manifest | params
#
# Adapter contract (per bubbles G074): each action delegates to deploy/<target>/<action>.sh.
# This dispatcher MUST NOT inline target-specific logic. All target knobs live in
# deploy/<target>/params.yaml.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEPLOY_ROOT="$REPO_ROOT/deploy"

usage() {
  cat <<EOF
Usage: ./smackerel.sh deploy-target <target> <action> [args]

Targets:
$(ls -1 "$DEPLOY_ROOT" 2>/dev/null | grep -v '\.\(yaml\|md\)$' | sed 's/^/  /' || echo "  (none)")

Actions:
  preconditions    Verify host has required tools and paths (read-only)
  bootstrap        One-time host setup
  apply            Pull images by digest, verify signatures, swap manifest, restart
                   args: --image-core=sha256:<digest>
                         --image-ml=sha256:<digest>
                         --config-bundle=<env>-<sourceSha>
                         [--source-sha=<sha>]
  rollback         Pointer-swap to previousManifest (no rebuild)
  verify           Post-deploy health checks (read-only)
  teardown         Remove what bootstrap/apply created
  status           Show stack status
  manifest         Print current deploy/<target>/manifest.yaml
  params           Print deploy/<target>/params.yaml
EOF
}

if [[ $# -lt 2 ]]; then
  usage
  exit 1
fi

TARGET="$1"
ACTION="$2"
shift 2

TARGET_DIR="$DEPLOY_ROOT/$TARGET"
[[ -d "$TARGET_DIR" ]] || {
  echo "ERROR: unknown target '$TARGET' (no $TARGET_DIR)" >&2
  echo "Available targets:" >&2
  ls -1 "$DEPLOY_ROOT" 2>/dev/null | grep -v '\.\(yaml\|md\)$' | sed 's/^/  /' >&2
  exit 1
}

case "$ACTION" in
  preconditions|bootstrap|apply|rollback|verify|teardown)
    SCRIPT="$TARGET_DIR/${ACTION}.sh"
    [[ -x "$SCRIPT" ]] || {
      echo "ERROR: $SCRIPT missing or not executable" >&2
      exit 1
    }
    exec "$SCRIPT" "$@"
    ;;
  status)
    docker ps --filter "label=com.docker.compose.project=smackerel-${TARGET}" \
      --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
    ;;
  manifest)
    MANIFEST="$TARGET_DIR/manifest.yaml"
    [[ -f "$MANIFEST" ]] || { echo "ERROR: $MANIFEST missing" >&2; exit 1; }
    cat "$MANIFEST"
    ;;
  params)
    PARAMS="$TARGET_DIR/params.yaml"
    [[ -f "$PARAMS" ]] || { echo "ERROR: $PARAMS missing" >&2; exit 1; }
    cat "$PARAMS"
    ;;
  help|--help|-h)
    usage
    ;;
  *)
    echo "ERROR: unknown action '$ACTION'" >&2
    usage
    exit 1
    ;;
esac
