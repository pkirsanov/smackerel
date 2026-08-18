#!/usr/bin/env bash
# session-review-selftest.sh — IMP-048 SCOPE-1 (LRN-8).
#
# Every assertion runs the SHIPPING script against a real fixture repository
# and reads the outcome back out of the JSONL store and the lessons file the
# script actually wrote. A churn rule that is only ever asked what it would do
# has never been shown to do it.
#
# The three adversarial cases the proposal names by hand, first:
#   changed nothing while a repeat failure was present  -- REFUSED, because
#     "I looked and everything was fine" is exactly the record a stuck session
#     writes, and admitting it turns the loop back into a scoreboard
#   Class B promoted BELOW the recurrence threshold    -- REFUSED
#   Class C re-emitted every turn                      -- DEDUPLICATED unless
#     the underlying metric worsened by >= 25%
#
# Also covered:
#   default off        an unconfigured repository is a clean no-op: zero
#                      records, no store, no .specify directory, exit 0
#   trigger defaults   each of the six triggers fires at its documented default
#                      and does not fire one unit below it
#   first to fire      with every dimension over its threshold, the most
#                      diagnostic trigger is the one recorded
#   budget bands       50 / 70 / 90 fire once each, never twice
#   promotion cap      a 4th eligible lesson is DROPPED with a recorded count
#                      rather than persisted
#   permanent dismissal a pattern in skill-proposals-dismissed.md is never a
#                      candidate again, by either path
#   write boundary     a store or lessons path that RESOLVES under bubbles/ or
#                      agents/ is refused before anything is written
#   no-adjustment      an empty review with nothing diagnostic observed is a
#                      valid, recorded outcome
#   review budget      the review is itself budgeted; only --close is exempt
#   one lessons loop   a promoted lesson is clustered by the EXISTING
#                      skill-evolution.sh with no change to that script
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the script under test is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SUT="$SCRIPT_DIR/session-review.sh"
SKILL_EVOLUTION="$SCRIPT_DIR/skill-evolution.sh"
ENVELOPE_SCHEMA="$REPO_ROOT/bubbles/schemas/result-envelope.schema.json"
NAME="session-review-selftest"
STORE_REL=".specify/runtime/session-review.jsonl"
LESSONS_REL=".specify/memory/lessons.md"
DISMISSED_REL=".specify/memory/skill-proposals-dismissed.md"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

