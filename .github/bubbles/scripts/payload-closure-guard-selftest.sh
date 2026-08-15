#!/usr/bin/env bash
# payload-closure-guard-selftest.sh — prove the closure guard can fail.
#
# IMP-042 SCOPE-12 / REG-11.
#
# A guard that only ever passes is decoration. This builds fixture repositories
# with a synthetic release manifest and asserts both directions: a managed file
# reaching an unshipped dependency FAILS, and each accepted form PASSES. The
# accepted forms are not conveniences -- each one was a real false positive
# during development, and each is a shape that degrades cleanly downstream.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/payload-closure-guard.sh"
LABEL="payload-closure-guard-selftest"

pass_count=0
fail_count=0

pass() {
  printf '[%s] PASS %s\n' "$LABEL" "$*"
  pass_count=$((pass_count + 1))
}

fail() {
  printf '[%s] FAIL %s\n' "$LABEL" "$*" >&2
  fail_count=$((fail_count + 1))
}

# Build a fixture repo. $1 = root, $2 = body of the managed script under test.
build_fixture() {
  local root="$1"
  local managed_body="$2"
  mkdir -p "$root/bubbles/scripts" "$root/tests/regression"

  printf '%s\n' "$managed_body" >"$root/bubbles/scripts/consumer.sh"
  printf '#!/usr/bin/env bash\necho harness\n' >"$root/bubbles/scripts/eval-harness.sh"
  printf '#!/usr/bin/env bash\necho regression\n' >"$root/tests/regression/test_99_fixture.sh"

  cat >"$root/bubbles/release-manifest.json" <<'JSON'
{
  "schemaVersion": 1,
  "managedFileCount": 1,
  "managedFileChecksums": [
    {"path": "bubbles/scripts/consumer.sh", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"}
  ],
  "sourceOnlyFileCount": 2,
  "sourceOnlyFileChecksums": [
    {"path": "bubbles/scripts/eval-harness.sh", "sha256": "1111111111111111111111111111111111111111111111111111111111111111"},
    {"path": "tests/regression/test_99_fixture.sh", "sha256": "2222222222222222222222222222222222222222222222222222222222222222"}
  ]
}
JSON
}

run_case() {
  local name="$1" expected="$2" body="$3"
  local root actual
  root="$(mktemp -d)"
  build_fixture "$root" "$body"
  bash "$GUARD" "$root" >/dev/null 2>&1
  actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    pass "$name (exit $actual)"
  else
    fail "$name (expected exit $expected, got $actual)"
  fi
  rm -rf "$root"
}

# --- RED: an unguarded variable-rooted reference must fail ------------------
run_case "T1 unguarded \$SCRIPT_DIR reference to a source-only script fails" 1 \
  '#!/usr/bin/env bash
HARNESS="$SCRIPT_DIR/eval-harness.sh"
bash "$HARNESS" run'

run_case "T2 unguarded \$REPO_ROOT reference to a source-only regression test fails" 1 \
  '#!/usr/bin/env bash
bash "$REPO_ROOT/tests/regression/test_99_fixture.sh"'

# --- GREEN: each accepted form must pass ------------------------------------
run_case "T3 existence-guarded reference passes" 0 \
  '#!/usr/bin/env bash
if [[ ! -f "$SCRIPT_DIR/eval-harness.sh" ]]; then
  echo "source-only subsystem, not available downstream" >&2
  exit 2
fi
bash "$SCRIPT_DIR/eval-harness.sh" run'

run_case "T4 run_check_self_only scheduling passes" 0 \
  '#!/usr/bin/env bash
run_check_self_only "fixture regression" bash "$REPO_ROOT/tests/regression/test_99_fixture.sh"'

run_case "T5 literal registry entry is not a dependency" 0 \
  '#!/usr/bin/env bash
source_only=(
  "bubbles/scripts/eval-harness.sh"
  "tests/regression/test_99_fixture.sh"
)
printf "%s\n" "${source_only[@]}"'

run_case "T6 comment mentioning the script is not a dependency" 0 \
  '#!/usr/bin/env bash
# Invoked by eval-harness.sh as: consumer.sh <out_dir>
# See validate_result() in bubbles/scripts/eval-harness.sh
echo ok'

run_case "T7 writing a fixture file is not a dependency" 0 \
  '#!/usr/bin/env bash
repo="$(mktemp -d)"
mkdir -p "$repo/tests/regression"
cat > "$repo/tests/regression/test_99_fixture.sh" <<EOF
echo generated
EOF'

run_case "T9 a reasoned payload-closure-allow marker exempts the reference" 0 \
  '#!/usr/bin/env bash
# payload-closure-allow: resolves against an operator-supplied source checkout,
# not against the installed payload.
bash "$operator_source/eval-harness.sh" run'

# --- Guard must refuse a missing manifest rather than silently pass ---------
missing_root="$(mktemp -d)"
bash "$GUARD" "$missing_root" >/dev/null 2>&1
missing_exit=$?
if [[ "$missing_exit" -eq 2 ]]; then
  pass "T8 missing manifest exits 2 rather than reporting closure"
else
  fail "T8 missing manifest (expected exit 2, got $missing_exit)"
fi
rm -rf "$missing_root"

printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
