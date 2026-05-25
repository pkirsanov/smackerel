#!/usr/bin/env bash
# trajectory-inspector.sh
#
# Print a human-readable trajectory report from the active Bubbles
# session, recent turn snapshots, lessons, and per-spec state files.
#
# Sources read (when present):
#   - .specify/memory/bubbles.session.json      (active session + turnSnapshots)
#   - .specify/memory/sessions/*.json           (archived sessions)
#   - .specify/memory/lessons.md                (lessons-learned memory)
#   - specs/<spec>/state.json                   (per-spec execution state)
#
# Output sections:
#   1. Session summary (sessionId, agent, mode, featureDir, status, phase)
#   2. Phase progression (turnSnapshots[].phase counts and last few entries)
#   3. Scope progression (active spec scopes by status)
#   4. Recent lessons (last N lines of lessons.md)
#   5. Active specs (status + workflowMode for each specs/*/state.json)
#
# Args:
#   --session <id>      Inspect a specific session JSON file by sessionId
#                       (searches .specify/memory/bubbles.session.json and
#                        .specify/memory/sessions/*.json)
#   --last <N>          Show only the last N turn snapshots (default 10)
#   --health           Emit a single-line convergence health summary
#   --input <file>     Health-mode JSON input from retro-convergence-health.sh
#   --latency          Append the validation latency report
#   --since <days>     Latency report window when --latency is present
#   --spec <path>      Health/latency report spec filter
#   --format text|json  Output format (default text)
#   --repo-root <path>  Repo root (default: cwd resolved upward to a repo
#                       containing .specify/memory or the script's repo)
#   --verbose           Print extra diagnostics
#
# Always exits 0 when there is no active session — output is "(no active
# session)". Only exits non-zero on usage errors (exit 2).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

session_id_filter=""
last_n=10
format="text"
repo_root=""
verbose="false"
show_health="false"
health_input=""
show_latency="false"
latency_since="7"
spec_filter=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/trajectory-inspector.sh \
         [--session <id>] [--last <N>] [--format text|json] \
         [--repo-root <path>] [--verbose] [--health] [--input <file>] \
         [--latency] [--since <days>] [--spec <path>]

Print a human-readable trajectory report from the current Bubbles session,
turn snapshots, lessons, and per-spec state. Always exits 0 unless invoked
with bad arguments (exit 2).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --health)
      show_health="true"
      shift
      ;;
    --input)
      shift
      health_input="${1:?--input requires a file}"
      shift
      ;;
    --latency)
      show_latency="true"
      shift
      ;;
    --since)
      shift
      latency_since="${1:?--since requires days}"
      if ! [[ "$latency_since" =~ ^[0-9]+$ ]]; then
        echo "trajectory-inspector: --since requires a non-negative integer" >&2
        exit 2
      fi
      shift
      ;;
    --spec)
      shift
      spec_filter="${1:?--spec requires a path}"
      spec_filter="${spec_filter#./}"
      spec_filter="${spec_filter%/}"
      shift
      ;;
    --session)
      shift
      session_id_filter="${1:?--session requires an id}"
      shift
      ;;
    --last)
      shift
      last_n="${1:?--last requires N}"
      if ! [[ "$last_n" =~ ^[0-9]+$ ]]; then
        echo "trajectory-inspector: --last requires a positive integer" >&2
        exit 2
      fi
      shift
      ;;
    --format)
      shift
      format="${1:?--format requires a value}"
      case "$format" in text|json) : ;; *)
        echo "trajectory-inspector: --format must be 'text' or 'json'" >&2
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
    --verbose)
      verbose="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "trajectory-inspector: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Resolve repo root: prefer explicit, else walk upward from cwd looking
# for .specify/memory, else fall back to the script's repo.
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
[[ "$verbose" == "true" ]] && echo "trajectory-inspector: repo_root=$repo_root" >&2

session_dir="$repo_root/.specify/memory"
session_file="$session_dir/bubbles.session.json"
lessons_file="$session_dir/lessons.md"
specs_dir="$repo_root/specs"

