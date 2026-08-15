#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/trust-metadata.sh"
source "$SCRIPT_DIR/interop-registry.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/adoption-profile-lib.sh"

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
  bubbles_adoption_profile_ids "$1"
}

mapfile -t managed_entries < <(bubbles_framework_manifest_entries "$REPO_ROOT" false)

# BUG-015 (BUG015-F1): the golden-task eval HARNESS is framework-source-only, not a
# downstream product tool. Its selftest is wired through run_check_self_only (SKIPPED
# in downstream framework-validate), and the entire bubbles/eval/ payload it consumes
# — task-v2.schema.json + evaluator-result.schema.json, golden tasks, regression data —
# is classified source-only below. The blanket `bubbles/scripts/*.sh` managed glob in
# bubbles_framework_manifest_entries() nonetheless swept eval-harness.sh + its selftest
# into the managed set, so install.sh shipped the harness downstream while its required
# schemas stayed source-only: a broken relative ref (`../eval/schemas/...`) that makes
# eval-harness.sh exit 2 with schema-contract-unavailable on every downstream call.
# Reclassify the harness pair as source-only so the manifest's managed section no longer
# owns them; install.sh's existing managed-script prune (release_manifest_owns_managed_path)
# then removes them downstream, keeping the eval subsystem cohesively source-only. No
# downstream-run script invokes eval-harness.sh at runtime (eval-heldout-guard.sh only
# names it in comments; forecast-eval-check.sh is standalone), so this creates no
# dangling reference.
demoted_source_only_scripts=(
  "bubbles/scripts/eval-harness.sh"
  "bubbles/scripts/eval-harness-selftest.sh"
  # Its labeled corpus lives under bubbles/eval/, which is source-only in full,
  # so shipping the scorer downstream would ship a script that can only ever
  # SKIP. Retrieval quality is a framework-development measurement; the six
  # recall selftests that exercise the SHIPPED provider, CLI, and authority
  # firewall stay managed and do run downstream.
  "bubbles/scripts/experience-recall-eval-selftest.sh"
  # These three drive eval-harness.sh through $SCRIPT_DIR. Shipping them while
  # the harness stays source-only put a managed executable in the payload whose
  # dependency was absent, so downstream it could only ever skip or fail. They
  # travel with the subsystem they exercise.
  "bubbles/scripts/eval-corpus-selftest.sh"
  "bubbles/scripts/gate-detection-selftest.sh"
  "bubbles/scripts/judge-adapter-contract-selftest.sh"
  # Not an eval script. v5.3-selftest builds a downstream fixture by running
  # install.sh, which lives at the source root and is never shipped. Shipping
  # this selftest downstream made it resolve $ROOT_DIR to the installed
  # .github/ directory, find no install.sh, and fail with rc=127 -- and it also
  # ran a FULL downstream framework-validate inside an already-running
  # framework-validate. It belongs with the installer it exercises.
  "bubbles/scripts/v5.3-selftest.sh"
  # The framework's own release gate. Its required-files list includes
  # install.sh, and cli.sh has always described it as "source-repo release
  # hygiene checks", so it can only ever run from a source checkout.
  "bubbles/scripts/release-check.sh"
  # Exercises release-check, so it travels with it.
  "bubbles/scripts/ci-annotation-emitter-selftest.sh"
)
filtered_managed_entries=()
for managed_entry in "${managed_entries[@]}"; do
  demote_entry=false
  for eval_script in "${demoted_source_only_scripts[@]}"; do
    if [[ "$managed_entry" == "$eval_script" ]]; then
      demote_entry=true
      break
    fi
  done
  [[ "$demote_entry" == true ]] && continue
  filtered_managed_entries+=("$managed_entry")
done
managed_entries=("${filtered_managed_entries[@]}")

source_only_entries=()
# The installer and the version file it stamps are source-root artifacts. They
# were in NEITHER checksum section, so nothing tracked their integrity and the
# payload-closure guard had no way to notice a managed script reaching for them.
for installer_entry in "install.sh" "VERSION"; do
  [[ -f "$REPO_ROOT/$installer_entry" ]] || continue
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$installer_entry" || continue
  source_only_entries+=("$installer_entry")
done
for eval_script in "${demoted_source_only_scripts[@]}"; do
  [[ -f "$REPO_ROOT/$eval_script" ]] || continue
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$eval_script" || continue
  source_only_entries+=("$eval_script")
