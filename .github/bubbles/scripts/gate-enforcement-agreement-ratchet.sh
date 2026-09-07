#!/usr/bin/env bash
# bubbles/scripts/gate-enforcement-agreement-ratchet.sh
#
# Ratchet for `gateEnforcement.derived[].agreement` (IMP-058 SCOPE-6 / REG-23).
#
# WHY THIS EXISTS
# `generate-gate-enforcement.sh` measures whether each gate's hand-written
# `enforcedBy` declaration matches the evidence: 72 of 121 gates disagree
# (`divergent` or `contradiction`) as of this scope. Fixing all 72 in one pass
# is the mass rewrite IMP-058 SCOPE-6 explicitly declines to attempt — most of
# them share one root cause (a `guard-check:NN` declaration never matches a
# derived `script:` reference syntactically, even when both name the same
# enforcer; `state-transition-guard.sh` alone accounts for a large share of
# the 72) and deserve a single, separately-scoped generator fix rather than 70
# individual hand edits done under this scope's evidence review.
#
# What this script prevents is regression while that backlog sits open: a NEW
# gate joining the disagreeing set with nobody noticing. The set is frozen in
# a baseline; `--check` fails only on an id that disagrees now but was not
# already known to. A baselined id that stops disagreeing is reported as
# stale so the baseline can only shrink, mirroring the ratchet
# agent-id-enum-lint.sh (IMP-036 SCOPE-7) established for the agent enum.
#
# The baseline lives beside the registry it describes, not per-consuming-repo:
# `gateEnforcement.derived` is generated FROM bubbles/registry/gates.yaml in
# THIS repo, so unlike agent-id-enum-lint's per-repo executionHistory baseline,
# there is exactly one baseline and it ships with the framework.
#
# Usage:
#   bash bubbles/scripts/gate-enforcement-agreement-ratchet.sh [--repo-root <path>] [--verbose]
#   bash bubbles/scripts/gate-enforcement-agreement-ratchet.sh [--repo-root <path>] --update-baseline
#
# Exit codes:
#   0 = no new disagreement beyond the baseline (or baseline updated)
#   1 = a gate disagrees now that the baseline does not already record
#   2 = usage error, missing registry, or a bypass-shaped flag

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VERBOSE="false"
UPDATE_BASELINE="false"

usage() {
  cat <<'USAGE'
usage: gate-enforcement-agreement-ratchet.sh [--repo-root <path>] [--verbose]
       gate-enforcement-agreement-ratchet.sh [--repo-root <path>] --update-baseline

Fails only when a gate's declared/derived enforcement disagreement
(`agreement: divergent` or `agreement: contradiction` in the generated
gateEnforcement.derived block) is NOT already recorded in the baseline. A
baselined gate id that no longer disagrees is reported as stale; remove it by
regenerating the baseline.

There is no --skip, --force or --ignore flag. A newly acceptable disagreement
is recorded by regenerating the baseline deliberately, never by bypassing the
check.
USAGE
}

die_usage() {
  printf 'gate-enforcement-agreement-ratchet: %s\n' "$1" >&2
  usage >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root) shift; REPO_ROOT="${1:?--repo-root requires a path}" ;;
    --verbose) VERBOSE="true" ;;
    --update-baseline) UPDATE_BASELINE="true" ;;
    -h|--help) usage; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*|--bypass*|--allow*)
      die_usage "bypass-shaped flag '$1' is not supported and never will be" ;;
    *) die_usage "unknown argument '$1'" ;;
  esac
  shift
done

GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
BASELINE_FILE="${BUBBLES_GATE_AGREEMENT_BASELINE_FILE:-$REPO_ROOT/bubbles/registry/gate-enforcement-agreement.baseline}"

[[ -f "$GATES" ]] || die_usage "gate registry not found: $GATES"

count_lines() { printf '%s' "${1:-}" | grep -c . 2>/dev/null || true; }

# --- current disagreeing set, read from the GENERATED block -----------------
# Line-oriented on purpose (this framework's generators avoid a hard PyYAML
# dependency); each generated row is one gate id on one line, which the
# generator itself guarantees.
current="$(grep -oE '^    G[0-9]{3}: \{[^}]*agreement: (divergent|contradiction)' "$GATES" 2>/dev/null \
  | grep -oE '^    G[0-9]{3}' | tr -d ' ' | LC_ALL=C sort -u || true)"

baseline=""
[[ -f "$BASELINE_FILE" ]] && baseline="$(grep -vE '^\s*(#|$)' "$BASELINE_FILE" 2>/dev/null | LC_ALL=C sort -u)"

if [[ "$UPDATE_BASELINE" == "true" ]]; then
  mkdir -p "$(dirname "$BASELINE_FILE")" 2>/dev/null || true
  {
    printf '# gate-enforcement-agreement.baseline (IMP-058 SCOPE-6)\n'
    printf '# Gate ids whose gateEnforcement.derived agreement is divergent or\n'
    printf '# contradiction, frozen so the ratchet can run without a mass rewrite.\n'
    printf '# This file may only SHRINK. Never add a new id here to silence a failure\n'
    printf '# -- resolve the gate'"'"'s declared enforcedBy against real evidence instead.\n'
    printf '# Regenerate deliberately: gate-enforcement-agreement-ratchet.sh --update-baseline\n'
    printf '%s\n' "$current"
  } >"$BASELINE_FILE"
  printf 'gate-enforcement-agreement-ratchet: baseline updated with %s id(s) at %s\n' \
    "$(count_lines "$current")" "${BASELINE_FILE#"$REPO_ROOT"/}"
  exit 0
fi

new_disagreement=""
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  printf '%s\n' "$baseline" | grep -qxF "$id" && continue
  new_disagreement="$new_disagreement  $id"$'\n'
done <<EOF
$current
EOF

stale=""
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  printf '%s\n' "$current" | grep -qxF "$id" && continue
  stale="$stale  $id"$'\n'
done <<EOF
$baseline
EOF

printf '[gate-enforcement-agreement-ratchet] %s gate(s) currently disagree (divergent/contradiction); %s baselined\n' \
  "$(count_lines "$current")" "$(count_lines "$baseline")"

if [[ -n "$stale" ]]; then
  printf '[gate-enforcement-agreement-ratchet] %s baseline entr(y/ies) no longer disagree - remove them:\n' \
    "$(count_lines "$stale")"
  printf '%s' "$stale"
fi

if [[ "$VERBOSE" == "true" ]]; then
  printf '[gate-enforcement-agreement-ratchet] baseline file: %s\n' "${BASELINE_FILE#"$REPO_ROOT"/}"
fi

if [[ -n "$new_disagreement" ]]; then
  printf '\n[gate-enforcement-agreement-ratchet] FAIL: gate id(s) disagree now but are not baselined:\n' >&2
  printf '%s' "$new_disagreement" >&2
  printf '\nEither correct the declared enforcedBy against real evidence (preferred), or\n' >&2
  printf 'if the disagreement is accepted debt, record it deliberately:\n' >&2
  printf '  bash %s --update-baseline\n' "${0#"$REPO_ROOT"/}" >&2
  exit 1
fi

printf '[gate-enforcement-agreement-ratchet] OK - no gate disagrees beyond the recorded baseline\n'
exit 0
