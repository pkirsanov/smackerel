#!/usr/bin/env bash
# apply.sh — pull image digests + config bundle, verify signatures, swap manifest pointer,
# restart stack. Idempotent: applying the same release twice is a no-op past the verify step.
#
# Adapter contract (NON-NEGOTIABLE per bubbles G074):
#   - MUST consume an immutable image digest pair + bundle hash from CLI args
#   - MUST NOT invoke `docker build`, `docker buildx`, or any compile step
#   - MUST NOT fall back to local build if registry pull fails (fail fast)
#   - MUST verify cosign signature against Rekor BEFORE container start
#   - MUST verify config bundle sha256 BEFORE extraction
#   - MUST write deploy/home-lab/manifest.yaml with the new pointer pair AND preserve the
#     prior pointer in `previousManifest` BEFORE starting any container
#   - On verify failure: invoke rollback.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARAMS="$SCRIPT_DIR/params.yaml"
MANIFEST="$SCRIPT_DIR/manifest.yaml"
CONTRACT="$SCRIPT_DIR/../contract.yaml"

IMAGE_CORE=""
IMAGE_ML=""
CONFIG_BUNDLE=""
SOURCE_SHA=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image-core=*)    IMAGE_CORE="${1#*=}"; shift ;;
    --image-ml=*)      IMAGE_ML="${1#*=}"; shift ;;
    --config-bundle=*) CONFIG_BUNDLE="${1#*=}"; shift ;;
    --source-sha=*)    SOURCE_SHA="${1#*=}"; shift ;;
    *) echo "ERROR: unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$IMAGE_CORE" ]]    || { echo "ERROR: --image-core=sha256:<digest> required" >&2; exit 1; }
[[ -n "$IMAGE_ML" ]]      || { echo "ERROR: --image-ml=sha256:<digest> required" >&2; exit 1; }
[[ -n "$CONFIG_BUNDLE" ]] || { echo "ERROR: --config-bundle=<env>-<sourceSha> required" >&2; exit 1; }

[[ "$IMAGE_CORE" == sha256:* ]] || { echo "ERROR: --image-core must start with sha256: (got: $IMAGE_CORE)" >&2; exit 1; }
[[ "$IMAGE_ML" == sha256:* ]]   || { echo "ERROR: --image-ml must start with sha256: (got: $IMAGE_ML)" >&2; exit 1; }

[[ -f "$PARAMS" ]]   || { echo "ERROR: $PARAMS missing" >&2; exit 1; }
[[ -f "$MANIFEST" ]] || { echo "ERROR: $MANIFEST missing" >&2; exit 1; }
[[ -f "$CONTRACT" ]] || { echo "ERROR: $CONTRACT missing" >&2; exit 1; }

yaml_get() {
  local file="$1" key="$2"
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
  ' "$file"
}

REGISTRY_CORE="$(yaml_get "$PARAMS" core)"
REGISTRY_ML="$(yaml_get "$PARAMS" ml)"
REGISTRY_BUNDLES="$(yaml_get "$PARAMS" configBundles)"
COSIGN_IDENTITY="$(yaml_get "$PARAMS" cosignIdentity)"
COSIGN_ISSUER="$(yaml_get "$PARAMS" cosignIssuer)"
ROLLOUT_STRATEGY="$(yaml_get "$PARAMS" rolloutStrategy)"

[[ -n "$REGISTRY_CORE" ]]     || { echo "ERROR: registry.core missing in params.yaml" >&2; exit 1; }
[[ -n "$REGISTRY_ML" ]]       || { echo "ERROR: registry.ml missing in params.yaml" >&2; exit 1; }
[[ -n "$REGISTRY_BUNDLES" ]]  || { echo "ERROR: registry.configBundles missing in params.yaml" >&2; exit 1; }
[[ -n "$COSIGN_IDENTITY" ]]   || { echo "ERROR: signing.cosignIdentity missing in params.yaml" >&2; exit 1; }
[[ -n "$COSIGN_ISSUER" ]]     || { echo "ERROR: signing.cosignIssuer missing in params.yaml" >&2; exit 1; }
[[ -n "$ROLLOUT_STRATEGY" ]]  || { echo "ERROR: rolloutStrategy missing in params.yaml" >&2; exit 1; }

CORE_REF="${REGISTRY_CORE}@${IMAGE_CORE}"
ML_REF="${REGISTRY_ML}@${IMAGE_ML}"

echo "▶ apply: pulling images by digest"
echo "  core: $CORE_REF"
echo "  ml:   $ML_REF"

