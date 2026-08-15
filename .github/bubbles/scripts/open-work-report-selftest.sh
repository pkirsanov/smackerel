#!/usr/bin/env bash
# open-work-report-selftest.sh (IMP-033 / SCOPE-3 — gaps WIP-1, WIP-2)
# ---------------------------------------------------------------------------
# Hermetic coverage for the open-work register and its renderer.
#
# The load-bearing case is a1/a2: change a `state.json` status and observe the
# rendered row change. That is what proves the spec rows are DERIVED through
# `work-tracker-project.sh` rather than authored — the property that keeps the
# register from decaying into the stale status mirror IMP-032 removed. A test
# that only checked "a row appears" would pass just as happily against a
# hand-written table.
#
# Every fixture is a synthesized repo under a temp root. Nothing here reads or
# writes the developer's own repository.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_SH="$SCRIPT_DIR/open-work-report.sh"

PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); echo "PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "FAIL: $1"; }

TMP_ROOT="$(mktemp -d 2>/dev/null || mktemp -d -t openwork)"
TMP_ROOT="$(cd "$TMP_ROOT" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT

setup() {
  if ! "$@" >/dev/null 2>&1; then
    echo "SETUP FAILED: $*" >&2
    exit 1
  fi
}

if ! command -v jq >/dev/null 2>&1; then
  echo "open-work-report-selftest: jq is not installed; derived-row cases cannot run." >&2
  echo "open-work-report-selftest: SKIPPED (dependency absent, not a failure of the code under test)." >&2
  exit 0
fi

echo "Running open-work report selftest (IMP-033 SCOPE-3)..."

# Minimal adopting repo: a memory dir (so root resolution finds it), a register,
# and a specs tree. Deliberately NOT a git repo unless a case needs one, so the
# ignore check is exercised only where it is the subject.
mk_repo() {
  local d="$1"
  mkdir -p "$d/.specify/memory" "$d/specs"
  cat > "$d/.specify/memory/open-work.md" <<'REGEOF'
# Open Work Register

## Residue

| id | title | kind | ref | state | next-owner | next-action | opened | last-seen |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
REGEOF
}

mk_spec() {
  local d="$1" name="$2" status="$3" mode="$4"
  mkdir -p "$d/specs/$name"
  cat > "$d/specs/$name/state.json" <<EOF
{
  "feature": "$name",
  "title": "A synthesized feature",
  "status": "$status",
  "workflowMode": "$mode",
  "createdAt": "2026-01-15T00:00:00Z",
  "nextRequiredOwner": "bubbles.implement",
  "scopes": []
}
EOF
}

report() { bash "$REPORT_SH" --repo-root "$1" "${@:2}" 2>/dev/null; }

# Capture BEFORE matching. `report ... | grep -q` looks equivalent but is not:
# `grep -q` exits on the first match and closes the pipe, the producer takes
# SIGPIPE, and `set -o pipefail` then reports the whole pipeline as failed even
# though the text was found. That turns a passing assertion into a false
# failure, and — worse — a negative assertion into a false pass.
report_has() {
  local out
  out="$(bash "$REPORT_SH" --repo-root "$1" 2>/dev/null)"
  printf '%s' "$out" | grep -q "$2"
}

lint_rc() {
  bash "$REPORT_SH" --repo-root "$1" --lint >/dev/null 2>&1
  return $?
}

row_state_for() {
  # Ask the JSON projection for one item's state, so the assertion does not
  # depend on the text renderer's column widths.
  report "$1" --format json | jq -r --arg id "$2" '(.items[]? | select(.id == $id) | .state) // "ABSENT"'
}

# --- (a) spec rows are derived, and they track state.json ---------------------
A="$TMP_ROOT/a"
mk_repo "$A"
mk_spec "$A" "101-derived-feature" "in_progress" "full-delivery"

got="$(row_state_for "$A" "101-derived-feature")"
if [[ "$got" == "in_progress" ]]; then
  pass "a1 a non-terminal spec is derived into the register view (state=in_progress)"
