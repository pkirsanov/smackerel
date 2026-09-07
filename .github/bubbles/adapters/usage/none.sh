#!/usr/bin/env bash
# bubbles/adapters/usage/none.sh — no-op host-usage adapter.
#
# DEFAULT adapter (IMP-039 SCOPE-2). A repository that has not configured a
# usage provider resolves here, so every consumer runs unchanged and the
# framework NEVER gains a dependency on a host telemetry artifact.
#
# THE HONESTY RULE THAT DEFINES THIS ADAPTER
# There is no estimation path. Prompt-token counts and credit figures are only
# knowable from a host-written record. Deriving them from message lengths,
# closure bytes, or dispatch counts is fabrication, which is the mistake
# IMP-028 was closed for. With no adapter configured, the honest answer is
# `unmeasured`, and every consumer MUST render exactly that rather than a zero.
#
# A zero and an unmeasured value are NOT the same claim. `status` therefore
# reports `measured: false` explicitly so a consumer can tell them apart without
# inferring anything from an empty array.
#
# Canonical per-verb shapes (validated by usage-adapter-contract-selftest.sh):
#   requests [sessionId]  → JSON array → neutral empty value: []
#   session  [sessionId]  → JSON map   → neutral empty value: {}
#   status                → JSON map   → {"measured":false,"adapter":"none",...}
#   capabilities          → JSON map   → neutral empty value: {}
#
# A per-request record emitted by a CONFIGURED adapter carries:
#   requestId, at, model, promptTokens, completionTokens, credits,
#   toolResultBytes, compactionCheckpoints
#
# `status` is the ONE verb that does not return the bare neutral map, because
# "no data" must be distinguishable from "measured zero". Everything else is
# neutral-empty.

set -euo pipefail

VERB="${1:-}"

V2_CAPABILITIES='[{"dimension":"modelRequestCount","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"inputTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"outputTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"cacheWriteTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"cacheReadTokens","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"providerCredits","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"monetaryMinorUnits","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"subagentDispatches","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"webCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"browserCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"toolCalls","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"retainedResultBytes","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"wallTimeMs","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"retries","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false},{"dimension":"concurrency","mode":"unsupported","postDispatchActual":false,"preDispatchBound":false}]'

v2_unavailable() {
  printf '%s\n' '{"code":"MBE-USAGE-UNAVAILABLE","contractType":"measured-budget-error","errorClass":"usage","message":"usage adapter none has no host measurement source","schemaVersion":1}' >&2
  exit 3
}

v2_input() {
  [[ -n "${1:-}" && -f "$1" ]] || { echo "[none][ERROR] v2 verb requires an input JSON file" >&2; exit 2; }
  command -v jq >/dev/null 2>&1 || { echo "[none][ERROR] jq is required for v2 record construction" >&2; exit 2; }
}

v2_dispatch() {
  local operation="${1:-}" input_file="${2:-}"
  case "$operation" in
    describe)
      printf '{"adapterId":"none","capabilities":%s,"contractType":"usage-adapter-description","hostSchemaIds":[],"mappingDigest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","schemaVersion":2,"sessionIdentity":"unscoped","supportedMajors":[1,2]}\n' "$V2_CAPABILITIES"
      ;;
    identify-session|quote) v2_unavailable ;;
    snapshot)
      v2_input "$input_file"
      jq -cS '{adapterId:"none",contractType:"usage-snapshot",measurement:[],observedAt:.observedAt,previousCursor:.previousCursor,schemaVersion:2,sessionIdentityId:.sessionIdentityId,snapshotId:.snapshotId,cursor:.cursor}' "$input_file"
      ;;
    receipt)
      v2_input "$input_file"
      jq -cS '{adapterContractVersion:2,adapterId:"none",contractType:"usage-receipt",finishedAt:.finishedAt,hostSchemaId:"none",intentId:.intentId,measurement:[],measurementStatus:"unmeasured",permitId:.permitId,providerReceiptDigest:"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",schemaVersion:2,startedAt:.startedAt,usageReceiptId:.usageReceiptId}' "$input_file"
      ;;
    verify-receipt)
      v2_input "$input_file"
      jq -cS '{contractType:"usage-receipt-verification",proofDigest:"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",schemaVersion:2,usageReceiptId:.usageReceiptId,verdict:"unsupported",verifiedAt:.verifiedAt}' "$input_file"
      ;;
    *) echo "[none][ERROR] unknown v2 verb '$operation'" >&2; exit 2 ;;
  esac
}

if [[ "$VERB" == "v2" ]]; then
  v2_dispatch "${2:-}" "${3:-}"
  exit 0
fi

case "$VERB" in
  requests)
    echo '[]'
    exit 0
    ;;
  session)
    echo '{}'
    exit 0
    ;;
  status)
    printf '%s\n' '{"measured":false,"adapter":"none","reason":"no usage adapter configured"}'
    exit 0
    ;;
  capabilities)
    # Declares nothing: it supports every verb neutrally. A consumer MUST read
    # an empty declaration as "no restrictions claimed".
    echo '{}'
    exit 0
    ;;
  describe|identify-session|quote|snapshot|receipt|verify-receipt)
    v2_dispatch "$VERB" "${2:-}"
    exit 0
    ;;
  selftest)
    case "${2:-}" in
      requests) echo '[]'; exit 0 ;;
      session | capabilities) echo '{}'; exit 0 ;;
      status) printf '%s\n' '{"measured":false,"adapter":"none","reason":"no usage adapter configured"}'; exit 0 ;;
      *) echo "[none][ERROR] selftest requires a known verb" >&2; exit 1 ;;
    esac
    ;;
  -h | --help | "")
    cat >&2 <<'EOF'
none.sh — no-op host-usage adapter (framework default)
Usage: none.sh <verb> [args...]
Verbs: requests [sessionId] (-> []) | session [sessionId] (-> {}) |
       status (-> {"measured":false,...}) | capabilities (-> {}) |
  selftest <verb> (-> canonical neutral shape) |
  v2 <describe|identify-session|quote|snapshot|receipt|verify-receipt> [input.json]
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
