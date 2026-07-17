#!/usr/bin/env bash
set -euo pipefail

# framework-dogfood-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/framework-dogfood-guard.sh`
# (Gate G085 — framework_dogfood_evidence_gate).
#
# Scenarios:
#   S0  Bubbles source repo with no specs/ and evidence surfaces    → exit 0
#   S1  Bubbles source repo with specs/                             → exit 1
#   S2  Bubbles source repo missing release manifest evidence        → exit 1
#   S3  downstream/fixture repo with no current numbered spec        → exit 1
#   S4  current done fast path without Git metadata                  → exit 0
#   S5  genuine first adoption in a path containing spaces           → exit 0
#   S6  malformed or symlinked current numbered state                → exit 2
#   S7  changed historical done state                                → exit 1
#   S8  deleted historical done state                                → exit 1
#   S9  historical done reachable only from another local ref        → exit 1
#   S10 missing Git metadata and nested root                          → exit 2
#   S11 effective file:// shallow clone                              → exit 2
#   S12a extensions.partialClone metadata                            → exit 2
#   S12b remote.promisor metadata                                    → exit 2
#   S13 failed commit, tree, and blob traversal                      → exit 2
#   S14 malformed reachable historical state                         → exit 2
#   S15 non-numbered and nested done evidence ignored                → exit 0
#   S16 delegated guidance names both downstream pass paths          → static
#
# Each scenario stages an isolated `mktemp` workspace, points the guard
# at it via --repo-root, and asserts the expected exit code (and, for
# violations, that stderr mentions Gate G085 and the recipe path).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/framework-dogfood-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "framework-dogfood-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g085-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

note() { printf '  · %s\n' "$*"; }
ok()   { PASS_COUNT=$((PASS_COUNT + 1)); printf '  ✅ PASS: %s\n' "$*"; }
ko()   { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_SCENARIOS+=("$*"); printf '  ❌ FAIL: %s\n' "$*"; }

# --- helpers --------------------------------------------------------------

stage_fresh_repo() {
  # $1 = scenario id (used to scope a fresh subdir inside $WORKSPACE)
  local sid="$1"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo/.specify/memory"
  printf '%s' "$repo"
}

stage_source_repo() {
  local sid="$1"
  local repo
  repo="$(stage_fresh_repo "$sid")"
  mkdir -p "$repo/bubbles/scripts" "$repo/agents"
  touch "$repo/install.sh" "$repo/VERSION" "$repo/bubbles/release-manifest.json"
  cat > "$repo/bubbles/scripts/framework-validate.sh" <<'EOF'
#!/usr/bin/env bash
run_check "Framework dogfood guard selftest" bash "$SCRIPT_DIR/framework-dogfood-guard-selftest.sh"
EOF
  cat > "$repo/bubbles/scripts/framework-dogfood-guard-selftest.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$repo/bubbles/scripts/framework-validate.sh" "$repo/bubbles/scripts/framework-dogfood-guard-selftest.sh"
  printf '%s' "$repo"
}

init_git_repo() {
  local repo="$1"
  git -C "$repo" init -q
  git -C "$repo" config user.name "Bubbles G085 Selftest"
  git -C "$repo" config user.email "g085-selftest@example.invalid"
}

commit_all() {
  local repo="$1"
  local message="$2"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "$message"
}

write_private_done_state_json() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  cat > "$path" <<'EOF'
{
  "version": 3,
  "featureDir": "specs/001-foo",
  "status": "done",
  "privateMarker": "G085_PRIVATE_BLOB_PAYLOAD"
}
EOF
}

write_nested_done_state_json() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  cat > "$path" <<'EOF'
{
  "version": 3,
  "featureDir": "specs/001-foo",
  "status": "in_progress",
  "certification": {
    "status": "done"
  }
}
EOF
}

write_state_json() {
  # $1 = path to state.json
  # $2 = status string
  local path="$1"
  local status="$2"
  mkdir -p "$(dirname "$path")"
  cat > "$path" <<EOF
{
  "version": 3,
  "featureDir": "specs/$(basename "$(dirname "$path")")",
  "status": "$status"
}
EOF
}

run_guard() {
  # $1 = repo root to point the guard at
  local repo="$1"
  set +e
  bash "$GUARD" --repo-root "$repo" --quiet > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
  return 0
}

