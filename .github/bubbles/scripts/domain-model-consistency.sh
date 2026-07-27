#!/usr/bin/env bash
# Gate G131: domain_model_consistency_gate
#
# ADVISORY-UNTIL-CONFIGURED consistency NUDGE that threads the always-available
# product-domain SST (the `domainModel:` block delivered by Gate G130) through a
# feature's own artifacts. Where G130 asks "is a declared invariant mechanically
# anchored?", G131 asks the sibling coherence question: "does this feature keep
# its domain declarations IN SYNC with the shared product-domain model, or is it
# siloing a per-feature model that should be promoted up?"
#
# It NUDGES (advisory, prints findings but exits 0 by default) when, for a repo
# that HAS opted into a `domainModel:` block:
#   (1) a feature's `design.md ## Data Model` declares an entity that is NOT
#       present in the shared `domainModel.entities` — nudge: promote the entity
#       into the shared product-domain model so every feature references one
#       source of truth (not a siloed per-feature model); AND
#   (2) an Outcome-Contract Hard Constraint (in `spec.md`/`design.md`) references
#       an `INV-*` id that is NOT declared in the shared `domainModel.invariants`
#       — nudge: declare the invariant in the shared model so G130 can anchor it.
#
# OPT-IN (advisory-until-configured): this gate is INERT unless a repo declares
# the OPTIONAL `domainModel:` block in `.github/bubbles-project.yaml` (the same
# sibling-of-`traceContracts:` block G130 consumes), inline OR as
# `domainModel: { $ref: config/domain-model.yaml }`. A repo with no `domainModel:`
# block (including the Bubbles source repo) is a CLEAN NO-OP (exit 0). The gate is
# also ADVISORY BY DEFAULT even when configured: findings are printed and it exits
# 0, so it never breaks a currently-green tree. It only exits non-zero (blocks)
# when a repo explicitly sets `domainModelConsistencyGuard: block` in
# `.github/bubbles-project.yaml` (the SAME advisory-until-opt-in shape as the
# G072 `claim-source-lint.sh` `claimSourceProvenanceGuard: block` key).
#
# WHY (feeds G044 a structured model): the regression conflict sweep
# (`agents/bubbles_shared/e2e-regression.md`, Gate G044) can now diff a spec's
# declared transition against the shared `domainModel` state machine (a structured
# target) instead of only grepping prose for "contradictory business rules".
#
# Parsing: mikefarah `yq` (v4) when present; otherwise a bounded awk/grep fallback
# that reads the canonical block-style `entities:` map keys and `invariants[].id`
# values and degrades to silence (no false nudge) on a shape it cannot parse.
# There is NO --skip/--force/bypass: a new promoted entity/invariant is declared
# in the shared `domainModel`, never skipped.
#
# Usage:
#   bash domain-model-consistency.sh <feature-dir> [--quiet]
#
# Exit codes:
#   0  clean, not applicable (no domainModel), OR findings in advisory mode
#   1  findings AND `domainModelConsistencyGuard: block` opt-in is set
#   2  runtime error / malformed input (missing / non-existent feature dir)
#
set -uo pipefail

quiet="false"
feature_dir=""

usage() {
  cat <<'EOF'
domain-model-consistency.sh — Gate G131 (domain_model_consistency_gate)

Advisory-until-configured consistency nudge for the shared product-domain SST.
INERT unless the repo declares a `domainModel:` block in
.github/bubbles-project.yaml (sibling of traceContracts:); a repo with no
domainModel: block is a clean no-op (exit 0). Advisory by default even when
configured (findings printed, exit 0); blocks (exit 1) ONLY when
`domainModelConsistencyGuard: block` is set. There is NO --skip/--force bypass.

Usage:
  bash domain-model-consistency.sh <feature-dir> [--quiet]

Exit codes:
  0  clean, not applicable (no domainModel), or findings in advisory mode
  1  findings AND domainModelConsistencyGuard: block opt-in is set
  2  runtime error / malformed input
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) quiet="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    --*) echo "domain-model-consistency: unknown option: $1" >&2; exit 2 ;;
    *)
      if [[ -n "$feature_dir" ]]; then
        echo "domain-model-consistency: only one feature dir may be supplied" >&2
        exit 2
      fi
      feature_dir="$1"
      shift
      ;;
  esac
