#!/usr/bin/env bash
#
# Bubbles v6.0 / B9 — generate-installer.sh selftest.
#
# Adversarial fixtures verify that the checker's invariants and
# marker checks would catch each bug class declared in
# bubbles/installer/installer.yaml.
#
# All fixtures live under $HOME/.cache/bubbles-installer-selftest/
# (snap-confined yq compatibility — /tmp is off-limits).

set -euo pipefail

REPO_ROOT="${BUBBLES_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
SCRIPT="${REPO_ROOT}/bubbles/scripts/generate-installer.sh"
MANIFEST="${REPO_ROOT}/bubbles/installer/installer.yaml"
REAL_INSTALL_SH="${REPO_ROOT}/install.sh"

FIXTURE_ROOT="${HOME}/.cache/bubbles-installer-selftest"
rm -rf "$FIXTURE_ROOT"
mkdir -p "$FIXTURE_ROOT"

declare -i pass_count=0
declare -i fail_count=0

pass() { echo "PASS: $*"; pass_count=$((pass_count + 1)); }
bad()  { echo "FAIL: $*"; fail_count=$((fail_count + 1)); }

portable_sed_inplace() {
  local expression="$1"
  local file_path="$2"
  local temp_file
  temp_file="$(mktemp)"
  sed "$expression" "$file_path" > "$temp_file"
  mv "$temp_file" "$file_path"
}

# ── Assertion 1: real install.sh + manifest passes ───────────────
if bash "$SCRIPT" >"$FIXTURE_ROOT/real.log" 2>&1; then
  pass "real install.sh + manifest checker exits 0"
else
  bad "real install.sh checker did NOT exit 0; tail:"
  tail -10 "$FIXTURE_ROOT/real.log"
fi

# Helper: copy the repo into a fixture and run the checker there
fixture_run() {
  local fixture_name="$1"
  local fixture_dir="$FIXTURE_ROOT/$fixture_name"
  mkdir -p "$fixture_dir/bubbles/installer" "$fixture_dir/bubbles/scripts"
  cp "$MANIFEST" "$fixture_dir/bubbles/installer/installer.yaml"
  cp "$SCRIPT" "$fixture_dir/bubbles/scripts/generate-installer.sh"
  chmod +x "$fixture_dir/bubbles/scripts/generate-installer.sh"
  cp "$REAL_INSTALL_SH" "$fixture_dir/install.sh"
  echo "$fixture_dir"
}

# ── Assertion 2: removing a step marker => FAIL ──────────────────
fix2="$(fixture_run mutation-remove-marker)"
portable_sed_inplace 's/Installing governance scripts/REPLACED_MARKER/g' "$fix2/install.sh"
if BUBBLES_REPO_ROOT="$fix2" bash "$fix2/bubbles/scripts/generate-installer.sh" >"$fix2/check.log" 2>&1; then
  bad "mutated install.sh (removed install_scripts marker) was NOT rejected"
else
  if grep -qF 'install_scripts marker missing' "$fix2/check.log"; then
    pass "removing install_scripts marker -> checker FAILs with explicit marker-missing message"
  else
    bad "removing install_scripts marker -> checker FAILed but did not name the step"
  fi
fi

# ── Assertion 3: wrong gitignore root (regression for ce01576) => FAIL ─
fix3="$(fixture_run mutation-wrong-gitignore-root)"
# Inject a fake write into ${TARGET}/.gitignore alongside the real one
portable_sed_inplace '/printf.*Bubbles framework-health proposals/a\
printf "improvements/" >> "${TARGET}/.gitignore"' "$fix3/install.sh"
if BUBBLES_REPO_ROOT="$fix3" bash "$fix3/bubbles/scripts/generate-installer.sh" >"$fix3/check.log" 2>&1; then
  bad "mutated install.sh (gitignore written to wrong root) was NOT rejected"
else
  if grep -qF 'I1 gitignore_root_is_repo_root' "$fix3/check.log"; then
    pass "writing improvements/ to \${TARGET}/.gitignore -> I1 FAILs (closes bug ce01576)"
  else
    bad "wrong gitignore root -> checker FAILed but did not flag I1"
  fi
fi

# ── Assertion 4: removing chmod +x on scripts => FAIL ────────────
fix4="$(fixture_run mutation-no-chmod-scripts)"
portable_sed_inplace 's|chmod +x "${TARGET}"/bubbles/scripts/\*\.sh|# chmod removed by mutation|' "$fix4/install.sh"
if BUBBLES_REPO_ROOT="$fix4" bash "$fix4/bubbles/scripts/generate-installer.sh" >"$fix4/check.log" 2>&1; then
  bad "mutated install.sh (no scripts chmod) was NOT rejected"
else
  if grep -qF 'I2 scripts_are_chmod_x' "$fix4/check.log"; then
    pass "removing chmod +x on bubbles/scripts -> I2 FAILs"
  else
    bad "no scripts chmod -> checker FAILed but did not flag I2"
  fi
fi

# ── Assertion 5: removing chmod +x on adapters => FAIL ───────────
fix5="$(fixture_run mutation-no-chmod-adapters)"
portable_sed_inplace 's|find "${TARGET}/bubbles/adapters" -type f -name|# find removed by mutation|' "$fix5/install.sh"
if BUBBLES_REPO_ROOT="$fix5" bash "$fix5/bubbles/scripts/generate-installer.sh" >"$fix5/check.log" 2>&1; then
  bad "mutated install.sh (no adapters chmod) was NOT rejected"
else
  if grep -qF 'I3 adapter_files_are_chmod_x' "$fix5/check.log"; then
    pass "removing chmod +x on bubbles/adapters -> I3 FAILs"
  else
    bad "no adapters chmod -> checker FAILed but did not flag I3"
  fi
fi

# ── Assertion 6: removing a provenance field => FAIL ─────────────
fix6="$(fixture_run mutation-no-source-dirty-field)"
portable_sed_inplace '/"sourceDirty":/d' "$fix6/install.sh"
if BUBBLES_REPO_ROOT="$fix6" bash "$fix6/bubbles/scripts/generate-installer.sh" >"$fix6/check.log" 2>&1; then
  bad "mutated install.sh (missing sourceDirty field) was NOT rejected"
else
  if grep -qF 'I5 provenance_records_required_fields' "$fix6/check.log"; then
    pass "removing sourceDirty from provenance heredoc -> I5 FAILs"
  else
    bad "missing sourceDirty -> checker FAILed but did not flag I5"
  fi
fi

# ── Assertion 7: missing install.sh => exit 2 ────────────────────
fix8="$FIXTURE_ROOT/mutation-no-install-sh"
mkdir -p "$fix8/bubbles/installer" "$fix8/bubbles/scripts"
cp "$MANIFEST" "$fix8/bubbles/installer/installer.yaml"
cp "$SCRIPT" "$fix8/bubbles/scripts/generate-installer.sh"
chmod +x "$fix8/bubbles/scripts/generate-installer.sh"
# Note: NO install.sh in this fixture
if BUBBLES_REPO_ROOT="$fix8" bash "$fix8/bubbles/scripts/generate-installer.sh" >"$fix8/check.log" 2>&1; then
  bad "checker did not exit non-zero when install.sh is missing"
else
  exit_code=$?
  if [[ $exit_code -eq 2 ]]; then
    pass "missing install.sh -> checker exits 2 (manifest/installer-source error)"
  else
    bad "missing install.sh -> checker exited $exit_code (expected 2)"
  fi
fi

echo
echo "generate-installer-selftest: $pass_count pass, $fail_count fail"

if [[ $fail_count -gt 0 ]]; then
  exit 1
fi
exit 0
