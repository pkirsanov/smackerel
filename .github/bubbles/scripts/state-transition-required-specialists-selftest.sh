#!/usr/bin/env bash
# bubbles/scripts/state-transition-required-specialists-selftest.sh
#
# IMP-105 SCOPE-3 — proves Check 6 ("Specialist Phase Completion", Gate G022)
# in state-transition-guard.sh no longer FAILS OPEN for a workflow mode that is
# ABSENT from the guard's explicit per-mode `required_specialists` case table.
# A mode missing from that table used to leave required_specialists empty, so
# Check 6 imposed ZERO specialist-completion enforcement (the historical
# rapid-tool-delivery bug). The SCOPE-3 fallback derives a safe non-empty set
# for any unlisted mode.
#
# MECHANISM (documented honestly):
#   The guard's contract resolver (transition-contract-resolver.sh) REJECTS an
#   unknown/synthetic mode and block_contract-exits(2) BEFORE Check 6 ever runs,
#   so a *full* real-guard invocation against a purely synthetic mode cannot
#   reach the fallback without also patching the resolver — which is out of
#   SCOPE-3. Instead this selftest exercises the REAL fallback bytes: it EXTRACTS
#   the exact block delimited by the IMP-105-SCOPE-3-FALLBACK-BEGIN/END sentinels
#   from the live guard source and executes it verbatim (via eval) against the
#   REAL mode-resolver.sh + guard-lib.sh, driven by a self-contained synthetic
#   BUBBLES_WORKFLOWS_FILE fixture (the canonical fixture pattern proven by
#   mode-resolver-selftest.sh). This runs the guard's ACTUAL derivation code —
#   not a hand-copied replica — and proves:
#     (1) an unlisted DELIVERY mode derives a NON-EMPTY set = phaseOrder ∩ core-
#         specialists, INCLUDING implement/test/validate/docs and EXCLUDING
#         control/conditional phases (select, bootstrap, devops, finalize);
#     (2) an unlisted READ-ONLY mode (no core phase in its phaseOrder) derives an
#         EMPTY set (correct — read-only modes need no delivery specialist);
#     (3) an unlisted FAN-OUT DISPATCHER mode (requiresTopLevelRuntime: true)
#         derives an EMPTY set EVEN THOUGH its phaseOrder carries core phases —
#         its phaseOrder is a dispatch plan, not a parent-execution requirement,
#         so the parent must NOT be over-required. The real-mode canary for this
#         is autonomous-goal (a fan-out convergence dispatcher): the prescribed
#         phaseOrder∩core recipe over-required it and broke the existing guard
#         selftest's positive fixture until the fallback was made dispatcher-aware.
#     (4) an unlisted PLANNING-MATURITY mode (auditProfile planning-maturity-v1)
#         derives an EMPTY set EVEN THOUGH its phaseOrder carries core phases —
#         its phaseOrder is the PLAN of future delivery, not phases executed at
#         planning maturity. The real-mode canary is product-to-planning, which
#         the prescribed recipe over-required (broke the G040/G068 planning
#         fixtures) until the fallback was gated to delivery-completion profiles.
#   A structural assertion additionally proves the block is positioned between
#   the case `esac` and the consuming `if [[ ${#required_specialists[@]} -gt 0 ]]`
#   so the guard actually REACHES it. The complementary listed-mode (hardcoded
#   table) path is covered end-to-end by state-transition-guard-selftest.sh,
#   which runs the whole real guard and must still pass (regression).
#
# Chosen mechanism: synthetic BUBBLES_WORKFLOWS_FILE (NOT a real unlisted mode),
# because the self-contained fixture resolves standalone (no inherits:), is fully
# deterministic, and lets the negative control assert an EMPTY set without
# depending on whatever real modes happen to be unlisted at a given moment.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh" # provides bubbles_run_with_timeout

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

# yq is a hard dependency of mode-resolver.sh; SKIP gracefully if absent so the
# selftest never hard-fails on an under-provisioned box (repo convention: an
# optional-dep-absent selftest SKIPs + exits 0 rather than FAILing).
if ! command -v yq >/dev/null 2>&1; then
  echo "state-transition-required-specialists-selftest: SKIP (yq not installed)"
  exit 0
fi

