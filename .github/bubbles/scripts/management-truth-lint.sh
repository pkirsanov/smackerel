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
#
# Two inventories list recipes and only one was checked, so docs/CATALOG.md
# drifted to 61 of 75 entries while docs/recipes/README.md stayed complete. Both
# are now checked. CATALOG.md is not redundant -- it carries the mode/agent
# mapping and the decision tree that the categorized index does not.
recipes_dir="$REPO_ROOT/docs/recipes"
recipes_readme="$recipes_dir/README.md"
recipe_catalog="$REPO_ROOT/docs/CATALOG.md"
if [[ -d "$recipes_dir" && -f "$recipes_readme" ]]; then
  for recipe in "$recipes_dir"/*.md; do
    [[ -e "$recipe" ]] || continue
    base="$(basename "$recipe")"
    [[ "$base" == "README.md" ]] && continue
    if ! grep -q "($base)" "$recipes_readme"; then
      err "recipe not linked in catalog: docs/recipes/$base"
      findings=$((findings + 1))
    fi
    if [[ -f "$recipe_catalog" ]] && ! grep -q "(recipes/$base)" "$recipe_catalog"; then
      err "recipe not listed in docs/CATALOG.md: docs/recipes/$base"
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

# ── Check 3: documented inventory counts match the live inventory ─────
# Robust by design: only compares when the anchored count phrase is present, so
# rewording never false-positives; a PRESENT count that disagrees with the real
# inventory is a finding (this is the class of drift DOC-101 hit — 34 vs 41).
compare_doc_count() {
  local label="$1" true_n="$2" file="$3" phrase="$4"
  [[ -f "$file" ]] || return 0
  local doc_n
  doc_n="$( { grep -oE "[0-9]+ ${phrase}" "$file" || true; } | head -1 | grep -oE "^[0-9]+" || true )"
  [[ -n "$doc_n" ]] || return 0
  if [[ "$doc_n" != "$true_n" ]]; then
    err "$label: docs say ${doc_n} '${phrase}' but the live inventory has ${true_n} ($file)"
    findings=$((findings + 1))
  fi
}

agents_n=0
for f in "$REPO_ROOT"/agents/*.agent.md; do [[ -e "$f" ]] && agents_n=$((agents_n + 1)); done
prompts_n=0
for f in "$REPO_ROOT"/prompts/*.prompt.md; do [[ -e "$f" ]] && prompts_n=$((prompts_n + 1)); done
tools_n=0
for f in "$REPO_ROOT"/bubbles/mcp/tools/*.json; do [[ -e "$f" ]] && tools_n=$((tools_n + 1)); done

compare_doc_count "agent count" "$agents_n" "$REPO_ROOT/docs/guides/INSTALLATION.md" "agent definitions"
compare_doc_count "prompt-shim count" "$prompts_n" "$REPO_ROOT/docs/guides/INSTALLATION.md" "prompt shims"
compare_doc_count "MCP tool count" "$tools_n" "$REPO_ROOT/docs/MCP.md" "annotated tools"
compare_doc_count "MCP prompt count" "$prompts_n" "$REPO_ROOT/docs/MCP.md" "prompts"

# ── Check 4: managed-doc requiredSections survive resolution ──────────
#
# The resolver parses the registry with portable awk, so an indentation
# assumption is enough to drop a whole list without any error. It accepted only
# sequences indented under their key while the registry writes them flush, and
# every requiredSections list vanished from both projections while the resolver
# still exited 0. Compare declared lists against resolved ones.
docs_registry="$REPO_ROOT/bubbles/docs-registry.yaml"
docs_resolver="$REPO_ROOT/bubbles/scripts/docs-registry-resolve.sh"
if [[ -f "$docs_registry" && -x "$docs_resolver" ]]; then
  declared_sections="$( { grep -cE '^[[:space:]]+requiredSections:' "$docs_registry" || true; } )"
  resolved_sections="$( { bash "$docs_resolver" --framework-default 2>/dev/null || true; } | { grep -cE '^[[:space:]]+requiredSections:' || true; } )"
  if [[ "$declared_sections" -ne "$resolved_sections" ]]; then
    err "managed-doc requiredSections lost in resolution: $declared_sections declared in bubbles/docs-registry.yaml, $resolved_sections survived docs-registry-resolve.sh --framework-default"
    findings=$((findings + 1))
  fi
else
  info "docs registry or resolver not present (skipping check 4)"
fi

# ── Check 5: improvement-index rows stay scannable ────────────────────
#
# The index is a routing surface: id, state, owner, gate, gap codes, date. When
# a full delivery narrative is pasted into the Status cell the table stops being
# readable and the row becomes the only copy of that narrative. Long detail
# belongs in the IMP file, or in git history once the IMP is deleted on delivery.
#
# IMP-037 is the single documented exception. Its IMP file was deleted on
# delivery, so its 13k-character cell is now the only record; migrating it would
# risk losing an audit trail for zero functional gain. It is grandfathered by id
# rather than by raising the bound, so a NEW oversized row still fails.
imp_index="$REPO_ROOT/improvements/INDEX.md"
index_row_max=2500
index_row_grandfathered="IMP-037"
if [[ -f "$imp_index" ]]; then
  while IFS= read -r row; do
    [[ -n "$row" ]] || continue
    row_id="$(printf '%s' "$row" | awk -F'|' '{gsub(/^[[:space:]]+|[[:space:]]+$/,"",$2); print $2}')"
    [[ "$row_id" == "$index_row_grandfathered" ]] && continue
    row_len="${#row}"
    if [[ "$row_len" -gt "$index_row_max" ]]; then
      err "improvement-index row '$row_id' is $row_len chars (max $index_row_max); move the narrative into the IMP file and keep the row to id, state, owner, gate, gap codes and date"
      findings=$((findings + 1))
    fi
  done < <( { grep -E '^\| IMP-' "$imp_index" || true; } )
else
  info "improvements/INDEX.md not present (skipping check 5)"
fi

if [[ "$findings" -gt 0 ]]; then
  err "found $findings management-truth catalog omission(s)"
  exit 1
fi

info "OK — recipe catalog, adoption-profile help, documented counts, managed-doc sections, and improvement-index rows match the live inventory"
exit 0