else
  fail "a1 a non-terminal spec is derived into the register view — expected in_progress, got '$got'"
fi

# Mutate ONLY state.json. Nothing authored changes. If the row still says
# in_progress, the value was not derived.
sed -i.bak 's/"status": "in_progress"/"status": "blocked"/' "$A/specs/101-derived-feature/state.json"
rm -f "$A/specs/101-derived-feature/state.json.bak"
got="$(row_state_for "$A" "101-derived-feature")"
if [[ "$got" == "blocked" ]]; then
  pass "a2 changing state.json changes the rendered row (proves derivation, not authorship)"
else
  fail "a2 changing state.json changes the rendered row — expected blocked, got '$got'"
fi

# Terminal-for-mode work is not open work.
sed -i.bak 's/"status": "blocked"/"status": "done"/' "$A/specs/101-derived-feature/state.json"
rm -f "$A/specs/101-derived-feature/state.json.bak"
got="$(row_state_for "$A" "101-derived-feature")"
if [[ "$got" == "ABSENT" ]]; then
  pass "a3 a done spec drops out of the register view"
else
  fail "a3 a done spec drops out of the register view — expected ABSENT, got '$got'"
fi

# A mode ceiling other than `done` is terminal too, and must not be reported as
# backlog. This is the case a hardcoded status list gets wrong.
mk_spec "$A" "102-docs-only" "docs_updated" "docs-only"
got="$(row_state_for "$A" "102-docs-only")"
if [[ "$got" == "ABSENT" ]]; then
  pass "a4 a mode ceiling status (docs_updated for docs-only) is treated as closed, not backlog"
else
  fail "a4 a mode ceiling status is treated as closed — expected ABSENT, got '$got'"
fi

# --- (b) bug rows are classified by location, not by hand ---------------------
B="$TMP_ROOT/b"
mk_repo "$B"
mkdir -p "$B/specs/200-host/bugs/BUG-001-broken-thing"
cat > "$B/specs/200-host/bugs/BUG-001-broken-thing/state.json" <<'EOF'
{
  "feature": "BUG-001-broken-thing",
  "title": "A synthesized bug",
  "status": "in_progress",
  "workflowMode": "bugfix-fastlane",
  "scopes": []
}
EOF
got="$(report "$B" --format json | jq -r '(.items[]? | select(.id == "BUG-001-broken-thing") | .kind) // "ABSENT"')"
if [[ "$got" == "bug" ]]; then
  pass "b1 a state.json under bugs/ is classified as kind=bug"
else
  fail "b1 a state.json under bugs/ is classified as kind=bug — got '$got'"
fi

# --- (c) improvement rows come from INDEX.md, and only while PROPOSED ---------
C="$TMP_ROOT/c"
mk_repo "$C"
mkdir -p "$C/improvements"
cat > "$C/improvements/INDEX.md" <<'EOF'
# Improvements

| ID | Title | Status | Source | Gaps | Date |
| --- | --- | --- | --- | --- | --- |
| IMP-900 | An open proposal | PROPOSED | audit | X-1 | 2026-01-01 |
| IMP-901 | A landed proposal | APPLIED 2026-01-02 | audit | X-2 | 2026-01-02 |
EOF
json="$(report "$C" --format json)"
if [[ "$(printf '%s' "$json" | jq -r '(.items[]? | select(.id == "IMP-900") | .kind) // "ABSENT"')" == "imp" ]]; then
  pass "c1 a PROPOSED improvement is open work"
else
  fail "c1 a PROPOSED improvement is open work"
fi
if [[ "$(printf '%s' "$json" | jq -r '(.items[]? | select(.id == "IMP-901") | .kind) // "ABSENT"')" == "ABSENT" ]]; then
  pass "c2 an APPLIED improvement is not open work"
else
  fail "c2 an APPLIED improvement is not open work"
fi

