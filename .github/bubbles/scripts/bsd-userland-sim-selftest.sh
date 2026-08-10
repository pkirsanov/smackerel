#!/usr/bin/env bash
# bsd-userland-sim-selftest.sh
#
# Hermetic selftest for bsd-userland-sim.sh. Builds throwaway simulations under a
# temp dir and asserts the simulator's contract:
#
#   - LIVENESS      the shim directory actually intercepts a shimmed tool, so a
#                   silently-empty shim can never green the whole suite.
#   - REJECTION     every GNU-only spelling in the divergence table FAILS. Only
#                   failure is asserted, never a per-tool status — the simulator
#                   header is explicit that it does not claim to know each BSD
#                   tool's exit number. grep and sort are the two exceptions and
#                   are asserted at exit 2 exactly, because grep reserves 1 for
#                   "no match" and a rejection exiting 1 would be read as a
#                   normal empty result and silently swallowed.
#   - TRANSLATION   the accepted BSD spelling produces the OBSERVABLE EFFECT, not
#                   merely exit 0. This is the load-bearing group. A shim that
#                   only rejected GNU spellings would break the CORRECT BSD
#                   branch too and produce false attributions, which is worse
#                   than having no simulator at all.
#   - DEVIATION     a date format carrying the nanosecond specifier is NOT
#                   rejected; the specifier letter is emitted literally, which is
#                   what BSD date does and what guard-lib.sh numerically guards
#                   against. Rejecting instead would model a failure macOS does
#                   not have and hide the one it does.
#   - MODE SPLIT    --prepend CANNOT hide a binary (the shell keeps searching
#                   later PATH elements); --sealed CAN. That asymmetry is the
#                   whole reason both modes exist, so a future edit collapsing
#                   them must fail here.
#   - CLEANUP       --cleanup removes a real simulation root and REFUSES any path
#                   without this simulator's marker, so a mistyped argument
#                   cannot delete an unrelated tree.
#   - USAGE         an unknown flag exits 2.
#   - PORTABILITY   the simulator parses (bash -n) and its own source passes the
#                   framework's macOS portability guard.
#
# Writes nothing outside its own mktemp workspace and cleans up via trap. TMPDIR
# is re-pointed at that workspace on purpose: it confines BOTH the simulator's
# default root and the `mktemp -t` translation output to the workspace, so a run
# leaves no /tmp/bubbles-bsd-sim.* behind.
#
# Exit 0 = all assertions pass; exit 1 = one or more failed. SKIPs (exit 0) only
# if a genuinely-required POSIX tool is absent, or if the host userland is not
# GNU, per the framework convention.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SELF="${BASH_SOURCE[0]}"
TARGET="$SCRIPT_DIR/bsd-userland-sim.sh"
GUARD="$SCRIPT_DIR/macos-portability-guard.sh"
MARKER_NAME=".bubbles-bsd-userland-sim"

if [[ ! -f "$TARGET" ]]; then
  echo "[selftest bsd-userland-sim] FAIL: target script missing at $TARGET" >&2
  exit 1
fi

# Graceful degradation: the simulator shims real coreutils. If one the assertions
# below actually exercise is genuinely absent, SKIP (exit 0) rather than reporting
# a failure that belongs to the environment.
for _dep in sed date stat grep sort mktemp ln; do
  if ! command -v "$_dep" >/dev/null 2>&1; then
    echo "[selftest bsd-userland-sim] SKIP ($_dep not installed)"
    exit 0
  fi
done

# Capability probe, never uname: GNU sed/date accept --version, the BSD ones reject it.
for _dep in sed date; do
  if ! "$_dep" --version >/dev/null 2>&1; then
    echo "[selftest bsd-userland-sim] SKIP (non-GNU userland: $_dep rejects --version; the simulator rewrites an accepted BSD spelling into the GNU form before running the real binary, so it requires GNU coreutils underneath and is a GNU/Linux-host tool)"
    exit 0
  fi
done

# The GNU reference mtime is read through the framework's portable helper rather
# than a raw GNU spelling, so this file stays clean under the same guard it
# asserts against below.
# shellcheck source=guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"

TMPDIR="$(mktemp -d)"
export TMPDIR
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() {
  echo "  FAIL: $1"
  failures=$((failures + 1))
}

RUN_OUT=""
RUN_RC=0

# run_as <path-value> <cmd...> — run a command under a simulated PATH, capturing
# combined output in RUN_OUT and the REAL exit status in RUN_RC. TZ is pinned so
# the date assertions are reproducible in any timezone.
run_as() {
  local path_value="$1"
  shift
  set +e
  RUN_OUT="$(PATH="$path_value" TZ=UTC "$@" 2>&1)"
  RUN_RC=$?
  set -e
}

