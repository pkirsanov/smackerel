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

assert_no_path_leak() {
  local provenance_file="$1"
  local forbidden_path="$2"
  local label="$3"

  if grep -Fq "$forbidden_path" "$provenance_file"; then
    fail "$label"
  else
    pass "$label"
  fi
}

checksum_snapshot_value() {
  local checksum_file="$1"
  local relative_path="$2"

  awk -F '\t' -v wanted="$relative_path" '$2 == wanted { print $1; exit }' "$checksum_file"
}

release_manifest_section_has_checksum() {
  local manifest_file="$1"
  local section_name="$2"
  local relative_path="$3"
  local expected_checksum="$4"

  awk -v section_name="$section_name" -v relative_path="$relative_path" -v expected_checksum="$expected_checksum" '
    BEGIN {
      section_line="  \"" section_name "\": ["
      expected_line="    {\"path\": \"" relative_path "\", \"sha256\": \"" expected_checksum "\"}"
    }
    $0 == section_line { in_section=1; next }
    in_section && ($0 == "  ]," || $0 == "  ]") { exit }
    in_section {
      candidate=$0
      sub(/,$/, "", candidate)
      if (candidate == expected_line) found=1
    }
    END { exit found ? 0 : 1 }
  ' "$manifest_file"
}

assert_bug_009_managed_install() {
  local relative_path="$1"
  local executable_required="${2:-false}"
  local assertion_scope="${3:-BUG-009}"
  local source_path="$ROOT_DIR/$relative_path"
  local installed_root="$LOCAL_FIXTURE/.github"
  local installed_path="$installed_root/$relative_path"
  local installed_manifest="$installed_root/bubbles/.manifest"
  local installed_checksums="$installed_root/bubbles/.checksums"
  local release_manifest="$installed_root/bubbles/release-manifest.json"
  local source_checksum=''
  local installed_checksum=''
  local snapshot_checksum=''

  if [[ ! -f "$source_path" ]]; then
    fail "$assertion_scope canonical source exists: $relative_path"
    return
  fi
  source_checksum="$(bubbles_sha256_file "$source_path")"

  if [[ -f "$installed_path" ]]; then
    pass "$assertion_scope managed file installed: $relative_path"
    installed_checksum="$(bubbles_sha256_file "$installed_path")"
  else
    fail "$assertion_scope managed file installed: $relative_path"
  fi

  if [[ "$executable_required" != 'true' ]] || [[ -x "$installed_path" ]]; then
    pass "$assertion_scope executable mode is correct: $relative_path"
  else
    fail "$assertion_scope executable mode is correct: $relative_path"
  fi

  if grep -Fxq "$relative_path" "$installed_manifest"; then
    pass "$assertion_scope .manifest owns: $relative_path"
  else
    fail "$assertion_scope .manifest owns: $relative_path"
  fi

  snapshot_checksum="$(checksum_snapshot_value "$installed_checksums" "$relative_path")"
  if [[ -n "$installed_checksum" && "$snapshot_checksum" == "$installed_checksum" ]]; then
    pass "$assertion_scope .checksums records installed bytes: $relative_path"
  else
    fail "$assertion_scope .checksums records installed bytes: $relative_path"
  fi

  if [[ -f "$installed_path" ]] && cmp -s "$source_path" "$installed_path"; then
    pass "$assertion_scope installed bytes match canonical source: $relative_path"
  else
    fail "$assertion_scope installed bytes match canonical source: $relative_path"
  fi

  if release_manifest_section_has_checksum "$release_manifest" managedFileChecksums "$relative_path" "$source_checksum"; then
    pass "$assertion_scope release manifest records managed checksum: $relative_path"
  else
    fail "$assertion_scope release manifest records managed checksum: $relative_path"
  fi
}

