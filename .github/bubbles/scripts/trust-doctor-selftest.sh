#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

expect_pattern() {
  local haystack="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" <<< "$haystack"; then
    pass "$label"
  else
    fail "$label"
  fi
}

expect_no_pattern() {
  local haystack="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" <<< "$haystack"; then
    fail "$label"
  else
    pass "$label"
  fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

REMOTE_FIXTURE="$TMP_ROOT/remote-fixture"
LOCAL_FIXTURE="$TMP_ROOT/local-fixture"
DIRTY_SOURCE="$TMP_ROOT/local-source-dirty"
MALFORMED_CURRENT_FIXTURE="$TMP_ROOT/malformed-current-fixture"
MALFORMED_SOURCE="$TMP_ROOT/malformed-source"
FOUNDATION_GAP_FIXTURE="$TMP_ROOT/foundation-gap-fixture"

mkdir -p "$REMOTE_FIXTURE" "$LOCAL_FIXTURE" "$MALFORMED_CURRENT_FIXTURE" "$FOUNDATION_GAP_FIXTURE"
git -C "$REMOTE_FIXTURE" init -q
git -C "$LOCAL_FIXTURE" init -q
git -C "$MALFORMED_CURRENT_FIXTURE" init -q
git -C "$FOUNDATION_GAP_FIXTURE" init -q
printf '# Foundation Gap Fixture\n' > "$FOUNDATION_GAP_FIXTURE/README.md"
cp -a "$ROOT_DIR" "$DIRTY_SOURCE"
cp -a "$ROOT_DIR" "$MALFORMED_SOURCE"
printf '\n# trust doctor selftest dirty marker\n' >> "$DIRTY_SOURCE/CHANGELOG.md"

echo "Running trust-doctor selftest..."
echo "Scenario: doctor, framework-write-guard, and upgrade dry-run expose trust state and local-source risk explicitly."

(
  cd "$REMOTE_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap >/dev/null
)
(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" --bootstrap >/dev/null
)
(
  cd "$MALFORMED_CURRENT_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap >/dev/null
)
(
  cd "$FOUNDATION_GAP_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap --profile foundation >/dev/null
)

rm -f "$FOUNDATION_GAP_FIXTURE/.specify/memory/agents.md"

perl -0pi -e 's/^  "version": ".*",\n//m' "$MALFORMED_CURRENT_FIXTURE/.github/bubbles/release-manifest.json"
perl -0pi -e 's/^  "supportedProfiles": \[[^\n]*\],\n//m; s/^  "supportedInteropSources": \[[^\n]*\],\n//m' "$MALFORMED_SOURCE/bubbles/release-manifest.json"

remote_doctor_output="$(cd "$REMOTE_FIXTURE" && bash .github/bubbles/scripts/cli.sh doctor 2>&1)"
local_doctor_output="$(cd "$LOCAL_FIXTURE" && bash .github/bubbles/scripts/cli.sh doctor 2>&1)"
local_guard_output="$(cd "$LOCAL_FIXTURE" && bash .github/bubbles/scripts/cli.sh framework-write-guard 2>&1)"
local_upgrade_output="$(cd "$LOCAL_FIXTURE" && bash .github/bubbles/scripts/cli.sh upgrade --dry-run --local-source "$DIRTY_SOURCE" 2>&1)"
foundation_doctor_output="$(cd "$FOUNDATION_GAP_FIXTURE" && bash .github/bubbles/scripts/cli.sh doctor 2>&1)"
foundation_readiness_output="$(cd "$FOUNDATION_GAP_FIXTURE" && bash .github/bubbles/scripts/repo-readiness.sh . 2>&1)"

set +e
malformed_guard_output="$(cd "$MALFORMED_CURRENT_FIXTURE" && bash .github/bubbles/scripts/cli.sh framework-write-guard 2>&1)"
malformed_guard_status=$?
malformed_upgrade_output="$(cd "$REMOTE_FIXTURE" && BUBBLES_SOURCE_OVERRIDE_DIR="$MALFORMED_SOURCE" bash .github/bubbles/scripts/cli.sh upgrade --dry-run 2>&1)"
malformed_upgrade_status=$?
set -e

expect_pattern "$remote_doctor_output" 'Installed release manifest: version=' "Doctor shows installed release manifest details for remote-ref installs"
expect_pattern "$remote_doctor_output" 'Install provenance: mode=remote-ref sourceRef=main' "Doctor shows remote-ref provenance"
expect_pattern "$local_doctor_output" 'Installed from a dirty local source checkout' "Doctor warns when the installed source checkout was dirty"
expect_pattern "$foundation_doctor_output" 'Active adoption profile: Foundation \(foundation\)' "Doctor shows the explicit foundation adoption profile"
expect_pattern "$foundation_doctor_output" 'foundation keeps this advisory during first-run onboarding' "Doctor downgrades project-readiness gaps to advisory under foundation"
expect_pattern "$foundation_doctor_output" 'Result: [0-9]+ passed, 0 failed, [1-9][0-9]* advisory' "Doctor reports foundation onboarding gaps as advisory instead of failing"
expect_pattern "$foundation_readiness_output" 'Active profile: Foundation \(foundation\)' "Repo-readiness shows the explicit foundation adoption profile"
expect_pattern "$foundation_readiness_output" 'foundation prioritizes onboarding fixes first; later delivery work stays advisory here' "Repo-readiness explains the foundation posture"
expect_pattern "$foundation_readiness_output" 'Boundary: advisory framework ops only; this does not replace bubbles.validate certification\.' "Repo-readiness keeps the certification boundary explicit"
expect_pattern "$local_guard_output" 'Managed-file integrity:' "Framework write guard reports managed-file integrity state"
expect_pattern "$local_guard_output" 'mode=local-source' "Framework write guard shows local-source provenance"
expect_pattern "$local_upgrade_output" 'Project-owned files that will not be touched' "Upgrade dry-run distinguishes untouched project-owned files"
expect_pattern "$local_upgrade_output" 'Target local source is dirty' "Upgrade dry-run surfaces local-source trust risk"
expect_pattern "$local_upgrade_output" 'Framework-managed files that will be replaced:' "Upgrade dry-run shows framework-managed replacement count"
if [[ "$malformed_guard_status" -ne 0 ]]; then
  pass "Framework write guard fails loud on malformed release manifest"
else
  fail "Framework write guard fails loud on malformed release manifest"
fi
expect_pattern "$malformed_guard_output" 'requires release-manifest\.json field "version" as a non-empty JSON string' "Framework write guard names the missing manifest field"
if [[ "$malformed_upgrade_status" -ne 0 ]]; then
  pass "Upgrade dry-run rejects malformed target release metadata"
else
  fail "Upgrade dry-run rejects malformed target release metadata"
fi
expect_pattern "$malformed_upgrade_output" 'requires release-manifest\.json field "supportedProfiles" as a JSON array' "Upgrade dry-run names the missing target profiles field"
expect_pattern "$malformed_upgrade_output" 'requires release-manifest\.json field "supportedInteropSources" as a JSON array' "Upgrade dry-run names the missing target interop field"
expect_no_pattern "$malformed_upgrade_output" 'unbound variable' "Upgrade dry-run malformed-manifest failure stays free of shell trap errors"

if [[ "$failures" -gt 0 ]]; then
  echo "trust-doctor selftest failed with $failures issue(s)."
  exit 1
fi

echo "trust-doctor selftest passed."