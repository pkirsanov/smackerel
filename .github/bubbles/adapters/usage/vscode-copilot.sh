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
# When the artifact is absent, unreadable, or carries none of those fields, this
# adapter reports `measured: false` and returns neutral-empty records. It does
# NOT fall back to a derived number, because a derived number is the failure
# this scope exists to prevent. A remote/SSH/WSL server install is a normal case
# of "absent": the records live on the CLIENT machine, so point
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

emit_unmeasured_status() {
  printf '{"measured":false,"adapter":"vscode-copilot","reason":"%s"}\n' "$1"
}

# --- locate candidate session files -----------------------------------------
# Ordered most-specific first. An explicit root wins outright: on a remote
# install the local guesses are all wrong and silently searching them would turn
# a configuration mistake into a false "unmeasured".
usage_roots() {
  if [[ -n "${BUBBLES_USAGE_VSCODE_ROOT:-}" ]]; then
    printf '%s\n' "$BUBBLES_USAGE_VSCODE_ROOT"
    return 0
  fi
  printf '%s\n' \
    "$HOME/Library/Application Support/Code/User/workspaceStorage" \
    "$HOME/Library/Application Support/Code - Insiders/User/workspaceStorage" \
    "$HOME/.config/Code/User/workspaceStorage" \
    "$HOME/.config/Code - Insiders/User/workspaceStorage" \
    "$HOME/AppData/Roaming/Code/User/workspaceStorage"
}

collect_files() {
  local root
  while IFS= read -r root; do
    [[ -d "$root" ]] || continue
    if [[ -n "$SESSION_FILTER" ]]; then
      find "$root" -path '*/chatSessions/*' -name "${SESSION_FILTER}*" -type f 2>/dev/null || true
    else
      find "$root" -path '*/chatSessions/*' -type f 2>/dev/null || true
    fi
  done < <(usage_roots)
}

# One record per completed request. `..|objects` walks the document rather than
# assuming a nesting depth, so a host layout change relocating the records does
# not silently zero the report; only a rename of promptTokens does, and that is
# reported as unmeasured rather than as zero.
JQ_NORMALIZE='
  [ .. | objects | select(has("promptTokens")) ]
  | map({
      requestId:             (.requestId // .id // null),
      at:                    (.timestamp // .requestTime // null),
      model:                 (.modelId // .model // null),
      promptTokens:          (.promptTokens // null),
      completionTokens:      (.completionTokens // null),
      credits:               (.copilotCredits // .credits // null),
      toolResultBytes:       (.toolResultBytes // null),
      compactionCheckpoints: (.compactionCheckpoints // null)
    })
'

read_records() {
  local files
  files="$(collect_files)"
  [[ -n "$files" ]] || return 1
  # -s slurps: a .jsonl yields one input per line, a .json yields one input.
  # Both then reduce to the same array through the recursive walk above.
  printf '%s\n' "$files" | tr '\n' '\0' |
    xargs -0 jq -s "$JQ_NORMALIZE" 2>/dev/null || return 1
}

require_jq() {
  command -v jq >/dev/null 2>&1 || {
    echo "[vscode-copilot][ERROR] jq is required" >&2
    return 1
  }
}

case "$VERB" in
  requests)
    require_jq || { echo '[]'; exit 0; }
    records="$(read_records || true)"
    [[ -n "$records" ]] || records='[]'
    printf '%s\n' "$records"
    exit 0
    ;;
  session)
    require_jq || { echo '{}'; exit 0; }
    records="$(read_records || true)"
    [[ -n "$records" ]] || { echo '{}'; exit 0; }
    # A record set with no usable promptTokens is NOT a measured zero.
    printf '%s' "$records" | jq '
      map(select(.promptTokens != null)) as $m
      | if ($m | length) == 0 then {}
        else {
          requests:         ($m | length),
          promptTokens:     ($m | map(.promptTokens) | add),
          completionTokens: ($m | map(.completionTokens // 0) | add),
          credits:          ($m | map(.credits // 0) | add),
          maxPromptTokens:  ($m | map(.promptTokens) | max),
          models:           ($m | map(.model) | map(select(. != null)) | unique)
        }
        end'
    exit 0
    ;;
  status)
    require_jq || { emit_unmeasured_status "jq is not installed"; exit 0; }
    files="$(collect_files)"
    if [[ -z "$files" ]]; then
      emit_unmeasured_status "no chatSessions artifact found; set BUBBLES_USAGE_VSCODE_ROOT"
      exit 0
    fi
    records="$(read_records || true)"
    count="$(printf '%s' "${records:-[]}" | jq '[.[] | select(.promptTokens != null)] | length' 2>/dev/null || echo 0)"
    if [[ "${count:-0}" -eq 0 ]]; then
      emit_unmeasured_status "artifact found but carries no promptTokens field"
      exit 0
    fi
    file_count="$(printf '%s\n' "$files" | grep -c '' || echo 0)"
    printf '{"measured":true,"adapter":"vscode-copilot","files":%s,"records":%s}\n' \
      "$file_count" "$count"
    exit 0
    ;;
  capabilities)
    printf '%s\n' '{"requests":"native","session":"derived","toolResultBytes":"unsupported","compactionCheckpoints":"unsupported"}'
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
       selftest <verb>
Env:   BUBBLES_USAGE_VSCODE_ROOT — workspaceStorage dir (required for remote installs)
EOF
    exit 0
    ;;
  *)
    echo "[vscode-copilot][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
