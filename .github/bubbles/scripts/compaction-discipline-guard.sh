#!/usr/bin/env bash
set -euo pipefail

# compaction-discipline-guard.sh
#
# Gate G083 — context_compaction_discipline_gate.
#
# Mechanically enforces the in-loop compaction rule documented in
# `agents/bubbles_shared/operating-baseline.md` → "Context Compaction
# Discipline (Orchestrator Agents)":
#
#   Orchestrator agents MUST compact accumulated RESULT-ENVELOPEs after
#   every 3 envelopes OR > 8 KB of raw payload (whichever comes first),
#   EXCLUDING the latest 2 raw envelopes which remain in working memory
#   until the next compaction round.
#
# This guard reads `.specify/memory/bubbles.session.json` and inspects
# `envelopesReceived[]` (additively appended by
# `bubbles/scripts/context-compactor.sh` when it compacts an envelope,
# and by orchestrator agents when they record receipt of a raw
# RESULT-ENVELOPE). Each entry SHOULD carry:
#
#   {
#     "specDir":         "<path>",
#     "agent":           "<bubbles.workflow|bubbles.goal|...>",
#     "receivedAt":      "<RFC3339>",
#     "rawSizeBytes":    <int>,
#     "incomingMessage": "<string|null>",
#     "compactedAt":     "<RFC3339|null>",
#     "rawPointer":      "<path|null>"
#   }
#
# The 3-envelope OR 8-KB rule means: among entries that are eligible
# for compaction (i.e., NOT one of the latest 2 raw entries kept in
# working memory by policy), more than 0 must NOT have
# `compactedAt: null`, AND the cumulative raw size of uncompacted
# eligible entries MUST NOT exceed 8 KB.
#
# Exit codes:
#   0  compaction respected (or insufficient envelopes to trigger)
#   1  compaction skipped — orchestrator violated the discipline;
#      stderr names Gate G083 and the specific threshold breached
#      (envelope count threshold OR size threshold)
#   2  malformed / missing inputs (workflows.yaml lookup is OPTIONAL
#      here — the thresholds are framework constants, not configurable
#      via workflows.yaml), missing/required arguments, or unparseable
#      session.json — diagnostic on stderr
#
# Usage:
#   bash bubbles/scripts/compaction-discipline-guard.sh <specDir> [--quiet]
#
# Inputs:
#   <specDir>   Path to the spec directory (e.g.
#               specs/900-convergence-fixture). Used to filter
#               envelopesReceived[] entries.
#   --quiet     Suppress informational stdout on success.
#
# Dependencies:
#   - jq      (hard dependency)
#
# Thresholds (framework constants, NOT operator-tunable):
#   COUNT_THRESHOLD = 3       — > 3 uncompacted eligible envelopes ⇒ fail
#   SIZE_THRESHOLD  = 8192    — > 8192 bytes of uncompacted eligible raw ⇒ fail
#   KEEP_RAW_LATEST = 2       — the latest 2 raw entries are ALWAYS kept
#                               raw per policy and never count against
#                               the thresholds
#
# Reference:
#   agents/bubbles_shared/operating-baseline.md
#   docs/Framework_Convergence_Health.md

COUNT_THRESHOLD=3
SIZE_THRESHOLD=8192
KEEP_RAW_LATEST=2

QUIET="false"
SPEC_DIR=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/compaction-discipline-guard.sh <specDir> [--quiet]

Required:
  <specDir>   Spec directory whose orchestrator compaction discipline is
              inspected (e.g. specs/900-convergence-fixture).

Optional:
  --quiet     Suppress informational stdout; the final PASS or VIOLATION
              line is still emitted (stdout on pass, stderr on fail).
  -h, --help  Print this usage and exit.

Exit codes:
  0 = compaction respected (or insufficient envelopes to trigger)
  1 = compaction skipped (Gate G083 violation — count or size threshold)
  2 = malformed inputs or missing arguments
