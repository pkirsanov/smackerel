#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

RUNTIME_DIR="$REPO_ROOT/.specify/runtime"
RUNTIME_FILE="$RUNTIME_DIR/resource-leases.json"
EVENT_FILE="$RUNTIME_DIR/framework-events.jsonl"
RUNTIME_LOCK_DIR="$RUNTIME_DIR/.locks/resource-leases.lock"
CONTROL_PLANE_CONFIG="$REPO_ROOT/.specify/memory/bubbles.config.json"
SESSION_FILE="$REPO_ROOT/.specify/memory/bubbles.session.json"

die() {
  echo "Error: $*" >&2
  exit 1
}

current_timestamp() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

timestamp_plus_minutes() {
  local minutes="$1"

  if date -u -d "+${minutes} minutes" +"%Y-%m-%dT%H:%M:%SZ" >/dev/null 2>&1; then
    date -u -d "+${minutes} minutes" +"%Y-%m-%dT%H:%M:%SZ"
  else
    date -u -v"+${minutes}"M +"%Y-%m-%dT%H:%M:%SZ"
  fi
}

to_epoch() {
  local timestamp="$1"

  if [[ -z "$timestamp" ]]; then
    printf '%s\n' '0'
    return 0
  fi

  if date -u -d "$timestamp" +%s >/dev/null 2>&1; then
    date -u -d "$timestamp" +%s
  else
    date -j -u -f "%Y-%m-%dT%H:%M:%SZ" "$timestamp" +%s 2>/dev/null || printf '%s\n' '0'
  fi
}

slugify() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
}

json_escape() {
  local raw="$1"

  JSON_ESCAPE_INPUT="$raw" python3 - <<'PY'
import json
import os
import sys

raw = os.environ.get('JSON_ESCAPE_INPUT', '')
sys.stdout.write(json.dumps(raw)[1:-1])
PY
}

append_jsonl() {
  local target_file="$1"
  local payload="$2"

  mkdir -p "$(dirname "$target_file")"
  printf '%s\n' "$payload" >> "$target_file"
}

record_framework_event() {
  local event_type="$1"
  local result="$2"
  local details="$3"
  local risk_class="$4"

  append_jsonl "$EVENT_FILE" "{\"version\":1,\"type\":\"$(json_escape "$event_type")\",\"timestamp\":\"$(current_timestamp)\",\"sessionId\":\"$(json_escape "$(derive_session_id)")\",\"command\":\"runtime\",\"target\":\"runtime\",\"riskClass\":\"$(json_escape "$risk_class")\",\"result\":\"$(json_escape "$result")\",\"durationMs\":0,\"details\":\"$(json_escape "$details")\"}"
}

hash_command() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s\n' 'sha256sum'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s\n' 'shasum'
  else
    die "No SHA-256 command available (expected sha256sum or shasum)"
  fi
}

hash_string() {
  local raw="$1"
  local cmd

  cmd="$(hash_command)"
  if [[ "$cmd" == "sha256sum" ]]; then
    printf '%s' "$raw" | sha256sum | awk '{print $1}'
  else
    printf '%s' "$raw" | shasum -a 256 | awk '{print $1}'
  fi
}

hash_file() {
  local path="$1"
  local cmd

  cmd="$(hash_command)"
  if [[ "$cmd" == "sha256sum" ]]; then
    sha256sum "$path" | awk '{print $1}'
  else
    shasum -a 256 "$path" | awk '{print $1}'
  fi
}

json_first_value_from_file() {
  local file="$1"
  local key="$2"
  local expected_type="$3"
  local value=''

  value="$(python3 - "$file" "$key" "$expected_type" <<'PY'
import json
import sys

file_path, target_key, expected_type = sys.argv[1:4]

def visit(node):
    if isinstance(node, dict):
        if target_key in node:
            value = node[target_key]
            if expected_type == 'string' and isinstance(value, str):
                return value
            if expected_type == 'number' and isinstance(value, int):
                return str(value)
        for child in node.values():
            result = visit(child)
            if result is not None:
                return result
    elif isinstance(node, list):
        for child in node:
            result = visit(child)
            if result is not None:
                return result
    return None

try:
    with open(file_path, 'r', encoding='utf-8') as handle:
        payload = json.load(handle)
except FileNotFoundError:
    sys.exit(0)

result = visit(payload)
if result is not None:
    sys.stdout.write(result)
PY
)"

  printf '%s\n' "$value"
}

json_string_field_from_line() {
  local line="$1"
  local field="$2"
  local value=''

  value="$(LEASE_JSON_LINE="$line" python3 - "$field" <<'PY'
import json
import os
import sys

field = sys.argv[1]
line = os.environ.get('LEASE_JSON_LINE', '')
if not line:
    sys.exit(0)

payload = json.loads(line)
value = payload.get(field)
if value is None:
    sys.exit(0)
if not isinstance(value, str):
    value = str(value)
sys.stdout.write(value)
PY
)" || die "Runtime lease registry contains malformed JSON while reading field '$field'"

  printf '%s\n' "$value"
}

config_number_value() {
  local _section="$1"
  local key="$2"
  local default_value="$3"
  local file="$4"
  local value

  value="$(json_first_value_from_file "$file" "$key" number)"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$default_value"
  fi
}