assert_exit() {
  # $1 = scenario label
  # $2 = expected exit code
  local label="$1"
  local expected="$2"
  local actual
  actual="$(cat "$WORKSPACE/exit.last")"
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label (exit=$actual)"
  else
    ko "$label (expected exit=$expected, actual=$actual)"
    note "stdout was:"
    sed 's/^/      /' "$WORKSPACE/stdout.last"
    note "stderr was:"
    sed 's/^/      /' "$WORKSPACE/stderr.last"
  fi
}

assert_stderr_contains() {
  # $1 = scenario label
  # $2 = substring
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stderr.last"; then
    ok "$label stderr contains '$needle'"
  else
    ko "$label stderr missing '$needle'"
    note "stderr was:"
    sed 's/^/      /' "$WORKSPACE/stderr.last"
  fi
}

assert_stdout_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last"; then
    ok "$label stdout contains '$needle'"
  else
    ko "$label stdout missing '$needle'"
    note "stdout was:"
    sed 's/^/      /' "$WORKSPACE/stdout.last"
  fi
}

assert_stderr_not_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stderr.last"; then
    ko "$label stderr unexpectedly contains '$needle'"
    note "stderr was:"
    sed 's/^/      /' "$WORKSPACE/stderr.last"
  else
    ok "$label stderr omits '$needle'"
  fi
}

assert_file_contains() {
  local label="$1"
  local file="$2"
  local needle="$3"
  if grep -qF "$needle" "$file"; then
    ok "$label"
  else
    ko "$label missing '$needle'"
  fi
}

assert_file_not_contains() {
  local label="$1"
  local file="$2"
  local needle="$3"
  if grep -qF "$needle" "$file"; then
    ko "$label unexpectedly contains '$needle'"
  else
    ok "$label"
  fi
}

snapshot_git_repository() {
  local repo="$1"
  local output_file="$2"
  {
    echo "HEAD=$(git -C "$repo" rev-parse HEAD)"
    echo "status:"
    git -C "$repo" status --porcelain=v1 --untracked-files=all
    echo "refs:"
    git -C "$repo" show-ref || true
    echo "objects:"
    find "$repo/.git/objects" -type f -print \
      | sed "s#^$repo/##" \
      | LC_ALL=C sort
  } > "$output_file"
}

assert_files_equal() {
  local label="$1"
  local expected_file="$2"
  local actual_file="$3"
  if cmp -s "$expected_file" "$actual_file"; then
    ok "$label"
  else
    ko "$label"
    note "expected snapshot:"
    sed 's/^/      /' "$expected_file"
    note "actual snapshot:"
    sed 's/^/      /' "$actual_file"
  fi
}

remove_loose_git_object() {
  local repo="$1"
  local object_id="$2"
  local object_path="$repo/.git/objects/${object_id:0:2}/${object_id:2}"
  if [[ ! -f "$object_path" ]]; then
    ko "fixture object is loose and removable: $object_id"
    return 1
  fi
  rm -f "$object_path"
}

echo "=== framework-dogfood-guard-selftest (Gate G085) ==="

# --- S0: Bubbles source repo, no specs/ -------------------------------------

echo ""
echo "--- S0: source repo has no specs/ and evidence surfaces exist ---"
repo="$(stage_source_repo s0)"
run_guard "$repo"
assert_exit "S0 source repo without specs/" 0
assert_stdout_contains "S0" "PASS Gate G085"
assert_stdout_contains "S0" "source repo has no persistent specs/"

# --- S1: Bubbles source repo with specs/ ------------------------------------

echo ""
echo "--- S1: source repo contains specs/ ---"
repo="$(stage_source_repo s1)"
mkdir -p "$repo/specs"
run_guard "$repo"
assert_exit "S1 source repo specs/ violation" 1
assert_stderr_contains "S1" "G085"
assert_stderr_contains "S1" "docs/recipes/framework-dogfood.md"
assert_stderr_contains "S1" "MUST NOT contain persistent specs/"

# --- S2: Bubbles source repo missing release manifest -----------------------

echo ""
echo "--- S2: source repo missing release manifest evidence ---"
repo="$(stage_source_repo s2)"
rm -f "$repo/bubbles/release-manifest.json"
run_guard "$repo"
assert_exit "S2 missing release manifest" 1
assert_stderr_contains "S2" "missing surfaces"
assert_stderr_contains "S2" "bubbles/release-manifest.json"

