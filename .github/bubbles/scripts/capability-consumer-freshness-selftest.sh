#!/usr/bin/env bash
set -euo pipefail

# capability-consumer-freshness-selftest.sh
#
# Hermetic selftest for Gate G127 (capability_consumer_freshness_gate). Builds
# throwaway fixture repos under $HOME/.bubbles-* (yq cannot read /tmp on some
# hosts) and drives capability-consumer-freshness.sh through every branch:
#
#   * shipped + real consumers            → PASS (exit 0)
#   * shipped + NO consumers              → FAIL (exit 1, ORPHAN)
#   * shipped + dangling consumer path    → FAIL (exit 1, DANGLING)
#   * shipped + one-good-one-dangling     → FAIL (exit 1, DANGLING) [adversarial]
#   * proposed + no consumers             → no-op PASS (exit 0)
#   * partial + no consumers              → no-op PASS (exit 0)
#   * deprecated + no consumers           → no-op PASS (exit 0)
#   * no ledger present                   → no-op PASS (exit 0)
#   * ledger present + yq stripped        → FAIL CLOSED (exit 1)
#   * bypass flags (--skip/--force/--ignore) → usage error (exit 2)
#   * --help                              → exit 0
#
# Per IMP-004 the selftest is HERMETIC: it never reads the live repo ledger
# (that is covered separately by the live guard wired into framework-validate).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/capability-consumer-freshness.sh"

WORK="$HOME/.bubbles-capability-consumer-freshness-selftest"
rm -rf "$WORK"
mkdir -p "$WORK"
trap 'rm -rf "$WORK"' EXIT

passed=0
failed=0

pass() {
  echo "[selftest] PASS: $1"
  passed=$((passed + 1))
}
fail() {
  echo "[selftest] FAIL: $1"
  failed=$((failed + 1))
}

# Assert the guard exits with an expected code for a given repo root.
assert_exit() {
  local label="$1" expected="$2"
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    pass "$label (exit $actual)"
  else
    fail "$label (expected $expected, got $actual)"
  fi
}

# Assert the guard's stderr contains a substring (and the exit code matches).
assert_stderr_contains() {
  local label="$1" expected_code="$2" needle="$3"
  shift 3
  local out actual=0
  out="$("$@" 2>&1)" || actual=$?
  if [[ "$actual" -eq "$expected_code" ]] && grep -q "$needle" <<<"$out"; then
    pass "$label (exit $actual, contains '$needle')"
  else
    fail "$label (expected exit $expected_code + '$needle'; got exit $actual)"
    echo "       output: $out"
  fi
}

# --- Fixture builders ----------------------------------------------------
make_repo() {
  # $1 = repo dir
  local dir="$1"
  mkdir -p "$dir/bubbles/scripts"
}

# Case 1: shipped + real consumers → PASS
R1="$WORK/r1-good"
make_repo "$R1"
: >"$R1/bubbles/scripts/consumer-a.sh"
: >"$R1/bubbles/scripts/consumer-b.sh"
cat >"$R1/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  good-cap:
    label: Good capability
    state: shipped
    summary: A shipped capability with real, existing consumers.
    ownerSurface: bubbles/scripts/consumer-a.sh
    consumers:
    - bubbles/scripts/consumer-a.sh
    - bubbles/scripts/consumer-b.sh
YAML
assert_exit "shipped + real consumers passes" 0 bash "$GUARD" --repo-root "$R1" --quiet

# Case 2: shipped + NO consumers → FAIL (ORPHAN)
R2="$WORK/r2-orphan"
make_repo "$R2"
cat >"$R2/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  orphan-cap:
    label: Orphan capability
    state: shipped
    summary: A shipped capability that declares no consumers at all.
    ownerSurface: bubbles/scripts/nobody.sh
YAML
assert_stderr_contains "shipped + no consumers fails (ORPHAN)" 1 "ORPHAN" bash "$GUARD" --repo-root "$R2"
assert_stderr_contains "ORPHAN names the capability" 1 "orphan-cap" bash "$GUARD" --repo-root "$R2"

# Case 3: shipped + dangling consumer path → FAIL (DANGLING)
R3="$WORK/r3-dangling"
make_repo "$R3"
cat >"$R3/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  dangling-cap:
    label: Dangling capability
    state: shipped
    summary: A shipped capability whose consumer path does not exist on disk.
    ownerSurface: bubbles/scripts/owner.sh
    consumers:
    - bubbles/scripts/does-not-exist.sh
YAML
assert_stderr_contains "shipped + dangling consumer fails (DANGLING)" 1 "DANGLING" bash "$GUARD" --repo-root "$R3"
assert_stderr_contains "DANGLING names the missing path" 1 "does-not-exist.sh" bash "$GUARD" --repo-root "$R3"

# Case 4 (adversarial): shipped + one good + one dangling → still FAIL.
# A tautological check that only looked at the FIRST consumer, or only counted
# the list, would PASS this. The per-path existence loop must catch the bad one.
R4="$WORK/r4-mixed"
make_repo "$R4"
: >"$R4/bubbles/scripts/real.sh"
cat >"$R4/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  mixed-cap:
    label: Mixed capability
    state: shipped
    summary: One existing consumer and one dangling consumer must still fail.
    ownerSurface: bubbles/scripts/real.sh
    consumers:
    - bubbles/scripts/real.sh
    - bubbles/scripts/ghost.sh
YAML
assert_stderr_contains "adversarial: one-good-one-dangling still fails" 1 "ghost.sh" bash "$GUARD" --repo-root "$R4"

