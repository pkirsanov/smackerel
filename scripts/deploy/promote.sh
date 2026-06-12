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

# Spec 082 SCOPE-082-07 — dual-shape build-manifest extraction helpers.
# promote.sh now parses BOTH the CI list-shape manifest and the
# local-operator (build-home-lab.sh) map/object-shape manifest.
# shellcheck source=scripts/deploy/promote_manifest_parse.sh
source "$SCRIPT_DIR/promote_manifest_parse.sh"

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

# Strict adapter resolution per .github/instructions/bubbles-deployment-target.instructions.md.
# Honors DEPLOY_TARGETS_ROOT as an explicit operator opt-in to out-of-tree adapters;
# refuses to silently fall back to in-tree when that env var is set.
IN_TREE_PARAMS="$REPO_ROOT/deploy/$TARGET/params.yaml"
if [[ -n "${DEPLOY_TARGETS_ROOT:-}" ]]; then
  OUT_OF_TREE_PARAMS="${DEPLOY_TARGETS_ROOT%/}/smackerel/${TARGET}/params.yaml"
  if [[ -f "$OUT_OF_TREE_PARAMS" ]]; then
    TARGET_PARAMS="$OUT_OF_TREE_PARAMS"
  else
    cat >&2 <<EOF
ERROR: deploy-target adapter not found for '${TARGET}'.
  DEPLOY_TARGETS_ROOT is set to: ${DEPLOY_TARGETS_ROOT}
  Tried (out-of-tree):           ${OUT_OF_TREE_PARAMS}  [missing]
  NOT consulted (in-tree):       ${IN_TREE_PARAMS}
  Setting DEPLOY_TARGETS_ROOT is an explicit opt-in to out-of-tree adapters.
  promote.sh refuses to silently fall back to the in-tree adapter.
  Either populate the out-of-tree path or unset DEPLOY_TARGETS_ROOT.
EOF
    exit 1
  fi
else
  if [[ -f "$IN_TREE_PARAMS" ]]; then
    TARGET_PARAMS="$IN_TREE_PARAMS"
  else
    echo "ERROR: $IN_TREE_PARAMS missing (and DEPLOY_TARGETS_ROOT unset)" >&2
    exit 1
  fi
fi

# Read target environment from params.yaml
TARGET_ENV="$(awk '/^environment:/ { sub(/^[^:]+:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); print; exit }' "$TARGET_PARAMS")"
[[ -n "$TARGET_ENV" ]] || { echo "ERROR: environment missing in $TARGET_PARAMS" >&2; exit 1; }

# Extract image refs + bundle ref from build manifest. Spec 082 SCOPE-082-07 —
# these helpers parse BOTH the CI list shape AND the local-operator map/object
# shape, so a locally-built (build-home-lab.sh) manifest promotes through the
# same in-tree path as a CI manifest.
SOURCE_SHA="$(manifest_source_sha "$MANIFEST")"
CORE_REF="$(manifest_image_ref "$MANIFEST" smackerel-core)"
ML_REF="$(manifest_image_ref "$MANIFEST" smackerel-ml)"
BUNDLE_REF="$(manifest_bundle_ref "$MANIFEST" "$TARGET_ENV")"
# BUG-047-001 / DEVOPS-HL-002 — pluck the per-env bundle sha256 alongside the
# bundle ref. Both manifest shapes emit `sha256:` for the bundle; the adapter
# `apply.sh` MUST verify the pulled bundle's sha256 byte-for-byte against this
# value before mounting. Fail-loud if the value is missing — refusing to
# promote is safer than promoting an unverifiable bundle (the bundle-tamper
# bypass DEVOPS-HL-002 closed).
BUNDLE_SHA="$(manifest_bundle_sha "$MANIFEST" "$TARGET_ENV")"

[[ -n "$SOURCE_SHA" ]] || { echo "ERROR: sourceSha missing in $MANIFEST" >&2; exit 1; }
[[ -n "$CORE_REF" ]]   || { echo "ERROR: smackerel-core ref missing in $MANIFEST" >&2; exit 1; }
[[ -n "$ML_REF" ]]     || { echo "ERROR: smackerel-ml ref missing in $MANIFEST" >&2; exit 1; }
[[ -n "$BUNDLE_REF" ]] || { echo "ERROR: bundle ref for env=$TARGET_ENV missing in $MANIFEST" >&2; exit 1; }
[[ -n "$BUNDLE_SHA" ]] || { echo "ERROR: bundle sha256 for env=$TARGET_ENV missing in $MANIFEST (BUG-047-001 / DEVOPS-HL-002 — every configBundles entry MUST carry a sha256 field for adapter-side hash verification; refusing to promote without it)" >&2; exit 1; }
[[ "$BUNDLE_SHA" =~ ^[0-9a-f]{64}$ ]] || { echo "ERROR: bundle sha256 for env=$TARGET_ENV is not a valid sha256 hex digest in $MANIFEST: $BUNDLE_SHA" >&2; exit 1; }

# Extract just the digest portion (after @)
CORE_DIGEST="${CORE_REF##*@}"
ML_DIGEST="${ML_REF##*@}"
# Extract just the bundle tag (after :)
BUNDLE_TAG="${BUNDLE_REF##*:}"

# Spec 021 B1a — resolve the manifest trust model and forward it to the adapter.
# The legacy (C4) PATH-2 promote previously dropped --trust-model entirely, so a
# CI ci-keyless manifest misresolved to the adapter's stored params default
# (local-operator). manifest_trust_model returns ci-keyless when the manifest
# omits trustModel (spec-019 FR-6 inference) or the explicit value otherwise.
# smackerel.sh deploy-target apply forwards "$@" to the adapter, which has a
# --trust-model arm, so forwarding here threads the value end-to-end.
TRUST_MODEL="$(manifest_trust_model "$MANIFEST")"

echo "▶ promote: target=$TARGET env=$TARGET_ENV sourceSha=$SOURCE_SHA"
echo "  coreDigest:       $CORE_DIGEST"
echo "  mlDigest:         $ML_DIGEST"
echo "  configBundle:     $BUNDLE_TAG"
echo "  configBundleSha:  $BUNDLE_SHA"
echo "  trustModel:       $TRUST_MODEL"

exec "$REPO_ROOT/smackerel.sh" deploy-target "$TARGET" apply \
  --image-core="$CORE_DIGEST" \
  --image-ml="$ML_DIGEST" \
  --config-bundle="$BUNDLE_TAG" \
  --config-bundle-sha="$BUNDLE_SHA" \
  --source-sha="$SOURCE_SHA" \
  --trust-model="$TRUST_MODEL"
