#!/usr/bin/env bash
# File: scenario-match-lib-selftest.sh
#
# Hermetic selftest for scenario-match-lib.sh (BUG-004).
#
# BUG-004 was NOT that one of the two G068 matchers was wrong. It was that the
# rule existed TWICE — scenario_matches_dod() in traceability-guard.sh and
# stg_scenario_matches_dod() in state-transition-guard.sh — held together by
# nothing but a comment saying they "MUST stay aligned". They drifted anyway.
#
# So this selftest does not merely check verdicts. It ENFORCES the property
# that made the drift impossible to notice:
#
#   Part 1  there is exactly ONE definition of each matcher function in the
#           whole script tree, and it lives in scenario-match-lib.sh; both
#           guards source the lib and neither carries a local copy.
#   Part 2  both guards reach the matcher through the SAME function, and at an
#           IDENTICAL policy that function returns IDENTICAL verdicts on every
#           fixture — i.e. there is genuinely one implementation, not two that
#           happen to agree today.
#   Part 3  the two POLICIES differ ONLY on the four divergences documented in
#           the lib header (D1 id families, D2 uncited id, D3 citation
#           boundary, D4 id lexeme). Every other fixture must agree.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/scenario-match-lib.sh"
TRACE_GUARD="$SCRIPT_DIR/traceability-guard.sh"
STG_GUARD="$SCRIPT_DIR/state-transition-guard.sh"

for required in "$LIB" "$TRACE_GUARD" "$STG_GUARD"; do
  if [[ ! -f "$required" ]]; then
    echo "[selftest scenario-match-lib] FAIL: missing $required" >&2
    exit 1
  fi
done

# shellcheck source=/dev/null
source "$LIB"

failures=0
passes=0
pass() {
  echo "PASS: $1"
  passes=$((passes + 1))
}
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# ---------------------------------------------------------------------------
# Fixture corpus: flag|||reason|||scenario|||dod-item
#
#   flag   same    — both policies MUST return the same verdict
#          differs — the policies MUST disagree, for the named documented reason
#   reason D1..D4 for a divergence; otherwise the coverage category
#
# Categories covered: exact id citation, id present but uncited, no id at all,
# small scenarios (<3 significant words), high word overlap, low word overlap,
# and morphological near-misses.
# ---------------------------------------------------------------------------
FIXTURES=(
  "same|||exact-scn-citation|||SCN-001 user login succeeds with valid credentials|||Implement SCN-001 login happy path"
  "same|||scn-citation-punctuation|||SCN-010 trailing punctuation citation|||Implement trailing punctuation citation, see SCN-010."
  "same|||uncited-scn-low-overlap|||SCN-001 user login succeeds with valid credentials|||Implement login happy path with valid credentials"
  "same|||uncited-ac-low-overlap|||AC-99 uncited and lexically unrelated|||completely orthogonal grafana dashboard work"
  "same|||ac-citation-and-lexical-agree|||AC-14 invoice totals recomputed on line change|||Recompute invoice totals on line change (AC-14)"
  "same|||no-id-high-overlap|||gateway returns 503 when upstream circuit breaker opens|||gateway returns 503 when the upstream circuit breaker opens"
  "same|||no-id-low-overlap|||gateway returns 503 when upstream circuit breaker opens|||dashboard sidebar collapses on narrow viewports"
  "same|||no-id-high-overlap-long|||audit log append only under concurrent writers|||audit log is append only under concurrent writers"
  "same|||no-id-low-overlap-long|||audit log append only under concurrent writers|||audit log rotated weekly"
  "same|||small-scenario-all-words-match|||CSV import|||CSV import implemented"
  "same|||small-scenario-partial-words|||CSV import|||protobuf schema regenerated"
  "same|||small-scenario-single-word-exact|||retry|||retry"
  "same|||small-scenario-single-word-nomatch|||retry|||backoff"
  "same|||morph-plural-tolerance|||JSON request rejected|||json requests rejected with 415 protobuf only middleware"
  "same|||morph-inflection-tolerance|||persisted trip group|||post api trips group handler implemented with protobuf decode postgresql persist"
  "same|||morph-near-miss-below-floor|||stale advisory badge|||staleness warning amber displayed when bundled data published at 90 days"
  "same|||morph-short-root-refused|||test coverage|||testament of unrelated things"
  "same|||morph-y-stem-not-a-plural|||retry|||retries"
  "same|||all-stop-words-substring-hit|||the and but|||the and but with all any"
  "same|||all-stop-words-substring-miss|||the and but|||completely different sentence"
  "differs|||D1|||AC-77 zebra quokka narwhal|||AC-77 implemented"
  "differs|||D1|||FR-90 zebra quokka narwhal|||FR-90 implemented"
  "differs|||D1|||UC-90 zebra quokka narwhal|||UC-90 implemented"
  "differs|||D2|||SCN-055 caching layer warms on boot with lots of shared vocabulary|||caching layer warms on boot with lots of shared vocabulary"
  "differs|||D2|||SCN-1 alpha bravo charlie delta|||SCN-12 alpha bravo charlie delta"
  "differs|||D3|||SCN-011 leading citation boundary|||xSCN-011 leading citation boundary is not a citation"
  "differs|||D4|||SCN-_zzz quokka narwhal zebra|||SCN-_zzz implemented"
)

