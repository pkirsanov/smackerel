#!/usr/bin/env bash
#
# gate-telemetry-capability-lint-selftest.sh — hermetic selftest for
# gate-telemetry-capability-lint.sh (IMP-058 SCOPE-3 / COV-24).
#
# Stages minimal gates.yaml + guard-script fixtures via BUBBLES_GATES_FILE /
# BUBBLES_GUARD_FILE and asserts:
#
#   a gate declaring preventionEvidence, enforced by the guard, with its own
#   id in a tagged pass/fail message -- ACCEPTED.
#
#   a gate declaring preventionEvidence, enforced by the guard, but whose
#   pass/fail messages never mention its own id -- REFUSED (the exact class
#   of gap SCOPE-3 found and fixed for G026/G063/G072/G074).
#
#   a gate declaring preventionEvidence but enforced by neither the guard nor
#   any of its guards/*.sh fragments -- REFUSED (the class of gap G090/G128
#   would have hit before the fragment-following fix, and that G005/G020/etc.
#   are genuinely in today, having no script-based enforcement at all).
#
#   a gate enforced only via a sourced guards/*.sh fragment, tagged there --
#   ACCEPTED (proves the fragment-derivation logic actually reaches into a
#   sourced file rather than only reading the guard's own text).
#
#   a gate with NO preventionEvidence declared is never checked at all
#   (SCOPE-2's own "keeps exactly today's behavior" contract).
#
# Exit 0 when all assertions pass; 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/gate-telemetry-capability-lint.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-telemetry-capability-lint-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  echo "gate-telemetry-capability-lint-selftest: SKIP (PyYAML not installed)"
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

mkdir -p "$TMPDIR/guards"

# run <gates-fixture> <guard-fixture> -> sets OUT / RC
run() {
  set +e
  OUT="$(BUBBLES_GATES_FILE="$1" BUBBLES_GUARD_FILE="$2" bash "$TARGET" 2>&1)"
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
    fail "$label — missing '$needle'. Output: $OUT"
  fi
}

expect_not_out() {
  local label="$1" needle="$2"
  if grep -qF -- "$needle" <<<"$OUT"; then
    fail "$label — unexpectedly contains '$needle'. Output: $OUT"
  else
    pass "$label"
  fi
}

# ---------------------------------------------------------------------------
# Case 1: G002 declares preventionEvidence, enforced by the guard, and the
# guard's own pass/fail messages tag G002. Must be accepted.
# ---------------------------------------------------------------------------
GATES_OK="$TMPDIR/gates-ok.yaml"
cat > "$GATES_OK" <<'YAML'
gates:
  G002:
    name: capable_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    preventionEvidence: { minRuns: 10, prevented: 0, sourceClass: product }
gateEnforcement:
  derived:
    G002: { enforcedBy: [ script:bubbles/scripts/state-transition-guard.sh ] }
YAML
GUARD_OK="$TMPDIR/guard-ok.sh"
cat > "$GUARD_OK" <<'EOS'
#!/usr/bin/env bash
pass "Widget check clean (Gate G002)"
EOS
run "$GATES_OK" "$GUARD_OK"
expect_rc "capable gate is accepted" 0
expect_out "capable gate names its own id checked" "all 1 gate(s)"

# ---------------------------------------------------------------------------
# Case 2: same gate, same enforcedBy, but the guard's message never mentions
# G002 -- the exact defect class SCOPE-3 found and fixed in production.
# ---------------------------------------------------------------------------
GUARD_UNTAGGED="$TMPDIR/guard-untagged.sh"
cat > "$GUARD_UNTAGGED" <<'EOS'
#!/usr/bin/env bash
pass "Widget check clean"
EOS
run "$GATES_OK" "$GUARD_UNTAGGED"
expect_rc "guard-enforced but untagged pass/fail is refused" 1
expect_out "untagged case names the exact gate" "FINDING: telemetry-incapable-criterion: G002"
expect_out "untagged case names the untagged-message reason" "no pass/fail call in that file mentions this gate id"

