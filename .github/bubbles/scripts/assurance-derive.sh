#!/usr/bin/env bash
# Assurance Level Derivation Resolver (IMP-100 Phase 3 choke point #1 — derivation half)
# ---------------------------------------------------------------------------
# Mechanical, FAIL-CLOSED derivation of the ACHIEVED assurance level of a
# delivered increment from its observable certification evidence. This is the
# DERIVATION half of the "certification derives + records achieved assurance"
# emitter (choke point #1 of the five deploy choke points). It COMPOSES with
# assurance-resolve.sh: this script answers "what level did we ACHIEVE?";
# assurance-resolve.sh answers "is that achieved level DEPLOY-eligible against
# the risk-derived floor?". They are deliberately separate (single
# responsibility) so the achieved level is derived ONCE, from evidence, and
# every deploy choke point consumes the same derivation instead of each
# re-inferring it (and getting it wrong).
#
# Assurance ordering: prototype < fast < full.
#
#   full      — the complete integrity chain: implementation complete AND full
#               test coverage AND all tests passing AND an independent audit
#               passed. Maps to terminal status `done`. missingForFull=none.
#   fast      — implementation complete AND full test coverage AND all tests
#               passing, but NO independent audit (the rapid-tool-delivery
#               achievement). Maps to `delivered_fast`. missingForFull=audit.
#   prototype — verification is incomplete or failing (missing test coverage OR
#               a failing test). A delivered spike; NEVER deployable. Maps to
#               `delivered_prototype`.
#
# FAIL-CLOSED bias: any incompleteness derives DOWN to the lower level. Missing
# implementation is not a level at all — it is an inconsistent certification
# attempt and exits 2 (there is no delivered increment to certify).
#
# Level ↔ terminal-state mapping: full ↔ `done`, fast ↔ `delivered_fast`,
# prototype ↔ `delivered_prototype` (matches assurance-resolve.sh + the R4
# tiered terminal-state model).
#
# Exit 0 always on a resolvable input; the derivation is on stdout as 5 lines:
#   achievedLevel=<full|fast|prototype>
#   terminalStatus=<done|delivered_fast|delivered_prototype>
#   riskClass=<low|high|unknown>
#   missingForFull=<none|comma-separated gaps>
#   reason=<why>
# Exit 2 = usage error / bad input / no-implementation (fail-closed: an
# un-resolvable input NEVER yields an achievedLevel).
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: assurance-derive.sh --implement-complete <true|false> \
                           --tests-complete    <true|false> \
                           --tests-passed      <true|false> \
                           --audit-complete    <true|false> \
                           [--risk-class <low|high|unknown>]

Prints:  achievedLevel=<full|fast|prototype>
         terminalStatus=<done|delivered_fast|delivered_prototype>
         riskClass=<low|high|unknown>
         missingForFull=<none|comma-separated gaps>
         reason=<why>

Derivation (fail-closed toward the lower level):
  full      = implement + full test coverage + all tests passing + audit passed
  fast      = implement + full test coverage + all tests passing, NO audit
  prototype = verification incomplete/failing (missing coverage OR a failing test)

Missing implementation exits 2 (no delivered increment to certify). Composes
with assurance-resolve.sh, which decides deploy-eligibility from the achieved
level and the risk-derived minimum floor.
EOF
}

implement=""
tests_complete=""
tests_passed=""
audit=""
risk_class="unknown"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --implement-complete) implement="${2:-}"; shift 2 ;;
    --tests-complete) tests_complete="${2:-}"; shift 2 ;;
    --tests-passed) tests_passed="${2:-}"; shift 2 ;;
    --audit-complete) audit="${2:-}"; shift 2 ;;
    --risk-class) risk_class="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "assurance-derive: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

require_bool() {
  # $1=value $2=flag-name
  case "$1" in
    true|false) ;;
    *) echo "assurance-derive: $2 must be true|false (got: '${1:-<empty>}')" >&2; exit 2 ;;
  esac
}

if [[ -z "$implement" || -z "$tests_complete" || -z "$tests_passed" || -z "$audit" ]]; then
  echo "assurance-derive: --implement-complete, --tests-complete, --tests-passed, and --audit-complete are all required" >&2
  usage >&2
  exit 2
fi
require_bool "$implement" "--implement-complete"
require_bool "$tests_complete" "--tests-complete"
require_bool "$tests_passed" "--tests-passed"
require_bool "$audit" "--audit-complete"
case "$risk_class" in
  low|high|unknown) ;;
  *) echo "assurance-derive: --risk-class must be low|high|unknown" >&2; exit 2 ;;
esac

# Missing implementation → not a deliverable level at all (fail-closed).
if [[ "$implement" != "true" ]]; then
  echo "assurance-derive: --implement-complete=false — no delivered increment to certify (inconsistent certification attempt)" >&2
  exit 2
fi

terminal_for_level() {
  case "$1" in
    full) printf 'done' ;;
    fast) printf 'delivered_fast' ;;
    prototype) printf 'delivered_prototype' ;;
  esac
}

decide() {
  local level="$1" reason="$2" missing="$3"
  printf 'achievedLevel=%s\n' "$level"
  printf 'terminalStatus=%s\n' "$(terminal_for_level "$level")"
  printf 'riskClass=%s\n' "$risk_class"
  printf 'missingForFull=%s\n' "$missing"
  printf 'reason=%s\n' "$reason"
  exit 0
}

# prototype: verification incomplete or failing → the assurance floor.
# (Uses explicit if/then/fi, NOT `[[ ]] && cmd`, to stay set -e safe.)
if [[ "$tests_complete" != "true" || "$tests_passed" != "true" ]]; then
  gaps=""
  if [[ "$tests_complete" != "true" ]]; then
    gaps="test-coverage-complete"
  fi
  if [[ "$tests_passed" != "true" ]]; then
    gaps="${gaps:+$gaps,}all-tests-passing"
  fi
  if [[ "$audit" != "true" ]]; then
    gaps="${gaps:+$gaps,}independent-audit"
  fi
  decide "prototype" \
    "verification incomplete/failing — delivered prototype (never deployable); reach full by closing: ${gaps}" \
    "$gaps"
fi

# full: the complete integrity chain incl. an independent audit.
if [[ "$audit" == "true" ]]; then
  decide "full" \
    "complete integrity chain (implementation + full test coverage + all tests passing + independent audit) — full assurance" \
    "none"
fi

# fast: verified (coverage + passing) but no independent audit.
decide "fast" \
  "implementation + full test coverage + all tests passing, but no independent audit — fast assurance (rapid-tool-delivery achievement)" \
  "independent-audit"
