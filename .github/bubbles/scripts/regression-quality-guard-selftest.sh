#!/usr/bin/env bash
# regression-quality-guard-selftest.sh
#
# Hermetic selftest for regression-quality-guard.sh.
#
# Stages synthetic test fixtures under a temp directory, invokes the guard
# script, and asserts:
#   - Test files containing silent-pass bailout patterns trigger
#     FALSE_NEGATIVE_BAILOUT and exit non-zero.
#   - Test files with concrete assertions and no bailout pass cleanly
#     (exit 0).
#   - In --bugfix mode, a regression file lacking adversarial signals
#     triggers ADVERSARIAL_REGRESSION_MISSING and exits non-zero, while
#     a file containing an adversarial signal (e.g. .not.) exits 0.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/regression-quality-guard.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "[selftest regression-quality-guard] FAIL: target script missing at $GUARD" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

mkdir -p "$TMPDIR/tests"

violating="$TMPDIR/tests/login-bailout.spec.ts"
cat > "$violating" <<'EOF'
import { test, expect } from '@playwright/test';

test('protected dashboard is reachable', async ({ page }) => {
  await page.goto('/dashboard');
  const url = page.url();
  if (url.includes('/login')) return;
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
});
EOF

clean="$TMPDIR/tests/login-strict.spec.ts"
cat > "$clean" <<'EOF'
import { test, expect } from '@playwright/test';

test('protected dashboard is reachable', async ({ page }) => {
  await page.goto('/dashboard');
  const url = page.url();
  expect(url).not.toContain('/login');
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
});
EOF

bugfix_missing="$TMPDIR/tests/regression-no-adversarial.spec.ts"
cat > "$bugfix_missing" <<'EOF'
import { test, expect } from '@playwright/test';

test('happy path still works', async ({ page }) => {
  await page.goto('/items');
  await expect(page.getByText('Item A')).toBeVisible();
});
EOF

bugfix_signal="$TMPDIR/tests/regression-adversarial.spec.ts"
cat > "$bugfix_signal" <<'EOF'
import { test, expect } from '@playwright/test';

test('item without optional metadata still appears (regression)', async ({ page }) => {
  await page.goto('/items');
  await expect(page.getByText('Item Without Metadata')).toBeVisible();
  expect(page.url()).not.toContain('/error');
});
EOF

echo "[selftest regression-quality-guard] Case 1: violating file → exit 1"
log1="$TMPDIR/log1.txt"
set +e
bash "$GUARD" --verbose "$violating" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -ne 0 ]]; then
  pass "violating file exits non-zero (got $status1)"
else
  fail "violating file should exit non-zero (got $status1)"
  sed -n '1,80p' "$log1"
fi
if grep -Fq 'FALSE_NEGATIVE_BAILOUT' "$log1"; then
  pass "violating file output contains FALSE_NEGATIVE_BAILOUT"
else
  fail "expected FALSE_NEGATIVE_BAILOUT token in output"
  sed -n '1,80p' "$log1"
fi

echo "[selftest regression-quality-guard] Case 2: clean file → exit 0"
log2="$TMPDIR/log2.txt"
set +e
bash "$GUARD" "$clean" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -eq 0 ]]; then
  pass "clean file exits 0"
else
  fail "clean file should exit 0 (got $status2)"
  sed -n '1,80p' "$log2"
fi

echo "[selftest regression-quality-guard] Case 3: --bugfix without adversarial signal → exit 1"
log3="$TMPDIR/log3.txt"
set +e
bash "$GUARD" --bugfix "$bugfix_missing" >"$log3" 2>&1
status3=$?
set -e
if [[ "$status3" -ne 0 ]]; then
  pass "bugfix-mode missing adversarial signal exits non-zero (got $status3)"
else
  fail "bugfix-mode missing adversarial signal should exit non-zero (got $status3)"
  sed -n '1,80p' "$log3"
fi
if grep -Fq 'ADVERSARIAL_REGRESSION_MISSING' "$log3"; then
  pass "bugfix-mode output contains ADVERSARIAL_REGRESSION_MISSING"
else
  fail "expected ADVERSARIAL_REGRESSION_MISSING token in output"
  sed -n '1,80p' "$log3"
fi

