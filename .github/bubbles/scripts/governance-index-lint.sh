#!/usr/bin/env bash
# governance-index-lint.sh
#
# Detects orphan governance docs that are not referenced from any
# well-known index doc. Orphan docs invisibly accumulate over time
# because no agent ever reads them.
#
# Discovers governance docs under:
#   - agents/bubbles_shared/*.md
#   - instructions/*.instructions.md
#   - skills/*/SKILL.md
#   - docs/recipes/*.md
#
# Considers a doc "indexed" if its basename appears as a markdown link
# target in at least one well-known index:
#   - README.md
#   - agents/bubbles_shared/agent-common.md
#   - agents/bubbles_shared/scope-workflow.md
#   - docs/governance-index.md (the canonical roll-up index)
#   - any agents/*.agent.md (agents reference the docs they consume)
#
# Allowlist (always treated as indexed):
#   - README.md, CHANGELOG.md, LICENSE, VERSION
#   - any docs/issues/*.md (issue tracker)
#   - any docs/generated/*.md (auto-generated)
#   - any *-selftest.md
#
# Exit 0 when zero orphans, 1 when any orphan exists.
#
# Usage:
#   bash bubbles/scripts/governance-index-lint.sh \
#       [--repo-root <path>] [--allow <regex>] [--verbose]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
allow_extra=""
verbose="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/governance-index-lint.sh [--repo-root <path>] [--allow <regex>] [--verbose]

Detects governance docs not referenced from any index. Exits 0 if no
orphans, 1 if any orphan is found.

Options:
  --repo-root <path>   Repo root to scan (default: script repo root)
  --allow <regex>      Extra regex appended to the allowlist (relative paths)
  --verbose            Print per-doc index hits in addition to orphans
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --allow)
      shift
      allow_extra="${1:?--allow requires a regex}"
      shift
      ;;
    --verbose)
      verbose="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "governance-index-lint: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "governance-index-lint: repo root not found: $repo_root" >&2
  exit 2
fi

# --- Discover governance docs (relative paths) -----------------------------

declare -a docs=()
push_doc() {
  local rel="$1"
  [[ -f "$repo_root/$rel" ]] || return 0
  docs+=("$rel")
}

# agents/bubbles_shared/*.md
if [[ -d "$repo_root/agents/bubbles_shared" ]]; then
  while IFS= read -r -d '' f; do
    rel="${f#"$repo_root/"}"
    push_doc "$rel"
  done < <(find "$repo_root/agents/bubbles_shared" -maxdepth 1 -name '*.md' -print0 | sort -z)
fi

# instructions/*.instructions.md
if [[ -d "$repo_root/instructions" ]]; then
  while IFS= read -r -d '' f; do
    rel="${f#"$repo_root/"}"
    push_doc "$rel"
  done < <(find "$repo_root/instructions" -maxdepth 1 -name '*.instructions.md' -print0 | sort -z)
fi

# skills/*/SKILL.md
if [[ -d "$repo_root/skills" ]]; then
  while IFS= read -r -d '' f; do
    rel="${f#"$repo_root/"}"
    push_doc "$rel"
  done < <(find "$repo_root/skills" -mindepth 2 -maxdepth 2 -name 'SKILL.md' -print0 | sort -z)
fi

# docs/recipes/*.md
if [[ -d "$repo_root/docs/recipes" ]]; then
  while IFS= read -r -d '' f; do
    rel="${f#"$repo_root/"}"
    push_doc "$rel"
  done < <(find "$repo_root/docs/recipes" -maxdepth 1 -name '*.md' -print0 | sort -z)
fi

# --- Collect well-known indexes ------------------------------------------

declare -a indexes=()
push_index() {
  local rel="$1"
  [[ -f "$repo_root/$rel" ]] || return 0
  indexes+=("$repo_root/$rel")
}

push_index "README.md"
push_index "agents/bubbles_shared/agent-common.md"
push_index "agents/bubbles_shared/scope-workflow.md"
push_index "docs/governance-index.md"

if [[ -d "$repo_root/agents" ]]; then
  while IFS= read -r -d '' f; do
    indexes+=("$f")
  done < <(find "$repo_root/agents" -maxdepth 1 -name '*.agent.md' -print0 | sort -z)
fi

# --- Allowlist (always treated as indexed) ---------------------------------

is_allowlisted() {
  local rel="$1"
  case "$rel" in
    README.md|CHANGELOG.md|LICENSE|VERSION) return 0 ;;
    # Docs that are themselves well-known indexes — they roll up other
    # docs, so it is fine for them not to be referenced from elsewhere.
    agents/bubbles_shared/agent-common.md) return 0 ;;
    agents/bubbles_shared/scope-workflow.md) return 0 ;;
    docs/governance-index.md) return 0 ;;
  esac
  if [[ "$rel" =~ ^docs/issues/.*\.md$ ]]; then return 0; fi
  if [[ "$rel" =~ ^docs/generated/.*\.md$ ]]; then return 0; fi
  if [[ "$rel" =~ -selftest\.md$ ]]; then return 0; fi
  if [[ -n "$allow_extra" && "$rel" =~ $allow_extra ]]; then return 0; fi
  return 1
}

# --- Check each doc against indexes ---------------------------------------

orphans=()
checked=0

for rel in "${docs[@]}"; do
  checked=$((checked + 1))

  if is_allowlisted "$rel"; then
    [[ "$verbose" == "true" ]] && echo "ALLOWED: $rel"
    continue
  fi

  basename="$(basename "$rel")"
  hit=""

  for idx in "${indexes[@]}"; do
    # Skip the doc itself if it appears to be an index too (it should
    # not self-reference to count as indexed).
    if [[ "$idx" == "$repo_root/$rel" ]]; then
      continue
    fi
    if grep -Fq -- "$basename" "$idx" 2>/dev/null; then
      hit="$idx"
      break
    fi
  done

  if [[ -z "$hit" ]]; then
    orphans+=("$rel")
  elif [[ "$verbose" == "true" ]]; then
    echo "INDEXED: $rel -> ${hit#"$repo_root/"}"
  fi
done

# --- Report ----------------------------------------------------------------

echo "governance-index-lint: scanned $checked governance doc(s)"
echo "governance-index-lint: indexes consulted: ${#indexes[@]}"

if [[ "${#orphans[@]}" -eq 0 ]]; then
  echo "governance-index-lint: PASS — zero orphan docs"
  exit 0
fi

echo "governance-index-lint: FAIL — ${#orphans[@]} orphan doc(s) detected"
echo "ORPHAN_GOVERNANCE_DOC:"
for rel in "${orphans[@]}"; do
  echo "  - $rel"
done
echo
echo "Action: link each orphan from a well-known index, OR add a"
echo "matching --allow <regex> entry, OR delete the doc if obsolete."
exit 1
