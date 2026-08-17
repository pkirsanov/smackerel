#!/usr/bin/env bash
set -euo pipefail

# orchestrator-persistence-lint.sh
#
# Gate G086 — orchestrator_persistence_lint_gate.
#
# Scans the 4 orchestrator prompt files for user-reprompt language that
# would make continuation depend on a fresh user prompt. Orchestrators
# must auto-continue non-terminal phases. At a terminal stop, they must
# invoke recap and return control instead of silently selecting new work.
#
# Usage:
#   bash bubbles/scripts/orchestrator-persistence-lint.sh [--quiet] [--root <repo>]
#
# Exit codes:
#   0  clean
#   1  one or more G086 findings
#   2  missing/unreadable target file, invalid arguments, or unresolved repo root

QUIET="false"
ROOT_FLAG=""

PERSISTENCE_TARGET_FILES=(
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.sprint.agent.md"
)

REQUIRED_RECAP_TARGET_FILES=(
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.sprint.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.bug.agent.md"
  "agents/bubbles.releases.agent.md"
  "agents/bubbles.train.agent.md"
  "agents/bubbles.upkeep.agent.md"
  "agents/bubbles.propagate.agent.md"
  "agents/bubbles.stabilize.agent.md"
  "agents/bubbles.retro.agent.md"
  "agents/bubbles.journey.agent.md"
  "agents/bubbles.super.agent.md"
  "agents/bubbles.code-review.agent.md"
  "agents/bubbles.system-review.agent.md"
)

RECAP_TARGET_FILES=()

array_contains() {
  local needle="$1"
  shift
  local value
  for value in "$@"; do
    [[ "$value" == "$needle" ]] && return 0
  done
  return 1
}

append_recap_target() {
  local candidate="$1"
  local existing
  for existing in "${RECAP_TARGET_FILES[@]}"; do
    [[ "$existing" == "$candidate" ]] && return 0
  done
  RECAP_TARGET_FILES+=("$candidate")
}

resolve_repo_file() {
  local rel="$1"
  local candidate

  for candidate in "$REPO_ROOT/$rel" "$REPO_ROOT/.github/$rel"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done

  return 1
}

FORBIDDEN_PHRASES=(
  "should i continue"
  "shall i proceed"
  "do you want me to"
  "would you like me to continue"
  "ask the user before continuing"
)

PERSISTENCE_REQUIRED_MARKERS=(
  "g086"
  "automatically continue"
  "non-terminal"
  "bubbles/scripts/continuation-intent-resolve.sh"
  "convergence achieved"
  "max iterations"
  "user requests stop"
  "fundamental impossibility"
)

RECAP_REQUIRED_MARKERS=(
  "terminal recap boundary"
  "runSubagent(bubbles.recap)"
)

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/orchestrator-persistence-lint.sh [--quiet] [--root <repo>]

Optional:
  --root <repo>  Bubbles repo root. Defaults to $BUBBLES_REPO_ROOT or
                 auto-detection by walking upward to .specify/memory.
  --quiet        Suppress success output.
  -h, --help     Print this usage and exit.

Exit codes:
  0 = clean
  1 = G086 findings
  2 = missing/unreadable target files, invalid arguments, or unresolved root
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "orchestrator-persistence-lint: --root requires a path argument" >&2
        usage >&2
        exit 2
      fi
      ROOT_FLAG="$1"
      shift
      ;;
    --root=*)
      ROOT_FLAG="${1#--root=}"
      shift
      ;;
    --*)
      echo "orchestrator-persistence-lint: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "orchestrator-persistence-lint: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "orchestrator-persistence-lint: $*"
  fi
}

