#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT_SCRIPT="$SCRIPT_DIR/batch-promotion-lint.sh"

tmp_root="$(mktemp -d)"
failures=0

cleanup() {
  if [[ "$failures" -eq 0 ]] && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$tmp_root"
  else
    echo "Preserving selftest workspace: $tmp_root"
  fi
}

trap cleanup EXIT

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

run_capture() {
  local log_file="$1"
  shift

  set +e
  "$@" >"$log_file" 2>&1
  local status=$?
  set -e

  echo "$status"
}

assert_log_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq "$needle" "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- log excerpt: $log_file ---"
    sed -n '1,200p' "$log_file"
    echo "--- end log excerpt ---"
  fi
}

init_repo() {
  local repo_dir="$1"
  mkdir -p "$repo_dir"
  git -C "$repo_dir" init >/dev/null
  git -C "$repo_dir" config user.email selftest@example.com
  git -C "$repo_dir" config user.name selftest
}

write_state() {
  local repo_dir="$1"
  local feature_path="$2"
  local status_value="$3"
  mkdir -p "$repo_dir/$feature_path"
  cat > "$repo_dir/$feature_path/state.json" <<EOF
{"status":"$status_value"}
EOF
}

echo "Running root-commit batch-promotion selftest..."
root_repo="$tmp_root/root-commit-repo"
init_repo "$root_repo"
write_state "$root_repo" "specs/001-root-a" "done"
write_state "$root_repo" "specs/002-root-b" "done"
git -C "$root_repo" add specs/001-root-a/state.json specs/002-root-b/state.json
git -C "$root_repo" commit -m "root batch" >/dev/null
root_sha="$(git -C "$root_repo" rev-parse HEAD)"
root_log="$tmp_root/root-commit.log"
root_status="$(cd "$root_repo" && run_capture "$root_log" bash "$LINT_SCRIPT" --ref="$root_sha" --max=1)"
if [[ "$root_status" -ne 0 ]]; then
  pass "Root commit with two done promotions fails the lint as expected"
else
  fail "Root commit with two done promotions should fail the lint"
  sed -n '1,200p' "$root_log"
fi
assert_log_contains "$root_log" "Promotions detected: 2" "Root commit path counts done promotions"
assert_log_contains "$root_log" "exceed batch limit" "Root commit path blocks over-limit promotions"

echo "Running single-promotion ref selftest..."
ref_repo="$tmp_root/ref-repo"
init_repo "$ref_repo"
write_state "$ref_repo" "specs/010-ref-a" "in_progress"
git -C "$ref_repo" add specs/010-ref-a/state.json
git -C "$ref_repo" commit -m "baseline" >/dev/null
write_state "$ref_repo" "specs/010-ref-a" "done"
git -C "$ref_repo" add specs/010-ref-a/state.json
git -C "$ref_repo" commit -m "promote one" >/dev/null
ref_sha="$(git -C "$ref_repo" rev-parse HEAD)"
ref_log="$tmp_root/ref.log"
ref_status="$(cd "$ref_repo" && run_capture "$ref_log" bash "$LINT_SCRIPT" --ref="$ref_sha" --max=1)"
if [[ "$ref_status" -eq 0 ]]; then
  pass "Single done promotion in ref mode passes within limit"
else
  fail "Single done promotion in ref mode should pass"
  sed -n '1,200p' "$ref_log"
fi
assert_log_contains "$ref_log" "Promotions detected: 1" "Ref mode detects a single promotion"
assert_log_contains "$ref_log" "within limit" "Ref mode reports within-limit pass"

echo "Running staged batch-promotion selftest..."
staged_repo="$tmp_root/staged-repo"
init_repo "$staged_repo"
write_state "$staged_repo" "specs/020-stage-a" "in_progress"
write_state "$staged_repo" "specs/021-stage-b" "in_progress"
git -C "$staged_repo" add specs/020-stage-a/state.json specs/021-stage-b/state.json
git -C "$staged_repo" commit -m "baseline" >/dev/null
write_state "$staged_repo" "specs/020-stage-a" "done"
write_state "$staged_repo" "specs/021-stage-b" "done"
git -C "$staged_repo" add specs/020-stage-a/state.json specs/021-stage-b/state.json
staged_log="$tmp_root/staged.log"
staged_status="$(cd "$staged_repo" && run_capture "$staged_log" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$staged_status" -ne 0 ]]; then
  pass "Staged mode blocks two promotions in one batch"
else
  fail "Staged mode should block two promotions in one batch"
  sed -n '1,200p' "$staged_log"
fi
assert_log_contains "$staged_log" "Promotions detected: 2" "Staged mode counts both promotions"

echo "Running override selftest..."
override_log="$tmp_root/override.log"
override_status="$(cd "$staged_repo" && run_capture "$override_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE=1 bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$override_status" -eq 0 ]]; then
  pass "Override allows an over-limit promotion batch to exit 0"
else
  fail "Override should allow the over-limit promotion batch"
  sed -n '1,200p' "$override_log"
fi
assert_log_contains "$override_log" "BUBBLES_BATCH_PROMOTION_OVERRIDE=1" "Override path is reported explicitly"

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "batch-promotion-lint selftest failed with $failures issue(s)."
  exit 1
fi

echo "batch-promotion-lint selftest passed."