# assert_rejected <name> <path-value> <cmd...> — the command must FAIL. The
# status is reported but not constrained: see the REJECTION note in the header.
assert_rejected() {
  local name="$1" path_value="$2"
  shift 2
  run_as "$path_value" "$@"
  if [[ "$RUN_RC" -ne 0 ]]; then
    pass "rejects $name (exit $RUN_RC)"
  else
    fail "rejects $name -> expected a non-zero exit, got 0"
  fi
}

# assert_rejected_rc <name> <expected-rc> <path-value> <cmd...> — for the two
# tools whose rejection status the simulator DOES commit to.
assert_rejected_rc() {
  local name="$1" want="$2" path_value="$3"
  shift 3
  run_as "$path_value" "$@"
  if [[ "$RUN_RC" -eq "$want" ]]; then
    pass "rejects $name at exit $want"
  else
    fail "rejects $name -> expected exit $want, got $RUN_RC"
  fi
}

assert_eq() {
  local name="$1" want="$2" got="$3"
  if [[ "$got" == "$want" ]]; then
    pass "$name"
  else
    fail "$name -> expected '$want', got '$got'"
  fi
}

assert_ne() {
  local name="$1" unwanted="$2" got="$3"
  if [[ "$got" != "$unwanted" ]]; then
    pass "$name"
  else
    fail "$name -> value should have differed from '$unwanted'"
  fi
}

assert_matches() {
  local name="$1" pattern="$2" got="$3"
  if [[ "$got" =~ $pattern ]]; then
    pass "$name"
  else
    fail "$name -> '$got' does not match /$pattern/"
  fi
}

assert_prefix() {
  local name="$1" prefix="$2" got="$3"
  if [[ "$got" == "$prefix"* ]]; then
    pass "$name"
  else
    fail "$name -> '$got' is not under '$prefix'"
  fi
}

assert_file() {
  local name="$1" path="$2"
  if [[ -f "$path" ]]; then
    pass "$name"
  else
    fail "$name -> no file at $path"
  fi
}

assert_dir() {
  local name="$1" path="$2"
  if [[ -d "$path" ]]; then
    pass "$name"
  else
    fail "$name -> no directory at $path"
  fi
}

assert_absent() {
  local name="$1" path="$2"
  if [[ ! -e "$path" ]]; then
    pass "$name"
  else
    fail "$name -> $path still exists"
  fi
}

# rc_of <cmd...> — exit status of a direct simulator invocation.
rc_of() {
  set +e
  "$@" >/dev/null 2>&1
  local rc=$?
  set -e
  echo "$rc"
}

echo "== bsd-userland-sim selftest =="

# --- Build the two simulations -------------------------------------------
SHIM_DIR="$(bash "$TARGET" --prepend --root "$TMPDIR/sim-prepend")"
SHIM_PATH="$SHIM_DIR:$PATH"
SEALED_PATH="$(bash "$TARGET" --sealed --root "$TMPDIR/sim-sealed")"

# --- Fixture liveness -----------------------------------------------------
# Without this, an empty shim directory would let every "accepted" assertion
# below pass by simply falling through to the real GNU tool.
run_as "$SHIM_PATH" bash -c 'command -v sed'
assert_eq "shim directory actually intercepts sed" "$SHIM_DIR/sed" "$RUN_OUT"

fixture="$TMPDIR/fixture.txt"
printf 'alpha\n' >"$fixture"

# --- Rejection: GNU-only spellings ---------------------------------------
#
# The command spellings below are FIXTURES: literal argv this script hands to the
# simulator to observe how it reacts. They are never forms this script depends on
# for its own portability, and the portable remedy each guard class recommends
# would defeat the assertion — routing through bubbles_sed_inplace would test the
# helper instead of the shim. Every line the guard flags therefore carries an
# inline pragma naming which kind of fixture it is.
echo "-- rejection --"

