#!/usr/bin/env bash
set -euo pipefail

# session-cap-guard.sh
#
# Gate G128 — session_cap_enforcement_gate.
#
# Mechanically enforces the IMP-003 `sessionBudget` aggregate safety caps
# for a whole goal/sprint SESSION. It is the AGGREGATE sibling of Gate
# G082 (`convergence-cap-guard.sh`):
#
#   * G082 caps convergence iterations PER (specDir, agent) — a per-spec
#     ceiling read from `bubbles/workflows.yaml` `maxConvergenceIterations`.
#   * G128 (this gate) caps aggregate usage across every spec and agent
#     attributed to one explicit host session. Other retained records remain
#     stored and excluded.
#
# The active budget is whatever the running session recorded under
# `sessionBudget` in the session file. Its event dimensions are:
#
#   maxTotalConvergenceIterations  aggregate sum of `convergenceLoops[].iterationCount`
#   maxWallClockMinutes            earliest → latest `turnSnapshots[].timestamp`, in minutes
#   maxToolCalls                   unmeasurable until an exact producer exists
#
# DEFAULT-OFF (no-op) for every existing repo: if the session file has no
# `sessionBudget`, or all seven caps are null/absent, the guard exits 0 and
# nothing is enforced. A dimension whose exact-session evidence is absent is
# UNMEASURABLE. Only a non-null cap with a measured observation is compared.
#
# Exit codes:
#   0  no active budget (no-op), no measured cap exceeded, or soft boundary
#      (INCLUDING a crossed SOFT boundary — see below; a soft boundary is a
#      recommendation, not a refusal)
#   1  a measured active-session cap exceeded — orchestrator MUST emit a `blocked`
#      RESULT-ENVELOPE with finding G128 and STOP the session; stderr names
#      the breached dimension(s) and observed-vs-cap
#   2  malformed or missing active inputs, or invalid adapter data
#
# SOFT BOUNDARY AT 70% (IMP-048 SCOPE-6 / COST-9)
# The hard stop was the guard's ONLY outcome, so a session learned it was over
# budget at the moment it was refused, with nothing preserved and no warning it
# could have acted on. A soft boundary fires when the most-consumed measurable
# dimension reaches 70% of its cap: the guard reports the crossing on STDOUT,
# asks `session-review.sh` for a Class C `handoff-to-fresh-session`
# recommendation, and CONTINUES with exit 0. The hard stop at 100% is
# unchanged.
#
# A rollover is NOT a blocked spec. The work is fine; the SESSION is full. The
# guard therefore never touches any `state.json`, and the soft-boundary block
# says so in the output, because "budget exhausted" being recorded as "work
# blocked" is how a healthy spec acquires a false terminal status. The owed
# response is a continuation envelope plus a `bubbles.handoff` packet.
#
# The recommendation reuses the EXISTING Class C surface rather than inventing
# a second one: `session-review.sh emit --trigger budget-threshold --budget-pct
# <pct> --class-c handoff-to-fresh-session=<pct>`. That surface already
# deduplicates (re-emitting only when the metric worsens by 25%) and already
# fires its 50/70/90 bands once each, so a long session is warned rather than
# nagged. With the default `sessionReview.adapter: none` the call is a clean
# no-op and the guard simply reports the recommendation as unrecorded.
#
# Usage:
#   bash bubbles/scripts/session-cap-guard.sh [--session-id <id>] [--quiet]
#
# Optional:
#   --session-id  Exact host-issued session ID. Required only for active caps.
#   --quiet       Suppress explanatory prose; required diagnostics remain.
#
# There is NO `--skip` / `--force` / `--ignore` bypass (matches G082).
#
# Dependencies:
#   - jq      (hard dependency; also parses RFC3339 timestamps via
#             `fromdateiso8601`, so no GNU/BSD `date` divergence)
#
# Schema (additive in bubbles.session.json):
#   {
#     "sessionBudget": {
#       "maxTotalConvergenceIterations": <int|null>,
#       "maxWallClockMinutes":           <int|null>,
#       "maxToolCalls":                  <int|null>
#     },
#     "convergenceLoops": [ { "hostSessionId": <id>, "iterationCount": <int>, ... }, ... ],
#     "turnSnapshots":    [ { "hostSessionId": <id>, "timestamp": "<RFC3339>", ... }, ... ],
#     "toolCallCount":    <int>
#   }
#
# CONTEXT-VOLUME DIMENSIONS (IMP-039 SCOPE-3)
# The three original dimensions cannot see how much text a session carries. A
# run can hold every one of them and still replay 1.77 MB of terminal records
# into every later request, which is what the measured session behind IMP-039
# did. Four OPTIONAL caps close that blind spot, each defaulting to null so
# every existing repository keeps the current no-op posture:
#
#   maxSingleToolResultBytes      largest single tool result retained
#   maxCumulativeToolResultBytes  sum of all retained tool results
#   maxPromptTokensPerRequest     largest single request's prompt tokens
#   maxCumulativePromptTokens     sum of prompt tokens across the session
#
# MEASURABILITY DIFFERS BY DIMENSION, and the existing rule applies unchanged:
# a cap whose dimension cannot be measured is UNMEASURABLE, never guessed.
#   - Byte dimensions read `.specify/runtime/tool-calls.jsonl`
#     (`stdoutBytes` + `stderrBytes`) for exact matching `sessionId` rows.
#   - Token dimensions require a configured usage adapter
#     (`bubbles/adapters/usage/`, default `none`). With `none` there is no
#     honest exact-session token number, so those caps stay unmeasurable.
#
# Reference: improvements/IMP-003-autonomy-dial-and-safety-caps.md (SCOPE-2),
#            IMP-039 SCOPE-3 (context-volume dimensions)


