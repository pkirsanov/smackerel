#!/usr/bin/env bash
#
# Bubbles v6.0 / C1 — one-shot rewrite of v5 mode names to v6 primitive+tag.
#
# Modes (idempotent in all cases):
#   --check         dry-run, list every occurrence that would be rewritten (exit 0 if none, 2 if any)
#   --write         rewrite in place (default off)
#   --paths "<glob>" comma-separated paths/globs to scan (default: scripts/ + docs/ + .specify/ + Makefile + repo-root *.md)
#   --include-instructions  also scan instructions/ + .github/instructions/ + .github/copilot-instructions.md
#   --aliases-file <path>  alternate v5->v6 aliases.yaml (selftest hook)
#
# Targets scanned:
#   any token matching ./<cli>.sh <v5-mode-name> arg shape
#   any token matching `bubbles.<agent> mode=<v5-mode-name>`
#   any /bubbles.<v5-mode-name> command in markdown
#   any direct reference to a v5 mode name in a list of allowed contexts
#
# Skips:
#   git history
#   .git/
#   any path under .pre-push-validated/
#   the alias map itself (bubbles/workflows/aliases.yaml)
#   the migration script + its selftest
#
# Exit codes:
#   0  no rewrites needed (--check) or rewrites applied successfully (--write)
#   1  rewrite failed (write error, validation failed)
#   2  --check found rewrites that would be applied (CI gate signal)

set -euo pipefail

REPO_ROOT="${BUBBLES_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
ALIASES_FILE="${BUBBLES_WORKFLOW_ALIASES_FILE:-$REPO_ROOT/bubbles/workflows/aliases.yaml}"
MODE="--check"
INCLUDE_INSTRUCTIONS="0"
declare -a SEARCH_PATHS=()
declare -a CUSTOM_PATHS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="--check"; shift ;;
    --write) MODE="--write"; shift ;;
    --paths) IFS=',' read -r -a CUSTOM_PATHS <<<"$2"; shift 2 ;;
    --include-instructions) INCLUDE_INSTRUCTIONS="1"; shift ;;
    --aliases-file) ALIASES_FILE="$2"; shift 2 ;;
    --help|-h)
      cat <<EOF
Usage: bash bubbles/scripts/migrate-modes-v5-to-v6.sh [--check|--write] [--paths "<glob,glob>"] [--include-instructions] [--aliases-file path]

Rewrites operator-side mentions of v5 workflow mode names to their v6
primitive+tag form. Idempotent and safe to re-run.

  --check               (default) dry-run; exit 2 if rewrites needed
  --write               rewrite in place
  --paths "a,b,c"       comma-separated paths/globs to scan
  --include-instructions also scan instructions/ + .github/instructions/
  --aliases-file PATH   alternate aliases.yaml (mainly for selftests)
EOF
      exit 0
      ;;
    *) echo "migrate-modes-v5-to-v6.sh: unknown argument: $1" >&2; exit 1 ;;
  esac
done

[[ -f "$ALIASES_FILE" ]] || { echo "migrate-modes-v5-to-v6.sh: missing aliases file: $ALIASES_FILE" >&2; exit 1; }

# ── Parse the alias map: v5_mode -> v6 primitive + tag set ───────
declare -A V5_TO_V6=()