config_string_value() {
  local _section="$1"
  local key="$2"
  local default_value="$3"
  local file="$4"
  local value

  value="$(json_first_value_from_file "$file" "$key" string)"

  if [[ -n "$value" ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$default_value"
  fi
}

load_runtime_defaults() {
  if [[ -f "$CONTROL_PLANE_CONFIG" ]]; then
    CFG_RUNTIME_TTL_MINUTES="$(config_number_value runtime leaseTtlMinutes 20 "$CONTROL_PLANE_CONFIG")"
    CFG_RUNTIME_STALE_AFTER_MINUTES="$(config_number_value runtime staleAfterMinutes 60 "$CONTROL_PLANE_CONFIG")"
    CFG_RUNTIME_REUSE_POLICY="$(config_string_value runtime reusePolicy fingerprint-match-only "$CONTROL_PLANE_CONFIG")"
  else
    CFG_RUNTIME_TTL_MINUTES=20
    CFG_RUNTIME_STALE_AFTER_MINUTES=60
    CFG_RUNTIME_REUSE_POLICY=fingerprint-match-only
  fi
}

ensure_runtime_registry() {
  mkdir -p "$RUNTIME_DIR" "$RUNTIME_DIR/.locks"

  if [[ ! -f "$RUNTIME_FILE" ]]; then
    cat > "$RUNTIME_FILE" <<'EOF'
{
  "version": 1,
  "leases": [
  ]
}
EOF
  fi
}

lock_acquired=false

acquire_registry_lock() {
  ensure_runtime_registry

  if mkdir "$RUNTIME_LOCK_DIR" 2>/dev/null; then
    lock_acquired=true
    trap 'release_registry_lock' EXIT INT TERM
  else
    die "Runtime lease registry is busy. Another session may be updating it."
  fi
}

release_registry_lock() {
  if [[ "$lock_acquired" == true ]]; then
    rmdir "$RUNTIME_LOCK_DIR" 2>/dev/null || true
    lock_acquired=false
    trap - EXIT INT TERM
  fi
}

lease_lines() {
  ensure_runtime_registry

  awk '
    /"leases"[[:space:]]*:[[:space:]]*\[/ { in_leases = 1; next }
    in_leases && /^[[:space:]]*\]/ { exit }
    in_leases && /"leaseId"/ {
      gsub(/^[[:space:]]+/, "", $0)
      sub(/,[[:space:]]*$/, "", $0)
      print $0
    }
  ' "$RUNTIME_FILE"
}

write_runtime_registry() {
  local lines="$1"
  local tmp_file="$RUNTIME_FILE.tmp"
  local count=0
  local index=0
  local line

  count=$(printf '%s\n' "$lines" | sed '/^$/d' | wc -l | tr -d ' ')

  {
    echo '{'
    echo '  "version": 1,'
    echo '  "leases": ['
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      index=$((index + 1))
      if [[ "$index" -lt "$count" ]]; then
        printf '    %s,\n' "$line"
      else
        printf '    %s\n' "$line"
      fi
    done <<< "$lines"
    echo '  ]'
    echo '}'
  } > "$tmp_file"

  mv "$tmp_file" "$RUNTIME_FILE"
}

field_from_line() {
  local line="$1"
  local field="$2"

  json_string_field_from_line "$line" "$field"
}

append_unique_csv() {
  local csv="$1"
  local value="$2"
  local item

  if [[ -z "$csv" ]]; then
    printf '%s\n' "$value"
    return 0
  fi

  IFS=','
  for item in $csv; do
    if [[ "$item" == "$value" ]]; then
      printf '%s\n' "$csv"
      unset IFS
      return 0
    fi
  done
  unset IFS

  printf '%s\n' "${csv},${value}"
}

csv_contains() {
  local csv="$1"
  local value="$2"
  local item

  [[ -n "$csv" && -n "$value" ]] || return 1

  IFS=','
  for item in $csv; do
    if [[ "$item" == "$value" ]]; then
      unset IFS
      return 0
    fi
  done
  unset IFS

  return 1
}

remove_csv_value() {
  local csv="$1"
  local value="$2"
  local item
  local updated=''

  [[ -n "$csv" ]] || {
    printf '%s\n' ''
    return 0
  }

  IFS=','
  for item in $csv; do
    if [[ "$item" == "$value" || -z "$item" ]]; then
      continue
    fi
    if [[ -z "$updated" ]]; then
      updated="$item"
    else
      updated="${updated},${item}"
    fi
  done
  unset IFS

  printf '%s\n' "$updated"
}

first_csv_value() {
  local csv="$1"
  local item

  [[ -n "$csv" ]] || {
    printf '%s\n' ''
    return 0
  }

  IFS=','
  for item in $csv; do
    if [[ -n "$item" ]]; then
      unset IFS
      printf '%s\n' "$item"
      return 0
    fi
  done
  unset IFS

  printf '%s\n' ''
}

derive_session_id() {
  if [[ -n "${BUBBLES_SESSION_ID:-}" ]]; then
    printf '%s\n' "$BUBBLES_SESSION_ID"
    return 0
  fi

  if [[ -f "$SESSION_FILE" ]]; then
    local session_id
    session_id="$(json_first_value_from_file "$SESSION_FILE" sessionId string)"
    if [[ -n "$session_id" ]]; then
      printf '%s\n' "$session_id"
      return 0
    fi
  fi

  printf 'shell-%s\n' "$$"
}

derive_agent_name() {
  if [[ -n "${BUBBLES_AGENT_NAME:-}" ]]; then
    printf '%s\n' "$BUBBLES_AGENT_NAME"
  else
    printf '%s\n' 'cli'
  fi
}

current_branch_name() {
  git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || printf '%s\n' 'unknown'
}

generate_lease_id() {
  printf 'rls_%s_%04d\n' "$(date -u +%Y%m%d%H%M%S)" "$((RANDOM % 10000))"
}

effective_status() {
  local line="$1"
  local status expires_at now_epoch expires_epoch

  status="$(field_from_line "$line" status)"
  expires_at="$(field_from_line "$line" expiresAt)"

  if [[ "$status" == "active" ]]; then
    now_epoch="$(to_epoch "$(current_timestamp)")"
    expires_epoch="$(to_epoch "$expires_at")"
    if [[ "$expires_epoch" -gt 0 && "$expires_epoch" -lt "$now_epoch" ]]; then
      printf '%s\n' 'stale'
      return 0
    fi
  fi

  printf '%s\n' "$status"
}

build_lease_line() {
  local lease_id="$1"
  local repo="$2"
  local session_id="$3"
  local agent="$4"
  local worktree="$5"
  local branch="$6"
  local purpose="$7"
  local environment="$8"
  local compose_project="$9"
  local stack_group="${10}"
  local share_mode="${11}"
  local compatibility_fingerprint="${12}"
  local resources="${13}"
  local attached_sessions="${14}"
  local started_at="${15}"
  local last_heartbeat_at="${16}"
  local expires_at="${17}"
  local status="${18}"

  printf '%s\n' "{\"leaseId\":\"$(json_escape "$lease_id")\",\"repo\":\"$(json_escape "$repo")\",\"sessionId\":\"$(json_escape "$session_id")\",\"agent\":\"$(json_escape "$agent")\",\"worktree\":\"$(json_escape "$worktree")\",\"branch\":\"$(json_escape "$branch")\",\"purpose\":\"$(json_escape "$purpose")\",\"environment\":\"$(json_escape "$environment")\",\"composeProject\":\"$(json_escape "$compose_project")\",\"stackGroup\":\"$(json_escape "$stack_group")\",\"shareMode\":\"$(json_escape "$share_mode")\",\"compatibilityFingerprint\":\"$(json_escape "$compatibility_fingerprint")\",\"resources\":\"$(json_escape "$resources")\",\"attachedSessions\":\"$(json_escape "$attached_sessions")\",\"startedAt\":\"$(json_escape "$started_at")\",\"lastHeartbeatAt\":\"$(json_escape "$last_heartbeat_at")\",\"expiresAt\":\"$(json_escape "$expires_at")\",\"status\":\"$(json_escape "$status")\"}"
}

rebuild_line_with_updates() {
  local line="$1"
  local session_id="$2"
  local attached_sessions="$3"
  local last_heartbeat_at="$4"
  local expires_at="$5"
  local status="$6"

  build_lease_line \
    "$(field_from_line "$line" leaseId)" \
    "$(field_from_line "$line" repo)" \
    "$session_id" \
    "$(field_from_line "$line" agent)" \
    "$(field_from_line "$line" worktree)" \
    "$(field_from_line "$line" branch)" \
    "$(field_from_line "$line" purpose)" \
    "$(field_from_line "$line" environment)" \
    "$(field_from_line "$line" composeProject)" \
    "$(field_from_line "$line" stackGroup)" \
    "$(field_from_line "$line" shareMode)" \
    "$(field_from_line "$line" compatibilityFingerprint)" \
    "$(field_from_line "$line" resources)" \
    "$attached_sessions" \
    "$(field_from_line "$line" startedAt)" \
    "$last_heartbeat_at" \
    "$expires_at" \
    "$status"
}

update_lease_line() {
  local lease_id="$1"
  local replacement_line="$2"
  local lines new_lines line current_id

  lines="$(lease_lines)"
  new_lines=''

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_id="$(field_from_line "$line" leaseId)"
    if [[ "$current_id" == "$lease_id" ]]; then
      line="$replacement_line"
    fi
    if [[ -z "$new_lines" ]]; then
      new_lines="$line"
    else
      new_lines="$new_lines
$line"
    fi
  done <<< "$lines"

  write_runtime_registry "$new_lines"
}

compatibility_fingerprint_for() {
  local compatibility_key="$1"
  local fingerprint_files="$2"
  local fingerprint_inputs="$3"
  local accumulator='' item path

  accumulator="repo=$(basename "$REPO_ROOT")\nbranch=$(current_branch_name)\ncompatKey=$compatibility_key"

  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    if [[ -f "$path" ]]; then
      accumulator="${accumulator}\nfile=${path}:$(hash_file "$path")"
    else
      accumulator="${accumulator}\nmissing=${path}"
    fi
  done <<< "$fingerprint_files"

  while IFS= read -r item; do
    [[ -n "$item" ]] || continue
    accumulator="${accumulator}\ninput=${item}"
  done <<< "$fingerprint_inputs"

  if [[ -z "$fingerprint_files" && -z "$fingerprint_inputs" ]]; then
    accumulator="${accumulator}\nhead=$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf '%s' 'no-git-head')"
  fi

  printf 'sha256:%s\n' "$(hash_string "$accumulator")"
}

generated_compose_project() {
  local repo="$1"
  local environment="$2"
  local purpose="$3"
  local share_mode="$4"
  local lease_id="$5"
  local fingerprint="$6"
  local suffix

  if [[ "$share_mode" == "shared-compatible" ]]; then
    suffix="cmp$(printf '%s' "$fingerprint" | sed 's/^sha256://' | cut -c1-8)"
  else
    suffix="$(printf '%s' "$lease_id" | cut -c1-12)"
  fi

  printf '%s-%s-%s-%s\n' "$(slugify "$repo")" "$(slugify "$environment")" "$(slugify "$purpose")" "$suffix"
}

format_lease_line() {
  local line="$1"
  local fingerprint

  fingerprint="$(field_from_line "$line" compatibilityFingerprint)"
  printf 'leaseId=%s repo=%s purpose=%s env=%s shareMode=%s status=%s composeProject=%s owner=%s attachedSessions=%s stackGroup=%s fingerprint=%s\n' \
    "$(field_from_line "$line" leaseId)" \
    "$(field_from_line "$line" repo)" \
    "$(field_from_line "$line" purpose)" \
    "$(field_from_line "$line" environment)" \
    "$(field_from_line "$line" shareMode)" \
    "$(effective_status "$line")" \
    "$(field_from_line "$line" composeProject)" \
    "$(field_from_line "$line" sessionId)" \
    "$(field_from_line "$line" attachedSessions)" \
    "$(field_from_line "$line" stackGroup)" \
    "${fingerprint#sha256:}"
}

line_matches_lookup_filters() {
  local line="$1"
  local filter_lease_id="$2"
  local filter_compose_project="$3"
  local filter_purpose="$4"
  local filter_environment="$5"
  local filter_share_mode="$6"
  local filter_session_id="$7"
  local filter_status="$8"
  local effective

  [[ -z "$filter_lease_id" || "$(field_from_line "$line" leaseId)" == "$filter_lease_id" ]] || return 1
  [[ -z "$filter_compose_project" || "$(field_from_line "$line" composeProject)" == "$filter_compose_project" ]] || return 1
  [[ -z "$filter_purpose" || "$(field_from_line "$line" purpose)" == "$filter_purpose" ]] || return 1
  [[ -z "$filter_environment" || "$(field_from_line "$line" environment)" == "$filter_environment" ]] || return 1
  [[ -z "$filter_share_mode" || "$(field_from_line "$line" shareMode)" == "$filter_share_mode" ]] || return 1

  if [[ -n "$filter_session_id" ]]; then
    if [[ "$(field_from_line "$line" sessionId)" != "$filter_session_id" ]] && ! csv_contains "$(field_from_line "$line" attachedSessions)" "$filter_session_id"; then
      return 1
    fi
  fi

  if [[ -n "$filter_status" && "$filter_status" != "any" ]]; then
    effective="$(effective_status "$line")"
    [[ "$effective" == "$filter_status" ]] || return 1
  fi

  return 0
}

cmd_lookup() {
  local filter_lease_id=''
  local filter_compose_project=''
  local filter_purpose=''
  local filter_environment=''
  local filter_share_mode=''
  local filter_session_id=''
  local filter_status='any'
  local all_matches=false
  local lines line matched=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --lease-id) filter_lease_id="$2"; shift 2 ;;
      --compose-project) filter_compose_project="$2"; shift 2 ;;
      --purpose) filter_purpose="$2"; shift 2 ;;
      --environment) filter_environment="$2"; shift 2 ;;
      --share-mode) filter_share_mode="$2"; shift 2 ;;
      --session-id) filter_session_id="$2"; shift 2 ;;
      --status) filter_status="$2"; shift 2 ;;
      --all) all_matches=true; shift ;;
      *) die "Unknown runtime lookup option: $1" ;;
    esac
  done

  lines="$(lease_lines)"
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if line_matches_lookup_filters "$line" "$filter_lease_id" "$filter_compose_project" "$filter_purpose" "$filter_environment" "$filter_share_mode" "$filter_session_id" "$filter_status"; then
      format_lease_line "$line"
      matched=1
      if [[ "$all_matches" == false ]]; then
        return 0
      fi
    fi
  done <<< "$lines"

  if [[ "$matched" -eq 0 ]]; then
    return 1
  fi
}

