#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
DISPATCHER="$REPO_ROOT/scripts/commands/deploy_target.sh"

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"

  if [[ "$haystack" != *"$needle"* ]]; then
    echo "----- captured output for $label -----" >&2
    printf '%s\n' "$haystack" >&2
    echo "----- end captured output -----" >&2
    fail "$label did not contain: $needle"
  fi
  echo "PASS: $label contains $needle"
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"

  if [[ "$haystack" == *"$needle"* ]]; then
    echo "----- captured output for $label -----" >&2
    printf '%s\n' "$haystack" >&2
    echo "----- end captured output -----" >&2
    fail "$label unexpectedly contained: $needle"
  fi
  echo "PASS: $label excludes $needle"
}

require_file() {
  local path="$1"
  local label="$2"

  [[ -f "$path" ]] || fail "$label missing at $path"
  echo "PASS: $label exists"
}

write_params() {
  local adapter_dir="$1"
  local name="$2"

  cat >"$adapter_dir/params.yaml" <<PARAMS
apiVersion: bubbles.deploy/v1
kind: DeployTargetParams
metadata:
  name: $name
  project: smackerel
runtime:
  composeProject: smackerel-$name
PARAMS
}

make_adapter_root() {
  local root="$1"
  local target="$2"
  local adapter_dir="$root/smackerel/$target"

  mkdir -p "$adapter_dir"
  write_params "$adapter_dir" "$target"
  printf '%s\n' "$adapter_dir"
}

TMP_ROOT="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

require_file "$DISPATCHER" "deploy-target dispatcher"

echo "--- SCN-009-017: executable adapter status.sh receives product status control ---"
delegate_target="fixture-status-delegates"
delegate_adapter="$(make_adapter_root "$TMP_ROOT" "$delegate_target")"
cat >"$delegate_adapter/status.sh" <<'STATUS'
#!/usr/bin/env bash
set -euo pipefail
echo "ADAPTER-STATUS-DELEGATED"
echo "status-args:$*"
echo "Overall status: runtime-drift"
STATUS
chmod +x "$delegate_adapter/status.sh"

delegate_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT" bash "$DISPATCHER" "$delegate_target" status --detail drift 2>&1)"
assert_contains "$delegate_output" "ADAPTER-STATUS-DELEGATED" "delegated status output"
assert_contains "$delegate_output" "status-args:--detail drift" "delegated status args"
assert_contains "$delegate_output" "Overall status: runtime-drift" "delegated adapter drift text"
assert_not_contains "$delegate_output" "adapter status script unavailable" "delegated status output"

echo "--- SCN-009-018: missing adapter status.sh uses explicit read-only fallback ---"
fallback_target="fixture-status-fallback"
make_adapter_root "$TMP_ROOT" "$fallback_target" >/dev/null
fallback_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT" bash "$DISPATCHER" "$fallback_target" status 2>&1)"
assert_contains "$fallback_output" "adapter status script unavailable" "fallback status output"
assert_contains "$fallback_output" "read-only generic fallback" "fallback status output"
assert_contains "$fallback_output" "target: $fallback_target" "fallback target summary"
assert_not_contains "$fallback_output" "ADAPTER-STATUS-DELEGATED" "fallback status output"

echo "--- SCN-009-018: non-executable adapter status.sh still uses fallback ---"
nonexec_target="fixture-status-nonexec"
nonexec_adapter="$(make_adapter_root "$TMP_ROOT" "$nonexec_target")"
cat >"$nonexec_adapter/status.sh" <<'STATUS'
#!/usr/bin/env bash
echo "THIS SHOULD NOT RUN"
STATUS
chmod 0644 "$nonexec_adapter/status.sh"
nonexec_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT" bash "$DISPATCHER" "$nonexec_target" status 2>&1)"
assert_contains "$nonexec_output" "adapter status script unavailable" "non-executable fallback output"
assert_contains "$nonexec_output" "read-only generic fallback" "non-executable fallback output"
assert_not_contains "$nonexec_output" "THIS SHOULD NOT RUN" "non-executable fallback output"

echo "--- FR-038: DEPLOY_TARGETS_ROOT remains strict for status ---"
strict_output=""
strict_status=0
set +e
strict_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT/missing-root" bash "$DISPATCHER" "$delegate_target" status 2>&1)"
strict_status=$?
set -e
if [[ "$strict_status" -eq 0 ]]; then
  fail "strict DEPLOY_TARGETS_ROOT status lookup unexpectedly succeeded"
fi
assert_contains "$strict_output" "DEPLOY_TARGETS_ROOT is set" "strict resolution error"
assert_contains "$strict_output" "NOT consulted (in-tree)" "strict resolution error"

echo "--- Product boundary: dispatcher remains generic ---"
dispatcher_content="$(cat "$DISPATCHER")"
assert_not_contains "$dispatcher_content" "40001" "dispatcher source"
assert_not_contains "$dispatcher_content" "40002" "dispatcher source"
assert_not_contains "$dispatcher_content" ".ts.net" "dispatcher source"

echo "deploy_target_status_test.sh OK"