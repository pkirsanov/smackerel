#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

since_days=7
spec_filter=""
group_by="phase"
repo_root=""
session_file=""
now_iso=""

usage() {
  cat <<'USAGE'
Usage: bash bubbles/scripts/validation-latency-report.sh [options]

Options:
  --since <days>       Include records completed within the last N days (default 7)
  --spec <path>        Include only records for one spec path
  --group <mode>       Group by phase, agent, or phase-agent (default phase)
  --repo-root <path>   Repository root containing .specify/memory
  --session <path>     Session JSON file to inspect
  --now <iso8601>      Override current time for hermetic tests
  -h, --help           Show this help

Exit codes:
  0  report rendered cleanly, including empty or partially malformed histories
  1  reserved for future policy violations; this observability script never emits it today
  2  runtime or input error
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
    --since)
      shift
      since_days="${1:?--since requires days}"
      if ! [[ "$since_days" =~ ^[0-9]+$ ]]; then
        echo "validation-latency-report: --since requires a non-negative integer" >&2
        exit 2
      fi
      shift
      ;;
    --spec)
      shift
      spec_filter="$(normalize_path "${1:?--spec requires a path}")"
      shift
      ;;
    --group)
      shift
      group_by="${1:?--group requires a mode}"
      case "$group_by" in
        phase|agent|phase-agent) : ;;
        *)
          echo "validation-latency-report: --group must be phase, agent, or phase-agent" >&2
          exit 2
          ;;
      esac
      shift
      ;;
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --session)
      shift
      session_file="${1:?--session requires a path}"
      shift
      ;;
    --now)
      shift
      now_iso="${1:?--now requires an ISO-8601 timestamp}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "validation-latency-report: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

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

print_empty_report() {
  local reason="$1"
  echo "# Validation Latency Report"
  echo
  echo "Repo root: $repo_root"
  echo "Session file: $session_file"
  echo "Since days: $since_days"
  echo "Spec filter: ${spec_filter:-<all>}"
  echo "Group: $group_by"
  echo
  echo "| Phase | Agent | Spec | Count | P50 | P95 | Max | Budget | Within? |"
  echo "|-------|-------|------|-------|-----|-----|-----|--------|---------|"
  echo
  echo "$reason"
}

if [[ ! -f "$session_file" ]]; then
  print_empty_report "No session JSON found; no latency samples available."
  exit 0
fi

if [[ ! -r "$session_file" ]]; then
  echo "validation-latency-report: session JSON is not readable: $session_file" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "validation-latency-report: jq is required" >&2
  exit 2
fi

if ! jq -e . "$session_file" >/dev/null 2>&1; then
  echo "validation-latency-report: malformed JSON: $session_file" >&2
  exit 2
fi

