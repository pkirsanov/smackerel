#!/usr/bin/env bash
# bubbles/scripts/micro-fix-admission.sh
#
# Capability: bug-packet-proportionality
#
# Enforce the micro-fix packet contract (IMP-042 SCOPE-9; activated IMP-047 S-D).
#
# WHY THIS EXISTS
# The full bug packet is seven artifacts. Paying that for a one-line guard does
# not buy assurance, it buys a thinner report written under time pressure. The
# compact packet trades artifact count for a NARROWER admission window -- but a
# proportionality rule with no mechanical check is just a suggestion, and the
# first defect to slip through it will be the one that was "obviously trivial".
#
# So this script refuses the compact packet whenever any admission condition
# fails, and refuses it again if the assurance floor (reproduce-before-fix,
# adversarial regression, stated root cause) is missing. There is no override:
# a discretionary downgrade is how a payment defect ships as a typo fix.
#
# ACTIVATED AS THE DEFAULT (IMP-047 S-D). The compact route was previously
# opt-in per bug, pending measurements of authoring time and defect escape that
# no producer existed for -- a precondition the system cannot produce is a
# permanent veto dressed as rigour. A bug answering every admission condition
# admissibly now uses the compact packet by default; any failed condition
# escalates AUTOMATICALLY to the full packet with no reviewer discretion and no
# override, and that mechanical escalation is what makes the default safe.
# Outcomes are recorded FORWARD through micro-fix-outcome-log.sh.
#
# The admission conditions live in bubbles/registry/micro-fix-packet.yaml. This
# script READS them. It does not restate them, because a second copy is a second
# answer.
#
# Usage:
#   bash bubbles/scripts/micro-fix-admission.sh <bugDir>
#   bash bubbles/scripts/micro-fix-admission.sh --registry   # print the contract
#
# Exit codes:
#   0 = the bug is on the full packet, or it is on the compact packet and conforms
#   1 = compact packet REFUSED (reason printed)
#   2 = usage error, or the registry is missing

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="$SCRIPT_DIR/../registry/micro-fix-packet.yaml"
NAME="micro-fix-admission"

usage() {
  cat <<'USAGE'
usage: micro-fix-admission.sh <bugDir>
       micro-fix-admission.sh --registry

Resolves the packet route for a bug and enforces the compact packet's admission
conditions and preserved obligations.

The compact packet is the DEFAULT route (IMP-047 S-D). A bug answering every
admission condition admissibly uses it; any failed condition escalates
AUTOMATICALLY to the full packet; a bug that answers nothing stays on the full
packet untouched.

No --force, --skip or --allow flag exists, and escalation has no override. A
failed condition is resolved by editing the bug, never by silencing the check.
USAGE
}

