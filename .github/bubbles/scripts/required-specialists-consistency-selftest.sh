#!/usr/bin/env bash
# bubbles/scripts/required-specialists-consistency-selftest.sh
#
# IMP-105 SCOPE-7 / ONT-UNIFY — hermetic adversarial selftest for
# required-specialists-consistency.sh.
#
# Proves the shadow-compare guard is REAL (non-tautological):
#   (a) LIVE  — the real bubbles/registry/required-specialists.yaml matches the
#               real state-transition-guard.sh Check 6 case (exit 0). This is
#               the continuous shadow-compare itself.
#   (b) FIXTURE POSITIVE — a hand-built consistent fixture (guard stub + matching
#               registry) exits 0, AND a decoy arm placed INSIDE the SCOPE-3
#               fallback region is correctly NOT parsed (proves the parser stops
#               at the # IMP-105-SCOPE-3-FALLBACK-BEGIN marker).
#   (c) MUTATED LIST  — a registry whose mode list is changed → FAILS (exit 1,
#               "list-mismatch").
#   (d) REMOVED MODE  — a mode present in the case but dropped from the registry
#               → FAILS (exit 1, "missing-in-registry").
#   (e) EXTRA MODE    — a mode added to the registry but absent from the case
#               → FAILS (exit 1, "extra-in-registry").
#
# Each negative case asserts the SPECIFIC divergence class + mode name, so the
# check is proven to detect each drift kind (not merely "some failure").
#
# Portable: bash 3.2 safe, ephemeral mktemp fixtures, PASS:/FAIL: lines,
# non-zero exit on any failure.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONSISTENCY="$SCRIPT_DIR/required-specialists-consistency.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

if [[ ! -x "$CONSISTENCY" && ! -f "$CONSISTENCY" ]]; then
  echo "required-specialists-consistency-selftest: ERROR guard script missing: $CONSISTENCY" >&2
  exit 2
fi

_tmp_base="${TMPDIR:-/tmp}"
TMP_DIR="$(mktemp -d "${_tmp_base%/}/bubbles-reqspec-consistency-selftest.XXXXXX")"
cleanup() {
  if [[ "$failures" -eq 0 && "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "Preserving selftest workspace: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

# LAST_OUT / LAST_RC are set by run_case for the caller to assert on.
LAST_OUT=""
LAST_RC=0
run_case() {
  # $1 = repo-root passed to the consistency guard
  LAST_OUT="$(bash "$CONSISTENCY" --repo-root "$1" 2>&1)" && LAST_RC=0 || LAST_RC=$?
}

expect_pass() {
  local label="$1" root="$2"
  run_case "$root"
  if [[ "$LAST_RC" -eq 0 ]]; then
    pass "$label (exit 0)"
  else
    fail "$label — expected exit 0, got $LAST_RC"
    printf '  --- output ---\n%s\n  --- end ---\n' "$LAST_OUT" >&2
  fi
}

expect_drift() {
  local label="$1" root="$2" needle="$3"
  run_case "$root"
  if [[ "$LAST_RC" -eq 1 ]] && printf '%s\n' "$LAST_OUT" | grep -q "$needle"; then
    pass "$label (exit 1, divergence matched: '$needle')"
  else
    fail "$label — expected exit 1 with divergence '$needle', got exit $LAST_RC"
    printf '  --- output ---\n%s\n  --- end ---\n' "$LAST_OUT" >&2
  fi
}

# ---------------------------------------------------------------------------
# (a) LIVE shadow-compare: the real registry matches the real guard case.
# ---------------------------------------------------------------------------
expect_pass "live: real required-specialists.yaml matches real guard Check 6 case" "$REPO_ROOT"

# ---------------------------------------------------------------------------
# Build a minimal fixture repo-root. The guard stub carries the exact structural
# anchors the parser keys on (the case header, two arms, and a SCOPE-3 fallback
# region with a DECOY arm that must NOT be parsed). alpha-mode also interleaves a
# comment between its label and its assignment to mirror the real
# rapid-tool-delivery shape and prove the parser tolerates it.
# ---------------------------------------------------------------------------
mkfixture_guard() {
  local dir="$1"
  mkdir -p "$dir/bubbles/scripts"
  cat >"$dir/bubbles/scripts/state-transition-guard.sh" <<'GUARD'
#!/usr/bin/env bash
# Minimal fixture guard for the required-specialists-consistency selftest.
if [[ -n "$state_workflow_mode" ]]; then
  required_specialists=()
  case "$state_workflow_mode" in
    alpha-mode)
      # interleaved comment between label and assignment (rapid-tool-delivery shape)
      required_specialists=("implement" "test" "validate" "docs")
      ;;
    beta-mode)
      required_specialists=("validate" "audit")
      ;;
  esac

  # IMP-105-SCOPE-3-FALLBACK-BEGIN
  # This region MUST NEVER be parsed as a case arm. decoy-mode is absent from the
  # fixture registry, so if the parser wrongly read past this marker the positive
  # fixture would FAIL with missing-in-registry: decoy-mode.
    decoy-mode)
      required_specialists=("should" "not" "be" "parsed")
      ;;
  # IMP-105-SCOPE-3-FALLBACK-END
fi
GUARD
}

