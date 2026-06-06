#!/usr/bin/env bash
#
# Bubbles v6.0 / C1 — migrate-modes-v5-to-v6 selftest.
#
# Adversarial fixtures exercise the migrator and verify:
#   - aliases parse from a fresh fixture file
#   - --check dry-run reports rewrites without modifying anything
#   - --write applies the rewrites byte-correctly
#   - --write is idempotent (second run is no-op)
#   - default scope excludes framework internals (bubbles/scripts/*, agents/, skills/)
#   - real install.sh dry-run produces the expected v5 -> v6 rewrites
#
# Fixtures live under $HOME/.cache/bubbles-migrate-modes-selftest/.

set -euo pipefail

REPO_ROOT="${BUBBLES_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
SCRIPT="${REPO_ROOT}/bubbles/scripts/migrate-modes-v5-to-v6.sh"
REAL_ALIASES="${REPO_ROOT}/bubbles/workflows/aliases.yaml"

FIXTURE_ROOT="${HOME}/.cache/bubbles-migrate-modes-selftest"
rm -rf "$FIXTURE_ROOT"
mkdir -p "$FIXTURE_ROOT"

declare -i pass_count=0
declare -i fail_count=0

pass() { echo "PASS: $*"; pass_count=$((pass_count + 1)); }
bad()  { echo "FAIL: $*"; fail_count=$((fail_count + 1)); }

# ── Build a minimal v5->v6 alias fixture ─────────────────────────
ALIASES_FIXTURE="$FIXTURE_ROOT/aliases.yaml"
cat > "$ALIASES_FIXTURE" <<'EOF'
version: 1

v6Primitives:
- implement
- fix
- ship

v5Aliases:
  full-delivery:
    primitive: implement
    tags: { action: full-delivery, target: spec }
    description: Test fixture.

  bugfix-fastlane:
    primitive: fix
    tags: { target: bug, action: fastlane }
    description: Test fixture.

  release-train-promote:
    primitive: ship
    tags: { target: release-train, action: promote }
    description: Test fixture.

  framework-health:
    primitive: framework-health
    tags: { action: proposal-first }
    description: Test fixture (self-named primitive).
EOF

# ── Assertion 1: aliases parse and --check accepts empty corpus ──
fix1="$FIXTURE_ROOT/fixture-empty"
mkdir -p "$fix1"
cat > "$fix1/README.md" <<'EOF'
# Test README

