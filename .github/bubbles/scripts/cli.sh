#!/usr/bin/env bash
# ────────────────────────────────────────────────────────────────────
# bubbles — Lightweight CLI for Bubbles governance queries and script dispatch
# ────────────────────────────────────────────────────────────────────
# Project-agnostic. Works in any repo with specs/ and bubbles/scripts/.
#
# Usage:
#   bash bubbles/scripts/cli.sh <command> [args...]
#
# Commands:
#   status                        Show all specs with status, mode, scope counts
#   specs [--range M-N] [--cat X] List/filter specs (categories: business, infra, all)
#   blocked                       Show only blocked specs with reasons
#   dod <spec>                    Show unchecked DoD items for a spec
#   policy <subcommand>           Manage control-plane defaults and provenance
#   runtime <subcommand>          Manage runtime leases and coordination
#   session                       Show current session state
#   lint <spec>                   Run artifact lint on a spec
#   agnosticity [--staged]        Check portable Bubbles surfaces for drift
#   guard <spec>                  Run state transition guard on a spec
#   runtime-selftest              Run runtime lease selftest coverage
#   finding-closure-selftest      Run finding-set closure selftest coverage
#   scan <spec>                   Run implementation reality scan on a spec
#   regression-quality [args...]  Run bailout/adversarial regression quality scan on test files or dirs
#   docs-registry [mode]          Show framework-default or effective managed-doc registry
#   framework-write-guard         Check downstream framework-managed files against install provenance
#   interop <subcommand>          Detect, import, apply, and inspect project-owned interop packets
#   framework-validate            Run framework self-validation across core guard and selftest surfaces
#   release-check                 Run source-repo release hygiene checks
#   framework-events [options]    Show typed framework event history
#   run-state [options]           Show active and recent workflow run-state records
#   repo-readiness [path] [--profile PROFILE]  Run advisory repo-readiness checks
#   framework-proposal <slug>     Scaffold a project-owned upstream Bubbles change proposal
#   audit-done [--fix]            Audit all specs marked done
#   autofix <spec>                Scaffold missing report sections
#   metrics <subcommand>          Manage metrics and activity tracking
#   lessons [--all|compact]       View or compact lessons-learned memory
#   skill-proposals [subcommand]  Show or dismiss generated skill proposals
#   profile [subcommand]          Show, list, or change adoption profiles plus developer observations
#   sunnyvale <alias>             Resolve a Sunnyvale alias (agent or mode)
#   aliases                       List all Sunnyvale aliases
#   help                          Show this help message
#
# Spec argument formats:
#   027                           Resolves to specs/027-* (first match)
#   specs/027-feature-name        Full path
#   027-feature-name              Folder name
# ────────────────────────────────────────────────────────────────────

set -uo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

# Source alias resolution
source "$(dirname "${BASH_SOURCE[0]}")/aliases.sh"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/.github/bubbles"
  AGENTS_DIR="$REPO_ROOT/.github/agents"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/bubbles"
  AGENTS_DIR="$REPO_ROOT/agents"
fi
SPECS_DIR="$REPO_ROOT/specs"
SESSION_FILE="$REPO_ROOT/.specify/memory/bubbles.session.json"
CONTROL_PLANE_CONFIG="$REPO_ROOT/.specify/memory/bubbles.config.json"
CONTROL_PLANE_METRICS_DIR="$REPO_ROOT/.specify/metrics"
CONTROL_PLANE_METRICS_FILE="$CONTROL_PLANE_METRICS_DIR/events.jsonl"
CONTROL_PLANE_ACTIVITY_FILE="$CONTROL_PLANE_METRICS_DIR/activity.jsonl"
CONTROL_PLANE_OBSERVATIONS_FILE="$CONTROL_PLANE_METRICS_DIR/observations.jsonl"
CONTROL_PLANE_RUNTIME_DIR="$REPO_ROOT/.specify/runtime"
CONTROL_PLANE_RUNTIME_FILE="$CONTROL_PLANE_RUNTIME_DIR/resource-leases.json"
CONTROL_PLANE_EVENT_FILE="$CONTROL_PLANE_RUNTIME_DIR/framework-events.jsonl"
CONTROL_PLANE_RUN_STATE_FILE="$CONTROL_PLANE_RUNTIME_DIR/workflow-runs.json"
LESSONS_FILE="$REPO_ROOT/.specify/memory/lessons.md"
SKILL_PROPOSALS_FILE="$REPO_ROOT/.specify/memory/skill-proposals.md"
DEVELOPER_PROFILE_FILE="$REPO_ROOT/.specify/memory/developer-profile.md"
ACTION_RISK_REGISTRY_FILE="$FRAMEWORK_DIR/action-risk-registry.yaml"
ADOPTION_PROFILES_FILE="$FRAMEWORK_DIR/adoption-profiles.yaml"

# ── Colors ──────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'; YELLOW='\033[0;33m'; RED='\033[0;31m'
  BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'
  DIM='\033[2m'; NC='\033[0m'
else
  GREEN='' YELLOW='' RED='' BLUE='' CYAN='' BOLD='' DIM='' NC=''
fi

# ── Helpers ─────────────────────────────────────────────────────────

die() { echo -e "${RED}Error:${NC} $*" >&2; exit 1; }

source "$SCRIPT_DIR/trust-metadata.sh"

active_adoption_profile() {
  if [[ -f "$CONTROL_PLANE_CONFIG" ]]; then
    grep -oE '"adoptionProfile"[[:space:]]*:[[:space:]]*"[^"]+"' "$CONTROL_PLANE_CONFIG" 2>/dev/null \
      | head -1 \
      | sed -E 's/.*"([^"]+)"$/\1/'
  fi
}

adoption_profile_ids() {
  [[ -f "$ADOPTION_PROFILES_FILE" ]] || return 0

  awk '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && /^  [A-Za-z0-9_-]+:$/ {
      profile=$1
      sub(":$", "", profile)
      print profile
    }
  ' "$ADOPTION_PROFILES_FILE"
}

adoption_profile_value() {
  local profile="$1"
  local key="$2"

  [[ -f "$ADOPTION_PROFILES_FILE" ]] || return 1

  awk -v profile="$profile" -v key="$key" '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && $0 ~ ("^  " profile ":$") { in_profile=1; next }
    in_profile && /^  [A-Za-z0-9_-]+:$/ { in_profile=0 }
    in_profile && $0 ~ ("^    " key ":") {
      sub("^    " key ":[[:space:]]*", "", $0)
      gsub(/^"|"$/, "", $0)
      print
      exit
    }
  ' "$ADOPTION_PROFILES_FILE"
}

adoption_profile_list() {
  local profile="$1"
  local key="$2"

  [[ -f "$ADOPTION_PROFILES_FILE" ]] || return 0

  awk -v profile="$profile" -v key="$key" '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && $0 ~ ("^  " profile ":$") { in_profile=1; next }
    in_profile && /^  [A-Za-z0-9_-]+:$/ { in_profile=0; in_list=0 }
    in_profile && $0 ~ ("^    " key ":$") { in_list=1; next }
    in_list && /^    [A-Za-z0-9_-]+:/ { in_list=0 }
    in_list && /^    - / {
      sub(/^    - /, "", $0)
      gsub(/^"|"$/, "", $0)
      print
    }
  ' "$ADOPTION_PROFILES_FILE"
}

effective_adoption_profile() {
  local requested_profile="${1:-}"
  local profile="$requested_profile"

  if [[ -z "$profile" ]]; then
    profile="$(active_adoption_profile)"
  fi

  if [[ -z "$profile" ]]; then
    profile='delivery'
  fi

  local known_profile
  while IFS= read -r known_profile; do
    [[ -n "$known_profile" ]] || continue
    if [[ "$known_profile" == "$profile" ]]; then
      printf '%s' "$profile"
      return 0
    fi
  done < <(adoption_profile_ids)

  printf '%s' 'delivery'
}

is_framework_repo() {
  [[ "$SCRIPT_DIR" != *"/.github/bubbles/scripts" ]]
}

project_root() {
  printf '%s\n' "$REPO_ROOT"
}

require_framework_repo_for_hooks() {
  if ! is_framework_repo; then
    die "Bubbles git hooks may only be installed in the Bubbles framework repo. Consumer repos should use Bubbles but must not install Bubbles-managed pre-commit/pre-push hooks."
  fi
}

require_downstream_repo_for_framework_proposal() {
  if is_framework_repo; then
    die "Framework proposals belong in consumer repos. Implement the change directly in the Bubbles source repo instead of scaffolding a downstream proposal here."
  fi
}

# Resolve a spec identifier to a directory path
resolve_spec() {
  local input="$1"

  # Already a valid path
  if [[ -d "$input" ]]; then echo "$input"; return 0; fi

  # Try specs/<input>
  if [[ -d "$SPECS_DIR/$input" ]]; then echo "$SPECS_DIR/$input"; return 0; fi

  # Try numeric prefix match: 027 → specs/027-*
  if [[ "$input" =~ ^[0-9]+$ ]]; then
    local padded
    padded=$(printf "%03d" "$input")
    local match
    match=$(find "$SPECS_DIR" -maxdepth 1 -type d -name "${padded}-*" 2>/dev/null | head -1)
    if [[ -n "$match" ]]; then echo "$match"; return 0; fi
  fi

  die "Cannot resolve spec: $input\nTry: number (027), name (027-feature), or path (specs/027-feature)"
}

# Extract a JSON string field value (simple grep-based, no jq dependency)
json_field() {
  local file="$1" field="$2"
  grep -oE "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]+\"" "$file" 2>/dev/null \
    | head -1 | sed -E 's/.*"([^"]+)"$/\1/' || true
}

default_control_plane_config() {
  cat << 'EOF'
{
  "version": 2,
  "adoptionProfile": "delivery",
  "defaults": {
    "grill": {
      "mode": "off",
      "source": "repo-default"
    },
    "tdd": {
      "mode": "scenario-first",
      "defaultForModes": ["bugfix-fastlane", "chaos-hardening"],
      "source": "repo-default"
    },
    "autoCommit": {
      "mode": "off",
      "source": "repo-default"
    },
    "lockdown": {
      "default": false,
      "requireGrillForInvalidation": true,
      "source": "repo-default"
    },
    "regression": {
      "immutability": "protected-scenarios",
      "source": "repo-default"
    },
    "validation": {
      "certificationRequired": true,
      "source": "repo-default"
    },
    "runtime": {
      "leaseTtlMinutes": 20,
      "staleAfterMinutes": 60,
      "reusePolicy": "fingerprint-match-only",
      "source": "repo-default"
    }
  },
  "modeOverrides": {
    "bugfix-fastlane": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    },
    "chaos-hardening": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    }
  },
  "metrics": {
    "enabled": false,
    "activityTrackingEnabled": false
  }
}
EOF
}

ensure_control_plane_config() {
  mkdir -p "$(dirname "$CONTROL_PLANE_CONFIG")" "$CONTROL_PLANE_METRICS_DIR"
  if [[ ! -f "$CONTROL_PLANE_CONFIG" ]]; then
    default_control_plane_config > "$CONTROL_PLANE_CONFIG"
  fi
}

bootstrap_placeholder_count() {
  local target_file="$1"

  if [[ ! -f "$target_file" ]]; then
    printf '%s' '0'
    return 0
  fi

  grep -oE '\[TODO([^]]*)\]|> \*\*TODO:\*\*|\{\{[A-Z_]+\}\}' "$target_file" 2>/dev/null | wc -l | tr -d ' '
}

