#!/usr/bin/env bash
# phase-coordinator.sh — the executable phase coordinator (IMP-047 S-C).
#
# Capability: occurrence-aware-phase-execution
# Capability: impact-aware-validation-trace-contracts
#
# WHY THIS EXISTS
# Phase relevance and test impact were registries that no executable consumed
# end to end: each runner decided for itself, so a "smart routing" claim was
# only as true as whichever runner happened to implement it. And the resume
# state was keyed BY PHASE NAME, so a mode that runs `validate` twice could not
# tell the two runs apart. Interrupt it after the first `validate` and it either
# replayed an accepted phase or skipped an unrun one, and both look identical in
# a log that only records names.
#
# This script is the consumer S-C requires. A resolver with no production
# consumer does not ship, because a verdict nobody executes is a claim, not a
# behavior.
#
# WHAT IT GUARANTEES
#   occurrence identity  Repeated phases get DISTINCT ids (`validate#1`,
#                        `validate#2`). Identity is positional, so the second
#                        run of a phase is a different thing from the first.
#   resume correctness   Work resumes at the FIRST UNRESOLVED occurrence, read
#                        from the cursor, never from a phase name or a guess.
#   no replay            An occurrence already accepted is reported ACCEPTED and
#                        its command is NOT run again. Re-running accepted work
#                        is how a green result gets manufactured by repetition.
#   blocked identity     A failed prerequisite marks its dependents
#                        BLOCKED_NOT_RUN. That is NOT a pass and NOT a failure.
#                        Independent phases STILL EXECUTE, because one failed
#                        prerequisite must not silently delete unrelated
#                        diagnostics.
#   plan fidelity        Every iteration records planned vs resolved vs drifted.
#   honest exhaustion    Running out of iterations with work outstanding exits
#                        NON-ZERO. Exhaustion is never reported as success.
#
# Usage:
#   bash bubbles/scripts/phase-coordinator.sh --spec-dir <dir> [options]
#
# Plan (order matters; occurrence ids are positional):
#   --phase <name>=<command>        Chained: depends on the previous --phase.
#   --independent <name>=<command>  No prerequisite; runs even when a chained
#                                   prerequisite failed.
#
# Options:
#   --cursor <path>        Cursor file (default <spec-dir>/.phase-cursor.json)
#   --changed-file <path>  Repeatable; forwarded to relevance and impact
#   --max-iterations <N>   Exhaust after N iterations (default 10)
#   --format text|json     Output format (default: text)
#   --dry-run              Resolve the plan and the cursor; execute NOTHING
#   --mbe-posture <mode>   off (default), shadow, advisory, or reference-enforce
#   --mbe-store-root PATH  Required only for reference-enforce
#   --mbe-action ID=PATH   Exact broker action file for one occurrence
#   --mbe-permit-consumption ID=PATH
#                          Exact one-use permit consumption for one occurrence
#
# reference-enforce dispatches through mutable-dispatch-gateway.sh (IMP-056
# SCOPE-4), never the broker directly (SCOPE-6: no orchestrator receives an
# implicit exemption from the composed authorization chain). All of the
# following are therefore REQUIRED together with reference-enforce; there is
# no reduced-authorization fallback:
#   --mbe-repo-root PATH           Repository root passed to the gateway
#   --mbe-authority-file PATH      mutable-dispatch-authorization signing key
#   --mbe-session-id ID            Current repository-binding session id
#   --mbe-session-control-file PATH
#   --mbe-packet-file PATH         Current actionable repository packet
#   --mbe-feature-dir DIR          Boundary-classification feature directory
#   --mbe-candidate-repo SLUG      Boundary-classification candidate repo
#   --mbe-receipt-file PATH        Current pre-dispatch G134 receipt
#   --mbe-session-file PATH        Current goal-contract session file
#
# There is no --skip, --force, --ignore or --replay flag. An occurrence is
# resolved by running it, never by asserting it.
#
# Exit codes:
#   0  every planned occurrence is resolved and accepted
#   1  work remains: a phase failed, a dependent is BLOCKED_NOT_RUN, or the
#      iteration budget is exhausted
#   2  usage error or a missing dependency

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="phase-coordinator"
RELEVANCE="$SCRIPT_DIR/phase-relevance-resolve.sh"
TEST_IMPACT="$SCRIPT_DIR/test-impact-plan.sh"
SCENARIO_RESOLVER="$SCRIPT_DIR/scenario-state-resolve.sh"
GATEWAY="${BUBBLES_MUTABLE_DISPATCH_GATEWAY:-$SCRIPT_DIR/mutable-dispatch-gateway.sh}"