assert_bug_009_source_only_release_entry() {
  local relative_path="$1"
  local assertion_scope="${2:-BUG-009}"
  local source_only_subject="${3:-source-only regression}"
  local checksum_subject="${4:-source-only}"
  local source_path="$ROOT_DIR/$relative_path"
  local installed_root="$LOCAL_FIXTURE/.github"
  local installed_manifest="$installed_root/bubbles/.manifest"
  local installed_checksums="$installed_root/bubbles/.checksums"
  local release_manifest="$installed_root/bubbles/release-manifest.json"
  local source_checksum=''

  if [[ ! -f "$source_path" ]]; then
    fail "$assertion_scope source-only file exists: $relative_path"
    return
  fi
  source_checksum="$(bubbles_sha256_file "$source_path")"

  [[ ! -e "$installed_root/$relative_path" ]] \
    && pass "$assertion_scope $source_only_subject is not installed: $relative_path" \
    || fail "$assertion_scope $source_only_subject is not installed: $relative_path"
  ! grep -Fxq "$relative_path" "$installed_manifest" \
    && pass "$assertion_scope $source_only_subject is absent from .manifest: $relative_path" \
    || fail "$assertion_scope $source_only_subject is absent from .manifest: $relative_path"
  [[ -z "$(checksum_snapshot_value "$installed_checksums" "$relative_path")" ]] \
    && pass "$assertion_scope $source_only_subject is absent from .checksums: $relative_path" \
    || fail "$assertion_scope $source_only_subject is absent from .checksums: $relative_path"
  release_manifest_section_has_checksum "$release_manifest" sourceOnlyFileChecksums "$relative_path" "$source_checksum" \
    && pass "$assertion_scope release manifest records $checksum_subject checksum: $relative_path" \
    || fail "$assertion_scope release manifest records $checksum_subject checksum: $relative_path"
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

REMOTE_FIXTURE="$TMP_ROOT/remote-fixture"
LOCAL_FIXTURE="$TMP_ROOT/local-fixture"
DIRTY_SOURCE="$TMP_ROOT/local-source-dirty"
QUOTED_FIXTURE="$TMP_ROOT/quoted-fixture"
QUOTED_SOURCE="$TMP_ROOT/local-source-quoted"
DEFAULT_BOOTSTRAP_FIXTURE="$TMP_ROOT/default-bootstrap-fixture"
FOUNDATION_BOOTSTRAP_FIXTURE="$TMP_ROOT/foundation-bootstrap-fixture"

mkdir -p "$REMOTE_FIXTURE" "$LOCAL_FIXTURE" "$QUOTED_FIXTURE" "$DEFAULT_BOOTSTRAP_FIXTURE" "$FOUNDATION_BOOTSTRAP_FIXTURE"
git -C "$REMOTE_FIXTURE" init -q
git -C "$LOCAL_FIXTURE" init -q
git -C "$QUOTED_FIXTURE" init -q
git -C "$DEFAULT_BOOTSTRAP_FIXTURE" init -q
git -C "$FOUNDATION_BOOTSTRAP_FIXTURE" init -q
cp -a "$ROOT_DIR" "$DIRTY_SOURCE"
cp -a "$ROOT_DIR" "$QUOTED_SOURCE"

initialize_local_source_fixture() {
  local fixture_root="$1"

  rm -rf "$fixture_root/.git"
  git -C "$fixture_root" init -q
  git -C "$fixture_root" config user.name 'Bubbles Validation'
  git -C "$fixture_root" config user.email 'validation@invalid'
  git -C "$fixture_root" add -A
  git -C "$fixture_root" commit -q -m 'install provenance fixture'
}

initialize_local_source_fixture "$DIRTY_SOURCE"
initialize_local_source_fixture "$QUOTED_SOURCE"
printf '\n# install provenance selftest dirty marker\n' >> "$DIRTY_SOURCE/CHANGELOG.md"
printf '#!/usr/bin/env bash\n# untracked source projection probe\n' > "$DIRTY_SOURCE/bubbles/scripts/__untracked_source_probe.sh"
chmod +x "$DIRTY_SOURCE/bubbles/scripts/__untracked_source_probe.sh"
git -C "$QUOTED_SOURCE" checkout -q -b 'scope02"quoted'

echo "Running install-provenance selftest..."
echo "Scenario: downstream installs capture release metadata and provenance for release and local-source modes."

(
  cd "$REMOTE_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main >/dev/null
)
remote_provenance="$REMOTE_FIXTURE/.github/bubbles/.install-source.json"
remote_manifest="$REMOTE_FIXTURE/.github/bubbles/release-manifest.json"

[[ -f "$remote_manifest" ]] && pass "Remote-ref install copies release manifest" || fail "Remote-ref install copies release manifest"
[[ -f "$remote_provenance" ]] && pass "Remote-ref install writes install provenance" || fail "Remote-ref install writes install provenance"

[[ "$(bubbles_json_string_field "$remote_provenance" installMode)" == 'remote-ref' ]] \
  && pass "Remote-ref provenance records install mode" \
  || fail "Remote-ref provenance records install mode"
[[ "$(bubbles_json_string_field "$remote_provenance" sourceRef)" == 'main' ]] \
  && pass "Remote-ref provenance records requested source ref" \
  || fail "Remote-ref provenance records requested source ref"
[[ "$(bubbles_json_bool_field "$remote_provenance" sourceDirty)" == 'false' ]] \
  && pass "Remote-ref provenance stays clean" \
  || fail "Remote-ref provenance stays clean"

default_bootstrap_output="$(
  cd "$DEFAULT_BOOTSTRAP_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap 2>&1
)"
default_bootstrap_config="$DEFAULT_BOOTSTRAP_FIXTURE/.specify/memory/bubbles.config.json"

