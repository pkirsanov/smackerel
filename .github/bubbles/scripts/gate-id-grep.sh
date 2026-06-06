#!/usr/bin/env bash
# gate-id-grep.sh
#
# Scans governance docs and scripts for gate ID references of the form
# G[0-9]{3} and detects two failure modes:
#
#   (a) duplicate-adjacent: the same gate ID appears twice in a row,
#       separated only by spaces, commas, or both. Examples:
#           "G028, G028"          -> finding
#           "G044, G044, G044"    -> finding
#           "G028 G028"           -> finding
#       This catches accidental copy-paste regressions in lists like
#       "Gates G024, G025, G028".
#
#   (b) unknown gate ID: any G\d{3} reference whose ID is not present in
#       the canonical set extracted from bubbles/workflows.yaml. The
#       canonical set is built from every line in that file that contains
#       either "requiredGates" or "delivery-gate-baseline" — those are the
#       authoritative sources of which gates exist as workflow contracts.
#       Gate-ID bands: the framework RESERVES G001–G199 (these MUST resolve
#       to a canonical workflow contract). Project-local custom gates use
#       G900+ — any ID >= G900 is treated as a custom/local gate and is
#       always allowed (never reported as unknown). The G200–G899 span is
#       an intentional reserved gap so the two bands can never overlap.
#
# Modes:
#   default   - only fail on duplicate-adjacent findings
#   --strict  - also fail on unknown-gate-id findings
#
# Scanned roots (relative to --repo-root):
#   - agents/
#   - instructions/
#   - docs/
#   - bubbles/scripts/
#
# Exit codes:
#   0 - no findings (in the active mode)
#   1 - one or more findings printed
#   2 - usage error / missing inputs
#
# Usage:
#   bash bubbles/scripts/gate-id-grep.sh [--repo-root <path>] [--strict]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"
strict="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/gate-id-grep.sh [--repo-root <path>] [--strict]

Scans agents/, instructions/, docs/, bubbles/scripts/ under <repo-root>
for gate ID patterns (G[0-9]{3}). Detects:
  - duplicate-adjacent IDs (e.g., "G028, G028")
  - unknown gate IDs not present in workflows.yaml requiredGates lists
    (only reported under --strict; project-local custom gates G900+ are
    always allowed; framework gates G001-G199 must resolve canonically)

Options:
  --repo-root <path>   Repo root to scan (default: script repo root)
  --strict             Also fail on unknown-gate-id findings
  -h, --help           Print this help

Exit 0 when no findings in the active mode, 1 when findings exist.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      repo_root="${1:?--repo-root requires a path}"
      shift
      ;;
    --strict)
      strict="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "gate-id-grep: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$repo_root" ]]; then
  echo "gate-id-grep: repo root not found: $repo_root" >&2
  exit 2
fi

workflows_yaml="$repo_root/bubbles/workflows.yaml"
if [[ ! -f "$workflows_yaml" ]]; then
  echo "gate-id-grep: bubbles/workflows.yaml not found at $workflows_yaml" >&2
  exit 2
fi

# v6.1 (S2 true split): mode definitions (including each mode's requiredGates)
# live in bubbles/workflows/modes.yaml. Include it in the canonical-gate
# extraction so a gate referenced only by a mode is not flagged unknown.
gate_source_files=("$workflows_yaml")
modes_yaml="$repo_root/bubbles/workflows/modes.yaml"
[[ -f "$modes_yaml" ]] && gate_source_files+=("$modes_yaml")

# --- Build canonical gate ID set from workflows.yaml -----------------------
#
# Canonical sources are lines mentioning either "requiredGates" or
# "delivery-gate-baseline". We extract every G[0-9]{3} occurrence on those
# lines. Project-local custom gates (>= G900) are also implicitly allowed at
# lookup time; framework gates (G001-G199) must resolve canonically.

declare -A canonical_set=()
while IFS= read -r gate_id; do
  [[ -z "$gate_id" ]] && continue
  canonical_set["$gate_id"]=1
