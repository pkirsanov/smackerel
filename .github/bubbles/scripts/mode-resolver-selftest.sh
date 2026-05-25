#!/usr/bin/env bash
# bubbles/scripts/mode-resolver-selftest.sh
#
# Selftest for mode-resolver.sh — exercises template inheritance,
# array dedup + sort, scalar latest-wins, deep-merge, cycle detection,
# unknown-template rejection, and real-registry --validate.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
REAL_WORKFLOWS="$ROOT_DIR/bubbles/workflows.yaml"

# HOME-based TMP_DIR — snap-confined yq cannot access /tmp.
_selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$_selftest_tmp_base"
TMP_DIR="$(mktemp -d -p "$_selftest_tmp_base" bubbles-mode-resolver-test.XXXXXX)"
selftest_timeout_seconds="${BUBBLES_MODE_RESOLVER_SELFTEST_TIMEOUT_SECONDS:-30}"

failures=0
cleanup() {
  if (( failures == 0 )) && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "Preserving selftest workspace: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

# Run resolver against a fixture, capturing stdout+stderr+exit.
# Args: $1 fixture-path  $2 args...
# Sets RC, OUT (combined output).
run_resolver() {
  local fixture="$1"; shift
  local out_file
  out_file="$(mktemp -p "$TMP_DIR")"
  set +e
  timeout "$selftest_timeout_seconds" env BUBBLES_WORKFLOWS_FILE="$fixture" "$RESOLVER" "$@" > "$out_file" 2>&1
  RC=$?
  set -e
  OUT="$(cat "$out_file")"
}

# -----------------------------------------------------------------------
# Fixture 1: single-template inheritance — scalar flows through.
# -----------------------------------------------------------------------
fixture1="$TMP_DIR/fixture1.yaml"
cat > "$fixture1" <<'YAML'
gates:
  G001: { id: G001, label: example, definition: ex }
modeTemplates:
  base:
    statusCeiling: done
modes:
  example:
    description: Example mode
    inherits: [ base ]
    phaseOrder: [ implement ]
    requiredGates: [ G001 ]
YAML

run_resolver "$fixture1" example
if (( RC == 0 )) && grep -qE '^statusCeiling:[[:space:]]*done$' <<< "$OUT"; then
  pass "Case 1: single-template inheritance — scalar flows through"
else
  fail "Case 1: single-template inheritance — scalar flows through (rc=$RC)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 2: multi-template with overlapping arrays — concat + dedup +
# requiredGates sort.
# -----------------------------------------------------------------------
fixture2="$TMP_DIR/fixture2.yaml"
cat > "$fixture2" <<'YAML'
gates:
  G001: { id: G001, label: a, definition: a }
  G002: { id: G002, label: b, definition: b }
  G003: { id: G003, label: c, definition: c }
modeTemplates:
  bundle-a:
    requiredGates: [ G003, G001 ]
  bundle-b:
    requiredGates: [ G002, G003 ]
modes:
  combo:
    description: Combo
    inherits: [ bundle-a, bundle-b ]
    requiredGates: [ G001 ]
YAML

run_resolver "$fixture2" combo
# Compare as compact JSON arrays — yq output style (flow vs block) varies.
gates_json="$(yq -o=json -I=0 '.requiredGates' <<< "$OUT" 2>/dev/null || echo 'BAD')"
if (( RC == 0 )) && [[ "$gates_json" == '["G001","G002","G003"]' ]]; then
  pass "Case 2: multi-template arrays concat + dedup + sort"
else
  fail "Case 2: multi-template arrays concat + dedup + sort (rc=$RC, gates=$gates_json)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 3: override semantics — mode scalar wins over template scalar.
# -----------------------------------------------------------------------
fixture3="$TMP_DIR/fixture3.yaml"
cat > "$fixture3" <<'YAML'
gates: {}
modeTemplates:
  base:
    statusCeiling: done
    constraints:
      foo: 1
      bar: 2
modes:
  override:
    description: Override mode
    inherits: [ base ]
    statusCeiling: in_progress
    constraints:
      bar: 99
      baz: 3
YAML

run_resolver "$fixture3" override
sc="$(yq -r '.statusCeiling' <<< "$OUT")"
foo="$(yq -r '.constraints.foo' <<< "$OUT")"
bar="$(yq -r '.constraints.bar' <<< "$OUT")"
baz="$(yq -r '.constraints.baz' <<< "$OUT")"
if (( RC == 0 )) && [[ "$sc" == "in_progress" && "$foo" == "1" && "$bar" == "99" && "$baz" == "3" ]]; then
  pass "Case 3: override semantics — mode wins over template"
else
  fail "Case 3: override semantics — mode wins over template (rc=$RC, sc=$sc, foo=$foo, bar=$bar, baz=$baz)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 4: cycle detection — template A → B → A.
# -----------------------------------------------------------------------
fixture4="$TMP_DIR/fixture4.yaml"
cat > "$fixture4" <<'YAML'
gates: {}
modeTemplates:
  alpha:
    inherits: [ beta ]
    statusCeiling: done
  beta:
    inherits: [ alpha ]
    statusCeiling: done
modes:
  cyclic:
    description: Cyclic
    inherits: [ alpha ]
YAML

run_resolver "$fixture4" cyclic
if (( RC != 0 )) && grep -qi 'cycle detected' <<< "$OUT"; then
  pass "Case 4: cycle detection — resolver rejects template cycle"
else
  fail "Case 4: cycle detection — resolver rejects template cycle (rc=$RC)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 5: unknown template reference.
# -----------------------------------------------------------------------
fixture5="$TMP_DIR/fixture5.yaml"
cat > "$fixture5" <<'YAML'
gates: {}
modeTemplates:
  real:
    statusCeiling: done
modes:
  ghost:
    description: Ghost
    inherits: [ does-not-exist ]
YAML

run_resolver "$fixture5" ghost
if (( RC != 0 )) && grep -qi 'unknown template' <<< "$OUT"; then
  pass "Case 5: unknown template reference rejected"
else
  fail "Case 5: unknown template reference rejected (rc=$RC)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 6: real workflows.yaml --validate.
# -----------------------------------------------------------------------
run_resolver "$REAL_WORKFLOWS" --validate
if (( RC == 0 )) && grep -q 'Validation passed' <<< "$OUT"; then
  pass "Case 6: real workflows.yaml --validate"
else
  fail "Case 6: real workflows.yaml --validate (rc=$RC)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 7: SCOPE-13 inherited spec-review default flows from template.
# -----------------------------------------------------------------------
fixture7="$TMP_DIR/fixture7.yaml"
cat > "$fixture7" <<'YAML'
gates: {}
modeTemplates:
  base-delivery:
    statusCeiling: done
  delivery-quality-constraints:
    constraints:
      specReviewDefault: once-before-implement
modes:
  inherited-delivery:
    description: Inherited delivery mode
    inherits: [ base-delivery, delivery-quality-constraints ]
YAML

run_resolver "$fixture7" inherited-delivery
sr_default="$(yq -r '.constraints.specReviewDefault // "MISSING"' <<< "$OUT" 2>/dev/null || echo 'BAD')"
if (( RC == 0 )) && [[ "$sr_default" == "once-before-implement" ]]; then
  pass "Case 7: SCOPE-13 inherited spec-review default flows from template"
else
  fail "Case 7: SCOPE-13 inherited spec-review default flows from template (rc=$RC, specReviewDefault=$sr_default)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 8: SCOPE-13 explicit mode opt-out remains machine-readable.
# -----------------------------------------------------------------------
fixture8="$TMP_DIR/fixture8.yaml"
cat > "$fixture8" <<'YAML'
gates: {}
modeTemplates:
  base-delivery:
    statusCeiling: done
  delivery-quality-constraints:
    constraints:
      specReviewDefault: once-before-implement
      specReviewOptOutRequiresReason: true
modes:
  deliberate-opt-out:
    description: Deliberate opt-out mode
    inherits: [ base-delivery, delivery-quality-constraints ]
    constraints:
      specReviewDefault: off
      specReviewOptOutReason: read-only spec-review-only mode
YAML

run_resolver "$fixture8" deliberate-opt-out
sr_override="$(yq -r '.constraints.specReviewDefault // "MISSING"' <<< "$OUT" 2>/dev/null || echo 'BAD')"
sr_reason="$(yq -r '.constraints.specReviewOptOutReason // "MISSING"' <<< "$OUT" 2>/dev/null || echo 'BAD')"
if (( RC == 0 )) && [[ "$sr_override" == "off" && "$sr_reason" != "MISSING" ]]; then
  pass "Case 8: SCOPE-13 explicit mode opt-out remains machine-readable"
else
  fail "Case 8: SCOPE-13 explicit mode opt-out remains machine-readable (rc=$RC, specReviewDefault=$sr_override, reason=$sr_reason)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 9: real inherited done-ceiling delivery mode receives default.
# -----------------------------------------------------------------------
run_resolver "$REAL_WORKFLOWS" bugfix-fastlane
real_sr_default="$(yq -r '.constraints.specReviewDefault // "MISSING"' <<< "$OUT" 2>/dev/null || echo 'BAD')"
if (( RC == 0 )) && [[ "$real_sr_default" == "once-before-implement" ]]; then
  pass "Case 9: real inherited done-ceiling mode receives spec-review default"
else
  fail "Case 9: real inherited done-ceiling mode receives spec-review default (rc=$RC, specReviewDefault=$real_sr_default)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Fixture 10: SCOPE-13/G091 planning-chain metadata validates without full
# mode materialization.
# -----------------------------------------------------------------------
fixture10="$TMP_DIR/fixture10.yaml"
cat > "$fixture10" <<'YAML'
gates: {}
modeTemplates:
  base-delivery:
    statusCeiling: done
  delivery-quality-constraints:
    constraints:
      specReviewDefault: once-before-implement
      specReviewDefaultScope: done-ceiling-delivery-modes
      specReviewOptOutRequiresReason: true
      requireCanonicalPlanningChain: true
      planningChainAgents: [ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]
modes:
  delivery-with-chain:
    description: Delivery mode with canonical planning-chain metadata
    inherits: [ base-delivery, delivery-quality-constraints ]
    phaseOrder: [ select, bootstrap, implement ]
    constraints:
      bootstrapAgents: [ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]
      improvementPreludeProfiles:
        analyze-ux-design-plan: [ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]
  read-only-opt-out:
    description: Read-only opt-out mode
    statusCeiling: validated
    phaseOrder: [ select, validate, finalize ]
    constraints:
      modeClass: validate-only
      specReviewDefault: off
      specReviewOptOutReason: validate-only has no implementation-capable phase
YAML

run_resolver "$fixture10" --validate
if (( RC == 0 )) && grep -q 'Validation passed' <<< "$OUT"; then
  pass "Case 10: SCOPE-13/G091 planning-chain metadata validates without stall"
else
  fail "Case 10: SCOPE-13/G091 planning-chain metadata validates without stall (rc=$RC)"
  echo "$OUT" >&2
fi

# -----------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------
echo
if (( failures == 0 )); then
  echo "All mode-resolver selftests passed."
  exit 0
else
  echo "FAILED: $failures selftest(s) failed." >&2
  exit 1
fi