# Occurrence identity is shared with test-leaf-receipt.sh (IMP-048 SCOPE-3),
# which extends these guarantees one level down to individual test leaves. One
# rule, one implementation, two consumers.
# shellcheck source=occurrence-identity-lib.sh
. "$SCRIPT_DIR/occurrence-identity-lib.sh"

SPEC_DIR=""
CURSOR=""
FORMAT="text"
DRY_RUN="false"
MAX_ITERATIONS=10
PLAN_NAMES=()
PLAN_COMMANDS=()
PLAN_KINDS=()
CHANGED_FILES=()
MBE_POSTURE="off"
MBE_STORE_ROOT=""
MBE_ACTION_BINDINGS=()
MBE_CONSUMPTION_BINDINGS=()
MBE_REPO_ROOT=""
MBE_AUTHORITY_FILE=""
MBE_SESSION_ID=""
MBE_SESSION_CONTROL_FILE=""
MBE_PACKET_FILE=""
MBE_FEATURE_DIR=""
MBE_CANDIDATE_REPO=""
MBE_RECEIPT_FILE=""
MBE_SESSION_FILE=""

usage() {
  sed -n '38,75p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

add_plan_entry() {
  local kind="$1" raw="$2"
  case "$raw" in
    *=*) ;;
    *) die_usage "--$kind expects <name>=<command> (got: $raw)" ;;
  esac
  PLAN_KINDS+=("$kind")
  PLAN_NAMES+=("${raw%%=*}")
  PLAN_COMMANDS+=("${raw#*=}")
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --spec-dir) SPEC_DIR="${2:-}"; shift 2 ;;
    --cursor) CURSOR="${2:-}"; shift 2 ;;
    --phase) add_plan_entry chained "${2:-}"; shift 2 ;;
    --independent) add_plan_entry independent "${2:-}"; shift 2 ;;
    --changed-file) CHANGED_FILES+=("${2:-}"); shift 2 ;;
    --max-iterations) MAX_ITERATIONS="${2:-}"; shift 2 ;;
    --format) FORMAT="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN="true"; shift ;;
    --mbe-posture) MBE_POSTURE="${2:-}"; shift 2 ;;
    --mbe-store-root) MBE_STORE_ROOT="${2:-}"; shift 2 ;;
    --mbe-action) MBE_ACTION_BINDINGS+=("${2:-}"); shift 2 ;;
    --mbe-permit-consumption) MBE_CONSUMPTION_BINDINGS+=("${2:-}"); shift 2 ;;
    --mbe-repo-root) MBE_REPO_ROOT="${2:-}"; shift 2 ;;
    --mbe-authority-file) MBE_AUTHORITY_FILE="${2:-}"; shift 2 ;;
    --mbe-session-id) MBE_SESSION_ID="${2:-}"; shift 2 ;;
    --mbe-session-control-file) MBE_SESSION_CONTROL_FILE="${2:-}"; shift 2 ;;
    --mbe-packet-file) MBE_PACKET_FILE="${2:-}"; shift 2 ;;
    --mbe-feature-dir) MBE_FEATURE_DIR="${2:-}"; shift 2 ;;
    --mbe-candidate-repo) MBE_CANDIDATE_REPO="${2:-}"; shift 2 ;;
    --mbe-receipt-file) MBE_RECEIPT_FILE="${2:-}"; shift 2 ;;
    --mbe-session-file) MBE_SESSION_FILE="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    --skip* | --force* | --ignore* | --replay* | --assume*)
      printf '%s: "%s" does not exist. An occurrence is resolved by running it.\n' "$NAME" "$1" >&2
      exit 2
      ;;
    *) die_usage "unknown argument: $1" ;;
  esac