mkfixture_registry() {
  local dir="$1"
  shift
  mkdir -p "$dir/bubbles/registry"
  {
    echo "version: 1"
    echo "modes:"
    local line
    for line in "$@"; do
      echo "  $line"
    done
  } >"$dir/bubbles/registry/required-specialists.yaml"
}

# ---------------------------------------------------------------------------
# (b) FIXTURE POSITIVE — consistent guard + registry; decoy arm ignored.
# ---------------------------------------------------------------------------
FX_OK="$TMP_DIR/ok"
mkfixture_guard "$FX_OK"
mkfixture_registry "$FX_OK" \
  "alpha-mode: [implement, test, validate, docs]" \
  "beta-mode: [validate, audit]"
expect_pass "fixture-positive: consistent guard+registry (and decoy fallback arm NOT parsed)" "$FX_OK"

# Extra proof the positive control is non-trivial: if the decoy arm HAD been
# parsed, the run would have failed with 'decoy-mode'. Assert it did not appear.
run_case "$FX_OK"
if printf '%s\n' "$LAST_OUT" | grep -q "decoy-mode"; then
  fail "fixture-positive: decoy-mode leaked from the SCOPE-3 fallback region into the parse"
else
  pass "fixture-positive: decoy-mode from the fallback region correctly excluded"
fi

# ---------------------------------------------------------------------------
# (c) MUTATED LIST — alpha-mode list changed (drop 'docs') → list-mismatch.
# ---------------------------------------------------------------------------
FX_MUT="$TMP_DIR/mutated"
mkfixture_guard "$FX_MUT"
mkfixture_registry "$FX_MUT" \
  "alpha-mode: [implement, test, validate]" \
  "beta-mode: [validate, audit]"
expect_drift "fixture-mutated-list: alpha-mode list differs" "$FX_MUT" "list-mismatch: mode alpha-mode"

# Order-sensitivity: same set, different order → still a mismatch.
FX_ORDER="$TMP_DIR/reordered"
mkfixture_guard "$FX_ORDER"
mkfixture_registry "$FX_ORDER" \
  "alpha-mode: [test, implement, validate, docs]" \
  "beta-mode: [validate, audit]"
expect_drift "fixture-reordered-list: alpha-mode same set wrong order" "$FX_ORDER" "list-mismatch: mode alpha-mode"

# ---------------------------------------------------------------------------
# (d) REMOVED MODE — beta-mode present in the case but dropped from registry.
# ---------------------------------------------------------------------------
FX_MISS="$TMP_DIR/removed"
mkfixture_guard "$FX_MISS"
mkfixture_registry "$FX_MISS" \
  "alpha-mode: [implement, test, validate, docs]"
expect_drift "fixture-removed-mode: beta-mode missing from registry" "$FX_MISS" "missing-in-registry: mode beta-mode"

# ---------------------------------------------------------------------------
# (e) EXTRA MODE — gamma-mode in registry but absent from the case.
# ---------------------------------------------------------------------------
FX_EXTRA="$TMP_DIR/extra"
mkfixture_guard "$FX_EXTRA"
mkfixture_registry "$FX_EXTRA" \
  "alpha-mode: [implement, test, validate, docs]" \
  "beta-mode: [validate, audit]" \
  "gamma-mode: [validate]"
expect_drift "fixture-extra-mode: gamma-mode present only in registry" "$FX_EXTRA" "extra-in-registry: mode gamma-mode"

echo "---"
if [[ "$failures" -ne 0 ]]; then
  echo "$failures case(s) failed." >&2
  exit 1
fi
echo "all cases passed."