# --- (d) the residue lint ------------------------------------------------------
D="$TMP_ROOT/d"
mk_repo "$D"
cat >> "$D/.specify/memory/open-work.md" <<'EOF'
| RES-1 | dashboard retry path never wired | residue | dashboard/src/api.ts | open | bubbles.implement | wire the retry path to the shared client | 2026-02-01 | 2026-02-01 |
EOF
if lint_rc "$D"; then
  pass "d1 a complete residue row passes the lint"
else
  fail "d1 a complete residue row passes the lint"
fi

D2="$TMP_ROOT/d2"
mk_repo "$D2"
cat >> "$D2/.specify/memory/open-work.md" <<'EOF'
| RES-2 | something noticed | residue | src/x.rs | open | | finish the thing | 2026-02-01 | 2026-02-01 |
EOF
lint_rc "$D2"
rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "d2 a residue row with no next-owner fails the lint (exit 1)"
else
  fail "d2 a residue row with no next-owner fails the lint — got exit $rc"
fi
if report_has "$D2" "has no next-owner"; then
  pass "d3 the lint names the missing field rather than reporting a generic defect"
else
  fail "d3 the lint names the missing field"
fi

D3="$TMP_ROOT/d3"
mk_repo "$D3"
cat >> "$D3/.specify/memory/open-work.md" <<'EOF'
| RES-3 | placeholder action | residue | src/x.rs | open | bubbles.implement | TBD | 2026-02-01 | 2026-02-01 |
EOF
lint_rc "$D3"
rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "d4 a placeholder next-action ('TBD') fails the lint"
else
  fail "d4 a placeholder next-action ('TBD') fails the lint — got exit $rc"
fi

D4="$TMP_ROOT/d4"
mk_repo "$D4"
cat >> "$D4/.specify/memory/open-work.md" <<'EOF'
| 300-some-feature | authored spec status | spec | specs/300-some-feature | in_progress | bubbles.implement | keep going | 2026-02-01 | 2026-02-01 |
EOF
if report_has "$D4" "only 'residue' rows may be authored"; then
  pass "d5 authoring a spec row is a defect (no second source of truth for status)"
else
  fail "d5 authoring a spec row is a defect"
fi

D5="$TMP_ROOT/d5"
mk_repo "$D5"
cat >> "$D5/.specify/memory/open-work.md" <<'EOF'
| RES-9 | first | residue | a | open | bubbles.implement | do a | 2026-02-01 | 2026-02-01 |
| RES-9 | second | residue | b | open | bubbles.implement | do b | 2026-02-01 | 2026-02-01 |
EOF
if report_has "$D5" "declared more than once"; then
  pass "d6 a duplicate residue id is a defect"
else
  fail "d6 a duplicate residue id is a defect"
fi

# An unescaped `|` inside a cell is invisible to every other check here: the row
# still parses, it just parses into the wrong columns. Only the column count
# catches it.
D6="$TMP_ROOT/d6"
mk_repo "$D6"
cat >> "$D6/.specify/memory/open-work.md" <<'EOF'
| RES-7 | unescaped pipe in a quoted command | residue | src/x.rs | open | bubbles.implement | rerun `ls | head -5` and compare the output | 2026-02-01 | 2026-02-01 |
EOF
lint_rc "$D6"
rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "d7 an unescaped '|' inside a cell fails the lint (exit 1)"
else
  fail "d7 an unescaped '|' inside a cell fails the lint — got exit $rc"
fi
if report_has "$D6" "has 11 column delimiters but the table header declares 10"; then
  pass "d8 the column-count defect names the row and both counts"
else
  fail "d8 the column-count defect names the row and both counts"
fi

# The counterpart case. Without it the check could pass d7 by banning pipes
# outright, which would make the register unable to quote a shell pipeline at
# all — a cure worse than the defect.
D7="$TMP_ROOT/d7"
mk_repo "$D7"
cat >> "$D7/.specify/memory/open-work.md" <<'EOF'
| RES-8 | escaped pipe in a quoted command | residue | src/x.rs | open | bubbles.implement | rerun `ls \| head -5` and compare the output | 2026-02-01 | 2026-02-01 |
EOF
if lint_rc "$D7"; then
  pass "d9 a pipe escaped as '\\|' is content, not a column break, and passes the lint"
