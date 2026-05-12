#!/usr/bin/env bash
set -euo pipefail

# edit-lint-gate.sh
# Project-pluggable linter-on-edit gate for Bubbles.
#
# Specialist agents (bubbles.implement, bubbles.devops, bubbles.simplify,
# bubbles.harden) MAY invoke this script after editing files. The framework
# supplies the gate dispatcher; downstream projects supply language-specific
# linters via .specify/memory/bubbles.config.json under
# `editLintGate.linters`.
#
# Default behavior is no-op (opt-in only) so the framework stays portable
# and never assumes a specific toolchain. The framework MUST NOT bundle
# default linters.
#
# Hard dependency: jq. (Already used elsewhere in the framework.)
#
# See: agents/bubbles_shared/operating-baseline.md
#      → "Linter-On-Edit Gate (Project-Pluggable)"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/edit-lint-gate.sh <changed-file> [<changed-file>...]

Project-pluggable linter-on-edit gate.

For each changed file, the gate consults
`.specify/memory/bubbles.config.json` under `editLintGate.linters` and
invokes any matching linter (selected by glob pattern). The configured
command is invoked with the changed file path appended as the final
argument. Exit code 0 if all matching linters pass; non-zero if any fail.

No-op safety:
  - If the config file is missing                → exit 0 silently.
  - If `editLintGate.enabled` is false           → exit 0 silently.
  - If no configured linter matches the file     → exit 0 silently.

Hard dependency:
  - `jq` is required to parse the config. If `jq` is missing, exit non-zero.

Configuration shape (.specify/memory/bubbles.config.json):
  {
    "editLintGate": {
      "enabled": true,
      "linters": [
        {
          "name": "rust-clippy",
          "match": "*.rs",
          "command": ["cargo", "clippy", "--no-deps", "--", "-D", "warnings"]
        },
        {
          "name": "ts-eslint",
          "match": "*.ts",
          "command": ["npx", "eslint", "--max-warnings=0"]
        }
      ]
    }
  }

The framework provides the gate; downstream supplies the linters. This
script does NOT bundle default linters.

Reference:
  agents/bubbles_shared/operating-baseline.md
    -> "Linter-On-Edit Gate (Project-Pluggable)"
EOF
}

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

case "$1" in
  -h|--help)
    usage
    exit 0
    ;;
esac

# --- jq dependency check ---------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "edit-lint-gate: jq is required but not found in PATH." >&2
  echo "  Install jq before invoking edit-lint-gate.sh." >&2
  exit 3
fi

# --- Repo root resolution --------------------------------------------------

resolve_repo_root() {
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  # Without a recognizable Bubbles repo, the gate has no config to read.
  # Fail-safe to no-op (exit 0) — this matches "config missing" semantics.
  exit 0
fi

CONFIG_FILE="$REPO_ROOT/.specify/memory/bubbles.config.json"

# No-op short-circuit when config is missing.
if [[ ! -f "$CONFIG_FILE" ]]; then
  exit 0
fi

# Validate JSON before reading further; corrupt config should fail loudly.
if ! jq empty "$CONFIG_FILE" >/dev/null 2>&1; then
  echo "edit-lint-gate: config file is not valid JSON: $CONFIG_FILE" >&2
  exit 4
fi

# Disabled gate → no-op.
ENABLED="$(jq -r '.editLintGate.enabled // false' "$CONFIG_FILE")"
if [[ "$ENABLED" != "true" ]]; then
  exit 0
fi

# --- Glob-match helper -----------------------------------------------------

glob_match() {
  # Returns 0 if $1 matches glob $2 against the file BASENAME first, then
  # against the full path as a fallback (so patterns like "src/*.rs" still
  # work). Uses bash extglob-free pattern matching.
  local file="$1"
  local pattern="$2"
  local base
  base="$(basename "$file")"
  # shellcheck disable=SC2053
  if [[ "$base" == $pattern ]]; then
    return 0
  fi
  # shellcheck disable=SC2053
  if [[ "$file" == $pattern ]]; then
    return 0
  fi
  return 1
}

# --- Iterate changed files -------------------------------------------------

LINTER_COUNT="$(jq '(.editLintGate.linters // []) | length' "$CONFIG_FILE")"
if [[ "$LINTER_COUNT" -eq 0 ]]; then
  exit 0
fi

overall_failures=0

for changed_file in "$@"; do
  # Loop over each linter index.
  for i in $(seq 0 $((LINTER_COUNT - 1))); do
    name="$(jq -r ".editLintGate.linters[$i].name // \"linter-$i\"" "$CONFIG_FILE")"
    match="$(jq -r ".editLintGate.linters[$i].match // \"\"" "$CONFIG_FILE")"

    if [[ -z "$match" ]]; then
      continue
    fi

    if ! glob_match "$changed_file" "$match"; then
      continue
    fi

    # Read the command array. Each element is a string token.
    mapfile -t cmd_tokens < <(jq -r ".editLintGate.linters[$i].command[]?" "$CONFIG_FILE")

    if [[ "${#cmd_tokens[@]}" -eq 0 ]]; then
      echo "edit-lint-gate: linter '$name' has empty command; skipping" >&2
      continue
    fi

    echo "edit-lint-gate: running '$name' on '$changed_file'"
    if "${cmd_tokens[@]}" "$changed_file"; then
      echo "edit-lint-gate: '$name' PASSED on '$changed_file'"
    else
      rc=$?
      echo "edit-lint-gate: '$name' FAILED on '$changed_file' (exit=$rc)" >&2
      overall_failures=$((overall_failures + 1))
    fi
  done
done

if [[ "$overall_failures" -gt 0 ]]; then
  echo "edit-lint-gate: $overall_failures linter invocation(s) failed." >&2
  exit 1
fi

exit 0
