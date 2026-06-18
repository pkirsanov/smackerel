#!/usr/bin/env bash
# File: regen-derived-selftest.sh
#
# Hermetic selftest for regen-derived.sh. Builds a fake source tree with stub
# generators that record their invocation order and respond to --check, then
# proves: (1) a clean tree regenerates in the correct DEPENDENCY ORDER and exits
# 0; (2) a generator still stale after regeneration makes the wrapper exit
# non-zero (fail loud); (3) --check-only verifies WITHOUT regenerating.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGEN="$SCRIPT_DIR/regen-derived.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# Fake source tree: <work>/bubbles/scripts/ (NOT a .github downstream layout, so
# regen-derived's source-repo guard passes) holding a copy of the real wrapper +
# POSIX stub generators.
mkdir -p "$work/bubbles/scripts"
cp "$REGEN" "$work/bubbles/scripts/regen-derived.sh"
order_log="$work/order.log"

make_stub() {
  local name="$1" stale_marker="$2"
  # POSIX sh stub: bare invocation records the name (write mode); --check reports
  # fresh unless its stale-marker file exists. POSIX so `sh stub` AND `bash stub`
  # both work (framework-stats is invoked via sh).
  cat >"$work/bubbles/scripts/$name" <<EOF
#!/bin/sh
if [ "\${1:-}" = "--check" ]; then
  if [ -f "$work/$stale_marker" ]; then echo "STALE: $name" >&2; exit 1; fi
  exit 0
fi
echo "$name" >> "$order_log"
exit 0
EOF
  chmod +x "$work/bubbles/scripts/$name"
}

make_stub generate-framework-stats.sh stats.stale
make_stub generate-cheatsheet.sh cheat.stale
make_stub generate-capability-ledger-docs.sh ledger.stale
make_stub generate-release-manifest.sh manifest.stale

# --- Case 1: clean tree → exit 0, generators invoked in dependency order -------
: >"$order_log"
set +e
bash "$work/bubbles/scripts/regen-derived.sh" >"$work/c1.log" 2>&1
c1=$?
set -e
if [[ "$c1" -eq 0 ]]; then
  pass "clean tree regenerates and exits 0"
else
  fail "clean tree should exit 0"
  sed -n '1,60p' "$work/c1.log"
fi

expected="$(printf '%s\n' generate-framework-stats.sh generate-cheatsheet.sh generate-capability-ledger-docs.sh generate-release-manifest.sh)"
if [[ "$(cat "$order_log")" == "$expected" ]]; then
  pass "generators invoked in dependency order (stats -> cheatsheet -> ledger -> manifest LAST)"
else
  fail "generator order is wrong"
  echo "--- expected ---"
  echo "$expected"
  echo "--- got ---"
  cat "$order_log"
fi

# --- Case 2: a generator still stale after regen → exit 1 (fail loud) ----------
touch "$work/manifest.stale"
set +e
bash "$work/bubbles/scripts/regen-derived.sh" >"$work/c2.log" 2>&1
c2=$?
set -e
if [[ "$c2" -ne 0 ]]; then
  pass "a generator still stale after regeneration makes the wrapper exit non-zero"
else
  fail "a stale generator should fail the wrapper"
  sed -n '1,60p' "$work/c2.log"
fi
rm -f "$work/manifest.stale"

# --- Case 3: --check-only verifies WITHOUT regenerating ------------------------
: >"$order_log"
set +e
bash "$work/bubbles/scripts/regen-derived.sh" --check-only >"$work/c3.log" 2>&1
c3=$?
set -e
if [[ "$c3" -eq 0 && ! -s "$order_log" ]]; then
  pass "--check-only verifies freshness without regenerating"
else
  fail "--check-only should verify without writing to the order log"
  echo "exit=$c3 order_log_size=$(wc -c <"$order_log")"
  sed -n '1,60p' "$work/c3.log"
fi

# --- Case 4: bad usage → exit 2 -----------------------------------------------
set +e
bash "$work/bubbles/scripts/regen-derived.sh" --bogus >"$work/c4.log" 2>&1
c4=$?
set -e
if [[ "$c4" -eq 2 ]]; then
  pass "unknown argument exits 2 (bad usage)"
else
  fail "unknown argument should exit 2"
  sed -n '1,60p' "$work/c4.log"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[regen-derived-selftest] OK"
else
  echo "[regen-derived-selftest] $failures failed"
  exit 1
fi
