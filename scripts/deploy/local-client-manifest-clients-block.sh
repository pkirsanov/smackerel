#!/usr/bin/env bash
# scripts/deploy/local-client-manifest-clients-block.sh
#
# Spec 086 — Local Client Build (local-operator trust model). Emit the `clients:`
# block that scripts/commands/local-client-build.sh embeds in the LOCAL build
# manifest (local-client-manifest-<sourceSha>.yaml) under the knb spec 025/028
# schema. This is the `local-operator` analogue of the spec-085 CI emitter
# (scripts/deploy/client-manifest-clients-block.sh): same schema, but
# `provenance: local-operator` (NOT cosign-keyless) and a LOCAL filesystem `ref`
# (no ghcr / no @sha256: in the ref) — because on <deploy-host> the client is BUILT +
# operator-signed LOCALLY and acquired by the knb adapter as a local path
# (knb/scripts/deploy/client-delivery-step.sh::knb_client_acquire_local).
#
# This emitter writes to STDOUT only; the caller embeds the output under the
# manifest root. It NEVER writes a file and NEVER touches the repo tree.
#
# Required inputs (NO-DEFAULTS / fail-loud SST — a missing input fails the build;
# there is no unsigned/empty fallback):
#   ANDROID_AAB_REF     local ref of the signed AAB (e.g. file:///srv/.../x.aab)
#   ANDROID_AAB_SHA256  64-char lowercase-hex content digest of the AAB
#   ANDROID_APK_REF     local ref of the signed APK
#   ANDROID_APK_SHA256  64-char lowercase-hex content digest of the APK
#
# Exit codes:
#   0  block emitted to stdout
#   1  a required input is missing OR a digest is empty/malformed (fail-closed)
#
# Unit test (native, reliable): internal/deploy/local_client_manifest_clients_block_test.go
# Schema authority: knb/.github/instructions/bubbles-client-binary-release.instructions.md
#                   knb/specs/028-client-binary-local-operator-trust-model/spec.md
#                   knb/scripts/lint/client-binary-conformance.sh (check c, local-operator)

set -euo pipefail

die() {
  printf 'local-client-manifest-clients-block: %s\n' "$*" >&2
  exit 1
}

# Fail-loud on any missing required input (smackerel-no-defaults).
: "${ANDROID_AAB_REF:?ANDROID_AAB_REF required (local file:// ref of the signed AAB)}"
: "${ANDROID_AAB_SHA256:?ANDROID_AAB_SHA256 required (android AAB content digest, 64-hex)}"
: "${ANDROID_APK_REF:?ANDROID_APK_REF required (local file:// ref of the signed APK)}"
: "${ANDROID_APK_SHA256:?ANDROID_APK_SHA256 required (android APK content digest, 64-hex)}"

# Fail-closed digest validation: each contracted-platform artifact MUST carry a
# non-empty, well-formed sha256. This mirrors the knb gate's check (c)
# (E025-CLIENT-MANIFEST-NO-DIGEST) but refuses at emit time so a bad digest never
# enters local-client-manifest-<sourceSha>.yaml.
validate_digest() {
  local name="$1" val="$2"
  [[ -n "$val" ]] || die "$name is empty (fail-closed: every client artifact MUST carry a digest)"
  [[ "$val" =~ ^[0-9a-f]{64}$ ]] || die "$name is not a 64-char lowercase-hex sha256: '$val' (fail-closed)"
}
validate_digest "ANDROID_AAB_SHA256" "$ANDROID_AAB_SHA256"
validate_digest "ANDROID_APK_SHA256" "$ANDROID_APK_SHA256"

# Emit the block. `sha256` is the canonical (AAB) content digest the gate's
# check (c) reads for the android/variant="-" key; the apk digest is carried
# alongside. `provenance: local-operator` matches the manifest's top-level
# `trustModel: local-operator` (knb spec 028 trust-model-aware provenance). The
# `ref:`/`aabRef:`/`apkRef:` values are LOCAL paths (no embedded `@sha256:`), so
# the gate's line-anchored sha256 parser does not false-match them.
cat <<YAML
clients:
  none: false
  artifacts:
  - platform: android
    variant: "-"
    kind: [aab, apk]
    ref: ${ANDROID_AAB_REF}
    sha256: ${ANDROID_AAB_SHA256}
    provenance: local-operator
    embeds: []
    laneB: false
    laneBTarget: play-store
    aabRef: ${ANDROID_AAB_REF}
    apkRef: ${ANDROID_APK_REF}
    apkSha256: ${ANDROID_APK_SHA256}
YAML