# Case 4b (adversarial — shape-not-substance hole): shipped + ONLY blank/empty
# consumer entries. The list is non-empty (yq emits 2 elements), so a naive
# array-size orphan check would PASS it with zero real consumers. The guard
# MUST count NON-EMPTY consumers and flag this as an ORPHAN (exit 1).
R4B="$WORK/r4b-allblank"
make_repo "$R4B"
cat >"$R4B/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  all-blank-cap:
    label: All-blank consumers
    state: shipped
    summary: A shipped cap whose consumers are all empty strings has no real consumer.
    consumers:
    - ""
    - ""
YAML
assert_stderr_contains "adversarial: all-blank consumers is an ORPHAN" 1 "ORPHAN" bash "$GUARD" --repo-root "$R4B"
assert_stderr_contains "all-blank ORPHAN names the capability" 1 "all-blank-cap" bash "$GUARD" --repo-root "$R4B"

# Case 4c (adversarial): shipped + one REAL consumer + one blank entry. The real
# consumer means it is NOT an orphan, but the blank entry is MALFORMED and MUST
# fail loud (exit 1) rather than being silently skipped — a stray blank line can
# never dilute the consumer list undetected.
R4C="$WORK/r4c-realplusblank"
make_repo "$R4C"
: >"$R4C/bubbles/scripts/real.sh"
cat >"$R4C/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  real-plus-blank-cap:
    label: Real plus blank consumer
    state: shipped
    summary: A real consumer plus a stray blank entry must still fail loud on the blank.
    ownerSurface: bubbles/scripts/real.sh
    consumers:
    - bubbles/scripts/real.sh
    - ""
YAML
assert_stderr_contains "adversarial: real+blank consumer fails MALFORMED" 1 "MALFORMED" bash "$GUARD" --repo-root "$R4C"

# Case 5: proposed + no consumers → no-op PASS
R5="$WORK/r5-proposed"
make_repo "$R5"
cat >"$R5/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  proposed-cap:
    label: Proposed capability
    state: proposed
    summary: A proposed capability has no consumers yet and must be a no-op.
YAML
assert_exit "proposed + no consumers is a no-op" 0 bash "$GUARD" --repo-root "$R5" --quiet

# Case 6: partial + no consumers → no-op PASS
R6="$WORK/r6-partial"
make_repo "$R6"
cat >"$R6/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  partial-cap:
    label: Partial capability
    state: partial
    summary: A partial capability is still maturing and must be a no-op.
YAML
assert_exit "partial + no consumers is a no-op" 0 bash "$GUARD" --repo-root "$R6" --quiet

# Case 7: deprecated + no consumers → no-op PASS
R7="$WORK/r7-deprecated"
make_repo "$R7"
cat >"$R7/bubbles/capability-ledger.yaml" <<'YAML'
version: 1
capabilities:
  deprecated-cap:
    label: Deprecated capability
    state: deprecated
    summary: A deprecated capability on the way out must be a no-op.
YAML
assert_exit "deprecated + no consumers is a no-op" 0 bash "$GUARD" --repo-root "$R7" --quiet

# Case 8: no ledger present → no-op PASS (downstream product repo)
R8="$WORK/r8-noledger"
mkdir -p "$R8/bubbles/scripts"
assert_exit "no ledger present is a no-op" 0 bash "$GUARD" --repo-root "$R8" --quiet

# Case 9: ledger present + yq stripped from PATH → FAIL CLOSED (blocking gate).
# Invoke bash by ABSOLUTE path so bash itself resolves even with a stripped
# PATH; only the guard's internal `command -v yq` lookup sees the empty PATH.
EMPTY_BIN="$WORK/empty-bin"
mkdir -p "$EMPTY_BIN"
BASH_BIN="$(command -v bash)"
no_yq_actual=0
PATH="$EMPTY_BIN" "$BASH_BIN" "$GUARD" --repo-root "$R1" >/dev/null 2>&1 || no_yq_actual=$?
if [[ "$no_yq_actual" -eq 1 ]]; then
  pass "ledger present + missing yq fails closed (exit 1)"
else
  fail "ledger present + missing yq fails closed (expected 1, got $no_yq_actual)"
fi
# And it must STILL no-op when yq is missing but no ledger exists (pre-check first).
no_yq_noledger=0
PATH="$EMPTY_BIN" "$BASH_BIN" "$GUARD" --repo-root "$R8" >/dev/null 2>&1 || no_yq_noledger=$?
if [[ "$no_yq_noledger" -eq 0 ]]; then
  pass "no ledger + missing yq still no-ops (pre-check before parser gate)"
else
  fail "no ledger + missing yq still no-ops (expected 0, got $no_yq_noledger)"
fi

# Case 10: bypass flags are rejected as usage errors (exit 2) — no silencing.
assert_exit "bypass flag --skip rejected" 2 bash "$GUARD" --skip --repo-root "$R1"
assert_exit "bypass flag --force rejected" 2 bash "$GUARD" --force --repo-root "$R1"
assert_exit "bypass flag --ignore rejected" 2 bash "$GUARD" --ignore --repo-root "$R1"

# Case 11: --help exits 0
assert_exit "--help exits 0" 0 bash "$GUARD" --help

echo
echo "capability-consumer-freshness-selftest: $passed passed, $failed failed"
if [[ "$failed" -gt 0 ]]; then
  echo "capability-consumer-freshness selftest FAILED."
  exit 1
fi
echo "capability-consumer-freshness selftest passed."
exit 0
