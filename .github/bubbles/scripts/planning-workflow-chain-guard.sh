#!/usr/bin/env bash
set -euo pipefail

# planning-workflow-chain-guard.sh
#
# Gate G091 - planning_workflow_chain_gate.
#
# Ensures delivery-capable planning, bootstrap, and fallback paths preserve
# the canonical planning chain:
#   bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan
#
# Usage:
#   bash bubbles/scripts/planning-workflow-chain-guard.sh [--root <repoRoot>] [--quiet]
#
# Exit codes:
#   0  clean
#   1  one or more G091 findings
#   2  missing/malformed inputs, invalid arguments, or unresolved repo root

QUIET="false"
ROOT_FLAG=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

REQUIRED_CHAIN_TEXT="bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan"
REQUIRED_AGENTS=("bubbles.analyst" "bubbles.ux" "bubbles.design" "bubbles.plan")

REQUIRED_PROMPT_FILES=(
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.sprint.agent.md"
  "agents/bubbles_shared/workflow-orchestration-core.md"
  "agents/bubbles_shared/workflow-input-bootstrap.md"
)

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

resolve_workflows_file() {
  resolve_repo_file "bubbles/workflows.yaml"
}

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/planning-workflow-chain-guard.sh [--root <repoRoot>] [--quiet]

Optional:
  --root <repoRoot>       Bubbles repo root. Defaults to $BUBBLES_REPO_ROOT
                          or the root inferred from this script path.
  --repo-root <repoRoot>  Alias for --root.
  --quiet                 Suppress success output.
  -h, --help              Print this usage and exit.