done < <(
  grep -E 'requiredGates|delivery-gate-baseline' "${gate_source_files[@]}" \
    | grep -oE '\bG[0-9]{3}\b' \
    | sort -u
)

if [[ "${#canonical_set[@]}" -eq 0 ]]; then
  echo "gate-id-grep: failed to extract any canonical gate IDs from $workflows_yaml" >&2
  exit 2
fi

# --- Discover scan targets -------------------------------------------------

declare -a scan_roots=()
for sub in agents instructions docs bubbles/scripts; do
  if [[ -d "$repo_root/$sub" ]]; then
    scan_roots+=("$repo_root/$sub")
  fi
done

if [[ "${#scan_roots[@]}" -eq 0 ]]; then
  echo "gate-id-grep: no scan roots present under $repo_root" >&2
  exit 2
fi

# --- Detect duplicate-adjacent findings ------------------------------------
#
# A duplicate-adjacent match is the same G\d{3} ID separated only by
# spaces and/or commas. Example regex match: "G028, G028" or "G044 G044".

dup_findings_file="$(mktemp)"
trap 'rm -f "$dup_findings_file" "$ref_findings_file" "$unknown_findings_file"' EXIT INT TERM
ref_findings_file="$(mktemp)"
unknown_findings_file="$(mktemp)"

# grep -P is needed for the back-reference \1; we constrain to text files
# via --binary-files=without-match and skip vendored .git dirs (find roots
# already exclude .git by virtue of not descending into them).
# NOTE: -P and -E are mutually exclusive in GNU grep; use -P only.
grep -rPn --binary-files=without-match \
  '\b(G[0-9]{3})\b[ ,]+\1\b' \
  "${scan_roots[@]}" \
  > "$dup_findings_file" 2>/dev/null || true

dup_count="$(wc -l < "$dup_findings_file" | tr -d ' ')"

# --- Detect unknown gate ID findings (used under --strict) -----------------
#
# Collect every <file>:<line>:<G\d{3}> reference, then for each ID check
# canonical membership. Project-local custom gates (>= G900) are always allowed.

grep -rPno --binary-files=without-match \
  '\bG[0-9]{3}\b' \
  "${scan_roots[@]}" \
  > "$ref_findings_file" 2>/dev/null || true

while IFS=: read -r file line id; do
  [[ -z "$id" ]] && continue
  # Project-local custom gates (>= G900) are always allowed; the framework
  # band G001-G199 must resolve canonically (G200-G899 is the reserved gap).
  numeric="${id#G}"
  numeric="${numeric#0}"
  numeric="${numeric#0}"
  if [[ -z "$numeric" ]]; then
    numeric=0
  fi
  if (( 10#$numeric >= 900 )); then
    continue
  fi
  if [[ -z "${canonical_set[$id]:-}" ]]; then
    printf '%s:%s:%s\n' "$file" "$line" "$id" >> "$unknown_findings_file"
  fi
done < "$ref_findings_file"

unknown_count="$(wc -l < "$unknown_findings_file" | tr -d ' ')"

# --- Report ---------------------------------------------------------------

active_failures=0

if [[ "$dup_count" -gt 0 ]]; then
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf 'FINDING: duplicate-adjacent: %s\n' "$line"
  done < "$dup_findings_file"
  active_failures=$((active_failures + dup_count))
fi

if [[ "$strict" == "true" && "$unknown_count" -gt 0 ]]; then
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    printf 'FINDING: unknown-gate-id: %s\n' "$line"
  done < "$unknown_findings_file"
  active_failures=$((active_failures + unknown_count))
fi

if [[ "$active_failures" -eq 0 ]]; then
  echo "[gate-id-grep] OK — zero findings"
  exit 0
fi

# Print a brief summary footer to make pipeline logs easier to scan.
if [[ "$strict" == "true" ]]; then
  echo "[gate-id-grep] FAIL — duplicate-adjacent: $dup_count, unknown: $unknown_count (strict)" >&2
else
  echo "[gate-id-grep] FAIL — duplicate-adjacent: $dup_count" >&2
fi
exit 1
