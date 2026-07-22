#!/usr/bin/env bash
# management-truth-lint-selftest.sh — hermetic selftest for management-truth-lint.sh.
#
# Builds throwaway fixture repos under mktemp and asserts the lint's exit code
# for: a clean catalog, an unlinked recipe, an undocumented adoption profile,
# a README-only catalog, absent surfaces, and a fully-documented profile set.
# No network, no dependency on the live framework tree.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/management-truth-lint.sh"

pass=0
fail=0

TMP_ROOT="$(mktemp -d)"
cleanup() { rm -rf "$TMP_ROOT"; }
trap cleanup EXIT

assert_exit() {
  local expected="$1" label="$2"; shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS: $label (exit $actual)"
    pass=$((pass + 1))
  else
    echo "FAIL: $label (expected exit $expected, got $actual)"
    fail=$((fail + 1))
  fi
}

mk_repo() {
  local repo="$1"
  mkdir -p "$repo/docs/recipes" "$repo/bubbles"
  printf '%s' "$repo"
}

# ── Case 1: clean recipe catalog + profile help (both checks pass) ─────
c1="$(mk_repo "$TMP_ROOT/c1")"
printf '# Recipes\n\n[A](alpha.md) and [B](beta.md)\n' > "$c1/docs/recipes/README.md"
printf '# Alpha\n' > "$c1/docs/recipes/alpha.md"
printf '# Beta\n' > "$c1/docs/recipes/beta.md"
printf 'profiles:\n  foundation:\n    id: foundation\n  assured:\n    id: assured\n' > "$c1/bubbles/adoption-profiles.yaml"
printf '#!/usr/bin/env bash\necho "  --profile ID  (foundation, assured)"\n' > "$c1/install.sh"
assert_exit 0 "Case 1: clean recipe catalog + profile help" bash "$LINT" "$c1"

# ── Case 2: an unlinked recipe fails check 1 ──────────────────────────
c2="$(mk_repo "$TMP_ROOT/c2")"
printf '# Recipes\n\n[A](alpha.md)\n' > "$c2/docs/recipes/README.md"
printf '# Alpha\n' > "$c2/docs/recipes/alpha.md"
printf '# Orphan\n' > "$c2/docs/recipes/orphan.md"
assert_exit 1 "Case 2: unlinked recipe fails" bash "$LINT" "$c2"

# ── Case 3: an undocumented adoption profile fails check 2 ────────────
c3="$(mk_repo "$TMP_ROOT/c3")"
printf '# Recipes\n\n[A](alpha.md)\n' > "$c3/docs/recipes/README.md"
printf '# Alpha\n' > "$c3/docs/recipes/alpha.md"
printf 'profiles:\n  foundation:\n    id: foundation\n  production:\n    id: production\n' > "$c3/bubbles/adoption-profiles.yaml"
printf '#!/usr/bin/env bash\necho "  --profile ID  (foundation)"\n' > "$c3/install.sh"
assert_exit 1 "Case 3: undocumented adoption profile fails" bash "$LINT" "$c3"

# ── Case 4: a README-only catalog passes (README is excluded) ─────────
c4="$(mk_repo "$TMP_ROOT/c4")"
printf '# Recipes\n\n(no recipe files besides me)\n' > "$c4/docs/recipes/README.md"
assert_exit 0 "Case 4: README-only catalog passes" bash "$LINT" "$c4"

# ── Case 5: absent surfaces skip gracefully ───────────────────────────
c5="$TMP_ROOT/c5"; mkdir -p "$c5"
assert_exit 0 "Case 5: absent surfaces skip gracefully" bash "$LINT" "$c5"

# ── Case 6: all four profiles documented in help passes ───────────────
c6="$(mk_repo "$TMP_ROOT/c6")"
printf '# Recipes\n\n[A](alpha.md)\n' > "$c6/docs/recipes/README.md"
printf '# Alpha\n' > "$c6/docs/recipes/alpha.md"
printf 'profiles:\n  foundation:\n    id: foundation\n  delivery:\n    id: delivery\n  production:\n    id: production\n  assured:\n    id: assured\n' > "$c6/bubbles/adoption-profiles.yaml"
printf '#!/usr/bin/env bash\necho "  --profile ID  Select adoption profile (foundation, delivery, production, or assured)"\n' > "$c6/install.sh"
assert_exit 0 "Case 6: all four profiles documented passes" bash "$LINT" "$c6"

# ── Case 7: documented count matches the inventory (pass) ──────────
c7="$(mk_repo "$TMP_ROOT/c7")"
mkdir -p "$c7/agents" "$c7/docs/guides"
printf '# a\n' > "$c7/agents/alpha.agent.md"
printf '# b\n' > "$c7/agents/beta.agent.md"
printf 'This installs 2 agent definitions and more.\n' > "$c7/docs/guides/INSTALLATION.md"
assert_exit 0 "Case 7: matching documented count passes" bash "$LINT" "$c7"

# ── Case 8: a stale documented count fails ─────────────────────
c8="$(mk_repo "$TMP_ROOT/c8")"
mkdir -p "$c8/agents" "$c8/docs/guides"
printf '# a\n' > "$c8/agents/alpha.agent.md"
printf 'This installs 7 agent definitions and more.\n' > "$c8/docs/guides/INSTALLATION.md"
assert_exit 1 "Case 8: stale documented count fails" bash "$LINT" "$c8"

# ── Case 9: absent count phrase skips gracefully (pass) ──────────
c9="$(mk_repo "$TMP_ROOT/c9")"
mkdir -p "$c9/agents" "$c9/docs/guides"
printf '# a\n' > "$c9/agents/alpha.agent.md"
printf 'Install instructions with no count phrase at all.\n' > "$c9/docs/guides/INSTALLATION.md"
assert_exit 0 "Case 9: absent count phrase skips" bash "$LINT" "$c9"

echo
echo "management-truth-lint selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
