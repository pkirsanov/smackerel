#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
  PROJECT_ROOT="${SCRIPT_DIR%/.github/bubbles/scripts}"
  FRAMEWORK_ROOT="$PROJECT_ROOT/.github/bubbles"
  MANAGED_ROOT="$PROJECT_ROOT/.github"
  SOURCE_REPO="false"
else
  PROJECT_ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  FRAMEWORK_ROOT="$PROJECT_ROOT/bubbles"
  MANAGED_ROOT="$PROJECT_ROOT"
  SOURCE_REPO="true"
fi

CHECKSUM_FILE="$FRAMEWORK_ROOT/.checksums"
quiet="false"
violations=0

usage() {
  cat <<'EOF'
Usage: bash .github/bubbles/scripts/downstream-framework-write-guard.sh [--quiet]

Checks that downstream framework-managed Bubbles files still match the last
upstream install/refresh checksum snapshot.

The Bubbles source repo is exempt because it is the framework authority.
EOF
}

info() {
  [[ "$quiet" == "true" ]] && return 0
  echo "ℹ️  $1"
}

pass() {
  [[ "$quiet" == "true" ]] && return 0
  echo "✅ $1"
}

fail_line() {
  echo "❌ $1"
  violations=$((violations + 1))
}

sha256_file() {
  local target_file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$target_file" | awk '{print $1}'
    return 0
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$target_file" | awk '{print $1}'
    return 0
  fi

  echo "ERROR: sha256sum or shasum is required for downstream framework provenance checks" >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet)
      quiet="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$SOURCE_REPO" == "true" ]]; then
  pass "Bubbles source repo detected — downstream framework write guard is not applicable here"
  exit 0
fi

if [[ ! -f "$CHECKSUM_FILE" ]]; then
  fail_line "Missing framework checksum snapshot: .github/bubbles/.checksums"
  [[ "$quiet" == "true" ]] || {
    echo "   Remediation: rerun the Bubbles installer or /bubbles.setup mode: refresh from the upstream Bubbles repo."
    echo "   Do not create or patch framework-managed files by hand in a consumer repo."
  }
  exit 1
fi

info "Checking downstream framework-managed files against .github/bubbles/.checksums"

while IFS=$'\t' read -r expected_hash relative_path; do
  [[ -n "$expected_hash" ]] || continue
  [[ "$expected_hash" == \#* ]] && continue
  [[ -n "$relative_path" ]] || continue

  target_file="$MANAGED_ROOT/$relative_path"
  if [[ ! -f "$target_file" ]]; then
    fail_line "Missing framework-managed file: $relative_path"
    continue
  fi

  actual_hash="$(sha256_file "$target_file")"
  if [[ "$actual_hash" != "$expected_hash" ]]; then
    fail_line "Framework-managed file drift detected: $relative_path"
    [[ "$quiet" == "true" ]] || {
      echo "   Expected: $expected_hash"
      echo "   Actual:   $actual_hash"
    }
  fi
done < "$CHECKSUM_FILE"

if [[ "$violations" -gt 0 ]]; then
  [[ "$quiet" == "true" ]] || {
    echo ""
    echo "Downstream repos must not directly author changes in framework-managed Bubbles files."
    echo "Use a project-owned proposal instead: .github/bubbles-project/proposals/"
    echo "Suggested flow: bubbles framework-proposal <slug>"
    echo "Then implement the change in the Bubbles source repo and refresh this repo's framework install."
  }
  exit 1
fi

pass "Downstream framework-managed files still match the installed upstream snapshot"