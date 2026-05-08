#!/usr/bin/env bash
# scripts/commands/deploy_target.sh — dispatcher for ./smackerel.sh deploy-target
#
# Usage:
#   ./smackerel.sh deploy-target <target> <action> [--flags...]
#
# Adapter resolution (per .github/instructions/bubbles-deployment-target.instructions.md):
#   - If DEPLOY_TARGETS_ROOT is set:
#       only ${DEPLOY_TARGETS_ROOT}/smackerel/<target>/ is consulted; FAIL if missing.
#   - Else (DEPLOY_TARGETS_ROOT unset):
#       only <repo>/deploy/<target>/ is consulted; FAIL if missing.
#   No silent fallback between modes.
#
# Targets: any directory matching the resolution rule above.
# Actions: preconditions | bootstrap | apply | rollback | verify | teardown |
#          status | manifest | params
#
# Adapter contract (per bubbles G074): each action delegates to <adapter>/<action>.sh.
# This dispatcher MUST NOT inline target-specific logic. All target knobs live in
# <adapter>/params.yaml.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IN_TREE_DEPLOY_ROOT="$REPO_ROOT/deploy"

# Strict adapter directory resolution. Honors DEPLOY_TARGETS_ROOT as an explicit
# operator opt-in to out-of-tree adapters; refuses to silently fall back to
# in-tree when that env var is set.
resolve_adapter_dir() {
  local target="$1"
  local in_tree="$IN_TREE_DEPLOY_ROOT/$target"

  if [[ -n "${DEPLOY_TARGETS_ROOT:-}" ]]; then
    local out_of_tree="${DEPLOY_TARGETS_ROOT%/}/smackerel/${target}"
    if [[ -f "${out_of_tree}/params.yaml" ]]; then
      echo "$out_of_tree"
      return 0
    fi
    cat >&2 <<EOF
ERROR: deploy-target adapter not found for '${target}'.
  DEPLOY_TARGETS_ROOT is set to: ${DEPLOY_TARGETS_ROOT}
  Tried (out-of-tree):           ${out_of_tree}/params.yaml  [missing]
  NOT consulted (in-tree):       ${in_tree}/params.yaml
  Setting DEPLOY_TARGETS_ROOT is an explicit opt-in to out-of-tree adapters.
  This CLI refuses to silently fall back to the in-tree adapter.
  Either populate ${out_of_tree}/ or unset DEPLOY_TARGETS_ROOT.
EOF
    return 1
  fi

  if [[ -f "${in_tree}/params.yaml" ]]; then
    echo "$in_tree"
    return 0
  fi

  cat >&2 <<EOF
ERROR: deploy-target adapter not found for '${target}'.
  Tried (in-tree): ${in_tree}/params.yaml  [missing]
  Hint: copy ${IN_TREE_DEPLOY_ROOT}/_example/<target-skeleton>/ as a starting point,
        or set DEPLOY_TARGETS_ROOT and create the adapter under
        \${DEPLOY_TARGETS_ROOT}/smackerel/${target}/.
EOF
  return 1
}

list_targets() {
  echo "Targets available (in-tree):"
  if [[ -d "$IN_TREE_DEPLOY_ROOT" ]]; then
    local d name
    for d in "$IN_TREE_DEPLOY_ROOT"/*/; do
      [[ -d "$d" ]] || continue
      name="$(basename "$d")"
      [[ "$name" == "_example" ]] && continue
      echo "  $name"
    done
  fi
  if [[ -n "${DEPLOY_TARGETS_ROOT:-}" ]]; then
    local out_root="${DEPLOY_TARGETS_ROOT%/}/smackerel"
    echo "Targets available (out-of-tree, DEPLOY_TARGETS_ROOT=${DEPLOY_TARGETS_ROOT}):"
    if [[ -d "$out_root" ]]; then
      local d
      for d in "$out_root"/*/; do
        [[ -d "$d" ]] || continue
        echo "  $(basename "$d")"
      done
    else
      echo "  (no directory at ${out_root})"
    fi
  else
    echo "(set DEPLOY_TARGETS_ROOT to use operator-private out-of-tree adapters)"
  fi
}

usage() {
  cat <<EOF
Usage: ./smackerel.sh deploy-target <target> <action> [args]

$(list_targets)

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
  manifest         Print current <adapter>/manifest.yaml
  params           Print <adapter>/params.yaml

Adapter resolution: when DEPLOY_TARGETS_ROOT is set, only
  \${DEPLOY_TARGETS_ROOT}/smackerel/<target>/ is consulted.
Otherwise only <repo>/deploy/<target>/ is consulted.
No silent fallback between modes.
EOF
}

if [[ $# -lt 2 ]]; then
  usage
  exit 1
fi

TARGET="$1"
ACTION="$2"
shift 2

if ! TARGET_DIR="$(resolve_adapter_dir "$TARGET")"; then
  exit 1
fi

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
