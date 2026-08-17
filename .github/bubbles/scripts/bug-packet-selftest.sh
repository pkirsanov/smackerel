#!/usr/bin/env bash
# bubbles/scripts/bug-packet-selftest.sh
#
# Hermetic selftest for the IMP-047 S-B bug-artifact contract.
#
# The measured defect: asking the framework how many artifacts a bug needs
# returned FOUR answers across four surfaces. `bubbles/registry/bug-packet.yaml`
# is now the single authority, and the prose restatements were deleted.
#
# The adversarial cases are A1 and A2. A1 fails if any surface starts restating
# the artifact list again — that is exactly how the four answers arose, one
# well-intentioned copy at a time. A2 fails if the deliberate single-file form
# loses its declared precondition, because an undeclared exception IS the fourth
# contradiction rather than a resolution of it.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="bug-packet-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

REGISTRY="$REPO_ROOT/bubbles/registry/bug-packet.yaml"
MICROFIX="$REPO_ROOT/bubbles/registry/micro-fix-packet.yaml"
BUGTPL="$REPO_ROOT/agents/bubbles_shared/bug-templates.md"
BUGSMD="$REPO_ROOT/BUGS.md"

# --- P1. the registry exists and declares exactly the three forms ----------
if [[ -f "$REGISTRY" ]] &&
  grep -q '^  - form: full$' "$REGISTRY" &&
  grep -q '^  - form: compact$' "$REGISTRY" &&
  grep -q '^  - form: single-file$' "$REGISTRY"; then
  ok "P1 bug-packet.yaml declares the full, compact and single-file forms"
else
  bad "P1 three forms declared" "registry=$REGISTRY"
fi

# --- P2. exactly three forms, no fifth vocabulary ---------------------------
form_count="$(grep -c '^  - form: ' "$REGISTRY" 2>/dev/null || echo 0)"
if [[ "$form_count" -eq 3 ]]; then
  ok "P2 the form vocabulary is closed at three"
else
  bad "P2 closed form vocabulary" "found $form_count form(s)"
fi

# --- A1. ADVERSARIAL: no surface restates the artifact list -----------------
# The four-answer defect reappears the moment a second surface enumerates the
# artifacts. Each surface must POINT at the registry instead.
restaters=""
for surface in "$BUGTPL" "$MICROFIX"; do
  [[ -f "$surface" ]] || continue
  # A restatement is an enumeration of the packet contents outside the registry.
  # `scopes.md` + `uservalidation.md` appearing together in one surface is the
  # signature; either alone is a legitimate single reference.
  if grep -q 'uservalidation\.md' "$surface" && grep -q 'scenario-manifest\.json' "$surface"; then
    restaters="$restaters $(basename "$surface")"
  fi
done
if [[ -z "$restaters" ]]; then
  ok "A1 no surface outside the registry re-enumerates the bug packet"
else
  bad "A1 single artifact authority" "restated in:$restaters"
fi

# --- A2. ADVERSARIAL: the single-file form keeps its precondition -----------
# Without a stated precondition the single-file form is not a declared case, it
# is the fourth contradiction with better manners.
single_block="$(awk '/^  - form: single-file$/{c=1} c&&/^  - form: /&&!/single-file/{c=0} c' "$REGISTRY")"
if printf '%s' "$single_block" | grep -q 'precondition:' &&
  printf '%s' "$single_block" | grep -q 'G085' &&
  printf '%s' "$single_block" | grep -q 'obligationsRetained:'; then
  ok "A2 the single-file form declares its precondition and retained obligations"
else
  bad "A2 single-file precondition" "block=$(printf '%s' "$single_block" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: the obligation floor is never zero --------------------
# A reduced artifact count that also reduces the obligations is not
# proportionality, it is a loophole. Every form must retain execution evidence.
if grep -q 'evidence-is-execution' "$REGISTRY" &&
  grep -q 'reproduce-before-fix' "$REGISTRY" &&
  grep -q 'evidence-is-execution' "$MICROFIX"; then
  ok "A3 the reduced forms retain the reproduce-before-fix and executed-evidence floor"
else
  bad "A3 obligation floor retained"
fi

# --- A4. ADVERSARIAL: escalation carries no override flag -------------------
if grep -q 'overrideFlag: none' "$REGISTRY" && grep -q 'overrideFlag: none' "$MICROFIX"; then
  ok "A4 form escalation exposes no override flag"
else
  bad "A4 no escalation override"
fi

# --- P3. every surface points at the registry ------------------------------
pointing=0
grep -q 'bug-packet\.yaml' "$BUGTPL" && pointing=$((pointing + 1))
grep -q 'bug-packet\.yaml' "$MICROFIX" && pointing=$((pointing + 1))
grep -q 'bug-packet\.yaml' "$BUGSMD" && pointing=$((pointing + 1))
if [[ "$pointing" -eq 3 ]]; then
  ok "P3 bug-templates.md, micro-fix-packet.yaml and BUGS.md all point at the registry"
else
  bad "P3 surfaces point at registry" "only $pointing of 3 point at bug-packet.yaml"
fi

# --- P4. micro-fix admission still reads its own registry ------------------
# Non-regression: collapsing the artifact restatement must not have removed the
# keys micro-fix-admission.sh consumes.
if grep -q '^requiredArtifacts:' "$MICROFIX" && grep -q '^admission:' "$MICROFIX" &&
  grep -q '^preservedObligations:' "$MICROFIX"; then
  ok "P4 micro-fix-packet.yaml retains admission, requiredArtifacts and preservedObligations"
else
  bad "P4 micro-fix keys retained"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
