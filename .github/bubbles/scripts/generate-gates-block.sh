#!/usr/bin/env bash
#
# bubbles generate-gates-block.sh — gate registry → workflows.yaml generator
# (v5.2 / F4).
#
# Canonical source: bubbles/registry/gates.yaml
# Generated target: the `gates:` block in bubbles/workflows.yaml
#                   (lines from `gates:` through the blank line BEFORE the
#                   next top-level key, currently `outcomeStates:`)
#
# Design: registry IS the canonical YAML form (verbatim copy of the gates
# block). Generator splices registry contents into workflows.yaml between
# the two markers. Round-trip is byte-identical by construction.
#
# Usage:
#   generate-gates-block.sh              # write workflows.yaml in place
#   generate-gates-block.sh --check      # exit 0 if workflows.yaml matches
#                                        # the registry; exit 1 if drifted
#   generate-gates-block.sh --print      # emit regenerated workflows.yaml
#                                        # to stdout (do not write)
#
# Exit codes:
#   0 — success (write/check passed/print emitted)
#   1 — drift detected in --check mode
#   2 — usage error or missing input

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REGISTRY="$REPO_ROOT/bubbles/registry/gates.yaml"
WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"

MODE="write"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="check"; shift;;
    --print) MODE="print"; shift;;
    -h|--help)
      sed -n '2,22p' "$0"
      exit 0
      ;;
    *) echo "generate-gates-block: unknown arg: $1" >&2; exit 2;;
  esac
done

[[ -f "$REGISTRY" ]] || {
  if [[ "$MODE" == "check" ]]; then
    # Downstream repos that installed v5.2.0 (before the v5.2.1 installer
    # fix) won't have the registry file. Emit SKIP rather than FAIL so
    # framework-validate stays green until the next install.sh run.
    echo "generate-gates-block: SKIP (bubbles/registry/gates.yaml missing — re-run install.sh to upgrade past v5.2.0)"
    exit 0
  fi
  echo "generate-gates-block: registry missing at $REGISTRY" >&2
  exit 2
}
[[ -f "$WORKFLOWS" ]] || { echo "generate-gates-block: workflows.yaml missing at $WORKFLOWS" >&2; exit 2; }

# Generated content is composed by:
#   1. Lines BEFORE the gates: line in workflows.yaml  (preamble, comments)
#   2. The registry file verbatim (which starts with `gates:`)
#   3. Lines FROM the blank line that follows the gates block onward
#
# The boundary "end of gates block" is the FIRST blank line whose NEXT
# non-blank line starts with a top-level key (i.e., not indented).

python3 - "$REGISTRY" "$WORKFLOWS" "$MODE" <<'PY'
import sys
from pathlib import Path

registry_path, workflows_path, mode = sys.argv[1], sys.argv[2], sys.argv[3]
registry = Path(registry_path).read_text()
current = Path(workflows_path).read_text()

# Find gates block boundaries in current workflows.yaml.
lines = current.splitlines(keepends=True)

# Find start: first line equal to 'gates:\n'.
start = None
for i, line in enumerate(lines):
    if line.rstrip('\n') == 'gates:':
        start = i
        break
if start is None:
    print("generate-gates-block: workflows.yaml has no 'gates:' top-level key", file=sys.stderr)
    sys.exit(2)

# Find end: walk forward until we hit a blank line whose NEXT non-blank line
# is either a top-level key OR a top-level comment (starts at column 0).
# Indented content (gate entries themselves) keeps us inside the block.
end = None
i = start + 1
while i < len(lines):
    if lines[i].strip() == '':
        # Lookahead for next non-blank.
        j = i + 1
        while j < len(lines) and lines[j].strip() == '':
            j += 1
        if j < len(lines):
            nxt = lines[j]
            # Top-level key OR top-level comment: starts at column 0
            # (not indented). Indented lines mean we're still inside gates.
            if nxt and not nxt.startswith(' ') and not nxt.startswith('\t'):
                end = i  # blank line BEFORE the next top-level thing
                break
    i += 1
if end is None:
    print("generate-gates-block: could not locate end of gates block", file=sys.stderr)
    sys.exit(2)

# Compose: preamble + registry verbatim + tail starting at the blank line.
preamble = ''.join(lines[:start])
tail = ''.join(lines[end:])

# Registry should start with 'gates:' line and end with no extra newline
# beyond the natural file ending. Normalize to exactly one trailing newline.
reg = registry
if not reg.endswith('\n'):
    reg += '\n'

new_content = preamble + reg + tail

if mode == 'print':
    sys.stdout.write(new_content)
    sys.exit(0)

if mode == 'check':
    if new_content == current:
        print(f"generate-gates-block: workflows.yaml is in sync with registry ({len(reg.splitlines())} registry lines)")
        sys.exit(0)
    print("generate-gates-block: DRIFT — workflows.yaml gates block does not match bubbles/registry/gates.yaml", file=sys.stderr)
    print("  Run: bash bubbles/scripts/generate-gates-block.sh", file=sys.stderr)
    sys.exit(1)

# write mode
if new_content == current:
    print(f"generate-gates-block: no change ({len(reg.splitlines())} registry lines)")
    sys.exit(0)
Path(workflows_path).write_text(new_content)
print(f"generate-gates-block: updated workflows.yaml gates block ({len(reg.splitlines())} registry lines)")
PY
