#!/usr/bin/env bash
#
# model-tier-advisory-selftest.sh — hermetic selftest for the v6.1 / S9 (R4)
# blocking model-tier floor (gate G126).
#
# Stages a minimal workflows.yaml fixture (via BUBBLES_WORKFLOWS_FILE) declaring
# modeDefaults.modelFloor + modelFloorEnforcedPhases, then asserts:
#
#   1. enforced phase + known model BELOW floor            -> exit 1 (BLOCKED)
#   2. enforced phase + known model AT/ABOVE floor         -> exit 0 (OK)
#   3. enforced phase + UNKNOWN model (env unset)          -> exit 0 (no false block)
#   4. non-enforced phase + known model below floor        -> exit 0 (advisory WARN)
#   5. phase with NO floor declared                        -> exit 0
#   6. --enforce forces blocking on a non-listed phase     -> exit 1
#   7. resolve op prints the resolved floor
#
# Exit 0 when all assertions pass; 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/model-tier-advisory.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "model-tier-advisory-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  echo "model-tier-advisory-selftest: SKIP (PyYAML not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

FIXTURE="$TMPDIR/workflows.yaml"
cat > "$FIXTURE" <<'YAML'
modes:
  full-delivery:
    description: fixture mode
modeDefaults:
  modelFloor:
    default: ""
    implement: sonnet-class
    validate: sonnet-class
    audit: sonnet-class
    security: opus-class
    test: ""
  modelFloorEnforcedPhases: [ audit, security, validate ]
YAML

export BUBBLES_WORKFLOWS_FILE="$FIXTURE"
# Keep selftest writes out of the real repo's tool-call log.
export BUBBLES_TOOL_LOG_FILE="$TMPDIR/tool-calls.jsonl"

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

run() {
  # run <expected_exit> <description> -- <args...>   (model passed via $MODEL)
  local expected="$1"; shift
  local desc="$1"; shift
  shift # drop the literal --
  local rc=0
  if [[ -n "${MODEL:-}" ]]; then
    BUBBLES_ACTIVE_MODEL="$MODEL" bash "$TARGET" "$@" >/dev/null 2>&1 || rc=$?
  else
    (unset BUBBLES_ACTIVE_MODEL; bash "$TARGET" "$@") >/dev/null 2>&1 || rc=$?
  fi
  if [[ "$rc" -eq "$expected" ]]; then
    pass "$desc (exit $rc)"
  else
    fail "$desc — expected exit $expected, got $rc"
  fi
}

# 1. enforced phase (audit) + haiku active (below sonnet floor) -> BLOCK (1)
MODEL="haiku-3.5" run 1 "enforced audit + below floor blocks" -- check --mode full-delivery --phase audit
# 2. enforced phase (audit) + opus active (above sonnet floor) -> OK (0)
MODEL="opus-4.7" run 0 "enforced audit + above floor passes" -- check --mode full-delivery --phase audit
# 2b. enforced phase (security) + sonnet active (below opus floor) -> BLOCK (1)
MODEL="sonnet-4.5" run 1 "enforced security + below opus floor blocks" -- check --mode full-delivery --phase security
# 3. enforced phase (audit) + UNKNOWN model -> no false block (0)
MODEL="" run 0 "enforced audit + unknown model does not block" -- check --mode full-delivery --phase audit
# 4. non-enforced phase (implement) + haiku active (below floor) -> advisory (0)
MODEL="haiku-3.5" run 0 "non-enforced implement + below floor is advisory" -- check --mode full-delivery --phase implement
# 5. phase with no floor (test) + haiku active -> 0
MODEL="haiku-3.5" run 0 "no-floor phase passes" -- check --mode full-delivery --phase test
# 6. --enforce forces blocking on a non-listed phase (implement) -> BLOCK (1)
MODEL="haiku-3.5" run 1 "--enforce forces blocking on non-listed phase" -- check --enforce --mode full-delivery --phase implement

# 7. resolve prints the floor
resolved="$(BUBBLES_WORKFLOWS_FILE="$FIXTURE" bash "$TARGET" resolve --mode full-delivery --phase security 2>/dev/null || true)"
if [[ "$resolved" == "opus-class" ]]; then
  pass "resolve prints declared floor (opus-class)"
else
  fail "resolve expected 'opus-class', got '$resolved'"
fi

# ---------------------------------------------------------------------------
# IMP-027 / SCOPE-11 — `retirement` reports gate obsolescence candidacy.
#
# The load-bearing assertion is not which gates come back eligible; it is that
# the report can NEVER be read as clearance to turn a gate off. Tier
# eligibility is only half a criterion, and the measured half does not exist
# yet. A report that omitted that caveat would look exactly like a green light.
# ---------------------------------------------------------------------------
RETIRE_FIXTURE="$TMPDIR/retire-workflows.yaml"
cat > "$RETIRE_FIXTURE" <<'YAML'
modes:
  full-delivery:
    description: fixture mode