assert_rejected "sed bare in-place flag" "$SHIM_PATH" sed -i "s/alpha/beta/" "$fixture" # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "date GNU parse flag" "$SHIM_PATH" date -d 2020-01-01 # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "date GNU long parse flag" "$SHIM_PATH" date --date=2020-01-01
assert_rejected "stat GNU format flag" "$SHIM_PATH" stat -c %Y "$fixture" # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "stat GNU long format flag" "$SHIM_PATH" stat --format=%Y "$fixture"
assert_rejected "readlink canonicalize flag" "$SHIM_PATH" readlink -f /etc # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "readlink long canonicalize flag" "$SHIM_PATH" readlink --canonicalize /etc
assert_rejected "find GNU printf primary" "$SHIM_PATH" find "$TMPDIR" -printf '%p\n'
assert_rejected "xargs empty-input guard flag" "$SHIM_PATH" bash -c 'xargs -r echo </dev/null'
assert_rejected "base64 wrap flag" "$SHIM_PATH" base64 -w 0 "$fixture"
assert_rejected "mktemp parent-directory flag" "$SHIM_PATH" mktemp -p "$TMPDIR" # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "mktemp suffix flag" "$SHIM_PATH" mktemp --suffix=.txt # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected "head all-but-last line count" "$SHIM_PATH" head -n -1 "$fixture"
assert_rejected "cp path-preserving flag" "$SHIM_PATH" cp --parents "$fixture" "$TMPDIR"
assert_rejected "du bytes flag" "$SHIM_PATH" du -b "$TMPDIR"

# grep and sort are asserted at an EXACT status. grep reserves exit 1 for "no
# match", so a rejection exiting 1 would be indistinguishable from a clean empty
# result and would be swallowed by the caller. This distinction is load-bearing.
assert_rejected_rc "grep PCRE flag" 2 "$SHIM_PATH" grep -P alpha "$fixture" # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse
assert_rejected_rc "grep long PCRE flag" 2 "$SHIM_PATH" grep --perl-regexp alpha "$fixture"
assert_rejected_rc "sort version-sort flag" 2 "$SHIM_PATH" sort -V "$fixture"
assert_rejected_rc "sort long version-sort flag" 2 "$SHIM_PATH" sort --version-sort "$fixture"

# --- Translation: the accepted BSD spelling must DO the work --------------
echo "-- translation (observable effect, not mere acceptance) --"

run_as "$SHIM_PATH" sed -i '' "s/alpha/beta/" "$fixture" # portable-ok: translation fixture, the BSD spelling under test
assert_eq "sed with an empty suffix operand exits 0" "0" "$RUN_RC"
assert_eq "sed with an empty suffix operand REWROTE the file" "beta" "$(cat "$fixture")"

run_as "$SHIM_PATH" sed -i.bak "s/beta/gamma/" "$fixture" # portable-ok: translation fixture, the BSD spelling under test
assert_eq "sed with an attached suffix exits 0" "0" "$RUN_RC"
assert_eq "sed with an attached suffix REWROTE the file" "gamma" "$(cat "$fixture")"
assert_file "sed with an attached suffix produced the backup" "$fixture.bak"
assert_eq "the attached-suffix backup holds the prior content" "beta" "$(cat "$fixture.bak")"

# The separated dot-suffix is the real BSD spelling, and exercises the fold into
# the attached form GNU understands.
run_as "$SHIM_PATH" sed -i .sep "s/gamma/delta/" "$fixture" # portable-ok: translation fixture, the BSD spelling under test
assert_eq "sed with a separated suffix operand exits 0" "0" "$RUN_RC"
assert_eq "sed with a separated suffix operand REWROTE the file" "delta" "$(cat "$fixture")"
assert_eq "the separated-suffix backup holds the prior content" "gamma" "$(cat "$fixture.sep")"

gnu_mtime="$(bubbles_file_mtime_epoch "$fixture")"
run_as "$SHIM_PATH" stat -f %m "$fixture"
assert_eq "stat BSD field selector returns the mtime GNU reports" "$gnu_mtime" "$RUN_OUT"

run_as "$SHIM_PATH" date -r 1700000000 +%Y-%m-%d
assert_eq "date BSD epoch flag returns the correct date" "2023-11-14" "$RUN_OUT"

run_as "$SHIM_PATH" date +%Y-%m-%d
today="$RUN_OUT"
run_as "$SHIM_PATH" date -v-1d +%Y-%m-%d
assert_matches "date BSD adjustment flag returns an ISO date" \
  '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' "$RUN_OUT"
assert_ne "date BSD adjustment flag actually SHIFTED the day" "$today" "$RUN_OUT"

run_as "$SHIM_PATH" mktemp -t bsdsimprobe
mktemp_out="$RUN_OUT"
assert_file "mktemp BSD prefix flag produced a real file" "$mktemp_out"
assert_prefix "mktemp BSD prefix flag honored TMPDIR" "$TMPDIR/" "$mktemp_out"

# --- Deliberate deviation: the nanosecond specifier is NOT rejected -------
echo "-- deliberate deviation (nanosecond specifier) --"

