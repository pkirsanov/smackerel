#!/usr/bin/env bash
set -uo pipefail

# observability-opt-out-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/observability-opt-out-guard.sh`
# (Gate G099 — observability_opt_out_freshness_gate).
#
# Like the posture-guard selftest (and release-train-guard-selftest.sh), the
# hermetic workspace is staged UNDER $HOME because the reference `yq` is often
# snap-confined and cannot read `/tmp`.
#
# Cases (matches IMP-001 SCOPE-2 T2.3):
#   opted-out-fresh             → exit 0, no reminder
#   opted-out-expired           → exit 0, route-required reminder (non-blocking)
#   opted-out-missing-revisitAfter → exit 1, malformed (fail loud)
#   malformed (no optOut block) → exit 1, fail loud
#   wired                       → exit 0, no-op
#   undeclared                  → exit 0, no-op
#   unsupported schema          → exit 1, fail loud before semantics
#   missing-parser              → exit 0, WARN-and-skip (PATH stripped of yq)
#   framework-repo-exempt       → exit 0, no-op (no reminder)
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD="$SCRIPT_DIR/observability-opt-out-guard.sh"
FIXTURES="$SCRIPT_DIR/../tests/fixtures/observability"
BASH_BIN="$(command -v bash)"

if [[ ! -x "$GUARD" ]]; then
  echo "observability-opt-out-guard-selftest: guard not executable: $GUARD" >&2
  exit 2
fi

if ! command -v yq >/dev/null 2>&1; then
  echo "SKIP: observability-opt-out-guard-selftest (yq not installed)"
  exit 0
fi

WORKSPACE="$(mktemp -d "${HOME}/.bubbles-selftest-obs-optout.XXXXXX")"
cleanup() { rm -rf "$WORKSPACE"; }
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

ok() { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { printf '[selftest] FAIL: %s\n' "$*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }

stage_fixture() {
  local name="$1" fixture="$2"
  mkdir -p "$WORKSPACE/$name/.github"
  cp "$FIXTURES/$fixture" "$WORKSPACE/$name/.github/bubbles-project.yaml"
}
stage_inline() {
  local name="$1" content="$2"
  mkdir -p "$WORKSPACE/$name/.github"
  printf '%s' "$content" > "$WORKSPACE/$name/.github/bubbles-project.yaml"
}

RC=""; OUT=""
run_guard() {
  local root="$1"
  local of="$WORKSPACE/out.last"
  "$BASH_BIN" "$GUARD" --repo-root "$root" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}
run_guard_no_parser() {
  local root="$1"
  local empty="$WORKSPACE/emptybin"
  mkdir -p "$empty"
  local of="$WORKSPACE/out.last"
  PATH="$empty" "$BASH_BIN" "$GUARD" --repo-root "$root" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}

assert_exit() {
  local want="$1" label="$2"
  if [[ "$RC" == "$want" ]]; then ok "$label (exit $RC)"; else
    ko "$label: expected exit $want, got $RC"; printf '  --- output ---\n%s\n' "$OUT" >&2
  fi
}
assert_contains() {
  local needle="$1" label="$2"
  if grep -qiF -- "$needle" <<<"$OUT"; then ok "$label (contains '$needle')"; else
    ko "$label: output missing '$needle'"; printf '  --- output ---\n%s\n' "$OUT" >&2
  fi
}

# --- Stage fixtures ------------------------------------------------------
stage_fixture opted-out-fresh      posture-opted-out-fresh.yaml
stage_fixture opted-out-expired    posture-opted-out-expired.yaml
stage_fixture malformed            posture-malformed.yaml
stage_fixture unsupported-schema   posture-unsupported-schema-version.yaml
stage_fixture wired                posture-wired.yaml
stage_inline   undeclared          $'traceContracts:\n  workflows: {}\n'
stage_inline   missing-revisit     $'traceContracts:\n  observability:\n    schemaVersion: 1\n    posture: opted-out\n    optOut:\n      reasonCode: no-runtime\n      reason: "framework source repo; nothing to monitor"\n'

# Framework-source-exempt synth repo carrying an EXPIRED opt-out the exemption
# must short-circuit (proves source repo never raises a reminder).
mkdir -p "$WORKSPACE/fwrepo/bubbles/scripts" "$WORKSPACE/fwrepo/.github"
printf '9.9.9\n' > "$WORKSPACE/fwrepo/VERSION"
printf '#!/usr/bin/env bash\necho installer\n' > "$WORKSPACE/fwrepo/install.sh"
cp "$FIXTURES/posture-opted-out-expired.yaml" "$WORKSPACE/fwrepo/.github/bubbles-project.yaml"

# --- Assertions ----------------------------------------------------------
run_guard "$WORKSPACE/opted-out-fresh";    assert_exit 0 "opted-out-fresh clean"; assert_contains "FRESH" "fresh message"
run_guard "$WORKSPACE/opted-out-expired";  assert_exit 0 "opted-out-expired reminder non-blocking"; assert_contains "EXPIRED" "expired message"; assert_contains "route-required" "expired reminder is route-required"
run_guard "$WORKSPACE/missing-revisit";    assert_exit 1 "opted-out missing revisitAfter rejected"; assert_contains "revisitAfter" "missing-revisitAfter message"
run_guard "$WORKSPACE/malformed";          assert_exit 1 "opted-out no optOut rejected"; assert_contains "optOut" "malformed message"
run_guard "$WORKSPACE/unsupported-schema"; assert_exit 1 "unsupported schema rejected"; assert_contains "schemaVersion" "unsupported-schema message"
run_guard "$WORKSPACE/wired";              assert_exit 0 "wired no-op"
run_guard "$WORKSPACE/undeclared";         assert_exit 0 "undeclared no-op"
run_guard_no_parser "$WORKSPACE/opted-out-expired"; assert_exit 0 "missing-parser WARN-and-skip"; assert_contains "WARN-and-skip" "missing-parser message"
run_guard "$WORKSPACE/fwrepo";             assert_exit 0 "framework-repo exempt no-op"; assert_contains "EXEMPT" "framework-exempt message"

echo ""
echo "observability-opt-out-guard-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "observability-opt-out-guard selftest passed."
  exit 0
else
  echo "observability-opt-out-guard selftest FAILED." >&2
  exit 1
fi