QUIET="false"
SESSION_ID=""
SESSION_ID_SEEN="false"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_IO_HELPER="$SCRIPT_DIR/session-state-io.py"
SOFT_BOUNDARY_PCT=70

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/session-cap-guard.sh [--session-id <opaque-id>] [--quiet]

Evaluates the seven existing sessionBudget dimensions for one exact host
session. An active budget requires exactly one explicit non-empty session ID.
No session file, no budget object, or an all-null budget remains a no-op.

Options:
  --session-id <id>  Exact host-issued session identity to evaluate.
  --quiet            Suppress explanatory prose. Required record, dimension,
                     action, summary, and verdict lines remain visible.
  -h, --help         Print this usage and exit.

Exit codes:
  0 = NO-ACTIVE-BUDGET, PASS, or SOFT-BOUNDARY
  1 = BREACH
  2 = INPUT-ERROR

No --skip / --force / --ignore bypass exists.
EOF
}

json_string() {
  local value="$1"
  local python_bin
  python_bin="$(command -v python3 2>/dev/null || true)"
  [[ -n "$python_bin" && -f "$STATE_IO_HELPER" ]] || return 1
  "$python_bin" "$STATE_IO_HELPER" json-string -- "$value" 2>/dev/null
}

input_error() {
  local reason="$1"
  local value_name="${2:-}"
  local value="${3:-}"
  local encoded=""
  if [[ -n "$value_name" ]]; then
    encoded="$(json_string "$value" || true)"
  fi
  if [[ -n "$value_name" && -n "$encoded" ]]; then
    printf 'session-cap-guard: input-error reason=%s %s=%s\n' \
      "$reason" "$value_name" "$encoded" >&2
  else
    printf 'session-cap-guard: input-error reason=%s\n' "$reason" >&2
  fi
  printf 'G128 action=correct-input\n' >&2
  printf 'G128 status=INPUT-ERROR exit=2 reason=%s\n' "$reason" >&2
  exit 2
}

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
    --session-id)
      [[ $# -ge 2 ]] || input_error "missing-session-id"
      if [[ "$SESSION_ID_SEEN" == "true" ]]; then
        if [[ "$SESSION_ID" == "$2" ]]; then
          input_error "duplicate-session-id"
        fi
        input_error "conflicting-session-id"
      fi
      SESSION_ID="$2"
      SESSION_ID_SEEN="true"
      shift 2
      ;;
    --*)
      input_error "unknown-flag" "argument" "$1"
      ;;
    *)
      input_error "unexpected-positional-argument" "argument" "$1"
      ;;
  esac
done

info() {
  if [[ "$QUIET" != "true" ]]; then
    printf 'session-cap-guard: %s\n' "$*"
  fi
}

if ! command -v jq >/dev/null 2>&1; then
  input_error "jq-unavailable"
fi

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
[[ -n "$REPO_ROOT" ]] || input_error "repository-root-unavailable"

no_active_budget() {
  local reason="$1"
  local message="$2"
  info "$message"
  printf 'G128 action=none\n'
  printf 'G128 status=NO-ACTIVE-BUDGET exit=0 reason=%s\n' "$reason"
  exit 0
}

PYTHON_BIN="$(command -v python3 2>/dev/null || true)"
[[ -n "$PYTHON_BIN" ]] || input_error "python-unavailable"
[[ -f "$STATE_IO_HELPER" && ! -L "$STATE_IO_HELPER" ]] || input_error "state-io-helper-unavailable"

CAPTURE_DIR="$(mktemp -d -t bubbles-g128-capture-XXXXXXXX 2>/dev/null || true)"
[[ -n "$CAPTURE_DIR" && -d "$CAPTURE_DIR" ]] || input_error "capture-directory-failed"
STATE_SNAPSHOT="$CAPTURE_DIR/bubbles.session.json"
EVALUATION_FILE="$CAPTURE_DIR/evaluation.json"
cleanup_capture() {
  rm -rf "$CAPTURE_DIR"
}
trap cleanup_capture EXIT HUP INT TERM

set +e
CAPTURE_OUTPUT="$("$PYTHON_BIN" "$STATE_IO_HELPER" capture \
  --root "$REPO_ROOT" \
  --relative-path '.specify/memory/bubbles.session.json' \
  --destination "$STATE_SNAPSHOT" 2>/dev/null)"
CAPTURE_RC=$?
set -e
if [[ "$CAPTURE_RC" -eq 4 ]]; then
  no_active_budget "no-session-file" "no session state is present; no measurement was attempted"
fi
[[ "$CAPTURE_RC" -eq 0 ]] || input_error "unsafe-session-state"

STATE_REVISION="$(printf '%s' "$CAPTURE_OUTPUT" | jq -er '
  select(type == "object" and .status == "captured")
  | .revision
  | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))
' 2>/dev/null || true)"
[[ -n "$STATE_REVISION" ]] || input_error "invalid-capture-result"
STATE_REVISION_JSON="$(json_string "$STATE_REVISION" || true)"
[[ -n "$STATE_REVISION_JSON" ]] || input_error "json-encoder-failed"

if ! jq -e 'type == "object"' "$STATE_SNAPSHOT" >/dev/null 2>&1; then
  input_error "invalid-session-json"
fi