cmd_list() {
  local lines line count

  lines="$(lease_lines)"
  count=$(printf '%s\n' "$lines" | sed '/^$/d' | wc -l | tr -d ' ')

  echo "Runtime lease registry: $RUNTIME_FILE"
  if [[ "$count" -eq 0 ]]; then
    echo "No runtime leases recorded."
    return 0
  fi

  printf '%-24s %-16s %-18s %-18s %-16s %-18s\n' 'LEASE ID' 'PURPOSE' 'SHARE MODE' 'STATUS' 'COMPOSE PROJECT' 'OWNER'
  printf '%-24s %-16s %-18s %-18s %-16s %-18s\n' '────────' '───────' '──────────' '──────' '──────────────' '─────'

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf '%-24s %-16s %-18s %-18s %-16s %-18s\n' \
      "$(field_from_line "$line" leaseId)" \
      "$(field_from_line "$line" purpose)" \
      "$(field_from_line "$line" shareMode)" \
      "$(effective_status "$line")" \
      "$(field_from_line "$line" composeProject)" \
      "$(field_from_line "$line" sessionId)"
  done <<< "$lines"
}

cmd_summary() {
  local lines line active=0 stale=0 released=0 conflicts=0
  local current compose purpose environment fingerprint pair_key other_compose other_purpose other_environment other_fingerprint
  local active_lines=''
  local seen_conflicts=''

  lines="$(lease_lines)"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current="$(effective_status "$line")"
    case "$current" in
      active)
        active=$((active + 1))
        if [[ -z "$active_lines" ]]; then
          active_lines="$line"
        else
          active_lines="$active_lines