# --- S3: downstream has no current numbered spec ----------------------------

echo ""
echo "--- S3: downstream specs/ exists, zero numbered feature state.json ---"
repo="$(stage_fresh_repo s3)"
mkdir -p "$repo/specs"
run_guard "$repo"
assert_exit "S3 zero numbered state.json" 1
assert_stderr_contains "S3" "failureCode=E085-NO-CURRENT-SPEC"
assert_stderr_contains "S3" "currentSpecs=0"

# --- S4: current done fast path does not require Git ------------------------

echo ""
echo "--- S4: one done numbered spec ---"
repo="$(stage_fresh_repo s4)"
write_state_json "$repo/specs/001-foo/state.json" "done"
run_guard "$repo"
assert_exit "S4 one done numbered spec" 0
assert_stdout_contains "S4" "decisionCode=G085-CURRENT-DONE"
assert_stdout_contains "S4" "currentDone=1"

# --- S5: genuine first adoption ---------------------------------------------

echo ""
echo "--- S5: first adoption with one committed in_progress spec in a path containing spaces ---"
repo="$(stage_fresh_repo "s5 first adoption")"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
snapshot_git_repository "$repo" "$WORKSPACE/s5-before.snapshot"
run_guard "$repo"
snapshot_git_repository "$repo" "$WORKSPACE/s5-after.snapshot"
assert_exit "S5 genuine first adoption" 0
assert_stdout_contains "S5" "decisionCode=G085-FIRST-ADOPTION"
assert_stdout_contains "S5" "currentDone=0"
assert_stdout_contains "S5" "historicalDone=0"
assert_stdout_contains "S5" "historyIntegrity=complete"
assert_files_equal "S5 guard leaves refs, index, worktree, and object inventory unchanged" \
  "$WORKSPACE/s5-before.snapshot" "$WORKSPACE/s5-after.snapshot"

# --- S6: malformed current state.json --------------------------------------

echo ""
echo "--- S6: malformed state.json (invalid JSON) ---"
repo="$(stage_fresh_repo s6)"
mkdir -p "$repo/specs/001-broken"
printf '%s' "{ this is not valid json" > "$repo/specs/001-broken/state.json"
run_guard "$repo"
assert_exit "S6 malformed state.json" 2
assert_stderr_contains "S6" "failureCode=E085-CURRENT-STATE-MALFORMED"

echo ""
echo "--- S6: numbered state.json symlink to external done JSON fails closed ---"
repo="$(stage_fresh_repo s6-symlink)"
external_state="$WORKSPACE/s6-external-done.json"
write_state_json "$external_state" "done"
mkdir -p "$repo/specs/001-linked"
ln -s "$external_state" "$repo/specs/001-linked/state.json"
run_guard "$repo"
assert_exit "S6 external current-state symlink" 2
assert_stderr_contains "S6 symlink" "failureCode=E085-CURRENT-STATE-MALFORMED"
assert_stderr_contains "S6 symlink" "current numbered state.json files must be regular non-symbolic-link files"

# --- S7: changed historical done remains established ------------------------

echo ""
echo "--- S7: done state changed to in_progress remains established ---"
repo="$(stage_fresh_repo s7)"
init_git_repo "$repo"
write_private_done_state_json "$repo/specs/001-foo/state.json"
commit_all "$repo" "complete first Bubbles feature"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "remove current done evidence"
run_guard "$repo"
assert_exit "S7 changed historical done" 1
assert_stderr_contains "S7" "failureCode=E085-ESTABLISHED-DONE-REMOVED"
assert_stderr_contains "S7" "historyPath=specs/001-foo/state.json"
assert_stderr_contains "S7" "historyCommit="
assert_stderr_not_contains "S7 blob privacy" "G085_PRIVATE_BLOB_PAYLOAD"

# --- S8: deleted historical done remains established ------------------------

echo ""
echo "--- S8: deleted done state remains established while another current state remains ---"
repo="$(stage_fresh_repo s8)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-done/state.json" "done"
write_state_json "$repo/specs/002-current/state.json" "in_progress"
commit_all "$repo" "done and current feature states"
rm -rf "$repo/specs/001-done"
commit_all "$repo" "delete current done evidence"
run_guard "$repo"
assert_exit "S8 deleted historical done" 1
assert_stderr_contains "S8" "failureCode=E085-ESTABLISHED-DONE-REMOVED"
assert_stderr_contains "S8" "historyPath=specs/001-done/state.json"

