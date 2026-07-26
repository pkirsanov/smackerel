#!/usr/bin/env bash
# agent-bundle-size-budget-selftest.sh — hermetic selftest for
# agent-bundle-size-budget.sh (IMP-102 / SCOPE-10).
#
# Proves the ratcheting per-agent budget has TEETH and RATCHETS:
#   * an over-ceiling agent FAILS --check (adversarial teeth);
#   * teeth count the TRANSITIVE closure — an agent under-ceiling on its own file
#     but over once its imported bubbles_shared module is added FAILS --check;
#   * an under-ceiling agent PASSES;
#   * an agent with NO recorded ceiling FAILS (a new agent must get one);
#   * a missing budget file (with agents present) FAILS;
#   * a repo with no agents/ SKIPS (exit 0);
#   * --seed writes a green budget, and re-seeding a SHRUNKEN agent RATCHETS the
#     ceiling DOWN (bloat can't creep back);
#   * a --seed breach without --accept-growth is REFUSED (exit 2), and no silent raise.
# Finally, if the real framework tree is present, asserts the COMMITTED budget is
# in sync (every real agent at/under its committed ceiling).
#
# Depends on effective-bundle-measure.sh + jq (both in the same dir / PATH).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUDGET="$SCRIPT_DIR/agent-bundle-size-budget.sh"

if ! command -v jq >/dev/null 2>&1; then
  echo "agent-bundle-size-budget-selftest: SKIP (jq not installed)"
  exit 0
fi

pass=0
fail=0
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

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

assert_true() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "PASS: $label"
    pass=$((pass + 1))
  else
    echo "FAIL: $label"
    fail=$((fail + 1))
  fi
}

# A repo fixture with a single plain agent (no shared imports).
mk_plain_repo() {
  local root="$1"
  mkdir -p "$root/agents"
  printf '# Tiny agent\nSome body content so the effective bundle has real bytes.\n' > "$root/agents/tiny.agent.md"
  printf '%s' "$root"
}

# A repo fixture where the agent IMPORTS a large shared module (transitive weight).
mk_transitive_repo() {
  local root="$1"
  mkdir -p "$root/agents/bubbles_shared"
  printf '# Importer\nLoads bubbles_shared/big.md for its contract.\n' > "$root/agents/imp.agent.md"
  # ~5 KB shared module so the closure dwarfs the agent file alone.
  { printf '# Big shared module\n'; head -c 5000 /dev/zero | tr '\0' 'x'; printf '\n'; } > "$root/agents/bubbles_shared/big.md"
  printf '%s' "$root"
}

budget_of() {
  # $1 = budget file, $2 = agent key -> stdout ceiling (or empty)
  jq -r --arg k "$2" '.[$k] // empty' "$1"
}

# ── Case 1: over-ceiling agent FAILS --check (adversarial teeth) ──
r1="$(mk_plain_repo "$TMP_ROOT/r1")"
printf '{"tiny.agent.md": 1}\n' > "$r1/budget.json"
assert_exit 1 "Case 1: over-ceiling agent fails --check" \
  bash "$BUDGET" --check --repo-root "$r1" --agents-dir "$r1/agents" --budget-file "$r1/budget.json"

# ── Case 2: under-ceiling agent PASSES --check ──
r2="$(mk_plain_repo "$TMP_ROOT/r2")"
printf '{"tiny.agent.md": 9999999}\n' > "$r2/budget.json"
assert_exit 0 "Case 2: under-ceiling agent passes --check" \
  bash "$BUDGET" --check --repo-root "$r2" --agents-dir "$r2/agents" --budget-file "$r2/budget.json"

# ── Case 3: transitive teeth — under on the file alone, OVER once the imported
#            bubbles_shared module is counted → FAILS. The 1500-byte ceiling is
#            above the ~90-byte agent file but below the ~5 KB effective closure. ──
r3="$(mk_transitive_repo "$TMP_ROOT/r3")"
agent_only_bytes="$(wc -c < "$r3/agents/imp.agent.md" | tr -d ' ')"
printf '{"imp.agent.md": 1500}\n' > "$r3/budget.json"
assert_exit 1 "Case 3: transitive closure counted (agent-only=$agent_only_bytes < 1500 < closure) fails" \
  bash "$BUDGET" --check --repo-root "$r3" --agents-dir "$r3/agents" --budget-file "$r3/budget.json"
# And it PASSES when the ceiling covers the whole closure.
printf '{"imp.agent.md": 9999999}\n' > "$r3/budget.json"
assert_exit 0 "Case 3b: closure within a large ceiling passes" \
  bash "$BUDGET" --check --repo-root "$r3" --agents-dir "$r3/agents" --budget-file "$r3/budget.json"

