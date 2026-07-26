#!/usr/bin/env bash
#
# Bubbles v6.0 / B9 — installer manifest checker.
#
# Mode (single, default): --check
#   Parses bubbles/installer/installer.yaml and verifies that
#   install.sh structurally implements every declared step.
#
# Exit codes:
#   0 = PASS (every required step's marker found; every invariant holds)
#   1 = FAIL (one or more steps or invariants violated)
#   2 = manifest parse error or install.sh missing
#
# This script is a STRUCTURAL CHECK only. It does not generate
# install.sh from the manifest in v6.0; that flip is deferred to a
# future increment. The goal in v6.0 is to make adapter/gitignore /
# missing-chmod / missing-step bug classes structurally impossible
# by failing framework-validate the moment install.sh deviates from
# the typed manifest.

set -euo pipefail

REPO_ROOT="${BUBBLES_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
INSTALLER_YAML="${REPO_ROOT}/bubbles/installer/installer.yaml"
INSTALL_SH="${REPO_ROOT}/install.sh"

if [[ $# -gt 0 ]]; then
  case "$1" in
    --check) shift ;;
    -h|--help)
      cat <<EOF
Usage: bash bubbles/scripts/generate-installer.sh [--check]

Verifies that install.sh implements every step declared in
bubbles/installer/installer.yaml.
EOF
      exit 0
      ;;
    *) echo "generate-installer.sh: unknown argument: $1" >&2; exit 2 ;;
  esac
fi

# ── Sanity ───────────────────────────────────────────────────────
[[ -f "$INSTALLER_YAML" ]] || { echo "generate-installer.sh: missing $INSTALLER_YAML" >&2; exit 2; }
[[ -f "$INSTALL_SH" ]] || { echo "generate-installer.sh: missing $INSTALL_SH" >&2; exit 2; }

declare -i fail_count=0
declare -i step_count=0

emit_fail() {
  echo "FAIL: $*"
  fail_count=$((fail_count + 1))
}

emit_pass() {
  echo "PASS: $*"
}

# ── Parse the manifest ───────────────────────────────────────────
# Pure bash parser — extract a list of (name, marker, required, type)
# tuples plus required-artifact mappings. The YAML shape is constrained so a
# simple awk script suffices.

mapfile -t parsed_lines < <(awk '
  function emit_step() {
    if (current_name != "") {
      printf "STEP\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", current_name, current_marker, current_required, current_type, current_source_dir, current_glob, current_source_file
      current_name=""
    }
  }
  function emit_artifact() {
    if (artifact_path != "") {
      printf "ARTIFACT\t%s\t%s\n", artifact_path, artifact_step
      artifact_path=""
    }
  }
  function clean_value(value) {
    gsub(/^[\47"]|[\47"]$/, "", value)
    return value
  }
  /^steps:/ { in_steps=1; in_artifacts=0; in_inv=0; next }
  /^required_artifacts:/ {
    emit_step()
    in_steps=0; in_artifacts=1; in_inv=0; next
  }
  /^invariants:/ {
    emit_step()
    emit_artifact()
    in_steps=0; in_artifacts=0; in_inv=1; next
  }
  in_steps && /^[[:space:]]*- name:[[:space:]]+/ {
    emit_step()
    line=$0
    sub(/^[[:space:]]*- name:[[:space:]]+/, "", line)
    current_name=clean_value(line); current_marker=""; current_required="true"; current_type=""
    current_source_dir="-"; current_glob="-"; current_source_file="-"
    next
  }
  in_steps && /^[[:space:]]+type:[[:space:]]+/ { current_type=$2; next }
  in_steps && /^[[:space:]]+source_dir:[[:space:]]+/ { current_source_dir=clean_value($2); next }
  in_steps && /^[[:space:]]+glob:[[:space:]]+/ { current_glob=clean_value($2); next }
  in_steps && /^[[:space:]]+source_file:[[:space:]]+/ { current_source_file=clean_value($2); next }
  in_steps && /^[[:space:]]+marker:[[:space:]]+/ {
    line=$0
    sub(/^[[:space:]]+marker:[[:space:]]+/, "", line)
    current_marker=clean_value(line)
    next
  }
  in_steps && /^[[:space:]]+required:[[:space:]]+/ { current_required=$2; next }
  in_artifacts && /^[[:space:]]*- path:[[:space:]]+/ {
    emit_artifact()
    line=$0
    sub(/^[[:space:]]*- path:[[:space:]]+/, "", line)
    artifact_path=clean_value(line)
    artifact_step=""
    next
  }
  in_artifacts && /^[[:space:]]+install_step:[[:space:]]+/ {
    artifact_step=clean_value($2)
    next
  }
  in_inv && /^[[:space:]]*- id:[[:space:]]+/ {
    line=$0
    sub(/^[[:space:]]*- id:[[:space:]]+/, "", line)
    if (current_inv != "") {
      printf "INV\t%s\n", current_inv
    }
    current_inv=clean_value(line)
    next
  }
  END {
    emit_step()
    emit_artifact()
    if (current_inv != "") {
      printf "INV\t%s\n", current_inv
    }
  }
' "$INSTALLER_YAML")

declare -a steps=()
declare -a required_artifacts=()
declare -a invariants=()
for ln in "${parsed_lines[@]}"; do
  case "$ln" in
    STEP$'\t'*) steps+=("${ln#STEP$'\t'}") ;;
    ARTIFACT$'\t'*) required_artifacts+=("${ln#ARTIFACT$'\t'}") ;;
    INV$'\t'*) invariants+=("${ln#INV$'\t'}") ;;
  esac
