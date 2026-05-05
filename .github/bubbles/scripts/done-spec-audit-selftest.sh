#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_SCRIPT="$SCRIPT_DIR/done-spec-audit.sh"

tmp_root="$(mktemp -d)"
failures=0

cleanup() {
  if [[ "$failures" -eq 0 && "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
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

assert_status() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  local log_file="$4"

  if [[ "$actual" -eq "$expected" ]]; then
    pass "$label"
  else
    fail "$label"
    echo "Expected status $expected, got $actual"
    sed -n '1,240p' "$log_file"
  fi
}

assert_log_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq -- "$needle" "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- log: $log_file ---"
    sed -n '1,240p' "$log_file"
    echo "--- end log ---"
  fi
}

assert_file_contains() {
  local target_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq -- "$needle" "$target_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- file: $target_file ---"
    sed -n '1,120p' "$target_file"
    echo "--- end file ---"
  fi
}

assert_file_not_contains() {
  local target_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq -- "$needle" "$target_file"; then
    fail "$label"
    echo "Unexpected text found: $needle"
    echo "--- file: $target_file ---"
    sed -n '1,260p' "$target_file"
    echo "--- end file ---"
  else
    pass "$label"
  fi
}

install_fixture_scripts() {
  local repo_dir="$1"
  mkdir -p "$repo_dir/bubbles/scripts"
  cp "$SOURCE_SCRIPT" "$repo_dir/bubbles/scripts/done-spec-audit.sh"

  cat > "$repo_dir/bubbles/scripts/fun-mode.sh" <<'EOF'
#!/usr/bin/env bash
fun_message() { :; }
EOF

  cat > "$repo_dir/bubbles/scripts/artifact-lint.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
spec_dir="${1:?spec dir required}"
if [[ -f "$spec_dir/lint.fail" ]]; then
  echo "fixture lint failure: $spec_dir"
  exit 1
fi
echo "fixture lint pass: $spec_dir"
EOF

  cat > "$repo_dir/bubbles/scripts/state-transition-guard.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
spec_dir="${1:?spec dir required}"
if [[ -f "$spec_dir/guard.fail" ]]; then
  echo "fixture guard failure: $spec_dir"
  exit 1
fi
echo "fixture guard pass: $spec_dir"
EOF

  cat > "$repo_dir/bubbles/scripts/traceability-guard.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
spec_dir="${1:?spec dir required}"
if [[ -f "$spec_dir/trace.fail" ]]; then
  echo "fixture traceability failure: $spec_dir"
  exit 1
fi
echo "fixture traceability pass: $spec_dir"
EOF

  chmod +x "$repo_dir/bubbles/scripts/"*.sh
}

write_state() {
  local repo_dir="$1"
  local spec_path="$2"
  local status_value="$3"

  mkdir -p "$repo_dir/$spec_path"
  cat > "$repo_dir/$spec_path/state.json" <<EOF
{
  "status": "$status_value",
  "execution": {
    "currentPhase": "validate"
  },
  "notes": "fixture",
  "lastUpdatedAt": "2026-05-05T00:00:00Z"
}
EOF
}

init_fixture_repo() {
  local repo_dir="$1"
  mkdir -p "$repo_dir"
  git -C "$repo_dir" -c init.defaultBranch=main init >/dev/null
  git -C "$repo_dir" config user.email selftest@example.com
  git -C "$repo_dir" config user.name selftest
  install_fixture_scripts "$repo_dir"
}

echo "Running done-spec-audit selftest..."

assert_file_contains "$SCRIPT_DIR/cli.sh" "guard-changed-done-specs" "Hook catalog names the changed done-spec guard"
assert_file_contains "$SCRIPT_DIR/cli.sh" "done-spec-audit.sh --profile changed" "Generated pre-push hook uses changed-profile done-spec audit"
assert_file_not_contains "$SCRIPT_DIR/cli.sh" "validating done specs" "Generated pre-push hook no longer advertises all done-spec validation"
assert_file_not_contains "$SCRIPT_DIR/cli.sh" "find specs -maxdepth 2 -name \"state.json\"" "Generated pre-push hook no longer scans every historical state.json"

advisory_repo="$tmp_root/advisory-repo"
init_fixture_repo "$advisory_repo"
write_state "$advisory_repo" "specs/001-historical" "done"
touch "$advisory_repo/specs/001-historical/lint.fail"
git -C "$advisory_repo" add .
git -C "$advisory_repo" commit -m "fixture" >/dev/null
advisory_log="$tmp_root/advisory.log"
advisory_status="$(cd "$advisory_repo" && run_capture "$advisory_log" bash bubbles/scripts/done-spec-audit.sh)"
assert_status "$advisory_status" 0 "Default advisory profile exits 0 for historical failures" "$advisory_log"
assert_log_contains "$advisory_log" "profile: advisory" "Default profile reports advisory"
assert_log_contains "$advisory_log" "Historical advisory findings" "Historical failures are advisory findings"
assert_log_contains "$advisory_log" "grandfathered until touched" "Advisory output explains grandfathering"

changed_repo="$tmp_root/changed-repo"
init_fixture_repo "$changed_repo"
write_state "$changed_repo" "specs/002-changed" "done"
touch "$changed_repo/specs/002-changed/guard.fail"
git -C "$changed_repo" add .
git -C "$changed_repo" commit -m "fixture" >/dev/null
changed_log="$tmp_root/changed.log"
changed_status="$(cd "$changed_repo" && run_capture "$changed_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/002-changed)"
assert_status "$changed_status" 1 "Changed profile blocks changed done-spec failures" "$changed_log"
assert_log_contains "$changed_log" "profile: changed" "Changed profile is reported"
assert_log_contains "$changed_log" "Current-policy failures" "Changed failures are current-policy failures"

