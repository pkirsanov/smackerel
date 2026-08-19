#!/usr/bin/env bash
# session-review.sh — the in-session review loop (IMP-048 SCOPE-1, LRN-8).
#
# Owner: bubbles.workflow
#
# WHY THIS EXISTS
# Every learning surface this framework has is POST-HOC. `bubbles.retro` runs
# after the work, `retro-framework-health.sh` writes a proposal after the fact,
# and `execution-ops.md` records one lesson at result-envelope close. Nothing
# reviews a run WHILE it is running. Measured across the eight workspace repos:
# `lessons.md` is header-only in each, `skill-proposals.md` exists in none, so
# `skill-evolution.sh` has never had a corpus to cluster. Five days of real
# diagnosis produced zero durable learning, and the corrections that WOULD have
# helped -- narrow the row, stop retrying identical bytes, hand off -- were
# available on day one and were never made.
#
# CORE PRINCIPLE: ADJUSTMENT IS FREE, ARTIFACTS ARE EXPENSIVE.
# The common outcome of a review is a behaviour change that writes nothing. A
# review loop that produced a document each time it fired would become the cost
# it exists to measure, which is why churn control here is the DEFAULT posture
# rather than a mitigation bolted on afterwards.
#
# THREE OUTPUT CLASSES
#   Class A  adjust now. No artifact, no approval, no threshold. The behaviour
#            changes for the rest of the session and is stated in one line.
#            This is the intended COMMON case.
#   Class B  improvement candidate. Buffered to the review store ONLY. Never
#            written to a policy, agent or gate file. Promoted to lessons.md at
#            session close, and only then.
#   Class C  user-only action -- the remedy is outside agent authority (hand
#            off, approve a widening, supply a credential, authorize a
#            destructive step, reduce concurrency, accept a budget rollover).
#            Deduplicated, because an unactionable request repeated every turn
#            is noise that trains the reader to ignore it.
#
# CHURN CONTROL (all seven mechanical, none advisory)
#   1  Class A is unlimited and costs nothing -- it writes no artifact.
#   2  Class B needs in-session recurrence >= 2 before it is even a candidate.
#   3  At most 3 lessons promoted per session; the excess is DROPPED with a
#      recorded count, never persisted.
#   4  The review NEVER writes under `bubbles/` or `agents/` (and therefore
#      never `bubbles/workflows.yaml`). Enforced on the PHYSICAL path, so a
#      symlinked store cannot smuggle a write past the boundary. Same
#      proposal-first rule `retro-framework-health.sh` already honours.
#   5  A pattern named in `skill-proposals-dismissed.md` is suppressed
#      PERMANENTLY. A reviewer who said no once is not asked again.
#   6  A review that finds nothing writes exactly `netEffect: no-adjustment`.
#      Empty is VALID and expected.
#   7  The review is itself budgeted: at most 12 reviews per session, so it
#      cannot become the cost it measures. `--close` is always allowed.
#
# THE ONE REFUSAL THAT MATTERS. A review emitted while a repeated failure
# signature or a resultless dispatch was OBSERVED, carrying no Class A, B or C
# entry and promoting nothing, is REFUSED. "I looked and everything was fine"
# is exactly the record a stuck session would write, and it is the one shape
# that turns this loop back into a scoreboard.
#
# FEEDING THE EXISTING LOOP, NOT A SECOND ONE. Promotion appends a plain
# bullet to `.specify/memory/lessons.md` -- the corpus `skill-evolution.sh`
# already normalises and clusters -- with the provenance carried in a
# `bubbles-lesson-meta` HTML comment, which that normaliser already strips
# before clustering. No second lessons mechanism, no second proposal file, no
# second threshold. Note the consequence: `skill-evolution.sh` ignores a
# normalised lesson under 20 characters, so a promoted pattern should be a
# descriptive slug rather than a bare word.
#
# DEFAULT OFF, per repo. With no `sessionReview:` block, no config file, or an
# explicit `adapter: none`, every subcommand is a clean no-op that writes ZERO
# records and creates no `.specify` directory. Same config shape as
# `dispatchReceipts:` and `testLeafReceipts:`.
#
# Store: append-only JSONL at <repo-root>/.specify/runtime/session-review.jsonl,
# schemaVersion `session-review/v1`, one object per REVIEW. The store IS the
# session ledger; past lines are never rewritten.
#
# Usage:
#   session-review.sh check [observations...] [--repo-root PATH]
#   session-review.sh emit  [observations...] [--trigger NAME]
#                           [--class-a <change>[=<resolvedPattern>]]...
#                           [--class-b <pattern>]...
#                           [--class-c <action>=<metric>]...
#                           [--class-c-reason <action>=<text>]...
#                           [--contradicted <change>]...
#                           [--close] [--promote <pattern>]...
#                           [--repo-root PATH]
#   session-review.sh show  [--names-only] [--active-adjustments] [--repo-root PATH]
#
# Observations (all default 0):
#   --turns N  --elapsed-minutes N  --retained-bytes N
#   --repeat-signature <signature>=<count>  --dispatch-no-result N  --budget-pct N
#
# Project config (project-owned, never framework-managed):
#
#   sessionReview:
#     adapter: none | jsonl
#
# Exit codes (emit):
#   0  the review was recorded, or the adapter is `none` so nothing is reviewed
#   2  usage error, or a refusal: an empty review over a diagnostic
#      observation, a promotion below the recurrence threshold, a suppressed or
#      already-resolved promotion, a write outside the boundary, or the review
#      budget exhausted
#
# Exit codes (check/show): 0 ok - 1 configured-but-broken adapter - 2 usage
#
# There is no --skip, --force, --ignore or --assume flag.

set -euo pipefail

