#!/usr/bin/env bash
# bubbles/scripts/orphaned-scaffolding-guard.sh
#
# Capability: registry-consolidation-safety
#
# The orphaned-scaffolding corollary, made mechanical (IMP-047 S-A).
#
# WHY THIS EXISTS
# `gates-block-reader-lint.sh` guarded a generated `gates:` block in
# `bubbles/workflows.yaml`. That block was removed. The lint then computed, on
# every run, that its own inventory was empty, printed "the block is safe to
# remove" and "the inventory is empty; SCOPE-13 removal precondition is met" --
# and exited 0. Because it PASSED, `framework-validate.sh` kept scheduling it,
# and 442 lines of apparatus across a script, a selftest, a registry file and two
# scheduled registrations survived long after the thing they guarded was gone.
#
# The lesson is not "notice sooner". A precondition that depends on a human
# reading a PASS line carefully is not a precondition. A check that has reported
# its own obsolescence must stop being green, so a scheduler surfaces it the same
# way it surfaces any other refusal.
#
# THE RULE
# A script under bubbles/scripts/ that EMITS a removal-precondition declaration
# must route it through `bubbles_removal_precondition_met` from guard-lib.sh,
# which returns 3 and can never report success. Emitting the declaration on a
# path that can still exit 0 is a finding.
#
# HOW EMISSION IS DETECTED
# Only OUTPUT statements count -- a line invoking `printf`, `echo` or `cat`
# whose text carries a phrase from the closed set below. Prose ABOUT the rule is
# not an emission of it, so full-line comments are stripped first; without that,
# this guard's own documentation and every changelog entry describing the pattern
# would be findings, which is how a guard becomes noise and then gets disabled.
#
# The phrase set is CLOSED and small on purpose. A broad "sounds obsolete"
# matcher would be the same guesswork that produced the reverted inventory in the
# first place. A new phrasing is added here deliberately, in review.
#
# Usage:
#   bash bubbles/scripts/orphaned-scaffolding-guard.sh [--repo-root DIR] [--list]
#
# Exit codes:
#   0 = no unrouted removal-precondition declaration
#   1 = one or more findings printed
#   2 = usage error, or a required input could not be read

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="orphaned-scaffolding-guard"
MODE="check"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/orphaned-scaffolding-guard.sh [--repo-root DIR] [--list]

Refuses a lint that announces its own removal precondition and still exits 0.
Such a declaration must be routed through `bubbles_removal_precondition_met`
(guard-lib.sh), which returns 3 and can never report success.

Options:
  --repo-root DIR   Repo root to scan (default: this script's repo root)
  --list            Print every routed and unrouted declaration found
  -h, --help        Print this help

Exit 0 clean - 1 findings - 2 usage/environment error.
There is no --skip/--force/--ignore: a check that guards nothing is deleted.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:-}"
      [[ -n "$REPO_ROOT" ]] || {
        printf '%s: --repo-root requires a path\n' "$NAME" >&2
        exit 2
      }
      ;;
    --list)
      MODE="list"
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This guard has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
  shift || true
done

SCRIPTS_DIR="$REPO_ROOT/bubbles/scripts"
if [[ ! -d "$SCRIPTS_DIR" ]]; then
  printf '%s: scripts directory not found: %s\n' "$NAME" "$SCRIPTS_DIR" >&2
  exit 2
fi

# The closed declaration set. Each phrase is a script stating that the surface it
# guards no longer exists.
DECLARATION_RE='removal precondition is met|safe to remove|guards nothing|no longer guards|inventory is empty|nothing can depend on'

# The sanctioned routing helper. A line that calls it is compliant by definition.
ROUTER='bubbles_removal_precondition_met'

# guard-lib.sh DEFINES the router, so its own emission is the routed one. This
# guard and its selftest necessarily quote the phrases they detect.
is_exempt_file() {
  case "${1##*/}" in
    guard-lib.sh | "$NAME.sh" | "$NAME-selftest.sh") return 0 ;;
  esac
  return 1
}

findings=0
routed=0

while IFS= read -r script_path; do
  [[ -f "$script_path" ]] || continue
  [[ -r "$script_path" ]] || continue
  is_exempt_file "$script_path" && continue
  relative_path="${script_path#"$REPO_ROOT"/}"

  # Strip full-line comments before matching: prose about the rule is not an
  # emission of it. Line numbers are preserved by blanking rather than deleting.
  while IFS= read -r hit; do
    [[ -n "$hit" ]] || continue
    line_no="${hit%%:*}"
    line_text="${hit#*:}"
    if [[ "$line_text" == *"$ROUTER"* ]]; then
      routed=$((routed + 1))
      [[ "$MODE" == "list" ]] && printf '%s: routed: %s:%s\n' "$NAME" "$relative_path" "$line_no"
      continue
    fi
    findings=$((findings + 1))
    printf 'FINDING: unrouted-removal-precondition: %s:%s\n' "$relative_path" "$line_no"
    printf '  %s\n' "$(printf '%s' "$line_text" | sed -E 's/^[[:space:]]+//')"
    printf '  This check declares that the surface it guards is gone, then can still\n'
    printf '  exit 0. Route the declaration through %s (guard-lib.sh),\n' "$ROUTER"
    printf '  which returns 3, or delete the check.\n'
  done < <(
    sed -E 's/^[[:space:]]*#.*$//' "$script_path" \
      | grep -nEi "(printf|echo|cat)[^|]*($DECLARATION_RE)|$ROUTER" || true
  )
done < <(find "$SCRIPTS_DIR" -maxdepth 2 -type f -name '*.sh' 2>/dev/null | LC_ALL=C sort)

if [[ "$MODE" == "list" ]]; then
  printf '%s: %s routed declaration(s), %s unrouted\n' "$NAME" "$routed" "$findings"
fi

if [[ "$findings" -gt 0 ]]; then
  printf '%s: FAIL (%s unrouted removal-precondition declaration(s))\n' "$NAME" "$findings" >&2
  exit 1
fi

printf '%s: OK — zero unrouted removal-precondition declarations (%s routed)\n' "$NAME" "$routed"
exit 0
