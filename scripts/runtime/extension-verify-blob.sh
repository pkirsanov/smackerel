#!/usr/bin/env bash
# scripts/runtime/extension-verify-blob.sh
#
# Spec 058 Scope 4 — LOCAL cosign sign + verify-blob supply-chain proof for the
# Chrome Extension Bridge sideload artifact.
#
# This is the locally-deliverable portion of the spec-058 "cosign verify-blob"
# DoD row. It proves, against the REAL built zip, that:
#
#   1. the recorded `.sha256` sidecar matches the actual artifact bytes
#      (the same hash the build manifest's chromeBridge.zipSha256 records), and
#   2. a cosign signature over the artifact verifies via `cosign verify-blob`
#      (the verification mechanics the operator runs at sideload time), and
#   3. tampering is DETECTED — a one-byte-truncated copy FAILS verify-blob
#      (proves the check is not tautological).
#
# It uses an EPHEMERAL throwaway key pair generated in a temp dir with an empty
# passphrase, signs OFFLINE (`--tlog-upload=false`), and verifies OFFLINE
# (`--insecure-ignore-tlog=true`). It deliberately does NOT upload anything to
# the public Rekor transparency log: pushing test signatures to a shared public
# log is forbidden environment pollution. The temp key material is destroyed on
# exit.
#
# What this does NOT cover (irreducibly CI): the KEYLESS OIDC identity binding
# (cosign sign-blob --yes under a GitHub Actions OIDC token) recorded in a real
# Rekor entry. That requires a published release signed by CI's OIDC identity
# and cannot be produced — or honestly faked — on a developer box.
#
# Usage: ./smackerel.sh test extension-supplychain

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist/extension"
BUILD_SCRIPT="$REPO_ROOT/scripts/commands/build-chrome-bridge.sh"

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  cat <<'SUPPLY_HELP'
Usage: ./smackerel.sh test extension-supplychain

LOCAL cosign sign + verify-blob proof for the Chrome Extension Bridge zip:
  - builds the real artifact (scripts/commands/build-chrome-bridge.sh),
  - asserts the .sha256 sidecar matches the zip bytes,
  - signs the zip with an ephemeral offline cosign key and verifies it,
  - proves tamper detection (a truncated copy FAILS verify-blob).

Self-contained and OFFLINE: nothing is uploaded to the public Rekor log
(that would be forbidden shared-system pollution). The keyless-OIDC identity
binding against a real Rekor entry remains a CI-only concern.
SUPPLY_HELP
  exit 0
fi

if ! command -v cosign >/dev/null 2>&1; then
  echo "ERROR: 'cosign' is required for the extension supply-chain proof but is not on PATH." >&2
  exit 127
fi
if ! command -v sha256sum >/dev/null 2>&1; then
  echo "ERROR: 'sha256sum' is required for the extension supply-chain proof but is not on PATH." >&2
  exit 127
fi
if ! command -v truncate >/dev/null 2>&1; then
  echo "ERROR: 'truncate' is required for the adversarial tamper check but is not on PATH." >&2
  exit 127
fi

echo "==> [1/5] Building the chrome-bridge zip (real artifact)"
# Clear stale chrome-bridge artifacts from prior builds FIRST. The build embeds
# the git short-SHA in the zip name, so different builds accumulate as
# differently-named zips in $DIST_DIR. Without this clean, the post-build glob
# below resolves to whatever sorts first (alphabetically) — which can be a
# STALE zip from an earlier SHA, not the artifact we just produced — so the
# proof would sign/verify the wrong bytes and still "PASS" against an old file.
# $DIST_DIR is gitignored, byte-reproducible build output: safe to clear.
rm -f "$DIST_DIR"/smackerel-chrome-bridge-*.zip "$DIST_DIR"/smackerel-chrome-bridge-*.zip.sha256
bash "$BUILD_SCRIPT"