$line"
        fi
        ;;
      stale) stale=$((stale + 1)) ;;
      released) released=$((released + 1)) ;;
    esac
  done <<< "$lines"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    compose="$(field_from_line "$line" composeProject)"
    purpose="$(field_from_line "$line" purpose)"
    environment="$(field_from_line "$line" environment)"
    fingerprint="$(field_from_line "$line" compatibilityFingerprint)"

    while IFS= read -r other_line; do
      [[ -n "$other_line" ]] || continue
      if [[ "$line" == "$other_line" ]]; then
        continue
      fi
      other_compose="$(field_from_line "$other_line" composeProject)"
      other_purpose="$(field_from_line "$other_line" purpose)"
      other_environment="$(field_from_line "$other_line" environment)"
      other_fingerprint="$(field_from_line "$other_line" compatibilityFingerprint)"
      if [[ "$compose" == "$other_compose" ]]; then
        pair_key="${compose}::compose"
        if [[ "$seen_conflicts" != *"|${pair_key}|"* ]]; then
          conflicts=$((conflicts + 1))
          seen_conflicts="${seen_conflicts}|${pair_key}|"
        fi
      elif [[ "$purpose" == "$other_purpose" && "$environment" == "$other_environment" && "$fingerprint" != "$other_fingerprint" ]]; then
        pair_key="${purpose}::${environment}::fingerprint"
        if [[ "$seen_conflicts" != *"|${pair_key}|"* ]]; then
          conflicts=$((conflicts + 1))
          seen_conflicts="${seen_conflicts}|${pair_key}|"
        fi
      fi
    done <<< "$active_lines"
  done <<< "$active_lines"

  echo "Runtime leases: active=$active stale=$stale released=$released conflicts=$conflicts"
}

