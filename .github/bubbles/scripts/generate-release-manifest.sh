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

git_sha="$(bubbles_local_source_sha "$REPO_ROOT")"
[[ -n "$git_sha" ]] || {
  echo "generate-release-manifest requires a git checkout with a readable HEAD SHA" >&2
  exit 1
}

version_value="$(cat "$REPO_ROOT/VERSION")"
generated_at=''
if bubbles_owns_git_checkout "$REPO_ROOT"; then
  generated_at="$({ git -C "$REPO_ROOT" log -1 --format=%cI; } || true)"
elif [[ -f "$OUTPUT_PATH" ]]; then
  generated_at="$(bubbles_json_string_field "$OUTPUT_PATH" generatedAt)"
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
mapfile -t managed_entries < <(bubbles_framework_manifest_entries "$REPO_ROOT" false)

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
  echo '  ]'
  echo '}'
} > "$temp_output"

if [[ "$CHECK_ONLY" == 'true' ]]; then
  [[ -f "$OUTPUT_PATH" ]] || {
    echo "Missing release manifest: $OUTPUT_PATH" >&2
    exit 1
  }

  if cmp -s "$temp_output" "$OUTPUT_PATH"; then
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