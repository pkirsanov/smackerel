#!/usr/bin/env bash
# scripts/deploy/client-manifest-clients-block.sh
#
# Spec 085 — Client Binary Release. Emit the `clients:` block that the CI
# `publish-build-manifest` job appends to build-manifest-<sourceSha>.yaml, under
# the knb spec 025 schema. The block pins smackerel's Android client artifact
# (AAB + APK) by digest so the knb self-hosted adapter (Lane A) can pull + cosign-
# verify it, and so the knb conformance gate's fail-closed manifest check (c)
# passes (a contracted platform with an empty/malformed digest is REFUSED here,
# before it ever reaches the manifest).
#
# This emitter writes to STDOUT only; the caller appends the output to the
# manifest. It NEVER writes a file and NEVER touches the repo tree.
#
# Required inputs (NO-DEFAULTS / fail-loud SST — a missing input fails the build;
# there is no unsigned/empty fallback):
#   CLIENTS_REGISTRY    e.g. ghcr.io/pkirsanov/smackerel-clients
#   ANDROID_AAB_SHA256  64-char lowercase-hex OCI digest of the pushed AAB
#   ANDROID_APK_SHA256  64-char lowercase-hex OCI digest of the pushed APK
#
# Exit codes:
#   0  block emitted to stdout
#   1  a required input is missing OR a digest is empty/malformed (fail-closed)
#
# Unit test (native, reliable): internal/deploy/client_manifest_clients_block_test.go
# Schema authority: .github/instructions/bubbles-client-binary-release.instructions.md
#                   deploy/contract.yaml::clients (locked by clients_contract_test.go)

set -euo pipefail

die() {
  printf 'client-manifest-clients-block: %s\n' "$*" >&2
  exit 1
}

# Fail-loud on any missing required input (smackerel-no-defaults).
: "${CLIENTS_REGISTRY:?CLIENTS_REGISTRY required (ghcr.io/<owner>/smackerel-clients)}"
: "${ANDROID_AAB_SHA256:?ANDROID_AAB_SHA256 required (android AAB OCI digest, 64-hex)}"
: "${ANDROID_APK_SHA256:?ANDROID_APK_SHA256 required (android APK OCI digest, 64-hex)}"

# Fail-closed digest validation: each contracted-platform artifact MUST carry a
# non-empty, well-formed sha256. This mirrors the knb gate's check (c)
# (E025-CLIENT-MANIFEST-NO-DIGEST) but refuses at emit time so a bad digest never
# enters build-manifest-<sourceSha>.yaml.
validate_digest() {
  local name="$1" val="$2"
  [[ -n "$val" ]] || die "$name is empty (fail-closed: every client artifact MUST carry a digest)"
  [[ "$val" =~ ^[0-9a-f]{64}$ ]] || die "$name is not a 64-char lowercase-hex sha256: '$val' (fail-closed)"
}
validate_digest "ANDROID_AAB_SHA256" "$ANDROID_AAB_SHA256"
validate_digest "ANDROID_APK_SHA256" "$ANDROID_APK_SHA256"

# Emit the block. `sha256` is the canonical (AAB) digest the gate's check (c)
# reads for the android/variant="-" key; the apk digest is carried alongside.
# Note: the `ref:`/`aabRef:`/`apkRef:` value lines embed `sha256:` inside the
# value (not at line-start), so the gate's key-anchored parser does not
# false-match them as the sha256 field (knb BUG-001 anchoring).
cat <<YAML
clients:
  none: false
  artifacts:
  - platform: android
    variant: "-"
    kind: [aab, apk]
    ref: ${CLIENTS_REGISTRY}@sha256:${ANDROID_AAB_SHA256}
    sha256: ${ANDROID_AAB_SHA256}
    provenance: cosign-keyless
    embeds: []
    laneB: false
    laneBTarget: play-store
    aabRef: ${CLIENTS_REGISTRY}@sha256:${ANDROID_AAB_SHA256}
    apkRef: ${CLIENTS_REGISTRY}@sha256:${ANDROID_APK_SHA256}
    apkSha256: ${ANDROID_APK_SHA256}
YAML