cmd_doctor() {
  local quiet=false
  local lines line summary_output conflicts

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --quiet) quiet=true ;;
      *) die "Unknown runtime doctor option: $1" ;;
    esac
    shift
  done

  summary_output="$(cmd_summary)"
  conflicts="$(printf '%s\n' "$summary_output" | sed -nE 's/.*conflicts=([0-9]+).*/\1/p')"
  conflicts="${conflicts:-0}"

  if [[ "$quiet" == false ]]; then
    echo "Runtime Lease Doctor"
    echo "===================="
    echo "$summary_output"
    echo

    lines="$(lease_lines)"
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      if [[ "$(effective_status "$line")" == "stale" ]]; then
        printf 'STALE %s owner=%s purpose=%s compose=%s expired=%s\n' \
          "$(field_from_line "$line" leaseId)" \
          "$(field_from_line "$line" sessionId)" \
          "$(field_from_line "$line" purpose)" \
          "$(field_from_line "$line" composeProject)" \
          "$(field_from_line "$line" expiresAt)"
      fi
    done <<< "$lines"
  fi

  if [[ "$conflicts" -gt 0 ]]; then
    return 1
  fi

  return 0
}

cmd_acquire() {
  local purpose=''
  local environment='dev'
  local share_mode='exclusive'
  local stack_group='validation'
  local ttl_minutes=''
  local compose_project=''
  local compatibility_key=''
  local fingerprint_files=''
  local fingerprint_inputs=''
  local resources=''
  local session_id=''
  local agent=''
  local lease_id=''
  local repo_name
  local worktree branch started_at expires_at compatibility_fingerprint
  local lines line current_status attached_sessions replacement_line explicit_compose=false

  load_runtime_defaults

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --purpose) purpose="$2"; shift 2 ;;
      --environment) environment="$2"; shift 2 ;;
      --share-mode) share_mode="$2"; shift 2 ;;
      --stack-group) stack_group="$2"; shift 2 ;;
      --ttl-minutes) ttl_minutes="$2"; shift 2 ;;
      --compose-project) compose_project="$2"; explicit_compose=true; shift 2 ;;
      --compatibility-key) compatibility_key="$2"; shift 2 ;;
      --fingerprint-file)
        fingerprint_files="${fingerprint_files}${fingerprint_files:+$'\n'}$2"
        shift 2
        ;;
      --fingerprint-input)
        fingerprint_inputs="${fingerprint_inputs}${fingerprint_inputs:+$'\n'}$2"
        shift 2
        ;;
      --resource)
        resources="${resources}${resources:+|}$2"
        shift 2
        ;;
      --session-id) session_id="$2"; shift 2 ;;
      --agent) agent="$2"; shift 2 ;;
      *) die "Unknown runtime acquire option: $1" ;;
    esac
  done

  [[ -n "$purpose" ]] || die "Usage: runtime acquire --purpose <name> [options]"

  case "$share_mode" in
    shared-compatible|exclusive|disposable|persistent-protected) ;;
    *) die "Invalid share mode: $share_mode" ;;
  esac

  if [[ -z "$ttl_minutes" ]]; then
    ttl_minutes="$CFG_RUNTIME_TTL_MINUTES"
  fi
  [[ "$ttl_minutes" =~ ^[0-9]+$ ]] || die "ttl-minutes must be numeric"

  if [[ -z "$session_id" ]]; then
    session_id="$(derive_session_id)"
  fi
  if [[ -z "$agent" ]]; then
    agent="$(derive_agent_name)"
  fi

  repo_name="$(basename "$REPO_ROOT")"
  worktree="$REPO_ROOT"
  branch="$(current_branch_name)"
  started_at="$(current_timestamp)"
  expires_at="$(timestamp_plus_minutes "$ttl_minutes")"
  compatibility_fingerprint="$(compatibility_fingerprint_for "$compatibility_key" "$fingerprint_files" "$fingerprint_inputs")"

  acquire_registry_lock
  lines="$(lease_lines)"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_status="$(effective_status "$line")"
    if [[ "$current_status" == "active" \
      && "$(field_from_line "$line" repo)" == "$repo_name" \
      && "$(field_from_line "$line" purpose)" == "$purpose" \
      && "$(field_from_line "$line" environment)" == "$environment" \
      && "$(field_from_line "$line" shareMode)" == "$share_mode" \
      && "$(field_from_line "$line" compatibilityFingerprint)" == "$compatibility_fingerprint" ]]; then
      if [[ "$explicit_compose" == true && "$(field_from_line "$line" composeProject)" != "$compose_project" ]]; then
        continue
      fi
      if csv_contains "$(field_from_line "$line" attachedSessions)" "$session_id"; then
        attached_sessions="$(append_unique_csv "$(field_from_line "$line" attachedSessions)" "$session_id")"
        replacement_line="$(rebuild_line_with_updates "$line" "$(field_from_line "$line" sessionId)" "$attached_sessions" "$started_at" "$expires_at" active)"
        update_lease_line "$(field_from_line "$line" leaseId)" "$replacement_line"
        release_registry_lock
        record_framework_event "runtime_lease_reused" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "owned_mutation"
        echo "✅ Reused existing runtime lease"
        format_lease_line "$replacement_line"
        return 0
      fi
    fi
  done <<< "$lines"

  if [[ "$share_mode" == "shared-compatible" && "$CFG_RUNTIME_REUSE_POLICY" == "fingerprint-match-only" ]]; then
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      current_status="$(effective_status "$line")"
      if [[ "$current_status" == "active" \
        && "$(field_from_line "$line" repo)" == "$repo_name" \
        && "$(field_from_line "$line" purpose)" == "$purpose" \
        && "$(field_from_line "$line" environment)" == "$environment" \
        && "$(field_from_line "$line" shareMode)" == "$share_mode" \
        && "$(field_from_line "$line" compatibilityFingerprint)" == "$compatibility_fingerprint" ]]; then
        attached_sessions="$(append_unique_csv "$(field_from_line "$line" attachedSessions)" "$session_id")"
        replacement_line="$(rebuild_line_with_updates "$line" "$(field_from_line "$line" sessionId)" "$attached_sessions" "$started_at" "$expires_at" active)"
        update_lease_line "$(field_from_line "$line" leaseId)" "$replacement_line"
        release_registry_lock
        record_framework_event "runtime_lease_reused" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "owned_mutation"
        echo "✅ Reused compatible runtime lease"
        format_lease_line "$replacement_line"
        return 0
      fi
    done <<< "$lines"
  fi

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_status="$(effective_status "$line")"
    if [[ "$current_status" != "active" ]]; then
      continue
    fi

    if [[ "$explicit_compose" == true && "$(field_from_line "$line" composeProject)" == "$compose_project" ]]; then
      release_registry_lock
      die "Compose project '$compose_project' is already owned by active lease $(field_from_line "$line" leaseId)"
    fi

    if [[ "$share_mode" == "exclusive" \
      && "$(field_from_line "$line" repo)" == "$repo_name" \
      && "$(field_from_line "$line" purpose)" == "$purpose" \
      && "$(field_from_line "$line" environment)" == "$environment" \
      && "$(field_from_line "$line" shareMode)" == "exclusive" ]]; then
      release_registry_lock
      die "Exclusive runtime already active for ${purpose}/${environment}: $(field_from_line "$line" leaseId)"
    fi
  done <<< "$lines"

  lease_id="$(generate_lease_id)"
  if [[ -z "$compose_project" ]]; then
    compose_project="$(generated_compose_project "$repo_name" "$environment" "$purpose" "$share_mode" "$lease_id" "$compatibility_fingerprint")"
  fi

  line="$(build_lease_line "$lease_id" "$repo_name" "$session_id" "$agent" "$worktree" "$branch" "$purpose" "$environment" "$compose_project" "$stack_group" "$share_mode" "$compatibility_fingerprint" "$resources" "$session_id" "$started_at" "$started_at" "$expires_at" active)"
  if [[ -n "$lines" ]]; then
    lines="$lines
