#!/usr/bin/env bash
# bubbles/scripts/test-inventory-resolve.sh — resolve the configured test-inventory adapter.
#
# Maps a repository to its test inventory by reading the `testDiscovery:` block
# from the PROJECT-OWNED config (`.github/bubbles-project.yaml` or
# `bubbles-project.yaml`) and printing the resolved adapter (IMP-040 SCOPE-1).
#
# OPT-IN BY DEFAULT-OFF. No `testDiscovery:` block, no config file, or an
# explicit `adapter: none` all resolve to the neutral `none` adapter and exit 0.
# Bubbles stays language-agnostic: projects own their runners, and the framework
# validates only the emitted contract document.
#
# TWO ADAPTER KINDS, deliberately only two:
#   none     framework-shipped neutral adapter (bubbles/adapters/test-inventory)
#   command  a PROJECT-OWNED executable that prints the contract document
# `command` is not a framework script, so it is resolved as a repo-relative path
# and validated for existence and executability here rather than at call time.
#
# A CONFIGURED-BUT-BROKEN adapter fails loud (exit 1) instead of degrading to
# `none`: a typo that silently produced "unmeasured" would be indistinguishable
# from a deliberate opt-out, which is exactly the ambiguity this contract exists
# to remove.
#
# Uses Python only for the security-sensitive executable approval. The YAML
# scan remains dependency-light, while repository-rooted descriptor walking
# rejects every symlink component and binds approval to one opened object.
#
# Output (stdout, one key=value per line):
#   adapter=<none|command>
#   adapterPath=<absolute path to the framework adapter>   (adapter=none)
#   command=<absolute path to the project executable>      (adapter=command)
#   commandRelative=<repository-relative executable path>  (adapter=command)
#   commandDevice=<device id>                              (adapter=command)
#   commandInode=<inode>                                   (adapter=command)
#   commandMode=<permission and special mode bits>         (adapter=command)
#   commandSize=<byte size>                                (adapter=command)
#   commandMtimeNs=<mtime in nanoseconds>                  (adapter=command)
#   commandSha256=<SHA-256 of approved descriptor content> (adapter=command)
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
ADAPTER_DIR="$FRAMEWORK_ROOT/adapters/test-inventory"

REPO_ROOT="$PWD"
NAMES_ONLY=0
DEFAULT_TIMEOUT=120

usage() {
  cat <<'EOF'
Usage: test-inventory-resolve.sh [--repo-root PATH] [--names-only]

Resolve the project-configured test-inventory adapter. Default is `none`.

Options:
  --repo-root PATH  Repository whose bubbles-project.yaml is read (default: $PWD)
  --names-only      Print only `adapter=<name>` and exit
  -h, --help        Show this help

Project config (project-owned, never framework-managed):

  testDiscovery:
    adapter: none | command
    command: scripts/bubbles-test-inventory
    timeoutSeconds: 120
EOF
}

fail() {
  echo "test-inventory-resolve: $1" >&2
  exit "${2:-1}"
}

