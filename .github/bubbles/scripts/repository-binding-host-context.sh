#!/usr/bin/env bash
set -euo pipefail

umask 077
export LC_ALL=C

usage() {
  cat <<'EOF'
Usage: repository-binding-host-context.sh \
  --session-log <host-session-log-path> \
  --workspace-root <declared-root> [--workspace-root <declared-root> ...]

Builds host-private repository-binding context for one VS Code chat session.
The session-log path supplies identity only; its contents are never read.
Workspace roots are inventory only and never select a repository by order.

Environment:
  BUBBLES_SESSION_CONTROL_HOME  Optional absolute private base directory.
                                Intended for host integration and hermetic tests.
EOF
}

fail() {
  printf 'repository-binding-host-context: %s\n' "$1" >&2
  exit 2
}

# IMP-033 / SCOPE-7. A workspace root that is not a Git worktree is a hard,
# unbypassable refusal, and it stays one: authority resolution is the wrong
# place to become permissive. What was wrong was not the refusal but its
# uselessness — it named the problem and left the operator to reconstruct the
# invocation by hand, which in a seven-root workspace is the moment the
# operator abandons the tool and works unbound instead.
#
# So the refusal now prints the exact command with the offending root REMOVED.
# The exit code and the fail-closed behaviour are unchanged; only the operator's
# next step is now typed out rather than inferred. Nothing is auto-retried:
# dropping a declared root is a decision about which repositories the session
# has authority over, and that decision belongs to the operator.
fail_non_git_root() {
  local offending="$1"
  local remaining="" root=""
  while IFS= read -r root; do
    [[ -n "$root" ]] || continue
    [[ "$root" != "$offending" ]] || continue
    remaining="${remaining} --workspace-root $(shell_quote "$root")"
  done <<< "$workspace_roots"

  printf 'repository-binding-host-context: workspace root is not a Git worktree: %s\n' "$offending" >&2
  if [[ -z "$remaining" ]]; then
    printf '  It was the only declared root, so there is no reduced command to run.\n' >&2
    printf '  Declare a root that is a Git worktree, or run git init in %s.\n' "$offending" >&2
  else
    printf '  Re-run without it (this drops that root from the session, it does not make it usable):\n' >&2
    printf '    bash bubbles/scripts/repository-binding-host-context.sh --session-log %s%s\n' \
      "$(shell_quote "$session_log")" "$remaining" >&2
  fi
  exit 2
}

# Single-quote for the shell, so a root containing a space or a quote produces a
# command the operator can paste rather than one that silently splits.
shell_quote() {
  printf "'%s'" "${1//\'/\'\\\'\'}"
}

path_has_symlink_component() {
  local path="$1"
  local remainder
  local component
  local cursor=""

  [[ "$path" == /* ]] || return 0
  remainder="${path#/}"
  while [[ -n "$remainder" ]]; do
    if [[ "$remainder" == */* ]]; then
      component="${remainder%%/*}"
      remainder="${remainder#*/}"
    else
      component="$remainder"
      remainder=""
    fi
    # An empty component comes from a redundant separator (a trailing-slash
    # XDG_RUNTIME_DIR yields "//"); it is not a path element to inspect.
    [[ -n "$component" ]] || continue
    [[ "$component" != "." && "$component" != ".." ]] || return 0
    cursor="$cursor/$component"
    [[ ! -L "$cursor" ]] || return 0
    [[ -e "$cursor" ]] || return 1
  done
  return 1
}

physical_directory() {
  (cd -P -- "$1" 2>/dev/null && pwd -P)
}

