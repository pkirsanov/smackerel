#!/usr/bin/env bash
# managed-docs-existence-lint-selftest.sh — prove the managed-doc check fails.
#
# IMP-042 SCOPE-13 / REG-12.
#
# The lint skips a framework source tree, which is the right call but also the
# easy way for it to become permanently green and never notice anything. These
# fixtures run it against product-shaped trees where it must actually decide.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/managed-docs-existence-lint.sh"
RESOLVER="$SCRIPT_DIR/docs-registry-resolve.sh"
REGISTRY="$SCRIPT_DIR/../docs-registry.yaml"
LABEL="managed-docs-existence-lint-selftest"

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

# Build a product-shaped fixture: framework scripts under .github/, no install.sh
# or VERSION at the root, so the lint does not take the framework-source skip.
build_fixture() {
  local root="$1"
  mkdir -p "$root/.github/bubbles/scripts" "$root/docs"
  cp "$LINT" "$root/.github/bubbles/scripts/"
  cp "$RESOLVER" "$root/.github/bubbles/scripts/"
  cp "$REGISTRY" "$root/.github/bubbles/docs-registry.yaml"
}

# --- RED: a required doc with no file must fail ----------------------------
root="$(mktemp -d)"
build_fixture "$root"
bash "$root/.github/bubbles/scripts/managed-docs-existence-lint.sh" "$root" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "T1 a required managed doc with no file fails (exit 1)"
else
  fail "T1 expected exit 1 for missing required docs, got $rc"
fi
rm -rf "$root"

# --- GREEN: create every required doc, and it passes -----------------------
root="$(mktemp -d)"
build_fixture "$root"
while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  mkdir -p "$root/$(dirname "$path")"
  printf '# fixture\n' >"$root/$path"
done < <(cd "$root" && bash .github/bubbles/scripts/docs-registry-resolve.sh --effective 2>/dev/null | awk '
  /^  [a-zA-Z][a-zA-Z0-9_-]*:[[:space:]]*$/ { path = ""; next }
  /^    path:[[:space:]]/ { path = $2; next }
  /^    required:[[:space:]]/ { if ($2 == "true" && path != "") print path; next }
')
bash "$root/.github/bubbles/scripts/managed-docs-existence-lint.sh" "$root" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then
  pass "T2 every required managed doc present passes (exit 0)"
else
  fail "T2 expected exit 0 with all required docs present, got $rc"
fi

# --- RED again: delete one of them, and it must fail -------------------------
one_required="$(cd "$root" && bash .github/bubbles/scripts/docs-registry-resolve.sh --effective 2>/dev/null | awk '
  /^  [a-zA-Z][a-zA-Z0-9_-]*:[[:space:]]*$/ { path = ""; next }
  /^    path:[[:space:]]/ { path = $2; next }
  /^    required:[[:space:]]/ { if ($2 == "true" && path != "") { print path; exit } }
')"
rm -f "$root/$one_required"
bash "$root/.github/bubbles/scripts/managed-docs-existence-lint.sh" "$root" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "T3 removing one required doc ($one_required) fails again (exit 1)"
else
  fail "T3 expected exit 1 after removing $one_required, got $rc"
fi
rm -rf "$root"

# --- The framework-source skip must be conditional, not unconditional ------
root="$(mktemp -d)"
build_fixture "$root"
printf '#!/usr/bin/env bash\n' >"$root/install.sh"
printf '7.0.0\n' >"$root/VERSION"
mkdir -p "$root/bubbles/scripts"
bash "$root/.github/bubbles/scripts/managed-docs-existence-lint.sh" "$root" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then
  pass "T4 a framework source tree skips rather than reporting guaranteed failures"
else
  fail "T4 expected the framework-source skip to exit 0, got $rc"
fi
rm -rf "$root"

printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
