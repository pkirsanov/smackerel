#!/usr/bin/env bash
# Assurance Deploy-Eligibility Resolver (IMP-100 Phase 2 R4d / Phase 3 R5)
# ---------------------------------------------------------------------------
# Mechanical, FAIL-CLOSED resolver for whether a delivered increment is
# DEPLOY-ELIGIBLE, given its ACHIEVED assurance level and the risk-derived
# MINIMUM assurance floor. It encodes the two security-relevant invariants of
# the tiered terminal-state model (IMP-100 R4/R5) in ONE testable place so the
# five deploy choke points (certification, release-delivery reconciliation,
# train cut/promote, build-manifest attestation, adapter preflight) all consume
# the SAME decision and cannot each re-derive it (and get it wrong — the
# "delivered_prototype silently ships" hole):
#
#   1. `prototype` tier is NEVER deployable — full stop.
#   2. `fast` tier is deployable ONLY when it meets the risk-derived minimum
#      assurance floor (a high-risk change requires `full`; `fast` under a
#      `full` floor is refused).
#   3. `full` always meets the floor.
#
# Assurance ordering: prototype < fast < full. The minimum floor is `fast` or
# `full` (never `prototype`). It COMPOSES risk-tier-resolve.sh, which emits the
# `riskClass` + `minimumAssurance` this resolver consumes. Defense in depth: a
# `high` OR `unknown` riskClass forces the effective floor to `full` regardless
# of the passed `--minimum-assurance`, so a mis-passed floor can never let
# high-risk/unknown work deploy at `fast`.
#
# Level ↔ terminal-state mapping: full ↔ `done`, fast ↔ `delivered_fast`,
# prototype ↔ `delivered_prototype`.
#
# Exit 0 always (a resolver, not a gate); the decision is on stdout as 4 lines:
#   deployEligible=<true|false>
#   achievedLevel=<full|fast|prototype>
#   minimumAssurance=<full|fast>   (the EFFECTIVE floor after risk escalation)
#   reason=<why>
# Exit 2 = usage error / unknown level (fail-closed: an un-resolvable input
# NEVER yields deployEligible=true).
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: assurance-resolve.sh --achieved-level <full|fast|prototype> \
                            --minimum-assurance <full|fast> \
                            [--risk-class <low|high|unknown>]

Prints:  deployEligible=<true|false>
         achievedLevel=<full|fast|prototype>
         minimumAssurance=<full|fast>   (effective floor after risk escalation)
         reason=<why>

Invariants: prototype is NEVER deployable; fast deploys only when it meets the
floor; full always meets it. A high/unknown riskClass forces the floor to full.
Fail-closed: an unknown level or bad usage exits 2 (never deployEligible=true).
EOF
}

achieved=""
minimum=""
risk_class=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --achieved-level) achieved="${2:-}"; shift 2 ;;
    --minimum-assurance) minimum="${2:-}"; shift 2 ;;
    --risk-class) risk_class="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "assurance-resolve: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$achieved" || -z "$minimum" ]]; then
  echo "assurance-resolve: --achieved-level and --minimum-assurance are required" >&2
  usage >&2
  exit 2
fi
case "$achieved" in
  full|fast|prototype) ;;
  *) echo "assurance-resolve: --achieved-level must be full|fast|prototype" >&2; exit 2 ;;
esac
case "$minimum" in
  full|fast) ;;
  *) echo "assurance-resolve: --minimum-assurance must be full|fast (never prototype)" >&2; exit 2 ;;
esac
if [[ -n "$risk_class" ]]; then
  case "$risk_class" in
    low|high|unknown) ;;
    *) echo "assurance-resolve: --risk-class must be low|high|unknown" >&2; exit 2 ;;
  esac
fi

# Defense in depth: high OR unknown risk forces the effective floor to full,
# regardless of the passed minimum (a mis-passed floor cannot create a hole).
effective_minimum="$minimum"
escalated=""
if [[ ( "$risk_class" == "high" || "$risk_class" == "unknown" ) && "$minimum" != "full" ]]; then
  effective_minimum="full"
  escalated=" (floor escalated to full because riskClass=$risk_class)"
fi

decide() {
  local eligible="$1" reason="$2"
  printf 'deployEligible=%s\n' "$eligible"
  printf 'achievedLevel=%s\n' "$achieved"
  printf 'minimumAssurance=%s\n' "$effective_minimum"
  printf 'reason=%s\n' "$reason"
  exit 0
}

rank() {
  case "$1" in
    prototype) printf '0' ;;
    fast) printf '1' ;;
    full) printf '2' ;;
    *) printf '-1' ;;
  esac
}

# 1) prototype is never deployable.
if [[ "$achieved" == "prototype" ]]; then
  decide "false" "prototype tier is never deployable (throwaway assurance) — refuse at every deploy choke point"
fi

# 2) achieved must meet the effective minimum floor.
if [[ "$(rank "$achieved")" -lt "$(rank "$effective_minimum")" ]]; then
  decide "false" "achieved assurance '$achieved' is below the required minimum '$effective_minimum'$escalated — refuse (raise assurance to '$effective_minimum' before deploy)"
fi

# 3) meets the floor → deploy-eligible.
decide "true" "achieved assurance '$achieved' meets the required minimum '$effective_minimum'$escalated"