[[ -f "$SUT" ]] || {
  printf '%s: script under test not found: %s\n' "$NAME" "$SUT" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-session-review.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# --- fixture helpers -------------------------------------------------------

repo_seq=0
REPO=""
# Sets the global REPO. NOT called through a command substitution: that would
# run in a subshell, repo_seq would never advance, and every "fresh" fixture
# would silently be the same directory carrying the previous test's records.
new_repo() {
  local adapter="${1:-jsonl}"
  repo_seq=$((repo_seq + 1))
  REPO="$TMP_DIR/repo$repo_seq"
  mkdir -p "$REPO/.github"
  if [[ "$adapter" != "unset" ]]; then
    printf 'sessionReview:\n  adapter: %s\n' "$adapter" > "$REPO/.github/bubbles-project.yaml"
  fi
}

LAST_OUT=""
LAST_RC=0
run_sut() {
  local sub="$1" repo="$2"
  shift 2
  LAST_OUT="$(bash "$SUT" "$sub" --repo-root "$repo" "$@" 2>&1)"
  LAST_RC=$?
}

# Read one key=value line out of the captured output. A herestring, never a
# discarding pipe: `... | grep -q` on unbounded input under pipefail is the
# BUG-009 SIGPIPE race.
kv() {
  awk -v k="$1" -F= '$1 == k { print substr($0, length(k) + 2); exit }' <<< "$LAST_OUT"
}

store_path() { printf '%s/%s' "$1" "$STORE_REL"; }
lessons_path() { printf '%s/%s' "$1" "$LESSONS_REL"; }

store_count() {
  local f
  f="$(store_path "$1")"
  if [[ -f "$f" ]]; then
    awk 'END { print NR + 0 }' "$f"
  else
    printf '0'
  fi
}

# Lesson bullets only, so the "# Lessons" header never inflates the count.
lesson_count() {
  local f
  f="$(lessons_path "$1")"
  if [[ -f "$f" ]]; then
    awk '/^- / { n++ } END { print n + 0 }' "$f"
  else
    printf '0'
  fi
}

lessons_mention() {
  local f
  f="$(lessons_path "$1")"
  if [[ -f "$f" ]]; then
    awk -v P="$2" 'index($0, P) > 0 { n++ } END { print n + 0 }' "$f"
  else
    printf '0'
  fi
}

# How many times this action was recorded as EMITTED across the whole store.
# Counted from the store the script wrote, not from what it printed.
store_emitted_count() {
  local f
  f="$(store_path "$1")"
  [[ -f "$f" ]] || {
    printf '0'
    return 0
  }
  awk -v A="$2" '
    {
      s = $0
      while (match(s, /\{"action":"[^"]*","metric":[0-9]+,"emitted":(true|false)/)) {
        obj = substr(s, RSTART, RLENGTH)
        s = substr(s, RSTART + RLENGTH)
        if (!match(obj, /"action":"[^"]*"/)) continue
        if (substr(obj, RSTART + 10, RLENGTH - 11) != A) continue
        if (match(obj, /"emitted":true/)) n++
      }
    }
    END { print n + 0 }
  ' "$f"
}

assert_eq() {
  if [[ "$2" == "$3" ]]; then
    pass "$1"
  else
    fail "$1 (expected '$3', got '$2')"
  fi
}

# ---------------------------------------------------------------------------
# DEFAULT OFF. An unconfigured repository reviews nothing and records nothing.
# ---------------------------------------------------------------------------
new_repo unset
run_sut check "$REPO" --turns 99
assert_eq "unconfigured repo: check exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: adapter is none" "$(kv adapter)" "none"
assert_eq "unconfigured repo: nothing is triggered" "$(kv triggered)" "false"

run_sut emit "$REPO" --turns 99 --class-a some-adjustment
assert_eq "unconfigured repo: emit exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: the review is skipped" "$(kv review)" "skipped"
assert_eq "unconfigured repo: zero records written" "$(store_count "$REPO")" "0"
if [[ -e "$REPO/.specify" ]]; then
  fail "unconfigured repo: no runtime directory is created"
else
  pass "unconfigured repo: no runtime directory is created"
fi

run_sut show "$REPO"
assert_eq "unconfigured repo: show exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: show reports zero records" "$(kv records)" "0"

# An explicit opt-out is the same no-op as never configuring the capability.
new_repo none
run_sut emit "$REPO" --turns 99 --class-a some-adjustment
assert_eq "adapter none: emit exits 0" "$LAST_RC" "0"
assert_eq "adapter none: zero records written" "$(store_count "$REPO")" "0"

# A configured-but-unknown adapter fails LOUD rather than degrading to none: a
# typo must not be indistinguishable from a deliberate opt-out.
new_repo bogus
run_sut show "$REPO"
assert_eq "unknown adapter fails loud" "$LAST_RC" "1"

# ---------------------------------------------------------------------------
# TRIGGERS. Each fires at its documented default and stays silent one unit
# below it, so a threshold cannot drift without this failing.
# ---------------------------------------------------------------------------
new_repo
run_sut check "$REPO" --turns 8
assert_eq "trigger: 8 turns fires" "$(kv trigger)" "turns"
run_sut check "$REPO" --turns 7
assert_eq "trigger: 7 turns does not fire" "$(kv triggered)" "false"

run_sut check "$REPO" --elapsed-minutes 45
assert_eq "trigger: 45 minutes fires" "$(kv trigger)" "elapsed-minutes"
run_sut check "$REPO" --elapsed-minutes 44
assert_eq "trigger: 44 minutes does not fire" "$(kv triggered)" "false"

run_sut check "$REPO" --retained-bytes 153600
assert_eq "trigger: 150 KB retained fires" "$(kv trigger)" "retained-bytes"
run_sut check "$REPO" --retained-bytes 153599
assert_eq "trigger: one byte under 150 KB does not fire" "$(kv triggered)" "false"

run_sut check "$REPO" --repeat-signature aggregate-timeout-240s=2
assert_eq "trigger: a signature seen twice fires" "$(kv trigger)" "repeat-failure-signature"
run_sut check "$REPO" --repeat-signature aggregate-timeout-240s=1
assert_eq "trigger: a signature seen once does not fire" "$(kv triggered)" "false"

run_sut check "$REPO" --dispatch-no-result 1
assert_eq "trigger: one resultless dispatch fires" "$(kv trigger)" "dispatch-no-result"
run_sut check "$REPO" --dispatch-no-result 0
assert_eq "trigger: zero resultless dispatches does not fire" "$(kv triggered)" "false"

run_sut check "$REPO" --budget-pct 50
assert_eq "trigger: 50% budget fires" "$(kv trigger)" "budget-threshold"
assert_eq "trigger: 50% budget names its band" "$(kv budgetBand)" "50"
run_sut check "$REPO" --budget-pct 49
assert_eq "trigger: 49% budget does not fire" "$(kv triggered)" "false"

# FIRST TO FIRE WINS, and the winner is the most diagnostic dimension. A turn
# count is why a quiet session surfaces; a repeated failure signature is why
# this loop exists.
run_sut check "$REPO" --turns 40 --elapsed-minutes 120 --retained-bytes 900000 \
  --budget-pct 95 --dispatch-no-result 3 --repeat-signature aggregate-timeout-240s=4
assert_eq "first to fire: the repeat signature outranks every other dimension" \
  "$(kv trigger)" "repeat-failure-signature"

# ---------------------------------------------------------------------------
# BUDGET BANDS fire once each. A band that re-armed every turn would make the
# 50% warning worthless by the time the 90% one mattered.
# ---------------------------------------------------------------------------
new_repo
run_sut emit "$REPO" --budget-pct 55 --class-c handoff-to-fresh-session=55
assert_eq "budget band: the 50 band is recorded" "$(kv budgetBand)" "50"
run_sut check "$REPO" --budget-pct 60
assert_eq "budget band: 50 does not fire twice" "$(kv triggered)" "false"
run_sut check "$REPO" --budget-pct 72
assert_eq "budget band: 70 fires on its own" "$(kv budgetBand)" "70"
run_sut check "$REPO" --budget-pct 95
assert_eq "budget band: 90 fires on its own" "$(kv budgetBand)" "90"

# ---------------------------------------------------------------------------
# ADVERSARIAL 1 — a review that changed nothing WHILE a repeat failure was
# present. This is the record a stuck session would write, and it is refused.
# ---------------------------------------------------------------------------
new_repo
run_sut emit "$REPO" --repeat-signature aggregate-timeout-240s=2
assert_eq "adversarial: an empty review over a repeat failure is refused" "$LAST_RC" "2"
assert_eq "adversarial: the refused review wrote nothing" "$(store_count "$REPO")" "0"

run_sut emit "$REPO" --dispatch-no-result 1
assert_eq "adversarial: an empty review over a resultless dispatch is refused" "$LAST_RC" "2"
assert_eq "adversarial: that refusal wrote nothing either" "$(store_count "$REPO")" "0"

# The SAME observation with a real adjustment is recorded. The rule refuses an
# empty claim, never an honest one.
run_sut emit "$REPO" --repeat-signature aggregate-timeout-240s=2 \
  --class-a focused-row-only-until-frozen-bytes
assert_eq "the same observation WITH an adjustment is recorded" "$LAST_RC" "0"
assert_eq "an adjustment sets netEffect to adjusted" "$(kv netEffect)" "adjusted"
assert_eq "one record was written" "$(store_count "$REPO")" "1"

# CHURN RULE 1 — Class A writes no artifact beyond the review line itself.
assert_eq "Class A promotes no lesson" "$(lesson_count "$REPO")" "0"

# CHURN RULE 6 — empty is valid when nothing diagnostic was observed.
new_repo
run_sut emit "$REPO" --turns 8
assert_eq "a quiet review is recorded" "$LAST_RC" "0"
assert_eq "a quiet review records no-adjustment" "$(kv netEffect)" "no-adjustment"
assert_eq "no-adjustment is a real record" "$(store_count "$REPO")" "1"

# ---------------------------------------------------------------------------
# ADVERSARIAL 2 — Class B promoted BELOW the recurrence threshold.
# ---------------------------------------------------------------------------
new_repo
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
assert_eq "Class B is buffered, not promoted, in flight" "$(kv classB)" "1"
assert_eq "first sighting counts once" "$(kv classB.outer-timeout-below-child-budget.occurrences)" "1"
assert_eq "Class B writes no lesson in flight" "$(lesson_count "$REPO")" "0"

run_sut emit "$REPO" --close --promote outer-timeout-below-child-budget
assert_eq "adversarial: promotion below the threshold is refused" "$LAST_RC" "2"
assert_eq "adversarial: the refused promotion wrote no lesson" "$(lesson_count "$REPO")" "0"

# The SECOND sighting makes it a candidate, and close promotes it.
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
assert_eq "second sighting counts twice" "$(kv classB.outer-timeout-below-child-budget.occurrences)" "2"
run_sut emit "$REPO" --close
assert_eq "at threshold the pattern is promoted" "$LAST_RC" "0"
assert_eq "promotion count is recorded" "$(kv promotedCount)" "1"
assert_eq "netEffect names the promotion" "$(kv netEffect)" "promoted"
assert_eq "exactly one lesson bullet was written" "$(lesson_count "$REPO")" "1"
assert_eq "the lesson names the pattern" "$(lessons_mention "$REPO" "outer-timeout-below-child-budget")" "1"
assert_eq "the lesson carries strippable provenance" "$(lessons_mention "$REPO" "bubbles-lesson-meta: source=session-review")" "1"

# A pattern already promoted is not promoted again by a later close.
run_sut emit "$REPO" --close
assert_eq "an already-promoted pattern is not promoted twice" "$(kv promotedCount)" "0"
assert_eq "and no second lesson bullet appears" "$(lesson_count "$REPO")" "1"

# ---------------------------------------------------------------------------
# CHURN RULE 2 second half — a Class A adjustment that RESOLVED the pattern
# takes it out of the running. The session already learned and acted on it.
# ---------------------------------------------------------------------------
new_repo
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
run_sut emit "$REPO" --turns 8 --class-a raise-outer-timeout=outer-timeout-below-child-budget
run_sut emit "$REPO" --close
assert_eq "a Class A adjustment that resolved the pattern blocks promotion" "$(kv promotedCount)" "0"
assert_eq "and no lesson is written" "$(lesson_count "$REPO")" "0"
run_sut emit "$REPO" --close --promote outer-timeout-below-child-budget
assert_eq "an explicit promotion of a resolved pattern is refused" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
# CHURN RULE 3 — at most 3 promotions per session; the excess is DROPPED with a
# recorded count and never persisted.
# ---------------------------------------------------------------------------
new_repo
for _ in 1 2; do
  for p in pattern-alpha-outer-timeout pattern-beta-retry-identical-bytes \
    pattern-gamma-aggregate-rerun pattern-delta-state-never-written; do
    run_sut emit "$REPO" --turns 8 --class-b "$p"
  done
done
assert_eq "eight sightings were recorded" "$(store_count "$REPO")" "8"
run_sut emit "$REPO" --close
assert_eq "promotion is capped at three" "$(kv promotedCount)" "3"
assert_eq "the excess is recorded as dropped" "$(kv droppedCount)" "1"
assert_eq "only three lesson bullets were written" "$(lesson_count "$REPO")" "3"

# ---------------------------------------------------------------------------
# CHURN RULE 5 — a dismissed pattern is suppressed permanently, by BOTH paths.
# ---------------------------------------------------------------------------
new_repo
mkdir -p "$REPO/.specify/memory"
{
  printf '## Dismissed 2026-08-01T00:00:00Z\n'
  printf '## Skill Proposal: outer-timeout-below-child-budget\n'
  printf -- '- Pattern: outer-timeout-below-child-budget\n'
} > "$REPO/$DISMISSED_REL"
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
assert_eq "a dismissed pattern is marked suppressed on sight" \
  "$(kv classB.outer-timeout-below-child-budget.suppressed)" "true"
run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
run_sut emit "$REPO" --close
assert_eq "a dismissed pattern is never promoted automatically" "$(kv promotedCount)" "0"
assert_eq "and no lesson is written for it" "$(lesson_count "$REPO")" "0"
run_sut emit "$REPO" --close --promote outer-timeout-below-child-budget
assert_eq "an explicit promotion of a dismissed pattern is refused" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
# ADVERSARIAL 3 — Class C re-emitted every turn. It is deduplicated unless the
# underlying metric worsened by >= 25%.
# ---------------------------------------------------------------------------
new_repo
for _ in 1 2 3 4 5; do
  run_sut emit "$REPO" --turns 8 --class-c handoff-to-fresh-session=62 \
    --class-c-reason "handoff-to-fresh-session=retained bytes at 62% of cap"
done
assert_eq "adversarial: five identical Class C emissions, five records" "$(store_count "$REPO")" "5"
assert_eq "adversarial: only the first was actually emitted" "$(store_emitted_count "$REPO" handoff-to-fresh-session)" "1"

# Worsening by less than 25% is still noise.
run_sut emit "$REPO" --turns 8 --class-c handoff-to-fresh-session=70
assert_eq "a 13% worsening is still deduplicated" "$(kv classC.handoff-to-fresh-session.emitted)" "false"
# Worsening by 25% or more is news.
run_sut emit "$REPO" --turns 8 --class-c handoff-to-fresh-session=78
assert_eq "a 25% worsening is re-emitted" "$(kv classC.handoff-to-fresh-session.emitted)" "true"
assert_eq "the store agrees two were emitted" "$(store_emitted_count "$REPO" handoff-to-fresh-session)" "2"
# A Class C that was emitted is an escalation even with no Class A.
assert_eq "an emitted Class C escalates" "$(kv netEffect)" "escalated"

# ---------------------------------------------------------------------------
# AGENT CONSUMPTION. Active adjustments are what a dispatch packet carries as
# activeAdjustments[], and an adjustment contradicted twice becomes an
# automatic Class B candidate rather than being re-applied forever.
# ---------------------------------------------------------------------------
new_repo
run_sut emit "$REPO" --turns 8 --class-a focused-row-only-until-frozen-bytes
run_sut emit "$REPO" --turns 8 --class-a stop-retrying-identical-bytes
run_sut show "$REPO" --active-adjustments
assert_eq "the most recent adjustment is listed first" "$(kv activeAdjustment.1)" "stop-retrying-identical-bytes"
assert_eq "earlier adjustments stay active" "$(kv activeAdjustment.2)" "focused-row-only-until-frozen-bytes"

run_sut emit "$REPO" --turns 8 --contradicted focused-row-only-until-frozen-bytes
assert_eq "one contradiction is not yet a candidate" "$(kv classB)" "0"
run_sut emit "$REPO" --turns 8 --contradicted focused-row-only-until-frozen-bytes
assert_eq "a twice-contradicted adjustment becomes a Class B candidate" "$(kv classB)" "1"
assert_eq "and it is named after the adjustment it contradicts" \
  "$(kv classB.adjustment-contradicted:focused-row-only-until-frozen-bytes.occurrences)" "1"

# G083 compaction: only the two most recent review records stay raw. Four
# reviews were recorded above, so two are compaction candidates.
run_sut show "$REPO"
assert_eq "only two review records stay raw" "$(kv rawRetained)" "2"
assert_eq "four records recorded in this fixture" "$(kv records)" "4"
assert_eq "the rest are compaction candidates" "$(kv compactable)" "2"

# ---------------------------------------------------------------------------
# CHURN RULE 7 — the review is itself budgeted. It must not become the cost it
# measures. Only --close is exempt, because refusing the closing review would
# strand the session's promotions.
# ---------------------------------------------------------------------------
new_repo
for _ in $(seq 1 12); do
  run_sut emit "$REPO" --turns 8 --class-a keep-going
done
assert_eq "twelve reviews are recorded" "$(store_count "$REPO")" "12"
run_sut emit "$REPO" --turns 8 --class-a one-more
assert_eq "a thirteenth review is refused" "$LAST_RC" "2"
assert_eq "and it wrote nothing" "$(store_count "$REPO")" "12"
run_sut emit "$REPO" --close
assert_eq "the closing review is always allowed" "$LAST_RC" "0"

# ---------------------------------------------------------------------------
# CHURN RULE 4 — the review NEVER writes under bubbles/ or agents/ (and so
# never bubbles/workflows.yaml). Enforced on the PHYSICAL path, because a
# symlinked store is how this boundary gets crossed by accident.
# ---------------------------------------------------------------------------
new_repo
mkdir -p "$REPO/agents/mem"
ln -s "$REPO/agents/mem" "$REPO/.specify"
run_sut emit "$REPO" --turns 8 --class-a some-adjustment
assert_eq "a store resolving under agents/ is refused" "$LAST_RC" "2"
if [[ -e "$REPO/agents/mem/runtime" ]]; then
  fail "the refused write created nothing under agents/"
else
  pass "the refused write created nothing under agents/"
fi

new_repo
mkdir -p "$REPO/.specify/runtime" "$REPO/bubbles/mem"
ln -s "$REPO/bubbles/mem" "$REPO/.specify/memory"
run_sut emit "$REPO" --turns 8 --class-a some-adjustment
assert_eq "a lessons path resolving under bubbles/ is refused" "$LAST_RC" "2"
assert_eq "the boundary is checked before the record is appended" "$(store_count "$REPO")" "0"

# No bypass exists.
new_repo
run_sut emit "$REPO" --turns 8 --force
assert_eq "a bypass flag is rejected by name" "$LAST_RC" "2"
run_sut emit "$REPO" --turns 8 --skip-churn-control
assert_eq "--skip-churn-control is rejected by name" "$LAST_RC" "2"
run_sut check "$REPO" --assume-reviewed
assert_eq "check has no bypass either" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
# ONE LESSONS LOOP, NOT A SECOND ONE. A promoted lesson is clustered by the
# EXISTING skill-evolution.sh, unchanged. Three sessions promote the same
# pattern; the shipped clusterer then reaches its own triggerThreshold and
# writes a proposal naming it. If promotion ever stopped feeding that corpus,
# this fails.
# ---------------------------------------------------------------------------
if [[ -f "$SKILL_EVOLUTION" ]]; then
  new_repo
  mkdir -p "$REPO/bubbles/scripts"
  cp "$SKILL_EVOLUTION" "$REPO/bubbles/scripts/skill-evolution.sh"
  {
    printf 'skillEvolution:\n'
    printf '  enabled: true\n'
    printf '  triggerThreshold: 3\n'
  } > "$REPO/bubbles/workflows.yaml"
  # Three SESSIONS: a session is one review ledger, so the store is reset
  # between them while the lessons corpus accumulates, exactly as it would
  # across three real runs in one repository.
  for _ in 1 2 3; do
    rm -f "$(store_path "$REPO")"
    run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
    run_sut emit "$REPO" --turns 8 --class-b outer-timeout-below-child-budget
    run_sut emit "$REPO" --close
  done
  assert_eq "three sessions promoted three lessons" "$(lesson_count "$REPO")" "3"
  SE_OUT="$(bash "$REPO/bubbles/scripts/skill-evolution.sh" show 2>&1)"
  if awk 'index($0, "## Skill Proposal:") > 0 { n++ } END { exit(n > 0 ? 0 : 1) }' <<< "$SE_OUT"; then
    pass "the existing skill-evolution clusterer produces a proposal from promoted lessons"
  else
    fail "promoted lessons did not reach skill-evolution clustering"
  fi
  if awk 'index($0, "outer-timeout-below-child-budget") > 0 { n++ } END { exit(n > 0 ? 0 : 1) }' <<< "$SE_OUT"; then
    pass "the proposal names the promoted pattern, not the provenance comment"
  else
    fail "the proposal does not name the promoted pattern"
  fi
else
  fail "skill-evolution.sh not found; the promotion feed cannot be proven"
fi

# ---------------------------------------------------------------------------
# RESULT-ENVELOPE `reviewCompliance` is OPTIONAL, so every envelope written
# before this scope existed stays valid.
# ---------------------------------------------------------------------------
if python3 - "$ENVELOPE_SCHEMA" <<'PY'; then
import json, sys
schema = json.load(open(sys.argv[1]))
props = schema["properties"]
assert "reviewCompliance" in props, "reviewCompliance is absent from the schema"
assert "reviewCompliance" not in schema.get("required", []), "reviewCompliance must be optional"
item = props["reviewCompliance"]["items"]
assert set(item["required"]) == {"adjustment", "honored"}, item["required"]
PY
  pass "result-envelope schema carries an OPTIONAL reviewCompliance"
else
  fail "result-envelope schema does not carry an optional reviewCompliance"
fi

# ---------------------------------------------------------------------------
printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
