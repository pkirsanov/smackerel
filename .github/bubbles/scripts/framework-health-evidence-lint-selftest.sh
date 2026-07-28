#!/usr/bin/env bash
# framework-health-evidence-lint-selftest.sh — hermetic selftest.
#
# Builds throwaway repos under mktemp and asserts the lint's verdict for each.
# Every RED fixture must FAIL (exit 1); every GREEN fixture must PASS (exit 0).
# A check that cannot be shown failing is not enforcement, so each check gets
# its own adversarial fixture.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/framework-health-evidence-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "framework-health-evidence-lint-selftest: lint not found: $LINT" >&2
  exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

pass_count=0
fail_count=0

# assert <name> <expected-exit> <repo>
assert() {
  local name="$1" expected="$2" repo="$3" actual=0 output=""
  output="$(bash "$LINT" --repo-root "$repo" 2>&1)" || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS  $name (exit $actual)"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL  $name — expected exit $expected, got $actual"
    echo "      output: $output"
    fail_count=$((fail_count + 1))
  fi
}

# --- fixture builders -------------------------------------------------------

index_body() {
  cat <<'EOF'
# Framework Improvement Proposals (IMP) — Index

## Status legend

| Status | Meaning |
|---|---|
| `PROPOSED` | Authored, awaiting owner review. |
| `APPLIED` | All scopes landed. |

## Proposals

| ID | Title | Status |
|---|---|---|
EOF
}

good_imp_body() {
  cat <<'EOF'
# IMP-001 — Example

**Status:** PROPOSED
**Motivation:** audit of the tree

## Provenance

- Input sources: `.specify/runtime/framework-events.jsonl`
EOF
}

# make_repo <dir> — bare skeleton with a conforming proposal + index
make_repo() {
  local d="$1"
  mkdir -p "$d/improvements"
  index_body >"$d/improvements/INDEX.md"
  echo "| IMP-001-example.md | Example | PROPOSED |" >>"$d/improvements/INDEX.md"
  good_imp_body >"$d/improvements/IMP-001-example.md"
}

# --- GREEN: no improvements dir at all --------------------------------------
green_none="$WORK/green-none"
mkdir -p "$green_none"
assert "green: no improvements dir is not a violation" 0 "$green_none"

# --- GREEN: conforming proposal ---------------------------------------------
green_ok="$WORK/green-ok"
make_repo "$green_ok"
assert "green: conforming proposal passes" 0 "$green_ok"

# --- RED: improvements/ exists but INDEX.md missing -------------------------
red_no_index="$WORK/red-no-index"
make_repo "$red_no_index"
rm -f "$red_no_index/improvements/INDEX.md"
assert "red: missing INDEX.md is caught" 1 "$red_no_index"

# --- RED: proposal without a Status line ------------------------------------
red_no_status="$WORK/red-no-status"
make_repo "$red_no_status"
grep -v '^\*\*Status:\*\*' "$red_no_status/improvements/IMP-001-example.md" >"$red_no_status/tmp"
mv "$red_no_status/tmp" "$red_no_status/improvements/IMP-001-example.md"
assert "red: missing Status is caught" 1 "$red_no_status"

# --- RED: Status value outside the index legend -----------------------------
red_bad_status="$WORK/red-bad-status"
make_repo "$red_bad_status"
sed 's/^\*\*Status:\*\* PROPOSED/**Status:** TOTALLY DONE/' \
  "$red_bad_status/improvements/IMP-001-example.md" >"$red_bad_status/tmp"
mv "$red_bad_status/tmp" "$red_bad_status/improvements/IMP-001-example.md"
assert "red: unknown Status value is caught" 1 "$red_bad_status"

# --- RED: no evidence citation ----------------------------------------------
red_no_src="$WORK/red-no-src"
make_repo "$red_no_src"
cat <<'EOF' >"$red_no_src/improvements/IMP-001-example.md"
# IMP-001 — Example

**Status:** PROPOSED
**Motivation:** it felt wrong

## Proposal

- do a thing
EOF
assert "red: uncited proposal is caught" 1 "$red_no_src"

# --- RED: proposal absent from the index ------------------------------------
red_no_row="$WORK/red-no-row"
make_repo "$red_no_row"
index_body >"$red_no_row/improvements/INDEX.md"
assert "red: proposal with no index row is caught" 1 "$red_no_row"

# --- RED: generator writing outside improvements/ ---------------------------
red_generator="$WORK/red-generator"
make_repo "$red_generator"
mkdir -p "$red_generator/bubbles/scripts"
cat <<'EOF' >"$red_generator/bubbles/scripts/retro-framework-health.sh"
#!/usr/bin/env bash
echo "proposal" > "$REPO_ROOT/improvements/IMP-002.md"
echo "sneaky" >> "$REPO_ROOT/agents/bubbles.retro.agent.md"
EOF
assert "red: generator writing into agents/ is caught" 1 "$red_generator"

# --- GREEN: generator writing only under improvements/ ----------------------
green_generator="$WORK/green-generator"
make_repo "$green_generator"
mkdir -p "$green_generator/bubbles/scripts"
cat <<'EOF' >"$green_generator/bubbles/scripts/retro-framework-health.sh"
#!/usr/bin/env bash
echo "proposal" > "$REPO_ROOT/improvements/IMP-002.md"
EOF
assert "green: contained generator passes" 0 "$green_generator"

# --- RED: untraceable co-mutation in the adding commit ----------------------
#
# Requires a real git repo so the lint can resolve the adding commit.
red_git="$WORK/red-git"
make_repo "$red_git"
mkdir -p "$red_git/agents"
(
  cd "$red_git"
  git init -q .
  git config user.email selftest@example.com
  git config user.name selftest
  echo "governed" >agents/bubbles.retro.agent.md
  git add -A
  git commit -q -m "land some framework change and a proposal"
) >/dev/null 2>&1
assert "red: untraceable co-mutation is caught" 1 "$red_git"

# --- GREEN: traceable co-mutation naming the IMP ----------------------------
green_git="$WORK/green-git"
make_repo "$green_git"
mkdir -p "$green_git/agents"
(
  cd "$green_git"
  git init -q .
  git config user.email selftest@example.com
  git config user.name selftest
  echo "governed" >agents/bubbles.retro.agent.md
  git add -A
  git commit -q -m "feat: land IMP-001 SCOPE-1 alongside its contract"
) >/dev/null 2>&1
assert "green: traceable co-mutation passes" 0 "$green_git"

echo ""
echo "framework-health-evidence-lint selftest: $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "All framework-health-evidence-lint selftests passed."
exit 0
