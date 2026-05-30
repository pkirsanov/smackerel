#!/usr/bin/env bash
# scripts/commands/build-home-lab.sh
#
# Spec 017 scope 03: `./smackerel.sh build --target home-lab`
#
# Produces operator-key-signed images + SBOM attestations + Trivy gate
# + a local-build-manifest consumed by the knb deploy-adapter.
#
# Trust model: local-operator (single-operator home-lab). The operator's
# cosign key signs each image and the manifest itself.
#
# Required env:
#   OPERATOR_COSIGN_KEY     path to operator cosign private key
#                           (default $HOME/.config/knb/operator-keys/cosign-operator.key)
#   OPERATOR_COSIGN_PUBKEY  path to operator cosign public key (for adapter handoff)
#   COSIGN_PASSWORD         passphrase for the private key
#
# Outputs:
#   ghcr.io/pkirsanov/smackerel-core@sha256:<digest>  (pushed, signed, attested)
#   ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>    (pushed, signed, attested)
#   dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml
#
# Exit codes:
#   [F017-BUILD-01]  missing required env var (OPERATOR_COSIGN_KEY/PUBKEY/COSIGN_PASSWORD)
#   [F017-BUILD-02]  required CLI tool missing
#   [F017-BUILD-03]  docker build failed
#   [F017-BUILD-04]  Trivy gate found CRITICAL/HIGH vulnerability
#   [F017-BUILD-05]  cosign sign or attest failed
#   [F017-BUILD-06]  git working tree dirty AND --allow-dirty not passed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ALLOW_DIRTY=0
for arg in "$@"; do
  case "$arg" in
    --allow-dirty) ALLOW_DIRTY=1 ;;
    *) ;;
  esac
done

bhl_fail() {
  local code="$1"
  shift
  echo "ERROR: [$code] $*" >&2
  exit 1
}

bhl_require_cmd() {
  command -v "$1" >/dev/null 2>&1 \
    || bhl_fail F017-BUILD-02 "required command '$1' not found on PATH"
}

bhl_require_env() {
  local var_name="$1"
  local value="${!var_name:-}"
  [[ -n "$value" ]] \
    || bhl_fail F017-BUILD-01 "$var_name env var required for build --target home-lab"
}

# Required tools.
bhl_require_cmd docker
bhl_require_cmd cosign
bhl_require_cmd syft
bhl_require_cmd trivy
bhl_require_cmd git
bhl_require_cmd sha256sum

# Required env.
: "${OPERATOR_COSIGN_KEY:=$HOME/.config/knb/operator-keys/cosign-operator.key}"
: "${OPERATOR_COSIGN_PUBKEY:=$HOME/.config/knb/operator-keys/cosign-operator.pub}"
[[ -f "$OPERATOR_COSIGN_KEY" ]] \
  || bhl_fail F017-BUILD-01 "OPERATOR_COSIGN_KEY not found at: $OPERATOR_COSIGN_KEY (run knb/scripts/operator-key/bootstrap.sh first)"
[[ -f "$OPERATOR_COSIGN_PUBKEY" ]] \
  || bhl_fail F017-BUILD-01 "OPERATOR_COSIGN_PUBKEY not found at: $OPERATOR_COSIGN_PUBKEY"
bhl_require_env COSIGN_PASSWORD

# Git state.
cd "$REPO_ROOT"
SOURCE_SHA="$(git rev-parse HEAD)"
SHORT_SHA="${SOURCE_SHA:0:12}"
if [[ -n "$(git status --porcelain)" ]]; then
  if [[ "$ALLOW_DIRTY" -ne 1 ]]; then
    bhl_fail F017-BUILD-06 "git working tree is dirty; pass --allow-dirty to override (will tag manifest builtDirty=true)"
  fi
  BUILT_DIRTY=true
else
  BUILT_DIRTY=false
fi

BUILT_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
BUILT_BY="$(git config user.email || echo 'unknown@local')"

CORE_REGISTRY='ghcr.io/pkirsanov/smackerel-core'
ML_REGISTRY='ghcr.io/pkirsanov/smackerel-ml'

DIST_DIR="$REPO_ROOT/dist/local-build-manifests"
mkdir -p "$DIST_DIR"
MANIFEST="$DIST_DIR/local-build-manifest-${SOURCE_SHA}.yaml"

echo "=================================================================="
echo "smackerel build --target home-lab"
echo "  sourceSha:   $SOURCE_SHA"
echo "  builtAt:     $BUILT_AT"
echo "  builtBy:     $BUILT_BY"
echo "  builtDirty:  $BUILT_DIRTY"
echo "  manifest:    $MANIFEST"
echo "  operator key: $OPERATOR_COSIGN_KEY"
echo "=================================================================="

# Step 1: build images via existing smackerel.sh build flow.
echo
echo "[1/7] docker build (smackerel-core + smackerel-ml)"
(cd "$REPO_ROOT" && bash smackerel.sh build) \
  || bhl_fail F017-BUILD-03 "docker build failed"

# Step 2: tag local images for ghcr push.
echo
echo "[2/7] tag local images for ghcr.io"
CORE_LOCAL_TAG="${CORE_REGISTRY}:home-lab-${SHORT_SHA}"
ML_LOCAL_TAG="${ML_REGISTRY}:home-lab-${SHORT_SHA}"
docker tag smackerel-smackerel-core:latest "$CORE_LOCAL_TAG" \
  || bhl_fail F017-BUILD-03 "failed to tag smackerel-core"
docker tag smackerel-smackerel-ml:latest "$ML_LOCAL_TAG" \
  || bhl_fail F017-BUILD-03 "failed to tag smackerel-ml"
