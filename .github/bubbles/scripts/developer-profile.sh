#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SPECS_DIR="$REPO_ROOT/specs"
CONFIG_FILE="$REPO_ROOT/.specify/memory/bubbles.config.json"
PROFILE_FILE="$REPO_ROOT/.specify/memory/developer-profile.md"
OBSERVATIONS_FILE="$REPO_ROOT/.specify/metrics/observations.jsonl"
ADOPTION_PROFILES_FILE="$REPO_ROOT/bubbles/adoption-profiles.yaml"

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

active_adoption_profile() {
  if [[ -f "$CONFIG_FILE" ]]; then
    grep -oE '"adoptionProfile"[[:space:]]*:[[:space:]]*"[^"]+"' "$CONFIG_FILE" 2>/dev/null \
      | sed -E 's/.*"([^"]+)"$/\1/'
  fi
}

adoption_profile_is_explicit() {
  [[ -f "$CONFIG_FILE" ]] && grep -q '"adoptionProfile"' "$CONFIG_FILE"
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

adoption_profile_scalar() {
  local profile="$1"
  local key="$2"

  [[ -f "$ADOPTION_PROFILES_FILE" ]] || return 1

  awk -v profile="$profile" -v key="$key" '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && $0 ~ ("^  " profile ":$") { in_profile=1; next }
    in_profile && /^  [A-Za-z0-9_-]+:$/ { in_profile=0 }
    in_profile && $0 ~ ("^    " key ":[[:space:]]*") {
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
  local active_profile
  local known_profile

  if [[ -n "$requested_profile" ]]; then
    active_profile="$requested_profile"
  else
    active_profile="$(active_adoption_profile)"
  fi

  [[ -n "$active_profile" ]] || active_profile='delivery'

  while IFS= read -r known_profile; do
    [[ -n "$known_profile" ]] || continue
    if [[ "$known_profile" == "$active_profile" ]]; then
      printf '%s\n' "$active_profile"
      return 0
    fi
  done < <(adoption_profile_ids)

  echo "Unknown adoption profile: $active_profile" >&2
  exit 1
}

set_active_profile() {
  local requested_profile="${1:-}"
  local effective_profile

  [[ -n "$requested_profile" ]] || {
    echo "Usage: $(basename "$0") set <foundation|delivery|assured>" >&2
    exit 1
  }

  [[ -f "$CONFIG_FILE" ]] || {
    echo "Missing $CONFIG_FILE. Re-run Bubbles bootstrap or install first." >&2
    exit 1
  }

  effective_profile="$(effective_adoption_profile "$requested_profile")"

  if grep -q '"adoptionProfile"' "$CONFIG_FILE"; then
    perl -0pi -e 's/"adoptionProfile"\s*:\s*"[^"]+"/"adoptionProfile": "'"$effective_profile"'"/' "$CONFIG_FILE"
  else
    perl -0pi -e 's/(\{\n  "version": [0-9]+,\n)/$1  "adoptionProfile": "'"$effective_profile"'",\n/' "$CONFIG_FILE"
  fi

  printf 'Set adoption profile to %s (%s).\n' "$(adoption_profile_scalar "$effective_profile" label)" "$effective_profile"
}

print_profile_list() {
  local current_profile
  local profile

  current_profile="$(effective_adoption_profile)"

  while IFS= read -r profile; do
    [[ -n "$profile" ]] || continue
    printf '%s|%s|%s\n' "$profile" "$(adoption_profile_scalar "$profile" label)" "$(adoption_profile_scalar "$profile" description)"
  done < <(adoption_profile_ids)
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

write_profile_list_section() {
  local temp_file="$1"
  local profile="$2"
  local label description intended_audience bootstrap_summary invariant posture doctor_readiness
  local item_found=false

  label="$(adoption_profile_scalar "$profile" label)"
  description="$(adoption_profile_scalar "$profile" description)"
  intended_audience="$(adoption_profile_scalar "$profile" intendedAudience)"
  bootstrap_summary="$(adoption_profile_scalar "$profile" bootstrapSummary)"
  invariant="$(adoption_profile_scalar "$profile" governanceInvariant)"
  posture="$(adoption_profile_scalar "$profile" repoReadinessPosture)"
  doctor_readiness="$(adoption_profile_scalar "$profile" doctorProjectReadiness)"

  {
    echo "### ${label} (${profile})"
    echo
    echo "- Description: ${description}"
    echo "- Intended audience: ${intended_audience}"
    echo "- Bootstrap summary: ${bootstrap_summary}"
    echo "- Repo-readiness posture: ${posture}"
    echo "- Doctor project readiness: ${doctor_readiness}"
    echo "- Governance invariant: ${invariant}"
    echo "- Required docs:"
  } >> "$temp_file"

  while IFS= read -r item; do
    [[ -n "$item" ]] || continue
    item_found=true
    echo "  - ${item}" >> "$temp_file"
  done < <(adoption_profile_list "$profile" requiredDocs)

  if [[ "$item_found" == false ]]; then
    echo "  - None declared" >> "$temp_file"
  fi

  echo "- Recommended next commands:" >> "$temp_file"
  item_found=false
  while IFS= read -r item; do
    [[ -n "$item" ]] || continue
    item_found=true
    echo "  - ${item}" >> "$temp_file"
  done < <(adoption_profile_list "$profile" recommendedNextCommands)

  if [[ "$item_found" == false ]]; then
    echo "  - None declared" >> "$temp_file"
  fi

  echo >> "$temp_file"
}

write_profile() {
  local selected_profile="${1:-}"
  local total_specs=0
  local total_scopes=0
  local total_dod=0
  local spec_dir
  local grill_mode tdd_mode auto_commit
  local temp_file
  local active_profile current_profile profile_source current_label current_description current_audience
  local current_invariant current_posture current_doctor_readiness comparison_notice value count avg_dod

  grill_mode="$(read_config_string grill mode off)"
  tdd_mode="$(read_config_string tdd mode scenario-first)"
  auto_commit="$(read_config_string autoCommit mode off)"
  active_profile="$(effective_adoption_profile)"
  current_profile="$(effective_adoption_profile "$selected_profile")"

  if [[ -d "$SPECS_DIR" ]]; then
    while read -r spec_dir; do
      [[ -n "$spec_dir" ]] || continue
      total_specs=$((total_specs + 1))
      total_scopes=$((total_scopes + $(count_scopes "$spec_dir")))
      total_dod=$((total_dod + $(count_dod_items "$spec_dir")))
    done < <(find "$SPECS_DIR" -maxdepth 1 -mindepth 1 -type d | sort)
  fi

  if adoption_profile_is_explicit; then
    profile_source='repo-local policy registry'
  else
    profile_source='installer default'
  fi

  current_label="$(adoption_profile_scalar "$current_profile" label)"
  current_description="$(adoption_profile_scalar "$current_profile" description)"
  current_audience="$(adoption_profile_scalar "$current_profile" intendedAudience)"
  current_invariant="$(adoption_profile_scalar "$current_profile" governanceInvariant)"
  current_posture="$(adoption_profile_scalar "$current_profile" repoReadinessPosture)"
  current_doctor_readiness="$(adoption_profile_scalar "$current_profile" doctorProjectReadiness)"

  temp_file="$(mktemp)"
  {
    echo "# Developer Profile"
    echo
    echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo
    echo "## Adoption Profile"
    echo "- Active profile: $(adoption_profile_scalar "$active_profile" label) (${active_profile})"
    echo "- Profile source: ${profile_source}"
    if [[ "$current_profile" != "$active_profile" ]]; then
      echo "- Comparison view: ${current_label} (${current_profile})"
    fi
    echo "- Description: ${current_description}"
    echo "- Intended audience: ${current_audience}"
    echo "- Repo-readiness posture: ${current_posture}"
    echo "- Doctor project readiness: ${current_doctor_readiness}"
    echo "- Governance invariant: ${current_invariant}"
    echo
    echo "### Required Docs"
  } > "$temp_file"

  comparison_notice=false
  while IFS= read -r value; do
    [[ -n "$value" ]] || continue
    comparison_notice=true
    echo "- ${value}" >> "$temp_file"
  done < <(adoption_profile_list "$current_profile" requiredDocs)

  if [[ "$comparison_notice" == false ]]; then
    echo "- None declared" >> "$temp_file"
  fi

  {
    echo
    echo "### Recommended Next Commands"
  } >> "$temp_file"

  comparison_notice=false
  while IFS= read -r value; do
    [[ -n "$value" ]] || continue
    comparison_notice=true
    echo "- ${value}" >> "$temp_file"
  done < <(adoption_profile_list "$current_profile" recommendedNextCommands)

  if [[ "$comparison_notice" == false ]]; then
    echo "- None declared" >> "$temp_file"
  fi

  {
    echo
    echo "## Invariants Preserved Across All Profiles"
    echo "- Validate-owned certification remains authoritative for completion state."
    echo "- Scenario contracts remain unchanged when profile guidance changes."
    echo "- Artifact ownership boundaries remain unchanged across profile transitions."
    echo
    echo "## Available Profiles"
    echo
  } >> "$temp_file"

  while IFS= read -r value; do
    [[ -n "$value" ]] || continue
    write_profile_list_section "$temp_file" "$value"
  done < <(adoption_profile_ids)

  {
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
  } >> "$temp_file"

  mkdir -p "$(dirname "$PROFILE_FILE")"
  mv "$temp_file" "$PROFILE_FILE"
}

show_profile() {
  write_observations
  write_profile "${1:-}"
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
    show_profile "${2:-}"
    ;;
  list|profiles)
    print_profile_list
    ;;
  set)
    set_active_profile "${2:-}"
    ;;
  stale|--stale)
    show_stale
    ;;
  clear-stale|--clear-stale)
    clear_stale
    ;;
  *)
    echo "Usage: $(basename "$0") [show [profile]|list|set <profile>|refresh|stale|clear-stale]" >&2
    exit 1
    ;;
esac