$line"
  else
    lines="$line"
  fi
  write_runtime_registry "$lines"
  release_registry_lock

  record_framework_event "runtime_lease_acquired" "success" "leaseId=$(field_from_line "$line" leaseId) composeProject=$(field_from_line "$line" composeProject)" "owned_mutation"
  echo "✅ Acquired runtime lease"
  format_lease_line "$line"
}

cmd_attach() {
  local lease_id="${1:-}"
  local takeover=false
  local session_id=''
  local lines line current_id current_status share_mode owner_session attached_sessions replacement_line now expires_at

  [[ -n "$lease_id" ]] || die "Usage: runtime attach <lease-id> [--takeover]"
  shift || true

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --takeover) takeover=true; shift ;;
      --session-id) session_id="$2"; shift 2 ;;
      *) die "Unknown runtime attach option: $1" ;;
    esac
  done

  load_runtime_defaults
  [[ -n "$session_id" ]] || session_id="$(derive_session_id)"

  acquire_registry_lock
  lines="$(lease_lines)"
  now="$(current_timestamp)"
  expires_at="$(timestamp_plus_minutes "$CFG_RUNTIME_TTL_MINUTES")"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_id="$(field_from_line "$line" leaseId)"
    if [[ "$current_id" != "$lease_id" ]]; then
      continue
    fi

    current_status="$(effective_status "$line")"
    share_mode="$(field_from_line "$line" shareMode)"
    owner_session="$(field_from_line "$line" sessionId)"

    if [[ "$share_mode" == "shared-compatible" || "$owner_session" == "$session_id" ]]; then
      attached_sessions="$(append_unique_csv "$(field_from_line "$line" attachedSessions)" "$session_id")"
      replacement_line="$(rebuild_line_with_updates "$line" "$owner_session" "$attached_sessions" "$now" "$expires_at" active)"
      update_lease_line "$lease_id" "$replacement_line"
      release_registry_lock
      record_framework_event "runtime_lease_attached" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "owned_mutation"
      echo "✅ Attached to runtime lease"
      format_lease_line "$replacement_line"
      return 0
    fi

    if [[ "$current_status" == "stale" && "$takeover" == true ]]; then
      replacement_line="$(rebuild_line_with_updates "$line" "$session_id" "$session_id" "$now" "$expires_at" active)"
      update_lease_line "$lease_id" "$replacement_line"
      release_registry_lock
      record_framework_event "runtime_lease_taken_over" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "owned_mutation"
      echo "✅ Took over stale runtime lease"
      format_lease_line "$replacement_line"
      return 0
    fi

    release_registry_lock
    die "Lease '$lease_id' is exclusive and owned by active session '$owner_session'"
  done <<< "$lines"

  release_registry_lock
  die "Unknown lease: $lease_id"
}

