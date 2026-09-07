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
printf '%s\n' "$*" >"$spec_dir/trace.argv"
if [[ "$#" -ne 3 || "$2" != "--all-scopes" || "$3" != "--coverage-policy=authored" ]]; then
  echo "fixture traceability argv mismatch: $*"
  exit 64
fi
if [[ -f "$spec_dir/trace.exit2" ]]; then
  echo "fixture traceability usage failure: $spec_dir"
  exit 2
fi
if [[ -f "$spec_dir/trace.exit3" ]]; then
  echo "fixture traceability contract failure: $spec_dir"
  exit 3
fi
if [[ -f "$spec_dir/authored.packet" ]]; then
  printf '%s\n' authored >"$spec_dir/trace.result"
  echo "fixture authored coverage accepts authored packet: $spec_dir"
  exit 0
fi
if [[ -f "$spec_dir/planned-only.packet" ]]; then
  printf '%s\n' planned >"$spec_dir/trace.result"
  echo "fixture authored coverage rejects planned-only packet: $spec_dir"
  exit 1
fi
if [[ -f "$spec_dir/trace.fail" ]]; then
  echo "fixture traceability failure: $spec_dir"
  exit 1
fi
printf '%s\n' unclassified >"$spec_dir/trace.result"
echo "fixture authored coverage rejects unclassified packet: $spec_dir"
exit 1
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

planned_only_repo="$tmp_root/planned-only-repo"
init_fixture_repo "$planned_only_repo"
write_state "$planned_only_repo" "specs/005-planned-only" "done"
touch "$planned_only_repo/specs/005-planned-only/planned-only.packet"
git -C "$planned_only_repo" add .
git -C "$planned_only_repo" commit -m "fixture" >/dev/null
planned_only_log="$tmp_root/planned-only.log"
planned_only_status="$(cd "$planned_only_repo" && run_capture "$planned_only_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/005-planned-only)"
assert_status "$planned_only_status" 1 "Done-spec authored policy rejects a planned-only packet" "$planned_only_log"
assert_log_contains "$planned_only_log" "fixture authored coverage rejects planned-only packet" "Planned-only marker determines authored-policy rejection"
assert_file_contains "$planned_only_repo/specs/005-planned-only/trace.argv" "specs/005-planned-only --all-scopes --coverage-policy=authored" "Done-spec audit passes exact all-scope authored-policy argv"

authored_repo="$tmp_root/authored-repo"
init_fixture_repo "$authored_repo"
write_state "$authored_repo" "specs/006-authored" "done"
touch "$authored_repo/specs/006-authored/authored.packet"
git -C "$authored_repo" add .
git -C "$authored_repo" commit -m "fixture" >/dev/null
authored_log="$tmp_root/authored.log"
authored_status="$(cd "$authored_repo" && run_capture "$authored_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/006-authored)"
assert_status "$authored_status" 0 "Done-spec authored policy accepts an authored packet" "$authored_log"
assert_file_contains "$authored_repo/specs/006-authored/trace.result" "authored" "Authored marker determines authored-policy acceptance"
assert_file_contains "$authored_repo/specs/006-authored/trace.argv" "specs/006-authored --all-scopes --coverage-policy=authored" "Authored packet uses the exact traceability argv"

unclassified_repo="$tmp_root/unclassified-repo"
init_fixture_repo "$unclassified_repo"
write_state "$unclassified_repo" "specs/008-unclassified" "done"
git -C "$unclassified_repo" add .
git -C "$unclassified_repo" commit -m "fixture" >/dev/null
unclassified_log="$tmp_root/unclassified.log"
unclassified_status="$(cd "$unclassified_repo" && run_capture "$unclassified_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/008-unclassified)"
assert_status "$unclassified_status" 1 "Done-spec authored policy rejects an unclassified packet" "$unclassified_log"
assert_log_contains "$unclassified_log" "fixture authored coverage rejects unclassified packet" "Unclassified authored-policy rejection is explicit"

marker_removed_repo="$tmp_root/marker-removed-repo"
init_fixture_repo "$marker_removed_repo"
write_state "$marker_removed_repo" "specs/009-marker-removed" "done"
touch "$marker_removed_repo/specs/009-marker-removed/authored.packet"
git -C "$marker_removed_repo" add .
git -C "$marker_removed_repo" commit -m "fixture" >/dev/null
rm "$marker_removed_repo/specs/009-marker-removed/authored.packet"
marker_removed_log="$tmp_root/marker-removed.log"
marker_removed_status="$(cd "$marker_removed_repo" && run_capture "$marker_removed_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/009-marker-removed)"
assert_status "$marker_removed_status" 1 "Removing authored.packet makes authored coverage fail" "$marker_removed_log"
assert_log_contains "$marker_removed_log" "fixture authored coverage rejects unclassified packet" "Authored marker removal is mutation-sensitive"

policy_reversal_repo="$tmp_root/policy-reversal-repo"
init_fixture_repo "$policy_reversal_repo"
write_state "$policy_reversal_repo" "specs/010-policy-reversal" "done"
touch "$policy_reversal_repo/specs/010-policy-reversal/authored.packet"
git -C "$policy_reversal_repo" add .
git -C "$policy_reversal_repo" commit -m "fixture" >/dev/null
sed 's/--coverage-policy=authored/--coverage-policy=planning/' \
  "$policy_reversal_repo/bubbles/scripts/done-spec-audit.sh" >"$policy_reversal_repo/bubbles/scripts/done-spec-audit.mutated"
mv "$policy_reversal_repo/bubbles/scripts/done-spec-audit.mutated" "$policy_reversal_repo/bubbles/scripts/done-spec-audit.sh"
policy_reversal_log="$tmp_root/policy-reversal.log"
policy_reversal_status="$(cd "$policy_reversal_repo" && run_capture "$policy_reversal_log" bash bubbles/scripts/done-spec-audit.sh --profile changed specs/010-policy-reversal)"
assert_status "$policy_reversal_status" 1 "Reversing done-spec coverage policy makes the caller fail" "$policy_reversal_log"
assert_log_contains "$policy_reversal_log" "fixture traceability argv mismatch" "Authored-policy reversal is mutation-sensitive"

for trace_exit in 2 3; do
  trace_error_repo="$tmp_root/trace-exit-$trace_exit-repo"
  init_fixture_repo "$trace_error_repo"
  write_state "$trace_error_repo" "specs/007-trace-exit-$trace_exit" "done"
  touch "$trace_error_repo/specs/007-trace-exit-$trace_exit/trace.exit$trace_exit"
  git -C "$trace_error_repo" add .
  git -C "$trace_error_repo" commit -m "fixture" >/dev/null
  trace_error_log="$tmp_root/trace-exit-$trace_exit.log"
  trace_error_status="$(cd "$trace_error_repo" && run_capture "$trace_error_log" bash bubbles/scripts/done-spec-audit.sh --profile changed "specs/007-trace-exit-$trace_exit")"
  assert_status "$trace_error_status" 1 "Traceability exit $trace_exit is never treated as done-spec success" "$trace_error_log"
  assert_log_contains "$trace_error_log" "Traceability: FAILED" "Traceability exit $trace_exit is reported as failed"
done

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
