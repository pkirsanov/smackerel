#!/usr/bin/env bash
# gate-strength-lint.sh — publishes the enforcement-strength taxonomy of the gate
# registry (IMP-101 SCOPE-11 / GATE-101).
#
# The headline "N registered gates" count conflates very different controls:
# mechanical blockers, mechanical advisories, behavioral (agent/prompt) contracts,
# conditional no-ops, and externally enforced gates. Counting them as equivalent
# obscures where assurance actually comes from. This guard derives each gate's
# enforcement strength by a TRANSPARENT, DETERMINISTIC heuristic over the gate's
# own declared description and publishes the counts by class, so the registry's
# real assurance profile is visible instead of a flat total.
#
# Classes (as declared by each gate's description):
#   mechanical-blocking  — a script blocks on violation ("BLOCKING", "MUST", ...)
#   mechanical-advisory  — a script reports but does not block ("advisory")
#   behavioral-contract  — enforced by agent/prompt behavior, not a script
#   conditional-noop     — applies only conditionally; a no-op otherwise
#   external-enforcement — enforced by CI / git hooks / external systems
#
# This is a description-DECLARED classification: it is deterministic and auditable
# (re-run it and diff), not a hand-asserted authoritative audit. Reconciling each
# declared enforcer against the check that actually runs (the G072/Check-12 class
# of mismatch) is a separate, deeper audit and is NOT claimed here.
#
# Usage:
#   gate-strength-lint.sh [REPO_ROOT]            # publish counts; fail if a gate
#                                                # cannot be classified
#   gate-strength-lint.sh --list [REPO_ROOT]     # print "GID<TAB>class" per gate
#
# Exit 0 = every gate classified (or gates.yaml / yq absent → skip).
# Exit 1 = a gate could not be classified (registry parse problem). No --force.

set -euo pipefail

LIST_MODE=0
if [[ "${1:-}" == "--list" ]]; then
  LIST_MODE=1
  shift
fi
REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
GATES="$REPO_ROOT/bubbles/registry/gates.yaml"

err() { echo "[gate-strength-lint][ERROR] $*" >&2; }
info() { echo "[gate-strength-lint] $*"; }

[[ -f "$GATES" ]] || { info "gates.yaml not present at $GATES (skipping)"; exit 0; }
command -v yq >/dev/null 2>&1 || { info "yq not available — skipping gate-strength taxonomy"; exit 0; }

classify_description() {
  local d
  d="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  if printf '%s' "$d" | grep -qE "not by (a )?script|agent behavior|behaviorally|prompt-behavior|enforced by [a-z.]*agent|by bubbles\.[a-z]+ agent"; then
    printf 'behavioral-contract'
  elif printf '%s' "$d" | grep -qE "\bci\b|git hook|pre-push hook|pre-commit hook|external"; then
    printf 'external-enforcement'
  elif printf '%s' "$d" | grep -qE "advisory|informational|non-blocking"; then
    printf 'mechanical-advisory'
  elif printf '%s' "$d" | grep -qE "no-op|skipped when|does not apply|not enforced when"; then
    printf 'conditional-noop'
  elif printf '%s' "$d" | grep -qE "blocking|must |required|absolute"; then
    printf 'mechanical-blocking'
  else
    printf 'behavioral-contract'
  fi
}

registry_gate_count="$( { grep -cE "^  G[0-9]+:" "$GATES" || true; } )"

declare -A counts
classified=0
unclassified=0

while IFS=$'\t' read -r gid desc; do
  [[ -n "$gid" ]] || continue
  cls="$(classify_description "$desc")"
  if [[ -z "$cls" ]]; then
    err "gate $gid could not be classified"
    unclassified=$((unclassified + 1))
    continue
  fi
  classified=$((classified + 1))
  counts[$cls]=$(( ${counts[$cls]:-0} + 1 ))
  if [[ "$LIST_MODE" -eq 1 ]]; then
    printf '%s\t%s\n' "$gid" "$cls"
  fi
done < <(yq -r '.gates | to_entries | .[] | .key + "\t" + (.value.description | sub("\n";" "))' "$GATES")

if [[ "$LIST_MODE" -eq 1 ]]; then
  exit 0
fi

info "gate enforcement-strength taxonomy (description-declared) for $classified gate(s):"
for c in mechanical-blocking mechanical-advisory behavioral-contract conditional-noop external-enforcement; do
  printf '  %-22s %s\n' "$c" "${counts[$c]:-0}"
done

if [[ "$unclassified" -gt 0 ]]; then
  err "$unclassified gate(s) could not be classified"
  exit 1
fi
if [[ "$classified" -ne "$registry_gate_count" ]]; then
  err "classified $classified gate(s) but the registry declares $registry_gate_count — a gate description failed to parse"
  exit 1
fi

info "OK — all $classified registered gates classified by enforcement strength"
exit 0