modeDefaults:
  modelFloor:
    default: ""
  modelFloorEnforcedPhases: [ audit ]
gates:
  G001:
    name: never_retires_gate
    classification: businessInvariant
    description: Deployment artifacts must be signed before release.
  G002:
    name: strict_model_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    description: Raw execution evidence is captured per policy.
  G003:
    name: lenient_model_gate
    classification: modelCompensation
    retireWhen: { minTier: sonnet-class, metric: routing-omission-rate, threshold: 0.02, window: 20 }
    description: Each specialist agent's output MUST be verified by the orchestrator.
YAML

retire_out() {
  # retire_out <tier>  — prints the retirement report for a declared tier.
  # IMP-058 SCOPE-1 / REG-22: `retirement` reads the GATE registry, not
  # workflows.yaml, so the fixture is fed via BUBBLES_GATES_FILE. The fixture
  # file itself is unchanged — it already carries a top-level `gates:` key in
  # the real registry's shape.
  if [[ -n "${1:-}" ]]; then
    BUBBLES_GATES_FILE="$RETIRE_FIXTURE" bash "$TARGET" retirement --tier "$1" 2>&1
  else
    (unset BUBBLES_ACTIVE_MODEL
      BUBBLES_GATES_FILE="$RETIRE_FIXTURE" bash "$TARGET" retirement) 2>&1
  fi
}

expect_in() {
  local label="$1" needle="$2" haystack="$3"
  if grep -qF -- "$needle" <<<"$haystack"; then
    pass "$label"
  else
    fail "$label — missing '$needle'. Output: $haystack"
  fi
}

OPUS_OUT="$(retire_out opus-4.7)"
expect_in "retirement counts only modelCompensation gates" \
  "modelCompensation gates: 2" "$OPUS_OUT"
expect_in "opus meets both tier preconditions" \
  "tier precondition MET (2): G002, G003" "$OPUS_OUT"

HAIKU_OUT="$(retire_out haiku-3.5)"
expect_in "haiku meets neither tier precondition" \
  "tier precondition MET (0): none" "$HAIKU_OUT"
expect_in "haiku report names the tier each gate still needs" \
  "G002(needs opus-class)" "$HAIKU_OUT"

SONNET_OUT="$(retire_out sonnet-4.5)"
expect_in "sonnet discriminates between the two criteria" \
  "tier precondition MET (1): G003" "$SONNET_OUT"

UNKNOWN_OUT="$(retire_out "")"
expect_in "unknown model is reported, not guessed" "model-unknown" "$UNKNOWN_OUT"

# The caveat must be present at EVERY tier, including the one where everything
# is eligible — that is precisely when a reader is most likely to act on it.
for tier_out_label in "opus:$OPUS_OUT" "haiku:$HAIKU_OUT" "unknown:$UNKNOWN_OUT"; do
  label="${tier_out_label%%:*}"
  body="${tier_out_label#*:}"
  expect_in "retirement states nothing is retired ($label)" \
    "NOTHING IS RETIRED BY THIS REPORT" "$body"
  expect_in "retirement states the evidence half is unmet ($label)" \
    "The EVIDENCE half is UNMET" "$body"
done

MODEL="opus-4.7" run 0 "retirement is advisory and never blocks" -- retirement

# ---------------------------------------------------------------------------
# IMP-058 SCOPE-2 / COV-24 — `preventionEvidence`, the satisfiable evidence
# half. G002 declares it; G003 does not (keeps exactly today's rate-only
# behavior). BUBBLES_GATE_HIT_ROOT points gate-hit-log.sh at a synthetic
# fixture repo instead of this repo's own real telemetry, so the three
# classes below are exact and reproducible rather than a live snapshot.
# ---------------------------------------------------------------------------
EVIDENCE_FIXTURE="$TMPDIR/evidence-workflows.yaml"
cat > "$EVIDENCE_FIXTURE" <<'YAML'
gates:
  G002:
    name: strict_model_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    preventionEvidence: { minRuns: 10, prevented: 0, sourceClass: product }
    description: Raw execution evidence is captured per policy.
  G003:
    name: lenient_model_gate
    classification: modelCompensation
    retireWhen: { minTier: sonnet-class, metric: routing-omission-rate, threshold: 0.02, window: 20 }
    description: Each specialist agent's output MUST be verified by the orchestrator.
YAML

EVIDENCE_REPO="$TMPDIR/evidence-repo"
mkdir -p "$EVIDENCE_REPO/.specify/runtime"
EVIDENCE_LOG="$EVIDENCE_REPO/.specify/runtime/gate-hits.jsonl"
# G002: fired 10 times (meets its declared minRuns), never prevented -> CANDIDATE.
for i in 1 2 3 4 5 6 7 8 9 10; do
  printf '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-09-04T00:00:0%dZ","sourceClass":"product","gate":"G002","outcome":"pass","fired":true,"prevented":false,"spec":"specs/fixture","mode":"full-delivery","targetStatus":"done","guardVerdict":"PASS","exitStatus":"0"}\n' \
    "$((i % 10))" >> "$EVIDENCE_LOG"