json_escape() {
  local raw="$1"
  raw=${raw//\\/\\\\}
  raw=${raw//"/\\"}
  raw=${raw//$'\n'/\\n}
  raw=${raw//$'\r'/\\r}
  raw=${raw//$'\t'/\\t}
  printf '%s' "$raw"
}

append_jsonl() {
  local target_file="$1"
  local payload="$2"

  mkdir -p "$(dirname "$target_file")"
  printf '%s\n' "$payload" >> "$target_file"
}

current_timestamp() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

derive_session_id() {
  if [[ -n "${BUBBLES_SESSION_ID:-}" ]]; then
    printf '%s' "$BUBBLES_SESSION_ID"
    return 0
  fi

  if [[ -f "$SESSION_FILE" ]]; then
    local session_id
    session_id="$({ grep -oE '"sessionId"[[:space:]]*:[[:space:]]*"[^"]+"' "$SESSION_FILE" | head -1 | sed -E 's/.*"([^"]+)"$/\1/'; } || true)"
    if [[ -n "$session_id" ]]; then
      printf '%s' "$session_id"
      return 0
    fi
  fi

  printf 'shell-%s' "$$"
}

derive_agent_name() {
  if [[ -n "${BUBBLES_AGENT_NAME:-}" ]]; then
    printf '%s' "$BUBBLES_AGENT_NAME"
  else
    printf '%s' 'cli'
  fi
}

current_branch_name() {
  git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || printf '%s' 'unknown'
}

generate_run_id() {
  printf 'wrn_%s_%04d' "$(date -u +%Y%m%d%H%M%S)" "$((RANDOM % 10000))"
}

ensure_runtime_event_log() {
  mkdir -p "$CONTROL_PLANE_RUNTIME_DIR"
  touch "$CONTROL_PLANE_EVENT_FILE"
}

ensure_run_state_registry() {
  mkdir -p "$CONTROL_PLANE_RUNTIME_DIR"
  if [[ ! -f "$CONTROL_PLANE_RUN_STATE_FILE" ]]; then
    cat > "$CONTROL_PLANE_RUN_STATE_FILE" <<'EOF'
{
  "version": 1,
  "activeRuns": [
  ],
  "recentRuns": [
  ]
}
EOF
  fi
}

run_state_lines() {
  local section="$1"

  ensure_run_state_registry

  awk -v target="$section" '
    $0 ~ "\"" target "\"[[:space:]]*:[[:space:]]*\\[" { in_target = 1; next }
    in_target && /^[[:space:]]*\]/ { exit }
    in_target && /\{/ {
      gsub(/^[[:space:]]+/, "", $0)
      sub(/,[[:space:]]*$/, "", $0)
      print $0
    }
  ' "$CONTROL_PLANE_RUN_STATE_FILE"
}

write_run_state_registry() {
  local active_lines="$1"
  local recent_lines="$2"
  local tmp_file="$CONTROL_PLANE_RUN_STATE_FILE.tmp"
  local active_count=0 recent_count=0 active_index=0 recent_index=0 line

  active_count=$(printf '%s\n' "$active_lines" | sed '/^$/d' | wc -l | tr -d ' ')
  recent_count=$(printf '%s\n' "$recent_lines" | sed '/^$/d' | wc -l | tr -d ' ')

  {
    echo '{'
    echo '  "version": 1,'
    echo '  "activeRuns": ['
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      active_index=$((active_index + 1))
      if [[ "$active_index" -lt "$active_count" ]]; then
        printf '    %s,\n' "$line"
      else
        printf '    %s\n' "$line"
      fi
    done <<< "$active_lines"
    echo '  ],'
    echo '  "recentRuns": ['
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      recent_index=$((recent_index + 1))
      if [[ "$recent_index" -lt "$recent_count" ]]; then
        printf '    %s,\n' "$line"
      else
        printf '    %s\n' "$line"
      fi
    done <<< "$recent_lines"
    echo '  ]'
    echo '}'
  } > "$tmp_file"

  mv "$tmp_file" "$CONTROL_PLANE_RUN_STATE_FILE"
}

field_from_json_line() {
  local line="$1"
  local field="$2"

  printf '%s\n' "$line" | sed -nE "s/.*\"${field}\":\"([^\"]*)\".*/\1/p"
}

runtime_attachment_for_session() {
  local session_id="$1"
  local attachments=''
  local line lease_id

  [[ -n "$session_id" && -f "$CONTROL_PLANE_RUNTIME_FILE" ]] || {
    printf '%s' ''
    return 0
  }

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if [[ "$line" == *'"status":"active"'* ]] && [[ "$line" == *"\"$session_id\""* ]]; then
      lease_id="$(field_from_json_line "$line" 'leaseId')"
      [[ -n "$lease_id" ]] || continue
      if [[ -z "$attachments" ]]; then
        attachments="$lease_id"
      else
        attachments="$attachments,$lease_id"
      fi
    fi
  done < <(awk '
    /"leases"[[:space:]]*:[[:space:]]*\[/ { in_leases = 1; next }
    in_leases && /^[[:space:]]*\]/ { exit }
    in_leases && /"leaseId"/ {
      gsub(/^[[:space:]]+/, "", $0)
      sub(/,[[:space:]]*$/, "", $0)
      print $0
    }
  ' "$CONTROL_PLANE_RUNTIME_FILE")

  printf '%s' "$attachments"
}

classify_run_posture() {
  local command_name="$1"
  local command_args="$2"

  case "$command_name" in
    session) printf '%s' 'resume' ;;
    *)
      if [[ "$command_args" == *'--retry'* || "$command_args" == *' retry'* ]]; then
        printf '%s' 'retry'
      elif [[ "$command_args" == *'resume'* || "$command_args" == *'continue'* ]]; then
        printf '%s' 'resume'
      else
        printf '%s' 'fresh'
      fi
      ;;
  esac
}

registry_default_risk_class() {
  local command_name="$1"
  local value=''

  if [[ -f "$ACTION_RISK_REGISTRY_FILE" ]]; then
    value="$({
      awk -v cmd="$command_name" '
        $0 == "  " cmd ":" { in_cmd = 1; next }
        in_cmd && $0 ~ /^  [^[:space:]].*:/ { exit }
        in_cmd && /defaultRiskClass:/ {
          sub(/.*defaultRiskClass:[[:space:]]*/, "", $0)
          print $0
          exit
        }
      ' "$ACTION_RISK_REGISTRY_FILE"
    } || true)"
  fi

  if [[ -n "$value" ]]; then
    printf '%s' "$value"
  else
    printf '%s' 'read_only'
  fi
}

command_effective_risk_class() {
  local command_name="$1"
  local command_args="$2"
  local default_risk

  default_risk="$(registry_default_risk_class "$command_name")"

  case "$command_name" in
    doctor)
      if [[ "$command_args" == *'--heal'* ]]; then
        printf '%s' 'owned_mutation'
      else
        printf '%s' "$default_risk"
      fi
      ;;
    hooks|framework-proposal|autofix|upgrade)
      printf '%s' 'owned_mutation'
      ;;
    policy)
      if [[ "$command_args" == set* || "$command_args" == reset* ]]; then
        printf '%s' 'owned_mutation'
      else
        printf '%s' "$default_risk"
      fi
      ;;
    runtime)
      case "${command_args%% *}" in
        release|reclaim-stale) printf '%s' 'runtime_teardown' ;;
        acquire|attach|heartbeat) printf '%s' 'owned_mutation' ;;
        *) printf '%s' "$default_risk" ;;
      esac
      ;;
    *)
      printf '%s' "$default_risk"
      ;;
  esac
}

record_framework_event() {
  local event_type="$1"
  local result="$2"
  local duration_ms="$3"
  local details="$4"
  local risk_class="$5"
  local target="$6"
  local run_id="$7"

  ensure_runtime_event_log
  append_jsonl "$CONTROL_PLANE_EVENT_FILE" "{\"version\":1,\"type\":\"$(json_escape "$event_type")\",\"timestamp\":\"$(current_timestamp)\",\"runId\":\"$(json_escape "$run_id")\",\"sessionId\":\"$(json_escape "${CURRENT_SESSION_ID:-unknown}")\",\"command\":\"$(json_escape "${CURRENT_BUBBLES_COMMAND:-unknown}")\",\"target\":\"$(json_escape "$target")\",\"riskClass\":\"$(json_escape "$risk_class")\",\"result\":\"$(json_escape "$result")\",\"durationMs\":${duration_ms},\"details\":\"$(json_escape "$details")\"}"
}

build_run_record_line() {
  local run_id="$1"
  local status="$2"
  local started_at="$3"
  local updated_at="$4"
  local completed_at="$5"
  local result="$6"
  local duration_ms="$7"
  local target="$8"
  local runtime_attachment="$9"
  local posture="${10}"
  local risk_class="${11}"

  printf '{"runId":"%s","command":"%s","args":"%s","sessionId":"%s","agent":"%s","repo":"%s","branch":"%s","worktree":"%s","status":"%s","startedAt":"%s","updatedAt":"%s","completedAt":"%s","result":"%s","durationMs":%s,"target":"%s","runtimeAttachment":"%s","posture":"%s","riskClass":"%s"}' \
    "$(json_escape "$run_id")" \
    "$(json_escape "${CURRENT_BUBBLES_COMMAND:-unknown}")" \
    "$(json_escape "${CURRENT_BUBBLES_ARGS:-}")" \
    "$(json_escape "${CURRENT_SESSION_ID:-unknown}")" \
    "$(json_escape "${CURRENT_AGENT_NAME:-cli}")" \
    "$(json_escape "$(basename "$REPO_ROOT")")" \
    "$(json_escape "${CURRENT_BRANCH_NAME:-unknown}")" \
    "$(json_escape "$REPO_ROOT")" \
    "$(json_escape "$status")" \
    "$(json_escape "$started_at")" \
    "$(json_escape "$updated_at")" \
    "$(json_escape "$completed_at")" \
    "$(json_escape "$result")" \
    "$duration_ms" \
    "$(json_escape "$target")" \
    "$(json_escape "$runtime_attachment")" \
    "$(json_escape "$posture")" \
    "$(json_escape "$risk_class")"
}

begin_cli_run_state() {
  local active_lines new_line

  CURRENT_SESSION_ID="$(derive_session_id)"
  CURRENT_AGENT_NAME="$(derive_agent_name)"
  CURRENT_BRANCH_NAME="$(current_branch_name)"
  CURRENT_RUN_ID="$(generate_run_id)"
  CURRENT_RUN_STARTED_AT="$(current_timestamp)"
  CURRENT_RUN_POSTURE="$(classify_run_posture "${CURRENT_BUBBLES_COMMAND:-unknown}" "${CURRENT_BUBBLES_ARGS:-}")"
  CURRENT_RISK_CLASS="$(command_effective_risk_class "${CURRENT_BUBBLES_COMMAND:-unknown}" "${CURRENT_BUBBLES_ARGS:-}")"

  ensure_run_state_registry
  active_lines="$(run_state_lines activeRuns)"
  new_line="$(build_run_record_line "$CURRENT_RUN_ID" 'active' "$CURRENT_RUN_STARTED_AT" "$CURRENT_RUN_STARTED_AT" '' 'pending' 0 "$(first_tracking_target "${CURRENT_BUBBLES_ARGS:-}")" "$(runtime_attachment_for_session "$CURRENT_SESSION_ID")" "$CURRENT_RUN_POSTURE" "$CURRENT_RISK_CLASS")"
  if [[ -n "$active_lines" ]]; then
    active_lines="$active_lines
$new_line"
  else
    active_lines="$new_line"
  fi
  write_run_state_registry "$active_lines" "$(run_state_lines recentRuns)"
  record_framework_event "framework_command_started" "pending" 0 "args=${CURRENT_BUBBLES_ARGS:-}" "$CURRENT_RISK_CLASS" "$(first_tracking_target "${CURRENT_BUBBLES_ARGS:-}")" "$CURRENT_RUN_ID"
}

trim_recent_run_lines() {
  local recent_lines="$1"
  printf '%s\n' "$recent_lines" | sed '/^$/d' | tail -n 25
}

complete_cli_run_state() {
  local result="$1"
  local duration_ms="$2"
  local target runtime_attachment active_lines recent_lines line updated_recent completed_line

  [[ -n "${CURRENT_RUN_ID:-}" ]] || return 0

  ensure_run_state_registry
  target="$(first_tracking_target "${CURRENT_BUBBLES_ARGS:-}")"
  runtime_attachment="$(runtime_attachment_for_session "${CURRENT_SESSION_ID:-unknown}")"

  active_lines=''
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if [[ "$(field_from_json_line "$line" 'runId')" == "$CURRENT_RUN_ID" ]]; then
      continue
    fi
    if [[ -z "$active_lines" ]]; then
      active_lines="$line"
    else
      active_lines="$active_lines
$line"
    fi
  done <<< "$(run_state_lines activeRuns)"

  completed_line="$(build_run_record_line "$CURRENT_RUN_ID" 'completed' "${CURRENT_RUN_STARTED_AT:-$(current_timestamp)}" "$(current_timestamp)" "$(current_timestamp)" "$result" "$duration_ms" "$target" "$runtime_attachment" "${CURRENT_RUN_POSTURE:-fresh}" "${CURRENT_RISK_CLASS:-read_only}")"
  recent_lines="$(run_state_lines recentRuns)"
  if [[ -n "$recent_lines" ]]; then
    updated_recent="$recent_lines
$completed_line"
  else
    updated_recent="$completed_line"
  fi

  write_run_state_registry "$active_lines" "$(trim_recent_run_lines "$updated_recent")"
}

slugify() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
}

activity_tracking_enabled() {
  [[ "${CFG_METRICS_ENABLED:-false}" == "true" && "${CFG_ACTIVITY_TRACKING_ENABLED:-false}" == "true" ]]
}

current_epoch_ms() {
  date +%s%3N
}

record_metric_event() {
  local event_type="$1"
  local result="$2"
  local duration_ms="$3"
  local details="$4"

  if [[ "${CFG_METRICS_ENABLED:-false}" != "true" ]]; then
    return 0
  fi

  append_jsonl "$CONTROL_PLANE_METRICS_FILE" "{\"type\":\"$(json_escape "$event_type")\",\"timestamp\":\"$(current_timestamp)\",\"command\":\"$(json_escape "${CURRENT_BUBBLES_COMMAND:-unknown}")\",\"result\":\"$(json_escape "$result")\",\"durationMs\":${duration_ms},\"details\":\"$(json_escape "$details")\"}"
}

record_observation() {
  local observation_type="$1"
  local value="$2"
  local source="$3"
  local confidence="$4"

  append_jsonl "$CONTROL_PLANE_OBSERVATIONS_FILE" "{\"timestamp\":\"$(current_timestamp)\",\"type\":\"$(json_escape "$observation_type")\",\"value\":\"$(json_escape "$value")\",\"source\":\"$(json_escape "$source")\",\"confidence\":\"$(json_escape "$confidence")\"}"
}

record_activity_event() {
  local command_name="$1"
  local result="$2"
  local duration_ms="$3"
  local target="$4"
  local detail_args="$5"

  if ! activity_tracking_enabled; then
    return 0
  fi

  append_jsonl "$CONTROL_PLANE_ACTIVITY_FILE" "{\"timestamp\":\"$(current_timestamp)\",\"command\":\"$(json_escape "$command_name")\",\"phase\":\"$(json_escape "$command_name")\",\"result\":\"$(json_escape "$result")\",\"durationMs\":${duration_ms},\"target\":\"$(json_escape "$target")\",\"args\":\"$(json_escape "$detail_args")\"}"
}

first_tracking_target() {
  local raw_args="${1:-}"
  local first_word

  if [[ -z "$raw_args" ]]; then
    printf '%s' 'global'
    return 0
  fi

  first_word="${raw_args%% *}"

  case "$first_word" in
    --*) printf '%s' 'global' ;;
    *) printf '%s' "$first_word" ;;
  esac
}

record_cli_completion() {
  local exit_code="$1"
  local end_ms duration_ms result target

  if [[ "${CLI_RECORDING_ACTIVE:-false}" != "true" ]]; then
    return 0
  fi

  end_ms="$(current_epoch_ms)"
  duration_ms=0
  if [[ -n "${COMMAND_START_MS:-}" ]]; then
    duration_ms=$((end_ms - COMMAND_START_MS))
  fi

  if [[ "$exit_code" -eq 0 ]]; then
    result="success"
  else
    result="fail"
  fi

  if [[ -n "${CURRENT_BUBBLES_COMMAND:-}" ]]; then
    load_control_plane_config
    target="$(first_tracking_target "${CURRENT_BUBBLES_ARGS:-}")"
    record_metric_event "cli_command" "$result" "$duration_ms" "args=${CURRENT_BUBBLES_ARGS:-}"
    record_activity_event "$CURRENT_BUBBLES_COMMAND" "$result" "$duration_ms" "$target" "${CURRENT_BUBBLES_ARGS:-}"
    record_framework_event "framework_command_completed" "$result" "$duration_ms" "args=${CURRENT_BUBBLES_ARGS:-}" "${CURRENT_RISK_CLASS:-read_only}" "$target" "${CURRENT_RUN_ID:-unknown}"
    complete_cli_run_state "$result" "$duration_ms"
  fi
}

config_string_value() {
  local section="$1"
  local key="$2"
  local default_value="$3"
  local file="$4"
  local value

  value="$({
    grep -A8 "\"$section\"" "$file" 2>/dev/null \
      | grep -m1 "\"$key\"" \
      | sed -E 's/.*:[[:space:]]*"([^"]+)".*/\1/'
  } || true)"

  if [[ -n "$value" ]]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

config_bool_value() {
  local section="$1"
  local key="$2"
  local default_value="$3"
  local file="$4"
  local value

  value="$({
    grep -A8 "\"$section\"" "$file" 2>/dev/null \
      | grep -m1 "\"$key\"" \
      | sed -E 's/.*:[[:space:]]*(true|false).*/\1/'
  } || true)"

  if [[ "$value" == "true" || "$value" == "false" ]]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

