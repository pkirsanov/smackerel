#!/usr/bin/env bash
# Hermetic selftest for vertical-delivery-plan-guard.sh (BFW-02 / IMP-022).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/vertical-delivery-plan-guard.sh"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# Foundation-only scope: DB/service/model boilerplate, NO consumer surface.
foundation_scope() { # $1=n $2=name
  cat <<EOF
## Scope $1: $2 persistence
Status: [ ] Not started
### Use Cases (Gherkin)
Given the persistent store
When a migration runs
Then the $2 table exists
### Implementation Plan
- DB schema/migrations: add $2 table
- Components/files: migrations/00$1_$2.sql
- service layer: ${2}Repository business logic
EOF
}

# Consumer-visible scope: HTTP route + dashboard/frontend surface.
consumer_scope() { # $1=n $2=name
  cat <<EOF
## Scope $1: $2 surface
Status: [ ] Not started
### Use Cases (Gherkin)
Given an authenticated user
When they GET /api/v1/$2
Then the dashboard renders the result
### Implementation Plan
- API endpoints: GET /api/v1/$2 wired via .route()
- Components/files: frontend dashboard page for $2
EOF
}

echo "Running vertical-delivery-plan-guard selftest..."

# ── T1: horizontal (8 foundation scopes → first consumer at scope 9) → advisory warn, exit 0
d="$TMP_ROOT/t1"; mkdir -p "$d"
{ for n in 1 2 3 4 5 6 7 8; do foundation_scope "$n" "layer$n"; echo; done; consumer_scope 9 "profile"; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'HORIZONTAL PLAN' && printf '%s\n' "$out" | grep -q 'first consumer-visible increment is scope 9'; then
  pass "T1 horizontal plan (consumer deferred to scope 9) warns advisorily (exit 0)"
else
  fail "T1 horizontal plan should warn advisorily naming scope 9 (rc=$rc)"
fi

# ── T2: vertical (consumer at scope 1) → OK, exit 0
d="$TMP_ROOT/t2"; mkdir -p "$d"
{ consumer_scope 1 "profile"; echo; foundation_scope 2 "audit"; echo; consumer_scope 3 "settings"; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'first usable increment is early'; then
  pass "T2 vertical plan (early consumer at scope 1) passes clean (exit 0)"
else
  fail "T2 vertical plan should pass clean (rc=$rc)"
fi

# ── T3: horizontal + verticalPlanGuard: block opt-in → exit 1
d="$TMP_ROOT/blockrepo"; mkdir -p "$d/.github" "$d/specs/feat"
printf 'verticalPlanGuard: block\n' > "$d/.github/bubbles-project.yaml"
{ for n in 1 2 3 4; do foundation_scope "$n" "layer$n"; echo; done; consumer_scope 5 "profile"; } > "$d/specs/feat/scopes.md"
out="$( (cd "$d" && "$GUARD" specs/feat) 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s\n' "$out" | grep -q 'verticalPlanGuard: block'; then
  pass "T3 horizontal plan with verticalPlanGuard: block FAILS (exit 1)"
else
  fail "T3 horizontal plan with block config should fail (rc=$rc)"
fi

# ── T4: small plan (2 foundation scopes, below threshold) → OK, exit 0
d="$TMP_ROOT/t4"; mkdir -p "$d"
{ foundation_scope 1 "schema"; echo; foundation_scope 2 "service"; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'below the horizontal-chain threshold'; then
  pass "T4 small plan (2 foundation scopes) is not flagged (below threshold)"
else
  fail "T4 small plan should not be flagged (rc=$rc)"
fi

# ── T5: no-consumer multi-scope plan (4 foundation, none consumer) → advisory, exit 0
d="$TMP_ROOT/t5"; mkdir -p "$d"
{ for n in 1 2 3 4; do foundation_scope "$n" "layer$n"; echo; done; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'no scope delivers a consumer-visible'; then
  pass "T5 no-consumer multi-scope plan warns advisorily (exit 0)"
else
  fail "T5 no-consumer plan should warn advisorily (rc=$rc)"
fi

# ── T6: per-scope-directory layout, vertical (consumer at scope 01) → OK, exit 0
d="$TMP_ROOT/t6"; mkdir -p "$d/scopes/01-api" "$d/scopes/02-db"
consumer_scope 1 "profile" > "$d/scopes/01-api/scope.md"
foundation_scope 2 "audit" > "$d/scopes/02-db/scope.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'first usable increment is early'; then
  pass "T6 per-scope-directory layout parsed (consumer at scope 1 = clean)"
else
  fail "T6 per-scope-dir layout should be parsed (rc=$rc)"
fi

# Low-risk tier marker: state.json on the rapid-tool-delivery fast lane.
write_rapid_state() { printf '%s\n' '{ "workflowMode": "rapid-tool-delivery" }' > "$1/state.json"; }

# ── T7: low-risk tier, 6 consumer scopes (early consumer, so NOT horizontal) →
#        over scope budget (>5) → advisory SCOPE BUDGET warn, exit 0.
d="$TMP_ROOT/t7"; mkdir -p "$d"; write_rapid_state "$d"
{ for n in 1 2 3 4 5 6; do consumer_scope "$n" "surface$n"; echo; done; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'SCOPE BUDGET (low-risk tier)' && printf '%s\n' "$out" | grep -q 'this plan has'; then
  pass "T7 low-risk tier over budget (6>5 scopes) warns advisorily (exit 0)"
else
  fail "T7 low-risk over-budget plan should warn advisorily (rc=$rc)"
fi

# ── T8: low-risk tier, exactly 5 consumer scopes → within budget → OK, exit 0.
d="$TMP_ROOT/t8"; mkdir -p "$d"; write_rapid_state "$d"
{ for n in 1 2 3 4 5; do consumer_scope "$n" "surface$n"; echo; done; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'within scope budget' && ! printf '%s\n' "$out" | grep -q 'SCOPE BUDGET (low-risk tier)'; then
  pass "T8 low-risk tier at budget (5 scopes) passes clean (exit 0)"
else
  fail "T8 low-risk at-budget plan should pass clean (rc=$rc)"
fi

# ── T9: low-risk tier over budget + verticalPlanGuard: block opt-in → exit 1.
d="$TMP_ROOT/budgetblock"; mkdir -p "$d/.github" "$d/specs/feat"
printf 'verticalPlanGuard: block\n' > "$d/.github/bubbles-project.yaml"
write_rapid_state "$d/specs/feat"
{ for n in 1 2 3 4 5 6; do consumer_scope "$n" "surface$n"; echo; done; } > "$d/specs/feat/scopes.md"
out="$( (cd "$d" && "$GUARD" specs/feat) 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s\n' "$out" | grep -q 'SCOPE BUDGET (low-risk tier)' && printf '%s\n' "$out" | grep -q 'verticalPlanGuard: block'; then
  pass "T9 low-risk over-budget with verticalPlanGuard: block FAILS (exit 1)"
else
  fail "T9 low-risk over-budget with block config should fail (rc=$rc)"
fi

# ── T10: NON-low-risk mode (full-delivery), 6 consumer scopes → budget NOT
#         imposed (unbounded like before) → OK, exit 0. Proves tier binding.
d="$TMP_ROOT/t10"; mkdir -p "$d"
printf '%s\n' '{ "workflowMode": "full-delivery" }' > "$d/state.json"
{ for n in 1 2 3 4 5 6; do consumer_scope "$n" "surface$n"; echo; done; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && ! printf '%s\n' "$out" | grep -q 'SCOPE BUDGET'; then
  pass "T10 non-low-risk mode (full-delivery) is unbounded (6 scopes, no budget finding)"
else
  fail "T10 non-low-risk mode should be unbounded (rc=$rc)"
fi

# ── T11: absent state.json, 6 consumer scopes → conservative (NOT low-risk) →
#         no budget imposed → OK, exit 0.
d="$TMP_ROOT/t11"; mkdir -p "$d"
{ for n in 1 2 3 4 5 6; do consumer_scope "$n" "surface$n"; echo; done; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && ! printf '%s\n' "$out" | grep -q 'SCOPE BUDGET'; then
  pass "T11 absent state.json is conservative (no budget imposed, exit 0)"
else
  fail "T11 absent state.json should impose no budget (rc=$rc)"
fi

# ── T12: Feature-010-shaped 14-scope HORIZONTAL plan (11 foundation layers →
#         first consumer at scope 12) → advisory HORIZONTAL PLAN + remediation,
#         exit 0. The negative fixture at the real pathological scale (IMP-022 SCOPE-6).
d="$TMP_ROOT/t12"; mkdir -p "$d"
{ for n in 1 2 3 4 5 6 7 8 9 10 11; do foundation_scope "$n" "layer$n"; echo; done; \
  consumer_scope 12 "profile"; echo; consumer_scope 13 "settings"; echo; consumer_scope 14 "admin"; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] \
  && printf '%s\n' "$out" | grep -q 'HORIZONTAL PLAN' \
  && printf '%s\n' "$out" | grep -q 'first consumer-visible increment is scope 12 of 14' \
  && printf '%s\n' "$out" | grep -q 'Remediation: restructure'; then
  pass "T12 Feature-010-shaped 14-scope horizontal plan warns with remediation (exit 0)"
else
  fail "T12 Feature-010 14-scope horizontal plan should warn with remediation (rc=$rc)"
fi

# ── T13: vertical TWIN of the same 14 scopes (early consumer at scope 1, work
#         reorganized) → clean, exit 0, NOT flagged. Proves the guard rewards a
#         reorganized-but-equivalent vertical plan (IMP-022 SCOPE-6 positive twin).
d="$TMP_ROOT/t13"; mkdir -p "$d"
{ consumer_scope 1 "profile"; echo; \
  for n in 2 3 4 5 6 7 8 9 10 11 12; do foundation_scope "$n" "layer$n"; echo; done; \
  consumer_scope 13 "settings"; echo; consumer_scope 14 "admin"; } > "$d/scopes.md"
out="$("$GUARD" "$d" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] \
  && printf '%s\n' "$out" | grep -q 'first usable increment is early' \
  && ! printf '%s\n' "$out" | grep -q 'HORIZONTAL PLAN'; then
  pass "T13 vertical twin (same 14 scopes, early consumer) passes clean (exit 0)"
else
  fail "T13 vertical twin should pass clean (rc=$rc)"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "vertical-delivery-plan-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "vertical-delivery-plan-guard-selftest: all cases passed."
