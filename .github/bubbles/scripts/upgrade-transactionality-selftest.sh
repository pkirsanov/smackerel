#!/usr/bin/env bash
#
# upgrade-transactionality-selftest.sh — IMP-102 SCOPE-6.
#
# Proves that `bubbles upgrade` (cli.sh cmd_upgrade) is TRANSACTIONAL: a failed
# install (or a failed post-install doctor) must NOT report success. cli.sh runs
# `set -uo pipefail` WITHOUT `-e`, so before this fix a non-zero install.sh /
# doctor fell through to the unconditional "✅ Upgrade complete." + fun_summary
# pass, reporting a broken upgrade as a good one.
#
# The harness drives the REAL cli.sh through its supported hermetic seam,
# BUBBLES_SOURCE_OVERRIDE_DIR (cmd_upgrade runs `<override>/install.sh <version>`
# instead of curl|bash), so no network is touched. cli.sh's run-state writes land
# in the gitignored .specify/runtime/ tree, so a run leaves the working tree clean.
#
# Cases:
#   A (deterministic, REQUIRED) — failing install.sh (exit 7) against the CURRENT
#     cli.sh must: return non-zero (7), NOT print "Upgrade complete.", and name
#     the failing install step. The fix returns BEFORE doctor, so this case is fast.
#   B (non-tautology proof) — the SAME failing fixture against the OLD cmd_upgrade
#     (git show ce40285:bubbles/scripts/cli.sh) must WRONGLY print success, proving
#     the Case-A assertions actually depend on the fix and are not tautological.
#   C (doctor-gating, second case) — a passing install.sh (exit 0) exercises the
#     doctor gate. The assertion follows the CONTRACT for whichever outcome doctor
#     produces in this tree: doctor-pass => exit 0 + success printed; doctor-fail
#     => non-zero + "doctor validation failed" + NO false "Upgrade complete."
#
# Graceful-skip: no bash / cli.sh absent -> SKIP (exit 0). Case B additionally
# skips if git or the ce40285 blob is unavailable, without failing the harness.
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI="$SCRIPT_DIR/cli.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OLD_CLI_REF="ce40285"
UPGRADE_TARGET="imp102-scope6-selftest"

pass_count=0
fail_count=0
pass() {
  printf 'PASS: %s\n' "$1"
  pass_count=$((pass_count + 1))
}
fail() {
  printf 'FAIL: %s\n' "$1"
  fail_count=$((fail_count + 1))
}

if [[ ! -f "$CLI" ]]; then
  echo "upgrade-transactionality-selftest: SKIP (cli.sh not found at $CLI)"
  exit 0
fi

WORK="$(mktemp -d)"
OLD_CLI=""
cleanup() {
  rm -rf "$WORK"
  [[ -n "$OLD_CLI" && -f "$OLD_CLI" ]] && rm -f "$OLD_CLI"
  return 0
}
trap cleanup EXIT

# ── Fixtures: a failing install source and a passing install source ──
FAIL_SRC="$WORK/fail-src"
PASS_SRC="$WORK/pass-src"
mkdir -p "$FAIL_SRC" "$PASS_SRC"
printf '#!/usr/bin/env bash\nexit 7\n' >"$FAIL_SRC/install.sh"
printf '#!/usr/bin/env bash\nexit 0\n' >"$PASS_SRC/install.sh"
chmod +x "$FAIL_SRC/install.sh" "$PASS_SRC/install.sh"

# ── Case A: failing install against the CURRENT cli.sh (deterministic) ──
a_out="$(BUBBLES_SOURCE_OVERRIDE_DIR="$FAIL_SRC" bash "$CLI" upgrade "$UPGRADE_TARGET" 2>&1)"
a_rc=$?

if [[ "$a_rc" -ne 0 ]]; then
  pass "failing install -> upgrade returns non-zero (rc=$a_rc)"
else
  fail "failing install -> upgrade should return non-zero, got rc=0"
fi

if [[ "$a_rc" -eq 7 ]]; then
  pass "failing install -> upgrade propagates the install exit code (7)"