grep -Fq '"adoptionProfile": "delivery"' "$default_bootstrap_config" \
  && pass "Default bootstrap records delivery as the active adoption profile" \
  || fail "Default bootstrap records delivery as the active adoption profile"
grep -Fq 'Delivery remains the installer default during the current rollout.' <<< "$default_bootstrap_output" \
  && pass "Default bootstrap keeps the installer default explicit" \
  || fail "Default bootstrap keeps the installer default explicit"

foundation_bootstrap_output="$(
  cd "$FOUNDATION_BOOTSTRAP_FIXTURE"
  BUBBLES_SOURCE_OVERRIDE_DIR="$ROOT_DIR" bash "$ROOT_DIR/install.sh" main --bootstrap --profile foundation 2>&1
)"
foundation_bootstrap_config="$FOUNDATION_BOOTSTRAP_FIXTURE/.specify/memory/bubbles.config.json"

grep -Fq '"adoptionProfile": "foundation"' "$foundation_bootstrap_config" \
  && pass "Foundation bootstrap records foundation in repo-local policy state" \
  || fail "Foundation bootstrap records foundation in repo-local policy state"
grep -Fq 'Active adoption profile: Foundation (foundation)' <<< "$foundation_bootstrap_output" \
  && pass "Foundation bootstrap output names the selected profile explicitly" \
  || fail "Foundation bootstrap output names the selected profile explicitly"
grep -Fq 'Foundation was selected explicitly; the installer default still remains delivery.' <<< "$foundation_bootstrap_output" \
  && pass "Foundation bootstrap keeps the installer default unchanged" \
  || fail "Foundation bootstrap keeps the installer default unchanged"

(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" >/dev/null
)
local_provenance="$LOCAL_FIXTURE/.github/bubbles/.install-source.json"
local_manifest="$LOCAL_FIXTURE/.github/bubbles/release-manifest.json"

[[ -f "$local_manifest" ]] && pass "Local-source install copies generated release manifest" || fail "Local-source install copies generated release manifest"
[[ -f "$local_provenance" ]] && pass "Local-source install writes install provenance" || fail "Local-source install writes install provenance"

[[ "$(bubbles_json_string_field "$local_provenance" installMode)" == 'local-source' ]] \
  && pass "Local-source provenance records install mode" \
  || fail "Local-source provenance records install mode"
