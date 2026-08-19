#!/usr/bin/env bash
# bubbles/scripts/collected-test-count-guard-selftest.sh
#
# Hermetic selftest for collected-test-count-guard.sh (IMP-036 SCOPE-3)
# (Gate G133 — collected_test_count_gate).
#
# Case 1 replays the ACTUAL defect: a downstream e2e suite emitted
# "No tests found" for 15 days while spec commits recorded passing evidence.
# A regression test built only from invented input would not prove the guard
# catches the thing that actually happened.
#
# Case 6 is the counterweight. A guard that fires on ordinary prose gets
# bypassed rather than fixed, so a non-test document mentioning the same phrase
# must NOT fail.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/collected-test-count-guard.sh"
NAME="collected-test-count-guard-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

WORK="$(mktemp -d 2>/dev/null)" || { printf '%s: cannot create temp dir\n' "$NAME" >&2; exit 1; }
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

make_spec() {
  local dir="$1" body="$2"
  mkdir -p "$dir"
  printf '%s\n' "$body" >"$dir/report.md"
}

# --- 1. ADVERSARIAL: the real downstream outage shape ------------------------
s1="$WORK/s1"
make_spec "$s1" '## Test Evidence

Command: ./run-e2e.sh
```
Running 148 spec files
Error: No tests found
148 files failed at collection
```
Exit Code: 1'
out="$("$TARGET" "$s1" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]] && printf '%s' "$out" | grep -q 'ZERO tests ran'; then
  ok "catches the real outage shape (No tests found in e2e evidence)"
else
  bad "catches the real outage shape" "rc=$rc: $out"
fi
# --- 2. jest zero-suite output ------------------------------------------------
s2="$WORK/s2"
make_spec "$s2" '## Test Evidence
```
$ npm test
Test Suites: 0 total
Tests:       0 total
```'
out="$("$TARGET" "$s2" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]]; then ok "catches jest zero-suite output"; else bad "catches jest zero-suite output" "rc=$rc: $out"; fi

# --- 3. pytest collected-nothing ---------------------------------------------
s3="$WORK/s3"
make_spec "$s3" '## Test Evidence
```
$ pytest tests/
collected 0 items
```'
out="$("$TARGET" "$s3" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]]; then ok "catches pytest collected 0 items"; else bad "catches pytest collected 0 items" "rc=$rc: $out"; fi

# --- 4. go zero-test warning --------------------------------------------------
s4="$WORK/s4"
make_spec "$s4" '## Test Evidence
```
$ go test ./...
Ran 0 tests
```'
out="$("$TARGET" "$s4" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]]; then ok "catches an explicit Ran 0 tests"; else bad "catches Ran 0 tests" "rc=$rc: $out"; fi

# --- 4b. ADVERSARIAL: jest Snapshots: 0 total is NORMAL, not a zero-test run --
# This was the single largest false positive in the field: 354 hits in one repo.
# A suite that uses no snapshots reports 0 snapshots and is perfectly healthy.
s4b="$WORK/s4b"
make_spec "$s4b" '## Test Evidence
```
$ npm test
Test Suites: 12 passed, 12 total
Tests:       147 passed, 147 total
Snapshots:   0 total
Time:        4.2 s
```
Exit Code: 0'
out="$("$TARGET" "$s4b" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then
  ok "jest Snapshots: 0 total does not fail a healthy run"
else
  bad "Snapshots: 0 total does not fail" "rc=$rc: $out"
fi

# --- 4c. ADVERSARIAL: Go per-package [no tests to run] is routine ------------
# Go prints this for any package with no test files and marks it ok. Treating it
# as a defect would fire on almost every Go repo and get the guard switched off.
s4c="$WORK/s4c"
make_spec "$s4c" '## Test Evidence
```
$ go test ./...
ok      example.com/app/internal/apikey    0.010s [no tests to run]
ok      example.com/app/internal/booking   0.240s
```
Exit Code: 0'
out="$("$TARGET" "$s4c" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then
  ok "Go per-package [no tests to run] does not fail"
else
  bad "Go [no tests to run] does not fail" "rc=$rc: $out"
fi

# --- 4d. ADVERSARIAL: "linkedTests: 0" must not match ------------------------
# "Tests:" without a leading word boundary matches the tail of "linkedTests:".
s4d="$WORK/s4d"
make_spec "$s4d" '## Test Evidence
```
All linkedTests files exist. Total links: 19. Scenarios with empty linkedTests: 0
```'
out="$("$TARGET" "$s4d" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then ok "linkedTests: 0 does not match Tests: 0"; else bad "linkedTests: 0 does not match" "rc=$rc: $out"; fi

