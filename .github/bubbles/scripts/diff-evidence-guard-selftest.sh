#!/usr/bin/env bash
#
# bubbles diff-evidence-guard-selftest.sh (v6.0 / B2).
#
# Asserts the v6 default-on policy AND the v5 grandfather clause:
#
#   1. Spec with state.json.modernization.diffEvidence = "enforce" runs in
#      strict mode (claims that don't match the diff fail).
#   2. Spec with state.json.modernization.diffEvidence = "advisory" runs in
#      advisory mode (claims that don't match the diff WARN, exit 0).
#   3. Spec with NO modernization block and NO pre-cutoff history runs in
#      strict mode by default (v6.0 / B2 default-on).
#   4. Spec with NO modernization block AND a pre-cutoff first commit runs
#      in advisory mode (v5 grandfather clause).
#   5. --strict flag forces strict mode regardless of state.json choice.
#   6. BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1 also forces strict mode.
#   7. Path-claims that DO match the diff always pass.
#
# All fixtures use a temp git repo under $HOME/.cache (snap-confined yq /
# git compatibility) so the selftest does not depend on the real bubbles
# git history.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/diff-evidence-guard.sh"

[[ -x "$GUARD" ]] || { echo "selftest: missing $GUARD" >&2; exit 2; }
command -v git >/dev/null 2>&1 || { echo "selftest: git required" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "selftest: python3 required" >&2; exit 2; }

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

TEST_ROOT_BASE="${HOME}/.cache/bubbles-diff-evidence-guard-selftest"
mkdir -p "$TEST_ROOT_BASE"
TEST_ROOT="$(mktemp -d -p "$TEST_ROOT_BASE")"
trap 'rm -rf "$TEST_ROOT"' EXIT

# Helper: initialize a git repo with a spec dir, a state.json, a scopes.md,
# and an optional committed file. Returns the absolute spec dir.
# Args: $1 = spec slug, $2 = state.json modernization JSON (or "" for none),
#       $3 = scopes.md content, $4 = optional source file path (relative),
#       $5 = pre-cutoff date YYYY-MM-DD (or "" for today).
#
# If a source file is committed in a follow-up commit, state.json.baseSha is
# set to the FIRST commit (initial spec) so the diff covers the source-add.
make_fixture() {
  local slug="$1"
  local mod_json="$2"
  local scopes_content="$3"
  local source_path="${4:-}"
  local commit_date="${5:-}"

  local repo="$TEST_ROOT/$slug"
  rm -rf "$repo"
  mkdir -p "$repo/specs/$slug"

  cd "$repo"
  git init -q
  git config user.email "selftest@bubbles.test"
  git config user.name "Bubbles Selftest"

  local mod_field='null'
  if [[ -n "$mod_json" ]]; then
    mod_field="$mod_json"
  fi
  # Placeholder baseSha; we rewrite after the first commit.
  cat > "specs/$slug/state.json" <<EOF
{
  "status": "in_progress",
  "executionHistory": [{"baseSha": "PLACEHOLDER"}],
  "modernization": ${mod_field}
}
EOF
  cat > "specs/$slug/scopes.md" <<EOF
$scopes_content
EOF

  git add -A
  if [[ -n "$commit_date" ]]; then
    GIT_AUTHOR_DATE="${commit_date}T12:00:00Z" \
    GIT_COMMITTER_DATE="${commit_date}T12:00:00Z" \
      git commit -q -m "spec $slug initial"
  else
    git commit -q -m "spec $slug initial"
  fi

  local base_sha
  base_sha="$(git rev-parse HEAD)"

  # Rewrite state.json with the real baseSha so the guard's diff window
  # actually includes any follow-up source commit.
  python3 -c "
import json
p = 'specs/$slug/state.json'
d = json.load(open(p))
d['executionHistory'][0]['baseSha'] = '$base_sha'
json.dump(d, open(p, 'w'), indent=2)
"
  git add -A
  if [[ -n "$commit_date" ]]; then
    GIT_AUTHOR_DATE="${commit_date}T12:00:00Z" \
    GIT_COMMITTER_DATE="${commit_date}T12:00:00Z" \
      git commit -q --amend --no-edit
  else
    git commit -q --amend --no-edit
  fi

  if [[ -n "$source_path" ]]; then
    mkdir -p "$(dirname "$source_path")"
    echo "// committed source" > "$source_path"
    git add -A
    git commit -q -m "add $source_path"
  fi

  echo "$repo/specs/$slug"
}

run_guard() {
  local spec_dir="$1"; shift
  out="$(bash "$GUARD" "$spec_dir" "$@" 2>&1 || true)"
  rc=$?
  if [[ "$rc" -ne 0 ]] && [[ -z "$out" ]]; then
    # `|| true` always returns 0; we need actual exit via $? inside command
    :
  fi
  # Capture exit explicitly.
  set +e
  bash "$GUARD" "$spec_dir" "$@" >/dev/null 2>&1
  rc=$?
  set -e
  echo "$out"
  return "$rc"
}

