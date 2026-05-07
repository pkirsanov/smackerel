#!/usr/bin/env bash
# scripts/deploy/promote.sh — promote a build artifact set to a deploy target.
#
# Reads a build manifest (produced by .github/workflows/build.yml), extracts the
# image digests + config bundle ref for the requested target environment, and
# invokes the per-target adapter `apply` action.
#
# Usage:
#   bash scripts/deploy/promote.sh \
#       --target home-lab \
#       --build-manifest path/to/build-manifest.yaml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

TARGET=""
MANIFEST=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)         TARGET="$2"; shift 2 ;;
    --target=*)       TARGET="${1#*=}"; shift ;;
    --build-manifest) MANIFEST="$2"; shift 2 ;;
    --build-manifest=*) MANIFEST="${1#*=}"; shift ;;
    *) echo "ERROR: unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$TARGET" ]]   || { echo "ERROR: --target required" >&2; exit 1; }
[[ -n "$MANIFEST" ]] || { echo "ERROR: --build-manifest required" >&2; exit 1; }
[[ -f "$MANIFEST" ]] || { echo "ERROR: build manifest not found: $MANIFEST" >&2; exit 1; }

TARGET_PARAMS="$REPO_ROOT/deploy/$TARGET/params.yaml"
[[ -f "$TARGET_PARAMS" ]] || { echo "ERROR: $TARGET_PARAMS missing" >&2; exit 1; }

# Read target environment from params.yaml
TARGET_ENV="$(awk '/^environment:/ { sub(/^[^:]+:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); print; exit }' "$TARGET_PARAMS")"
[[ -n "$TARGET_ENV" ]] || { echo "ERROR: environment missing in $TARGET_PARAMS" >&2; exit 1; }

# Extract image refs + bundle ref from build manifest (simple yaml grep)
SOURCE_SHA="$(awk '/^sourceSha:/ { sub(/^[^:]+:[[:space:]]*/, ""); print; exit }' "$MANIFEST")"
CORE_REF="$(awk '/^[[:space:]]*- name: smackerel-core/ { found=1; next } found && /^[[:space:]]*ref:/ { sub(/^[^:]+:[[:space:]]*/, ""); print; exit }' "$MANIFEST")"
ML_REF="$(awk '/^[[:space:]]*- name: smackerel-ml/ { found=1; next } found && /^[[:space:]]*ref:/ { sub(/^[^:]+:[[:space:]]*/, ""); print; exit }' "$MANIFEST")"
BUNDLE_REF="$(awk -v env="$TARGET_ENV" '/^[[:space:]]*- env: / { found=($3==env); next } found && /^[[:space:]]*ref:/ { sub(/^[^:]+:[[:space:]]*/, ""); print; exit }' "$MANIFEST")"

[[ -n "$SOURCE_SHA" ]] || { echo "ERROR: sourceSha missing in $MANIFEST" >&2; exit 1; }
[[ -n "$CORE_REF" ]]   || { echo "ERROR: smackerel-core ref missing in $MANIFEST" >&2; exit 1; }
[[ -n "$ML_REF" ]]     || { echo "ERROR: smackerel-ml ref missing in $MANIFEST" >&2; exit 1; }
[[ -n "$BUNDLE_REF" ]] || { echo "ERROR: bundle ref for env=$TARGET_ENV missing in $MANIFEST" >&2; exit 1; }

# Extract just the digest portion (after @)
CORE_DIGEST="${CORE_REF##*@}"
ML_DIGEST="${ML_REF##*@}"
# Extract just the bundle tag (after :)
BUNDLE_TAG="${BUNDLE_REF##*:}"

echo "▶ promote: target=$TARGET env=$TARGET_ENV sourceSha=$SOURCE_SHA"
echo "  coreDigest:    $CORE_DIGEST"
echo "  mlDigest:      $ML_DIGEST"
echo "  configBundle:  $BUNDLE_TAG"

exec "$REPO_ROOT/smackerel.sh" deploy-target "$TARGET" apply \
  --image-core="$CORE_DIGEST" \
  --image-ml="$ML_DIGEST" \
  --config-bundle="$BUNDLE_TAG" \
  --source-sha="$SOURCE_SHA"
