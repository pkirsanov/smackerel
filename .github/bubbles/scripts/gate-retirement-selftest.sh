#!/usr/bin/env bash
#
# gate-retirement-selftest.sh — hermetic selftest for gate-retirement.sh
# (IMP-027 / SCOPE-11: the obsolescence curve).
#
# Stages minimal gates.yaml fixtures via BUBBLES_GATES_FILE and asserts:
#
#   lint accepts a well-formed criterion, and REFUSES every way a criterion can
#   be wrong (absent, malformed, wrong tier, out-of-range rate, zero window) as
#   well as a criterion placed on a gate that can never retire.
#
#   bind derives the class from the gate's ENFORCEMENT SUBJECT, never
#   overwrites a value a human already recorded, and falls back to the
#   STRICTEST criterion when no signal fires.
#
#   report states that nothing is measured, so a green report can never be read
#   as permission to turn a gate off.
#
# Exit 0 when all assertions pass; 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/gate-retirement.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-retirement-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  echo "gate-retirement-selftest: SKIP (PyYAML not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() {
  echo "  PASS: $1"
  pass_count=$((pass_count + 1))
}
fail() {
  echo "  FAIL: $1"
  fail_count=$((fail_count + 1))
}

# run <fixture> <subcommand> -> sets OUT / RC
run() {
  set +e
  OUT="$(BUBBLES_GATES_FILE="$1" bash "$TARGET" "$2" 2>&1)"
  RC=$?
  set -e
}

expect_rc() {
  local label="$1" want="$2"
  if [[ "$RC" -eq "$want" ]]; then
    pass "$label (exit $RC)"
  else
    fail "$label — expected exit $want, got $RC. Output: $OUT"
  fi
}

expect_out() {
  local label="$1" needle="$2"
  if grep -qF -- "$needle" <<<"$OUT"; then
    pass "$label"
  else
    fail "$label — output did not contain '$needle'. Output: $OUT"
  fi
}

expect_not_out() {
  local label="$1" needle="$2"
  if grep -qF -- "$needle" <<<"$OUT"; then
    fail "$label — output unexpectedly contained '$needle'. Output: $OUT"
  else
    pass "$label"
  fi
}

# ---------------------------------------------------------------------------
# 1. A well-formed registry lints clean.
# ---------------------------------------------------------------------------
GOOD="$TMPDIR/good.yaml"
cat > "$GOOD" <<'YAML'
gates:
  G001:
    name: sample_business_gate
    classification: businessInvariant
    description: Deployment artifacts must be signed before release.
  G002:
    name: sample_model_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    description: Raw execution evidence is captured per policy.
YAML
run "$GOOD" lint
expect_rc "well-formed registry lints clean" 0
expect_out "reports the covered gate count" "all 1 modelCompensation gate(s)"

# ---------------------------------------------------------------------------
# 2. A modelCompensation gate with NO criterion is a finding — this is the
#    whole point of the scope: an unbounded-in-time gate must be visible.
# ---------------------------------------------------------------------------
MISSING="$TMPDIR/missing.yaml"
cat > "$MISSING" <<'YAML'
gates:
  G002:
    name: sample_model_gate
    classification: modelCompensation
    description: Raw execution evidence is captured per policy.
YAML
run "$MISSING" lint
expect_rc "missing criterion fails lint" 1
expect_out "names the missing-criterion finding" "retirement-missing"
expect_out "explains why it matters" "unbounded in time"

# ---------------------------------------------------------------------------
# 3. A gate that can NEVER retire must not carry a criterion. Allowing it would
#    let a businessInvariant gate be argued out of existence on model quality.
# ---------------------------------------------------------------------------
for cls in businessInvariant hybrid; do
  ILLEGAL="$TMPDIR/illegal-$cls.yaml"
  cat > "$ILLEGAL" <<YAML
gates:
  G003:
    name: sample_gate
    classification: $cls
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    description: Deployment artifacts must be signed before release.
YAML
  run "$ILLEGAL" lint
  expect_rc "criterion on a $cls gate fails lint" 1
  expect_out "names the illegal-placement finding ($cls)" "retirement-illegal"
done

# ---------------------------------------------------------------------------
# 4. Malformed criteria are refused, one shape at a time.
# ---------------------------------------------------------------------------
malformed_case() {
  local label="$1" value="$2"
  local f="$TMPDIR/malformed.yaml"
  cat > "$f" <<YAML
gates:
  G004:
    name: sample_model_gate
    classification: modelCompensation
    retireWhen: $value
    description: Raw execution evidence is captured per policy.
YAML
  run "$f" lint
  expect_rc "$label fails lint" 1
  expect_out "$label reports retirement-malformed" "retirement-malformed"
}

malformed_case "a scalar criterion" "'soon'"
malformed_case "a criterion missing keys" "{ minTier: opus-class }"
malformed_case "an unknown tier" \
  "{ minTier: gpt-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }"
malformed_case "a non-numeric threshold" \
  "{ minTier: opus-class, metric: fabricated-evidence-rate, threshold: low, window: 50 }"
malformed_case "a threshold of 1.5 (not a rate)" \
  "{ minTier: opus-class, metric: fabricated-evidence-rate, threshold: 1.5, window: 50 }"
malformed_case "a threshold of 0 (unreachable)" \
  "{ minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0, window: 50 }"
malformed_case "a window of 0 runs" \
  "{ minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 0 }"

