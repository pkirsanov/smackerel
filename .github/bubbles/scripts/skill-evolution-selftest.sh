#!/usr/bin/env bash
# Bubbles skill-evolution selftest (v7.16.0 / IMP-016).
#
# Hermetic: copies bubbles/scripts/skill-evolution.sh into a throwaway temp
# tree (with its own bubbles/workflows.yaml + .specify/memory/lessons.md) so
# the script's REPO_ROOT="$SCRIPT_DIR/../.." resolves INSIDE the temp tree and
# every write lands there, never in the real repo.
#
# Asserts:
#   (a) a lesson pattern repeated >= triggerThreshold produces a
#       "## Skill Proposal:" block in .specify/memory/skill-proposals.md
#   (b) the proposal output carries the IMP-016 quality-bar scaffolding
#       (decision rule + Reusable/Verified + INVENTORY.md dedup line)
#   (c) `dismiss` removes the proposals file and appends to the dismissed log
#   (d) ADVERSARIAL: lessons all BELOW threshold produce NO proposal (this
#       fails loudly if the triggerThreshold gate is ever removed)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REAL_SCRIPT="$SCRIPT_DIR/skill-evolution.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

if [[ ! -f "$REAL_SCRIPT" ]]; then
  echo "skill-evolution-selftest: missing $REAL_SCRIPT" >&2
  exit 2
fi

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

# Build a hermetic mini-repo tree: copied script + minimal workflows.yaml
# (skillEvolution.triggerThreshold: 3). The caller writes lessons.md.
build_tree() {
  local tree="$1"
  mkdir -p "$tree/bubbles/scripts" "$tree/.specify/memory"
  cp "$REAL_SCRIPT" "$tree/bubbles/scripts/skill-evolution.sh"
  {
    echo "skillEvolution:"
    echo "  enabled: true"
    echo "  triggerThreshold: 3"
  } > "$tree/bubbles/workflows.yaml"
}

# Invoke the COPIED script inside its temp tree; capture combined output in
# RUN_OUT regardless of exit status.
run_script() {
  local tree="$1"
  shift
  set +e
  RUN_OUT="$(bash "$tree/bubbles/scripts/skill-evolution.sh" "$@" 2>&1)"
  set -e
}

# ── Positive tree: the same pattern repeated 3x (== threshold) ──────
POS="$TMP_ROOT/pos"
build_tree "$POS"
{
  echo "# Lessons"
  echo
  echo "- always reproduce the failing scenario before writing the fix"
  echo "- always reproduce the failing scenario before writing the fix"
  echo "- always reproduce the failing scenario before writing the fix"
} > "$POS/.specify/memory/lessons.md"

POS_PROPOSALS="$POS/.specify/memory/skill-proposals.md"
POS_DISMISSED="$POS/.specify/memory/skill-proposals-dismissed.md"

run_script "$POS" show

# (a) repeated pattern over threshold -> a proposal block is generated.
if [[ -f "$POS_PROPOSALS" ]] && grep -q "## Skill Proposal:" "$POS_PROPOSALS"; then
  pass "repeated lesson at threshold generates a skill proposal"
else
  fail "expected a '## Skill Proposal:' block for the repeated pattern"
fi

# (b) proposal output carries the IMP-016 quality-bar scaffolding.
if grep -qF "recurring + non-obvious + verified" "$POS_PROPOSALS" \
  && grep -qF "Reusable" "$POS_PROPOSALS" \
  && grep -qF "Verified" "$POS_PROPOSALS" \
  && grep -qF "INVENTORY.md" "$POS_PROPOSALS"; then
  pass "proposal output carries decision-rule + quality-bar + dedup scaffolding"
else
  fail "proposal output missing IMP-016 quality-bar/decision-rule/dedup scaffolding"
fi

# (c) dismiss removes the proposals file and appends to the dismissed log.
run_script "$POS" dismiss
if [[ ! -f "$POS_PROPOSALS" ]] && [[ -f "$POS_DISMISSED" ]] \
  && grep -q "## Skill Proposal:" "$POS_DISMISSED"; then
  pass "dismiss removes proposals and appends to the dismissed log"
else
  fail "dismiss did not remove proposals and/or append to the dismissed log"
fi

# ── Adversarial tree: every pattern BELOW threshold (<= 2) ──────────
NEG="$TMP_ROOT/neg"
build_tree "$NEG"
{
  echo "# Lessons"
  echo
  echo "- only observed twice so it must not cross the trigger threshold"
  echo "- only observed twice so it must not cross the trigger threshold"
  echo "- a distinct one-off lesson seen a single time and never again"
} > "$NEG/.specify/memory/lessons.md"

NEG_PROPOSALS="$NEG/.specify/memory/skill-proposals.md"

run_script "$NEG" show

# (d) below-threshold lessons must produce NO proposal (threshold gate holds).
if [[ ! -f "$NEG_PROPOSALS" ]] && printf '%s\n' "$RUN_OUT" | grep -q "No skill proposals"; then
  pass "below-threshold lessons produce no proposal (threshold gate intact)"
else
  fail "adversarial: below-threshold lessons unexpectedly produced a proposal"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "skill-evolution-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "skill-evolution-selftest: PASS"
exit 0
