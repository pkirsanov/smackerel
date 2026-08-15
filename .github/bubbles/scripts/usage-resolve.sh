#!/usr/bin/env bash
# bubbles/scripts/usage-resolve.sh — resolve the configured host-usage adapter.
#
# Maps a repository to its usage provider by reading `usage.adapter` from the
# PROJECT-OWNED config (`.github/bubbles-project.yaml` or
# `bubbles-project.yaml`) and printing the resolved adapter name plus the
# absolute path of the adapter script (IMP-039 SCOPE-2).
#
# OPT-IN BY DEFAULT-OFF. No `usage:` block, no config file, or an explicit
# `adapter: none` all resolve to the neutral `none` adapter and exit 0. The
# framework therefore never depends on a host telemetry artifact, and a repo
# that has not opted in reports `unmeasured` rather than a fabricated figure.
#
# A CONFIGURED-BUT-BROKEN adapter fails loud (exit 1) instead of degrading to
# `none`: a typo that silently produced "unmeasured" would be indistinguishable
# from a deliberate opt-out, which is exactly the ambiguity this contract exists
# to remove.
#
# Uses NO yq/jq/python — a plain awk scan keeps it dependency-free on the
# minimal PATH used by framework validation.
#
# Output (stdout, one key=value per line):
#   adapter=<name>
#   adapterPath=<absolute path to the adapter script>
#   repoRoot=<absolute repository root>
#
# Exit codes:
#   0  resolved (INCLUDING the neutral adapter=none resolution)
#   1  configured adapter is unknown / unsafe / missing on disk
#   2  usage error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/usage"

REPO_ROOT="$PWD"
NAMES_ONLY=0

usage() {
  cat <<'EOF'
Usage: usage-resolve.sh [--repo-root PATH] [--names-only]

Resolve the project-configured host-usage adapter. Default is `none`.

Options:
  --repo-root PATH  Repository whose bubbles-project.yaml is read (default: $PWD)
  --names-only      Print only `adapter=<name>` and exit
  -h, --help        Show this help

Project config (project-owned, never framework-managed):

  usage:
    adapter: none | vscode-copilot
EOF
}

fail() {
  echo "usage-resolve: $1" >&2
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

# Absent block, absent key, and absent file all yield '' -> neutral `none`.
ADAPTER=''
if [ -n "$CONFIG_FILE" ]; then
  ADAPTER="$(awk '
    /^[[:space:]]*#/ { next }
    /^usage:[[:space:]]*$/ { inblock = 1; next }
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
# filesystem: the value is operator-supplied and must never traverse out of the
# adapter directory.
case "$ADAPTER" in
  *[!a-z0-9-]* | '' | -*)
    fail "invalid usage.adapter '$ADAPTER' (expected a lowercase token such as none or vscode-copilot)"
    ;;
esac

ADAPTER_PATH="$ADAPTER_DIR/$ADAPTER.sh"
[ -f "$ADAPTER_PATH" ] ||
  fail "configured usage.adapter '$ADAPTER' has no adapter at $ADAPTER_PATH"

echo "adapter=$ADAPTER"
if [ "$NAMES_ONLY" = "1" ]; then
  exit 0
fi
echo "adapterPath=$ADAPTER_PATH"
echo "repoRoot=$REPO_ROOT"
exit 0