# Registry readers. Kept deliberately simple: the registry is a fixed shallow
# shape, so awk is enough and yq stays an optional convenience rather than a
# hard dependency that would make this guard unavailable where it matters.
registry_ids() {
  awk '/^admission:/{a=1;next} /^[a-z]/{a=0} a && /^  - id:/{print $3}' "$REGISTRY"
}
registry_required_artifacts() {
  awk '/^requiredArtifacts:/{a=1;next} /^[a-z]/{a=0} a && /^  - /{sub(/^  - /,"");print}' "$REGISTRY"
}
registry_obligation_ids() {
  awk '/^preservedObligations:/{a=1;next} /^[a-z]/{a=0} a && /^  - id:/{print $3}' "$REGISTRY"
}
registry_admit_when() {
  awk -v want="$1" '
    /^admission:/{a=1;next} /^[a-z]/{a=0}
    a && $0 ~ "^  - id: " want "$" {f=1;next}
    f && /^    admitWhen:/{gsub(/"/,"");print $2;exit}
    f && /^  - id:/{exit}
  ' "$REGISTRY"
}

[[ $# -ge 1 ]] || { usage >&2; exit 2; }
case "$1" in
  -h | --help)
    usage
    exit 0
    ;;
  --registry)
    [[ -f "$REGISTRY" ]] || {
      printf '%s: registry not found: %s\n' "$NAME" "$REGISTRY" >&2
      exit 2
    }
    cat "$REGISTRY"
    exit 0
    ;;
  --*)
    # Bypass-shaped flags are rejected by name so a future caller cannot invent
    # one and have it silently ignored.
    printf '%s: unsupported flag "%s". This guard has no bypass.\n' "$NAME" "$1" >&2
    exit 2
    ;;
esac

BUG_DIR="$1"
[[ -d "$BUG_DIR" ]] || {
  printf '%s: not a directory: %s\n' "$NAME" "$BUG_DIR" >&2
  exit 2
}
[[ -f "$REGISTRY" ]] || {
  printf '%s: registry not found: %s\n' "$NAME" "$REGISTRY" >&2
  exit 2
}

STATE="$BUG_DIR/state.json"
BUG_MD="$BUG_DIR/bug.md"

# --- route resolution (IMP-047 S-D: the compact packet is the DEFAULT) -------
#
# The compact route used to require an explicit `"packet": "micro"` opt-in,
# pending measurements of authoring time and defect escape that no producer
# existed for. A precondition the system cannot produce is a permanent veto, so
# the ordering is inverted: the route is on, and the measurement runs forward
# from activation through micro-fix-outcome-log.sh.
#
# Resolution, in order:
#   1. state.json says "packet": "full"  -> full, no enforcement here.
#   2. state.json says "packet": "micro" -> compact, enforced.
#   3. no declaration                    -> DEFAULT. Every admission condition
#      answered admissibly in bug.md means compact; any failed condition
#      escalates automatically to the full packet; no answers at all means the
#      bug has not been assessed and stays on the full packet untouched.
#
# Escalation is mechanical in every branch. There is no reviewer discretion and
# no override flag, which is exactly what makes turning the default on safe: a
# payment defect cannot ship as a typo fix because nobody has the authority to
# wave it through.
declared_packet=""
if [[ -f "$STATE" ]]; then
  if grep -q '"packet"[[:space:]]*:[[:space:]]*"micro"' "$STATE" 2>/dev/null; then
    declared_packet="micro"
  elif grep -q '"packet"[[:space:]]*:[[:space:]]*"full"' "$STATE" 2>/dev/null; then
    declared_packet="full"
  fi
fi

log_route() {
  local route="$1" resolution="$2" failed="${3:-}"
  local logger="$SCRIPT_DIR/micro-fix-outcome-log.sh"
  [[ -f "$logger" ]] || return 0
  bash "$logger" route --bug "$BUG_DIR" --route "$route" \
    --resolution "$resolution" --failed "$failed" 2>/dev/null || true
}

# Which admission conditions does bug.md actually answer, and how?
answered=0
failed_ids=""
for id in $(registry_ids); do
  want="$(registry_admit_when "$id")"
  line="$(grep -i -m1 "micro-fix-admission:[[:space:]]*${id}[[:space:]]*=" "$BUG_MD" 2>/dev/null)"
  if [[ -z "$line" ]]; then
    continue
  fi
  answered=$((answered + 1))
  got="$(printf '%s' "$line" | sed -E 's/.*=[[:space:]]*([A-Za-z]+).*/\1/' | tr '[:upper:]' '[:lower:]')"
  [[ "$got" == "$want" ]] || failed_ids="$failed_ids $id"
done
failed_ids="${failed_ids# }"

if [[ "$declared_packet" == "full" ]]; then
  printf '[%s] %s declares packet: full - nothing to enforce.\n' "$NAME" "$BUG_DIR"
  log_route full declared
  exit 0
fi

if [[ -z "$declared_packet" ]]; then
  if [[ "$answered" -eq 0 ]]; then
    printf '[%s] %s answers no admission condition - it uses the full bug packet.\n' "$NAME" "$BUG_DIR"
    log_route full unassessed
    exit 0
  fi
  if [[ -n "$failed_ids" ]]; then
    printf '[%s] %s fails admission (%s) - it escalates automatically to the full bug packet.\n' \
      "$NAME" "$BUG_DIR" "$failed_ids"
    printf '[%s] Escalation is mechanical. There is no reviewer discretion and no override flag.\n' "$NAME"
    log_route full escalated "$failed_ids"
    exit 0
  fi
  printf '[%s] %s passes every admission condition - the COMPACT packet is the default route.\n' "$NAME" "$BUG_DIR"
fi

refusals=0
refuse() {
  refusals=$((refusals + 1))
  printf '  REFUSED (%s): %s\n' "$1" "$2"
}

if [[ "$declared_packet" == "micro" ]]; then
  printf '[%s] %s declares packet: micro. Checking admission.\n' "$NAME" "$BUG_DIR"
fi

# 1. Required artifacts must exist. Fewer than the full packet, never zero.
for artifact in $(registry_required_artifacts); do
  [[ -f "$BUG_DIR/$artifact" ]] ||
    refuse "missing-artifact" "the compact packet still requires $artifact"
done

# 2. Every admission condition must be ANSWERED in bug.md and answered the way
#    the registry admits. An unanswered condition is a refusal, not a default:
#    silence is how the inconvenient question gets skipped.
for id in $(registry_ids); do
  want="$(registry_admit_when "$id")"
  line="$(grep -i -m1 "micro-fix-admission:[[:space:]]*${id}[[:space:]]*=" "$BUG_MD" 2>/dev/null)"
  if [[ -z "$line" ]]; then
    refuse "unanswered" "$id has no 'micro-fix-admission: $id = yes|no' line in bug.md"
    continue
  fi
  got="$(printf '%s' "$line" | sed -E 's/.*=[[:space:]]*([A-Za-z]+).*/\1/' | tr '[:upper:]' '[:lower:]')"
  if [[ "$got" != "$want" ]]; then
    refuse "escalate" "$id answered '$got', compact packet admits only '$want' - use the full bug packet"
  fi
done

# 3. The assurance floor. These are the obligations proportionality may never
#    trade away, so they are checked by name against report.md and bug.md.
report="$BUG_DIR/report.md"
for oid in $(registry_obligation_ids); do
  case "$oid" in
    reproduce-before-fix)
      grep -qi 'reproduc' "$report" 2>/dev/null ||
        refuse "floor" "report.md shows no reproduction before the fix"
      ;;
    adversarial-regression)
      # Both runs must be present. A test that only passes after the fix proves
      # the suite is green, not that it would have caught the defect.
      if ! grep -qi 'fails without the fix\|fail without fix\|before fix' "$report" 2>/dev/null ||
        ! grep -qi 'passes with the fix\|pass with fix\|after fix' "$report" 2>/dev/null; then
        refuse "floor" "report.md must show the regression test failing WITHOUT the fix and passing WITH it"
      fi
      ;;
    root-cause-stated)
      grep -qi 'root cause' "$BUG_MD" 2>/dev/null ||
        refuse "floor" "bug.md does not state a root cause"
      ;;
    evidence-is-execution)
      grep -qiE 'exit code|exit status|\$\?' "$report" 2>/dev/null ||
        refuse "floor" "report.md carries no exit code, so its claims are not execution evidence"
      ;;
  esac
done

if [[ "$refusals" -eq 0 ]]; then
  printf '[%s] admitted: compact packet is proportionate for this defect.\n' "$NAME"
  log_route compact "${declared_packet:-default}"
  exit 0
fi

printf '\n[%s] %d refusal(s). This bug uses the FULL packet (%s).\n' \
  "$NAME" "$refusals" "$(awk '/^escalation:/{e=1;next} e && /^  target:/{print $2;exit}' "$REGISTRY")"
printf '[%s] Fix the bug artifacts or escalate. There is no override flag.\n' "$NAME"
log_route full refused "$failed_ids"
exit 1
