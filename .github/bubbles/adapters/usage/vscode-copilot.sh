#!/usr/bin/env bash
# bubbles/adapters/usage/vscode-copilot.sh — reference host-usage adapter.
#
# Reads per-request usage records the VS Code Copilot Chat host writes to disk
# (IMP-039 SCOPE-2). It MEASURES; it never estimates.
#
# WHY A REFERENCE ADAPTER EXISTS
# `bubbles/workflows.yaml` excluded `tokenCount` because the VS Code Copilot
# API does not expose it. That is true of the API and incomplete about the host:
# the host also writes the numbers to a file. This adapter reads that file. It
# is the only sanctioned route from a real token count into a Bubbles surface.
#
# SCHEMA OWNERSHIP — READ BEFORE TRUSTING THIS
# The artifact and its field names are HOST-OWNED and versioned by the host, not
# by this framework. This adapter is written against the documented shape:
#
#   <workspaceStorage>/<workspace-id>/chatSessions/<session-id>.jsonl
#   per completed request: promptTokens, completionTokens, copilotCredits,
#                          modelId, promptTokenDetails
#
# When no exact artifact exists, or one exact stable artifact carries no
# request-like usage object, this adapter returns neutral-empty records. Unsafe,
# unstable, unreadable, malformed, or mixed exact input fails loud instead of
# becoming a valid-looking subset. It does NOT fall back to a derived number,
# because a derived number is the failure this scope exists to prevent. A
# remote/SSH/WSL server install is a normal case of "absent": the records live
# on the CLIENT machine, so point
# BUBBLES_USAGE_VSCODE_ROOT at the client-side workspaceStorage or accept
# `unmeasured`.
#
# Configuration:
#   BUBBLES_USAGE_VSCODE_ROOT   explicit workspaceStorage directory. Required
#                               for remote installs; otherwise the standard
#                               per-platform locations are searched.
#
# Verbs and shapes are identical to none.sh, so a consumer never branches on
# which adapter answered — only on `status.measured`.

set -euo pipefail

VERB="${1:-}"
SESSION_FILTER="${2:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_IO_HELPER="$SCRIPT_DIR/../../scripts/session-state-io.py"
PYTHON_BIN="$(command -v python3 2>/dev/null || true)"

V2_HOST_SCHEMA='vscode-copilot-chat-session-v1'
V2_MAPPING_DIGEST='sha256:3b939f2f461b2d4f436b0f95f5769551ac4bd0572dc32c38eb2434e5db781d7e'
V2_CAPABILITIES='[{"dimension":"modelRequestCount","mode":"trusted-derived","postDispatchActual":true,"preDispatchBound":false},{"dimension":"inputTokens","mode":"native","postDispatchActual":true,"preDispatchBound":false},{"dimension":"outputTokens","mode":"native","postDispatchActual":true,"preDispatchBound":false},{"dimension":"cacheWriteTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"cacheReadTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"providerCredits","mode":"native","postDispatchActual":true,"preDispatchBound":false},{"dimension":"monetaryMinorUnits","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"subagentDispatches","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"webCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"browserCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"toolCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"retainedResultBytes","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"wallTimeMs","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"retries","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"concurrency","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false}]'

v2_fail() { printf '{"code":"%s","message":"%s"}\n' "$1" "$2" >&2; exit "${3:-3}"; }
v2_input() {
  [[ -n "${1:-}" && -f "$1" ]] || v2_fail "MBE-USAGE-INPUT" "v2 verb requires an input JSON file" 2
  require_jq || exit 2
}

v2_dispatch() {
  local operation="${1:-}" input_file="${2:-}"
  case "$operation" in
    describe)
      printf '{"adapterId":"vscode-copilot","capabilities":%s,"contractType":"usage-adapter-description","hostSchemaIds":["%s"],"mappingDigest":"%s","schemaVersion":2,"sessionIdentity":"exact","supportedMajors":[1,2]}\n' "$V2_CAPABILITIES" "$V2_HOST_SCHEMA" "$V2_MAPPING_DIGEST"
      ;;
    identify-session)
      v2_input "$input_file"
      [[ "$(jq -r '.hostSchemaId // ""' "$input_file")" == "$V2_HOST_SCHEMA" ]] || v2_fail "MBE-HOST-SCHEMA-UNSUPPORTED" "explicit host schema profile is unsupported"
      artifact="$(jq -r '.artifactPath // ""' "$input_file")"
      [[ -n "$artifact" && -f "$artifact" ]] || v2_fail "MBE-SESSION-IDENTITY-UNRESOLVED" "exact host artifact is unavailable"
      [[ "$(jq -r '.hostSessionId // ""' "$input_file")" != "" ]] || v2_fail "MBE-SESSION-IDENTITY-UNRESOLVED" "hostSessionId is required"
      matches="$(jq -s --arg sid "$(jq -r '.hostSessionId' "$input_file")" '[.[] | select(.hostSessionId == $sid)] | length' "$artifact" 2>/dev/null || echo 0)"
      [[ "$matches" == "1" ]] || v2_fail "MBE-SESSION-IDENTITY-UNRESOLVED" "exactly one host session record is required"
      jq -cS --arg adapter "vscode-copilot" '{adapterId:$adapter,artifactSessionId:.artifactSessionId,contractType:"host-session-identity",hostInstanceId:.hostInstanceId,hostSchemaId:.hostSchemaId,hostSessionId:.hostSessionId,proofDigest:.proofDigest,repositoryDecisionId:.repositoryDecisionId,schemaVersion:2,sessionIdentityId:.sessionIdentityId,startedAt:.startedAt,workspaceIdentity:.workspaceIdentity}' "$input_file"
      ;;
    quote) v2_fail "MBE-USAGE-QUOTE-UNSUPPORTED" "VS Code artifact does not provide a trusted pre-dispatch bound" ;;
    snapshot|receipt|verify-receipt)
      v2_fail "MBE-USAGE-RECEIPT-UNSUPPORTED" "exact provider receipt correlation is unavailable from this host schema"
      ;;
    *) v2_fail "MBE-USAGE-VERB" "unknown v2 verb" 2 ;;
  esac
}