done
# BUG015-F2: the adversarial-sample record schema is now MANAGED (installed)
# because the managed agent-common.md red-team contract names it as the
# authoritative record schema. bubbles_framework_manifest_entries() enumerates it
# in the managed set above, so it MUST NOT also be swept into source-only here — a
# path in BOTH checksum sections is a provenance defect (install-provenance and
# release-manifest selftests reject it). The other two eval schemas (task-v2 /
# evaluator-result) stay source-only: BUG015-F1 demoted the eval-harness itself,
# which downstream never runs.
eval_managed_schemas=(
  "bubbles/eval/schemas/adversarial-sample.schema.json"
)
while IFS= read -r eval_source_path; do
  [[ -f "$eval_source_path" ]] || continue
  eval_relative_path="${eval_source_path#$REPO_ROOT/}"
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$eval_relative_path" || continue
  eval_entry_is_managed=false
  for eval_managed_schema in "${eval_managed_schemas[@]}"; do
    if [[ "$eval_relative_path" == "$eval_managed_schema" ]]; then
      eval_entry_is_managed=true
      break
    fi
  done
  [[ "$eval_entry_is_managed" == true ]] && continue
  source_only_entries+=("$eval_relative_path")
done < <(find "$REPO_ROOT/bubbles/eval" -type f 2>/dev/null | LC_ALL=C sort)
while IFS= read -r regression_test_path; do
  [[ -f "$regression_test_path" ]] || continue
  regression_relative_path="${regression_test_path#$REPO_ROOT/}"
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$regression_relative_path" || continue
  source_only_entries+=("$regression_relative_path")
done < <(find "$REPO_ROOT/tests/regression" -maxdepth 1 -type f -name '*.sh' 2>/dev/null | LC_ALL=C sort)

# IMP-102 / SCOPE-10: the per-agent effective-bundle budget file is a source-repo
# artifact consumed ONLY by agent-bundle-size-budget.sh --check, which
# framework-validate runs via run_check_self_only (source-only). Downstream installs
# carry agents under .github/agents and never run the live budget check, so the
# committed ceilings are integrity-tracked here but NOT shipped downstream.
agent_bundle_budget_entry="bubbles/agent-bundle-budgets.json"
if [[ -f "$REPO_ROOT/$agent_bundle_budget_entry" ]] &&
  bubbles_manifest_entry_is_tracked "$REPO_ROOT" "$agent_bundle_budget_entry"; then
  source_only_entries+=("$agent_bundle_budget_entry")
fi

# Keep the source-only inventory deterministically sorted. The demoted eval-harness
# pair lives under bubbles/scripts/, which sorts between bubbles/eval/** and
# tests/regression/**, so it must be merged into sort order rather than appended.
if [[ "${#source_only_entries[@]}" -gt 0 ]]; then
  mapfile -t source_only_entries < <(printf '%s\n' "${source_only_entries[@]}" | LC_ALL=C sort)
fi

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

# Hash both inventories in one pass each (IMP-042 SCOPE-4). A short batch result
# means the tool skipped an entry, which would silently pair a path with another
# file's hash, so any count mismatch falls back to the per-file form rather than
# emitting a manifest that looks well-formed and is wrong.
hash_inventory() { # hash_inventory <result-array-name> <entry...>
  local result_name="$1"
  shift
  local -a entries=("$@")
  local -a hashes=()
  local entry

  if [[ "${#entries[@]}" -gt 0 ]]; then
    mapfile -t hashes < <(printf '%s\n' "${entries[@]}" | bubbles_sha256_batch "$REPO_ROOT" | cut -f1)
    if [[ "${#hashes[@]}" -ne "${#entries[@]}" ]]; then
      hashes=()
      for entry in "${entries[@]}"; do
        hashes+=("$(bubbles_sha256_file "$REPO_ROOT/$entry")")
      done
    fi
  fi
  eval "$result_name=(\"\${hashes[@]}\")"
}

managed_checksums=()
source_only_checksums=()
hash_inventory managed_checksums ${managed_entries[@]+"${managed_entries[@]}"}
hash_inventory source_only_checksums ${source_only_entries[@]+"${source_only_entries[@]}"}

temp_output="$(mktemp)"
trap 'rm -f "$temp_output"' EXIT

{
  echo '{'
  printf '  "schemaVersion": %s,\n' '1'
  printf '  "version": "%s",\n' "$version_value"
  printf '  "gitSha": "%s",\n' "$git_sha"
  printf '  "generatedAt": "%s",\n' "$generated_at"
  printf '  "capabilityLedgerVersion": %s,\n' "$capability_ledger_version"
  # IMP-027 SCOPE-4 / SEC-1. install.sh guarded payload verification with
  # `if [[ -f "$PAYLOAD_VERIFIER" ]]` and no else branch, so deleting one file
  # from a tarball silently disabled integrity checking for the whole install.
  # Declaring the requirement here lets the installer tell "legacy payload that
  # predates the verifier" apart from "payload that should have shipped it and
  # did not" — the second is a tampering signal and now refuses.
  printf '  "payloadVerifierRequired": %s,\n' 'true'

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
    printf '    {"path": "%s", "sha256": "%s"}' "$entry" "${managed_checksums[$idx]}"
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
    printf '    {"path": "%s", "sha256": "%s"}' "$entry" "${source_only_checksums[$idx]}"
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