done

step_count=${#steps[@]}

if [[ $step_count -eq 0 ]]; then
  echo "generate-installer.sh: no steps parsed from manifest" >&2
  exit 2
fi

# ── Step check: every required step's marker appears in install.sh ─
for stuple in "${steps[@]}"; do
  IFS=$'\t' read -r sname smarker srequired _ _ _ _ <<<"$stuple"
  if [[ "$srequired" != "true" ]]; then
    continue
  fi
  if [[ -z "$smarker" ]]; then
    emit_fail "Step $sname has no marker declared"
    continue
  fi
  if grep -qF -- "$smarker" "$INSTALL_SH"; then
    emit_pass "Step $sname marker present: $smarker"
  else
    emit_fail "Step $sname marker missing from install.sh: $smarker (bug class: missing step)"
  fi
done

# ── Required-artifact coverage ───────────────────────────────────
# Each capability-specific artifact must exist and be covered by the declared
# directory/glob/file copy step. The step marker must also exist in install.sh,
# proving that coverage is implemented rather than merely declared.
for atuple in "${required_artifacts[@]}"; do
  IFS=$'\t' read -r artifact_path artifact_step <<<"$atuple"
  artifact_source_exists="false"
  artifact_step_found="false"
  artifact_step_covers="false"
  artifact_step_implemented="false"

  [[ -f "$REPO_ROOT/$artifact_path" ]] && artifact_source_exists="true"

  for stuple in "${steps[@]}"; do
    IFS=$'\t' read -r sname smarker _ stype ssource_dir sglob ssource_file <<<"$stuple"
    [[ "$sname" == "$artifact_step" ]] || continue
    artifact_step_found="true"
    case "$stype" in
      directory_copy)
        [[ "$artifact_path" == "${ssource_dir}/"* ]] && artifact_step_covers="true"
        ;;
      glob_install)
        # The canonical manifest field is intentionally interpreted as a glob.
        # shellcheck disable=SC2053
        [[ "$artifact_path" == $sglob ]] && artifact_step_covers="true"
        ;;
      file_copy)
        [[ "$artifact_path" == "$ssource_file" ]] && artifact_step_covers="true"
        ;;
    esac
    if [[ -n "$smarker" ]] && grep -qF -- "$smarker" "$INSTALL_SH"; then
      artifact_step_implemented="true"
    fi
    break
  done

  if [[ "$artifact_source_exists" != "true" ]]; then
    emit_fail "Required artifact source missing: $artifact_path"
  elif [[ "$artifact_step_found" != "true" ]]; then
    emit_fail "Required artifact $artifact_path references unknown install step: $artifact_step"
  elif [[ "$artifact_step_covers" != "true" ]]; then
    emit_fail "Required artifact $artifact_path is not covered by install step $artifact_step"
  elif [[ "$artifact_step_implemented" != "true" ]]; then
    emit_fail "Required artifact $artifact_path maps to unimplemented install step $artifact_step"
  else
    emit_pass "Required artifact covered: $artifact_path -> $artifact_step"
  fi