resolve_repo_root() {
  if [[ -n "$ROOT_FLAG" ]]; then
    printf '%s' "$ROOT_FLAG"
    return 0
  fi
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  echo "orchestrator-persistence-lint: unable to resolve repo root" >&2
  echo "  Pass --root <repo>, set BUBBLES_REPO_ROOT, or run from inside a Bubbles repo." >&2
  exit 2
fi

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "orchestrator-persistence-lint: repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi

CLASSIFIER_FILE="$(resolve_repo_file "bubbles/scripts/continuation-intent-resolve.sh" || true)"
SHARED_CONTRACT_FILE="$(resolve_repo_file "agents/bubbles_shared/agent-common.md" || true)"
RECAP_AGENT_FILE="$(resolve_repo_file "agents/bubbles.recap.agent.md" || true)"
if [[ -z "$CLASSIFIER_FILE" || ! -x "$CLASSIFIER_FILE" ]]; then
  echo "orchestrator-persistence-lint: missing/non-executable continuation classifier" >&2
  exit 2
fi
if [[ -z "$SHARED_CONTRACT_FILE" || -z "$RECAP_AGENT_FILE" ]]; then
  echo "orchestrator-persistence-lint: missing shared completion contract or recap agent" >&2
  exit 2
fi

CAPABILITIES_FILE="$(resolve_repo_file "bubbles/agent-capabilities.yaml" || true)"
if [[ -z "$CAPABILITIES_FILE" ]]; then
  echo "orchestrator-persistence-lint: missing workflow grant registry: bubbles/agent-capabilities.yaml" >&2
  exit 2
fi

while IFS= read -r runner_file; do
  [[ -n "$runner_file" ]] || continue
  append_recap_target "$runner_file"
done < <(awk '
  /^workflowModeGrants:[[:space:]]*$/ { in_grants = 1; next }
  in_grants && /^  agents:[[:space:]]*$/ { in_agents = 1; next }
  in_agents && /^    bubbles\.[a-z0-9-]+:[[:space:]]*$/ {
    name = $1
    sub(/:$/, "", name)
    print "agents/" name ".agent.md"
    next
  }
  in_agents && /^[^[:space:]]/ { exit }
' "$CAPABILITIES_FILE")

while IFS= read -r agent_file; do
  [[ -n "$agent_file" ]] || continue
  append_recap_target "$agent_file"
done < <(awk '
  /^terminalRecapPolicy:[[:space:]]*$/ { in_policy = 1; next }
  in_policy && /^  additionalAgents:[[:space:]]*$/ { in_agents = 1; next }
  in_agents && /^  - bubbles\.[a-z0-9-]+[[:space:]]*$/ {
    name = $2
    print "agents/" name ".agent.md"
    next
  }
  in_policy && /^[^[:space:]]/ { exit }
' "$CAPABILITIES_FILE")

if [[ "${#RECAP_TARGET_FILES[@]}" -eq 0 ]]; then
  echo "orchestrator-persistence-lint: no terminal recap agents resolved from workflowModeGrants or terminalRecapPolicy" >&2
  exit 2
fi

finding_count=0
for required_recap_file in "${REQUIRED_RECAP_TARGET_FILES[@]}"; do
  if ! array_contains "$required_recap_file" "${RECAP_TARGET_FILES[@]}"; then
    echo "G086 orchestrator_persistence_lint_gate violation: required recap agent absent from registry policy: $required_recap_file" >&2
    finding_count=$((finding_count + 1))
  fi
done
for configured_recap_file in "${RECAP_TARGET_FILES[@]}"; do
  if ! array_contains "$configured_recap_file" "${REQUIRED_RECAP_TARGET_FILES[@]}"; then
    echo "G086 orchestrator_persistence_lint_gate violation: unreviewed recap agent added to registry policy: $configured_recap_file" >&2
    finding_count=$((finding_count + 1))
  fi
done

missing_count=0
for rel in "${RECAP_TARGET_FILES[@]}"; do
  path="$(resolve_repo_file "$rel" || true)"
  if [[ -z "$path" ]]; then
    echo "orchestrator-persistence-lint: missing target file: $rel" >&2
    missing_count=$((missing_count + 1))
  elif [[ ! -r "$path" ]]; then
    echo "orchestrator-persistence-lint: unreadable target file: $rel" >&2
    missing_count=$((missing_count + 1))
  fi
done

if [[ "$missing_count" -gt 0 ]]; then
  echo "orchestrator-persistence-lint: missing/unreadable target files block Gate G086" >&2
  exit 2
fi

if ! grep -qF 'directInvocationOnly: true' "$CAPABILITIES_FILE"; then
  echo "G086 orchestrator_persistence_lint_gate violation: terminalRecapPolicy.directInvocationOnly must be true" >&2
  finding_count=$((finding_count + 1))
fi
if ! grep -qF 'phaseOwnersReturnUpward: true' "$CAPABILITIES_FILE"; then
  echo "G086 orchestrator_persistence_lint_gate violation: terminalRecapPolicy.phaseOwnersReturnUpward must be true" >&2
  finding_count=$((finding_count + 1))
fi
if ! grep -qF 'A dispatched phase-owner subagent MUST NOT invoke recap' "$SHARED_CONTRACT_FILE"; then
  echo "G086 orchestrator_persistence_lint_gate violation: shared phase-owner upward-return contract missing" >&2
  finding_count=$((finding_count + 1))
fi
if grep -qF 'runSubagent(bubbles.recap)' "$RECAP_AGENT_FILE"; then
  echo "G086 orchestrator_persistence_lint_gate violation: bubbles.recap must not recursively dispatch itself" >&2
  finding_count=$((finding_count + 1))
fi
# shellcheck disable=SC2016 # Literal Markdown backticks, not shell expansion.
if ! grep -qF '`bubbles.recap` never invokes itself' "$RECAP_AGENT_FILE"; then
  echo "G086 orchestrator_persistence_lint_gate violation: recap non-recursion contract missing" >&2
  finding_count=$((finding_count + 1))
fi

scan_forbidden_phrases() {
  local rel="$1"
  local path="$2"
  local line
  local lower_line
  local line_no=0
  local in_fence="false"
  local fence_marked="false"
  local in_marked_block="false"
  local previous_nonempty_marked="false"

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_no=$((line_no + 1))
    lower_line="${line,,}"
    same_line_marked="false"
    if [[ "$lower_line" == *"forbidden example"* ]]; then
      same_line_marked="true"
    fi

    if [[ "$line" == *'```'* ]]; then
      if [[ "$in_fence" == "false" ]]; then
        in_fence="true"
        if [[ "$same_line_marked" == "true" || "$in_marked_block" == "true" || "$previous_nonempty_marked" == "true" ]]; then
          fence_marked="true"
        else
          fence_marked="false"
        fi
      else
        in_fence="false"
        fence_marked="false"
      fi
      if [[ -n "${line//[[:space:]]/}" ]]; then
        previous_nonempty_marked="$same_line_marked"
      fi
      continue
    fi

    if [[ "$in_fence" == "false" && -z "${line//[[:space:]]/}" ]]; then
      in_marked_block="false"
      previous_nonempty_marked="false"
      continue
    fi

    allowed_context="false"
    if [[ "$same_line_marked" == "true" || "$in_marked_block" == "true" || ( "$in_fence" == "true" && "$fence_marked" == "true" ) ]]; then
      allowed_context="true"
    fi

    for phrase in "${FORBIDDEN_PHRASES[@]}"; do
      if [[ "$lower_line" == *"$phrase"* && "$allowed_context" != "true" ]]; then
        echo "G086 orchestrator_persistence_lint_gate violation: $rel:$line_no contains forbidden phrase '$phrase'" >&2
        finding_count=$((finding_count + 1))
      fi
    done

    if [[ "$allowed_context" != "true" && "$lower_line" != *"do not"* && "$lower_line" != *"refuse"* && "$lower_line" == *"continue"* && ( "$lower_line" == *"type: implement"* || "$lower_line" == *"create new scope"* ) ]]; then
      echo "G086 orchestrator_persistence_lint_gate violation: $rel:$line_no maps continuation to new implementation work" >&2
      finding_count=$((finding_count + 1))
    fi

    if [[ "$same_line_marked" == "true" && "$in_fence" == "false" ]]; then
      in_marked_block="true"
    fi
    if [[ -n "${line//[[:space:]]/}" ]]; then
      previous_nonempty_marked="$same_line_marked"
    fi
  done < "$path"
}

first_line_with() {
  local path="$1"
  local marker="$2"
  awk -v marker="$marker" 'index($0, marker) { print NR; exit }' "$path"
}

first_exact_line() {
  local path="$1"
  local marker="$2"
  awk -v marker="$marker" '$0 == marker { print NR; exit }' "$path"
}

check_classifier_order() {
  local rel="$1"
  local path="$2"
  local parser_marker="$3"
  local classifier_line parser_line
  classifier_line="$(first_line_with "$path" 'bubbles/scripts/continuation-intent-resolve.sh')"
  parser_line="$(first_exact_line "$path" "$parser_marker")"
  if [[ -z "$classifier_line" || -z "$parser_line" || "$classifier_line" -ge "$parser_line" ]]; then
    echo "G086 orchestrator_persistence_lint_gate violation: $rel must classify continuation before '$parser_marker'" >&2
    finding_count=$((finding_count + 1))
  fi
}

check_required_language() {
  local rel="$1"
  local path="$2"
  local marker

  for marker in "${PERSISTENCE_REQUIRED_MARKERS[@]}"; do
    if ! grep -qiF "$marker" "$path"; then
      echo "G086 orchestrator_persistence_lint_gate violation: $rel missing required persistence-default marker '$marker'" >&2
      finding_count=$((finding_count + 1))
    fi
  done
}

check_terminal_recap_language() {
  local rel="$1"
  local path="$2"
  local marker

  for marker in "${RECAP_REQUIRED_MARKERS[@]}"; do
    if ! grep -qiF "$marker" "$path"; then
      echo "G086 orchestrator_persistence_lint_gate violation: $rel missing required terminal recap marker '$marker'" >&2
      finding_count=$((finding_count + 1))
    fi
  done
}

for rel in "${PERSISTENCE_TARGET_FILES[@]}"; do
  path="$(resolve_repo_file "$rel" || true)"
  scan_forbidden_phrases "$rel" "$path"
  check_required_language "$rel" "$path"
  case "$rel" in
    agents/bubbles.goal.agent.md) check_classifier_order "$rel" "$path" 'phase_1_understand:' ;;
    agents/bubbles.workflow.agent.md) check_classifier_order "$rel" "$path" '### Phase 0: Resolve Inputs' ;;
    agents/bubbles.iterate.agent.md) check_classifier_order "$rel" "$path" '## Scope Selection Priority' ;;
    agents/bubbles.sprint.agent.md) check_classifier_order "$rel" "$path" 'phase_1_parse_and_estimate:' ;;
  esac
done

for rel in "${RECAP_TARGET_FILES[@]}"; do
  path="$(resolve_repo_file "$rel" || true)"
  check_terminal_recap_language "$rel" "$path"
done

if [[ "$finding_count" -gt 0 ]]; then
  echo "G086 orchestrator_persistence_lint_gate blocked: findings=$finding_count root=$REPO_ROOT" >&2
  exit 1
fi

info "persistenceFiles=${#PERSISTENCE_TARGET_FILES[@]} recapFiles=${#RECAP_TARGET_FILES[@]} findings=0 root=$REPO_ROOT"
if [[ "$QUIET" != "true" ]]; then
  echo "PASS Gate G086 (orchestrator_persistence_lint_gate) — persistenceFiles=${#PERSISTENCE_TARGET_FILES[@]}, recapFiles=${#RECAP_TARGET_FILES[@]}, findings=0"
fi
exit 0