approve_command() {
  command -v python3 >/dev/null 2>&1 || return 1
  python3 - "$1" "$REPO_ROOT" <<'PY'
import hashlib, os, stat, sys

relative, root = sys.argv[1:]
parts = relative.split("/")
if not parts or any(part in ("", ".", "..") for part in parts):
  raise SystemExit(1)
if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_DIRECTORY"):
  raise SystemExit(1)
opened = []
try:
  parent = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
  opened.append(parent)
  root_metadata = os.fstat(parent)
  if not stat.S_ISDIR(root_metadata.st_mode):
    raise OSError("repository root is not a directory")
  for component in parts[:-1]:
    child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                    dir_fd=parent)
    opened.append(child)
    metadata = os.fstat(child)
    if not stat.S_ISDIR(metadata.st_mode):
      raise OSError("intermediate component is not a directory")
    parent = child
  descriptor = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW, dir_fd=parent)
  opened.append(descriptor)
  metadata = os.fstat(descriptor)
  if not stat.S_ISREG(metadata.st_mode):
    raise OSError("not a regular file")
  effective_bits = metadata.st_mode & 0o001
  if metadata.st_gid in os.getgroups() or metadata.st_gid == os.getegid():
    effective_bits |= metadata.st_mode & 0o010
  if metadata.st_uid == os.geteuid():
    effective_bits |= metadata.st_mode & 0o100
  if not effective_bits:
    raise PermissionError("not executable")
  digest = hashlib.sha256()
  while True:
    chunk = os.read(descriptor, 1024 * 1024)
    if not chunk:
      break
    digest.update(chunk)
  confirmed = os.fstat(descriptor)
  if (confirmed.st_dev, confirmed.st_ino, confirmed.st_mode, confirmed.st_size,
      confirmed.st_mtime_ns) != (metadata.st_dev, metadata.st_ino,
                                 metadata.st_mode, metadata.st_size,
                                 metadata.st_mtime_ns):
    raise OSError("metadata changed while hashing approved descriptor")
  absolute = os.path.join(root, *parts)
  print("\t".join(str(value) for value in (
    absolute, relative, metadata.st_dev, metadata.st_ino,
    metadata.st_mode & 0o7777,
    metadata.st_size, metadata.st_mtime_ns, digest.hexdigest())))
except (OSError, ValueError):
  raise SystemExit(1)
finally:
  for descriptor in reversed(opened):
    try:
      os.close(descriptor)
    except OSError:
      pass
PY
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
REPO_ROOT="$(cd -P "$REPO_ROOT" && pwd)"

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
    /^testDiscovery:[[:space:]]*$/ { inblock = 1; next }
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
    fail "invalid testDiscovery.adapter '$ADAPTER' (expected none or command)"
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
      fail "testDiscovery.adapter is 'command' but no command: was declared"
    # Repo-relative only. An absolute path or a parent traversal would let a
    # config file point the framework at an executable outside the repository.
    case "$COMMAND_REL" in
      /* | *..*)
        fail "testDiscovery.command must be a repo-relative path without '..' (got '$COMMAND_REL')"
        ;;
    esac
    [ -e "$REPO_ROOT/$COMMAND_REL" ] || [ -L "$REPO_ROOT/$COMMAND_REL" ] ||
      fail "testDiscovery.command '$COMMAND_REL' not found at $REPO_ROOT/$COMMAND_REL"
    [ -x "$REPO_ROOT/$COMMAND_REL" ] ||
      fail "testDiscovery.command '$COMMAND_REL' is not executable"
    COMMAND_APPROVAL="$(approve_command "$COMMAND_REL")" || {
      fail "testDiscovery.command '$COMMAND_REL' could not be approved: every intermediate and final component must be a non-symlink stable path"
    }
    IFS="$(printf '\t')" read -r COMMAND_ABS COMMAND_REL_APPROVED COMMAND_DEVICE COMMAND_INODE COMMAND_MODE COMMAND_SIZE COMMAND_MTIME_NS COMMAND_SHA256 <<EOF
$COMMAND_APPROVAL
EOF
    [ -n "$TIMEOUT" ] || TIMEOUT="$DEFAULT_TIMEOUT"
    case "$TIMEOUT" in
      '' | *[!0-9]*)
        fail "testDiscovery.timeoutSeconds must be a positive integer (got '$TIMEOUT')"
        ;;
    esac
    [ "$TIMEOUT" -gt 0 ] ||
      fail "testDiscovery.timeoutSeconds must be greater than zero"
    echo "command=$COMMAND_ABS"
    echo "commandRelative=$COMMAND_REL_APPROVED"
    echo "commandDevice=$COMMAND_DEVICE"
    echo "commandInode=$COMMAND_INODE"
    echo "commandMode=$COMMAND_MODE"
    echo "commandSize=$COMMAND_SIZE"
    echo "commandMtimeNs=$COMMAND_MTIME_NS"
    echo "commandSha256=$COMMAND_SHA256"
    echo "timeoutSeconds=$TIMEOUT"
    ;;
  *)
    fail "unknown testDiscovery.adapter '$ADAPTER' (expected none or command)"
    ;;
esac

echo "repoRoot=$REPO_ROOT"
exit 0
