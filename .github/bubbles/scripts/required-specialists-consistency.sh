#!/usr/bin/env bash
# bubbles/scripts/required-specialists-consistency.sh
#
# IMP-105 SCOPE-7 / ONT-UNIFY — shadow-compare guard.
#
# Enforces that the mode->required-specialists mapping is backed by a SINGLE
# canonical source. The mapping is DUPLICATED today: it lives as a hardcoded
# `case "$state_workflow_mode" in ... esac` in state-transition-guard.sh Check 6
# ("Specialist Phase Completion", Gate G022) — whose arms carry load-bearing
# inline documentation a data-only generator cannot reproduce byte-identically —
# AND as data in bubbles/registry/required-specialists.yaml. This guard proves
# they are IDENTICAL (same mode keys, same ORDERED specialist lists). Any drift
# — a mode in the guard case but not the registry, a mode in the registry but
# not the case, or a differing/reordered specialist list — prints the specific
# divergence to stderr and exits 1. Exact match exits 0.
#
# It reads Check 6's case block ONLY (it stops at the
# `# IMP-105-SCOPE-3-FALLBACK-BEGIN` marker, so the SCOPE-3 fallback region is
# never parsed as a case arm) and does NOT modify the guard. The guard's runtime
# behavior stays byte-identical; this guard just pins the case to the registry.
#
# Usage:
#   required-specialists-consistency.sh [--repo-root <path>]
#   required-specialists-consistency.sh --help
#
# No --skip / --force / bypass flag exists; a mapping change is made by editing
# BOTH the guard case AND the registry, never by skipping this check.
#
# Portable: bash 3.2 safe (no associative arrays), GNU/BSD-neutral awk only.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

repo_root="$REPO_ROOT_DEFAULT"

usage() {
  cat <<'USAGE'
required-specialists-consistency.sh — shadow-compare the state-transition-guard
Check 6 mode->required-specialists case against bubbles/registry/required-specialists.yaml.

Usage:
  required-specialists-consistency.sh [--repo-root <path>]
  required-specialists-consistency.sh --help

Exit codes:
  0  guard case and registry are identical (same modes, same ordered lists)
  1  drift detected (divergences printed to stderr)
  2  usage error / missing input / unparseable source

There is no --skip / --force / bypass flag. Change the mapping by editing BOTH
the guard Check 6 case AND the registry in the same change.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --repo-root)
      repo_root="${2:?--repo-root requires a path}"
      shift 2
      ;;
    --repo-root=*)
      repo_root="${1#*=}"
      shift
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

GUARD="$repo_root/bubbles/scripts/state-transition-guard.sh"
REGISTRY="$repo_root/bubbles/registry/required-specialists.yaml"

if [[ ! -f "$GUARD" ]]; then
  echo "error: guard not found: $GUARD" >&2
  exit 2
fi
if [[ ! -f "$REGISTRY" ]]; then
  echo "error: registry not found: $REGISTRY" >&2
  exit 2
fi

# --- Parse the guard's Check 6 case block into `mode spec1 spec2 ...` lines. ---
# Anchors on the exact `case "$state_workflow_mode" in` header, tracks the most
# recent `<mode>)` case label, and emits on each `required_specialists=(...)`
# assignment. Stops at the SCOPE-3 fallback marker so the fallback is never
# parsed as an arm. Comment lines between a case label and its assignment (e.g.
# the rapid-tool-delivery rationale) are skipped, so the label->assignment
# association survives them.
parse_guard() {
  awk '
    index($0, "# IMP-105-SCOPE-3-FALLBACK-BEGIN") { exit }
    index($0, "case \"$state_workflow_mode\" in") { in_case = 1; next }
    in_case == 1 {
      if ($0 ~ /^[[:space:]]*[a-z0-9][a-z0-9-]*\)[[:space:]]*$/) {
        m = $0
        sub(/^[[:space:]]*/, "", m)
        sub(/\).*$/, "", m)
        cur = m
        next
      }
      if (index($0, "required_specialists=(") > 0) {
        line = $0
        sub(/^[^(]*\(/, "", line)
        sub(/\).*$/, "", line)
        gsub(/"/, "", line)
        gsub(/[[:space:]]+/, " ", line)
        sub(/^ /, "", line)
        sub(/ $/, "", line)
        if (line != "" && cur != "") {
          print cur " " line
        }
      }
    }
  ' "$GUARD"
}

