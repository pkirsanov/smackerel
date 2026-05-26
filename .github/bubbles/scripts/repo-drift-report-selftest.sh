#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for SCOPE-14 repo drift report visibility.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT="$SCRIPT_DIR/repo-drift-report.sh"
FRAMEWORK_VALIDATE="$SCRIPT_DIR/framework-validate.sh"

if [[ ! -f "$REPORT" ]]; then
  echo "repo-drift-report-selftest: report script not found at $REPORT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-scope14-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

ok() { PASS_COUNT=$((PASS_COUNT + 1)); printf '  PASS: %s\n' "$*"; }
ko() { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_SCENARIOS+=("$*"); printf '  FAIL: %s\n' "$*"; }

stage_repo() {
  local sid="$1"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo/specs" "$repo/src"
  git -C "$repo" -c init.defaultBranch=main init >/dev/null
  git -C "$repo" config user.email selftest@example.com
  git -C "$repo" config user.name selftest
  printf '%s' "$repo"
}

stage_repo_without_specs() {
  local sid="$1"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo/src"
  printf 'fixture\n' > "$repo/README.md"
  git -C "$repo" -c init.defaultBranch=main init >/dev/null
  git -C "$repo" config user.email selftest@example.com
  git -C "$repo" config user.name selftest
  printf '%s' "$repo"
}

commit_all() {
  local repo="$1"
  local when="$2"
  local message="$3"
  git -C "$repo" add .
  GIT_AUTHOR_DATE="$when" GIT_COMMITTER_DATE="$when" git -C "$repo" commit -m "$message" >/dev/null
}

write_state() {
  local repo="$1"
  local spec="$2"
  local status="$3"
  local certified_at_json="$4"
  local requires_revalidation="$5"
  local extra_json="$6"
  mkdir -p "$repo/$spec"
  cat > "$repo/$spec/state.json" <<EOF
{
  "version": 3,
  "featureDir": "$spec",
  "featureName": "$spec",
  "status": "$status",
  "workflowMode": "full-delivery",
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": null,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": [],
  "certifiedAt": $certified_at_json,
  "requiresRevalidation": $requires_revalidation,
  "executionHistory": []$extra_json
}
EOF
}

write_planning_files() {
  local repo="$1"
  local spec="$2"
  mkdir -p "$repo/$spec/scopes/01-example"
  cat > "$repo/$spec/spec.md" <<'EOF'
# Fixture Spec

Initial spec truth.
EOF
  cat > "$repo/$spec/design.md" <<'EOF'
# Fixture Design

Initial design truth.
EOF
  cat > "$repo/$spec/scopes/_index.md" <<'EOF'
# Fixture Scope Index

| # | Scope | Status |
|---|-------|--------|
| 01 | example | Done |
EOF
  cat > "$repo/$spec/scopes/01-example/scope.md" <<'EOF'
# Fixture Scope

**Status:** Done
EOF
}

run_report() {
  local repo="$1"
  set +e
  bash "$REPORT" --repo-root "$repo" --now "2026-05-24T00:00:00Z" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

assert_exit() {
  local label="$1"
  local expected="$2"
  local actual
  actual="$(cat "$WORKSPACE/exit.last")"
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label exit=$actual"
  else
    ko "$label expected exit=$expected actual=$actual"
    cat "$WORKSPACE/stdout.last"
    cat "$WORKSPACE/stderr.last"
  fi
}

assert_stdout_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last"; then
    ok "$label stdout contains '$needle'"
  else
    ko "$label stdout missing '$needle'"
    cat "$WORKSPACE/stdout.last"
  fi
}

assert_stdout_not_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label stdout unexpectedly contains '$needle'"
    cat "$WORKSPACE/stdout.last"
  else
    ok "$label stdout does not contain '$needle'"
  fi
}

echo "=== repo-drift-report-selftest (SCOPE-14) ==="

echo ""
echo "--- S0: missing specs directory is a clean source-repo no-op ---"
repo="$(stage_repo_without_specs s0-no-specs)"
commit_all "$repo" "2026-05-01T00:00:00Z" "source repo without specs"
run_report "$repo"
assert_exit "S0 no specs report" 0
assert_stdout_contains "S0" "No specs directory found under repository root"
assert_stdout_contains "S0" "expected for the Bubbles source repository"

echo ""
echo "--- S1: orphan planning packets older than 30 days are reported ---"
repo="$(stage_repo s1-orphan)"
write_state "$repo" "specs/100-orphan-packet" "specs_hardened" '"2026-04-01T00:00:00Z"' "false" ""
commit_all "$repo" "2026-04-01T00:00:00Z" "old hardened planning packet"
run_report "$repo"
assert_exit "S1 orphan packet report" 0
assert_stdout_contains "S1" "| orphan-planning-packet | specs/100-orphan-packet |"
assert_stdout_contains "S1" "linkedImplementationSpec missing"

echo ""
echo "--- S1b: fresh orphan planning packets are not reported before age threshold ---"
repo="$(stage_repo s1b-fresh-orphan)"
write_state "$repo" "specs/101-fresh-packet" "specs_hardened" '"2026-05-20T00:00:00Z"' "false" ""
commit_all "$repo" "2026-05-20T00:00:00Z" "fresh planning packet"
run_report "$repo"
assert_exit "S1b fresh orphan report" 0
assert_stdout_not_contains "S1b" "orphan-planning-packet"