# ---------------------------------------------------------------------------
# 5. bind derives from the enforcement SUBJECT, not from the rationale.
#    Both gates below mention fabrication in passing; only one is ABOUT
#    evidence authenticity. A deriver that keys on the word "fabricated" gets
#    this wrong, which is exactly the lexical-proxy failure IMP-027 targets.
# ---------------------------------------------------------------------------
DERIVE="$TMPDIR/derive.yaml"
cat > "$DERIVE" <<'YAML'
gates:
  G005:
    name: evidence_gate
    classification: modelCompensation
    description: Raw execution evidence is captured per policy.
  G006:
    name: scope_index_parity_gate
    classification: modelCompensation
    description: The status column of scopes/_index.md MUST match every linked scope.md. Fabricated batch promotions historically updated individual files while leaving the index stale.
  G007:
    name: batch_promotion_limit_gate
    classification: modelCompensation
    description: A single git commit MUST NOT promote more than one spec to done. Mass promotions are a documented fabrication pattern.
YAML
run "$DERIVE" bind
expect_rc "bind succeeds on an unseeded registry" 0
expect_out "evidence gate derives evidence-authenticity" "G005: evidence-authenticity"
expect_out "index-parity gate derives structural-integrity" "G006: structural-integrity"
expect_out "batch-promotion gate derives process-discipline" "G007: process-discipline"
expect_not_out "rationale mention of fabrication does not decide the class" \
  "G006: evidence-authenticity"

# The blast-radius scaling must survive binding: completion/evidence classes get
# the tighter bound, process discipline the looser one.
run "$DERIVE" report
expect_out "evidence class binds the strict threshold" "fabricated-evidence-rate < 0.5% over 50 runs"
expect_out "process class binds the looser threshold" "process-violation-rate < 2% over 20 runs"

# ---------------------------------------------------------------------------
# 6. bind NEVER overwrites a value a human recorded. Classification is a
#    judgement; the tool seeds, the human corrects, and a re-run must not
#    silently undo the correction.
# ---------------------------------------------------------------------------
HUMAN="$TMPDIR/human.yaml"
cat > "$HUMAN" <<'YAML'
gates:
  G008:
    name: evidence_gate
    classification: modelCompensation
    retireWhen: { minTier: sonnet-class, metric: human-chosen-rate, threshold: 0.03, window: 7 }
    description: Raw execution evidence is captured per policy.
YAML
run "$HUMAN" bind
expect_rc "bind is a no-op when every gate is already covered" 0
expect_out "bind says it changed nothing" "no change"
if grep -qF "human-chosen-rate" "$HUMAN" && grep -qF "window: 7" "$HUMAN"; then
  pass "bind preserved the human-recorded criterion verbatim"
else
  fail "bind overwrote a human-recorded criterion: $(cat "$HUMAN")"
fi

# ---------------------------------------------------------------------------
# 7. An uncharacterised gate falls back to the STRICTEST criterion, never the
#    loosest. Guessing "probably harmless" about a failure mode nobody has
#    characterised is the move this framework exists to block.
# ---------------------------------------------------------------------------
UNKNOWN="$TMPDIR/unknown.yaml"
cat > "$UNKNOWN" <<'YAML'
gates:
  G009:
    name: mystery_gate
    classification: modelCompensation
    description: An entirely unremarkable requirement with no recognisable subject anchor whatsoever.
YAML
run "$UNKNOWN" bind
expect_out "unrecognised gate reports the default signal" "no signal; strictest criterion by default"
run "$UNKNOWN" report
expect_out "unrecognised gate gets the strictest threshold" "< 0.5% over 50 runs"

# ---------------------------------------------------------------------------
# 8. report must never read as permission to retire. A green report that omits
#    the unmeasured-evidence caveat is worse than no report, because it looks
#    like a clearance.
# ---------------------------------------------------------------------------
run "$GOOD" report
expect_rc "report exits 0 on a clean registry" 0
expect_out "report states nothing has been measured" "MEASURED: none"
expect_out "report says retirement is a human edit" "never a tool inference"

# ---------------------------------------------------------------------------
# 9. No bypass. A gate-retirement tool with a --force is a contradiction: it
#    would let a gate be turned off by asserting models improved. Each flag is
#    ACTUALLY INVOKED — asserting on the script text instead would pass even if
#    the flag were implemented, since the text contains these words either way.
# ---------------------------------------------------------------------------
for bypass in "--skip" "--force" "--ignore" "--no-verify" "--allow-once"; do
  set +e
  OUT="$(bash "$TARGET" "$bypass" 2>&1)"
  RC=$?
  set -e
  if [[ "$RC" -eq 2 ]]; then
    pass "rejects $bypass (exit 2)"
  else
    fail "$bypass was accepted (exit $RC) — a retirement bypass must not exist. Output: $OUT"
  fi
done

# The same must hold when a bypass is passed AFTER a legitimate subcommand, so
# `lint --force` cannot become a way to wave findings through.
for bypass in "--skip" "--force"; do
  set +e
  OUT="$(BUBBLES_GATES_FILE="$MISSING" bash "$TARGET" lint "$bypass" 2>&1)"
  RC=$?
  set -e
  if [[ "$RC" -ne 0 ]]; then
    pass "lint $bypass does not suppress findings (exit $RC)"
  else
    fail "lint $bypass suppressed a real finding — output: $OUT"
  fi
done

# ---------------------------------------------------------------------------
# 10. The real registry must satisfy its own tool.
# ---------------------------------------------------------------------------
set +e
OUT="$(bash "$TARGET" lint 2>&1)"
RC=$?
set -e
expect_rc "the live gates registry lints clean" 0

echo ""
echo "[gate-retirement-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[gate-retirement-selftest] OK"
exit 0