done

# ── Invariant checks ─────────────────────────────────────────────
# I1: gitignore_root_is_repo_root — the improvements/ gitignore write
#     MUST target the repo-root .gitignore, not ${TARGET}/.gitignore.
if grep -qE '\$\{TARGET\}/\.gitignore' "$INSTALL_SH" 2>/dev/null; then
  emit_fail "I1 gitignore_root_is_repo_root: install.sh writes \${TARGET}/.gitignore (must be repo-root .gitignore) — closes bug ce01576"
else
  if grep -qE 'grep -qx .improvements/. ".gitignore"' "$INSTALL_SH" \
    || grep -qF "'improvements/' \".gitignore\"" "$INSTALL_SH" \
    || grep -qF 'improvements/" ".gitignore"' "$INSTALL_SH"; then
    emit_pass "I1 gitignore_root_is_repo_root: improvements/ written to repo-root .gitignore"
  else
    emit_fail "I1 gitignore_root_is_repo_root: cannot verify improvements/ is written to repo-root .gitignore"
  fi
fi

# I2: scripts_are_chmod_x
if grep -qE 'chmod \+x "\$\{TARGET\}"/bubbles/scripts/\*\.sh' "$INSTALL_SH" \
  || grep -qE 'find "\$\{TARGET\}/bubbles/scripts".*chmod \+x' "$INSTALL_SH"; then
  emit_pass "I2 scripts_are_chmod_x: bubbles/scripts/*.sh receives chmod +x"
else
  emit_fail "I2 scripts_are_chmod_x: install.sh does not chmod +x bubbles/scripts/*.sh"
fi

# I3: adapter_files_are_chmod_x
if grep -qE 'find "\$\{TARGET\}/bubbles/adapters".*chmod \+x' "$INSTALL_SH"; then
  emit_pass "I3 adapter_files_are_chmod_x: bubbles/adapters/*.sh receives chmod +x"
else
  emit_fail "I3 adapter_files_are_chmod_x: install.sh does not chmod +x bubbles/adapters/*.sh"
fi

# I4: every_step_has_a_marker — already handled per-step above; record a header pass
emit_pass "I4 every_step_has_a_marker: $step_count required-step markers checked"

# I5: provenance_records_required_fields
provenance_fields=(installedVersion installMode sourceRef sourceGitSha sourceDirty targetRepoSlug installedAt)
missing_field=""
for f in "${provenance_fields[@]}"; do
  if ! grep -qF "\"$f\":" "$INSTALL_SH"; then
    missing_field="$f"
    break
  fi
done
if [[ -z "$missing_field" ]]; then
  emit_pass "I5 provenance_records_required_fields: all ${#provenance_fields[@]} fields present in .install-source.json heredoc"
else
  emit_fail "I5 provenance_records_required_fields: field missing from install.sh heredoc: $missing_field"
fi

# ── Summary ──────────────────────────────────────────────────────
echo
echo "generate-installer.sh: $step_count step(s), ${#required_artifacts[@]} required artifact(s), ${#invariants[@]} invariant id(s) declared"
if [[ $fail_count -eq 0 ]]; then
  echo "generate-installer.sh: PASS"
  exit 0
else
  echo "generate-installer.sh: FAIL ($fail_count violation(s))"
  exit 1
fi
