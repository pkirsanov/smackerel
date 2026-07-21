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
TRUST_REGISTRY="${BUBBLES_TOOL_TRUST_REGISTRY:-$FRAMEWORK_DIR/tool-trust-registry.yaml}"

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

# Structured-event (IMP-020 S3) fields.
EV_SERVER=""
EV_OPERATION=""
EV_TOOL=""
EV_TARGET=""
EV_DATA_CLASSES=""
EV_EGRESS="none"
EV_APPROVAL_FILE=""

# Pull out --confirm anywhere in the args.
ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirm) CONFIRM="1"; shift;;
    --risk-class) MODE="direct"; RISK_CLASS_DIRECT="${2:-}"; shift 2;;
    --resolve) MODE="resolve"; shift;;
    --event) MODE="event"; shift;;
    --server) MODE="event"; EV_SERVER="${2:-}"; shift 2;;
    --operation) MODE="event"; EV_OPERATION="${2:-}"; shift 2;;
    --tool) EV_TOOL="${2:-}"; shift 2;;
    --target) EV_TARGET="${2:-}"; shift 2;;
    --data-classes) EV_DATA_CLASSES="${2:-}"; shift 2;;
    --egress) EV_EGRESS="${2:-none}"; shift 2;;
    --approval-file) EV_APPROVAL_FILE="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) ARGS+=("$1"); shift;;
  esac
done

# --- Structured tool/server/operation event decision (IMP-020 S3 / AF-005) ---
# Consumes a structured event + the tool-trust registry for a FAIL-CLOSED
# decision. Unknown servers and unknown operations do NOT default to read_only.
# Sensitive classes and any approvalRequired op need an ACTION-BOUND, host-
# verified approval; a generic --confirm/env can never satisfy them. When the
# host cannot enforce (hostEnforceable=false) a sensitive decision is
# 'enforcement=unavailable' and BLOCKED — never silently passed.
trust_srv_field() {
  awk -v s="$1" -v f="$2" '
    $0 ~ "^  " s ":[[:space:]]*$" {inx=1; next}
    inx && /^  [^[:space:]]/ {inx=0}
    inx && /^    operations:/ {ops=1}
    inx && !ops && $0 ~ "^    " f ":" {sub(/^[^:]*:[[:space:]]*/,""); print; exit}
  ' "$TRUST_REGISTRY" 2>/dev/null
}
trust_op_field() {
  awk -v s="$1" -v o="$2" -v f="$3" '
    $0 ~ "^  " s ":[[:space:]]*$" {inx=1; next}
    inx && /^  [^[:space:]]/ && $0 !~ "^  " s ":" {inx=0}
    inx && /^    operations:/ {ops=1; next}
    inx && ops && $0 ~ "^      " o ":" {
      line=$0
      if (f=="permittedDataClasses") { if (match(line, /permittedDataClasses:[[:space:]]*\[[^]]*\]/)) {v=substr(line,RSTART,RLENGTH); sub(/^[^[]*\[/,"",v); sub(/\].*/,"",v); gsub(/[[:space:]]/,"",v); print v}; exit }
      if (match(line, f":[[:space:]]*[^,}]+")) {v=substr(line,RSTART+length(f)+1); sub(/^[[:space:]]*/,"",v); sub(/[[:space:]]*[,}].*/,"",v); print v}
      exit
    }
  ' "$TRUST_REGISTRY" 2>/dev/null
}
trust_default() {
  awk -v a="$1" -v b="$2" '
    $0 ~ "^defaults:" {ind=1; next}
    ind && $0 ~ "^  " a ":" {inb=1; next}
    inb && /^  [^[:space:]]/ {inb=0}
    inb && $0 ~ "^    " b ":" {sub(/^[^:]*:[[:space:]]*/,""); print; exit}
  ' "$TRUST_REGISTRY" 2>/dev/null
}
trust_request_hash() {
  printf '%s|%s|%s|%s|%s|%s' "$EV_TOOL" "$EV_SERVER" "$EV_OPERATION" "$EV_TARGET" "$EV_DATA_CLASSES" "$EV_EGRESS" \
    | { if command -v shasum >/dev/null 2>&1; then shasum -a 256; else sha256sum; fi; } | awk '{print $1}'
}