# ---------------------------------------------------------------------------
# Case 3: same gate, but not enforced by the guard (or any fragment) at all --
# the class G090/G128 would have hit before fragment-following, and the class
# G005/G020/etc. are genuinely in (behavioral-only, no script).
# ---------------------------------------------------------------------------
GATES_UNENFORCED="$TMPDIR/gates-unenforced.yaml"
cat > "$GATES_UNENFORCED" <<'YAML'
gates:
  G002:
    name: unenforced_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    preventionEvidence: { minRuns: 10, prevented: 0, sourceClass: product }
gateEnforcement:
  derived:
    G002: { enforcedBy: [ behavioral:agents/bubbles_shared/operating-baseline.md ] }
YAML
run "$GATES_UNENFORCED" "$GUARD_OK"
expect_rc "non-guard-enforced gate is refused even with a tagged guard present" 1
expect_out "non-guard-enforced case names the integration-gap reason" "no telemetry integration point exists there"

# ---------------------------------------------------------------------------
# Case 4: G002 is enforced ONLY via a sourced guards/*.sh fragment, tagged
# there -- proves fragment-derivation actually reaches into the sourced file.
# ---------------------------------------------------------------------------
GATES_FRAGMENT="$TMPDIR/gates-fragment.yaml"
cat > "$GATES_FRAGMENT" <<'YAML'
gates:
  G002:
    name: fragment_enforced_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
    preventionEvidence: { minRuns: 10, prevented: 0, sourceClass: product }
gateEnforcement:
  derived:
    G002: { enforcedBy: [ script:bubbles/scripts/guards/widget-checks.sh ] }
YAML
GUARD_SOURCER="$TMPDIR/guard-sourcer.sh"
cat > "$GUARD_SOURCER" <<EOS
#!/usr/bin/env bash
SCRIPT_DIR="$TMPDIR"
source "\$SCRIPT_DIR/guards/widget-checks.sh"
EOS
cat > "$TMPDIR/guards/widget-checks.sh" <<'EOS'
#!/usr/bin/env bash
pass "Widget check clean (Gate G002)"
EOS
run "$GATES_FRAGMENT" "$GUARD_SOURCER"
expect_rc "fragment-only enforcement with a tagged message is accepted" 0

# ---------------------------------------------------------------------------
# Case 5: no gate declares preventionEvidence at all -- SCOPE-2's own
# contract ("a gate carrying only an unmeasurable rate keeps exactly today's
# behavior") means this lint has nothing to check and must not invent a
# finding.
# ---------------------------------------------------------------------------
GATES_NONE="$TMPDIR/gates-none.yaml"
cat > "$GATES_NONE" <<'YAML'
gates:
  G003:
    name: rate_only_gate
    classification: modelCompensation
    retireWhen: { minTier: opus-class, metric: fabricated-evidence-rate, threshold: 0.005, window: 50 }
gateEnforcement:
  derived:
    G003: { enforcedBy: [ behavioral:agents/bubbles_shared/operating-baseline.md ] }
YAML
run "$GATES_NONE" "$GUARD_OK"
expect_rc "a rate-only gate with no preventionEvidence is never checked" 0
expect_out "zero-checked case says so explicitly" "all 0 gate(s)"
expect_not_out "zero-checked case names no finding" "FINDING"

# ---------------------------------------------------------------------------
# Case 6: real registry. Must be clean today -- SCOPE-3 bound preventionEvidence
# only to the four gates it verified are telemetry-capable.
# ---------------------------------------------------------------------------
set +e
REAL_OUT="$(bash "$TARGET" 2>&1)"
REAL_RC=$?
set -e
if [[ "$REAL_RC" -eq 0 ]]; then
  pass "the live registry's preventionEvidence gates all lint clean (exit 0)"
else
  fail "the live registry has an unverified preventionEvidence gate: $REAL_OUT"
fi

echo ""
echo "[gate-telemetry-capability-lint-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[gate-telemetry-capability-lint-selftest] OK"