run_as "$SHIM_PATH" date +%s%N # portable-ok: deviation fixture, the form the simulator corrupts literally instead of rejecting
assert_eq "a nanosecond-bearing date format is NOT rejected" "0" "$RUN_RC"
assert_matches "the nanosecond specifier letter is emitted LITERALLY" \
  '^[0-9]+N$' "$RUN_OUT"

# --- Mode split: prepend cannot hide, sealed can -------------------------
echo "-- sealed vs prepend --"

# Listing gtimeout beside timeout keeps this a data list rather than a raw
# timeout invocation, matching the simulator's own HIDDEN_TOOLS line.
hidden_names="timeout gtimeout sha256sum"

for hidden in $hidden_names; do
  run_as "$SHIM_PATH" bash -c 'command -v "$1" >/dev/null 2>&1' _ "$hidden"
  prepend_rc="$RUN_RC"
  run_as "$SEALED_PATH" bash -c 'command -v "$1" >/dev/null 2>&1' _ "$hidden"
  sealed_rc="$RUN_RC"

  if command -v "$hidden" >/dev/null 2>&1; then
    assert_eq "--prepend CANNOT hide $hidden" "0" "$prepend_rc"
    assert_ne "--sealed DOES hide $hidden" "0" "$sealed_rc"
  else
    pass "$hidden absent on this host; hiding assertions not applicable"
  fi
done

run_as "$SEALED_PATH" bash -c 'command -v shasum >/dev/null 2>&1'
assert_eq "--sealed still PROVIDES shasum (macOS ships it)" "0" "$RUN_RC"

# Sealed must be a superset: it hides names AND keeps every behavioural shim.
assert_rejected "sed bare in-place flag under --sealed" "$SEALED_PATH" sed -i "s/delta/omega/" "$fixture" # portable-ok: rejection fixture, the exact GNU spelling the shim must refuse

# --- Cleanup contract -----------------------------------------------------
echo "-- cleanup --"

# A default-root build, so the cleanup assertion runs against a root the
# simulator chose. TMPDIR is this workspace, so it cannot land in /tmp.
throwaway_shim="$(bash "$TARGET" --prepend)"
throwaway_root="$(dirname "$throwaway_shim")"
assert_file "a fresh simulation root carries the marker file" \
  "$throwaway_root/$MARKER_NAME"
assert_eq "--cleanup removes a real simulation root" "0" "$(rc_of bash "$TARGET" --cleanup "$throwaway_shim")"
assert_absent "the simulation root is gone after --cleanup" "$throwaway_root"

# The decoy is created HERE and is never a path the simulator built, so the
# refusal is exercised without ever risking an unrelated tree.
decoy="$TMPDIR/decoy"
mkdir -p "$decoy"
printf 'keep\n' >"$decoy/sentinel"
assert_eq "--cleanup REFUSES a path with no marker" "2" "$(rc_of bash "$TARGET" --cleanup "$decoy")"
assert_dir "the refused decoy directory still exists" "$decoy"
assert_file "the refused decoy contents are untouched" "$decoy/sentinel"

# --cleanup also accepts a sealed PATH string, whose first element is the shim
# directory. Run last: it dismantles the sealed simulation.
assert_eq "--cleanup accepts a sealed PATH value" "0" "$(rc_of bash "$TARGET" --cleanup "$SEALED_PATH")"
assert_absent "the sealed simulation root is gone" "$TMPDIR/sim-sealed"

# --- Usage errors ---------------------------------------------------------
echo "-- usage --"

assert_eq "an unknown flag exits 2" "2" "$(rc_of bash "$TARGET" --no-such-flag)"
assert_eq "--help exits 0" "0" "$(rc_of bash "$TARGET" --help)"
assert_eq "--root with no argument exits 2" "2" "$(rc_of bash "$TARGET" --root)"
assert_eq "--cleanup with no argument exits 2" "2" "$(rc_of bash "$TARGET" --cleanup)"

# --- Self-portability -----------------------------------------------------
echo "-- self-portability --"

assert_eq "the simulator parses (bash -n)" "0" "$(rc_of bash -n "$TARGET")"
assert_eq "this selftest parses (bash -n)" "0" "$(rc_of bash -n "$SELF")"

if [[ -f "$GUARD" ]]; then
  assert_eq "the simulator source passes the macOS portability guard" "0" \
    "$(rc_of bash "$GUARD" "$TARGET")"
  assert_eq "this selftest source passes the macOS portability guard" "0" \
    "$(rc_of bash "$GUARD" "$SELF")"
else
  echo "  SKIP: macos-portability-guard.sh not present; self-portability scan not run"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest bsd-userland-sim] OK — all assertions passed."
  exit 0
fi
echo "[selftest bsd-userland-sim] FAIL — $failures assertion(s) failed." >&2
exit 1
