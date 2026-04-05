#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_ROOT="$DEFAULT_ROOT"
DEEP=false
JSON_MODE=false

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
    --help|-h)
      echo "Usage: repo-readiness.sh [repo-root] [--deep] [--json]"
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

is_bubbles_source=false
is_bubbles_downstream=false
source_cli="$TARGET_ROOT/bubbles/scripts/cli.sh"
downstream_cli="$TARGET_ROOT/.github/bubbles/scripts/cli.sh"

if [[ -f "$source_cli" && -f "$TARGET_ROOT/agents/bubbles.super.agent.md" ]]; then
  is_bubbles_source=true
elif [[ -f "$downstream_cli" && -f "$TARGET_ROOT/.github/agents/bubbles.super.agent.md" ]]; then
  is_bubbles_downstream=true
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
  else
    add_check warn "Agent-facing instructions" "Neither .github/copilot-instructions.md nor AGENTS.md is present"
  fi

  if [[ -d "$TARGET_ROOT/specs" ]]; then
    add_check pass "Specs directory" "$TARGET_ROOT/specs"
  else
    add_check warn "Specs directory" "specs/ missing; structured delivery packets are not available"
  fi

  if [[ -f "$TARGET_ROOT/.specify/memory/agents.md" ]]; then
    add_check pass "Command registry" "$TARGET_ROOT/.specify/memory/agents.md"
  else
    add_check warn "Command registry" ".specify/memory/agents.md missing"
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
  printf '{"repo":"%s","pass":%d,"warn":%d,"fail":%d,"bubblesSource":%s,"bubblesDownstream":%s,"checks":[' \
    "$TARGET_ROOT" "$PASS_COUNT" "$WARN_COUNT" "$FAIL_COUNT" "$is_bubbles_source" "$is_bubbles_downstream"
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
