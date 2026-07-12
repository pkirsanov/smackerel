#!/usr/bin/env bash
# tests/unit/build_target_selfhosted_requires_key_test.sh
#
# Spec 017 scope 03 / Scenario (adversarial): "Missing operator key blocks the build".

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/commands/build-self-hosted.sh"

[[ -f "$SCRIPT" ]] || {
  echo "FAIL: $SCRIPT missing"
  exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Adversarial 1: OPERATOR_COSIGN_KEY pointing at a nonexistent path.
set +e
(
  OPERATOR_COSIGN_KEY="$TMP/nonexistent.key" \
    OPERATOR_COSIGN_PUBKEY="$TMP/nonexistent.pub" \
    COSIGN_PASSWORD=ignored \
    bash "$SCRIPT"
) >"$TMP/out1" 2>"$TMP/err1"
RC1=$?
set -e
[[ "$RC1" -eq 1 ]] \
  || {
    echo "FAIL: missing key file should exit 1, got $RC1"
    cat "$TMP/err1"
    exit 1
  }
grep -q 'F017-BUILD-01' "$TMP/err1" \
  || {
    echo "FAIL: expected F017-BUILD-01 in stderr"
    cat "$TMP/err1"
    exit 1
  }

# Adversarial 2: real OPERATOR_COSIGN_KEY but COSIGN_PASSWORD unset.
echo "fake-key" >"$TMP/k.key"
echo "fake-pub" >"$TMP/k.pub"
set +e
(
  OPERATOR_COSIGN_KEY="$TMP/k.key" \
    OPERATOR_COSIGN_PUBKEY="$TMP/k.pub" \
    COSIGN_PASSWORD='' \
    bash "$SCRIPT"
) >"$TMP/out2" 2>"$TMP/err2"
RC2=$?
set -e
[[ "$RC2" -eq 1 ]] \
  || {
    echo "FAIL: missing COSIGN_PASSWORD should exit 1, got $RC2"
    cat "$TMP/err2"
    exit 1
  }
grep -q 'F017-BUILD-01' "$TMP/err2" \
  || {
    echo "FAIL: expected F017-BUILD-01 for missing COSIGN_PASSWORD"
    cat "$TMP/err2"
    exit 1
  }

echo "PASS: build --target self-hosted refuses to start without operator key OR without COSIGN_PASSWORD (F017-BUILD-01 in both cases)"