[[ -n "$(bubbles_json_string_field "$local_provenance" sourceRef)" ]] \
  && pass "Local-source provenance records a symbolic source ref" \
  || fail "Local-source provenance records a symbolic source ref"
[[ "$(bubbles_json_bool_field "$local_provenance" sourceDirty)" == 'true' ]] \
  && pass "Local-source provenance records dirty working tree risk" \
  || fail "Local-source provenance records dirty working tree risk"
assert_no_path_leak "$local_provenance" "$DIRTY_SOURCE" "Local-source provenance never persists the absolute checkout path"
[[ ! -e "$LOCAL_FIXTURE/.github/bubbles/scripts/__untracked_source_probe.sh" ]] \
  && pass "Local-source install excludes untracked source scripts absent from release provenance" \
  || fail "Local-source install excludes untracked source scripts absent from release provenance"
! grep -Fxq 'bubbles/scripts/__untracked_source_probe.sh' "$LOCAL_FIXTURE/.github/bubbles/.manifest" \
  && [[ -z "$(checksum_snapshot_value "$LOCAL_FIXTURE/.github/bubbles/.checksums" 'bubbles/scripts/__untracked_source_probe.sh')" ]] \
  && pass "Untracked source script is absent from manifest and checksum provenance" \
  || fail "Untracked source script is absent from manifest and checksum provenance"

# Stale-script prune: a framework script removed upstream must NOT linger in a
# downstream after re-install. Plant an orphan in the installed scripts dir,
# re-install from source, and assert the orphan is gone while a real script
# survives. (Regression for the v7.3.x orphan guard scripts that broke
# registry-consistency in downstreams when a removed script kept a stale gate
# reference.)
local_scripts_dir="$LOCAL_FIXTURE/.github/bubbles/scripts"
printf '#!/usr/bin/env bash\n# orphan prune probe (intentionally stale)\n' > "$local_scripts_dir/__orphan_prune_probe.sh"
(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" >/dev/null
)
[[ ! -e "$local_scripts_dir/__orphan_prune_probe.sh" ]] \
  && pass "Re-install prunes a stale framework script not present in source" \
  || fail "Re-install prunes a stale framework script not present in source"
[[ -f "$local_scripts_dir/framework-validate.sh" ]] \
  && pass "Re-install keeps real framework scripts after prune" \
  || fail "Re-install keeps real framework scripts after prune"
! grep -Fxq 'bubbles/scripts/__orphan_prune_probe.sh' "$LOCAL_FIXTURE/.github/bubbles/.manifest" \
  && [[ -z "$(checksum_snapshot_value "$LOCAL_FIXTURE/.github/bubbles/.checksums" 'bubbles/scripts/__orphan_prune_probe.sh')" ]] \
  && pass "Pruned framework script is absent from manifest and checksum provenance" \
  || fail "Pruned framework script is absent from manifest and checksum provenance"

# IMP-008: the orphan prune now also covers agents/prompts/instructions/skills,
# keyed on the PREVIOUS install's manifest so a framework file removed upstream
# is pruned while operator-authored files (never in the manifest) survive.
local_github="$LOCAL_FIXTURE/.github"
mkdir -p "$local_github/instructions" "$local_github/skills/bubbles-__orphan_probe" "$local_github/skills/operator-custom"
# (a) framework-pattern orphans recorded in the OLD manifest (= previously
#     framework-owned) but absent from the new source payload.
printf -- '---\nname: probe\n---\n' >"$local_github/agents/bubbles.__orphan_probe.agent.md"
printf -- '---\nname: probe\n---\n' >"$local_github/prompts/bubbles.__orphan_probe.prompt.md"
printf -- '# orphan instruction probe\n' >"$local_github/instructions/bubbles-__orphan_probe.instructions.md"
printf -- '# orphan skill probe\n' >"$local_github/skills/bubbles-__orphan_probe/SKILL.md"
{
  echo "agents/bubbles.__orphan_probe.agent.md"
  echo "prompts/bubbles.__orphan_probe.prompt.md"
  echo "instructions/bubbles-__orphan_probe.instructions.md"
  echo "skills/bubbles-__orphan_probe/SKILL.md"
} >>"$local_github/bubbles/.manifest"
# (b) operator-owned files that are NOT in the manifest — must survive.
printf -- '# operator instruction\n' >"$local_github/instructions/operator-custom.instructions.md"
printf -- '# operator skill\n' >"$local_github/skills/operator-custom/SKILL.md"

