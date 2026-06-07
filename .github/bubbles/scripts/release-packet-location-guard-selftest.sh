#!/usr/bin/env bash
set -uo pipefail

# release-packet-location-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/release-packet-location-guard.sh`
# (enforces the canonical release-packet location owned by bubbles.releases).
#
# The guard flags any file named one of the 8 canonical release-packet docs
# (vision/features/actions/business-plan/deployment/marketing/monetization/
# ops-scalability .md) whose path contains "release" (case-insensitive) but is
# NOT under docs/releases/<phase>/<basename>. Generic same-named files that are
# not on a release-shaped path are intentionally NOT flagged.
#
# Scenarios:
#   S0   Non-existent repo root                     → exit 2
#   S1   Canonical docs/releases/<phase>/...        → exit 0
#   S2   Misplaced specs/releases/<phase>/vision.md  → exit 1  (ADVERSARIAL:
#                                                       release-shaped non-
#                                                       canonical path BLOCKs)
#   S3   Misplaced docs/RELEASE-1/features.md        → exit 1  (ADVERSARIAL:
#                                                       upper-case RELEASE dir)
#   S4   Generic docs/guides/features.md (no         → exit 0  (false-positive
#        "release" in path)                             avoidance)
#   S5   Empty repo (no release-packet docs)         → exit 0
#   S6   Full canonical packet (all 8 docs)          → exit 0
#
# Reference:
#   agents/bubbles.releases.agent.md → canonical release-packet location

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/release-packet-location-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-rploc-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# Stage a fresh repo root under the workspace and emit its absolute path.
new_repo() {
  local name="$1"
  local d="$WORKSPACE/$name"
  mkdir -p "$d"
  printf '%s' "$d"
}

run_guard() {
  bash "$GUARD_SCRIPT" "$@" >/dev/null 2>&1
  RC=$?
}

# -----------------------------------------------------------------------
# S0: non-existent repo root → exit 2
# -----------------------------------------------------------------------
run_guard "$WORKSPACE/does-not-exist-$$"
if [[ "$RC" -eq 2 ]]; then
  pass "S0 non-existent repo root exits 2"
else
  bad "S0 non-existent repo root expected exit 2, got $RC"
fi

# -----------------------------------------------------------------------
# S1: canonical docs/releases/<phase>/... → exit 0
# -----------------------------------------------------------------------
s1="$(new_repo s1-canonical)"
mkdir -p "$s1/docs/releases/phase-1"
echo "# vision" >"$s1/docs/releases/phase-1/vision.md"
run_guard "$s1"
if [[ "$RC" -eq 0 ]]; then
  pass "S1 canonical release-packet location passes (exit 0)"
else
  bad "S1 canonical location expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S2: misplaced specs/releases/<phase>/vision.md → exit 1 (ADVERSARIAL)
# -----------------------------------------------------------------------
s2="$(new_repo s2-misplaced-specs)"
mkdir -p "$s2/specs/releases/phase-1"
echo "# vision" >"$s2/specs/releases/phase-1/vision.md"
run_guard "$s2"
if [[ "$RC" -eq 1 ]]; then
  pass "S2 misplaced specs/releases/.../vision.md BLOCKs (exit 1)"
else
  bad "S2 misplaced specs path expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S3: misplaced docs/RELEASE-1/features.md → exit 1 (ADVERSARIAL, upper-case)
# -----------------------------------------------------------------------
s3="$(new_repo s3-misplaced-upper)"
mkdir -p "$s3/docs/RELEASE-1"
echo "# features" >"$s3/docs/RELEASE-1/features.md"
run_guard "$s3"
if [[ "$RC" -eq 1 ]]; then
  pass "S3 misplaced docs/RELEASE-1/features.md BLOCKs (exit 1)"
else
  bad "S3 misplaced upper-case RELEASE path expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S4: generic docs/guides/features.md (no "release" in path) → exit 0
#     (false-positive avoidance)
# -----------------------------------------------------------------------
s4="$(new_repo s4-generic-noise)"
mkdir -p "$s4/docs/guides"
echo "# features" >"$s4/docs/guides/features.md"
run_guard "$s4"
if [[ "$RC" -eq 0 ]]; then
  pass "S4 generic features.md off a release path is NOT flagged (exit 0)"
else
  bad "S4 generic non-release features.md expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S5: empty repo (no release-packet docs) → exit 0
# -----------------------------------------------------------------------
s5="$(new_repo s5-empty)"
mkdir -p "$s5/docs"
run_guard "$s5"
if [[ "$RC" -eq 0 ]]; then
  pass "S5 empty repo passes (exit 0)"
else
  bad "S5 empty repo expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S6: full canonical packet (all 8 docs in canonical location) → exit 0
# -----------------------------------------------------------------------
s6="$(new_repo s6-full-packet)"
mkdir -p "$s6/docs/releases/v2"
for doc in vision features actions business-plan deployment marketing monetization ops-scalability; do
  echo "# $doc" >"$s6/docs/releases/v2/${doc}.md"
done
run_guard "$s6"
if [[ "$RC" -eq 0 ]]; then
  pass "S6 full canonical 8-doc packet passes (exit 0)"
else
  bad "S6 full canonical packet expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------
echo
echo "============================================================"
echo "  release-packet-location-guard selftest verdict"
echo "    passed assertions: $PASS_COUNT"
echo "    failed assertions: $FAIL_COUNT"
echo "============================================================"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  FAILED: %s\n' "${FAILED_SCENARIOS[@]}" >&2
  echo "release-packet-location-guard-selftest: FAILED" >&2
  exit 1
fi
echo "release-packet-location-guard-selftest: PASSED"
exit 0
