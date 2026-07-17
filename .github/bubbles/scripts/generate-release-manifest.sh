#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/trust-metadata.sh"
source "$SCRIPT_DIR/interop-registry.sh"

REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_PATH="$REPO_ROOT/bubbles/release-manifest.json"
CHECK_ONLY='false'

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      CHECK_ONLY='true'
      shift
      ;;
    --output)
      OUTPUT_PATH="$2"
      shift 2
      ;;
    --repo-root)
      REPO_ROOT="$2"
      shift 2
      ;;
    --help|-h)
      cat <<'EOF'
Usage: bash bubbles/scripts/generate-release-manifest.sh [--check] [--output PATH] [--repo-root PATH]

Generates bubbles/release-manifest.json for the Bubbles source repo.
Use --check to verify the committed manifest matches the current source tree.
EOF
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 2
      ;;
  esac
done

[[ -f "$REPO_ROOT/VERSION" ]] || {
  echo "Missing VERSION file in $REPO_ROOT" >&2
  exit 1
}

[[ -f "$REPO_ROOT/bubbles/capability-ledger.yaml" ]] || {
  echo "Missing capability ledger in $REPO_ROOT/bubbles" >&2
  exit 1
}

[[ -f "$REPO_ROOT/bubbles/adoption-profiles.yaml" ]] || {
  echo "Missing adoption profile registry in $REPO_ROOT/bubbles" >&2
  exit 1
}

adoption_profile_ids() {
  local registry_file="$1"

  awk '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && /^  [A-Za-z0-9_-]+:$/ {
      profile=$1
      sub(":$", "", profile)
      print profile
    }
  ' "$registry_file"
}

mapfile -t managed_entries < <(bubbles_framework_manifest_entries "$REPO_ROOT" false)
source_only_entries=()
while IFS= read -r eval_source_path; do
  [[ -f "$eval_source_path" ]] || continue
  eval_relative_path="${eval_source_path#$REPO_ROOT/}"
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$eval_relative_path" || continue
  source_only_entries+=("$eval_relative_path")
done < <(find "$REPO_ROOT/bubbles/eval" -type f 2>/dev/null | LC_ALL=C sort)
while IFS= read -r regression_test_path; do
  [[ -f "$regression_test_path" ]] || continue
  regression_relative_path="${regression_test_path#$REPO_ROOT/}"
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$regression_relative_path" || continue
  source_only_entries+=("$regression_relative_path")
done < <(find "$REPO_ROOT/tests/regression" -maxdepth 1 -type f -name '*.sh' 2>/dev/null | LC_ALL=C sort)

payload_git_sha=''
payload_generated_at=''
if bubbles_owns_git_checkout "$REPO_ROOT" && [[ "${#managed_entries[@]}" -gt 0 ]]; then
  # H10 (v5.0.1): exclude the manifest itself from the git-log lookup so that a
  # commit which ALSO regenerates the manifest does not produce a stale-by-design
  # gitSha (the new commit's SHA would only be knowable after the commit is made,
  # leaving the manifest's gitSha pointing at the previous head). The manifest is
  # a *derived* artifact of the other managed entries — its own history is not
  # an input to the payload SHA.
  payload_inputs=()
  for entry in "${managed_entries[@]}"; do
    [[ "$entry" == "bubbles/release-manifest.json" ]] && continue
    payload_inputs+=("$entry")
  done
  if [[ "${#payload_inputs[@]}" -gt 0 ]]; then
    payload_git_sha="$({ git -C "$REPO_ROOT" log -1 --format=%H -- "${payload_inputs[@]}"; } 2>/dev/null || true)"
    payload_generated_at="$({ git -C "$REPO_ROOT" log -1 --format=%cI -- "${payload_inputs[@]}"; } 2>/dev/null || true)"
  fi
fi

git_sha="${payload_git_sha:-$(bubbles_local_source_sha "$REPO_ROOT") }"
git_sha="${git_sha% }"
[[ -n "$git_sha" ]] || {
  echo "generate-release-manifest requires a git checkout with a readable managed-payload SHA" >&2
  exit 1
}

version_value="$(cat "$REPO_ROOT/VERSION")"
generated_at="$payload_generated_at"
if [[ -z "$generated_at" && -f "$OUTPUT_PATH" ]]; then
  generated_at="$({ bubbles_json_string_field "$OUTPUT_PATH" generatedAt; } 2>/dev/null || true)"
fi
[[ -n "$generated_at" ]] || generated_at="$(bubbles_current_timestamp)"
capability_ledger_version="$({ awk '/^version:/ { print $2; exit }' "$REPO_ROOT/bubbles/capability-ledger.yaml"; } || true)"
[[ -n "$capability_ledger_version" ]] || capability_ledger_version='1'