summary_program='
  def parse_ts($v):
    if ($v | type) == "string" then (try (($v | gsub("\\.[0-9]+Z$"; "Z") | gsub("\\+00:00$"; "Z")) | fromdateiso8601) catch null) else null end;
  def record_count:
    ((.executionHistory // []) | length)
    + ((.turnSnapshots // []) | map(select(
        has("runStartedAt") or has("runCompletedAt") or has("startedAt") or
        has("completedAt") or has("phaseStartedAt") or has("phaseCompletedAt") or
        has("startAt") or has("endAt") or has("startTime") or has("endTime")
      )) | length);
  def phases($e):
    if (($e.phasesExecuted // null) | type) == "array" and (($e.phasesExecuted // []) | length) > 0 then
      ($e.phasesExecuted // [])
    elif (($e.phase // null) | type) == "string" then
      [$e.phase]
    elif (($e.currentPhase // null) | type) == "string" then
      [$e.currentPhase]
    else
      ["unknown"]
    end;
  def spec_of($e): ($e.specDir // $e.featureDir // $e.spec // $e.feature // "unknown");
  def agent_of($e): ($e.agent // "unknown");
  def event_from($source; $e):
    (parse_ts($e.runStartedAt // $e.startedAt // $e.phaseStartedAt // $e.startAt // $e.startTime)) as $start |
    (parse_ts($e.runCompletedAt // $e.completedAt // $e.phaseCompletedAt // $e.endAt // $e.endTime)) as $end |
    if $start != null and $end != null and $end >= $start then
      phases($e)[] as $phase |
      {
        source: $source,
        phase: $phase,
        agent: agent_of($e),
        spec: spec_of($e),
        endEpoch: $end,
        durationSeconds: ($end - $start)
      }
    else empty end;
  def events:
    [(.executionHistory // [])[] | event_from("executionHistory"; .)]
    + [(.turnSnapshots // [])[] | event_from("turnSnapshots"; .)];
  def norm: tostring | sub("/+$"; "");
  ($since | tonumber) as $sinceDays |
  (if $nowIso == "" then now else ($nowIso | fromdateiso8601) end) as $nowEpoch |
  (($nowEpoch - ($sinceDays * 86400)) | floor) as $cutoff |
  ($spec | norm) as $specNorm |
  events as $events |
  ($events | map(select(.endEpoch >= $cutoff and ($specNorm == "" or (.spec | norm) == $specNorm)))) as $filtered |
  {
    scannedRecords: record_count,
    validEvents: ($events | length),
    filteredEvents: ($filtered | length),
    skippedRecords: (record_count - ($events | length))
  }
  | "scannedRecords=\(.scannedRecords) validEvents=\(.validEvents) filteredEvents=\(.filteredEvents) skippedRecords=\(.skippedRecords)"
'

rows_program='
  def parse_ts($v):
    if ($v | type) == "string" then (try (($v | gsub("\\.[0-9]+Z$"; "Z") | gsub("\\+00:00$"; "Z")) | fromdateiso8601) catch null) else null end;
  def phases($e):
    if (($e.phasesExecuted // null) | type) == "array" and (($e.phasesExecuted // []) | length) > 0 then
      ($e.phasesExecuted // [])
    elif (($e.phase // null) | type) == "string" then
      [$e.phase]
    elif (($e.currentPhase // null) | type) == "string" then
      [$e.currentPhase]
    else
      ["unknown"]
    end;
  def spec_of($e): ($e.specDir // $e.featureDir // $e.spec // $e.feature // "unknown");
  def agent_of($e): ($e.agent // "unknown");
  def event_from($source; $e):
    (parse_ts($e.runStartedAt // $e.startedAt // $e.phaseStartedAt // $e.startAt // $e.startTime)) as $start |
    (parse_ts($e.runCompletedAt // $e.completedAt // $e.phaseCompletedAt // $e.endAt // $e.endTime)) as $end |
    if $start != null and $end != null and $end >= $start then
      phases($e)[] as $phase |
      {
        source: $source,
        phase: $phase,
        agent: agent_of($e),
        spec: spec_of($e),
        endEpoch: $end,
        durationSeconds: ($end - $start)
      }
    else empty end;
  def events:
    [(.executionHistory // [])[] | event_from("executionHistory"; .)]
    + [(.turnSnapshots // [])[] | event_from("turnSnapshots"; .)];
  def norm: tostring | sub("/+$"; "");
  def group_key($mode):
    if $mode == "agent" then .agent
    elif $mode == "phase-agent" then (.phase + "\u0001" + .agent)
    else .phase
    end;
  def pct($p):
    sort as $s |
    if ($s | length) == 0 then 0
    else (($s | length) - 1) as $last |
      (($last * $p / 100) | ceil) as $idx |
      $s[$idx]
    end;
  def fmt_seconds($seconds):
    ($seconds | floor) as $n |
    (($n / 3600) | floor) as $h |
    (((($n % 3600) / 60) | floor)) as $m |
    ($n % 60) as $s |
    if $h > 0 then "\($h)h\($m)m\($s)s"
    elif $m > 0 then "\($m)m\($s)s"
    else "\($s)s"
    end;
  def budget_seconds($phase):
    {
      bootstrap: 600,
      design: 900,
      plan: 900,
      implement: 1800,
      test: 900,
      regression: 900,
      validate: 600,
      audit: 600,
      docs: 600,
      finalize: 300
    }[$phase];
  ($since | tonumber) as $sinceDays |
  (if $nowIso == "" then now else ($nowIso | fromdateiso8601) end) as $nowEpoch |
  (($nowEpoch - ($sinceDays * 86400)) | floor) as $cutoff |
  ($spec | norm) as $specNorm |
  events
  | map(select(.endEpoch >= $cutoff and ($specNorm == "" or (.spec | norm) == $specNorm)))
  | sort_by(group_key($group))
  | group_by(group_key($group))
  | map(
      map(.durationSeconds) as $durations |
      ($durations | pct(50)) as $p50 |
      ($durations | pct(95)) as $p95 |
      ($durations | max) as $max |
      (if $group == "agent" then "all" else .[0].phase end) as $phaseOut |
      (if $group == "phase" then "all" else .[0].agent end) as $agentOut |
      (budget_seconds($phaseOut)) as $budget |
      {
        phase: $phaseOut,
        agent: $agentOut,
        spec: (if $specNorm == "" then "all" else $specNorm end),
        count: ($durations | length),
        p50: fmt_seconds($p50),
        p95: fmt_seconds($p95),
        max: fmt_seconds($max),
        budget: (if $budget == null then "n/a" else fmt_seconds($budget) end),
        within: (if $budget == null then "n/a" elif $p95 <= $budget then "yes" else "no" end)
      }
    )
  | .[]
  | [.phase, .agent, .spec, (.count | tostring), .p50, .p95, .max, .budget, .within]
  | @tsv
'

if ! summary_line="$(jq -r --arg since "$since_days" --arg spec "$spec_filter" --arg nowIso "$now_iso" "$summary_program" "$session_file")"; then
  echo "validation-latency-report: failed to summarize session JSON" >&2
  exit 2
fi

if ! rows="$(jq -r --arg since "$since_days" --arg spec "$spec_filter" --arg group "$group_by" --arg nowIso "$now_iso" "$rows_program" "$session_file")"; then
  echo "validation-latency-report: failed to render latency rows" >&2
  exit 2
fi

echo "# Validation Latency Report"
echo
echo "Repo root: $repo_root"
echo "Session file: $session_file"
echo "Since days: $since_days"
echo "Spec filter: ${spec_filter:-<all>}"
echo "Group: $group_by"
echo "$summary_line"
if [[ "$summary_line" =~ scannedRecords=([0-9]+)[[:space:]]validEvents=([0-9]+)[[:space:]]filteredEvents=([0-9]+)[[:space:]]skippedRecords=([0-9]+) ]]; then
  echo "Scanned records: ${BASH_REMATCH[1]}"
  echo "Valid durations: ${BASH_REMATCH[2]}"
  echo "Filtered durations: ${BASH_REMATCH[3]}"
  echo "Skipped records: ${BASH_REMATCH[4]}"
fi
echo
echo "| Phase | Agent | Spec | Count | P50 | P95 | Max | Budget | Within? |"
echo "|-------|-------|------|-------|-----|-----|-----|--------|---------|"

if [[ -n "$rows" ]]; then
  while IFS=$'\t' read -r phase agent spec count p50 p95 max budget within; do
    printf '| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n' \
      "$phase" "$agent" "$spec" "$count" "$p50" "$p95" "$max" "$budget" "$within"
  done <<< "$rows"
else
  echo "| <none> | <none> | ${spec_filter:-all} | 0 | n/a | n/a | n/a | n/a | n/a |"
  echo
  echo "No valid phase durations found for the selected filters."
fi

exit 0
