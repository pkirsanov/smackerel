#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

spec_dir=""
repo_root=""
session_file=""
out_file=""
format="json"
schema="full"

usage() {
  cat <<'USAGE'
Usage: bash bubbles/scripts/retro-convergence-health.sh <specDir> [options]

Options:
  --session <path>       Session JSON or transcript/session-store export to inspect
  --repo-root <path>     Repository root containing .specify/memory
  --out <path>           Write the markdown Convergence Health section to this file
  --format json|markdown|both
                         Output format for stdout (default: json)
  --schema full|legacy   JSON schema for --format json (default: full)
                         full includes convergenceHealth recap/handoff SLOs;
                         legacy emits only the historical R7 metric keys.
  -h, --help             Show this help

Exit codes:
  0  convergence health SLO is pass or degraded
  1  convergence health SLO failed (Gate G090 breach)
  2  usage/runtime error
USAGE
}

normalize_path() {
  local value="$1"
  value="${value#./}"
  value="${value%/}"
  printf '%s\n' "$value"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --session)
      shift
      session_file="${1:?--session requires a path}"
      shift
      ;;
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --out)
      shift
      out_file="${1:?--out requires a path}"
      shift
      ;;
    --format)
      shift
      format="${1:?--format requires a value}"
      case "$format" in
        json|markdown|both) : ;;
        *)
          echo "retro-convergence-health: --format must be json, markdown, or both" >&2
          exit 2
          ;;
      esac
      shift
      ;;
    --schema)
      shift
      schema="${1:?--schema requires a value}"
      case "$schema" in
        full|legacy) : ;;
        *)
          echo "retro-convergence-health: --schema must be full or legacy" >&2
          exit 2
          ;;
      esac
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "retro-convergence-health: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$spec_dir" ]]; then
        echo "retro-convergence-health: multiple specDir arguments provided" >&2
        usage >&2
        exit 2
      fi
      spec_dir="$(normalize_path "$1")"
      shift
      ;;
  esac
done

if [[ -z "$spec_dir" ]]; then
  echo "retro-convergence-health: specDir argument is required" >&2
  usage >&2
  exit 2
fi

resolve_repo_root() {
  if [[ -n "$repo_root" ]]; then
    printf '%s\n' "$repo_root"
    return 0
  fi

  local dir="${PWD:-$(pwd)}"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  printf '%s\n' "$(cd "$SCRIPT_DIR/../.." && pwd)"
}

repo_root="$(resolve_repo_root)"
if [[ -z "$session_file" ]]; then
  session_file="$repo_root/.specify/memory/bubbles.session.json"
fi

if [[ ! -f "$session_file" ]]; then
  echo "retro-convergence-health: session JSON not found: $session_file" >&2
  exit 2
fi

if [[ ! -r "$session_file" ]]; then
  echo "retro-convergence-health: session JSON is not readable: $session_file" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "retro-convergence-health: jq is required" >&2
  exit 2
fi

if ! jq -e . "$session_file" >/dev/null 2>&1; then
  echo "retro-convergence-health: malformed JSON: $session_file" >&2
  exit 2
fi

