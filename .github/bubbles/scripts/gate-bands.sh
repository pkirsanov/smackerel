#!/usr/bin/env bash
#
# gate-bands.sh — generate the framework's gate-ID band strings from the
# registry (IMP-027 / SCOPE-2d).
#
# WHY THIS EXISTS
# ---------------
# Two prose surfaces stated the framework's gate bands by hand:
#
#   bubbles/workflows.yaml header      "currently G001-G095 plus G110-G125"
#   docs/recipes/custom-gates.md       the same sentence
#
# Both were wrong — the registry's highest gate is G131, and the real ID set is
# fragmented across eleven runs, not two. A downstream author reading either
# line would have been told the framework stops well below where it actually
# does.
#
# Worse, `customGatesDiscovery.idRange` advertised `G100+` for PROJECT-LOCAL
# gates while the framework itself occupies G110-G131. Anyone who followed it
# would pick an ID the next upgrade overwrites. Nothing read that key, so
# nothing caught the collision.
#
# The bands are now derived here and spliced into both surfaces between
# GENERATED markers, so neither can drift from the registry again.
#
# Usage:
#   gate-bands.sh            write the band strings into both surfaces
#   gate-bands.sh --check    exit 1 if either surface is stale
#   gate-bands.sh --print    print the computed band string
#
# Exit codes: 0 ok - 1 drift (--check) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODE="write"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      MODE="check"
      shift
      ;;
    --print)
      MODE="print"
      shift
      ;;
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    -h | --help)
      sed -n '2,31p' "$0"
      exit 0
      ;;
    *)
      echo "gate-bands: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
if [[ ! -f "$GATES" ]]; then
  echo "gate-bands: SKIP (bubbles/registry/gates.yaml missing)"
  exit 0
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "gate-bands: SKIP (python3 not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$MODE" <<'PY'
import re
import sys
from pathlib import Path

repo_root = Path(sys.argv[1])
mode = sys.argv[2]

gates_text = (repo_root / "bubbles/registry/gates.yaml").read_text()
ids = sorted({int(m) for m in re.findall(r"^  G(\d{3}):", gates_text, re.M)})
if not ids:
    print("gate-bands: no gate ids found in the registry", file=sys.stderr)
    sys.exit(2)

# Collapse the id list into contiguous runs. The gaps are real (retired and
# absorbed gates), so the honest statement enumerates the runs rather than
# implying one unbroken span.
runs = []
start = prev = ids[0]
for n in ids[1:]:
    if n == prev + 1:
        prev = n
        continue
    runs.append((start, prev))
    start = prev = n
runs.append((start, prev))

band_str = " plus ".join(
    f"G{a:03d}" if a == b else f"G{a:03d}-G{b:03d}" for a, b in runs
)

if mode == "print":
    print(band_str)
    sys.exit(0)

targets = [
    (
        repo_root / "bubbles/workflows.yaml",
        "# GENERATED:GATE_BANDS_START",
        "# GENERATED:GATE_BANDS_END",
        f"# Gate-ID bands: the framework RESERVES G001-G199 (currently {band_str}).",
    ),
    (
        repo_root / "docs/recipes/custom-gates.md",
        "<!-- GENERATED:GATE_BANDS_START",
        "<!-- GENERATED:GATE_BANDS_END -->",
        "- Framework (built-in) gates: **G001\u2013G199 reserved**. Active IDs are listed in "
        f"`bubbles/registry/gates.yaml` (the framework currently uses {band_str}).",
    ),
]

stale = []
for path, start_marker, end_marker, body in targets:
    if not path.is_file():
        continue
    text = path.read_text()
    lines = text.splitlines(keepends=True)
    s = e = None
    for i, line in enumerate(lines):
        if s is None and line.startswith(start_marker):
            s = i
        elif s is not None and line.startswith(end_marker):
            e = i
            break
    if s is None or e is None:
        print(f"gate-bands: markers not found in {path.relative_to(repo_root)}", file=sys.stderr)
        sys.exit(2)

    current = "".join(lines[s + 1:e])
    desired = body + "\n"
    if current == desired:
        continue
    if mode == "check":
        stale.append(str(path.relative_to(repo_root)))
        continue
    path.write_text("".join(lines[: s + 1]) + desired + "".join(lines[e:]))

if mode == "check":
    if stale:
        print("gate-bands: DRIFT — stale band strings in: " + ", ".join(stale))
        print("  Run: bash bubbles/scripts/gate-bands.sh")
        sys.exit(1)
    print(f"gate-bands: band strings are current ({band_str})")
    sys.exit(0)

print(f"gate-bands: wrote band strings ({band_str})")
sys.exit(0)
PY
