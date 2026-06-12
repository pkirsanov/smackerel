#!/usr/bin/env bash
set -uo pipefail

# observability-posture-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/observability-posture-guard.sh`
# (Gate G098 — observability_posture_declared_gate).
#
# Stages throwaway repo surfaces (each carrying a `.github/bubbles-project.yaml`)
# and asserts the guard's exit codes + message fingerprints across every
# posture state.
#
# NOTE on workspace location: the reference `yq` is frequently snap-confined
# (strict confinement cannot read `/tmp`). Like `release-train-guard-selftest.sh`,
# this selftest stages its hermetic workspace UNDER $HOME so snap-yq can read
# the fixtures. Staging files inside a throwaway $HOME workspace is allowed by
# terminal-discipline policy (the workspace never becomes part of the working
# tree).
#
# Cases (matches IMP-001 SCOPE-2 T2.3):
#   undeclared            → exit 0, WARN nag
#   wired                 → exit 0, accepted
#   fake-wired            → exit 1, rejected
#   opted-out-fresh       → exit 0 (G098 accepts declared+well-formed opt-out)
#   opted-out-expired     → exit 0 (freshness is G099's job)
#   malformed (no optOut) → exit 1, rejected
#   unsupported schema    → exit 1, fail loud before semantics
#   undeclared+policy:block → exit 1, blocking
#   missing-parser        → exit 0, WARN-and-skip (PATH stripped of yq)
#   framework-repo-exempt → exit 0, EXEMPT, NO nag
#   --print-state tokens  → WIRED / OPTED-OUT-FRESH / OPTED-OUT-EXPIRED /
#                           EXEMPT / UNDECLARED (proves the doctor mapping)
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD="$SCRIPT_DIR/observability-posture-guard.sh"
FIXTURES="$SCRIPT_DIR/../tests/fixtures/observability"
BASH_BIN="$(command -v bash)"

if [[ ! -x "$GUARD" ]]; then
  echo "observability-posture-guard-selftest: guard not executable: $GUARD" >&2
  exit 2
fi

if ! command -v yq >/dev/null 2>&1; then
  echo "SKIP: observability-posture-guard-selftest (yq not installed)"
  exit 0
fi

WORKSPACE="$(mktemp -d "${HOME}/.bubbles-selftest-obs-posture.XXXXXX")"
cleanup() { rm -rf "$WORKSPACE"; }
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