done

[[ -n "$SPEC_DIR" ]] || die_usage "--spec-dir is required"
[[ -d "$SPEC_DIR" ]] || die_usage "spec dir not found: $SPEC_DIR"
[[ "${#PLAN_NAMES[@]}" -gt 0 ]] || die_usage "at least one --phase or --independent is required"
[[ "$MAX_ITERATIONS" =~ ^[0-9]+$ ]] || die_usage "--max-iterations must be a non-negative integer"
case "$FORMAT" in
  text | json) ;;
  *) die_usage "--format must be text or json (got: $FORMAT)" ;;
esac
case "$MBE_POSTURE" in
  off | shadow | advisory | reference-enforce) ;;
  *) die_usage "--mbe-posture must be off, shadow, advisory, or reference-enforce (got: $MBE_POSTURE)" ;;
esac
if [[ "$MBE_POSTURE" == "reference-enforce" ]]; then
  [[ -n "$MBE_STORE_ROOT" && -d "$MBE_STORE_ROOT" ]] || die_usage "reference-enforce requires an existing --mbe-store-root"
  # IMP-056 SCOPE-6: reference-enforce dispatches through the canonical
  # gateway, never the broker directly, so every invariant the gateway
  # composes must have its input supplied here too. There is no reduced
  # form -- an orchestrator that cannot supply these is not exempt, it
  # simply cannot use reference-enforce.
  for pair in "mbe-repo-root:$MBE_REPO_ROOT" "mbe-authority-file:$MBE_AUTHORITY_FILE" \
    "mbe-session-id:$MBE_SESSION_ID" "mbe-session-control-file:$MBE_SESSION_CONTROL_FILE" \
    "mbe-packet-file:$MBE_PACKET_FILE" "mbe-feature-dir:$MBE_FEATURE_DIR" \
    "mbe-candidate-repo:$MBE_CANDIDATE_REPO" "mbe-receipt-file:$MBE_RECEIPT_FILE" \
    "mbe-session-file:$MBE_SESSION_FILE"; do
    [[ -n "${pair#*:}" ]] || die_usage "reference-enforce requires --${pair%%:*}"
  done
fi

command -v python3 >/dev/null 2>&1 || {
  printf '%s: python3 is required\n' "$NAME" >&2
  exit 2
}
if [[ "$MBE_POSTURE" == "reference-enforce" ]]; then
  command -v jq >/dev/null 2>&1 || {
    printf '%s: jq is required for reference-enforce (gateway dispatch)\n' "$NAME" >&2
    exit 2
  }
fi

[[ -n "$CURSOR" ]] || CURSOR="$SPEC_DIR/.phase-cursor.json"

# --- occurrence identity ---------------------------------------------------
# Positional, and assigned BEFORE anything is read from the cursor, so the ids
# a resume compares against are the same ids the original run produced. The rule
# itself lives in occurrence-identity-lib.sh; this is one of its two consumers.
OCCURRENCE_IDS=()
while IFS= read -r _occ_id; do
  [[ -n "$_occ_id" ]] || continue
  OCCURRENCE_IDS+=("$_occ_id")
done < <(occurrence_ids_for "${PLAN_NAMES[@]}")

# --- cursor read -----------------------------------------------------------
CURSOR_JSON='{"schemaVersion":1,"occurrences":[],"iterations":[]}'
if [[ -f "$CURSOR" ]]; then
  CURSOR_JSON="$(cat "$CURSOR")"
fi

# An outcome that RESOLVES an occurrence. A failure and a BLOCKED_NOT_RUN both
# leave the occurrence outstanding, which is what makes resume land on them.
# The closed set is owned by occurrence-identity-lib.sh so the leaf coordinator
# cannot drift from the phase coordinator on what "resolved" means.
accepting_outcome() { occurrence_resolving_outcome "$1"; }

