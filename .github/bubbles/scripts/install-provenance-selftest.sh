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
printf '\n# install provenance selftest dirty marker\n' >> "$DIRTY_SOURCE/CHANGELOG.md"
git -C "$QUOTED_SOURCE" init -q
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

(
  cd "$QUOTED_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$QUOTED_SOURCE" >/dev/null
)
quoted_provenance="$QUOTED_FIXTURE/.github/bubbles/.install-source.json"
grep -Fq '"sourceRef": "local-source"' "$quoted_provenance" \
  && pass "Unsafe local-source refs fall back to literal local-source provenance" \
  || fail "Unsafe local-source refs fall back to literal local-source provenance"

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