mapfile -t supported_profiles < <(adoption_profile_ids "$REPO_ROOT/bubbles/adoption-profiles.yaml")
[[ "${#supported_profiles[@]}" -gt 0 ]] || {
  echo "Adoption profile registry must expose at least one supported profile" >&2
  exit 1
}
mapfile -t supported_interop_sources < <(bubbles_interop_source_ids "$(bubbles_interop_registry_path "$REPO_ROOT")")
mapfile -t validated_surfaces < <(printf '%s\n' \
  'framework-validate' \
  'release-check' \
  'release-manifest-selftest' \
  'install-provenance-selftest' \
  'finding-closure-selftest' \
  'interop-import-selftest' \
  'trust-doctor-selftest')

docs_digest_material=''
for docs_file in \
  "$REPO_ROOT/docs/guides/INSTALLATION.md" \
  "$REPO_ROOT/docs/recipes/framework-ops.md" \
  "$REPO_ROOT/CHANGELOG.md"; do
  [[ -f "$docs_file" ]] || continue
  docs_digest_material+="${docs_file#$REPO_ROOT/}\t$(bubbles_sha256_file "$docs_file")"$'\n'
done
docs_digest="$(printf '%s' "$docs_digest_material" | bubbles_sha256_stdin)"
managed_file_count="${#managed_entries[@]}"
source_only_file_count="${#source_only_entries[@]}"

temp_output="$(mktemp)"
trap 'rm -f "$temp_output"' EXIT

{
  echo '{'
  printf '  "schemaVersion": %s,\n' '1'
  printf '  "version": "%s",\n' "$version_value"
  printf '  "gitSha": "%s",\n' "$git_sha"
  printf '  "generatedAt": "%s",\n' "$generated_at"
  printf '  "capabilityLedgerVersion": %s,\n' "$capability_ledger_version"

  printf '  "supportedProfiles": ['
  for idx in "${!supported_profiles[@]}"; do
    [[ "$idx" -gt 0 ]] && printf ', '
    printf '"%s"' "${supported_profiles[$idx]}"
  done
  echo '],'

  printf '  "supportedInteropSources": ['
  first_item='true'
  for source_id in "${supported_interop_sources[@]}"; do
    [[ -z "$source_id" ]] && continue
    [[ "$first_item" == 'false' ]] && printf ', '
    printf '"%s"' "$source_id"
    first_item='false'
  done
  echo '],'

  printf '  "validatedSurfaces": ['
  for idx in "${!validated_surfaces[@]}"; do
    [[ "$idx" -gt 0 ]] && printf ', '
    printf '"%s"' "${validated_surfaces[$idx]}"
  done
  echo '],'

  printf '  "docsDigest": "%s",\n' "$docs_digest"
  printf '  "managedFileCount": %s,\n' "$managed_file_count"
  echo '  "managedFileChecksums": ['
  for idx in "${!managed_entries[@]}"; do
    entry="${managed_entries[$idx]}"
    checksum_value="$(bubbles_sha256_file "$REPO_ROOT/$entry")"
    printf '    {"path": "%s", "sha256": "%s"}' "$entry" "$checksum_value"
    if [[ "$idx" -lt $((managed_file_count - 1)) ]]; then
      echo ','
    else
      echo
    fi
  done
  echo '  ],'
  printf '  "sourceOnlyFileCount": %s,\n' "$source_only_file_count"
  echo '  "sourceOnlyFileChecksums": ['
  for idx in "${!source_only_entries[@]}"; do
    entry="${source_only_entries[$idx]}"
    checksum_value="$(bubbles_sha256_file "$REPO_ROOT/$entry")"
    printf '    {"path": "%s", "sha256": "%s"}' "$entry" "$checksum_value"
    if [[ "$idx" -lt $((source_only_file_count - 1)) ]]; then
      echo ','
    else
      echo
    fi
  done
  echo '  ]'
  echo '}'
} > "$temp_output"

if [[ "$CHECK_ONLY" == 'true' ]]; then
  [[ -f "$OUTPUT_PATH" ]] || {
    echo "Missing release manifest: $OUTPUT_PATH" >&2
    exit 1
  }

  # H10 (v5.0.1): compare manifest content EXCLUDING gitSha + generatedAt
  # fields. Those two fields naturally update whenever the manifest itself is
  # part of the commit being prepared, so a byte-exact `cmp` produces a
  # chicken-and-egg "stale" verdict even when the payload is correct. The
  # checksums + counts + file lists are still compared exactly.
  filter_volatile() {
    sed -E -e 's/^  "gitSha": ".*",$/  "gitSha": "<volatile>",/' \
           -e 's/^  "generatedAt": ".*",$/  "generatedAt": "<volatile>",/'
  }
  if diff -q <(filter_volatile < "$temp_output") <(filter_volatile < "$OUTPUT_PATH") >/dev/null 2>&1; then
    printf 'Release manifest is current: %s (%s managed files)\n' "$version_value" "$managed_file_count"
    exit 0
  fi

  echo "Release manifest is stale. Run bubbles/scripts/generate-release-manifest.sh" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_PATH")"
mv "$temp_output" "$OUTPUT_PATH"
trap - EXIT
printf 'Updated release manifest: %s (%s managed files)\n' "$version_value" "$managed_file_count"