cursor_outcome() {
  CURSOR_JSON="$CURSOR_JSON" OCC="$1" python3 -c '
import json, os, sys
try:
    data = json.loads(os.environ["CURSOR_JSON"])
except Exception:
    sys.exit(0)
for row in data.get("occurrences") or []:
    if row.get("occurrenceId") == os.environ["OCC"]:
        print(row.get("outcome") or "")
        break
'
}

ITERATION="$(CURSOR_JSON="$CURSOR_JSON" python3 -c '
import json, os
try:
    data = json.loads(os.environ["CURSOR_JSON"])
except Exception:
    data = {}
print(len(data.get("iterations") or []) + 1)
')"

# --- resume point ----------------------------------------------------------
# The FIRST occurrence the cursor has not accepted. Not the first phase whose
# NAME is unfamiliar, which is what made a repeated phase indistinguishable.
RESUME_INDEX=-1
for i in "${!OCCURRENCE_IDS[@]}"; do
  prior="$(cursor_outcome "${OCCURRENCE_IDS[$i]}")"
  if ! accepting_outcome "$prior"; then
    RESUME_INDEX="$i"
    break
  fi
done

CHANGED_ARGS=()
for changed in ${CHANGED_FILES[@]+"${CHANGED_FILES[@]}"}; do
  CHANGED_ARGS+=(--changed-file "$changed")
done

CHANGED_LIST=""
if [[ "${#CHANGED_FILES[@]}" -gt 0 ]]; then
  CHANGED_LIST="$(mktemp "${TMPDIR:-/tmp}/phase-coordinator-changed.XXXXXX")"
  printf '%s\n' "${CHANGED_FILES[@]}" > "$CHANGED_LIST"
fi
cleanup() { [[ -n "$CHANGED_LIST" ]] && rm -f "$CHANGED_LIST"; }
trap cleanup EXIT INT TERM

# --- resolution ------------------------------------------------------------
RESULT_IDS=()
RESULT_OUTCOMES=()
RESULT_REASONS=()
RESULT_EXITS=()

binding_path() {
  local occurrence="$1"
  shift
  local binding
  for binding in "$@"; do
    if [[ "${binding%%=*}" == "$occurrence" ]]; then
      printf '%s' "${binding#*=}"
      return 0
    fi
  done
  return 1
}

chained_failed=""

