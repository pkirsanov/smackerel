#!/usr/bin/env bash
# tests/integration/build_target_selfhosted_trivy_test.sh
#
# Spec 017 scope 03 / Scenario (adversarial): "Trivy CRITICAL/HIGH gate blocks a bad image".
#
# Verifies the Trivy gate wiring in build-self-hosted.sh by running trivy
# directly against a known-vulnerable image and asserting it exits
# non-zero. This proves the gate would catch a real CRITICAL CVE
# without needing to deliberately ship one through the full build.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/commands/build-self-hosted.sh"

command -v trivy >/dev/null 2>&1 || {
  echo "SKIP: trivy not installed"
  exit 0
}

# Step 1: confirm the script invokes trivy with --exit-code 1 and the
# CRITICAL+HIGH severity flag (the wiring assertion).
if ! grep -q 'trivy image.*--severity CRITICAL,HIGH.*--exit-code 1' "$SCRIPT"; then
  echo "FAIL: build-self-hosted.sh does not invoke trivy with the expected --severity CRITICAL,HIGH --exit-code 1 wiring"
  exit 1
fi
echo "OK: build-self-hosted.sh invokes trivy with --severity CRITICAL,HIGH --exit-code 1"

# Step 2: run trivy directly against a known-CVE-laden public image
# (alpine:3.10 has dozens of CRITICAL+HIGH fixed CVEs). Confirms the
# trivy tool itself fires non-zero under the gate conditions used in
# the build script.
TARGET_IMAGE='alpine:3.10'
echo "Running adversarial trivy scan against $TARGET_IMAGE (known to have CRITICAL/HIGH CVEs with available fixes)..."

set +e
trivy image --quiet --severity CRITICAL,HIGH --exit-code 1 \
  --ignore-unfixed --no-progress "$TARGET_IMAGE" >/dev/null 2>&1
RC=$?
set -e

[[ "$RC" -eq 1 ]] \
  || {
    echo "FAIL: trivy gate should have exit 1 on $TARGET_IMAGE, got $RC"
    echo "(if RC=0, the image may have no known CRITICAL/HIGH with fixes — try a different known-vulnerable image)"
    exit 1
  }

echo "PASS: trivy gate fires non-zero exit on a known-vulnerable image (proving the build wiring would block a real CRITICAL/HIGH CVE)"
