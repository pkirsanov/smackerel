#!/usr/bin/env bash
#
# Bubbles — payload integrity verifier (IMP-101 SCOPE-8).
#
# Verifies that every INSTALLED framework-managed file matches the CANONICAL
# sha256 recorded in the release manifest's managedFileChecksums (computed at
# release time in the source repo). install.sh calls this immediately after the
# copy flow, before it stamps + snapshots the install, to catch a payload that
# arrived corrupt — a truncated download, a failed tar extraction, a partial
# disk write, or a single tampered file — none of which the self-referential
# .checksums snapshot install.sh writes can detect (that snapshot is computed
# FROM the installed bytes, so it faithfully records corruption instead of
# flagging it).
#
# SCOPE (honest limits): this is an INTEGRITY check — do the bytes we installed
# match the release's recorded bytes? — NOT an AUTHENTICITY check. Because the
# manifest ships inside the same payload, a coordinated tamper that rewrites
# BOTH a managed file AND its manifest entry is out of scope; defeating that
# needs a cryptographically SIGNED manifest (keys the operator does not hold at
# install time) and is deliberately deferred. This check nonetheless closes the
# corruption / incomplete-download / single-file-tamper class at zero
# key-management cost.
#
# Every managed entry is REQUIRED unless the active install profile explicitly
# omits it. Source-only files are recorded in sourceOnlyFileChecksums, not in the
# managed section scanned here. The agents-only profile may omit instructions/
# and skills/, while a registered optional skill may be absent until the
# downstream opts in. Any other missing managed file is a hard FAILURE.
#
# RELATIONSHIP TO bubbles-drift-check.sh (NOT redundant — complementary):
#   - bubbles-drift-check.sh is the ON-DEMAND, post-install "am I drifted?" check
#     (doctor / pre-push). It also compares installed files against
#     managedFileChecksums, but it REQUIRES python3 (advisory-skips when python3
#     is absent) and treats a manifest path missing on disk as DRIFT/MISSING.
#   - THIS verifier is the INSTALL-TIME gate: pure bash + awk, so it verifies
#     even when python3 is absent (install.sh explicitly supports that path).
#     It receives the install profile explicitly and reads the installed
#     optional-skill registry so only contract-declared omissions are allowed.
#     Deleting either one reopens a real gap.
#
# Usage:
#   verify-payload-integrity.sh [--target DIR] [--manifest FILE]
#                               [--install-profile full|agents-only] [--quiet]
#
#   --target DIR     Install root that holds the managed trees (default: .github)
#   --manifest FILE  release-manifest.json to verify against
#                    (default: <target>/bubbles/release-manifest.json)
#   --install-profile
#                    Install shape to verify (default: full)
#   --quiet          Suppress the success line (failures always print to stderr)
#
# Exit codes:
#   0 = every required managed file is present and matches
#       (or manifest absent -> advisory)
#   1 = one or more required managed files are missing or mismatch the manifest
#   2 = usage / environment error (unknown flag, or no sha256 tool)
#
set -euo pipefail

TARGET_DIR=".github"
MANIFEST_FILE=""
INSTALL_PROFILE="full"
QUIET="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)          TARGET_DIR="$2"; shift 2 ;;
    --manifest)        MANIFEST_FILE="$2"; shift 2 ;;
    --install-profile) INSTALL_PROFILE="$2"; shift 2 ;;
    --quiet)           QUIET="true"; shift ;;
    -h | --help)
      cat <<'EOF'
Usage: verify-payload-integrity.sh [--target DIR] [--manifest FILE]
                                   [--install-profile full|agents-only] [--quiet]

Verifies every required framework-managed file against the canonical sha256
recorded in the release manifest (managedFileChecksums). Missing or mismatched
required files are hard failures. Source-only entries are outside the managed
section; agents-only and unselected optional-skill omissions remain valid.

Exit codes: 0 = clean, 1 = missing/mismatch found, 2 = usage / no sha256 tool.
EOF
      exit 0
      ;;
    *)
      echo "verify-payload-integrity: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

case "$INSTALL_PROFILE" in
  full | agents-only) ;;
  *)
    echo "verify-payload-integrity: unknown install profile: $INSTALL_PROFILE" >&2
    exit 2
    ;;
esac

[[ -n "$MANIFEST_FILE" ]] || MANIFEST_FILE="${TARGET_DIR}/bubbles/release-manifest.json"

say() { [[ "$QUIET" == "true" ]] || printf '%s\n' "$*"; }