# Resolve the artifact the build JUST produced. After the pre-build clean the
# glob must match exactly one zip; refuse to guess if zero or more-than-one
# are present (a >1 result would mean the stale-zip ambiguity has regressed).
mapfile -t BRIDGE_ZIPS < <(ls -1 "$DIST_DIR"/smackerel-chrome-bridge-*.zip 2>/dev/null || true)
if [[ "${#BRIDGE_ZIPS[@]}" -eq 0 ]]; then
  echo "ERROR: build did not produce a chrome-bridge zip under $DIST_DIR" >&2
  exit 1
fi
if [[ "${#BRIDGE_ZIPS[@]}" -gt 1 ]]; then
  echo "ERROR: expected exactly one freshly-built chrome-bridge zip under $DIST_DIR, found ${#BRIDGE_ZIPS[@]}:" >&2
  printf '  %s\n' "${BRIDGE_ZIPS[@]}" >&2
  exit 1
fi
ZIP_PATH="${BRIDGE_ZIPS[0]}"
if [[ ! -f "$ZIP_PATH" ]]; then
  echo "ERROR: resolved chrome-bridge zip path is not a file: $ZIP_PATH" >&2
  exit 1
fi
SHA_SIDECAR="${ZIP_PATH}.sha256"
if [[ ! -f "$SHA_SIDECAR" ]]; then
  echo "ERROR: missing .sha256 sidecar for $ZIP_PATH" >&2
  exit 1
fi
echo "    artifact: $ZIP_PATH"

echo "==> [2/5] Verifying the recorded .sha256 matches the artifact bytes"
RECORDED_SHA="$(awk '{print $1}' "$SHA_SIDECAR")"
ACTUAL_SHA="$(sha256sum "$ZIP_PATH" | awk '{print $1}')"
if [[ "$RECORDED_SHA" != "$ACTUAL_SHA" ]]; then
  echo "FAIL: recorded sha256 ($RECORDED_SHA) != actual ($ACTUAL_SHA)" >&2
  exit 1
fi
echo "    sha256 OK: $ACTUAL_SHA"

# Ephemeral key material + signatures live in a temp dir destroyed on exit.
WORK_DIR="$(mktemp -d)"
cleanup() { rm -rf "$WORK_DIR"; }
trap cleanup EXIT

echo "==> [3/5] Generating an ephemeral offline cosign key pair"
(
  cd "$WORK_DIR"
  COSIGN_PASSWORD="" cosign generate-key-pair >/dev/null
)
COSIGN_KEY="$WORK_DIR/cosign.key"
COSIGN_PUB="$WORK_DIR/cosign.pub"
SIG_PATH="$WORK_DIR/chrome-bridge.zip.sig"

echo "==> [4/5] Signing the artifact offline and verifying the signature"
COSIGN_PASSWORD="" cosign sign-blob \
  --key "$COSIGN_KEY" \
  --output-signature "$SIG_PATH" \
  --tlog-upload=false \
  --yes \
  "$ZIP_PATH"

if cosign verify-blob \
  --key "$COSIGN_PUB" \
  --signature "$SIG_PATH" \
  --insecure-ignore-tlog=true \
  "$ZIP_PATH"; then
  echo "    verify-blob OK: signature verifies against the artifact"
else
  echo "FAIL: verify-blob rejected a valid signature over the untampered artifact" >&2
  exit 1
fi

echo "==> [5/5] Adversarial: a tampered copy MUST fail verify-blob"
TAMPERED="$WORK_DIR/tampered.zip"
cp "$ZIP_PATH" "$TAMPERED"
truncate -s -1 "$TAMPERED" # drop the last byte so the bytes no longer match the signature
if cosign verify-blob \
  --key "$COSIGN_PUB" \
  --signature "$SIG_PATH" \
  --insecure-ignore-tlog=true \
  "$TAMPERED" 2>/dev/null; then
  echo "FAIL: verify-blob ACCEPTED a tampered artifact — the check is tautological" >&2
  exit 1
fi
echo "    tamper detection OK: verify-blob rejected the truncated copy"

echo ""
echo "PASS: chrome-bridge supply-chain proof — sha256 binding + offline cosign sign/verify-blob + tamper detection all hold."
echo "NOTE: the keyless-OIDC identity binding against a real Rekor entry is a CI-only concern (not run here; no public-Rekor pollution)."