DOCUMENTED_REASONS="D1 D2 D3 D4"

# ---------------------------------------------------------------------------
# Part 1: exactly one definition of each matcher function, in the lib
# ---------------------------------------------------------------------------
echo "--- Part 1: single implementation ---"

# Any local re-definition would be named after one of these, with or without a
# guard-local prefix (the drifted copies were stg_-prefixed).
MATCHER_FNS="scenario_matches_dod word_matches_text significant_words normalize_text extract_trace_ids"

for fn in $MATCHER_FNS; do
  # Definition sites only (`name() {` at column 0), excluding selftests so this
  # check cannot match its own fixtures.
  defs="$(grep -lE "^[a-z_]*${fn}\(\)" "$SCRIPT_DIR"/*.sh 2>/dev/null | grep -v 'selftest' || true)"
  def_count="$(printf '%s' "$defs" | grep -c . || true)"
  if [[ "$def_count" -eq 1 ]] && [[ "$(basename "$defs")" == "scenario-match-lib.sh" ]]; then
    pass "single definition of *${fn}() and it lives in scenario-match-lib.sh"
  else
    fail "*${fn}() must be defined exactly once, in scenario-match-lib.sh; found: ${defs:-<none>}"
  fi
done

for guard in "$TRACE_GUARD" "$STG_GUARD"; do
  if grep -q 'source "\$SCRIPT_DIR/scenario-match-lib.sh"' "$guard"; then
    pass "$(basename "$guard") sources scenario-match-lib.sh"
  else
    fail "$(basename "$guard") must source scenario-match-lib.sh"
  fi
done

# ---------------------------------------------------------------------------
# Part 2: both guards reach the SAME function, and it agrees with itself
# ---------------------------------------------------------------------------
echo "--- Part 2: both call paths resolve to one function ---"

# Derived from the guard sources, not hard-coded here: if a guard is rewired to
# a different function or a different policy, these reads change with it.
# Both reads are anchored to the INVOCATION, so prose in a nearby comment that
# names the other policy cannot be mistaken for a call site.
# Prints the unique value on stdout, or nothing when it is absent/ambiguous.
resolve_unique() {
  local file="$1" pattern="$2" got count
  got="$(grep -Eo "$pattern" "$file" | sort -u || true)"
  count="$(printf '%s' "$got" | grep -c . || true)"
  [[ "$count" -eq 1 ]] || return 0
  printf '%s' "$got"
}

CALL_RE='[a-z_]*scenario_matches_dod[^#]*(structural-strict|id-hint-lenient)'

resolve_policy() {
  local got count
  # Two call sites in one guard are fine; what must be unique is the POLICY.
  got="$(grep -Eo "$CALL_RE" "$1" | grep -Eo 'structural-strict|id-hint-lenient' | sort -u || true)"
  count="$(printf '%s' "$got" | grep -c . || true)"
  [[ "$count" -eq 1 ]] || return 0
  printf '%s' "$got"
}

trace_fn="$(resolve_unique "$TRACE_GUARD" '[a-z_]*scenario_matches_dod')"
stg_fn="$(resolve_unique "$STG_GUARD" '[a-z_]*scenario_matches_dod')"
trace_policy="$(resolve_policy "$TRACE_GUARD")"
stg_policy="$(resolve_policy "$STG_GUARD")"

if [[ "$trace_fn" == "bubbles_scenario_matches_dod" && "$stg_fn" == "bubbles_scenario_matches_dod" ]]; then
  pass "both guards invoke bubbles_scenario_matches_dod"
else
  fail "both guards must invoke bubbles_scenario_matches_dod (traceability=$trace_fn state-transition=$stg_fn)"