# Pick the session JSON to inspect
pick_session_file() {
  if [[ -n "$session_id_filter" ]]; then
    # Search default + archived
    for f in "$session_file" "$session_dir"/sessions/*.json; do
      [[ -f "$f" ]] || continue
      if command -v jq >/dev/null 2>&1; then
        local id
        id="$(jq -r '.sessionId // empty' "$f" 2>/dev/null || true)"
        if [[ "$id" == "$session_id_filter" ]]; then
          printf '%s\n' "$f"
          return 0
        fi
      else
        if grep -Fq "\"sessionId\": \"$session_id_filter\"" "$f" 2>/dev/null \
            || grep -Fq "\"sessionId\":\"$session_id_filter\"" "$f" 2>/dev/null; then
          printf '%s\n' "$f"
          return 0
        fi
      fi
    done
    return 1
  fi
  if [[ -f "$session_file" ]]; then
    printf '%s\n' "$session_file"
    return 0
  fi
  return 1
}

active_session_file=""
if active_session_file="$(pick_session_file)"; then
  : # found
fi

# --- JSON helpers (graceful when jq is missing) --------------------------

have_jq=false
command -v jq >/dev/null 2>&1 && have_jq=true

as_int() {
  local value="${1:-0}"
  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "$value"
  elif [[ "$value" =~ ^[0-9]+[.][0-9]+$ ]]; then
    printf '%s\n' "${value%%.*}"
  else
    printf '0\n'
  fi
}

derive_health_status() {
  local raw_status="${1:-}"
  local compaction_invocations
  local recap_invocations
  local handoff_invocations
  local blocked_findings
  compaction_invocations="$(as_int "${2:-0}")"
  recap_invocations="$(as_int "${3:-0}")"
  handoff_invocations="$(as_int "${4:-0}")"
  blocked_findings="$(as_int "${5:-0}")"

  if [[ "$blocked_findings" -gt 0 ]]; then
    printf 'FAILED\n'
    return 0
  fi

  case "${raw_status,,}" in
    healthy|pass|passed|ok)
      printf 'HEALTHY\n'
      return 0
      ;;
    degraded|warn|warning)
      printf 'DEGRADED\n'
      return 0
      ;;
    failed|fail|failure|error)
      printf 'FAILED\n'
      return 0
      ;;
  esac

  if [[ $((recap_invocations + handoff_invocations)) -gt 2 ]]; then
    printf 'FAILED\n'
  elif [[ "$recap_invocations" -gt 0 || "$handoff_invocations" -gt 0 || "$compaction_invocations" -gt 0 ]]; then
    printf 'DEGRADED\n'
  else
    printf 'HEALTHY\n'
  fi
}

print_health_line() {
  local turn_count
  local compaction_invocations
  local recap_invocations
  local handoff_invocations
  local blocked_findings
  local raw_status
  local status

  turn_count="$(as_int "${1:-0}")"
  compaction_invocations="$(as_int "${2:-0}")"
  recap_invocations="$(as_int "${3:-0}")"
  handoff_invocations="$(as_int "${4:-0}")"
  blocked_findings="$(as_int "${5:-0}")"
  raw_status="${6:-}"
  status="$(derive_health_status "$raw_status" "$compaction_invocations" "$recap_invocations" "$handoff_invocations" "$blocked_findings")"

  printf 'Convergence Health: turnCount=%s compactionInvocations=%s recapInvocations=%s handoffInvocations=%s blockedFindings=%s status=%s\n' \
    "$turn_count" "$compaction_invocations" "$recap_invocations" "$handoff_invocations" "$blocked_findings" "$status"
}

health_from_json_input() {
  local input_file="$1"
  local resolved_input="$input_file"
  if [[ ! -f "$resolved_input" && -f "$repo_root/$input_file" ]]; then
    resolved_input="$repo_root/$input_file"
  fi

  if [[ ! -f "$resolved_input" || ! -r "$resolved_input" ]]; then
    print_health_line 0 0 0 0 1 failed
    return 0
  fi

  if ! $have_jq; then
    print_health_line 0 0 0 0 1 degraded
    return 0
  fi

  local metrics_line
  metrics_line="$(jq -r '
    def numeric($value):
      if ($value | type) == "number" then $value
      elif ($value | type) == "array" then ($value | length)
      elif ($value | type) == "object" and (($value.count? | type) == "number") then $value.count
      elif ($value | type) == "string" and ($value | test("^[0-9]+$")) then ($value | tonumber)
      else 0
      end;
    [
      numeric(.turnCount // .metrics.turnCount // .convergenceHealth.turnCount // 0),
      numeric(.compactionInvocations // .compactionEvents // .compactionCount // .convergenceHealth.compactionInvocations // .convergenceHealth.compactionEvents // .convergenceHealth.compactionCount // 0),
      numeric(.recapInvocations // .recapCount // .convergenceHealth.recapInvocations // .convergenceHealth.recapCount // 0),
      numeric(.handoffInvocations // .handoffCount // .convergenceHealth.handoffInvocations // .convergenceHealth.handoffCount // 0),
      numeric(.blockedFindings // .blockedFindingsCount // .convergenceHealth.blockedFindings // .convergenceHealth.blockedFindingsCount // 0),
      (.status // .healthStatus // .convergenceHealth.status // .convergenceHealth.slo // .slo // "")
    ] | @tsv
  ' "$resolved_input" 2>/dev/null || true)"

  if [[ -z "$metrics_line" ]]; then
    print_health_line 0 0 0 0 1 failed
    return 0
  fi

  local turn_count compaction_invocations recap_invocations handoff_invocations blocked_findings raw_status
  IFS=$'\t' read -r turn_count compaction_invocations recap_invocations handoff_invocations blocked_findings raw_status <<< "$metrics_line"
  print_health_line "$turn_count" "$compaction_invocations" "$recap_invocations" "$handoff_invocations" "$blocked_findings" "$raw_status"
}

blocked_findings_from_spec_state() {
  local target_spec="$1"
  local spec_state=""
  if [[ -n "$target_spec" && -f "$repo_root/$target_spec/state.json" ]]; then
    spec_state="$repo_root/$target_spec/state.json"
  fi

  if [[ -z "$spec_state" || ! -r "$spec_state" || "$have_jq" != "true" ]]; then
    printf '0\n'
    return 0
  fi

  jq -r '
    def unresolved_rework:
      [(.reworkQueue // [])[] | select(((.resolved // false) != true) and ((.status // "") != "resolved"))] | length;
    def pending_transitions:
      ((.transitionRequests // []) | length) + ((.execution.pendingTransitionRequests // []) | length);
    def blocked_arrays:
      ([.. | objects | .blockedFindings? | if type == "array" then length elif type == "number" then . else 0 end] | add // 0);
    unresolved_rework + pending_transitions + blocked_arrays
  ' "$spec_state" 2>/dev/null || printf '0\n'
}

health_from_session() {
  local target_spec="${spec_filter:-${feat:-}}"

  if [[ -z "${active_session_file:-}" || ! -f "$active_session_file" || "$have_jq" != "true" ]]; then
    local blocked_without_session
    blocked_without_session="$(blocked_findings_from_spec_state "$target_spec")"
    print_health_line 0 0 0 0 "$blocked_without_session" ""
    return 0
  fi

  local metrics_line
  metrics_line="$(jq -r --arg spec "$target_spec" '
    def norm: tostring | sub("^\\./"; "") | sub("/+$"; "");
    def object_matches($target):
      ($target == "") or (((.specDir // .featureDir // .spec // .feature // "") | norm) == $target);
    def related_records($target):
      if $target == "" then [.]
      elif object_matches($target) then [.]
      else ([.. | objects | select(object_matches($target))]) as $records |
      if ($records | length) > 0 then $records
      elif (has("turnSnapshots") or has("turns") or has("messages") or has("transcript") or has("executionHistory") or has("envelopesReceived")) then [.]
      else []
      end
      end;
    def strings_from($records): [$records[] | .. | strings];
    def count_regex($records; $pattern):
      (strings_from($records) | map(select(test($pattern; "i"))) | length);
    def envelope_records($records):
      [$records[] | .. | objects | select(has("rawSizeBytes") or has("incomingMessage") or has("rawPointer") or has("compactedAt"))];
    def snapshot_records($records):
      [$records[] | .. | objects | select(has("turnNumber") or has("turnIndex"))];
    def message_records($records):
      [$records[] | .. | objects | select(has("role") or has("message") or has("content"))];
    def blocked_arrays($records):
      ([ $records[] | .. | objects | .blockedFindings? | if type == "array" then length elif type == "number" then . else 0 end ] | add // 0);
    def blocked_statuses($records):
      [$records[] | .. | objects | select(((.outcome? // .status? // "") | tostring | ascii_downcase) == "blocked")] | length;
    related_records(($spec | norm)) as $records |
    envelope_records($records) as $envelopes |
    snapshot_records($records) as $snapshots |
    message_records($records) as $messages |
    ([ $records[] | .. | objects | .turnCount? | numbers ] | max // (if ($snapshots | length) > 0 then ($snapshots | length) else ($messages | length) end)) as $turnCount |
    ($envelopes | map(select((.compactedAt? // null) != null)) | length) as $compactionInvocations |
    count_regex($records; "(^|[^A-Za-z0-9_])recap([^A-Za-z0-9_]|$)") as $recapInvocations |
    count_regex($records; "(^|[^A-Za-z0-9_])handoff([^A-Za-z0-9_]|$)") as $handoffInvocations |
    (blocked_arrays($records) + blocked_statuses($records)) as $blockedFindings |
    [$turnCount, $compactionInvocations, $recapInvocations, $handoffInvocations, $blockedFindings, ""] | @tsv
  ' "$active_session_file" 2>/dev/null || true)"

  if [[ -z "$metrics_line" ]]; then
    metrics_line=$'0\t0\t0\t0\t0\t'
  fi

  local turn_count compaction_invocations recap_invocations handoff_invocations blocked_findings raw_status
  IFS=$'\t' read -r turn_count compaction_invocations recap_invocations handoff_invocations blocked_findings raw_status <<< "$metrics_line"
  local spec_blocked_findings
  spec_blocked_findings="$(blocked_findings_from_spec_state "$target_spec")"
  blocked_findings=$(( $(as_int "$blocked_findings") + $(as_int "$spec_blocked_findings") ))
  print_health_line "$turn_count" "$compaction_invocations" "$recap_invocations" "$handoff_invocations" "$blocked_findings" "$raw_status"
}

if [[ "$show_health" == "true" ]]; then
  if [[ -n "$health_input" ]]; then
    health_from_json_input "$health_input"
  else
    health_from_session
  fi
  exit 0
fi

field_text() {
  local file="$1" field="$2"
  if $have_jq; then
    jq -r --arg f "$field" '.[$f] // empty' "$file" 2>/dev/null
  else
    grep -oE "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$file" 2>/dev/null \
      | head -1 | sed -E "s/.*:[[:space:]]*\"([^\"]*)\".*/\1/"
  fi
}

