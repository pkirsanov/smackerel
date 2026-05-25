#!/usr/bin/env bash
set -euo pipefail

# orchestrator-persistence-lint.sh
#
# Gate G086 — orchestrator_persistence_lint_gate.
#
# Scans the 4 orchestrator prompt files for user-reprompt language that
# would make continuation depend on a fresh user prompt. Orchestrators
# must default to persistence: after non-terminal phases, continue to the
# next phase until convergence achieved, max iterations reached, the user
# requests stop, or fundamental impossibility is encountered.
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

TARGET_FILES=(
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.sprint.agent.md"
)

FORBIDDEN_PHRASES=(
  "should i continue"
  "shall i proceed"
  "do you want me to"
  "would you like me to continue"
  "ask the user before continuing"
)

REQUIRED_MARKERS=(
  "g086"
  "automatically continue"
  "non-terminal"
  "convergence achieved"
  "max iterations"
  "user requests stop"
  "fundamental impossibility"
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

missing_count=0
for rel in "${TARGET_FILES[@]}"; do
  path="$REPO_ROOT/$rel"
  if [[ ! -f "$path" ]]; then
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

finding_count=0

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

    if [[ "$same_line_marked" == "true" && "$in_fence" == "false" ]]; then
      in_marked_block="true"
    fi
    if [[ -n "${line//[[:space:]]/}" ]]; then
      previous_nonempty_marked="$same_line_marked"
    fi
  done < "$path"
}

check_required_language() {
  local rel="$1"
  local path="$2"
  local marker

  for marker in "${REQUIRED_MARKERS[@]}"; do
    if ! grep -qiF "$marker" "$path"; then
      echo "G086 orchestrator_persistence_lint_gate violation: $rel missing required persistence-default marker '$marker'" >&2
      finding_count=$((finding_count + 1))
    fi
  done
}

for rel in "${TARGET_FILES[@]}"; do
  path="$REPO_ROOT/$rel"
  scan_forbidden_phrases "$rel" "$path"
  check_required_language "$rel" "$path"
done

if [[ "$finding_count" -gt 0 ]]; then
  echo "G086 orchestrator_persistence_lint_gate blocked: findings=$finding_count root=$REPO_ROOT" >&2
  exit 1
fi

info "scannedFiles=${#TARGET_FILES[@]} findings=0 root=$REPO_ROOT"
if [[ "$QUIET" != "true" ]]; then
  echo "PASS Gate G086 (orchestrator_persistence_lint_gate) — scannedFiles=${#TARGET_FILES[@]}, findings=0"
fi
exit 0