cmd_heartbeat() {
  local lease_id="${1:-}"
  local session_id="${2:-}"
  local lines line current_id replacement_line now expires_at attached_sessions

  [[ -n "$lease_id" ]] || die "Usage: runtime heartbeat <lease-id> [session-id]"

  load_runtime_defaults
  [[ -n "$session_id" ]] || session_id="$(derive_session_id)"

  acquire_registry_lock
  lines="$(lease_lines)"
  now="$(current_timestamp)"
  expires_at="$(timestamp_plus_minutes "$CFG_RUNTIME_TTL_MINUTES")"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_id="$(field_from_line "$line" leaseId)"
    if [[ "$current_id" != "$lease_id" ]]; then
      continue
    fi

    attached_sessions="$(append_unique_csv "$(field_from_line "$line" attachedSessions)" "$session_id")"
    replacement_line="$(rebuild_line_with_updates "$line" "$(field_from_line "$line" sessionId)" "$attached_sessions" "$now" "$expires_at" active)"
    update_lease_line "$lease_id" "$replacement_line"
    release_registry_lock
    record_framework_event "runtime_lease_heartbeat" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "owned_mutation"
    echo "✅ Renewed runtime lease heartbeat"
    format_lease_line "$replacement_line"
    return 0
  done <<< "$lines"

  release_registry_lock
  die "Unknown lease: $lease_id"
}