canonical_host_path() {
  local path="$1"
  local parent
  local base
  local physical_parent

  [[ "$path" == /* && ( -e "$path" || -d "$path" ) ]] || return 1
  path_has_symlink_component "$path" && return 1
  if [[ -d "$path" ]]; then
    physical_directory "$path"
    return $?
  fi
  [[ -f "$path" && -O "$path" ]] || return 1
  parent="$(dirname "$path")"
  base="$(basename "$path")"
  physical_parent="$(physical_directory "$parent")" || return 1
  printf '%s/%s' "${physical_parent%/}" "$base"
}

canonical_git_root() {
  local candidate="$1"
  local physical_candidate
  local git_root

  physical_candidate="$(physical_directory "$candidate")" || return 1
  git_root="$(git -C "$physical_candidate" rev-parse --show-toplevel 2>/dev/null)" || return 1
  physical_directory "$git_root"
}

sha256_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    return 1
  fi
}

private_directory() {
  local directory="$1"
  local permissions

  path_has_symlink_component "$directory" && return 1
  if [[ ! -e "$directory" ]]; then
    mkdir -p "$directory" || return 1
    chmod 700 "$directory" || return 1
  fi
  [[ -d "$directory" && -O "$directory" && -w "$directory" && -x "$directory" ]] || return 1
  permissions="$(ls -ld "$directory" 2>/dev/null)" || return 1
  permissions="${permissions%% *}"
  case "$permissions" in
    drwx------*) return 0 ;;
    *) return 1 ;;
  esac
}

session_log=""
workspace_roots=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --session-log)
      [[ $# -ge 2 ]] || fail '--session-log requires a value'
      session_log="$2"
      shift 2
      ;;
    --workspace-root)
      [[ $# -ge 2 ]] || fail '--workspace-root requires a value'
      workspace_roots="${workspace_roots}${workspace_roots:+$'\n'}$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *) fail "unknown option: $1" ;;
  esac
done

[[ -n "$session_log" ]] || fail '--session-log is required'
[[ "$session_log" != *'{{VSCODE_'* ]] || fail 'VS Code session-log template was not resolved by the host'
[[ -n "$workspace_roots" ]] || fail 'at least one --workspace-root is required'
command -v git >/dev/null 2>&1 || fail 'git is required'
command -v jq >/dev/null 2>&1 || fail 'jq is required'

canonical_session_log="$(canonical_host_path "$session_log")" || \
  fail '--session-log must be an absolute, existing, caller-owned, non-symlink host path'
session_digest="$(printf '%s' "$canonical_session_log" | sha256_text)" || \
  fail 'sha256sum or shasum is required'
session_id="vscode-${session_digest:0:32}"

control_home="${BUBBLES_SESSION_CONTROL_HOME:-}"
if [[ -z "$control_home" ]]; then
  if [[ -n "${XDG_RUNTIME_DIR:-}" && "$XDG_RUNTIME_DIR" == /* && -d "$XDG_RUNTIME_DIR" && -O "$XDG_RUNTIME_DIR" ]]; then
    control_home="${XDG_RUNTIME_DIR%/}/bubbles/repository-binding"
  else
    [[ -n "${HOME:-}" && "$HOME" == /* ]] || fail 'HOME or XDG_RUNTIME_DIR is required for host-private state'
    control_home="$HOME/.local/state/bubbles/repository-binding"
  fi
fi
[[ "$control_home" == /* ]] || fail 'BUBBLES_SESSION_CONTROL_HOME must be absolute'
private_directory "$control_home" || fail 'session control home must be caller-owned, mode 0700, and free of symlinks'
session_control_dir="$control_home/$session_id"
private_directory "$session_control_dir" || fail 'session control directory must be caller-owned, mode 0700, and free of symlinks'
session_control_file="$session_control_dir/repository-binding.json"

expected_control_revision=0
if [[ -e "$session_control_file" || -L "$session_control_file" ]]; then
  [[ -f "$session_control_file" && -O "$session_control_file" && ! -L "$session_control_file" ]] || \
    fail 'session control file must be a caller-owned, non-symlink regular file'
  control_permissions="$(ls -l "$session_control_file" 2>/dev/null)" || \
    fail 'session control file permissions are unreadable'
  control_permissions="${control_permissions%% *}"
  case "$control_permissions" in
    -rw-------*) ;;
    *) fail 'session control file must have mode 0600' ;;
  esac
  expected_control_digest="$(printf '%s' "$session_control_file" | sha256_text)" || \
    fail 'sha256sum or shasum is required'
  expected_control_revision="$(jq -er \
    --arg sessionId "$session_id" \
    --arg controlPathDigest "sha256:$expected_control_digest" \
    'select(.sessionId == $sessionId)
     | select(.controlPathDigest == $controlPathDigest)
     | .revision
     | select(type == "number" and . >= 1 and floor == .)' \
    "$session_control_file" 2>/dev/null)" || \
    fail 'session control file does not match the active host session and control path'
fi

canonical_roots=""
while IFS= read -r candidate; do
  [[ -n "$candidate" ]] || continue
  [[ "$candidate" == /* ]] || fail 'workspace roots must be absolute'
  canonical_root="$(canonical_git_root "$candidate")" || fail_non_git_root "$candidate"
  if ! printf '%s\n' "$canonical_roots" | grep -Fqx -- "$canonical_root"; then
    canonical_roots="${canonical_roots}${canonical_roots:+$'\n'}$canonical_root"
  fi
done <<< "$workspace_roots"

sorted_roots="$(printf '%s\n' "$canonical_roots" | LC_ALL=C sort)"
roots_json="$(printf '%s\n' "$sorted_roots" | jq -Rsc 'split("\n") | map(select(length > 0))')"
jq -cn \
  --arg sessionId "$session_id" \
  --arg sessionControlFile "$session_control_file" \
  --arg sessionLogIdentity "sha256:$session_digest" \
  --argjson expectedControlRevision "$expected_control_revision" \
  --argjson workspaceRoots "$roots_json" \
  '{
    schemaVersion: 1,
    hostAdapter: "vscode-session-log",
    sessionId: $sessionId,
    sessionControlFile: $sessionControlFile,
    sessionLogIdentity: $sessionLogIdentity,
    expectedControlRevision: $expectedControlRevision,
    workspaceRoots: $workspaceRoots
  }'