# --- Parse the registry into the same `mode spec1 spec2 ...` line shape. ---
# Prefers yq (hard dep of most guards here); degrades to a bounded awk parser
# for the flow-style `mode: [a, b, c]` lists when yq is absent.
parse_registry() {
  if command -v yq >/dev/null 2>&1; then
    yq -r '.modes | to_entries[] | .key + " " + (.value | join(" "))' "$REGISTRY" 2>/dev/null
  else
    echo "note: yq not found — using bounded awk YAML parser for $REGISTRY" >&2
    awk '
      /^modes:[[:space:]]*$/ { in_modes = 1; next }
      /^[a-zA-Z]/ { in_modes = 0 }
      in_modes == 1 {
        if ($0 ~ /^[[:space:]]+[a-z0-9][a-z0-9-]*:[[:space:]]*\[/) {
          key = $0
          sub(/^[[:space:]]*/, "", key)
          sub(/:.*$/, "", key)
          val = $0
          sub(/^[^[]*\[/, "", val)
          sub(/\].*$/, "", val)
          gsub(/,/, " ", val)
          gsub(/"/, "", val)
          gsub(/[[:space:]]+/, " ", val)
          sub(/^ /, "", val)
          sub(/ $/, "", val)
          if (key != "") {
            print key " " val
          }
        }
      }
    ' "$REGISTRY"
  fi
}

_tmp_base="${TMPDIR:-/tmp}"
work_dir="$(mktemp -d "${_tmp_base%/}/bubbles-reqspec-consistency.XXXXXX")"
cleanup() { rm -rf "$work_dir"; }
trap cleanup EXIT

guard_tsv="$work_dir/guard.txt"
registry_tsv="$work_dir/registry.txt"

parse_guard >"$guard_tsv"
parse_registry >"$registry_tsv"

guard_count="$(awk 'END { print NR }' "$guard_tsv")"
registry_count="$(awk 'END { print NR }' "$registry_tsv")"

if [[ "$guard_count" -eq 0 ]]; then
  echo "error: parsed zero mode arms from the guard Check 6 case in $GUARD" >&2
  echo "       (expected the 'case \"\$state_workflow_mode\" in' block before the SCOPE-3 fallback marker)" >&2
  exit 2
fi
if [[ "$registry_count" -eq 0 ]]; then
  echo "error: parsed zero modes from $REGISTRY (expected a 'modes:' map of flow-style lists)" >&2
  exit 2
fi

# --- Compare. Order-sensitive on each list, set-comparison on the mode keys. ---
if awk '
  FNR == NR {
    m = $1
    sub(/^[^ ]* */, "", $0)
    guard[m] = $0
    gorder[++gn] = m
    next
  }
  {
    m = $1
    sub(/^[^ ]* */, "", $0)
    reg[m] = $0
    rseen[m] = 1
  }
  END {
    rc = 0
    for (i = 1; i <= gn; i++) {
      m = gorder[i]
      if (!(m in reg)) {
        printf("DRIFT missing-in-registry: mode %s is in the guard Check 6 case (specialists=[%s]) but absent from required-specialists.yaml\n", m, guard[m]) > "/dev/stderr"
        rc = 1
        continue
      }
      if (guard[m] != reg[m]) {
        printf("DRIFT list-mismatch: mode %s guard=[%s] registry=[%s]\n", m, guard[m], reg[m]) > "/dev/stderr"
        rc = 1
      }
    }
    for (m in reg) {
      if (!(m in guard)) {
        printf("DRIFT extra-in-registry: mode %s is in required-specialists.yaml (specialists=[%s]) but absent from the guard Check 6 case\n", m, reg[m]) > "/dev/stderr"
        rc = 1
      }
    }
    exit rc
  }
' "$guard_tsv" "$registry_tsv"; then
  echo "required-specialists-consistency: OK — guard Check 6 case matches required-specialists.yaml ($guard_count modes reconciled)"
  exit 0
else
  echo "required-specialists-consistency: FAIL — guard Check 6 case and required-specialists.yaml diverge (see DRIFT lines above)" >&2
  exit 1
fi