config_metrics_enabled_value() {
  local file="$1"
  local legacy_value
  local value

  legacy_value="$({
    grep -m1 -E '"metrics"[[:space:]]*:[[:space:]]*(true|false)' "$file" 2>/dev/null \
      | sed -E 's/.*:[[:space:]]*(true|false).*/\1/'
  } || true)"
  if [[ "$legacy_value" == "true" || "$legacy_value" == "false" ]]; then
    printf '%s' "$legacy_value"
    return 0
  fi

  value="$({
    grep -A3 '"metrics"' "$file" 2>/dev/null \
      | grep -m1 '"enabled"' \
      | sed -E 's/.*:[[:space:]]*(true|false).*/\1/'
  } || true)"
  if [[ "$value" == "true" || "$value" == "false" ]]; then
    printf '%s' "$value"
  else
    printf '%s' 'false'
  fi
}

config_activity_tracking_enabled_value() {
  local file="$1"
  local value

  value="$({
    grep -A4 '"metrics"' "$file" 2>/dev/null \
      | grep -m1 '"activityTrackingEnabled"' \
      | sed -E 's/.*:[[:space:]]*(true|false).*/\1/'
  } || true)"

  if [[ "$value" == "true" || "$value" == "false" ]]; then
    printf '%s' "$value"
  else
    printf '%s' 'false'
  fi
}

config_number_value() {
  local section="$1"
  local key="$2"
  local default_value="$3"
  local file="$4"
  local value

  value="$({
    grep -A8 "\"$section\"" "$file" 2>/dev/null \
      | grep -m1 "\"$key\"" \
      | sed -E 's/.*:[[:space:]]*([0-9]+).*/\1/'
  } || true)"

  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

load_control_plane_config() {
  ensure_control_plane_config

  CFG_ADOPTION_PROFILE="$(active_adoption_profile)"
  [[ -n "$CFG_ADOPTION_PROFILE" ]] || CFG_ADOPTION_PROFILE='delivery'
  CFG_GRILL_MODE="$(config_string_value grill mode off "$CONTROL_PLANE_CONFIG")"
  CFG_GRILL_SOURCE="$(config_string_value grill source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_TDD_MODE="$(config_string_value tdd mode scenario-first "$CONTROL_PLANE_CONFIG")"
  CFG_TDD_SOURCE="$(config_string_value tdd source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_AUTOCOMMIT_MODE="$(config_string_value autoCommit mode off "$CONTROL_PLANE_CONFIG")"
  CFG_AUTOCOMMIT_SOURCE="$(config_string_value autoCommit source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_LOCKDOWN_DEFAULT="$(config_bool_value lockdown default false "$CONTROL_PLANE_CONFIG")"
  CFG_LOCKDOWN_REQUIRE_GRILL="$(config_bool_value lockdown requireGrillForInvalidation true "$CONTROL_PLANE_CONFIG")"
  CFG_LOCKDOWN_SOURCE="$(config_string_value lockdown source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_REGRESSION_IMMUTABILITY="$(config_string_value regression immutability protected-scenarios "$CONTROL_PLANE_CONFIG")"
  CFG_REGRESSION_SOURCE="$(config_string_value regression source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_VALIDATION_CERT_REQUIRED="$(config_bool_value validation certificationRequired true "$CONTROL_PLANE_CONFIG")"
  CFG_VALIDATION_SOURCE="$(config_string_value validation source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_RUNTIME_LEASE_TTL_MINUTES="$(config_number_value runtime leaseTtlMinutes 20 "$CONTROL_PLANE_CONFIG")"
  CFG_RUNTIME_STALE_AFTER_MINUTES="$(config_number_value runtime staleAfterMinutes 60 "$CONTROL_PLANE_CONFIG")"
  CFG_RUNTIME_REUSE_POLICY="$(config_string_value runtime reusePolicy fingerprint-match-only "$CONTROL_PLANE_CONFIG")"
  CFG_RUNTIME_SOURCE="$(config_string_value runtime source repo-default "$CONTROL_PLANE_CONFIG")"
  CFG_METRICS_ENABLED="$(config_metrics_enabled_value "$CONTROL_PLANE_CONFIG")"
  CFG_ACTIVITY_TRACKING_ENABLED="$(config_activity_tracking_enabled_value "$CONTROL_PLANE_CONFIG")"
}

save_control_plane_config() {
  local tmp_file="$CONTROL_PLANE_CONFIG.tmp"

  mkdir -p "$(dirname "$CONTROL_PLANE_CONFIG")" "$CONTROL_PLANE_METRICS_DIR"
  cat > "$tmp_file" << EOF
{
  "version": 2,
  "adoptionProfile": "$CFG_ADOPTION_PROFILE",
  "defaults": {
    "grill": {
      "mode": "$CFG_GRILL_MODE",
      "source": "$CFG_GRILL_SOURCE"
    },
    "tdd": {
      "mode": "$CFG_TDD_MODE",
      "defaultForModes": ["bugfix-fastlane", "chaos-hardening"],
      "source": "$CFG_TDD_SOURCE"
    },
    "autoCommit": {
      "mode": "$CFG_AUTOCOMMIT_MODE",
      "source": "$CFG_AUTOCOMMIT_SOURCE"
    },
    "lockdown": {
      "default": $CFG_LOCKDOWN_DEFAULT,
      "requireGrillForInvalidation": $CFG_LOCKDOWN_REQUIRE_GRILL,
      "source": "$CFG_LOCKDOWN_SOURCE"
    },
    "regression": {
      "immutability": "$CFG_REGRESSION_IMMUTABILITY",
      "source": "$CFG_REGRESSION_SOURCE"
    },
    "validation": {
      "certificationRequired": $CFG_VALIDATION_CERT_REQUIRED,
      "source": "$CFG_VALIDATION_SOURCE"
    },
    "runtime": {
      "leaseTtlMinutes": $CFG_RUNTIME_LEASE_TTL_MINUTES,
      "staleAfterMinutes": $CFG_RUNTIME_STALE_AFTER_MINUTES,
      "reusePolicy": "$CFG_RUNTIME_REUSE_POLICY",
      "source": "$CFG_RUNTIME_SOURCE"
    }
  },
  "modeOverrides": {
    "bugfix-fastlane": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    },
    "chaos-hardening": {
      "tdd": {
        "mode": "scenario-first",
        "source": "workflow-forced"
      }
    }
  },
  "metrics": {
    "enabled": $CFG_METRICS_ENABLED,
    "activityTrackingEnabled": $CFG_ACTIVITY_TRACKING_ENABLED
  }
}
EOF
  mv "$tmp_file" "$CONTROL_PLANE_CONFIG"
}

default_value_for_policy_path() {
  local path="$1"

  case "$path" in
    grill.mode) printf '%s' 'off' ;;
    grill.source) printf '%s' 'repo-default' ;;
    tdd.mode) printf '%s' 'scenario-first' ;;
    tdd.source) printf '%s' 'repo-default' ;;
    autoCommit.mode) printf '%s' 'off' ;;
    autoCommit.source) printf '%s' 'repo-default' ;;
    lockdown.default) printf '%s' 'false' ;;
    lockdown.requireGrillForInvalidation) printf '%s' 'true' ;;
    lockdown.source) printf '%s' 'repo-default' ;;
    regression.immutability) printf '%s' 'protected-scenarios' ;;
    regression.source) printf '%s' 'repo-default' ;;
    validation.certificationRequired) printf '%s' 'true' ;;
    validation.source) printf '%s' 'repo-default' ;;
    runtime.leaseTtlMinutes) printf '%s' '20' ;;
    runtime.staleAfterMinutes) printf '%s' '60' ;;
    runtime.reusePolicy) printf '%s' 'fingerprint-match-only' ;;
    runtime.source) printf '%s' 'repo-default' ;;
    metrics.enabled) printf '%s' 'false' ;;
    metrics.activityTrackingEnabled) printf '%s' 'false' ;;
    bugfix-fastlane.tdd.mode|chaos-hardening.tdd.mode) printf '%s' 'scenario-first' ;;
    bugfix-fastlane.tdd.source|chaos-hardening.tdd.source) printf '%s' 'workflow-forced' ;;
    *) return 1 ;;
  esac
}

read_control_plane_setting() {
  local path="$1"

  case "$path" in
    grill.mode) printf '%s' "$CFG_GRILL_MODE" ;;
    grill.source) printf '%s' "$CFG_GRILL_SOURCE" ;;
    tdd.mode) printf '%s' "$CFG_TDD_MODE" ;;
    tdd.source) printf '%s' "$CFG_TDD_SOURCE" ;;
    autoCommit.mode) printf '%s' "$CFG_AUTOCOMMIT_MODE" ;;
    autoCommit.source) printf '%s' "$CFG_AUTOCOMMIT_SOURCE" ;;
    lockdown.default) printf '%s' "$CFG_LOCKDOWN_DEFAULT" ;;
    lockdown.requireGrillForInvalidation) printf '%s' "$CFG_LOCKDOWN_REQUIRE_GRILL" ;;
    lockdown.source) printf '%s' "$CFG_LOCKDOWN_SOURCE" ;;
    regression.immutability) printf '%s' "$CFG_REGRESSION_IMMUTABILITY" ;;
    regression.source) printf '%s' "$CFG_REGRESSION_SOURCE" ;;
    validation.certificationRequired) printf '%s' "$CFG_VALIDATION_CERT_REQUIRED" ;;
    validation.source) printf '%s' "$CFG_VALIDATION_SOURCE" ;;
    runtime.leaseTtlMinutes) printf '%s' "$CFG_RUNTIME_LEASE_TTL_MINUTES" ;;
    runtime.staleAfterMinutes) printf '%s' "$CFG_RUNTIME_STALE_AFTER_MINUTES" ;;
    runtime.reusePolicy) printf '%s' "$CFG_RUNTIME_REUSE_POLICY" ;;
    runtime.source) printf '%s' "$CFG_RUNTIME_SOURCE" ;;
    metrics.enabled) printf '%s' "$CFG_METRICS_ENABLED" ;;
    metrics.activityTrackingEnabled) printf '%s' "$CFG_ACTIVITY_TRACKING_ENABLED" ;;
    bugfix-fastlane.tdd.mode|chaos-hardening.tdd.mode) printf '%s' 'scenario-first' ;;
    bugfix-fastlane.tdd.source|chaos-hardening.tdd.source) printf '%s' 'workflow-forced' ;;
    *) return 1 ;;
  esac
}

validate_policy_source() {
  local source="$1"
  case "$source" in
    user-request|repo-default|workflow-forced|spec-lockdown) return 0 ;;
    *) die "Invalid policy source: $source. Allowed: user-request, repo-default, workflow-forced, spec-lockdown" ;;
  esac
}

validate_boolean_literal() {
  local value="$1"
  case "$value" in
    true|false) return 0 ;;
    *) die "Invalid boolean value: $value. Use true or false." ;;
  esac
}

validate_policy_value() {
  local path="$1"
  local value="$2"

  case "$path" in
    grill.mode)
      case "$value" in
        off|on-demand|required-on-ambiguity|required-for-lockdown) ;;
        *) die "Invalid grill.mode: $value" ;;
      esac
      ;;
    tdd.mode)
      case "$value" in
        scenario-first|off) ;;
        *) die "Invalid tdd.mode: $value" ;;
      esac
      ;;
    autoCommit.mode)
      case "$value" in
        off|scope|dod) ;;
        *) die "Invalid autoCommit.mode: $value" ;;
      esac
      ;;
    lockdown.default|lockdown.requireGrillForInvalidation|validation.certificationRequired|metrics.enabled|metrics.activityTrackingEnabled)
      validate_boolean_literal "$value"
      ;;
    runtime.leaseTtlMinutes|runtime.staleAfterMinutes)
      [[ "$value" =~ ^[0-9]+$ ]] || die "Invalid numeric value for $path: $value"
      ;;
    regression.immutability)
      case "$value" in
        protected-scenarios|mutable-by-spec-change) ;;
        *) die "Invalid regression.immutability: $value" ;;
      esac
      ;;
    runtime.reusePolicy)
      case "$value" in
        fingerprint-match-only|always-isolate) ;;
        *) die "Invalid runtime.reusePolicy: $value" ;;
      esac
      ;;
    grill.source|tdd.source|autoCommit.source|lockdown.source|regression.source|validation.source|runtime.source)
      validate_policy_source "$value"
      ;;
    bugfix-fastlane.tdd.mode|chaos-hardening.tdd.mode|bugfix-fastlane.tdd.source|chaos-hardening.tdd.source)
      die "Mode overrides are workflow-forced and read-only: $path"
      ;;
    *)
      die "Unknown policy path: $path"
      ;;
  esac
}

write_control_plane_setting() {
  local path="$1"
  local value="$2"

  case "$path" in
    grill.mode) CFG_GRILL_MODE="$value" ;;
    grill.source) CFG_GRILL_SOURCE="$value" ;;
    tdd.mode) CFG_TDD_MODE="$value" ;;
    tdd.source) CFG_TDD_SOURCE="$value" ;;
    autoCommit.mode) CFG_AUTOCOMMIT_MODE="$value" ;;
    autoCommit.source) CFG_AUTOCOMMIT_SOURCE="$value" ;;
    lockdown.default) CFG_LOCKDOWN_DEFAULT="$value" ;;
    lockdown.requireGrillForInvalidation) CFG_LOCKDOWN_REQUIRE_GRILL="$value" ;;
    lockdown.source) CFG_LOCKDOWN_SOURCE="$value" ;;
    regression.immutability) CFG_REGRESSION_IMMUTABILITY="$value" ;;
    regression.source) CFG_REGRESSION_SOURCE="$value" ;;
    validation.certificationRequired) CFG_VALIDATION_CERT_REQUIRED="$value" ;;
    validation.source) CFG_VALIDATION_SOURCE="$value" ;;
    runtime.leaseTtlMinutes) CFG_RUNTIME_LEASE_TTL_MINUTES="$value" ;;
    runtime.staleAfterMinutes) CFG_RUNTIME_STALE_AFTER_MINUTES="$value" ;;
    runtime.reusePolicy) CFG_RUNTIME_REUSE_POLICY="$value" ;;
    runtime.source) CFG_RUNTIME_SOURCE="$value" ;;
    metrics.enabled) CFG_METRICS_ENABLED="$value" ;;
    metrics.activityTrackingEnabled) CFG_ACTIVITY_TRACKING_ENABLED="$value" ;;
    *) die "Unknown policy path: $path" ;;
  esac
}

# Classify a spec as business or infra by reading spec.md title/first lines
classify_spec() {
  local spec_dir="$1"
  local spec_file="$spec_dir/spec.md"
  if [[ ! -f "$spec_file" ]]; then echo "unknown"; return; fi

  local header
  header="$(head -15 "$spec_file" | tr '[:upper:]' '[:lower:]')"

  # Infrastructure keywords
  if echo "$header" | grep -qE 'docker|deploy|ci.?cd|monitoring|observability|migration.?tool|platform.?setup|config.?management|devops|infra|kubernetes|helm|terraform|github.?action|pipeline|setup|tooling|framework'; then
    echo "infra"
  else
    echo "business"
  fi
}

# ── Commands ────────────────────────────────────────────────────────