for i in "${!OCCURRENCE_IDS[@]}"; do
  occ="${OCCURRENCE_IDS[$i]}"
  phase="${PLAN_NAMES[$i]}"
  command_line="${PLAN_COMMANDS[$i]}"
  kind="${PLAN_KINDS[$i]}"
  prior="$(cursor_outcome "$occ")"

  # NO REPLAY. An accepted occurrence is reported, not re-run.
  if accepting_outcome "$prior"; then
    RESULT_IDS+=("$occ")
    RESULT_OUTCOMES+=("ACCEPTED")
    RESULT_REASONS+=("already accepted as $prior in a prior iteration; not replayed")
    RESULT_EXITS+=("0")
    continue
  fi

  # BLOCKED_NOT_RUN. Only CHAINED phases depend on the failed prerequisite;
  # independent phases still execute so their diagnostics survive.
  if [[ -n "$chained_failed" && "$kind" == "chained" ]]; then
    RESULT_IDS+=("$occ")
    RESULT_OUTCOMES+=("BLOCKED_NOT_RUN")
    RESULT_REASONS+=("prerequisite $chained_failed did not pass; this dependent was never executed")
    RESULT_EXITS+=("")
    continue
  fi

  # PHASE RELEVANCE drives ACTUAL execution here, not a report.
  relevance_verdict="run"
  relevance_reason="relevance resolver unavailable; defaulting to run"
  if [[ -x "$RELEVANCE" || -f "$RELEVANCE" ]]; then
    relevance_out="$(bash "$RELEVANCE" --phase "$phase" --spec-dir "$SPEC_DIR" \
      ${CHANGED_LIST:+--changed-surface-file "$CHANGED_LIST"} --runner "$NAME" 2>/dev/null || true)"
    if [[ -n "$relevance_out" ]]; then
      relevance_verdict="$(printf '%s\n' "$relevance_out" | sed -n 's/^verdict=//p' | head -n 1)"
      relevance_reason="$(printf '%s\n' "$relevance_out" | sed -n 's/^reason=//p' | head -n 1)"
      [[ -n "$relevance_verdict" ]] || relevance_verdict="run"
    fi
  fi

  # TEST IMPACT drives ACTUAL execution here too (IMP-047 S-E, criterion 13).
  # `test-impact-plan.sh` shipped with no production consumer at all: the ledger
  # named `framework-validate.sh`, which only SCHEDULED its selftest. A resolver
  # nothing executes is a claim, not a behaviour, so the coordinator consumes it
  # for real. It is a no-op when a project declares no `testImpact` section, and
  # it can only ADD work: a full-suite requirement OVERRIDES a relevance skip and
  # never creates one, because an impact map must not be able to delete a phase.
  impact_full_suite="false"
  if [[ -f "$TEST_IMPACT" && -n "$CHANGED_LIST" ]]; then
    impact_out="$(bash "$TEST_IMPACT" --changed-file-list "$CHANGED_LIST" --format json 2>/dev/null || true)"
    [[ "$impact_out" == *'"fullSuiteRequired": true'* ]] && impact_full_suite="true"
  fi

  if [[ "$relevance_verdict" == "skip" && "$impact_full_suite" != "true" ]]; then
    RESULT_IDS+=("$occ")
    RESULT_OUTCOMES+=("SKIPPED_IRRELEVANT")
    RESULT_REASONS+=("${relevance_reason:-not relevant to this scope}")
    RESULT_EXITS+=("")
    continue
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    RESULT_IDS+=("$occ")
    RESULT_OUTCOMES+=("PENDING")
    RESULT_REASONS+=("dry run; would execute: $command_line")
    RESULT_EXITS+=("")
    [[ "$kind" == "chained" ]] && chained_failed="$occ"
    continue
  fi

  # ACTUAL EXECUTION. Absent/off, shadow, and advisory retain the exact legacy
  # path. Only the explicit repository-reference posture can interpose, and it
  # consumes action/permit files created by admission rather than minting its
  # own authority. Native host interception remains unsupported.
  rc=0
  if [[ "$MBE_POSTURE" == "reference-enforce" ]]; then
    action_file="$(binding_path "$occ" "${MBE_ACTION_BINDINGS[@]}" || true)"
    consumption_file="$(binding_path "$occ" "${MBE_CONSUMPTION_BINDINGS[@]}" || true)"
    if [[ -z "$action_file" || ! -f "$action_file" || -z "$consumption_file" || ! -f "$consumption_file" ]]; then
      printf '%s: reference-enforce occurrence %s requires existing action and permit-consumption files\n' "$NAME" "$occ" >&2
      rc=3
    else
      # IMP-056 SCOPE-6: dispatches through the canonical gateway, never the
      # broker directly. permit_id is read from the SAME consumption file
      # already bound to this occurrence, not a separate binding -- it is
      # the one fact the gateway needs that this file already carries.
      occ_permit_id="$(jq -r '.permit_id // .permitId // ""' "$consumption_file")"
      if [[ -z "$occ_permit_id" ]]; then
        printf '%s: reference-enforce occurrence %s permit-consumption file has no permit id\n' "$NAME" "$occ" >&2
        rc=3
      else
        bash "$GATEWAY" dispatch \
          --repo-root "$MBE_REPO_ROOT" --store-root "$MBE_STORE_ROOT" --authority-file "$MBE_AUTHORITY_FILE" \
          --session-id "$MBE_SESSION_ID" --session-control-file "$MBE_SESSION_CONTROL_FILE" \
          --packet-file "$MBE_PACKET_FILE" --feature-dir "$MBE_FEATURE_DIR" --candidate-repo "$MBE_CANDIDATE_REPO" \
          --receipt-file "$MBE_RECEIPT_FILE" --session-file "$MBE_SESSION_FILE" \
          --permit-id "$occ_permit_id" --action "$action_file" --permit-consumption "$consumption_file" || rc=$?
      fi
    fi
  else
    BUBBLES_PHASE_OCCURRENCE="$occ" bash -c "$command_line" || rc=$?
  fi
  RESULT_IDS+=("$occ")
  RESULT_EXITS+=("$rc")
  if [[ "$rc" -eq 0 ]]; then
    RESULT_OUTCOMES+=("RAN_PASS")
    RESULT_REASONS+=("executed and passed")
  else
    RESULT_OUTCOMES+=("RAN_FAIL")
    RESULT_REASONS+=("executed and failed with exit $rc")
    [[ "$kind" == "chained" ]] && chained_failed="$occ"
  fi
