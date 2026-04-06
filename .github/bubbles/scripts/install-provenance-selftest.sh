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

(
  cd "$QUOTED_FIXTURE"
  bash "$ROOT_DIR/install.sh" --local-source "$QUOTED_SOURCE" >/dev/null
)
quoted_provenance="$QUOTED_FIXTURE/.github/bubbles/.install-source.json"
grep -Fq '"sourceRef": "local-source"' "$quoted_provenance" \
  && pass "Unsafe local-source refs fall back to literal local-source provenance" \
  || fail "Unsafe local-source refs fall back to literal local-source provenance"

if [[ "$failures" -gt 0 ]]; then
  echo "install-provenance selftest failed with $failures issue(s)."
  exit 1
fi

echo "install-provenance selftest passed."