SESSION_ID_SEEN_JSON="false"
[[ "$SESSION_ID_SEEN" != "true" ]] || SESSION_ID_SEEN_JSON="true"
set +e
jq -c \
  --arg sid "$SESSION_ID" \
  --argjson sidSeen "$SESSION_ID_SEEN_JSON" '
  def cap_keys:
    ["schemaVersion", "maxTotalConvergenceIterations", "maxWallClockMinutes",
     "maxToolCalls", "maxSingleToolResultBytes", "maxCumulativeToolResultBytes",
     "maxPromptTokensPerRequest", "maxCumulativePromptTokens"];
  def outer_keys:
    ["recordSchemaVersion", "hostSessionId", "revision", "supersedesRevision",
     "recordedAt", "budget"];
  def non_negative_integer:
    type == "number" and . >= 0 and floor == .;
  def valid_timestamp:
    if type != "string" then false
    else
      test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
      and (try (fromdateiso8601 | type == "number") catch false)
    end;
  def valid_budget:
    if type != "object" then false
    else
      ((keys - cap_keys) | length) == 0
      and (.schemaVersion | type) == "number"
      and .schemaVersion == 1
      and ([cap_keys[1:][] as $key
        | ((has($key) | not) or .[$key] == null
           or (.[$key] | non_negative_integer))]
        | all)
    end;
  def valid_record:
    if type != "object" then false
    else
      ((keys | sort) == (outer_keys | sort))
      and (.recordSchemaVersion | type) == "number"
      and .recordSchemaVersion == 1
      and (.hostSessionId | type) == "string"
      and (.hostSessionId | length) > 0
      and (.revision | non_negative_integer)
      and .revision > 0
      and (
        .supersedesRevision == null
        or (
          (.supersedesRevision | non_negative_integer)
          and .supersedesRevision > 0
          and .supersedesRevision < .revision
        )
      )
      and (.recordedAt | valid_timestamp)
      and (.budget | valid_budget)
    end;
  def normalized_budget:
    {
      schemaVersion: 1,
      maxTotalConvergenceIterations: (.maxTotalConvergenceIterations // null),
      maxWallClockMinutes: (.maxWallClockMinutes // null),
      maxToolCalls: (.maxToolCalls // null),
      maxSingleToolResultBytes: (.maxSingleToolResultBytes // null),
      maxCumulativeToolResultBytes: (.maxCumulativeToolResultBytes // null),
      maxPromptTokensPerRequest: (.maxPromptTokensPerRequest // null),
      maxCumulativePromptTokens: (.maxCumulativePromptTokens // null)
    };
  def active_budget:
    [cap_keys[1:][] as $key | .[$key] != null] | any;
  def invalid_cap_name($records):
    [$records[]
     | select(type == "object")
     | .budget
     | select(type == "object")
     | . as $budget
     | cap_keys[1:][] as $key
     | select(($budget | has($key)) and $budget[$key] != null)
     | select(($budget[$key] | non_negative_integer) | not)
     | $key][0] // null;
  def chain_reason($records):
    if (all($records[]; valid_record) | not) then "invalid-session-policy-record"
    elif (($records | map(.revision) | unique | length) != ($records | length)) then "duplicate-policy-revision"
    elif (($records | map(select(.revision == 1 and .supersedesRevision == null)) | length) != 1) then "missing-policy-root"
    elif ([$records[] | select(.revision > 1) as $record
           | (([$records[] | select(.revision == $record.supersedesRevision)] | length) != 1
              or $record.supersedesRevision != ($record.revision - 1))]
          | any) then "invalid-policy-chain"
    else null
    end;
  if (.sessionBudgetHistory? == null) then
    {_g128Policy: {state: "no-active", reason: "no-session-budget-history"}}
  elif (.sessionBudgetHistory | type) != "array" then
    {_g128Policy: {state: "input-error", reason: "invalid-policy-history"}}
  elif (($sidSeen | not) or ($sid | length) == 0) then
    invalid_cap_name(.sessionBudgetHistory) as $invalidCap
    | if $invalidCap != null then
      {_g128Policy: {
        state: "input-error",
        reason: "invalid-cap",
        dimension: $invalidCap
      }}
    elif (all(.sessionBudgetHistory[]; valid_record) | not) then
      {_g128Policy: {state: "input-error", reason: "invalid-session-policy-record"}}
    elif ([.sessionBudgetHistory[] | .budget | active_budget] | any) then
      {_g128Policy: {state: "input-error", reason: "missing-session-id"}}
    else
      {_g128Policy: {state: "no-active", reason: "all-caps-null"}}
    end
  else
    . as $state
    | ([.sessionBudgetHistory[]
        | select(type == "object" and .hostSessionId == $sid)]) as $records
    | if ($records | length) == 0 then
        {_g128Policy: {state: "no-active", reason: "no-exact-session-policy"}}
      else
        invalid_cap_name($records) as $invalidCap
        | chain_reason($records) as $chainReason
        | if $invalidCap != null then
            {_g128Policy: {
              state: "input-error",
              reason: "invalid-cap",
              dimension: $invalidCap
            }}
          elif $chainReason != null then
            {_g128Policy: {
              state: "input-error",
              reason: $chainReason,
              dimension: null
            }}
          else
            ($records | max_by(.revision)) as $head
            | ($head.budget | normalized_budget) as $budget
            | if ($budget | active_budget) then
                $state + {
                  sessionBudget: $budget,
                  _g128Policy: {
                    state: "active",
                    reason: null,
                    revision: $head.revision,
                    policyCount: ($records | length)
                  }
                }
              else
                {_g128Policy: {state: "no-active", reason: "all-caps-null"}}
              end
          end
      end
  end
' "$STATE_SNAPSHOT" > "$EVALUATION_FILE" 2>/dev/null
POLICY_RC=$?
set -e
[[ "$POLICY_RC" -eq 0 ]] || input_error "invalid-session-policy"
POLICY_STATE="$(jq -r '._g128Policy.state // "invalid"' "$EVALUATION_FILE" 2>/dev/null || true)"
POLICY_REASON="$(jq -r '._g128Policy.reason // "invalid-session-policy"' "$EVALUATION_FILE" 2>/dev/null || true)"
POLICY_DIMENSION="$(jq -r '._g128Policy.dimension // empty' "$EVALUATION_FILE" 2>/dev/null || true)"
case "$POLICY_STATE" in
  no-active)
    case "$POLICY_REASON" in
      no-session-budget-history)
        no_active_budget "$POLICY_REASON" "no exact-session policy history is recorded; no measurement was attempted"
        ;;
      no-exact-session-policy)
        no_active_budget "$POLICY_REASON" "the requested session has no policy head; no measurement was attempted"
        ;;
      all-caps-null)
        no_active_budget "$POLICY_REASON" "the exact-session policy head has seven null caps; no measurement was attempted"
        ;;
      *)
        input_error "invalid-session-policy"
        ;;
    esac
    ;;
  input-error)
    case "$POLICY_REASON" in
      invalid-cap)
        if [[ -n "$POLICY_DIMENSION" ]]; then
          input_error "invalid-cap" "dimension" "$POLICY_DIMENSION"
        fi
        input_error "invalid-cap"
        ;;
      invalid-session-policy-record)
        input_error "$POLICY_REASON"
        ;;
      invalid-policy-history|duplicate-policy-revision|missing-policy-root|invalid-policy-chain|missing-session-id)
        input_error "$POLICY_REASON"
        ;;
      *)
        input_error "invalid-session-policy"
        ;;
    esac
    ;;
  active)
    ;;
  *)
    input_error "invalid-session-policy"
    ;;
