#!/usr/bin/env bash
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/admission-selftest.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

mbe_output="$(python3 "$SCRIPT_DIR/measured-budget-runtime-v2-selftest.py" 2>&1)"
printf '%s\n' "$mbe_output"
for required_scenario in \
  test_independent_action_authorization_denial_blocks_permit \
  test_reference_broker_constructs_settlement_and_refuses_replay \
  test_reference_broker_action_mismatch_never_starts_child \
  test_reference_broker_denial_never_starts_child \
  test_full_graph_is_acyclic_bound_and_one_use; do
  [[ "$mbe_output" == *"$required_scenario"*" ... ok"* ]] || {
    echo "admission-contract-selftest: required production scenario did not pass: $required_scenario" >&2
    exit 1
  }
done
if grep -Eq '(^|[^A-Za-z])(skip|skipIf|skipUnless|expectedFailure)[[:space:]]*\(' "$SCRIPT_DIR/measured-budget-runtime-v2-selftest.py"; then
  echo "admission-contract-selftest: required production scenarios contain disabled-test markers" >&2
  exit 1
fi
echo "ok - required denial, replay, action-binding, settlement, and one-use scenarios pass through production paths"

default_dir="$tmp/default"
typo_dir="$tmp/typo"
mkdir -p "$default_dir" "$typo_dir"
[[ "$("$SCRIPT_DIR/dispatch-adapter-resolve.sh" --repo-root "$default_dir" --names-only)" == 'adapter=none' ]]
none_description="$($SCRIPT_DIR/../adapters/dispatch/none.sh describe)"
[[ "$(printf '%s' "$none_description" | jq -r '.permitIssuance')" == false ]]
if "$SCRIPT_DIR/../adapters/dispatch/none.sh" dispatch >/dev/null 2>&1; then
  echo "admission-contract-selftest: default-off adapter dispatched a child" >&2
  exit 1
fi
echo "ok - absent adapter resolves to none and cannot dispatch"
cat >"$typo_dir/bubbles-project.yaml" <<'EOF'
dispatchAdmission:
  adapter: typo-adapter
EOF
if "$SCRIPT_DIR/dispatch-adapter-resolve.sh" --repo-root "$typo_dir" >/dev/null 2>&1; then
  echo "admission-contract-selftest: configured typo did not fail" >&2
  exit 1
fi
echo "ok - configured typo adapter fails loud"

first="$($SCRIPT_DIR/risk-tier-resolve.sh --surface 'static html' --typed-json)"
second="$($SCRIPT_DIR/risk-tier-resolve.sh --surface 'static html' --typed-json)"
[[ "$first" == "$second" ]]
[[ "$(printf '%s' "$first" | jq -r '.decisionDigest')" == sha256:* ]]

if grep -Eq -- '--(skip|force|ignore|insecure|no-verify)' "$SCRIPT_DIR/dispatch-admission.sh" "$SCRIPT_DIR/goal-budget-ledger.sh" "$SCRIPT_DIR/session-epoch-authority.sh" "$SCRIPT_DIR/dispatch-adapter-resolve.sh" "$SCRIPT_DIR/../adapters/dispatch/none.sh" "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh"; then
  echo "admission-contract-selftest: bypass flag found" >&2
  exit 1
fi
echo "ok - admission and broker surfaces expose no bypass flags"
echo "admission-contract-selftest: PASS"