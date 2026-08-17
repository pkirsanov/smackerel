#!/usr/bin/env bash
# shellcheck disable=SC2015 # Assertion helpers are intentionally used in compact A && pass || fail form.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/trust-metadata.sh"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

fixture_root="$(mktemp -d -t bubbles-release-manifest-selftest-XXXXXXXX)"
# shellcheck disable=SC2329 # Invoked indirectly by trap.
cleanup() { rm -rf "$fixture_root"; }
trap cleanup EXIT INT TERM

manifest_section_has_path() {
  local section_name="$1"
  local relative_path="$2"

  awk -v section_name="$section_name" -v relative_path="$relative_path" '
    BEGIN {
      section_line="  \"" section_name "\": ["
      expected_prefix="    {\"path\": \"" relative_path "\", \"sha256\": \""
    }
    $0 == section_line { in_section=1; next }
    in_section && ($0 == "  ]," || $0 == "  ]") { exit }
    in_section && index($0, expected_prefix) == 1 { found=1; exit }
    END { exit found ? 0 : 1 }
  ' "$manifest_file"
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

git -C "$fixture_root" init --quiet
mkdir -p "$fixture_root/bubbles/scripts"
printf '%s\n' '#!/usr/bin/env bash' > "$fixture_root/bubbles/scripts/new-managed.sh"
printf '%s\n' 'bubbles/scripts/ignored.sh' > "$fixture_root/.gitignore"
printf '%s\n' '#!/usr/bin/env bash' > "$fixture_root/bubbles/scripts/ignored.sh"
if ! bubbles_manifest_entry_is_tracked "$fixture_root" 'bubbles/scripts/new-managed.sh'; then
  pass "Unstaged new file stays outside the payload until it is tracked"
else
  fail "Unstaged new file stays outside the payload until it is tracked"
fi
git -C "$fixture_root" add bubbles/scripts/new-managed.sh >/dev/null 2>&1
if bubbles_manifest_entry_is_tracked "$fixture_root" 'bubbles/scripts/new-managed.sh'; then
  pass "A tracked managed file is admitted to the payload"
else
  fail "A tracked managed file is admitted to the payload"
fi
if ! bubbles_manifest_entry_is_tracked "$fixture_root" 'bubbles/scripts/ignored.sh'; then
  pass "Ignored scratch file stays outside payload"
else
  fail "Ignored scratch file stays outside payload"
fi

source_only_count="$(bubbles_json_number_field "$manifest_file" sourceOnlyFileCount)"
if [[ -n "$source_only_count" && "$source_only_count" -gt 0 ]]; then
  pass "Manifest records source-only file count (${source_only_count})"
else
  fail "Manifest records source-only file count"
fi

if grep -q '"path": "tests/regression/test_18_capability_foundation_gate.sh"' "$manifest_file"; then
  pass "Source-only checksum inventory includes G094 regression test"
else
  fail "Source-only checksum inventory includes G094 regression test"
fi

if manifest_section_has_path managedFileChecksums 'bubbles/scripts/adversarial-aggregate.sh'; then
  pass "Managed checksum inventory includes IMP-020 S2 aggregator"
else
  fail "Managed checksum inventory includes IMP-020 S2 aggregator"
fi

if manifest_section_has_path managedFileChecksums 'bubbles/scripts/adversarial-aggregate-selftest.sh'; then
  pass "Managed checksum inventory includes IMP-020 S2 aggregator selftest"
else
  fail "Managed checksum inventory includes IMP-020 S2 aggregator selftest"
fi

# BUG015-F2: the adversarial-sample record schema is now MANAGED because the
# managed agent-common.md red-team contract names it as the authoritative record
# schema; the installed package must ship it. It must appear ONLY in the managed
# section and never in source-only (a path in both sections is a provenance defect).
if manifest_section_has_path managedFileChecksums 'bubbles/eval/schemas/adversarial-sample.schema.json'; then
  pass "Managed checksum inventory includes IMP-020 S2 sample schema (BUG015-F2)"
else
  fail "Managed checksum inventory includes IMP-020 S2 sample schema (BUG015-F2)"
fi

if ! manifest_section_has_path sourceOnlyFileChecksums 'bubbles/eval/schemas/adversarial-sample.schema.json'; then
  pass "IMP-020 S2 sample schema is no longer source-only (BUG015-F2)"
else
  fail "IMP-020 S2 sample schema is no longer source-only (BUG015-F2)"
fi

# Scope guard (BUG015-F1): the eval-harness runtime schemas MUST remain
# source-only. Promoting them would re-ship the harness's broken downstream ref.
for eval_runtime_schema in \
  'bubbles/eval/schemas/task-v2.schema.json' \
  'bubbles/eval/schemas/evaluator-result.schema.json'; do
  if manifest_section_has_path sourceOnlyFileChecksums "$eval_runtime_schema"; then
    pass "Source-only inventory retains eval-harness schema: $eval_runtime_schema"
  else
    fail "Source-only inventory retains eval-harness schema: $eval_runtime_schema"
  fi
  if ! manifest_section_has_path managedFileChecksums "$eval_runtime_schema"; then
    pass "eval-harness schema stays out of managed set: $eval_runtime_schema"
  else
    fail "eval-harness schema stays out of managed set: $eval_runtime_schema"
  fi
done

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