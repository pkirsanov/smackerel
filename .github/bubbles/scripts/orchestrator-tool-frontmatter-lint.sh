#!/usr/bin/env bash
# orchestrator-tool-frontmatter-lint.sh
#
# Detects orchestrator agents whose YAML frontmatter declares a `tools:`
# allowlist that OMITS the `agent` tool. An orchestrator that restricts its
# tools but forgets `agent` cannot call `runSubagent(...)` at runtime — the
# IDE blocks the tool, the orchestrator silently degrades into a single-agent
# transcript, and the entire delegation pipeline collapses without any visible
# error.
#
# IMPORTANT — absent `tools:` is NOT a violation. When an agent declares no
# `tools:` allowlist, it inherits ALL available tools (including `agent`), so
# delegation works. Only a PRESENT allowlist missing `agent` is a real defect.
#
# An agent is considered an orchestrator when ANY of these signals are
# present in its body:
#   - `runSubagent(`            (direct delegation call)
#   - `runUntilComplete: true`  (multi-turn delegation loop)
#   - the words "delegate to", "specialist", or "invokes specialist"
#     near a runSubagent / handoff reference
# Or when its name is in the well-known orchestrator allowlist below.
#
# Frontmatter `delegationModel: none` is honored as an explicit opt-out
# (excludes the agent from the lint). This is for terminal agents that
# intentionally do not delegate (e.g. a pure executor).
#
# Args:
#   --repo-root <path>     Repo root to scan (default: script repo root)
#   --allow <agent-name>   Skip a specific agent name (repeatable)
#   --verbose              Print per-agent decisions
#
# Exit 0 when every detected orchestrator declares `agent` in tools.
# Exit 1 when one or more orchestrators are missing it.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
verbose="false"
declare -a allow_names=()

# Hardcoded orchestrator name allowlist (canonical orchestrators that
# always delegate even if the body keyword scan misses them).
declare -a known_orchestrators=(
  "bubbles.workflow"
  "bubbles.iterate"
  "bubbles.goal"
  "bubbles.sprint"
  "bubbles.harden"
  "bubbles.gaps"
  "bubbles.bug"
  "bubbles.system-review"
  "bubbles.code-review"
  "bubbles.releases"
  "bubbles.bootstrap"
  "bubbles.setup"
  "bubbles.handoff"
  "bubbles.recap"
  "bubbles.status"
  "bubbles.retro"
  "bubbles.spec-review"
  "bubbles.regression"
)

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/orchestrator-tool-frontmatter-lint.sh \
         [--repo-root <path>] [--allow <agent-name>] [--verbose]

Asserts every orchestrator agent declares `agent` in its frontmatter
`tools:` array. Exits 0 if all OK, 1 if any orchestrator is missing it.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --allow)
      shift
      allow_names+=("${1:?--allow requires an agent name}")
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
      echo "orchestrator-tool-frontmatter-lint: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

agents_dir="$repo_root/agents"
if [[ ! -d "$agents_dir" ]]; then
  echo "orchestrator-tool-frontmatter-lint: agents/ not found at $agents_dir" >&2
  exit 2
fi

# --- Helpers --------------------------------------------------------------

# Extract the YAML frontmatter block (lines between the first two `---`)
extract_frontmatter() {
  local file="$1"
  awk '
    BEGIN { state = 0 }
    /^---[[:space:]]*$/ {
      if (state == 0) { state = 1; next }
      else if (state == 1) { exit }
    }
    state == 1 { print }
  ' "$file"
}

# Extract the body (lines AFTER the second `---`)
extract_body() {
  local file="$1"
  awk '
    BEGIN { state = 0 }
    /^---[[:space:]]*$/ {
      if (state == 0) { state = 1; next }
      else if (state == 1) { state = 2; next }
    }
    state == 2 { print }
  ' "$file"
}

# Pull the agent name from a `name:` field in the body or default to file basename
agent_name_for() {
  local file="$1"
  local body_name
  body_name="$(grep -m1 -E '^\*\*Name:\*\*[[:space:]]+' "$file" 2>/dev/null \
    | sed -E 's/^\*\*Name:\*\*[[:space:]]+([^[:space:]]+).*/\1/' \
    || true)"
  if [[ -n "$body_name" ]]; then
    printf '%s\n' "$body_name"
    return 0
  fi
  basename "$file" .agent.md
}

is_in_allowlist() {
  local name="$1"
  local n
  for n in "${allow_names[@]:-}"; do
    [[ "$n" == "$name" ]] && return 0
  done
  return 1
}

is_known_orchestrator_name() {
  local name="$1"
  local n
  for n in "${known_orchestrators[@]}"; do
    [[ "$n" == "$name" ]] && return 0
  done
  return 1
}