cmd_help() {
  cat << 'HELPEOF'
Usage: bubbles <command> [args...]

Commands:
  status                        Show all specs with status, mode, scope counts
  specs [--range M-N] [--cat X] List/filter specs
  blocked                       Show only blocked specs with reasons
  dod <spec>                    Show unchecked DoD items for a spec
  policy <subcommand>           Manage control-plane defaults (status|get|set|reset)
  runtime <subcommand>          Manage runtime leases and coordination
  session                       Show current session state
  lint <spec>                   Run artifact lint on a spec
  agnosticity [--staged]        Check portable Bubbles surfaces for drift
  guard <spec>                  Run state transition guard on a spec
  guard-selftest                Run the transition guard selftest suite
  runtime-selftest              Run the runtime lease selftest suite
  finding-closure-selftest      Run the finding-set closure selftest suite
  workflow-selftest             Run workflow command-surface smoke checks
  scan <spec>                   Run implementation reality scan on a spec
  regression-quality [args...]  Run bailout/adversarial regression quality scan on test files or dirs
  docs-registry [mode]          Show framework-default or effective managed-doc registry
  framework-write-guard         Check downstream framework-managed files against install provenance
  interop <subcommand>          Detect, import, apply, and inspect project-owned interop packets
  framework-validate            Run framework self-validation across core guard and selftest surfaces
  release-check                 Run source-repo release hygiene checks
  framework-events [options]    Show typed framework event history
  run-state [options]           Show active and recent workflow run-state records
  repo-readiness [path] [--profile PROFILE]
                                Run advisory repo-readiness checks
  framework-proposal <slug>     Scaffold a project-owned upstream Bubbles change proposal
  audit-done [--fix]            Audit all specs marked done
  autofix <spec>                Scaffold missing report sections
  dag <spec>                    Show scope dependency graph (Mermaid)
  doctor [--heal]               Check project health, optionally auto-fix
  hooks <subcommand>            Manage git hooks (catalog|list|install|add|remove|run|status)
  project [gates <subcmd>]      Manage project extensions (bubbles-project.yaml)
  metrics <subcommand>          Manage metrics (enable|disable|activity-enable|activity-disable|status|summary|gates|agents)
  lessons [--all|compact]       View or compact lessons-learned memory
  skill-proposals [subcommand]  Show or dismiss generated skill proposals
  profile [subcommand]          Show, list, or change adoption profiles plus developer observations
  upgrade [version] [--dry-run] Upgrade Bubbles to latest or specific version
  lint-budget                   Measure instruction density in agent prompts
  sunnyvale <alias>             Resolve a Sunnyvale alias
  aliases                       List all Sunnyvale aliases
  help                          Show this help message
HELPEOF
}

cmd_status() {
  bash "$SCRIPT_DIR/spec-dashboard.sh" "$SPECS_DIR"
  if [[ -x "$SCRIPT_DIR/runtime-leases.sh" ]]; then
    bash "$SCRIPT_DIR/runtime-leases.sh" summary || true
    echo ""
  fi
}

cmd_specs() {
  local range_start="" range_end="" category="all"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --range)
        shift
        if [[ "$1" =~ ^([0-9]+)-([0-9]+)$ ]]; then
          range_start="${BASH_REMATCH[1]}"
          range_end="${BASH_REMATCH[2]}"
        else
          die "Invalid range format. Use: --range 4-49"
        fi
        ;;
      --cat|--category)
        shift
        category="$1"
        if [[ "$category" != "business" && "$category" != "infra" && "$category" != "all" ]]; then
          die "Invalid category. Use: business, infra, or all"
        fi
        ;;
      *) die "Unknown option: $1" ;;
    esac
    shift
  done

  printf "\n${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n"
  printf "${BLUE}  Bubbles Spec Inventory${NC}"
  [[ "$category" != "all" ]] && printf " ${DIM}(filter: %s)${NC}" "$category"
  [[ -n "$range_start" ]] && printf " ${DIM}(range: %03d-%03d)${NC}" "$range_start" "$range_end"
  printf "\n${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n\n"

  printf "%-6s %-38s %-14s %-10s\n" "ID" "NAME" "STATUS" "CATEGORY"
  printf "%-6s %-38s %-14s %-10s\n" "──" "────" "──────" "────────"

  local count=0
  local shown=0

  for spec_dir in $(find "$SPECS_DIR" -maxdepth 1 -mindepth 1 -type d | sort); do
    local name
    name="$(basename "$spec_dir")"

    # Extract numeric ID
    local num_str="${name%%-*}"
    if [[ ! "$num_str" =~ ^[0-9]+$ ]]; then continue; fi
    local num=$((10#$num_str))

    # Range filter
    if [[ -n "$range_start" ]]; then
      if (( num < 10#$range_start || num > 10#$range_end )); then continue; fi
    fi

    # Category filter
    local cat
    cat="$(classify_spec "$spec_dir")"
    if [[ "$category" != "all" && "$cat" != "$category" ]]; then continue; fi

    # Status
    local status="-"
    if [[ -f "$spec_dir/state.json" ]]; then
      status="$(json_field "$spec_dir/state.json" "status")"
      [[ -z "$status" ]] && status="-"
    fi

    # Color status
    local status_display
    case "$status" in
      done) status_display="${GREEN}done${NC}" ;;
      in_progress) status_display="${YELLOW}in_progress${NC}" ;;
      blocked) status_display="${RED}blocked${NC}" ;;
      *) status_display="$status" ;;
    esac

    # Color category
    local cat_display
    case "$cat" in
      business) cat_display="${CYAN}business${NC}" ;;
      infra) cat_display="${DIM}infra${NC}" ;;
      *) cat_display="$cat" ;;
    esac

    printf "%-6s %-38s %-14b %-10b\n" "$num_str" "${name#*-}" "$status_display" "$cat_display"
    ((shown++))
    ((count++))
  done

  printf "\n${DIM}Showing %d specs${NC}\n\n" "$shown"
}

cmd_blocked() {
  printf "\n${RED}${BOLD}Blocked Specs${NC}\n"
  printf "%-40s %-60s\n" "SPEC" "REASON"
  printf "%-40s %-60s\n" "────" "──────"

  local found=0

  for state_file in $(find "$SPECS_DIR" -maxdepth 2 -name "state.json" -not -path "*/bugs/*" | sort); do
    local status
    status="$(json_field "$state_file" "status")"
    if [[ "$status" != "blocked" ]]; then continue; fi

    local spec_dir spec_name
    spec_dir="$(dirname "$state_file")"
    spec_name="$(basename "$spec_dir")"

    # Try to extract block reason from failures array or blockedReason field
    local reason
    reason="$(json_field "$state_file" "blockedReason")"
    if [[ -z "$reason" ]]; then
      reason="$(grep -oE '"reason"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null | tail -1 | sed -E 's/.*"([^"]+)"$/\1/' || echo "-")"
    fi

    # Truncate reason
    if [[ ${#reason} -gt 58 ]]; then
      reason="${reason:0:55}..."
    fi

    printf "%-40s %-60s\n" "$spec_name" "$reason"
    ((found++))
  done

  if [[ "$found" -eq 0 ]]; then
    printf "${GREEN}  No blocked specs!${NC}\n"
  fi
  echo
}

cmd_dod() {
  [[ $# -lt 1 ]] && die "Usage: bubbles dod <spec>"

  local spec_dir
  spec_dir="$(resolve_spec "$1")"
  local spec_name
  spec_name="$(basename "$spec_dir")"

  printf "\n${BOLD}Unchecked DoD items for ${CYAN}%s${NC}\n\n" "$spec_name"

  local found_unchecked=0

  # Check scopes.md
  if [[ -f "$spec_dir/scopes.md" ]]; then
    local unchecked
    unchecked="$(grep -n '^\- \[ \]' "$spec_dir/scopes.md" 2>/dev/null || true)"
    if [[ -n "$unchecked" ]]; then
      printf "${DIM}From scopes.md:${NC}\n"
      echo "$unchecked" | while IFS= read -r line; do
        local linenum="${line%%:*}"
        local content="${line#*:}"
        printf "  ${YELLOW}L%-4s${NC} %s\n" "$linenum" "$content"
      done
      found_unchecked=1
    fi
  fi

  # Check per-scope directories
  if [[ -d "$spec_dir/scopes" ]]; then
    for scope_file in $(find "$spec_dir/scopes" -name "scope.md" | sort); do
      local scope_rel="${scope_file#$spec_dir/}"
      local unchecked
      unchecked="$(grep -n '^\- \[ \]' "$scope_file" 2>/dev/null || true)"
      if [[ -n "$unchecked" ]]; then
        printf "${DIM}From %s:${NC}\n" "$scope_rel"
        echo "$unchecked" | while IFS= read -r line; do
          local linenum="${line%%:*}"
          local content="${line#*:}"
          printf "  ${YELLOW}L%-4s${NC} %s\n" "$linenum" "$content"
        done
        found_unchecked=1
      fi
    done
  fi

  # Check bug scopes
  if [[ -d "$spec_dir/bugs" ]]; then
    for bug_scope in $(find "$spec_dir/bugs" -name "scopes.md" | sort); do
      local bug_rel="${bug_scope#$spec_dir/}"
      local unchecked
      unchecked="$(grep -n '^\- \[ \]' "$bug_scope" 2>/dev/null || true)"
      if [[ -n "$unchecked" ]]; then
        printf "${DIM}From %s:${NC}\n" "$bug_rel"
        echo "$unchecked" | while IFS= read -r line; do
          local linenum="${line%%:*}"
          local content="${line#*:}"
          printf "  ${YELLOW}L%-4s${NC} %s\n" "$linenum" "$content"
        done
        found_unchecked=1
      fi
    done
  fi

  if [[ "$found_unchecked" -eq 0 ]]; then
    printf "${GREEN}  All DoD items are checked!${NC}\n"
  fi

  # Summary counts
  local total checked unchecked_count
  total=0; checked=0; unchecked_count=0
  for f in $(find "$spec_dir" -name "scopes.md" -o -name "scope.md" | grep -v node_modules); do
    local t c u
    t="$(grep -c '^\- \[' "$f" 2>/dev/null || true)"
    c="$(grep -c '^\- \[x\]' "$f" 2>/dev/null || true)"
    u="$(grep -c '^\- \[ \]' "$f" 2>/dev/null || true)"
    t="${t:-0}"
    c="${c:-0}"
    u="${u:-0}"
    total=$((total + t))
    checked=$((checked + c))
    unchecked_count=$((unchecked_count + u))
  done

  printf "\n${DIM}Total: %d items | ${GREEN}%d checked${NC}${DIM} | ${YELLOW}%d unchecked${NC}\n\n" "$total" "$checked" "$unchecked_count"
}

cmd_session() {
  if [[ ! -f "$SESSION_FILE" ]]; then
    printf "${DIM}No active session found at %s${NC}\n" "$SESSION_FILE"
    return 0
  fi

  printf "\n${BLUE}${BOLD}Bubbles Session State${NC}\n"
  printf "${BLUE}───────────────────────────────────────${NC}\n"

  local session_id agent feature status phase last_updated
  session_id="$(json_field "$SESSION_FILE" "sessionId")"
  agent="$(json_field "$SESSION_FILE" "agent")"
  feature="$(json_field "$SESSION_FILE" "featureDir")"
  status="$(json_field "$SESSION_FILE" "status")"
  phase="$(json_field "$SESSION_FILE" "currentPhase")"
  last_updated="$(json_field "$SESSION_FILE" "lastUpdatedAt")"

  # Color status
  local status_display
  case "$status" in
    done) status_display="${GREEN}$status${NC}" ;;
    blocked) status_display="${RED}$status${NC}" ;;
    *) status_display="${YELLOW}$status${NC}" ;;
  esac

  printf "  %-16s %s\n" "Session:" "$session_id"
  printf "  %-16s %s\n" "Agent:" "$agent"
  printf "  %-16s %s\n" "Feature:" "$feature"
  printf "  %-16s %b\n" "Status:" "$status_display"
  printf "  %-16s %s\n" "Phase:" "$phase"
  printf "  %-16s %s\n" "Last Updated:" "$last_updated"

  # Show resume info if available
  local resume_agent resume_note
  resume_agent="$(json_field "$SESSION_FILE" "recommendedAgent")"
  resume_note="$(grep -oE '"note"[[:space:]]*:[[:space:]]*"[^"]+"' "$SESSION_FILE" 2>/dev/null | tail -1 | sed -E 's/.*"([^"]+)"$/\1/' || true)"

  if [[ -n "$resume_agent" ]]; then
    printf "\n  ${BOLD}Resume:${NC}\n"
    printf "  %-16s %s\n" "Agent:" "$resume_agent"
    [[ -n "$resume_note" ]] && printf "  %-16s %s\n" "Note:" "$resume_note"
  fi

  # Show failure count
  local failure_count
  failure_count="$(grep -c '"phase"' "$SESSION_FILE" 2>/dev/null || echo "0")"
  if [[ "$failure_count" -gt 0 ]]; then
    printf "\n  ${DIM}Recorded failures: %d${NC}\n" "$failure_count"
  fi

  echo
}

cmd_lint() {
  [[ $# -lt 1 ]] && die "Usage: bubbles lint <spec>"
  local spec_dir
  spec_dir="$(resolve_spec "$1")"
  bash "$SCRIPT_DIR/artifact-lint.sh" "$spec_dir"
}

cmd_agnosticity() {
  bash "$SCRIPT_DIR/agnosticity-lint.sh" "$@"
}

cmd_guard() {
  [[ $# -lt 1 ]] && die "Usage: bubbles guard <spec>"
  local spec_dir
  spec_dir="$(resolve_spec "$1")"
  bash "$SCRIPT_DIR/state-transition-guard.sh" "$spec_dir"
}

cmd_guard_selftest() {
  bash "$SCRIPT_DIR/state-transition-guard-selftest.sh"
}

cmd_finding_closure_selftest() {
  bash "$SCRIPT_DIR/finding-closure-selftest.sh"
}

cmd_workflow_selftest() {
  bash "$SCRIPT_DIR/workflow-surface-selftest.sh"
}

cmd_runtime_selftest() {
  bash "$SCRIPT_DIR/runtime-lease-selftest.sh"
}

cmd_scan() {
  [[ $# -lt 1 ]] && die "Usage: bubbles scan <spec>"
  local spec_dir
  spec_dir="$(resolve_spec "$1")"
  local verbose=""
  [[ "${2:-}" == "--verbose" || "${2:-}" == "-v" ]] && verbose="--verbose"
  bash "$SCRIPT_DIR/implementation-reality-scan.sh" "$spec_dir" $verbose
}

cmd_regression_quality() {
  [[ $# -lt 1 ]] && die "Usage: bubbles regression-quality [--bugfix] [--verbose] <test-file-or-dir> [...]"
  bash "$SCRIPT_DIR/regression-quality-guard.sh" "$@"
}

cmd_docs_registry() {
  local mode="effective"
  local passthrough=()

  if [[ $# -gt 0 ]]; then
    case "$1" in
      effective)
        mode="effective"
        shift
        ;;
      framework-default|default)
        mode="framework-default"
        shift
        ;;
    esac
  fi

  while [[ $# -gt 0 ]]; do
    passthrough+=("$1")
    shift
  done

  if [[ "$mode" == "framework-default" ]]; then
    bash "$SCRIPT_DIR/docs-registry-resolve.sh" --framework-default "${passthrough[@]}"
  else
    bash "$SCRIPT_DIR/docs-registry-resolve.sh" --effective "${passthrough[@]}"
  fi
}

cmd_framework_validate() {
  bash "$SCRIPT_DIR/framework-validate.sh" "$@"
}

cmd_release_check() {
  bash "$SCRIPT_DIR/release-check.sh" "$@"
}

cmd_framework_events() {
  local tail_count=20
  local type_filter=''
  local output_mode='text'
  local line shown=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --tail)
        tail_count="$2"
        shift 2
        ;;
      --type)
        type_filter="$2"
        shift 2
        ;;
      --json)
        output_mode='json'
        shift
        ;;
      *)
        die "Usage: bubbles framework-events [--tail N] [--type TYPE] [--json]"
        ;;
    esac
  done

  ensure_runtime_event_log

  if [[ "$output_mode" == 'json' ]]; then
    cat "$CONTROL_PLANE_EVENT_FILE"
    return 0
  fi

  echo "Framework events: $CONTROL_PLANE_EVENT_FILE"
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if [[ -n "$type_filter" && "$line" != *"\"type\":\"$type_filter\""* ]]; then
      continue
    fi
    printf '%s\n' "$line"
    shown=$((shown + 1))
  done < <(tail -n "$tail_count" "$CONTROL_PLANE_EVENT_FILE")

  if [[ "$shown" -eq 0 ]]; then
    echo "No matching framework events."
  fi
}

cmd_run_state() {
  local mode='summary'
  local output_mode='text'
  local active_lines recent_lines filtered_active active_count recent_count line

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --active) mode='active' ; shift ;;
      --recent) mode='recent' ; shift ;;
      --all) mode='all' ; shift ;;
      --json) output_mode='json' ; shift ;;
      *) die "Usage: bubbles run-state [--active|--recent|--all] [--json]" ;;
    esac
  done

  ensure_run_state_registry

  if [[ "$output_mode" == 'json' ]]; then
    cat "$CONTROL_PLANE_RUN_STATE_FILE"
    return 0
  fi

  active_lines="$(run_state_lines activeRuns)"
  recent_lines="$(run_state_lines recentRuns)"
  filtered_active=''
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if [[ "$(field_from_json_line "$line" 'runId')" == "${CURRENT_RUN_ID:-}" ]]; then
      continue
    fi
    if [[ -z "$filtered_active" ]]; then
      filtered_active="$line"
    else
      filtered_active="$filtered_active
$line"
    fi
  done <<< "$active_lines"

  echo "Workflow run-state: $CONTROL_PLANE_RUN_STATE_FILE"
  case "$mode" in
    active)
      if [[ -n "$filtered_active" ]]; then
        printf '%s\n' "$filtered_active"
      else
        echo "No active runs."
      fi
      ;;
    recent)
      if [[ -n "$recent_lines" ]]; then
        printf '%s\n' "$recent_lines"
      else
        echo "No recent runs."
      fi
      ;;
    all)
      echo
      echo "Active Runs"
      if [[ -n "$filtered_active" ]]; then
        printf '%s\n' "$filtered_active"
      else
        echo "No active runs."
      fi
      echo
      echo "Recent Runs"
      if [[ -n "$recent_lines" ]]; then
        printf '%s\n' "$recent_lines"
      else
        echo "No recent runs."
      fi
      ;;
    summary)
      active_count="$(printf '%s\n' "$filtered_active" | sed '/^$/d' | wc -l | tr -d ' ')"
      recent_count="$(printf '%s\n' "$recent_lines" | sed '/^$/d' | wc -l | tr -d ' ')"
      echo "Active runs: $active_count"
      echo "Recent runs: $recent_count"
      ;;
  esac
}