done

# --- scenario states -------------------------------------------------------
# Consumed, never re-derived. A second derivation would be a second answer.
SCENARIO_JSON="null"
if [[ -f "$SCENARIO_RESOLVER" && -f "$SPEC_DIR/scenario-manifest.json" ]]; then
  SCENARIO_JSON="$(bash "$SCENARIO_RESOLVER" --spec-dir "$SPEC_DIR" --format json \
    ${CHANGED_ARGS[@]+"${CHANGED_ARGS[@]}"} 2>/dev/null || echo 'null')"
  printf '%s' "$SCENARIO_JSON" | python3 -c 'import json,sys; json.load(sys.stdin)' 2>/dev/null || SCENARIO_JSON="null"
fi

# --- cursor write, plan fidelity, verdict ----------------------------------
CURSOR_JSON="$CURSOR_JSON" \
  CURSOR_PATH="$CURSOR" \
  SPEC_DIR="$SPEC_DIR" \
  ITERATION="$ITERATION" \
  MAX_ITERATIONS="$MAX_ITERATIONS" \
  RESUME_INDEX="$RESUME_INDEX" \
  DRY_RUN="$DRY_RUN" \
  FORMAT="$FORMAT" \
  SCENARIO_JSON="$SCENARIO_JSON" \
  IDS="$(printf '%s\n' ${RESULT_IDS[@]+"${RESULT_IDS[@]}"})" \
  OUTCOMES="$(printf '%s\n' ${RESULT_OUTCOMES[@]+"${RESULT_OUTCOMES[@]}"})" \
  REASONS="$(printf '%s\n' ${RESULT_REASONS[@]+"${RESULT_REASONS[@]}"})" \
  EXITS="$(printf '%s\n' ${RESULT_EXITS[@]+"${RESULT_EXITS[@]}"})" \
  python3 - <<'PY'
import json, os, sys, datetime

def lines(name):
    raw = os.environ.get(name, '')
    out = raw.split('\n')
    while out and out[-1] == '':
        out.pop()
    return out

ids = lines('IDS')
outcomes = lines('OUTCOMES')
reasons = lines('REASONS')
exits = lines('EXITS')
while len(exits) < len(ids):
    exits.append('')

cursor_path = os.environ['CURSOR_PATH']
iteration = int(os.environ['ITERATION'])
max_iterations = int(os.environ['MAX_ITERATIONS'])
resume_index = int(os.environ['RESUME_INDEX'])
dry_run = os.environ['DRY_RUN'] == 'true'
fmt = os.environ['FORMAT']

try:
    cursor = json.loads(os.environ.get('CURSOR_JSON') or '{}')
except Exception:
    cursor = {}
cursor.setdefault('schemaVersion', 1)
cursor.setdefault('occurrences', [])
cursor.setdefault('iterations', [])
cursor['specDir'] = os.environ['SPEC_DIR']

ACCEPTING = {'RAN_PASS', 'SKIPPED_IRRELEVANT', 'ACCEPTED'}

