#!/usr/bin/env bash
set -uo pipefail

# Selftest for bubbles/scripts/scope-universe-resolver.py (BUG-026 F001).
# Proves the current-scope applicable-universe projection (omit iff a
# transitive descendant is not_started) and the fail-closed v3 contract.
#
# Exit codes: 0 all pass, 1 a contract failure, 2 harness error.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/scope-universe-resolver.py"

if [[ ! -f "$RESOLVER" ]]; then
  printf 'scope-universe-resolver-selftest: missing %s\n' "$RESOLVER" >&2
  exit 2
fi
if ! command -v python3 >/dev/null 2>&1; then
  printf 'scope-universe-resolver-selftest: SKIP (python3 not installed)\n'
  exit 0
fi
for c in mktemp rm; do
  command -v "$c" >/dev/null 2>&1 || { printf 'missing command %s\n' "$c" >&2; exit 2; }
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/scope-universe-resolver-selftest-XXXXXX")" || { printf 'mktemp failed\n' >&2; exit 2; }
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }

# write_state <dir> <json>
write_state() { mkdir -p "$1"; printf '%s\n' "$2" > "$1/state.json"; }
run_resolver() { python3 "$RESOLVER" "$1" current-scope; }

# ---------------------------------------------------------------------------
# Case 1: happy path — descendant omission predicate.
# scope 2 is current (in_progress); prereq 1 done; descendants 3 (not_started
# -> OMITTED) and 4 (in_progress -> APPLICABLE).
d="$WORK/happy"
write_state "$d" '{
  "version": 3,
  "status": "in_progress",
  "certification": {
    "status": "in_progress",
    "scopeProgress": [
      {"scope": 1, "status": "done", "dependsOn": [], "scopeDir": "scopes/01-foundation"},
      {"scope": 2, "status": "in_progress", "dependsOn": ["1"], "scopeDir": "scopes/02-current"},
      {"scope": 3, "status": "not_started", "dependsOn": ["2"], "scopeDir": "scopes/03-later"},
      {"scope": 4, "status": "in_progress", "dependsOn": ["2"], "scopeDir": "scopes/04-parallel"}
    ]
  },
  "execution": {"currentScope": 2, "currentPhase": "implement"}
}'
out="$(run_resolver "$d")"; rc=$?
if [[ "$rc" -eq 0 ]]; then pass "happy path resolves (exit 0)"; else fail "happy path exit $rc"; fi
printf '%s\n' "$out" | awk -F'\t' '$1=="RECORD"&&$2=="2"&&$4=="true"{f=1}END{exit !f}' && pass "scope 2 marked isCurrent" || fail "scope 2 not marked current"
printf '%s\n' "$out" | awk -F'\t' '$1=="RECORD"&&$2=="3"&&$5=="true"&&$6=="false"&&$7=="scopes/03-later"{f=1}END{exit !f}' && pass "not_started descendant (3) omitted (applicable=false), scopeDir emitted" || fail "scope 3 omission/scopeDir wrong"
printf '%s\n' "$out" | awk -F'\t' '$1=="RECORD"&&$2=="4"&&$5=="true"&&$6=="true"&&$7=="scopes/04-parallel"{f=1}END{exit !f}' && pass "in_progress descendant (4) stays applicable" || fail "scope 4 applicability wrong"
printf '%s\n' "$out" | awk -F'\t' '$1=="RECORD"&&$2=="1"&&$5=="false"&&$6=="true"&&$7=="scopes/01-foundation"{f=1}END{exit !f}' && pass "prerequisite (1) stays applicable" || fail "scope 1 applicability wrong"
expected_aliases='["01","01-foundation","1","scopes/01-foundation","scopes/01-foundation/scope.md"]'
printf '%s\n' "$out" | awk -F'\t' -v expected="$expected_aliases" '$1=="RECORD"&&$2=="1"&&NF==8&&$8==expected{f=1}END{exit !f}' && pass "eighth RECORD field is compact deterministic sorted alias JSON" || fail "alias JSON protocol/order wrong"

# Case 1b: a symbolic canonical scopeId resolves independently of the physical
# directory basename. Resolver-owned aliases include canonical identity, legacy
# numeric aliases, scopeDir path/basename, and the scope-file form only.
d="$WORK/symbolic"
write_state "$d" '{
  "version": 3,
  "status": "in_progress",
  "certification": {
    "status": "in_progress",
    "scopeProgress": [
      {"scopeId":"payments-core","scope":2,"status":"in_progress","dependsOn":[],"scopeDir":"scopes/17-payments"}
    ]
  },
  "execution": {"currentScope":"payments-core","currentPhase":"implement"}
}'
symbolic_out="$(run_resolver "$d")"; symbolic_rc=$?
[[ "$symbolic_rc" -eq 0 ]] && pass "symbolic canonical scopeId resolves" || fail "symbolic canonical scopeId exit $symbolic_rc"
symbolic_aliases='["02","17-payments","2","payments-core","scopes/17-payments","scopes/17-payments/scope.md"]'
printf '%s\n' "$symbolic_out" | awk -F'\t' -v expected="$symbolic_aliases" '$1=="RECORD"&&$2=="payments-core"&&$7=="scopes/17-payments"&&$8==expected{f=1}END{exit !f}' && pass "symbolic record emits only resolver-owned aliases in sorted order" || fail "symbolic alias projection wrong"
symbolic_repeat_out="$(run_resolver "$d")"; symbolic_repeat_rc=$?
[[ "$symbolic_repeat_rc" -eq 0 && "$symbolic_repeat_out" == "$symbolic_out" ]] && pass "alias projection is deterministic across identical resolver runs" || fail "alias projection changed across identical resolver runs"

