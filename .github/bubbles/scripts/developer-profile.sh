#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SPECS_DIR="$REPO_ROOT/specs"
CONFIG_FILE="$REPO_ROOT/.specify/memory/bubbles.config.json"
PROFILE_FILE="$REPO_ROOT/.specify/memory/developer-profile.md"
OBSERVATIONS_FILE="$REPO_ROOT/.specify/metrics/observations.jsonl"

fresh_cutoff_days=90
stale_cutoff_days=180

read_config_string() {
  local anchor="$1"
  local field="$2"
  local default_value="$3"
  local value

  value="$({
    grep -A6 "\"${anchor}\"" "$CONFIG_FILE" 2>/dev/null \
      | grep -m1 "\"${field}\"" \
      | sed -E 's/.*:[[:space:]]*"([^"]+)".*/\1/'
  } || true)"

  if [[ -n "$value" ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$default_value"
  fi
}

count_scopes() {
  local spec_dir="$1"

  if [[ -d "$spec_dir/scopes" ]]; then
    find "$spec_dir/scopes" -name 'scope.md' | wc -l | tr -d ' '
    return 0
  fi

  if [[ -f "$spec_dir/scopes.md" ]]; then
    local count
    count="$(grep -c '^Status:' "$spec_dir/scopes.md" 2>/dev/null || true)"
    if [[ -n "$count" && "$count" != "0" ]]; then
      printf '%s\n' "$count"
      return 0
    fi
  fi

  printf '0\n'
}

count_dod_items() {
  local spec_dir="$1"

  if [[ -d "$spec_dir/scopes" ]]; then
    find "$spec_dir/scopes" -name 'scope.md' -exec grep -c '^\- \[' {} + 2>/dev/null | awk '{sum += $1} END {print sum + 0}'
    return 0
  fi

  if [[ -f "$spec_dir/scopes.md" ]]; then
    grep -c '^\- \[' "$spec_dir/scopes.md" 2>/dev/null || printf '0\n'
    return 0
  fi

  printf '0\n'
}

build_mode_preferences() {
  if [[ ! -d "$SPECS_DIR" ]]; then
    return 0
  fi

  find "$SPECS_DIR" -maxdepth 2 -name 'state.json' -not -path '*/bugs/*' -print \
    | while read -r state_file; do
        grep -oE '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null \
          | sed -E 's/.*"([^"]+)"$/\1/'
      done \
    | sed '/^$/d' \
    | sort \
    | uniq -c \
    | awk '$1 >= 3 {printf "%s|%s\n", $2, $1}'
}

recent_surface_focus() {
  git -C "$REPO_ROOT" log -n 40 --name-only --pretty=format: 2>/dev/null \
    | sed '/^$/d' \
    | awk -F/ '
        /^docs\// {docs++}
        /^agents\// {agents++}
        /^bubbles\/scripts\// {scripts++}
        /^bubbles\// {control++}
        END {
          if (docs + agents + scripts + control == 0) {
            exit 0
          }
          printf "docs|%d\n", docs + 0
          printf "agents|%d\n", agents + 0
          printf "scripts|%d\n", scripts + 0
          printf "control-plane|%d\n", control + 0
        }
      ' \
    | sort -t'|' -k2,2nr | head -n 2
}

write_observations() {
  local temp_file
  temp_file="$(mktemp)"

  mkdir -p "$(dirname "$OBSERVATIONS_FILE")"

  while IFS='|' read -r mode count; do
    [[ -n "$mode" ]] || continue
    printf '{"timestamp":"%s","type":"workflowMode","value":"%s","source":"state.json","count":%s,"confidence":"explicit"}\n' \
      "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$mode" "$count" >> "$temp_file"
  done <<< "$(build_mode_preferences)"

  while IFS='|' read -r surface count; do
    [[ -n "$surface" ]] || continue
    printf '{"timestamp":"%s","type":"surfaceFocus","value":"%s","source":"git-log","count":%s,"confidence":"inferred"}\n' \
      "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$surface" "$count" >> "$temp_file"
  done <<< "$(recent_surface_focus)"

  mv "$temp_file" "$OBSERVATIONS_FILE"
}