# A missing manifest is advisory, never fatal here: install.sh's own preflight
# already refuses to install without a manifest, and an older downstream layout
# predating this file should not hard-fail a re-scan.
if [[ ! -f "$MANIFEST_FILE" ]]; then
  say "verify-payload-integrity: no release manifest at ${MANIFEST_FILE} — skipped (advisory)"
  exit 0
fi

# Resolve the sha256 tool once (fail fast if neither is available). Indexed array
# keeps the invocation shellcheck-clean and works on bash 3.2 (macOS system bash).
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD=(shasum -a 256)
else
  echo "verify-payload-integrity: sha256sum or shasum is required" >&2
  exit 2
fi

sha256_of() { "${SHA_CMD[@]}" "$1" | awk '{print $1}'; }

project_config_path() {
  if [[ -f "${TARGET_DIR}/bubbles-project.yaml" ]]; then
    printf '%s\n' "${TARGET_DIR}/bubbles-project.yaml"
  elif [[ -f "${TARGET_DIR}/../bubbles-project.yaml" ]]; then
    printf '%s\n' "${TARGET_DIR}/../bubbles-project.yaml"
  fi
}

design_language_enabled() {
  local token="$1"
  local config_file

  config_file="$(project_config_path)"
  [[ -n "$config_file" ]] || return 1
  awk '/^designLanguages:/{f=1; next} /^[A-Za-z0-9_-]+:/{f=0} f' "$config_file" | grep -qiF "$token"
}

optional_skill_token() {
  local relative_path="$1"
  local registry_file="${TARGET_DIR}/bubbles/registry/optional-skills.txt"
  local skill_name
  local registered_name
  local enablement_token
  local rest

  [[ "$relative_path" == skills/*/* ]] || return 1
  [[ -f "$registry_file" ]] || return 1
  skill_name="${relative_path#skills/}"
  skill_name="${skill_name%%/*}"

  while read -r registered_name enablement_token rest; do
    [[ -z "$registered_name" || "$registered_name" == \#* ]] && continue
    if [[ "$registered_name" == "$skill_name" ]]; then
      printf '%s\n' "${enablement_token:-$registered_name}"
      return 0
    fi
  done < "$registry_file"
  return 1
}

managed_path_may_be_absent() {
  local relative_path="$1"
  local enablement_token

  if [[ "$INSTALL_PROFILE" == "agents-only" ]]; then
    case "$relative_path" in
      instructions/* | skills/*) return 0 ;;
    esac
  fi

  if enablement_token="$(optional_skill_token "$relative_path")"; then
    ! design_language_enabled "$enablement_token"
    return $?
  fi

  return 1
}

verified=0
skipped=0
failures=""

# One pass over managedFileChecksums. The awk section detection mirrors the
# existing release_manifest_owns_managed_path() parser in install.sh (no JSON
# dependency; BSD/GNU awk compatible). Each emitted record is "<path>\t<sha256>".
while IFS=$'\t' read -r rel_path expected_sha; do
  [[ -n "$rel_path" ]] || continue
  installed="${TARGET_DIR}/${rel_path}"
  if [[ ! -f "$installed" ]]; then
    if managed_path_may_be_absent "$rel_path"; then
      skipped=$((skipped + 1))
      continue
    fi
    failures="${failures}
  ${rel_path}
    required managed file is missing"
    continue
  fi
  actual_sha=""
  if ! actual_sha="$(sha256_of "$installed" 2>/dev/null)"; then
    failures="${failures}
  ${rel_path}
    could not hash installed file"
    continue
  fi
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    failures="${failures}
  ${rel_path}
    expected ${expected_sha}
    actual   ${actual_sha}"
  else
    verified=$((verified + 1))
  fi
done < <(
  awk '
    BEGIN { section_line = "  \"managedFileChecksums\": [" }
    $0 == section_line { in_section = 1; next }
    in_section && ($0 == "  ]," || $0 == "  ]") { exit }
    in_section {
      path_value = $0
      sub(/^.*"path": "/, "", path_value)
      sub(/".*/, "", path_value)
      sha_value = $0
      sub(/^.*"sha256": "/, "", sha_value)
      sub(/".*/, "", sha_value)
      if (path_value != "" && sha_value != "")
        print path_value "\t" sha_value
    }
  ' "$MANIFEST_FILE"
)

if [[ -n "$failures" ]]; then
  echo "verify-payload-integrity: FAILED — ${verified} file(s) verified, but the following required framework file(s) are missing or do NOT match the release manifest (corruption or incomplete download):${failures}" >&2
  exit 1
fi

say "verify-payload-integrity: OK — ${verified} required framework file(s) match the release manifest (${skipped} profile-conditional managed entries omitted)"
exit 0
