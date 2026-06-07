#!/usr/bin/env bash
set -euo pipefail

# File: shellcheck-lint-selftest.sh
#
# Hermetic selftest for shellcheck-lint.sh. Proves the gate PASSes on a clean
# fixture, FAILs on a dirty fixture (adversarial — a reintroduced SC2034 warning
# must be caught), and rejects invalid arguments. When shellcheck is not
# installed, the gate advisory-skips and the selftest asserts that path instead.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="$SCRIPT_DIR/shellcheck-lint.sh"

pass_count=0
fail_count=0
ok() {
  echo "PASS: $*"
  pass_count=$((pass_count + 1))
}
bad() {
  echo "FAIL: $*" >&2
  fail_count=$((fail_count + 1))
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Clean fixture (no findings at -S warning).
clean_dir="$TMP/clean-only"
mkdir -p "$clean_dir"
cat >"$clean_dir/clean.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
greeting="hello"
echo "$greeting"
EOF

# Dirty fixture (SC2034 unused variable — a warning).
dirty_dir="$TMP/dirty-only"
mkdir -p "$dirty_dir"
cat >"$dirty_dir/dirty.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
never_read="oops"
echo "done"
EOF

if ! command -v shellcheck >/dev/null 2>&1; then
  # No shellcheck: gate must advisory-skip (exit 0) even on a dirty dir.
  set +e
  bash "$GATE" --dir "$dirty_dir" --quiet >/dev/null 2>&1
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    ok "advisory-skip (exit 0) when shellcheck is not installed"
  else
    bad "gate must exit 0 when shellcheck is not installed (got $rc)"
  fi
  echo "shellcheck-lint-selftest: ${pass_count} pass, ${fail_count} fail (shellcheck absent)"
  [[ "$fail_count" -eq 0 ]] || exit 1
  echo "shellcheck-lint-selftest: PASSED"
  exit 0
fi

# 1. Clean dir -> PASS (exit 0).
set +e
bash "$GATE" --dir "$clean_dir" --quiet
rc=$?
set -e
if [[ "$rc" -eq 0 ]]; then
  ok "gate PASSes on a clean fixture directory"
else
  bad "gate should PASS on a clean fixture directory (got $rc)"
fi

# 2. Dirty dir -> FAIL (exit 1). Adversarial: proves a reintroduced SC2034
#    warning would be caught (the regression this gate exists to prevent).
set +e
bash "$GATE" --dir "$dirty_dir" --quiet >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 1 ]]; then
  ok "gate FAILs (exit 1) on a dirty fixture (SC2034) — a reintroduced warning is caught"
else
  bad "gate should FAIL with exit 1 on an SC2034 warning (got $rc)"
fi

# 3. Invalid severity -> exit 2.
set +e
bash "$GATE" --severity bogus --dir "$clean_dir" >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "gate rejects an invalid severity with exit 2"
else
  bad "gate should exit 2 on an invalid severity (got $rc)"
fi

# 4. The live tracked shell surface must be clean at -S warning.
set +e
bash "$GATE" --quiet
rc=$?
set -e
if [[ "$rc" -eq 0 ]]; then
  ok "live tracked shell surface is clean at -S warning"
else
  bad "live tracked shell surface has shellcheck warnings (got $rc)"
fi

echo "shellcheck-lint-selftest: ${pass_count} pass, ${fail_count} fail"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "shellcheck-lint-selftest: PASSED"