# --- Build report ---------------------------------------------------------

if [[ "$format" == "json" ]]; then
  if ! $have_jq; then
    echo "trajectory-inspector: --format json requires jq to be installed" >&2
    exit 2
  fi
  if [[ -z "$active_session_file" ]]; then
    jq -n --arg n "$last_n" '{
      sessionFound: false,
      message: "(no active session)",
      lastN: ($n | tonumber)
    }'
    exit 0
  fi

  # Collect spec states
  spec_states="[]"
  if [[ -d "$specs_dir" ]]; then
    spec_states="$(
      while IFS= read -r -d '' sf; do
        spec_dir="$(dirname "$sf")"
        spec_name="$(basename "$spec_dir")"
        jq --arg name "$spec_name" '
          {
            spec: $name,
            status: (.status // .certification.status // "unknown"),
            workflowMode: (.workflowMode // .mode // null),
            currentPhase: (.execution.currentPhase // .currentPhase // null),
            currentScope: (.execution.currentScope // null)
          }
        ' "$sf" 2>/dev/null || true
      done < <(find "$specs_dir" -maxdepth 2 -name 'state.json' -not -path '*/bugs/*' -print0 2>/dev/null) \
        | jq -s '.'
    )"
  fi

  jq --arg lastN "$last_n" --argjson specs "$spec_states" '
    {
      sessionFound: true,
      sessionFile: input_filename,
      sessionId: .sessionId,
      agent: .agent,
      featureDir: .featureDir,
      mode: (.mode // .workflowMode),
      status: .status,
      currentPhase: .currentPhase,
      lastUpdatedAt: .lastUpdatedAt,
      turnSnapshots: ((.turnSnapshots // []) | sort_by(.turnNumber) | .[-($lastN | tonumber):]),
      scopesReportedFromSpecs: $specs
    }
  ' "$active_session_file"
  exit 0
fi

# --- Text format ----------------------------------------------------------

print_header() {
  echo "═══════════════════════════════════════════════════════════════"
  echo "  $1"
  echo "═══════════════════════════════════════════════════════════════"
}

print_header "Bubbles Trajectory Inspector"
echo "Repo root:    $repo_root"
echo "Session file: ${active_session_file:-<none>}"
echo "Filter:       session=${session_id_filter:-<latest>}  last=$last_n"
echo

if [[ -z "$active_session_file" ]]; then
  echo "(no active session)"
  exit 0
fi

# Section 1: Session summary
print_header "1. Session Summary"
sid="$(field_text "$active_session_file" sessionId)"
agent="$(field_text "$active_session_file" agent)"
feat="$(field_text "$active_session_file" featureDir)"
mode="$(field_text "$active_session_file" mode)"
[[ -z "$mode" ]] && mode="$(field_text "$active_session_file" workflowMode)"
status="$(field_text "$active_session_file" status)"
phase="$(field_text "$active_session_file" currentPhase)"
last_updated="$(field_text "$active_session_file" lastUpdatedAt)"

printf "  sessionId:     %s\n" "${sid:-<unset>}"
printf "  agent:         %s\n" "${agent:-<unset>}"
printf "  featureDir:    %s\n" "${feat:-<unset>}"
printf "  mode:          %s\n" "${mode:-<unset>}"
printf "  status:        %s\n" "${status:-<unset>}"
printf "  currentPhase:  %s\n" "${phase:-<unset>}"
printf "  lastUpdatedAt: %s\n" "${last_updated:-<unset>}"
echo

# Section 2: Phase progression
print_header "2. Phase Progression (turnSnapshots[])"
if $have_jq; then
  total="$(jq '(.turnSnapshots // []) | length' "$active_session_file")"
  echo "  total snapshots: $total"
  if [[ "$total" -gt 0 ]]; then
    echo "  phase counts:"
    jq -r '
      (.turnSnapshots // [])
      | group_by(.phase)
      | map({phase: (.[0].phase // "null"), count: length})
      | sort_by(-.count)
      | .[]
      | "    \(.count)  \(.phase)"
    ' "$active_session_file"
    echo
    echo "  last $last_n snapshot(s):"
    jq -r --arg n "$last_n" '
      (.turnSnapshots // [])
      | sort_by(.turnNumber)
      | .[-($n | tonumber):]
      | .[]
      | "    turn=\(.turnNumber)  \(.timestamp)  phase=\(.phase // "null")  scope=\(.scopeId // "null")  agent=\(.agent // "null")"
    ' "$active_session_file"
  else
    echo "  (no turn snapshots recorded)"
  fi
else
  echo "  jq not installed — cannot summarize turnSnapshots in detail"
fi
echo

# Section 3: Scope progression (from active spec featureDir)
print_header "3. Scope Progression"
if [[ -n "$feat" && -d "$repo_root/$feat" ]]; then
  spec_state="$repo_root/$feat/state.json"
  scopes_file="$repo_root/$feat/scopes.md"
  scopes_dir="$repo_root/$feat/scopes"

  if [[ -f "$spec_state" ]] && $have_jq; then
    echo "  spec state.json: $feat/state.json"
    completed="$(jq -r '
      ((.execution.completedScopes // .completedScopes // [])
        | if length == 0 then "    (none)"
          else map("    - " + .) | join("\n")
          end)
    ' "$spec_state")"
    echo "  completedScopes:"
    echo "$completed"
  fi

  if [[ -d "$scopes_dir" ]]; then
    echo "  per-scope status (from scopes/*/scope.md):"
    while IFS= read -r -d '' sf; do
      scope_name="$(basename "$(dirname "$sf")")"
      status_line="$(grep -m1 -E '^Status:[[:space:]]*' "$sf" 2>/dev/null | sed -E 's/^Status:[[:space:]]*//' || true)"
      printf "    - %-30s %s\n" "$scope_name" "${status_line:-<no Status: line>}"
    done < <(find "$scopes_dir" -mindepth 2 -maxdepth 2 -name 'scope.md' -print0 | sort -z)
  elif [[ -f "$scopes_file" ]]; then
    echo "  scope status lines (from scopes.md):"
    grep -nE '^Status:' "$scopes_file" | head -20 || echo "    (no Status: lines)"
  else
    echo "  (no scopes file found in $feat)"
  fi
else
  echo "  (no featureDir on the active session)"
fi
echo

# Section 4: Recent lessons
print_header "4. Recent Lessons (lessons.md tail)"
if [[ -f "$lessons_file" ]]; then
  tail -20 "$lessons_file" | sed 's/^/  /'
else
  echo "  (no lessons.md)"
fi
echo

# Section 5: Active specs (one row per state.json)
print_header "5. Active Specs (specs/*/state.json)"
if [[ -d "$specs_dir" ]]; then
  found=0
  while IFS= read -r -d '' sf; do
    found=$((found + 1))
    spec_name="$(basename "$(dirname "$sf")")"
    if $have_jq; then
      st="$(jq -r '.status // .certification.status // "unknown"' "$sf" 2>/dev/null || echo "?")"
      md="$(jq -r '.workflowMode // .mode // ""' "$sf" 2>/dev/null || echo "")"
      cp="$(jq -r '.execution.currentPhase // .currentPhase // ""' "$sf" 2>/dev/null || echo "")"
    else
      st="$(field_text "$sf" status)"
      md="$(field_text "$sf" workflowMode)"
      cp="$(field_text "$sf" currentPhase)"
    fi
    printf "    - %-32s status=%-14s mode=%-22s phase=%s\n" \
      "$spec_name" "${st:-?}" "${md:-?}" "${cp:-?}"
  done < <(find "$specs_dir" -maxdepth 2 -name 'state.json' -not -path '*/bugs/*' -print0 | sort -z)
  if [[ "$found" -eq 0 ]]; then
    echo "    (no specs/*/state.json files found)"
  fi
else
  echo "    (no specs/ directory)"
fi
echo

if [[ "$show_latency" == "true" ]]; then
  print_header "6. Validation Latency (--latency)"
  latency_report="$SCRIPT_DIR/validation-latency-report.sh"
  if [[ -f "$latency_report" ]]; then
    latency_args=(--repo-root "$repo_root" --since "$latency_since")
    if [[ -n "$spec_filter" ]]; then
      latency_args+=(--spec "$spec_filter")
    elif [[ -n "${feat:-}" ]]; then
      latency_args+=(--spec "$feat")
    fi
    bash "$latency_report" "${latency_args[@]}"
  else
    echo "  validation-latency-report.sh not found at $latency_report"
  fi
  echo
fi

print_header "End of trajectory report"
exit 0