# Mutation twin: changing only scopeDir must update only the directory-derived
# aliases, while retaining compact lexical ordering and every identity alias.
d="$WORK/symbolic-mutated"
write_state "$d" '{
  "version": 3,
  "status": "in_progress",
  "certification": {
    "status": "in_progress",
    "scopeProgress": [
      {"scopeId":"payments-core","scope":2,"status":"in_progress","dependsOn":[],"scopeDir":"scopes/09-checkout"}
    ]
  },
  "execution": {"currentScope":"payments-core","currentPhase":"implement"}
}'
symbolic_mutated_out="$(run_resolver "$d")"; symbolic_mutated_rc=$?
symbolic_mutated_aliases='["02","09-checkout","2","payments-core","scopes/09-checkout","scopes/09-checkout/scope.md"]'
printf '%s\n' "$symbolic_mutated_out" | awk -F'\t' -v expected="$symbolic_mutated_aliases" '$1=="RECORD"&&$2=="payments-core"&&$7=="scopes/09-checkout"&&$8==expected{f=1}END{exit !f}' && pass "scopeDir mutation changes directory aliases and preserves sorted protocol order" || fail "scopeDir mutation alias projection wrong"
[[ "$symbolic_mutated_rc" -eq 0 && "$symbolic_mutated_out" != "$symbolic_out" ]] && pass "alias projection is mutation-sensitive" || fail "alias projection ignored scopeDir mutation"

# helper: assert a fixture refuses with exit 2
assert_refuse() {
  local label="$1" dir="$2"
  run_resolver "$dir" >/dev/null 2>&1
  [[ "$?" -eq 2 ]] && pass "refuses: $label" || fail "did NOT refuse (exit!=2): $label"
}

# Case 2: version not 3
d="$WORK/badver"; write_state "$d" '{"version": 2, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "version != 3" "$d"

# Case 3: malformed JSON
d="$WORK/malformed"; write_state "$d" '{"version": 3, not json'
assert_refuse "malformed JSON" "$d"

# Case 4: duplicate keys
d="$WORK/dupkey"; write_state "$d" '{"version": 3, "version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "duplicate object key" "$d"

# Case 5: current status not in_progress/blocked
d="$WORK/curdone"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"done","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "current scope status done (must be in_progress/blocked)" "$d"

# Case 6: dependency cycle
d="$WORK/cycle"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":["2"]},{"scope":2,"status":"in_progress","dependsOn":["1"]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "dependency cycle" "$d"

# Case 7: unknown dependency edge
d="$WORK/unknown"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":["99"]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "unknown dependency edge" "$d"

# Case 8: transitive prerequisite of current not done
d="$WORK/prereq"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"not_started","dependsOn":[]},{"scope":2,"status":"in_progress","dependsOn":["1"]}]}, "execution": {"currentScope": 2, "currentPhase": "implement"}}'
assert_refuse "current prerequisite not done" "$d"

# Case 9: terminal currentPhase
d="$WORK/phase"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "validate"}}'
assert_refuse "terminal currentPhase (validate)" "$d"

# Case 10: top-level and certification status disagree
d="$WORK/statusdisagree"; write_state "$d" '{"version": 3, "status": "blocked", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "implement"}}'
assert_refuse "packet/certification status disagree" "$d"

# Case 11: execution overlay disagrees with certification
d="$WORK/overlay"; write_state "$d" '{"version": 3, "status": "in_progress", "certification": {"status": "in_progress", "scopeProgress": [{"scope":1,"status":"in_progress","dependsOn":[]}]}, "execution": {"currentScope": 1, "currentPhase": "implement", "scopeProgress": [{"scope":1,"status":"done","dependsOn":[]}]}}'
assert_refuse "execution overlay status disagrees with certification" "$d"

# Case 12: bad usage (wrong context token)
python3 "$RESOLVER" "$WORK/happy" all-scopes >/dev/null 2>&1
[[ "$?" -eq 2 ]] && pass "refuses non-current-scope context token" || fail "did not refuse bad context token"

# Case 13: aliases colliding across otherwise-distinct canonical records make
# resolution ambiguous rather than selecting by record order.
d="$WORK/ambiguous"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scopeId":"alpha","scope":"shared","status":"in_progress","dependsOn":[],"scopeDir":"scopes/01-alpha"},{"scopeId":"beta","scope":"shared","status":"not_started","dependsOn":[],"scopeDir":"scopes/02-beta"}]},"execution":{"currentScope":"shared","currentPhase":"implement"}}'
assert_refuse "ambiguous legacy scope alias" "$d"