_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$_tmp_base"
# A template inside the base directory, not `-p`: the parent-directory flag is
# GNU-only and BSD mktemp rejects it, which took this selftest down on macOS.
TMP_DIR="$(mktemp -d "$_tmp_base/bubbles-reqspec-selftest.XXXXXX")"
cleanup() {
  if [[ "$failures" -eq 0 && "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "Preserving selftest workspace: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Synthetic self-contained workflows fixture. Because it carries an inline
# `modes:` block, mode-resolver.sh SKIPS composition with the real modes.yaml
# and resolves purely from this fixture. Neither synthetic mode appears in the
# guard's hardcoded Check 6 case, so both exercise the fallback.
#   - imp105-synth-delivery: phaseOrder interleaves control/conditional phases
#     (select, bootstrap, devops, finalize) with core specialists
#     (implement, test, validate, docs). Expected derived set = the 4 core
#     phases in phaseOrder order; the control/conditional phases MUST be excluded.
#   - imp105-synth-readonly: phaseOrder has NO core specialist phase. Expected
#     derived set = EMPTY.
# ---------------------------------------------------------------------------
FIXTURE="$TMP_DIR/workflows.yaml"
cat >"$FIXTURE" <<'YAML'
gates: {}
modeTemplates: {}
modes:
  imp105-synth-delivery:
    description: Synthetic delivery mode absent from the guard hardcoded Check 6 case
    statusCeiling: done
    transitionAudit: { profile: delivery-completion-v1, target: statusCeiling }
    requiredGates: []
    phaseOrder: [ select, bootstrap, implement, test, devops, validate, docs, finalize ]
  imp105-synth-readonly:
    description: Synthetic delivery-profile mode whose phaseOrder carries no core specialist phase
    statusCeiling: done
    transitionAudit: { profile: delivery-completion-v1, target: statusCeiling }
    requiredGates: []
    phaseOrder: [ select, review, finalize ]
  imp105-synth-dispatcher:
    description: Synthetic fan-out dispatcher mode (requiresTopLevelRuntime) with a full-lifecycle phaseOrder
    statusCeiling: done
    transitionAudit: { profile: delivery-completion-v1, target: statusCeiling }
    requiredGates: []
    phaseOrder: [ discover, bootstrap, implement, test, validate, audit, chaos, harden, gaps, security, regression, simplify, docs, finalize ]
    constraints:
      requiresTopLevelRuntime: true
  imp105-synth-planning:
    description: Synthetic planning-maturity mode whose phaseOrder carries core phases as a PLAN of future work
    statusCeiling: specs_hardened
    transitionAudit: { profile: planning-maturity-v1, target: statusCeiling }
    requiredGates: []
    phaseOrder: [ discover, analyze, harden, docs, validate, audit, finalize ]
YAML

export BUBBLES_WORKFLOWS_FILE="$FIXTURE"

# ---------------------------------------------------------------------------
# PRE-VERIFY the fixture wiring BEFORE asserting the derivation: prove the real
# mode-resolver.sh actually resolves each synthetic mode's phaseOrder from the
# fixture. If this fails, the fixture/env is wrong and derivation assertions
# would be meaningless.
# ---------------------------------------------------------------------------
resolve_phase_order() {
  BUBBLES_MODE_GRANDFATHER=1 bubbles_run_with_timeout 30 bash "$RESOLVER" "$1" 2>/dev/null |
    yq -r '.phaseOrder[]' 2>/dev/null || true
}

delivery_raw="$(resolve_phase_order imp105-synth-delivery)"
if [[ -n "$delivery_raw" ]] &&
  grep -qx implement <<<"$delivery_raw" &&
  grep -qx bootstrap <<<"$delivery_raw" &&
  grep -qx devops <<<"$delivery_raw"; then
  pass "pre-verify: mode-resolver resolves imp105-synth-delivery phaseOrder from the fixture (raw includes implement+bootstrap+devops)"
else
  fail "pre-verify: mode-resolver did NOT resolve imp105-synth-delivery phaseOrder from the fixture"
  echo "--- raw phaseOrder ---"
  printf '%s\n' "$delivery_raw"
  echo "--- end ---"
fi

readonly_raw="$(resolve_phase_order imp105-synth-readonly)"
if [[ -n "$readonly_raw" ]] && grep -qx review <<<"$readonly_raw"; then
  pass "pre-verify: mode-resolver resolves imp105-synth-readonly phaseOrder from the fixture"
else
  fail "pre-verify: mode-resolver did NOT resolve imp105-synth-readonly phaseOrder from the fixture"
  echo "--- raw phaseOrder ---"
  printf '%s\n' "$readonly_raw"
  echo "--- end ---"
fi

dispatcher_flag="$(BUBBLES_MODE_GRANDFATHER=1 bubbles_run_with_timeout 30 bash "$RESOLVER" imp105-synth-dispatcher 2>/dev/null | yq -r '.constraints.requiresTopLevelRuntime // false' 2>/dev/null || echo false)"
dispatcher_raw="$(resolve_phase_order imp105-synth-dispatcher)"
if [[ "$dispatcher_flag" == "true" ]] && grep -qx implement <<<"$dispatcher_raw"; then
  pass "pre-verify: mode-resolver resolves imp105-synth-dispatcher with requiresTopLevelRuntime=true AND a core-bearing phaseOrder (proves the skip is non-trivial)"
else
  fail "pre-verify: mode-resolver did NOT resolve imp105-synth-dispatcher as a dispatcher (flag='$dispatcher_flag')"
  echo "--- raw phaseOrder ---"
  printf '%s\n' "$dispatcher_raw"
  echo "--- end ---"
fi

planning_prof="$(BUBBLES_MODE_GRANDFATHER=1 bubbles_run_with_timeout 30 bash "$RESOLVER" imp105-synth-planning 2>/dev/null | yq -r '.transitionAudit.profile // "-"' 2>/dev/null || echo -)"
planning_raw="$(resolve_phase_order imp105-synth-planning)"
if [[ "$planning_prof" == "planning-maturity-v1" ]] && grep -qx harden <<<"$planning_raw"; then
  pass "pre-verify: mode-resolver resolves imp105-synth-planning as planning-maturity-v1 AND a core-bearing phaseOrder (proves the profile gate is non-trivial)"
else
  fail "pre-verify: mode-resolver did NOT resolve imp105-synth-planning as planning-maturity (profile='$planning_prof')"
  echo "--- raw phaseOrder ---"
  printf '%s\n' "$planning_raw"
  echo "--- end ---"
fi

# ---------------------------------------------------------------------------
# Extract the REAL fallback block from the guard source (between the stable
# BEGIN/END sentinels) and execute it verbatim. This runs the guard's ACTUAL
# derivation logic — not a hand-copied replica.
# ---------------------------------------------------------------------------
FALLBACK_BLOCK="$(awk '
  /# IMP-105-SCOPE-3-FALLBACK-BEGIN/ { capture=1; next }
  /# IMP-105-SCOPE-3-FALLBACK-END/   { capture=0 }
  capture { print }
' "$GUARD_SCRIPT")"

if [[ -z "$FALLBACK_BLOCK" ]]; then
  fail "extraction: could not extract the IMP-105 SCOPE-3 fallback block from the guard (sentinels missing?)"
  echo "$failures case(s) failed." >&2
  exit 1
fi
pass "extraction: fallback block extracted from the live guard between BEGIN/END sentinels"

# info/warn are guard helpers referenced by the block but irrelevant to the
# derived array; stub them as no-ops so the extracted block runs standalone.
info() { :; }
warn() { :; }

# Run the extracted block for a given mode and echo the derived
# required_specialists array (space-joined). SCRIPT_DIR, bubbles_run_with_timeout,
# and BUBBLES_WORKFLOWS_FILE are already the REAL surfaces in this process.
derive_required_specialists() {
  # state_workflow_mode and transition_audit_profile are consumed by the eval'd
  # guard fallback block (extracted from state-transition-guard.sh) — shellcheck
  # cannot see into the eval string, hence the SC2034 suppressions.
  # shellcheck disable=SC2034
  local state_workflow_mode="$1"
  # shellcheck disable=SC2034
  local transition_audit_profile="$2"
  local required_specialists=()
  local _imp105_resolved _imp105_phase_order _imp105_core _imp105_ph _imp105_c
  local _eval_rc=0
  eval "$FALLBACK_BLOCK" || _eval_rc=$?
  if [[ "$_eval_rc" -ne 0 ]]; then
    printf 'EVAL_ERROR(rc=%s)\n' "$_eval_rc"
    return 0
  fi
  printf '%s\n' "${required_specialists[*]-}"
  return 0
}

# --- Assertion 1: unlisted DELIVERY mode derives the 4 core phases, in order. ---
delivery_set="$(derive_required_specialists imp105-synth-delivery delivery-completion-v1)"
if [[ "$delivery_set" == "implement test validate docs" ]]; then
  pass "delivery: unlisted delivery mode derived exactly 'implement test validate docs' (fail-open hole closed)"
else
  fail "delivery: expected 'implement test validate docs', got '$delivery_set'"
fi

# --- Assertion 2: control/conditional phases are EXCLUDED from the derived set. ---
excluded_ok=1
for _ctrl in select bootstrap devops finalize; do
  if grep -qw "$_ctrl" <<<"$delivery_set"; then
    excluded_ok=0
    fail "delivery: control/conditional phase '$_ctrl' was WRONGLY required (over-require regression)"
  fi
done
if [[ "$excluded_ok" -eq 1 ]]; then
  pass "delivery: control/conditional phases (select bootstrap devops finalize) correctly EXCLUDED"
fi

# --- Assertion 3: NEGATIVE control — a delivery-profile mode whose phaseOrder
#     carries no core specialist phase derives an EMPTY set. ---
readonly_set="$(derive_required_specialists imp105-synth-readonly delivery-completion-v1)"
if [[ -z "$readonly_set" ]]; then
  pass "no-core-phase: delivery mode with no core phase in phaseOrder correctly derived an EMPTY set — no false specialist requirement"
else
  fail "no-core-phase: expected EMPTY set, got '$readonly_set'"
fi

# --- Assertion 3b: DISPATCHER guard — a fan-out mode (requiresTopLevelRuntime)
#     with a full-lifecycle phaseOrder derives an EMPTY set. Its phaseOrder is a
#     dispatch plan; specialist enforcement lives in the child workflows it
#     dispatches. This is the real-mode over-require regression that the naive
#     phaseOrder∩core recipe caused for autonomous-goal (and autonomous-sprint,
#     idea-to-release-completion, retro-quality-sweep). ---
dispatcher_set="$(derive_required_specialists imp105-synth-dispatcher delivery-completion-v1)"
if [[ -z "$dispatcher_set" ]]; then
  pass "dispatcher: fan-out mode (requiresTopLevelRuntime) correctly derived an EMPTY set — no parent over-require despite a core-bearing phaseOrder"
else
  fail "dispatcher: expected EMPTY set for a requiresTopLevelRuntime dispatcher, got '$dispatcher_set'"
fi

# --- Assertion 3c: PROFILE guard — a planning-maturity mode whose phaseOrder
#     carries core phases derives an EMPTY set (its phaseOrder is a PLAN of future
#     delivery, not phases executed at planning maturity). Real-mode canary:
#     product-to-planning / spec-scope-hardening. ---
planning_set="$(derive_required_specialists imp105-synth-planning planning-maturity-v1)"
if [[ -z "$planning_set" ]]; then
  pass "planning: planning-maturity mode (core-bearing phaseOrder) correctly derived an EMPTY set — profile gate excludes planning from delivery-specialist enforcement"
else
  fail "planning: expected EMPTY set for a planning-maturity mode, got '$planning_set'"
fi

# --- Assertion 4: structural placement — the block sits between the registry
#     read and the consuming `if [[ ${#required_specialists[@]} -gt 0 ]]`
#     (anchored on the unique 'missing_phases=0' line) so the guard actually
#     reaches the fallback when the registry left the array empty.
#     This used to anchor on the `esac` of the hardcoded specialist table. That
#     table is gone, so the anchor silently degraded to whatever earlier `esac`
#     happened to precede the sentinel and the assertion stopped testing its
#     own claim. ---
struct_ok="$(awk '
  /required-specialists\.yaml/ { last_read=NR }
  /# IMP-105-SCOPE-3-FALLBACK-BEGIN/ { begin=NR; read_before=last_read }
  /# IMP-105-SCOPE-3-FALLBACK-END/ { end=NR }
  $0 == "    missing_phases=0" && !seen_mp { mp=NR; seen_mp=1 }
  END {
    if (read_before > 0 && begin > read_before && end > begin && mp > end) print "OK"; else print "BAD"
  }
' "$GUARD_SCRIPT")"
if [[ "$struct_ok" == "OK" ]]; then
  pass "structure: fallback block sits after the registry read and before the consuming 'if -gt 0' (guard reaches it)"
else
  fail "structure: fallback block is NOT positioned between the registry read and the consuming 'if -gt 0'"
fi

# IMP-052 SCOPE-2: Check 6B must resolve the registered owner before it attempts
# specialist provenance matching. This complements the end-to-end adversarial
# fixtures in state-transition-guard-selftest.sh and prevents a future refactor
# from restoring the old `bubbles.<phase>` match ahead of registry validation.
owner_resolution_order="$(awk '
  /CHECK 6B: Phase-claim provenance/ { check6b=NR }
  /resolve_phase_owner "\$claimed_phase"/ { resolve=NR }
  /matched_agent=.*expected_agent/ { match_line=NR }
  END {
    if (check6b > 0 && resolve > check6b && match_line > resolve) print "OK"; else print "BAD"
  }
' "$GUARD_SCRIPT")"
if [[ "$owner_resolution_order" == "OK" ]]; then
  pass "structure: Check 6B resolves phase ownership before specialist provenance matching"
else
  fail "structure: Check 6B does NOT resolve phase ownership before specialist provenance matching"
fi

if [[ "$failures" -ne 0 ]]; then
  echo "$failures case(s) failed." >&2
  exit 1
fi
echo "all cases passed."
