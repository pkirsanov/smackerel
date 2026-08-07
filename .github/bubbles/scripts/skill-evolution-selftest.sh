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
  local similarity="${2:-}"
  mkdir -p "$tree/bubbles/scripts" "$tree/.specify/memory"
  cp "$REAL_SCRIPT" "$tree/bubbles/scripts/skill-evolution.sh"
  {
    echo "skillEvolution:"
    echo "  enabled: true"
    echo "  triggerThreshold: 3"
    if [[ -n "$similarity" ]]; then
      echo "  similarityThreshold: ${similarity}"
    fi
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

# ── Paraphrase tree: one root cause, three wordings, zero identical lines ──
# This is the case that fails under whole-line equality (IMP-034 LRN-2) and is
# the regression guard that keeps the loop from silently re-breaking.
PARA="$TMP_ROOT/para"
build_tree "$PARA"
{
  echo "# Lessons"
  echo
  echo "- health check failed because the service was still starting"
  echo "- health check failed since the service had not finished starting"
  echo "- health check failed while the service was starting up"
} > "$PARA/.specify/memory/lessons.md"

PARA_PROPOSALS="$PARA/.specify/memory/skill-proposals.md"

run_script "$PARA" show

# (e) paraphrased lessons describing one root cause cluster into one proposal.
if [[ -f "$PARA_PROPOSALS" ]] \
  && [[ "$(grep -c '^## Skill Proposal:' "$PARA_PROPOSALS")" -eq 1 ]] \
  && grep -q '^- Observed: 3 times' "$PARA_PROPOSALS"; then
  pass "paraphrased lessons (no identical lines) cluster into one proposal"
else
  fail "expected exactly one proposal observed 3 times for the paraphrase set"
fi

# (f) the proposal shows what was grouped so a reviewer can reject a bad merge.
if grep -q '^- Grouped variants:' "$PARA_PROPOSALS" \
  && grep -q 'had not finished starting' "$PARA_PROPOSALS" \
  && grep -q 'was starting up' "$PARA_PROPOSALS"; then
  pass "proposal lists the grouped variants alongside the representative"
else
  fail "proposal did not list the grouped variants"
fi

# ── Near-miss tree: three DIFFERENT root causes sharing vocabulary ──
# Without this, SCOPE-1 could pass by over-merging everything (IMP-034 R1).
MISS="$TMP_ROOT/miss"
build_tree "$MISS"
{
  echo "# Lessons"
  echo
  echo "- health check returned 500 because the database connection pool was exhausted"
  echo "- health check used the wrong port in the compose file so nothing responded"
  echo "- health check ran before migrations finished and reported a false failure"
} > "$MISS/.specify/memory/lessons.md"

MISS_PROPOSALS="$MISS/.specify/memory/skill-proposals.md"

run_script "$MISS" show

# (g) shared vocabulary alone MUST NOT cluster distinct root causes.
if [[ ! -f "$MISS_PROPOSALS" ]]; then
  pass "distinct root causes sharing vocabulary do not over-merge"
else
  fail "adversarial: distinct root causes were merged into a proposal"
fi

# ── Metadata compatibility tree: legacy + anchored entries are equivalent ──
META="$TMP_ROOT/meta"
build_tree "$META"
{
  echo "# Lessons"
  echo
  echo '- problem: config drift; root cause: generated state was stale; fix: regenerate state; applies when: config changes'
  echo '- problem: config drift; root cause: generated state was stale; fix: regenerate state; applies when: config changes <!-- bubbles-lesson-meta:{"lessonId":"lesson-one","metadataPoison":"copper-signal"} -->'
  echo '- problem: config drift; root cause: generated state was stale; fix: regenerate state; applies when: config changes <!-- bubbles-lesson-meta:{"lessonId":"lesson-two","metadataPoison":"violet-signal"} -->'
} > "$META/.specify/memory/lessons.md"

META_PROPOSALS="$META/.specify/memory/skill-proposals.md"

run_script "$META" show

if [[ -f "$META_PROPOSALS" ]] \
  && grep -qF -- '- Pattern: problem: config drift; root cause: generated state was stale; fix: regenerate state; applies when: config changes' "$META_PROPOSALS" \
  && grep -q '^- Observed: 3 times' "$META_PROPOSALS"; then
  pass "legacy and anchored equivalent lessons cluster as one visible pattern"
else
  fail "lesson metadata changed legacy clustering behavior"
fi

if [[ -f "$META_PROPOSALS" ]] \
  && ! grep -Eq 'bubbles-lesson-meta|lesson-one|lesson-two|copper-signal|violet-signal' "$META_PROPOSALS"; then
  pass "lesson metadata tokens do not influence proposal grouping"
else
  fail "lesson metadata leaked into skill-evolution proposal content"
fi

# ── Strict tree: same paraphrases, similarityThreshold raised to 1.0 ──
# Proves two things at once: similarityThreshold is really read from the
# registry (not ignored), and at exact-match strictness the paraphrase set
# goes back to producing nothing — which is the pre-IMP-034 behavior this
# change exists to fix.
STRICT="$TMP_ROOT/strict"
build_tree "$STRICT" "1.0"
{
  echo "# Lessons"
  echo
  echo "- health check failed because the service was still starting"
  echo "- health check failed since the service had not finished starting"
  echo "- health check failed while the service was starting up"
} > "$STRICT/.specify/memory/lessons.md"

STRICT_PROPOSALS="$STRICT/.specify/memory/skill-proposals.md"

run_script "$STRICT" show

# (h) the similarity knob is live, and exact strictness reproduces the old miss.
if [[ ! -f "$STRICT_PROPOSALS" ]]; then
  pass "similarityThreshold is honored (1.0 stops the paraphrase cluster)"
else
  fail "similarityThreshold appears ignored: paraphrases clustered at 1.0"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "skill-evolution-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "skill-evolution-selftest: PASS"
exit 0