NAME="session-review"
SCHEMA_VERSION="session-review/v1"
STORE_REL=".specify/runtime/session-review.jsonl"
LESSONS_REL=".specify/memory/lessons.md"
DISMISSED_REL=".specify/memory/skill-proposals-dismissed.md"

# Trigger defaults (IMP-048 SCOPE-1). Signature and dispatch triggers carry the
# diagnostic value; turns and elapsed time exist so a quiet-but-stuck session
# still surfaces. Precedence is that same ordering: the most diagnostic trigger
# that fires is the one recorded, because "first to fire wins" is only useful
# if the winner is the one worth reading.
TRIGGER_TURNS=8
TRIGGER_ELAPSED_MINUTES=45
TRIGGER_RETAINED_BYTES=153600 # 150 KB
TRIGGER_REPEAT_FAILURES=2
TRIGGER_DISPATCH_NO_RESULT=1
BUDGET_BANDS="90 70 50" # highest first; each band fires at most once

CLASS_B_RECURRENCE=2
MAX_PROMOTIONS_PER_SESSION=3
MAX_REVIEWS_PER_SESSION=12
# Class C re-emits only when the underlying metric worsens by 25%. Integer
# math, so no shell float dependency: emit when metric * 100 >= previous * 125.
CLASS_C_WORSEN_PCT=125

usage() {
  cat <<'EOF'
Usage:
  session-review.sh check [observations...] [--repo-root PATH]
  session-review.sh emit  [observations...] [--trigger NAME]
                          [--class-a <change>[=<resolvedPattern>]]...
                          [--class-b <pattern>]...
                          [--class-c <action>=<metric>]...
                          [--class-c-reason <action>=<text>]...
                          [--contradicted <change>]...
                          [--close] [--promote <pattern>]...
                          [--repo-root PATH]
  session-review.sh show  [--names-only] [--active-adjustments] [--repo-root PATH]

Observations (all default 0):
  --turns N  --elapsed-minutes N  --retained-bytes N
  --repeat-signature <signature>=<count>  --dispatch-no-result N  --budget-pct N

Triggers (closed set, evaluated at phase boundaries, first to fire wins):
  repeat-failure-signature   a signature seen >= 2 times
  dispatch-no-result         a dispatch returned no envelope
  budget-threshold           budget crossed 50% / 70% / 90%, once each
  retained-bytes             >= 150 KB retained since the last review
  turns                      >= 8 turns since the last review
  elapsed-minutes            >= 45 minutes since the last review
  manual / session-close     emitted without a firing trigger

Output classes: A adjust now (no artifact) - B candidate (buffered) -
C user-only action (deduplicated).

Project config (default OFF):

  sessionReview:
    adapter: none | jsonl
EOF
}

fail() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  exit "${2:-1}"
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

# Identifiers that are later matched back out of the JSONL store are restricted
# to a charset that cannot contain a quote or a bracket. That is what makes the
# store readable with awk instead of a JSON parser this framework does not
# require. Free prose (a Class C reason) is escaped and never matched.
valid_token() {
  case "$1" in
    '' | *[!A-Za-z0-9._:-]*) return 1 ;;
    *) return 0 ;;
  esac
}

valid_count() {
  case "$1" in
    '' | *[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

# Membership over a newline-delimited list. Written once because the same test
# is needed for de-duplicating Class B patterns and reading it inline twice is
# how two spellings of one rule start to drift.
list_contains() {
  local list="$1" needle="$2" item
  while IFS= read -r item; do
    if [ "$item" = "$needle" ]; then
      return 0
    fi
  done <<< "$list"
  return 1
}

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

valid_trigger() {
  case "$1" in
    repeat-failure-signature | dispatch-no-result | budget-threshold | retained-bytes | turns | elapsed-minutes | manual | session-close) return 0 ;;
    *) return 1 ;;
  esac
}

resolve_adapter() {
  local repo_root="$1" config_file='' adapter=''
  if [ -f "$repo_root/.github/bubbles-project.yaml" ]; then
    config_file="$repo_root/.github/bubbles-project.yaml"
  elif [ -f "$repo_root/bubbles-project.yaml" ]; then
    config_file="$repo_root/bubbles-project.yaml"
  fi

  if [ -n "$config_file" ]; then
    adapter="$(awk '
      /^[[:space:]]*#/ { next }
      /^sessionReview:[[:space:]]*$/ { inblock = 1; next }
      inblock && /^[^[:space:]]/ { inblock = 0 }
      inblock && $1 == "adapter:" {
        value = $2
        gsub(/["\047]/, "", value)
        print value
        exit
      }
    ' "$config_file" 2> /dev/null || true)"
  fi

  [ -n "$adapter" ] || adapter='none'

  case "$adapter" in
    *[!a-z0-9-]* | '' | -*)
      fail "invalid sessionReview.adapter '$adapter' (expected none or jsonl)"
      ;;
  esac

  # A configured-but-unknown adapter fails LOUD rather than degrading to `none`.
  # A typo that silently produced "not reviewing" would be indistinguishable
  # from a deliberate opt-out, and the session would run unreviewed on the
  # strength of a misspelling.
  case "$adapter" in
    none | jsonl) ;;
    *) fail "unknown sessionReview.adapter '$adapter' (expected none or jsonl)" ;;
  esac

  printf '%s' "$adapter"
}

