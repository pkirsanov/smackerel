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

echo "Running override hardening selftest..."
# The staged_repo still has two over-limit staged promotions; the target sha in
# staged mode is its HEAD (the baseline commit). The hardened override requires
# a well-formed, unexpired, sha-bound token — a bare "1" no longer works.
staged_head="$(git -C "$staged_repo" rev-parse HEAD)"
now_epoch="$(date -u +%s)"
future_expiry="$((now_epoch + 3600))"
past_expiry="$((now_epoch - 3600))"
ledger_file="$tmp_root/override-ledger.jsonl"

# (a) Legacy bare "1" MUST no longer be honored — the block stands (exit != 0).
bare_log="$tmp_root/override-bare.log"
bare_status="$(cd "$staged_repo" && run_capture "$bare_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE=1 BUBBLES_BATCH_PROMOTION_LEDGER="$ledger_file" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$bare_status" -ne 0 ]]; then
  pass "Legacy bare '1' override is REFUSED (block stands)"
else
  fail "Legacy bare '1' override must no longer exit 0"
  sed -n '1,200p' "$bare_log"
fi
assert_log_contains "$bare_log" "bare value" "Bare '1' rejection explains the token requirement"

# (b) Expired token (correct sha, past expiry) MUST be refused.
expired_log="$tmp_root/override-expired.log"
expired_status="$(cd "$staged_repo" && run_capture "$expired_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE="alice:$past_expiry:$staged_head" BUBBLES_BATCH_PROMOTION_LEDGER="$ledger_file" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$expired_status" -ne 0 ]]; then
  pass "Expired override token is REFUSED"
else
  fail "Expired override token must be refused"
  sed -n '1,200p' "$expired_log"
fi
assert_log_contains "$expired_log" "EXPIRED" "Expired token rejection is explained"

# (c) Wrong-sha token (valid expiry, bogus sha) MUST be refused.
wrongsha_log="$tmp_root/override-wrongsha.log"
wrongsha_status="$(cd "$staged_repo" && run_capture "$wrongsha_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE="alice:$future_expiry:deadbeefdeadbeef" BUBBLES_BATCH_PROMOTION_LEDGER="$ledger_file" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$wrongsha_status" -ne 0 ]]; then
  pass "Wrong-sha override token is REFUSED (defeats replay onto a different commit)"
else
  fail "Wrong-sha override token must be refused"
  sed -n '1,200p' "$wrongsha_log"
fi
assert_log_contains "$wrongsha_log" "does not match target" "Wrong-sha rejection is explained"

# (d) Valid token (actor:future_expiry:HEAD sha) is HONORED (exit 0).
valid_log="$tmp_root/override-valid.log"
valid_status="$(cd "$staged_repo" && run_capture "$valid_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE="alice:$future_expiry:$staged_head" BUBBLES_BATCH_PROMOTION_LEDGER="$ledger_file" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$valid_status" -eq 0 ]]; then
  pass "Valid actor:expiry:sha override token is honored (exit 0)"
else
  fail "Valid override token should be honored"
  sed -n '1,200p' "$valid_log"
fi
assert_log_contains "$valid_log" "honored" "Honored override is reported explicitly"

# (e) The honored override wrote an append-only audit line binding actor + sha.
if [[ -f "$ledger_file" ]] && grep -Fq '"actor":"alice"' "$ledger_file" && grep -Fq "$staged_head" "$ledger_file"; then
  pass "Audit ledger records the honored override (actor + target sha)"
else
  fail "Audit ledger should record the honored override (actor + target sha)"
  [[ -f "$ledger_file" ]] && sed -n '1,50p' "$ledger_file"
fi

# (f) Prefix (short) sha token is also honored — short-sha ergonomics preserved.
short_sha="${staged_head:0:12}"
prefix_log="$tmp_root/override-prefix.log"
prefix_status="$(cd "$staged_repo" && run_capture "$prefix_log" env BUBBLES_BATCH_PROMOTION_OVERRIDE="bob:$future_expiry:$short_sha" BUBBLES_BATCH_PROMOTION_LEDGER="$ledger_file" bash "$LINT_SCRIPT" --staged --max=1)"
if [[ "$prefix_status" -eq 0 ]]; then
  pass "Prefix (short) sha override token is honored"
else
  fail "Prefix (short) sha override token should be honored"
  sed -n '1,200p' "$prefix_log"
fi

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "batch-promotion-lint selftest failed with $failures issue(s)."
  exit 1
fi

echo "batch-promotion-lint selftest passed."
