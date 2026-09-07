#!/usr/bin/env bash
# governance-index-lint-selftest.sh
#
# Hermetic selftest for governance-index-lint.sh.
#
# Stages a synthetic Bubbles-style repo under a temp directory with:
#   - one indexed governance doc (linked from agent-common.md)  -> expects PASS
#   - one orphan governance doc                                 -> expects FAIL
#
# Asserts:
#   - PASS fixture exits 0 with "PASS — zero orphan docs"
#   - FAIL fixture exits 1, prints "ORPHAN_GOVERNANCE_DOC:", and lists
#     the orphan basename
#   - --allow regex can rescue an orphan and convert FAIL -> PASS
#
# Cleans up on exit.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/governance-index-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest governance-index-lint] FAIL: target script missing at $LINT" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# --- PASS fixture ---------------------------------------------------------

pass_root="$TMPDIR/repo-pass"
mkdir -p "$pass_root/agents/bubbles_shared" "$pass_root/instructions" \
  "$pass_root/skills/example" "$pass_root/docs/recipes"

cat > "$pass_root/README.md" <<'EOF'
# Fake Bubbles repo (PASS fixture)
EOF

cat > "$pass_root/agents/bubbles_shared/agent-common.md" <<'EOF'
# agent-common (fake)

See [used-doc.md](used-doc.md) for the only governance doc in this fixture.
EOF

cat > "$pass_root/agents/bubbles_shared/used-doc.md" <<'EOF'
# used-doc

This doc IS referenced from agent-common.md.
EOF

set +e
pass_log="$TMPDIR/pass.log"
bash "$LINT" --repo-root "$pass_root" >"$pass_log" 2>&1
pass_rc=$?
set -e

if [[ "$pass_rc" -eq 0 ]]; then
  pass "PASS fixture exits 0"
else
  fail "PASS fixture expected exit 0, got $pass_rc"
  sed -n '1,80p' "$pass_log"
fi

if grep -Fq "PASS — zero orphan docs" "$pass_log"; then
  pass "PASS fixture reports zero orphans"
else
  fail "PASS fixture missing 'PASS — zero orphan docs'"
  sed -n '1,80p' "$pass_log"
fi

# --- FAIL fixture ---------------------------------------------------------

fail_root="$TMPDIR/repo-fail"
mkdir -p "$fail_root/agents/bubbles_shared" "$fail_root/instructions" \
  "$fail_root/skills/example" "$fail_root/docs/recipes"

cat > "$fail_root/README.md" <<'EOF'
# Fake Bubbles repo (FAIL fixture)
EOF

cat > "$fail_root/agents/bubbles_shared/agent-common.md" <<'EOF'
# agent-common (fake)

References [used-doc.md](used-doc.md).
EOF

cat > "$fail_root/agents/bubbles_shared/used-doc.md" <<'EOF'
# used-doc
EOF

cat > "$fail_root/agents/bubbles_shared/orphan-doc.md" <<'EOF'
# orphan-doc — NOT linked from any index
EOF

set +e
fail_log="$TMPDIR/fail.log"
bash "$LINT" --repo-root "$fail_root" >"$fail_log" 2>&1
fail_rc=$?
set -e

if [[ "$fail_rc" -eq 1 ]]; then
  pass "FAIL fixture exits 1"
else
  fail "FAIL fixture expected exit 1, got $fail_rc"
  sed -n '1,80p' "$fail_log"
fi

if grep -Fq "ORPHAN_GOVERNANCE_DOC:" "$fail_log"; then
  pass "FAIL fixture prints ORPHAN_GOVERNANCE_DOC marker"
else
  fail "FAIL fixture missing ORPHAN_GOVERNANCE_DOC marker"
  sed -n '1,80p' "$fail_log"
fi

if grep -Fq "orphan-doc.md" "$fail_log"; then
  pass "FAIL fixture lists the orphan basename"
else
  fail "FAIL fixture did not list orphan-doc.md"
  sed -n '1,80p' "$fail_log"
fi

# --- --allow rescue test --------------------------------------------------

set +e
allow_log="$TMPDIR/allow.log"
bash "$LINT" --repo-root "$fail_root" \
  --allow 'agents/bubbles_shared/orphan-doc\.md' >"$allow_log" 2>&1
allow_rc=$?
set -e

if [[ "$allow_rc" -eq 0 ]]; then
  pass "--allow rescues an orphan (exit 0)"
else
  fail "--allow expected exit 0, got $allow_rc"
  sed -n '1,80p' "$allow_log"
fi

# --- copilot-instructions.md is an index in the downstream layout ---------

# A downstream install roots at .github/, which has no README.md. The project's
# own governance docs are indexed from copilot-instructions.md, so a lint that
# does not consult it reports every one of them as an orphan.

ci_root="$TMPDIR/repo-copilot"
mkdir -p "$ci_root/instructions"

cat > "$ci_root/copilot-instructions.md" <<'EOF'
# Project governance root (fake downstream .github/)

Terminal rules: [terminal-discipline.instructions.md](instructions/terminal-discipline.instructions.md)
EOF

cat > "$ci_root/instructions/terminal-discipline.instructions.md" <<'EOF'
# terminal-discipline — indexed ONLY from copilot-instructions.md
EOF

set +e
ci_log="$TMPDIR/copilot.log"
bash "$LINT" --repo-root "$ci_root" >"$ci_log" 2>&1
ci_rc=$?
set -e

if [[ "$ci_rc" -eq 0 ]]; then
  pass "copilot-instructions.md indexes a doc in a README-less downstream layout"
else
  fail "copilot-instructions.md index expected exit 0, got $ci_rc"
  sed -n '1,80p' "$ci_log"
fi

# The new index must be a real match, not a blanket pass: drop the reference and
# the same fixture has to fail, otherwise adding it would have disabled the lint.

cat > "$ci_root/copilot-instructions.md" <<'EOF'
# Project governance root that references nothing
EOF

set +e
ci_neg_log="$TMPDIR/copilot-neg.log"
bash "$LINT" --repo-root "$ci_root" >"$ci_neg_log" 2>&1
ci_neg_rc=$?
set -e

if [[ "$ci_neg_rc" -eq 1 ]] && grep -Fq "terminal-discipline.instructions.md" "$ci_neg_log"; then
  pass "an unreferenced doc still FAILS once copilot-instructions.md stops naming it"
else
  fail "unreferenced doc expected exit 1 naming the doc, got $ci_neg_rc"
  sed -n '1,80p' "$ci_neg_log"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest governance-index-lint] OK — all assertions passed"
  exit 0
else
  echo "[selftest governance-index-lint] FAIL — $failures assertion(s) failed"
  exit 1
fi
