#!/usr/bin/env bash
# scripts/commands/local-client-build.sh
#
# Spec 086 — `./smackerel.sh local-client-build --target home-lab`
#
# Builds smackerel's Flutter Android client (clients/mobile/assistant) LOCALLY on
# evo-x2 (AAB + APK), operator-signs each artifact with the operator cosign key,
# computes the real content sha256, and emits a LOCAL build manifest with
# `trustModel: local-operator` + `provenance: local-operator` consumable by the
# knb home-lab adapter's local-operator Lane-A delivery
# (knb/scripts/deploy/client-delivery-step.sh::knb_client_acquire_local →
#  cosign verify-blob --key <pubkey> → sha256 byte-match → place under serveRoot).
#
# This is the local-operator analogue of the spec-085 CI `build-clients` job and
# composes by reference with the knb spec 028 "Local Client-Build Phase
# Contract". It mirrors scripts/commands/build-home-lab.sh (which already does
# LOCAL operator-key signing of the two server images). The knb adapter stays
# consume-only; smackerel BUILDS + SIGNS (FC-2).
#
# Trust model: local-operator (single-operator evo-x2). Aligns with
# smackerel/home-lab/params.yaml::signing.trustModel: local-operator. The
# ci-keyless path (spec 085) stays parked (FC-1).
#
# Required env:
#   OPERATOR_COSIGN_KEY     path to operator cosign private key
#                           (default $HOME/.config/knb/operator-keys/cosign-operator.key)
#   OPERATOR_COSIGN_PUBKEY  path to operator cosign public key (for adapter handoff)
#   COSIGN_PASSWORD         passphrase for the private key (presence-checked; NEVER echoed)
#
# Test seams (default to the real tool/path; tool plumbing, NOT SST runtime config —
# mirrors build-home-lab.sh's `${OPERATOR_COSIGN_KEY:=...}` precedent):
#   SMACKEREL_FLUTTER_BUILD_CMD   build command (default: flutter)
#   SMACKEREL_COSIGN_CMD          sign command  (default: cosign)
#   SMACKEREL_LCB_PROJECT_DIR     Flutter project dir (default: <repo>/clients/mobile/assistant)
#
# Outputs (under --out-dir, default dist/local-clients/<shortSha>/):
#   smackerel-assistant-<shortSha>.aab        (+ .sig)
#   smackerel-assistant-<shortSha>.apk        (+ .sig)
#   local-client-manifest-<sourceSha>.yaml    (+ .sig)
#
# Exit codes:
#   [F086-LCB-01]  missing required env var / operator key file / bad --target
#   [F086-LCB-02]  required CLI tool missing
#   [F086-LCB-03]  flutter build failed OR a built artifact is missing/empty
#   [F086-LCB-04]  a content sha256 is empty/malformed (fail-closed)
#   [F086-LCB-05]  cosign sign-blob failed OR an expected .sig is missing (fail-closed)
#   [F086-LCB-06]  git working tree dirty AND --allow-dirty not passed
#
# FC-4 (no fabrication): the REAL `flutter build` + REAL operator-sign + REAL
# placement run ON evo-x2 (node n11). This script's logic is proven locally with
# a stubbed build + a recording cosign shim (internal/deploy/local_client_build_test.go).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

lcb_fail() {
  local code="$1"
  shift
  echo "ERROR: [$code] $*" >&2
  exit 1
}

lcb_require_cmd() {
  command -v "$1" >/dev/null 2>&1 \
    || lcb_fail F086-LCB-02 "required command '$1' not found on PATH"
}

lcb_require_env() {
  local var_name="$1"
  local value="${!var_name:-}"
  [[ -n "$value" ]] \
    || lcb_fail F086-LCB-01 "$var_name env var required for local-client-build"
}

# ---- Arg parse -------------------------------------------------------------
TARGET=""
ALLOW_DIRTY=0
OUT_DIR=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      [[ $# -ge 2 && -n "${2:-}" ]] || lcb_fail F086-LCB-01 "--target requires a value (only: home-lab)"
      TARGET="$2"
      shift 2
      ;;
    --target=*)
      TARGET="${1#*=}"
      shift
      ;;
    --out-dir)
      [[ $# -ge 2 && -n "${2:-}" ]] || lcb_fail F086-LCB-01 "--out-dir requires a value"
      OUT_DIR="$2"
      shift 2
      ;;
    --out-dir=*)
      OUT_DIR="${1#*=}"
      shift
      ;;
    --allow-dirty)
      ALLOW_DIRTY=1
      shift
      ;;
    *)
      lcb_fail F086-LCB-01 "unknown argument: $1 (usage: local-client-build --target home-lab [--allow-dirty] [--out-dir <dir>])"
      ;;
  esac
done

[[ -n "$TARGET" ]] \
  || lcb_fail F086-LCB-01 "--target is required (only supported value: home-lab)"
[[ "$TARGET" == "home-lab" ]] \
  || lcb_fail F086-LCB-01 "unsupported --target '$TARGET' (only supported value: home-lab)"

