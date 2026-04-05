#!/usr/bin/env sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)

check_only=false
if [ "${1:-}" = "--check" ]; then
  check_only=true
fi

agents_dir="$repo_root/agents"
workflows_file="$repo_root/bubbles/workflows.yaml"
generated_dir="$repo_root/docs/generated"

count_agents() {
  find "$agents_dir" -maxdepth 1 -type f -name 'bubbles.*.agent.md' | wc -l | tr -d ' '
}

count_workflow_modes() {
  awk '
    /^  [a-z][a-z0-9-]*:$/ {
      candidate = $1
      sub(/:$/, "", candidate)
      if ((getline next_line) > 0) {
        if (next_line ~ /^    description:/) {
          count++
        }
      }
    }
    END {
      print count + 0
    }
  ' "$workflows_file"
}

count_section_entries() {
  section_name="$1"
  entry_pattern="$2"

  awk -v section_name="$section_name" -v entry_pattern="$entry_pattern" '
    /^[A-Za-z][A-Za-z0-9_-]*:/ {
      current = $0
      sub(/:.*/, "", current)
    }
    current == section_name && $0 ~ entry_pattern {
      count++
    }
    END {
      print count + 0
    }
  ' "$workflows_file"
}

write_file() {
  target_file="$1"
  content_file="$2"
  mv "$content_file" "$target_file"
}

replace_block() {
  target_file="$1"
  start_marker="$2"
  end_marker="$3"
  content_file="$4"
  temp_file=$(mktemp)

  awk -v start_marker="$start_marker" -v end_marker="$end_marker" -v content_file="$content_file" '
    BEGIN {
      in_block = 0
      replaced = 0
    }
    index($0, start_marker) {
      print
      while ((getline line < content_file) > 0) {
        print line
      }
      close(content_file)
      in_block = 1
      replaced = 1
      next
    }
    index($0, end_marker) {
      in_block = 0
      print
      next
    }
    !in_block {
      print
    }
    END {
      if (!replaced) {
        exit 2
      }
    }
  ' "$target_file" > "$temp_file"

  mv "$temp_file" "$target_file"
}

mkdir -p "$generated_dir"

version=$(cat "$repo_root/VERSION" | tr -d '[:space:]')

agent_count=$(count_agents)
gate_count=$(count_section_entries gates '^  G[0-9][0-9][0-9]:')
workflow_mode_count=$(count_workflow_modes)
phase_count=$(count_section_entries phases '^  [a-z][a-z0-9-]*:')

summary_line="$agent_count Agents · $gate_count Gates · $workflow_mode_count Workflow Modes · $phase_count Phases"
generated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

