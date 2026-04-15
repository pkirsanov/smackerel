#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_ROOT="$DEFAULT_ROOT"
DEEP=false
JSON_MODE=false
PROFILE_OVERRIDE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --deep)
      DEEP=true
      shift
      ;;
    --json)
      JSON_MODE=true
      shift
      ;;
    --profile)
      PROFILE_OVERRIDE="$2"
      shift 2
      ;;
    --help|-h)
      echo "Usage: repo-readiness.sh [repo-root] [--deep] [--json] [--profile PROFILE]"
      exit 0
      ;;
    --*)
      echo "Unknown repo-readiness option: $1" >&2
      exit 1
      ;;
    *)
      TARGET_ROOT="$1"
      shift
      ;;
  esac
done

TARGET_ROOT="$(cd "$TARGET_ROOT" && pwd)"

CHECKS=''
PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

add_check() {
  local level="$1"
  local label="$2"
  local detail="$3"

  case "$level" in
    pass) PASS_COUNT=$((PASS_COUNT + 1)) ;;
    warn) WARN_COUNT=$((WARN_COUNT + 1)) ;;
    fail) FAIL_COUNT=$((FAIL_COUNT + 1)) ;;
  esac

  if [[ -z "$CHECKS" ]]; then
    CHECKS="${level}|${label}|${detail}"
  else
    CHECKS="$CHECKS
${level}|${label}|${detail}"
  fi
}

json_escape() {
  local raw="$1"

  raw=${raw//\\/\\\\}
  raw=${raw//"/\\"}
  raw=${raw//$'\n'/ }
  raw=${raw//$'\r'/ }
  printf '%s' "$raw"
}

active_adoption_profile() {
  local config_file="$TARGET_ROOT/.specify/memory/bubbles.config.json"

  if [[ -f "$config_file" ]]; then
    grep -oE '"adoptionProfile"[[:space:]]*:[[:space:]]*"[^"]+"' "$config_file" 2>/dev/null \
      | sed -E 's/.*"([^"]+)"$/\1/'
  fi
}

adoption_profile_is_explicit() {
  local config_file="$TARGET_ROOT/.specify/memory/bubbles.config.json"

  [[ -f "$config_file" ]] && grep -q '"adoptionProfile"' "$config_file"
}

adoption_profile_ids() {
  local registry_file="$1"

  [[ -f "$registry_file" ]] || return 0

  awk '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && /^  [A-Za-z0-9_-]+:$/ {
      profile=$1
      sub(":$", "", profile)
      print profile
    }
  ' "$registry_file"
}

adoption_profile_value() {
  local registry_file="$1"
  local profile="$2"
  local key="$3"

  [[ -f "$registry_file" ]] || return 1

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
  ' "$registry_file"
}

adoption_profile_list() {
  local registry_file="$1"
  local profile="$2"
  local key="$3"

  [[ -f "$registry_file" ]] || return 0

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
  ' "$registry_file"
}

adoption_profile_map_value() {
  local registry_file="$1"
  local profile="$2"
  local key="$3"
  local map_key="$4"

  [[ -f "$registry_file" ]] || return 1

  awk -v profile="$profile" -v key="$key" -v map_key="$map_key" '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && $0 ~ ("^  " profile ":$") { in_profile=1; next }
    in_profile && /^  [A-Za-z0-9_-]+:$/ { in_profile=0; in_map=0 }
    in_profile && $0 ~ ("^    " key ":$") { in_map=1; next }
    in_map && /^    [A-Za-z0-9_-]+:/ { in_map=0 }
    in_map && $0 ~ ("^      " map_key ":[[:space:]]*") {
      sub("^      " map_key ":[[:space:]]*", "", $0)
      gsub(/^"|"$/, "", $0)
      print
      exit
    }
  ' "$registry_file"
}

effective_adoption_profile() {
  local registry_file="$1"
  local profile="$PROFILE_OVERRIDE"

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
  done < <(adoption_profile_ids "$registry_file")

  echo "Unknown adoption profile: $profile" >&2
  exit 1
}

profile_readiness_level() {
  local logical_key="$1"
  local default_level="$2"
  local level=''

  if [[ -n "$ADOPTION_PROFILES_FILE" ]]; then
    level="$(adoption_profile_map_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" repoReadinessSeverityMap "$logical_key" || true)"
  fi

  if [[ -n "$level" ]]; then
    printf '%s' "$level"
  else
    printf '%s' "$default_level"
  fi
}

add_profiled_missing_check() {
  local logical_key="$1"
  local label="$2"
  local detail="$3"
  local default_level="$4"
  local level

  level="$(profile_readiness_level "$logical_key" "$default_level")"
  add_check "$level" "$label" "$detail"
}

is_bubbles_source=false
is_bubbles_downstream=false
source_cli="$TARGET_ROOT/bubbles/scripts/cli.sh"
downstream_cli="$TARGET_ROOT/.github/bubbles/scripts/cli.sh"

if [[ -f "$source_cli" && -f "$TARGET_ROOT/agents/bubbles.super.agent.md" ]]; then
  is_bubbles_source=true
elif [[ -f "$downstream_cli" && -f "$TARGET_ROOT/.github/agents/bubbles.super.agent.md" ]]; then
  is_bubbles_downstream=true
fi

ADOPTION_PROFILES_FILE=''
if [[ "$is_bubbles_source" == true ]]; then
  ADOPTION_PROFILES_FILE="$TARGET_ROOT/bubbles/adoption-profiles.yaml"
elif [[ "$is_bubbles_downstream" == true ]]; then
  ADOPTION_PROFILES_FILE="$TARGET_ROOT/.github/bubbles/adoption-profiles.yaml"
fi

CURRENT_PROFILE='delivery'
CURRENT_PROFILE_LABEL=''
CURRENT_PROFILE_SUMMARY=''
CURRENT_PROFILE_INVARIANT=''
CURRENT_PROFILE_POSTURE=''
CURRENT_PROFILE_DESCRIPTION=''
CURRENT_PROFILE_AUDIENCE=''
PROFILE_SOURCE='installer-default'
if [[ -n "$ADOPTION_PROFILES_FILE" ]]; then
  CURRENT_PROFILE="$(effective_adoption_profile "$ADOPTION_PROFILES_FILE")"
  CURRENT_PROFILE_LABEL="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" label)"
  CURRENT_PROFILE_DESCRIPTION="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" description)"
  CURRENT_PROFILE_AUDIENCE="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" intendedAudience)"
  CURRENT_PROFILE_SUMMARY="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" bootstrapSummary)"
  CURRENT_PROFILE_INVARIANT="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" governanceInvariant)"
  CURRENT_PROFILE_POSTURE="$(adoption_profile_value "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" repoReadinessPosture)"
  if adoption_profile_is_explicit; then
    PROFILE_SOURCE='repo-local-policy-registry'
  fi