# ---- Tool/path seams (default to the real tool; mirrors build-home-lab.sh) --
: "${SMACKEREL_FLUTTER_BUILD_CMD:=flutter}"
: "${SMACKEREL_COSIGN_CMD:=cosign}"
: "${SMACKEREL_LCB_PROJECT_DIR:=$REPO_ROOT/clients/mobile/assistant}"

lcb_require_cmd "$SMACKEREL_FLUTTER_BUILD_CMD"
lcb_require_cmd "$SMACKEREL_COSIGN_CMD"
lcb_require_cmd git
lcb_require_cmd sha256sum

# ---- Operator key env (mirrors build-home-lab.sh) --------------------------
: "${OPERATOR_COSIGN_KEY:=$HOME/.config/knb/operator-keys/cosign-operator.key}"
: "${OPERATOR_COSIGN_PUBKEY:=$HOME/.config/knb/operator-keys/cosign-operator.pub}"
[[ -f "$OPERATOR_COSIGN_KEY" ]] \
  || lcb_fail F086-LCB-01 "OPERATOR_COSIGN_KEY not found at: $OPERATOR_COSIGN_KEY (run knb/scripts/operator-key/bootstrap.sh first)"
[[ -f "$OPERATOR_COSIGN_PUBKEY" ]] \
  || lcb_fail F086-LCB-01 "OPERATOR_COSIGN_PUBKEY not found at: $OPERATOR_COSIGN_PUBKEY"
lcb_require_env COSIGN_PASSWORD
# cosign reads COSIGN_PASSWORD from the environment; export so the child inherits
# it. Presence only — the value is NEVER echoed (terminal discipline).
export COSIGN_PASSWORD

[[ -d "$SMACKEREL_LCB_PROJECT_DIR" ]] \
  || lcb_fail F086-LCB-01 "Flutter project dir not found: $SMACKEREL_LCB_PROJECT_DIR"

# ---- Git state -------------------------------------------------------------
cd "$REPO_ROOT"
SOURCE_SHA="$(git rev-parse HEAD)"
SHORT_SHA="${SOURCE_SHA:0:12}"
BUILT_DIRTY=false
if [[ -n "$(git status --porcelain)" ]]; then
  BUILT_DIRTY=true
  [[ "$ALLOW_DIRTY" -eq 1 ]] \
    || lcb_fail F086-LCB-06 "git working tree is dirty; pass --allow-dirty to build anyway (manifest records builtDirty: true)"
fi
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
BUILT_BY="$(whoami 2>/dev/null || echo "${USER:-unknown}")"

[[ -n "$OUT_DIR" ]] || OUT_DIR="$REPO_ROOT/dist/local-clients/$SHORT_SHA"
mkdir -p "$OUT_DIR" \
  || lcb_fail F086-LCB-03 "cannot create out-dir: $OUT_DIR"

echo "=================================================================="
echo "local-client-build  (trustModel: local-operator)"
echo "  target:       $TARGET"
echo "  sourceSha:    $SOURCE_SHA"
echo "  project:      $SMACKEREL_LCB_PROJECT_DIR"
echo "  out-dir:      $OUT_DIR"
echo "  operator key: $OPERATOR_COSIGN_KEY"
echo "  COSIGN_PASSWORD: ${COSIGN_PASSWORD:+present}"
echo "  builtDirty:   $BUILT_DIRTY"
echo "=================================================================="

# ---- Step 1: build (LOCAL; stubbable seam) ---------------------------------
# The REAL `flutter build aab/apk` runs on evo-x2 (node n11). Tests inject a stub
# via SMACKEREL_FLUTTER_BUILD_CMD; FC-4 forbids fabricating a real AAB here.
AAB_SRC="$SMACKEREL_LCB_PROJECT_DIR/build/app/outputs/bundle/release/app-release.aab"
APK_SRC="$SMACKEREL_LCB_PROJECT_DIR/build/app/outputs/flutter-apk/app-release.apk"

echo "[1/5] flutter build aab + apk ($SMACKEREL_FLUTTER_BUILD_CMD)"
(cd "$SMACKEREL_LCB_PROJECT_DIR" && "$SMACKEREL_FLUTTER_BUILD_CMD" build aab) \
  || lcb_fail F086-LCB-03 "flutter build aab failed"
(cd "$SMACKEREL_LCB_PROJECT_DIR" && "$SMACKEREL_FLUTTER_BUILD_CMD" build apk) \
  || lcb_fail F086-LCB-03 "flutter build apk failed"

# Fail-closed: each produced artifact must exist and be non-empty.
[[ -s "$AAB_SRC" ]] || lcb_fail F086-LCB-03 "AAB not produced or empty: $AAB_SRC"
[[ -s "$APK_SRC" ]] || lcb_fail F086-LCB-03 "APK not produced or empty: $APK_SRC"

AAB_OUT="$OUT_DIR/smackerel-assistant-$SHORT_SHA.aab"
APK_OUT="$OUT_DIR/smackerel-assistant-$SHORT_SHA.apk"
cp -f "$AAB_SRC" "$AAB_OUT" || lcb_fail F086-LCB-03 "cannot stage AAB into $OUT_DIR"
cp -f "$APK_SRC" "$APK_OUT" || lcb_fail F086-LCB-03 "cannot stage APK into $OUT_DIR"