echo "  tagged: $CORE_LOCAL_TAG"
echo "  tagged: $ML_LOCAL_TAG"

# Step 3: trivy CRITICAL+HIGH gate per image. Failure here blocks signing.
echo
echo "[3/7] trivy CRITICAL/HIGH gate"
for img in "$CORE_LOCAL_TAG" "$ML_LOCAL_TAG"; do
  echo "  scanning $img"
  trivy image --quiet --severity CRITICAL,HIGH --exit-code 1 \
    --ignore-unfixed --no-progress "$img" \
    || bhl_fail F017-BUILD-04 "trivy gate failed for $img (CRITICAL or HIGH CVE with available fix)"
  echo "  PASS $img"
done

# Step 4: push images to ghcr.io to capture stable digests for signing.
echo
echo "[4/7] docker push (capture stable digests)"
docker push "$CORE_LOCAL_TAG" >/dev/null \
  || bhl_fail F017-BUILD-03 "docker push smackerel-core failed (check ghcr.io auth: gh auth status)"
docker push "$ML_LOCAL_TAG" >/dev/null \
  || bhl_fail F017-BUILD-03 "docker push smackerel-ml failed"

CORE_DIGEST="$(docker inspect --format='{{index .RepoDigests 0}}' "$CORE_LOCAL_TAG" | sed -E 's@.*@\0@; s@^[^@]*@@')"
ML_DIGEST="$(docker inspect --format='{{index .RepoDigests 0}}' "$ML_LOCAL_TAG" | sed -E 's@.*@\0@; s@^[^@]*@@')"
# Strip leading '@' if present
CORE_DIGEST="${CORE_DIGEST#@}"
ML_DIGEST="${ML_DIGEST#@}"
[[ "$CORE_DIGEST" == sha256:* ]] \
  || bhl_fail F017-BUILD-03 "failed to extract sha256 digest for smackerel-core (got: $CORE_DIGEST)"
[[ "$ML_DIGEST" == sha256:* ]] \
  || bhl_fail F017-BUILD-03 "failed to extract sha256 digest for smackerel-ml (got: $ML_DIGEST)"
CORE_IMAGE_REF="${CORE_REGISTRY}@${CORE_DIGEST}"
ML_IMAGE_REF="${ML_REGISTRY}@${ML_DIGEST}"
echo "  core: $CORE_IMAGE_REF"
echo "  ml:   $ML_IMAGE_REF"

# Step 5: cosign sign each image digest with operator key.
echo
echo "[5/7] cosign sign (operator key)"
for ref in "$CORE_IMAGE_REF" "$ML_IMAGE_REF"; do
  cosign sign --yes --key "$OPERATOR_COSIGN_KEY" "$ref" >/dev/null 2>&1 \
    || bhl_fail F017-BUILD-05 "cosign sign failed for $ref"
  echo "  signed: $ref"
done

# Step 6: syft SBOM + cosign attest per image.
echo
echo "[6/7] syft SBOM + cosign attest"
SBOM_DIR="$REPO_ROOT/dist/sboms"
mkdir -p "$SBOM_DIR"
for ref in "$CORE_IMAGE_REF" "$ML_IMAGE_REF"; do
  name="$(basename "${ref%@*}")"
  sbom_path="$SBOM_DIR/${name}-${SHORT_SHA}.spdx.json"
  syft "$ref" -o spdx-json="$sbom_path" --quiet \
    || bhl_fail F017-BUILD-05 "syft SBOM generation failed for $ref"
  cosign attest --yes --key "$OPERATOR_COSIGN_KEY" \
    --predicate "$sbom_path" --type spdxjson "$ref" >/dev/null 2>&1 \
    || bhl_fail F017-BUILD-05 "cosign attest (SBOM) failed for $ref"
  echo "  attested: $ref (sbom: $sbom_path)"
done

# Step 7: emit local-build-manifest.
echo
echo "[7/7] emit local-build-manifest"
{
  echo "---"
  echo "buildManifestVersion: 1"
  echo "trustModel: local-operator"
  echo "product: smackerel"
  echo "sourceSha: \"$SOURCE_SHA\""
  echo "builtAt: \"$BUILT_AT\""
  echo "builtBy: \"$BUILT_BY\""
  echo "builtDirty: $BUILT_DIRTY"
  echo "images:"
  echo "  smackerel-core: \"$CORE_IMAGE_REF\""
  echo "  smackerel-ml: \"$ML_IMAGE_REF\""
  echo "signatures:"
  echo "  images: cosign-key-operator"
  echo "  operatorPubkeyPath: \"$OPERATOR_COSIGN_PUBKEY\""
  echo "  operatorPubkeySha256: \"$(sha256sum "$OPERATOR_COSIGN_PUBKEY" | awk '{print $1}')\""
} >"$MANIFEST"

# Sign the manifest itself.
cosign sign-blob --yes --key "$OPERATOR_COSIGN_KEY" \
  --output-signature "${MANIFEST}.sig" "$MANIFEST" >/dev/null 2>&1 \
  || bhl_fail F017-BUILD-05 "cosign sign-blob failed for manifest"

echo
echo "=================================================================="
echo "build --target home-lab COMPLETE"
echo "  manifest:      $MANIFEST"
echo "  manifest sig:  ${MANIFEST}.sig"
echo
echo "Next: cd ~/knb && OPERATOR_COSIGN_PUBKEY=$OPERATOR_COSIGN_PUBKEY \\"
echo "        bash scripts/deploy/promote.sh --target home-lab --product smackerel \\"
echo "          --local-build-manifest $MANIFEST"
echo "=================================================================="
