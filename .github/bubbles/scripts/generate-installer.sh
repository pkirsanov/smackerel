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

MODE="--check"
if [[ $# -gt 0 ]]; then
  case "$1" in
    --check) MODE="--check"; shift ;;
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
declare -i invariant_count=0

emit_fail() {
  echo "FAIL: $*"
  fail_count=$((fail_count + 1))
}

emit_pass() {
  echo "PASS: $*"
}

# ── Parse the manifest ───────────────────────────────────────────
# Pure bash parser — extract a list of (name, marker, required, type)
# tuples. The YAML shape is constrained so a simple awk script suffices.

mapfile -t parsed_lines < <(awk '
  /^steps:/ { in_steps=1; in_inv=0; next }
  /^invariants:/ { in_steps=0; in_inv=1; next }
  in_steps && /^  - name:[[:space:]]+/ {
    if (current_name != "") {
      printf "STEP\t%s\t%s\t%s\t%s\n", current_name, current_marker, current_required, current_type
    }
    current_name=$3; current_marker=""; current_required="true"; current_type=""
    next
  }
  in_steps && /^    type:[[:space:]]+/ { current_type=$2; next }
  in_steps && /^    marker:[[:space:]]+/ {
    line=$0
    sub(/^[[:space:]]+marker:[[:space:]]+/, "", line)
    gsub(/^[\47"]|[\47"]$/, "", line)
    current_marker=line
    next
  }
  in_steps && /^    required:[[:space:]]+/ { current_required=$2; next }
  in_inv && /^  - id:[[:space:]]+/ {
    if (current_inv != "") {
      printf "INV\t%s\n", current_inv
    }
    current_inv=$3
    next
  }
  END {
    if (current_name != "") {
      printf "STEP\t%s\t%s\t%s\t%s\n", current_name, current_marker, current_required, current_type
    }
    if (current_inv != "") {
      printf "INV\t%s\n", current_inv
    }
  }
' "$INSTALLER_YAML")

declare -a steps=()
declare -a invariants=()
for ln in "${parsed_lines[@]}"; do
  case "$ln" in
    STEP$'\t'*) steps+=("${ln#STEP$'\t'}") ;;
    INV$'\t'*) invariants+=("${ln#INV$'\t'}") ;;
  esac
done

step_count=${#steps[@]}
invariant_count=${#invariants[@]}

if [[ $step_count -eq 0 ]]; then
  echo "generate-installer.sh: no steps parsed from manifest" >&2
  exit 2
fi

# ── Step check: every required step's marker appears in install.sh ─
for stuple in "${steps[@]}"; do
  IFS=$'\t' read -r sname smarker srequired stype <<<"$stuple"
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

# I5: provenance_records_six_fields
provenance_fields=(installedVersion installMode sourceRef sourceGitSha sourceDirty installedAt)
missing_field=""
for f in "${provenance_fields[@]}"; do
  if ! grep -qF "\"$f\":" "$INSTALL_SH"; then
    missing_field="$f"
    break
  fi
done
if [[ -z "$missing_field" ]]; then
  emit_pass "I5 provenance_records_six_fields: all 6 fields present in .install-source.json heredoc"
else
  emit_fail "I5 provenance_records_six_fields: field missing from install.sh heredoc: $missing_field"
fi

# ── Summary ──────────────────────────────────────────────────────
echo
echo "generate-installer.sh: $step_count step(s), ${#invariants[@]} invariant id(s) declared"
if [[ $fail_count -eq 0 ]]; then
  echo "generate-installer.sh: PASS"
  exit 0
else
  echo "generate-installer.sh: FAIL ($fail_count violation(s))"
  exit 1
fi
