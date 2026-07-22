#!/usr/bin/env bash
# claim-source-lint.sh — mechanical Claim-Source provenance enforcer
# (IMP-101 SCOPE-1 / AF-101 / gate G072).
#
# Gate G072 requires every evidence block that records a command outcome to carry
# a `**Claim Source:**` tag (executed | interpreted | not-run). The taxonomy has
# lived in policy (agents/bubbles_shared/evidence-rules.md) with NO mechanical
# check verifying it. This lint scans report.md evidence blocks and flags any
# execution-evidence block (one carrying a `**Exit Code:**` marker) that is
# missing the tag, or that carries an invalid tag value.
#
# Advisory-until-opt-in: by DEFAULT it prints findings and exits 0, so it can
# never break a currently-green tree or reject historical artifacts. Set
# `claimSourceProvenanceGuard: block` in .github/bubbles-project.yaml to make
# findings fail (exit 1). There is no other bypass.
#
# Usage:
#   claim-source-lint.sh [TARGET]   # TARGET dir searched for report.md; default .
#
# Exit 0 = clean, OR advisory mode. Exit 1 = findings AND block mode enabled.

set -euo pipefail

TARGET="${1:-.}"
TARGET="$(cd "$TARGET" && pwd)"

err() { echo "[claim-source-lint][ERROR] $*" >&2; }
info() { echo "[claim-source-lint] $*"; }

# ── advisory-until-opt-in: walk up for .github/bubbles-project.yaml ────
mode="advisory"
project_config=""
dir="$TARGET"
while :; do
  if [[ -f "$dir/.github/bubbles-project.yaml" ]]; then
    project_config="$dir/.github/bubbles-project.yaml"
    break
  fi
  [[ "$dir" == "/" || -z "$dir" ]] && break
  dir="$(dirname "$dir")"
done
if [[ -n "$project_config" ]] && grep -qE '^[[:space:]]*claimSourceProvenanceGuard:[[:space:]]*block[[:space:]]*$' "$project_config"; then
  mode="block"
fi

findings=0
while IFS= read -r report; do
  [[ -n "$report" ]] || continue
  out="$(awk '
    { lines[NR] = $0 }
    END {
      for (i = 1; i <= NR; i++) {
        if (lines[i] ~ /^\*\*Exit Code:\*\*/) {
          found = 0
          for (j = i - 3; j <= i + 4; j++)
            if (j >= 1 && j <= NR && lines[j] ~ /^\*\*Claim Source:\*\*/) { found = 1; break }
          if (!found) printf "%d\tmissing\n", i
        }
        if (lines[i] ~ /^\*\*Claim Source:\*\*/) {
          v = lines[i]
          sub(/^\*\*Claim Source:\*\*[[:space:]]*/, "", v)
          sub(/[[:space:]]+$/, "", v)
          if (v !~ /^(executed|interpreted|not-run)/) printf "%d\tinvalid:%s\n", i, v
        }
      }
    }
  ' "$report" || true)"
  [[ -n "$out" ]] || continue
  rel="${report#"$TARGET"/}"
  while IFS=$'\t' read -r ln kind; do
    [[ -n "$ln" ]] || continue
    if [[ "$kind" == "missing" ]]; then
      err "$rel:$ln execution-evidence block (Exit Code) missing **Claim Source:** tag"
    else
      err "$rel:$ln invalid **Claim Source:** value ('${kind#invalid:}') — expected executed|interpreted|not-run"
    fi
    findings=$((findings + 1))
  done <<< "$out"
done < <( { find "$TARGET" -name report.md -not -path "*/eval/fixtures/*" -not -path "*/node_modules/*" 2>/dev/null || true; } )

if [[ "$findings" -eq 0 ]]; then
  info "OK — every execution-evidence block carries a valid Claim Source tag"
  exit 0
fi

if [[ "$mode" == "block" ]]; then
  err "$findings Claim-Source provenance finding(s) — failing (claimSourceProvenanceGuard: block)"
  exit 1
fi
info "$findings Claim-Source provenance finding(s) — advisory only (exit 0). Set claimSourceProvenanceGuard: block in .github/bubbles-project.yaml to enforce."
exit 0
