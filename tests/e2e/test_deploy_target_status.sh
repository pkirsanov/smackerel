#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

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

write_params() {
  local adapter_dir="$1"
  local target="$2"

  cat >"$adapter_dir/params.yaml" <<PARAMS
apiVersion: bubbles.deploy/v1
kind: DeployTargetParams
metadata:
  name: $target
  project: smackerel
runtime:
  composeProject: smackerel-$target
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

echo "=== SCN-009-017: product deploy-target status delegates to adapter status.sh ==="
target="fixture-status-e2e"
adapter_dir="$(make_adapter_root "$TMP_ROOT" "$target")"
cat >"$adapter_dir/status.sh" <<'STATUS'
#!/usr/bin/env bash
set -euo pipefail
echo "ADAPTER-STATUS-DELEGATED"
echo "status-args:$*"
echo "Overall status: runtime-drift"
echo "Contract drift: fixture adapter text surfaced through product CLI"
STATUS
chmod +x "$adapter_dir/status.sh"

delegated_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT" "$REPO_DIR/smackerel.sh" deploy-target "$target" status --explain 2>&1)"
assert_contains "$delegated_output" "ADAPTER-STATUS-DELEGATED" "product CLI delegated output"
assert_contains "$delegated_output" "status-args:--explain" "product CLI delegated args"
assert_contains "$delegated_output" "Overall status: runtime-drift" "product CLI adapter drift text"
assert_contains "$delegated_output" "Contract drift: fixture adapter text surfaced through product CLI" "product CLI adapter contract text"

echo "=== SCN-009-018: product deploy-target status fallback is explicit and read-only ==="
fallback_target="fixture-status-e2e-fallback"
make_adapter_root "$TMP_ROOT" "$fallback_target" >/dev/null
fallback_output="$(DEPLOY_TARGETS_ROOT="$TMP_ROOT" "$REPO_DIR/smackerel.sh" deploy-target "$fallback_target" status 2>&1)"
assert_contains "$fallback_output" "adapter status script unavailable" "product CLI fallback output"
assert_contains "$fallback_output" "read-only generic fallback" "product CLI fallback output"
assert_contains "$fallback_output" "target: $fallback_target" "product CLI fallback target"
assert_not_contains "$fallback_output" "ADAPTER-STATUS-DELEGATED" "product CLI fallback output"

echo "PASS: deploy-target status delegation and fallback fixture"