# ── v4.1.0: executionRuntime skip (Gate G090 refinement) ─────────────────
# Convergence health metrics (recap/handoff/summarize ratios, snapshot
# completeness) are only meaningful when the work was driven by an
# orchestrated loop (sprint, goal-loop, workflow). For manual or
# direct-implement runtimes those signals are not produced — refusing to
# pass G090 in that case is a false positive.
#
# executionRuntime is read from (in priority order):
#   1. session JSON top-level `executionRuntime`
#   2. last entry under `runs[]` with field `runtime`
#   3. spec-dir state.json `executionRuntime`
exec_runtime="$(jq -r '
  .executionRuntime //
  (.runs // [] | last.runtime? // "") //
  (.execution.runtime? // "") //
  ""
' "$session_file" 2>/dev/null || true)"
if [[ -z "$exec_runtime" || "$exec_runtime" == "null" ]]; then
  spec_state_file="$repo_root/$spec_dir/state.json"
  if [[ -f "$spec_state_file" ]]; then
    exec_runtime="$(jq -r '.executionRuntime // .execution.runtime? // ""' "$spec_state_file" 2>/dev/null || true)"
  fi
fi
case "$exec_runtime" in
  manual|direct-implement|direct|adhoc|ad-hoc)
    skip_payload="$(jq -nc --arg rt "$exec_runtime" --arg spec "$spec_dir" '{
      avgLoopIterations: 0,
      maxConvergenceIterations: 0,
      compactionFrequency: 0,
      preExistingDeferralCount: 0,
      snapshotCompleteness: 1,
      convergenceHealth: {
        recapCount: 0,
        handoffCount: 0,
        summarizeHistoryCount: 0,
        turnCount: 0,
        slo: "skipped",
        skipReason: ("executionRuntime=" + $rt + " — convergence-loop metrics not applicable")
      },
      thresholds: {
        recapHandoffFailedWhenGreaterThan: 2,
        summarizeHistoryFailedWhenGreaterThan: 2,
        snapshotCompletenessRequired: 1
      }
    }')"
    if [[ -n "$out_file" ]]; then
      printf '## Convergence Health\n\nSpec: `%s`\nSLO: `skipped` (executionRuntime=%s — convergence-loop metrics not applicable to non-orchestrated runtime)\n' "$spec_dir" "$exec_runtime" > "$out_file"
    fi
    case "$format" in
      json|both) printf '%s\n' "$skip_payload" ;;
      markdown)  printf '## Convergence Health\n\nSpec: `%s`\nSLO: `skipped` (executionRuntime=%s)\n' "$spec_dir" "$exec_runtime" ;;
    esac
    exit 0
    ;;
esac

