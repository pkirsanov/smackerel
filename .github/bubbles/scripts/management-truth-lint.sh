#!/usr/bin/env bash
# management-truth-lint.sh — mechanical backstop for DOC-101 management-truth drift.
#
# Documentation "management surfaces" enumerate the framework's real inventory:
# the recipe catalog lists every recipe, and the installer `--profile` help lists
# every adoption profile. These hand-maintained catalogs drift SILENTLY — the
# recipe index once linked only 60 of 70 recipe files, and the `--profile` help
# once omitted the `production` profile. Manifest freshness checks prove
# deterministic regeneration of managed files, NOT that a hand-maintained catalog
# still enumerates the live inventory.
#
# This guard mechanically enforces completeness for the two robust,
# machine-checkable management surfaces (IMP-101 SCOPE-10 / DOC-101):
#   1. Recipe catalog — every docs/recipes/*.md (except README) MUST be linked
#      from docs/recipes/README.md.
#   2. Adoption-profile help — every id declared in bubbles/adoption-profiles.yaml
#      MUST appear in the install.sh `--profile` help text.
#
# The broader SCOPE-10 goal (generating agent/prompt/MCP-tool/count prose from
# registries, plus a machine-readable bug-disposition ledger) remains future
# work; this guard locks in the two catalogs that actually regressed so they
# cannot silently drift again.
#
# Usage:
#   management-truth-lint.sh [REPO_ROOT]   # default REPO_ROOT=.
#
# Exit 0 = clean. Exit 1 = at least one catalog omission. No --skip / --force.

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

err() { echo "[management-truth-lint][ERROR] $*" >&2; }
info() { echo "[management-truth-lint] $*"; }

findings=0

# ── Check 1: recipe catalog completeness ──────────────────────────────
recipes_dir="$REPO_ROOT/docs/recipes"
recipes_readme="$recipes_dir/README.md"
if [[ -d "$recipes_dir" && -f "$recipes_readme" ]]; then
  for recipe in "$recipes_dir"/*.md; do
    [[ -e "$recipe" ]] || continue
    base="$(basename "$recipe")"
    [[ "$base" == "README.md" ]] && continue
    if ! grep -q "($base)" "$recipes_readme"; then
      err "recipe not linked in catalog: docs/recipes/$base"
      findings=$((findings + 1))
    fi
  done
else
  info "recipe catalog not present at docs/recipes/ (skipping check 1)"
fi

# ── Check 2: adoption-profile help completeness ───────────────────────
profiles_file="$REPO_ROOT/bubbles/adoption-profiles.yaml"
install_sh="$REPO_ROOT/install.sh"
if [[ -f "$profiles_file" && -f "$install_sh" ]]; then
  # Collect every install.sh line that documents a profile (the `--profile`
  # usage line and any example/comment mentioning an adoption profile). A
  # declared id must appear somewhere in that documentation surface.
  profile_help="$( { grep -iE -- "--profile|adoption profile" "$install_sh" || true; } )"
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    if ! printf '%s\n' "$profile_help" | grep -qw -- "$pid"; then
      err "adoption profile '$pid' is declared in adoption-profiles.yaml but absent from install.sh --profile help"
      findings=$((findings + 1))
    fi
  done < <( { grep -E "^[[:space:]]+id:[[:space:]]" "$profiles_file" || true; } | awk '{print $2}' )
else
  info "adoption-profiles.yaml or install.sh not present (skipping check 2)"
fi

if [[ "$findings" -gt 0 ]]; then
  err "found $findings management-truth catalog omission(s)"
  exit 1
fi

info "OK — recipe catalog and adoption-profile help enumerate the live inventory"
exit 0
