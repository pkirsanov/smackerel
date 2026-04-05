#!/usr/bin/env bash
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

AGENTS_DIR="${1:-.github/agents}"

if [[ ! -d "$AGENTS_DIR" ]]; then
  echo "ERROR: agents directory not found: $AGENTS_DIR"
  exit 2
fi

if ! command -v tsort >/dev/null 2>&1; then
  echo "ERROR: tsort is required but not installed"
  exit 2
fi

tmp_edges="$(mktemp)"
tmp_nodes="$(mktemp)"
trap 'rm -f "$tmp_edges" "$tmp_nodes"' EXIT

found_any=0
missing_targets=0

while IFS= read -r file; do
  found_any=1

  source_name="$(sed -n 's/^\*\*Name:\*\*[[:space:]]*//p' "$file" | head -n 1 | tr -d '\r')"
  if [[ -z "$source_name" ]]; then
    source_name="$(basename "$file" .agent.md)"
  fi

  echo "$source_name" >> "$tmp_nodes"

  while IFS= read -r target; do
    target_clean="$(echo "$target" | tr -d '\r' | xargs)"
    [[ -z "$target_clean" ]] && continue

    echo "$source_name $target_clean" >> "$tmp_edges"
    echo "$target_clean" >> "$tmp_nodes"

    if [[ ! -f "$AGENTS_DIR/$target_clean.agent.md" ]]; then
      echo "MISSING_TARGET: $source_name -> $target_clean (expected $AGENTS_DIR/$target_clean.agent.md)"
      missing_targets=1
    fi
  done < <(sed -n 's/^[[:space:]]*agent:[[:space:]]*//p' "$file")
done < <(find "$AGENTS_DIR" -maxdepth 1 -type f -name '*.agent.md' | sort)

if [[ "$found_any" -eq 0 ]]; then
  echo "ERROR: no .agent.md files found under $AGENTS_DIR"
  exit 2
fi

sort -u "$tmp_nodes" >/dev/null

if [[ ! -s "$tmp_edges" ]]; then
  echo "PASS: no handoff edges found; no cycles possible"
  exit 0
fi

echo "Checking handoff graph cycles with tsort..."
if tsort "$tmp_edges" >/dev/null 2>&1; then
  if [[ "$missing_targets" -eq 1 ]]; then
    echo "FAIL: no cycle detected, but missing handoff targets were found"
    exit 1
  fi
  echo "PASS: no cycles detected and all handoff targets exist"
  fun_message all_gates_pass
  exit 0
fi

echo "FAIL: cycle detected in handoff graph"
fun_message gate_failed
# Re-run without suppression so cycle details are printed
set +e
tsort "$tmp_edges"
exit 1