# ── Case 4: an agent with NO recorded ceiling FAILS (new agent must get one) ──
r4="$(mk_plain_repo "$TMP_ROOT/r4")"
printf '{"someone-else.agent.md": 9999999}\n' > "$r4/budget.json"
assert_exit 1 "Case 4: missing ceiling for an agent fails --check" \
  bash "$BUDGET" --check --repo-root "$r4" --agents-dir "$r4/agents" --budget-file "$r4/budget.json"

# ── Case 5: missing budget file (agents present) FAILS ──
r5="$(mk_plain_repo "$TMP_ROOT/r5")"
assert_exit 1 "Case 5: missing budget file with agents present fails --check" \
  bash "$BUDGET" --check --repo-root "$r5" --agents-dir "$r5/agents" --budget-file "$r5/nope.json"

# ── Case 6: no agents/ directory SKIPS (exit 0) ──
r6="$TMP_ROOT/r6"; mkdir -p "$r6"
assert_exit 0 "Case 6: no agents directory skips" \
  bash "$BUDGET" --check --repo-root "$r6" --agents-dir "$r6/agents" --budget-file "$r6/budget.json"

# ── Case 7: --seed writes a green budget; --check then passes ──
r7="$(mk_plain_repo "$TMP_ROOT/r7")"
assert_exit 0 "Case 7a: --seed writes a budget" \
  bash "$BUDGET" --seed --repo-root "$r7" --agents-dir "$r7/agents" --budget-file "$r7/budget.json"
assert_exit 0 "Case 7b: --check passes on the just-seeded budget" \
  bash "$BUDGET" --check --repo-root "$r7" --agents-dir "$r7/agents" --budget-file "$r7/budget.json"

# ── Case 8: RATCHET DOWN — re-seeding a SHRUNKEN agent lowers its ceiling ──
r8="$(mk_plain_repo "$TMP_ROOT/r8")"
# Start large so there is room to shrink.
printf '# Big agent\n' > "$r8/agents/tiny.agent.md"
head -c 4000 /dev/zero | tr '\0' 'y' >> "$r8/agents/tiny.agent.md"
bash "$BUDGET" --seed --repo-root "$r8" --agents-dir "$r8/agents" --budget-file "$r8/budget.json" >/dev/null 2>&1
ceiling_before="$(budget_of "$r8/budget.json" tiny.agent.md)"
# Shrink the agent dramatically, then re-seed.
printf '# Small now\n' > "$r8/agents/tiny.agent.md"
bash "$BUDGET" --seed --repo-root "$r8" --agents-dir "$r8/agents" --budget-file "$r8/budget.json" >/dev/null 2>&1
ceiling_after="$(budget_of "$r8/budget.json" tiny.agent.md)"
assert_true "Case 8: re-seed ratchets ceiling DOWN ($ceiling_before -> $ceiling_after)" \
  test "$ceiling_after" -lt "$ceiling_before"

# ── Case 9: a --seed BREACH without --accept-growth is refused (exit 2), no raise ──
r9="$(mk_plain_repo "$TMP_ROOT/r9")"
printf '{"tiny.agent.md": 5}\n' > "$r9/budget.json"   # deliberately below current size
assert_exit 2 "Case 9a: --seed breach without --accept-growth is refused" \
  bash "$BUDGET" --seed --repo-root "$r9" --agents-dir "$r9/agents" --budget-file "$r9/budget.json"
assert_true "Case 9b: refused --seed left the ceiling UNRAISED (still 5)" \
  test "$(budget_of "$r9/budget.json" tiny.agent.md)" -eq 5
assert_exit 0 "Case 9c: --seed --accept-growth accepts the larger bundle" \
  bash "$BUDGET" --seed --accept-growth --repo-root "$r9" --agents-dir "$r9/agents" --budget-file "$r9/budget.json"

# ── Case 10: real-tree sync — the COMMITTED budget is in sync (guarded/skipped
#            when the framework source tree is not present, e.g. downstream). ──
REPO_ROOT_REAL="$(cd "$SCRIPT_DIR/../.." && pwd)"
REAL_BUDGET="$REPO_ROOT_REAL/bubbles/agent-bundle-budgets.json"
if [[ -d "$REPO_ROOT_REAL/agents" && -f "$REAL_BUDGET" ]]; then
  assert_exit 0 "Case 10: committed budget is in sync with the real agent tree" \
    bash "$BUDGET" --check --repo-root "$REPO_ROOT_REAL"
else
  echo "SKIP: Case 10 real-tree sync (agents/ or committed budget absent — non-source tree)"
fi

echo
echo "agent-bundle-size-budget selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
