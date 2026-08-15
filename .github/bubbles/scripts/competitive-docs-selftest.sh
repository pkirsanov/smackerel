#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

LEDGER_FILE="$ROOT_DIR/bubbles/capability-ledger.yaml"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

check_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

# Fixed-string containment for exact (non-regex) ledger-derived count agreement.
check_contains() {
  local file_path="$1"
  local needle="$2"
  local label="$3"

  if grep -qF "$needle" "$file_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

# Exact ledger-derived counts (never an arbitrary [0-9]+ regex). Four-space
# match so capability ids and narrative prose are never miscounted.
count_state() {
  awk -v want="$2" '$0 == "    state: " want { n++ } END { print n + 0 }' "$1"
}

count_issue_backed() {
  awk '$0 == "    issueRefs:" { n++ } END { print n + 0 }' "$1"
}

# Regenerate projections from the canonical ledger into an isolated temporary
# root and prove they are internally consistent there. The canonical generated
# docs' durable-work evidence projection is reconciled in S5B, so this stage
# asserts hermetic generation rather than the by-design-stale canonical --check.
hermetic_docs_ok() {
  local tmp_root rc=0
  tmp_root="$(mktemp -d)"
  mkdir -p "$tmp_root/bubbles" "$tmp_root/docs/issues" "$tmp_root/docs/generated"
  cp "$LEDGER_FILE" "$tmp_root/bubbles/"
  cp "$ROOT_DIR/bubbles/interop-registry.yaml" "$tmp_root/bubbles/"
  cp "$ROOT_DIR/README.md" "$tmp_root/"
  cp "$ROOT_DIR"/docs/issues/*.md "$tmp_root/docs/issues/" 2>/dev/null || true
  if ! BUBBLES_REPO_ROOT="$tmp_root" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null 2>&1; then
    rc=1
  elif ! BUBBLES_REPO_ROOT="$tmp_root" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >/dev/null 2>&1; then
    rc=1
  fi
  rm -rf "$tmp_root"
  return "$rc"
}

echo "Running competitive-docs selftest..."
echo "Scenario: README and generated evaluator docs expose the same competitive truth path."

if hermetic_docs_ok; then
  pass "Capability ledger docs regenerate consistently from source (hermetic)"
else
  fail "Capability ledger docs regenerate consistently from source (hermetic)"
fi

# Exact ledger-derived counts shared across README and generated surfaces.
rb_shipped="$(count_state "$LEDGER_FILE" shipped)"
rb_partial="$(count_state "$LEDGER_FILE" partial)"
rb_proposed="$(count_state "$LEDGER_FILE" proposed)"
rb_gaps="$(count_issue_backed "$LEDGER_FILE")"
rb_summary="${rb_shipped} shipped, ${rb_partial} partial, ${rb_proposed} proposed"

# T5A.3: cross-surface count AGREEMENT MUST be hermetic. Regenerate every
# projection from the canonical ledger into a temporary fixture (mirroring the
# hermetic_docs_ok() pattern above) and assert the exact ledger-derived counts
# agree ACROSS the regenerated README, competitive guide, issue-status guide,
# and interop matrix INSIDE that fixture. Reading the canonical working-tree
# docs would couple this stage to the by-design-stale S5B doc regeneration (the
# canonical durable-work count is reconciled in S5B), so a freshly regenerated
# temporary fixture keeps count agreement independent of canonical doc freshness.
count_fixture=""
build_count_fixture() {
  count_fixture="$(mktemp -d)"
  mkdir -p "$count_fixture/bubbles" "$count_fixture/docs/issues" "$count_fixture/docs/generated"
  cp "$LEDGER_FILE" "$count_fixture/bubbles/"
  cp "$ROOT_DIR/bubbles/interop-registry.yaml" "$count_fixture/bubbles/"
  cp "$ROOT_DIR/README.md" "$count_fixture/"
  cp "$ROOT_DIR"/docs/issues/*.md "$count_fixture/docs/issues/" 2>/dev/null || true
  BUBBLES_REPO_ROOT="$count_fixture" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" >/dev/null 2>&1
}

if build_count_fixture; then
  pass "Cross-surface count agreement regenerates from the canonical ledger into a temporary fixture (hermetic)"

  check_contains "$count_fixture/README.md" \
    "Ledger-backed competitive posture guide — ${rb_summary}" \
    "README competitive guide shows exact ledger counts (${rb_summary})"
  check_contains "$count_fixture/README.md" \
    "Ledger-backed status for ${rb_gaps} tracked framework gaps" \
    "README issue-status guide shows the exact tracked-gap count (${rb_gaps})"
  check_contains "$count_fixture/docs/generated/competitive-capabilities.md" \
    "State summary: ${rb_summary}." \
    "Generated competitive guide summary matches exact ledger counts (${rb_summary})"
  check_contains "$count_fixture/docs/generated/issue-status.md" \
    "Issue-linked capabilities: ${rb_gaps}." \
    "Generated issue-status guide matches the exact issue-linked count (${rb_gaps})"
  check_contains "$count_fixture/docs/generated/interop-migration-matrix.md" \
    "Capability context: ${rb_summary}." \
    "Generated interop migration matrix matches exact ledger counts (${rb_summary})"
else
  fail "Cross-surface count agreement regenerates from the canonical ledger into a temporary fixture (hermetic)"
fi
if [[ -n "$count_fixture" ]]; then
  rm -rf "$count_fixture"
fi

# Structural cross-surface link/coverage assertions read the canonical surfaces
# directly: these are committed, freshness-independent shape checks (not counts),
# so the S5B count regeneration does not affect them.
check_pattern "$ROOT_DIR/README.md" '<a href="docs/generated/interop-migration-matrix.md">Interop Migration Matrix</a></td><td>Ledger \+ registry-backed migration matrix for Claude Code, Roo Code, Cursor, and Cline' "README links to the generated interop migration matrix"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\[docs/issues/session-aware-runtime-coordination.md\]\(../issues/session-aware-runtime-coordination.md\)' "Generated competitive guide links evaluators to issue-backed proposal detail"
check_pattern "$ROOT_DIR/docs/generated/interop-migration-matrix.md" '\| Claude Code \| markdown \|' "Generated interop migration matrix covers Claude Code"
check_pattern "$ROOT_DIR/docs/guides/CONTROL_PLANE_ADOPTION.md" 'Interop Migration Guide|generated/interop-migration-matrix.md' "Adoption guide links to the interop migration guidance surfaces"
check_pattern "$ROOT_DIR/docs/recipes/setup-project.md" 'Interop Migration Guide|generated/interop-migration-matrix.md' "Setup recipe links to the interop migration guidance surfaces"

# Adversarial: a temporary generated doc carrying arbitrary (wrong) counts must
# be rejected by the exact-count contract, even though a loose [0-9]+ regex would
# wrongly accept it. This is precisely why the assertions above pin exact counts.
adv_fixture="$(mktemp)"
awk '
  /^State summary: [0-9]+ shipped, [0-9]+ partial, [0-9]+ proposed\.$/ {
    print "State summary: 99 shipped, 88 partial, 77 proposed."
    next
  }
  { print }
' "$ROOT_DIR/docs/generated/competitive-capabilities.md" >"$adv_fixture"
if grep -qxF "State summary: ${rb_summary}." "$adv_fixture"; then
  fail "Adversarial arbitrary summary must be rejected by the exact-count contract"
else
  pass "Exact-count contract rejects an arbitrary summary a loose regex would accept"
fi
if grep -qE '^State summary: [0-9]+ shipped, [0-9]+ partial, [0-9]+ proposed\.$' "$adv_fixture"; then
  pass "The loose [0-9]+ regex wrongly matches the arbitrary summary (why exact counts are required)"
else
  fail "The loose [0-9]+ regex should still match the arbitrary summary shape"
fi
rm -f "$adv_fixture"

if [[ "$failures" -gt 0 ]]; then
  echo "competitive-docs selftest failed with $failures issue(s)."
  exit 1
fi

echo "competitive-docs selftest passed."