ok() { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { printf '[selftest] FAIL: %s\n' "$*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }

# Stage a repo whose .github/bubbles-project.yaml is a copy of a shipped fixture.
stage_fixture() {
  local name="$1" fixture="$2"
  mkdir -p "$WORKSPACE/$name/.github"
  cp "$FIXTURES/$fixture" "$WORKSPACE/$name/.github/bubbles-project.yaml"
}

# Stage a repo whose .github/bubbles-project.yaml is inline content.
stage_inline() {
  local name="$1" content="$2"
  mkdir -p "$WORKSPACE/$name/.github"
  printf '%s' "$content" > "$WORKSPACE/$name/.github/bubbles-project.yaml"
}

RC=""; OUT=""
run_guard() {
  local root="$1"; shift
  local of="$WORKSPACE/out.last"
  "$BASH_BIN" "$GUARD" "$@" --repo-root "$root" >"$of" 2>&1
  RC=$?
  OUT="$(cat "$of")"
}

# Run the guard with a PATH stripped of yq (proves WARN-and-skip).
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
assert_not_contains() {
  local needle="$1" label="$2"
  if grep -qiF -- "$needle" <<<"$OUT"; then
    ko "$label: output unexpectedly contains '$needle'"; printf '  --- output ---\n%s\n' "$OUT" >&2
  else ok "$label (absent '$needle')"; fi
}
assert_state() {
  local root="$1" want="$2" label="$3"
  local got
  got="$("$BASH_BIN" "$GUARD" --print-state --repo-root "$root" 2>/dev/null)"
  if [[ "$got" == "$want" ]]; then ok "$label (--print-state '$got')"; else
    ko "$label: --print-state expected '$want', got '$got'"
  fi
}

# --- Stage fixtures ------------------------------------------------------
stage_fixture wired                posture-wired.yaml
stage_fixture fake-wired           posture-fake-wired.yaml
stage_fixture malformed            posture-malformed.yaml
stage_fixture unsupported-schema   posture-unsupported-schema-version.yaml
stage_fixture opted-out-fresh      posture-opted-out-fresh.yaml
stage_fixture opted-out-expired    posture-opted-out-expired.yaml
stage_inline   undeclared          $'traceContracts:\n  workflows:\n    booking.create:\n      requiredSpans:\n        - name: http.request\n'
stage_inline   undeclared-block    $'traceContracts:\n  observability:\n    schemaVersion: 1\n    policy:\n      undeclaredPosture: block\n'

# Framework-source-exempt synth repo (VERSION + install.sh + bubbles/scripts,
# and a declared wired posture that the exemption must short-circuit).
mkdir -p "$WORKSPACE/fwrepo/bubbles/scripts" "$WORKSPACE/fwrepo/.github"
printf '9.9.9\n' > "$WORKSPACE/fwrepo/VERSION"
printf '#!/usr/bin/env bash\necho installer\n' > "$WORKSPACE/fwrepo/install.sh"
cp "$FIXTURES/posture-wired.yaml" "$WORKSPACE/fwrepo/.github/bubbles-project.yaml"

# --- Gate-mode assertions ------------------------------------------------
run_guard "$WORKSPACE/undeclared";        assert_exit 0 "undeclared nag"; assert_contains "UNDECLARED" "undeclared message"; assert_contains "G098" "undeclared cites gate"
run_guard "$WORKSPACE/wired";             assert_exit 0 "wired accepted"; assert_contains "WIRED" "wired message"
run_guard "$WORKSPACE/fake-wired";        assert_exit 1 "fake-wired rejected"; assert_contains "fake-wired" "fake-wired message"
run_guard "$WORKSPACE/opted-out-fresh";   assert_exit 0 "opted-out-fresh accepted by G098"
run_guard "$WORKSPACE/opted-out-expired"; assert_exit 0 "opted-out-expired accepted by G098 (freshness is G099)"
run_guard "$WORKSPACE/malformed";         assert_exit 1 "malformed (no optOut) rejected"; assert_contains "optOut" "malformed message"
run_guard "$WORKSPACE/unsupported-schema";assert_exit 1 "unsupported schema rejected"; assert_contains "schemaVersion" "unsupported-schema message"
run_guard "$WORKSPACE/undeclared-block";  assert_exit 1 "undeclared+policy:block blocks"; assert_contains "block" "undeclared-block message"

# Missing parser → WARN-and-skip
run_guard_no_parser "$WORKSPACE/wired";   assert_exit 0 "missing-parser WARN-and-skip"; assert_contains "WARN-and-skip" "missing-parser message"

# Framework-repo exemption → EXEMPT, NO nag
run_guard "$WORKSPACE/fwrepo";            assert_exit 0 "framework-repo exempt"; assert_contains "EXEMPT" "framework-exempt message"; assert_not_contains "UNDECLARED" "framework-exempt has no nag"

# --- --print-state token assertions (prove the doctor mapping) -----------
assert_state "$WORKSPACE/wired"             "WIRED"                    "print-state wired"
assert_state "$WORKSPACE/opted-out-fresh"   "OPTED-OUT-FRESH|2099-06-11"   "print-state opted-out-fresh"
assert_state "$WORKSPACE/opted-out-expired" "OPTED-OUT-EXPIRED|2020-01-01" "print-state opted-out-expired"
assert_state "$WORKSPACE/undeclared"        "UNDECLARED"               "print-state undeclared"
assert_state "$WORKSPACE/fwrepo"            "EXEMPT"                   "print-state framework-exempt"

echo ""
echo "observability-posture-guard-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "observability-posture-guard selftest passed."
  exit 0
else
  echo "observability-posture-guard selftest FAILED." >&2
  exit 1
fi