Exit codes:
  0 = clean
  1 = G091 planning workflow chain findings
  2 = missing/malformed workflow or prompt input, invalid args, unresolved root
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
    --root|--repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "planning-workflow-chain-guard: --root requires a path argument" >&2
        usage >&2
        exit 2
      fi
      ROOT_FLAG="$1"
      shift
      ;;
    --root=*|--repo-root=*)
      ROOT_FLAG="${1#*=}"
      shift
      ;;
    --*)
      echo "planning-workflow-chain-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "planning-workflow-chain-guard: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "planning-workflow-chain-guard: $*"
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
  if [[ "$(basename "$SCRIPT_DIR")" == "scripts" && "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" ]]; then
    if [[ "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
      (cd "$SCRIPT_DIR/../../.." && pwd)
      return 0
    fi
    (cd "$SCRIPT_DIR/../.." && pwd)
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" && ( -f "$dir/bubbles/workflows.yaml" || -f "$dir/.github/bubbles/workflows.yaml" ) ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  echo "planning-workflow-chain-guard: unable to resolve repo root" >&2
  echo "  Pass --root <repoRoot>, set BUBBLES_REPO_ROOT, or run from inside a Bubbles repo." >&2
  exit 2
fi

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "planning-workflow-chain-guard: repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi

WORKFLOWS="$(resolve_workflows_file || true)"
if [[ -z "$WORKFLOWS" ]]; then
  echo "planning-workflow-chain-guard: missing workflows file: bubbles/workflows.yaml or .github/bubbles/workflows.yaml" >&2
  exit 2
fi
if [[ ! -r "$WORKFLOWS" ]]; then
  echo "planning-workflow-chain-guard: unreadable workflows file: $WORKFLOWS" >&2
  exit 2
fi

FINDING_COUNT=0
DELIVERY_CAPABLE_MODES=0
BOOTSTRAP_CHAINS_CHECKED=0
PROMPT_FILES_SCANNED=0
CHAIN_REASON=""

finding() {
  echo "G091 planning_workflow_chain_gate violation: $*" >&2
  FINDING_COUNT=$((FINDING_COUNT + 1))
}

lower() {
  local value="$1"
  printf '%s' "${value,,}"
}

index_of() {
  local text="$1"
  local needle="$2"
  local prefix
  prefix="${text%%"$needle"*}"
  if [[ "$prefix" == "$text" ]]; then
    printf '%s' "-1"
  else
    printf '%s' "${#prefix}"
  fi
}

chain_order_ok() {
  local raw_text="$1"
  local text
  text="$(lower "$raw_text")"
  local analyst ux design plan
  analyst="$(index_of "$text" "bubbles.analyst")"
  ux="$(index_of "$text" "bubbles.ux")"
  design="$(index_of "$text" "bubbles.design")"
  plan="$(index_of "$text" "bubbles.plan")"

  local missing=()
  [[ "$analyst" -ge 0 ]] || missing+=("bubbles.analyst")
  [[ "$ux" -ge 0 ]] || missing+=("bubbles.ux")
  [[ "$design" -ge 0 ]] || missing+=("bubbles.design")
  [[ "$plan" -ge 0 ]] || missing+=("bubbles.plan")

  if [[ "${#missing[@]}" -gt 0 ]]; then
    CHAIN_REASON="missing ${missing[*]}"
    return 1
  fi
  if [[ "$analyst" -gt "$ux" ]]; then
    CHAIN_REASON="bubbles.analyst must run before bubbles.ux"
    return 1
  fi
  if [[ "$ux" -gt "$design" ]]; then
    CHAIN_REASON="bubbles.ux must run before bubbles.design"
    return 1
  fi
  if [[ "$design" -gt "$plan" ]]; then
    CHAIN_REASON="bubbles.design must run before bubbles.plan"
    return 1
  fi
  CHAIN_REASON=""
  return 0
}

require_ordered_chain() {
  local label="$1"
  local text="$2"
  if chain_order_ok "$text"; then
    return 0
  fi
  finding "$label must include ordered canonical planning chain ($REQUIRED_CHAIN_TEXT); $CHAIN_REASON; text='$text'"
  return 1
}

extract_yaml_block() {
  local anchor_regex="$1"
  local next_regex="$2"
  awk -v anchor="$anchor_regex" -v next_pattern="$next_regex" '
    $0 ~ anchor { in_block = 1; next }
    in_block && $0 ~ next_pattern { exit }
    in_block { print }
  ' "$WORKFLOWS"
}

check_delivery_quality_constraints() {
  local block
  block="$(extract_yaml_block '^  delivery-quality-constraints:[[:space:]]*$' '^  [A-Za-z0-9_-]+:[[:space:]]*$')"
  if [[ -z "$block" ]]; then
    finding "modeTemplates.delivery-quality-constraints block is missing"
    return 0
  fi
  if ! grep -qE 'requireCanonicalPlanningChain:[[:space:]]*true' <<<"$block"; then
    finding "modeTemplates.delivery-quality-constraints missing requireCanonicalPlanningChain: true"
  fi
  local chain_line
  chain_line="$(grep -E 'planningChainAgents:' <<<"$block" || true)"
  if [[ -z "$chain_line" ]]; then
    finding "modeTemplates.delivery-quality-constraints missing planningChainAgents canonical chain"
  else
    require_ordered_chain "modeTemplates.delivery-quality-constraints.planningChainAgents" "$chain_line" || true
  fi
}

extract_inline_action() {
  local action_key="$1"
  awk -v key="$action_key" '
    $0 ~ "^[[:space:]]{4}" key ":[[:space:]]*$" { in_action = 1; next }
    in_action && $0 ~ /^[[:space:]]{4}[A-Za-z0-9_-]+:[[:space:]]*$/ { exit }
    in_action && $0 ~ /^[[:space:]]{6}action:/ {
      sub(/^[[:space:]]*action:[[:space:]]*/, "", $0)
      print
      exit
    }
  ' "$WORKFLOWS"
}

check_inline_actions() {
  local key action lower_action
  for key in missingArtifacts missingDesignBeforeImplement artifactFreshnessDrift; do
    action="$(extract_inline_action "$key")"
    if [[ -z "$action" ]]; then
      finding "autoEscalation.inlineActions.$key missing action text"
      continue
    fi
    if [[ "$key" == "artifactFreshnessDrift" ]]; then
      lower_action="$(lower "$action")"
      if [[ "$lower_action" == *"and/or"* ]]; then
        finding "autoEscalation.inlineActions.artifactFreshnessDrift uses discretionary 'and/or' repair text"
      fi
    fi
    require_ordered_chain "autoEscalation.inlineActions.$key" "$action" || true
  done
}

is_exception_mode() {
  case "$1" in
    docs-only|validate-only|spec-review-only|spec-review-to-doc)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

block_has_ordered_chain() {
  local block_file="$1"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if chain_order_ok "$line"; then
      return 0
    fi
  done < "$block_file"
  return 1
}

check_mode_block() {
  local mode="$1"
  local block_file="$2"
  local block
  block="$(cat "$block_file")"
  local lower_block
  lower_block="$(lower "$block")"

  local delivery_capable="false"
  if [[ "$lower_block" == *"statusceiling: done"* || "$lower_block" == *"delivery-quality-constraints"* || "$lower_block" == *"allowbootstrapiterations: true"* || "$lower_block" == *"bootstraploopexit: design_spec_scopes_ready"* || "$lower_block" == *"planningtruthmutation: true"* || "$lower_block" == *"bugpacketmutation: true"* ]]; then
    delivery_capable="true"
    DELIVERY_CAPABLE_MODES=$((DELIVERY_CAPABLE_MODES + 1))
  fi

  if is_exception_mode "$mode"; then
    if ! grep -qE 'planningTruthMutation:[[:space:]]*false' "$block_file"; then
      finding "modes.$mode is a machine-readable planning-chain exception but lacks planningTruthMutation: false"
    fi
    if grep -qE 'planningTruthMutation:[[:space:]]*true' "$block_file"; then
      finding "modes.$mode declares planningTruthMutation: true; docs-only/validate-only/spec-review exceptions may skip the chain only when planningTruthMutation is false"
    fi
  fi

  if grep -qE 'planningTruthMutation:[[:space:]]*true|bugPacketMutation:[[:space:]]*true|createPlanningArtifacts:[[:space:]]*true|repairPlanningArtifacts:[[:space:]]*true' "$block_file"; then
    if ! block_has_ordered_chain "$block_file"; then
      finding "modes.$mode mutates planning truth but lacks an ordered canonical planning chain ($REQUIRED_CHAIN_TEXT)"
    fi
  fi

  if grep -qE 'bootstrapAgents:' "$block_file"; then
    BOOTSTRAP_CHAINS_CHECKED=$((BOOTSTRAP_CHAINS_CHECKED + 1))
    while IFS= read -r line || [[ -n "$line" ]]; do
      if [[ "$(lower "$block")" == *"usecanonicalplanningchain: true"* ]]; then
        continue
      fi
      require_ordered_chain "modes.$mode.bootstrapAgents" "$line" || true
    done < <(grep -E 'bootstrapAgents:' "$block_file" || true)
  fi

  if grep -qE 'improvementPreludeProfiles:' "$block_file"; then
    while IFS= read -r profile_line || [[ -n "$profile_line" ]]; do
      local profile_name
      profile_name="$(sed -E 's/^[[:space:]]*([^:]+):.*/\1/' <<<"$profile_line")"
      require_ordered_chain "modes.$mode.improvementPreludeProfiles.$profile_name" "$profile_line" || true
    done < <(awk '
      /improvementPreludeProfiles:/ { in_profiles = 1; next }
      in_profiles && /^[[:space:]]{8}[A-Za-z0-9_-]+:/ { print }
      in_profiles && /^[[:space:]]{0,7}[A-Za-z0-9_-]+:/ { in_profiles = 0 }
    ' "$block_file")
  fi

  if [[ "$delivery_capable" == "true" && "$lower_block" == *"bootstrapagents:"* && "$lower_block" == *"bubbles.design"* && "$lower_block" == *"bubbles.plan"* ]]; then
    :
  fi
}

check_modes() {
  local mode_dir
  mode_dir="$(mktemp -d -t bubbles-g091-modes-XXXXXXXX)"
  trap 'rm -rf "$mode_dir" 2>/dev/null || true' RETURN
  awk -v out="$mode_dir" '
    function safe_name(value) { gsub(/[^A-Za-z0-9_.-]/, "_", value); return value }
    /^modes:[[:space:]]*$/ { in_modes = 1; next }
    in_modes && /^[A-Za-z0-9_-]+:/ { if (file != "") close(file); exit }
    in_modes && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ {
      mode = $1
      sub(/^  /, "", mode)
      sub(/:$/, "", mode)
      file = out "/" safe_name(mode) ".block"
      print > file
      next
    }
    in_modes && file != "" { print >> file }
  ' "$WORKFLOWS"

  local mode_file mode
  for mode_file in "$mode_dir"/*.block; do
    [[ -e "$mode_file" ]] || continue
    mode="$(basename "$mode_file" .block)"
    check_mode_block "$mode" "$mode_file"
  done
  rm -rf "$mode_dir"
  trap - RETURN
}

line_is_marked_forbidden() {
  local lower_line="$1"
  [[ "$lower_line" == *"forbidden"* ]]
}

line_has_active_shortcut() {
  local lower_line="$1"
  if [[ "$lower_line" == *"bubbles.design"* && "$lower_line" == *"bubbles.plan"* ]]; then
    if ! [[ "$lower_line" == *"bubbles.analyst"* && "$lower_line" == *"bubbles.ux"* ]]; then
      return 0
    fi
  fi
  if [[ "$lower_line" == *"design -> plan"* || "$lower_line" == *"design → plan"* || "$lower_line" == *"design + plan"* || "$lower_line" == *"[design, plan]"* || "$lower_line" == *"[bubbles.design, bubbles.plan]"* ]]; then
    if ! [[ "$lower_line" == *"analyst"* && "$lower_line" == *"ux"* ]]; then
      return 0
    fi
  fi
  if [[ "$lower_line" == *"design and plan"* && ( "$lower_line" == *"invoke"* || "$lower_line" == *"route"* || "$lower_line" == *"fallback"* || "$lower_line" == *"bootstrap"* || "$lower_line" == *"auto-escalation"* ) ]]; then
    if ! [[ "$lower_line" == *"analyst"* && "$lower_line" == *"ux"* ]]; then
      return 0
    fi
  fi
  if [[ "$lower_line" == *"analyst"* && "$lower_line" == *"design"* && "$lower_line" == *"plan"* && "$lower_line" != *"ux"* ]]; then
    if [[ "$lower_line" == *"analyze-design-plan"* || "$lower_line" == *"planning prelude"* || "$lower_line" == *"planning chain"* || "$lower_line" == *"run analyst"* ]]; then
      return 0
    fi
  fi
  return 1
}

scan_prompt_file() {
  local rel="$1"
  local path
  path="$(resolve_repo_file "$rel" || true)"
  if [[ -z "$path" ]]; then
    finding "required prompt/shared-doc file missing: $rel"
    return 0
  fi
  if [[ ! -r "$path" ]]; then
    finding "required prompt/shared-doc file unreadable: $rel"
    return 0
  fi

  PROMPT_FILES_SCANNED=$((PROMPT_FILES_SCANNED + 1))
  local line lower_line line_no=0 in_fence="false" fence_marked="false" previous_nonempty_marked="false"
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_no=$((line_no + 1))
    lower_line="$(lower "$line")"
    local same_line_marked="false"
    if line_is_marked_forbidden "$lower_line"; then
      same_line_marked="true"
    fi

    if [[ "$line" == *'```'* ]]; then
      if [[ "$in_fence" == "false" ]]; then
        in_fence="true"
        if [[ "$same_line_marked" == "true" || "$previous_nonempty_marked" == "true" ]]; then
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

    if [[ "$in_fence" == "true" && "$fence_marked" == "true" ]]; then
      continue
    fi

    if line_has_active_shortcut "$lower_line" && [[ "$same_line_marked" != "true" ]]; then
      finding "$rel:$line_no contains active design-to-plan shortcut without FORBIDDEN marker; required chain is $REQUIRED_CHAIN_TEXT; line='$line'"
    fi

    if [[ -n "${line//[[:space:]]/}" ]]; then
      previous_nonempty_marked="$same_line_marked"
    else
      previous_nonempty_marked="false"
    fi
  done < "$path"
}

collect_prompt_files() {
  local rel
  for rel in "${REQUIRED_PROMPT_FILES[@]}"; do
    printf '%s\n' "$rel"
  done
  if [[ -d "$REPO_ROOT/prompts" ]]; then
    while IFS= read -r prompt_path; do
      rel="${prompt_path#"$REPO_ROOT/"}"
      printf '%s\n' "$rel"
    done < <(find "$REPO_ROOT/prompts" -type f -name 'bubbles.*.prompt.md' | sort)
  fi
  if [[ -d "$REPO_ROOT/.github/prompts" ]]; then
    while IFS= read -r prompt_path; do
      rel="${prompt_path#"$REPO_ROOT/"}"
      printf '%s\n' "$rel"
    done < <(find "$REPO_ROOT/.github/prompts" -type f -name 'bubbles.*.prompt.md' | sort)
  fi
}

check_prompt_shortcuts() {
  local seen_file
  seen_file="$(mktemp -t bubbles-g091-prompts-XXXXXXXX)"
  trap 'rm -f "$seen_file" 2>/dev/null || true' RETURN
  while IFS= read -r rel || [[ -n "$rel" ]]; do
    [[ -n "$rel" ]] || continue
    if grep -qxF "$rel" "$seen_file" 2>/dev/null; then
      continue
    fi
    printf '%s\n' "$rel" >> "$seen_file"
    scan_prompt_file "$rel"
  done < <(collect_prompt_files)
  rm -f "$seen_file"
  trap - RETURN
}

check_delivery_quality_constraints
check_inline_actions
check_modes
check_prompt_shortcuts

if [[ "$FINDING_COUNT" -gt 0 ]]; then
  echo "G091 planning_workflow_chain_gate blocked: findings=$FINDING_COUNT root=$REPO_ROOT requiredChain='$REQUIRED_CHAIN_TEXT'" >&2
  exit 1
fi

info "deliveryCapableModes=$DELIVERY_CAPABLE_MODES bootstrapChainsChecked=$BOOTSTRAP_CHAINS_CHECKED promptFilesScanned=$PROMPT_FILES_SCANNED root=$REPO_ROOT"
if [[ "$QUIET" != "true" ]]; then
  echo "PASS Gate G091 (planning_workflow_chain_gate) - ordered planning chain valid: $REQUIRED_CHAIN_TEXT"
fi
exit 0