(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" >/dev/null
)

[[ ! -e "$local_github/agents/bubbles.__orphan_probe.agent.md" ]] \
  && pass "Re-install prunes an orphan framework agent (IMP-008)" \
  || fail "Re-install prunes an orphan framework agent (IMP-008)"
[[ ! -e "$local_github/prompts/bubbles.__orphan_probe.prompt.md" ]] \
  && pass "Re-install prunes an orphan framework prompt (IMP-008)" \
  || fail "Re-install prunes an orphan framework prompt (IMP-008)"
[[ ! -e "$local_github/instructions/bubbles-__orphan_probe.instructions.md" ]] \
  && pass "Re-install prunes an orphan framework instruction (IMP-008)" \
  || fail "Re-install prunes an orphan framework instruction (IMP-008)"
[[ ! -d "$local_github/skills/bubbles-__orphan_probe" ]] \
  && pass "Re-install prunes an orphan framework skill directory (IMP-008)" \
  || fail "Re-install prunes an orphan framework skill directory (IMP-008)"
[[ -f "$local_github/agents/bubbles.goal.agent.md" ]] \
  && pass "Re-install keeps real framework agents after prune (IMP-008)" \
  || fail "Re-install keeps real framework agents after prune (IMP-008)"
[[ -f "$local_github/instructions/operator-custom.instructions.md" ]] \
  && pass "Re-install keeps operator-owned instructions not in the manifest (IMP-008)" \
  || fail "Re-install keeps operator-owned instructions not in the manifest (IMP-008)"
[[ -f "$local_github/skills/operator-custom/SKILL.md" ]] \
  && pass "Re-install keeps operator-owned skills not in the manifest (IMP-008)" \
  || fail "Re-install keeps operator-owned skills not in the manifest (IMP-008)"

(
  cd "$QUOTED_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$QUOTED_SOURCE" >/dev/null
)
quoted_provenance="$QUOTED_FIXTURE/.github/bubbles/.install-source.json"
grep -Fq '"sourceRef": "local-source"' "$quoted_provenance" \
  && pass "Unsafe local-source refs fall back to literal local-source provenance" \
  || fail "Unsafe local-source refs fall back to literal local-source provenance"

# BUG-009 S08: exercise the real installer and provenance artifacts for every
# changed install-managed interface. The persistent regression is intentionally
# source-only: release provenance covers it, but downstream install provenance
# must not claim that it was copied.
bug_009_executable_paths=(
  bubbles/scripts/audit-result-contract-lint-selftest.sh
  bubbles/scripts/audit-result-contract-lint.sh
  bubbles/scripts/framework-validate.sh
  bubbles/scripts/state-transition-guard-perf-selftest.sh
  bubbles/scripts/state-transition-guard-selftest.sh
  bubbles/scripts/state-transition-guard.sh
  bubbles/scripts/transition-contract-resolver-selftest.sh
  bubbles/scripts/transition-contract-resolver.sh
)
bug_009_managed_paths=(
  bubbles/schemas/workflows.schema.json
  bubbles/workflows/modes.yaml
  bubbles/workflows.yaml
  agents/bubbles.audit.agent.md
  agents/bubbles.validate.agent.md
  agents/bubbles_shared/feature-templates.md
  agents/bubbles_shared/scope-templates.md
  agents/bubbles_shared/scope-workflow.md
  agents/bubbles_shared/validation-profiles.md
  agents/bubbles_shared/workflow-phase-engine.md
  docs/guides/AGENT_MANUAL.md
  docs/guides/CONTROL_PLANE_DESIGN.md
  docs/recipes/framework-ops.md
)