emit_unmeasured_status() {
  printf '{"measured":false,"adapter":"vscode-copilot","reason":"%s"}\n' "$1"
}

declare -a USAGE_ROOTS=()
if [[ -n "${BUBBLES_USAGE_VSCODE_ROOT:-}" ]]; then
  USAGE_ROOTS+=("$BUBBLES_USAGE_VSCODE_ROOT")
else
  USAGE_ROOTS+=(
    "$HOME/Library/Application Support/Code/User/workspaceStorage"
    "$HOME/Library/Application Support/Code - Insiders/User/workspaceStorage"
    "$HOME/.config/Code/User/workspaceStorage"
    "$HOME/.config/Code - Insiders/User/workspaceStorage"
    "$HOME/AppData/Roaming/Code/User/workspaceStorage"
  )
fi

require_usage_reader() {
  if [[ -z "$PYTHON_BIN" || ! -f "$STATE_IO_HELPER" || -L "$STATE_IO_HELPER" ]]; then
    echo "[vscode-copilot][ERROR] safe usage reader is unavailable" >&2
    return 1
  fi
}

read_usage() {
  local projection="$1"
  local requested="${2:-}"
  local root
  local -a command=(
    "$PYTHON_BIN"
    "$STATE_IO_HELPER"
    parse-usage
    --projection "$projection"
  )
  if [[ -n "$requested" ]]; then
    command+=(--session-id "$requested")
  fi
  for root in "${USAGE_ROOTS[@]}"; do
    command+=(--root "$root")
  done
  "${command[@]}"
}

emit_usage_or_error() {
  local projection="$1"
  local requested="${2:-}"
  local output rc
  set +e
  output="$(read_usage "$projection" "$requested")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    echo "[vscode-copilot][ERROR] exact usage input is unsafe or invalid" >&2
    return 2
  fi
  printf '%s\n' "$output"
}

if [[ "$VERB" == "v2" ]]; then
  v2_dispatch "${2:-}" "${3:-}"
  exit 0
fi

case "$VERB" in
  requests)
    [[ -n "$SESSION_FILTER" ]] || { echo '[]'; exit 0; }
    require_usage_reader || exit 2
    emit_usage_or_error requests "$SESSION_FILTER" || exit $?
    exit 0
    ;;
  session)
    [[ -n "$SESSION_FILTER" ]] || { echo '{}'; exit 0; }
    require_usage_reader || exit 2
    emit_usage_or_error session "$SESSION_FILTER" || exit $?
    exit 0
    ;;
  status)
    require_usage_reader || { emit_unmeasured_status "safe usage reader is unavailable"; exit 0; }
    emit_usage_or_error status || exit $?
    exit 0
    ;;
  capabilities)
    printf '%s\n' '{"requests":"native","session":"derived","toolResultBytes":"unsupported","compactionCheckpoints":"unsupported"}'
    exit 0
    ;;
  describe|identify-session|quote|snapshot|receipt|verify-receipt)
    v2_dispatch "$VERB" "${2:-}"
    exit 0
    ;;
  selftest)
    case "${2:-}" in
      requests) echo '[]'; exit 0 ;;
      session) echo '{}'; exit 0 ;;
      capabilities) printf '%s\n' '{"requests":"native","session":"derived","toolResultBytes":"unsupported","compactionCheckpoints":"unsupported"}'; exit 0 ;;
      status) emit_unmeasured_status "selftest"; exit 0 ;;
      *) echo "[vscode-copilot][ERROR] selftest requires a known verb" >&2; exit 1 ;;
    esac
    ;;
  -h | --help | "")
    cat >&2 <<'EOF'
vscode-copilot.sh — reference host-usage adapter (reads VS Code chatSessions)
Usage: vscode-copilot.sh <verb> [sessionId]
Verbs: requests [sessionId] | session [sessionId] | status | capabilities |
  selftest <verb> | v2 <describe|identify-session|quote|snapshot|receipt|verify-receipt> [input.json]
Env:   BUBBLES_USAGE_VSCODE_ROOT — workspaceStorage dir (required for remote installs)
EOF
    exit 0
    ;;
  *)
    echo "[vscode-copilot][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