echo ""
echo "--- S2: done specs with post-cert planning edits are reported ---"
repo="$(stage_repo s2-planning-edit)"
write_state "$repo" "specs/200-done-planning-edit" "done" '"2026-05-01T00:00:00Z"' "false" ""
write_planning_files "$repo" "specs/200-done-planning-edit"
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline done planning spec"
printf '\nPost-cert planning edit.\n' >> "$repo/specs/200-done-planning-edit/design.md"
commit_all "$repo" "2026-05-10T00:00:00Z" "post-cert design edit"
run_report "$repo"
assert_exit "S2 post-cert planning report" 0
assert_stdout_contains "S2" "| post-cert-planning-edit | specs/200-done-planning-edit |"
assert_stdout_contains "S2" "changedFamily=design.md"

echo ""
echo "--- S3: done specs with post-cert source edits referenced by report.md are reported ---"
repo="$(stage_repo s3-source-edit)"
write_state "$repo" "specs/300-done-source-edit" "done" '"2026-05-01T00:00:00Z"' "false" ""
write_planning_files "$repo" "specs/300-done-source-edit"
cat > "$repo/src/source.sh" <<'EOF'
#!/usr/bin/env bash
echo baseline
EOF
cat > "$repo/specs/300-done-source-edit/report.md" <<'EOF'
# Report

## Code Diff Evidence

Changed source path: src/source.sh
EOF
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline done source spec"
printf 'echo changed\n' >> "$repo/src/source.sh"
commit_all "$repo" "2026-05-10T00:00:00Z" "post-cert source edit"
run_report "$repo"
assert_exit "S3 post-cert source report" 0
assert_stdout_contains "S3" "| post-cert-source-edit | specs/300-done-source-edit |"
assert_stdout_contains "S3" "sourceFile=src/source.sh"
assert_stdout_contains "S3" "referencedIn=Code Diff Evidence"

echo ""
echo "--- S4: requiresRevalidation specs are reported with reason and dependencies ---"
repo="$(stage_repo s4-revalidation)"
write_state "$repo" "specs/010-dependency" "done" '"2026-05-01T00:00:00Z"' "false" ""
write_state "$repo" "specs/400-revalidation" "in_progress" 'null' "true" ', "specDependsOn": ["specs/010-dependency"], "revalidationReason": "dependency demoted in review"'
commit_all "$repo" "2026-05-02T00:00:00Z" "revalidation fixture"
run_report "$repo"
assert_exit "S4 revalidation report" 0
assert_stdout_contains "S4" "| requires-revalidation | specs/400-revalidation |"
assert_stdout_contains "S4" "reason=dependency demoted in review"
assert_stdout_contains "S4" "dependencies=specs/010-dependency"

echo ""
echo "--- S4b: invalid dependencies reuse G089 diagnostics in the report ---"
repo="$(stage_repo s4b-dependency)"
write_state "$repo" "specs/500-bad-dependency" "in_progress" 'null' "false" ', "specDependsOn": ["specs/999-missing"]'
commit_all "$repo" "2026-05-02T00:00:00Z" "bad dependency fixture"
run_report "$repo"
assert_exit "S4b dependency report" 0
assert_stdout_contains "S4b" "| dependency-revalidation | specs/500-bad-dependency |"
assert_stdout_contains "S4b" "G089 dependency finding"
assert_stdout_contains "S4b" "specs/999-missing"

echo ""
echo "--- S5: framework validation prints report non-blockingly ---"
repo="$(stage_repo s5-framework-validate)"
mkdir -p "$repo/bubbles/scripts" "$repo/agents"
cp "$FRAMEWORK_VALIDATE" "$repo/bubbles/scripts/framework-validate.sh"
cp "$REPORT" "$repo/bubbles/scripts/repo-drift-report.sh"
chmod +x "$repo/bubbles/scripts/framework-validate.sh" "$repo/bubbles/scripts/repo-drift-report.sh"
while IFS= read -r script_name; do
  [[ -n "$script_name" ]] || continue
  case "$script_name" in
    framework-validate.sh|repo-drift-report.sh) continue ;;
  esac
  cat > "$repo/bubbles/scripts/$script_name" <<'EOF'
#!/usr/bin/env bash
echo "stub: $0"
exit 0
EOF
  chmod +x "$repo/bubbles/scripts/$script_name"
done < <(grep -Eo '\$SCRIPT_DIR/[A-Za-z0-9._-]+[.]sh' "$FRAMEWORK_VALIDATE" | sed 's|\$SCRIPT_DIR/||' | sort -u)
write_state "$repo" "specs/100-orphan-packet" "specs_hardened" '"2026-04-01T00:00:00Z"' "false" ""
commit_all "$repo" "2026-04-01T00:00:00Z" "framework validate fixture"
set +e
bash "$repo/bubbles/scripts/framework-validate.sh" > "$WORKSPACE/framework-stdout.last" 2> "$WORKSPACE/framework-stderr.last"
framework_rc=$?
set -e
cp "$WORKSPACE/framework-stdout.last" "$WORKSPACE/stdout.last"
cp "$WORKSPACE/framework-stderr.last" "$WORKSPACE/stderr.last"
echo "$framework_rc" > "$WORKSPACE/exit.last"
assert_exit "S5 framework validate non-blocking" 0
assert_stdout_contains "S5" "Repository drift report (informational)"
assert_stdout_contains "S5" "# Repository Drift Report"
assert_stdout_contains "S5" "| orphan-planning-packet | specs/100-orphan-packet |"
assert_stdout_contains "S5" "Framework validation passed."

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "repo-drift-report-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "repo-drift-report-selftest: PASSED"
exit 0