# --- 4e. ADVERSARIAL: "Tests: 0 failed" is a PASS, not a zero-test run -------
s4e="$WORK/s4e"
make_spec "$s4e" '## Test Evidence
```
Tests: 0 failed, all passed
```'
out="$("$TARGET" "$s4e" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then ok "Tests: 0 failed is read as a pass, not a zero run"; else bad "Tests: 0 failed is a pass" "rc=$rc: $out"; fi

# --- 5. healthy evidence passes ----------------------------------------------
s5="$WORK/s5"
make_spec "$s5" '## Test Evidence
```
$ npm test
Test Suites: 12 passed, 12 total
Tests:       147 passed, 147 total
```
Exit Code: 0'
out="$("$TARGET" "$s5" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then ok "healthy evidence passes"; else bad "healthy evidence passes" "rc=$rc: $out"; fi

# --- 6. ADVERSARIAL: prose outside a test context must NOT fail --------------
# A guard that fires on ordinary sentences gets bypassed rather than fixed.
s6="$WORK/s6"
make_spec "$s6" '## Summary

The migration touched configuration only. No tests found in this area were
affected, and the deployment notes below describe the rollout window for the
regional configuration change to the billing address formatter used by the
downstream invoicing surface.'
out="$("$TARGET" "$s6" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then
  ok "prose outside a test context does not fail"
else
  bad "prose outside a test context does not fail" "rc=$rc: $out"
fi

# --- 7. evidence with no recognisable count is reported, not failed ----------
s7="$WORK/s7"
make_spec "$s7" '## Test Evidence
```
$ ./run-suite --format=custom
suite complete
```'
out="$("$TARGET" "$s7" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then
  ok "unrecognised runner format does not hard-fail"
else
  bad "unrecognised runner format does not hard-fail" "rc=$rc: $out"
fi

# --- 8. ADVERSARIAL: bypass-shaped flags are refused -------------------------
bypass_ok=1
for flag in --skip --force --ignore --no-verify --bypass --allow-once; do
  "$TARGET" "$s1" "$flag" >/dev/null 2>&1; rc=$?
  if [[ $rc -ne 2 ]]; then bypass_ok=0; bad "bypass flag '$flag' refused" "rc=$rc"; break; fi
done
[[ $bypass_ok -eq 1 ]] && ok "every bypass-shaped flag exits 2"

# --- 9. no target is a usage error -------------------------------------------
"$TARGET" >/dev/null 2>&1; rc=$?
if [[ $rc -eq 2 ]]; then ok "missing target exits 2 (no default surface)"; else bad "missing target exits 2" "rc=$rc"; fi

# --- 10. scans scope.md and scopes.md too ------------------------------------
s10="$WORK/s10"
mkdir -p "$s10"
# shellcheck disable=SC2016  # literal markdown checkbox, expansion is not wanted
printf '## DoD\n- [x] e2e pass\n```\nplaywright: No tests found\n```\n' >"$s10/scopes.md"
out="$("$TARGET" "$s10" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]]; then ok "scans scopes.md, not just report.md"; else bad "scans scopes.md" "rc=$rc: $out"; fi

# --- 11. ADVERSARIAL: a nonzero count ENDING in 0 must not match -------------
# "0 passed" is a substring of "10 passed" and "0 total" of "147 total". Without
# non-digit anchoring this guard reported 2,133 false hits across six repos,
# which is the failure mode that makes a gate get ignored.
s11="$WORK/s11"
make_spec "$s11" '## Test Evidence
```
$ npm test
Test Suites: 10 passed, 10 total
Tests:       147 passed, 147 total
Specs:       20 passed, 20 total
Ran 130 tests in 4.2s
```
Exit Code: 0'
out="$("$TARGET" "$s11" 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then
  ok "counts ending in 0 (10 passed, 147 total, 20 passed) do not match"
else
  bad "counts ending in 0 do not match" "rc=$rc: $out"
fi

# --- 12. ...while a genuine zero in the same shape still fails ---------------
s12="$WORK/s12"
make_spec "$s12" '## Test Evidence
```
$ npm test
Test Suites: 0 passed, 0 total
Tests:       0 passed, 0 total
```'
out="$("$TARGET" "$s12" 2>&1)"; rc=$?
if [[ $rc -eq 1 ]]; then
  ok "a genuine zero in the same output shape still fails"
else
  bad "genuine zero still fails" "rc=$rc: $out"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