fi

if [[ "$trace_policy" == "id-hint-lenient" ]]; then
  pass "traceability-guard.sh declares the id-hint-lenient policy"
else
  fail "traceability-guard.sh must declare id-hint-lenient (got: $trace_policy)"
fi

if [[ "$stg_policy" == "structural-strict" ]]; then
  pass "state-transition-guard.sh declares the structural-strict policy"
else
  fail "state-transition-guard.sh must declare structural-strict (got: $stg_policy)"
fi

# Call through the DERIVED function names, so this compares the two guards'
# actual call paths rather than a name written into this file.
verdict() {
  local fn="$1" scenario="$2" dod="$3" policy="$4"
  if "$fn" "$scenario" "$dod" "$policy"; then printf '0'; else printf '1'; fi
}

if [[ -n "$trace_fn" && -n "$stg_fn" ]]; then
  agree=0
  disagree=0
  for policy in structural-strict id-hint-lenient; do
    for row in "${FIXTURES[@]}"; do
      rest="${row#*|||}"
      rest="${rest#*|||}"
      scenario="${rest%%|||*}"
      dod="${rest#*|||}"
      tv="$(verdict "$trace_fn" "$scenario" "$dod" "$policy")"
      sv="$(verdict "$stg_fn" "$scenario" "$dod" "$policy")"
      if [[ "$tv" == "$sv" ]]; then
        agree=$((agree + 1))
      else
        disagree=$((disagree + 1))
        fail "call paths disagree at policy=$policy on: $scenario  ||  $dod"
      fi
    done
  done
  if [[ "$disagree" -eq 0 ]]; then
    pass "both call paths agree on all $agree (fixture x policy) verdicts"
  fi
fi

# ---------------------------------------------------------------------------
# Part 3: the policies differ ONLY where documented
# ---------------------------------------------------------------------------
echo "--- Part 3: policy difference is exactly the documented set ---"

seen_reasons=""
for row in "${FIXTURES[@]}"; do
  flag="${row%%|||*}"
  rest="${row#*|||}"
  reason="${rest%%|||*}"
  rest="${rest#*|||}"
  scenario="${rest%%|||*}"
  dod="${rest#*|||}"

  strict="$(verdict bubbles_scenario_matches_dod "$scenario" "$dod" structural-strict)"
  lenient="$(verdict bubbles_scenario_matches_dod "$scenario" "$dod" id-hint-lenient)"

  case "$flag" in
    same)
      if [[ "$strict" == "$lenient" ]]; then
        pass "policies agree ($reason): strict=$strict lenient=$lenient"
      else
        fail "UNDOCUMENTED policy divergence ($reason): strict=$strict lenient=$lenient on: $scenario  ||  $dod"
      fi
      ;;
    differs)
      case " $DOCUMENTED_REASONS " in
        *" $reason "*) ;;
        *)
          fail "divergence fixture cites an undocumented reason '$reason'"
          continue
          ;;
      esac
      seen_reasons="$seen_reasons $reason"
      if [[ "$strict" != "$lenient" ]]; then
        pass "documented divergence $reason: strict=$strict lenient=$lenient"
      else
        fail "documented divergence $reason no longer diverges (strict=$strict lenient=$lenient) on: $scenario  ||  $dod"
      fi
      ;;
    *)
      fail "fixture has an unknown flag '$flag'"
      ;;
  esac
done

# Every documented divergence must still be exercised, so a reason cannot be
# silently dropped from the fixture table while staying in the lib header.
for reason in $DOCUMENTED_REASONS; do
  case " $seen_reasons " in
    *" $reason "*) pass "documented divergence $reason is covered by a fixture" ;;
    *) fail "documented divergence $reason has no fixture" ;;
  esac
done

# ---------------------------------------------------------------------------
# Unknown policy must be loud, never a silent "no match" that disables G068.
# ---------------------------------------------------------------------------
echo "--- Part 4: unknown policy is refused ---"
rc=0
bubbles_scenario_matches_dod "SCN-001 anything" "SCN-001 anything" bogus-policy 2>/dev/null || rc=$?
if [[ "$rc" -eq 2 ]]; then
  pass "unknown id policy returns 2 (loud) rather than a silent no-match"
else
  fail "unknown id policy must return 2; got $rc"
fi

echo ""
echo "scenario-match-lib selftest: $passes passed, $failures failed"
[[ "$failures" -eq 0 ]]