echo "[selftest regression-quality-guard] Case 4: --bugfix with adversarial signal → exit 0"
log4="$TMPDIR/log4.txt"
set +e
bash "$GUARD" --bugfix "$bugfix_signal" >"$log4" 2>&1
status4=$?
set -e
if [[ "$status4" -eq 0 ]]; then
  pass "bugfix-mode with adversarial signal exits 0"
else
  fail "bugfix-mode with adversarial signal should exit 0 (got $status4)"
  sed -n '1,80p' "$log4"
fi

# --- IMP-040 SCOPE-5: production-path fidelity (COV-10) ---------------------
#
# Cases 5-7 are the substitution shapes. Cases 8-9 are their GUARDS: the same
# shapes must PASS when the file also asserts the current visible surface,
# because the rule is "offered as the sole proof", not "present anywhere". If
# 8-9 ever fail, the check has become a blanket ban on internal inspection and
# will be switched off by the first team that hits it.

detached="$TMPDIR/tests/detached-render.spec.ts"
cat > "$detached" <<'EOF'
import { render } from 'react-dom';
import { Dashboard } from '../src/Dashboard';

test('dashboard heading renders', () => {
  const host = document.createElement('div');
  ReactDOM.render(<Dashboard />, host);
  expect(host.innerHTML).toContain('Dashboard');
});
EOF

intercepted="$TMPDIR/tests/intercepted.spec.ts"
cat > "$intercepted" <<'EOF'
import { test, expect } from '@playwright/test';

test('items list loads', async ({ page }) => {
  await page.route('**/api/items', (r) => r.fulfill({ body: '[{"id":1}]' }));
  await page.goto('/items');
  const html = await page.evaluate(() => document.querySelector('#list').innerHTML);
  expect(html).toContain('1');
});
EOF

hidden="$TMPDIR/tests/hidden-surface.spec.ts"
cat > "$hidden" <<'EOF'
import { test, expect } from '@playwright/test';

test('banner is present', async ({ page }) => {
  await page.goto('/home');
  const node = await page.evaluate(() => document.querySelector('.banner').innerHTML);
  expect(node).toContain('Welcome');
});
EOF

mixed_route="$TMPDIR/tests/mixed-route.spec.ts"
cat > "$mixed_route" <<'EOF'
import { test, expect } from '@playwright/test';

test('banner is visible and carries the right payload', async ({ page }) => {
  await page.goto('/home');
  await expect(page.getByRole('banner')).toBeVisible();
  const raw = await page.evaluate(() => document.querySelector('.banner').innerHTML);
  expect(raw).toContain('Welcome');
});
EOF

mixed_intercept="$TMPDIR/tests/mixed-intercept.spec.ts"
cat > "$mixed_intercept" <<'EOF'
import { test, expect } from '@playwright/test';

test('empty provider response paints the unavailable state', async ({ page }) => {
  await page.route('**/api/items', (r) => r.fulfill({ body: '[]' }));
  await page.goto('/items');
  await expect(page.getByText('No items available')).toBeVisible();
});
EOF

check_fidelity() {
  local label="$1" fixture="$2" want="$3" logf="$TMPDIR/log-$4.txt"
  set +e
  bash "$GUARD" "$fixture" >"$logf" 2>&1
  local st=$?
  set -e
  if [[ "$want" == "reject" ]]; then
    if [[ "$st" -ne 0 ]] && grep -Fq 'PRODUCTION_PATH_SUBSTITUTION' "$logf"; then
      pass "$label"
    else
      fail "$label (exit $st)"
      sed -n '1,60p' "$logf"
    fi
  else
    if [[ "$st" -eq 0 ]]; then
      pass "$label"
    else
      fail "$label (exit $st)"
      sed -n '1,60p' "$logf"
    fi
  fi
}

echo "[selftest regression-quality-guard] Case 5-7: sole-proof substitution → exit 1"
check_fidelity "detached render as sole proof is refused" "$detached" reject 5
check_fidelity "interception plus internal DOM as sole proof is refused" "$intercepted" reject 6
check_fidelity "internal DOM read as sole proof is refused" "$hidden" reject 7

echo "[selftest regression-quality-guard] Case 8-9: mixed tests still pass → exit 0"
check_fidelity "internal read WITH a visible assertion is accepted" "$mixed_route" accept 8
check_fidelity "interception WITH a visible assertion is accepted" "$mixed_intercept" accept 9

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest regression-quality-guard] PASS"
  exit 0
fi

echo "[selftest regression-quality-guard] FAIL: $failures assertion(s)"
exit 1