# Decide whether a frontmatter declares `agent` in `tools:`.
# Accepts both inline list `tools: [a, b, agent]` and block list
#   tools:
#     - a
#     - agent
frontmatter_has_agent_tool() {
  local frontmatter="$1"
  # Find the tools: line and capture either inline list or following block list
  local block
  block="$(printf '%s\n' "$frontmatter" \
    | awk '
        /^tools:[[:space:]]*\[/ { print; exit }
        /^tools:[[:space:]]*$/ {
          print; capture = 1; next
        }
        capture {
          if ($0 ~ /^[[:space:]]*-/) { print; next }
          else if ($0 ~ /^[[:space:]]*$/) { next }
          else { exit }
        }
      ')"
  if [[ -z "$block" ]]; then
    return 1
  fi
  # Word-boundary match: 'agent' as a standalone token (so 'agentic' or
  # 'agent-foo' do NOT match, but '[agent,' / '- agent' / 'agent]' do).
  if printf '%s' "$block" | grep -qE '\bagent\b'; then
    return 0
  fi
  return 1
}

# True when frontmatter declares a `tools:` allowlist at all (inline
# `tools: [..]` or a block `tools:` followed by `- item` lines). Absent
# allowlist => inherits all tools => never a violation.
frontmatter_declares_tools() {
  local frontmatter="$1"
  printf '%s\n' "$frontmatter" | grep -qE '^tools:[[:space:]]*(\[|$)'
}

frontmatter_opt_out() {
  local frontmatter="$1"
  printf '%s' "$frontmatter" | grep -qE '^delegationModel:[[:space:]]*none[[:space:]]*$'
}

body_indicates_orchestration() {
  local body="$1"
  if printf '%s' "$body" | grep -qE 'runSubagent[[:space:]]*\('; then
    return 0
  fi
  if printf '%s' "$body" | grep -qE 'runUntilComplete:[[:space:]]*true'; then
    return 0
  fi
  if printf '%s' "$body" | grep -qiE 'delegate to|invokes specialist|specialist agent'; then
    return 0
  fi
  return 1
}

# --- Scan -----------------------------------------------------------------

scanned=0
orchestrators=0
missing=()
optouts=0
allowed=0

while IFS= read -r -d '' agent_file; do
  scanned=$((scanned + 1))
  rel="${agent_file#"$repo_root/"}"
  name="$(agent_name_for "$agent_file")"

  fm="$(extract_frontmatter "$agent_file")"
  body="$(extract_body "$agent_file")"

  if frontmatter_opt_out "$fm"; then
    optouts=$((optouts + 1))
    [[ "$verbose" == "true" ]] && echo "OPT-OUT (delegationModel: none): $rel"
    continue
  fi

  is_orch="false"
  reason=""
  if is_known_orchestrator_name "$name"; then
    is_orch="true"
    reason="known-orchestrator-name=$name"
  elif body_indicates_orchestration "$body"; then
    is_orch="true"
    reason="body-keyword"
  fi

  if [[ "$is_orch" != "true" ]]; then
    [[ "$verbose" == "true" ]] && echo "NON-ORCHESTRATOR: $rel"
    continue
  fi

  if is_in_allowlist "$name"; then
    allowed=$((allowed + 1))
    [[ "$verbose" == "true" ]] && echo "ALLOWED (--allow): $rel ($name)"
    continue
  fi

  orchestrators=$((orchestrators + 1))
  if ! frontmatter_declares_tools "$fm"; then
    # No tools: allowlist declared -> inherits ALL tools (including agent) ->
    # delegation works at runtime. Not a violation.
    [[ "$verbose" == "true" ]] && echo "OK (no tools: allowlist; inherits agent): $rel ($name) [$reason]"
  elif frontmatter_has_agent_tool "$fm"; then
    [[ "$verbose" == "true" ]] && echo "OK: $rel ($name) [$reason]"
  else
    missing+=("$rel|$name|$reason")
  fi
done < <(find "$agents_dir" -maxdepth 1 -name '*.agent.md' -print0 | sort -z)

# --- Report ---------------------------------------------------------------

echo "orchestrator-tool-frontmatter-lint: scanned $scanned agent file(s)"
echo "orchestrator-tool-frontmatter-lint: orchestrators detected: $orchestrators"
echo "orchestrator-tool-frontmatter-lint: opt-outs (delegationModel: none): $optouts"
echo "orchestrator-tool-frontmatter-lint: --allow exclusions: $allowed"

if [[ "${#missing[@]}" -eq 0 ]]; then
  echo "orchestrator-tool-frontmatter-lint: PASS — every orchestrator can delegate (declares 'agent' or inherits all tools)"
  exit 0
fi

echo "orchestrator-tool-frontmatter-lint: FAIL — ${#missing[@]} orchestrator(s) missing 'agent' tool"
echo "ORCHESTRATOR_MISSING_AGENT_TOOL:"
for entry in "${missing[@]}"; do
  IFS='|' read -r rel name reason <<<"$entry"
  echo "  - $rel  (name=$name, signal=$reason)"
done
echo
echo "Action: add 'agent' to the frontmatter 'tools:' list, e.g.:"
echo "  tools: [read, search, edit, agent, todo, web, execute]"
echo "Or, if the agent is genuinely terminal, add 'delegationModel: none'"
echo "to its frontmatter to opt out of this lint."
exit 1