done

if [[ -z "$feature_dir" ]]; then
  echo "domain-model-consistency: missing feature directory argument" >&2
  echo "Usage: bash domain-model-consistency.sh <feature-dir> [--quiet]" >&2
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "domain-model-consistency: feature directory not found: $feature_dir" >&2
  exit 2
fi

feature_dir_abs="$(cd "$feature_dir" 2>/dev/null && pwd -P)" || {
  echo "domain-model-consistency: cannot resolve feature directory: $feature_dir" >&2
  exit 2
}
feature_dir="$feature_dir_abs"

say() {
  [[ "$quiet" == "true" ]] && return 0
  echo "$1"
}

# BUBBLES_DOMAIN_MODEL_DISABLE_YQ is a TEST-ONLY seam: it forces the awk/grep
# fallback parser so the fallback path is genuinely exercised by the selftest.
# It changes only the PARSER, never the enforcement.
have_yq() {
  [[ -n "${BUBBLES_DOMAIN_MODEL_DISABLE_YQ:-}" ]] && return 1
  command -v yq >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Walk up from the feature dir for the nearest .github/bubbles-project.yaml
# (or bubbles-project.yaml). The directory that CONTAINS it is the project root.
# ---------------------------------------------------------------------------
find_project_config() {
  local dir="$feature_dir"
  while :; do
    if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then
      printf '%s\t%s\n' "$dir" "$dir/.github/bubbles-project.yaml"
      return 0
    fi
    if [[ -f "$dir/bubbles-project.yaml" ]]; then
      printf '%s\t%s\n' "$dir" "$dir/bubbles-project.yaml"
      return 0
    fi
    [[ "$dir" == "/" ]] && break
    dir="$(dirname "$dir")"
  done
  return 1
}

pc="$(find_project_config || true)"
if [[ -z "$pc" ]]; then
  say "ℹ️  G131: no .github/bubbles-project.yaml above $feature_dir — no domainModel declared; domain-model consistency check not applicable (advisory-until-configured no-op)"
  exit 0
fi
project_root="${pc%%$'\t'*}"
config_file="${pc#*$'\t'}"

config_has_domain_model() {
  if have_yq; then
    [[ "$(yq 'has("domainModel")' "$config_file" 2>/dev/null || echo false)" == "true" ]]
  else
    grep -qE '^domainModel:' "$config_file"
  fi
}

if ! config_has_domain_model; then
  say "ℹ️  G131: no domainModel block in $config_file — domain-model consistency check not applicable (advisory-until-configured no-op)"
  exit 0
fi

# ---------------------------------------------------------------------------
# Opt-in: advisory by default; blocks only when domainModelConsistencyGuard: block
# is set (mirrors the G072 claim-source-lint claimSourceProvenanceGuard: block key).
# ---------------------------------------------------------------------------
mode="advisory"
if grep -qE '^[[:space:]]*domainModelConsistencyGuard:[[:space:]]*block[[:space:]]*$' "$config_file"; then
  mode="block"
fi

# ---------------------------------------------------------------------------
# Resolve inline vs `$ref: config/domain-model.yaml`. Both resolve to model_file.
# ---------------------------------------------------------------------------
detect_ref() {
  if have_yq; then
    yq '.domainModel["$ref"] // ""' "$config_file" 2>/dev/null || true
  else
    awk '
      /^domainModel:/ { d = 1 }
      d && /\$ref:/ {
        line = $0
        sub(/.*\$ref:[[:space:]]*/, "", line)
        gsub(/[}"{[:space:]]/, "", line)
        gsub(/\047/, "", line)
        print line
        exit
      }
    ' "$config_file" 2>/dev/null || true
  fi
}

model_file="$config_file"
model_is_ref="false"
ref_path="$(detect_ref | tr -d '[:space:]')"
if [[ -n "$ref_path" ]]; then
  resolved_ref=""
  if [[ -f "$project_root/$ref_path" ]]; then
    resolved_ref="$project_root/$ref_path"
  elif [[ -f "$(dirname "$config_file")/$ref_path" ]]; then
    resolved_ref="$(dirname "$config_file")/$ref_path"
  elif [[ -f "$ref_path" ]]; then
    resolved_ref="$ref_path"
  fi
  if [[ -z "$resolved_ref" ]]; then
    say "⚠️  G131: domainModel.\$ref -> '$ref_path' not found (searched relative to $project_root and $(dirname "$config_file")); domain-model consistency check cannot proceed — treating as not applicable (advisory)"
    exit 0
  fi
  model_file="$resolved_ref"
  model_is_ref="true"
fi

spec_md="$feature_dir/spec.md"
design_md="$feature_dir/design.md"

# ---------------------------------------------------------------------------
# SHARED MODEL — entity keys (domainModel.entities) and invariant ids
# (domainModel.invariants[].id). yq-preferred, awk/grep fallback.
# ---------------------------------------------------------------------------
shared_entities_yq() {
  if [[ "$model_is_ref" == "true" ]]; then
    yq '((.entities // .domainModel.entities) // {}) | keys | .[]' "$model_file" 2>/dev/null || true
  else
    yq '(.domainModel.entities // {}) | keys | .[]' "$model_file" 2>/dev/null || true
  fi
}

# Bounded awk: read direct child keys of the first `entities:` block.
shared_entities_awk() {
  awk '
    function indent(s,   n) { match(s, /^[ ]*/); return RLENGTH }
    BEGIN { mode = 0; base = -1; child = -1 }
    mode == 0 && $0 ~ /^[[:space:]]*entities:[[:space:]]*$/ { base = indent($0); mode = 1; child = -1; next }
    mode == 1 {
      if ($0 ~ /^[[:space:]]*$/) next
      if ($0 ~ /^[[:space:]]*#/) next
      ind = indent($0)
      if (ind <= base) { mode = 0; next }
      if (child == -1) child = ind
      if (ind == child && $0 ~ /^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*:/) {
        key = $0
        sub(/^[[:space:]]*/, "", key)
        sub(/[[:space:]]*:.*/, "", key)
        gsub(/["\047]/, "", key)
        print key
      }
    }
  ' "$model_file" 2>/dev/null || true
}

shared_invariant_ids_yq() {
  if [[ "$model_is_ref" == "true" ]]; then
    yq '(((.invariants // .domainModel.invariants) // []))[] | (.id // "")' "$model_file" 2>/dev/null || true
  else
    yq '((.domainModel.invariants // []))[] | (.id // "")' "$model_file" 2>/dev/null || true
  fi
}

shared_invariant_ids_awk() {
  awk '
    /^[[:space:]]*invariants:[[:space:]]*$/ { inv = 1; next }
    inv && /^[^[:space:]#]/ { inv = 0 }
    inv && /^[[:space:]]*-[[:space:]]+id:/ {
      line = $0
      sub(/.*id:[[:space:]]*/, "", line)
      gsub(/["\047[:space:]]/, "", line)
      if (line != "") print line
    }
  ' "$model_file" 2>/dev/null || true
}

if have_yq; then
  shared_entities="$(shared_entities_yq)"
  shared_inv_ids="$(shared_invariant_ids_yq)"
else
  shared_entities="$(shared_entities_awk)"
  shared_inv_ids="$(shared_invariant_ids_awk)"
fi

# Normalization: entities compared case-insensitively, ignoring `_`/`-`/backticks
# and one trailing plural `s` (so Order == order == orders == `orders`).
normalize_entity() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -d '`_-' | sed -E 's/s$//'
}
# Invariant ids compared case-insensitively (canonical ids are UPPER already).
normalize_inv() {
  printf '%s' "$1" | tr '[:lower:]' '[:upper:]'
}

shared_entities_norm=""
while IFS= read -r e; do
  [[ -n "$e" ]] || continue
  shared_entities_norm+="$(normalize_entity "$e")"$'\n'
done <<< "$shared_entities"

shared_inv_norm=""
while IFS= read -r i; do
  [[ -n "$i" ]] || continue
  shared_inv_norm+="$(normalize_inv "$i")"$'\n'
done <<< "$shared_inv_ids"

# ---------------------------------------------------------------------------
# FEATURE — entities declared in design.md `## Data Model` (structured forms
# only: bold single-identifier tokens, and single-identifier / backticked-
# identifier `###`/`####` subheadings — prose section labels like "New Tables"
# are excluded, keeping the nudge high-signal).
# ---------------------------------------------------------------------------
datamodel_section() {
  [[ -f "$design_md" ]] || return 0
  awk '
    /^##[[:space:]]+Data Model/ { insec = 1; next }
    insec && /^##[[:space:]]/ { insec = 0 }
    insec { print }
  ' "$design_md"
}

feature_entities() {
  local section
  section="$(datamodel_section)"
  [[ -n "$section" ]] || return 0
  {
    printf '%s\n' "$section" | grep -oE '\*\*[A-Za-z_][A-Za-z0-9_]*\*\*' 2>/dev/null | sed -E 's/^\*\*//; s/\*\*$//'
    printf '%s\n' "$section" | awk '
      /^#{3,4}[[:space:]]+/ {
        h = $0
        sub(/^#{3,4}[[:space:]]+/, "", h)
        sub(/[[:space:]]+$/, "", h)
        gsub(/`/, "", h)
        if (h ~ /^[A-Za-z_][A-Za-z0-9_]*$/) print h
      }
    '
  } | sed '/^[[:space:]]*$/d' | sort -u
}

# ---------------------------------------------------------------------------
# FEATURE — INV-* ids referenced by an Outcome-Contract Hard Constraint, from
# the `## Outcome Contract` section of spec.md and design.md.
# ---------------------------------------------------------------------------
outcome_contract_inv_refs() {
  local f
  for f in "$spec_md" "$design_md"; do
    [[ -f "$f" ]] || continue
    awk '
      /^##[[:space:]]+Outcome Contract/ { insec = 1; next }
      insec && /^##[[:space:]]/ { insec = 0 }
      insec { print }
    ' "$f"
  done | grep -oE 'INV-[A-Za-z0-9][A-Za-z0-9_-]*' 2>/dev/null | sort -u
}

# ---------------------------------------------------------------------------
# Evaluate. Each unmatched entity / invariant is one advisory nudge.
# ---------------------------------------------------------------------------
findings=0

while IFS= read -r entity; do
  [[ -n "$entity" ]] || continue
  norm="$(normalize_entity "$entity")"
  [[ -n "$norm" ]] || continue
  if ! printf '%s' "$shared_entities_norm" | grep -Fxq "$norm"; then
    echo "🟡 G131 NUDGE: design.md '## Data Model' declares entity '$entity' which is NOT promoted to the shared domainModel.entities — promote it into the shared product-domain model so every feature references one source of truth instead of siloing a per-feature model."
    findings=$((findings + 1))
  fi
done <<< "$(feature_entities)"

while IFS= read -r inv; do
  [[ -n "$inv" ]] || continue
  up="$(normalize_inv "$inv")"
  [[ -n "$up" ]] || continue
  if ! printf '%s' "$shared_inv_norm" | grep -Fxq "$up"; then
    echo "🟡 G131 NUDGE: an Outcome-Contract Hard Constraint references invariant '$inv' which is NOT declared in the shared domainModel.invariants — declare it in the shared model so Gate G130 can check it is enforced-by-code or proved-by an adversarial test."
    findings=$((findings + 1))
  fi
done <<< "$(outcome_contract_inv_refs)"

# ---------------------------------------------------------------------------
# Verdict.
# ---------------------------------------------------------------------------
if [[ "$findings" -eq 0 ]]; then
  say "✅ G131: feature domain declarations are consistent with the shared domainModel (every '## Data Model' entity is promoted and every Hard-Constraint invariant is declared)."
  exit 0
fi

if [[ "$mode" == "block" ]]; then
  echo ""
  echo "G131: $findings domain-model consistency nudge(s) — FAILING (domainModelConsistencyGuard: block)."
  echo "Promote each unpromoted entity into domainModel.entities, and declare each referenced INV-* in domainModel.invariants, so the shared product-domain model stays the single source of truth (and G130 can anchor the invariants)."
  exit 1
fi

echo ""
echo "G131: $findings domain-model consistency nudge(s) — advisory only (exit 0). Promote the entities/invariants into the shared domainModel, or set 'domainModelConsistencyGuard: block' in .github/bubbles-project.yaml to enforce."
exit 0