EOF
}

# --- Argument parsing ----------------------------------------------------

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --*)
      echo "compaction-discipline-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$SPEC_DIR" ]]; then
        SPEC_DIR="$1"
      else
        echo "compaction-discipline-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR" ]]; then
  echo "compaction-discipline-guard: <specDir> is required" >&2
  usage >&2
  exit 2
fi

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "compaction-discipline-guard: $*"
  fi
}

# --- jq dependency check -------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "compaction-discipline-guard: jq is required but not found in PATH" >&2
  exit 2
fi

# --- Repo root resolution ------------------------------------------------

resolve_repo_root() {
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  echo "compaction-discipline-guard: unable to resolve repo root (no .specify/memory found)" >&2
  echo "  Set BUBBLES_REPO_ROOT explicitly or run from inside a Bubbles repo." >&2
  exit 2
fi

# --- Locate session.json -------------------------------------------------

SESSION_FILE="$REPO_ROOT/.specify/memory/bubbles.session.json"
if [[ ! -f "$SESSION_FILE" ]]; then
  # No session file recorded yet — no envelopes to discipline.
  info "no $SESSION_FILE present; nothing to enforce"
  echo "PASS Gate G083 (context_compaction_discipline_gate) — no session.json, specDir=$SPEC_DIR"
  exit 0
fi

# --- Validate session.json is parseable JSON -----------------------------

if ! jq empty "$SESSION_FILE" >/dev/null 2>&1; then
  echo "compaction-discipline-guard: $SESSION_FILE is not valid JSON" >&2
  exit 2
fi

NORMALIZED_SPEC="${SPEC_DIR%/}"

# --- Project envelope entries for the requested spec ---------------------
#
# Filter envelopesReceived[] to entries whose specDir matches either the
# raw spec dir argument OR a trailing-path equivalent (so callers can
# pass absolute or repo-relative forms interchangeably). For each
# matching entry we project: orderingTimestamp, rawSize (preferring
# rawSizeBytes if present, else falling back to length(incomingMessage)),
# and compactedAt. Entries with malformed shapes are ignored
# defensively — the goal is to FAIL CLOSED on missing fields rather
# than silently miscount.
#
# Output is a JSON array of {ts, rawSize, compactedAt} objects sorted
# ascending by ts (so latest entries are at the tail of the array, which
# is exactly the slice the KEEP_RAW_LATEST policy needs to exclude from
# threshold checks).

ENV_FILTER='
  (.envelopesReceived // [])
  | map(select(
      (.specDir // "") == $specDir
      or ((.specDir // "") | endswith("/" + ($specDir | sub("^.*/"; ""))))
    ))
  | map({
      ts: (.receivedAt // .timestamp // ""),
      rawSize: (
        if (.rawSizeBytes // null) != null then (.rawSizeBytes // 0)
        elif (.incomingMessage // null) != null then ((.incomingMessage // "") | length)
        else 0
        end
      ),
      compactedAt: (.compactedAt // null)
    })
  | sort_by(.ts)
'

PROJECTED_JSON="$(
  jq -c --arg specDir "$NORMALIZED_SPEC" "$ENV_FILTER" "$SESSION_FILE" 2>/dev/null || true
)"

if [[ -z "$PROJECTED_JSON" ]] || ! echo "$PROJECTED_JSON" | jq empty >/dev/null 2>&1; then
  echo "compaction-discipline-guard: failed to project envelopesReceived[] from $SESSION_FILE" >&2
  exit 2
fi

TOTAL_FOR_SPEC="$(echo "$PROJECTED_JSON" | jq 'length')"

# --- Eligibility slice ---------------------------------------------------
#
# By policy, the latest KEEP_RAW_LATEST entries (most recent 2 raw
# envelopes) remain in working memory and are NOT counted against the
# thresholds. Everything older is "eligible" — those entries MUST have
# `compactedAt: non-null`.

ELIGIBLE_JSON="$(
  echo "$PROJECTED_JSON" \
    | jq --argjson keep "$KEEP_RAW_LATEST" '
        if length <= $keep then []
        else .[0 : (length - $keep)]
        end
      '
)"

ELIGIBLE_COUNT="$(echo "$ELIGIBLE_JSON" | jq 'length')"
UNCOMPACTED_COUNT="$(echo "$ELIGIBLE_JSON" | jq '[.[] | select(.compactedAt == null)] | length')"
UNCOMPACTED_BYTES="$(echo "$ELIGIBLE_JSON" | jq '[.[] | select(.compactedAt == null) | .rawSize] | add // 0')"

# Sanity: ensure integer.
if ! [[ "$UNCOMPACTED_COUNT" =~ ^[0-9]+$ ]] || ! [[ "$UNCOMPACTED_BYTES" =~ ^[0-9]+$ ]]; then
  echo "compaction-discipline-guard: malformed envelopesReceived[] computation: count=$UNCOMPACTED_COUNT bytes=$UNCOMPACTED_BYTES" >&2
  exit 2
fi

# --- Decision -----------------------------------------------------------

# Helper: emit a violation block to stderr.
emit_violation() {
  local kind="$1"
  local detail="$2"
  {
    echo "G083 context_compaction_discipline_gate violation ($kind)"
    echo "  specDir:                          $NORMALIZED_SPEC"
    echo "  session.json:                     $SESSION_FILE"
    echo "  total envelopes (this spec):      $TOTAL_FOR_SPEC"
    echo "  eligible-for-compaction count:    $ELIGIBLE_COUNT"
    echo "  uncompacted eligible count:       $UNCOMPACTED_COUNT"
    echo "  uncompacted eligible bytes:       $UNCOMPACTED_BYTES"
    echo "  count threshold (envelopes):      > $COUNT_THRESHOLD"
    echo "  size threshold (bytes):           > $SIZE_THRESHOLD"
    echo "  keep-raw-latest (policy):         $KEEP_RAW_LATEST"
    echo "  detail:                           $detail"
    echo "  remediation:                      orchestrator MUST invoke bubbles/scripts/context-compactor.sh on the offending eligible envelope(s) and additively set 'compactedAt' on each entry before continuing"
  } >&2
}

# Count breach: more than COUNT_THRESHOLD uncompacted entries in the eligible slice.
if [[ "$UNCOMPACTED_COUNT" -gt "$COUNT_THRESHOLD" ]]; then
  emit_violation "envelope count threshold" "uncompacted eligible envelopes ($UNCOMPACTED_COUNT) exceeds count threshold ($COUNT_THRESHOLD)"
  exit 1
fi

# Size breach: cumulative uncompacted eligible bytes exceed SIZE_THRESHOLD.
if [[ "$UNCOMPACTED_BYTES" -gt "$SIZE_THRESHOLD" ]]; then
  emit_violation "size threshold" "uncompacted eligible bytes ($UNCOMPACTED_BYTES) exceeds size threshold ($SIZE_THRESHOLD)"
  exit 1
fi

info "specDir=$NORMALIZED_SPEC totalEnvelopes=$TOTAL_FOR_SPEC eligible=$ELIGIBLE_COUNT uncompacted=$UNCOMPACTED_COUNT bytes=$UNCOMPACTED_BYTES (thresholds: count>$COUNT_THRESHOLD, bytes>$SIZE_THRESHOLD, keep-raw-latest=$KEEP_RAW_LATEST)"
echo "PASS Gate G083 (context_compaction_discipline_gate) — total=$TOTAL_FOR_SPEC eligible=$ELIGIBLE_COUNT uncompacted=$UNCOMPACTED_COUNT bytes=$UNCOMPACTED_BYTES, specDir=$NORMALIZED_SPEC"
exit 0