# Scope content with a fake path claim that does NOT exist in the diff.
SCOPES_FAKE_CLAIM=$'## Scope 1\n\n### Definition of Done\n\n- [x] Added unit test at `tests/fake_nonexistent.rs`\n'
# Scope content with a real path claim (we will commit the file).
SCOPES_REAL_CLAIM=$'## Scope 1\n\n### Definition of Done\n\n- [x] Added unit test at `tests/real_committed.rs`\n'

# Case 1: modernization.diffEvidence = "enforce" -> strict
spec1="$(make_fixture case1 '{"diffEvidence": "enforce"}' "$SCOPES_FAKE_CLAIM")"
out1="$(bash "$GUARD" "$spec1" 2>&1 || true)"
rc1="$(bash "$GUARD" "$spec1" >/dev/null 2>&1; echo $?)"
if [[ "$rc1" -ne 0 ]] && echo "$out1" | grep -q FAIL; then
  pass "1. enforce in state.json -> strict mode rejects fake claim"
else
  fail "1. enforce mode: rc=$rc1 out=$out1"
fi

# Case 2: modernization.diffEvidence = "advisory" -> WARN, exit 0
spec2="$(make_fixture case2 '{"diffEvidence": "advisory"}' "$SCOPES_FAKE_CLAIM")"
rc2="$(bash "$GUARD" "$spec2" >/dev/null 2>&1; echo $?)"
out2="$(bash "$GUARD" "$spec2" 2>&1 || true)"
if [[ "$rc2" -eq 0 ]] && echo "$out2" | grep -q -E 'WARN|advisory'; then
  pass "2. advisory in state.json -> WARN, exit 0"
else
  fail "2. advisory mode: rc=$rc2 out=$out2"
fi

# Case 3: NO modernization block + today's commit (post-cutoff) -> strict (v6 default-on)
spec3="$(make_fixture case3 '' "$SCOPES_FAKE_CLAIM")"
rc3="$(bash "$GUARD" "$spec3" >/dev/null 2>&1; echo $?)"
out3="$(bash "$GUARD" "$spec3" 2>&1 || true)"
if [[ "$rc3" -ne 0 ]] && echo "$out3" | grep -q -E 'enforcing|FAIL'; then
  pass "3. no modernization block + new spec -> strict mode (v6 default-on)"
else
  fail "3. v6 default-on: rc=$rc3 out=$out3"
fi

# Case 4: NO modernization block + pre-cutoff commit (2026-05-01) -> advisory (v5 grandfather)
spec4="$(make_fixture case4 '' "$SCOPES_FAKE_CLAIM" "" "2026-05-01")"
rc4="$(bash "$GUARD" "$spec4" >/dev/null 2>&1; echo $?)"
out4="$(bash "$GUARD" "$spec4" 2>&1 || true)"
if [[ "$rc4" -eq 0 ]] && echo "$out4" | grep -q -E 'WARN|advisory|grandfather'; then
  pass "4. no modernization block + pre-cutoff spec -> advisory (v5 grandfather)"
else
  fail "4. v5 grandfather: rc=$rc4 out=$out4"
fi

# Case 5: --strict flag forces strict regardless of advisory in state.json
spec5="$(make_fixture case5 '{"diffEvidence": "advisory"}' "$SCOPES_FAKE_CLAIM")"
rc5="$(bash "$GUARD" "$spec5" --strict >/dev/null 2>&1; echo $?)"
if [[ "$rc5" -ne 0 ]]; then
  pass "5. --strict flag overrides advisory choice"
else
  fail "5. --strict override: rc=$rc5"
fi

# Case 6: BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1 forces strict
spec6="$(make_fixture case6 '{"diffEvidence": "advisory"}' "$SCOPES_FAKE_CLAIM")"
rc6="$(BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1 bash "$GUARD" "$spec6" >/dev/null 2>&1; echo $?)"
if [[ "$rc6" -ne 0 ]]; then
  pass "6. env var BUBBLES_DIFF_EVIDENCE_GUARD_STRICT=1 overrides advisory"
else
  fail "6. env override: rc=$rc6"
fi

# Case 7: real committed path claim passes even in strict mode.
spec7="$(make_fixture case7 '{"diffEvidence": "enforce"}' "$SCOPES_REAL_CLAIM" "tests/real_committed.rs")"
rc7="$(bash "$GUARD" "$spec7" >/dev/null 2>&1; echo $?)"
out7="$(bash "$GUARD" "$spec7" 2>&1 || true)"
if [[ "$rc7" -eq 0 ]] && echo "$out7" | grep -q PASS; then
  pass "7. real committed path claim PASSes in strict mode"
else
  fail "7. real-claim PASS: rc=$rc7 out=$out7"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "diff-evidence-guard-selftest: FAIL ($failures issue(s))"
  exit 1
fi
echo "diff-evidence-guard-selftest: PASS"
exit 0
