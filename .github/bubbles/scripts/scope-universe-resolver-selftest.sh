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

printf 'ASSERTIONS=%s PASSED=%s FAILED=%s\n' "$((PASS + FAIL))" "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1
exit 0