write_profile() {
  local total_specs=0
  local total_scopes=0
  local total_dod=0
  local spec_dir
  local grill_mode tdd_mode auto_commit
  local temp_file

  grill_mode="$(read_config_string grill mode off)"
  tdd_mode="$(read_config_string tdd mode scenario-first)"
  auto_commit="$(read_config_string autoCommit mode off)"

  if [[ -d "$SPECS_DIR" ]]; then
    while read -r spec_dir; do
      [[ -n "$spec_dir" ]] || continue
      total_specs=$((total_specs + 1))
      total_scopes=$((total_scopes + $(count_scopes "$spec_dir")))
      total_dod=$((total_dod + $(count_dod_items "$spec_dir")))
    done < <(find "$SPECS_DIR" -maxdepth 1 -mindepth 1 -type d | sort)
  fi

  temp_file="$(mktemp)"
  {
    echo "# Developer Profile"
    echo
    echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo
    echo "## Fresh Preferences"
    if [[ -f "$OBSERVATIONS_FILE" ]]; then
      while IFS= read -r observation; do
        if [[ "$observation" == *'"type":"workflowMode"'* ]]; then
          value="$(printf '%s\n' "$observation" | sed -E 's/.*"value":"([^"]+)".*/\1/')"
          count="$(printf '%s\n' "$observation" | sed -E 's/.*"count":([0-9]+).*/\1/')"
          echo "- Workflow mode preference: ${value} (observed ${count} times from state.json)"
        elif [[ "$observation" == *'"type":"surfaceFocus"'* ]]; then
          value="$(printf '%s\n' "$observation" | sed -E 's/.*"value":"([^"]+)".*/\1/')"
          count="$(printf '%s\n' "$observation" | sed -E 's/.*"count":([0-9]+).*/\1/')"
          echo "- Surface focus: ${value} (recent git activity count ${count})"
        fi
      done < "$OBSERVATIONS_FILE"
    fi
    if [[ "$total_specs" -gt 0 && "$total_scopes" -gt 0 ]]; then
      avg_dod=$((total_dod / total_scopes))
      echo "- Scope sizing tendency: approximately ${avg_dod} DoD items per scope across ${total_scopes} scopes"
    fi
    echo "- Control-plane defaults in use: grill=${grill_mode}, tdd=${tdd_mode}, autoCommit=${auto_commit}"
    echo
    echo "## Aging Preferences"
    echo "- None currently inferred from repo history older than ${fresh_cutoff_days} days"
    echo
    echo "## Stale Preferences"
    echo "- None older than ${stale_cutoff_days} days"
  } > "$temp_file"

  mkdir -p "$(dirname "$PROFILE_FILE")"
  mv "$temp_file" "$PROFILE_FILE"
}

show_profile() {
  write_observations
  write_profile
  cat "$PROFILE_FILE"
}

show_stale() {
  if [[ ! -f "$OBSERVATIONS_FILE" ]]; then
    echo "No developer observations recorded yet."
    return 0
  fi

  awk -v cutoff="$(date -u -d "${stale_cutoff_days} days ago" +"%Y-%m-%dT%H:%M:%SZ")" '
    match($0, /"timestamp":"([^"]+)"/, ts) && ts[1] < cutoff { print $0; found = 1 }
    END { if (!found) print "No stale developer observations." }
  ' "$OBSERVATIONS_FILE"
}

clear_stale() {
  local cutoff temp_file

  if [[ ! -f "$OBSERVATIONS_FILE" ]]; then
    echo "No developer observations recorded yet."
    return 0
  fi

  cutoff="$(date -u -d "${stale_cutoff_days} days ago" +"%Y-%m-%dT%H:%M:%SZ")"
  temp_file="$(mktemp)"
  awk -v cutoff="$cutoff" '
    match($0, /"timestamp":"([^"]+)"/, ts) {
      if (ts[1] >= cutoff) {
        print
      }
      next
    }
    { print }
  ' "$OBSERVATIONS_FILE" > "$temp_file"
  mv "$temp_file" "$OBSERVATIONS_FILE"
  write_profile
  echo "Cleared stale developer observations older than ${stale_cutoff_days} days."
}

case "${1:-show}" in
  show|refresh)
    show_profile
    ;;
  stale|--stale)
    show_stale
    ;;
  clear-stale|--clear-stale)
    clear_stale
    ;;
  *)
    echo "Usage: $(basename "$0") [show|refresh|stale|clear-stale]" >&2
    exit 1
    ;;
esac