cmd_repo_readiness() {
  bash "$SCRIPT_DIR/repo-readiness.sh" "$@"
}

cmd_framework_write_guard() {
  bash "$SCRIPT_DIR/downstream-framework-write-guard.sh" "$@"
}

cmd_interop() {
  bash "$SCRIPT_DIR/interop-intake.sh" "$@"
}

cmd_framework_proposal() {
  require_downstream_repo_for_framework_proposal

  [[ $# -lt 1 ]] && die "Usage: bubbles framework-proposal <slug> [--title \"Title\"]"

  local slug="$1"
  shift

  local explicit_title=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --title)
        [[ $# -lt 2 ]] && die "Usage: bubbles framework-proposal <slug> [--title \"Title\"]"
        explicit_title="$2"
        shift 2
        ;;
      *)
        die "Unknown option for framework-proposal: $1"
        ;;
    esac
  done

  local normalized_slug
  normalized_slug="$(printf '%s' "$slug" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')"
  [[ -n "$normalized_slug" ]] || die "framework-proposal slug must contain letters or numbers"

  local proposal_dir="$REPO_ROOT/.github/bubbles-project/proposals"
  local date_prefix
  date_prefix="$(date -u +%Y%m%d)"
  local proposal_file="$proposal_dir/${date_prefix}-${normalized_slug}.md"
  local title="$explicit_title"

  if [[ -z "$title" ]]; then
    title="$(printf '%s' "$normalized_slug" | sed -E 's/-+/ /g; s/\b(.)/\u\1/g')"
  fi

  [[ -e "$proposal_file" ]] && die "Proposal already exists: $proposal_file"

  mkdir -p "$proposal_dir"

  cat > "$proposal_file" <<EOF
# Bubbles Framework Change Proposal

- Title: $title
- Slug: $normalized_slug
- Created: $(date -u +"%Y-%m-%d")
- Created From: $(basename "$REPO_ROOT")
- Requested Upstream Repo: bubbles

## Summary

Describe the framework change you want in one short paragraph.

## Why This Must Be Upstream

Explain why this cannot live in project-owned files such as \`.github/bubbles-project.yaml\`, \`scripts/\`, or \`specs/\`.

## Current Downstream Limitation

Describe the current pain or blocked workflow in the consumer repo.

## Proposed Bubbles Change

List the desired upstream edits, commands, scripts, docs, or generated files.

## Affected Framework Paths

- \`.github/bubbles/scripts/...\`
- \`.github/bubbles/workflows.yaml\`
- \`.github/agents/bubbles...\`
- Other:

## Expected Downstream Outcome

Describe what a consumer repo should be able to do after the upstream Bubbles change ships and the repo refreshes.

## Acceptance Criteria

- [ ] Upstream Bubbles implementation exists
- [ ] Installer or refresh flow distributes the change
- [ ] Downstream repos no longer need a local framework patch
- [ ] Docs explain the new behavior

## Notes

- Do not edit \`.github/bubbles/**\`, \`.github/agents/bubbles*\`, or other framework-managed files locally.
- Implement the framework fix in the Bubbles source repo, then refresh this repo via install/refresh.
EOF

  echo "✅ Framework proposal created: $proposal_file"
  echo "ℹ️  Next step: implement the change in the Bubbles source repo, then refresh this repo's framework layer."
}

cmd_audit_done() {
  local fix_flag=""
  [[ "${1:-}" == "--fix" ]] && fix_flag="--fix"
  bash "$SCRIPT_DIR/done-spec-audit.sh" $fix_flag
}

cmd_autofix() {
  [[ $# -lt 1 ]] && die "Usage: bubbles autofix <spec>"
  local spec_dir
  spec_dir="$(resolve_spec "$1")"
  bash "$SCRIPT_DIR/report-section-autofix.sh" "$spec_dir" --write
}

cmd_sunnyvale() {
  [[ $# -lt 1 ]] && { list_all_aliases; return 0; }
  local alias="$1"
  if ! sunnyvale_lookup "$alias"; then
    die "Unknown Sunnyvale alias: $alias\nRun 'bubbles aliases' to see all aliases."
  fi
}

cmd_aliases() {
  list_all_aliases
}

# ── New v2 commands ─────────────────────────────────────────────────

cmd_dag() {
  local spec_dir
  spec_dir=$(resolve_spec "${1:?Usage: bubbles dag <spec>}")

  echo '```mermaid'
  echo 'graph TD'

  local scope_files=()
  if [[ -f "$spec_dir/scopes.md" ]]; then
    scope_files=("$spec_dir/scopes.md")
  elif [[ -d "$spec_dir/scopes" ]]; then
    for sf in "$spec_dir"/scopes/*/scope.md; do
      [[ -f "$sf" ]] && scope_files+=("$sf")
    done
  fi

  for sf in "${scope_files[@]}"; do
    local scope_id scope_name scope_status depends_on status_icon
    scope_id=$(grep -oE '^## (Scope [0-9]+|[0-9]+-[a-z][-a-z0-9]*)' "$sf" | head -1 | sed 's/## //')
    scope_name=$(grep -E '^## ' "$sf" | head -1 | sed 's/^## //' | sed 's/ *(.*//')
    scope_status=$(grep -oE 'Status: (Done|In Progress|Not Started|Blocked)' "$sf" | head -1 | sed 's/Status: //')
    depends_on=$(grep -oE 'Depends On:.*' "$sf" | head -1 | sed 's/Depends On: *//')

    case "$scope_status" in
      Done)         status_icon="✅" ;;
      "In Progress") status_icon="🔄" ;;
      Blocked)      status_icon="🚫" ;;
      *)            status_icon="⏳" ;;
    esac

    local node_id
    node_id=$(echo "$scope_id" | tr -cd '[:alnum:]')
    echo "  ${node_id}[${scope_id} ${status_icon}]"

    if [[ -n "$depends_on" && "$depends_on" != "None" && "$depends_on" != "none" ]]; then
      for dep in $(echo "$depends_on" | tr ',' ' '); do
        local dep_id
        dep_id=$(echo "$dep" | tr -cd '[:alnum:]')
        echo "  ${dep_id} --> ${node_id}"
      done
    fi
  done

  echo '```'
}

