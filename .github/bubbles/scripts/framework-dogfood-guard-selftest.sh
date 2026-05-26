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
#   S3  downstream/fixture repo with no done specs                   → exit 1
#   S4  downstream/fixture repo with one done spec                   → exit 0
#   S5  downstream/fixture repo with one in_progress spec            → exit 1
#   S6  downstream/fixture repo with malformed state.json            → exit 2
#   S7  downstream/fixture repo with non-numbered done spec ignored  → exit 1
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
    note "stdout: $(cat "$WORKSPACE/stdout.last" | head -3 | tr '\n' '|')"
    note "stderr: $(cat "$WORKSPACE/stderr.last" | head -3 | tr '\n' '|')"
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

# --- S3: downstream no done specs ------------------------------------------

echo ""
echo "--- S3: downstream specs/ exists, zero numbered feature state.json ---"
repo="$(stage_fresh_repo s3)"
mkdir -p "$repo/specs"
run_guard "$repo"
assert_exit "S3 zero numbered state.json" 1
assert_stderr_contains "S3" "G085"
assert_stderr_contains "S3" "count with status==done:                 0"

# --- S4: downstream one done numbered spec ---------------------------------

echo ""
echo "--- S4: one done numbered spec ---"
repo="$(stage_fresh_repo s4)"
write_state_json "$repo/specs/001-foo/state.json" "done"
run_guard "$repo"
assert_exit "S4 one done numbered spec" 0
assert_stdout_contains "S4" "PASS Gate G085"
assert_stdout_contains "S4" "doneCount=1/1"

# --- S5: downstream one in_progress numbered spec --------------------------

echo ""
echo "--- S5: one in_progress numbered spec ---"
repo="$(stage_fresh_repo s5)"
write_state_json "$repo/specs/001-foo/state.json" "in_progress"
run_guard "$repo"
assert_exit "S5 one in_progress numbered spec" 1
assert_stderr_contains "S5" "G085"
assert_stderr_contains "S5" "status=in_progress"

# --- S6: malformed state.json ---------------------------------------------

echo ""
echo "--- S6: malformed state.json (invalid JSON) ---"
repo="$(stage_fresh_repo s6)"
mkdir -p "$repo/specs/001-broken"
printf '%s' "{ this is not valid json" > "$repo/specs/001-broken/state.json"
run_guard "$repo"
assert_exit "S6 malformed state.json" 2
assert_stderr_contains "S6" "failed to parse"

# --- S7: non-numbered specs are ignored -----------------------------------

echo ""
echo "--- S7: non-numbered spec dirs are ignored ---"
repo="$(stage_fresh_repo s7)"
# specs/foo-bar/state.json with status=done — but does NOT match NNN- pattern
write_state_json "$repo/specs/foo-bar/state.json" "done"
run_guard "$repo"
assert_exit "S7 non-numbered spec dir ignored" 1
assert_stderr_contains "S7" "G085"
assert_stderr_contains "S7" "numbered-feature state.json files found: 0"

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