mapfile -t alias_lines < <(awk '
  /^v5Aliases:/ { in_aliases=1; next }
  in_aliases && /^[a-zA-Z]/ && !/^[[:space:]]/ { in_aliases=0 }
  in_aliases && /^  [a-zA-Z0-9._-]+:$/ {
    if (current_name != "") {
      printf "ALIAS\t%s\t%s\t%s\n", current_name, current_primitive, current_tags
    }
    current_name=$1
    sub(":$", "", current_name)
    current_primitive=""
    current_tags=""
    next
  }
  in_aliases && /^    primitive:[[:space:]]+/ {
    line=$0
    sub(/^[[:space:]]+primitive:[[:space:]]+/, "", line)
    gsub(/^[\47"]|[\47"]$/, "", line)
    current_primitive=line
    next
  }
  in_aliases && /^    tags:[[:space:]]+\{/ {
    # Inline form: tags: { action: x, target: y }
    line=$0
    sub(/^[[:space:]]+tags:[[:space:]]+\{[[:space:]]*/, "", line)
    sub(/[[:space:]]*\}[[:space:]]*$/, "", line)
    # Split by comma, strip whitespace, output as "key:val key:val"
    n=split(line, parts, ",")
    tags=""
    for (i=1; i<=n; i++) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", parts[i])
      if (parts[i] == "") continue
      if (tags == "") {
        tags=parts[i]
      } else {
        tags = tags " " parts[i]
      }
      gsub(/[[:space:]]*:[[:space:]]*/, ":", tags)
    }
    current_tags=tags
    next
  }
  END {
    if (current_name != "") {
      printf "ALIAS\t%s\t%s\t%s\n", current_name, current_primitive, current_tags
    }
  }
' "$ALIASES_FILE")

for ln in "${alias_lines[@]}"; do
  case "$ln" in
    ALIAS$'\t'*)
      rest="${ln#ALIAS$'\t'}"
      IFS=$'\t' read -r v5name primitive tags <<<"$rest"
      [[ -n "$v5name" && -n "$primitive" ]] || continue
      v6form="$primitive"
      if [[ -n "$tags" ]]; then
        v6form="$primitive $tags"
      fi
      V5_TO_V6["$v5name"]="$v6form"
      ;;
  esac
done

