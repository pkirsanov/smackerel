#!/usr/bin/env bash
#
# scenario-match-lib.sh — the single implementation of Gherkin-scenario → DoD
# item matching for Gate G068 (BUG-004). Sourced, not executed.
#
# BUG-004: this rule was written TWICE — scenario_matches_dod() in
# traceability-guard.sh and stg_scenario_matches_dod() in
# state-transition-guard.sh — with only a comment ("both implementations MUST
# stay aligned") holding them together. Nothing enforced that comment, so they
# drifted. This lib removes the duplication WITHOUT changing either guard's
# observable verdict: the difference between them is now an EXPLICIT, NAMED
# policy argument instead of accidental divergence.
#
# Provides:
#   bubbles_scenario_matches_dod <scenario> <dod-item> <id-policy>
#                                           0 iff the DoD item faithfully
#                                           represents the scenario's claim
#   bubbles_scenario_extract_trace_ids <text>
#                                           print SCN/AC/FR/UC ids, one per line
#   bubbles_scenario_normalize_text <text>  lowercase + collapse to [a-z0-9 ]
#   bubbles_scenario_significant_words <text>
#                                           print scoring words, one per line
#   bubbles_scenario_word_matches_text <word> <normalized-text>
#                                           0 iff <word> occurs, modulo the
#                                           bounded plural/inflection tolerance
#
# ---------------------------------------------------------------------------
# The two id policies (the ONLY legitimate difference between the two callers)
# ---------------------------------------------------------------------------
#
#   structural-strict   — used by state-transition-guard.sh (Check 22).
#     A scenario that CARRIES an id but whose DoD item does not cite it returns
#     1 with NO lexical fallback. This is the IMP-027 SCOPE-8 decision: when a
#     stable id exists the linkage is a FACT, so the tunable lexical proxy is
#     not allowed to override it.
#
#   id-hint-lenient     — used by traceability-guard.sh.
#     A scenario that carries an id whose DoD item does not cite it FALLS
#     THROUGH to lexical word overlap. The id is treated as a hint that can
#     confirm a match, never as a fact that can veto one.
#
# The policies differ in exactly FOUR documented ways. scenario-match-lib-
# selftest.sh pins this list: any fixture outside it that starts to diverge
# fails the selftest.
#
#   D1 id families       strict: SCN only. lenient: SCN, AC, FR, UC.
#   D2 uncited id        strict: decisive failure, no lexical fallback.
#                        lenient: falls through to lexical word overlap.
#   D3 citation boundary strict requires a non-[A-Za-z0-9_-] neighbour on BOTH
#                        sides of the cited id, so `SCN-12` does not cite
#                        `SCN-1` and neither does `xSCN-1`. lenient compares
#                        ids extracted from both sides with the same
#                        expression, which has no left boundary, so `xSCN-1`
#                        DOES cite `SCN-1`.
#   D4 id lexeme         strict requires the first character after the family
#                        dash to be alphanumeric. lenient also accepts `_`
#                        and `-` there, so it sees an id in `SCN-_foo` where
#                        strict sees none and falls to lexical.
#
# Everything below the id stage — normalization, significant-word selection,
# the plural/inflection tolerance, and the percentage-threshold-with-floor
# scoring — is IDENTICAL for both policies. That part never legitimately
# differed, and scenario-match-lib-selftest.sh asserts it cannot start to.
#
# Idempotent: guarded against double-source. No side effects at source time.

[[ -n "${_BUBBLES_SCENARIO_MATCH_LIB_SOURCED:-}" ]] && return 0
_BUBBLES_SCENARIO_MATCH_LIB_SOURCED=1

# Trace ids in the id-hint-lenient vocabulary (SCN, AC, FR, UC). One per line;
# empty output (exit 0) when the text carries none.
bubbles_scenario_extract_trace_ids() {
  local value="$1"
  printf '%s\n' "$value" | grep -Eo '(SCN|AC|FR|UC)-[A-Za-z0-9_-]+' || true
}

bubbles_scenario_normalize_text() {
  local value="$1"
  value="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  value="$(printf '%s' "$value" | sed -E 's/[^a-z0-9]+/ /g; s/[[:space:]]+/ /g; s/^ //; s/ $//')"
  printf '%s' "$value"
}

