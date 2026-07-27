#!/usr/bin/env bash
set -uo pipefail

# domain-invariant-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/domain-invariant-guard.sh`
# (Gate G130 — domain_invariant_correspondence_gate).
#
# Stages disposable fake-repo fixtures (each with a `.github/bubbles-project.yaml`
# domainModel block + a specs/130-fixture spec dir) under a `mktemp -d`
# workspace and asserts the exit-code contract plus key output tokens for the
# guard's real behaviors: the core "declared-invariant-with-no-anchor" BLOCK,
# an adversarial provedBy test clearing it, enforcedBy code evidence clearing
# it, a justification clearing it, grandfathering, the no-domainModel no-op, the
# awk/grep fallback (yq disabled) still enforcing, and the `$ref` form.
#
# Scenarios:
#   S0   Missing feature dir argument                    → exit 2
#   S0b  Non-existent feature dir path                    → exit 2
#   S1   invariant declared, NO enforcedBy code evidence, → exit 1  (ADVERSARIAL:
#        provedBy test is HAPPY-path only, no justificn,     the core "prose-only
#        new spec                                            invariant" shape)
#   S2   same invariant WITH an adversarial provedBy test → exit 0  (rejection
#        (rejects a violating input)                          test clears it)
#   S3   same gap but enforcedBy db-constraint present in → exit 0  (code
#        a declared .sql migration (CHECK constraint)         evidence clears it)
#   S4   spec whose project config has NO domainModel     → exit 0  (no-op)
#   S5   same gap as S1 but a `## Domain-Invariant         → exit 0  (disclosure
#        Justifications` section discloses INV-id             clears it)
#   S6   same gap as S1 but createdAt before cutoff       → exit 0  (ADVERSARIAL:
#                                                            grandfather warn)
#   S7   same gap as S1, run with yq DISABLED             → exit 1  (ADVERSARIAL:
#        (BUBBLES_DOMAIN_INVARIANT_DISABLE_YQ=1)              awk fallback still
#                                                            parses + enforces)
#   S8   domainModel: { $ref: config/domain-model.yaml }  → exit 1  ($ref form
#        with an unanchored invariant, new spec              resolves + enforces)
#
# Reference:
#   bubbles/registry/gates.yaml → G130
#   bubbles/scripts/domain-invariant-guard.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/domain-invariant-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g130-selftest-XXXXXXXX)"
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

# Stage a fresh fake repo with a specs/130-fixture spec dir and emit the SPEC
# DIR path. The guard walks up from the spec dir to find the repo's
# .github/bubbles-project.yaml.
new_repo() {
  local name="$1"
  local root="$WORKSPACE/$name"
  mkdir -p "$root/.github" "$root/specs/130-fixture" "$root/src" "$root/tests" "$root/migrations" "$root/config"
  printf '%s' "$root"
}

write_state() {
  local spec="$1" created="$2"
  cat >"$spec/state.json" <<EOF
{
  "version": 3,
  "status": "in_progress",
  "createdAt": "$created"
}
EOF
}

# The canonical inline domainModel block used by most fixtures. Arg $1 is the
# provedBy entry so a fixture can choose a happy vs adversarial test.
write_inline_domain_model() {
  local root="$1" proved="$2"
  cat >"$root/.github/bubbles-project.yaml" <<EOF
scans: {}
domainModel:
  entities:
    Order: { states: [created, paid, shipped, refunded], terminal: [refunded] }
  invariants:
    - id: INV-ORDER-STATUS-ENUM
      rule: "Order.status in {created, paid, shipped, refunded}"
      kind: enumeration
      enforcedBy: [db-constraint, type]
      provedBy: ["$proved"]
docsRegistryOverrides: {}
EOF
}

# A plain-string Order impl with NO enum, NO db constraint — no enforcedBy
# code evidence for INV-ORDER-STATUS-ENUM.
write_unenforced_impl() {
  local root="$1"
  cat >"$root/src/order.rs" <<'EOF'
pub struct Order { pub status: String }

pub fn set_status(o: &mut Order, s: String) {
    o.status = s;
}
EOF
}

write_scopes_impl_rs() {
  local spec="$1"
  cat >"$spec/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/order.rs`
EOF
}