json_temp=$(mktemp)
cat <<EOF > "$json_temp"
{
  "agents": $agent_count,
  "gates": $gate_count,
  "workflowModes": $workflow_mode_count,
  "phases": $phase_count,
  "generatedAt": "$generated_at"
}
EOF
if [ "$check_only" = true ]; then
  current_agents=$(grep -oE '"agents":[[:space:]]*[0-9]+' "$generated_dir/framework-stats.json" | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/' || true)
  current_gates=$(grep -oE '"gates":[[:space:]]*[0-9]+' "$generated_dir/framework-stats.json" | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/' || true)
  current_modes=$(grep -oE '"workflowModes":[[:space:]]*[0-9]+' "$generated_dir/framework-stats.json" | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/' || true)
  current_phases=$(grep -oE '"phases":[[:space:]]*[0-9]+' "$generated_dir/framework-stats.json" | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/' || true)
  if [ "$current_agents" != "$agent_count" ] || [ "$current_gates" != "$gate_count" ] || [ "$current_modes" != "$workflow_mode_count" ] || [ "$current_phases" != "$phase_count" ]; then
    printf '%s\n' "Generated framework stats JSON is stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  fi
  rm -f "$json_temp"
else
  write_file "$generated_dir/framework-stats.json" "$json_temp"
fi

markdown_temp=$(mktemp)
cat <<EOF > "$markdown_temp"
# Framework Stats

- Agents: $agent_count
- Gates: $gate_count
- Workflow modes: $workflow_mode_count
- Phases: $phase_count
- Generated at: $generated_at
EOF
if [ "$check_only" = true ]; then
  grep -q -- "- Agents: $agent_count" "$generated_dir/framework-stats.md" || {
    printf '%s\n' "Generated framework stats Markdown is stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  }
  grep -q -- "- Gates: $gate_count" "$generated_dir/framework-stats.md" || {
    printf '%s\n' "Generated framework stats Markdown is stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  }
  grep -q -- "- Workflow modes: $workflow_mode_count" "$generated_dir/framework-stats.md" || {
    printf '%s\n' "Generated framework stats Markdown is stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  }
  grep -q -- "- Phases: $phase_count" "$generated_dir/framework-stats.md" || {
    printf '%s\n' "Generated framework stats Markdown is stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  }
  rm -f "$markdown_temp"
else
  write_file "$generated_dir/framework-stats.md" "$markdown_temp"
fi

if [ "$check_only" = true ]; then
  if grep -q 'workflow mode definitions' "$repo_root/README.md" && ! grep -q "# $workflow_mode_count workflow mode definitions" "$repo_root/README.md"; then
    printf '%s\n' "README generated workflow mode count appears stale. Run bubbles/scripts/generate-framework-stats.sh"
    exit 1
  fi
  printf '%s\n' "Framework stats are current: $summary_line (v$version)"
  exit 0
fi

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
  <img src="https://img.shields.io/badge/agents-$agent_count-58a6ff?style=flat-square" alt="$agent_count agents">
  <img src="https://img.shields.io/badge/gates-$gate_count-3fb950?style=flat-square" alt="$gate_count gates">
  <img src="https://img.shields.io/badge/workflow_modes-$workflow_mode_count-bc8cff?style=flat-square" alt="$workflow_mode_count modes">
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_BADGES_START" "GENERATED:FRAMEWORK_STATS_BADGES_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
<tr><td width="64"><img src="icons/bubbles-glasses.svg" width="48"></td><td><strong>$agent_count specialized agents</strong> — each with a defined role, from implementation to framework ops</td></tr>
<tr><td width="64"><img src="icons/lahey-badge.svg" width="48"></td><td><strong>$gate_count quality gates</strong> — nothing ships without evidence. Nothing.</td></tr>
<tr><td width="64"><img src="icons/julian-glass.svg" width="48"></td><td><strong>$workflow_mode_count workflow modes</strong> — from full delivery to quick bugfixes to chaos sweeps</td></tr>
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_CALLOUTS_START" "GENERATED:FRAMEWORK_STATS_CALLOUTS_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
│   ├── bubbles.workflow.agent.md    # $agent_count agent definitions
│   ├── bubbles.implement.agent.md
│   ├── bubbles.super.agent.md       # NEW: first-touch assistant + framework operations
│   ├── ...
│   └── bubbles_shared/              # Shared governance docs
│       ├── agent-common.md
│       ├── scope-workflow.md
│       └── ...
├── prompts/
│   └── bubbles.*.prompt.md          # $agent_count prompt shims
├── bubbles/
│   ├── workflows.yaml               # $workflow_mode_count workflow mode definitions
│   ├── scripts/                     # Governance scripts
│   │   ├── cli.sh                   # Main CLI
│   │   ├── artifact-lint.sh
│   │   ├── state-transition-guard.sh
│   │   └── ...
│   └── docs/                        # Generated docs
└── scripts/
    └── bubbles.sh                   # CLI shim (dispatches to bubbles/scripts/cli.sh)
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_INSTALL_TREE_START" "GENERATED:FRAMEWORK_STATS_INSTALL_TREE_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
Bubbles supports $workflow_mode_count workflow modes plus optional execution tags. Here are the most common:
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_WORKFLOW_INTRO_START" "GENERATED:FRAMEWORK_STATS_WORKFLOW_INTRO_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
See [docs/guides/WORKFLOW_MODES.md](docs/guides/WORKFLOW_MODES.md) for all $workflow_mode_count modes.
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_WORKFLOW_OUTRO_START" "GENERATED:FRAMEWORK_STATS_WORKFLOW_OUTRO_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
### $gate_count Quality Gates
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_GATES_HEADING_START" "GENERATED:FRAMEWORK_STATS_GATES_HEADING_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
| [Workflow Modes](docs/guides/WORKFLOW_MODES.md) | All $workflow_mode_count workflow modes explained |
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_DOCS_ROW_START" "GENERATED:FRAMEWORK_STATS_DOCS_ROW_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
├── agents/                    # $agent_count agent definitions
│   ├── bubbles_shared/        # Shared governance docs
│   ├── bubbles.workflow.agent.md
│   ├── bubbles.implement.agent.md
│   ├── bubbles.super.agent.md # NEW: first-touch assistant + framework operations
│   └── ...
├── prompts/                   # $agent_count prompt shims
EOF
replace_block "$repo_root/README.md" "GENERATED:FRAMEWORK_STATS_PROJECT_TREE_START" "GENERATED:FRAMEWORK_STATS_PROJECT_TREE_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
> **$summary_line**
EOF
replace_block "$repo_root/docs/CHEATSHEET.md" "GENERATED:FRAMEWORK_STATS_SUMMARY_START" "GENERATED:FRAMEWORK_STATS_SUMMARY_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
## <img src="../icons/lahey-badge.svg" width="32"> The $gate_count Gates
EOF
replace_block "$repo_root/docs/CHEATSHEET.md" "GENERATED:FRAMEWORK_STATS_CHEATSHEET_GATES_START" "GENERATED:FRAMEWORK_STATS_CHEATSHEET_GATES_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
<div class="section-title"><img src="../icons/bubbles-glasses.svg" width="24" style="vertical-align: middle; margin-right: 8px;"> The Sunnyvale Roster — All $agent_count Agents</div>
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_ROSTER_START" "GENERATED:FRAMEWORK_STATS_ROSTER_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
<!-- ═══════════════════════════════ THE $gate_count GATES ═══════════════════════════════ -->
<div class="section-title"><img src="../icons/lahey-badge.svg" width="24" style="vertical-align: middle; margin-right: 8px;"> The $gate_count Gates of Sunnyvale (Quality Never Takes a Day Off)</div>
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_HTML_GATES_START" "GENERATED:FRAMEWORK_STATS_HTML_GATES_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
  "$gate_count gates, boys. That's more security than the trailer park has ever had. 
  Something's fucky? We'll find it. That's greasy? We'll clean it. 
  Red first, green after. Small scopes. Tiny fix loops.
  Can't defer things either — you can't just NOT do things, Corey!
  Worst case Ontario, we revert and try again. DEEEE-CENT."
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_HTML_QUOTE_START" "GENERATED:FRAMEWORK_STATS_HTML_QUOTE_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
  <p>$gate_count Gates • $agent_count Agents • $workflow_mode_count Workflow Modes • $phase_count Phases • 0 Shit Hawks</p>
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_HTML_FOOTER_START" "GENERATED:FRAMEWORK_STATS_HTML_FOOTER_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
  <div class="subtitle">Agent Orchestration System v$version</div>
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_HTML_VERSION_HEADER_START" "GENERATED:FRAMEWORK_STATS_HTML_VERSION_HEADER_END" "$block_temp"

block_temp=$(mktemp)
cat <<EOF > "$block_temp"
  <p>Bubbles Agent System v$version — Sunnyvale Trailer Park Software Division</p>
EOF
replace_block "$repo_root/docs/its-not-rocket-appliances.html" "GENERATED:FRAMEWORK_STATS_HTML_VERSION_FOOTER_START" "GENERATED:FRAMEWORK_STATS_HTML_VERSION_FOOTER_END" "$block_temp"

printf '%s\n' "Updated Bubbles framework stats: $summary_line (v$version)"