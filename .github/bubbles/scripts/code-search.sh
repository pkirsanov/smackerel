#!/usr/bin/env bash
#
# bubbles code-search.sh — uniform code search facade (v5.1 / M8).
#
# Delegates to the host's best available tool (rg → grep) so agents
# don't reinvent search per repo. Output is line-oriented and stable.
#
# Usage:
#   code-search.sh <pattern> [path...]
#   code-search.sh --files <glob>             # list matching paths
#   code-search.sh --kind rust <pattern>      # restrict by language
#
# Exit:
#   0 if matches found
#   1 if no matches (matches grep/rg convention)
#   2 if usage error
#
# Token-efficiency notes for agent callers:
# - Returns at most 400 lines unless --no-cap is passed.
# - Output is stable across rg/grep backends so prompts can rely on shape.

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  code-search.sh <pattern> [path...]
  code-search.sh --files <glob> [path]
  code-search.sh --kind <lang> <pattern> [path...]
  code-search.sh --no-cap <pattern> [path...]    # don't limit lines
USAGE
}

if [[ $# -lt 1 ]]; then usage; exit 2; fi

MODE="search"
KIND=""
CAP=400
while [[ $# -gt 0 ]]; do
  case "$1" in
    --files) MODE="files"; shift;;
    --kind) KIND="$2"; shift 2;;
    --no-cap) CAP=0; shift;;
    -h|--help) usage; exit 0;;
    *) break;;
  esac
done

if [[ $# -lt 1 ]]; then usage; exit 2; fi

PATTERN="$1"
shift
SEARCH_PATHS=("$@")
[[ "${#SEARCH_PATHS[@]}" -eq 0 ]] && SEARCH_PATHS=(".")

# Map --kind to file extensions.
kind_globs=()
case "$KIND" in
  "") ;;
  rust) kind_globs=(--include='*.rs') ;;
  go) kind_globs=(--include='*.go') ;;
  ts|typescript) kind_globs=(--include='*.ts' --include='*.tsx') ;;
  js|javascript) kind_globs=(--include='*.js' --include='*.jsx') ;;
  py|python) kind_globs=(--include='*.py') ;;
  sh|bash) kind_globs=(--include='*.sh') ;;
  md|markdown) kind_globs=(--include='*.md') ;;
  yaml|yml) kind_globs=(--include='*.yaml' --include='*.yml') ;;
  json) kind_globs=(--include='*.json') ;;
  *) echo "code-search: unknown --kind: $KIND" >&2; exit 2 ;;
esac

run_with_cap() {
  if [[ "$CAP" -eq 0 ]]; then
    cat
  else
    awk -v cap="$CAP" 'NR <= cap { print } NR == cap+1 { print "  [code-search] output capped at "cap" lines; pass --no-cap to disable" }'
  fi
}

# v5.2 / F6: auto-select backend on first call, cache the choice to
# .specify/runtime/code-search.tool so subsequent calls skip the
# `command -v rg` probe. The cache is per-repo and survives restarts.
# Manual override: export BUBBLES_CODE_SEARCH_BACKEND=rg|grep before invocation.
SEARCH_BACKEND="${BUBBLES_CODE_SEARCH_BACKEND:-}"
if [[ -z "$SEARCH_BACKEND" ]]; then
  # Locate runtime cache. Best-effort repo root.
  CS_REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  CS_CACHE_DIR="$CS_REPO_ROOT/.specify/runtime"
  CS_CACHE_FILE="$CS_CACHE_DIR/code-search.tool"
  if [[ -f "$CS_CACHE_FILE" ]]; then
    SEARCH_BACKEND="$(tr -d '[:space:]' < "$CS_CACHE_FILE" 2>/dev/null || echo '')"
  fi
  if [[ -z "$SEARCH_BACKEND" ]]; then
    if command -v rg >/dev/null 2>&1; then
      SEARCH_BACKEND="rg"
    else
      SEARCH_BACKEND="grep"
    fi
    # Persist (best-effort; OK to fail if dir is read-only).
    mkdir -p "$CS_CACHE_DIR" 2>/dev/null && echo "$SEARCH_BACKEND" > "$CS_CACHE_FILE" 2>/dev/null || true
  fi
fi

if [[ "$SEARCH_BACKEND" == "rg" ]] && command -v rg >/dev/null 2>&1; then
  # rg path — fastest and most agent-friendly.
  rg_args=(--line-number --no-heading --color=never)
  if [[ "${#kind_globs[@]}" -gt 0 ]]; then
    # Translate --include='*.rs' → rg -g '*.rs'.
    for g in "${kind_globs[@]}"; do
      ext="${g#--include=}"
      rg_args+=(-g "$ext")
    done
  fi
  if [[ "$MODE" == "files" ]]; then
    rg --files "${SEARCH_PATHS[@]}" 2>/dev/null | grep -E "$PATTERN" | run_with_cap
  else
    rg "${rg_args[@]}" -- "$PATTERN" "${SEARCH_PATHS[@]}" 2>/dev/null | run_with_cap
  fi
else
  # grep fallback.
  if [[ "$MODE" == "files" ]]; then
    find "${SEARCH_PATHS[@]}" -type f -name "$PATTERN" 2>/dev/null | run_with_cap
  else
    grep -rnE "${kind_globs[@]+"${kind_globs[@]}"}" "$PATTERN" "${SEARCH_PATHS[@]}" 2>/dev/null | run_with_cap
  fi
fi
