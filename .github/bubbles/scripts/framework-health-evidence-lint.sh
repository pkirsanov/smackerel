#!/usr/bin/env bash
# framework-health-evidence-lint.sh — mechanical enforcer for Gate G125
# (framework_health_evidence_gate).
#
# WHY THIS EXISTS (IMP-027 / REG-2)
# --------------------------------
# G125 is declared BLOCKING in bubbles/registry/gates.yaml and states that
# `bubbles.retro target: framework` MUST emit an improvement proposal under
# improvements/IMP-NNN-<slug>.md that references the input data sources it
# analyzed, and MUST NOT auto-mutate bubbles/*, agents/*, or
# bubbles/workflows.yaml.
#
# Before this script, `grep -rln 'G125' bubbles/scripts/*.sh` returned ZERO
# results. The only related script, retro-framework-health.sh, is the GENERATOR
# — it writes a conforming proposal. A generator cannot enforce a gate: it only
# constrains the artifacts it happens to produce. A hand-authored proposal, a
# later edit that strips the provenance, or a commit that lands a proposal
# together with framework mutations were all invisible.
#
# This script is the independent VERIFIER. It reads the artifacts that exist and
# re-derives whether each one satisfies G125.
#
# CHECKS
#   1. index-exists        improvements/INDEX.md is present.
#   2. status-declared     each IMP carries a "**Status:**" line whose value is
#                          one of the statuses INDEX.md's legend defines.
#   3. sources-cited       each IMP cites its evidence: a canonical runtime input
#                          (framework-events.jsonl / workflow-runs.json /
#                          capability-ledger.yaml), a "## Provenance" section, or
#                          a Motivation naming a commit SHA.
#   4. index-row-present   each IMP has a row in INDEX.md.
#   5. generator-contained retro-framework-health.sh writes ONLY under
#                          improvements/. This is G125's literal guarantee and
#                          has NO exemption.
#   6. proposal-traceable  a commit that ADDS an IMP and also mutates bubbles/*
#                          or agents/* must name that IMP in its message, so the
#                          mutation is attributable to a reviewed proposal.
#
# WHY CHECK 6 IS TRACEABILITY AND NOT A FLAT PROHIBITION
# -----------------------------------------------------
# G125 forbids the PROPOSAL from auto-mutating the framework. The subject of
# that sentence is the generator, which Check 5 pins with no escape hatch. A
# human commit that lands an approved scope and introduces the contract it
# implements is not auto-mutation; forbidding it outright would flag legitimate
# work while doing nothing about the automated path G125 actually targets.
# Check 6 therefore requires the link to be explicit: an unattributed mutation
# riding along with a new proposal still fails.
#
# Exit codes:
#   0 — all improvements satisfy G125 (or none exist yet)
#   1 — one or more findings
#   2 — usage / environment error
#
# Usage:
#   bash bubbles/scripts/framework-health-evidence-lint.sh [--repo-root <path>] [--quiet]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
quiet=0

usage() {
  cat <<'EOF'
framework-health-evidence-lint.sh — mechanical enforcer for Gate G125

Usage:
  bash bubbles/scripts/framework-health-evidence-lint.sh [options]

Options:
  --repo-root <path>   Repo root to inspect (default: script repo root)
  --quiet              Suppress the OK sentinel; still prints findings
  -h, --help           Show this help

Exit: 0 clean - 1 findings - 2 usage/environment error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --quiet)
      quiet=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "framework-health-evidence-lint: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "framework-health-evidence-lint: repo root not found: $repo_root" >&2
  exit 2
fi

improvements_dir="$repo_root/improvements"
index_file="$improvements_dir/INDEX.md"

findings=0

report() {
  printf 'FINDING: %s: %s\n' "$1" "$2"
  findings=$((findings + 1))
}

# An absent improvements/ directory is legitimate: a downstream repo that has
# never run framework-health has nothing to enforce. Only a PRESENT directory
# carries obligations.
if [[ ! -d "$improvements_dir" ]]; then
  [[ "$quiet" -eq 1 ]] || echo "[framework-health-evidence-lint] OK — no improvements/ directory (nothing to enforce)"
  exit 0
fi

# --- Check 1: INDEX.md exists ----------------------------------------------
#
# TEMPLATE.md instructs authors to "add a row to improvements/INDEX.md". If the
# index does not exist that instruction is unfollowable and proposals become
# undiscoverable.
if [[ ! -f "$index_file" ]]; then
  report "index-missing" "improvements/INDEX.md does not exist but improvements/ does (TEMPLATE.md requires an index row per proposal)"
fi

# Collect proposals. TEMPLATE.md and INDEX.md are infrastructure, not proposals.
declare -a imp_files=()
while IFS= read -r f; do
  [[ -n "$f" ]] && imp_files+=("$f")
done < <(find "$improvements_dir" -maxdepth 1 -type f -name 'IMP-*.md' 2>/dev/null | LC_ALL=C sort)

if [[ "${#imp_files[@]}" -eq 0 ]]; then
  if [[ "$findings" -eq 0 ]]; then
    [[ "$quiet" -eq 1 ]] || echo "[framework-health-evidence-lint] OK — no IMP proposals present"
    exit 0
  fi
  echo "[framework-health-evidence-lint] FAIL — findings: $findings"
  exit 1
fi

# Statuses recognised by the index legend. Derived from INDEX.md so the lint
# cannot drift from the documented vocabulary.
valid_statuses=""
if [[ -f "$index_file" ]]; then
  valid_statuses="$(grep -oE '^\| `[A-Z ]+`' "$index_file" 2>/dev/null | tr -d '|`' | sed 's/^ *//; s/ *$//' | LC_ALL=C sort -u || true)"
