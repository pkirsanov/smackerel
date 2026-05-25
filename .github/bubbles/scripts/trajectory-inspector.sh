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

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/trajectory-inspector.sh \
         [--session <id>] [--last <N>] [--format text|json] \
         [--repo-root <path>] [--verbose]

Print a human-readable trajectory report from the current Bubbles session,
turn snapshots, lessons, and per-spec state. Always exits 0 unless invoked
with bad arguments (exit 2).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
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

print_header "End of trajectory report"
exit 0