done
# G003: fired once and prevented once -> EARNING, despite no preventionEvidence
# declared. Earning needs no threshold; it is a fact, not a judgment call.
printf '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-09-04T00:01:00Z","sourceClass":"product","gate":"G003","outcome":"fail","fired":true,"prevented":true,"spec":"specs/fixture","mode":"full-delivery","targetStatus":"done","guardVerdict":"FAIL","exitStatus":"1"}\n' \
  >> "$EVIDENCE_LOG"

EVIDENCE_OUT="$(BUBBLES_GATES_FILE="$EVIDENCE_FIXTURE" BUBBLES_GATE_HIT_ROOT="$EVIDENCE_REPO" \
  bash "$TARGET" retirement --tier opus-class 2>&1)"

expect_in "preventionEvidence: G003 is EARNING on prevention alone, no minRuns needed" \
  "EARNING (1)" "$EVIDENCE_OUT"
expect_in "preventionEvidence: G003 named in the EARNING line" \
  "G003(prevented=1/1)" "$EVIDENCE_OUT"
expect_in "preventionEvidence: G002 is CANDIDATE at fired == declared minRuns, never prevented" \
  "CANDIDATE (1)" "$EVIDENCE_OUT"
expect_in "preventionEvidence: G002 named in the CANDIDATE line" \
  "G002(fired=10)" "$EVIDENCE_OUT"
expect_in "preventionEvidence: nothing left unmeasured once both gates have telemetry" \
  "UNMEASURED (0)" "$EVIDENCE_OUT"
expect_in "preventionEvidence: a CANDIDATE gate does not read as retirement" \
  "CANDIDATE is not retirement" "$EVIDENCE_OUT"

# Adversarial bound: fired BELOW the declared minRuns must stay UNMEASURED,
# not be promoted to CANDIDATE by an off-by-one in the comparison.
UNDER_LOG="$TMPDIR/under-repo/.specify/runtime/gate-hits.jsonl"
mkdir -p "$(dirname "$UNDER_LOG")"
for i in 1 2 3 4 5 6 7 8 9; do
  printf '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-09-04T00:00:0%dZ","sourceClass":"product","gate":"G002","outcome":"pass","fired":true,"prevented":false,"spec":"specs/fixture","mode":"full-delivery","targetStatus":"done","guardVerdict":"PASS","exitStatus":"0"}\n' \
    "$i" >> "$UNDER_LOG"
done
UNDER_OUT="$(BUBBLES_GATES_FILE="$EVIDENCE_FIXTURE" BUBBLES_GATE_HIT_ROOT="$TMPDIR/under-repo" \
  bash "$TARGET" retirement --tier opus-class 2>&1)"
expect_in "preventionEvidence adversarial: fired == minRuns - 1 stays UNMEASURED, not CANDIDATE" \
  "CANDIDATE (0)" "$UNDER_OUT"

# Adversarial bound: a fixture repo with no gate-hits.jsonl at all (no
# telemetry store) must degrade to UNMEASURED for everything, never crash and
# never silently promote to CANDIDATE/EARNING.
NOLOG_OUT="$(BUBBLES_GATES_FILE="$EVIDENCE_FIXTURE" BUBBLES_GATE_HIT_ROOT="$TMPDIR/nolog-repo" \
  bash "$TARGET" retirement --tier opus-class 2>&1)"
expect_in "preventionEvidence adversarial: an absent telemetry store reports UNMEASURED, not a crash" \
  "UNMEASURED (2)" "$NOLOG_OUT"

# ---------------------------------------------------------------------------
# IMP-058 SCOPE-1 / REG-22 regression pin: `retirement` against the REAL,
# unoverridden gate registry must report a non-zero modelCompensation gate
# count. This is the exact defect that shipped — the reader was bound to
# workflows.yaml, which has no top-level `gates:` key, so the count was
# always zero and the empty result rendered as a clean report rather than a
# failure. Deliberately runs with no BUBBLES_GATES_FILE override.
# ---------------------------------------------------------------------------
REAL_OUT="$(bash "$TARGET" retirement --tier opus-class 2>&1)"
if grep -qE "modelCompensation gates: 0$" <<<"$REAL_OUT"; then
  fail "retirement against the real registry reports zero modelCompensation gates (REG-22 regression): $REAL_OUT"
else
  pass "retirement against the real registry reports a non-zero modelCompensation gate count"
fi

echo ""
echo "[model-tier-advisory-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[model-tier-advisory-selftest] OK"
exit 0