fi
[[ -n "$valid_statuses" ]] || valid_statuses=$'PROPOSED\nACCEPTED\nIN PROGRESS\nAPPLIED\nSUPERSEDED\nREJECTED'

# git is only needed for Check 5; its absence degrades that check, not the run.
have_git=0
if command -v git >/dev/null 2>&1 && git -C "$repo_root" rev-parse --git-dir >/dev/null 2>&1; then
  have_git=1
fi

for imp in "${imp_files[@]}"; do
  rel="${imp#"$repo_root"/}"
  base="$(basename "$imp")"

  # --- Check 2: a declared Status ------------------------------------------
  status_line="$(grep -m1 -oE '^\*\*Status:\*\*[[:space:]]*[A-Z][A-Z ]*' "$imp" 2>/dev/null || true)"
  if [[ -z "$status_line" ]]; then
    report "status-missing" "$rel has no '**Status:** <STATUS>' line (G125 proposals must declare review state)"
  else
    status_value="$(printf '%s' "$status_line" | sed -E 's/^\*\*Status:\*\*[[:space:]]*//; s/[[:space:]]+$//')"
    if ! printf '%s\n' "$valid_statuses" | grep -qxF "$status_value"; then
      report "status-unknown" "$rel declares Status '$status_value' which is not in the INDEX.md legend"
    fi
  fi

  # --- Check 3: evidence citation ------------------------------------------
  #
  # G125 requires the proposal to reference the input data it analyzed. Three
  # shapes are legitimate: a canonical runtime input (auto-generated retro), an
  # explicit Provenance section, or a Motivation naming the commit audited.
  cites_evidence=0
  if grep -qF 'framework-events.jsonl' "$imp" 2>/dev/null ||
    grep -qF 'workflow-runs.json' "$imp" 2>/dev/null ||
    grep -qF 'capability-ledger.yaml' "$imp" 2>/dev/null; then
    cites_evidence=1
  elif grep -qE '^## Provenance' "$imp" 2>/dev/null; then
    cites_evidence=1
  elif grep -qE '^\*\*Motivation:\*\*.*\b[0-9a-f]{7,40}\b' "$imp" 2>/dev/null; then
    cites_evidence=1
  fi
  if [[ "$cites_evidence" -eq 0 ]]; then
    report "sources-uncited" "$rel cites no input data source (expected a runtime input path, a '## Provenance' section, or a Motivation naming an audited commit SHA)"
  fi

  # --- Check 4: discoverable from the index --------------------------------
  if [[ -f "$index_file" ]] && ! grep -qF "$base" "$index_file" 2>/dev/null; then
    report "index-row-missing" "$rel has no row in improvements/INDEX.md"
  fi

  # --- Check 6: proposal traceability --------------------------------------
  #
  # Scoped to the commit that ADDED the file. If that commit also mutated the
  # framework, its message must name this IMP so the change is attributable to
  # a reviewed proposal. Unattributed co-mutation is exactly the smuggling
  # G125 exists to prevent.
  if [[ "$have_git" -eq 1 ]]; then
    add_commit="$(git -C "$repo_root" log --diff-filter=A --format=%H -1 -- "$rel" 2>/dev/null || true)"
    if [[ -n "$add_commit" ]]; then
      mutated="$(git -C "$repo_root" show --name-only --format= "$add_commit" 2>/dev/null |
        grep -E '^(bubbles/|agents/)' |
        grep -vE '^bubbles/release-manifest\.json$' || true)"
      if [[ -n "$mutated" ]]; then
        imp_id="$(printf '%s' "$base" | grep -oE '^IMP-[0-9]+' || true)"
        commit_msg="$(git -C "$repo_root" log -1 --format=%B "$add_commit" 2>/dev/null || true)"
        if [[ -z "$imp_id" ]] || ! printf '%s' "$commit_msg" | grep -qF "$imp_id"; then
          mutated_list="$(printf '%s' "$mutated" | tr '\n' ' ' | sed 's/ *$//')"
          report "proposal-untraceable" "$rel was introduced in $add_commit which mutated governed paths without naming ${imp_id:-the proposal} in its message: $mutated_list"
        fi
      fi
    fi
  fi
done

# --- Check 5: generator containment (NO exemption) --------------------------
#
# G125's registry description asserts enforcement "by bubbles/scripts/
# retro-framework-health.sh writing only to improvements/". Nothing verified
# that assertion. Re-derive it: every filesystem write the generator performs
# must target its output directory. A redirect, tee, or mkdir aimed at
# bubbles/, agents/, or workflows.yaml is auto-mutation by definition.
generator="$repo_root/bubbles/scripts/retro-framework-health.sh"
if [[ -f "$generator" ]]; then
  stray_writes="$(grep -nE '(>>?[[:space:]]*"?\$?\{?(REPO_ROOT|repo_root)[^"]*/(bubbles|agents)/|tee[[:space:]]+[^|]*/(bubbles|agents)/|workflows\.yaml"?[[:space:]]*$)' "$generator" 2>/dev/null |
    grep -vE '^[[:space:]]*[0-9]+:[[:space:]]*#' || true)"
  if [[ -n "$stray_writes" ]]; then
    while IFS= read -r line; do
      [[ -n "$line" ]] && report "generator-writes-outside-improvements" "retro-framework-health.sh:$line"
    done <<<"$stray_writes"
  fi
fi

if [[ "$findings" -gt 0 ]]; then
  echo "[framework-health-evidence-lint] FAIL — G125 findings: $findings"
  exit 1
fi

[[ "$quiet" -eq 1 ]] || echo "[framework-health-evidence-lint] OK — ${#imp_files[@]} proposal(s) satisfy G125"
exit 0
