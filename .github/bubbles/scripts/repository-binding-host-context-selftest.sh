#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ADAPTER="$SCRIPT_DIR/repository-binding-host-context.sh"
RESOLVER="$SCRIPT_DIR/repository-binding.sh"
INSTRUCTION="$SCRIPT_DIR/../../instructions/bubbles-agents.instructions.md"
# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

passes=0
failures=0
pass() { printf 'PASS: %s\n' "$1"; passes=$((passes + 1)); }
fail() { printf 'FAIL: %s\n' "$1"; failures=$((failures + 1)); }

create_repo() {
  local root="$1"
  mkdir -p "$root/bubbles/scripts" "$root/agents" "$root/specs/001-sentinel"
  git -C "$root" init -q
  printf 'fixture\n' >"$root/VERSION"
  printf '#!/usr/bin/env bash\n' >"$root/install.sh"
  printf '#!/usr/bin/env bash\n' >"$root/bubbles/scripts/cli.sh"
}

mkdir -p "$TMP_ROOT/session-logs/chat-a" "$TMP_ROOT/session-logs/chat-b" \
  "$TMP_ROOT/control-home" "$TMP_ROOT/xdg-runtime" "$TMP_ROOT/home"
chmod 700 "$TMP_ROOT/control-home"
chmod 700 "$TMP_ROOT/xdg-runtime"
create_repo "$TMP_ROOT/repo-a"
create_repo "$TMP_ROOT/repo-b"

context_a="$(BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
  --session-log "$TMP_ROOT/session-logs/chat-a" \
  --workspace-root "$TMP_ROOT/repo-b" --workspace-root "$TMP_ROOT/repo-a")"
context_a_reordered="$(cd "$TMP_ROOT/repo-b" && BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
  --session-log "$TMP_ROOT/session-logs/chat-a" \
  --workspace-root "$TMP_ROOT/repo-a" --workspace-root "$TMP_ROOT/repo-b")"
context_b="$(BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
  --session-log "$TMP_ROOT/session-logs/chat-b" \
  --workspace-root "$TMP_ROOT/repo-a" --workspace-root "$TMP_ROOT/repo-b")"