RC=""
OUT=""
run_guard() {
  OUT="$(bash "$GUARD_SCRIPT" "$@" 2>&1)"
  RC=$?
}
run_guard_no_yq() {
  OUT="$(BUBBLES_DOMAIN_INVARIANT_DISABLE_YQ=1 bash "$GUARD_SCRIPT" "$@" 2>&1)"
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
# S1: invariant declared, no enforcedBy code evidence, provedBy is happy-path
# only, no justification, new spec → exit 1 (ADVERSARIAL — the prose-only shape)
# -----------------------------------------------------------------------
r1="$(new_repo s1-unanchored)"
spec1="$r1/specs/130-fixture"
write_inline_domain_model "$r1" "tests/order_status_test.rs::test_happy_path"
write_unenforced_impl "$r1"
write_scopes_impl_rs "$spec1"
write_state "$spec1" "2026-07-28"
cat >"$spec1/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: The order status transitions through created, paid, shipped, refunded.
EOF
cat >"$r1/tests/order_status_test.rs" <<'EOF'
#[test]
fn test_happy_path() {
    let mut o = Order { status: String::new() };
    set_status(&mut o, "paid".into());
    assert_eq!(o.status, "paid");
}
EOF
run_guard "$spec1"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "G130 BLOCK"; then
  pass "S1 declared invariant with no anchor BLOCKs (exit 1)"
else
  bad "S1 expected exit 1 with BLOCK, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S2: same invariant WITH an adversarial provedBy test → exit 0
# -----------------------------------------------------------------------
r2="$(new_repo s2-adversarial-test)"
spec2="$r2/specs/130-fixture"
write_inline_domain_model "$r2" "tests/order_status_test.rs::rejects_unknown_status"
write_unenforced_impl "$r2"
write_scopes_impl_rs "$spec2"
write_state "$spec2" "2026-07-28"
cat >"$r2/tests/order_status_test.rs" <<'EOF'
#[test]
fn rejects_unknown_status() {
    let mut o = Order { status: String::new() };
    let result = try_set_status(&mut o, "bogus".into());
    assert!(result.is_err(), "an unknown status must be rejected");
}
EOF
run_guard "$spec2"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "adversarial provedBy test"; then
  pass "S2 adversarial provedBy test anchors the invariant (exit 0)"
else
  bad "S2 expected exit 0 with adversarial-test anchor, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S3: enforcedBy db-constraint present in a declared .sql migration → exit 0
# (isolates the enforcedBy code-evidence branch: provedBy is empty)
# -----------------------------------------------------------------------
r3="$(new_repo s3-code-evidence)"
spec3="$r3/specs/130-fixture"
cat >"$r3/.github/bubbles-project.yaml" <<'EOF'
domainModel:
  invariants:
    - id: INV-ORDER-STATUS-ENUM
      rule: "Order.status in {created, paid, shipped, refunded}"
      enforcedBy: [db-constraint]
      provedBy: []
EOF
cat >"$spec3/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `migrations/001_orders.sql`
EOF
cat >"$r3/migrations/001_orders.sql" <<'EOF'
-- create the orders table
CREATE TABLE orders (id UUID PRIMARY KEY, status TEXT NOT NULL);
ALTER TABLE orders ADD CONSTRAINT chk_status
    CHECK (status IN ('created', 'paid', 'shipped', 'refunded'));
EOF
write_state "$spec3" "2026-07-28"
cat >"$spec3/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: order status is constrained at the database layer.
EOF
run_guard "$spec3"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "enforcedBy code evidence"; then
  pass "S3 enforcedBy db-constraint in a .sql migration anchors the invariant (exit 0)"
else
  bad "S3 expected exit 0 with code-evidence anchor, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S4: project config with NO domainModel block → exit 0 (no-op)
# -----------------------------------------------------------------------
r4="$(new_repo s4-no-domainmodel)"
spec4="$r4/specs/130-fixture"
cat >"$r4/.github/bubbles-project.yaml" <<'EOF'
scans: {}
docsRegistryOverrides: {}
EOF
write_state "$spec4" "2026-07-28"
cat >"$spec4/spec.md" <<'EOF'
# Pagination

## Requirements

- FR-1: list returns 25 items per page.
EOF
cat >"$spec4/scopes.md" <<'EOF'
# Scopes

### Implementation Files

- `src/list.rs`
EOF
run_guard "$spec4"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "not applicable"; then
  pass "S4 no domainModel block is not applicable (exit 0 no-op)"
else
  bad "S4 expected exit 0 no-op, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S5: same gap as S1 but a Domain-Invariant Justifications section discloses
# the INV-id → exit 0
# -----------------------------------------------------------------------
r5="$(new_repo s5-justified)"
spec5="$r5/specs/130-fixture"
write_inline_domain_model "$r5" "tests/order_status_test.rs::test_happy_path"
write_unenforced_impl "$r5"
write_scopes_impl_rs "$spec5"
write_state "$spec5" "2026-07-28"
cat >"$r5/tests/order_status_test.rs" <<'EOF'
#[test]
fn test_happy_path() { assert_eq!(1, 1); }
EOF
cat >"$spec5/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: order status set is enforced upstream.

## Domain-Invariant Justifications

- INV-ORDER-STATUS-ENUM: the status enum is owned and enforced by the upstream
  payment gateway's own state machine; this read-only mirror service does not
  re-enforce it. Reviewed and accepted.
EOF
run_guard "$spec5"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "justification discloses"; then
  pass "S5 Domain-Invariant justification clears the finding (exit 0)"
else
  bad "S5 expected exit 0 justified, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S6: same gap as S1 but createdAt before cutoff → exit 0 (grandfathered)
# (ADVERSARIAL — proves the grandfather downgrade)
# -----------------------------------------------------------------------
r6="$(new_repo s6-grandfathered)"
spec6="$r6/specs/130-fixture"
write_inline_domain_model "$r6" "tests/order_status_test.rs::test_happy_path"
write_unenforced_impl "$r6"
write_scopes_impl_rs "$spec6"
write_state "$spec6" "2026-05-01"
cat >"$r6/tests/order_status_test.rs" <<'EOF'
#[test]
fn test_happy_path() { assert_eq!(1, 1); }
EOF
cat >"$spec6/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: order status transitions.
EOF
run_guard "$spec6"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qi "grandfathered\|DOWNGRADED"; then
  pass "S6 pre-cutoff spec is grandfathered to warning (exit 0)"
else
  bad "S6 expected exit 0 grandfathered, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S7: same gap as S1 but yq DISABLED → exit 1 (ADVERSARIAL — the awk/grep
# fallback still parses the flow-style invariant AND enforces the finding)
# -----------------------------------------------------------------------
r7="$(new_repo s7-awk-fallback)"
spec7="$r7/specs/130-fixture"
write_inline_domain_model "$r7" "tests/order_status_test.rs::test_happy_path"
write_unenforced_impl "$r7"
write_scopes_impl_rs "$spec7"
write_state "$spec7" "2026-07-28"
cat >"$r7/tests/order_status_test.rs" <<'EOF'
#[test]
fn test_happy_path() { assert_eq!(1, 1); }
EOF
cat >"$spec7/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: order status transitions.
EOF
run_guard_no_yq "$spec7"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "G130 BLOCK"; then
  pass "S7 awk/grep fallback (yq disabled) parses + enforces the finding (exit 1)"
else
  bad "S7 expected exit 1 via fallback, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# S8: domainModel: { $ref: config/domain-model.yaml } with an unanchored
# invariant → exit 1 ($ref form resolves + enforces)
# -----------------------------------------------------------------------
r8="$(new_repo s8-ref-form)"
spec8="$r8/specs/130-fixture"
cat >"$r8/.github/bubbles-project.yaml" <<'EOF'
scans: {}
domainModel: { $ref: config/domain-model.yaml }
EOF
cat >"$r8/config/domain-model.yaml" <<'EOF'
entities:
  Order: { states: [created, paid, shipped, refunded], terminal: [refunded] }
invariants:
  - id: INV-ORDER-STATUS-ENUM
    rule: "Order.status in {created, paid, shipped, refunded}"
    enforcedBy: [db-constraint, type]
    provedBy: ["tests/order_status_test.rs::test_happy_path"]
EOF
write_unenforced_impl "$r8"
write_scopes_impl_rs "$spec8"
write_state "$spec8" "2026-07-28"
cat >"$r8/tests/order_status_test.rs" <<'EOF'
#[test]
fn test_happy_path() { assert_eq!(1, 1); }
EOF
cat >"$spec8/spec.md" <<'EOF'
# Order Service

## Requirements

- FR-1: order status transitions.
EOF
run_guard "$spec8"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q "G130 BLOCK"; then
  pass "S8 \$ref form resolves the model file and enforces the finding (exit 1)"
else
  bad "S8 expected exit 1 via \$ref, got $RC; out=$OUT"
fi

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------
echo
echo "============================================================"
echo "  domain-invariant-guard selftest verdict"
echo "    passed assertions: $PASS_COUNT"
echo "    failed assertions: $FAIL_COUNT"
echo "============================================================"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  FAILED: %s\n' "${FAILED_SCENARIOS[@]}" >&2
  echo "domain-invariant-guard-selftest: FAILED" >&2
  exit 1
fi
echo "domain-invariant-guard-selftest: PASSED"
exit 0