cmd_doctor() {
  local heal=false
  for arg in "$@"; do
    [[ "$arg" == "--heal" ]] && heal=true
  done

  local passed=0 failed=0 healed=0

  echo ""
  echo -e "${BLUE}🫧 Bubbles Doctor — Project Health Check${NC}"
  echo -e "${BLUE}═══════════════════════════════════════════${NC}"
  echo ""

  echo -e "${BOLD}Framework Integrity${NC}"
  echo -e "${DIM}Installer payload, trust provenance, and managed-file integrity.${NC}"
  echo ""

  # Check 1: Core agents
  local agent_count
  agent_count=$(ls "$AGENTS_DIR/bubbles."*.agent.md 2>/dev/null | wc -l)
  if [[ "$agent_count" -ge 25 ]]; then
    echo -e "  ${GREEN}✅${NC} Core agents installed (${agent_count})"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} Core agents: expected ≥25, found ${agent_count}"
    failed=$((failed + 1))
  fi

  # Check 2: Governance scripts
  local script_count
  script_count=$(ls "$FRAMEWORK_DIR/scripts/"*.sh 2>/dev/null | wc -l)
  if [[ "$script_count" -ge 10 ]]; then
    echo -e "  ${GREEN}✅${NC} Governance scripts installed (${script_count})"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} Scripts: expected ≥10, found ${script_count}"
    failed=$((failed + 1))
  fi

  # Check 3: workflows.yaml
  if [[ -f "$FRAMEWORK_DIR/workflows.yaml" ]]; then
    echo -e "  ${GREEN}✅${NC} Workflow config present"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} workflows.yaml missing"
    failed=$((failed + 1))
  fi

  # Check 4: Control-plane bootstrap registry
  if [[ -f "$CONTROL_PLANE_CONFIG" ]]; then
    echo -e "  ${GREEN}✅${NC} Control-plane policy registry present (.specify/memory/bubbles.config.json)"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} Missing control-plane policy registry: .specify/memory/bubbles.config.json"
    echo -e "     ${DIM}Bootstrap-owned artifact. Do not create it manually.${NC}"
    echo -e "     ${DIM}Remediation: rerun install.sh with --bootstrap from the Bubbles repo.${NC}"
    echo -e "     ${DIM}Use /bubbles.setup mode: refresh for .github drift after bootstrap.${NC}"
    failed=$((failed + 1))
  fi

  # Check 5: Script permissions
  local unexec=0
  for s in "$FRAMEWORK_DIR/scripts/"*.sh; do
    [[ -f "$s" && ! -x "$s" ]] && unexec=$((unexec + 1))
  done
  if [[ "$unexec" -eq 0 ]]; then
    echo -e "  ${GREEN}✅${NC} All scripts executable"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} $unexec scripts not executable"
    if [[ "$heal" == "true" ]]; then
      chmod +x "$FRAMEWORK_DIR/scripts/"*.sh
      echo -e "  ${GREEN}🔧${NC} Fixed: chmod +x on all scripts"
      healed=$((healed + 1))
    fi
    failed=$((failed + 1))
  fi

  # Check 6: Version stamp
  if [[ -f "$FRAMEWORK_DIR/.version" ]]; then
    local ver
    ver=$(cat "$FRAMEWORK_DIR/.version")
    echo -e "  ${GREEN}✅${NC} Bubbles version: $ver"
    passed=$((passed + 1))
  elif is_framework_repo && [[ -f "$REPO_ROOT/VERSION" ]]; then
    local ver
    ver=$(cat "$REPO_ROOT/VERSION")
    echo -e "  ${GREEN}✅${NC} Bubbles source version: $ver"
    passed=$((passed + 1))
  else
    echo -e "  ${YELLOW}⚠️${NC}  No version stamp found"
  fi

  # Check 7: Portable Bubbles surfaces remain agnostic
  if bash "$SCRIPT_DIR/agnosticity-lint.sh" --quiet; then
    echo -e "  ${GREEN}✅${NC} Portable Bubbles surfaces pass agnosticity lint"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} Portable Bubbles surfaces contain project/tool drift"
    failed=$((failed + 1))
  fi

  # Check 8: Downstream framework-managed files still match install provenance
  if is_framework_repo; then
    echo -e "  ${GREEN}✅${NC} Framework write guard not applicable in the Bubbles source repo"
    passed=$((passed + 1))
  else
    local trust_output=''
    local trust_status='pass'
    if ! trust_output="$(bash "$SCRIPT_DIR/downstream-framework-write-guard.sh" 2>&1)"; then
      trust_status='fail'
    fi
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      echo "  $line"
    done <<< "$trust_output"
    if [[ "$trust_status" == 'pass' ]]; then
      passed=$((passed + 1))
    else
      failed=$((failed + 1))
    fi
  fi

  # Check 9: Workflow registry and documented command surface stay aligned
  if bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet; then
    echo -e "  ${GREEN}✅${NC} Workflow inventory and documented control-plane surfaces are consistent"
    passed=$((passed + 1))
  else
    echo -e "  ${RED}❌${NC} Workflow inventory or documented control-plane surfaces drifted"
    failed=$((failed + 1))
  fi

  # Check 10: Runtime lease registry is readable and conflict free
  if [[ -x "$SCRIPT_DIR/runtime-leases.sh" ]]; then
    local runtime_summary runtime_stale runtime_conflicts
    runtime_summary="$(bash "$SCRIPT_DIR/runtime-leases.sh" summary 2>/dev/null || true)"
    runtime_stale="$(printf '%s\n' "$runtime_summary" | sed -nE 's/.*stale=([0-9]+).*/\1/p')"
    runtime_conflicts="$(printf '%s\n' "$runtime_summary" | sed -nE 's/.*conflicts=([0-9]+).*/\1/p')"
    runtime_stale="${runtime_stale:-0}"
    runtime_conflicts="${runtime_conflicts:-0}"
    if bash "$SCRIPT_DIR/runtime-leases.sh" doctor --quiet >/dev/null 2>&1; then
      echo -e "  ${GREEN}✅${NC} Runtime lease registry readable"
      passed=$((passed + 1))
      if [[ "$runtime_stale" -gt 0 ]]; then
        echo -e "  ${YELLOW}⚠️${NC}  Runtime registry contains $runtime_stale stale lease(s)"
      fi
    else
      echo -e "  ${RED}❌${NC} Runtime lease registry has active conflicts (${runtime_conflicts})"
      failed=$((failed + 1))
    fi
  fi

  echo ""
  echo -e "${BOLD}Adoption Profile Progress${NC}"
  echo -e "${DIM}Profile guidance is advisory and separate from trust or certification.${NC}"
  echo ""

  local current_profile=''
  local current_profile_label=''
  local current_profile_description=''
  local current_profile_audience=''
  local current_profile_summary=''
  local current_profile_invariant=''
  local current_profile_readiness=''
  local adoption_profile_explicit='false'
  local advisory_count=0
  local guidance_item=''

  current_profile="$(active_adoption_profile)"
  if [[ -n "$current_profile" ]]; then
    adoption_profile_explicit='true'
  else
    current_profile='delivery'
  fi
  current_profile="$(effective_adoption_profile "$current_profile")"
  current_profile_label="$(adoption_profile_value "$current_profile" label)"
  current_profile_description="$(adoption_profile_value "$current_profile" description)"
  current_profile_audience="$(adoption_profile_value "$current_profile" intendedAudience)"
  current_profile_summary="$(adoption_profile_value "$current_profile" bootstrapSummary)"
  current_profile_invariant="$(adoption_profile_value "$current_profile" governanceInvariant)"
  current_profile_readiness="$(adoption_profile_value "$current_profile" doctorProjectReadiness)"

  if [[ "$adoption_profile_explicit" == 'true' ]]; then
    echo -e "  ${GREEN}✅${NC} Active adoption profile: ${current_profile_label} (${current_profile})"
  else
    echo -e "  ${CYAN}ℹ️${NC}  Active adoption profile: ${current_profile_label} (${current_profile}, installer default)"
  fi
  [[ -n "$current_profile_description" ]] && echo "     ${current_profile_description}"
  [[ -n "$current_profile_audience" ]] && echo "     Intended audience: ${current_profile_audience}"
  [[ -n "$current_profile_summary" ]] && echo "     ${current_profile_summary}"
  [[ -n "$current_profile_invariant" ]] && echo "     Governance invariant: ${current_profile_invariant}"
  if [[ "$current_profile" == 'foundation' ]]; then
    echo "     Foundation narrows first-run guidance only; certification still belongs to bubbles.validate."
  elif [[ "$current_profile" == 'assured' ]]; then
    echo "     Assured raises early guardrail visibility only; it does not change certification authority or scenario contracts."
  fi
  echo "     Required docs:"
  while IFS= read -r guidance_item; do
    [[ -n "$guidance_item" ]] || continue
    echo "       - $guidance_item"
  done < <(adoption_profile_list "$current_profile" requiredDocs)
  echo "     Recommended next commands:"
  while IFS= read -r guidance_item; do
    [[ -n "$guidance_item" ]] || continue
    echo "       - $guidance_item"
  done < <(adoption_profile_list "$current_profile" recommendedNextCommands)

  echo ""
  echo -e "${BOLD}Project Readiness Advisory${NC}"
  echo -e "${DIM}Project-owned setup, customization, and readiness guidance.${NC}"
  echo ""

  # Check 11: Project config files
  if is_framework_repo; then
    echo -e "  ${GREEN}✅${NC} Project bootstrap config checks not required in the Bubbles source repo"
    passed=$((passed + 1))
  else
    local config_ok=true
    for cfg in .github/copilot-instructions.md .specify/memory/constitution.md .specify/memory/agents.md; do
      if [[ ! -f "$REPO_ROOT/$cfg" ]]; then
        if [[ "$current_profile_readiness" == 'advisory' ]]; then
          echo -e "  ${YELLOW}⚠️${NC}  Missing: $cfg (foundation keeps this advisory during first-run onboarding)"
          advisory_count=$((advisory_count + 1))
        else
          echo -e "  ${RED}❌${NC} Missing: $cfg"
          config_ok=false
          failed=$((failed + 1))
        fi
      fi
    done
    if [[ "$config_ok" == "true" ]]; then
      echo -e "  ${GREEN}✅${NC} Project config files exist"
      passed=$((passed + 1))
    fi
  fi

  # Check 12: TODO markers
  local todo_count=0
  for cfg in .github/copilot-instructions.md .specify/memory/agents.md; do
    if [[ -f "$REPO_ROOT/$cfg" ]]; then
      local c
      c=$(bootstrap_placeholder_count "$REPO_ROOT/$cfg")
      c=${c:-0}
      todo_count=$((todo_count + c))
    fi
  done
  if [[ "$todo_count" -eq 0 ]]; then
    echo -e "  ${GREEN}✅${NC} No unfilled TODO markers"
    passed=$((passed + 1))
  else
    echo -e "  ${YELLOW}⚠️${NC}  $todo_count unfilled TODO items in project config"
  fi

  # Check 13: Specs directory
  if is_framework_repo; then
    echo -e "  ${GREEN}✅${NC} specs/ directory check not required in the Bubbles source repo"
    passed=$((passed + 1))
  elif [[ -d "$SPECS_DIR" ]]; then
    echo -e "  ${GREEN}✅${NC} specs/ directory exists"
    passed=$((passed + 1))
  else
    if [[ "$current_profile_readiness" == 'advisory' ]]; then
      echo -e "  ${YELLOW}⚠️${NC}  specs/ directory missing (foundation keeps delivery packet work advisory during first-run onboarding)"
      advisory_count=$((advisory_count + 1))
    else
      echo -e "  ${RED}❌${NC} specs/ directory missing"
      failed=$((failed + 1))
    fi
    if [[ "$heal" == "true" ]]; then
      mkdir -p "$SPECS_DIR"
      echo -e "  ${GREEN}🔧${NC} Created specs/"
      healed=$((healed + 1))
    fi
  fi

  # Check 14: Custom gate scripts
  local project_config
  local proj_root
  proj_root="$(project_root)"
  project_config="$proj_root/.github/bubbles-project.yaml"
  if is_framework_repo; then
    echo -e "  ${GREEN}✅${NC} Project-owned custom gate scan not required in the Bubbles source repo"
    passed=$((passed + 1))
  elif [[ -f "$project_config" ]]; then
    local gate_ok=true
    local active_gate_count=0
    while IFS= read -r line; do
      local spath
      spath=$(echo "$line" | sed 's/.*script:\s*//' | tr -d '[:space:]')
      [[ -z "$spath" ]] && continue
      active_gate_count=$((active_gate_count + 1))
      if [[ ! -x "$REPO_ROOT/.github/$spath" ]]; then
        if [[ "$current_profile_readiness" == 'advisory' ]]; then
          echo -e "  ${YELLOW}⚠️${NC}  Custom gate script missing/not executable: .github/$spath (foundation keeps custom gates advisory during first-run onboarding)"
          advisory_count=$((advisory_count + 1))
        else
          echo -e "  ${RED}❌${NC} Custom gate script missing/not executable: .github/$spath"
          gate_ok=false
          failed=$((failed + 1))
        fi
      fi
    done < <(grep -E '^[[:space:]]*script:' "$project_config")
    if [[ "$active_gate_count" -eq 0 ]]; then
      echo -e "  ${GREEN}✅${NC} No custom gate scripts defined"
      passed=$((passed + 1))
    elif [[ "$gate_ok" == "true" ]]; then
      echo -e "  ${GREEN}✅${NC} Custom gate scripts present"
      passed=$((passed + 1))
    fi
  else
    echo -e "  ${GREEN}✅${NC} No custom gate scripts defined"
    passed=$((passed + 1))
  fi

  # Check 15: Project scan config auto-generation
  if is_framework_repo; then
    echo -e "  ${GREEN}✅${NC} Project scan config auto-generation not required in the Bubbles source repo"
    passed=$((passed + 1))
  elif [[ ! -f "$project_config" ]] || ! grep -q '^scans:' "$project_config" 2>/dev/null; then
    local setup_script="$SCRIPT_DIR/project-scan-setup.sh"
    if [[ -f "$setup_script" ]]; then
      echo -e "  ${YELLOW}🔧${NC} Auto-generating project scan config..."
      (cd "$proj_root" && bash "$setup_script" --quiet 2>/dev/null) || true
      if [[ -f "$project_config" ]] && grep -q '^scans:' "$project_config" 2>/dev/null; then
        echo -e "  ${GREEN}✅${NC} Project scan config auto-generated (.github/bubbles-project.yaml)"
        passed=$((passed + 1))
        healed=$((healed + 1))
      else
        echo -e "  ${YELLOW}⚠️${NC}  Could not auto-generate scan config (will use generic defaults)"
        passed=$((passed + 1))
      fi
    fi
  else
    echo -e "  ${GREEN}✅${NC} Project scan config present (.github/bubbles-project.yaml)"
    passed=$((passed + 1))
  fi

  echo ""
  echo -e "${BOLD}Result: $passed passed, $failed failed, $advisory_count advisory"
  if [[ "$healed" -gt 0 ]]; then
    echo -e "  🔧 Auto-healed $healed issue(s)${NC}"
  fi
  echo ""

  [[ "$failed" -eq 0 ]]
}

cmd_hooks() {
  local subcmd="${1:-status}"
  shift 2>/dev/null || true

  local hooks_json="$FRAMEWORK_DIR/hooks.json"
  local git_hooks_dir
  git_hooks_dir="$(cd "$REPO_ROOT" && git rev-parse --git-dir 2>/dev/null)/hooks"

  case "$subcmd" in
    catalog)
      echo "Built-in hooks available:"
      echo "  artifact-lint       pre-commit   Fast artifact lint on staged spec files"
      echo "  agnosticity-lint    pre-commit   Portable Bubbles drift check on staged files"
      echo "  guard-done-specs    pre-push     State transition guard on done specs"
      echo "  agnosticity-full    pre-push     Full portable Bubbles drift check"
      echo "  reality-scan        pre-push     Implementation reality scan on changed specs"
      ;;

    list)
      if [[ -f "$hooks_json" ]]; then
        echo "Installed hooks (from hooks.json):"
        cat "$hooks_json"
      else
        echo "No hooks installed. Run: bubbles hooks install --all"
      fi
      ;;

    install)
      require_framework_repo_for_hooks
      mkdir -p "$(dirname "$hooks_json")"
      local hook_name="${1:-}"

      if [[ "$hook_name" == "--all" || -z "$hook_name" ]]; then
        # Install all built-in hooks
        cat > "$hooks_json" << 'HJEOF'
{
  "pre-commit": [
    {"name": "artifact-lint", "type": "builtin"},
    {"name": "agnosticity-lint", "type": "builtin"}
  ],
  "pre-push": [
    {"name": "agnosticity-full", "type": "builtin"},
    {"name": "guard-done-specs", "type": "builtin"},
    {"name": "reality-scan", "type": "builtin"}
  ]
}
HJEOF
        _regenerate_hooks "$hooks_json" "$git_hooks_dir"
        echo "✅ All built-in hooks installed"
      else
        echo "Installing single hook: $hook_name (add to hooks.json manually for now)"
      fi
      ;;

    uninstall)
      require_framework_repo_for_hooks
      local removed_any="false"
      for ht in pre-commit pre-push; do
        if [[ -x "$git_hooks_dir/$ht" ]] && grep -q 'Generated by: bubbles.sh hooks install' "$git_hooks_dir/$ht" 2>/dev/null; then
          rm -f "$git_hooks_dir/$ht"
          echo "Removed Bubbles-managed $ht hook"
          removed_any="true"
        fi
      done
      if [[ -f "$hooks_json" ]]; then
        rm -f "$hooks_json"
        echo "Removed $hooks_json"
        removed_any="true"
      fi
      if [[ "$removed_any" == "false" ]]; then
        echo "No Bubbles-managed hooks found to uninstall."
      fi
      ;;

    add)
      local hook_type="${1:?Usage: bubbles hooks add <pre-commit|pre-push> <script> --name <name>}"
      local script="${2:?Missing script path}"
      local name=""
      shift 2
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --name) name="$2"; shift 2 ;;
          *) shift ;;
        esac
      done
      [[ -z "$name" ]] && name=$(basename "$script" .sh)
      echo "Added custom hook: $name ($hook_type → $script)"
      echo "Note: Manually add to $hooks_json and run: bubbles hooks install"
      ;;

    remove)
      echo "Remove hook: $1 (manually edit $hooks_json and run: bubbles hooks install)"
      ;;

    run)
      local hook_type="${1:-pre-push}"
      if [[ -x "$git_hooks_dir/$hook_type" ]]; then
        echo "Running $hook_type hook..."
        bash "$git_hooks_dir/$hook_type"
      else
        echo "No $hook_type hook installed."
      fi
      ;;

    status)
      echo "Git hooks directory: $git_hooks_dir"
      for ht in pre-commit pre-push; do
        if [[ -x "$git_hooks_dir/$ht" ]] && grep -q 'Generated by.*bubbles' "$git_hooks_dir/$ht" 2>/dev/null; then
          echo "  $ht: ✅ installed (Bubbles-managed)"
        elif [[ -x "$git_hooks_dir/$ht" ]]; then
          echo "  $ht: ⚠️  installed (NOT Bubbles-managed)"
        else
          echo "  $ht: ❌ not installed"
        fi
      done
      ;;

    *) die "Unknown hooks subcommand: $subcmd. Try: catalog, list, install, uninstall, add, remove, run, status" ;;
  esac
}

