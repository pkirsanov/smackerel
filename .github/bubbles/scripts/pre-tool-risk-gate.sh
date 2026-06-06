#!/usr/bin/env bash
#
# pre-tool-risk-gate.sh — real-time PreToolUse risk gate (review R10, v6.1).
#
# Bubbles classifies every CLI command in bubbles/action-risk-registry.yaml.
# Until v6.1 that classification was only RECORDED (framework events) after the
# fact. This gate consumes the SAME registry and makes a BLOCK/WARN/ALLOW
# decision BEFORE a classified action runs, so destructive/external-side-effect
# actions are stopped at the door instead of audited at commit time.
#
# It is the "pre-tool" hook declared in bubbles/hooks.json and is callable by
# the CLI, the MCP server, or a host PreToolUse integration.
#
# Policy (default; override via env):
#   read_only            -> ALLOW (exit 0)
#   owned_mutation       -> ALLOW (exit 0)
#   runtime_teardown     -> WARN  (exit 0, stderr notice)
#   destructive_mutation -> BLOCK (exit 3) unless confirmed
#   external_side_effect -> BLOCK (exit 3) unless confirmed
#
# Confirmation (lets an operator/agent proceed on a blocked class):
#   BUBBLES_RISK_CONFIRM=1   — confirm this single invocation
#   --confirm                — same, as a flag
#
# Env overrides (space-separated risk-class lists):
#   BUBBLES_RISK_BLOCK="destructive_mutation external_side_effect"
#   BUBBLES_RISK_WARN="runtime_teardown"
#
# Usage:
#   pre-tool-risk-gate.sh <command-name> [command args...]
#   pre-tool-risk-gate.sh --risk-class <class>
#   pre-tool-risk-gate.sh --resolve <command-name> [args...]   # print class only
#
# Exit codes:
#   0 — allowed (or warned)
#   2 — usage / missing registry
#   3 — blocked (risk class requires confirmation)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)/.github/bubbles"
  [[ -d "$FRAMEWORK_DIR" ]] || FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
  FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
REGISTRY_FILE="${BUBBLES_ACTION_RISK_REGISTRY:-$FRAMEWORK_DIR/action-risk-registry.yaml}"

BLOCK_CLASSES="${BUBBLES_RISK_BLOCK:-destructive_mutation external_side_effect}"
WARN_CLASSES="${BUBBLES_RISK_WARN:-runtime_teardown}"

usage() {
  cat >&2 <<'USAGE'
Usage:
  pre-tool-risk-gate.sh <command-name> [command args...]
  pre-tool-risk-gate.sh --risk-class <class>
  pre-tool-risk-gate.sh --resolve <command-name> [args...]

Decides ALLOW / WARN / BLOCK for a classified Bubbles action BEFORE it runs,
using bubbles/action-risk-registry.yaml. Exit 0 allow/warn, 3 blocked, 2 usage.
Set BUBBLES_RISK_CONFIRM=1 (or pass --confirm) to proceed past a blocked class.
USAGE
}

[[ $# -lt 1 ]] && { usage; exit 2; }

CONFIRM="${BUBBLES_RISK_CONFIRM:-0}"
MODE="gate"
RISK_CLASS_DIRECT=""

# Pull out --confirm anywhere in the args.
ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirm) CONFIRM="1"; shift;;
    --risk-class) MODE="direct"; RISK_CLASS_DIRECT="${2:-}"; shift 2;;
    --resolve) MODE="resolve"; shift;;
    -h|--help) usage; exit 0;;
    *) ARGS+=("$1"); shift;;
  esac
done

# --- Resolve the effective risk class ---------------------------------------
default_risk_class() {
  local command_name="$1"
  local value=''
  if [[ -f "$REGISTRY_FILE" ]]; then
    value="$({
      awk -v cmd="$command_name" '
        $0 == "  " cmd ":" { in_cmd = 1; next }
        in_cmd && $0 ~ /^  [^[:space:]].*:/ { exit }
        in_cmd && /defaultRiskClass:/ {
          sub(/.*defaultRiskClass:[[:space:]]*/, "", $0)
          print $0
          exit
        }
      ' "$REGISTRY_FILE"
    } || true)"
  fi
  printf '%s' "${value:-read_only}"
}

effective_risk_class() {
  local command_name="$1"; shift
  local command_args="${*:-}"
  local default_risk
  default_risk="$(default_risk_class "$command_name")"
  case "$command_name" in
    doctor)
      [[ "$command_args" == *'--heal'* ]] && { printf 'owned_mutation'; return; } ;;
    hooks|framework-proposal|autofix|upgrade)
      printf 'owned_mutation'; return ;;
    audit-done|audit)
      [[ "$command_args" == *'--fix'* || "$command_args" == *'--reopen-failing'* ]] && { printf 'owned_mutation'; return; } ;;
    policy)
      [[ "$command_args" == set* || "$command_args" == reset* ]] && { printf 'owned_mutation'; return; } ;;
    runtime)
      case "${command_args%% *}" in
        release|reclaim-stale) printf 'runtime_teardown'; return ;;
        acquire|attach|heartbeat) printf 'owned_mutation'; return ;;
      esac ;;
  esac
  printf '%s' "$default_risk"
}

if [[ "$MODE" == "direct" ]]; then
  RISK_CLASS="$RISK_CLASS_DIRECT"
  TARGET="(risk-class)"
else
  [[ ${#ARGS[@]} -ge 1 ]] || { usage; exit 2; }
  CMD="${ARGS[0]}"
  REST=("${ARGS[@]:1}")
  RISK_CLASS="$(effective_risk_class "$CMD" "${REST[@]:-}")"
  TARGET="$CMD ${REST[*]:-}"
fi

if [[ "$MODE" == "resolve" ]]; then
  printf '%s\n' "$RISK_CLASS"
  exit 0
fi

# --- Decision ---------------------------------------------------------------
in_list() { local needle="$1" hay="$2" x; for x in $hay; do [[ "$x" == "$needle" ]] && return 0; done; return 1; }

if in_list "$RISK_CLASS" "$BLOCK_CLASSES"; then
  if [[ "$CONFIRM" == "1" ]]; then
    echo "pre-tool-risk-gate: ALLOW (confirmed) — '$TARGET' is $RISK_CLASS" >&2
    exit 0
  fi
  echo "pre-tool-risk-gate: BLOCK — '$TARGET' is $RISK_CLASS (destructive/external)." >&2
  echo "  This action is blocked BEFORE execution. To proceed, re-run with" >&2
  echo "  --confirm or BUBBLES_RISK_CONFIRM=1 after confirming intent." >&2
  exit 3
fi

if in_list "$RISK_CLASS" "$WARN_CLASSES"; then
  echo "pre-tool-risk-gate: WARN — '$TARGET' is $RISK_CLASS (proceeding)." >&2
  exit 0
fi

# read_only / owned_mutation / anything else -> allow silently.
exit 0