else
  fail "failing install -> upgrade should propagate exit 7, got rc=$a_rc"
fi

if printf '%s' "$a_out" | grep -q "Upgrade complete."; then
  fail "failing install -> upgrade WRONGLY printed 'Upgrade complete.'"
else
  pass "failing install -> upgrade does NOT print 'Upgrade complete.'"
fi

if printf '%s' "$a_out" | grep -q "Upgrade failed: install.sh exited 7"; then
  pass "failing install -> upgrade names the failing install step + code"
else
  fail "failing install -> upgrade should name 'install.sh exited 7', got: $a_out"
fi

# ── Case B: NON-TAUTOLOGY — the SAME fixture against the OLD cmd_upgrade ──
# The worktree HEAD is ce40285 (pre-fix); the OLD cmd_upgrade ignored install.sh's
# exit and printed success unconditionally. Prove Case A's assertions actually
# depend on the fix by showing the OLD code WRONGLY succeeds on the same fixture.
if command -v git >/dev/null 2>&1 &&
  git -C "$REPO_ROOT" cat-file -e "${OLD_CLI_REF}:bubbles/scripts/cli.sh" 2>/dev/null; then
  # Extract the OLD cli.sh INTO the real scripts dir under a temp name so its
  # relative `source .../fun-mode.sh|aliases.sh|trust-metadata.sh` calls resolve
  # exactly as the real cli.sh does. Removed by the EXIT trap.
  OLD_CLI="$SCRIPT_DIR/.upgrade-transactionality-selftest-old.$$.sh"
  if git -C "$REPO_ROOT" show "${OLD_CLI_REF}:bubbles/scripts/cli.sh" >"$OLD_CLI" 2>/dev/null; then
    old_out="$(BUBBLES_SOURCE_OVERRIDE_DIR="$FAIL_SRC" bash "$OLD_CLI" upgrade "$UPGRADE_TARGET" 2>&1)"
    old_rc=$?
    if printf '%s' "$old_out" | grep -q "Upgrade complete."; then
      pass "non-tautology: OLD cli.sh ($OLD_CLI_REF) WRONGLY prints 'Upgrade complete.' on a failing install (rc=$old_rc)"
    else
      fail "non-tautology: OLD cli.sh ($OLD_CLI_REF) should exhibit the bug (print success), but did not; out: $old_out"
    fi
    rm -f "$OLD_CLI"
    OLD_CLI=""
  else
    echo "SKIP: non-tautology check (could not extract ${OLD_CLI_REF}:bubbles/scripts/cli.sh)"
  fi
else
  echo "SKIP: non-tautology check (git or ${OLD_CLI_REF} blob unavailable)"
fi

# ── Case C: passing install exercises the doctor gate (second case) ──
# Doctor runs against this real tree; assert the CONTRACT for whichever outcome it
# yields, so the case is robust rather than pinned to a fixed doctor result.
c_out="$(BUBBLES_SOURCE_OVERRIDE_DIR="$PASS_SRC" bash "$CLI" upgrade "$UPGRADE_TARGET" 2>&1)"
c_rc=$?
if printf '%s' "$c_out" | grep -q "Upgrade complete."; then
  if [[ "$c_rc" -eq 0 ]]; then
    pass "install passes + doctor passes -> exit 0 with 'Upgrade complete.'"
  else
    fail "install passes and 'Upgrade complete.' printed, but rc=$c_rc (expected 0)"
  fi
else
  if [[ "$c_rc" -ne 0 ]] && printf '%s' "$c_out" | grep -q "doctor validation failed"; then
    pass "install passes + doctor fails -> non-zero (rc=$c_rc) with 'doctor validation failed', no false success"
  else
    fail "install passes but doctor gating is wrong (rc=$c_rc); out: $c_out"
  fi
fi

echo "---"
echo "upgrade-transactionality-selftest: ${pass_count} passed, ${fail_count} failed"
[[ "$fail_count" -eq 0 ]]
