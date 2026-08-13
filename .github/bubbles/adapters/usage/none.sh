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
       selftest <verb> (-> canonical neutral shape)
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