esac

SESSION_FILE="$EVALUATION_FILE"
POLICY_REVISION="$(jq -r '._g128Policy.revision' "$SESSION_FILE")"
POLICY_COUNT="$(jq -r '._g128Policy.policyCount' "$SESSION_FILE")"
if ! jq -e '.sessionBudget | type == "object"' "$SESSION_FILE" >/dev/null 2>&1; then
  input_error "invalid-session-policy"
fi

validate_cap_type() {
  local label="$1"
  if ! jq -e --arg label "$label" '
    ((.sessionBudget | has($label)) | not)
    or .sessionBudget[$label] == null
    or (
      (.sessionBudget[$label] | type) == "number"
      and .sessionBudget[$label] >= 0
      and (.sessionBudget[$label] | floor) == .sessionBudget[$label]
    )
  ' "$SESSION_FILE" >/dev/null 2>&1; then
    input_error "invalid-cap" "dimension" "$label"
  fi
}

validate_cap_type "maxTotalConvergenceIterations"
validate_cap_type "maxWallClockMinutes"
validate_cap_type "maxToolCalls"
validate_cap_type "maxSingleToolResultBytes"
validate_cap_type "maxCumulativeToolResultBytes"
validate_cap_type "maxPromptTokensPerRequest"
validate_cap_type "maxCumulativePromptTokens"

CAP_CONV="$(jq -r '.sessionBudget.maxTotalConvergenceIterations // "null"' "$SESSION_FILE")"
CAP_MINS="$(jq -r '.sessionBudget.maxWallClockMinutes // "null"' "$SESSION_FILE")"
CAP_TOOLS="$(jq -r '.sessionBudget.maxToolCalls // "null"' "$SESSION_FILE")"
CAP_SINGLE_BYTES="$(jq -r '.sessionBudget.maxSingleToolResultBytes // "null"' "$SESSION_FILE")"
CAP_CUM_BYTES="$(jq -r '.sessionBudget.maxCumulativeToolResultBytes // "null"' "$SESSION_FILE")"
CAP_REQ_TOKENS="$(jq -r '.sessionBudget.maxPromptTokensPerRequest // "null"' "$SESSION_FILE")"
CAP_CUM_TOKENS="$(jq -r '.sessionBudget.maxCumulativePromptTokens // "null"' "$SESSION_FILE")"

if [[ "$CAP_CONV" == "null" && "$CAP_MINS" == "null" && "$CAP_TOOLS" == "null" &&
  "$CAP_SINGLE_BYTES" == "null" && "$CAP_CUM_BYTES" == "null" &&
  "$CAP_REQ_TOKENS" == "null" && "$CAP_CUM_TOKENS" == "null" ]]; then
  no_active_budget "all-caps-null" "the exact-session policy head has seven null caps; no measurement was attempted"
fi

validate_cap() {
  local label="$1"
  local value="$2"
  [[ "$value" == "null" ]] && return 0
  [[ "$value" =~ ^[0-9]+$ ]] || input_error "invalid-cap" "dimension" "$label"
}

validate_cap "maxTotalConvergenceIterations" "$CAP_CONV"
validate_cap "maxWallClockMinutes" "$CAP_MINS"
validate_cap "maxToolCalls" "$CAP_TOOLS"
validate_cap "maxSingleToolResultBytes" "$CAP_SINGLE_BYTES"
validate_cap "maxCumulativeToolResultBytes" "$CAP_CUM_BYTES"
validate_cap "maxPromptTokensPerRequest" "$CAP_REQ_TOKENS"
validate_cap "maxCumulativePromptTokens" "$CAP_CUM_TOKENS"

if [[ "$SESSION_ID_SEEN" != "true" || -z "$SESSION_ID" ]]; then
  input_error "missing-session-id"
fi

SESSION_DISPLAY="$(json_string "$SESSION_ID" || true)"
[[ -n "$SESSION_DISPLAY" ]] || input_error "json-encoder-failed"

if ! jq -e '
  (((has("turnSnapshots") | not) or .turnSnapshots == null or (.turnSnapshots | type) == "array"))
  and (((has("convergenceLoops") | not) or .convergenceLoops == null or (.convergenceLoops | type) == "array"))
' "$SESSION_FILE" >/dev/null 2>&1; then
  input_error "invalid-session-records"