# Cases 14-16: currentScope is never normalized from bool, zero, or negative.
d="$WORK/current-bool"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scope":1,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":true,"currentPhase":"implement"}}'
assert_refuse "boolean currentScope" "$d"
d="$WORK/current-zero"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scope":1,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":0,"currentPhase":"implement"}}'
assert_refuse "zero currentScope" "$d"
d="$WORK/current-negative"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scope":1,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":-1,"currentPhase":"implement"}}'
assert_refuse "negative currentScope" "$d"

# Case 17: JSON has one numeric type. A positive integral number written with a
# decimal point is a legacy numeric alias and normalizes to the canonical string.
d="$WORK/integral-number"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scopeId":"numeric-scope","scope":2.0,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":2.0,"currentPhase":"implement"}}'
integral_out="$(run_resolver "$d")"; integral_rc=$?
[[ "$integral_rc" -eq 0 ]] && pass "positive integral JSON-number aliases normalize to strings" || fail "integral JSON-number alias exit $integral_rc"
printf '%s\n' "$integral_out" | awk -F'\t' '$1=="RECORD"&&$2=="numeric-scope"&&$4=="true"&&$8=="[\"02\",\"2\",\"numeric-scope\"]"{f=1}END{exit !f}' && pass "integral number emits normalized numeric aliases" || fail "integral number alias projection wrong"

# Case 18: ambiguity is a registry defect even when currentScope uses an
# unrelated canonical ID. Refuse deterministically instead of leaving a token
# that would resolve according to whichever record a later consumer selects.
d="$WORK/latent-ambiguity"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scopeId":"alpha","scope":"shared","status":"not_started","dependsOn":[]},{"scopeId":"beta","scope":"shared","status":"not_started","dependsOn":[]},{"scopeId":"current","scope":3,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":"current","currentPhase":"implement"}}'
assert_refuse "latent ambiguous legacy alias" "$d"

# Case 19: fractional numeric values remain invalid identities and aliases.
d="$WORK/fractional-number"; write_state "$d" '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":[{"scope":2.5,"status":"in_progress","dependsOn":[]}]},"execution":{"currentScope":2.5,"currentPhase":"implement"}}'
assert_refuse "fractional numeric scope identity" "$d"

# Case 20: a dependency chain deeper than Python's default recursion limit must
# resolve without RecursionError. Records are deliberately ordered from the
# current scope toward the terminal prerequisite so cycle detection must walk
# the entire chain before it can mark any node complete.
d="$WORK/deep-graph"; mkdir -p "$d"
{
  printf '{"version":3,"status":"in_progress","certification":{"status":"in_progress","scopeProgress":['
  i=1
  while [[ "$i" -le 1200 ]]; do
    [[ "$i" -gt 1 ]] && printf ','
    if [[ "$i" -eq 1 ]]; then status='in_progress'; else status='done'; fi
    if [[ "$i" -lt 1200 ]]; then depends="\"$((i + 1))\""; else depends=''; fi
    printf '{"scope":%s,"status":"%s","dependsOn":[%s]}' "$i" "$status" "$depends"
    i=$((i + 1))
  done
  printf ']},"execution":{"currentScope":1,"currentPhase":"implement"}}\n'
} >"$d/state.json"
run_resolver "$d" >/dev/null 2>&1; deep_rc=$?
[[ "$deep_rc" -eq 0 ]] && pass "dependency graph deeper than recursion limit resolves iteratively" || fail "deep dependency graph crashed/refused with exit $deep_rc"

# Case 21: canonical scopeId follows the resolver's documented alias
# normalization rule. Edge whitespace is identity padding, so the canonical
# projection, currentScope lookup, and dependency lookup all use the trimmed
# token while numeric and symbolic legacy aliases remain available.
d="$WORK/trimmed-canonical"
write_state "$d" '{
  "version":3,
  "status":"in_progress",
  "certification":{"status":"in_progress","scopeProgress":[
    {"scopeId":" foundation ","scope":1,"status":"done","dependsOn":[]},
    {"scopeId":" current ","scope":"legacy-current","status":"in_progress","dependsOn":[" foundation "]}
  ]},
  "execution":{"currentScope":" current ","currentPhase":"implement"}
}'
trimmed_out="$(run_resolver "$d")"; trimmed_rc=$?
[[ "$trimmed_rc" -eq 0 ]] && pass "edge-whitespace canonical scopeId resolves through its trimmed token" || fail "trimmed canonical scopeId exit $trimmed_rc"
printf '%s\n' "$trimmed_out" | awk -F'\t' '$1=="RECORD"&&$2=="foundation"&&$8=="[\"01\",\"1\",\"foundation\"]"{a=1}$1=="RECORD"&&$2=="current"&&$4=="true"&&$8=="[\"current\",\"legacy-current\"]"{b=1}END{exit !(a&&b)}' && pass "canonical output and aliases trim scopeId edge whitespace consistently" || fail "canonical scopeId whitespace leaked into projection or aliases"

printf 'ASSERTIONS=%s PASSED=%s FAILED=%s\n' "$((PASS + FAIL))" "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1
exit 0