# ---- Step 2: real content sha256 (fail-closed) -----------------------------
echo "[2/5] sha256 (real content digest)"
AAB_SHA="$(sha256sum "$AAB_OUT" | awk '{print $1}')"
APK_SHA="$(sha256sum "$APK_OUT" | awk '{print $1}')"
[[ "$AAB_SHA" =~ ^[0-9a-f]{64}$ ]] || lcb_fail F086-LCB-04 "AAB sha256 empty/malformed: '$AAB_SHA'"
[[ "$APK_SHA" =~ ^[0-9a-f]{64}$ ]] || lcb_fail F086-LCB-04 "APK sha256 empty/malformed: '$APK_SHA'"
echo "  aab sha256: $AAB_SHA"
echo "  apk sha256: $APK_SHA"

# ---- Step 3: operator-sign each artifact (cosign sign-blob → adjacent .sig) --
# The knb local-operator adapter copies <ref> + adjacent <ref>.sig and verifies
# offline with `cosign verify-blob --key <pubkey> --insecure-ignore-tlog`. So the
# signature MUST be the artifact path + ".sig", and the sign side MUST be fully
# OFFLINE (`--tlog-upload=false`, no Rekor) to pair with that offline verify.
lcb_sign_blob() {
  local artifact="$1"
  local sig="$artifact.sig"
  "$SMACKEREL_COSIGN_CMD" sign-blob --yes --key "$OPERATOR_COSIGN_KEY" \
    --tlog-upload=false --output-signature "$sig" "$artifact" >/dev/null 2>&1 \
    || lcb_fail F086-LCB-05 "cosign sign-blob failed for $artifact"
  [[ -s "$sig" ]] \
    || lcb_fail F086-LCB-05 "signature missing/empty after sign-blob: $sig (fail-closed)"
}

echo "[3/5] cosign sign-blob (operator key)"
lcb_sign_blob "$AAB_OUT"
lcb_sign_blob "$APK_OUT"

AAB_REF="file://$(readlink -f "$AAB_OUT")"
APK_REF="file://$(readlink -f "$APK_OUT")"

# ---- Step 4: emit the local-operator clients block -------------------------
echo "[4/5] emit clients block (provenance: local-operator)"
CLIENTS_BLOCK="$(
  ANDROID_AAB_REF="$AAB_REF" \
    ANDROID_AAB_SHA256="$AAB_SHA" \
    ANDROID_APK_REF="$APK_REF" \
    ANDROID_APK_SHA256="$APK_SHA" \
    bash "$SCRIPT_DIR/../deploy/local-client-manifest-clients-block.sh"
)" || lcb_fail F086-LCB-04 "clients-block emitter refused (fail-closed digest check)"

# ---- Step 5: assemble + sign the local build manifest ----------------------
echo "[5/5] emit + sign local-client-manifest"
MANIFEST="$OUT_DIR/local-client-manifest-$SOURCE_SHA.yaml"
PUBKEY_SHA="$(sha256sum "$OPERATOR_COSIGN_PUBKEY" | awk '{print $1}')"
{
  echo "---"
  echo "buildManifestVersion: 1"
  echo "trustModel: local-operator"
  echo "product: smackerel"
  echo "clientBuildManifest: true"
  echo "sourceSha: \"$SOURCE_SHA\""
  echo "builtAt: \"$BUILT_AT\""
  echo "builtBy: \"$BUILT_BY\""
  echo "builtDirty: $BUILT_DIRTY"
  echo "$CLIENTS_BLOCK"
  echo "signatures:"
  echo "  clients: cosign-key-operator"
  echo "  operatorPubkeyPath: \"$OPERATOR_COSIGN_PUBKEY\""
  echo "  operatorPubkeySha256: \"$PUBKEY_SHA\""
} >"$MANIFEST"

"$SMACKEREL_COSIGN_CMD" sign-blob --yes --key "$OPERATOR_COSIGN_KEY" \
  --tlog-upload=false --output-signature "$MANIFEST.sig" "$MANIFEST" >/dev/null 2>&1 \
  || lcb_fail F086-LCB-05 "cosign sign-blob failed for manifest"
[[ -s "$MANIFEST.sig" ]] \
  || lcb_fail F086-LCB-05 "manifest signature missing/empty: $MANIFEST.sig (fail-closed)"

echo
echo "=================================================================="
echo "local-client-build COMPLETE"
echo "  aab:        $AAB_OUT (+ .sig)"
echo "  apk:        $APK_OUT (+ .sig)"
echo "  manifest:   $MANIFEST (+ .sig)"
echo
echo "Next (on evo-x2): the knb home-lab adapter acquires + verifies these LOCAL"
echo "  signed artifacts under trustModel local-operator. cd ~/knb && \\"
echo "    OPERATOR_COSIGN_PUBKEY=$OPERATOR_COSIGN_PUBKEY \\"
echo "    bash scripts/deploy/promote.sh --target home-lab --product smackerel \\"
echo "      --local-build-manifest $MANIFEST"
echo "=================================================================="
