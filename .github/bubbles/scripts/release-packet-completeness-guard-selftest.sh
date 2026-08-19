#!/usr/bin/env bash
# release-packet-completeness-guard-selftest.sh — hermetic coverage for Gate G138.
#
# The location guard's selftest covers only placement; every one of its scenarios
# is about where a doc sits. IMP-050 SCOPE-3 requires the adversarial case that
# suite structurally lacks: a packet holding a strict SUBSET of the canonical
# eight must FAIL. Without it the completeness check would be untested in the
# only dimension it exists for.
#
# Scenarios:
#   S1  No docs/releases/ directory                        → exit 0
#   S2  Empty docs/releases/                               → exit 0
#   S3  Complete packet (all 8 docs)                       → exit 0
#   S4  Strict subset, observed downstream shape: 5 of 8
#       with marketing/monetization/ops-scalability absent → exit 1
#   S5  S4 names EACH absent doc (not merely a count)      → report assertion
#   S6  Sibling phases, one complete + one incomplete      → exit 1, only the
#                                                            incomplete named
#   S7  Boundary: exactly one doc absent (7 of 8)          → exit 1
#   S8  Non-packet directory holding zero canonical docs   → exit 0
#   S9  No skip/force/ignore flag is honoured              → exit 1 still

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/release-packet-completeness-guard.sh"

if [[ ! -f "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not found: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-rpcomp-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

ALL_DOCS="vision.md features.md actions.md business-plan.md deployment.md marketing.md monetization.md ops-scalability.md"

new_repo() {
  local d="$WORKSPACE/$1"
  mkdir -p "$d"
  printf '%s' "$d"
}

# stage_phase <repo> <phase> <doc...>
stage_phase() {
  local repo="$1" phase="$2"; shift 2
  mkdir -p "$repo/docs/releases/$phase"
  local doc
  for doc in "$@"; do
    printf '# %s\n' "$doc" > "$repo/docs/releases/$phase/$doc"
  done
}

run_guard() {
  bash "$GUARD_SCRIPT" "$1" >"$WORKSPACE/out.txt" 2>"$WORKSPACE/err.txt"
  echo $?
}

echo "release-packet-completeness-guard selftest"

# ---- S1: no docs/releases/ at all -------------------------------------------
r="$(new_repo s1)"
mkdir -p "$r/docs"
rc="$(run_guard "$r")"
if [[ "$rc" -eq 0 ]]; then pass "S1 no docs/releases/ → exit 0"; else bad "S1 expected 0, got $rc"; fi

# ---- S2: empty docs/releases/ -----------------------------------------------
r="$(new_repo s2)"
mkdir -p "$r/docs/releases"
rc="$(run_guard "$r")"
if [[ "$rc" -eq 0 ]]; then pass "S2 empty docs/releases/ → exit 0"; else bad "S2 expected 0, got $rc"; fi

# ---- S3: complete packet ----------------------------------------------------
r="$(new_repo s3)"
# shellcheck disable=SC2086
stage_phase "$r" phase-1 $ALL_DOCS
rc="$(run_guard "$r")"
if [[ "$rc" -eq 0 ]]; then pass "S3 complete packet → exit 0"; else bad "S3 expected 0, got $rc"; fi

# ---- S4/S5: observed downstream shape — 5 of 8 ------------------------------
r="$(new_repo s4)"
stage_phase "$r" phase-1 vision.md features.md actions.md business-plan.md deployment.md
rc="$(run_guard "$r")"
if [[ "$rc" -eq 1 ]]; then pass "S4 strict subset (5/8) → exit 1"; else bad "S4 expected 1, got $rc"; fi

named=0
for doc in marketing.md monetization.md ops-scalability.md; do
  grep -q -- "$doc" "$WORKSPACE/err.txt" || named=1
done
if [[ "$named" -eq 0 ]]; then
  pass "S5 report names each absent doc individually"
else
  bad "S5 report did not name every absent doc"
fi
# A bare count is explicitly insufficient (IMP-050 SCOPE-2 requirement 1).
if grep -q '5/8' "$WORKSPACE/err.txt"; then
  pass "S5 report also states the present/expected ratio"
else
  bad "S5 report omitted the present/expected ratio"
fi

# ---- S6: sibling phases, one complete one not -------------------------------
r="$(new_repo s6)"
# shellcheck disable=SC2086
stage_phase "$r" phase-1 $ALL_DOCS
stage_phase "$r" phase-2 vision.md features.md
rc="$(run_guard "$r")"
if [[ "$rc" -eq 1 ]]; then pass "S6 sibling phases → exit 1"; else bad "S6 expected 1, got $rc"; fi
if grep -q 'phase-2' "$WORKSPACE/err.txt" && ! grep -q 'docs/releases/phase-1/' "$WORKSPACE/err.txt"; then
  pass "S6 names only the incomplete phase"
else
  bad "S6 misreported which phase is incomplete"
fi

# ---- S7: boundary — exactly one doc absent ----------------------------------
r="$(new_repo s7)"
stage_phase "$r" phase-1 vision.md features.md actions.md business-plan.md deployment.md marketing.md monetization.md
rc="$(run_guard "$r")"
if [[ "$rc" -eq 1 ]]; then pass "S7 one doc absent (7/8) → exit 1"; else bad "S7 expected 1, got $rc"; fi
if grep -q 'ops-scalability.md' "$WORKSPACE/err.txt"; then
  pass "S7 names the single absent doc"
else
  bad "S7 did not name the single absent doc"
fi

# ---- S8: non-packet directory ------------------------------------------------
r="$(new_repo s8)"
# shellcheck disable=SC2086
stage_phase "$r" phase-1 $ALL_DOCS
mkdir -p "$r/docs/releases/assets"
printf 'x\n' > "$r/docs/releases/assets/diagram.md"
rc="$(run_guard "$r")"
if [[ "$rc" -eq 0 ]]; then pass "S8 directory with zero canonical docs is not a packet"; else bad "S8 expected 0, got $rc"; fi

# ---- S9: no skip flag --------------------------------------------------------
r="$(new_repo s9)"
stage_phase "$r" phase-1 vision.md
BUBBLES_SKIP_RELEASE_PACKET_COMPLETENESS=1 bash "$GUARD_SCRIPT" "$r" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 1 ]]; then pass "S9 no env escape hatch is honoured"; else bad "S9 expected 1, got $rc"; fi

echo
echo "PASSED=$PASS_COUNT FAILED=$FAIL_COUNT"
[[ "$FAIL_COUNT" -eq 0 ]] || exit 1
exit 0
