#!/usr/bin/env bash
set -euo pipefail

# File: shellcheck-lint.sh
#
# Gate: every tracked shell script must pass `shellcheck -S warning` with zero
# findings. Locks in the v7.0.2 shellcheck cleanup so warnings cannot silently
# regress back into the framework's shell surface.
#
# Usage:
#   bash bubbles/scripts/shellcheck-lint.sh                  # lint the tracked shell surface
#   bash bubbles/scripts/shellcheck-lint.sh --dir <path>     # lint *.sh under <path> (used by selftest)
#   bash bubbles/scripts/shellcheck-lint.sh --severity error # override severity (default: warning)
#   bash bubbles/scripts/shellcheck-lint.sh --quiet          # suppress the PASS summary line
#
# Exit codes:
#   0  clean (or shellcheck not installed -> advisory skip)
#   1  one or more shellcheck findings at the chosen severity
#   2  invalid arguments

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${BUBBLES_REPO_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"

SEVERITY="warning"
TARGET_DIR=""
QUIET="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/shellcheck-lint.sh [--dir <path>] [--severity error|warning] [--quiet]

Lints the tracked shell surface (git ls-files '*.sh') with shellcheck at the
chosen severity (default: warning) and fails on any finding.

Options:
  --dir <path>              Lint *.sh under <path> instead of the tracked surface.
  --severity error|warning  Severity floor (default: warning).
  --quiet                   Suppress the PASS summary line.
  -h, --help                Print this usage and exit.

Exit codes:
  0  clean (or shellcheck not installed -> advisory skip)
  1  one or more findings
  2  invalid arguments
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --dir)
      shift
      [[ $# -gt 0 ]] || {
        echo "shellcheck-lint: --dir requires a path" >&2
        exit 2
      }
      TARGET_DIR="$1"
      shift
      ;;
    --dir=*)
      TARGET_DIR="${1#*=}"
      shift
      ;;
    --severity)
      shift
      [[ $# -gt 0 ]] || {
        echo "shellcheck-lint: --severity requires a value" >&2
        exit 2
      }
      SEVERITY="$1"
      shift
      ;;
    --severity=*)
      SEVERITY="${1#*=}"
      shift
      ;;
    *)
      echo "shellcheck-lint: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$SEVERITY" in
  error | warning | info | style) ;;
  *)
    echo "shellcheck-lint: invalid severity: $SEVERITY (expected error|warning|info|style)" >&2
    exit 2
    ;;
esac

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "shellcheck-lint: shellcheck not installed; skipping (install shellcheck to enforce -S $SEVERITY)" >&2
  exit 0
fi

# Collect targets.
declare -a targets=()
if [[ -n "$TARGET_DIR" ]]; then
  [[ -d "$TARGET_DIR" ]] || {
    echo "shellcheck-lint: --dir is not a directory: $TARGET_DIR" >&2
    exit 2
  }
  while IFS= read -r f; do
    targets+=("$f")
  done < <(find "$TARGET_DIR" -type f -name '*.sh' | sort)
elif git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  while IFS= read -r f; do
    targets+=("$REPO_ROOT/$f")
  done < <(git -C "$REPO_ROOT" ls-files '*.sh' | sort)
else
  while IFS= read -r f; do
    targets+=("$f")
  done < <(find "$REPO_ROOT" -type f -name '*.sh' -not -path '*/.git/*' | sort)
fi

if [[ "${#targets[@]}" -eq 0 ]]; then
  [[ "$QUIET" == "true" ]] || echo "shellcheck-lint: no shell scripts found to lint"
  exit 0
fi

# Lint all targets in a single invocation; respects in-file directives.
findings_output="$(shellcheck -S "$SEVERITY" -f gcc "${targets[@]}" 2>/dev/null || true)"
if [[ -n "$findings_output" ]]; then
  printf '%s\n' "$findings_output"
  finding_count="$(printf '%s\n' "$findings_output" | grep -c ' \[SC' || true)"
  echo "shellcheck-lint: FAIL — ${finding_count} finding(s) at -S ${SEVERITY} across ${#targets[@]} script(s)" >&2
  exit 1
fi

[[ "$QUIET" == "true" ]] || echo "shellcheck-lint: PASS — ${#targets[@]} script(s) clean at -S ${SEVERITY}"
exit 0
