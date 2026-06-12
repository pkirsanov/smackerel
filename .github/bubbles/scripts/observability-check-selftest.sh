#!/usr/bin/env bash
set -uo pipefail

# observability-check-selftest.sh
#
# Hermetic selftest for observability-check.sh, the canonical bash twin behind
# the MCP check_observability tool. This proves the full JSON envelope on a
# WIRED repo fixture, including the endpoints block populated by the real
# observability-endpoint-resolve.sh --names-only consumer path.

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CHECK="$SCRIPT_DIR/observability-check.sh"
FIXTURES="$REPO_ROOT/bubbles/tests/fixtures/observability"

if ! command -v jq >/dev/null 2>&1; then
  echo "SKIP: observability-check-selftest (jq not installed)"
  exit 0
fi
if [[ ! -x "$CHECK" ]]; then
  echo "observability-check-selftest: check twin not executable: $CHECK" >&2
  exit 1
fi

WORKSPACE="$HOME/.bubbles-observability-check-selftest"
rm -rf "$WORKSPACE"
mkdir -p "$WORKSPACE/.github"
trap 'rm -rf "$WORKSPACE"' EXIT

PASS_COUNT=0
FAIL_COUNT=0
ok() { echo "[selftest] PASS: $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { echo "[selftest] FAIL: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }

OUT=""
RC=0
run_check() {
  local repo="$1" of="$WORKSPACE/out.json" ef="$WORKSPACE/err.txt"
  bash "$CHECK" --repo-root "$repo" >"$of" 2>"$ef"
  RC=$?
  OUT="$(cat "$of")"
}

assert_exit() {
  local want="$1" label="$2"
  if [[ "$RC" == "$want" ]]; then ok "$label (exit $RC)"; else
    ko "$label: expected exit $want, got $RC"
    printf '  --- stdout ---\n%s\n  --- stderr ---\n%s\n' "$OUT" "$(cat "$WORKSPACE/err.txt")" >&2
  fi
}
assert_jq() {
  local filter="$1" label="$2"
  if jq -e "$filter" >/dev/null 2>&1 <<<"$OUT"; then ok "$label"; else
    ko "$label: jq filter failed [$filter]"
    printf '  --- json ---\n%s\n' "$OUT" >&2
  fi
}

# --- wired fixture: endpoint resolver consumed through observability-check ---
cp "$FIXTURES/posture-wired.yaml" "$WORKSPACE/.github/bubbles-project.yaml"
run_check "$WORKSPACE"
assert_exit 0 "wired fixture check exits ok (no instrumented scope, SLO no-op)"
assert_jq '.tool == "check_observability" and .schemaVersion == 1' "envelope has tool + schemaVersion"
assert_jq '.posture.state == "WIRED" and .posture.verdict == "ok"' "posture section reports WIRED"
assert_jq '.endpoints.validate.sloBurn == "prometheus"' "endpoints.validate.sloBurn reports prometheus"
assert_jq '.endpoints.validate.errorRate == "prometheus"' "endpoints.validate.errorRate reports prometheus"
assert_jq '.endpoints.validate.alerts == "none"' "endpoints.validate.alerts reports none"
assert_jq '.endpoints.operate.alerts == "prometheus"' "endpoints.operate.alerts reports prometheus"
assert_jq '.endpoints.operate.deployImpact == "prometheus"' "endpoints.operate.deployImpact reports prometheus"
assert_jq '.overall == "ok"' "overall verdict ok"

# --- source repo: EXEMPT still emits valid endpoints object --------------
run_check "$REPO_ROOT"
assert_exit 0 "source repo check exits ok"
assert_jq '.posture.state == "EXEMPT"' "source repo posture EXEMPT"
assert_jq '.endpoints.validate.alerts == "none" and .endpoints.operate.sloBurn == "none"' "source repo endpoints default to none"

echo ""
echo "observability-check-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "observability-check selftest passed."
  exit 0
fi
echo "observability-check selftest FAILED." >&2
exit 1
