#!/usr/bin/env bash
# bubbles/scripts/mutation-resolve.sh — resolve the configured mutation adapter.
#
# Maps a repository to its mutation-execution tooling by reading the
# `mutationExecution:` block from the PROJECT-OWNED config
# (`.github/bubbles-project.yaml` or `bubbles-project.yaml`) and printing the
# resolved adapter (IMP-040 SCOPE-7).
#
# OPT-IN BY DEFAULT-OFF. No `mutationExecution:` block, no config file, or an
# explicit `adapter: none` all resolve to the neutral `none` adapter and exit 0.
# Bubbles stays language-agnostic: projects own their mutation runners, and the
# framework validates only the emitted contract document.
#
# TWO ADAPTER KINDS, deliberately only two:
#   none     framework-shipped neutral adapter (bubbles/adapters/mutation)
#   command  a PROJECT-OWNED executable that prints the contract document
# `command` is not a framework script, so it is resolved as a repo-relative path
# and validated for existence and executability here rather than at call time.
#
# A CONFIGURED-BUT-BROKEN adapter fails loud (exit 1) instead of degrading to
# `none`: a typo that silently produced "unmeasured" would be indistinguishable
# from a deliberate opt-out, and a high-risk scenario would then accept a weak
# negative control on the strength of a misspelling.
#
# Uses NO yq/jq/python — a plain awk scan keeps it dependency-free on the
# minimal PATH used by framework validation.
#
# Output (stdout, one key=value per line):
#   adapter=<none|command>
#   adapterPath=<absolute path to the framework adapter>   (adapter=none)
#   command=<absolute path to the project executable>      (adapter=command)
#   timeoutSeconds=<integer>                               (adapter=command)
#   repoRoot=<absolute repository root>
#
# Exit codes:
#   0  resolved (INCLUDING the neutral adapter=none resolution)
#   1  configured adapter is unknown / unsafe / missing / not executable
#   2  usage error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/mutation"

REPO_ROOT="$PWD"
NAMES_ONLY=0
DEFAULT_TIMEOUT=600

usage() {
  cat <<'EOF'
Usage: mutation-resolve.sh [--repo-root PATH] [--names-only]

Resolve the project-configured mutation adapter. Default is `none`.

Options:
  --repo-root PATH  Repository whose bubbles-project.yaml is read (default: $PWD)
  --names-only      Print only `adapter=<name>` and exit
  -h, --help        Show this help

Project config (project-owned, never framework-managed):

  mutationExecution:
    adapter: none | command
    command: scripts/bubbles-mutation
    timeoutSeconds: 600
EOF
}

fail() {
  echo "mutation-resolve: $1" >&2
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

# Read all three keys in ONE pass so a malformed block cannot yield a mixed
# reading (e.g. an adapter from the block and a command from outside it).
read_key() {
  [ -n "$CONFIG_FILE" ] || return 0
  awk -v want="$1" '
    /^[[:space:]]*#/ { next }
    /^mutationExecution:[[:space:]]*$/ { inblock = 1; next }
    inblock && /^[^[:space:]]/ { inblock = 0 }
    inblock && $1 == want {
      value = $2
      gsub(/["\047]/, "", value)
      print value
      exit
    }
  ' "$CONFIG_FILE" 2>/dev/null || true
}

ADAPTER="$(read_key 'adapter:')"
COMMAND_REL="$(read_key 'command:')"
TIMEOUT="$(read_key 'timeoutSeconds:')"

[ -n "$ADAPTER" ] || ADAPTER='none'

# Reject anything that is not a plain lowercase token BEFORE touching the
# filesystem: the value is operator-supplied and must never traverse out of the
# adapter directory.
case "$ADAPTER" in
  *[!a-z0-9-]* | '' | -*)
    fail "invalid mutationExecution.adapter '$ADAPTER' (expected none or command)"
    ;;
esac

echo "adapter=$ADAPTER"
if [ "$NAMES_ONLY" = "1" ]; then
  exit 0
fi

case "$ADAPTER" in
  none)
    ADAPTER_PATH="$ADAPTER_DIR/none.sh"
    [ -f "$ADAPTER_PATH" ] ||
      fail "framework adapter missing at $ADAPTER_PATH"
    echo "adapterPath=$ADAPTER_PATH"
    ;;
  command)
    [ -n "$COMMAND_REL" ] ||
      fail "mutationExecution.adapter is 'command' but no command: was declared"
    # Repo-relative only. An absolute path or a parent traversal would let a
    # config file point the framework at an executable outside the repository.
    case "$COMMAND_REL" in
      /* | *..*)
        fail "mutationExecution.command must be a repo-relative path without '..' (got '$COMMAND_REL')"
        ;;
    esac
    COMMAND_ABS="$REPO_ROOT/$COMMAND_REL"
    [ -f "$COMMAND_ABS" ] ||
      fail "mutationExecution.command '$COMMAND_REL' not found at $COMMAND_ABS"
    [ -x "$COMMAND_ABS" ] ||
      fail "mutationExecution.command '$COMMAND_REL' is not executable"
    [ -n "$TIMEOUT" ] || TIMEOUT="$DEFAULT_TIMEOUT"
    case "$TIMEOUT" in
      '' | *[!0-9]*)
        fail "mutationExecution.timeoutSeconds must be a positive integer (got '$TIMEOUT')"
        ;;
    esac
    [ "$TIMEOUT" -gt 0 ] ||
      fail "mutationExecution.timeoutSeconds must be greater than zero"
    echo "command=$COMMAND_ABS"
    echo "timeoutSeconds=$TIMEOUT"
    ;;
  *)
    fail "unknown mutationExecution.adapter '$ADAPTER' (expected none or command)"
    ;;
esac

echo "repoRoot=$REPO_ROOT"
exit 0