# ── Gate G090 refinement: spec-attribution guard ─────────────────────────
# When a spec is certified inside a long-running or multi-spec orchestrator
# session, the session JSON holds convergence telemetry (recap/handoff/summarize
# strings, turn snapshots) produced for OTHER specs. If NO record is attributed
# to the spec under certification, the metrics_program below falls back to the
# whole file ([.]) and mis-attributes that cross-spec telemetry to this spec —
# the same class of false positive the executionRuntime skip above guards
# against ("signals not produced for THIS spec"). When there is nothing
# attributable to measure, skip instead of counting ambient cross-spec strings.
# ($spec_dir empty = repo/session-level invocation → keep whole-file behavior.)
spec_attributed_count="$(jq -r --arg spec "$spec_dir" '
  def norm: tostring | sub("^\\./"; "") | sub("/+$"; "");
  ($spec | norm) as $t |
  if $t == "" then 1
  else [ .. | objects
         | select(((.specDir // .featureDir // .spec // .feature // "") | norm) == $t) ]
       | length
  end
' "$session_file" 2>/dev/null || echo 0)"
if [[ "${spec_attributed_count:-0}" -eq 0 ]]; then
  skip_reason="no convergence records attributed to ${spec_dir} — session telemetry is ambient/cross-spec and not attributable to this spec"
  skip_payload="$(jq -nc --arg reason "$skip_reason" '{
    avgLoopIterations: 0,
    maxConvergenceIterations: 0,
    compactionFrequency: 0,
    preExistingDeferralCount: 0,
    snapshotCompleteness: 1,
    convergenceHealth: {
      recapCount: 0, handoffCount: 0, summarizeHistoryCount: 0, turnCount: 0,
      slo: "skipped", skipReason: $reason
    },
    thresholds: { recapHandoffFailedWhenGreaterThan: 2, summarizeHistoryFailedWhenGreaterThan: 2, snapshotCompletenessRequired: 1 }
  }')"
  if [[ -n "$out_file" ]]; then
    printf '## Convergence Health\n\nSpec: `%s`\nSLO: `skipped` (%s)\n' "$spec_dir" "$skip_reason" > "$out_file"
  fi
  case "$format" in
    json|both) printf '%s\n' "$skip_payload" ;;
    markdown)  printf '## Convergence Health\n\nSpec: `%s`\nSLO: `skipped` (%s)\n' "$spec_dir" "$skip_reason" ;;
  esac
  exit 0
fi

metrics_program='
  def norm: tostring | sub("^\\./"; "") | sub("/+$"; "");
  def spec_match($target):
    ($target == "") or (((.specDir // .featureDir // .spec // .feature // "") | norm) == $target);
  def related_records($target):
    ([.. | objects | select(spec_match($target))]) as $records |
    if ($records | length) > 0 then $records
    elif (has("turnSnapshots") or has("turns") or has("messages") or has("transcript")) then [.]
    else []
    end;
  def strings_from($records): [$records[] | .. | strings];
  def count_regex($records; $pattern):
    (strings_from($records) | map(select(test($pattern; "i"))) | length);
  def convergence_counts($records):
    [$records[] | .. | objects | (.iterationCount? // .convergenceIterations?) | numbers];
  def envelope_records($records):
    [$records[] | .. | objects | select(has("rawSizeBytes") or has("incomingMessage") or has("rawPointer") or has("compactedAt"))];
  def snapshot_records($records):
    [$records[] | .. | objects | select(
      has("turnNumber") or has("turnIndex") or
      has("runStartedAt") or has("startedAt") or has("phaseStartedAt") or has("startAt") or has("startTime") or
      has("runCompletedAt") or has("completedAt") or has("phaseCompletedAt") or has("endAt") or has("endTime")
    )];
  def message_records($records):
    [$records[] | .. | objects | select(has("role") or has("message") or has("content"))];
  def state_snapshot_mode:
    ((.mode? // "") == "start" or (.mode? // "") == "end");
  def state_snapshot_key:
    [
      ((.phase? // "") | tostring),
      (((.scopeId? // .scopeID? // .scope? // .specDir? // .featureDir? // .spec? // .feature? // "") | norm)),
      ((.agent? // "") | tostring)
    ] | @json;
  def start_present:
    ((.runStartedAt? // .startedAt? // .phaseStartedAt? // .startAt? // .startTime?) != null);
  def end_present:
    ((.runCompletedAt? // .completedAt? // .phaseCompletedAt? // .endAt? // .endTime?) != null);
  def mode_snapshot_groups($snapshots):
    [$snapshots[] | select(state_snapshot_mode) | . + {__stateSnapshotKey: state_snapshot_key}]
    | group_by(.__stateSnapshotKey)
    | map({
        startCount: (map(select((.mode? // "") == "start")) | length),
        endCount: (map(select((.mode? // "") == "end")) | length)
      });
  def logical_snapshot_counts($snapshots):
    ($snapshots | map(select(state_snapshot_mode | not))) as $classicSnapshots |
    (mode_snapshot_groups($snapshots)) as $modeGroups |
    ($classicSnapshots | map(select(start_present and end_present)) | length) as $classicComplete |
    ($classicSnapshots | length) as $classicTotal |
    ($modeGroups | map(([.startCount, .endCount] | min)) | add // 0) as $modeComplete |
    ($modeGroups | map(([.startCount, .endCount] | max)) | add // 0) as $modeTotal |
    {complete: ($classicComplete + $modeComplete), total: ($classicTotal + $modeTotal)};
  def average($values):
    if ($values | length) == 0 then 0 else (($values | add) / ($values | length)) end;
  def ratio($num; $den; $empty):
    if $den == 0 then $empty else ($num / $den) end;
  ($spec | norm) as $specNorm |
  related_records($specNorm) as $records |
  convergence_counts($records) as $loops |
  envelope_records($records) as $envelopes |
  snapshot_records($records) as $snapshots |
  message_records($records) as $messages |
  logical_snapshot_counts($snapshots) as $snapshotCounts |
  ([ $records[] | .. | objects | .turnCount? | numbers ] | max // (if ($snapshots | length) > 0 then ($snapshots | length) else ($messages | length) end)) as $turnCount |
  ($envelopes | map(select((.compactedAt? // null) != null)) | length) as $compactionEvents |
  ($snapshotCounts.complete) as $completeSnapshots |
  count_regex($records; "(^|[^A-Za-z0-9_])recap([^A-Za-z0-9_]|$)") as $recapCount |
  count_regex($records; "(^|[^A-Za-z0-9_])handoff([^A-Za-z0-9_]|$)") as $handoffCount |
  count_regex($records; "summarizeConversationHistory|summarize[[:space:]_-]+conversation[[:space:]_-]+history") as $summarizeHistoryCount |
  count_regex($records; "pre-existing failure|pre-existing test failure|carried forward|out of session scope|previous-session failure|not introduced by this spec") as $preExistingDeferralCount |
  (ratio($completeSnapshots; $snapshotCounts.total; 1)) as $snapshotCompleteness |
  (ratio($compactionEvents; ($envelopes | length); 1)) as $compactionFrequency |
  (($recapCount + $handoffCount) > 2 or $summarizeHistoryCount > 2 or $snapshotCompleteness < 1) as $failed |
  (($recapCount + $handoffCount) == 2 or $summarizeHistoryCount > 0) as $degraded |
  (if $failed then "failed" elif $degraded then "degraded" else "pass" end) as $slo |
  {
    avgLoopIterations: average($loops),
    maxConvergenceIterations: (if ($loops | length) == 0 then 0 else ($loops | max) end),
    compactionFrequency: $compactionFrequency,
    preExistingDeferralCount: $preExistingDeferralCount,
    snapshotCompleteness: $snapshotCompleteness,
    convergenceHealth: {
      recapCount: $recapCount,
      handoffCount: $handoffCount,
      summarizeHistoryCount: $summarizeHistoryCount,
      turnCount: $turnCount,
      slo: $slo
    },
    thresholds: {
      recapHandoffFailedWhenGreaterThan: 2,
      summarizeHistoryFailedWhenGreaterThan: 2,
      snapshotCompletenessRequired: 1
    }
  }
'

metrics_json="$(jq -c --arg spec "$spec_dir" "$metrics_program" "$session_file")"

json_for_stdout() {
  if [[ "$schema" == "legacy" ]]; then
    jq -c '{avgLoopIterations, maxConvergenceIterations, compactionFrequency, preExistingDeferralCount, snapshotCompleteness}' <<< "$metrics_json"
  else
    printf '%s\n' "$metrics_json"
  fi
}

render_markdown() {
  jq -r --arg spec "$spec_dir" '
    "## Convergence Health",
    "",
    "Spec: `\($spec)`",
    "SLO: `\(.convergenceHealth.slo)`",
    "",
    "| Signal | Value | Threshold |",
    "|--------|-------|-----------|",
    "| Recap count | \(.convergenceHealth.recapCount) | recap + handoff <= 2 |",
    "| Handoff count | \(.convergenceHealth.handoffCount) | recap + handoff <= 2 |",
    "| summarizeConversationHistory count | \(.convergenceHealth.summarizeHistoryCount) | <= 2 |",
    "| Turn count | \(.convergenceHealth.turnCount) | observed |",
    "| Average loop iterations | \(.avgLoopIterations) | informational |",
    "| Max convergence iterations | \(.maxConvergenceIterations) | <= configured cap |",
    "| Compaction frequency | \(.compactionFrequency) | informational |",
    "| Pre-existing deferral count | \(.preExistingDeferralCount) | 0 |",
    "| Snapshot completeness | \(.snapshotCompleteness) | 1.0 |"
  ' <<< "$metrics_json"
}

if [[ -n "$out_file" ]]; then
  markdown_output="$(render_markdown)"
  printf '%s\n' "$markdown_output" > "$out_file"
fi

case "$format" in
  json)
    json_for_stdout
    ;;
  markdown)
    render_markdown
    ;;
  both)
    render_markdown
    echo
    echo '```json'
    json_for_stdout
    echo '```'
    ;;
esac

slo="$(jq -r '.convergenceHealth.slo' <<< "$metrics_json")"
if [[ "$slo" == "failed" ]]; then
  recap_handoff_total="$(jq -r '(.convergenceHealth.recapCount + .convergenceHealth.handoffCount)' <<< "$metrics_json")"
  summarize_total="$(jq -r '.convergenceHealth.summarizeHistoryCount' <<< "$metrics_json")"
  snapshot_completeness="$(jq -r '.snapshotCompleteness' <<< "$metrics_json")"
  echo "G090 retro_convergence_health_evidence_gate failed: slo=failed" >&2
  echo "metric recapHandoffInvocationCount=$recap_handoff_total threshold<=2" >&2
  echo "metric summarizeHistoryCount=$summarize_total threshold<=2" >&2
  echo "metric snapshotCompleteness=$snapshot_completeness threshold=1" >&2
  if [[ "$recap_handoff_total" -gt 2 ]]; then
    echo "P0 convergence regression: recap/handoff invocations exceeded 2" >&2
  fi
  exit 1
fi

exit 0