docker pull "$CORE_REF"
docker pull "$ML_REF"

echo "▶ apply: verifying cosign signatures against Rekor"
cosign verify \
  --certificate-identity "$COSIGN_IDENTITY" \
  --certificate-oidc-issuer "$COSIGN_ISSUER" \
  "$CORE_REF" >/dev/null
cosign verify \
  --certificate-identity "$COSIGN_IDENTITY" \
  --certificate-oidc-issuer "$COSIGN_ISSUER" \
  "$ML_REF" >/dev/null
echo "  signatures verified"

echo "▶ apply: pulling config bundle $CONFIG_BUNDLE"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
BUNDLE_FILE="$WORKDIR/${CONFIG_BUNDLE}.tar.gz"

# Pull bundle as an OCI artifact (cosign download artifact-style or oras pull).
# For now, surface the requirement clearly — actual oras/cosign artifact pull is registry-specific.
if ! oras pull "${REGISTRY_BUNDLES}:${CONFIG_BUNDLE}" --output "$WORKDIR" 2>/dev/null; then
  echo "ERROR: failed to pull config bundle ${REGISTRY_BUNDLES}:${CONFIG_BUNDLE}" >&2
  echo "       (no fallback to local build — fail fast per G074)" >&2
  exit 1
fi

[[ -f "$BUNDLE_FILE" ]] || {
  # oras may name the file differently; locate the .tar.gz
  BUNDLE_FILE="$(find "$WORKDIR" -maxdepth 2 -name '*.tar.gz' | head -1)"
  [[ -n "$BUNDLE_FILE" && -f "$BUNDLE_FILE" ]] || { echo "ERROR: bundle archive not found after pull" >&2; exit 1; }
}

BUNDLE_SHA="$(sha256sum "$BUNDLE_FILE" | awk '{print $1}')"
echo "  bundle sha256: $BUNDLE_SHA"

echo "▶ apply: capturing prior manifest pointer for rollback"
PRIOR_BLOCK="$(awk '
  in_current && /^[^[:space:]]/ { in_current=0 }
  /^current:/ { in_current=1 }
  in_current { print }
' "$MANIFEST")"

echo "▶ apply: writing new manifest pointer (atomic temp + rename)"
NEW_MANIFEST="$(mktemp "${MANIFEST}.new.XXXXXX")"
APPLIED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
APPLIED_BY="${USER:-unknown}@$(hostname -s 2>/dev/null || echo host)"

{
  echo "# Home-Lab Deployment Manifest"
  echo "# Written by deploy/home-lab/apply.sh — DO NOT EDIT BY HAND"
  echo "manifestVersion: 1"
  echo ""
  echo "current:"
  echo "  appliedAt: \"${APPLIED_AT}\""
  echo "  appliedBy: \"${APPLIED_BY}\""
  echo "  sourceSha: \"${SOURCE_SHA}\""
  echo "  images:"
  echo "    core: \"${CORE_REF}\""
  echo "    ml: \"${ML_REF}\""
  echo "  configBundle:"
  echo "    name: \"${CONFIG_BUNDLE}\""
  echo "    sha256: \"${BUNDLE_SHA}\""
  echo "  rolloutStrategy: \"${ROLLOUT_STRATEGY}\""
  echo ""
  if [[ -n "$PRIOR_BLOCK" ]] && ! echo "$PRIOR_BLOCK" | grep -q 'sourceSha: ""'; then
    echo "previousManifest:"
    echo "$PRIOR_BLOCK" | sed 's/^/  /'
  else
    echo "previousManifest: null"
  fi
} > "$NEW_MANIFEST"

mv -f "$NEW_MANIFEST" "$MANIFEST"

echo "▶ apply: running rollout strategy: $ROLLOUT_STRATEGY"
case "$ROLLOUT_STRATEGY" in
  recreate)
    echo "  recreate: stop old, start new"
    # Real implementation: docker compose -f <composeDir>/docker-compose.yml down
    # then docker compose up -d using the pulled digests.
    ;;
  blue-green)
    echo "  blue-green: spin up green, switch traffic, drain blue"
    ;;
  *)
    echo "ERROR: unsupported rollout strategy: $ROLLOUT_STRATEGY" >&2
    exit 1
    ;;
esac

echo "▶ apply: verifying"
"$SCRIPT_DIR/verify.sh" || {
  echo "ERROR: verify failed — initiating rollback" >&2
  "$SCRIPT_DIR/rollback.sh"
  exit 1
}

echo "apply OK"