fix_guard_log="$tmp_root/fix-guard.log"
fix_guard_status="$(cd "$changed_repo" && run_capture "$fix_guard_log" bash bubbles/scripts/done-spec-audit.sh --fix specs/002-changed)"
assert_status "$fix_guard_status" 2 "Deprecated --fix is blocked without explicit recertification" "$fix_guard_log"
assert_log_contains "$fix_guard_log" "requires explicit historical recertification" "Fix guard explains required flags"
assert_file_contains "$changed_repo/specs/002-changed/state.json" '"status": "done"' "Blocked --fix does not reopen state"

recert_repo="$tmp_root/recert-repo"
init_fixture_repo "$recert_repo"
write_state "$recert_repo" "specs/003-recert" "done"
touch "$recert_repo/specs/003-recert/trace.fail"
git -C "$recert_repo" add .
git -C "$recert_repo" commit -m "fixture" >/dev/null
recert_log="$tmp_root/recert.log"
recert_status="$(cd "$recert_repo" && run_capture "$recert_log" bash bubbles/scripts/done-spec-audit.sh --recertify-all --reopen-failing)"
assert_status "$recert_status" 1 "Recertification reopen exits nonzero when failures were found" "$recert_log"
assert_log_contains "$recert_log" "profile: recertification" "Recertification profile is reported"
assert_log_contains "$recert_log" "REOPENED" "Explicit reopen reports mutation"
assert_file_contains "$recert_repo/specs/003-recert/state.json" '"status": "in_progress"' "Explicit reopen mutates failing done spec"

empty_changed_repo="$tmp_root/empty-changed-repo"
init_fixture_repo "$empty_changed_repo"
write_state "$empty_changed_repo" "specs/004-unchanged" "done"
git -C "$empty_changed_repo" add .
git -C "$empty_changed_repo" commit -m "fixture" >/dev/null
empty_changed_log="$tmp_root/empty-changed.log"
empty_changed_status="$(cd "$empty_changed_repo" && run_capture "$empty_changed_log" bash bubbles/scripts/done-spec-audit.sh --profile changed)"
assert_status "$empty_changed_status" 0 "Changed profile exits 0 when no changed specs are detected" "$empty_changed_log"
assert_log_contains "$empty_changed_log" "No changed spec directories" "Changed no-op is explicit"

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "done-spec-audit selftest failed with $failures issue(s)."
  exit 1
fi

echo "done-spec-audit selftest passed."
