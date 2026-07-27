#!/usr/bin/env bash
set -uo pipefail

# domain-model-consistency-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/domain-model-consistency.sh`
# (Gate G131 — domain_model_consistency_gate, advisory-until-configured).
#
# Stages disposable fake-repo fixtures (each with a `.github/bubbles-project.yaml`
# domainModel block + a specs/131-fixture spec dir) under a `mktemp -d` workspace
# and asserts the exit-code contract plus key output tokens for the guard's real
# behaviors. Every positive nudge scenario is paired with a NON-TAUTOLOGICAL
# check: a domain concept that IS in the shared model must NOT be nudged, and a
# fully-promoted feature must produce ZERO nudges — proving the guard genuinely
# DIFFS against the shared `domainModel`, not "nudge everything".
#
# Scenarios:
#   S0   Missing feature dir argument                       → exit 2
#   S0b  Non-existent feature dir path                       → exit 2
#   A    design.md `## Data Model` declares Order (in shared → exit 0, advisory
#        model) AND Shipment (NOT in shared model)             nudge NAMES Shipment
#                                                              but NOT Order
#   B    Outcome-Contract Hard Constraint references         → exit 0, advisory
#        INV-ORDER-STATUS-ENUM (in shared) AND                 nudge NAMES the
#        INV-SHIPMENT-TRACKED (NOT in shared)                  unknown INV, not the
#                                                              known one
#   C    project config with NO domainModel block            → exit 0 (no-op)
#   D    same unpromoted entity as A but                      → exit 1 (blocks under
#        `domainModelConsistencyGuard: block` set              the opt-in)
#   E    fully-promoted feature (only known entity +          → exit 0, ZERO nudges
#        known invariant)                                       (strong non-taut.)
#   F    same unpromoted entity as A, yq DISABLED             → exit 0, advisory
#        (BUBBLES_DOMAIN_MODEL_DISABLE_YQ=1)                    nudge (awk fallback
#                                                              parses the shared
#                                                              model + still diffs)
#
# Reference:
#   bubbles/registry/gates.yaml → G131
#   bubbles/scripts/domain-model-consistency.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/domain-model-consistency.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g131-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# Stage a fresh fake repo with a specs/131-fixture spec dir and emit the SPEC
# DIR path. The guard walks up from the spec dir to find the repo's
# .github/bubbles-project.yaml.
new_repo() {
  local name="$1"
  local root="$WORKSPACE/$name"
  mkdir -p "$root/.github" "$root/specs/131-fixture" "$root/config"
  printf '%s' "$root"
}

# The canonical inline domainModel block: shared model knows ONLY `Order` and
# INV-ORDER-STATUS-ENUM. Arg $1 (optional) is appended verbatim (e.g. the opt-in
# `domainModelConsistencyGuard: block` line).
write_inline_domain_model() {
  local root="$1" extra="${2:-}"
  cat >"$root/.github/bubbles-project.yaml" <<EOF
scans: {}
${extra}domainModel:
  entities:
    Order: { states: [created, paid, shipped, refunded], terminal: [refunded] }
  invariants:
    - id: INV-ORDER-STATUS-ENUM
      rule: "Order.status in {created, paid, shipped, refunded}"
      kind: enumeration
docsRegistryOverrides: {}
EOF
}

RC=""
OUT=""
run_guard() {
  OUT="$(bash "$GUARD_SCRIPT" "$@" 2>&1)"
  RC=$?
}
run_guard_no_yq() {
  OUT="$(BUBBLES_DOMAIN_MODEL_DISABLE_YQ=1 bash "$GUARD_SCRIPT" "$@" 2>&1)"
  RC=$?
}

# -----------------------------------------------------------------------
# S0: missing feature dir argument → exit 2
# -----------------------------------------------------------------------
run_guard
if [[ "$RC" -eq 2 ]]; then pass "S0 missing feature dir exits 2"; else bad "S0 expected exit 2, got $RC"; fi

# -----------------------------------------------------------------------
# S0b: non-existent feature dir path → exit 2
# -----------------------------------------------------------------------
run_guard "$WORKSPACE/does-not-exist-$$"
if [[ "$RC" -eq 2 ]]; then pass "S0b non-existent feature dir exits 2"; else bad "S0b expected exit 2, got $RC"; fi

# -----------------------------------------------------------------------
# A: `## Data Model` declares a KNOWN entity (Order) and an UNKNOWN entity
# (Shipment) → exit 0 advisory; nudge NAMES Shipment but NOT Order.
# -----------------------------------------------------------------------
ra="$(new_repo a-entity-nudge)"
speca="$ra/specs/131-fixture"
write_inline_domain_model "$ra"
cat >"$speca/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: shipments are created for paid orders.
EOF
cat >"$speca/design.md" <<'EOF'
# Design: Order Service

## Data Model

### Order

The order aggregate (already in the shared model).

### Shipment

A new shipment aggregate introduced by this feature.
EOF
run_guard "$speca"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q "G131 NUDGE" \
  && printf '%s' "$OUT" | grep -q "entity 'Shipment'" \
  && ! printf '%s' "$OUT" | grep -q "entity 'Order'"; then
  pass "A unpromoted entity Shipment is nudged, promoted entity Order is NOT (exit 0 advisory)"
else
  bad "A expected exit 0 + Shipment nudge + no Order nudge, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# B: Outcome-Contract Hard Constraint references a KNOWN invariant
