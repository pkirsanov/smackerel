#!/usr/bin/env bash
# bubbles/scripts/gate-vintage-guard.sh
#
# Answers one question for a spec: which gates did not exist yet when this spec
# was opened? (IMP-036 SCOPE-8)
#
# WHY THIS EXISTS
# Gate ids grew from 71 to 134 in four months, and about 176 specs across six
# repos carry reopen / recertification / sweep language. A catalogue-wide sweep
# reopens specs that were certified before the gate it now fails existed. The
# code did not change; only the rules did. That is the least defensible cost in
# the system, and it was invisible because nothing recorded a gate's vintage.
#
# DELIBERATELY ADVISORY. This tool reports; it does NOT auto-excuse a failing
# gate. Auto-grandfathering inside the transition guard would create a silent
# mechanism for excusing real defects, which is the opposite of what a framework
# built on anti-fabrication should ship. The decision to grandfather stays with a
# named owner, recorded on the spec.
#
# A spec's vintage is its FIRST COMMIT DATE, derived from git. Specs do not
# record a framework version, and adding one would have required migrating 598
# state files; the first-commit date is already true for every spec that exists.
#
# OVERRIDE. A spec may deliberately opt a newer gate back IN by listing it in
# state.json as:
#   "gateVintageOverrides": [
#     { "gate": "G133", "owner": "<name>", "reason": "<why this old spec must meet it>" }
#   ]
# An override is the exception. Overriding every gate is a sweep by another name.
#
# Usage:
#   bash bubbles/scripts/gate-vintage-guard.sh <specDir> [--failed "G057 G060"] [--json]
#
# Exit codes:
#   0 = reported successfully
#   2 = usage error, missing spec, or unreadable gate registry

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
GATES_FILE="${BUBBLES_GATES_FILE:-$ROOT_DIR/bubbles/registry/gates.yaml}"

SPEC=""
FAILED=""
AS_JSON="false"
die_usage() { printf 'gate-vintage-guard: %s\n' "$1" >&2; sed -n '31,35p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --failed) shift; FAILED="${1:-}" ;;
    --json) AS_JSON="true" ;;
    -h|--help) sed -n '2,36p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*|--bypass*)
      die_usage "bypass-shaped flag '$1' is not supported and never will be" ;;
    -*) die_usage "unknown flag '$1'" ;;
    *) [[ -n "$SPEC" ]] && die_usage "unexpected extra argument '$1'"; SPEC="$1" ;;
  esac
  shift
done

[[ -n "$SPEC" ]] || die_usage "a spec directory is required (this tool has no default surface)"
[[ -d "$SPEC" ]] || die_usage "spec directory does not exist: $SPEC"
[[ -f "$GATES_FILE" ]] || die_usage "gate registry not found: $GATES_FILE"

# --- spec vintage ------------------------------------------------------------
spec_opened=""
if command -v git >/dev/null 2>&1 && git -C "$SPEC" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  spec_opened="$(git -C "$SPEC" log --reverse --format=%ad --date=short -- . 2>/dev/null | head -1)"
fi
if [[ -z "$spec_opened" ]]; then
  printf '[gate-vintage-guard] cannot determine when %s was opened (no git history).\n' "$SPEC" >&2
  printf '[gate-vintage-guard] Reporting nothing rather than guessing a vintage.\n' >&2
  exit 0
fi

# --- gate vintages -----------------------------------------------------------
# Pairs of "Gxxx<TAB>YYYY-MM-DD" for every annotated gate.
vintages="$(awk '
  /^  G[0-9][0-9][0-9]:$/ { g=$0; sub(/^  /,"",g); sub(/:$/,"",g); next }
  g != "" && /^    sinceDate:/ { d=$2; gsub(/"/,"",d); print g "\t" d; g="" }
' "$GATES_FILE")"

# --- overrides ---------------------------------------------------------------
overrides=""
if [[ -f "$SPEC/state.json" ]]; then
  overrides="$(grep -oE '"gate"[[:space:]]*:[[:space:]]*"G[0-9]{3}"' "$SPEC/state.json" 2>/dev/null |
    grep -oE 'G[0-9]{3}' | LC_ALL=C sort -u)"
fi

newer=""
while IFS=$'\t' read -r g d; do
  [[ -n "$g" && -n "$d" ]] || continue
  # String compare is correct for ISO-8601 dates and needs no GNU date.
  if [[ "$d" > "$spec_opened" ]]; then
    printf '%s\n' "$overrides" | grep -qxF "$g" && continue
    newer="$newer $g"
  fi
done <<<"$vintages"

# Intersect with the caller's failing set when one was supplied.
out_of_vintage_failures=""
if [[ -n "$FAILED" ]]; then
  for f in $FAILED; do
    printf '%s' "$newer" | tr ' ' '\n' | grep -qxF "$f" && out_of_vintage_failures="$out_of_vintage_failures $f"
  done
fi

count_words() { printf '%s' "${1:-}" | tr ' ' '\n' | grep -c . 2>/dev/null || true; }

if [[ "$AS_JSON" == "true" ]]; then
  printf '{"schemaVersion":"gate-vintage/v1","spec":"%s","specOpened":"%s","gatesNewerThanSpec":%s,"outOfVintageFailures":%s,"overrides":%s}\n' \
    "$SPEC" "$spec_opened" "$(count_words "$newer")" "$(count_words "$out_of_vintage_failures")" \
    "$(printf '%s' "$overrides" | grep -c . 2>/dev/null || true)"
  exit 0
fi

printf '=== gate vintage for %s ===\n' "$SPEC"
printf '  spec opened          : %s\n' "$spec_opened"
printf '  gates newer than it  : %s\n' "$(count_words "$newer")"
[[ -n "$overrides" ]] && printf '  deliberate overrides : %s\n' "$(printf '%s' "$overrides" | tr '\n' ' ')"
if [[ -n "$FAILED" ]]; then
  if [[ -n "$out_of_vintage_failures" ]]; then
    printf '  FAILING but out of vintage:%s\n' "$out_of_vintage_failures"
    printf '  These gates did not exist when this spec was certified. Reopening the\n'
    printf '  spec for them is a rule change, not a defect. Applying one anyway needs a\n'
    printf '  named owner and a reason in state.json gateVintageOverrides.\n'
  else
    printf '  every failing gate predates this spec - all in vintage\n'
  fi
fi
exit 0