cmd_release() {
  local lease_id="${1:-}"
  local session_id=''
  local lines line current_id replacement_line now
  local attached_sessions updated_sessions next_owner current_owner share_mode current_status

  if [[ -n "$lease_id" ]]; then
    shift || true
  fi

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-id) session_id="$2"; shift 2 ;;
      *) die "Unknown runtime release option: $1" ;;
    esac
  done

  [[ -n "$lease_id" ]] || die "Usage: runtime release <lease-id>"

  if [[ -n "$session_id" ]]; then
    load_runtime_defaults
  fi

  acquire_registry_lock
  lines="$(lease_lines)"
  now="$(current_timestamp)"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_id="$(field_from_line "$line" leaseId)"
    if [[ "$current_id" != "$lease_id" ]]; then
      continue
    fi

    if [[ -n "$session_id" ]]; then
      attached_sessions="$(field_from_line "$line" attachedSessions)"
      current_owner="$(field_from_line "$line" sessionId)"
      share_mode="$(field_from_line "$line" shareMode)"
      current_status="$(effective_status "$line")"

      if [[ "$current_owner" != "$session_id" ]] && ! csv_contains "$attached_sessions" "$session_id"; then
        release_registry_lock
        die "Session '$session_id' is not attached to lease '$lease_id'"
      fi

      updated_sessions="$(remove_csv_value "$attached_sessions" "$session_id")"
      if [[ -z "$updated_sessions" ]]; then
        replacement_line="$(rebuild_line_with_updates "$line" "$current_owner" "$updated_sessions" "$now" "$now" released)"
        update_lease_line "$lease_id" "$replacement_line"
        release_registry_lock
        record_framework_event "runtime_lease_released" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "runtime_teardown"
        echo "✅ Released runtime lease"
        format_lease_line "$replacement_line"
        return 0
      fi

      next_owner="$current_owner"
      if [[ "$current_owner" == "$session_id" ]]; then
        next_owner="$(first_csv_value "$updated_sessions")"
      fi
      if [[ -z "$next_owner" ]]; then
        next_owner="$(first_csv_value "$updated_sessions")"
      fi

      replacement_line="$(rebuild_line_with_updates "$line" "$next_owner" "$updated_sessions" "$now" "$(timestamp_plus_minutes "$CFG_RUNTIME_TTL_MINUTES")" active)"
      update_lease_line "$lease_id" "$replacement_line"
      release_registry_lock
      record_framework_event "runtime_lease_detached" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "runtime_teardown"
      echo "✅ Detached session from runtime lease"
      format_lease_line "$replacement_line"
      return 0
    fi

    replacement_line="$(rebuild_line_with_updates "$line" "$(field_from_line "$line" sessionId)" "$(field_from_line "$line" attachedSessions)" "$now" "$now" released)"
    update_lease_line "$lease_id" "$replacement_line"
    release_registry_lock
    record_framework_event "runtime_lease_released" "success" "leaseId=$(field_from_line "$replacement_line" leaseId) composeProject=$(field_from_line "$replacement_line" composeProject)" "runtime_teardown"
    echo "✅ Released runtime lease"
    format_lease_line "$replacement_line"
    return 0
  done <<< "$lines"

  release_registry_lock
  die "Unknown lease: $lease_id"
}

cmd_reclaim_stale() {
  local lines line current_status updated='' now

  acquire_registry_lock
  lines="$(lease_lines)"
  now="$(current_timestamp)"

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    current_status="$(effective_status "$line")"
    if [[ "$current_status" == "stale" ]]; then
      line="$(rebuild_line_with_updates "$line" "$(field_from_line "$line" sessionId)" "$(field_from_line "$line" attachedSessions)" "$now" "$(field_from_line "$line" expiresAt)" stale)"
    fi
    if [[ -z "$updated" ]]; then
      updated="$line"
    else
      updated="$updated
$line"
    fi
  done <<< "$lines"

  write_runtime_registry "$updated"
  release_registry_lock
  record_framework_event "runtime_leases_reclaimed" "success" "stale leases marked in $RUNTIME_FILE" "runtime_teardown"
  echo "✅ Marked stale runtime leases"
  cmd_list
}

cmd_help() {
  cat <<'EOF'
Usage: runtime-leases.sh <command> [args]

Commands:
  leases|list                     Show recorded runtime leases
  summary                         Show active/stale/released/conflict counts
  doctor [--quiet]                Detect stale leases and active conflicts
  lookup [filters]                Find a lease by compose project, purpose, session, or status
  acquire --purpose <name> [opts] Acquire or reuse a runtime lease
  attach <lease-id> [--takeover]  Attach to a compatible lease or take over a stale one
  heartbeat <lease-id>            Renew an existing lease
  release <lease-id> [--session-id <id>] Mark a lease as released or detach one session
  reclaim-stale                   Mark expired active leases as stale

Acquire options:
  --environment <env>             Default: dev
  --share-mode <mode>             shared-compatible|exclusive|disposable|persistent-protected
  --stack-group <group>           Default: validation
  --ttl-minutes <n>               Default from bubbles.config.json runtime.leaseTtlMinutes
  --compose-project <name>        Explicit compose project name
  --compatibility-key <text>      Extra compatibility discriminator
  --fingerprint-file <path>       Include file digest in compatibility fingerprint (repeatable)
  --fingerprint-input <text>      Include literal input in compatibility fingerprint (repeatable)
  --resource <kind:name>          Record container/volume/network/image ownership (repeatable)
  --session-id <id>               Override derived session id
  --agent <name>                  Override agent name
EOF
}

load_runtime_defaults

case "${1:-help}" in
  leases|list)
    shift
    cmd_list "$@"
    ;;
  summary)
    shift
    cmd_summary "$@"
    ;;
  doctor)
    shift
    cmd_doctor "$@"
    ;;
  lookup)
    shift
    cmd_lookup "$@"
    ;;
  acquire)
    shift
    cmd_acquire "$@"
    ;;
  attach)
    shift
    cmd_attach "$@"
    ;;
  heartbeat)
    shift
    cmd_heartbeat "$@"
    ;;
  release)
    shift
    cmd_release "$@"
    ;;
  reclaim-stale)
    shift
    cmd_reclaim_stale "$@"
    ;;
  help|--help|-h)
    cmd_help
    ;;
  *)
    die "Unknown runtime leases command: $1"
    ;;
esac