#!/usr/bin/env bash
# bubbles/scripts/learning-loop-selftest.sh
#
# Capability: learning-loop-integration
#
# End-to-end selftest for the learning loop (IMP-043 SCOPE-6 / COV-18).
#
# WHY THIS EXISTS
# Every component of the learning loop already passes its own selftest: the
# lesson writer, the clustering engine, and the recall index. Yet `lessons.md`
# is empty in every repository that has one. Component tests cannot catch that,
# because the defect is not in any component -- it is in the seams between them.
#
# So this test drives the CHAIN in one run: write lessons through the real CLI,
# cross the clustering threshold, and require a proposal that names the pattern.
# Cases 4 and 5 are the load-bearing ones: they prove the test cannot pass by
# generating proposals unconditionally, and that compaction is bounded by the
# declared limit rather than a hardcoded one.
#
# Usage: bash bubbles/scripts/learning-loop-selftest.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="learning-loop-selftest"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

command -v jq >/dev/null 2>&1 || {
  printf '%s: SKIP (jq not installed)\n' "$NAME"
  exit 0
}
[[ -f "$SCRIPT_DIR/cli.sh" ]] || {
  printf '%s: SKIP (cli.sh not found)\n' "$NAME"
  exit 0
}
[[ -f "$SCRIPT_DIR/skill-evolution.sh" ]] || {
  printf '%s: SKIP (skill-evolution.sh not found)\n' "$NAME"
  exit 0
}

# Canonicalized: several framework guards refuse a path with a symlink component,
# and macOS /var is a symlink to /private/var.
WORK="$(cd "$(mktemp -d)" && pwd -P)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# A throwaway repository shaped like a real install: the CLI resolves its root
# from its own location, so the framework scripts must live inside the fixture.
REPO="$WORK/repo"
mkdir -p "$REPO/bubbles/scripts" "$REPO/.specify/memory"
cp "$SCRIPT_DIR"/*.sh "$REPO/bubbles/scripts/" 2>/dev/null || true
if [[ -f "$REPO_ROOT/bubbles/workflows.yaml" ]]; then
  cp "$REPO_ROOT/bubbles/workflows.yaml" "$REPO/bubbles/workflows.yaml"
fi

LESSONS="$REPO/.specify/memory/lessons.md"
add_lesson() {
  (cd "$REPO" && bash bubbles/scripts/cli.sh lessons add \
    --problem "$1" --root-cause "$2" --fix "$3" --applies-when "$4" \
    >/dev/null 2>&1)
}

# --- 1. a lesson written through the real CLI lands in the register ---------
add_lesson "the guard timed out" "a bare timeout call" \
  "use the portable timeout helper" "writing a selftest"
if [[ -f "$LESSONS" ]] && grep -q 'bubbles-lesson-meta:' "$LESSONS"; then
  ok "a lesson written through cli.sh lands with its metadata marker"
else
  bad "lesson written through cli.sh" "lessons.md missing or unmarked"
fi

# --- 2. the lesson carries a resolvable id ----------------------------------
# SCOPE-1 requires an envelope claiming `captured` to name a lessonId. If the
# writer produced no id, that contract would be unsatisfiable.
if grep -o '"lessonId":"[^"]*"' "$LESSONS" 2>/dev/null | grep -q 'lesson-' ||
  grep -o 'lesson-[0-9a-f]\{8,\}' "$LESSONS" >/dev/null 2>&1; then
  ok "the written lesson carries a resolvable lesson id"
else
  bad "lesson id present" "$(head -c 200 "$LESSONS" 2>/dev/null)"
fi

# --- 3. crossing the cluster threshold produces a proposal naming the pattern -
add_lesson "the lint timed out" "another bare timeout call" \
  "use the portable timeout helper" "writing a guard"
add_lesson "the check timed out" "a third bare timeout call" \
  "use the portable timeout helper" "writing a lint"
proposal_out="$( (cd "$REPO" && bash bubbles/scripts/skill-evolution.sh show 2>&1) )"
if printf '%s' "$proposal_out" | grep -qi 'timeout'; then
  ok "three clustered lessons produce a proposal naming the pattern"
else
  bad "proposal names the pattern" "$(printf '%s' "$proposal_out" | tr '\n' '|' | head -c 300)"
fi

# --- 4. ADVERSARIAL: one lesson must NOT produce a proposal -----------------
# Without this, a generator that emits proposals unconditionally would pass
# case 3 and the whole test would prove nothing.
REPO2="$WORK/repo2"
mkdir -p "$REPO2/bubbles/scripts" "$REPO2/.specify/memory"
cp "$SCRIPT_DIR"/*.sh "$REPO2/bubbles/scripts/" 2>/dev/null || true
[[ -f "$REPO_ROOT/bubbles/workflows.yaml" ]] &&
  cp "$REPO_ROOT/bubbles/workflows.yaml" "$REPO2/bubbles/workflows.yaml"
(cd "$REPO2" && bash bubbles/scripts/cli.sh lessons add \
  --problem "a solitary problem" --root-cause "a solitary cause" \
  --fix "a solitary fix" --applies-when "never again" >/dev/null 2>&1)
single_out="$( (cd "$REPO2" && bash bubbles/scripts/skill-evolution.sh show 2>&1) )"
if printf '%s' "$single_out" | grep -qi 'solitary'; then
  bad "a single lesson produces no proposal" "one lesson crossed the threshold"
else
  ok "a single lesson produces no proposal (threshold is real)"
fi

# --- 5. compaction is bounded by the DECLARED limit, not a hardcoded one -----
# IMP-043 SCOPE-3 made cli.sh read lessonsMemory.maxLines. Proving that with a
# small limit is what stops the 150 from creeping back in as a literal.
REPO3="$WORK/repo3"
mkdir -p "$REPO3/bubbles/scripts" "$REPO3/.specify/memory"
cp "$SCRIPT_DIR"/*.sh "$REPO3/bubbles/scripts/" 2>/dev/null || true
printf 'lessonsMemory:\n  file: .specify/memory/lessons.md\n  maxLines: 5\n' \
  >"$REPO3/bubbles/workflows.yaml"
for i in 1 2 3 4 5 6 7 8; do
  (cd "$REPO3" && bash bubbles/scripts/cli.sh lessons add \
    --problem "problem $i" --root-cause "cause $i" --fix "fix $i" \
    --applies-when "case $i" >/dev/null 2>&1)
done
kept="$(wc -l <"$REPO3/.specify/memory/lessons.md" 2>/dev/null | tr -d ' ')"
if [[ -n "$kept" ]] && [[ "$kept" -le 5 ]]; then
  ok "compaction honours the declared maxLines (kept $kept, limit 5)"
else
  bad "compaction honours declared maxLines" "kept ${kept:-none} with maxLines 5"
fi

# --- 6. compaction ARCHIVES rather than discards -----------------------------
# Bounding the file must not destroy the history the clustering engine needs.
if [[ -f "$REPO3/.specify/memory/lessons-archive.md" ]] &&
  grep -q 'problem 1' "$REPO3/.specify/memory/lessons-archive.md" 2>/dev/null; then
  ok "compacted lessons are archived, not discarded"
else
  bad "compacted lessons archived" "lessons-archive.md missing or incomplete"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
