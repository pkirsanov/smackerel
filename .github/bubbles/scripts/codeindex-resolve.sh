#!/usr/bin/env bash
# bubbles/scripts/codeindex-resolve.sh — resolve the configured code-index adapter.
#
# Maps a repository to its code-index provider adapter by reading
# `codeIndex.adapter` from the PROJECT-OWNED config
# (`.github/bubbles-project.yaml` or `bubbles-project.yaml`) and printing the
# resolved adapter name plus the absolute path of the adapter script, so a
# caller can invoke `bubbles/adapters/codeindex/<adapter>.sh` directly.
#
# OPT-IN BY DEFAULT-OFF. A repository with no `codeIndex:` block, no config
# file at all, or an explicit `adapter: none` resolves to the neutral `none`
# adapter and exits 0. The framework therefore NEVER acquires a hard dependency
# on an external indexer, which keeps portable surfaces free of concrete
# tooling assumptions (agnosticity-lint).
#
# This resolver deliberately uses NO yq/jq/python: a plain awk scan keeps it
# dependency-free on the minimal PATH used by framework validation.
#
# Output (stdout, one key=value per line):
#   adapter=<name>
#   adapterPath=<absolute path to the adapter script>
#   repoRoot=<absolute repository root>
#
# Exit codes:
#   0  resolved (INCLUDING the neutral adapter=none resolution)
#   1  configured adapter is unknown / unsafe / missing on disk (fail loud —
#      a typo must never silently degrade to none and hide the misconfiguration)
#   2  usage error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Support both the source layout (bubbles/scripts) and a downstream install
# (.github/bubbles/scripts), matching the convention used by sibling scripts.
if [ "$(basename "$(dirname "$SCRIPT_DIR")")" = "bubbles" ] &&
  [ "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" = ".github" ]; then
  FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
else
  FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/codeindex"

REPO_ROOT="$PWD"

usage() {
  cat <<'EOF'
Usage: codeindex-resolve.sh [--repo-root PATH] [--names-only]

Resolve the project-configured code-index adapter. Default is `none`.

Options:
  --repo-root PATH  Repository whose bubbles-project.yaml is read (default: $PWD)
  --names-only      Print only `adapter=<name>` and exit
  -h, --help        Show this help

Project config (project-owned, never framework-managed):

  codeIndex:
    adapter: none | codegraph
EOF
}

fail() {
  echo "codeindex-resolve: $1" >&2
  exit "${2:-1}"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo-root)
      [ "$#" -ge 2 ] || fail "--repo-root requires a value" 2
      REPO_ROOT="$2"
      shift 2
      ;;
    --names-only)
      NAMES_ONLY=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1" 2
      ;;
  esac
done

[ -d "$REPO_ROOT" ] || fail "repo root not found: $REPO_ROOT" 2
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

CONFIG_FILE=''
if [ -f "$REPO_ROOT/.github/bubbles-project.yaml" ]; then
  CONFIG_FILE="$REPO_ROOT/.github/bubbles-project.yaml"
elif [ -f "$REPO_ROOT/bubbles-project.yaml" ]; then
  CONFIG_FILE="$REPO_ROOT/bubbles-project.yaml"
fi

# Extract codeIndex.adapter with a portable awk block scan. Absent block, absent
# key, and absent file all yield the empty string -> neutral `none`.
ADAPTER=''
if [ -n "$CONFIG_FILE" ]; then
  ADAPTER="$(awk '
    /^[[:space:]]*#/ { next }
    /^codeIndex:[[:space:]]*$/ { inblock = 1; next }
    inblock && /^[^[:space:]]/ { inblock = 0 }
    inblock && $1 == "adapter:" {
      value = $2
      gsub(/["\047]/, "", value)
      print value
      exit
    }
  ' "$CONFIG_FILE" 2>/dev/null || true)"
fi

[ -n "$ADAPTER" ] || ADAPTER='none'

# Reject anything that is not a plain lowercase token BEFORE touching the
# filesystem: the value is operator-supplied config and must never be able to
# traverse out of the adapter directory.
case "$ADAPTER" in
  *[!a-z0-9-]* | '' | -*)
    fail "invalid codeIndex.adapter '$ADAPTER' (expected a lowercase token such as none or codegraph)"
    ;;
esac

ADAPTER_PATH="$ADAPTER_DIR/$ADAPTER.sh"
[ -f "$ADAPTER_PATH" ] ||
  fail "configured codeIndex.adapter '$ADAPTER' has no adapter at $ADAPTER_PATH"

echo "adapter=$ADAPTER"
if [ "${NAMES_ONLY:-0}" = "1" ]; then
  exit 0
fi
echo "adapterPath=$ADAPTER_PATH"
echo "repoRoot=$REPO_ROOT"
exit 0