for bug_009_path in "${bug_009_executable_paths[@]}"; do
  assert_bug_009_managed_install "$bug_009_path" true
done
for bug_009_path in "${bug_009_managed_paths[@]}"; do
  assert_bug_009_managed_install "$bug_009_path"
done
assert_bug_009_source_only_release_entry 'tests/regression/test_23_planning_audit_contract.sh'

# BUG-013: the semantic classifier is a managed dependency of Scan 2B, while
# the persistent regression remains source-only.
assert_bug_009_managed_install \
  'bubbles/scripts/guards/sensitive-client-storage-scan.py' \
  false \
  'BUG-013'
assert_bug_009_managed_install \
  'bubbles/scripts/implementation-reality-scan.sh' \
  true \
  'BUG-013'
assert_bug_009_managed_install \
  'bubbles/scripts/implementation-reality-scan-selftest.sh' \
  true \
  'BUG-013'
assert_bug_009_managed_install \
  'agents/bubbles_shared/project-config-contract.md' \
  false \
  'BUG-013'
assert_bug_009_source_only_release_entry \
  'tests/regression/test_24_g028_sensitive_client_storage.sh' \
  'BUG-013'

# BUG-018: the production guard and its managed selftest install downstream;
# the persistent production-path regression remains source-only.
assert_bug_009_managed_install \
  'bubbles/scripts/traceability-guard.sh' \
  true \
  'BUG-018'
assert_bug_009_managed_install \
  'bubbles/scripts/traceability-guard-selftest.sh' \
  true \
  'BUG-018'
assert_bug_009_source_only_release_entry \
  'tests/regression/test_25_traceability_test_plan_heading_depth.sh' \
  'BUG-018'

# BUG-019: Check 8 and its selftest remain install-managed; the persistent
# production-path regression is release-recorded but source-only.
assert_bug_009_managed_install \
  'bubbles/scripts/state-transition-guard.sh' \
  true \
  'BUG-019'
assert_bug_009_managed_install \
  'bubbles/scripts/state-transition-guard-selftest.sh' \
  true \
  'BUG-019'
assert_bug_009_source_only_release_entry \
  'tests/regression/test_26_state_transition_spec_mjs_path.sh' \
  'BUG-019'

# IMP-020 S2: top-level governance scripts are install-managed. bubbles/eval is
# source-only under the current installer; the aggregator embeds its matching
# manual validator and does not load this schema at runtime, so downstream
# execution remains complete without fabricating a managed schema installation.
imp_020_s2_executable_paths=(
  bubbles/scripts/adversarial-resolve.sh
  bubbles/scripts/adversarial-resolve-selftest.sh
  bubbles/scripts/adversarial-aggregate.sh
  bubbles/scripts/adversarial-aggregate-selftest.sh
)
for imp_020_s2_path in "${imp_020_s2_executable_paths[@]}"; do
  assert_bug_009_managed_install "$imp_020_s2_path" true 'IMP-020 S2'
done
assert_bug_009_source_only_release_entry \
  'bubbles/eval/schemas/adversarial-sample.schema.json' \
  'IMP-020 S2' \
  'source-only schema' \
  'source-only schema'

# Prove checksum drift and an installed-file removal are detected, then prove a
# supported re-install repairs bytes, mode, membership, and checksum provenance.
drift_probe="$LOCAL_FIXTURE/.github/bubbles/scripts/transition-contract-resolver.sh"
printf '\n# BUG-009 install drift probe\n' >> "$drift_probe"
chmod -x "$drift_probe"
if bubbles_source_bundle_clean "$LOCAL_FIXTURE/.github"; then
  fail "BUG-009 checksum snapshot detects managed-file byte drift"
else
  pass "BUG-009 checksum snapshot detects managed-file byte drift"
fi
(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" >/dev/null
)
assert_bug_009_managed_install 'bubbles/scripts/transition-contract-resolver.sh' true