fi

if [[ -d "$TARGET_ROOT/.git" ]]; then
  add_check pass "Git repository" "$TARGET_ROOT"
else
  add_check fail "Git repository" "Not a git repository root"
fi

if [[ -f "$TARGET_ROOT/README.md" ]]; then
  add_check pass "Top-level README" "$TARGET_ROOT/README.md"
else
  add_check fail "Top-level README" "README.md missing"
fi

if [[ "$is_bubbles_source" == true ]]; then
  add_check pass "Bubbles layout" "Source framework repo detected"
elif [[ "$is_bubbles_downstream" == true ]]; then
  add_check pass "Bubbles layout" "Installed downstream framework detected"
else
  add_check warn "Bubbles layout" "No Bubbles CLI surface detected"
fi

if [[ "$is_bubbles_source" == true ]]; then
  if [[ -d "$TARGET_ROOT/agents" ]]; then
    add_check pass "Framework agent inventory" "$TARGET_ROOT/agents"
  else
    add_check fail "Framework agent inventory" "agents/ missing in source framework repo"
  fi

  if [[ -d "$TARGET_ROOT/docs/recipes" && -d "$TARGET_ROOT/docs/guides" ]]; then
    add_check pass "Framework guidance docs" "$TARGET_ROOT/docs"
  else
    add_check warn "Framework guidance docs" "Expected docs/recipes and docs/guides in source framework repo"
  fi

  if [[ -f "$TARGET_ROOT/agents/bubbles.super.agent.md" ]]; then
    add_check pass "Front-door super agent" "$TARGET_ROOT/agents/bubbles.super.agent.md"
  else
    add_check fail "Front-door super agent" "agents/bubbles.super.agent.md missing"
  fi
else
  if [[ -f "$TARGET_ROOT/.github/copilot-instructions.md" || -f "$TARGET_ROOT/AGENTS.md" ]]; then
    add_check pass "Agent-facing instructions" "Found repo guidance for coding agents"
    # Recommend both files for maximum AI tool coverage
    if [[ -f "$TARGET_ROOT/.github/copilot-instructions.md" && ! -f "$TARGET_ROOT/AGENTS.md" ]]; then
      add_check info "Root AGENTS.md" "Consider adding AGENTS.md for broader AI tool coverage (Claude Code, Cursor, Gemini)"
    fi
  else
    add_profiled_missing_check "agentFacingInstructions" "Agent-facing instructions" "Neither .github/copilot-instructions.md nor AGENTS.md is present" warn
  fi

  if [[ -d "$TARGET_ROOT/specs" ]]; then
    add_check pass "Specs directory" "$TARGET_ROOT/specs"
  else
    add_profiled_missing_check "specsDirectory" "Specs directory" "specs/ missing; structured delivery packets are not available" warn
  fi

  if [[ -f "$TARGET_ROOT/.specify/memory/agents.md" ]]; then
    add_check pass "Command registry" "$TARGET_ROOT/.specify/memory/agents.md"
  else
    add_profiled_missing_check "commandRegistry" "Command registry" ".specify/memory/agents.md missing" warn
  fi
fi

