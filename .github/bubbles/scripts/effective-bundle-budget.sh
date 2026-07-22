#!/usr/bin/env bash
# effective-bundle-budget.sh — optional effective-context budget (IMP-101
# SCOPE-6 / CTX-101), layered on top of effective-bundle-measure.sh.
#
# instruction-budget-lint budgets INDIVIDUAL agent files, but the real loaded
# prompt is an agent's transitive closure of bubbles_shared contracts. This
# guard measures that effective bundle per agent and enforces an OPTIONAL budget:
#
#   * Default (no config): reports each agent's effective bundle size and exits 0
#     — pure information, no enforcement, so it can never break a green tree.
#   * Opt-in: set `effectiveBundleMaxBytes: N` in .github/bubbles-project.yaml to
#     flag agents whose effective bundle exceeds N bytes. Findings are advisory
#     (exit 0) unless you also set `effectiveBundleBudget: block` (exit 1).
#
# No `--skip`/`--force` bypass. The budget is a ceiling YOU choose; the framework
# ships no arbitrary default so it never imposes an unverified limit.
#
# Usage: effective-bundle-budget.sh [REPO_ROOT]
# Exit 0 = within budget, informational, or advisory. Exit 1 = over AND block.

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MEASURE="$SCRIPT_DIR/effective-bundle-measure.sh"
agents_dir="$REPO_ROOT/agents"

err() { echo "[effective-bundle-budget][ERROR] $*" >&2; }
info() { echo "[effective-bundle-budget] $*"; }

[[ -d "$agents_dir" ]] || { info "no agents/ directory at $agents_dir (skipping)"; exit 0; }
[[ -x "$MEASURE" ]] || { info "effective-bundle-measure.sh not available (skipping)"; exit 0; }

# ── read optional budget config (walk up for .github/bubbles-project.yaml) ──
max_bytes=""
mode="advisory"
dir="$REPO_ROOT"
project_config=""
while :; do
  if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then project_config="$dir/.github/bubbles-project.yaml"; break; fi
  [[ "$dir" == "/" || -z "$dir" ]] && break
  dir="$(dirname "$dir")"
done
if [[ -n "$project_config" ]]; then
  max_bytes="$( { grep -oE '^[[:space:]]*effectiveBundleMaxBytes:[[:space:]]*[0-9]+' "$project_config" || true; } | grep -oE '[0-9]+' | head -1 || true)"
  if grep -qE '^[[:space:]]*effectiveBundleBudget:[[:space:]]*block[[:space:]]*$' "$project_config"; then
    mode="block"
  fi
fi

over=0
measured=0
while IFS= read -r agent; do
  [[ -n "$agent" ]] || continue
  json="$(bash "$MEASURE" "$agent" --agents-dir "$agents_dir" 2>/dev/null || true)"
  tb="$( printf '%s' "$json" | grep -oE '"totalBytes"[[:space:]]*:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | head -1 || true)"
  [[ -n "$tb" ]] || continue
  measured=$((measured + 1))
  base="$(basename "$agent")"
  if [[ -n "$max_bytes" && "$tb" -gt "$max_bytes" ]]; then
    err "$base: effective bundle ${tb} bytes exceeds budget ${max_bytes} bytes"
    over=$((over + 1))
  elif [[ -z "$max_bytes" ]]; then
    info "$base: effective bundle ${tb} bytes"
  fi
done < <( { find "$agents_dir" -maxdepth 1 -name '*.agent.md' 2>/dev/null || true; } | sort )

if [[ -z "$max_bytes" ]]; then
  info "measured $measured agent bundle(s); no effectiveBundleMaxBytes configured — informational only"
  exit 0
fi
if [[ "$over" -eq 0 ]]; then
  info "OK — all $measured agent bundle(s) within the ${max_bytes}-byte budget"
  exit 0
fi
if [[ "$mode" == "block" ]]; then
  err "$over agent bundle(s) over budget — failing (effectiveBundleBudget: block)"
  exit 1
fi
info "$over agent bundle(s) over budget — advisory only (exit 0). Set effectiveBundleBudget: block in .github/bubbles-project.yaml to enforce."
exit 0