rows = []
for i, occ in enumerate(ids):
    rows.append({
        'occurrenceId': occ,
        'phase': occ.rsplit('#', 1)[0],
        'occurrence': int(occ.rsplit('#', 1)[1]),
        'outcome': outcomes[i],
        'reason': reasons[i],
        'exitCode': int(exits[i]) if exits[i].strip() else None,
    })

# Accepted occurrences are recorded ONCE and never rewritten. Rewriting a prior
# outcome is how a failed run becomes a passing history.
existing = {r.get('occurrenceId'): r for r in cursor['occurrences']}
if not dry_run:
    ts = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    for row in rows:
        if row['outcome'] == 'ACCEPTED':
            continue
        record = dict(row)
        record['iteration'] = iteration
        record['ts'] = ts
        if row['occurrenceId'] in existing:
            idx = cursor['occurrences'].index(existing[row['occurrenceId']])
            cursor['occurrences'][idx] = record
        else:
            cursor['occurrences'].append(record)

# PLAN FIDELITY. Recorded every iteration, whether or not anything passed.
planned = [r['occurrenceId'] for r in rows]
recorded = [r.get('occurrenceId') for r in cursor['occurrences']]
outstanding = [r['occurrenceId'] for r in rows if r['outcome'] not in ACCEPTING]
fidelity = {
    'iteration': iteration,
    'plannedOccurrences': len(planned),
    'resolvedOccurrences': len([r for r in rows if r['outcome'] in ACCEPTING]),
    'outstandingOccurrences': outstanding,
    'blockedNotRun': [r['occurrenceId'] for r in rows if r['outcome'] == 'BLOCKED_NOT_RUN'],
    'failed': [r['occurrenceId'] for r in rows if r['outcome'] == 'RAN_FAIL'],
    'notReplayed': [r['occurrenceId'] for r in rows if r['outcome'] == 'ACCEPTED'],
    'offPlanRecords': sorted(set(recorded) - set(planned)),
    'resumedAt': planned[resume_index] if 0 <= resume_index < len(planned) else None,
    'dryRun': dry_run,
}
if not dry_run:
    cursor['iterations'].append(fidelity)
    with open(cursor_path, 'w', encoding='utf-8') as fh:
        json.dump(cursor, fh, indent=2)
        fh.write('\n')

# HONEST EXHAUSTION. Running out of budget with work outstanding is a failure,
# and it is named as one. A loop that reports success on exhaustion converts
# "we never finished" into "we finished".
exhausted = bool(outstanding) and iteration >= max_iterations
complete = not outstanding

try:
    scenario = json.loads(os.environ.get('SCENARIO_JSON') or 'null')
except Exception:
    scenario = None

out = {
    'specDir': os.environ['SPEC_DIR'],
    'cursor': cursor_path,
    'iteration': iteration,
    'maxIterations': max_iterations,
    'resumedAt': fidelity['resumedAt'],
    'occurrences': rows,
    'planFidelity': fidelity,
    'scenarioStates': scenario,
    'complete': complete,
    'exhausted': exhausted,
}

if fmt == 'json':
    print(json.dumps(out, indent=2))
else:
    print('phase-coordinator: %s' % out['specDir'])
    print('  iteration %d of %d, resuming at %s' % (iteration, max_iterations, fidelity['resumedAt'] or '<nothing outstanding>'))
    for row in rows:
        suffix = '' if row['exitCode'] is None else ' (exit %d)' % row['exitCode']
        print('  %-16s %-18s %s%s' % (row['occurrenceId'], row['outcome'], row['reason'], suffix))
    print('  plan fidelity: %d/%d resolved, %d blocked, %d failed, %d not replayed'
          % (fidelity['resolvedOccurrences'], fidelity['plannedOccurrences'],
             len(fidelity['blockedNotRun']), len(fidelity['failed']), len(fidelity['notReplayed'])))
    if exhausted:
        print('  EXHAUSTED: the iteration budget ran out with %d occurrence(s) outstanding' % len(outstanding))
    elif not complete:
        print('  INCOMPLETE: %d occurrence(s) outstanding' % len(outstanding))

sys.exit(0 if complete else 1)
PY
exit $?