event_decision() {
  [[ -n "$EV_SERVER" && -n "$EV_OPERATION" ]] || { echo "pre-tool-risk-gate: --event requires --server and --operation" >&2; exit 2; }
  [[ -f "$TRUST_REGISTRY" ]] || { echo "pre-tool-risk-gate: BLOCK — tool-trust registry missing ($TRUST_REGISTRY); failing closed." >&2; echo "decision=block reason=registry-missing enforcement=unavailable"; exit 3; }

  local label="$EV_SERVER/$EV_OPERATION"
  local trust_state host_enforceable risk egress permitted approval_required
  trust_state="$(trust_srv_field "$EV_SERVER" trustState)"

  # 1. Unregistered server -> fail closed (default-deny).
  if [[ -z "$trust_state" ]]; then
    echo "pre-tool-risk-gate: BLOCK — '$label' server is UNREGISTERED (default-deny)." >&2
    echo "  Register it in tool-trust-registry.yaml (source/trustState/operations) before use." >&2
    echo "decision=block reason=unregistered-server enforcement=unavailable riskClass=external_side_effect"
    exit 3
  fi

  host_enforceable="$(trust_srv_field "$EV_SERVER" hostEnforceable)"
  risk="$(trust_op_field "$EV_SERVER" "$EV_OPERATION" riskClass)"
  egress="$(trust_op_field "$EV_SERVER" "$EV_OPERATION" egress)"
  permitted="$(trust_op_field "$EV_SERVER" "$EV_OPERATION" permittedDataClasses)"
  approval_required="$(trust_op_field "$EV_SERVER" "$EV_OPERATION" approvalRequired)"

  # 2. Unknown operation on a known server -> fail closed to the sensitive default.
  if [[ -z "$risk" ]]; then
    risk="$(trust_default unknownOperation riskClass)"; [[ -n "$risk" ]] || risk="external_side_effect"
    approval_required="true"
    egress="${egress:-external}"
    permitted=""
  fi

  # 3. Data-class check: every event data class must be permitted; secret never egresses.
  if [[ -n "$EV_DATA_CLASSES" ]]; then
    local dc
    for dc in ${EV_DATA_CLASSES//,/ }; do
      if [[ "$dc" == "secret" && "$egress" != "none" ]]; then
        echo "pre-tool-risk-gate: BLOCK — '$label' would move 'secret' data via egress '$egress'." >&2
        echo "decision=block reason=secret-egress enforcement=enforced riskClass=$risk"; exit 3
      fi
      if [[ ",$permitted," != *",$dc,"* ]]; then
        echo "pre-tool-risk-gate: BLOCK — '$label' not permitted for data class '$dc' (permitted: ${permitted:-none})." >&2
        echo "decision=block reason=data-class-violation enforcement=enforced riskClass=$risk"; exit 3
      fi
    done
  fi

  # 4. Egress check: event egress must not exceed the declared operation egress.
  if [[ "$EV_EGRESS" == "external" && "$egress" != "external" ]]; then
    echo "pre-tool-risk-gate: BLOCK — '$label' attempted external egress but declares '$egress'." >&2
    echo "decision=block reason=undeclared-egress enforcement=enforced riskClass=$risk"; exit 3
  fi

  # 5. Sensitive class or approvalRequired -> action-bound host-verified approval.
  local sensitive="false"
  case "$risk" in destructive_mutation|external_side_effect) sensitive="true";; esac
  [[ "$approval_required" == "true" ]] && sensitive="true"

  if [[ "$sensitive" == "true" ]]; then
    if [[ "$host_enforceable" != "true" ]]; then
      echo "pre-tool-risk-gate: BLOCK — '$label' is sensitive ($risk) and the host cannot enforce approval (hostEnforceable=false)." >&2
      echo "  Ambient/non-routed tools are not interceptable by Bubbles; least privilege + untrusted-content policy apply." >&2
      echo "decision=block reason=sensitive-host-unenforceable enforcement=unavailable riskClass=$risk"; exit 3
    fi
    if [[ -z "$EV_APPROVAL_FILE" || ! -f "$EV_APPROVAL_FILE" ]]; then
      echo "pre-tool-risk-gate: BLOCK — '$label' is sensitive ($risk); needs an action-bound approval (--approval-file). --confirm does NOT apply." >&2
      echo "decision=block reason=approval-required enforcement=enforced riskClass=$risk"; exit 3
    fi
    local a_hostverified a_hash a_expiry now computed
    a_hostverified="$(sed -nE 's/^hostVerified=(.*)$/\1/p' "$EV_APPROVAL_FILE" | head -n1)"
    a_hash="$(sed -nE 's/^requestHash=(.*)$/\1/p' "$EV_APPROVAL_FILE" | head -n1)"
    a_expiry="$(sed -nE 's/^expiry=(.*)$/\1/p' "$EV_APPROVAL_FILE" | head -n1)"
    computed="$(trust_request_hash)"
    now="$(date +%s)"
    if [[ "$a_hostverified" != "true" ]]; then
      echo "pre-tool-risk-gate: BLOCK — approval is not host-verified (hostVerified!=true); Bubbles never simulates human approval." >&2
      echo "decision=block reason=approval-not-host-verified enforcement=enforced riskClass=$risk"; exit 3
    fi
    if [[ "$a_hash" != "$computed" ]]; then
      echo "pre-tool-risk-gate: BLOCK — approval requestHash does not bind this action (replay/mismatch)." >&2
      echo "decision=block reason=approval-hash-mismatch enforcement=enforced riskClass=$risk"; exit 3
    fi
    if [[ ! "$a_expiry" =~ ^[0-9]+$ || "$a_expiry" -le "$now" ]]; then
      echo "pre-tool-risk-gate: BLOCK — approval is expired or has no valid expiry." >&2
      echo "decision=block reason=approval-expired enforcement=enforced riskClass=$risk"; exit 3
    fi
    echo "pre-tool-risk-gate: ALLOW — '$label' authorized by valid action-bound approval." >&2
    echo "decision=allow reason=approved enforcement=enforced riskClass=$risk"; exit 0
  fi

  # 6. Non-sensitive: runtime_teardown warns; read_only/owned_mutation allow.
  if [[ "$risk" == "runtime_teardown" ]]; then
    echo "pre-tool-risk-gate: WARN — '$label' is runtime_teardown (proceeding)." >&2
    echo "decision=warn reason=runtime-teardown enforcement=enforced riskClass=$risk"; exit 0
  fi
  echo "decision=allow reason=non-sensitive enforcement=enforced riskClass=$risk"; exit 0
}

if [[ "$MODE" == "event" ]]; then
  event_decision
fi

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