# --- S9: all reachable refs are scanned -------------------------------------

echo ""
echo "--- S9: done state reachable only from another local branch is established ---"
repo="$(stage_fresh_repo s9)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "current first-adoption candidate"
current_branch="$(git -C "$repo" symbolic-ref --short HEAD)"
git -C "$repo" checkout -q -b historical-done
write_state_json "$repo/specs/001-foo/state.json" "done"
commit_all "$repo" "done evidence on alternate ref"
git -C "$repo" checkout -q "$current_branch"
run_guard "$repo"
assert_exit "S9 all-ref historical done" 1
assert_stderr_contains "S9" "failureCode=E085-ESTABLISHED-DONE-REMOVED"
assert_stderr_contains "S9" "historyPath=specs/001-foo/state.json"

# --- S10: missing Git metadata and nested root -------------------------------

echo ""
echo "--- S10: missing Git metadata and nested requested root fail unavailable ---"
repo="$(stage_fresh_repo s10-missing)"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
run_guard "$repo"
assert_exit "S10 missing Git metadata" 2
assert_stderr_contains "S10 missing" "failureCode=E085-HISTORY-UNAVAILABLE"

parent_repo="$(stage_fresh_repo s10-parent)"
init_git_repo "$parent_repo"
write_state_json "$parent_repo/specs/001-parent/state.json" "in_progress"
mkdir -p "$parent_repo/nested/.specify/memory"
write_state_json "$parent_repo/nested/specs/001-foo/state.json" "in_progress"
commit_all "$parent_repo" "parent and nested state"
run_guard "$parent_repo/nested"
assert_exit "S10 nested root" 2
assert_stderr_contains "S10 nested" "failureCode=E085-HISTORY-UNAVAILABLE"
assert_stderr_contains "S10 nested" "not the exact Git worktree root"

# --- S11: shallow history ----------------------------------------------------

echo ""
echo "--- S11: effective file:// shallow clone fails closed ---"
source_repo="$(stage_fresh_repo s11-source)"
init_git_repo "$source_repo"
write_state_json "$source_repo/specs/001-foo/state.json" "done"
commit_all "$source_repo" "historical done state"
write_state_json "$source_repo/specs/001-foo/state.json" "in_progress"
commit_all "$source_repo" "current nonterminal state"
repo="$WORKSPACE/s11-shallow"
git clone -q --depth 1 "file://$source_repo" "$repo"
shallow_fixture_state="$(git -C "$repo" rev-parse --is-shallow-repository)"
if [[ "$shallow_fixture_state" == "true" ]]; then
  ok "S11 fixture is genuinely shallow"
else
  ko "S11 fixture setup expected shallow=true, actual=$shallow_fixture_state"
fi
run_guard "$repo"
assert_exit "S11 shallow history" 2
assert_stderr_contains "S11" "failureCode=E085-HISTORY-SHALLOW"
assert_stderr_contains "S11" "historyIntegrity=shallow"

# --- S12a/S12b: partial-clone and promisor metadata --------------------------

echo ""
echo "--- S12a: extensions.partialClone metadata fails closed as partial history ---"
repo="$(stage_fresh_repo s12-partial-clone)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
git -C "$repo" config core.repositoryformatversion 1
git -C "$repo" config extensions.partialClone origin
run_guard "$repo"
assert_exit "S12a extensions.partialClone metadata" 2
assert_stderr_contains "S12a" "failureCode=E085-HISTORY-PARTIAL"
assert_stderr_contains "S12a" "historyIntegrity=partial"
assert_stderr_contains "S12a" "extensions.partialClone metadata is present"

echo ""
echo "--- S12b: remote.promisor metadata fails closed as partial history ---"
repo="$(stage_fresh_repo s12-promisor)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
git -C "$repo" config remote.origin.promisor true
run_guard "$repo"
assert_exit "S12b remote.promisor metadata" 2
assert_stderr_contains "S12b" "failureCode=E085-HISTORY-PARTIAL"
assert_stderr_contains "S12b" "historyIntegrity=partial"
assert_stderr_contains "S12b" "remote promisor metadata is enabled"

# --- S13: failed commit, tree, and blob traversal ----------------------------