fi

SESSION_STATS="$(jq -c --arg sid "$SESSION_ID" '
  def decorate($field):
    . as $row
    | (if ($row | type) == "object" then $row[$field] else null end) as $value
    | {
        row: $row,
        class: (
          if ($value | type) != "string" or ($value | length) == 0 then "unattributed"
          elif $value == $sid then "matching"
          else "mismatched"
          end
        )
      };
  def class_count($rows; $class): [ $rows[] | select(.class == $class) ] | length;
  def non_negative_integer: type == "number" and . >= 0 and floor == .;
    def valid_timestamp:
      type == "string"
      and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
      and (try (fromdateiso8601 | type == "number") catch false);
  ((.turnSnapshots // []) | map(decorate("hostSessionId"))) as $turns
  | ((.convergenceLoops // []) | map(decorate("hostSessionId"))) as $convergence
    | ([ $turns[] | select(.class == "matching") ]) as $matching_turns
    | ([ $matching_turns[]
      | select((.row.timestamp | valid_timestamp) | not) ] | length) as $invalid_timestamps
    | ([ $matching_turns[]
      | select(.row.timestamp | valid_timestamp)
      | .row.timestamp
      | fromdateiso8601 ]) as $timestamps
  | ([ $convergence[] | select(.class == "matching") ]) as $matching_convergence
  | ([ $matching_convergence[]
       | select((.row.iterationCount | non_negative_integer) | not) ] | length) as $invalid_convergence
  | {
      turns: {
        matching: class_count($turns; "matching"),
        mismatched: class_count($turns; "mismatched"),
        unattributed: class_count($turns; "unattributed"),
        invalidMatching: $invalid_timestamps,
        eligible: ($timestamps | length),
        observed: (
          if ($matching_turns | length) == 0 or $invalid_timestamps > 0 then null
          else (($timestamps | max) - ($timestamps | min)) / 60
          end
        )
      },
      convergence: {
        matching: class_count($convergence; "matching"),
        mismatched: class_count($convergence; "mismatched"),
        unattributed: class_count($convergence; "unattributed"),
        eligible: (($matching_convergence | length) - $invalid_convergence),
        invalidMatching: $invalid_convergence,
        observed: (
          if ($matching_convergence | length) == 0 or $invalid_convergence > 0 then null
          else ($matching_convergence | map(.row.iterationCount) | add)
          end
        )
      },
      scalar: {
        matching: 0,
        mismatched: 0,
        unattributed: (if has("toolCallCount") then 1 else 0 end),
        eligible: 0
      }
    }
' "$SESSION_FILE" 2>/dev/null || true)"

if [[ -z "$SESSION_STATS" ]] || ! printf '%s' "$SESSION_STATS" | jq -e 'type == "object"' >/dev/null 2>&1; then
  input_error "invalid-session-records"
fi

TURN_INVALID="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.invalidMatching')"
[[ "$TURN_INVALID" == "0" ]] || input_error "invalid-matching-turn-timestamp"
CONV_INVALID="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.invalidMatching')"
[[ "$CONV_INVALID" == "0" ]] || input_error "invalid-matching-convergence"

TURN_MATCHING="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.matching')"
TURN_MISMATCHED="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.mismatched')"
TURN_UNATTRIBUTED="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.unattributed')"
TURN_ELIGIBLE="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.eligible')"
MIN_OBSERVED="$(printf '%s' "$SESSION_STATS" | jq -r '.turns.observed // "null"')"
CONV_MATCHING="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.matching')"
CONV_MISMATCHED="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.mismatched')"
CONV_UNATTRIBUTED="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.unattributed')"
CONV_ELIGIBLE="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.eligible')"
CONV_OBSERVED="$(printf '%s' "$SESSION_STATS" | jq -r '.convergence.observed // "null"')"
SCALAR_UNATTRIBUTED="$(printf '%s' "$SESSION_STATS" | jq -r '.scalar.unattributed')"

TOOL_MATCHING=0
TOOL_MISMATCHED=0
TOOL_UNATTRIBUTED=0
TOOL_ELIGIBLE=0
SINGLE_BYTES_OBSERVED="null"
CUM_BYTES_OBSERVED="null"
TOOL_LOG_SNAPSHOT="$CAPTURE_DIR/tool-calls.jsonl"
TOOL_LOG_REVISION_JSON=""

set +e
TOOL_CAPTURE_OUTPUT="$("$PYTHON_BIN" "$STATE_IO_HELPER" capture \
  --root "$REPO_ROOT" \
  --relative-path '.specify/runtime/tool-calls.jsonl' \
  --destination "$TOOL_LOG_SNAPSHOT" 2>/dev/null)"
TOOL_CAPTURE_RC=$?
set -e
if [[ "$TOOL_CAPTURE_RC" -eq 0 ]]; then
  TOOL_LOG_REVISION="$(printf '%s' "$TOOL_CAPTURE_OUTPUT" | jq -er '
    select(type == "object" and .status == "captured")
    | .revision
    | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))
  ' 2>/dev/null || true)"
  [[ -n "$TOOL_LOG_REVISION" ]] || input_error "invalid-tool-log-capture"
  TOOL_LOG_REVISION_JSON="$(json_string "$TOOL_LOG_REVISION" || true)"
  [[ -n "$TOOL_LOG_REVISION_JSON" ]] || input_error "json-encoder-failed"
  set +e
  BYTES_JSON="$(jq -Rs -c --arg sid "$SESSION_ID" '
    def non_negative_integer: type == "number" and . >= 0 and floor == .;
    def decorate:
      . as $row
      | (if ($row | type) == "object" then $row.sessionId else null end) as $value
      | {
          row: $row,
          class: (
            if ($value | type) != "string" or ($value | length) == 0 then "unattributed"
            elif $value == $sid then "matching"
            else "mismatched"
            end
          )
        };
    def class_count($rows; $class): [ $rows[] | select(.class == $class) ] | length;
    def valid_part($row; $key):
        (($row | has($key)) | not) or ($row[$key] | non_negative_integer);
      (split("\n") | if length > 0 and .[-1] == "" then .[0:-1] else . end) as $physical
      | ($physical | map(fromjson) | map(decorate)) as $rows
    | ([ $rows[] | select(.class == "matching") ]) as $matching
    | ([ $matching[]
         | select((valid_part(.row; "stdoutBytes") and valid_part(.row; "stderrBytes")) | not) ] | length) as $invalid
    | ([ $matching[]
         | select((.row | type) == "object")
         | select((.row | has("stdoutBytes")) or (.row | has("stderrBytes")))
         | select(valid_part(.row; "stdoutBytes") and valid_part(.row; "stderrBytes"))
         | ((.row.stdoutBytes // 0) + (.row.stderrBytes // 0)) ]) as $bytes
    | {
        matching: class_count($rows; "matching"),
        mismatched: class_count($rows; "mismatched"),
        unattributed: class_count($rows; "unattributed"),
        eligible: ($bytes | length),
        invalidMatching: $invalid,
        max: (if ($bytes | length) == 0 then null else ($bytes | max) end),
        sum: (if ($bytes | length) == 0 then null else ($bytes | add) end)
      }
  ' "$TOOL_LOG_SNAPSHOT" 2>/dev/null)"
  BYTES_RC=$?
  set -e
  if [[ "$BYTES_RC" -ne 0 || -z "$BYTES_JSON" ]] || ! printf '%s' "$BYTES_JSON" | jq -e 'type == "object"' >/dev/null 2>&1; then
    input_error "invalid-tool-log"
  fi
  BYTE_INVALID="$(printf '%s' "$BYTES_JSON" | jq -r '.invalidMatching')"
  [[ "$BYTE_INVALID" == "0" ]] || input_error "invalid-matching-tool-bytes"
  TOOL_MATCHING="$(printf '%s' "$BYTES_JSON" | jq -r '.matching')"
  TOOL_MISMATCHED="$(printf '%s' "$BYTES_JSON" | jq -r '.mismatched')"
  TOOL_UNATTRIBUTED="$(printf '%s' "$BYTES_JSON" | jq -r '.unattributed')"
  TOOL_ELIGIBLE="$(printf '%s' "$BYTES_JSON" | jq -r '.eligible')"
  SINGLE_BYTES_OBSERVED="$(printf '%s' "$BYTES_JSON" | jq -r '.max // "null"')"
  CUM_BYTES_OBSERVED="$(printf '%s' "$BYTES_JSON" | jq -r '.sum // "null"')"
elif [[ "$TOOL_CAPTURE_RC" -ne 4 ]]; then
  input_error "unsafe-tool-log"
fi

USAGE_MATCHING=0
USAGE_MISMATCHED=0
USAGE_UNATTRIBUTED=0
USAGE_ELIGIBLE=0
REQ_TOKENS_OBSERVED="null"
CUM_TOKENS_OBSERVED="null"
USAGE_RESOLVE="$SCRIPT_DIR/usage-resolve.sh"

if [[ -f "$USAGE_RESOLVE" ]]; then
  set +e
  USAGE_RESOLUTION="$(bash "$USAGE_RESOLVE" --repo-root "$REPO_ROOT" 2>/dev/null)"
  USAGE_RESOLVE_RC=$?
  set -e
  [[ "$USAGE_RESOLVE_RC" -eq 0 ]] || input_error "usage-adapter-resolution-failed"
  USAGE_ADAPTER_NAME="$(printf '%s\n' "$USAGE_RESOLUTION" | awk -F= '$1 == "adapter" { print $2; exit }')"
  USAGE_ADAPTER_PATH="$(printf '%s\n' "$USAGE_RESOLUTION" | awk -F= '$1 == "adapterPath" { print $2; exit }')"
  if [[ -n "$USAGE_ADAPTER_PATH" && "$USAGE_ADAPTER_NAME" != "none" ]]; then
    set +e
    USAGE_RAW="$(bash "$USAGE_ADAPTER_PATH" session "$SESSION_ID" 2>&1)"
    USAGE_RC=$?
    set -e
    [[ "$USAGE_RC" -eq 0 ]] || input_error "invalid-usage-result"
    USAGE_SESSION="$(printf '%s' "$USAGE_RAW" | jq -s -c 'if length == 1 and (.[0] | type) == "object" then .[0] else empty end' 2>/dev/null || true)"
    [[ -n "$USAGE_SESSION" ]] || input_error "invalid-usage-result"
    if ! printf '%s' "$USAGE_SESSION" | jq -e 'length == 0' >/dev/null 2>&1; then
      if printf '%s' "$USAGE_SESSION" | jq -e 'has("promptTokens") or has("maxPromptTokens")' >/dev/null 2>&1; then
        if ! printf '%s' "$USAGE_SESSION" | jq -e --arg sid "$SESSION_ID" '
          (keys | sort) == ([
            "artifactCount", "completionTokens", "credits", "identityMatch",
            "maxPromptTokens", "models", "promptTokens", "requests", "sessionId"
          ] | sort)
          and .sessionId == $sid
          and .identityMatch == "exact"
          and .artifactCount == 1
          and (.requests | type) == "number" and .requests > 0 and (.requests | floor) == .requests
          and (.promptTokens | type) == "number" and .promptTokens >= 0 and (.promptTokens | floor) == .promptTokens
          and (.maxPromptTokens | type) == "number" and .maxPromptTokens >= 0 and (.maxPromptTokens | floor) == .maxPromptTokens
          and .maxPromptTokens <= .promptTokens
          and (.completionTokens | type) == "number" and .completionTokens >= 0 and (.completionTokens | floor) == .completionTokens
          and (.credits | type) == "number" and .credits >= 0
          and (.models | type) == "array" and all(.models[]; type == "string")
        ' >/dev/null 2>&1; then
          input_error "invalid-usage-result"
        fi
        USAGE_MATCHING=1
        USAGE_ELIGIBLE=1
        REQ_TOKENS_OBSERVED="$(printf '%s' "$USAGE_SESSION" | jq -r '.maxPromptTokens')"
        CUM_TOKENS_OBSERVED="$(printf '%s' "$USAGE_SESSION" | jq -r '.promptTokens')"
      else
        USAGE_UNATTRIBUTED=1
      fi
    fi
  fi
fi

declare -a DIM_NAMES=(
  "maxTotalConvergenceIterations"
  "maxWallClockMinutes"
  "maxToolCalls"
  "maxSingleToolResultBytes"
  "maxCumulativeToolResultBytes"
  "maxPromptTokensPerRequest"
  "maxCumulativePromptTokens"
)
declare -a DIM_CAPS=(
  "$CAP_CONV"
  "$CAP_MINS"
  "$CAP_TOOLS"
  "$CAP_SINGLE_BYTES"
  "$CAP_CUM_BYTES"
  "$CAP_REQ_TOKENS"
  "$CAP_CUM_TOKENS"
)
declare -a DIM_OBSERVED=(
  "$CONV_OBSERVED"
  "$MIN_OBSERVED"
  "null"
  "$SINGLE_BYTES_OBSERVED"
  "$CUM_BYTES_OBSERVED"
  "$REQ_TOKENS_OBSERVED"
  "$CUM_TOKENS_OBSERVED"
)
declare -a DIM_REASONS=(
  "no-matching-convergence-record"
  "no-matching-turn-timestamp"
  "no-exact-producer"
  "no-matching-byte-record"
  "no-matching-byte-record"
  "no-exact-usage-result"
  "no-exact-usage-result"
)
declare -a DIM_STATES=()
declare -a DIM_PCTS=()
declare -a BREACHES=()
declare -a CONSUMPTION=()

record_pct() {
  local observed="$1"
  local cap="$2"
  awk -v observed="$observed" -v cap="$cap" 'BEGIN {
    if (cap + 0 <= 0) { print (observed + 0 > 0) ? 100 : 0; exit }
    pct = int(observed * 100 / cap)
    if (pct < 0) pct = 0
    print pct
  }'
}

index=0
while [[ "$index" -lt 7 ]]; do
  observed="${DIM_OBSERVED[$index]}"
  cap="${DIM_CAPS[$index]}"
  if [[ "$observed" == "null" ]]; then
    DIM_STATES+=("UNMEASURABLE")
    DIM_PCTS+=("-")
  else
    DIM_STATES+=("MEASURED")
    if [[ "$cap" == "null" ]]; then
      DIM_PCTS+=("-")
    else
      pct="$(record_pct "$observed" "$cap")"
      DIM_PCTS+=("$pct")
      CONSUMPTION+=("$pct|${DIM_NAMES[$index]}|$observed|$cap")
      if awk -v observed="$observed" -v cap="$cap" 'BEGIN { exit !(observed > cap) }'; then
        BREACHES+=("${DIM_NAMES[$index]}|$observed|$cap")
      fi
    fi
  fi
  index=$((index + 1))
done

TURN_EXCLUDED=$((TURN_MISMATCHED + TURN_UNATTRIBUTED))
CONV_EXCLUDED=$((CONV_MISMATCHED + CONV_UNATTRIBUTED))
SCALAR_EXCLUDED=$SCALAR_UNATTRIBUTED
TOOL_EXCLUDED=$((TOOL_MISMATCHED + TOOL_UNATTRIBUTED))
USAGE_EXCLUDED=$((USAGE_MISMATCHED + USAGE_UNATTRIBUTED))
TOTAL_EXCLUDED=$((TURN_EXCLUDED + CONV_EXCLUDED + SCALAR_EXCLUDED + TOOL_EXCLUDED + USAGE_EXCLUDED))

MEASURED_COUNT=0
index=0
while [[ "$index" -lt 7 ]]; do
  [[ "${DIM_STATES[$index]}" != "MEASURED" ]] || MEASURED_COUNT=$((MEASURED_COUNT + 1))
  index=$((index + 1))
done
UNMEASURABLE_COUNT=$((7 - MEASURED_COUNT))

emit_diagnostics() {
  local index cap_display observed_display reason
  printf 'G128 identity session=%s authority=not-validated enforcement=diagnostic-only\n' "$SESSION_DISPLAY"
  printf 'G128 evaluation revision=%s immutable=true\n' "$STATE_REVISION_JSON"
  if [[ -n "$TOOL_LOG_REVISION_JSON" ]]; then
    printf 'G128 evaluation toolReceiptRevision=%s immutable=true\n' "$TOOL_LOG_REVISION_JSON"
  fi
  printf 'G128 budget session=%s revision=%s policyCount=%s capCount=7\n' \
    "$SESSION_DISPLAY" "$POLICY_REVISION" "$POLICY_COUNT"
  printf 'G128 records source=turns matching=%s mismatched=%s unattributed=%s excluded=%s eligible=%s\n' \
    "$TURN_MATCHING" "$TURN_MISMATCHED" "$TURN_UNATTRIBUTED" "$TURN_EXCLUDED" "$TURN_ELIGIBLE"
  printf 'G128 records source=convergence matching=%s mismatched=%s unattributed=%s excluded=%s eligible=%s\n' \
    "$CONV_MATCHING" "$CONV_MISMATCHED" "$CONV_UNATTRIBUTED" "$CONV_EXCLUDED" "$CONV_ELIGIBLE"
  printf 'G128 records source=legacy-tool-call-scalar matching=0 mismatched=0 unattributed=%s excluded=%s eligible=0\n' \
    "$SCALAR_UNATTRIBUTED" "$SCALAR_EXCLUDED"
  printf 'G128 records source=tool-results matching=%s mismatched=%s unattributed=%s excluded=%s eligible=%s\n' \
    "$TOOL_MATCHING" "$TOOL_MISMATCHED" "$TOOL_UNATTRIBUTED" "$TOOL_EXCLUDED" "$TOOL_ELIGIBLE"
  printf 'G128 records source=usage matching=%s mismatched=%s unattributed=%s excluded=%s eligible=%s\n' \
    "$USAGE_MATCHING" "$USAGE_MISMATCHED" "$USAGE_UNATTRIBUTED" "$USAGE_EXCLUDED" "$USAGE_ELIGIBLE"
  index=0
  while [[ "$index" -lt 7 ]]; do
    cap_display="${DIM_CAPS[$index]}"
    [[ "$cap_display" != "null" ]] || cap_display="unset"
    observed_display="${DIM_OBSERVED[$index]}"
    reason="-"
    if [[ "$observed_display" == "null" ]]; then
      observed_display="-"
      reason="${DIM_REASONS[$index]}"
    fi
    printf 'G128 dimension name=%s cap=%s state=%s observed=%s reason=%s pct=%s\n' \
      "${DIM_NAMES[$index]}" "$cap_display" "${DIM_STATES[$index]}" \
      "$observed_display" "$reason" "${DIM_PCTS[$index]}"
    index=$((index + 1))
  done
  printf 'G128 summary measured=%s/7 unmeasurable=%s/7 excluded=%s\n' \
    "$MEASURED_COUNT" "$UNMEASURABLE_COUNT" "$TOTAL_EXCLUDED"
}

if [[ "${#BREACHES[@]}" -gt 0 ]]; then
  {
    emit_diagnostics
    for breach in "${BREACHES[@]}"; do
      breach_dimension="${breach%%|*}"
      breach_rest="${breach#*|}"
      breach_observed="${breach_rest%%|*}"
      breach_cap="${breach_rest##*|}"
      printf 'G128 breach name=%s observed=%s cap=%s comparison=greater-than\n' \
        "$breach_dimension" "$breach_observed" "$breach_cap"
    done
    printf 'G128 action=stop-session\n'
    printf 'G128 status=BREACH exit=1 session=%s\n' "$SESSION_DISPLAY"
  } >&2
  exit 1
fi

request_handoff_recommendation() {
  local pct="$1"
  local detail="$2"
  local review="$SCRIPT_DIR/session-review.sh"
  local out rc adapter emitted
  if [[ ! -f "$review" ]]; then
    printf 'unrecorded'
    return 0
  fi
  set +e
  out="$(bash "$review" emit \
    --repo-root "$REPO_ROOT" \
    --trigger budget-threshold \
    --budget-pct "$pct" \
    --class-c "handoff-to-fresh-session=$pct" \
    --class-c-reason "handoff-to-fresh-session=session budget at ${pct}% of cap on ${detail}; roll over before the hard stop" 2>&1)"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    printf 'unrecorded'
    return 0
  fi
  adapter="$(printf '%s\n' "$out" | awk -F= '$1 == "adapter" { print $2; exit }')"
  if [[ "$adapter" == "none" ]]; then
    printf 'unrecorded'
    return 0
  fi
  emitted="$(printf '%s\n' "$out" | awk -F= '$1 == "classCEmitted" { print $2; exit }')"
  if [[ "${emitted:-0}" -gt 0 ]]; then
    printf 'recorded-emitted'
  else
    printf 'recorded-deduplicated'
  fi
}

TOP_PCT=0
TOP_LABEL=""
TOP_OBSERVED=""
TOP_CAP=""
if [[ "${#CONSUMPTION[@]}" -gt 0 ]]; then
  TOP_CONSUMPTION="$(printf '%s\n' "${CONSUMPTION[@]}" |
    awk -F'|' '$1 + 0 > max { max = $1 + 0; best = $0 } END { print best }')"
  TOP_PCT="${TOP_CONSUMPTION%%|*}"
  TOP_REST="${TOP_CONSUMPTION#*|}"
  TOP_LABEL="${TOP_REST%%|*}"
  TOP_REST="${TOP_REST#*|}"
  TOP_OBSERVED="${TOP_REST%%|*}"
  TOP_CAP="${TOP_REST##*|}"
fi

emit_diagnostics
if [[ "${TOP_PCT:-0}" -ge "$SOFT_BOUNDARY_PCT" ]]; then
  HANDOFF_STATUS="$(request_handoff_recommendation "$TOP_PCT" "$TOP_LABEL")"
  printf 'G128 softBoundary=crossed dimension=%s observed=%s cap=%s consumedPct=%s threshold=%s\n' \
    "$TOP_LABEL" "$TOP_OBSERVED" "$TOP_CAP" "$TOP_PCT" "$SOFT_BOUNDARY_PCT"
  printf 'G128 handoffRecommendation=%s\n' "$HANDOFF_STATUS"
  printf 'G128 action=handoff\n'
  printf 'G128 status=SOFT-BOUNDARY exit=0 session=%s\n' "$SESSION_DISPLAY"
  exit 0
fi

printf 'G128 action=continue\n'
printf 'G128 status=PASS exit=0 session=%s\n' "$SESSION_DISPLAY"
exit 0
