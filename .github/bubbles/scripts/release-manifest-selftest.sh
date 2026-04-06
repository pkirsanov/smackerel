#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/trust-metadata.sh"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

manifest_file="$ROOT_DIR/bubbles/release-manifest.json"

echo "Running release-manifest selftest..."
echo "Scenario: release hygiene generates one complete trust manifest for downstream installs."

if bash "$SCRIPT_DIR/generate-release-manifest.sh" --check >/dev/null; then
  pass "Committed release manifest is current"
else
  fail "Committed release manifest is current"
fi

[[ -f "$manifest_file" ]] && pass "Release manifest exists" || fail "Release manifest exists"

version_value="$(bubbles_json_string_field "$manifest_file" version)"
[[ -n "$version_value" ]] && pass "Manifest records release version" || fail "Manifest records release version"

git_sha="$(bubbles_json_string_field "$manifest_file" gitSha)"
[[ -n "$git_sha" ]] && pass "Manifest records source git SHA" || fail "Manifest records source git SHA"

docs_digest="$(bubbles_json_string_field "$manifest_file" docsDigest)"
[[ -n "$docs_digest" ]] && pass "Manifest records trust docs digest" || fail "Manifest records trust docs digest"

managed_count="$(bubbles_json_number_field "$manifest_file" managedFileCount)"
if [[ -n "$managed_count" && "$managed_count" -gt 0 ]]; then
  pass "Manifest records framework-managed file count (${managed_count})"
else
  fail "Manifest records framework-managed file count"
fi

if grep -q '"path": "agents/' "$manifest_file"; then
  pass "Managed checksum inventory includes framework agents"
else
  fail "Managed checksum inventory includes framework agents"
fi

if grep -q '"path": "bubbles/scripts/cli.sh"' "$manifest_file"; then
  pass "Managed checksum inventory includes shared CLI surface"
else
  fail "Managed checksum inventory includes shared CLI surface"
fi

profiles="$(bubbles_json_array_joined "$manifest_file" supportedProfiles ', ')"
[[ "$profiles" == *foundation* ]] && pass "Manifest exposes foundation as a supported profile" || fail "Manifest exposes foundation as a supported profile"
[[ "$profiles" == *delivery* ]] && pass "Manifest exposes delivery as a supported profile" || fail "Manifest exposes delivery as a supported profile"

interop_sources="$(bubbles_json_array_joined "$manifest_file" supportedInteropSources ', ')"
[[ "$interop_sources" == *claude-code* ]] && pass "Manifest exposes Claude Code as a supported interop source" || fail "Manifest exposes Claude Code as a supported interop source"
[[ "$interop_sources" == *roo-code* ]] && pass "Manifest exposes Roo Code as a supported interop source" || fail "Manifest exposes Roo Code as a supported interop source"
[[ "$interop_sources" == *cursor* ]] && pass "Manifest exposes Cursor as a supported interop source" || fail "Manifest exposes Cursor as a supported interop source"
[[ "$interop_sources" == *cline* ]] && pass "Manifest exposes Cline as a supported interop source" || fail "Manifest exposes Cline as a supported interop source"

if [[ "$failures" -gt 0 ]]; then
  echo "release-manifest selftest failed with $failures issue(s)."
  exit 1
fi

echo "release-manifest selftest passed."