bubbles_scenario_significant_words() {
  local text="$1"
  local normalized
  local word

  normalized="$(bubbles_scenario_normalize_text "$text")"
  for word in $normalized; do
    # G068 false-positive fix (v3.8.0): min word length is 3, not 4, so
    # 3-letter domain words (API, DoD, SLA, CSV, CSP, JWT, SDK, CLI, CRD,
    # SBOM) are counted as significant instead of stripped as noise.
    if [[ ${#word} -lt 3 ]]; then
      continue
    fi
    # G068 false-positive fix (v3.8.0): TRUE stop words only. Domain-relevant
    # words (user, users, system, should, must, have, has, will, given, after,
    # before, where, their, there, about, only) are frequently the
    # distinguishing words in a Gherkin scenario title, so they stay.
    case "$word" in
      the | are | was | were | been | being | for | from | with | and | but | not | then | else | while | when | that | this | these | those | its | into | onto | out | all | any | each | every | some | more | less | also)
        continue
        ;;
    esac
    printf '%s\n' "$word"
  done
}

# G068 false-positive fix: whole-word overlap with no stemming meant a single
# singular/plural mismatch could sink an otherwise near-verbatim DoD item.
# Scenario "JSON request rejected" scored 2 against DoD "JSON requests rejected
# with 415" — below the >=3 floor — because "request" != "requests".
# Kept to regular -s/-es forms; no general stemmer, so unrelated words still
# cannot collide.
bubbles_scenario_word_matches_text() {
  local word="$1"
  local text=" $2 "
  local singular
  local tok

  case "$text" in
    *" $word "* | *" ${word}s "* | *" ${word}es "*) return 0 ;;
  esac

  if [[ "$word" == *es && ${#word} -gt 4 ]]; then
    singular="${word%es}"
    case "$text" in *" $singular "*) return 0 ;; esac
  fi
  if [[ "$word" == *s && ${#word} -gt 3 ]]; then
    singular="${word%s}"
    case "$text" in *" $singular "*) return 0 ;; esac
  fi

  # Inflection/derivation, e.g. persisted~persist and stale~staleness. Both were
  # observed sinking otherwise-identical claims below the >=3 overlap floor.
  # Bounded to stems of 5+ chars so short roots cannot collide (test~testament).
  [[ ${#word} -ge 5 ]] || return 1
  for tok in $2; do
    case "$tok" in "$word"*) return 0 ;; esac
    [[ ${#tok} -ge 5 ]] || continue
    case "$word" in "$tok"*) return 0 ;; esac
  done

  return 1
}

# --- id stage: structural-strict ---------------------------------------------
# Returns 0 (cited), 1 (carries an id, not cited), 2 (carries no id).
_bubbles_scenario_id_verdict_strict() {
  local scenario="$1"
  local dod_item="$2"
  local scenario_scn

  scenario_scn="$(printf '%s' "$scenario" | grep -oE 'SCN-[A-Za-z0-9][A-Za-z0-9_-]*' | head -1 || true)"
  [[ -n "$scenario_scn" ]] || return 2

  # Word-boundary compare so SCN-1 does not match SCN-12.
  if printf '%s' "$dod_item" | grep -qE "(^|[^A-Za-z0-9_-])${scenario_scn}([^A-Za-z0-9_-]|\$)"; then
    return 0
  fi
  return 1
}

# --- id stage: id-hint-lenient -----------------------------------------------
# Returns 0 (cited), 1 (carries an id, not cited), 2 (carries no id).
_bubbles_scenario_id_verdict_lenient() {
  local scenario="$1"
  local dod_item="$2"
  local scenario_id
  local dod_id

  scenario_id="$(bubbles_scenario_extract_trace_ids "$scenario" | head -n 1 || true)"
  [[ -n "$scenario_id" ]] || return 2

  while IFS= read -r dod_id; do
    if [[ -n "$dod_id" ]] && [[ "$dod_id" == "$scenario_id" ]]; then
      return 0
    fi
  done < <(bubbles_scenario_extract_trace_ids "$dod_item")
  return 1
}

# bubbles_scenario_matches_dod <scenario> <dod-item> <id-policy>
#   id-policy: structural-strict | id-hint-lenient   (see the header)
# Returns 0 when the DoD item faithfully represents the scenario's claim.
# Returns 2 on an unknown policy — a caller typo must be loud, not a silent
# "no match" that would quietly disable G068.
bubbles_scenario_matches_dod() {
  local scenario="$1"
  local dod_item="$2"
  local id_policy="${3:-}"
  local dod_norm
  local words
  local word
  local score=0
  local word_count=0
  local threshold=0
  local id_verdict

  case "$id_policy" in
    structural-strict)
      _bubbles_scenario_id_verdict_strict "$scenario" "$dod_item" && id_verdict=0 || id_verdict=$?
      # A carried-but-uncited id is decisive: no lexical fallback.
      if [[ "$id_verdict" -ne 2 ]]; then
        return "$id_verdict"
      fi
      ;;
    id-hint-lenient)
      _bubbles_scenario_id_verdict_lenient "$scenario" "$dod_item" && id_verdict=0 || id_verdict=$?
      # A citation confirms a match; anything else falls through to lexical.
      if [[ "$id_verdict" -eq 0 ]]; then
        return 0
      fi
      ;;
    *)
      printf 'scenario-match-lib: unknown id policy: %s\n' "$id_policy" >&2
      return 2
      ;;
  esac

  # Lexical word overlap — IDENTICAL under both policies.
  #
  # G068 false-positive fix (v3.8.0): percentage-based threshold with floor.
  # - Very small scenarios (<3 significant words): require ALL words to match
  #   so a hard >=3 floor doesn't penalize them.
  # - Larger scenarios: require BOTH (overlap >= ceil(50% * word_count))
  #   AND (overlap >= 3) — percentage threshold with absolute floor.
  dod_norm="$(bubbles_scenario_normalize_text "$dod_item")"
  words="$(bubbles_scenario_significant_words "$scenario")"
  if [[ -z "$words" ]]; then
    [[ "$dod_norm" == *"$(bubbles_scenario_normalize_text "$scenario")"* ]]
    return
  fi

  while IFS= read -r word; do
    [[ -n "$word" ]] || continue
    word_count=$((word_count + 1))
    if bubbles_scenario_word_matches_text "$word" "$dod_norm"; then
      score=$((score + 1))
    fi
  done <<<"$words"

  if [[ "$word_count" -lt 3 ]]; then
    [[ "$score" -eq "$word_count" ]]
    return
  fi

  threshold=$(((word_count + 1) / 2))
  [[ "$score" -ge 3 && "$score" -ge "$threshold" ]]
}