# CHURN RULE 4, enforced rather than documented. The physical path is resolved
# before anything is created, so a `.specify` symlinked into `agents/` -- the
# realistic way this boundary gets crossed by accident -- is refused instead of
# followed.
physical_target() {
  local target="$1" probe tail='' base
  base="$(basename "$target")"
  probe="$(dirname "$target")"
  while [ ! -d "$probe" ] && [ "$probe" != "/" ] && [ "$probe" != "." ]; do
    tail="$(basename "$probe")${tail:+/$tail}"
    probe="$(dirname "$probe")"
  done
  [ -d "$probe" ] || fail "cannot resolve a physical path for $target" 2
  local phys
  phys="$(cd "$probe" && pwd -P)"
  [ -z "$tail" ] || phys="$phys/$tail"
  printf '%s/%s' "$phys" "$base"
}

guard_write_path() {
  local repo_root="$1" target="$2" phys root_phys
  root_phys="$(cd "$repo_root" && pwd -P)"
  phys="$(physical_target "$target")"
  case "$phys" in
    "$root_phys"/bubbles/* | "$root_phys"/agents/*)
      fail "refusing to write $phys: the review never mutates bubbles/ or agents/ (churn rule 4)" 2
      ;;
  esac
}

# --- store scanning --------------------------------------------------------
#
# One line per review, self-generated, with the field order below fixed. Every
# scan below relies on that: identifiers cannot contain a quote or a bracket,
# and the only free-prose field (a Class C reason) is placed LAST inside its
# object so an escaped quote can never break a match.

store_records() {
  local store="$1"
  if [ -f "$store" ]; then
    awk 'END { print NR + 0 }' "$store"
  else
    printf '0'
  fi
}

# How many REVIEWS carried this pattern in their classB list.
classb_prior_reviews() {
  local store="$1" pattern="$2"
  [ -f "$store" ] || {
    printf '0'
    return 0
  }
  awk -v P="$pattern" '
    {
      if (!match($0, /"classB":\[[^]]*\]/)) next
      region = substr($0, RSTART, RLENGTH)
      seen = 0
      while (match(region, /"pattern":"[^"]*"/)) {
        v = substr(region, RSTART + 11, RLENGTH - 12)
        if (v == P) seen = 1
        region = substr(region, RSTART + RLENGTH)
      }
      if (seen) n++
    }
    END { print n + 0 }
  ' "$store"
}

# A Class A adjustment that names this pattern has already resolved it, so it
# is no longer a candidate: promoting it would record a lesson the session had
# already learned and acted on.
classa_resolved() {
  local store="$1" pattern="$2"
  [ -f "$store" ] || return 1
  awk -v P="$pattern" '
    {
      s = $0
      while (match(s, /"resolves":"[^"]*"/)) {
        if (substr(s, RSTART + 12, RLENGTH - 13) == P) { found = 1 }
        s = substr(s, RSTART + RLENGTH)
      }
    }
    END { exit(found ? 0 : 1) }
  ' "$store"
}

already_promoted() {
  local store="$1" pattern="$2"
  [ -f "$store" ] || return 1
  awk -v P="$pattern" '
    {
      if (!match($0, /"promotedPatterns":\[[^]]*\]/)) next
      region = substr($0, RSTART, RLENGTH)
      while (match(region, /"[^"]*"/)) {
        v = substr(region, RSTART + 1, RLENGTH - 2)
        if (v == P) found = 1
        region = substr(region, RSTART + RLENGTH)
      }
    }
    END { exit(found ? 0 : 1) }
  ' "$store"
}

promoted_total() {
  local store="$1"
  [ -f "$store" ] || {
    printf '0'
    return 0
  }
  awk '
    {
      if (!match($0, /"promotedCount":[0-9]+/)) next
      n += substr($0, RSTART + 16, RLENGTH - 16) + 0
    }
    END { print n + 0 }
  ' "$store"
}

# The metric carried by the most recent EMITTED Class C for this action, or -1
# when it has never been emitted.
classc_last_metric() {
  local store="$1" action="$2"
  [ -f "$store" ] || {
    printf -- '-1'
    return 0
  }
  awk -v A="$action" '
    {
      s = $0
      while (match(s, /\{"action":"[^"]*","metric":[0-9]+,"emitted":(true|false)/)) {
        obj = substr(s, RSTART, RLENGTH)
        s = substr(s, RSTART + RLENGTH)
        if (!match(obj, /"action":"[^"]*"/)) continue
        if (substr(obj, RSTART + 10, RLENGTH - 11) != A) continue
        if (!match(obj, /"emitted":true/)) continue
        if (match(obj, /"metric":[0-9]+/)) last = substr(obj, RSTART + 9, RLENGTH - 9)
      }
    }
    END { print (last == "" ? -1 : last) }
  ' "$store"
}

contradiction_prior_count() {
  local store="$1" change="$2"
  [ -f "$store" ] || {
    printf '0'
    return 0
  }
  awk -v C="$change" '
    {
      if (!match($0, /"contradicted":\[[^]]*\]/)) next
      region = substr($0, RSTART, RLENGTH)
      while (match(region, /"[^"]*"/)) {
        v = substr(region, RSTART + 1, RLENGTH - 2)
        if (v == C) n++
        region = substr(region, RSTART + RLENGTH)
      }
    }
    END { print n + 0 }
  ' "$store"
}

budget_band_fired() {
  local store="$1" band="$2"
  [ -f "$store" ] || return 1
  awk -v B="$band" '
    {
      if (!match($0, /"budgetBand":[0-9]+/)) next
      if (substr($0, RSTART + 13, RLENGTH - 13) + 0 == B + 0) found = 1
    }
    END { exit(found ? 0 : 1) }
  ' "$store"
}

# Class A adjustments, most recent first, deduplicated. This is what a
# dispatch packet carries as `activeAdjustments[]` so a subagent inherits the
# correction instead of rediscovering it.
active_adjustments() {
  local store="$1"
  [ -f "$store" ] || return 0
  awk '
    {
      if (!match($0, /"classA":\[[^]]*\]/)) next
      region = substr($0, RSTART, RLENGTH)
      while (match(region, /"change":"[^"]*"/)) {
        lines[++n] = substr(region, RSTART + 10, RLENGTH - 11)
        region = substr(region, RSTART + RLENGTH)
      }
    }
    END {
      for (i = n; i >= 1; i--) {
        if (lines[i] in seen) continue
        seen[lines[i]] = 1
        print lines[i]
      }
    }
  ' "$store"
}

latest_field() {
  local store="$1" key="$2"
  [ -f "$store" ] || return 0
  awk -v K="$key" '
    { if (match($0, "\"" K "\":\"[^\"]*\"")) v = substr($0, RSTART + length(K) + 4, RLENGTH - length(K) - 5) }
    END { if (v != "") print v }
  ' "$store"
}

# CHURN RULE 5. A pattern a reviewer already dismissed is suppressed
# permanently, matched case-insensitively against the dismissal log
# `skill-evolution.sh dismiss` writes.
pattern_dismissed() {
  local repo_root="$1" pattern="$2"
  local file="$repo_root/$DISMISSED_REL"
  [ -f "$file" ] || return 1
  awk -v P="$pattern" '
    BEGIN { p = tolower(P) }
    index(tolower($0), p) > 0 { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$file"
}

# --- trigger evaluation ----------------------------------------------------

# Prints "<trigger> <band>" where band is empty unless the trigger is
# budget-threshold. Prints "none " when nothing fires.
evaluate_trigger() {
  local store="$1" turns="$2" elapsed="$3" bytes="$4" sig_max="$5" no_result="$6" budget_pct="$7"

  if [ "$sig_max" -ge "$TRIGGER_REPEAT_FAILURES" ]; then
    printf 'repeat-failure-signature '
    return 0
  fi
  if [ "$no_result" -ge "$TRIGGER_DISPATCH_NO_RESULT" ]; then
    printf 'dispatch-no-result '
    return 0
  fi
  local band
  for band in $BUDGET_BANDS; do
    if [ "$budget_pct" -ge "$band" ] && ! budget_band_fired "$store" "$band"; then
      printf 'budget-threshold %s' "$band"
      return 0
    fi
  done
  if [ "$bytes" -ge "$TRIGGER_RETAINED_BYTES" ]; then
    printf 'retained-bytes '
    return 0
  fi
  if [ "$turns" -ge "$TRIGGER_TURNS" ]; then
    printf 'turns '
    return 0
  fi
  if [ "$elapsed" -ge "$TRIGGER_ELAPSED_MINUTES" ]; then
    printf 'elapsed-minutes '
    return 0
  fi
  printf 'none '
}

# --- shared observation parsing --------------------------------------------

OBS_TURNS=0
OBS_ELAPSED=0
OBS_BYTES=0
OBS_NO_RESULT=0
OBS_BUDGET=0
OBS_SIGNATURES=''
OBS_SIG_MAX=0

reset_observations() {
  OBS_TURNS=0
  OBS_ELAPSED=0
  OBS_BYTES=0
  OBS_NO_RESULT=0
  OBS_BUDGET=0
  OBS_SIGNATURES=''
  OBS_SIG_MAX=0
}

# Returns 0 when the argument was an observation flag and consumed 2 words.
# Callers `shift 2` on success. Keeping this in one place is what stops `check`
# and `emit` from ever disagreeing about what an observation means.
take_observation() {
  local flag="$1" value="${2:-}"
  case "$flag" in
    --turns)
      [ -n "$value" ] || die_usage "--turns requires a value"
      valid_count "$value" || die_usage "--turns expects a non-negative integer"
      OBS_TURNS="$value"
      ;;
    --elapsed-minutes)
      [ -n "$value" ] || die_usage "--elapsed-minutes requires a value"
      valid_count "$value" || die_usage "--elapsed-minutes expects a non-negative integer"
      OBS_ELAPSED="$value"
      ;;
    --retained-bytes)
      [ -n "$value" ] || die_usage "--retained-bytes requires a value"
      valid_count "$value" || die_usage "--retained-bytes expects a non-negative integer"
      OBS_BYTES="$value"
      ;;
    --dispatch-no-result)
      [ -n "$value" ] || die_usage "--dispatch-no-result requires a value"
      valid_count "$value" || die_usage "--dispatch-no-result expects a non-negative integer"
      OBS_NO_RESULT="$value"
      ;;
    --budget-pct)
      [ -n "$value" ] || die_usage "--budget-pct requires a value"
      valid_count "$value" || die_usage "--budget-pct expects a non-negative integer"
      OBS_BUDGET="$value"
      ;;
    --repeat-signature)
      [ -n "$value" ] || die_usage "--repeat-signature requires <signature>=<count>"
      case "$value" in
        *=*) ;;
        *) die_usage "--repeat-signature expects <signature>=<count>, got '$value'" ;;
      esac
      local sig="${value%%=*}" cnt="${value#*=}"
      valid_token "$sig" || die_usage "invalid signature '$sig' (allowed: A-Z a-z 0-9 . _ : -)"
      valid_count "$cnt" || die_usage "--repeat-signature count must be a non-negative integer"
      OBS_SIGNATURES="${OBS_SIGNATURES}${OBS_SIGNATURES:+$'\n'}$sig"
      if [ "$cnt" -gt "$OBS_SIG_MAX" ]; then
        OBS_SIG_MAX="$cnt"
      fi
      ;;
    *) return 1 ;;
  esac
  return 0
}

# --- subcommands -----------------------------------------------------------

cmd_check() {
  local repo_root="$PWD"
  reset_observations
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        continue
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --assume*)
        printf '%s: "%s" does not exist. A review is due or it is not; it is never waived.\n' "$NAME" "$1" >&2
        exit 2
        ;;
    esac
    if take_observation "$1" "${2:-}"; then
      shift 2
      continue
    fi
    die_usage "unknown option: $1"
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter store
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  if [ "$adapter" = "none" ]; then
    printf 'triggered=false\n'
    printf 'trigger=none\n'
    return 0
  fi

  store="$repo_root/$STORE_REL"
  local verdict trigger band
  verdict="$(evaluate_trigger "$store" "$OBS_TURNS" "$OBS_ELAPSED" "$OBS_BYTES" "$OBS_SIG_MAX" "$OBS_NO_RESULT" "$OBS_BUDGET")"
  trigger="${verdict%% *}"
  band="${verdict#* }"

  printf 'store=%s\n' "$store"
  if [ "$trigger" = "none" ]; then
    printf 'triggered=false\n'
  else
    printf 'triggered=true\n'
  fi
  printf 'trigger=%s\n' "$trigger"
  printf 'budgetBand=%s\n' "${band:-none}"
  printf 'threshold.turns=%s\n' "$TRIGGER_TURNS"
  printf 'threshold.elapsedMinutes=%s\n' "$TRIGGER_ELAPSED_MINUTES"
  printf 'threshold.retainedBytes=%s\n' "$TRIGGER_RETAINED_BYTES"
  printf 'threshold.repeatFailures=%s\n' "$TRIGGER_REPEAT_FAILURES"
  printf 'threshold.dispatchNoResult=%s\n' "$TRIGGER_DISPATCH_NO_RESULT"
  printf 'observed.turns=%s\n' "$OBS_TURNS"
  printf 'observed.elapsedMinutes=%s\n' "$OBS_ELAPSED"
  printf 'observed.retainedBytes=%s\n' "$OBS_BYTES"
  printf 'observed.repeatFailureMax=%s\n' "$OBS_SIG_MAX"
  printf 'observed.dispatchNoResult=%s\n' "$OBS_NO_RESULT"
  printf 'observed.budgetPct=%s\n' "$OBS_BUDGET"
  local used
  used="$(store_records "$store")"
  printf 'reviewBudget=%s/%s\n' "$used" "$MAX_REVIEWS_PER_SESSION"
  return 0
}

cmd_emit() {
  local repo_root="$PWD" trigger_arg='' close=0
  local class_a='' class_b='' class_c='' class_c_reasons='' contradicted='' promote_req=''
  reset_observations

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        continue
        ;;
      --trigger)
        [ "$#" -ge 2 ] || die_usage "--trigger requires a value"
        trigger_arg="$2"
        shift 2
        continue
        ;;
      --class-a)
        [ "$#" -ge 2 ] || die_usage "--class-a requires a value"
        class_a="${class_a}${class_a:+$'\n'}$2"
        shift 2
        continue
        ;;
      --class-b)
        [ "$#" -ge 2 ] || die_usage "--class-b requires a value"
        class_b="${class_b}${class_b:+$'\n'}$2"
        shift 2
        continue
        ;;
      --class-c)
        [ "$#" -ge 2 ] || die_usage "--class-c requires <action>=<metric>"
        class_c="${class_c}${class_c:+$'\n'}$2"
        shift 2
        continue
        ;;
      --class-c-reason)
        [ "$#" -ge 2 ] || die_usage "--class-c-reason requires <action>=<text>"
        class_c_reasons="${class_c_reasons}${class_c_reasons:+$'\n'}$2"
        shift 2
        continue
        ;;
      --contradicted)
        [ "$#" -ge 2 ] || die_usage "--contradicted requires a value"
        contradicted="${contradicted}${contradicted:+$'\n'}$2"
        shift 2
        continue
        ;;
      --promote)
        [ "$#" -ge 2 ] || die_usage "--promote requires a value"
        promote_req="${promote_req}${promote_req:+$'\n'}$2"
        shift 2
        continue
        ;;
      --close)
        close=1
        shift
        continue
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --assume*)
        printf '%s: "%s" does not exist. A review is recorded by reviewing, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
    esac
    if take_observation "$1" "${2:-}"; then
      shift 2
      continue
    fi
    die_usage "unknown option: $1"
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  if [ "$adapter" = "none" ]; then
    # DEFAULT OFF. Nothing reviewed, nothing recorded, exit 0. An unconfigured
    # repository behaves exactly as it does today.
    printf 'adapter=none\n'
    printf 'review=skipped\n'
    return 0
  fi

  local store lessons
  store="$repo_root/$STORE_REL"
  lessons="$repo_root/$LESSONS_REL"
  # CHURN RULE 4 is checked BEFORE anything is created, and for BOTH targets,
  # so a boundary violation can never leave a half-written session behind.
  guard_write_path "$repo_root" "$store"
  guard_write_path "$repo_root" "$lessons"

  local records
  records="$(store_records "$store")"

  # CHURN RULE 7. The review is budgeted like everything else it measures.
  # `--close` is exempt: refusing the closing review would strand the session's
  # promotions, which is the one artifact this loop exists to produce.
  if [ "$close" = "0" ] && [ "$records" -ge "$MAX_REVIEWS_PER_SESSION" ]; then
    fail "review budget exhausted ($records/$MAX_REVIEWS_PER_SESSION reviews this session); the review must not become the cost it measures" 2
  fi

  local trigger band verdict
  verdict="$(evaluate_trigger "$store" "$OBS_TURNS" "$OBS_ELAPSED" "$OBS_BYTES" "$OBS_SIG_MAX" "$OBS_NO_RESULT" "$OBS_BUDGET")"
  trigger="${verdict%% *}"
  band="${verdict#* }"
  if [ -n "$trigger_arg" ]; then
    valid_trigger "$trigger_arg" || die_usage "unknown --trigger '$trigger_arg'"
    if [ "$trigger_arg" != "budget-threshold" ]; then
      band=''
    fi
    trigger="$trigger_arg"
  elif [ "$close" = "1" ]; then
    trigger='session-close'
    band=''
  elif [ "$trigger" = "none" ]; then
    trigger='manual'
    band=''
  fi

  # --- Class A ------------------------------------------------------------
  local now a_json='' a_count=0 entry change resolves
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    change="${entry%%=*}"
    resolves=''
    case "$entry" in
      *=*) resolves="${entry#*=}" ;;
    esac
    valid_token "$change" || die_usage "invalid --class-a change '$change' (allowed: A-Z a-z 0-9 . _ : -)"
    if [ -n "$resolves" ]; then
      valid_token "$resolves" || die_usage "invalid --class-a resolved pattern '$resolves'"
      a_json="${a_json}${a_json:+,}{\"change\":\"$(json_escape "$change")\",\"appliedAt\":\"$now\",\"resolves\":\"$(json_escape "$resolves")\"}"
    else
      a_json="${a_json}${a_json:+,}{\"change\":\"$(json_escape "$change")\",\"appliedAt\":\"$now\",\"resolves\":null}"
    fi
    a_count=$((a_count + 1))
  done <<< "$class_a"

  # --- contradictions promote themselves into Class B ---------------------
  # An adjustment contradicted twice is no longer a correction the session can
  # keep applying; it is a pattern worth recording.
  local c_json='' c_count=0 auto_b='' prior
  while IFS= read -r change; do
    [ -n "$change" ] || continue
    valid_token "$change" || die_usage "invalid --contradicted change '$change'"
    c_json="${c_json}${c_json:+,}\"$(json_escape "$change")\""
    c_count=$((c_count + 1))
    prior="$(contradiction_prior_count "$store" "$change")"
    if [ $((prior + 1)) -ge 2 ]; then
      auto_b="${auto_b}${auto_b:+$'\n'}adjustment-contradicted:$change"
    fi
  done <<< "$contradicted"

  # --- Class B ------------------------------------------------------------
  local b_json='' b_count=0 b_report='' pattern occurrences suppressed b_seen=''
  local all_b
  all_b="$class_b"
  if [ -n "$auto_b" ]; then
    all_b="${all_b}${all_b:+$'\n'}$auto_b"
  fi
  while IFS= read -r pattern; do
    [ -n "$pattern" ] || continue
    valid_token "$pattern" || die_usage "invalid --class-b pattern '$pattern' (allowed: A-Z a-z 0-9 . _ : -)"
    if list_contains "$b_seen" "$pattern"; then
      continue
    fi
    b_seen="${b_seen}${b_seen:+$'\n'}$pattern"
    prior="$(classb_prior_reviews "$store" "$pattern")"
    occurrences=$((prior + 1))
    if pattern_dismissed "$repo_root" "$pattern"; then
      suppressed='true'
    else
      suppressed='false'
    fi
    # `promoted` is always false here. Promotion is a session-close decision,
    # never an in-flight one -- that ordering is what keeps churn rule 2 and
    # rule 3 enforceable at all.
    b_json="${b_json}${b_json:+,}{\"pattern\":\"$(json_escape "$pattern")\",\"occurrences\":$occurrences,\"promoted\":false,\"suppressed\":$suppressed}"
    b_count=$((b_count + 1))
    b_report="${b_report}${b_report:+$'\n'}classB.$pattern.occurrences=$occurrences"$'\n'"classB.$pattern.suppressed=$suppressed"
  done <<< "$all_b"

  # --- Class C ------------------------------------------------------------
  local cc_json='' cc_count=0 cc_emitted=0 action metric reason last_metric emitted
  local cc_report=''
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    case "$entry" in
      *=*) ;;
      *) die_usage "--class-c expects <action>=<metric>, got '$entry'" ;;
    esac
    action="${entry%%=*}"
    metric="${entry#*=}"
    valid_token "$action" || die_usage "invalid --class-c action '$action'"
    valid_count "$metric" || die_usage "--class-c metric must be a non-negative integer, got '$metric'"
    reason="$(awk -v A="$action" -F= '$1 == A { print substr($0, length(A) + 2); exit }' <<< "$class_c_reasons")"
    last_metric="$(classc_last_metric "$store" "$action")"
    if [ "$last_metric" -lt 0 ]; then
      emitted='true'
    elif [ $((metric * 100)) -ge $((last_metric * CLASS_C_WORSEN_PCT)) ]; then
      emitted='true'
    else
      # Deduplicated. An unactionable request repeated every turn is how a real
      # handoff recommendation becomes background noise.
      emitted='false'
    fi
    cc_json="${cc_json}${cc_json:+,}{\"action\":\"$(json_escape "$action")\",\"metric\":$metric,\"emitted\":$emitted,\"reason\":\"$(json_escape "$reason")\"}"
    cc_count=$((cc_count + 1))
    if [ "$emitted" = "true" ]; then
      cc_emitted=$((cc_emitted + 1))
    fi
    cc_report="${cc_report}${cc_report:+$'\n'}classC.$action.emitted=$emitted"
  done <<< "$class_c"

  # --- promotion (session close only) -------------------------------------
  local promoted='' promoted_count=0 dropped_count=0 p_json=''
  if [ "$close" = "1" ]; then
    local candidates=''
    if [ -n "$promote_req" ]; then
      # An EXPLICIT promotion request is checked against exactly the same rules
      # the automatic path uses. It can never ask for more than the rules
      # allow; it can only fail loud when it tries.
      while IFS= read -r pattern; do
        [ -n "$pattern" ] || continue
        valid_token "$pattern" || die_usage "invalid --promote pattern '$pattern'"
        occurrences="$(classb_prior_reviews "$store" "$pattern")"
        if [ "$occurrences" -lt "$CLASS_B_RECURRENCE" ]; then
          fail "refusing to promote '$pattern': observed $occurrences time(s), the candidate threshold is $CLASS_B_RECURRENCE (churn rule 2)" 2
        fi
        if pattern_dismissed "$repo_root" "$pattern"; then
          fail "refusing to promote '$pattern': it is recorded in $DISMISSED_REL and is suppressed permanently (churn rule 5)" 2
        fi
        if classa_resolved "$store" "$pattern"; then
          fail "refusing to promote '$pattern': a Class A adjustment already resolved it in this session" 2
        fi
        if already_promoted "$store" "$pattern"; then
          fail "refusing to promote '$pattern': it was already promoted in this session" 2
        fi
        candidates="${candidates}${candidates:+$'\n'}$occurrences $pattern"
      done <<< "$promote_req"
    else
      candidates="$(
        collect_candidates "$store" "$repo_root"
      )"
    fi

    local rank sorted
    sorted="$(LC_ALL=C sort -k1,1nr -k2,2 <<< "$candidates")"
    rank=0
    while IFS=' ' read -r occurrences pattern; do
      [ -n "$pattern" ] || continue
      rank=$((rank + 1))
      if [ "$rank" -gt "$MAX_PROMOTIONS_PER_SESSION" ]; then
        # CHURN RULE 3. The excess is DROPPED with a recorded count, never
        # persisted and never carried into the next session.
        dropped_count=$((dropped_count + 1))
        continue
      fi
      promoted="${promoted}${promoted:+$'\n'}$occurrences $pattern"
      p_json="${p_json}${p_json:+,}\"$(json_escape "$pattern")\""
      promoted_count=$((promoted_count + 1))
    done <<< "$sorted"

    if [ "$promoted_count" -gt 0 ]; then
      write_lessons "$lessons" "$now" "$promoted"
    fi
  fi

  # --- the one refusal that matters ---------------------------------------
  local diagnostic=0
  if [ "$OBS_SIG_MAX" -ge "$TRIGGER_REPEAT_FAILURES" ] || [ "$OBS_NO_RESULT" -ge "$TRIGGER_DISPATCH_NO_RESULT" ]; then
    diagnostic=1
  fi
  if [ "$diagnostic" = "1" ] &&
    [ "$a_count" = "0" ] && [ "$b_count" = "0" ] && [ "$cc_count" = "0" ] && [ "$promoted_count" = "0" ]; then
    fail "refusing an empty review: a repeated failure signature or a resultless dispatch was observed, so 'no-adjustment' is a claim this review has not earned" 2
  fi

  # --- net effect ---------------------------------------------------------
  local net_effect
  if [ "$promoted_count" -gt 0 ]; then
    net_effect='promoted'
  elif [ "$a_count" -gt 0 ]; then
    net_effect='adjusted'
  elif [ "$b_count" -gt 0 ] || [ "$cc_emitted" -gt 0 ]; then
    net_effect='escalated'
  else
    # CHURN RULE 6. Empty is valid and expected.
    net_effect='no-adjustment'
  fi

  local sig_json='' sig
  while IFS= read -r sig; do
    [ -n "$sig" ] || continue
    sig_json="${sig_json}${sig_json:+,}\"$(json_escape "$sig")\""
  done <<< "$OBS_SIGNATURES"

  local runtime_dir review_id band_json
  runtime_dir="$(dirname "$store")"
  mkdir -p "$runtime_dir" || fail "cannot create the review store directory $runtime_dir"
  review_id="$(printf 'rev-%03d' $((records + 1)))"
  if [ -n "$band" ]; then
    band_json="$band"
  else
    band_json='null'
  fi

  printf '{"schemaVersion":"%s","reviewId":"%s","at":"%s","trigger":"%s","budgetBand":%s,"observed":{"turnsSinceLastReview":%s,"elapsedMinutesSinceLastReview":%s,"retainedBytesSinceLastReview":%s,"repeatedFailureSignatures":[%s],"dispatchNoResultCount":%s,"budgetConsumedPct":%s},"classA":[%s],"classB":[%s],"classC":[%s],"contradicted":[%s],"promotedPatterns":[%s],"promotedCount":%s,"droppedCount":%s,"netEffect":"%s"}\n' \
    "$SCHEMA_VERSION" "$review_id" "$now" "$trigger" "$band_json" \
    "$OBS_TURNS" "$OBS_ELAPSED" "$OBS_BYTES" "$sig_json" "$OBS_NO_RESULT" "$OBS_BUDGET" \
    "$a_json" "$b_json" "$cc_json" "$c_json" "$p_json" \
    "$promoted_count" "$dropped_count" "$net_effect" \
    >> "$store" || fail "cannot append to the review store $store"

  printf 'adapter=jsonl\n'
  printf 'review=recorded\n'
  printf 'reviewId=%s\n' "$review_id"
  printf 'trigger=%s\n' "$trigger"
  printf 'budgetBand=%s\n' "${band:-none}"
  printf 'classA=%s\n' "$a_count"
  printf 'classB=%s\n' "$b_count"
  printf 'classC=%s\n' "$cc_count"
  printf 'classCEmitted=%s\n' "$cc_emitted"
  printf 'contradicted=%s\n' "$c_count"
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    printf '%s\n' "$entry"
  done <<< "$b_report"
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    printf '%s\n' "$entry"
  done <<< "$cc_report"
  while IFS=' ' read -r occurrences pattern; do
    [ -n "$pattern" ] || continue
    printf 'promoted.%s=%s\n' "$pattern" "$occurrences"
  done <<< "$promoted"
  printf 'promotedCount=%s\n' "$promoted_count"
  printf 'droppedCount=%s\n' "$dropped_count"
  printf 'netEffect=%s\n' "$net_effect"
  printf 'records=%s\n' $((records + 1))
  printf 'reviewBudget=%s/%s\n' $((records + 1)) "$MAX_REVIEWS_PER_SESSION"
  printf 'store=%s\n' "$store"
  return 0
}

# Every Class B pattern in the store that is a legitimate candidate: recurrence
# >= 2, not dismissed, not already resolved by a Class A adjustment, not
# already promoted. Prints "<occurrences> <pattern>" per line.
collect_candidates() {
  local store="$1" repo_root="$2"
  [ -f "$store" ] || return 0
  local pattern occurrences
  while IFS= read -r pattern; do
    [ -n "$pattern" ] || continue
    occurrences="$(classb_prior_reviews "$store" "$pattern")"
    [ "$occurrences" -ge "$CLASS_B_RECURRENCE" ] || continue
    ! pattern_dismissed "$repo_root" "$pattern" || continue
    ! classa_resolved "$store" "$pattern" || continue
    ! already_promoted "$store" "$pattern" || continue
    printf '%s %s\n' "$occurrences" "$pattern"
  done <<< "$(distinct_patterns "$store")"
}

distinct_patterns() {
  local store="$1"
  [ -f "$store" ] || return 0
  awk '
    {
      if (!match($0, /"classB":\[[^]]*\]/)) next
      region = substr($0, RSTART, RLENGTH)
      while (match(region, /"pattern":"[^"]*"/)) {
        v = substr(region, RSTART + 11, RLENGTH - 12)
        if (!(v in seen)) { seen[v] = 1; order[++n] = v }
        region = substr(region, RSTART + RLENGTH)
      }
    }
    END { for (i = 1; i <= n; i++) print order[i] }
  ' "$store"
}

# Append promoted patterns to the SAME lessons corpus `skill-evolution.sh`
# already clusters. The provenance rides in a `bubbles-lesson-meta` comment,
# which that normaliser strips before tokenising, so the clustered text is the
# pattern itself and nothing else -- shared boilerplate in the lesson line
# would over-merge unrelated patterns at the 0.6 similarity threshold.
write_lessons() {
  local lessons="$1" now="$2" promoted="$3" dir occurrences pattern
  dir="$(dirname "$lessons")"
  mkdir -p "$dir" || fail "cannot create the lessons directory $dir"
  if [ ! -f "$lessons" ]; then
    printf '# Lessons\n\n' > "$lessons" || fail "cannot create $lessons"
  fi
  while IFS=' ' read -r occurrences pattern; do
    [ -n "$pattern" ] || continue
    printf -- '- %s <!-- bubbles-lesson-meta: source=session-review occurrences=%s at=%s -->\n' \
      "$pattern" "$occurrences" "$now" >> "$lessons" || fail "cannot append to $lessons"
  done <<< "$promoted"
}

cmd_show() {
  local repo_root="$PWD" names_only=0 adjustments_only=0
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --names-only)
        names_only=1
        shift
        ;;
      --active-adjustments)
        adjustments_only=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter store
  adapter="$(resolve_adapter "$repo_root")"
  if [ "$adjustments_only" = "0" ]; then
    printf 'adapter=%s\n' "$adapter"
  fi
  if [ "$names_only" = "1" ]; then
    return 0
  fi
  store="$repo_root/$STORE_REL"
  if [ "$adapter" = "none" ]; then
    if [ "$adjustments_only" = "0" ]; then
      printf 'records=0\n'
      printf 'activeAdjustments=0\n'
    fi
    return 0
  fi

  local adjustments count=0 line
  adjustments="$(active_adjustments "$store")"
  if [ "$adjustments_only" = "1" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      count=$((count + 1))
      printf 'activeAdjustment.%s=%s\n' "$count" "$line"
    done <<< "$adjustments"
    return 0
  fi

  local records
  records="$(store_records "$store")"
  printf 'schemaVersion=%s\n' "$SCHEMA_VERSION"
  printf 'store=%s\n' "$store"
  printf 'records=%s\n' "$records"
  printf 'reviewBudget=%s/%s\n' "$records" "$MAX_REVIEWS_PER_SESSION"
  printf 'latest.reviewId=%s\n' "$(latest_field "$store" reviewId)"
  printf 'latest.trigger=%s\n' "$(latest_field "$store" trigger)"
  printf 'latest.netEffect=%s\n' "$(latest_field "$store" netEffect)"
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    count=$((count + 1))
    printf 'activeAdjustment.%s=%s\n' "$count" "$line"
  done <<< "$adjustments"
  printf 'activeAdjustments=%s\n' "$count"
  printf 'promotedTotal=%s\n' "$(promoted_total "$store")"
  printf 'promotionLimit=%s\n' "$MAX_PROMOTIONS_PER_SESSION"
  # Review records compact under G083 exactly like envelopes: the two most
  # recent stay raw, everything older is a compaction candidate.
  local raw=2
  if [ "$records" -lt 2 ]; then
    raw="$records"
  fi
  printf 'rawRetained=%s\n' "$raw"
  printf 'compactable=%s\n' $((records - raw))
  return 0
}

main() {
  local sub="${1:-}"
  if [ "$#" -gt 0 ]; then
    shift
  fi
  case "$sub" in
    check) cmd_check "$@" ;;
    emit) cmd_emit "$@" ;;
    show) cmd_show "$@" ;;
    -h | --help)
      usage
      exit 0
      ;;
    '')
      usage >&2
      exit 2
      ;;
    *) die_usage "unknown subcommand: $sub" ;;
  esac
}

main "$@"