This file contains no v5 mode references.
EOF
if out=$(BUBBLES_REPO_ROOT="$fix1" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" --paths "$fix1/README.md" 2>&1); then
  if echo "$out" | grep -qF 'PASS (no rewrites needed)'; then
    pass "empty corpus -> --check exits 0 with PASS message"
  else
    bad "empty corpus -> exit 0 but no PASS message: $out"
  fi
else
  bad "empty corpus -> --check exited non-zero unexpectedly: $out"
fi

# ── Assertion 2: file with a v5 reference -> --check exits 2 ─────
fix2="$FIXTURE_ROOT/fixture-needs-rewrite"
mkdir -p "$fix2"
cat > "$fix2/operator-doc.md" <<'EOF'
Run `/bubbles.workflow full-delivery` to start the pipeline.
Run `/bubbles.workflow bugfix-fastlane` when reporting a bug.
EOF
if BUBBLES_REPO_ROOT="$fix2" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" --paths "$fix2/operator-doc.md" >"$fix2/check.log" 2>&1; then
  bad "fixture with v5 references -> --check exited 0 (expected 2)"
else
  exit_code=$?
  if [[ $exit_code -eq 2 ]]; then
    pass "fixture with v5 references -> --check exits 2"
  else
    bad "fixture with v5 references -> --check exited $exit_code (expected 2)"
  fi
fi

# ── Assertion 3: --check leaves the file unchanged ───────────────
original_hash=$(sha256sum "$fix2/operator-doc.md" | awk '{print $1}')
BUBBLES_REPO_ROOT="$fix2" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" --paths "$fix2/operator-doc.md" >/dev/null 2>&1 || true
after_hash=$(sha256sum "$fix2/operator-doc.md" | awk '{print $1}')
if [[ "$original_hash" == "$after_hash" ]]; then
  pass "--check does not modify the file"
else
  bad "--check modified the file (original $original_hash, after $after_hash)"
fi

# ── Assertion 4: --write rewrites the file ───────────────────────
if BUBBLES_REPO_ROOT="$fix2" bash "$SCRIPT" --write --aliases-file "$ALIASES_FIXTURE" --paths "$fix2/operator-doc.md" >/dev/null 2>&1; then
  if grep -qF '/bubbles.workflow implement action:full-delivery target:spec' "$fix2/operator-doc.md" \
    && grep -qF '/bubbles.workflow fix target:bug action:fastlane' "$fix2/operator-doc.md"; then
    pass "--write rewrites full-delivery and bugfix-fastlane to v6 form"
  else
    bad "--write did not produce expected v6 forms; current file:"
    cat "$fix2/operator-doc.md"
  fi
else
  bad "--write exited non-zero"
fi

# ── Assertion 5: --write is idempotent (second run is no-op) ─────
if BUBBLES_REPO_ROOT="$fix2" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" --paths "$fix2/operator-doc.md" >/dev/null 2>&1; then
  pass "after --write, --check on the same file exits 0 (idempotent)"
else
  bad "after --write, --check still reports pending rewrites (NOT idempotent)"
fi

# ── Assertion 5b: self-named primitive (framework-health) is idempotent ──
# Adversarial regression guard. The v6 form begins with the v5 token itself
# ("framework-health action:proposal-first"), so a naive rewrite re-matches
# "workflow framework-health" on the second pass and double-applies the tail.
# The bare form must rewrite exactly ONCE and then be a no-op.
fix5b="$FIXTURE_ROOT/fixture-self-named"
mkdir -p "$fix5b"
cat > "$fix5b/recipe.md" <<'EOF'
Run `/bubbles.workflow framework-health` to self-observe the framework.
EOF
BUBBLES_REPO_ROOT="$fix5b" bash "$SCRIPT" --write --aliases-file "$ALIASES_FIXTURE" --paths "$fix5b/recipe.md" >/dev/null 2>&1 || true
if grep -qF '/bubbles.workflow framework-health action:proposal-first' "$fix5b/recipe.md" \
  && [[ "$(grep -c 'action:proposal-first' "$fix5b/recipe.md")" -eq 1 ]]; then
  pass "self-named primitive rewrites once to 'framework-health action:proposal-first'"
else
  bad "self-named primitive rewrite wrong (expected exactly one tail). Current file:"
  cat "$fix5b/recipe.md"
fi
if BUBBLES_REPO_ROOT="$fix5b" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" --paths "$fix5b/recipe.md" >/dev/null 2>&1; then
  pass "self-named primitive --check is idempotent (exit 0, no double-apply)"
else
  bad "self-named primitive NOT idempotent — --check still reports pending rewrites"
fi

# ── Assertion 6: default scope excludes framework internals ──────
fix6="$FIXTURE_ROOT/fixture-default-scope"
mkdir -p "$fix6/bubbles/scripts" "$fix6/agents" "$fix6/skills/some-skill" "$fix6/docs"
cat > "$fix6/bubbles/scripts/internal.sh" <<'EOF'
#!/usr/bin/env bash
echo "Test fixture: /bubbles.workflow full-delivery"
EOF
cat > "$fix6/agents/some.agent.md" <<'EOF'
Mention /bubbles.workflow full-delivery here.
EOF
cat > "$fix6/skills/some-skill/SKILL.md" <<'EOF'
Mention /bubbles.workflow full-delivery here.
EOF
cat > "$fix6/docs/operator-guide.md" <<'EOF'
Operators run /bubbles.workflow full-delivery to ship.
EOF
# Add a minimal .git to look like a repo root
mkdir -p "$fix6/.git"
echo 'ref: refs/heads/main' > "$fix6/.git/HEAD"

if out=$(BUBBLES_REPO_ROOT="$fix6" bash "$SCRIPT" --check --aliases-file "$ALIASES_FIXTURE" 2>&1); then
  bad "default-scope dry-run exited 0 (expected 2 — operator-guide.md needs rewriting)"
else
  if echo "$out" | grep -qF 'docs/operator-guide.md' && ! echo "$out" | grep -qF 'bubbles/scripts/internal.sh' && ! echo "$out" | grep -qF 'agents/some.agent.md' && ! echo "$out" | grep -qF 'skills/some-skill/SKILL.md'; then
    pass "default scope picks up docs/operator-guide.md but excludes bubbles/scripts/, agents/, skills/"
  else
    bad "default scope did not behave as expected. Output:"
    echo "$out"
  fi
fi

# ── Assertion 7: real repo operator surfaces are fully migrated to v6 (v7) ──
# v7.0 removed bare v5 mode names as operator input, so every operator-facing
# surface scanned by the default scope (README, docs/guides, docs/recipes,
# install.sh, .specify) MUST be free of bare v5 leading-token forms. A clean
# real-repo --check is the v7 invariant; it also guards against regressions that
# reintroduce a bare /bubbles.workflow <v5> form into an operator surface.
if real_out=$(bash "$SCRIPT" --check 2>&1); then
  pass "real-repo --check exits 0 — operator surfaces carry no bare v5 forms (v7 invariant)"
else
  bad "real-repo --check exited non-zero — an operator surface still has a bare v5 form. Output:"
  echo "$real_out"
fi

# ── Assertion 8: unknown argument -> exit 1 ──────────────────────
if bash "$SCRIPT" --bogus-flag 2>/dev/null; then
  bad "unknown flag did not exit non-zero"
else
  exit_code=$?
  if [[ $exit_code -eq 1 ]]; then
    pass "unknown flag exits 1"
  else
    bad "unknown flag exited $exit_code (expected 1)"
  fi
fi

# ── Assertion 9: missing aliases file -> exit 1 ─────────────────
if bash "$SCRIPT" --check --aliases-file "/nonexistent-aliases.yaml" 2>/dev/null; then
  bad "missing aliases file did not exit non-zero"
else
  exit_code=$?
  if [[ $exit_code -eq 1 ]]; then
    pass "missing aliases file exits 1"
  else
    bad "missing aliases file exited $exit_code (expected 1)"
  fi
fi

echo
echo "migrate-modes-v5-to-v6-selftest: $pass_count pass, $fail_count fail"

if [[ $fail_count -gt 0 ]]; then
  exit 1
fi
exit 0