xdg_context=""
if xdg_context="$(BUBBLES_SESSION_CONTROL_HOME='' XDG_RUNTIME_DIR="$TMP_ROOT/xdg-runtime/" HOME="$TMP_ROOT/home" \
    bash "$ADAPTER" --session-log "$TMP_ROOT/session-logs/chat-a" \
    --workspace-root "$TMP_ROOT/repo-a" 2>/dev/null)"; then
  xdg_control_file="$(jq -r '.sessionControlFile' <<< "$xdg_context")"
  if [[ "$xdg_control_file" == "$TMP_ROOT/xdg-runtime/bubbles/repository-binding/"* && \
        "$xdg_control_file" != *//* ]]; then
    pass 'trailing-slash XDG runtime produces a normalized private control path'
  else
    fail 'trailing-slash XDG runtime must not produce an empty path component'
  fi
else
  fail 'trailing-slash XDG runtime must remain a valid private control base'
fi

if [[ "$(jq -r '.expectedControlRevision' <<< "$context_a")" == "0" ]]; then
  pass 'new chat context supplies expected control revision zero'
else
  fail 'new chat context must supply expected control revision zero'
fi

if [[ "$(jq -r '.sessionId' <<< "$context_a")" == "$(jq -r '.sessionId' <<< "$context_a_reordered")" ]]; then
  pass 'same chat keeps one session id across CWD and workspace-root order'
else
  fail 'same chat must keep one stable session id'
fi
if [[ "$(jq -c '.workspaceRoots' <<< "$context_a")" == "$(jq -c '.workspaceRoots' <<< "$context_a_reordered")" ]]; then
  pass 'workspace inventory is canonical and order-independent'
else
  fail 'workspace inventory must be order-independent'
fi
if [[ "$(jq -r '.sessionId' <<< "$context_a")" != "$(jq -r '.sessionId' <<< "$context_b")" ]]; then
  pass 'different chats receive isolated session ids'
else
  fail 'different chats must not share session ids'
fi
control_file="$(jq -r '.sessionControlFile' <<< "$context_a")"
control_parent="$(dirname "$control_file")"
# Match the tolerant glob the adapter itself uses: macOS suffixes the mode field
# with '@' (xattr) or '+' (ACL).
if [[ "$control_file" == "$TMP_ROOT/control-home/"* && "$(ls -ld "$control_parent" | awk '{print $1}')" == drwx------* ]]; then
  pass 'control path is external and its immediate parent is mode 0700'
else
  fail 'control path must be external under a private parent'
fi

session_id="$(jq -r '.sessionId' <<< "$context_a")"
expected_revision="$(jq -r '.expectedControlRevision' <<< "$context_a")"
workspace_args=()
while IFS= read -r root; do
  workspace_args+=(--workspace-root "$root")
done < <(jq -r '.workspaceRoots[]' <<< "$context_a")
first_output="$(bash "$RESOLVER" preflight --session-id "$session_id" \
  --session-control-file "$control_file" --expected-control-revision "$expected_revision" \
  --request-class TARGETLESS_MODE --repository-root "$TMP_ROOT/repo-a" \
  "${workspace_args[@]}")"
context_a_continued="$(BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
  --session-log "$TMP_ROOT/session-logs/chat-a" \
  --workspace-root "$TMP_ROOT/repo-a" --workspace-root "$TMP_ROOT/repo-b")"
continued_revision="$(jq -r '.expectedControlRevision' <<< "$context_a_continued")"
if [[ "$continued_revision" == "1" ]]; then
  pass 'continued chat context supplies the current control revision'
else
  fail 'continued chat context must supply the current control revision'
fi
second_output="$(cd "$TMP_ROOT/repo-b" && bash "$RESOLVER" preflight \
  --session-id "$session_id" --session-control-file "$control_file" \
  --expected-control-revision "$continued_revision" --request-class TARGETLESS_MODE \
  "${workspace_args[@]}")"
if [[ "$first_output" == *'PREFLIGHT_COMMITTED'* && "$second_output" == *"root=$TMP_ROOT/repo-a"* && "$second_output" == *'source=session-work-boundary'* ]]; then
  pass 'targetless follow-up continues the first explicitly bound repository'
else
  fail 'targetless follow-up must continue the durable repository boundary'
fi

ln -s "$TMP_ROOT/session-logs/chat-a" "$TMP_ROOT/session-logs/chat-link"
if BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
    --session-log "$TMP_ROOT/session-logs/chat-link" --workspace-root "$TMP_ROOT/repo-a" >/dev/null 2>&1; then
  fail 'symlinked session-log path must refuse'
else
  pass 'symlinked session-log path refuses'
fi
if BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home" bash "$ADAPTER" \
    --session-log '{{VSCODE_TARGET_SESSION_LOG}}' --workspace-root "$TMP_ROOT/repo-a" >/dev/null 2>&1; then
  fail 'unresolved VS Code template must refuse'
else
  pass 'unresolved VS Code template refuses'
fi
if grep -Fq '{{VSCODE_TARGET_SESSION_LOG}}' "$INSTRUCTION" && \
   grep -Fq 'repository-binding-host-context.sh' "$INSTRUCTION"; then
  pass 'auto-loaded agent instruction supplies the VS Code host adapter contract'
else
  fail 'agent instruction must supply the VS Code host adapter contract'
fi
if grep -Fq 'expectedControlRevision' "$INSTRUCTION" && \
   grep -Fq -- '--expected-control-revision' "$INSTRUCTION"; then
  pass 'auto-loaded agent instruction passes the observed control revision into preflight'
else
  fail 'agent instruction must pass expectedControlRevision into preflight'
fi

printf 'repository-binding-host-context-selftest: %s passed, %s failed\n' "$passes" "$failures"
[[ "$failures" -eq 0 ]]
