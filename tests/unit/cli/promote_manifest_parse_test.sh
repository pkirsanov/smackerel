#!/usr/bin/env bash
# tests/unit/cli/promote_manifest_parse_test.sh
#
# Spec 082 SCOPE-082-07 — build-manifest schema convergence.
#
# Proves scripts/deploy/promote_manifest_parse.sh extracts IDENTICAL values
# from BOTH build-manifest shapes:
#   1. CI list shape       (.github/workflows/build.yml)
#   2. local-operator shape (scripts/commands/build-self-hosted.sh)
#
# Auto-discovered by `./smackerel.sh test unit` (tests/unit/cli/*.sh
# discovery). Also runnable directly: bash tests/unit/cli/promote_manifest_parse_test.sh
#
# Adversarial coverage:
#   - a malformed manifest (neither shape) yields empty values so promote.sh
#     fails loud rather than promoting garbage.
#   - an env that does not match the local-operator object's single env
#     yields empty (no cross-env bundle promotion).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
HELPER="$REPO_ROOT/scripts/deploy/promote_manifest_parse.sh"

[[ -f "$HELPER" ]] || { echo "FAIL: helper not found: $HELPER" >&2; exit 1; }
# shellcheck source=scripts/deploy/promote_manifest_parse.sh
source "$HELPER"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# Canonical logical values shared by both fixture shapes.
EXP_SHA="abc123def4567890abc123def4567890abc123de"
EXP_CORE="ghcr.io/pkirsanov/smackerel-core@sha256:1111111111111111111111111111111111111111111111111111111111111111"
EXP_ML="ghcr.io/pkirsanov/smackerel-ml@sha256:2222222222222222222222222222222222222222222222222222222222222222"
EXP_BUNDLE="ghcr.io/pkirsanov/smackerel-config-bundles:self-hosted-${EXP_SHA}"
EXP_BUNDLE_SHA="3333333333333333333333333333333333333333333333333333333333333333"
ENV="self-hosted"

CI_MANIFEST="$TMP/ci-build-manifest.yaml"
cat >"$CI_MANIFEST" <<EOF
manifestVersion: 1
trustModel: ci-keyless
sourceSha: ${EXP_SHA}
images:
  - name: smackerel-core
    ref: ${EXP_CORE}
  - name: smackerel-ml
    ref: ${EXP_ML}
configBundles:
  - env: ${ENV}
    ref: ${EXP_BUNDLE}
    sha256: ${EXP_BUNDLE_SHA}
EOF

LOCAL_MANIFEST="$TMP/local-build-manifest.yaml"
cat >"$LOCAL_MANIFEST" <<EOF
buildManifestVersion: 1
trustModel: local-operator
product: smackerel
sourceSha: "${EXP_SHA}"
images:
  smackerel-core: "${EXP_CORE}"
  smackerel-ml: "${EXP_ML}"
configBundle:
  ref: "${EXP_BUNDLE}"
  tag: "self-hosted-${EXP_SHA}"
  env: "${ENV}"
  sha256: "${EXP_BUNDLE_SHA}"
signatures:
  images: cosign-key-operator
EOF

assert_eq() { # $1=label $2=expected $3=actual
  [[ "$2" == "$3" ]] || fail "$1: expected '$2', got '$3'"
  echo "  ok: $1 = $3"
}

for shape in CI LOCAL; do
  if [[ "$shape" == CI ]]; then M="$CI_MANIFEST"; else M="$LOCAL_MANIFEST"; fi
  echo "== $shape shape =="
  assert_eq "$shape sourceSha"  "$EXP_SHA"        "$(manifest_source_sha "$M")"
  assert_eq "$shape core ref"   "$EXP_CORE"       "$(manifest_image_ref "$M" smackerel-core)"
  assert_eq "$shape ml ref"     "$EXP_ML"         "$(manifest_image_ref "$M" smackerel-ml)"
  assert_eq "$shape bundle ref" "$EXP_BUNDLE"     "$(manifest_bundle_ref "$M" "$ENV")"
  assert_eq "$shape bundle sha" "$EXP_BUNDLE_SHA" "$(manifest_bundle_sha "$M" "$ENV")"
done

# Cross-check: both shapes yield byte-identical extractions.
[[ "$(manifest_image_ref "$CI_MANIFEST" smackerel-core)" == "$(manifest_image_ref "$LOCAL_MANIFEST" smackerel-core)" ]] \
  || fail "core ref differs between CI and local shapes"
[[ "$(manifest_bundle_sha "$CI_MANIFEST" "$ENV")" == "$(manifest_bundle_sha "$LOCAL_MANIFEST" "$ENV")" ]] \
  || fail "bundle sha differs between CI and local shapes"
echo "  ok: CI and local shapes yield identical extractions"

# Adversarial 1 — malformed manifest (neither shape) yields empty values.
BAD="$TMP/garbage.yaml"
cat >"$BAD" <<EOF
something: else
nothing: here
EOF
[[ -z "$(manifest_source_sha "$BAD")" ]]               || fail "adversarial: malformed manifest yielded a non-empty sourceSha"
[[ -z "$(manifest_image_ref "$BAD" smackerel-core)" ]] || fail "adversarial: malformed manifest yielded a non-empty core ref"
[[ -z "$(manifest_bundle_ref "$BAD" "$ENV")" ]]        || fail "adversarial: malformed manifest yielded a non-empty bundle ref"
echo "  ok: malformed manifest yields empty values (promote.sh will fail loud)"

# Adversarial 2 — env that does not match the local object's single env.
[[ -z "$(manifest_bundle_ref "$LOCAL_MANIFEST" staging)" ]] \
  || fail "adversarial: local object returned a bundle ref for non-matching env 'staging'"
[[ -z "$(manifest_bundle_sha "$LOCAL_MANIFEST" staging)" ]] \
  || fail "adversarial: local object returned a bundle sha for non-matching env 'staging'"
echo "  ok: non-matching env yields empty bundle (no cross-env promotion)"

echo "PASS: promote.sh parses both CI list-shape and local-operator object-shape manifests identically (SCOPE-082-07)"