if [[ ${#V5_TO_V6[@]} -eq 0 ]]; then
  echo "migrate-modes-v5-to-v6.sh: no aliases parsed from $ALIASES_FILE" >&2
  exit 1
fi

# ── Choose paths to scan ─────────────────────────────────────────
if [[ ${#CUSTOM_PATHS[@]} -gt 0 ]]; then
  SEARCH_PATHS=("${CUSTOM_PATHS[@]}")
else
  # Default scan: operator-visible surfaces only.
  # EXCLUDED by design:
  #   .git, .pre-push-validated
  #   bubbles/workflows (alias map itself)
  #   bubbles/scripts/migrate-modes-v5-to-v6*.sh (self)
  #   bubbles/scripts/*selftest.sh (selftests carry v5 names as fixtures)
  #   bubbles/scripts/* (framework internals, NOT operator-side)
  #   skills, agents (framework internals, owned by maintainers)
  #   docs/CHEATSHEET.md and docs/its-not-rocket-appliances.html (generated)
  #   docs/v5.2-design.md, docs/v6-mcp-design.md (historical design docs preserve v5 vocabulary)
  #   CHANGELOG.md (historical record preserves v5 vocabulary)
  # NOTE: docs/recipes ARE scanned (v7) — they are operator-facing surfaces and
  #   must stay free of bare v5 leading-token forms. Pedagogical v5 mentions in
  #   prose/backticks (e.g. upgrade recipes) are not matched by the rewrite
  #   patterns, which only target /bubbles.workflow <v5>, /bubbles.<v5>,
  #   mode=<v5>, and run-mode <v5> — never the valid `mode: <v5>` key form.
  while IFS= read -r f; do
    SEARCH_PATHS+=("$f")
  done < <(
    find "$REPO_ROOT" \
      \( -path "$REPO_ROOT/.git" -prune \) -o \
      \( -path "$REPO_ROOT/.pre-push-validated" -prune \) -o \
      \( -path "$REPO_ROOT/bubbles/workflows" -prune \) -o \
      \( -path "$REPO_ROOT/bubbles/scripts" -prune \) -o \
      \( -path "$REPO_ROOT/skills" -prune \) -o \
      \( -path "$REPO_ROOT/agents" -prune \) -o \
      \( -path "$REPO_ROOT/docs/CHEATSHEET.md" -prune \) -o \
      \( -path "$REPO_ROOT/docs/its-not-rocket-appliances.html" -prune \) -o \
      \( -path "$REPO_ROOT/docs/v5.2-design.md" -prune \) -o \
      \( -path "$REPO_ROOT/docs/v6-mcp-design.md" -prune \) -o \
      \( -path "$REPO_ROOT/CHANGELOG.md" -prune \) -o \
      \( -type f \( -name '*.md' -o -name 'Makefile' \) -print \) \
      2>/dev/null
  )
  # Always include install.sh (it's the operator-installer surface)
  [[ -f "$REPO_ROOT/install.sh" ]] && SEARCH_PATHS+=("$REPO_ROOT/install.sh")
  if [[ "$INCLUDE_INSTRUCTIONS" == "1" ]]; then
    while IFS= read -r f; do
      SEARCH_PATHS+=("$f")
    done < <(find "$REPO_ROOT/instructions" "$REPO_ROOT/.github/instructions" "$REPO_ROOT/.github/copilot-instructions.md" -type f 2>/dev/null)
  fi
fi

# ── Build sed program ────────────────────────────────────────────
# Pattern: match v5 mode name when it appears as a complete shell
# argument (after the CLI invocation) OR as the slash-command form
# /bubbles.<v5name>. Word-boundary alone is unsafe because v5 mode
# names contain hyphens.

declare -i rewrite_count=0
declare -a changed_files=()

scan_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  [[ -r "$file" ]] || return 0

  # Skip binary files
  if file --mime-encoding "$file" 2>/dev/null | grep -qE 'binary$'; then
    return 0
  fi

  local tmpfile
  tmpfile="$(mktemp)"
  cp "$file" "$tmpfile"

  local v5 v6 v6_first v6_rest la file_changed=0
  for v5 in "${!V5_TO_V6[@]}"; do
    v6="${V5_TO_V6[$v5]}"
    # Skip primitives that have no v5 alias different from themselves
    [[ "$v5" == "$v6" ]] && continue

    # Idempotency guard for SELF-NAMED primitives, where the v6 form begins with
    # the v5 token itself (e.g. framework-health -> "framework-health
    # action:proposal-first"). Without this, the already-rewritten string
    # "workflow framework-health action:proposal-first" would re-match
    # "workflow framework-health" and double-apply the tail. The negative
    # lookahead skips a match that is already followed by the EXACT v6 tail, so
    # the rewrite stays idempotent. (Non-self-named v6 forms can never re-match
    # because their first token differs from the v5 name.)
    v6_first="${v6%% *}"
    if [[ "$v6_first" == "$v5" && "$v6" == *" "* ]]; then
      v6_rest="${v6#* }"
      la="(?![[:space:]]+\\Q${v6_rest}\\E)"
    else
      la=""
    fi

    # Pattern A: slash command /bubbles.<v5name>
    # Pattern B: bare argument after a CLI: "./*.sh <v5name>" or "bubbles <v5name>"
    # Pattern C: mode=<v5name>
    # We use perl in-place with multi-pattern alternation.
    if grep -qE "(/bubbles\\.${v5}\\b|mode=${v5}\\b|run-mode[[:space:]]+${v5}\\b|workflow[[:space:]]+${v5}\\b)" "$tmpfile"; then
      perl -pi -e "
        s|/bubbles\\.${v5}\\b|/bubbles.${v6_first}|g;
        s|mode=${v5}\\b${la}|mode=${v6}|g;
        s|run-mode[[:space:]]+${v5}\\b${la}|run-mode ${v6}|g;
        s|workflow[[:space:]]+${v5}\\b${la}|workflow ${v6}|g;
      " "$tmpfile"
      file_changed=1
    fi
  done

  if [[ $file_changed -eq 1 ]]; then
    if ! cmp -s "$file" "$tmpfile"; then
      rewrite_count=$((rewrite_count + 1))
      changed_files+=("$file")
      if [[ "$MODE" == "--write" ]]; then
        cp "$tmpfile" "$file"
      fi
    fi
  fi

  rm -f "$tmpfile"
}

for f in "${SEARCH_PATHS[@]}"; do
  scan_file "$f"
done

# ── Report ───────────────────────────────────────────────────────
echo "migrate-modes-v5-to-v6: scanned ${#SEARCH_PATHS[@]} file(s); $rewrite_count file(s) need(s) rewriting"

if [[ ${#changed_files[@]} -gt 0 ]]; then
  for f in "${changed_files[@]}"; do
    rel="${f#$REPO_ROOT/}"
    echo "  - $rel"
  done
fi

if [[ "$MODE" == "--check" ]]; then
  if [[ $rewrite_count -eq 0 ]]; then
    echo "migrate-modes-v5-to-v6: PASS (no rewrites needed)"
    exit 0
  else
    echo "migrate-modes-v5-to-v6: rewrites pending — run with --write to apply"
    exit 2
  fi
fi

if [[ "$MODE" == "--write" ]]; then
  echo "migrate-modes-v5-to-v6: $rewrite_count file(s) rewritten in place"
  exit 0
fi