else
  fail "d9 a pipe escaped as '\\|' passes the lint"
fi

# --- (e) the register has to travel -------------------------------------------
E="$TMP_ROOT/e"
mk_repo "$E"
setup git init -q "$E"
setup git -C "$E" config user.email "selftest@bubbles.local"
setup git -C "$E" config user.name "Bubbles Selftest"
printf '.specify/memory/open-work.md\n' > "$E/.gitignore"
if report_has "$E" "is git-ignored"; then
  pass "e1 an ignored register is reported as a defect (a record that does not travel is lost)"
else
  fail "e1 an ignored register is reported as a defect"
fi

E2="$TMP_ROOT/e2"
mk_repo "$E2"
setup git init -q "$E2"
setup git -C "$E2" config user.email "selftest@bubbles.local"
setup git -C "$E2" config user.name "Bubbles Selftest"
if report_has "$E2" "is git-ignored"; then
  fail "e2 a tracked register is not reported as ignored"
else
  pass "e2 a tracked register is not reported as ignored"
fi

# The shipped register in this very repository must not be ignored either —
# a template that installs into an ignored path would defeat the whole scope.
#
# Two corrections live here. The repository root was resolved as
# "$SCRIPT_DIR/../..", which is the framework root: correct in the source tree,
# but `.github/` in an installed downstream, where `.specify/` sits at the real
# repository root instead. And the absence of a register was treated as a
# defect, which is only true of the framework source tree — what ships is
# `templates/open-work.md.tmpl`, so a repository that has not adopted a register
# yet correctly has no file, and this selftest failed every downstream install
# for it.
REPO_ROOT_SELF="$(cd "$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || printf '%s' "$SCRIPT_DIR/../..")" && pwd -P)"
REGISTER_SELF="$REPO_ROOT_SELF/.specify/memory/open-work.md"
if git -C "$REPO_ROOT_SELF" check-ignore -q "$REGISTER_SELF" 2>/dev/null; then
  fail "e3 the open-work register path must not be git-ignored"
elif [[ -f "$REGISTER_SELF" ]]; then
  pass "e3 the register in this repository is not git-ignored"
elif [[ -f "$REPO_ROOT_SELF/install.sh" && -f "$REPO_ROOT_SELF/VERSION" ]]; then
  fail "e3 the framework source tree ships .specify/memory/open-work.md"
else
  pass "e3 the register path is not git-ignored (none adopted in this repository)"
fi

# --- (f) the report is read-only ----------------------------------------------
F="$TMP_ROOT/f"
mk_repo "$F"
mk_spec "$F" "400-feature" "in_progress" "full-delivery"
before="$(find "$F" -type f | sort)"
report "$F" >/dev/null
report "$F" --format json >/dev/null
after="$(find "$F" -type f | sort)"
if [[ "$before" == "$after" ]]; then
  pass "f1 the report creates and removes nothing (no temp files in the repo it inspects)"
else
  fail "f1 the report creates and removes nothing"
fi

# --- (g) missing register degrades honestly ------------------------------------
G="$TMP_ROOT/g"
mkdir -p "$G/.specify/memory"
if report_has "$G" "register not found"; then
  pass "g1 a missing register is named, not silently treated as empty"
else
  fail "g1 a missing register is named"
fi
bash "$REPORT_SH" --repo-root "$G" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then
  pass "g2 a missing register still exits 0 without --lint (this is a report, not a gate)"
else
  fail "g2 a missing register still exits 0 without --lint — got exit $rc"
fi

echo
if [[ "$FAIL" -gt 0 ]]; then
  echo "open-work-report-selftest: $PASS passed, $FAIL FAILED."
  exit 1
fi
echo "open-work-report-selftest: all $PASS cases passed."
exit 0