_regenerate_hooks() {
  local hooks_json="$1" git_hooks_dir="$2"
  mkdir -p "$git_hooks_dir"

  # Generate pre-commit hook
  cat > "$git_hooks_dir/pre-commit" << 'PCHOOK'
#!/usr/bin/env bash
# Generated by: bubbles.sh hooks install
set -uo pipefail
failed=0
echo "🫧 Bubbles pre-commit: checking portable surfaces for drift..."
bash bubbles/scripts/agnosticity-lint.sh --staged || failed=1
if git diff --cached --name-only | grep -q '^specs/'; then
  echo "🫧 Bubbles pre-commit: artifact lint on staged specs..."
  for spec_dir in $(git diff --cached --name-only | grep '^specs/' | sed 's|/[^/]*$||' | sort -u); do
    if [[ -d "$spec_dir" && -f "$spec_dir/state.json" ]]; then
      bash bubbles/scripts/artifact-lint.sh "$spec_dir" --quick || failed=1
      status=$(grep -o '"status"[[:space:]]*:[[:space:]]*"[^"]*"' "$spec_dir/state.json" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')
      if [[ "$status" == "done" ]]; then
        bash bubbles/scripts/state-transition-guard.sh "$spec_dir" || failed=1
      fi
    fi
  done
fi
exit $failed
PCHOOK
  chmod +x "$git_hooks_dir/pre-commit"

  # Generate pre-push hook
  cat > "$git_hooks_dir/pre-push" << 'PPHOOK'
#!/usr/bin/env bash
# Generated by: bubbles.sh hooks install
set -uo pipefail
cat >/dev/null || true
echo "🫧 Bubbles pre-push: validating done specs..."
failed=0
bash bubbles/scripts/agnosticity-lint.sh || failed=1
for state_file in $(find specs -maxdepth 2 -name "state.json" -not -path "*/bugs/*" 2>/dev/null); do
  status=$(grep -o '"status"[[:space:]]*:[[:space:]]*"[^"]*"' "$state_file" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')
  if [[ "$status" == "done" ]]; then
    spec_dir=$(dirname "$state_file")
    bash bubbles/scripts/state-transition-guard.sh "$spec_dir" || failed=1
  fi
done
[[ $failed -ne 0 ]] && { echo "❌ Pre-push blocked."; exit 1; }
echo "✅ All done specs validated."
PPHOOK
  chmod +x "$git_hooks_dir/pre-push"
}

cmd_project() {
  local subcmd="${1:-}"
  shift 2>/dev/null || true
  local project_yaml="$REPO_ROOT/.github/bubbles-project.yaml"

  case "$subcmd" in
    ""|gates)
      local gates_subcmd="${1:-list}"
      shift 2>/dev/null || true

      case "$gates_subcmd" in
        list|"")
          if [[ -f "$project_yaml" ]]; then
            echo "Project-defined gates (from .github/bubbles-project.yaml):"
            grep -E '^\s+\S+:$|script:|blocking:|description:' "$project_yaml" 2>/dev/null || echo "  (none)"
          else
            echo "No project gates defined. Create .github/bubbles-project.yaml to add custom gates."
          fi
          ;;
        add)
          local name="${1:?Usage: bubbles project gates add <name> --script <path> [--blocking] [--description <desc>]}"
          shift
          local script="" blocking="false" description=""
          while [[ $# -gt 0 ]]; do
            case "$1" in
              --script) script="$2"; shift 2 ;;
              --blocking) blocking="true"; shift ;;
              --description) description="$2"; shift 2 ;;
              *) shift ;;
            esac
          done
          [[ -z "$script" ]] && die "Missing --script argument"

          if [[ ! -f "$project_yaml" ]]; then
            echo "gates:" > "$project_yaml"
          fi

          cat >> "$project_yaml" << GATEEOF
  $name:
    script: $script
    blocking: $blocking
    description: $description
GATEEOF
          echo "✅ Added gate: $name → $script (blocking=$blocking)"
          ;;
        remove)
          echo "Remove gate: $1 (manually edit $project_yaml for now)"
          ;;
        test)
          local name="${1:?Usage: bubbles project gates test <name>}"
          local spath
          spath=$(grep -A2 "^  ${name}:" "$project_yaml" | grep 'script:' | sed 's/.*script:\s*//' | tr -d '[:space:]')
          if [[ -n "$spath" && -x "$REPO_ROOT/.github/$spath" ]]; then
            echo "Testing gate: $name ($spath)..."
            bash "$REPO_ROOT/.github/$spath" && echo "✅ PASSED" || echo "❌ FAILED"
          else
            die "Gate script not found or not executable: $spath"
          fi
          ;;
        *) die "Unknown gates subcommand: $gates_subcmd" ;;
      esac
      ;;
    setup)
      # Analyze project and generate/update bubbles-project.yaml scans section
      local setup_script="$SCRIPT_DIR/project-scan-setup.sh"
      local proj_root
      proj_root="$(cd "$SCRIPT_DIR/../../.." && pwd)"
      if [[ -f "$setup_script" ]]; then
        (cd "$proj_root" && bash "$setup_script" "$@")
      else
        die "Setup script not found: $setup_script"
      fi
      ;;
    *) die "Unknown project subcommand: $subcmd. Try: gates, setup" ;;
  esac
}

cmd_runtime() {
  bash "$SCRIPT_DIR/runtime-leases.sh" "$@"
}

cmd_policy() {
  local subcmd="${1:-status}"
  shift || true
  local adoption_profile_label=''
  local adoption_profile_summary=''
  local adoption_profile_invariant=''

  load_control_plane_config
  adoption_profile_label="$(adoption_profile_value "$CFG_ADOPTION_PROFILE" label)"
  adoption_profile_summary="$(adoption_profile_value "$CFG_ADOPTION_PROFILE" bootstrapSummary)"
  adoption_profile_invariant="$(adoption_profile_value "$CFG_ADOPTION_PROFILE" governanceInvariant)"

  case "$subcmd" in
    status)
      echo "Control-plane policy registry: $CONTROL_PLANE_CONFIG"
      echo ""
      if [[ -n "$adoption_profile_label" ]]; then
        echo "Adoption profile: $adoption_profile_label ($CFG_ADOPTION_PROFILE)"
      else
        echo "Adoption profile: $CFG_ADOPTION_PROFILE"
      fi
      [[ -n "$adoption_profile_summary" ]] && echo "  summary = $adoption_profile_summary"
      [[ -n "$adoption_profile_invariant" ]] && echo "  governanceInvariant = $adoption_profile_invariant"
      echo ""
      echo "Defaults:"
      echo "  grill.mode = $CFG_GRILL_MODE (source=$CFG_GRILL_SOURCE)"
      echo "  tdd.mode = $CFG_TDD_MODE (source=$CFG_TDD_SOURCE)"
      echo "  autoCommit.mode = $CFG_AUTOCOMMIT_MODE (source=$CFG_AUTOCOMMIT_SOURCE)"
      echo "  lockdown.default = $CFG_LOCKDOWN_DEFAULT (source=$CFG_LOCKDOWN_SOURCE)"
      echo "  lockdown.requireGrillForInvalidation = $CFG_LOCKDOWN_REQUIRE_GRILL"
      echo "  regression.immutability = $CFG_REGRESSION_IMMUTABILITY (source=$CFG_REGRESSION_SOURCE)"
      echo "  validation.certificationRequired = $CFG_VALIDATION_CERT_REQUIRED (source=$CFG_VALIDATION_SOURCE)"
      echo "  runtime.leaseTtlMinutes = $CFG_RUNTIME_LEASE_TTL_MINUTES (source=$CFG_RUNTIME_SOURCE)"
      echo "  runtime.staleAfterMinutes = $CFG_RUNTIME_STALE_AFTER_MINUTES"
      echo "  runtime.reusePolicy = $CFG_RUNTIME_REUSE_POLICY"
      echo ""
      echo "Workflow-forced overrides:"
      echo "  bugfix-fastlane.tdd.mode = scenario-first (source=workflow-forced)"
      echo "  chaos-hardening.tdd.mode = scenario-first (source=workflow-forced)"
      echo ""
      echo "Auxiliary settings:"
      echo "  metrics.enabled = $CFG_METRICS_ENABLED"
      echo "  metrics.activityTrackingEnabled = $CFG_ACTIVITY_TRACKING_ENABLED"
      ;;
    get)
      local path="${1:-}"
      [[ -n "$path" ]] || die "Usage: bubbles policy get <path>"
      if ! read_control_plane_setting "$path"; then
        die "Unknown policy path: $path"
      fi
      echo ""
      ;;
    set)
      local path="${1:-}"
      local value="${2:-}"
      local source="repo-default"

      [[ -n "$path" && -n "$value" ]] || die "Usage: bubbles policy set <path> <value> [--source <provenance>]"
      shift 2
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --source)
            shift
            [[ $# -gt 0 ]] || die "Missing value after --source"
            source="$1"
            ;;
          *) die "Unknown option for bubbles policy set: $1" ;;
        esac
        shift
      done

      validate_policy_value "$path" "$value"
      validate_policy_source "$source"

      write_control_plane_setting "$path" "$value"
      case "$path" in
        grill.mode) CFG_GRILL_SOURCE="$source" ;;
        tdd.mode) CFG_TDD_SOURCE="$source" ;;
        autoCommit.mode) CFG_AUTOCOMMIT_SOURCE="$source" ;;
        lockdown.default|lockdown.requireGrillForInvalidation) CFG_LOCKDOWN_SOURCE="$source" ;;
        regression.immutability) CFG_REGRESSION_SOURCE="$source" ;;
        validation.certificationRequired) CFG_VALIDATION_SOURCE="$source" ;;
        runtime.leaseTtlMinutes|runtime.staleAfterMinutes|runtime.reusePolicy) CFG_RUNTIME_SOURCE="$source" ;;
      esac
      save_control_plane_config
      echo "✅ Updated $path = $value"
      ;;
    reset)
      local path="${1:-}"

      if [[ -z "$path" ]]; then
        ensure_control_plane_config
        default_control_plane_config > "$CONTROL_PLANE_CONFIG"
        echo "✅ Reset control-plane policy registry to defaults"
        return 0
      fi

      default_value="$(default_value_for_policy_path "$path" || true)"
      [[ -n "$default_value" ]] || die "Unknown policy path: $path"

      validate_policy_value "$path" "$default_value"
      write_control_plane_setting "$path" "$default_value"
      case "$path" in
        grill.mode|grill.source) CFG_GRILL_SOURCE="repo-default" ;;
        tdd.mode|tdd.source) CFG_TDD_SOURCE="repo-default" ;;
        autoCommit.mode|autoCommit.source) CFG_AUTOCOMMIT_SOURCE="repo-default" ;;
        lockdown.default|lockdown.requireGrillForInvalidation|lockdown.source) CFG_LOCKDOWN_SOURCE="repo-default" ;;
        regression.immutability|regression.source) CFG_REGRESSION_SOURCE="repo-default" ;;
        validation.certificationRequired|validation.source) CFG_VALIDATION_SOURCE="repo-default" ;;
        runtime.leaseTtlMinutes|runtime.staleAfterMinutes|runtime.reusePolicy|runtime.source) CFG_RUNTIME_SOURCE="repo-default" ;;
      esac
      save_control_plane_config
      echo "✅ Reset $path to $(read_control_plane_setting "$path")"
      ;;
    *)
      die "Unknown policy subcommand: $subcmd. Try: status, get, set, reset"
      ;;
  esac
}

cmd_metrics() {
  local subcmd="${1:-status}"
  load_control_plane_config

  case "$subcmd" in
    enable)
      CFG_METRICS_ENABLED="true"
      save_control_plane_config
      echo "✅ Metrics enabled. Events will be logged to $CONTROL_PLANE_METRICS_FILE"
      ;;
    disable)
      CFG_METRICS_ENABLED="false"
      CFG_ACTIVITY_TRACKING_ENABLED="false"
      save_control_plane_config
      echo "✅ Metrics disabled. Existing data preserved."
      ;;
    activity-enable)
      CFG_METRICS_ENABLED="true"
      CFG_ACTIVITY_TRACKING_ENABLED="true"
      save_control_plane_config
      echo "✅ Activity tracking enabled. Metrics events will be logged to $CONTROL_PLANE_METRICS_FILE and activity snapshots to $CONTROL_PLANE_ACTIVITY_FILE"
      ;;
    activity-disable)
      CFG_ACTIVITY_TRACKING_ENABLED="false"
      save_control_plane_config
      echo "✅ Activity tracking disabled. Existing activity data preserved."
      ;;
    status)
      if [[ "$CFG_METRICS_ENABLED" == "true" ]]; then
        local count=0 activity_count=0
        [[ -f "$CONTROL_PLANE_METRICS_FILE" ]] && count=$(wc -l < "$CONTROL_PLANE_METRICS_FILE")
        [[ -f "$CONTROL_PLANE_ACTIVITY_FILE" ]] && activity_count=$(wc -l < "$CONTROL_PLANE_ACTIVITY_FILE")
        echo "Metrics: ENABLED ($count events collected)"
        if [[ "$CFG_ACTIVITY_TRACKING_ENABLED" == "true" ]]; then
          echo "Activity tracking: ENABLED ($activity_count records collected)"
        else
          echo "Activity tracking: DISABLED (run: bubbles metrics activity-enable)"
        fi
      else
        echo "Metrics: DISABLED (run: bubbles metrics enable)"
      fi
      ;;
    summary)
      if [[ ! -f "$CONTROL_PLANE_METRICS_FILE" ]]; then
        echo "No metrics data. Enable with: bubbles metrics enable"
        return
      fi
      echo "Metrics Summary:"
      echo "  Total events: $(wc -l < "$CONTROL_PLANE_METRICS_FILE")"
      local gate_checks phase_completions
      gate_checks="$(grep -c '"type":"gate_check"' "$CONTROL_PLANE_METRICS_FILE" 2>/dev/null || true)"
      phase_completions="$(grep -c '"type":"phase_complete"' "$CONTROL_PLANE_METRICS_FILE" 2>/dev/null || true)"
      gate_checks="${gate_checks:-0}"
      phase_completions="${phase_completions:-0}"
      echo "  Gate checks: $gate_checks"
      echo "  Phase completions: $phase_completions"
      if [[ -f "$CONTROL_PLANE_ACTIVITY_FILE" ]]; then
        local activity_total command_invocations avg_duration
        activity_total="$(wc -l < "$CONTROL_PLANE_ACTIVITY_FILE")"
        command_invocations="$(grep -c '"command":' "$CONTROL_PLANE_ACTIVITY_FILE" 2>/dev/null || true)"
        avg_duration="$(awk -F'"durationMs":' 'NF > 1 { split($2, rest, ","); sum += rest[1]; count += 1 } END { if (count == 0) { print 0 } else { printf "%d", sum / count } }' "$CONTROL_PLANE_ACTIVITY_FILE")"
        echo "  Activity records: $activity_total"
        echo "  Command invocations tracked: ${command_invocations:-0}"
        echo "  Avg command duration: ${avg_duration}ms"
      fi
      ;;
    gates)
      if [[ ! -f "$CONTROL_PLANE_METRICS_FILE" ]]; then echo "No data."; return; fi
      echo "Gate failure frequency:"
      grep '"type":"gate_check".*"result":"fail"' "$CONTROL_PLANE_METRICS_FILE" 2>/dev/null \
        | grep -oE '"gate":"[^"]*"' | sort | uniq -c | sort -rn || echo "  No failures recorded"
      ;;
    agents)
      if [[ ! -f "$CONTROL_PLANE_METRICS_FILE" ]]; then echo "No data."; return; fi
      echo "Agent invocations:"
      grep '"type":"phase_complete"' "$CONTROL_PLANE_METRICS_FILE" 2>/dev/null \
        | grep -oE '"agent":"[^"]*"' | sort | uniq -c | sort -rn || echo "  No invocations recorded"
      ;;
    *) die "Unknown metrics subcommand: $subcmd. Try: enable, disable, activity-enable, activity-disable, status, summary, gates, agents" ;;
  esac
}