removal_probe="$LOCAL_FIXTURE/.github/bubbles/scripts/audit-result-contract-lint.sh"
rm -f "$removal_probe"
if bubbles_source_bundle_clean "$LOCAL_FIXTURE/.github"; then
  fail "BUG-009 checksum snapshot detects managed-file removal"
else
  pass "BUG-009 checksum snapshot detects managed-file removal"
fi
(
  cd "$LOCAL_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$DIRTY_SOURCE" >/dev/null
)
assert_bug_009_managed_install 'bubbles/scripts/audit-result-contract-lint.sh' true

# ─────────────────────────────────────────────────────────────────────────────
# v5.0.1 H5: post-install fixture assertions — would have caught the latest
# class of bugs (adapter dir missing, .gitignore written to wrong target).
# ─────────────────────────────────────────────────────────────────────────────
adapters_dir="$LOCAL_FIXTURE/.github/bubbles/adapters/observability"
[[ -d "$adapters_dir" ]] \
  && pass "Local install creates adapters/observability/ directory" \
  || fail "Local install creates adapters/observability/ directory"

for adapter in none.sh prometheus.sh; do
  if [[ -f "$adapters_dir/$adapter" ]]; then
    pass "Adapter installed: $adapter"
  else
    fail "Adapter missing: $adapter"
  fi
  if [[ -x "$adapters_dir/$adapter" ]]; then
    pass "Adapter is executable: $adapter"
  else
    fail "Adapter not executable: $adapter"
  fi
done

# v5.0.1 H4 follow-up: schemas directory landed downstream so
# yaml-schema-validate.sh can run there.
schemas_dir="$LOCAL_FIXTURE/.github/bubbles/schemas"
[[ -d "$schemas_dir" ]] \
  && pass "Local install creates schemas/ directory" \
  || fail "Local install creates schemas/ directory"
for schema in workflows.schema.json capability-ledger.schema.json adoption-profiles.schema.json; do
  if [[ -f "$schemas_dir/$schema" ]]; then
    pass "Schema installed: $schema"
  else
    fail "Schema missing: $schema"
  fi
done

# improvements/ MUST be appended to the repo-root .gitignore (NOT
# .github/.gitignore). The earlier installer bug wrote to TARGET/.gitignore
# where TARGET=.github, which silently produced a useless .github/.gitignore.
root_gitignore="$LOCAL_FIXTURE/.gitignore"
[[ -f "$root_gitignore" ]] \
  && pass "Local install creates/preserves repo-root .gitignore" \
  || fail "Local install creates/preserves repo-root .gitignore"

grep -qx 'improvements/' "$root_gitignore" 2>/dev/null \
  && pass "Repo-root .gitignore contains improvements/ entry" \
  || fail "Repo-root .gitignore contains improvements/ entry (yesterday's installer bug)"

# Negative assertion: there MUST NOT be a stray .github/.gitignore created
# by the installer (regression check for the misdirected-write bug).
github_gitignore="$LOCAL_FIXTURE/.github/.gitignore"
if [[ -f "$github_gitignore" ]] && grep -q 'improvements/' "$github_gitignore" 2>/dev/null; then
  fail "Installer wrote improvements/ to .github/.gitignore instead of repo root"
else
  pass "Installer did NOT create stray .github/.gitignore with improvements/"
fi

# Manifest count: ensures release-manifest.json is non-empty and well-formed.
manifest="$LOCAL_FIXTURE/.github/bubbles/release-manifest.json"
managed_count="$(python3 -c "import json; d=json.load(open('$manifest')); print(d.get('managedFileCount', 0))" 2>/dev/null || echo 0)"
if [[ "${managed_count:-0}" -ge 300 ]]; then
  pass "Installed manifest reports $managed_count managed files (>=300 sanity floor)"
else
  fail "Installed manifest reports only $managed_count managed files (<300 — manifest enumeration regressed)"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "install-provenance selftest failed with $failures issue(s)."
  exit 1
fi

echo "install-provenance selftest passed."