echo ""
echo "--- S13: broken reachable ref fails commit traversal ---"
repo="$(stage_fresh_repo s13-commit)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
mkdir -p "$repo/.git/refs/heads"
printf '%s\n' "1111111111111111111111111111111111111111" > "$repo/.git/refs/heads/broken-history"
run_guard "$repo"
assert_exit "S13 failed reachable-ref traversal" 2
assert_stderr_contains "S13 commit" "failureCode=E085-HISTORY-QUERY-FAILED"
assert_stderr_contains "S13 commit" "historyIntegrity=query-failed"

echo ""
echo "--- S13: missing historical tree object fails closed ---"
repo="$(stage_fresh_repo s13-tree)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
tree_object="$(git -C "$repo" rev-parse 'HEAD^{tree}')"
remove_loose_git_object "$repo" "$tree_object"
run_guard "$repo"
assert_exit "S13 failed historical tree traversal" 2
assert_stderr_contains "S13 tree" "failureCode=E085-HISTORY-QUERY-FAILED"
assert_stderr_contains "S13 tree" "historyIntegrity=query-failed"

echo ""
echo "--- S13: missing historical state blob fails closed ---"
repo="$(stage_fresh_repo s13-blob)"
init_git_repo "$repo"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "first Bubbles feature in progress"
blob_object="$(git -C "$repo" rev-parse 'HEAD:specs/001-foo/state.json')"
remove_loose_git_object "$repo" "$blob_object"
run_guard "$repo"
assert_exit "S13 failed historical blob traversal" 2
assert_stderr_contains "S13 blob" "failureCode=E085-HISTORY-QUERY-FAILED"
assert_stderr_contains "S13 blob" "historyIntegrity=query-failed"

# --- S14: malformed historical state ---------------------------------------

echo ""
echo "--- S14: malformed reachable historical state fails distinctly ---"
repo="$(stage_fresh_repo s14)"
init_git_repo "$repo"
mkdir -p "$repo/specs/001-foo"
printf '%s\n' "{ malformed historical json" > "$repo/specs/001-foo/state.json"
commit_all "$repo" "malformed historical state"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
commit_all "$repo" "valid current state"
run_guard "$repo"
assert_exit "S14 malformed historical state" 2
assert_stderr_contains "S14" "failureCode=E085-HISTORICAL-STATE-MALFORMED"
assert_stderr_contains "S14" "historyIntegrity=malformed"
assert_stderr_contains "S14" "historyPath=specs/001-foo/state.json"

# --- S15: only exact numbered top-level done evidence counts ----------------

echo ""
echo "--- S15: non-numbered and nested done evidence are ignored ---"
repo="$(stage_fresh_repo s15)"
init_git_repo "$repo"
write_state_json "$repo/specs/not-numbered/state.json" "done"
write_nested_done_state_json "$repo/specs/001-foo/state.json"
write_state_json "$repo/specs/001-foo/nested/state.json" "done"
commit_all "$repo" "non-counting done evidence"
run_guard "$repo"
assert_exit "S15 ignored non-numbered and nested done" 0
assert_stdout_contains "S15" "decisionCode=G085-FIRST-ADOPTION"
assert_stdout_contains "S15" "historicalDone=0"

# --- S16: delegated failure guidance names both valid downstream paths -------

echo ""
echo "--- S16: delegated G085 guidance names current-done and genuine first-adoption paths ---"
delegated_gates="$SCRIPT_DIR/guards/tail-delegated-gates.sh"
assert_file_contains "S16 current-done guidance" "$delegated_gates" \
  "Downstream/fixture pass path G085-CURRENT-DONE:"
assert_file_contains "S16 genuine first-adoption guidance" "$delegated_gates" \
  "Downstream/fixture pass path G085-FIRST-ADOPTION: genuine first adoption"
assert_file_not_contains "S16 stale single-path guidance is absent" "$delegated_gates" \
  "Downstream/fixture requirement: at least one specs/[0-9]*-*/state.json has top-level"

# --- Final verdict --------------------------------------------------------

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"
echo ""

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "🔴 framework-dogfood-guard-selftest: FAILED" >&2
  for s in "${FAILED_SCENARIOS[@]}"; do
    echo "    - $s" >&2
  done
  exit 1
fi

echo "🟢 framework-dogfood-guard-selftest: PASSED"
exit 0
