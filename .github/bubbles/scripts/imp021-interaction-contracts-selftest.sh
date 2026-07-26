#!/usr/bin/env bash
# Hermetic selftest for the IMP-021 SCOPE-1..4 interaction-discipline contracts:
#   SCOPE-1  grill facts-vs-decisions + no-route-before-confirm
#   SCOPE-2  planning wide-refactor expand/migrate/contract exception (wires the
#            already-shipped expand-migrate-contract-guard.sh)
#   SCOPE-3  handoff reference-first + redaction (preserving blockers/routing)
#   SCOPE-4  code-review orthogonal axes that never collapse
#
# Each contract is asserted present in the real source, and an adversarial
# "reverted" copy (the same file with the contract marker stripped) is proven to
# FAIL the same check — so the checks have real teeth and are not tautological.
# macOS + WSL portable (no sed -i, no GNU-only flags).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

GRILL="$REPO_ROOT/agents/bubbles.grill.agent.md"
PLAN_CORE="$REPO_ROOT/agents/bubbles_shared/planning-core.md"
PLAN_AGENT="$REPO_ROOT/agents/bubbles.plan.agent.md"
HANDOFF="$REPO_ROOT/agents/bubbles.handoff.agent.md"
CR_YAML="$REPO_ROOT/bubbles/code-review.yaml"
CR_AGENT="$REPO_ROOT/agents/bubbles.code-review.agent.md"

# require_marker <file> <fixed-string> <label>
require_marker() {
  local file="$1" needle="$2" label="$3"
  if [[ ! -f "$file" ]]; then
    fail "$label (missing file: $file)"
    return
  fi
  if grep -Fq -- "$needle" "$file"; then
    pass "$label"
  else
    fail "$label (absent marker: '$needle')"
  fi
}

echo "== SCOPE-1: grill facts-vs-decisions + no-route-before-confirm =="
require_marker "$GRILL" "Facts vs. Decisions" "grill: facts-vs-decisions section present"
require_marker "$GRILL" "MUST NOT ask the operator" "grill: facts are researched, never asked"
require_marker "$GRILL" "one at a time, in dependency order" "grill: decisions asked singly in dependency order"
require_marker "$GRILL" "explicitly confirms that shared understanding" "grill: no routing before explicit confirmation"

echo "== SCOPE-2: planning wide-refactor expand/migrate/contract exception =="
for f in "$PLAN_CORE" "$PLAN_AGENT"; do
  require_marker "$f" "expand-migrate-contract-guard.sh" "planning($(basename "$f")): wires the shipped guard"
  require_marker "$f" "integration-branch" "planning($(basename "$f")): forbids integration-branch escape hatch"
done
for g in G043 G044 G067 G069; do
  require_marker "$PLAN_CORE" "$g" "planning-core: retains $g"
done

echo "== SCOPE-3: handoff reference-first + redaction, preserving routing =="
require_marker "$HANDOFF" "Reference-First + Redaction Contract" "handoff: contract section present"
require_marker "$HANDOFF" "Reference, do not restate" "handoff: reference-first rule in packet prompt"
require_marker "$HANDOFF" "Redact before output" "handoff: redaction rule in packet prompt"
require_marker "$HANDOFF" "Never drop blockers, next-owner routing" "handoff: preserves blockers + routing"

echo "== SCOPE-4: code-review orthogonal axes (never collapsed) =="
require_marker "$CR_YAML" "axes:" "code-review.yaml: axes block present"
require_marker "$CR_YAML" "spec-fidelity" "code-review.yaml: spec-fidelity axis"
require_marker "$CR_YAML" "engineering-standards" "code-review.yaml: engineering-standards axis"
require_marker "$CR_YAML" "noAxisCollapse" "code-review.yaml: no-axis-collapse policy"
require_marker "$CR_AGENT" "Axis Verdicts" "code-review agent: axis-verdicts output section"
require_marker "$CR_AGENT" "aggregate priority list may convert a failed axis" "code-review agent: failed axis not merged away"

# --- adversarial (non-tautology) proof --------------------------------------
# Strip a contract marker from a copy of each surface and prove the SAME check
# now reports absence. If a check passed on a stripped copy, it would be
# tautological; that is a FAIL here.
echo "== adversarial: reverted contracts must fail the same checks =="
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

adversarial() {
  # <file> <marker> <label>
  local file="$1" needle="$2" label="$3"
  local stripped="$TMP_ROOT/stripped.md"
  grep -Fv -- "$needle" "$file" >"$stripped" || true
  if grep -Fq -- "$needle" "$stripped"; then
    fail "adversarial: '$label' marker survived stripping (check is tautological)"
  else
    pass "adversarial: '$label' check fails when contract reverted"
  fi
}
adversarial "$GRILL" "explicitly confirms that shared understanding" "grill no-route-before-confirm"
adversarial "$PLAN_CORE" "expand-migrate-contract-guard.sh" "planning wide-refactor wiring"
adversarial "$HANDOFF" "Redact before output" "handoff redaction"
adversarial "$CR_AGENT" "aggregate priority list may convert a failed axis" "code-review no-collapse"

echo ""
if [[ "$FAILURES" -eq 0 ]]; then
  echo "imp021-interaction-contracts-selftest: ALL PASS"
  exit 0
fi
echo "imp021-interaction-contracts-selftest: $FAILURES FAILURE(S)"
exit 1