# (INV-ORDER-STATUS-ENUM) and an UNKNOWN one (INV-SHIPMENT-TRACKED) → exit 0
# advisory; nudge NAMES the unknown INV but NOT the known one.
# -----------------------------------------------------------------------
rb="$(new_repo b-invariant-nudge)"
specb="$rb/specs/131-fixture"
write_inline_domain_model "$rb"
cat >"$specb/spec.md" <<'EOF'
# Order Service

## Outcome Contract
**Intent:** Ship paid orders.
**Success Signal:** A paid order produces exactly one tracked shipment.
**Hard Constraints:** Order.status is a closed enum (INV-ORDER-STATUS-ENUM) and every shipment is tracked (INV-SHIPMENT-TRACKED).
**Failure Condition:** A shipment is created without tracking.

## Requirements

- FR-1: shipments are tracked.
EOF
cat >"$specb/design.md" <<'EOF'
# Design: Order Service

## Data Model

### Order

The order aggregate (already in the shared model).
EOF
run_guard "$specb"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q "G131 NUDGE" \
  && printf '%s' "$OUT" | grep -q "invariant 'INV-SHIPMENT-TRACKED'" \
  && ! printf '%s' "$OUT" | grep -q "invariant 'INV-ORDER-STATUS-ENUM'"; then
  pass "B undeclared Hard-Constraint invariant is nudged, declared one is NOT (exit 0 advisory)"
else
  bad "B expected exit 0 + INV-SHIPMENT-TRACKED nudge + no INV-ORDER-STATUS-ENUM nudge, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# C: project config with NO domainModel block → exit 0 (no-op)
# -----------------------------------------------------------------------
rc="$(new_repo c-no-domainmodel)"
specc="$rc/specs/131-fixture"
cat >"$rc/.github/bubbles-project.yaml" <<'EOF'
scans: {}
docsRegistryOverrides: {}
EOF
cat >"$specc/design.md" <<'EOF'
# Design

## Data Model

### Pagination

A per-page cursor (no shared domain model in this repo).
EOF
run_guard "$specc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "not applicable"; then
  pass "C no domainModel block is not applicable (exit 0 no-op)"
else
  bad "C expected exit 0 no-op, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# D: same unpromoted entity as A but domainModelConsistencyGuard: block set
# → exit 1 (the advisory nudge becomes a blocking finding under the opt-in).
# -----------------------------------------------------------------------
rd="$(new_repo d-block-optin)"
specd="$rd/specs/131-fixture"
write_inline_domain_model "$rd" $'domainModelConsistencyGuard: block\n'
cat >"$specd/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: shipments exist.
EOF
cat >"$specd/design.md" <<'EOF'
# Design: Order Service

## Data Model

### Shipment

A new shipment aggregate not promoted to the shared model.
EOF
run_guard "$specd"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "FAILING"; then
  pass "D domainModelConsistencyGuard: block turns the nudge into a blocking finding (exit 1)"
else
  bad "D expected exit 1 with FAILING under opt-in, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# E: fully-promoted feature (only the KNOWN entity + KNOWN invariant) → exit 0
# with ZERO nudges (the strongest non-tautology proof).
# -----------------------------------------------------------------------
re="$(new_repo e-clean)"
spece="$re/specs/131-fixture"
write_inline_domain_model "$re"
cat >"$spece/spec.md" <<'EOF'
# Order Service

## Outcome Contract
**Intent:** Keep order status closed.
**Hard Constraints:** Order.status stays a closed enum (INV-ORDER-STATUS-ENUM).
**Failure Condition:** An unknown status is accepted.

## Requirements

- FR-1: closed enum.
EOF
cat >"$spece/design.md" <<'EOF'
# Design: Order Service

## Data Model

### Order

The order aggregate (already in the shared model).
EOF
run_guard "$spece"
if [[ "$RC" -eq 0 ]] && ! printf '%s' "$OUT" | grep -q "G131 NUDGE"; then
  pass "E fully-promoted feature produces ZERO nudges (exit 0, non-tautological)"
else
  bad "E expected exit 0 with no nudge, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# F: same unpromoted entity as A but yq DISABLED → exit 0 advisory; the awk/grep
# fallback still parses the shared entities and diffs (ADVERSARIAL fallback).
# -----------------------------------------------------------------------
rf="$(new_repo f-awk-fallback)"
specf="$rf/specs/131-fixture"
write_inline_domain_model "$rf"
cat >"$specf/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: shipments exist.
EOF
cat >"$specf/design.md" <<'EOF'
# Design: Order Service

## Data Model

### Order

The order aggregate (already in the shared model).

### Shipment

A new shipment aggregate introduced by this feature.
EOF
run_guard_no_yq "$specf"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q "entity 'Shipment'" \
  && ! printf '%s' "$OUT" | grep -q "entity 'Order'"; then
  pass "F awk/grep fallback (yq disabled) parses the shared model + nudges only the unpromoted entity (exit 0)"
else
  bad "F expected exit 0 + Shipment nudge via fallback, got RC=$RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------
echo
echo "============================================================"
echo "  domain-model-consistency selftest verdict"
echo "    passed assertions: $PASS_COUNT"
echo "    failed assertions: $FAIL_COUNT"
echo "============================================================"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  FAILED: %s\n' "${FAILED_SCENARIOS[@]}" >&2
  echo "domain-model-consistency-selftest: FAILED" >&2
  exit 1
fi
echo "all cases passed."
echo "domain-model-consistency-selftest: PASSED"
exit 0