if [[ "$is_bubbles_source" == true || "$is_bubbles_downstream" == true ]]; then
  if [[ "$is_bubbles_source" == true ]]; then
    framework_root="$TARGET_ROOT/bubbles"
    cli_path="$source_cli"
  else
    framework_root="$TARGET_ROOT/.github/bubbles"
    cli_path="$downstream_cli"
  fi

  required_framework_files=(
    "$framework_root/workflows.yaml"
    "$framework_root/action-risk-registry.yaml"
    "$framework_root/scripts/cli.sh"
  )

  for required_file in "${required_framework_files[@]}"; do
    if [[ -f "$required_file" ]]; then
      add_check pass "Framework file" "$required_file"
    else
      add_check fail "Framework file" "Missing $required_file"
    fi
  done

  if [[ "$DEEP" == true ]]; then
    if output="$(cd "$TARGET_ROOT" && bash "$cli_path" doctor 2>&1)"; then
      add_check pass "Deep framework doctor" "doctor passed"
    else
      add_check fail "Deep framework doctor" "$output"
    fi
  fi
fi

if [[ "$JSON_MODE" == true ]]; then
  printf '{"repo":"%s","pass":%d,"warn":%d,"fail":%d,"bubblesSource":%s,"bubblesDownstream":%s,"profile":"%s","profileLabel":"%s","profileDescription":"%s","profileAudience":"%s","profileInvariant":"%s","profileSource":"%s","checks":[' \
    "$TARGET_ROOT" "$PASS_COUNT" "$WARN_COUNT" "$FAIL_COUNT" "$is_bubbles_source" "$is_bubbles_downstream" "$(json_escape "$CURRENT_PROFILE")" "$(json_escape "$CURRENT_PROFILE_LABEL")" "$(json_escape "$CURRENT_PROFILE_DESCRIPTION")" "$(json_escape "$CURRENT_PROFILE_AUDIENCE")" "$(json_escape "$CURRENT_PROFILE_INVARIANT")" "$(json_escape "$PROFILE_SOURCE")"
  first=true
  while IFS='|' read -r level label detail; do
    [[ -n "$level" ]] || continue
    if [[ "$first" == false ]]; then
      printf ','
    fi
    first=false
    printf '{"level":"%s","label":"%s","detail":"%s"}' \
      "$(json_escape "$level")" "$(json_escape "$label")" "$(json_escape "$detail")"
  done <<< "$CHECKS"
  printf ']}'
  exit $(( FAIL_COUNT > 0 ? 1 : 0 ))
fi

echo "Bubbles Repo-Readiness"
echo "Repository: $TARGET_ROOT"
echo "Boundary: advisory framework ops only; this does not replace bubbles.validate certification."
if [[ -n "$CURRENT_PROFILE_LABEL" ]]; then
  echo "Active profile: $CURRENT_PROFILE_LABEL ($CURRENT_PROFILE)"
  echo "Profile source: $PROFILE_SOURCE"
  [[ -n "$CURRENT_PROFILE_DESCRIPTION" ]] && echo "Profile description: $CURRENT_PROFILE_DESCRIPTION"
  [[ -n "$CURRENT_PROFILE_AUDIENCE" ]] && echo "Intended audience: $CURRENT_PROFILE_AUDIENCE"
  [[ -n "$CURRENT_PROFILE_SUMMARY" ]] && echo "Profile summary: $CURRENT_PROFILE_SUMMARY"
  [[ -n "$CURRENT_PROFILE_INVARIANT" ]] && echo "Governance invariant: $CURRENT_PROFILE_INVARIANT"
  if [[ "$CURRENT_PROFILE_POSTURE" == 'onboarding-first' ]]; then
    echo "Profile posture: foundation prioritizes onboarding fixes first; later delivery work stays advisory here."
  elif [[ "$CURRENT_PROFILE_POSTURE" == 'guardrail-forward' ]]; then
    echo "Profile posture: assured front-loads guardrail visibility, but certification rigor still remains full-strength."
  else
    echo "Profile posture: delivery expects the standard readiness checklist before broader delivery work scales up."
  fi
  if [[ -n "$ADOPTION_PROFILES_FILE" ]]; then
    echo "Required docs:"
    while IFS= read -r item; do
      [[ -n "$item" ]] || continue
      echo "  - $item"
    done < <(adoption_profile_list "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" requiredDocs)
    echo "Recommended next commands:"
    while IFS= read -r item; do
      [[ -n "$item" ]] || continue
      echo "  - $item"
    done < <(adoption_profile_list "$ADOPTION_PROFILES_FILE" "$CURRENT_PROFILE" recommendedNextCommands)
  fi
fi
echo

while IFS='|' read -r level label detail; do
  [[ -n "$level" ]] || continue
  case "$level" in
    pass) prefix='PASS' ;;
    warn) prefix='WARN' ;;
    fail) prefix='FAIL' ;;
  esac
  printf '%-5s %-24s %s\n' "$prefix" "$label" "$detail"
done <<< "$CHECKS"

echo
echo "Summary: pass=$PASS_COUNT warn=$WARN_COUNT fail=$FAIL_COUNT"

exit $(( FAIL_COUNT > 0 ? 1 : 0 ))
