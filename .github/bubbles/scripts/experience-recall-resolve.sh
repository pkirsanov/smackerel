#!/usr/bin/env bash
# Resolve the project-configured experience-recall adapter.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/experience-recall"
REPO_ROOT="$PWD"

usage() {
  cat <<'EOF'
Usage: experience-recall-resolve.sh [--repo-root PATH] [--names-only]

Resolve the project-owned experienceRecall.adapter. The neutral adapter is
`none` when the config file, experienceRecall block, or adapter key is absent.

Options:
  --repo-root PATH  Repository whose bubbles-project.yaml is read (default: $PWD)
  --names-only      Print only `adapter=<name>` and exit
  -h, --help        Show this help

Project config:

  experienceRecall:
    adapter: none
EOF
}

fail() {
  echo "experience-recall-resolve: $1" >&2
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

ADAPTER=''
if [ -n "$CONFIG_FILE" ]; then
  ADAPTER="$(awk '
    /^[[:space:]]*#/ { next }
    /^experienceRecall:[[:space:]]*$/ { inblock = 1; next }
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

case "$ADAPTER" in
  *[!a-z0-9-]* | '' | -*)
    fail "invalid experienceRecall.adapter '$ADAPTER' (expected a lowercase provider token)"
    ;;
esac

ADAPTER_PATH="$ADAPTER_DIR/$ADAPTER.sh"
[ -f "$ADAPTER_PATH" ] ||
  fail "configured experienceRecall.adapter '$ADAPTER' has no adapter at $ADAPTER_PATH"

echo "adapter=$ADAPTER"
if [ "${NAMES_ONLY:-0}" = "1" ]; then
  exit 0
fi
echo "adapterPath=$ADAPTER_PATH"
echo "repoRoot=$REPO_ROOT"