cmd_lessons() {
  local lessons_file="$REPO_ROOT/.specify/memory/lessons.md"
  local subcmd="${1:-}"

  case "$subcmd" in
    compact)
      if [[ ! -f "$lessons_file" ]]; then
        echo "No lessons file found."
        return
      fi
      local line_count
      line_count=$(wc -l < "$lessons_file")
      echo "Compacting lessons.md ($line_count lines)..."
      # Simple compaction: keep last 150 lines
      if [[ "$line_count" -gt 150 ]]; then
        local archive_file="$REPO_ROOT/.specify/memory/lessons-archive.md"
        local cut_at=$((line_count - 150))
        head -n "$cut_at" "$lessons_file" >> "$archive_file"
        tail -n 150 "$lessons_file" > "$lessons_file.tmp"
        mv "$lessons_file.tmp" "$lessons_file"
        echo "✅ Archived $cut_at lines to lessons-archive.md. Kept 150 lines."
      else
        echo "✅ File is under 150 lines, no compaction needed."
      fi
      ;;
    --all)
      if [[ -f "$lessons_file" ]]; then
        cat "$lessons_file"
      else
        echo "No lessons recorded yet."
      fi
      ;;
    "")
      if [[ -f "$lessons_file" ]]; then
        tail -50 "$lessons_file"
      else
        echo "No lessons recorded yet."
      fi
      ;;
    *) die "Unknown lessons subcommand: $subcmd. Try: compact, --all, or no argument for recent" ;;
  esac
}

cmd_skill_proposals() {
  bash "$SCRIPT_DIR/skill-evolution.sh" "${1:-show}"
}

cmd_profile() {
  if [[ $# -eq 0 ]]; then
    bash "$SCRIPT_DIR/developer-profile.sh" show
  else
    bash "$SCRIPT_DIR/developer-profile.sh" "$@"
  fi
}

cmd_lint_budget() {
  local agents_dir
  agents_dir="$(cd "$SCRIPT_DIR/../.." && pwd)/agents"
  if [[ ! -d "$agents_dir" ]]; then
    agents_dir="$(find_repo_root)/.github/agents"
  fi
  bash "$SCRIPT_DIR/instruction-budget-lint.sh" "$agents_dir" "$@"
}

cmd_upgrade() {
  local target_version="main"
  local dry_run=false
  local local_source=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      --local-source)
        local_source="$2"
        shift 2
        ;;
      --help|-h)
        echo "Usage: bubbles upgrade [version] [--dry-run] [--local-source DIR]"
        return 0
        ;;
      --*)
        die "Unknown upgrade option: $1"
        ;;
      *)
        if [[ "$target_version" != "main" ]]; then
          die "Upgrade accepts at most one target version. Got: $target_version and $1"
        fi
        target_version="$1"
        shift
        ;;
    esac
  done

  local repo="pkirsanov/bubbles"
  echo "🫧 Upgrading Bubbles to ${target_version}..."

  if [[ "$dry_run" == "true" ]]; then
    local target_manifest=''
    local cleanup_target_manifest='false'
    local target_mode='remote-ref'
    local target_ref="$target_version"
    local target_sha=''
    local target_dirty='false'
    local target_version_from_manifest=''
    local target_sha_from_manifest=''
    local target_profiles=''
    local target_interop=''
    local target_managed_count=''
    local current_manifest="$FRAMEWORK_DIR/release-manifest.json"
    local current_provenance="$FRAMEWORK_DIR/.install-source.json"
    local current_version=''
    local current_ref=''
    local current_sha=''
    local current_mode=''
    local current_dirty=''
    local current_version_from_provenance=''
    local current_sha_from_provenance=''
    local current_profiles=''
    local current_interop=''
    local current_managed_count=''
    local write_guard_clean='unknown'
    local project_owned_untouched=''

    if [[ -n "$local_source" ]]; then
      [[ -d "$local_source/bubbles" ]] || die "Local source is missing bubbles/: $local_source"
      target_mode='local-source'
      target_ref="$(bubbles_local_source_ref "$local_source")"
      target_sha="$(bubbles_local_source_sha "$local_source")"
      target_dirty="$(bubbles_local_source_dirty "$local_source")"
      target_manifest="$(mktemp)"
      cleanup_target_manifest='true'
      bash "$local_source/bubbles/scripts/generate-release-manifest.sh" --repo-root "$local_source" --output "$target_manifest"
    elif [[ -n "${BUBBLES_SOURCE_OVERRIDE_DIR:-}" ]]; then
      target_manifest="$BUBBLES_SOURCE_OVERRIDE_DIR/bubbles/release-manifest.json"
      [[ -f "$target_manifest" ]] || die "Source override is missing bubbles/release-manifest.json: $BUBBLES_SOURCE_OVERRIDE_DIR"
      target_sha="$(bubbles_json_string_field "$target_manifest" gitSha)"
    else
      target_manifest="$(mktemp)"
      cleanup_target_manifest='true'
      curl -fsSL "https://raw.githubusercontent.com/${repo}/${target_version}/bubbles/release-manifest.json" -o "$target_manifest" \
        || die "Could not resolve target release manifest for ${target_version}"
      target_sha="$(bubbles_json_string_field "$target_manifest" gitSha)"
    fi

    trap 'if [[ "${cleanup_target_manifest:-false}" == "true" && -n "${target_manifest:-}" ]]; then rm -f "$target_manifest"; fi' RETURN

    bubbles_validate_release_manifest_schema "$target_manifest" 'bubbles upgrade --dry-run target manifest' || return 1

    bubbles_read_release_manifest_summary "$target_manifest" \
      target_version_from_manifest \
      target_sha_from_manifest \
      target_profiles \
      target_interop \
      target_managed_count

    target_sha="${target_sha:-$target_sha_from_manifest}"

    if [[ -f "$current_manifest" ]]; then
      bubbles_validate_release_manifest_schema "$current_manifest" 'bubbles upgrade --dry-run current manifest' || return 1
      bubbles_read_release_manifest_summary "$current_manifest" \
        current_version \
        current_sha \
        current_profiles \
        current_interop \
        current_managed_count
    fi

    if [[ -f "$current_provenance" ]]; then
      bubbles_read_install_provenance_summary "$current_provenance" \
        current_version_from_provenance \
        current_mode \
        current_ref \
        current_sha_from_provenance \
        current_dirty
      current_version="${current_version:-$current_version_from_provenance}"
      current_sha="${current_sha:-$current_sha_from_provenance}"
    fi

    bubbles_fill_unknown_if_empty \
      current_profiles \
      current_interop \
      current_version \
      current_ref \
      current_sha \
      current_mode \
      current_dirty \
      current_managed_count

    if is_framework_repo; then
      write_guard_clean='source-repo'
    elif bash "$SCRIPT_DIR/downstream-framework-write-guard.sh" --quiet >/dev/null 2>&1; then
      write_guard_clean='true'
    else
      write_guard_clean='false'
    fi

    project_owned_untouched="$(bubbles_join_list_items '; ' \
      '.github/copilot-instructions.md' \
      '.specify/memory/constitution.md' \
      '.specify/memory/agents.md' \
      '.github/bubbles-project.yaml' \
      '.github/bubbles/hooks.json')"

    echo "Upgrade trust preview"
    echo "---------------------"
    echo "Current installed trust"
    echo "  Version: ${current_version}"
    echo "  Source ref: ${current_ref}"
    echo "  Source git SHA: ${current_sha}"
    echo "  Install mode: ${current_mode}"
    echo "  Dirty source: ${current_dirty}"
    echo "  Supported profiles: ${current_profiles}"
    echo "  Supported interop sources: ${current_interop}"
    echo "  Managed files in current manifest: ${current_managed_count}"
    echo ""
    echo "Target trust"
    echo "  Target ref: ${target_ref}"
    echo "  Target git SHA: ${target_sha}"
    echo "  Install mode: ${target_mode}"
    echo "  Dirty source: ${target_dirty}"
    echo "  Supported profiles: ${target_profiles}"
    echo "  Supported interop sources: ${target_interop}"
    echo "  Framework-managed files that will be replaced: ${target_managed_count}"
    echo ""
    echo "Project-owned files that will not be touched"
    echo "  ${project_owned_untouched}"
    echo ""
    echo "Trust warnings"

    if [[ ! -f "$current_manifest" || ! -f "$current_provenance" ]]; then
      echo "  - Current install is missing release trust metadata. Rerun install or upgrade before trusting this snapshot."
    fi
    if [[ "$current_dirty" == 'true' ]]; then
      echo "  - Current install came from a dirty local source checkout. Treat it as an unpublished framework snapshot."
    fi
    if [[ "$target_dirty" == 'true' ]]; then
      echo "  - Target local source is dirty. This preview is not equivalent to a clean published release."
    fi
    if [[ "$write_guard_clean" == 'false' ]]; then
      echo "  - Current framework-managed files already drift from the installed snapshot. Run bubbles framework-write-guard before upgrading."
    elif [[ "$write_guard_clean" == 'source-repo' ]]; then
      echo "  - Framework write guard is not applicable in the Bubbles source repo."
    fi
    if [[ "$current_profiles" != "$target_profiles" ]]; then
      echo "  - Supported profile set changes from ${current_profiles} to ${target_profiles}."
    fi
    if [[ "$current_interop" != "$target_interop" ]]; then
      echo "  - Supported interop sources change from ${current_interop} to ${target_interop}."
    fi

    rm -f "$target_manifest"
    trap - RETURN
    return
  fi

  # Download and run install.sh
  proj_root="$(project_root)"
  if [[ -n "$local_source" ]]; then
    bash "$local_source/install.sh" --local-source "$local_source"
  elif [[ -n "${BUBBLES_SOURCE_OVERRIDE_DIR:-}" ]]; then
    BUBBLES_SOURCE_OVERRIDE_DIR="$BUBBLES_SOURCE_OVERRIDE_DIR" bash "$BUBBLES_SOURCE_OVERRIDE_DIR/install.sh" "$target_version"
  else
    curl -fsSL "https://raw.githubusercontent.com/${repo}/${target_version}/install.sh" | bash -s -- "$target_version"
  fi

  # Run doctor to validate
  echo ""
  cmd_doctor

  # Staleness recommendations
  echo ""
  echo "📋 Checking user-owned files for staleness..."
  local recs=0
  if [[ -f "$REPO_ROOT/.github/copilot-instructions.md" ]]; then
    local t
    t=$(bootstrap_placeholder_count "$REPO_ROOT/.github/copilot-instructions.md")
    t=${t:-0}
    if [[ "$t" -gt 0 ]]; then
      echo "  ⚠️  copilot-instructions.md has $t unfilled bootstrap placeholder items"
      recs=$((recs + 1))
    fi
  fi
  if [[ -f "$REPO_ROOT/.specify/memory/agents.md" ]]; then
    local t
    t=$(bootstrap_placeholder_count "$REPO_ROOT/.specify/memory/agents.md")
    t=${t:-0}
    if [[ "$t" -gt 0 ]]; then
      echo "  ⚠️  agents.md has $t unfilled bootstrap placeholder items"
      recs=$((recs + 1))
    fi
  fi
  if [[ "$recs" -eq 0 ]]; then
    echo "  ✅ No staleness issues found."
  else
    echo ""
    echo "ℹ️  These are recommendations, not errors. Your files were NOT modified."
  fi

  echo ""
  echo "✅ Upgrade complete."
  fun_summary pass
}

# ── Main dispatch ───────────────────────────────────────────────────

main() {
  if [[ $# -eq 0 ]]; then
    cmd_status
    return 0
  fi

  local command="$1"
  shift

  CURRENT_BUBBLES_COMMAND="$command"
  CURRENT_BUBBLES_ARGS="$*"
  COMMAND_START_MS="$(current_epoch_ms)"
  CLI_RECORDING_ACTIVE=true
  begin_cli_run_state
  trap 'record_cli_completion $?' EXIT

  case "$command" in
    status|dashboard)   cmd_status "$@" ;;
    specs|list)         cmd_specs "$@" ;;
    blocked)            cmd_blocked "$@" ;;
    dod)                cmd_dod "$@" ;;
    policy)             cmd_policy "$@" ;;
    runtime)            cmd_runtime "$@" ;;
    session)            cmd_session "$@" ;;
    lint)               cmd_lint "$@" ;;
    agnosticity)        cmd_agnosticity "$@" ;;
    guard)              cmd_guard "$@" ;;
    guard-selftest)     cmd_guard_selftest "$@" ;;
    runtime-selftest)   cmd_runtime_selftest "$@" ;;
    finding-closure-selftest) cmd_finding_closure_selftest "$@" ;;
    workflow-selftest)  cmd_workflow_selftest "$@" ;;
    scan)               cmd_scan "$@" ;;
    regression-quality) cmd_regression_quality "$@" ;;
    docs-registry)      cmd_docs_registry "$@" ;;
    framework-write-guard) cmd_framework_write_guard "$@" ;;
    interop)            cmd_interop "$@" ;;
    framework-validate) cmd_framework_validate "$@" ;;
    release-check)      cmd_release_check "$@" ;;
    framework-events)   cmd_framework_events "$@" ;;
    run-state)          cmd_run_state "$@" ;;
    repo-readiness)     cmd_repo_readiness "$@" ;;
    framework-proposal)  cmd_framework_proposal "$@" ;;
    audit-done|audit)   cmd_audit_done "$@" ;;
    autofix)            cmd_autofix "$@" ;;
    dag)                cmd_dag "$@" ;;
    doctor)             cmd_doctor "$@" ;;
    hooks)              cmd_hooks "$@" ;;
    project)            cmd_project "$@" ;;
    metrics)            cmd_metrics "$@" ;;
    lessons)            cmd_lessons "$@" ;;
    skill-proposals)    cmd_skill_proposals "$@" ;;
    profile)            cmd_profile "$@" ;;
    upgrade)            cmd_upgrade "$@" ;;
    lint-budget)        cmd_lint_budget "$@" ;;
    sunnyvale)          cmd_sunnyvale "$@" ;;
    aliases)            cmd_aliases "$@" ;;
    help|-h|--help)     cmd_help ;;
    *)                  die "Unknown command: $command\nRun 'bubbles help' for usage." ;;
  esac
}

main "$@"
